package app

import (
	"context"
	"time"

	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/core"
)

// ModelChoice is one model /model can switch to, as the interface's
// picker shows it (/model with nothing after it, as Claude Code's).
type ModelChoice struct {
	// Ref is the model as /model takes it: provider/model.
	Ref string
	// Window is its context window in tokens, where it is known.
	Window int
	// Levels are its thinking levels, and Level the one it thinks at: the
	// one in use for the model in use, and for another the one a switch
	// to it with no level would take (the one saved for it, or the
	// default: "on" of an on/off switch, none of a dial).
	Levels []string
	Level  string
	// Current marks the model in use.
	Current bool
	// State is whether the server has it loaded, as a server that loads
	// and unloads models says it (core.ModelInfo.State): a switch to one
	// "unloaded" waits while the server loads it.
	State string
}

// stateWait is how long a server is given to say which models it has
// loaded, when the picker opens.
const stateWait = 5 * time.Second

// ModelChoices lists the models /model can switch to, the one in use
// marked. It asks the providers' servers which they have, within ctx.
func (c *controls) ModelChoices(ctx context.Context) []ModelChoice { return c.cur().modelChoices(ctx) }

// modelChoices is SessionControls.ModelChoices: what /model lists, as
// choices.
func (l *live) modelChoices(ctx context.Context) []ModelChoice {
	l.switchMu.Lock()
	defer l.switchMu.Unlock()
	cur, has := l.modelRef()
	level, window := "", 0
	if l.agent != nil {
		m := l.agent.Model()
		level, window = m.Level, m.Info.ContextWindow
	}
	var saved map[string]string
	if st, probs := config.ReadState(config.OS{}, l.cfg.Paths.StateFile); len(probs) == 0 {
		saved = st.Thinking
	}
	var out []ModelChoice
	for _, pc := range l.cfg.Usable() {
		ids, _ := l.offered(ctx, pc)
		listed := l.serverSays(ctx, pc)
		for _, id := range ids {
			ref := config.ModelRef{Provider: pc.ID, ID: id}
			ch := ModelChoice{Ref: ref.String(), Levels: thinkingLevelsOf(l.cfg, pc, id), Level: saved[ref.String()]}
			if m, ok := listed[id]; ok {
				ch.State, ch.Window = m.State, m.ContextWindow
			}
			if cm, ok := l.cfg.Model(ref); ok && cm.ContextWindow.Set {
				ch.Window = cm.ContextWindow.V
			}
			if has && cur.Provider == pc.ID && cur.ID == id {
				ch.Current, ch.Level = true, level
				if window > 0 {
					ch.Window = window
				}
			} else if ch.Level == "" {
				ch.Level, _ = thinkingLevel(ch.Levels, config.Value[string]{})
			}
			out = append(out, ch)
		}
	}
	return out
}

// serverSays asks a provider's server, afresh, what it says of its models
// now (which are loaded changes as a server swaps them), by id. It is
// empty where the server cannot be asked or does not answer in time.
func (l *live) serverSays(ctx context.Context, pc config.Provider) map[string]core.ModelInfo {
	pu, err := l.providerFor(ctx, pc.ID)
	if err != nil {
		return nil
	}
	lister, ok := pu.prov.(core.ModelLister)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, stateWait)
	defer cancel()
	models, err := lister.Models(ctx)
	if err != nil {
		return nil
	}
	out := make(map[string]core.ModelInfo, len(models))
	for _, m := range models {
		out[m.ID] = m
	}
	return out
}

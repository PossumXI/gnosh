package app

import (
	"context"

	"github.com/gnosh-ai/gnosh/internal/config"
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
}

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
		for _, id := range ids {
			ref := config.ModelRef{Provider: pc.ID, ID: id}
			ch := ModelChoice{Ref: ref.String(), Levels: thinkingLevelsOf(l.cfg, pc, id), Level: saved[ref.String()]}
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

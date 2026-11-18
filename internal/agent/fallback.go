package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// A Fallback is a model a turn goes on with when the session's model
// cannot answer (Options.Fallbacks). Name is how it is written in messages
// before it is made. Make makes it ready, its provider's client and, where
// it can be learned, its window; it is called only once the fallback is
// needed, so that a fallback whose server is down costs nothing until then.
// A Make that fails passes the fallback over for the rest of the turn.
type Fallback struct {
	Name string
	Make func(ctx context.Context) (Model, error)
}

// errFellBack says a request went unanswered and the turn now goes on with
// a fallback model: the request is to be made again, for that model.
var errFellBack = errors.New("agent: the turn goes on with a fallback model")

// fallsBack says whether a failure is one a fallback model is for: the
// model's server cannot be reached, says it is not ready or too busy, or
// does not have the model. A limit on the person's requests, a key that is
// refused, and a request that is too long or that no model would take are
// not: another model would meet them too, or they are the person's to
// settle. gaveUp says the wait for the model ran out, which hands any other
// failure on as well.
func fallsBack(pe *core.ProviderError, gaveUp bool) bool {
	switch {
	case pe.Kind == core.ErrRateLimit, pe.Kind == core.ErrAuth, pe.Kind == core.ErrContextTooLong:
		return false
	case pe.Kind == core.ErrInvalidRequest:
		return pe.Status == 404 // no such model on that server
	case pe.Kind == core.ErrOverloaded, sortTrouble(pe) == troubleUnreachable:
		return true
	}
	return gaveUp
}

// fellBack moves the turn to a fallback model when the failure pe of the
// model m is one a fallback is for (gaveUp: the wait for m ran out), and
// says so with a RetryEvent that names it. It reports whether it moved.
func (a *Agent) fellBack(ctx context.Context, t *turn, purpose string, m Model, pe *core.ProviderError, gaveUp bool, rep *reply) bool {
	if !fallsBack(pe, gaveUp) {
		return false
	}
	to, ok := a.fallTo(ctx, t, m)
	if !ok {
		return false
	}
	a.mu.Lock()
	primary := a.model
	a.mu.Unlock()
	next := "the next turn asks " + primary.label() + " first again"
	if modelKey(m) != modelKey(primary) {
		next = "the next turn asks " + primary.label() + " first"
	}
	a.emit(&RetryEvent{
		Purpose: purpose, Attempt: 1, Kind: pe.Kind, Status: pe.Status, Discarded: !rep.empty(), FellBack: to.label(),
		Message: fmt.Sprintf("%s %s. This turn goes on with the fallback model %s; %s.", m.label(), whatHappened(pe), to.label(), next),
	})
	return true
}

// fallTo moves the turn to the first fallback that is not the model that
// failed, nor one that failed earlier in the turn, nor one whose window is
// smaller than the session model's (the conversation would be cut to fit
// it), and that can be made, and returns it. Nothing moves outside a turn:
// a switch holds for the rest of a turn, and there is none to hold it.
func (a *Agent) fallTo(ctx context.Context, t *turn, from Model) (Model, bool) {
	if t == nil || len(a.opts.Fallbacks) == 0 {
		return Model{}, false
	}
	a.mu.Lock()
	primary := a.model
	if a.fellOut == nil {
		a.fellOut = map[string]bool{}
	}
	a.fellOut[modelKey(from)] = true
	if a.fell != nil && modelKey(*a.fell) == modelKey(from) {
		a.fellOut[a.fellName] = true
	}
	a.mu.Unlock()
	for _, fb := range a.opts.Fallbacks {
		a.mu.Lock()
		out := a.fellOut[fb.Name]
		a.mu.Unlock()
		if out || fb.Make == nil {
			continue
		}
		m, err := fb.Make(ctx)
		if err == nil && m.Provider == nil {
			err = errors.New("it has no provider")
		}
		if err != nil {
			if ctx.Err() != nil {
				return Model{}, false
			}
			a.mu.Lock()
			a.fellOut[fb.Name] = true
			a.mu.Unlock()
			a.notice(LevelWarning, "provider", fmt.Sprintf("The fallback model %s cannot be used: %v.", fb.Name, err))
			continue
		}
		a.mu.Lock()
		out = a.fellOut[modelKey(m)]
		a.mu.Unlock()
		if out || modelKey(m) == modelKey(from) {
			continue
		}
		if from.Local && (m.Provider.Name() == from.Provider.Name() || m.Address != "" && m.Address == from.Address) {
			// A model server of one's own that is down or busy is no help
			// for another of its models, and one that holds a model at a
			// time would unload the one in use to load it.
			a.mu.Lock()
			a.fellOut[fb.Name] = true
			a.mu.Unlock()
			a.notice(LevelWarning, "provider", fmt.Sprintf(
				"The fallback model %s is passed over: it is on the same server as %s, which would be no help and could unload the model in use.",
				fb.Name, from.label()))
			continue
		}
		if w, pw := m.Info.ContextWindow, primary.Info.ContextWindow; w > 0 && pw > 0 && w < pw {
			a.mu.Lock()
			a.fellOut[fb.Name] = true
			a.mu.Unlock()
			a.notice(LevelWarning, "provider", fmt.Sprintf(
				"The fallback model %s is passed over: its context window (%s tokens) is smaller than that of %s (%s), and the conversation would have to be cut to fit it.",
				fb.Name, tokens(w), primary.label(), tokens(pw)))
			continue
		}
		// The level of thinking chosen for the session holds where the
		// fallback has it.
		if primary.Level != "" && slices.Contains(m.Levels, primary.Level) {
			m.Level = primary.Level
		}
		if checkLevel(m) != nil {
			m.Level = ""
		}
		a.mu.Lock()
		a.fell, a.fellName = &m, fb.Name
		a.mu.Unlock()
		return m, true
	}
	return Model{}, false
}

// fellBackEnds puts the session's own model back in force, at the end of a
// turn.
func (a *Agent) fellBackEnds() {
	a.mu.Lock()
	a.fell, a.fellName, a.fellOut = nil, "", nil
	a.mu.Unlock()
}

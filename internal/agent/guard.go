package agent

import (
	"fmt"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// RepeatPolicy tunes the guard that notices a model repeating itself. The
// guard never ends a turn: it tells the model what it keeps doing, and the
// turn goes on. The zero value gives the defaults.
type RepeatPolicy struct {
	// Window is how many of the latest rounds are looked at (ten), and
	// Over how many of them may be the same before the model is told
	// (five: the sixth tells it).
	Window int
	Over   int

	// Key returns what is compared for one call and its result. Rounds
	// are the same when the keys of their calls are. Nil compares the
	// tool, the arguments and the result's text as they are; the wiring
	// gives a function that blanks the numbers in a result for tools
	// whose every answer ends in a duration or a counter.
	Key func(call core.ToolCall, res core.ToolResult) string

	// Nudge may return its own words for a repetition, for a case that
	// deserves them: a close that mem keeps refusing is told that the
	// work is still to do. Returning "" leaves the words to the loop.
	// What it returns is stored as given.
	Nudge func(rep Repetition) string
}

// Repetition is what the guard found.
type Repetition struct {
	// Rounds is how many of the last Window rounds were the same as the
	// newest one, and Window how many rounds were looked at.
	Rounds int
	Window int
	// Calls are the calls of the newest round with their results.
	Calls []RepeatedCall
	// Other holds the calls of the other round when two rounds took
	// turns, and is nil when one round was repeated.
	Other []RepeatedCall
}

// RepeatedCall is one call of a repeated round, with its result.
type RepeatedCall struct {
	// Call is the call as it ran and Result what it returned.
	Call   core.ToolCall
	Result core.ToolResult
}

// repeatGuard remembers the latest rounds of a turn.
type repeatGuard struct {
	sigs   []string
	rounds [][]RepeatedCall
}

// reset forgets everything, so that counting starts afresh.
func (g *repeatGuard) reset() {
	g.sigs, g.rounds = nil, nil
}

// defaultRepeatKey compares a call and its result as they are.
func defaultRepeatKey(call core.ToolCall, res core.ToolResult) string {
	failed := "ok"
	if res.IsError {
		failed = "error"
	}
	return call.Name + "\x00" + string(call.Input) + "\x00" + failed + "\x00" + core.TextOf(res.Content)
}

// checkRepeat looks at the round that just ended. It is only ever called
// after a round the model will be asked again after, so a final answer is
// never replaced by a nudge and a turn that was ended is never started
// again by one. When the model is found to repeat itself it is told so,
// with the call quoted, and the counting starts afresh; there is no limit
// on how often that may happen.
func (a *Agent) checkRepeat(t *turn, ro roundOutcome) error {
	window, over := a.opts.Repeat.Window, a.opts.Repeat.Over
	if window <= 0 {
		window = 10
	}
	if over <= 0 {
		over = 5
	}
	key := a.opts.Repeat.Key
	if key == nil {
		key = defaultRepeatKey
	}

	var sig strings.Builder
	calls := make([]RepeatedCall, 0, len(ro.calls))
	for _, c := range ro.calls {
		call := core.ToolCall{ID: c.call.ID, Name: c.call.Name, Input: c.input}
		calls = append(calls, RepeatedCall{Call: call, Result: c.res})
		sig.WriteString(key(call, c.res))
		sig.WriteString("\x01")
	}
	g := &t.guard
	g.sigs = append(g.sigs, sig.String())
	g.rounds = append(g.rounds, calls)
	if len(g.sigs) > window {
		g.sigs = g.sigs[len(g.sigs)-window:]
		g.rounds = g.rounds[len(g.rounds)-window:]
	}

	newest := g.sigs[len(g.sigs)-1]
	same := 0
	otherAt := -1
	distinct := map[string]bool{}
	for i, s := range g.sigs {
		distinct[s] = true
		if s == newest {
			same++
		} else {
			otherAt = i
		}
	}

	var rep Repetition
	switch {
	case same > over:
		rep = Repetition{Rounds: same, Window: len(g.sigs), Calls: calls}
	case len(g.sigs) == window && len(distinct) == 2:
		// Two rounds taking turns each fill half the window, under the
		// bar; a full window with only two kinds of round in it is a
		// loop all the same.
		rep = Repetition{Rounds: same, Window: len(g.sigs), Calls: calls, Other: g.rounds[otherAt]}
	default:
		return nil
	}
	g.reset()

	text := ""
	if f := a.opts.Repeat.Nudge; f != nil {
		text = f(rep)
	}
	if text == "" {
		if rep.Other == nil {
			text = a.mark(fmt.Sprintf(a.texts.RepeatNudge, rep.Rounds, rep.Window, quoteCalls(rep.Calls)))
		} else {
			text = a.mark(fmt.Sprintf(a.texts.RepeatPair, rep.Window, quoteCalls(rep.Calls)+"\n\nand\n\n"+quoteCalls(rep.Other)))
		}
	}
	return a.ownPrompt(t, "nudge:repeat", text)
}

// quoteCalls writes calls and their results so that a model can see what
// is meant.
func quoteCalls(calls []RepeatedCall) string {
	var b strings.Builder
	for i, c := range calls {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s %s\n", c.Call.Name, clip(string(c.Call.Input), 300))
		result := strings.TrimSpace(core.TextOf(c.Result.Content))
		if result == "" {
			result = "(no text)"
		}
		if c.Result.IsError {
			b.WriteString("failed: ")
		} else {
			b.WriteString("returned: ")
		}
		b.WriteString(clip(result, 600))
	}
	return b.String()
}

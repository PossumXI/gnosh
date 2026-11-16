package ui

import (
	"context"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
	"github.com/gnosh-ai/gnosh/internal/ui/menu"
)

// /model with nothing after it opens a picker, as Claude Code's does and
// the earlier gnosh's did (Jae, 16 November 2026: "the modal doesn't seem
// to pop up when i type ssh ai and /model"): the models there are, the
// one in use marked and the cursor on it; Enter takes the one picked, and
// a model with thinking levels then asks which; Esc keeps the model as it
// is. The switch itself is the command /model provider/model [level],
// which saves the choice, as typing it does.

// modelChooser is a session whose controls list the models /model can
// switch to (app.SessionControls).
type modelChooser interface {
	ModelChoices(ctx context.Context) []app.ModelChoice
}

// modelWords are the lines that open the picker.
var modelWords = map[string]bool{"/model": true}

// modelWait is how long the providers' servers are given to say which
// models they have.
const modelWait = 15 * time.Second

// chooseModel opens the picker, once the servers have said which models
// they have. line is what was typed, or "" for Alt+P, which leaves the
// prompt box and its history as they are.
func (s *session) chooseModel(line string) {
	mc, ok := s.c.(modelChooser)
	switch {
	case !ok && line == "":
		s.flash("Switching models is not joined to this build.")
		return
	case !ok:
		s.run(line)
		return
	case line != "":
		s.ed.AddHistory(editor.Draft{Text: line})
	}
	s.flash("Asking the model servers which models they have…")
	s.host.spawn(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), modelWait)
		defer cancel()
		choices := mc.ModelChoices(ctx)
		return func() { s.modelMenu(choices) }
	})
}

// modelMenu puts the picker on the screen.
func (s *session) modelMenu(choices []app.ModelChoice) {
	s.hint = ""
	if s.q != nil {
		// A question came meanwhile: it goes first.
		s.flash("A question came first: /model again opens the list of models.")
		return
	}
	if len(choices) == 0 {
		// The command's own words say why there is none.
		s.run("/model")
		return
	}
	var opts []menu.Option
	sel := 0
	for i, ch := range choices {
		var about []string
		if ch.Current {
			about = append(about, "in use")
			sel = i
		}
		if ch.Window > 0 {
			about = append(about, "window "+tokens(ch.Window))
		}
		switch {
		case len(ch.Levels) > 0 && ch.Level != "":
			about = append(about, "thinking "+ch.Level+" (of "+strings.Join(ch.Levels, ", ")+")")
		case len(ch.Levels) > 0:
			about = append(about, "thinking: "+strings.Join(ch.Levels, ", "))
		}
		switch {
		case ch.Current:
		case ch.State == "unloaded":
			// (A server that holds one model at a time puts the other out
			// to load this one: the first answer waits for that.)
			about = append(about, "not loaded: the server loads it first, which takes a while")
		case ch.State == "loading":
			about = append(about, "loading now")
		}
		opts = append(opts, menu.Option{Label: oneLine(ch.Ref), Description: strings.Join(about, s.th.Sep)})
	}
	opts = append(opts, menu.Option{Label: "Keep the model as it is"})
	pick := &question{q: agent.Question{Kind: "model", Title: "Which model? The choice is saved for later sessions."}}
	pick.ui = menu.NewSelect(menu.SelectOptions{Title: pick.q.Title, Options: opts, Selected: sel, CancelHint: "keep it", Theme: s.th.Menu, Metrics: s.m})
	pick.local = func(res menu.Result) {
		if res.Cancelled || res.Index < 0 || res.Index >= len(choices) {
			return
		}
		ch := choices[res.Index]
		switch {
		case len(ch.Levels) > 0:
			s.levelMenu(ch)
		case ch.Current:
			s.flash(ch.Ref + " is the model in use already.")
		default:
			s.run("/model " + ch.Ref)
		}
	}
	s.openLocal(pick)
}

// levelMenu asks how the model picked is to think.
func (s *session) levelMenu(ch app.ModelChoice) {
	var opts []menu.Option
	sel := 0
	for i, lv := range ch.Levels {
		o := menu.Option{Label: lv}
		if lv == ch.Level {
			o.Description = "as it is now"
			if !ch.Current {
				o.Description = "what it would use"
			}
			sel = i
		}
		opts = append(opts, o)
	}
	opts = append(opts, menu.Option{Label: "Never mind"})
	pick := &question{q: agent.Question{Kind: "model", Title: "How is " + oneLine(ch.Ref) + " to think?"}}
	pick.ui = menu.NewSelect(menu.SelectOptions{Title: pick.q.Title, Options: opts, Selected: sel, CancelHint: "never mind", Theme: s.th.Menu, Metrics: s.m})
	pick.local = func(res menu.Result) {
		if res.Cancelled || res.Index < 0 || res.Index >= len(ch.Levels) {
			return
		}
		lv := ch.Levels[res.Index]
		if ch.Current && lv == ch.Level {
			s.flash(ch.Ref + " is the model in use already, thinking " + lv + ".")
			return
		}
		s.run("/model " + ch.Ref + " " + lv)
	}
	s.openLocal(pick)
}

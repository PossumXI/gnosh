package ui

import (
	"slices"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
	"github.com/gnosh-ai/gnosh/internal/ui/menu"
)

// How much the model thinks, by picker and by key, as Claude Code has
// them: /effort with nothing after it lists the model's thinking levels,
// the one in force marked, and Alt+T turns thinking over (on and off for
// a model with that switch; the next level for one with a dial). Each
// change is the command /effort and the level, which keeps it for later
// sessions, as typing it does.

// effortTeller is a session whose controls say the model's thinking
// levels (app.SessionControls).
type effortTeller interface {
	Effort() (levels []string, level string)
}

// effortWords are the lines that open the picker.
var effortWords = map[string]bool{"/effort": true}

// chooseEffort opens the picker, where the model has thinking levels.
func (s *session) chooseEffort(line string) bool {
	et, ok := s.c.(effortTeller)
	if !ok {
		return false
	}
	levels, level := et.Effort()
	if len(levels) == 0 {
		// (The command says how to declare them.)
		return false
	}
	s.ed.AddHistory(editor.Draft{Text: line})
	choices := append(slices.Clone(levels), "auto")
	var opts []menu.Option
	sel := len(levels)
	for i, c := range choices {
		o := menu.Option{Label: c}
		switch {
		case c == "auto" && level == "":
			o.Description, sel = "the model's own default, as it is now", i
		case c == "auto":
			o.Description = "the model's own default"
		case c == level:
			o.Description, sel = "as it is now", i
		}
		opts = append(opts, o)
	}
	opts = append(opts, menu.Option{Label: "Never mind"})
	pick := &question{q: agent.Question{Kind: "effort", Title: "How much is the model to think? It is kept for later sessions."}}
	pick.ui = menu.NewSelect(menu.SelectOptions{Title: pick.q.Title, Options: opts, Selected: sel, CancelHint: "never mind", Theme: s.th.Menu, Metrics: s.m})
	pick.local = func(res menu.Result) {
		if res.Cancelled || res.Index < 0 || res.Index >= len(choices) || res.Index == sel {
			return
		}
		s.run("/effort " + choices[res.Index])
	}
	s.openLocal(pick)
	return true
}

// toggleThinking is Alt+T: the model thinks the other way from now on.
func (s *session) toggleThinking() {
	et, ok := s.c.(effortTeller)
	if !ok {
		s.flash("Turning thinking over (Alt+T) is not joined to this build.")
		return
	}
	next := nextThinking(et.Effort())
	if next == "" {
		s.flash("gnosh knows of no thinking levels for this model: /effort says how to declare them.")
		return
	}
	s.run("/effort " + next)
}

// nextThinking is the level Alt+T goes to from level: off and on turn
// over (a model with that switch thinks when nothing chose); a dial goes
// to its next level, and from its last to its first.
func nextThinking(levels []string, level string) string {
	if len(levels) == 0 {
		return ""
	}
	if slices.Contains(levels, "off") && slices.Contains(levels, "on") {
		if level == "off" {
			return "on"
		}
		return "off"
	}
	return levels[(slices.Index(levels, level)+1)%len(levels)]
}

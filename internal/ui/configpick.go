package ui

import (
	"encoding/json"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
	"github.com/gnosh-ai/gnosh/internal/ui/menu"
)

// /config with nothing after it opens a picker of gnosh's options, as
// Claude Code's opens its settings: each option with its value and where
// that comes from. Enter on one that is true or false turns it over; one
// that takes one of a few words asks which; any other asks for its value,
// the one in force to begin with. The change is the command /config
// key=value, which keeps it for every project from gnosh's next start
// (and puts the few that can be in force at once), as typing it does.

// configChooser is a session whose controls list gnosh's options
// (app.SessionControls).
type configChooser interface {
	ConfigChoices() []app.ConfigChoice
}

// configWords are the lines that open the picker.
var configWords = map[string]bool{"/config": true}

// chooseConfig opens the picker.
func (s *session) chooseConfig(line string) {
	cc, ok := s.c.(configChooser)
	choices := []app.ConfigChoice(nil)
	if ok {
		choices = cc.ConfigChoices()
	}
	if len(choices) == 0 {
		s.run(line)
		return
	}
	s.ed.AddHistory(editor.Draft{Text: line})
	keyW := 0
	for _, ch := range choices {
		keyW = max(keyW, len(ch.Key))
	}
	var opts []menu.Option
	for _, ch := range choices {
		opts = append(opts, menu.Option{
			Label:       ch.Key + strings.Repeat(" ", keyW-len(ch.Key)+2) + oneLine(ch.Value),
			Description: oneLine(ch.What) + s.th.Sep + oneLine(ch.Origin),
		})
	}
	opts = append(opts, menu.Option{Label: "Done"})
	pick := &question{q: agent.Question{Kind: "config", Title: "gnosh's options. Enter changes one; it is kept for every project."}}
	pick.ui = menu.NewSelect(menu.SelectOptions{Title: pick.q.Title, Options: opts, CancelHint: "done", Theme: s.th.Menu, Metrics: s.m})
	pick.local = func(res menu.Result) {
		if res.Cancelled || res.Index < 0 || res.Index >= len(choices) {
			return
		}
		s.changeOption(choices[res.Index])
	}
	s.openLocal(pick)
}

// changeOption asks how an option is to change, and changes it.
func (s *session) changeOption(ch app.ConfigChoice) {
	set := func(value string) {
		if strings.ContainsAny(value, " \t\"'") {
			// (A value with a blank or a quote in it goes as a quoted
			// string, which /config reads as one.)
			q, _ := json.Marshal(value)
			value = string(q)
		}
		s.run("/config " + ch.Key + "=" + value)
	}
	switch {
	case ch.List:
		s.flash(ch.Key + " holds a list, which is set in gnosh.json.")
	case len(ch.Choices) == 2 && ch.Choices[0] == "true" && ch.Choices[1] == "false":
		if ch.Value == "true" {
			set("false")
		} else {
			set("true")
		}
	case len(ch.Choices) > 0:
		var opts []menu.Option
		sel := 0
		for i, c := range ch.Choices {
			o := menu.Option{Label: c}
			if c == ch.Value {
				o.Description, sel = "as it is now", i
			}
			opts = append(opts, o)
		}
		opts = append(opts, menu.Option{Label: "Never mind"})
		pick := &question{q: agent.Question{Kind: "config", Title: ch.Key + ": " + oneLine(ch.What) + "?"}}
		pick.ui = menu.NewSelect(menu.SelectOptions{Title: pick.q.Title, Options: opts, Selected: sel, CancelHint: "never mind", Theme: s.th.Menu, Metrics: s.m})
		pick.local = func(res menu.Result) {
			if res.Cancelled || res.Index < 0 || res.Index >= len(ch.Choices) || ch.Choices[res.Index] == ch.Value {
				return
			}
			set(ch.Choices[res.Index])
		}
		s.openLocal(pick)
	default:
		initial := ""
		if ch.Set {
			initial = ch.Value
		}
		pick := &question{q: agent.Question{Kind: "config", Title: ch.Key + " takes " + oneLine(ch.Takes) + ". Enter sets it; with nothing, what /config set is taken away."}}
		pick.ui = menu.NewInput(menu.InputOptions{Title: pick.q.Title, Initial: initial, Placeholder: oneLine(ch.Value), AllowEmpty: true, Theme: s.th.Menu, Metrics: s.m})
		pick.local = func(res menu.Result) {
			if res.Cancelled {
				return
			}
			value := strings.TrimSpace(res.Text)
			if value == "" {
				s.run("/config " + ch.Key + "=")
				return
			}
			set(value)
		}
		s.openLocal(pick)
	}
}

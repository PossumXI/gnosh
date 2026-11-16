package ui

import (
	"slices"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
	"github.com/gnosh-ai/gnosh/internal/ui/menu"
)

// /export with nothing after it asks where the conversation goes, as
// Claude Code's does: the clipboard (through gnosh ssh, the Mac's too) or
// a file here named for the day and the session. What is picked runs as
// /export --clipboard or /export, as a command typed then would: queued
// while a turn runs.

// exportWords are the lines that open the picker.
var exportWords = map[string]bool{"/export": true}

// chooseExport opens the picker, where the session has the command.
func (s *session) chooseExport(line string) bool {
	if !slices.Contains(s.info.Commands, "export") {
		return false
	}
	s.ed.AddHistory(editor.Draft{Text: line})
	opts := []menu.Option{
		{Label: "Copy it to the clipboard"},
		{Label: "Save it to a file here", Description: "named for today and the session; /export name.txt names it"},
		{Label: "Never mind"},
	}
	pick := &question{q: agent.Question{Kind: "export", Title: "The conversation as plain text: where to?"}}
	pick.ui = menu.NewSelect(menu.SelectOptions{Title: pick.q.Title, Options: opts, CancelHint: "never mind", Theme: s.th.Menu, Metrics: s.m})
	pick.local = func(res menu.Result) {
		var line string
		switch {
		case res.Cancelled || res.Index < 0 || res.Index > 1:
			return
		case res.Index == 0:
			line = "/export --clipboard"
		default:
			line = "/export"
		}
		if s.busy {
			// As the command typed now would: it waits for the turn's end.
			s.cmdq = append(s.cmdq, line)
			s.queued()
			return
		}
		s.run(line)
	}
	s.openLocal(pick)
	return true
}

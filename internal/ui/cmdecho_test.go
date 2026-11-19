package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// A command of the session's is shown as typed when it starts, by its name
// or an alias: what is said while it runs (the notes of a summary it waits
// for, minutes on a local model) comes after it, and what it answers after
// those, as a block of its own. It was shown only once it had run, after
// its own notes (the box check of /rewind N summarize, 2026-11-19).
func TestCommandShownWhenItStarts(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor, func(c *controls) {
		c.info.Commands = append(c.info.Commands, "rewind")
		c.info.CommandList = append(c.info.CommandList, app.CommandInfo{Name: "rewind", Aliases: []string{"undo"}})
		c.command = func(ctx context.Context, line string) (app.CommandResult, error) {
			return app.CommandResult{Handled: true, Name: "rewind", Text: "BACK-TO-BEFORE"}, nil
		}
	})
	g.hold = true
	g.keys(`"/undo 2 summarize" <Enter>`)
	g.ev(note("SUMMARIZING-NOW"))
	g.release()
	scroll := flat(g.scroll())
	typed, said, back := strings.Index(scroll, "/undo 2 summarize"), strings.Index(scroll, "SUMMARIZING-NOW"), strings.Index(scroll, "BACK-TO-BEFORE")
	if typed < 0 || said < 0 || back < 0 || typed > said || said > back {
		t.Fatalf("the command, its note and its answer came at %d, %d, %d:\n%s", typed, said, back, scroll)
	}
	if n := strings.Count(scroll, "/undo 2 summarize"); n != 1 {
		t.Errorf("the command was shown %d times:\n%s", n, scroll)
	}
}

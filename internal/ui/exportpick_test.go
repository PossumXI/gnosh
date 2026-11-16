package ui

import (
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// /export with nothing after it asks where to: the clipboard runs
// /export --clipboard, a file /export; Esc runs nothing; with a name the
// command runs as typed; a session without the command is left alone.
func TestExportPicker(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor, func(c *controls) {
		c.info.Commands = append(c.info.Commands, "export")
	})
	ran := func() []string {
		g.c.mu.Lock()
		defer g.c.mu.Unlock()
		return slices.Clone(g.c.commands)
	}
	g.keys(`"/export" <Enter>`)
	g.liveHas("where to?", "Copy it to the clipboard", "Save it to a file here", "Never mind")
	g.keys(`<Enter>`)
	g.keys(`"/export" <Enter> <Down> <Enter>`)
	g.keys(`"/export" <Enter> <Esc>`)
	g.keys(`"/export notes.txt" <Enter>`)
	if got := ran(); !slices.Equal(got, []string{"/export --clipboard", "/export", "/export notes.txt"}) {
		t.Fatalf("ran %q", got)
	}

	h := newRig(t, 100, 24, text.NoColor)
	h.keys(`"/export" <Enter>`)
	if h.s.q != nil {
		t.Fatal("a session without /export opened the picker")
	}
}

package ui

import (
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// /memory with nothing after it lists the context files, and the one
// picked opens in the person's editor; Esc opens nothing; with no files
// listed, the command runs as before.
func TestMemoryPicker(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor, func(c *controls) {
		c.memFiles = []app.MemoryFile{
			{Path: "/home/jae/src/proj/GNOSH.md", About: "project, 1.2 KB"},
			{Path: "/home/jae/.config/gnosh/GNOSH.md", About: "yours, for every project, not there yet"},
		}
	})
	g.keys(`"/memory" <Enter>`)
	g.liveHas("Open which context file", "GNOSH.md", "project, 1.2 KB", "~/.config/gnosh/GNOSH.md", "not there yet", "Never mind")
	g.keys(`<Esc>`)
	if len(g.editedFiles) != 0 {
		t.Fatalf("Esc opened %q", g.editedFiles)
	}
	g.keys(`"/memory" <Enter> <Down> <Enter>`)
	if !slices.Equal(g.editedFiles, []string{"/home/jae/.config/gnosh/GNOSH.md"}) {
		t.Fatalf("opened %q", g.editedFiles)
	}
	g.liveHas("is as you left it")

	h := newRig(t, 100, 24, text.NoColor)
	h.keys(`"/memory" <Enter>`)
	if h.s.q != nil {
		t.Fatal("with no files listed /memory opened a picker")
	}
}

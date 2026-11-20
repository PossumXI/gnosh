package ui

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// Ctrl+V puts the picture the session kept off the clipboard into the box
// as @image-N, and says why when there is none.
func TestCtrlVPastesAPicture(t *testing.T) {
	answers := []struct {
		name string
		err  error
	}{{"image-1", nil}, {"", app.ErrNoPicture}}
	n := 0
	g := newRig(t, 100, 24, text.NoColor, func(c *controls) {
		c.picture = func(context.Context) (string, error) {
			a := answers[min(n, len(answers)-1)]
			n++
			return a.name, a.err
		}
	})
	g.keys(`"what is in " <Ctrl+V>`)
	if got := g.box(); got != "what is in @image-1 " {
		t.Fatalf("the box holds %q", got)
	}
	g.liveHas("The picture goes with the prompt as @image-1.")
	g.keys(`<Ctrl+V>`)
	g.liveHas("The clipboard holds no picture.")
	if got := g.box(); got != "what is in @image-1 " {
		t.Errorf("a clipboard with no picture changed the box: %q", got)
	}
	// Without a clipboard to read.
	h := newRig(t, 100, 24, text.NoColor)
	h.keys(`<Ctrl+V>`)
	h.liveHas("cannot read here")
}

// Picture files dragged into the terminal, which pastes their paths, are
// mentioned; a paste with anything else in it, and a shell line, are left
// as they are.
func TestDraggedPicturesAreMentioned(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "shot one.png")
	two := filepath.Join(dir, "b.jpg")
	notes := filepath.Join(dir, "notes.txt")
	for _, p := range []string{one, two, notes} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paste := func(g *rig, text string) {
		t.Helper()
		g.keys(`<Paste ` + strconv.Quote(text) + `>`)
	}
	g := newRig(t, 100, 24, text.NoColor)
	paste(g, strings.ReplaceAll(one, " ", `\ `)+" "+two+" ")
	if got, want := g.box(), `@"`+one+`" @`+two+` `; got != want {
		t.Errorf("the box holds %q, want %q", got, want)
	}
	h := newRig(t, 100, 24, text.NoColor)
	paste(h, "file://"+strings.ReplaceAll(one, " ", "%20"))
	if got, want := h.box(), `@"`+one+`" `; got != want {
		t.Errorf("a file:// paste: %q, want %q", got, want)
	}
	for _, pasted := range []string{notes, one + " and more", filepath.Join(dir, "gone.png")} {
		k := newRig(t, 100, 24, text.NoColor)
		paste(k, pasted)
		if got := k.box(); got != pasted {
			t.Errorf("%q became %q", pasted, got)
		}
	}
	sh := newRig(t, 100, 24, text.NoColor)
	sh.keys(`"!open "`)
	paste(sh, two)
	if got := sh.box(); got != "!open "+two {
		t.Errorf("in a shell line: %q", got)
	}
}

package ui

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// /config editorMode=vim gives the box vim's keys at once, the status line
// names their mode as it changes, and editorMode=normal takes them away.
func TestVimMode(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor, func(c *controls) {
		c.info.Commands = append(c.info.Commands, "config")
	})
	g.liveLacks("-- INSERT --", "-- NORMAL --")
	g.keys(`"/config editorMode=vim" <Enter>`)
	g.liveHas("-- INSERT --", "manual mode")
	g.keys(`"abc" <Esc>`)
	g.liveHas("-- NORMAL --").liveLacks("-- INSERT --")
	g.keys(`"0x"`)
	if got := g.box(); got != "bc" {
		t.Fatalf("0x in NORMAL mode left %q in the box, want bc", got)
	}
	g.keys(`"v"`)
	g.liveHas("-- VISUAL --")
	g.keys(`<Esc> "A"`)
	g.liveHas("-- INSERT --")
	// The box is sent from INSERT mode as from any, and a new prompt
	// starts in INSERT mode.
	g.keys(`<Esc> "dd" "i/config editorMode=normal" <Enter>`)
	g.liveLacks("-- INSERT --", "-- NORMAL --")
	// Esc with text only asks for a second to clear, and y is typed.
	g.keys(`"x" <Esc> "y"`)
	if got := g.box(); got != "xy" {
		t.Fatalf("with vim's keys off the box holds %q, want xy", got)
	}
}

// A custom status line keeps vim's mode in sight, on the line of the
// permission mode.
func TestVimModeBesideACustomStatusLine(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor)
	g.s.ed.SetVim(true)
	g.s.custom = []string{"my status"}
	g.keys(`"a"`)
	g.liveHas("-- INSERT --", "my status")
}

package editor

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

// rig is an editor under test at one width, with a clock the test moves
// by hand.
type rig struct {
	t     *testing.T
	e     *Editor
	width int
	now   time.Time
	// last is what the last event of the last script did, and all is
	// what every event of it did.
	last Result
	all  []Result
}

// newRig makes an editor for a terminal width cells wide. The clock is
// the rig's own, so that a second press is early or late as the test
// says.
func newRig(t *testing.T, width int, o Options) *rig {
	t.Helper()
	g := &rig{t: t, width: width, now: time.Unix(1_800_000_000, 0)}
	if o.Now == nil {
		o.Now = func() time.Time { return g.now }
	}
	g.e = New(o)
	g.e.Render(width)
	return g
}

// do feeds a key script to the editor, rendering after every event as the
// application does, and checks after each that the view is sound.
func (g *rig) do(script string) *rig {
	g.t.Helper()
	evs, err := keyscript.Parse(script)
	if err != nil {
		g.t.Fatal(err)
	}
	g.all = g.all[:0]
	for _, ev := range evs {
		g.last = g.e.Handle(ev)
		g.all = append(g.all, g.last)
		checkView(g.t, g.e, g.width, fmt.Sprintf("after %v", ev))
	}
	return g
}

// wait moves the clock on.
func (g *rig) wait(d time.Duration) *rig {
	g.now = g.now.Add(d)
	return g
}

// text checks what the box holds.
func (g *rig) text(want string) *rig {
	g.t.Helper()
	if got := g.e.Text(); got != want {
		g.t.Fatalf("text is %q, want %q", got, want)
	}
	return g
}

// cursor checks where the cursor is, as a byte offset into the text.
func (g *rig) cursor(want int) *rig {
	g.t.Helper()
	if got := g.e.Cursor(); got != want {
		g.t.Fatalf("cursor is at %d of %q, want %d", got, g.e.Text(), want)
	}
	return g
}

// screen checks the view: its rows without their styles, and the cursor's
// row and column. A cursor row of -1 means hidden.
func (g *rig) screen(row, col int, lines ...string) *rig {
	g.t.Helper()
	v := g.e.Render(g.width)
	got := plainLines(v)
	if strings.Join(got, "\n") != strings.Join(lines, "\n") || v.CursorRow != row || row >= 0 && v.CursorCol != col {
		g.t.Fatalf("view at width %d:\n%s\ncursor at row %d col %d\nwant:\n%s\ncursor at row %d col %d",
			g.width, box(got), v.CursorRow, v.CursorCol, box(lines), row, col)
	}
	return g
}

// action checks what the last event asked the application to do.
func (g *rig) action(want Action) *rig {
	g.t.Helper()
	if g.last.Action != want {
		g.t.Fatalf("last event asked for %v, want %v (result %+v)", g.last.Action, want, g.last)
	}
	return g
}

// submitted checks that the last event sent text, in mode.
func (g *rig) submitted(text string, mode Mode) *rig {
	g.t.Helper()
	if g.last.Action != ActionSubmit || g.last.Text != text || g.last.Mode != mode {
		g.t.Fatalf("last event: %+v, want a submit of %q in %v mode", g.last, text, mode)
	}
	return g
}

// plainLines is a view's rows without their styles and without the blanks
// at their ends.
func plainLines(v View) []string {
	out := make([]string, len(v.Lines))
	for i, l := range v.Lines {
		out[i] = strings.TrimRight(text.Strip(l), " ")
	}
	return out
}

// box draws rows for a failed test.
func box(lines []string) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "  |%s|\n", l)
	}
	return strings.TrimRight(b.String(), "\n")
}

// checkView fails the test unless the editor's view at width is sound:
// no row wider than the width, no control character in any row but the
// escape sequences of styles, and the cursor on a row and a cell that
// exist.
func checkView(t *testing.T, e *Editor, width int, when string) {
	t.Helper()
	v := e.Render(width)
	again := e.Render(width)
	if fmt.Sprint(v) != fmt.Sprint(again) {
		t.Fatalf("%s: rendering twice gave two views:\n%+v\n%+v", when, v, again)
	}
	for i, l := range v.Lines {
		if w := text.VisibleWidth(l); w > width {
			t.Fatalf("%s: row %d is %d cells wide in a terminal of %d: %q", when, i, w, width, l)
		}
		for tok := range text.Tokens(l) {
			if tok.Kind != text.Plain {
				if !tok.IsSGR() {
					t.Fatalf("%s: row %d holds an escape sequence that is not a style: %q", when, i, l)
				}
				continue
			}
			for _, r := range tok.Text {
				if r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0 {
					t.Fatalf("%s: row %d holds the control character %U: %q", when, i, r, l)
				}
			}
		}
	}
	if v.CursorRow < 0 {
		return
	}
	if v.CursorRow >= len(v.Lines) {
		t.Fatalf("%s: cursor on row %d of %d", when, v.CursorRow, len(v.Lines))
	}
	if v.CursorCol < 0 || v.CursorCol >= width {
		t.Fatalf("%s: cursor at cell %d in a terminal of %d", when, v.CursorCol, width)
	}
}

// key is one key from a script.
func key(name string) term.Key {
	k, err := keyscript.Key(name)
	if err != nil {
		panic(err)
	}
	return k
}

// pasteOf is a bracketed paste of s, for text a key script cannot spell.
func pasteOf(s string) term.Paste { return term.Paste{Text: s} }

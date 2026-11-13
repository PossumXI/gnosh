package menu

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

// rig is a question under test at one width.
type rig struct {
	t     *testing.T
	p     Prompt
	width int
	// last is what the last event of the last script did, and all is
	// what every event of it did.
	last Result
	all  []Result
}

func newRig(t *testing.T, width int, p Prompt) *rig {
	t.Helper()
	g := &rig{t: t, p: p, width: width}
	checkView(t, p, width, "at the start")
	return g
}

// do feeds a key script to the question, drawing it after every event as
// the application does and checking that what is drawn is sound.
func (g *rig) do(script string) *rig {
	g.t.Helper()
	evs, err := keyscript.Parse(script)
	if err != nil {
		g.t.Fatal(err)
	}
	g.all = g.all[:0]
	for _, ev := range evs {
		g.last = g.p.Handle(ev)
		g.all = append(g.all, g.last)
		checkView(g.t, g.p, g.width, fmt.Sprintf("after %v", ev))
	}
	return g
}

// screen checks the view: its rows without their styles, and the
// cursor's row and column. A cursor row of -1 means hidden.
func (g *rig) screen(row, col int, lines ...string) *rig {
	g.t.Helper()
	v := g.p.Render(g.width)
	got := plainLines(v)
	if strings.Join(got, "\n") != strings.Join(lines, "\n") || v.CursorRow != row || row >= 0 && v.CursorCol != col {
		g.t.Fatalf("view at width %d:\n%s\ncursor at row %d col %d\nwant:\n%s\ncursor at row %d col %d",
			g.width, box(got), v.CursorRow, v.CursorCol, box(lines), row, col)
	}
	return g
}

// open checks that the question is still being asked: the last event was
// understood and did not end it.
func (g *rig) open() *rig {
	g.t.Helper()
	if !g.last.Handled || g.last.Done || g.last.Cancelled || g.last.Index != -1 || g.last.Text != "" || g.last.Answers != nil {
		g.t.Fatalf("last event: %+v, want it handled and the question still open", g.last)
	}
	return g
}

// chose checks that the last event answered with option index and text.
func (g *rig) chose(index int, text string) *rig {
	g.t.Helper()
	if !g.last.Handled || !g.last.Done || g.last.Cancelled || g.last.Index != index || g.last.Text != text {
		g.t.Fatalf("last event: %+v, want option %d with text %q", g.last, index, text)
	}
	g.over()
	return g
}

// cancelled checks that the last event ended the question with no
// answer.
func (g *rig) cancelled() *rig {
	g.t.Helper()
	if !g.last.Handled || !g.last.Done || !g.last.Cancelled || g.last.Index != -1 || g.last.Text != "" || g.last.Answers != nil {
		g.t.Fatalf("last event: %+v, want the question cancelled and nothing chosen", g.last)
	}
	g.over()
	return g
}

// over checks that a question that has ended takes nothing more and draws
// nothing.
func (g *rig) over() {
	g.t.Helper()
	for _, ev := range keyscript.MustParse(`<Enter> "1" "y" <Esc> <Down> <Tab> <Paste "p">`) {
		if r := g.p.Handle(ev); r.Handled || r.Done || r.Cancelled || r.Text != "" || r.Answers != nil {
			g.t.Fatalf("%v after the question ended: %+v", ev, r)
		}
	}
	if v := g.p.Render(g.width); len(v.Lines) != 0 || v.CursorRow != -1 {
		g.t.Fatalf("a question that has ended still draws: %+v", v)
	}
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

// checkView fails the test unless the question's view at width is sound:
// no row wider than the width, nothing in any row but text and styles,
// the cursor on a row and a cell that exist, and the same view when drawn
// twice.
func checkView(t *testing.T, p Prompt, width int, when string) View {
	t.Helper()
	v := p.Render(width)
	again := p.Render(width)
	if fmt.Sprint(v) != fmt.Sprint(again) {
		t.Fatalf("%s: drawing twice gave two views:\n%+v\n%+v", when, v, again)
	}
	for i, l := range v.Lines {
		if w := text.VisibleWidth(l); w > width {
			t.Fatalf("%s: row %d is %d cells wide in a terminal of %d: %q", when, i, w, width, l)
		}
		for tok := range text.Tokens(l) {
			if tok.Kind != text.Plain {
				if !tok.IsSGR() && !tok.IsHyperlink() {
					t.Fatalf("%s: row %d holds an escape sequence that is not a style or a link: %q", when, i, l)
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
	if v.CursorRow >= len(v.Lines) || v.CursorRow < -1 {
		t.Fatalf("%s: cursor on row %d of %d", when, v.CursorRow, len(v.Lines))
	}
	if v.CursorRow >= 0 && (v.CursorCol < 0 || v.CursorCol >= width) {
		t.Fatalf("%s: cursor at cell %d in a terminal of %d", when, v.CursorCol, width)
	}
	return v
}

// key is one key from a script.
func key(name string) term.Key {
	k, err := keyscript.Key(name)
	if err != nil {
		panic(err)
	}
	return k
}

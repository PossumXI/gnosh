package editor

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// With more rows than the box may show, the rows shown follow the cursor
// and the edge of the box marks where rows are hidden.
func TestScrolling(t *testing.T) {
	g := newRig(t, 20, Options{MaxHeight: 3})
	g.do(`"1" <Ctrl+J> "2" <Ctrl+J> "3"`).screen(2, 3, "> 1", "  2", "  3")
	g.do(`<Ctrl+J> "4" <Ctrl+J> "5"`).screen(2, 3, "↑ 3", "  4", "  5")
	// The window moves only when the cursor would leave it.
	g.do(`<Up>`).screen(1, 3, "↑ 3", "  4", "  5")
	g.do(`<Up>`).screen(0, 3, "↑ 3", "  4", "  5")
	g.do(`<Up>`).screen(0, 3, "↑ 2", "  3", "↓ 4")
	g.do(`<Up>`).screen(0, 3, "> 1", "  2", "↓ 3")
	g.do(`<Down>*2`).screen(2, 3, "> 1", "  2", "↓ 3")
	g.do(`<Down>`).screen(2, 3, "↑ 2", "  3", "↓ 4")
	g.do(`<Ctrl+End>`).screen(2, 3, "↑ 3", "  4", "  5")
	g.do(`<Ctrl+Home>`).screen(0, 2, "> 1", "  2", "↓ 3")
	// PgDn and PgUp move by the height of the box.
	g.do(`<PgDn>`).screen(2, 2, "↑ 2", "  3", "↓ 4")
	g.do(`<PgDn>`).screen(2, 2, "↑ 3", "  4", "  5")
	g.do(`<PgDn>`).screen(2, 2, "↑ 3", "  4", "  5")
	g.do(`<PgUp>`).screen(0, 2, "↑ 2", "  3", "↓ 4")
	g.do(`<PgUp>`).screen(0, 2, "> 1", "  2", "↓ 3")
	// When the text gets shorter the window stays full.
	g.do(`<Ctrl+End> <Backspace>*2`).screen(2, 3, "↑ 2", "  3", "  4")
	g.do(`<Backspace>*2`).screen(2, 3, "> 1", "  2", "  3")
	g.do(`<Backspace>*2`).screen(1, 3, "> 1", "  2")
	// Text loaded from outside is shown at its end.
	g.e.SetText("a\nb\nc\nd\ne\nf")
	g.screen(2, 3, "↑ d", "  e", "  f")
	g.e.SetCursor(0)
	g.screen(0, 2, "> a", "  b", "↓ c")
	g.e.SetText("short")
	g.screen(0, 7, "> short")
}

// Wrapped rows count as rows.
func TestScrollingWrappedRows(t *testing.T) {
	g := newRig(t, 12, Options{MaxHeight: 2})
	g.do(`"aaa bbb ccc ddd eee fff"`).screen(1, 9, "↑ ccc ddd", "  eee fff")
	g.do(`<Home>`).screen(0, 2, "> aaa bbb", "↓ ccc ddd")
	g.do(`<Down>`).screen(1, 2, "> aaa bbb", "↓ ccc ddd")
	g.do(`<Down>`).screen(1, 2, "↑ ccc ddd", "  eee fff")
	// The height can be changed while the box is in use.
	g.e.SetMaxHeight(3)
	g.screen(2, 2, "> aaa bbb", "  ccc ddd", "  eee fff")
	g.e.SetMaxHeight(0)
	g.screen(2, 2, "> aaa bbb", "  ccc ddd", "  eee fff")
}

// A box one row high marks both ways at once, except on the first row of
// the text, where the mark of the mode stays.
func TestScrollingOneRow(t *testing.T) {
	g := newRig(t, 20, Options{MaxHeight: 1})
	g.do(`"1" <Ctrl+J> "2" <Ctrl+J> "3"`).screen(0, 3, "↑ 3")
	g.do(`<Up>`).screen(0, 3, "↕ 2")
	g.do(`<Up>`).screen(0, 3, "> 1")
}

// The marks have the look the theme gives them, and a theme may leave
// them out.
func TestScrollMarks(t *testing.T) {
	th := DefaultTheme(text.TrueColor)
	g := newRig(t, 20, Options{MaxHeight: 2, Theme: th})
	g.do(`"1" <Ctrl+J> "2" <Ctrl+J> "3" <Up>`)
	v := g.e.Render(20)
	if want := th.More.Render("↑") + " 2"; v.Lines[0] != want {
		t.Errorf("first row %q, want %q", v.Lines[0], want)
	}
	th.MoreAbove, th.MoreBelow, th.MoreBoth = "", "", ""
	g.e.SetTheme(th)
	g.screen(0, 3, "  2", "  3")
	// A mark too wide for the edge is left out rather than pushing the
	// text over.
	th.MoreAbove = "above"
	g.e.SetTheme(th)
	g.screen(0, 3, "  2", "  3")
}

// A locked box is dimmed, with no cursor.
func TestLockedLook(t *testing.T) {
	th := DefaultTheme(text.TrueColor)
	g := newRig(t, 30, Options{Theme: th})
	g.do(`"!ls "`)
	g.e.Handle(pasteOf(strings.Repeat("x\n", 12)))
	g.e.SetLocked(true)
	v := g.e.Render(30)
	want := th.Locked.Render("! ") + th.Locked.Render("ls ") + th.Locked.Render("[pasted 12 lines]")
	if len(v.Lines) != 1 || v.Lines[0] != want || v.CursorRow != -1 {
		t.Errorf("locked view %q cursor row %d, want %q", v.Lines, v.CursorRow, want)
	}
	g.e.SetLocked(false)
	v = g.e.Render(30)
	want = th.ShellStyle.Render("! ") + "ls " + th.Paste.Render("[pasted 12 lines]")
	if v.Lines[0] != want || v.CursorRow != 0 {
		t.Errorf("unlocked view %q cursor row %d, want %q", v.Lines, v.CursorRow, want)
	}
}

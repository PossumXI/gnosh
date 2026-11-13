package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/term/vtest"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

// stage is the editor drawn by gnosh's renderer on an emulated terminal,
// below two lines of finished output, the way the application shows it.
type stage struct {
	t         *testing.T
	e         *Editor
	scr       *vtest.Screen
	r         *term.Renderer
	width     int
	committed []string
}

func newStage(t *testing.T, width, rows int, reflow bool, o Options) *stage {
	t.Helper()
	s := &stage{t: t, e: New(o), scr: vtest.New(width, rows), width: width}
	// tmux's ways are the ones that punish a mistake.
	s.scr.SetScrollOnClear(true)
	s.scr.SetReflow(reflow)
	s.r = term.NewRenderer(s.scr, width, rows, text.Width)
	s.r.SetReflow(reflow)
	// Short enough to stay whole in the narrowest terminal used here:
	// what a terminal does to finished output on a resize is its own
	// business.
	s.committed = []string{"an earlier answer", "its second line"}
	if err := s.r.Commit(s.committed...); err != nil {
		t.Fatal(err)
	}
	s.show("at the start")
	return s
}

// show draws the editor and checks that the terminal shows exactly its
// view, below the finished output, with the cursor where the view says.
func (s *stage) show(when string) {
	s.t.Helper()
	v := s.e.Render(s.width)
	if err := s.r.SetLive(v.Lines, v.CursorRow, v.CursorCol); err != nil {
		s.t.Fatal(err)
	}
	fail := func(format string, args ...any) {
		s.t.Helper()
		s.t.Fatalf("%s, width %d: %s\nview:\n%s\ncursor row %d col %d\nterminal:\n%s",
			when, s.width, fmt.Sprintf(format, args...), box(plainLines(v)), v.CursorRow, v.CursorCol, s.scr.Dump())
	}
	want := append(append([]string{}, s.committed...), plainLines(v)...)
	for len(want) > 0 && want[len(want)-1] == "" {
		want = want[:len(want)-1]
	}
	got := s.scr.AllLines()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		fail("the terminal shows\n%s\nwant\n%s", box(got), box(want))
	}
	if bad := s.scr.Unhandled(); len(bad) > 0 {
		fail("the terminal was sent sequences it does not know: %q", bad)
	}
	if s.scr.AltScreenEver() || s.scr.MouseEver() {
		fail("the alternate screen or the mouse was switched on")
	}
	if v.CursorRow < 0 {
		if s.scr.CursorVisible() {
			fail("the cursor is showing, want it hidden")
		}
		return
	}
	if !s.scr.CursorVisible() {
		fail("the cursor is hidden")
	}
	top := len(s.committed) - len(s.scr.Scrollback())
	row, col := s.scr.Cursor()
	if row != top+v.CursorRow || col != v.CursorCol {
		fail("terminal cursor at row %d col %d, want row %d col %d", row, col, top+v.CursorRow, v.CursorCol)
	}
}

// do feeds a key script, drawing and checking after every event.
func (s *stage) do(script string) *stage {
	s.t.Helper()
	for _, ev := range keyscript.MustParse(script) {
		s.e.Handle(ev)
		s.show(fmt.Sprintf("after %v", ev))
	}
	return s
}

// resize changes the terminal's size, as dragging its edge does, and
// draws again.
func (s *stage) resize(width, rows int) *stage {
	s.t.Helper()
	s.scr.Resize(width, rows)
	if err := s.r.Resize(width, rows); err != nil {
		s.t.Fatal(err)
	}
	s.width = width
	s.show(fmt.Sprintf("after a resize to %dx%d", width, rows))
	return s
}

// A long session of typing, moving, pasting, completing and searching,
// drawn on a terminal after every key: the screen and the cursor are
// what the view says, at each width.
const longSession = `
	"The quick brown fox jumps over the lazy dog and keeps going past the edge"
	<Ctrl+A> <Alt+F>*3 <Ctrl+K> <Ctrl+Y> <Ctrl+E>
	<Ctrl+J> "wide 世界 and 👍 and e\u0301 here"
	<Left>*6 <Backspace>*3 <Ctrl+_> <Up> <Up> <Down> <End>
	<Paste "tab\there\nand a control \x1b[31m character">
	<Shift+Enter> "last line, to the very edge of the box and over it"
	<Home> <Ctrl+W> <Ctrl+U> <Ctrl+U> <Alt+/>
	<Ctrl+Home> <Ctrl+End> <PgUp> <PgDn>
	<Ctrl+C>
	"/mo" <Down> <Up> <Tab> "opus" <Ctrl+U>*2
	"see @sr" <Tab> <Down> <Enter> "and" <Enter>
	"!gi" <Tab> <Down> <Esc> <Esc> <Esc>
	<Ctrl+R> "wide" <Ctrl+R> <Backspace>*2 <Tab> <Ctrl+E> " again"
	<Ctrl+R> "zzz" <Ctrl+C>
	<Paste "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"> " after" <Left>*6 <Tab> <Ctrl+_> <Ctrl+_>
	<Ctrl+S> "aside" <Enter> <Ctrl+S>
	<Enter>
`

func TestThroughTheRenderer(t *testing.T) {
	for _, reflow := range []bool{false, true} {
		for _, width := range []int{20, 40, 80} {
			t.Run(fmt.Sprintf("width %d, reflow %v", width, reflow), func(t *testing.T) {
				h := &hook{}
				s := newStage(t, width, 40, reflow, Options{Complete: h.complete, Placeholder: "Type a prompt"})
				s.e.SetHistory(drafts("an old prompt", "wide open spaces\non two lines", "a wide berth"))
				s.do(longSession)
				if !s.e.IsEmpty() {
					t.Errorf("the session ended with %q in the box", s.e.Text())
				}
			})
		}
	}
}

// The same through changes of the terminal's width, in the middle of
// editing: after every resize the box is drawn for the new width and the
// cursor is where the view says.
func TestThroughTheRendererAcrossResizes(t *testing.T) {
	for _, reflow := range []bool{false, true} {
		t.Run(fmt.Sprintf("reflow %v", reflow), func(t *testing.T) {
			h := &hook{}
			s := newStage(t, 80, 40, reflow, Options{Complete: h.complete, MaxHeight: 6})
			s.do(`"The quick brown fox jumps over the lazy dog, twice: 世界 👍 e\u0301" <Ctrl+J> "second line"`)
			for _, width := range []int{40, 20, 80, 20, 40, 21, 79} {
				s.resize(width, 40)
				s.do(`<Up> <Left> "x" <Down> <End> " and more"`)
			}
			// With the list of candidates open, and with a search open.
			s.do(`<Ctrl+C> "/"`).resize(20, 40).do(`<Down>`).resize(80, 40).do(`<Tab>`)
			s.do(`<Ctrl+C> <Ctrl+R> "lazy"`).resize(20, 40).do(`<Ctrl+R>`).resize(40, 40).do(`<Esc>`)
			// A locked box has no cursor, at any width.
			s.e.SetLocked(true)
			s.show("locked")
			s.resize(20, 40)
			s.e.SetLocked(false)
			s.show("unlocked")
			// A shorter window: the box stays whole.
			s.resize(40, 12).do(`<Ctrl+End> " tail"`)
		})
	}
}

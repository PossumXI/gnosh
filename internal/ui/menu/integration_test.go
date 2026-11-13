package menu

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/term/vtest"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

// stage is a question drawn by gnosh's renderer on an emulated terminal,
// below two lines of finished output, the way the application shows it.
type stage struct {
	t         *testing.T
	p         Prompt
	scr       *vtest.Screen
	r         *term.Renderer
	width     int
	committed []string
}

func newStage(t *testing.T, width, rows int, reflow bool, p Prompt) *stage {
	t.Helper()
	s := &stage{t: t, p: p, scr: vtest.New(width, rows), width: width}
	// tmux's ways are the ones that punish a mistake.
	s.scr.SetScrollOnClear(true)
	s.scr.SetReflow(reflow)
	s.r = term.NewRenderer(s.scr, width, rows, text.Width)
	s.r.SetReflow(reflow)
	// Short enough to stay whole in the narrowest terminal used here.
	s.committed = []string{"an earlier answer", "its second line"}
	if err := s.r.Commit(s.committed...); err != nil {
		t.Fatal(err)
	}
	s.show("at the start")
	return s
}

// show draws the question and checks that the terminal shows exactly its
// view, below the finished output, with the cursor where the view says.
func (s *stage) show(when string) {
	s.t.Helper()
	v := s.p.Render(s.width)
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

// do feeds a key script, drawing and checking after every event, and
// returns what the last event did.
func (s *stage) do(script string) Result {
	s.t.Helper()
	var res Result
	for _, ev := range keyscript.MustParse(script) {
		res = s.p.Handle(ev)
		s.show(fmt.Sprintf("after %v", ev))
	}
	return res
}

// resize changes the terminal's size, as dragging its edge does, and
// draws again.
func (s *stage) resize(width, rows int) {
	s.t.Helper()
	s.scr.Resize(width, rows)
	if err := s.r.Resize(width, rows); err != nil {
		s.t.Fatal(err)
	}
	s.width = width
	s.show(fmt.Sprintf("after a resize to %dx%d", width, rows))
}

// Every kind of question, answered on a terminal with the screen and the
// cursor checked after every key, at three widths and through changes of
// width in the middle.
func TestThroughTheRenderer(t *testing.T) {
	sessions := []struct {
		name   string
		make   func() Prompt
		before string // keys before the terminal changes width
		after  string // and after it changes back
		check  func(t *testing.T, res Result)
	}{
		{"permission", func() Prompt { return permission() },
			`<Down> <Down> <Up> <Up> <Tab> "then run the linter" <Tab> <End> <Enter> "use yarn, not npm, and say so in the notes"`,
			`<Shift+Enter> "thanks" <Enter>`,
			func(t *testing.T, res Result) {
				if !res.Done || res.Index != 3 || res.Text != "use yarn, not npm, and say so in the notes\nthanks" {
					t.Errorf("answer: %+v", res)
				}
			}},
		{"a tall diff", func() Prompt { return asked(1) },
			`<PgDn> <Shift+Down> <PgDn>*5 <PgUp> <Up>`,
			`<Shift+Up> "y"`,
			func(t *testing.T, res Result) {
				if !res.Yes() {
					t.Errorf("answer: %+v", res)
				}
			}},
		{"form", func() Prompt { return survey(FormOptions{}) },
			`<Down> <Enter> <Space> <Down> <Space> <Tab> "wide 世界 and e\u0301 and 👍"`,
			`<Enter> <Shift+Tab>*3 <End> <Enter> "teal" <Enter> <Tab>*2 <Enter>`,
			func(t *testing.T, res Result) {
				if !res.Done || len(res.Answers) != 3 || res.Answers[0].Other != "teal" || len(res.Answers[1].Picked) != 2 || res.Answers[2].Other != "wide 世界 and e\u0301 and 👍" {
					t.Errorf("answers: %+v", res)
				}
			}},
		{"input", func() Prompt { return asked(3) },
			`"a name that is long enough to wrap in a narrow terminal" <Ctrl+A> <Alt+F>`,
			`<Ctrl+K> <Enter>`,
			func(t *testing.T, res Result) {
				if !res.Done || res.Text != "a" {
					t.Errorf("answer: %+v", res)
				}
			}},
		{"secret", func() Prompt { return asked(4) },
			`"QZ7%QZ7%QZ7%QZ7%QZ7%QZ7%" <Backspace>`,
			`<Ctrl+U> "Z" <Enter>`,
			func(t *testing.T, res Result) {
				if !res.Done || res.Text != "Z" {
					t.Errorf("answer: %d bytes, done %v", len(res.Text), res.Done)
				}
			}},
		{"cancelled", func() Prompt { return numbered(15, SelectOptions{MaxHeight: 6}) },
			`<Down>*9`,
			`<Up>*3 <Esc>`,
			func(t *testing.T, res Result) {
				if !res.Done || !res.Cancelled || res.Index != -1 {
					t.Errorf("answer: %+v", res)
				}
			}},
	}
	for _, c := range sessions {
		for _, reflow := range []bool{false, true} {
			for _, width := range []int{20, 40, 80} {
				t.Run(fmt.Sprintf("%s, width %d, reflow %v", c.name, width, reflow), func(t *testing.T) {
					s := newStage(t, width, 40, reflow, c.make())
					s.do(c.before)
					for _, w := range []int{80, 20, 40, width} {
						s.resize(w, 40)
					}
					res := s.do(c.after)
					c.check(t, res)
					// A question that has ended leaves nothing behind.
					if got := s.scr.AllLines(); strings.Join(got, "\n") != strings.Join(s.committed, "\n") {
						t.Errorf("after the question ended the terminal shows\n%s", box(got))
					}
				})
			}
		}
	}
}

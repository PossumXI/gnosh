package editor

import (
	"fmt"
	"strings"
	"testing"
)

// The view after typing, at several widths: what is on each row and
// exactly where the cursor is.
func TestViewAfterTyping(t *testing.T) {
	a17 := strings.Repeat("a", 17)
	cases := []struct {
		name   string
		width  int
		script string
		row    int
		col    int
		lines  []string
	}{
		{"empty", 20, ``, 0, 2, []string{">"}},
		{"ascii", 20, `"hello"`, 0, 7, []string{"> hello"}},
		{"second line", 20, `"ab" <Ctrl+J> "c"`, 1, 3, []string{"> ab", "  c"}},
		{"empty second line", 20, `"ab" <Ctrl+J>`, 1, 2, []string{"> ab", ""}},

		// Text other than spaces stops one cell short of the edge, so the
		// cursor after it is still on the screen.
		{"as much as fits", 20, `"` + a17 + `"`, 0, 19, []string{"> " + a17}},
		{"one more wraps", 20, `"` + a17 + `a"`, 1, 3, []string{"> " + a17, "  a"}},
		// A space may take the last cell. The line then fills the width
		// exactly, and its end is on a row of its own.
		{"space in the last cell", 20, `"` + a17 + ` "`, 1, 2, []string{"> " + a17, ""}},
		{"word after that space", 20, `"` + a17 + ` b"`, 1, 3, []string{"> " + a17, "  b"}},
		{"second space wraps", 20, `"` + a17 + `  "`, 1, 3, []string{"> " + a17, ""}},

		// Words wrap whole, and the space stays at the end of its row.
		{"word wrap", 20, `"hello world foo bar"`, 1, 5, []string{"> hello world foo", "  bar"}},
		{"word that just fits", 20, `"hello world fooo"`, 0, 18, []string{"> hello world fooo"}},
		{"word one too long", 20, `"hello world foooo"`, 0, 19, []string{"> hello world foooo"}},
		{"word two too long", 20, `"hello world fooooo"`, 1, 8, []string{"> hello world", "  fooooo"}},
		{"long word is cut", 12, `"abcdefghijkl"`, 1, 5, []string{"> abcdefghi", "  jkl"}},
		{"narrow", 6, `"ab cd"`, 1, 4, []string{"> ab", "  cd"}},

		// Chinese: two cells each, breaking between any two.
		{"cjk", 20, `"世界"`, 0, 6, []string{"> 世界"}},
		{"cjk wraps between characters", 10, `"世界世界"`, 1, 4, []string{"> 世界世", "  界"}},
		{"wide character reaching the last cell but one", 10, `"abcde世"`, 0, 9, []string{"> abcde世"}},
		{"wide character at the right edge wraps", 10, `"abcdef世"`, 1, 4, []string{"> abcdef", "  世"}},
		{"wide character with one cell left wraps", 10, `"abcdefg世"`, 1, 4, []string{"> abcdefg", "  世"}},

		// Emoji and combining sequences are one step and one or two cells.
		{"emoji", 20, `"a👍b"`, 0, 6, []string{"> a👍b"}},
		{"emoji joined by zero-width joiners", 20, `"👨\u200D👩\u200D👧x"`, 0, 5, []string{"> 👨\u200D👩\u200D👧x"}},
		{"flag", 20, `"🇺🇸x"`, 0, 5, []string{"> 🇺🇸x"}},
		{"skin tone", 20, `"👍🏽x"`, 0, 5, []string{"> 👍🏽x"}},
		{"combining mark", 20, `"e\u0301x"`, 0, 4, []string{"> e\u0301x"}},
		{"hangul jamo", 20, `"\u1112\u1161\u11ABx"`, 0, 5, []string{"> \u1112\u1161\u11ABx"}},

		// A tab reaches to the next stop, four cells apart, counted from
		// where the text starts.
		{"tab", 20, `<Paste "a\tb">`, 0, 7, []string{"> a   b"}},
		{"tab at a stop", 20, `<Paste "abcd\tb">`, 0, 11, []string{"> abcd    b"}},
		{"tab first", 20, `<Paste "\tb">`, 0, 7, []string{">     b"}},
		{"tab cut short at the edge", 12, `<Paste "abcdefgh\t">`, 1, 2, []string{"> abcdefgh", ""}},
		{"tab on the next row", 12, `<Paste "abcdefghi \tb">`, 1, 7, []string{"> abcdefghi", "      b"}},

		// What a terminal would not show, or must not be sent, is drawn
		// as a stand-in and is still one step.
		{"escape character", 20, `<Paste "a\x1b[31mb">`, 0, 10, []string{"> a^[[31mb"}},
		{"zero-width space", 20, `<Paste "a\u200Bb">`, 0, 12, []string{"> a<U+200B>b"}},
		{"lone combining mark", 20, `<Paste "\u0301">`, 0, 10, []string{"> <U+0301>"}},
		{"carriage return is a line break", 20, `<Paste "a\r\nb\rc">`, 2, 3, []string{"> a", "  b", "  c"}},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s at %d", c.name, c.width), func(t *testing.T) {
			newRig(t, c.width, Options{}).do(c.script).screen(c.row, c.col, c.lines...)
		})
	}
}

// Stepping through a line with Right and back with Left: every item is
// one step, and the cursor is on the cell where the item begins.
func TestCursorSteps(t *testing.T) {
	type at struct{ row, col int }
	cases := []struct {
		name   string
		width  int
		script string
		stops  []at // the cursor at the start and after each Right
	}{
		{"ascii", 20, `"abc"`, []at{{0, 2}, {0, 3}, {0, 4}, {0, 5}}},
		{"cjk", 20, `"a世b"`, []at{{0, 2}, {0, 3}, {0, 5}, {0, 6}}},
		{"emoji", 20, `"👍👨\u200D👩\u200D👧🇺🇸"`, []at{{0, 2}, {0, 4}, {0, 6}, {0, 8}}},
		{"combining", 20, `"e\u0301o\u0308\u0304"`, []at{{0, 2}, {0, 3}, {0, 4}}},
		{"tabs", 20, `<Paste "a\t\tb">`, []at{{0, 2}, {0, 3}, {0, 6}, {0, 10}, {0, 11}}},
		{"control characters", 20, `<Paste "a\x00\x7fb">`, []at{{0, 2}, {0, 3}, {0, 5}, {0, 7}, {0, 8}}},
		{"space with a mark on it", 20, `<Paste "a \u0301 \u200Db">`, []at{{0, 2}, {0, 3}, {0, 4}, {0, 5}, {0, 6}}},
		{"across a line break", 20, `"a" <Ctrl+J> "b"`, []at{{0, 2}, {0, 3}, {1, 2}, {1, 3}}},
		// The place after the last character of a wrapped row is the
		// start of the next row.
		{"across a wrap", 6, `"abcd"`, []at{{0, 2}, {0, 3}, {0, 4}, {1, 2}, {1, 3}}},
		{"across a wrap after a space", 7, `"ab  cd"`, []at{{0, 2}, {0, 3}, {0, 4}, {0, 5}, {1, 2}, {1, 3}, {1, 4}}},
		{"wide character pushed to the next row", 7, `"abc世"`, []at{{0, 2}, {0, 3}, {0, 4}, {1, 2}, {1, 4}}},
		// A line that fills the width exactly: its end is on the next
		// row.
		{"line that fills the width", 6, `"abc "`, []at{{0, 2}, {0, 3}, {0, 4}, {0, 5}, {1, 2}}},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s at %d", c.name, c.width), func(t *testing.T) {
			g := newRig(t, c.width, Options{}).do(c.script).do(`<Ctrl+Home>`)
			check := func(i int, what string) {
				t.Helper()
				v := g.e.Render(c.width)
				if v.CursorRow != c.stops[i].row || v.CursorCol != c.stops[i].col {
					t.Fatalf("%s: stop %d: cursor at row %d col %d, want row %d col %d\n%s",
						what, i, v.CursorRow, v.CursorCol, c.stops[i].row, c.stops[i].col, box(plainLines(v)))
				}
			}
			check(0, "at the start")
			for i := 1; i < len(c.stops); i++ {
				g.do(`<Right>`)
				check(i, "going right")
			}
			g.do(`<Right>`)
			check(len(c.stops)-1, "right at the end")
			for i := len(c.stops) - 2; i >= 0; i-- {
				g.do(`<Left>`)
				check(i, "going left")
			}
			g.do(`<Left>`)
			check(0, "left at the start")
		})
	}
}

// Up and Down go by rows, wrapped rows included, and keep to the column
// they started from.
func TestUpDownKeepColumn(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.do(`"abcdefghij" <Ctrl+J> "xy" <Ctrl+J> "abcdefghij"`).screen(2, 12, "> abcdefghij", "  xy", "  abcdefghij")
	g.do(`<Up>`).screen(1, 4, "> abcdefghij", "  xy", "  abcdefghij")
	g.do(`<Up>`).screen(0, 12, "> abcdefghij", "  xy", "  abcdefghij")
	g.do(`<Down> <Down>`).screen(2, 12, "> abcdefghij", "  xy", "  abcdefghij")
	// Any other key forgets the column.
	g.do(`<Up> <Left> <Up>`).screen(0, 3, "> abcdefghij", "  xy", "  abcdefghij")
	// Ctrl+P and Ctrl+N are Up and Down.
	g.do(`<Ctrl+N> <Ctrl+N> <Ctrl+E> <Ctrl+P> <Ctrl+P>`).screen(0, 12, "> abcdefghij", "  xy", "  abcdefghij")
}

func TestUpDownThroughWrappedRows(t *testing.T) {
	g := newRig(t, 12, Options{})
	// Ten cells for text: "aaa bbb ccc ddd eee" wraps after each pair.
	g.do(`"aaa bbb ccc ddd eee"`).screen(2, 5, "> aaa bbb", "  ccc ddd", "  eee")
	g.do(`<Up>`).screen(1, 5, "> aaa bbb", "  ccc ddd", "  eee")
	g.do(`<Up>`).screen(0, 5, "> aaa bbb", "  ccc ddd", "  eee")
	// The end of a wrapped row belongs to the next row, so the furthest
	// right the cursor goes on it is its last character: here the space
	// the row ends with.
	g = newRig(t, 12, Options{})
	g.do(`"aaa bbb ccc ddd eeeeeeeee"`).screen(2, 11, "> aaa bbb", "  ccc ddd", "  eeeeeeeee")
	g.do(`<Up>`).screen(1, 9, "> aaa bbb", "  ccc ddd", "  eeeeeeeee")
	g.do(`<Up>`).screen(0, 9, "> aaa bbb", "  ccc ddd", "  eeeeeeeee")
	g.do(`<Down>`).screen(1, 9, "> aaa bbb", "  ccc ddd", "  eeeeeeeee")
	g.do(`<Down>`).screen(2, 11, "> aaa bbb", "  ccc ddd", "  eeeeeeeee")
	// A wide character under the column: the cursor goes to its start.
	g = newRig(t, 20, Options{})
	g.do(`"abcd" <Ctrl+J> "世界世" <Up> <Left> <Down>`).screen(1, 4, "> abcd", "  世界世")
}

// The same text drawn at many widths: every row fits, and typing it gives
// the same rows as loading it.
func TestWidths(t *testing.T) {
	const sample = "The quick brown 狐狸 jumps over the lazy dog 👍🏽 and e\u0301te\u0301.\n\tindented\twith tabs\nsecond paragraph with a very-long-unbroken-token-that-must-be-cut-somewhere"
	for width := 1; width <= 60; width++ {
		typed := newRig(t, width, Options{})
		for _, r := range sample {
			if r == '\n' {
				typed.do(`<Ctrl+J>`)
			} else if r == '\t' {
				typed.do(`<Paste "\t">`)
			} else {
				typed.e.Handle(key(string(r)))
			}
		}
		loaded := newRig(t, width, Options{})
		loaded.e.SetText(sample)
		a, b := typed.e.Render(width), loaded.e.Render(width)
		if strings.Join(a.Lines, "\n") != strings.Join(b.Lines, "\n") || a.CursorRow != b.CursorRow || a.CursorCol != b.CursorCol {
			t.Fatalf("width %d: typing and loading differ:\n%s\n%s", width, box(plainLines(a)), box(plainLines(b)))
		}
		if typed.e.Text() != sample {
			t.Fatalf("width %d: text is %q", width, typed.e.Text())
		}
		checkView(t, typed.e, width, fmt.Sprintf("width %d", width))
		// Walking the whole text keeps the cursor on the screen.
		typed.do(`<Ctrl+Home>`)
		for range len(sample) {
			typed.do(`<Right>`)
		}
		typed.cursor(len(sample))
	}
}

// Widths too small to draw anything sensible still give a sound view.
func TestTinyWidths(t *testing.T) {
	for width := -1; width <= 0; width++ {
		e := New(Options{})
		e.SetText("hello")
		if v := e.Render(width); len(v.Lines) != 0 || v.CursorRow != -1 {
			t.Errorf("width %d: %+v, want nothing and a hidden cursor", width, v)
		}
	}
	// One cell: every character on its own row, a wide one cut down.
	g := newRig(t, 1, Options{})
	g.do(`"a世"`).screen(2, 0, "a", "", "")
	// The prefix is dropped when there is no room beside it.
	g = newRig(t, 5, Options{})
	g.do(`"abcdef"`).screen(1, 2, "abcd", "ef")
}

// A space with a mark on it is one item: Up and Down do not put the
// cursor between the two, and a row does not end between them.
func TestSpaceWithAMark(t *testing.T) {
	g := newRig(t, 8, Options{})
	g.do(`<Paste "abc \u0301de fg">`).screen(1, 6, "> abc \u0301d", "  e fg")
	g.do(`<Up>`).cursor(6)
	g.do(`<Left>`).cursor(3)
	g.do(`<Down> <Up>`).cursor(3)
	g.do(`<Right> <Right>`).cursor(7)
	g = newRig(t, 6, Options{})
	g.do(`<Paste "ab \u0301cd">`).screen(1, 4, "> ab \u0301", "  cd")
}

// The placeholder shows while the box is empty, cut at a word to fit.
func TestPlaceholder(t *testing.T) {
	g := newRig(t, 40, Options{Placeholder: "Ask gnosh, or / for commands"})
	g.screen(0, 2, "> Ask gnosh, or / for commands")
	g.do(`"x"`).screen(0, 3, "> x")
	g.do(`<Backspace>`).screen(0, 2, "> Ask gnosh, or / for commands")
	g = newRig(t, 20, Options{Placeholder: "Ask gnosh, or / for commands"})
	g.screen(0, 2, "> Ask gnosh, or /")
	g.e.SetPlaceholder("")
	g.screen(0, 2, ">")
}

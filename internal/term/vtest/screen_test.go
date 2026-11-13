package vtest

import (
	"reflect"
	"testing"
)

// feed makes a screen and writes input to it.
func feed(cols, rows int, input string) *Screen {
	s := New(cols, rows)
	s.WriteString(input)
	return s
}

// wantLines checks the visible rows; rows not listed must be empty.
func wantLines(t *testing.T, s *Screen, want ...string) {
	t.Helper()
	got := s.Lines()
	for len(want) < len(got) {
		want = append(want, "")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("screen is\n%s\nwant rows %q", s.Dump(), want)
	}
}

func wantCursor(t *testing.T, s *Screen, row, col int) {
	t.Helper()
	if r, c := s.Cursor(); r != row || c != col {
		t.Errorf("cursor at row %d col %d, want row %d col %d\n%s", r, c, row, col, s.Dump())
	}
}

func wantScrollback(t *testing.T, s *Screen, want ...string) {
	t.Helper()
	got := s.Scrollback()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scrollback is %q, want %q\n%s", got, want, s.Dump())
	}
}

func wantNothingUnhandled(t *testing.T, s *Screen) {
	t.Helper()
	if u := s.Unhandled(); len(u) > 0 {
		t.Errorf("sequences the screen does not know: %q", u)
	}
}

// ECMA-48 8.3.15 (CR), 8.3.74 (LF), 8.3.5 (BS), 8.3.60 (HT).
func TestControlCharacters(t *testing.T) {
	s := feed(20, 4, "abc\r\ndef")
	wantLines(t, s, "abc", "def")
	wantCursor(t, s, 1, 3)

	s = feed(20, 4, "abc\ndef")
	wantLines(t, s, "abc", "   def")

	s = feed(20, 4, "abc\rX")
	wantLines(t, s, "Xbc")

	s = feed(20, 4, "abc\b\bX")
	wantLines(t, s, "aXc")
	wantCursor(t, s, 0, 2)

	s = feed(20, 4, "\b\bX")
	wantLines(t, s, "X")

	s = feed(20, 4, "a\tb\tc")
	wantLines(t, s, "a       b       c")
	s = feed(20, 4, "\t\t\t\tX")
	wantCursor(t, s, 0, 19)

	s = feed(20, 4, "a\vb\fc")
	wantLines(t, s, "a", " b", "  c")

	s = feed(20, 4, "a\x07b\x00c\x7fd")
	wantLines(t, s, "abcd")
	if s.Bells() != 1 {
		t.Errorf("bells = %d, want 1", s.Bells())
	}
}

// The DEC terminals' deferred wrap (VT100 User Guide, "Auto Wrap"; xterm
// calls it the wrap-pending flag): a character written in the last column
// leaves the cursor there, and only the next character moves to a new line.
func TestDeferredWrap(t *testing.T) {
	s := feed(5, 3, "abcde")
	wantLines(t, s, "abcde")
	wantCursor(t, s, 0, 4)
	if !s.WrapPending() {
		t.Error("no wrap pending after filling the last column")
	}

	s.WriteString("f")
	wantLines(t, s, "abcde", "f")
	wantCursor(t, s, 1, 1)
	if !s.Wrapped(0) || s.Wrapped(1) {
		t.Error("row 0 should be marked as wrapped into row 1, and row 1 not")
	}

	// A full line followed by CR LF leaves no blank line.
	s = feed(5, 3, "abcde\r\nfgh")
	wantLines(t, s, "abcde", "fgh")
	if s.Wrapped(0) {
		t.Error("row 0 is marked wrapped, but it was ended by CR LF")
	}

	// CR cancels the pending wrap.
	s = feed(5, 3, "abcde\rX")
	wantLines(t, s, "Xbcde")

	// Cursor movement cancels it too.
	s = feed(5, 3, "abcde\x1b[DX")
	wantLines(t, s, "abcXe")
	s = feed(5, 3, "abcde\x1b[5GX")
	wantLines(t, s, "abcdX")
	wantCursor(t, s, 0, 4)

	// Wrapping at the bottom of the screen scrolls.
	s = feed(3, 2, "abcdefg")
	wantLines(t, s, "def", "g")
	wantScrollback(t, s, "abc")
}

// VT100 DECAWM (mode 7): with wrapping off, text stays on the last column.
func TestAutoWrapOff(t *testing.T) {
	s := feed(5, 3, "\x1b[?7labcdefgh")
	wantLines(t, s, "abcdh")
	wantCursor(t, s, 0, 4)
	if s.WrapPending() {
		t.Error("wrap pending with wrapping off")
	}
	s.WriteString("\x1b[?7h\r\nxy")
	wantLines(t, s, "abcdh", "xy")

	// Turning wrapping off cancels a pending wrap.
	s = feed(5, 3, "abcde\x1b[?7lX")
	wantLines(t, s, "abcdX")
	wantNothingUnhandled(t, s)
}

func TestWideCharacters(t *testing.T) {
	s := feed(6, 3, "a世b")
	wantLines(t, s, "a世b")
	wantCursor(t, s, 0, 4)
	if c := s.Cell(0, 1); c.Text != "世" || c.Width != 2 {
		t.Errorf("cell 1 = %+v, want the wide character with width 2", c)
	}
	if c := s.Cell(0, 2); c.Text != "" || c.Width != 0 {
		t.Errorf("cell 2 = %+v, want the second half of the wide character", c)
	}

	// A wide character that has one cell left wraps whole.
	s = feed(5, 3, "abcd世")
	wantLines(t, s, "abcd", "世")
	wantCursor(t, s, 1, 2)
	if !s.Wrapped(0) {
		t.Error("row 0 is not marked wrapped")
	}

	// A wide character that ends exactly at the edge defers the wrap.
	s = feed(5, 3, "abc世")
	wantLines(t, s, "abc世")
	wantCursor(t, s, 0, 4)
	if !s.WrapPending() {
		t.Error("no wrap pending after a wide character filled the edge")
	}

	// With wrapping off it is not printed at all.
	s = feed(5, 3, "\x1b[?7labcd世")
	wantLines(t, s, "abcd")

	// Writing over half of a wide character removes all of it.
	s = feed(6, 3, "世界\rX")
	wantLines(t, s, "X 界")
	s = feed(6, 3, "世界\x1b[2GX")
	wantLines(t, s, " X界")
	s = feed(6, 3, "ab\r世")
	wantLines(t, s, "世")
	s = feed(6, 3, "a世\rXY")
	wantLines(t, s, "XY")
}

func TestCombiningMarks(t *testing.T) {
	s := feed(6, 2, "e\u0301x")
	wantLines(t, s, "e\u0301x")
	wantCursor(t, s, 0, 2)
	if c := s.Cell(0, 0); c.Text != "e\u0301" {
		t.Errorf("cell 0 = %q, want e with the accent", c.Text)
	}
	// At the last column the mark joins the character there.
	s = feed(3, 2, "abc\u0301")
	wantLines(t, s, "abc\u0301")
	// With the terminal's wrapping off there is no pending wrap, and the
	// mark joins the character to the left of the last column (as in
	// tmux; see combine).
	s = feed(3, 2, "\x1b[?7labe\u0301")
	wantLines(t, s, "ab\u0301e")
	// Wrapping turned back on for the last column alone puts it right.
	s = feed(3, 2, "\x1b[?7lab\x1b[?7he\u0301\x1b[?7l")
	wantLines(t, s, "abe\u0301")
	// After the cursor has moved, a mark joins the character before it.
	s = feed(6, 2, "abc\x1b[2G\u0301")
	wantLines(t, s, "a\u0301bc")
	// With nothing before it a mark is dropped.
	s = feed(3, 2, "\u0301a")
	wantLines(t, s, "a")
}

// ECMA-48 8.3.22 CUU, 8.3.19 CUD, 8.3.20 CUF, 8.3.18 CUB, 8.3.12 CNL,
// 8.3.13 CPL, 8.3.9 CHA, 8.3.21 CUP, 8.3.63 HVP, 8.3.158 VPA.
func TestCursorMovement(t *testing.T) {
	cases := []struct {
		name, input string
		row, col    int
	}{
		{"CUP", "\x1b[3;5H", 2, 4},
		{"CUP default is home", "\x1b[3;5H\x1b[H", 0, 0},
		{"CUP zero means one", "\x1b[0;0H", 0, 0},
		{"CUP row only", "\x1b[3H", 2, 0},
		{"CUP clamps", "\x1b[99;99H", 4, 9},
		{"HVP", "\x1b[2;3f", 1, 2},
		{"CUU", "\x1b[3;5H\x1b[2A", 0, 4},
		{"CUU default one", "\x1b[3;5H\x1b[A", 1, 4},
		{"CUU zero is one", "\x1b[3;5H\x1b[0A", 1, 4},
		{"CUU stops at the top", "\x1b[3;5H\x1b[9A", 0, 4},
		{"CUD", "\x1b[2B", 2, 0},
		{"CUD stops at the bottom", "\x1b[99B", 4, 0},
		{"CUF", "\x1b[3C", 0, 3},
		{"CUF stops at the edge", "\x1b[99C", 0, 9},
		{"CUB", "\x1b[1;6H\x1b[2D", 0, 3},
		{"CUB stops at the edge", "\x1b[1;6H\x1b[99D", 0, 0},
		{"CNL", "\x1b[1;6H\x1b[2E", 2, 0},
		{"CPL", "\x1b[4;6H\x1b[2F", 1, 0},
		{"CHA", "\x1b[3;1H\x1b[7G", 2, 6},
		{"CHA default", "\x1b[3;5H\x1b[G", 2, 0},
		{"CHA clamps", "\x1b[99G", 0, 9},
		{"HPA", "\x1b[4`", 0, 3},
		{"VPA", "\x1b[1;4H\x1b[3d", 2, 3},
		{"HPR and VPR", "\x1b[2a\x1b[2e", 2, 2},
		{"CHT and CBT", "\x1b[40G\x1b[Z", 0, 8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := feed(10, 5, c.input)
			wantCursor(t, s, c.row, c.col)
			wantNothingUnhandled(t, s)
		})
	}
}

// ECMA-48 8.3.41 EL and 8.3.39 ED; xterm adds ED 3 for the scrollback.
func TestErase(t *testing.T) {
	fill := "aaaaa\r\nbbbbb\r\nccccc"
	s := feed(5, 3, fill+"\x1b[2;3H\x1b[K")
	wantLines(t, s, "aaaaa", "bb", "ccccc")
	wantCursor(t, s, 1, 2)
	s = feed(5, 3, fill+"\x1b[2;3H\x1b[0K")
	wantLines(t, s, "aaaaa", "bb", "ccccc")
	s = feed(5, 3, fill+"\x1b[2;3H\x1b[1K")
	wantLines(t, s, "aaaaa", "   bb", "ccccc")
	s = feed(5, 3, fill+"\x1b[2;3H\x1b[2K")
	wantLines(t, s, "aaaaa", "", "ccccc")
	wantCursor(t, s, 1, 2)

	s = feed(5, 3, fill+"\x1b[2;3H\x1b[J")
	wantLines(t, s, "aaaaa", "bb", "")
	s = feed(5, 3, fill+"\x1b[2;3H\x1b[1J")
	wantLines(t, s, "", "   bb", "ccccc")
	s = feed(5, 3, fill+"\x1b[2;3H\x1b[2J")
	wantLines(t, s, "", "", "")
	wantCursor(t, s, 1, 2)

	s = feed(5, 2, "one\r\ntwo\r\nthree\x1b[3J")
	wantLines(t, s, "two", "three")
	wantScrollback(t, s)

	// In xterm, EL with a wrap pending erases the last column, where
	// the cursor still is. (tmux erases nothing there. A program that
	// has filled a line must not erase "the rest" of it.)
	s = feed(5, 3, "abcde\x1b[K")
	wantLines(t, s, "abcd")
	if s.WrapPending() {
		t.Error("wrap still pending after EL")
	}
	wantNothingUnhandled(t, s)
}

// tmux keeps what a program clears off the screen (scroll-on-clear).
func TestScrollOnClear(t *testing.T) {
	fill := "one\r\ntwo\r\nthree"
	clear := func(seq string) *Screen {
		s := New(8, 4)
		s.SetScrollOnClear(true)
		s.WriteString(fill + seq)
		return s
	}
	// Erasing the whole screen saves it.
	s := clear("\x1b[2J")
	wantLines(t, s)
	wantScrollback(t, s, "one", "two", "three")
	wantCursor(t, s, 2, 5)
	// So does erasing to the end of the screen from the top left corner.
	s = clear("\x1b[H\x1b[J")
	wantLines(t, s)
	wantScrollback(t, s, "one", "two", "three")
	// From anywhere else it erases.
	s = clear("\x1b[1;2H\x1b[J")
	wantLines(t, s, "o")
	wantScrollback(t, s)
	s = clear("\x1b[2;1H\x1b[J")
	wantLines(t, s, "one")
	wantScrollback(t, s)
	// An empty screen saves nothing.
	s = New(8, 4)
	s.SetScrollOnClear(true)
	s.WriteString("\x1b[2J\x1b[H\x1b[J")
	wantScrollback(t, s)
	// Without the option nothing is saved.
	s = feed(8, 4, fill+"\x1b[H\x1b[J")
	wantLines(t, s)
	wantScrollback(t, s)
}

// ECMA-48 8.3.67 IL, 8.3.32 DL, 8.3.64 ICH, 8.3.26 DCH, 8.3.38 ECH.
func TestInsertAndDelete(t *testing.T) {
	fill := "one\r\ntwo\r\nthree\r\nfour"
	s := feed(8, 4, fill+"\x1b[2;2H\x1b[L")
	wantLines(t, s, "one", "", "two", "three")
	wantCursor(t, s, 1, 1)
	s = feed(8, 4, fill+"\x1b[2;1H\x1b[2L")
	wantLines(t, s, "one", "", "", "two")
	s = feed(8, 4, fill+"\x1b[2;1H\x1b[M")
	wantLines(t, s, "one", "three", "four", "")
	s = feed(8, 4, fill+"\x1b[2;1H\x1b[9M")
	wantLines(t, s, "one", "", "", "")

	s = feed(8, 2, "abcdef\x1b[1;3H\x1b[2@")
	wantLines(t, s, "ab  cdef")
	wantCursor(t, s, 0, 2)
	s = feed(8, 2, "abcdefgh\x1b[1;3H\x1b[3@")
	wantLines(t, s, "ab   cde")
	s = feed(8, 2, "abcdef\x1b[1;3H\x1b[2P")
	wantLines(t, s, "abef")
	s = feed(8, 2, "abcdef\x1b[1;3H\x1b[99P")
	wantLines(t, s, "ab")
	s = feed(8, 2, "abcdef\x1b[1;3H\x1b[2X")
	wantLines(t, s, "ab  ef")
	wantCursor(t, s, 0, 2)
	s = feed(8, 2, "abcdef\x1b[1;3H\x1b[99X")
	wantLines(t, s, "ab")

	// Insert mode (IRM, ECMA-48 7.2.10): typing pushes text right.
	s = feed(8, 2, "abc\r\x1b[4hXY\x1b[4lZ")
	wantLines(t, s, "XYZbc")
	wantNothingUnhandled(t, s)
}

// ECMA-48 8.3.147 SU and 8.3.113 SD; VT100 DECSTBM; ESC M (RI), ESC D
// (IND) and ESC E (NEL).
func TestScrolling(t *testing.T) {
	fill := "1\r\n2\r\n3\r\n4"
	s := feed(4, 4, fill+"\x1b[S")
	wantLines(t, s, "2", "3", "4", "")
	wantScrollback(t, s, "1")
	wantCursor(t, s, 3, 1)
	s = feed(4, 4, fill+"\x1b[2T")
	wantLines(t, s, "", "", "1", "2")
	wantScrollback(t, s)

	// A scroll region: line feeds at its bottom move only its lines,
	// and nothing reaches the scrollback unless it starts at row 1.
	s = feed(4, 4, fill+"\x1b[2;3r")
	wantCursor(t, s, 0, 0)
	s.WriteString("\x1b[3;1H\nX")
	wantLines(t, s, "1", "3", "X", "4")
	wantScrollback(t, s)
	s.WriteString("\x1b[2;1H\x1bMY")
	wantLines(t, s, "1", "Y", "3", "4")

	// CUU and CUD stop at the region's edges from inside it.
	s = feed(4, 4, fill+"\x1b[2;3r\x1b[3;1H\x1b[9A")
	wantCursor(t, s, 1, 0)
	s.WriteString("\x1b[9B")
	wantCursor(t, s, 2, 0)

	// A region from row 1 feeds the scrollback.
	s = feed(4, 4, fill+"\x1b[1;2r\x1b[2;1H\n")
	wantLines(t, s, "2", "", "3", "4")
	wantScrollback(t, s, "1")

	// A region that is not at least two rows is ignored.
	s = feed(4, 4, fill+"\x1b[3;3r\x1b[4;1H\n")
	wantLines(t, s, "2", "3", "4", "")

	// Origin mode (DECOM) counts rows from the region's top.
	s = feed(4, 4, fill+"\x1b[2;3r\x1b[?6h\x1b[1;1HX\x1b[9;1HY")
	wantLines(t, s, "1", "X", "Y", "4")

	s = feed(4, 4, "a\x1bDb\x1bEc\x1bMd")
	wantLines(t, s, "a", " d", "c")
	s = feed(4, 3, "top\x1b[1;1H\x1bM")
	wantLines(t, s, "", "top")
	wantNothingUnhandled(t, s)
}

func TestScrollback(t *testing.T) {
	s := New(10, 3)
	for _, l := range []string{"one", "two", "three", "four", "five"} {
		s.WriteString(l + "\r\n")
	}
	wantLines(t, s, "four", "five", "")
	wantScrollback(t, s, "one", "two", "three")
	if got, want := s.AllLines(), []string{"one", "two", "three", "four", "five"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AllLines = %q, want %q", got, want)
	}
	// The alternate screen keeps no scrollback of its own.
	s.WriteString("\x1b[?1049h1\r\n2\r\n3\r\n4\r\n5")
	wantLines(t, s, "3", "4", "5")
	wantScrollback(t, s, "one", "two", "three")
	s.WriteString("\x1b[?1049l")
	wantLines(t, s, "four", "five", "")
}

package vtest

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// ECMA-48 8.3.117 SGR; xterm's 256-colour and direct-colour forms.
func TestSGR(t *testing.T) {
	s := feed(40, 2, "\x1b[1mB\x1b[2mD\x1b[22;3mI\x1b[23;4mU\x1b[24;7mR\x1b[27;9mS\x1b[29;5mK\x1b[25;8mH\x1b[mN")
	want := []Attr{
		{Bold: true}, {Bold: true, Dim: true}, {Italic: true}, {Underline: true},
		{Reverse: true}, {Strike: true}, {Blink: true}, {Hidden: true}, {},
	}
	for i, w := range want {
		if got := s.Cell(0, i).Attr; got != w {
			t.Errorf("cell %d (%s) attr = %+v, want %+v", i, s.Cell(0, i).Text, got, w)
		}
	}

	s = feed(40, 2, "\x1b[31;42ma\x1b[91;102mb\x1b[38;5;200;48;5;17mc\x1b[38;2;1;2;3;48;2;4;5;6md\x1b[38:5:99me\x1b[38:2::7:8:9mf\x1b[38:2:7:8:9mg\x1b[39;49mh\x1b[4:3mi\x1b[4:0mj")
	colours := []struct{ fg, bg Color }{
		{Indexed(1), Indexed(2)},
		{Indexed(9), Indexed(10)},
		{Indexed(200), Indexed(17)},
		{RGB(1, 2, 3), RGB(4, 5, 6)},
		{Indexed(99), RGB(4, 5, 6)},
		{RGB(7, 8, 9), RGB(4, 5, 6)},
		{RGB(7, 8, 9), RGB(4, 5, 6)},
		{Color{}, Color{}},
	}
	for i, w := range colours {
		got := s.Cell(0, i).Attr
		if got.FG != w.fg || got.BG != w.bg {
			t.Errorf("cell %d colours = %v on %v, want %v on %v", i, got.FG, got.BG, w.fg, w.bg)
		}
	}
	if !s.Cell(0, 8).Attr.Underline || s.Cell(0, 9).Attr.Underline {
		t.Error("the colon forms of underline (4:3 on, 4:0 off) were not understood")
	}
	wantNothingUnhandled(t, s)

	// Erasing fills with the background colour in force (xterm's
	// "back colour erase").
	s = feed(6, 2, "abc\x1b[44m\x1b[K\x1b[m")
	if got := s.Cell(0, 4).Attr.BG; got != Indexed(4) {
		t.Errorf("erased cell has background %v, want 4", got)
	}
	if got := s.Cell(0, 1).Attr.BG; got != (Color{}) {
		t.Errorf("cell before the erase has background %v, want default", got)
	}
}

// VT100 DECSC and DECRC (ESC 7, ESC 8); the ANSI.SYS forms CSI s and CSI u.
func TestSaveRestoreCursor(t *testing.T) {
	s := feed(10, 4, "\x1b[2;3H\x1b[1m\x1b7\x1b[4;8H\x1b[mx\x1b8y")
	wantLines(t, s, "", "  y", "", "       x")
	if !s.Cell(1, 2).Attr.Bold {
		t.Error("ESC 8 did not bring back the bold attribute")
	}
	s = feed(10, 4, "\x1b[2;3H\x1b[s\x1b[4;8Hx\x1b[uy")
	wantLines(t, s, "", "  y", "", "       x")
	wantNothingUnhandled(t, s)
}

func TestModes(t *testing.T) {
	s := New(10, 4)
	if !s.CursorVisible() || !s.Mode(25) {
		t.Error("the cursor starts hidden")
	}
	s.WriteString("\x1b[?25l")
	if s.CursorVisible() {
		t.Error("CSI ? 25 l did not hide the cursor")
	}
	s.WriteString("\x1b[?25h\x1b[?2004h\x1b[?1004h")
	if !s.CursorVisible() || !s.Mode(2004) || !s.Mode(1004) {
		t.Error("modes 25, 2004 and 1004 are not all set")
	}
	s.WriteString("\x1b[?2004;1004l")
	if s.Mode(2004) || s.Mode(1004) {
		t.Error("two modes in one sequence were not both reset")
	}

	s.WriteString("\x1b[?2026hx\x1b[?2026l\x1b[?2026h")
	if b, e := s.SyncUpdates(); b != 2 || e != 1 || !s.Mode(2026) {
		t.Errorf("synchronized updates: %d begun, %d ended, mode set %v; want 2, 1, true", b, e, s.Mode(2026))
	}
	if s.AltScreenEver() || s.MouseEver() {
		t.Error("alternate screen or mouse reporting recorded, but neither was used")
	}
	wantNothingUnhandled(t, s)

	s.WriteString("\x1b[?9999h")
	if len(s.Unhandled()) != 1 {
		t.Errorf("an unknown mode was not recorded: %q", s.Unhandled())
	}
}

// xterm's alternate screen (modes 47, 1047, 1049) and mouse modes. gnosh
// uses neither; the screen remembers if anything ever did.
func TestAltScreenAndMouseAreRemembered(t *testing.T) {
	s := feed(10, 3, "main\x1b[?1049h\x1b[Halt")
	wantLines(t, s, "alt")
	if !s.AltScreenEver() {
		t.Error("AltScreenEver is false after CSI ? 1049 h")
	}
	s.WriteString("\x1b[?1049l")
	wantLines(t, s, "main")
	wantCursor(t, s, 0, 4)
	if !s.AltScreenEver() {
		t.Error("AltScreenEver forgot")
	}
	for _, mode := range []string{"9", "1000", "1002", "1003", "1006", "1015"} {
		s = feed(10, 3, "\x1b[?"+mode+"h\x1b[?"+mode+"l")
		if !s.MouseEver() {
			t.Errorf("MouseEver is false after mode %s was set", mode)
		}
	}
	s = feed(10, 3, "\x1b[?47hx\x1b[?47l\x1b[?1047hy\x1b[?1047l")
	wantLines(t, s)
	if !s.AltScreenEver() {
		t.Error("AltScreenEver is false after modes 47 and 1047")
	}
}

// xterm's OSC 0 and 2 (title), OSC 8 (hyperlinks) and OSC 52 (clipboard).
func TestOSC(t *testing.T) {
	s := feed(20, 2, "\x1b]2;first\x07\x1b]0;second\x1b\\x")
	if s.Title() != "second" {
		t.Errorf("title = %q, want %q", s.Title(), "second")
	}
	wantLines(t, s, "x")

	s = feed(20, 2, "a\x1b]8;;https://example.com/\x1b\\bc\x1b]8;;\x1b\\d")
	wantLines(t, s, "abcd")
	for i, want := range []string{"", "https://example.com/", "https://example.com/", ""} {
		if got := s.Cell(0, i).Attr.Link; got != want {
			t.Errorf("cell %d link = %q, want %q", i, got, want)
		}
	}

	s = feed(20, 2, "\x1b]52;c;aGVsbG8=\x1b\\\x1b]52;c;d29ybGQ=\x07")
	if got, want := s.Clipboard(), []string{"hello", "world"}; !reflect.DeepEqual(got, want) {
		t.Errorf("clipboard = %q, want %q", got, want)
	}

	s = feed(20, 2, "\x1b]133;A\x07\x1bPq#0\x1b\\\x1b_app\x1b\\ok")
	if got, want := s.Strings(), []string{"]133;A", "Pq#0", "_app"}; !reflect.DeepEqual(got, want) {
		t.Errorf("strings = %q, want %q", got, want)
	}
	wantLines(t, s, "ok")
	wantNothingUnhandled(t, s)
}

// The questions a program may ask: DA (ECMA-48 8.3.24), DSR and CPR
// (8.3.35, 8.3.14), DECRQM (VT300 and later), and the kitty keyboard
// protocol's flags.
func TestAnswers(t *testing.T) {
	ask := func(s *Screen, q string) string {
		s.WriteString(q)
		return string(s.TakeReplies())
	}
	s := New(20, 5)
	if got := ask(s, "\x1b[c"); got != "\x1b[?62;22c" {
		t.Errorf("DA1 answer = %q", got)
	}
	if got := ask(s, "\x1b[>c"); got != "\x1b[>1;10;0c" {
		t.Errorf("DA2 answer = %q", got)
	}
	if got := ask(s, "\x1b[3;7H\x1b[6n"); got != "\x1b[3;7R" {
		t.Errorf("CPR answer = %q", got)
	}
	if got := ask(s, "\x1b[?6n"); got != "\x1b[?3;7R" {
		t.Errorf("DECXCPR answer = %q", got)
	}
	if got := ask(s, "\x1b[5n"); got != "\x1b[0n" {
		t.Errorf("DSR answer = %q", got)
	}
	if got := ask(s, "\x1b[?2026$p"); got != "\x1b[?2026;2$y" {
		t.Errorf("DECRQM 2026 answer = %q", got)
	}
	if got := ask(s, "\x1b[?2026h\x1b[?2026$p\x1b[?2026l"); got != "\x1b[?2026;1$y" {
		t.Errorf("DECRQM 2026 answer while set = %q", got)
	}
	if got := ask(s, "\x1b[?25$p\x1b[?4242$p"); got != "\x1b[?25;1$y\x1b[?4242;0$y" {
		t.Errorf("DECRQM answers = %q", got)
	}
	if got := ask(s, "\x1b[18t"); got != "\x1b[8;5;20t" {
		t.Errorf("size answer = %q", got)
	}

	// The kitty keyboard protocol: a stack of flags.
	if got := ask(s, "\x1b[?u"); got != "\x1b[?0u" {
		t.Errorf("kitty answer = %q", got)
	}
	if got := ask(s, "\x1b[>1u\x1b[?u"); got != "\x1b[?1u" {
		t.Errorf("kitty answer after a push = %q", got)
	}
	if got := ask(s, "\x1b[>5u\x1b[=2;3u\x1b[?u"); got != "\x1b[?5u" {
		t.Errorf("kitty answer after push 5 and clearing bit 2 = %q", got)
	}
	if flags, depth := s.KittyFlags(); flags != 5 || depth != 2 {
		t.Errorf("kitty flags %d depth %d, want 5 and 2", flags, depth)
	}
	if got := ask(s, "\x1b[<u\x1b[?u"); got != "\x1b[?1u" {
		t.Errorf("kitty answer after a pop = %q", got)
	}
	if got := ask(s, "\x1b[<5u\x1b[?u"); got != "\x1b[?0u" {
		t.Errorf("kitty answer after popping everything = %q", got)
	}
	wantNothingUnhandled(t, s)

	// A terminal without synchronized output says the mode is unknown
	// and does not keep it.
	s = New(20, 5)
	s.SetAnswers(Answers{DeviceAttributes: true, Modes: true})
	if got := ask(s, "\x1b[?2026$p\x1b[?u\x1b[>1u\x1b[6n\x1b[c"); got != "\x1b[?2026;0$y\x1b[?62;22c" {
		t.Errorf("answers of an older terminal = %q", got)
	}
	s.WriteString("\x1b[?2026h")
	if b, _ := s.SyncUpdates(); b != 0 || s.Mode(2026) {
		t.Error("a terminal without synchronized output kept mode 2026")
	}

	// A terminal that answers nothing.
	s = New(20, 5)
	s.SetAnswers(Answers{})
	if got := ask(s, "\x1b[c\x1b[>c\x1b[6n\x1b[?6n\x1b[5n\x1b[?2026$p\x1b[?u\x1b[18t\x1bZ"); got != "" {
		t.Errorf("a silent terminal answered %q", got)
	}
	wantNothingUnhandled(t, s)
}

func TestParserEdges(t *testing.T) {
	// A control character in the middle of a sequence is carried out,
	// and the sequence goes on (the DEC parser's rule).
	s := feed(10, 3, "ab\x1b[\r2Cx")
	wantLines(t, s, "abx")
	// CAN and SUB abandon a sequence.
	s = feed(10, 3, "a\x1b[2\x18Cb\x1b]0;t\x1aC")
	wantLines(t, s, "aCbC")
	// ESC in the middle of a sequence starts over.
	s = feed(10, 3, "\x1b[2\x1b[3Cx")
	wantLines(t, s, "   x")
	// A parameter after an intermediate byte makes the sequence void.
	s = feed(10, 3, "\x1b[1 2Cx")
	wantLines(t, s, "x")
	if len(s.Unhandled()) != 1 {
		t.Errorf("unhandled = %q, want the one malformed sequence", s.Unhandled())
	}
	// Sequences the screen has no meaning for are recorded.
	s = feed(10, 3, "\x1b[5y\x1b[?5y\x1bQok")
	wantLines(t, s, "ok")
	if got, want := s.Unhandled(), []string{"\x1b[5y", "\x1b[?5y", "\x1bQ"}; !reflect.DeepEqual(got, want) {
		t.Errorf("unhandled = %q, want %q", got, want)
	}
	// Character set choices and keypad modes are taken silently.
	s = feed(10, 3, "\x1b(B\x1b)0\x1b=\x1b>ok")
	wantLines(t, s, "ok")
	wantNothingUnhandled(t, s)
	// UTF-8 that is cut short or wrong prints the replacement character.
	s = feed(10, 3, "a\xe4\xb8b\xffc")
	wantLines(t, s, "a\uFFFDb\uFFFDc")
	// REP (ECMA-48 8.3.103) repeats the last character.
	s = feed(10, 3, "ab\x1b[3b")
	wantLines(t, s, "abbbb")
	// DECALN and RIS.
	s = feed(3, 2, "\x1b#8")
	wantLines(t, s, "EEE", "EEE")
	s.WriteString("\x1b[2;2H\x1b[1m\x1bcx")
	wantLines(t, s, "x")
	if s.Cell(0, 0).Attr.Bold {
		t.Error("RIS did not reset the attributes")
	}
	// The soft reset.
	s = feed(5, 3, "\x1b[?25l\x1b[?7l\x1b[2;3r\x1b[1m\x1b[!pabcdefg")
	wantLines(t, s, "abcde", "fg")
	if !s.CursorVisible() || s.Cell(0, 0).Attr.Bold {
		t.Error("DECSTR did not show the cursor and reset the attributes")
	}
}

// A sequence cut anywhere between two writes does the same as one write.
func TestWritesCutAnywhere(t *testing.T) {
	inputs := []string{
		"plain 世界 text\r\nsecond\x1b[1;31mred\x1b[m",
		"\x1b[2;3Hx\x1b[K\x1b[?25l\x1b[?2026h\x1b]2;title\x1b\\y\x1b]8;;http://example.com\x07z",
		"abcdefghij\x1b[5D\x1b[2P\x1b[3@e\u0301\x1b[38;2;1;2;3mq",
	}
	for _, in := range inputs {
		whole := feed(12, 4, in)
		for cut := 1; cut < len(in); cut++ {
			s := New(12, 4)
			s.WriteString(in[:cut])
			s.WriteString(in[cut:])
			if s.Dump() != whole.Dump() || s.Title() != whole.Title() {
				t.Errorf("input %q cut at %d:\n%s\nwhole:\n%s", in, cut, s.Dump(), whole.Dump())
			}
			for c := 0; c < 12; c++ {
				if s.Cell(1, c) != whole.Cell(1, c) {
					t.Errorf("input %q cut at %d: cell 1,%d differs", in, cut, c)
				}
			}
		}
	}
}

func TestResizeWithoutReflow(t *testing.T) {
	s := feed(10, 4, "0123456789\r\nabc\r\nxyz")
	s.Resize(6, 4)
	wantLines(t, s, "012345", "abc", "xyz")
	wantCursor(t, s, 2, 3)
	s.Resize(12, 4)
	wantLines(t, s, "012345", "abc", "xyz")
	if cols, rows := s.Size(); cols != 12 || rows != 4 {
		t.Errorf("size = %dx%d, want 12x4", cols, rows)
	}

	// Shorter: rows below the cursor go first.
	s = feed(10, 5, "1\r\n2\r\n3\x1b[1;1H")
	s.Resize(10, 2)
	wantLines(t, s, "1", "2")
	wantScrollback(t, s)
	// Then the top rows scroll off, keeping the cursor in view.
	s = feed(10, 5, "1\r\n2\r\n3\r\n4")
	s.Resize(10, 2)
	wantLines(t, s, "3", "4")
	wantScrollback(t, s, "1", "2")
	wantCursor(t, s, 1, 1)
	// Taller: blank rows at the bottom.
	s.Resize(10, 4)
	wantLines(t, s, "3", "4", "", "")
	wantCursor(t, s, 1, 1)

	// The scroll region goes back to the whole screen.
	s = feed(4, 4, "\x1b[2;3r")
	s.Resize(4, 3)
	s.WriteString("\x1b[3;1Ha\nb")
	wantLines(t, s, "", "a", " b")

	// A wide character cut in half by the new edge goes.
	s = feed(6, 2, "abc世")
	s.Resize(4, 2)
	wantLines(t, s, "abc")
}

func TestResizeWithReflow(t *testing.T) {
	// A line the terminal wrapped is joined and wrapped again.
	s := feed(10, 4, "0123456789abcde\r\nxy")
	s.SetReflow(true)
	wantLines(t, s, "0123456789", "abcde", "xy")
	s.Resize(15, 4)
	wantLines(t, s, "0123456789abcde", "xy")
	wantCursor(t, s, 1, 2)
	s.Resize(6, 4)
	wantLines(t, s, "012345", "6789ab", "cde", "xy")
	wantCursor(t, s, 3, 2)

	// A line that ended with CR LF is cut too when it no longer fits,
	// and the cursor stays on its character.
	s = feed(10, 4, "0123456789\r\nabcdefgh\x1b[3D")
	s.SetReflow(true)
	wantCursor(t, s, 1, 5)
	s.Resize(4, 6)
	wantLines(t, s, "0123", "4567", "89", "abcd", "efgh")
	wantCursor(t, s, 4, 1)
	s.Resize(10, 6)
	wantLines(t, s, "0123456789", "abcdefgh")
	wantCursor(t, s, 1, 5)

	// What no longer fits on the screen goes to the scrollback, and
	// comes back when there is room.
	s = feed(10, 3, "0123456789\r\nabcdefghij\r\nz")
	s.SetReflow(true)
	s.Resize(5, 3)
	wantLines(t, s, "abcde", "fghij", "z")
	wantScrollback(t, s, "01234", "56789")
	wantCursor(t, s, 2, 1)
	s.Resize(10, 3)
	wantLines(t, s, "0123456789", "abcdefghij", "z")
	wantScrollback(t, s)

	// The cursor in the blank space past the end of a line stays on
	// that line.
	s = feed(10, 3, "ab\x1b[9G")
	s.SetReflow(true)
	s.Resize(4, 3)
	wantLines(t, s, "ab")
	wantCursor(t, s, 0, 3)

	// tmux's rule: a line is as long as it was ever written. One that
	// was overwritten with less and erased to its end still wraps...
	s = feed(10, 4, "0123456789\rab\x1b[K\r\nnext")
	s.SetReflow(true)
	s.Resize(6, 4)
	wantLines(t, s, "ab", "", "next")
	// ...unless the whole row was erased before it was written again.
	s = feed(10, 4, "0123456789\r\x1b[2Kab\r\nnext")
	s.SetReflow(true)
	s.Resize(6, 4)
	wantLines(t, s, "ab", "next")
	s = feed(10, 4, "0123456789\r\x1b[Kab\r\nnext")
	s.SetReflow(true)
	s.Resize(6, 4)
	wantLines(t, s, "ab", "next")

	// A wide character is never split.
	s = feed(6, 3, "ab世界")
	s.SetReflow(true)
	s.Resize(3, 4)
	wantLines(t, s, "ab", "世", "界")
}

func TestDump(t *testing.T) {
	s := feed(5, 2, "one\r\ntwo\r\nthree")
	dump := s.Dump()
	for _, want := range []string{"5x2", "cursor at row 1 col 4", "|one", "|two  |", "|three|"} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump lacks %q:\n%s", want, dump)
		}
	}
}

// checkInvariants fails if the screen's own bookkeeping is broken.
func checkInvariants(t *testing.T, s *Screen) {
	t.Helper()
	cols, rows := s.Size()
	if len(s.lines) != rows {
		t.Fatalf("%d rows of cells on a screen %d rows high", len(s.lines), rows)
	}
	for r, l := range s.lines {
		if len(l.cells) != cols {
			t.Fatalf("row %d has %d cells on a screen %d columns wide", r, len(l.cells), cols)
		}
		for c, cell := range l.cells {
			switch {
			case cell.Width == 2 && (c+1 >= cols || l.cells[c+1].Width != 0):
				t.Fatalf("row %d col %d: a wide character without its second half", r, c)
			case cell.Width == 0 && (c == 0 || l.cells[c-1].Width != 2):
				t.Fatalf("row %d col %d: a second half without its wide character", r, c)
			}
		}
	}
	if row, col := s.Cursor(); row < 0 || row >= rows || col < 0 || col >= cols {
		t.Fatalf("cursor at row %d col %d is off a %dx%d screen", row, col, cols, rows)
	}
	if s.top < 0 || s.bot >= rows || s.top >= s.bot && rows > 1 {
		t.Fatalf("scroll region %d..%d on a screen %d rows high", s.top, s.bot, rows)
	}
}

// Random bytes, heavy on escape sequences, never break the screen.
func TestRandomInput(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	pieces := []string{
		"\x1b[", "\x1b]", "\x1b", "?", ";", ":", "$", " ", "!", ">", "<", "=",
		"0", "1", "2", "5", "7", "9", "25", "1049", "2026", "99999",
		"A", "B", "C", "D", "H", "J", "K", "L", "M", "P", "@", "X", "S", "T",
		"r", "m", "h", "l", "n", "c", "u", "s", "b", "d", "G", "p", "q", "t",
		"\r", "\n", "\b", "\t", "\x07", "\x18", "\x1b\\", "世", "e\u0301", "abc", "\xff", "\xe4",
		"\x1b7", "\x1b8", "\x1bM", "\x1bD", "\x1bE", "\x1bc", "\x1b#8",
	}
	for round := 0; round < 300; round++ {
		s := New(1+rng.Intn(12), 1+rng.Intn(6))
		s.SetReflow(rng.Intn(2) == 0)
		for i := 0; i < 200; i++ {
			if rng.Intn(40) == 0 {
				s.Resize(1+rng.Intn(12), 1+rng.Intn(6))
			} else {
				s.WriteString(pieces[rng.Intn(len(pieces))])
			}
			checkInvariants(t, s)
		}
		s.Dump()
	}
}

func FuzzScreen(f *testing.F) {
	f.Add([]byte("hello\x1b[2;2H\x1b[Kworld\x1b[?1049h"), uint8(10), uint8(4))
	f.Add([]byte("\x1b[1;1r\x1b[9999S\x1b[9999@世\x1b[?6h\x1b[5;5H"), uint8(3), uint8(2))
	f.Fuzz(func(t *testing.T, data []byte, cols, rows uint8) {
		s := New(1+int(cols%40), 1+int(rows%20))
		s.Write(data)
		checkInvariants(t, s)
		s.SetReflow(true)
		s.Resize(1+int(rows%40), 1+int(cols%20))
		checkInvariants(t, s)
		s.Write(data)
		checkInvariants(t, s)
		s.Dump()
	})
}

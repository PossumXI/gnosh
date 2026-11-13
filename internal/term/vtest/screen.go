// Package vtest is a terminal screen emulator for tests. It takes the bytes
// a program writes to a terminal and keeps what a person would see: the
// visible grid, the lines that scrolled off the top, the cursor, and each
// cell's colours and styles. It also answers the questions a program may
// ask a terminal, so that code which waits for answers can be tested.
//
// It follows xterm where terminals differ, because xterm's behaviour is the
// one that is written down (the control sequences document, ECMA-48, and
// the DEC manuals xterm follows), with these exceptions, each of which is
// what tmux was seen to do and each of which punishes a careless program:
// a combining mark written after a character in the last column while
// line wrapping is off joins the character to its left (see combine);
// SetScrollOnClear makes clearing the screen save it in the scrollback;
// and SetReflow makes Resize wrap lines again, counting a line as long as
// it was ever written. The tests in tmux_test.go compare the emulator
// with a real tmux, sequence by sequence.
package vtest

import (
	"fmt"
	"strings"
	"unicode"
)

// ColorKind says what sort of colour a Color is.
type ColorKind uint8

// The kinds of colour a cell can have.
const (
	// ColorDefault is the terminal's own foreground or background.
	ColorDefault ColorKind = iota
	// ColorIndexed is one of the 256 palette colours: Index.
	ColorIndexed
	// ColorRGB is a 24-bit colour: R, G and B.
	ColorRGB
)

// Color is a foreground, background or underline colour as it was written.
type Color struct {
	Kind    ColorKind
	Index   uint8
	R, G, B uint8
}

// Indexed is palette colour n.
func Indexed(n uint8) Color { return Color{Kind: ColorIndexed, Index: n} }

// RGB is a 24-bit colour.
func RGB(r, g, b uint8) Color { return Color{Kind: ColorRGB, R: r, G: g, B: b} }

// String shows the colour for a dump: "default", "5" or "#rrggbb".
func (c Color) String() string {
	switch c.Kind {
	case ColorIndexed:
		return fmt.Sprint(c.Index)
	case ColorRGB:
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return "default"
}

// Attr is how a cell is drawn.
type Attr struct {
	Bold, Dim, Italic, Underline, Blink, Reverse, Hidden, Strike bool
	// FG and BG are the text and background colours.
	FG, BG Color
	// Link is the address of the hyperlink the cell belongs to (OSC 8),
	// or empty.
	Link string
}

// Cell is one character cell of the screen.
type Cell struct {
	// Text is the character in the cell, with any combining marks that
	// were written after it. It is empty for a cell nothing was written
	// to (or that was erased), and for the second half of a wide
	// character.
	Text string
	// Width is 1 for an ordinary cell, 2 for the first half of a wide
	// character and 0 for its second half.
	Width int
	// Attr is the cell's colours and styles.
	Attr Attr
}

// line is one row of cells. wrapped is true when the text ran past its
// right edge and continued on the next row. used is how many cells from
// the left have been written to since the whole row was last erased; see
// resizeReflow.
type line struct {
	cells   []Cell
	wrapped bool
	used    int
}

// savedCursor is what ESC 7 keeps and ESC 8 brings back.
type savedCursor struct {
	row, col int
	attr     Attr
	origin   bool
	pending  bool
}

// Answers says which of a program's questions the emulated terminal
// answers, so that tests can stand in for terminals of different ages.
// The zero value is a terminal that answers nothing.
type Answers struct {
	// DeviceAttributes: CSI c is answered (every real terminal does).
	DeviceAttributes bool
	// CursorPosition: CSI 6 n and CSI ? 6 n are answered.
	CursorPosition bool
	// Modes: mode questions (DECRQM, CSI ? n $ p) are answered.
	Modes bool
	// Kitty: the kitty keyboard protocol is understood: the flags are
	// kept and CSI ? u is answered.
	Kitty bool
	// SynchronizedOutput: mode 2026 is known. Without it the mode is
	// reported as not recognized.
	SynchronizedOutput bool
}

// AllAnswers is a present-day terminal that answers everything.
var AllAnswers = Answers{DeviceAttributes: true, CursorPosition: true, Modes: true, Kitty: true, SynchronizedOutput: true}

// Screen is the emulated terminal. It is an io.Writer: write to it what
// the program under test writes to its terminal. It is not safe for use by
// several goroutines at once.
type Screen struct {
	cols, rows int
	width      func(string) int

	lines      []line // the visible grid
	scrollback []line // lines that scrolled off the top, oldest first
	altSaved   []line // the main grid, while the alternate screen shows

	row, col   int
	pending    bool // the deferred wrap: a character filled the last column
	attr       Attr
	saved      savedCursor
	top, bot   int // the scroll region, inclusive, counted from 0
	autowrap   bool
	origin     bool
	insertMode bool
	newline    bool // LNM: line feed also returns the carriage
	cursorOn   bool
	inAlt      bool
	reflow     bool
	clearSaves bool
	lastChar   rune
	modes      map[int]bool

	answers    Answers
	replies    []byte
	kittyStack []int
	kittyFlags int

	altEver, mouseEver bool
	title              string
	clipboard          []string
	strings            []string
	unhandled          []string
	bells              int
	syncBegins         int
	syncEnds           int
	writes             int

	p parser
}

// New makes a screen of the given size, with the cursor at the top left,
// that answers every question (AllAnswers) and measures characters with
// Width.
func New(cols, rows int) *Screen {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	s := &Screen{cols: cols, rows: rows, width: Width, answers: AllAnswers}
	s.reset()
	return s
}

// reset puts the screen in its power-on state, keeping its size.
func (s *Screen) reset() {
	s.lines = make([]line, s.rows)
	for i := range s.lines {
		s.lines[i] = s.blankLine()
	}
	s.scrollback = nil
	s.altSaved = nil
	s.row, s.col, s.pending = 0, 0, false
	s.attr = Attr{}
	s.saved = savedCursor{}
	s.top, s.bot = 0, s.rows-1
	s.autowrap = true
	s.origin = false
	s.insertMode = false
	s.newline = false
	s.cursorOn = true
	s.inAlt = false
	s.modes = map[int]bool{7: true, 25: true}
	s.kittyStack = nil
	s.kittyFlags = 0
	s.title = ""
}

// SetWidth replaces the function that says how many cells a character
// takes: 0 for a combining mark, 1, or 2. It is called with one character
// at a time.
func (s *Screen) SetWidth(width func(string) int) { s.width = width }

// SetAnswers chooses which questions the terminal answers.
func (s *Screen) SetAnswers(a Answers) { s.answers = a }

// SetReflow chooses what Resize does to lines that no longer fit. Off, the
// default, is xterm's way: nothing moves, and what does not fit is cut
// off. On is the way of most other terminals (iTerm2, Apple's Terminal,
// Ghostty, tmux): every line is wrapped again at the new width and the
// cursor stays on the character it was on.
//
// With it on, the screen also has tmux's idea of how long a line is: as
// far as it was ever written since the whole row was last erased. Text
// that was overwritten by something shorter and then erased "from the
// cursor to the end of the line" still counts, and such a line can wrap
// onto an empty row. A program that redraws rows in place keeps lines
// honest by erasing the whole row before writing it.
func (s *Screen) SetReflow(on bool) { s.reflow = on }

// SetScrollOnClear chooses what clearing the screen does. Off, the
// default, is xterm's way: what was on the screen is gone. On is tmux's
// way (its scroll-on-clear option, on unless the person turns it off):
// "erase the whole screen", and also "erase to the end of the screen" when
// the cursor is in the top left corner, first move the screen's lines into
// the scrollback. A program that wants text erased, not kept, must
// therefore never erase to the end of the screen from that corner.
func (s *Screen) SetScrollOnClear(on bool) { s.clearSaves = on }

// Size is the screen's size in cells.
func (s *Screen) Size() (cols, rows int) { return s.cols, s.rows }

func (s *Screen) blankCell() Cell { return Cell{Width: 1, Attr: Attr{BG: s.attr.BG}} }

func (s *Screen) blankLine() line {
	cells := make([]Cell, s.cols)
	blank := s.blankCell()
	for i := range cells {
		cells[i] = blank
	}
	return line{cells: cells}
}

// lineText is the text of a row with the blanks at its end removed.
func lineText(l line) string {
	var b strings.Builder
	for _, c := range l.cells {
		switch {
		case c.Width == 0:
		case c.Text == "":
			b.WriteByte(' ')
		default:
			b.WriteString(c.Text)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Lines is the text of each visible row, top to bottom, without the blanks
// at the end of a row.
func (s *Screen) Lines() []string {
	out := make([]string, len(s.lines))
	for i, l := range s.lines {
		out[i] = lineText(l)
	}
	return out
}

// Line is the text of visible row row, counted from 0.
func (s *Screen) Line(row int) string {
	if row < 0 || row >= len(s.lines) {
		return ""
	}
	return lineText(s.lines[row])
}

// Scrollback is the text of the lines that scrolled off the top of the
// screen, oldest first.
func (s *Screen) Scrollback() []string {
	out := make([]string, len(s.scrollback))
	for i, l := range s.scrollback {
		out[i] = lineText(l)
	}
	return out
}

// AllLines is the scrollback followed by the visible rows, without the
// empty rows at the bottom of the screen.
func (s *Screen) AllLines() []string {
	out := append(s.Scrollback(), s.Lines()...)
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// Cursor is where the cursor is, counted from 0. After a character has
// filled the last column the cursor is still on that column.
func (s *Screen) Cursor() (row, col int) { return s.row, s.col }

// CursorVisible reports whether the cursor is shown (mode 25).
func (s *Screen) CursorVisible() bool { return s.cursorOn }

// WrapPending reports whether a character has filled the last column, so
// that the next one will begin a new line.
func (s *Screen) WrapPending() bool { return s.pending }

// Cell is the cell at a visible row and column, counted from 0.
func (s *Screen) Cell(row, col int) Cell {
	if row < 0 || row >= len(s.lines) || col < 0 || col >= s.cols {
		return Cell{}
	}
	return s.lines[row].cells[col]
}

// Wrapped reports whether the text of a visible row ran on into the next.
func (s *Screen) Wrapped(row int) bool {
	return row >= 0 && row < len(s.lines) && s.lines[row].wrapped
}

// Mode reports whether a DEC private mode (the kind set with CSI ? n h) is
// set.
func (s *Screen) Mode(n int) bool { return s.modes[n] }

// AltScreenEver reports whether the alternate screen was ever switched to.
func (s *Screen) AltScreenEver() bool { return s.altEver }

// MouseEver reports whether any mouse reporting mode was ever turned on.
func (s *Screen) MouseEver() bool { return s.mouseEver }

// Title is the window title last set (OSC 0 or OSC 2).
func (s *Screen) Title() string { return s.title }

// Clipboard is every text written to the clipboard (OSC 52), in order.
func (s *Screen) Clipboard() []string { return s.clipboard }

// Strings is every OSC, DCS, APC, PM and SOS string received that the
// screen did not act on, each with its opening character first.
func (s *Screen) Strings() []string { return s.strings }

// Unhandled is every control sequence received that the screen does not
// know. A test of a program that should write only what it means to can
// assert that this is empty.
func (s *Screen) Unhandled() []string { return s.unhandled }

// Bells is how many times the bell rang.
func (s *Screen) Bells() int { return s.bells }

// SyncUpdates is how many synchronized updates began and how many ended
// (mode 2026 set and reset).
func (s *Screen) SyncUpdates() (begun, ended int) { return s.syncBegins, s.syncEnds }

// KittyFlags is the kitty keyboard flags in force, and how many entries
// are on the stack.
func (s *Screen) KittyFlags() (flags, depth int) { return s.kittyFlags, len(s.kittyStack) }

// Writes is how many times Write was called.
func (s *Screen) Writes() int { return s.writes }

// TakeReplies returns the bytes the terminal has sent back to the program
// since the last call: its answers to questions.
func (s *Screen) TakeReplies() []byte {
	r := s.replies
	s.replies = nil
	return r
}

func (s *Screen) reply(format string, args ...any) {
	s.replies = append(s.replies, fmt.Sprintf(format, args...)...)
}

// Dump draws the scrollback and the screen for a failed assertion, with
// the cursor's place and the modes that matter.
func (s *Screen) Dump() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%dx%d, cursor at row %d col %d", s.cols, s.rows, s.row, s.col)
	if !s.cursorOn {
		b.WriteString(" (hidden)")
	}
	if s.pending {
		b.WriteString(" (wrap pending)")
	}
	b.WriteByte('\n')
	for i, l := range s.Scrollback() {
		fmt.Fprintf(&b, "  ^%-3d |%s\n", len(s.scrollback)-i, l)
	}
	b.WriteString("       +" + strings.Repeat("-", s.cols) + "+\n")
	for i, l := range s.lines {
		text := lineText(l)
		pad := s.cols - len([]rune(text))
		if pad < 0 {
			pad = 0
		}
		mark := " "
		if i == s.row {
			mark = ">"
		}
		fmt.Fprintf(&b, "  %s%-3d |%s%s|\n", mark, i, text, strings.Repeat(" ", pad))
	}
	b.WriteString("       +" + strings.Repeat("-", s.cols) + "+\n")
	return b.String()
}

// Width is a plain measure of how many cells a character takes: 0 for
// combining marks and other characters that take no room, 2 for East Asian
// wide characters and emoji, 1 for everything else. For a longer string it
// is the sum over its characters. It is good enough for tests; gnosh's own
// measure is internal/text's.
func Width(str string) int {
	n := 0
	for _, r := range str {
		n += runeWidth(r)
	}
	return n
}

func runeWidth(r rune) int {
	switch {
	case r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r < 0x300:
		return 1
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r):
		return 0
	case r >= 0x1100 && r <= 0x115f,
		r >= 0x2e80 && r <= 0x303e,
		r >= 0x3041 && r <= 0x33ff,
		r >= 0x3400 && r <= 0x4dbf,
		r >= 0x4e00 && r <= 0x9fff,
		r >= 0xa000 && r <= 0xa4cf,
		r >= 0xac00 && r <= 0xd7a3,
		r >= 0xf900 && r <= 0xfaff,
		r >= 0xfe30 && r <= 0xfe4f,
		r >= 0xff00 && r <= 0xff60,
		r >= 0xffe0 && r <= 0xffe6,
		r >= 0x1f300 && r <= 0x1f64f,
		r >= 0x1f900 && r <= 0x1f9ff,
		r >= 0x20000 && r <= 0x3fffd:
		return 2
	}
	return 1
}

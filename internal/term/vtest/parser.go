package vtest

import (
	"encoding/base64"
	"strings"
	"unicode/utf8"
)

// The parser follows the state diagram DEC terminals use (Paul Williams'
// "A parser for DEC's ANSI-compatible video terminals", vt100.net), cut
// down to what a program is likely to write, with UTF-8 decoding added in
// the ground state.

const (
	stGround = iota
	stEscape
	stCSI
	stString
)

// parser is where the screen is in the middle of a control sequence.
type parser struct {
	state  int
	params []byte // a CSI sequence's parameter bytes
	inter  []byte // intermediate bytes of an escape or CSI sequence
	ignore bool   // the CSI sequence is malformed and will be dropped
	kind   byte   // the byte that began a string: ] P _ ^ X
	str    []byte // the body of a string
	utf    []byte // the first bytes of a UTF-8 character
	raw    []byte // the whole sequence so far, for Unhandled
}

// Write takes bytes written to the terminal. It never fails.
func (s *Screen) Write(p []byte) (int, error) {
	s.writes++
	for _, b := range p {
		s.step(b)
	}
	return len(p), nil
}

// WriteString is Write for a string.
func (s *Screen) WriteString(str string) (int, error) {
	return s.Write([]byte(str))
}

// flushUTF prints what is left of a UTF-8 character that was cut short.
func (s *Screen) flushUTF() {
	if len(s.p.utf) > 0 {
		s.p.utf = s.p.utf[:0]
		s.put(utf8.RuneError)
	}
}

func (s *Screen) step(b byte) {
	p := &s.p
	// CAN and SUB end any sequence; ESC begins a new one from anywhere.
	switch b {
	case 0x18, 0x1a:
		s.flushUTF()
		p.state = stGround
		return
	case 0x1b:
		s.flushUTF()
		if p.state == stString {
			s.endString()
		}
		p.state = stEscape
		p.params, p.inter, p.ignore = p.params[:0], p.inter[:0], false
		p.raw = append(p.raw[:0], b)
		return
	}
	if p.state != stGround {
		p.raw = append(p.raw, b)
	}
	switch p.state {
	case stGround:
		switch {
		case b < 0x20:
			s.flushUTF()
			s.control(b)
		case b == 0x7f:
			s.flushUTF()
		case b < 0x80:
			s.flushUTF()
			s.put(rune(b))
		default:
			p.utf = append(p.utf, b)
			for len(p.utf) > 0 && utf8.FullRune(p.utf) {
				r, size := utf8.DecodeRune(p.utf)
				p.utf = p.utf[:copy(p.utf, p.utf[size:])]
				if r >= 0x80 && r < 0xa0 {
					continue // a C1 control: not printed
				}
				s.put(r)
			}
		}
	case stEscape:
		switch {
		case b < 0x20:
			s.control(b)
		case b <= 0x2f:
			p.inter = append(p.inter, b)
		case b == 0x7f:
		case len(p.inter) > 0:
			s.escape(b)
			p.state = stGround
		case b == '[':
			p.state = stCSI
		case b == ']', b == 'P', b == '_', b == '^', b == 'X':
			p.state = stString
			p.kind = b
			p.str = p.str[:0]
		default:
			s.escape(b)
			p.state = stGround
		}
	case stCSI:
		switch {
		case b < 0x20:
			s.control(b)
		case b <= 0x2f:
			p.inter = append(p.inter, b)
		case b <= 0x3f:
			if len(p.inter) > 0 {
				p.ignore = true
			} else {
				p.params = append(p.params, b)
			}
		case b <= 0x7e:
			if p.ignore {
				s.unknown()
			} else {
				s.csi(b)
			}
			p.state = stGround
		case b == 0x7f:
		default:
			// A byte that cannot be in a sequence: drop the
			// sequence and take the byte as text.
			s.unknown()
			p.state = stGround
			s.step(b)
		}
	case stString:
		switch {
		case b == 0x07:
			s.endString()
			p.state = stGround
		case b < 0x20:
		default:
			p.str = append(p.str, b)
		}
	}
}

// unknown records the sequence in progress as one the screen does not
// know.
func (s *Screen) unknown() {
	s.unhandled = append(s.unhandled, string(s.p.raw))
}

// control carries out a C0 control character.
func (s *Screen) control(b byte) {
	switch b {
	case 0x07:
		s.bells++
	case 0x08:
		s.setCol(s.col - 1)
	case 0x09:
		s.tab()
	case 0x0a, 0x0b, 0x0c:
		s.lineFeed()
		if s.newline {
			s.col = 0
		}
		s.pending = false
	case 0x0d:
		s.setCol(0)
	}
}

// escape carries out a sequence of ESC, perhaps intermediate bytes, and a
// final byte.
func (s *Screen) escape(b byte) {
	if len(s.p.inter) > 0 {
		switch s.p.inter[0] {
		case '(', ')', '*', '+', '-', '.', '/', ' ':
			// Character set and 7/8-bit control choices: nothing to do.
		case '#':
			if b == '8' {
				// DECALN: fill the screen with E.
				for r := range s.lines {
					for c := range s.lines[r].cells {
						s.lines[r].cells[c] = Cell{Text: "E", Width: 1}
					}
				}
				s.top, s.bot = 0, s.rows-1
				s.row, s.col, s.pending = 0, 0, false
				return
			}
			s.unknown()
		default:
			s.unknown()
		}
		return
	}
	switch b {
	case '7':
		s.saveCursor()
	case '8':
		s.restoreCursor()
	case 'D':
		s.lineFeed()
		s.pending = false
	case 'E':
		s.col = 0
		s.lineFeed()
		s.pending = false
	case 'M':
		s.reverseIndex()
		s.pending = false
	case 'c':
		s.reset()
	case 'Z':
		s.deviceAttributes()
	case 'H', '=', '>', '\\':
		// Set a tab stop, keypad modes, and the end of a string.
	default:
		s.unknown()
	}
}

// endString acts on a finished OSC, DCS, APC, PM or SOS string.
func (s *Screen) endString() {
	body := string(s.p.str)
	s.p.str = s.p.str[:0]
	if s.p.kind != ']' {
		s.strings = append(s.strings, string(s.p.kind)+body)
		return
	}
	code, rest, _ := strings.Cut(body, ";")
	switch code {
	case "0", "2":
		s.title = rest
	case "1":
		// The icon name: nothing shows it.
	case "8":
		// A hyperlink: OSC 8 ; params ; address. An empty address
		// ends the link.
		_, uri, _ := strings.Cut(rest, ";")
		s.attr.Link = uri
	case "52":
		_, data, _ := strings.Cut(rest, ";")
		if text, err := base64.StdEncoding.DecodeString(data); err == nil {
			s.clipboard = append(s.clipboard, string(text))
		} else {
			s.strings = append(s.strings, "]"+body)
		}
	default:
		s.strings = append(s.strings, "]"+body)
	}
}

func (s *Screen) deviceAttributes() {
	if s.answers.DeviceAttributes {
		s.reply("\x1b[?62;22c")
	}
}

// fields splits a CSI sequence's parameter bytes: fields at ";", the parts
// of a field at ":". A part left out is -1.
func fields(b []byte) [][]int {
	if len(b) == 0 {
		return nil
	}
	out := [][]int{nil}
	n, have := 0, false
	push := func() {
		v := -1
		if have {
			v = n
		}
		out[len(out)-1] = append(out[len(out)-1], v)
		n, have = 0, false
	}
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
			have = true
			if n < 1<<20 {
				n = n*10 + int(c-'0')
			}
		case c == ':':
			push()
		case c == ';':
			push()
			out = append(out, nil)
		}
	}
	push()
	return out
}

// csi carries out a CSI sequence that ended with the byte final.
func (s *Screen) csi(final byte) {
	ps := s.p.params
	var prefix byte
	if len(ps) > 0 && ps[0] >= '<' && ps[0] <= '?' {
		prefix = ps[0]
		ps = ps[1:]
	}
	f := fields(ps)
	// arg is parameter i, or def when it was left out.
	arg := func(i, def int) int {
		if i >= len(f) || f[i][0] < 0 {
			return def
		}
		return f[i][0]
	}
	// count is parameter 0 as a number of times: left out or zero is one.
	count := func() int {
		if n := arg(0, 1); n > 0 {
			return min(n, 1<<16)
		}
		return 1
	}
	inter := string(s.p.inter)

	switch {
	case prefix == 0 && inter == "":
		switch final {
		case 'A':
			s.up(count())
		case 'B', 'e':
			s.down(count())
		case 'C', 'a':
			s.setCol(s.col + count())
		case 'D':
			s.setCol(s.col - count())
		case 'E':
			s.down(count())
			s.col = 0
		case 'F':
			s.up(count())
			s.col = 0
		case 'G', '`':
			s.setCol(count() - 1)
		case 'd':
			s.setRow(count() - 1)
		case 'H', 'f':
			s.setRow(max(arg(0, 1), 1) - 1)
			s.setCol(max(arg(1, 1), 1) - 1)
		case 'I':
			for n := count(); n > 0; n-- {
				s.tab()
			}
		case 'Z':
			for n := count(); n > 0; n-- {
				s.backTab()
			}
		case 'J':
			s.eraseDisplay(arg(0, 0))
		case 'K':
			s.eraseLine(arg(0, 0))
		case 'L':
			s.insertLines(count())
		case 'M':
			s.deleteLines(count())
		case 'P':
			s.deleteChars(count())
		case '@':
			s.insertChars(count())
		case 'X':
			s.eraseCells(s.row, s.col, s.col+count())
			s.pending = false
		case 'S':
			s.scrollUp(count())
		case 'T':
			if len(f) > 1 {
				s.unknown()
				return
			}
			s.scrollDown(count())
		case 'b':
			if s.lastChar != 0 {
				for n := count(); n > 0; n-- {
					s.put(s.lastChar)
				}
			}
		case 'm':
			s.sgr(f)
		case 'r':
			top, bot := max(arg(0, 1), 1)-1, arg(1, s.rows)-1
			if bot < 0 || bot >= s.rows {
				bot = s.rows - 1
			}
			if top < bot {
				s.top, s.bot = top, bot
				s.setRow(0)
				s.setCol(0)
			}
		case 's':
			if len(f) > 0 {
				s.unknown()
				return
			}
			s.saveCursor()
		case 'u':
			if len(f) > 0 {
				s.unknown()
				return
			}
			s.restoreCursor()
		case 'h', 'l':
			for i := range f {
				switch arg(i, 0) {
				case 4:
					s.insertMode = final == 'h'
				case 20:
					s.newline = final == 'h'
				default:
					s.unknown()
				}
			}
		case 'n':
			switch arg(0, 0) {
			case 5:
				if s.answers.DeviceAttributes {
					s.reply("\x1b[0n")
				}
			case 6:
				if s.answers.CursorPosition {
					s.reply("\x1b[%d;%dR", s.reportRow(), s.col+1)
				}
			default:
				s.unknown()
			}
		case 'c':
			if arg(0, 0) == 0 {
				s.deviceAttributes()
			}
		case 't':
			if arg(0, 0) == 18 && s.answers.DeviceAttributes {
				s.reply("\x1b[8;%d;%dt", s.rows, s.cols)
			}
		case 'g':
			// Clearing tab stops: the stops here are fixed.
		default:
			s.unknown()
		}
	case prefix == '?' && inter == "":
		switch final {
		case 'h', 'l':
			for i := range f {
				s.setMode(arg(i, 0), final == 'h')
			}
		case 'n':
			if arg(0, 0) == 6 {
				if s.answers.CursorPosition {
					s.reply("\x1b[?%d;%dR", s.reportRow(), s.col+1)
				}
			} else {
				s.unknown()
			}
		case 'u':
			if s.answers.Kitty {
				s.reply("\x1b[?%du", s.kittyFlags)
			}
		case 'J':
			s.eraseDisplay(arg(0, 0))
		case 'K':
			s.eraseLine(arg(0, 0))
		default:
			s.unknown()
		}
	case prefix == '?' && inter == "$" && final == 'p':
		if !s.answers.Modes {
			return
		}
		mode := arg(0, 0)
		state := 0
		if _, known := knownModes[mode]; known && (mode != 2026 || s.answers.SynchronizedOutput) {
			state = 2
			if s.modes[mode] {
				state = 1
			}
		}
		s.reply("\x1b[?%d;%d$y", mode, state)
	case prefix == 0 && inter == "$" && final == 'p':
		if !s.answers.Modes {
			return
		}
		mode := arg(0, 0)
		state := 0
		switch mode {
		case 4:
			state = 2
			if s.insertMode {
				state = 1
			}
		case 20:
			state = 2
			if s.newline {
				state = 1
			}
		}
		s.reply("\x1b[%d;%d$y", mode, state)
	case prefix == '>' && inter == "":
		switch final {
		case 'c':
			if s.answers.DeviceAttributes {
				s.reply("\x1b[>1;10;0c")
			}
		case 'q':
			if s.answers.DeviceAttributes {
				s.reply("\x1bP>|vtest\x1b\\")
			}
		case 'u':
			if s.answers.Kitty {
				s.kittyStack = append(s.kittyStack, s.kittyFlags)
				s.kittyFlags = arg(0, 0)
			}
		default:
			s.unknown()
		}
	case prefix == '<' && inter == "" && final == 'u':
		if s.answers.Kitty {
			for n := count(); n > 0; n-- {
				if len(s.kittyStack) == 0 {
					s.kittyFlags = 0
					break
				}
				s.kittyFlags = s.kittyStack[len(s.kittyStack)-1]
				s.kittyStack = s.kittyStack[:len(s.kittyStack)-1]
			}
		}
	case prefix == '=' && inter == "" && final == 'u':
		if s.answers.Kitty {
			switch flags := arg(0, 0); arg(1, 1) {
			case 1:
				s.kittyFlags = flags
			case 2:
				s.kittyFlags |= flags
			case 3:
				s.kittyFlags &^= flags
			}
		}
	case prefix == 0 && inter == " " && final == 'q':
		// DECSCUSR, the cursor's shape: nothing here draws it.
	case prefix == 0 && inter == "!" && final == 'p':
		// DECSTR, the soft reset.
		s.attr = Attr{}
		s.saved = savedCursor{}
		s.top, s.bot = 0, s.rows-1
		s.origin, s.insertMode, s.autowrap, s.cursorOn = false, false, true, true
		s.modes[6], s.modes[7], s.modes[25] = false, true, true
		s.pending = false
	default:
		s.unknown()
	}
}

// reportRow is the cursor's row as a position report gives it: counted
// from 1, and from the top of the scroll region in origin mode.
func (s *Screen) reportRow() int {
	if s.origin {
		return s.row - s.top + 1
	}
	return s.row + 1
}

// knownModes are the DEC private modes the screen keeps. The value is
// true for the mouse reporting modes.
var knownModes = map[int]bool{
	1: false, 5: false, 6: false, 7: false, 12: false, 25: false,
	47: false, 1004: false, 1047: false, 1048: false, 1049: false,
	2004: false, 2026: false,
	9: true, 1000: true, 1001: true, 1002: true, 1003: true, 1005: true,
	1006: true, 1015: true, 1016: true,
}

// setMode sets or resets a DEC private mode.
func (s *Screen) setMode(mode int, on bool) {
	mouse, known := knownModes[mode]
	if !known {
		s.unknown()
		return
	}
	if mouse && on {
		s.mouseEver = true
	}
	switch mode {
	case 6:
		s.origin = on
		s.setRow(0)
		s.setCol(0)
	case 7:
		s.autowrap = on
		if !on {
			s.pending = false
		}
	case 25:
		s.cursorOn = on
	case 47, 1047:
		s.useAlt(on)
	case 1048:
		if on {
			s.saveCursor()
		} else {
			s.restoreCursor()
		}
	case 1049:
		if on {
			s.saveCursor()
			s.useAlt(true)
			s.eraseDisplay(2)
		} else {
			s.useAlt(false)
			s.restoreCursor()
		}
	case 2026:
		if !s.answers.SynchronizedOutput {
			// A terminal that does not know the mode ignores it.
			return
		}
		if on {
			s.syncBegins++
		} else {
			s.syncEnds++
		}
	}
	s.modes[mode] = on
}

// sgr sets colours and styles (SGR).
func (s *Screen) sgr(f [][]int) {
	if len(f) == 0 {
		f = [][]int{{0}}
	}
	a := &s.attr
	for i := 0; i < len(f); i++ {
		p := f[i]
		code := max(p[0], 0)
		switch {
		case code == 0:
			*a = Attr{Link: a.Link}
		case code == 1:
			a.Bold = true
		case code == 2:
			a.Dim = true
		case code == 3:
			a.Italic = true
		case code == 4:
			a.Underline = len(p) < 2 || p[1] != 0
		case code == 5, code == 6:
			a.Blink = true
		case code == 7:
			a.Reverse = true
		case code == 8:
			a.Hidden = true
		case code == 9:
			a.Strike = true
		case code == 21:
			a.Underline = true
		case code == 22:
			a.Bold, a.Dim = false, false
		case code == 23:
			a.Italic = false
		case code == 24:
			a.Underline = false
		case code == 25:
			a.Blink = false
		case code == 27:
			a.Reverse = false
		case code == 28:
			a.Hidden = false
		case code == 29:
			a.Strike = false
		case code >= 30 && code <= 37:
			a.FG = Indexed(uint8(code - 30))
		case code == 39:
			a.FG = Color{}
		case code >= 40 && code <= 47:
			a.BG = Indexed(uint8(code - 40))
		case code == 49:
			a.BG = Color{}
		case code >= 90 && code <= 97:
			a.FG = Indexed(uint8(code - 90 + 8))
		case code >= 100 && code <= 107:
			a.BG = Indexed(uint8(code - 100 + 8))
		case code == 38, code == 48, code == 58:
			var c Color
			var ok bool
			if len(p) > 1 {
				c, ok = extendedColor(p[1:])
			} else {
				var used int
				c, used, ok = extendedColorFields(f[i+1:])
				i += used
			}
			if ok {
				switch code {
				case 38:
					a.FG = c
				case 48:
					a.BG = c
				}
			}
		case code == 59:
			// The underline colour is not kept.
		case code >= 10 && code <= 20, code == 26, code >= 50 && code <= 55, code >= 60 && code <= 65, code >= 73 && code <= 75:
			// Fonts, proportional spacing, frames, overline, ideogram
			// marks and super- and subscript: not kept.
		default:
			s.unknown()
		}
	}
}

// extendedColor reads the parts after 38, 48 or 58 written with colons:
// 5:n, or 2:r:g:b, or 2:colourspace:r:g:b.
func extendedColor(p []int) (Color, bool) {
	b := func(v int) uint8 { return uint8(min(max(v, 0), 255)) }
	switch {
	case len(p) >= 2 && p[0] == 5:
		return Indexed(b(p[1])), true
	case len(p) >= 5 && p[0] == 2:
		return RGB(b(p[2]), b(p[3]), b(p[4])), true
	case len(p) == 4 && p[0] == 2:
		return RGB(b(p[1]), b(p[2]), b(p[3])), true
	}
	return Color{}, false
}

// extendedColorFields reads the fields after 38, 48 or 58 written with
// semicolons: 5;n or 2;r;g;b. It says how many fields it used.
func extendedColorFields(f [][]int) (Color, int, bool) {
	b := func(i int) uint8 { return uint8(min(max(f[i][0], 0), 255)) }
	switch {
	case len(f) >= 2 && f[0][0] == 5:
		return Indexed(b(1)), 2, true
	case len(f) >= 4 && f[0][0] == 2:
		return RGB(b(1), b(2), b(3)), 4, true
	}
	return Color{}, len(f), false
}

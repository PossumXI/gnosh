package term

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// rline is one line the renderer draws, made safe.
type rline struct {
	// raw is the line as the caller gave it.
	raw string
	// text is what is written to the terminal: raw without anything that
	// could move the cursor, cut to the terminal's width, with its
	// styles and links closed.
	text string
	// plain is text without its escape sequences.
	plain string
	// width is how many cells plain takes.
	width int
}

// The kinds of escape sequence a line may hold.
const (
	seqOther = iota // anything the renderer does not let through
	seqStyle        // SGR: colours and styles
	seqLink         // OSC 8: a hyperlink's start or end
)

const linkEnd = "\x1b]8;;\x1b\\"

// scanEscape reads the escape sequence at the start of s, which begins
// with ESC, and says what kind it is. A sequence that is cut short or
// broken comes back as seqOther, as far as it goes.
func scanEscape(s string) (seq string, kind int) {
	if len(s) == 1 {
		return s, seqOther
	}
	switch s[1] {
	case '[':
		j := 2
		for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
			j++
		}
		pEnd := j
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j == len(s) || s[j] < 0x40 || s[j] > 0x7e {
			return s[:j], seqOther
		}
		if s[j] == 'm' && j == pEnd && strings.Trim(s[2:pEnd], "0123456789;:") == "" {
			return s[:j+1], seqStyle
		}
		return s[:j+1], seqOther
	case ']', 'P', '_', '^', 'X':
		for j := 2; j < len(s); j++ {
			switch c := s[j]; {
			case c == 0x07:
				return stringSeq(s[:j+1])
			case c == 0x1b:
				if j+1 < len(s) && s[j+1] == '\\' {
					return stringSeq(s[:j+2])
				}
				return s[:j], seqOther
			case c < 0x20 || c == 0x7f:
				return s[:j], seqOther
			}
		}
		return s, seqOther
	}
	j := 1
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
		j++
	}
	if j < len(s) && s[j] >= 0x30 && s[j] <= 0x7e {
		j++
	}
	return s[:j], seqOther
}

// stringSeq says what kind a complete string sequence is. A hyperlink
// whose address holds bytes that are not UTF-8, or C1 controls, is not let
// through: a terminal set to an 8-bit character set could read such a
// byte as the end of the string and what follows as a sequence of its own.
func stringSeq(seq string) (string, int) {
	if strings.HasPrefix(seq, "\x1b]8;") {
		body := strings.TrimSuffix(strings.TrimSuffix(seq[2:], "\x07"), "\x1b\\")
		if isPlain(body) {
			return seq, seqLink
		}
	}
	return seq, seqOther
}

// isPlain reports whether s is nothing but printable text: valid UTF-8
// with no control characters.
func isPlain(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c == 0x7f {
			return false
		}
		if c == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] < 0xa0 {
			return false
		}
	}
	return utf8.ValidString(s)
}

// cleanText makes a run of bytes with no C0 controls into printable text:
// bytes that are not UTF-8 become U+FFFD and C1 controls are dropped.
func cleanText(s string) string {
	if isPlain(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if !isC1(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// prepare makes a line safe to draw at the renderer's width.
//
// Only text, colours and styles (SGR) and hyperlinks (OSC 8) are let
// through. Every other control character and escape sequence is dropped,
// because any of them could move the cursor or change a mode, and the
// renderer's bookkeeping depends on a line being exactly one row. A tab
// becomes a space. Text past the terminal's width is cut off.
func (r *Renderer) prepare(raw string) rline {
	if isPlain(raw) {
		if w := r.measure(raw); w <= r.cols {
			text := raw
			if w == r.cols {
				text = r.guardEdge(raw, raw)
			}
			return rline{raw: raw, text: text, plain: raw, width: w}
		}
	}
	var text, plain strings.Builder
	room := r.cols
	styled, linked, cut := false, false, false
	for i := 0; i < len(raw) && !cut; {
		c := raw[i]
		switch {
		case c == 0x1b:
			seq, kind := scanEscape(raw[i:])
			i += len(seq)
			switch kind {
			case seqStyle:
				text.WriteString(seq)
				styled = true
			case seqLink:
				text.WriteString(seq)
				linked = true
			}
		case c == '\t':
			i++
			if room > 0 {
				text.WriteByte(' ')
				plain.WriteByte(' ')
				room--
			} else {
				cut = true
			}
		case c < 0x20 || c == 0x7f:
			i++
		default:
			j := i
			for j < len(raw) && raw[j] >= 0x20 && raw[j] != 0x7f {
				j++
			}
			run := cleanText(raw[i:j])
			i = j
			w := r.measure(run)
			if w > room {
				run = r.fit(run, room)
				w = r.measure(run)
				cut = true
			}
			text.WriteString(run)
			plain.WriteString(run)
			room -= w
		}
	}
	if styled {
		text.WriteString(StyleReset)
	}
	if linked {
		text.WriteString(linkEnd)
	}
	out := text.String()
	if room == 0 {
		out = r.guardEdge(out, plain.String())
	}
	return rline{raw: raw, text: out, plain: plain.String(), width: r.cols - room}
}

// guardEdge looks after an accent in the last column. text is a line that
// fills the terminal's width and plain is the same without its escape
// sequences.
//
// The renderer draws with the terminal's own wrapping off. A terminal in
// that state leaves the cursor on top of a character written in the last
// column, and some (tmux is one) then join a combining mark that follows
// to the character on the cursor's left: the accent lands on the wrong
// letter. So when the line ends in a letter one cell wide followed by
// combining marks, wrapping is turned back on for just those characters.
// One narrow character cannot wrap, wherever the terminal thinks the
// cursor is, so the protection that drawing with wrapping off gives is
// kept. Variation selectors are left alone: a terminal may take one to
// mean that its character is two cells wide, and wrap.
func (r *Renderer) guardEdge(text, plain string) string {
	end := len(plain)
	for end > 0 {
		ch, size := utf8.DecodeLastRuneInString(plain[:end])
		if !isAccent(ch) || r.measure(string(ch)) != 0 {
			break
		}
		end -= size
	}
	if end == len(plain) || end == 0 {
		return text
	}
	base, size := utf8.DecodeLastRuneInString(plain[:end])
	if r.measure(string(base)) != 1 {
		return text
	}
	tail := plain[end-size:]
	i := strings.LastIndex(text, tail)
	if i < 0 {
		return text
	}
	// Only escape sequences may follow the tail.
	for rest := text[i+len(tail):]; rest != ""; {
		if rest[0] != 0x1b {
			return text
		}
		seq, _ := scanEscape(rest)
		rest = rest[len(seq):]
	}
	return text[:i] + AutoWrapOn + tail + AutoWrapOff + text[i+len(tail):]
}

// isAccent reports whether ch is a combining mark that is drawn on the
// character before it, other than a variation selector.
func isAccent(ch rune) bool {
	if ch >= 0xfe00 && ch <= 0xfe0f || ch >= 0xe0100 && ch <= 0xe01ef {
		return false
	}
	return unicode.Is(unicode.Mn, ch)
}

// measure is the caller's width function, never negative.
func (r *Renderer) measure(s string) int {
	return max(r.width(s), 0)
}

// fit is the longest beginning of s that takes no more than room cells.
// It never cuts a character in half; it may cut between a character and a
// mark that combines with it.
func (r *Renderer) fit(s string, room int) string {
	used, end := 0, 0
	for i, ch := range s {
		w := r.measure(string(ch))
		if used+w > room {
			break
		}
		used += w
		end = i + utf8.RuneLen(ch)
	}
	// The caller's measure has the last word: it may count a run of
	// characters differently from the sum of its parts.
	for end > 0 && r.measure(s[:end]) > room {
		_, size := utf8.DecodeLastRuneInString(s[:end])
		end -= size
	}
	return s[:end]
}

// wrapRows says how many rows a line takes when a terminal wraps it at
// cols cells, and which of those rows holds the cell numbered at (counted
// from 0). A cell past the end of the text is on the last row.
func (r *Renderer) wrapRows(plain string, cols, at int) (rows, atRow int) {
	rows, atRow = 1, -1
	col, cell := 0, 0
	for _, ch := range plain {
		w := min(r.measure(string(ch)), cols)
		if w == 0 {
			continue
		}
		if col+w > cols {
			rows++
			col = 0
		}
		if atRow < 0 && cell+w > at {
			atRow = rows - 1
		}
		col += w
		cell += w
	}
	if atRow < 0 {
		atRow = rows - 1
	}
	return rows, atRow
}

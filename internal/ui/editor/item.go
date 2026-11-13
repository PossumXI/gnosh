package editor

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// How the editor keeps its text.
//
// The text is a list of lines, each a string with no line feed in it. The
// cursor and every edit move by items. An item is one of two things: a
// grapheme cluster (what a reader takes for one character, as
// internal/text splits them), or a collapsed paste, which holds its whole
// text and still counts as a single step and a single delete.
//
// A collapsed paste is written into the line as its text between two
// marks, with the line feeds inside it written as a third mark, so that a
// line never holds a real line feed and can always be split and joined on
// "\n". The marks are three of the code points Unicode sets aside for a
// program's own use and promises never to give a meaning (U+FDD0 to
// U+FDEF). Text from outside is cleaned on the way in, and any of those
// code points in it becomes U+FFFD, so a mark in a line is always one the
// editor wrote.
const (
	pasteOpen  = "\uFDD0" // begins a collapsed paste
	pasteClose = "\uFDD1" // ends it
	pasteBreak = "\uFDD2" // a line feed inside it

	// marksLead is the two bytes all three marks begin with in UTF-8,
	// which few other characters share (only U+FDC0 to U+FDFF).
	marksLead = "\xef\xb7"
)

// clean makes text from outside fit to keep: valid UTF-8, "\n" for every
// kind of line end, and none of the code points the editor uses as marks.
// Nothing else is changed: tabs, control characters and escape sequences
// stay in the text and are made harmless when they are drawn.
func clean(s string) string {
	if strings.IndexByte(s, '\r') < 0 && !strings.Contains(s, marksLead) && utf8.ValidString(s) {
		return s
	}
	s = strings.ToValidUTF8(s, "\uFFFD")
	if strings.IndexByte(s, '\r') >= 0 {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
	}
	if strings.Contains(s, marksLead) {
		s = strings.Map(func(r rune) rune {
			if r >= 0xFDD0 && r <= 0xFDEF {
				return utf8.RuneError
			}
			return r
		}, s)
	}
	return s
}

// encodePaste writes cleaned text as a collapsed paste.
func encodePaste(content string) string {
	return pasteOpen + strings.ReplaceAll(content, "\n", pasteBreak) + pasteClose
}

// pasteContent is the text a collapsed paste stands for.
func pasteContent(item string) string {
	return strings.ReplaceAll(item[len(pasteOpen):len(item)-len(pasteClose)], pasteBreak, "\n")
}

// appendPlain appends to b the text that s, text as the editor keeps it,
// stands for: the same without the marks, with the line feeds of its
// pastes put back.
func appendPlain(b *strings.Builder, s string) {
	for {
		i := strings.Index(s, marksLead)
		if i < 0 || i+3 > len(s) {
			b.WriteString(s)
			return
		}
		b.WriteString(s[:i])
		switch s[i+2] {
		case pasteOpen[2], pasteClose[2]:
		case pasteBreak[2]:
			b.WriteByte('\n')
		default:
			// Another character that begins with the same two bytes.
			b.WriteString(s[i : i+3])
		}
		s = s[i+3:]
	}
}

// plain is the text that s, text as the editor keeps it, stands for.
func plain(s string) string {
	if !strings.Contains(s, marksLead) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	appendPlain(&b, s)
	return b.String()
}

// plainLen is len(plain(s)), without building it.
func plainLen(s string) int {
	if !strings.Contains(s, marksLead) {
		return len(s)
	}
	return len(s) - 3*strings.Count(s, pasteOpen) - 3*strings.Count(s, pasteClose) - 2*strings.Count(s, pasteBreak)
}

// countLines is how many lines a person would say text has: a line feed
// at the very end does not begin another.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n") + 1
	if strings.HasSuffix(s, "\n") {
		n--
	}
	return n
}

// pasteLabel is what stands on screen for a collapsed paste: its size, in
// gnosh's own words.
func pasteLabel(item string) string {
	// Counted on the paste as it is kept, where a line feed is one mark,
	// so that a large paste is not copied each time it is measured.
	content := item[len(pasteOpen) : len(item)-len(pasteClose)]
	lines := strings.Count(content, pasteBreak) + 1
	if strings.HasSuffix(content, pasteBreak) {
		lines--
	}
	if lines > 1 {
		return "[pasted " + strconv.Itoa(lines) + " lines]"
	}
	n := utf8.RuneCountInString(content)
	if n == 1 {
		return "[pasted 1 character]"
	}
	return "[pasted " + strconv.Itoa(n) + " characters]"
}

// itemEnd returns where the item that starts at off in s ends. off must be
// the start of an item and less than len(s).
func (e *Editor) itemEnd(s string, off int) int {
	rest := s[off:]
	if strings.HasPrefix(rest, pasteOpen) {
		i := strings.Index(rest, pasteClose)
		if i < 0 {
			return len(s)
		}
		return off + i + len(pasteClose)
	}
	if rest[0] < utf8.RuneSelf && (len(rest) == 1 || rest[1] < utf8.RuneSelf) {
		// An ASCII character with ASCII after it, or nothing, stands
		// alone. (No line holds a carriage return.)
		return off + 1
	}
	cluster, _, _ := e.m.NextGrapheme(rest)
	n := len(cluster)
	// A character of the kind that joins what follows it must not take
	// in the mark that begins a paste.
	if i := strings.Index(cluster, pasteOpen); i > 0 {
		n = i
	}
	return off + n
}

// safeStart returns a place at or before off, and no earlier than floor,
// where an item is known to start: floor itself, the end of a collapsed
// paste, or the gap between two ASCII characters, which no rule of
// grapheme clustering closes. floor must be the start of an item, and off
// must not be inside a collapsed paste.
func safeStart(s string, off, floor int) int {
	p := off
	for p > floor {
		if p < len(s) && s[p-1] < utf8.RuneSelf && s[p] < utf8.RuneSelf {
			break
		}
		if s[p-1] == pasteClose[2] && strings.HasSuffix(s[:p], pasteClose) {
			break
		}
		p--
	}
	return p
}

// itemStart returns where the item that ends at off in s starts. off must
// be the end of an item and greater than floor, which must be the start of
// one. Grapheme clusters can only be found reading forwards, so it goes
// back to a safe place and reads forwards from there.
func (e *Editor) itemStart(s string, off, floor int) int {
	if strings.HasSuffix(s[:off], pasteClose) {
		if i := strings.LastIndex(s[:off], pasteOpen); i >= floor {
			return i
		}
	}
	start := safeStart(s, off-1, floor)
	for {
		next := e.itemEnd(s, start)
		if next >= off {
			return start
		}
		start = next
	}
}

// itemBefore returns the start of the item that holds byte off of s: off
// itself when an item starts there, otherwise the nearest item start
// before it. It is for offsets that come from outside.
func (e *Editor) itemBefore(s string, off, floor int) int {
	if off <= floor {
		return floor
	}
	if off >= len(s) {
		return len(s)
	}
	// Inside a collapsed paste, the paste is the item.
	if open := strings.LastIndex(s[:off], pasteOpen); open >= floor {
		if end := strings.Index(s[open:], pasteClose); end >= 0 && open+end+len(pasteClose) > off {
			return open
		}
	}
	for !utf8.RuneStart(s[off]) {
		off--
	}
	start := safeStart(s, off, floor)
	for {
		next := e.itemEnd(s, start)
		if next > off {
			return start
		}
		start = next
	}
}

// The kinds of item the word keys tell apart.
const (
	classOther = iota // punctuation and symbols
	classSpace        // spaces and tabs
	classWord         // letters and digits
	classPaste        // a collapsed paste, a word by itself
)

// classOf says what kind of item it is, by its first character.
func classOf(item string) int {
	if strings.HasPrefix(item, pasteOpen) {
		return classPaste
	}
	r, _ := utf8.DecodeRuneInString(item)
	switch {
	case unicode.IsSpace(r):
		return classSpace
	case unicode.IsLetter(r) || unicode.IsDigit(r):
		return classWord
	}
	return classOther
}

// standIn is what the editor draws for a character a terminal would not
// show, or that must not reach it: "^[" for a control character and
// "<U+200B>" for anything else that takes no cell. It is plain ASCII, one
// cell a byte.
func standIn(cluster string) string {
	if len(cluster) == 1 {
		switch c := cluster[0]; {
		case c < 0x20:
			return string([]byte{'^', c + 0x40})
		case c == 0x7f:
			return "^?"
		}
	}
	var b strings.Builder
	b.WriteByte('<')
	for i, r := range cluster {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString("U+")
		hex := strings.ToUpper(strconv.FormatInt(int64(r), 16))
		for n := len(hex); n < 4; n++ {
			b.WriteByte('0')
		}
		b.WriteString(hex)
	}
	b.WriteByte('>')
	return b.String()
}

// Visible makes a string safe to draw: every control character in it
// becomes the stand-in the editor draws for it ("^[" for Esc, "^J" for a
// line feed), so nothing in it can reach the terminal as a control
// sequence or move to another row. It is for text that comes from
// outside and is drawn as it is: a candidate's label, a file name, what a
// person typed.
func Visible(s string) string {
	ok := utf8.ValidString(s)
	for i := 0; ok && i < len(s); i++ {
		if c := s[i]; c < 0x20 || c == 0x7f || c == 0xc2 {
			ok = false
		}
	}
	if ok {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0 {
			b.WriteString(standIn(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

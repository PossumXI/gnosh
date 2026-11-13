package text

import (
	"strings"
	"unicode/utf8"
)

// Metrics holds the choices about how wide text is. The zero value is what
// most terminals do, and the package's functions (Width, Truncate, Wrap and
// the rest) measure with it; a program that has learned its terminal is
// different keeps a Metrics value and calls the same functions as methods.
type Metrics struct {
	// AmbiguousWide makes the characters Unicode calls East Asian
	// "ambiguous" two cells wide instead of one: Greek and Cyrillic letters,
	// box-drawing lines, some symbols, the private-use area. Terminals set
	// up for Chinese, Japanese or Korean legacy fonts draw them that way.
	AmbiguousWide bool
}

// Width returns how many terminal cells s takes when printed, taking s as
// text with no escape sequences in it (see VisibleWidth for those).
//
// The width is counted one grapheme cluster at a time:
//
//   - East Asian wide and fullwidth characters (CJK ideographs, kana, Hangul
//     syllables, fullwidth forms, most emoji) take two cells.
//   - Combining marks, zero-width joiners and spaces, variation selectors and
//     other format characters, and the Hangul vowels and trailing consonants
//     that complete the syllable before them take none. Neither do control
//     characters: a tab, a newline, an ESC all count zero, so expand tabs
//     with ExpandTabs first.
//   - A cluster shown as a single emoji takes two cells whatever it is made
//     of: a character followed by the emoji presentation selector U+FE0F
//     (which includes keycaps such as "1" U+FE0F U+20E3), a person or hand
//     followed by a skin tone, and emoji joined by zero-width joiners. A
//     flag, two regional indicators, takes two cells as well.
//   - The text presentation selector U+FE0E changes nothing: the character
//     before it keeps the width it has alone. A regional indicator without
//     a partner takes one cell, half of a flag.
//   - East Asian ambiguous characters take one cell. So does the soft
//     hyphen, which terminals have always shown.
//   - Everything else takes one cell, and a cluster of several such
//     characters (an Indic syllable with a spacing vowel sign, say) takes
//     the sum of them.
//   - A byte that is not valid UTF-8 counts as the replacement character a
//     terminal would show for it.
//
// Width does not allocate and takes time proportional to len(s).
func Width(s string) int { return Metrics{}.Width(s) }

// Width is like the package's Width, with ambiguous-width characters counted
// by m's rule.
func (m Metrics) Width(s string) int { return m.measure(s, false) }

// measure adds up the widths of the grapheme clusters of s. With
// skipSequences it passes over escape sequences as if they were not there:
// the cluster that was open before a sequence may go on after it.
//
// It runs one clusterState over the whole string, so that every code point
// is decoded and looked up once.
func (m Metrics) measure(s string, skipSequences bool) int {
	total := 0
	var st clusterState // the cluster that is open, if st.n > 0
	for i := 0; i < len(s); {
		b := s[i]
		if b == escapeByte && skipSequences {
			n, _, _ := scanSequence(s[i:])
			i += n
			continue
		}
		// alone is true when the byte after b cannot continue a cluster
		// that ends in an ASCII character: it is ASCII itself, and not the
		// start of an escape sequence that the cluster might go on after.
		alone := i+1 == len(s) || s[i+1] < utf8.RuneSelf && !(skipSequences && s[i+1] == escapeByte)
		if b < utf8.RuneSelf && st.n == 0 && alone {
			// No cluster is open and none can form: b is a cluster.
			total += asciiCells(b)
			i++
			continue
		}
		r, size, class, p, w := m.decode(s[i:], skipSequences)
		if !st.take(r, class, p, w) {
			total += st.cells()
			st = clusterState{}
			st.take(r, class, p, w)
		}
		i += size
		if b < utf8.RuneSelf && alone {
			// Nothing can join an ASCII character but a non-ASCII one, or
			// a line feed its carriage return, and those two take no room
			// together or apart: close the cluster.
			total += st.cells()
			st = clusterState{}
		}
	}
	return total + st.cells()
}

// ExpandTabs replaces every tab in s with the spaces that take the text to
// the next tab stop. Tab stops are tabWidth cells apart, counted from the
// left edge of the line; startCol is the column the first line of s starts
// at, and every line after a newline or carriage return starts at column
// zero. Columns are counted as VisibleWidth counts them, so escape sequences
// in s take no room and wide characters take two cells.
//
// A tab inside an escape sequence is part of the sequence and stays. An
// escape sequence that was cut short (see Token.Complete) is removed, since
// the spaces written in a tab's place could otherwise become part of it.
//
// With a tabWidth below one there are no tab stops and the tabs stay.
// ExpandTabs takes time proportional to len(s) and allocates only when it
// has something to change.
func ExpandTabs(s string, tabWidth int, startCol int) string {
	return Metrics{}.ExpandTabs(s, tabWidth, startCol)
}

// ExpandTabs is like the package's ExpandTabs, with columns counted by m's
// rules.
func (m Metrics) ExpandTabs(s string, tabWidth int, startCol int) string {
	s = finished(s)
	if tabWidth < 1 || strings.IndexByte(s, '\t') < 0 {
		return s
	}
	col := max(startCol, 0)
	var b strings.Builder
	b.Grow(len(s) + tabWidth)
	for rest := s; rest != ""; {
		var p piece
		p, rest = m.nextPiece(rest)
		switch {
		case p.kind != Plain:
			b.WriteString(p.text)
		case p.text == "\t":
			n := tabWidth - col%tabWidth
			for range n {
				b.WriteByte(' ')
			}
			col += n
		case p.text[0] == '\n' || p.text[0] == '\r':
			b.WriteString(p.text)
			col = 0
		default:
			b.WriteString(p.text)
			col += p.width
		}
	}
	return b.String()
}

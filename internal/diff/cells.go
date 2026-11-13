package diff

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file turns one line of a text into what is drawn for it.
//
// A line of a file is not safe to print as it is. It can hold an escape
// sequence, which would move the cursor or change the colours and so alter
// the very diff a person is deciding on. It can hold characters that take no
// room, which make two different lines look alike. It can end in spaces that
// nobody would see. So a line is first cut into cells, each a character as
// it will be drawn, and anything that would not show as itself is given a
// stand-in that does.

// The stand-ins.
const (
	markSpace = "·" // a space that is shown: at the end of a changed line, or in a change that is only spaces
	markTab   = "→" // the first cell of a tab that is shown
	markCR    = "␍" // the carriage return of a CRLF, where line endings are mixed
)

// A cell is one character of a line as it is drawn.
type cell struct {
	text   string // what is written: the character, or its stand-in
	width  int    // the terminal cells it takes
	marked bool   // it lies in a stretch that changed
	symbol bool   // it is a stand-in
	space  bool   // it is a blank: a row may end after it
}

// cells appends to dst the cells of a line's text s, which has no line
// ending.
//
//   - A tab becomes the spaces that take the text to the next tab stop,
//     counted from the start of the line's text, so the same line is laid out
//     the same way whichever side of the diff it stands on.
//   - A control character becomes its picture from the Unicode block made
//     for that ("␛" for ESC), so no escape sequence in a file reaches the
//     terminal. Any other character that takes no room (a zero-width space,
//     a mark that sets the direction of the text, a byte-order mark, a
//     combining mark with nothing to sit on) becomes its number, as
//     "<U+200B>", one cell to each character of that.
//   - A character that hides behind another is written out the same way: a
//     zero-width joiner or non-joiner, a variation selector or any other
//     invisible format character that follows a plain ASCII character, where
//     it has no use but to make two names that look alike differ, and a
//     "tag" character anywhere but in the flag emoji that tags were made
//     for.
//   - Spaces and tabs from offset trail on, the ones at the end of a changed
//     line, are shown as "·" and "→". So are the blanks inside the spans when
//     reveal is set, which the caller does when a deleted and an inserted
//     line would otherwise look the same.
//
// spans are the stretches of s that changed, in order; their cells are
// marked. most, when it is not zero, is a number of cells after which the
// rest of the line is left out: the caller knows it has no room for more.
func (r *renderer) cells(dst []cell, s string, spans []Span, reveal bool, trail, most int) []cell {
	col := 0 // the column the next cell starts at
	next := 0
	pos := 0
	add := func(c cell) {
		dst = append(dst, c)
		col += c.width
	}
	// symbol adds the stand-in for the character c, one cell for each
	// character of the stand-in, so that a long one wraps like any text.
	symbol := func(c rune, marked, space bool) {
		for _, part := range standIn(c) {
			s := string(part)
			add(cell{text: s, width: r.m.Width(s), marked: marked, symbol: true, space: space})
		}
	}
	for rest := s; rest != ""; {
		if most > 0 && len(dst) >= most {
			break
		}
		cluster, width, after := r.m.NextGrapheme(rest)
		start := pos
		pos += len(cluster)
		rest = after
		for next < len(spans) && spans[next].End <= start {
			next++
		}
		marked := next < len(spans) && spans[next].Start <= start
		shown := start >= trail || marked && reveal

		switch {
		case cluster == "\t":
			n := r.tab - col%r.tab
			if w := r.m.Width(markTab); shown && w <= n {
				add(cell{text: markTab, width: w, marked: marked, symbol: true, space: true})
				n -= w
			}
			for ; n > 0; n-- {
				add(cell{text: " ", width: 1, marked: marked, space: true})
			}
		case cluster == " ":
			if shown {
				add(cell{text: markSpace, width: r.m.Width(markSpace), marked: marked, symbol: true, space: true})
			} else {
				add(cell{text: " ", width: 1, marked: marked, space: true})
			}
		case len(cluster) == 1 && cluster[0] < utf8.RuneSelf && width > 0:
			add(cell{text: cluster, width: width, marked: marked})
		case width == 0 || !utf8.ValidString(cluster) || unprintable(cluster):
			// Nothing of it would show, or it is not text at all.
			for _, c := range cluster {
				symbol(c, marked, false)
			}
		default:
			first, size := utf8.DecodeRuneInString(cluster)
			if shown && unicode.Is(unicode.Zs, first) {
				// A blank that is not the plain space, where blanks are
				// what changed.
				symbol(first, marked, true)
				cluster = cluster[size:]
				first = utf8.RuneError // what is left has nothing to sit on
			}
			kept, hidden := cluster, ""
			if hides(cluster, first) {
				var k, h strings.Builder
				for i, c := range cluster {
					if i > 0 && hiddenAfter(c, first) {
						h.WriteRune(c)
					} else {
						k.WriteRune(c)
					}
				}
				kept, hidden = k.String(), h.String()
			}
			if w := r.m.Width(kept); w > 0 {
				add(cell{text: kept, width: w, marked: marked, space: unicode.Is(unicode.Zs, first)})
			} else {
				for _, c := range kept {
					symbol(c, marked, false)
				}
			}
			for _, c := range hidden {
				symbol(c, marked, false)
			}
		}
	}
	return dst
}

// unprintable reports whether a cluster begins with a character that is an
// instruction and not something to read: a control character, one of
// Unicode's line and paragraph separators, or a format character such as
// the soft hyphen or a mark that turns the direction of the text. Most of
// these take no room and are caught by that; this catches the few that a
// terminal may give a cell, or act on.
func unprintable(cluster string) bool {
	first, _ := utf8.DecodeRuneInString(cluster)
	return unicode.In(first, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp)
}

// hides reports whether a cluster that begins with first holds a character
// that hiddenAfter would write out. It is the quick test that lets ordinary
// text through untouched.
func hides(cluster string, first rune) bool {
	for i, c := range cluster {
		if i > 0 && hiddenAfter(c, first) {
			return true
		}
	}
	return false
}

// hiddenAfter reports whether c, standing in a cluster after first, is there
// to be overlooked.
func hiddenAfter(c, first rune) bool {
	if c >= 0xE0000 && c <= 0xE007F {
		// Tag characters spell invisible text. Their one use is in the
		// flags made of a black flag and tags.
		return first != 0x1F3F4
	}
	if first >= utf8.RuneSelf {
		// In other scripts and in emoji, joiners and selectors do real
		// work.
		return false
	}
	switch {
	case c == 0xFE0E || c == 0xFE0F:
		// A selector after a digit, "#" or "*" makes a keycap.
		return !(first >= '0' && first <= '9' || first == '#' || first == '*')
	case c >= 0xFE00 && c <= 0xFE0D, c >= 0xE0100 && c <= 0xE01EF:
		return true // the other variation selectors
	case c == 0x034F:
		return true // the combining grapheme joiner, which shows nothing
	}
	return unicode.Is(unicode.Cf, c)
}

// standIn names a character that cannot be shown as itself: the picture of a
// control character, or the number of anything else.
func standIn(c rune) string {
	switch {
	case c < 0x20:
		return string(rune(0x2400) + c)
	case c == 0x7F:
		return "␡"
	case c == utf8.RuneError:
		return "\ufffd"
	}
	hex := strings.ToUpper(strconv.FormatInt(int64(c), 16))
	if len(hex) < 4 {
		hex = strings.Repeat("0", 4-len(hex)) + hex
	}
	return "<U+" + hex + ">"
}

// rowEnd returns where the row that begins at cell start ends: the index of
// the first cell of the next row. A row holds as many cells as fit in the
// room for text. When the line goes on, the row ends after its last run of
// blanks instead, so that words stay whole, unless that would leave the row
// less than half full: then, and inside a word longer than a row, it ends
// where the room does. Nothing is dropped at the break, blanks included. A
// cell wider than the room has a row of its own.
func (r *renderer) rowEnd(cells []cell, start int) int {
	if r.room <= 0 {
		return len(cells)
	}
	width, i := 0, start
	gap, gapWidth := -1, 0 // the last place after blanks, and the row's width up to it
	for i < len(cells) && width+cells[i].width <= r.room {
		width += cells[i].width
		i++
		if cells[i-1].space && i < len(cells) && !cells[i].space {
			gap, gapWidth = i, width
		}
	}
	switch {
	case i == len(cells):
		return i
	case i == start:
		return start + 1
	case gap > start && 2*gapWidth >= r.room:
		return gap
	}
	return i
}

// clean returns s as one line that is safe to print: its control characters
// and invisible characters replaced by their stand-ins.
func (r *renderer) clean(s string) string {
	plain := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c >= 0x7F {
			plain = false
			break
		}
	}
	if plain {
		return s
	}
	var b strings.Builder
	for _, c := range r.cells(nil, strings.ReplaceAll(s, "\t", " "), nil, false, len(s)+1, 0) {
		b.WriteString(c.text)
	}
	return b.String()
}

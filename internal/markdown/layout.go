package markdown

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// A span says that the text up to end, from where the span before it ends,
// is drawn in one style.
type span struct {
	end int
	st  style
}

// A lineBuf is inline text whose look is settled and which waits to be
// broken into lines: clean text, and the styles of its parts. A hard line
// break stands in the text as a newline.
type lineBuf struct {
	txt   []byte
	spans []span
}

// add appends s, drawn in st, making it clean on the way.
func (b *lineBuf) add(s string, st style) {
	if s == "" {
		return
	}
	b.txt = clean(b.txt, s)
	b.mark(st)
}

// addBreak appends a hard line break.
func (b *lineBuf) addBreak() {
	b.txt = append(b.txt, '\n')
	b.mark(style{})
}

// mark says that the text added since the last span is drawn in st.
func (b *lineBuf) mark(st style) {
	n := len(b.txt)
	if k := len(b.spans); k > 0 && b.spans[k-1].st == st {
		b.spans[k-1].end = n
		return
	}
	b.spans = append(b.spans, span{n, st})
}

// cut drops the first n bytes of the text.
func (b *lineBuf) cut(n int) {
	if n <= 0 {
		return
	}
	b.txt = append(b.txt[:0], b.txt[n:]...)
	kept := b.spans[:0]
	for _, sp := range b.spans {
		if sp.end > n {
			kept = append(kept, span{sp.end - n, sp.st})
		}
	}
	b.spans = kept
}

// clone returns a copy of b that shares nothing with it.
func (b *lineBuf) clone() lineBuf {
	return lineBuf{txt: append([]byte(nil), b.txt...), spans: append([]span(nil), b.spans...)}
}

// render returns the text from byte from up to byte to, drawn in its styles.
func (b *lineBuf) render(cfg *config, from, to int) string {
	if from >= to {
		return ""
	}
	var out strings.Builder
	start := 0
	for _, sp := range b.spans {
		a, e := max(start, from), min(sp.end, to)
		if a < e {
			out.WriteString(cfg.paint(sp.st, string(b.txt[a:e])))
		}
		start = sp.end
		if start >= to {
			break
		}
	}
	return out.String()
}

// The classes of character that decide where a line may end. kWide is a
// flag that goes with any of them: the character takes two cells.
const (
	kOther  uint8 = iota
	kSpace        // a space
	kBreak        // a hard line break
	kAlnum        // a letter or a digit
	kHyphen       // a hyphen right after a letter or digit: a line may end after it
	kClose        // closing punctuation: a line does not start with it
	kOpen         // opening punctuation: a line does not end with it
	kMask   = 0x0f
	kWide   = 0x80
)

// The punctuation, small kana and the like that East Asian text does not
// start a line with, and the brackets it does not end a line with (the
// common members of the classes CL, CP, NS, IS, EX and OP of UAX #14).
const (
	noLineStart = "、。，．：；？！）］｝〉》」』】〕〗〙〛’”…‥々ーゝゞヽヾ" +
		"ぁぃぅぇぉっゃゅょゎァィゥェォッャュョヮヵヶ・％‰°′″℃"
	noLineEnd = "（［｛〈《「『【〔〖〘〚‘“"
)

// classOf returns the class of the cluster cl, w cells wide, that follows a
// character of class before.
func classOf(cl string, w int, before uint8) uint8 {
	c := cl[0]
	if c < utf8.RuneSelf {
		switch {
		case c == ' ':
			return kSpace
		case c == '\n':
			return kBreak
		case c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			return kAlnum
		case c == '-':
			if before&kMask == kAlnum {
				return kHyphen
			}
		case strings.IndexByte("!%),.:;?]}", c) >= 0:
			return kClose
		case c == '(' || c == '[' || c == '{':
			return kOpen
		}
		return kOther
	}
	r, _ := utf8.DecodeRuneInString(cl)
	k := kOther
	switch {
	case strings.ContainsRune(noLineStart, r):
		k = kClose
	case strings.ContainsRune(noLineEnd, r):
		k = kOpen
	case unicode.IsLetter(r) || unicode.IsDigit(r):
		k = kAlnum
	}
	if w >= 2 {
		k |= kWide
	}
	return k
}

// breakAllowed reports whether a line may end between a character of class
// a and the cluster cl of class b that follows it with no space between.
func breakAllowed(a, b uint8, cl string) bool {
	if a&kMask == kHyphen {
		r, _ := utf8.DecodeRuneInString(cl)
		return unicode.IsLetter(r)
	}
	if b&kMask == kClose || a&kMask == kOpen {
		return false
	}
	return a&kWide != 0 || b&kWide != 0
}

// cluster splits the first grapheme cluster off s and returns it with its
// width. whole is the length of the cluster as it stands in s; the cluster
// returned is shorter when it is wider than room and wider than any single
// character: it is then cut down to its first character, so that it fits
// wherever two cells are free.
func (cfg *config) cluster(s string, room int) (cl string, w, whole int) {
	if c := s[0]; c < utf8.RuneSelf && (len(s) == 1 || s[1] < utf8.RuneSelf) {
		if c == '\n' {
			return s[:1], 0, 1
		}
		return s[:1], 1, 1
	}
	// A cluster has no limit in Unicode: a letter can carry any number of
	// marks. Here it has one, so that a text made of marks alone cannot
	// keep a line waiting for the cluster to end.
	window := s
	if len(window) > 2*maxCluster {
		window = window[:2*maxCluster]
	}
	cl, w, _ = cfg.m.NextGrapheme(window)
	if len(cl) > maxCluster {
		k := maxCluster
		for !utf8.RuneStart(s[k]) {
			k--
		}
		cl = s[:k]
		w = cfg.m.Width(cl)
	}
	whole = len(cl)
	if w > room && w > 2 {
		if _, size := utf8.DecodeRuneInString(cl); size < len(cl) {
			cl = cl[:size]
			w = cfg.m.Width(cl)
		}
	}
	return cl, w, whole
}

// maxCluster is the most bytes that are taken as one character. What is
// left of a longer cluster starts a new one.
const maxCluster = 256

// lineBytes is the most bytes a line of room cells may hold. Characters
// that take no room could otherwise fill a line without end.
func lineBytes(room int) int {
	return 32 * max(room, 1)
}

// A breakState is what wrap remembers between two calls for the same text.
type breakState struct {
	later  bool  // a line of the text has been ended: the next is not its first
	cont   bool  // the line being filled starts where the one before was wrapped
	before uint8 // the class of the character before the line being filled
}

// wrap breaks the text of b into lines of at most room cells and calls emit
// for each line that is complete, with the bytes of b.txt it holds and the
// number of spaces to put before it. Lines after the first are set in by
// hang cells, when that leaves room. It returns the number of bytes at the
// start of b.txt that are used up; the caller cuts them off.
//
// Lines end at hard breaks and, when the next word does not fit, at spaces
// (which are dropped), after a hyphen between letters, and between East
// Asian characters. A word longer than a line is moved to a line of its
// own and broken where the line is full.
//
// With eof false the text may still grow. Then only lines that no more text
// can change are emitted: those ended by a character that was seen not to
// fit. The last line stays, and so does the last character, which may yet
// combine with what follows. With eof true everything is emitted.
func (cfg *config) wrap(b *lineBuf, bs *breakState, room, hang int, eof bool, emit func(from, to, indent int)) int {
	s := string(b.txt)
	n := len(s)
	cfg.did(n)
	room = max(room, 1)
	if hang > room/2 || room-hang < 2 {
		hang = 0
	}
	avail, indent := room, 0
	if bs.later {
		avail, indent = room-hang, hang
	}
	var (
		start, pos, end   = 0, 0, 0 // the line being filled, and its last character that is not a space
		used, gap         = 0, 0    // cells taken, and cells of spaces since
		filled            = false   // the line holds something other than spaces
		candEnd, candNext = -1, -1  // the last place the line could end, and where the next would start
		candBefore        uint8
		last              = bs.before
	)
	newLine := func(at int, cont bool, before uint8) {
		bs.later, bs.cont, bs.before = true, cont, before
		avail, indent = room-hang, hang
		start, pos, end = at, at, at
		used, gap, filled = 0, 0, false
		candEnd, candNext = -1, -1
		last = before
	}
	for pos < n {
		cl, w, whole := cfg.cluster(s[pos:], avail)
		if !eof && pos+whole == n {
			break
		}
		k := classOf(cl, w, last)
		switch k & kMask {
		case kBreak:
			emit(start, end, indent)
			newLine(pos+1, true, kSpace)
			continue
		case kSpace:
			pos++
			last = k
			if !filled && bs.cont {
				start, end = pos, pos
			} else {
				gap++
			}
			continue
		}
		can := filled && (gap > 0 || breakAllowed(last, k, cl))
		if used+gap+w <= avail && (!filled || pos+len(cl)-start <= lineBytes(avail)) {
			if can {
				candEnd, candNext, candBefore = end, pos, last
			}
			used += gap + w
			gap = 0
			pos += len(cl)
			end, filled, last = pos, true, k
			continue
		}
		switch {
		case !filled && gap > 0:
			// Spaces at the start that leave no room for a character go.
			start, end, gap = pos, pos, 0
		case !filled:
			// A character wider than a whole line gets a line to itself.
			used = w
			pos += len(cl)
			end, filled, last = pos, true, k
		case can:
			emit(start, end, indent)
			newLine(pos, true, last)
		case candNext >= 0:
			e, next, before := candEnd, candNext, candBefore
			emit(start, e, indent)
			newLine(next, true, before)
		default:
			emit(start, end, indent)
			newLine(pos, true, last)
		}
	}
	if !eof {
		return start
	}
	if filled {
		emit(start, end, indent)
		bs.later = true
	}
	return n
}

// lines returns the whole text of b broken into lines of at most room
// cells, each drawn in its styles.
func (cfg *config) lines(b *lineBuf, room, hang int) []string {
	var out []string
	var bs breakState
	cfg.wrap(b, &bs, room, hang, true, func(from, to, indent int) {
		out = append(out, spaces(indent)+b.render(cfg, from, to))
	})
	return out
}

// cutRows cuts s, which is clean and holds no escape sequences, into rows
// of at most room cells without regard to words, as a terminal would wrap
// it, and calls emit for each. It never splits a character, so a row may
// be a cell short. With eof false the last row is not emitted, since more
// text may join it. It returns the number of bytes of s that were emitted.
func (cfg *config) cutRows(s string, room int, eof bool, emit func(row string)) int {
	cfg.did(len(s))
	room = max(room, 1)
	start, used := 0, 0
	for pos := 0; pos < len(s); {
		cl, w, whole := cfg.cluster(s[pos:], room)
		if !eof && pos+whole == len(s) {
			break
		}
		if (used+w > room || pos+len(cl)-start > lineBytes(room)) && pos > start {
			emit(s[start:pos])
			start, used = pos, 0
			continue
		}
		used += w
		pos += len(cl)
	}
	if !eof {
		return start
	}
	if start < len(s) {
		emit(s[start:])
	}
	return len(s)
}

// cutStyled is cutRows for a whole line that holds SGR sequences, such as a
// highlighter returns. A style that is on where a row ends is closed there
// and turned on again at the start of the next row.
func (cfg *config) cutStyled(s string, room int, emit func(row string)) {
	room = max(room, 1)
	var row, on strings.Builder
	used, any := 0, false
	flush := func() {
		if on.Len() > 0 {
			row.WriteString(text.Reset)
		}
		emit(row.String())
		row.Reset()
		row.WriteString(on.String())
		used, any = 0, false
	}
	for rest := s; rest != ""; {
		var tok text.Token
		tok, rest = text.NextToken(rest)
		if tok.Kind != text.Plain {
			if !tok.IsSGR() {
				continue
			}
			row.WriteString(tok.Text)
			if tok.Text == text.Reset || tok.Text == "\x1b[m" {
				on.Reset()
			} else {
				on.WriteString(tok.Text)
			}
			continue
		}
		for part := tok.Text; part != ""; {
			cl, w, _ := cfg.cluster(part, room)
			if used+w > room && any {
				flush()
				continue
			}
			row.WriteString(cl)
			used += w
			any = true
			part = part[len(cl):]
		}
	}
	if any {
		flush()
	}
}

// cells returns the number of terminal cells s takes. s is clean text with
// no escape sequences in it.
func (cfg *config) cells(s string) int {
	return cfg.m.Width(s)
}

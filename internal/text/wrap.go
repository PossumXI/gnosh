package text

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// WrapOptions are the choices Wrap leaves at their defaults. The zero value
// wraps plainly.
type WrapOptions struct {
	// First is written at the start of the first line, and Rest at the
	// start of every line after it, whether that line was made by wrapping
	// or follows a newline in the text. Their widths count against the
	// line: a list item is wrapped with First "- " and Rest "  ", a quote
	// with "> " for both. They may hold escape sequences, and should close
	// any style they open.
	First string
	Rest  string
	// Hang is a number of extra spaces put at the start of each
	// continuation line, a line made by wrapping as opposed to one that
	// follows a newline, after Rest and the line's own indentation. It
	// gives a hanging indent to every line of a text of several lines.
	Hang int
}

// Wrap breaks s into lines at most width terminal cells wide, for showing in
// a terminal. It returns the lines without their newlines; written out with
// a newline after each, they show the text of s.
//
// Where lines end:
//
//   - At a newline in s, always. "\r\n" counts as a newline. The text after
//     the last newline is a line too, so s with n newlines gives at least
//     n+1 lines, the empty string gives one empty line, and a text that
//     needs no wrapping comes back as it would from splitting at newlines.
//   - At spaces and tabs, when the next word does not fit. The spaces and
//     tabs at the break are dropped. Wrap does not expand tabs: a tab counts
//     as a space with no width, so expand them first (ExpandTabs).
//   - After a hyphen or dash that joins two words, as in "well-known" or
//     "this—that": it must have a letter or digit before it and a letter
//     after it (a digit will also do after a dash other than "-").
//   - Between characters that are written without spaces: Chinese, Japanese
//     and Korean text, fullwidth forms and emoji. As UAX #14 requires, a
//     line does not start with closing punctuation, a small kana or the
//     like, nor end with opening punctuation; a break after a zero-width
//     space is allowed.
//   - Anywhere between two grapheme clusters, when a single word is longer
//     than a whole line. A word that does not fit in what is left of a line
//     is moved to the next line before it is broken this way.
//
// What is kept:
//
//   - A line's own indentation, its leading spaces, stays, and the lines it
//     wraps into start with the same indentation (plus Hang), unless that
//     would take more than half of the room or leave less than two cells;
//     then they start at the left. Indentation is only cut down when the
//     first word would not fit after it.
//   - Spaces at the end of a line of s stay as far as they fit.
//   - Escape sequences take no room. Colours, styles and hyperlinks that
//     are open where a line ends are closed at the end of that line and
//     opened again at the start of the next, after its prefix and
//     indentation, so that every returned line is whole in itself: it
//     starts from the terminal's default state and leaves it there. To that
//     end, style and hyperlink sequences that stand at the edge of a line
//     are replaced by the ones the line needs; inside a line they stay as
//     they are. Other escape sequences stay on their line, except those that
//     were cut short (see Token.Complete), which are removed: one of those
//     would take in whatever is printed after its line.
//
// Every returned line is at most width cells wide, as VisibleWidth counts
// them, whenever width is at least two and the prefixes leave at least two
// cells for text. To keep that promise, a grapheme cluster that is wider
// than a whole line (it takes an unusual one, such as a long Indic conjunct
// in a line three cells wide) is cut between its characters. With less room
// than two cells a wide character cannot fit: the room for text is never
// taken to be less than one cell, each line gets at least one character,
// and a wide one overflows.
//
// A width below one means no limit: lines end only at newlines.
//
// Wrap takes time proportional to len(s). When nothing has to be added to a
// line (no prefix, no style to carry over) the line is a substring of s and
// is not copied. The prefixes should hold no escape sequence that is cut
// short.
func Wrap(s string, width int, opts WrapOptions) []string {
	return Metrics{}.Wrap(s, width, opts)
}

// Wrap is like the package's Wrap, with widths counted by m's rules.
func (m Metrics) Wrap(s string, width int, opts WrapOptions) []string {
	s = finished(s)
	w := wrapper{m: m, s: s, opts: opts, roomFirst: unlimited, roomRest: unlimited}
	// Guesses at the number of lines and at the number of pieces in the
	// longest paragraph, to save growing the slices.
	lines, longest := 1, 0
	for rest := s; ; lines++ {
		i := strings.IndexByte(rest, '\n')
		if i < 0 {
			longest = max(longest, len(rest))
			break
		}
		longest = max(longest, i)
		rest = rest[i+1:]
	}
	if width >= 1 {
		w.roomFirst = max(width-m.VisibleWidth(opts.First), 1)
		w.roomRest = max(width-m.VisibleWidth(opts.Rest), 1)
		lines += len(s) / width
	}
	w.lines = make([]string, 0, lines)
	w.pieces = make([]wrapPiece, 0, min(longest, 4096))
	for pos := 0; ; {
		next, newline := w.readParagraph(pos)
		w.layout()
		if newline == "" {
			break
		}
		// Sequences caught between a carriage return and its line feed.
		w.look.applyText(newline)
		pos = next
	}
	return w.lines
}

// unlimited is the room on a line when there is no width limit. It leaves
// headroom for sums.
const unlimited = math.MaxInt / 4

// The kinds of piece Wrap tells apart.
const (
	wrapCluster = iota // a grapheme cluster that is not a space
	wrapSpace          // a space or a tab
	wrapStyle          // an SGR or hyperlink sequence
	wrapOther          // any other escape sequence
)

// A wrapPiece is one piece of the paragraph being wrapped. The pieces lie end
// to end in the text, so a piece starts where the one before it ends.
type wrapPiece struct {
	end   int   // where it ends in the text
	width int32 // its width in cells
	kind  uint8 // one of the wrap kinds above
	// breakBefore is set on a cluster that follows another cluster with no
	// space between them: a line may end between the two.
	breakBefore bool
	// inner is set on a cluster with escape sequences inside it.
	inner bool
}

// A wrapper holds what Wrap is working on.
type wrapper struct {
	m         Metrics
	s         string
	opts      WrapOptions
	roomFirst int // cells for text on the first line
	roomRest  int // cells for text on the other lines

	pieces    []wrapPiece // the current paragraph
	base      int         // where the paragraph starts in the text
	sequences bool        // whether the paragraph holds any escape sequence
	widest    int         // the width of its widest cluster

	// What readParagraph remembers about the cluster it read last, to
	// decide whether a line may end before the next one.
	afterCluster bool  // the last piece that is not a sequence is a cluster
	after        uint8 // what that cluster says about a break after it
	first        rune  // its first code point
	joined       bool  // it is a hyphen with a letter or digit right before it

	look  look // what is in effect at the point the lines have reached
	lines []string
	buf   []byte // scratch for building a line
}

// start is where piece i starts in the text.
func (w *wrapper) start(i int) int {
	if i == 0 {
		return w.base
	}
	return w.pieces[i-1].end
}

// text is the text of pieces first through last.
func (w *wrapper) text(first, last int) string {
	return w.s[w.start(first):w.pieces[last].end]
}

// readParagraph splits the text from pos up to the next newline into pieces.
// It returns the position after that newline and the newline itself, which
// is empty when the text ended first.
func (w *wrapper) readParagraph(pos int) (next int, newline string) {
	w.pieces = w.pieces[:0]
	w.base = pos
	w.sequences, w.widest, w.afterCluster = false, 0, false
	s := w.s
	for pos < len(s) {
		// An ASCII byte with ASCII after it, or nothing, is a cluster of
		// its own. (A carriage return may be half of a newline, and an
		// escape sequence after the byte may hide the rest of a cluster.)
		if b := s[pos]; b < utf8.RuneSelf && b != escapeByte && b != '\r' &&
			(pos+1 == len(s) || s[pos+1] < utf8.RuneSelf && s[pos+1] != escapeByte) {
			if b == '\n' {
				return pos + 1, s[pos : pos+1]
			}
			pos++
			w.addCluster(pos, asciiCells(b), rune(b), b == ' ' || b == '\t', false)
			continue
		}
		p, _ := w.m.nextPiece(s[pos:])
		pos += len(p.text)
		switch {
		case p.kind != Plain:
			kind := uint8(wrapOther)
			if isStyle(p) {
				kind = wrapStyle
			}
			w.pieces = append(w.pieces, wrapPiece{end: pos, kind: kind})
			w.sequences = true
		case isNewline(p.text):
			return pos, p.text
		default:
			r, _ := utf8.DecodeRuneInString(p.text)
			w.addCluster(pos, p.width, r, p.text == " " || p.text == "\t", p.inner)
		}
	}
	return pos, ""
}

// addCluster appends a grapheme cluster to the paragraph: one that ends at
// end in the text, is width cells wide and starts with the code point r.
// space says that it is a space or a tab, inner that it has escape sequences
// inside it.
func (w *wrapper) addCluster(end, width int, r rune, space, inner bool) {
	piece := wrapPiece{end: end, width: int32(width), kind: wrapCluster, inner: inner}
	w.widest = max(w.widest, width)
	if inner {
		w.sequences = true
	}
	if space {
		piece.kind = wrapSpace
		w.afterCluster = false
		w.pieces = append(w.pieces, piece)
		return
	}
	// The letters and digits of ASCII have no opinion about line breaks;
	// the table need not be asked.
	before, after := uint8(wrapNeutral), uint8(wrapNeutral)
	if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
		pr := lookup(r)
		before, after = pr.wrapBefore(), pr.wrapAfter()
	}
	if w.afterCluster && before != wrapForbid {
		switch w.after {
		case wrapForbid:
		case wrapHyphen:
			piece.breakBefore = w.joined && (unicode.IsLetter(r) || w.first != '-' && unicode.IsDigit(r))
		default:
			piece.breakBefore = w.after == wrapAllow || before == wrapAllow
		}
	}
	if after == wrapHyphen {
		// The cluster is a hyphen or dash: note whether it joins on to a
		// letter or digit, for when the cluster after it comes.
		w.joined = w.afterCluster && (unicode.IsLetter(w.first) || unicode.IsDigit(w.first))
	}
	w.afterCluster, w.after, w.first = true, after, r
	w.pieces = append(w.pieces, piece)
}

// room is the number of cells for text on the next line to be written.
func (w *wrapper) room() int {
	if len(w.lines) == 0 {
		return w.roomFirst
	}
	return w.roomRest
}

// layout decides where the lines of the current paragraph end, and writes
// them.
func (w *wrapper) layout() {
	ps := w.pieces
	room := w.room()

	// The paragraph's indentation is the spaces before its first cluster.
	firstCluster, indent := len(ps), 0
	for i := range ps {
		if ps[i].kind == wrapCluster {
			firstCluster = i
			break
		}
		if ps[i].kind == wrapSpace {
			indent += int(ps[i].width)
		}
	}
	if firstCluster == len(ps) {
		// Nothing but spaces and escape sequences: one line, with as many
		// of the spaces as fit.
		first, last, used := len(ps), len(ps)-1, 0
		for i := range ps {
			if ps[i].kind != wrapSpace {
				continue
			}
			if used+int(ps[i].width) > room {
				break
			}
			used += int(ps[i].width)
			if first == len(ps) {
				first = i
			}
			last = i
		}
		w.emit(0, first, last, len(ps), 0)
		return
	}

	// Cut the indentation down if the first word would not fit after it.
	// start becomes the first piece of the line's text: the first space
	// that is kept, or the first cluster.
	start := 0
	if indent > 0 {
		keep := max(room-w.wordWidth(firstCluster, room), 0)
		for ; start < firstCluster && indent > keep; start++ {
			if ps[start].kind == wrapSpace {
				indent -= int(ps[start].width)
			}
		}
	}
	for start < firstCluster && ps[start].kind != wrapSpace {
		start++
	}

	// Lines made by wrapping start with the same indentation, and Hang.
	hang := indent + max(w.opts.Hang, 0)
	if hang > w.roomRest/2 || w.roomRest-hang < 2 {
		hang = 0
	}

	// A cluster too wide for any line is cut into parts that fit. The
	// pieces before the first cluster are not touched, so the positions
	// worked out above still hold.
	if fits := min(room, w.roomRest-hang); w.widest > fits {
		w.splitWide(fits)
		ps = w.pieces
	}

	var (
		lead        = 0     // where the pieces before the line's text begin
		lineStart   = start // the first piece of the line's text
		lineIndent  = 0     // spaces to put before the line
		used        = indent
		lastCluster = -1 // the last cluster kept on the line
		gap         = 0  // width of the spaces since that cluster
		spaced      = false
		breakEnd    = -1 // the latest place the line could end: after this piece,
		breakNext   = -1 // with the next line starting at this one
	)
	for i := firstCluster; i < len(ps); i++ {
		p := &ps[i]
		switch p.kind {
		case wrapSpace:
			gap += int(p.width)
			spaced = true
			continue
		case wrapCluster:
		default:
			continue
		}
		width := int(p.width)
		if lastCluster >= 0 {
			canBreak := spaced || p.breakBefore
			if used+gap+width <= room {
				if canBreak {
					breakEnd, breakNext = lastCluster, i
				}
			} else {
				// The cluster does not fit. End the line here if a line
				// may end here, else at the latest place one may, else
				// (a word longer than the line) here all the same.
				end, next := lastCluster, i
				if !canBreak && breakNext >= 0 {
					end, next = breakEnd, breakNext
				}
				w.emit(lead, lineStart, end, next, lineIndent)
				room = w.roomRest
				lead, lineStart, lineIndent = next, next, hang
				used, lastCluster, gap, spaced = hang, -1, 0, false
				breakEnd, breakNext = -1, -1
				i = next - 1
				continue
			}
		}
		used += gap + width
		lastCluster, gap, spaced = i, 0, false
	}

	// The last line keeps the spaces after its last word as far as they fit.
	last := lastCluster
	for i := lastCluster + 1; i < len(ps); i++ {
		if ps[i].kind != wrapSpace {
			continue
		}
		if used+int(ps[i].width) > room {
			break
		}
		used += int(ps[i].width)
		last = i
	}
	w.emit(lead, lineStart, last, len(ps), lineIndent)
}

// splitWide cuts every cluster wider than fits into parts: each part is one
// character that takes room, with the zero-width characters that follow it.
// A part is at most two cells wide, so it fits wherever two cells are free.
// The parts stay together like the letters of a word, and are only separated
// where a word longer than the line has to be broken.
func (w *wrapper) splitWide(fits int) {
	old := w.pieces
	w.pieces = make([]wrapPiece, 0, len(old)+8)
	start := w.base
	for _, p := range old {
		if p.kind != wrapCluster || int(p.width) <= fits {
			w.pieces = append(w.pieces, p)
			start = p.end
			continue
		}
		text := w.s[start:p.end]
		from, started := 0, false
		for i := 0; i < len(text); {
			if text[i] == escapeByte {
				n, _, _ := scanSequence(text[i:])
				i += n
				continue
			}
			size, width := w.m.codePoint(text[i:])
			if width > 0 {
				if started {
					w.addPart(text[from:i], start+i, from == 0 && p.breakBefore)
					from = i
				}
				started = true
			}
			i += size
		}
		w.addPart(text[from:], p.end, from == 0 && p.breakBefore)
		start = p.end
	}
}

// addPart appends one part of a cluster that was cut up: its text, where it
// ends in the whole text, and whether a line may end before it.
func (w *wrapper) addPart(text string, end int, breakBefore bool) {
	w.pieces = append(w.pieces, wrapPiece{
		end:         end,
		width:       int32(w.m.VisibleWidth(text)),
		kind:        wrapCluster,
		breakBefore: breakBefore,
		inner:       strings.IndexByte(text, escapeByte) >= 0,
	})
}

// wordWidth is the width of the run of clusters that starts at piece i and
// has no place for a line to end in it. It stops counting once the run is
// wider than limit.
func (w *wrapper) wordWidth(i, limit int) int {
	ps := w.pieces
	width := int(ps[i].width)
	for i++; i < len(ps) && width <= limit; i++ {
		switch ps[i].kind {
		case wrapSpace:
			return width
		case wrapCluster:
			if ps[i].breakBefore {
				return width
			}
			width += int(ps[i].width)
		}
	}
	return width
}

// emit writes one line. Its text is pieces first through last. The pieces
// from lead up to first come before the text and the pieces after last up to
// next come after it: spaces among them are dropped, style and hyperlink
// sequences change the look without being written, and other sequences are
// written before and after the text. indent is the number of spaces to put
// before the line. A line without text has first greater than last.
func (w *wrapper) emit(lead, first, last, next, indent int) {
	prefix := w.opts.Rest
	if len(w.lines) == 0 {
		prefix = w.opts.First
	}
	text := ""
	if first <= last {
		text = w.text(first, last)
	}
	if !w.sequences && w.look.plain() && prefix == "" && indent == 0 {
		// Nothing to add and nothing to carry: the line is its text.
		w.lines = append(w.lines, text)
		return
	}

	b := append(w.buf[:0], prefix...)
	for range indent {
		b = append(b, ' ')
	}
	b = w.absorb(b, lead, first)
	if text != "" {
		b = w.look.appendOpen(b)
		b = append(b, text...)
		if w.sequences {
			for i := first; i <= last; i++ {
				w.apply(i)
			}
		}
		b = w.look.appendClose(b)
	}
	b = w.absorb(b, last+1, next)
	w.buf = b
	if len(b) == len(text) {
		// Nothing was added after all: the line is its text.
		w.lines = append(w.lines, text)
		return
	}
	w.lines = append(w.lines, string(b))
}

// absorb takes in the pieces from index from up to to, which lie between the
// texts of two lines: style and hyperlink sequences change the look, other
// sequences are appended to b, and spaces are dropped.
func (w *wrapper) absorb(b []byte, from, to int) []byte {
	if !w.sequences {
		return b
	}
	for i := from; i < to; i++ {
		switch w.pieces[i].kind {
		case wrapStyle:
			w.apply(i)
		case wrapOther:
			b = append(b, w.text(i, i)...)
		}
	}
	return b
}

// apply changes the look the way piece i does.
func (w *wrapper) apply(i int) {
	switch p := &w.pieces[i]; {
	case p.kind == wrapStyle:
		seq := w.text(i, i)
		_, kind, complete := scanSequence(seq)
		w.look.applySequence(kind, complete, seq)
	case p.inner:
		w.look.applyText(w.text(i, i))
	}
}

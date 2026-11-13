package markdown

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// A node is one piece of inline text that the scanner has read: some text
// and its look. pos is where its source starts, which says whether the
// piece lies before the point up to which everything is settled.
type node struct {
	pos  int
	text string
	st   style
	brk  bool // a hard line break; text is empty
}

// A delim is an entry on the delimiter stack: a run of "*", "_" or "~" that
// may open or close emphasis, or a "[" or "![" that may open a link or an
// image. Its node holds the characters of the run that are not used up.
type delim struct {
	node    int
	pos     int  // where what is left of the run starts in the source
	hold    int  // where a scan must start again to get this entry as it is
	ch      byte // '*', '_', '~', '[', or '!' for "!["
	n, orig int  // characters left, and the length the run had
	open    bool // may open emphasis
	close   bool // may close emphasis
	active  bool // a bracket that may still become a link
	seen    bool // has had its chance to close
}

// A linkRef is what a link reference definition gives a label.
type linkRef struct {
	url, title string
}

// A scanner reads one stretch of inline Markdown from left to right.
//
// It follows the strategy the CommonMark specification describes: a stack
// of delimiters, on which a closing run looks back for the nearest run
// that can open with it, and brackets that become links when their closing
// bracket is followed by a destination. Two things are its own.
//
// A closing run is matched as soon as it is read, and not at the end of the
// text, unless a bracket that may still become a link stands below it on
// the stack: a link takes the delimiters inside it for itself, so until
// that bracket is decided the run waits. (It waits on nothing else: a
// bracket further on cannot have it inside.)
//
// And nothing waits for ever: an entry leaves the stack, as plain text,
// once the scan has moved reach bytes past it.
type scanner struct {
	cfg  *config
	refs map[string]linkRef
	src  string
	prev rune // the character before src
	run  bool // src begins inside a run of prev that is too long to mean anything
	base uint8
	eof  bool

	nodes   []node
	stack   []delim
	unseen  int          // every entry below this index has had its chance to close; no bracket that may still become a link is below it
	bottoms [3][3][2]int // below this index no entry opens for a closer of this kind
	ticks   map[int]int  // no closing run of this many backticks starts before here
	stop    int          // where the scan stopped
	runEnd  int          // where the last run that was too long to mean anything ended
}

// scanInline reads src as inline text. prev is the character before src: a
// newline at the start of a block. It returns the pieces whose look is
// settled, in order, and the number of bytes of src they stand for. The
// pieces are only good until the next call.
//
// With eof true src is all there is, and everything is settled. With eof
// false src may still grow, and the scan settles only what no more text
// could change: it stops in front of anything whose meaning depends on
// what follows (a run of "*" at the very end, a "`" whose partner has not
// come, a word that may be the start of an address), and it does not
// settle anything from the first delimiter that is still waiting on. The
// rest is read again on the next call.
//
// A run of one of the characters "`", "*", "_", "~" or of blanks that is
// longer than reach bytes means nothing: it is text. Such a run is
// settled as it arrives, so the next call may begin in the middle of one:
// run says that src does, and the run returned says that the settled
// text ends in one.
func scanInline(cfg *config, refs map[string]linkRef, src string, prev rune, run bool, base uint8, eof bool) (_ []node, n int, _ bool) {
	cfg.did(len(src))
	s := scanner{cfg: cfg, refs: refs, src: src, prev: prev, run: run, base: base, eof: eof, runEnd: -1}
	s.nodes, s.stack = cfg.nodes[:0], cfg.stack[:0]
	s.scan()
	if eof {
		// What never found a partner is plain text, and the closing runs
		// that waited on a bracket get their chance now.
		for k := range s.stack {
			s.stack[k].active = false
		}
		s.advance()
		cfg.nodes, cfg.stack = s.nodes[:0], s.stack[:0]
		return s.nodes, len(src), false
	}
	// Everything before the lowest entry that is still waiting is settled.
	// A bracket that can no longer become a link waits for nothing: all it
	// will ever do is turn one "]" into plain text, which a "]" with no
	// bracket to answer is anyway. An entry that is what is left of a
	// closing run stands for the whole match it was part of.
	q := s.stop
	for k := range s.stack {
		if d := &s.stack[k]; d.ch != '[' || d.active {
			q = min(q, d.hold)
			break
		}
	}
	k := sort.Search(len(s.nodes), func(i int) bool { return s.nodes[i].pos >= q })
	cfg.nodes, cfg.stack = s.nodes[:0], s.stack[:0]
	return s.nodes[:k], q, q == s.runEnd
}

// longRun reads a run of n of the same character at position i as text, if
// it is too long to mean anything: longer than reach bytes, or more of
// such a run from before src, which same says the character before src
// allows.
func (s *scanner) longRun(i, n int, same bool) bool {
	if n <= s.cfg.reach && !(i == 0 && s.run && same) {
		return false
	}
	s.add(i, s.src[i:i+n], 0)
	s.runEnd = i + n
	return true
}

// scan reads the source token by token. A reader returns the number of
// bytes it took, or zero when the text ends before it can tell what it has.
func (s *scanner) scan() {
	src := s.src
	for i := 0; i < len(src); {
		s.expire(i)
		var n int
		switch c := src[i]; c {
		case '\\':
			n = s.backslash(i)
		case '&':
			n = s.entity(i)
		case '`':
			n = s.backticks(i)
		case '*', '_', '~':
			n = s.delims(i)
		case '[':
			n = s.openBracket(i, 1)
		case '!':
			n = s.bang(i)
		case ']':
			n = s.closeBracket(i)
		case '<':
			n = s.angle(i)
		case '\n':
			n = s.newline(i, i)
		case ' ', '\t':
			n = s.blanks(i)
		default:
			n = s.word(i)
		}
		if n == 0 {
			s.stop = i
			s.expire(i)
			return
		}
		i += n
	}
	s.stop = len(src)
	s.expire(len(src))
}

// add appends a piece of text that starts at pos in the source.
func (s *scanner) add(pos int, text string, bits uint16) {
	s.nodes = append(s.nodes, node{pos: pos, text: text, st: style{bits: bits, base: s.base}})
}

// before returns the character in front of position i.
func (s *scanner) before(i int) rune {
	if i == 0 {
		return s.prev
	}
	r, _ := utf8.DecodeLastRuneInString(s.src[:i])
	return r
}

// after returns the character at position i, or a newline at the end.
func (s *scanner) after(i int) rune {
	if i >= len(s.src) {
		return '\n'
	}
	r, _ := utf8.DecodeRuneInString(s.src[i:])
	return r
}

// isSpaceRune reports whether r is whitespace as CommonMark counts it for
// emphasis: a space separator, or a tab, line feed, form feed or carriage
// return.
func isSpaceRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\f' || r == '\r' || unicode.Is(unicode.Zs, r)
}

// isPunctRune reports whether r is punctuation as CommonMark counts it for
// emphasis: any character of the Unicode categories P and S.
func isPunctRune(r rune) bool {
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

// isPunctByte reports whether c is an ASCII punctuation character, the kind
// a backslash can escape.
func isPunctByte(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// blanks reads a run of spaces and tabs. Before a newline it belongs to the
// line break; at the end of the text it is dropped, or waits to see whether
// a newline follows; anywhere else it is text.
func (s *scanner) blanks(i int) int {
	k := i
	for k < len(s.src) && (s.src[k] == ' ' || s.src[k] == '\t') {
		k++
	}
	if s.longRun(i, k-i, s.prev == ' ' || s.prev == '\t') {
		return k - i
	}
	switch {
	case k == len(s.src):
		if !s.eof {
			return 0
		}
	case s.src[k] == '\n':
		return s.newline(i, k)
	default:
		s.add(i, s.src[i:k], 0)
	}
	return k - i
}

// newline reads the line ending at position at, with the blanks before it,
// which start at from. Two or more spaces before the ending make a hard
// break; otherwise the ending is a space. The blanks that start the next
// line go with it.
func (s *scanner) newline(from, at int) int {
	end := at + 1
	for end < len(s.src) && (s.src[end] == ' ' || s.src[end] == '\t') {
		end++
	}
	if end == len(s.src) {
		// Nothing follows yet. At the end of a block the break is void.
		if !s.eof {
			return 0
		}
		return end - from
	}
	if at-from >= 2 && s.src[at-1] == ' ' && s.src[at-2] == ' ' {
		s.nodes = append(s.nodes, node{pos: from, brk: true})
	} else {
		s.add(from, " ", 0)
	}
	return end - from
}

// backslash reads a backslash: an escaped punctuation character, a hard
// break when it ends the line, or itself.
func (s *scanner) backslash(i int) int {
	if i+1 == len(s.src) {
		if !s.eof {
			return 0
		}
		s.add(i, "\\", 0)
		return 1
	}
	switch c := s.src[i+1]; {
	case c == '\n':
		end := i + 2
		for end < len(s.src) && (s.src[end] == ' ' || s.src[end] == '\t') {
			end++
		}
		if end == len(s.src) {
			if !s.eof {
				return 0
			}
			s.add(i, "\\", 0)
			return end - i
		}
		s.nodes = append(s.nodes, node{pos: i, brk: true})
		return end - i
	case isPunctByte(c):
		s.add(i, s.src[i+1:i+2], 0)
		return 2
	}
	s.add(i, "\\", 0)
	return 1
}

// entity reads a character reference such as "&amp;" or "&#x27;", or a
// plain "&".
func (s *scanner) entity(i int) int {
	n, text, need := parseEntity(s.src, i)
	if need && !s.eof {
		return 0
	}
	if n == 0 {
		s.add(i, "&", 0)
		return 1
	}
	s.add(i, text, 0)
	return n
}

// bang reads "!": the start of an image when "[" follows.
func (s *scanner) bang(i int) int {
	if i+1 == len(s.src) {
		if !s.eof {
			return 0
		}
	} else if s.src[i+1] == '[' {
		return s.openBracket(i, 2)
	}
	s.add(i, "!", 0)
	return 1
}

// delims reads a run of "*", "_" or "~" and puts it on the stack if it can
// open or close emphasis.
func (s *scanner) delims(i int) int {
	src := s.src
	c := src[i]
	n := 1
	for i+n < len(src) && src[i+n] == c {
		n++
	}
	if s.longRun(i, n, rune(c) == s.prev) {
		return n
	}
	if i+n == len(src) && !s.eof {
		return 0 // the run may grow, and what follows it decides what it can do
	}
	before, after := s.before(i), s.after(i+n)
	spaceBefore, spaceAfter := isSpaceRune(before), isSpaceRune(after)
	punctBefore, punctAfter := isPunctRune(before), isPunctRune(after)
	left := !spaceAfter && (!punctAfter || spaceBefore || punctBefore)
	right := !spaceBefore && (!punctBefore || spaceAfter || punctAfter)
	d := delim{node: len(s.nodes), pos: i, hold: i, ch: c, n: n, orig: n, open: left, close: right}
	switch {
	case c == '_':
		d.open = left && (!right || punctBefore)
		d.close = right && (!left || punctAfter)
	case c == '~' && n > 2:
		d.open, d.close = false, false
	}
	s.add(i, src[i:i+n], 0)
	if !d.open && !d.close {
		return n
	}
	s.stack = append(s.stack, d)
	s.advance()
	return n
}

// expire takes off the stack the entries that the scan, now at position i,
// has moved more than reach bytes past. They stay in the text as they were
// written.
//
// Each entry goes at its own time, the lowest first, and what its going
// allows is done before the next one goes: when a bracket that closing
// runs waited on is gone, they get their chance against whatever is on
// the stack at that moment. The result is then the same wherever the scan
// happens to look: plain text is read in pieces of different sizes
// depending on how much of the text has arrived, and the outcome must not
// depend on that.
func (s *scanner) expire(i int) {
	for len(s.stack) > 0 && i-s.stack[0].pos > s.cfg.reach {
		s.remove(0, 1)
		s.advance()
	}
}

// remove takes the entries from index a up to index b off the stack.
func (s *scanner) remove(a, b int) {
	if a >= b {
		return
	}
	s.stack = append(s.stack[:a], s.stack[b:]...)
	shift := func(v int) int {
		switch {
		case v >= b:
			return v - (b - a)
		case v > a:
			return a
		}
		return v
	}
	s.unseen = shift(s.unseen)
	for x := range s.bottoms {
		for y := range s.bottoms[x] {
			for z := range s.bottoms[x][y] {
				s.bottoms[x][y][z] = shift(s.bottoms[x][y][z])
			}
		}
	}
}

// bottom returns the place where the scanner remembers how far down the
// stack a closer like d has already looked in vain.
func (s *scanner) bottom(d *delim) *int {
	x, z := 0, 0
	switch d.ch {
	case '_':
		x = 1
	case '~':
		x = 2
	}
	if d.open {
		z = 1
	}
	return &s.bottoms[x][d.orig%3][z]
}

// advance gives their chance to close to the entries that have not had it
// and no longer wait on anything: those below the lowest bracket that may
// still become a link. It is called whenever that may have changed.
func (s *scanner) advance() {
	i := s.unseen
	for i < len(s.stack) {
		d := &s.stack[i]
		if d.active {
			break
		}
		if d.ch == '[' || d.ch == '!' || !d.close || d.seen {
			i++
			continue
		}
		i = s.tryClose(i, 0)
	}
	s.unseen = i
}

// inside gives every entry from index lo up that can close its chance to
// close against the entries from lo up: the emphasis inside a link.
func (s *scanner) inside(lo int) {
	for i := lo; i < len(s.stack); {
		if d := &s.stack[i]; !d.close || d.seen {
			i++
			continue
		}
		i = s.tryClose(i, lo)
	}
}

// tryClose matches the closing run at index ci of the stack with the
// nearest runs below it that can open with it, down to index lo, as often
// as it has characters left. It returns the index to go on from.
func (s *scanner) tryClose(ci, lo int) int {
	for {
		c := &s.stack[ci]
		floor := lo
		if lo == 0 {
			floor = *s.bottom(c)
		}
		oi := -1
		for k := ci - 1; k >= floor; k-- {
			o := &s.stack[k]
			if o.ch != c.ch || !o.open {
				continue
			}
			if c.ch == '~' {
				if o.n != c.n {
					continue
				}
			} else if (o.close || c.open) && (o.orig+c.orig)%3 == 0 && (o.orig%3 != 0 || c.orig%3 != 0) {
				continue
			}
			oi = k
			break
		}
		if oi < 0 {
			if lo == 0 {
				*s.bottom(c) = ci
			}
			if !c.open {
				s.remove(ci, ci+1)
				return ci
			}
			c.seen = true
			return ci + 1
		}
		o := &s.stack[oi]
		use, bit := 1, sEm
		switch {
		case c.ch == '~':
			use, bit = c.n, sStrike
		case o.n >= 2 && c.n >= 2:
			use, bit = 2, sStrong
		}
		for k := o.node + 1; k < c.node; k++ {
			s.nodes[k].st.bits |= bit
		}
		// The opener gives up its last characters and the closer its first.
		on, cn := &s.nodes[o.node], &s.nodes[c.node]
		on.text = on.text[:len(on.text)-use]
		cn.text = cn.text[use:]
		cn.pos += use
		o.n -= use
		c.n -= use
		c.pos += use
		c.hold = min(c.hold, o.hold)
		openerGone, closerGone := o.n == 0, c.n == 0
		s.remove(oi+1, ci) // what stood between them can no longer match
		ci = oi + 1
		if openerGone {
			s.remove(oi, oi+1)
			ci--
		}
		if closerGone {
			s.remove(ci, ci+1)
			return ci
		}
	}
}

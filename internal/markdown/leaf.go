package markdown

import (
	"strings"
	"unicode/utf8"
)

// The kinds of leaf block that can be open.
const (
	lNone  uint8 = iota
	lPara        // a paragraph
	lHead        // an ATX heading, while its line is arriving
	lCode        // a code block or an HTML block
	lTable       // a table
)

// A leaf is the open leaf block, if any: the block that takes the text of
// lines. Only the part named by kind is in use.
type leaf struct {
	kind  uint8
	para  paraLeaf
	head  headLeaf
	code  codeLeaf
	table *tableLeaf
}

// clone returns a copy of the leaf that shares nothing that either copy
// may change.
func (l *leaf) clone() leaf {
	c := *l
	switch l.kind {
	case lPara:
		c.para.fl = l.para.fl.clone()
		c.para.held = append([]byte(nil), l.para.held...)
	case lHead:
		c.head.fl = l.head.fl.clone()
		c.head.tail = append([]byte(nil), l.head.tail...)
	case lCode:
		c.code.raw = append([]byte(nil), l.code.raw...)
		c.code.disp = append([]byte(nil), l.code.disp...)
		c.code.tail = append([]byte(nil), l.code.tail...)
	case lTable:
		c.table = l.table.clone()
	}
	return c
}

// A flow is inline text on its way to becoming lines: source that the
// inline scanner has not settled yet, and settled text that has not filled
// a line yet. Paragraphs and headings are flows.
type flow struct {
	base  uint8      // the kind of block, for the style
	src   []byte     // source that is not settled
	prev  rune       // the character before src, once some of it has been settled
	cut   bool       // some of the source has been settled: prev is set
	run   bool       // src begins inside a run of prev that is too long to mean anything
	buf   lineBuf    // settled text that is not written
	bs    breakState // where the line breaking has got to
	hang  int        // how far lines after the first are set in
	dirty bool       // src has grown since it was last scanned
}

// clone returns a copy of the flow that shares nothing with it.
func (f *flow) clone() flow {
	c := *f
	c.src = append([]byte(nil), f.src...)
	c.buf = f.buf.clone()
	return c
}

// add appends source text.
func (f *flow) add(s string) {
	if s != "" {
		f.src = append(f.src, s...)
		f.dirty = true
	}
}

// advance settles as much of the source as can be settled and writes the
// lines that are complete. With eof the source is all there will be, and
// everything is written.
//
// This is where prose becomes final. A line is written once (1) the look
// of everything on it is settled, which the scanner decides, and (2) the
// line has ended, because a word that is settled too was seen not to fit
// on it, or because of a hard break.
func (f *flow) advance(st *state, cfg *config, eof bool) {
	if f.dirty || eof {
		f.dirty = false
		prev := '\n'
		if f.cut {
			prev = f.prev
		}
		nodes, n, run := scanInline(cfg, st.refs, string(f.src), prev, f.run, f.base, eof)
		for i := range nodes {
			if nd := &nodes[i]; nd.brk {
				f.buf.addBreak()
			} else {
				f.buf.add(nd.text, nd.st)
			}
		}
		if n > 0 {
			f.prev, _ = utf8.DecodeLastRune(f.src[:n])
			f.cut, f.run = true, run
			f.src = append(f.src[:0], f.src[n:]...)
		}
	}
	used := cfg.wrap(&f.buf, &f.bs, st.room(cfg), f.hang, eof, func(from, to, indent int) {
		st.emit(cfg, spaces(indent)+f.buf.render(cfg, from, to))
	})
	f.buf.cut(used)
}

// A paraLeaf is an open paragraph.
//
// Text reaches its flow one line at a time, and not at once: a line is
// held back for as long as the line after it could still turn it into
// something else. That is the case for the first line of a paragraph while
// it is at most headMax bytes long, which an underline would make a setext
// heading, and for any line with a pipe in its first headMax bytes and at
// most Reach bytes in all, which a delimiter row would make the header of
// a table. Every line is held until it has outgrown headMax bytes or has
// ended, since until then it may be either.
type paraLeaf struct {
	fl      flow
	lines   int    // the lines that have gone to the flow
	held    []byte // the line that is held back
	holding bool   // a line is being held back
	ended   bool   // the held line is complete
	lead    bool   // nothing but blanks has come of the current line
	piped   bool   // the line that went to the flow last began with a pipe
}

// hasPipe reports whether s holds a pipe that is not escaped.
func hasPipe(s []byte) bool {
	for k := 0; k < len(s); k++ {
		switch s[k] {
		case '\\':
			k++
		case '|':
			return true
		}
	}
	return false
}

// next starts a new line of the paragraph. A complete line that was held
// back is paragraph text after all.
func (p *paraLeaf) next(st *state, cfg *config) {
	if p.holding && p.ended {
		p.release(st, cfg)
	}
	p.holding, p.ended, p.lead, p.held = true, false, true, p.held[:0]
}

// text takes more of the current line. The blanks a line starts with are
// no part of it.
func (p *paraLeaf) text(st *state, cfg *config, s string) {
	if p.lead {
		if s = strings.TrimLeft(s, " \t"); s == "" {
			return
		}
		p.lead = false
	}
	if !p.holding {
		p.fl.add(s)
		p.fl.advance(st, cfg, false)
		return
	}
	p.held = append(p.held, s...)
	if n := len(p.held); n > headMax && (n > cfg.reach || !hasPipe(p.held[:headMax])) {
		p.release(st, cfg)
	}
}

// endLine ends the current line. It stays held back if the next line could
// make it a heading or the header of a table.
func (p *paraLeaf) endLine(st *state, cfg *config) {
	if !p.holding {
		return
	}
	p.ended = true
	if _, table := p.candidate(cfg); !table && !p.setextOK() {
		p.release(st, cfg)
	}
}

// setextOK reports whether an underline on the next line would make the
// paragraph a heading: it must be a single line of at most headMax bytes.
func (p *paraLeaf) setextOK() bool {
	return p.holding && p.ended && p.lines == 0 && len(p.held) > 0 && len(p.held) <= headMax
}

// candidate reports whether a delimiter row on the next line would make
// the last line of the paragraph the header of a table, and returns that
// line: it must have a pipe in its first headMax bytes and be at most
// Reach bytes long.
func (p *paraLeaf) candidate(cfg *config) (string, bool) {
	if !p.holding || !p.ended || len(p.held) > cfg.reach || !hasPipe(p.held[:min(len(p.held), headMax)]) {
		return "", false
	}
	return string(p.held), true
}

// release lets the held line go to the flow as paragraph text.
//
// A line that begins with a pipe was meant as a row of a table, and here
// it is not one: the table has no delimiter row, or the wrong one. Such
// lines are kept apart, each on a line of its own, instead of being joined
// like prose, since that is how they were meant to be read.
func (p *paraLeaf) release(st *state, cfg *config) {
	piped := len(p.held) > 0 && p.held[0] == '|'
	switch {
	case p.lines == 0:
	case piped || p.piped:
		p.fl.add("  \n") // a hard break
	default:
		p.fl.add("\n")
	}
	p.piped = piped
	p.fl.add(string(p.held))
	p.lines++
	p.holding, p.held = false, p.held[:0]
	p.fl.advance(st, cfg, false)
}

// finish ends the paragraph and writes what is left of it.
func (p *paraLeaf) finish(st *state, cfg *config) {
	if p.holding {
		p.release(st, cfg)
	}
	p.fl.advance(st, cfg, true)
}

// A headLeaf is a heading whose line is arriving. Its text flows like a
// paragraph's, behind the "#" marks that say its level.
type headLeaf struct {
	fl     flow
	tail   []byte // the end of the line so far that may be a closing run of "#"
	any    bool   // something other than blanks has come
	last   byte   // the last byte that went to the flow
	setext bool   // the heading was made by an underline: it has no closing run
}

// start begins a heading of the given level.
func (h *headLeaf) start(level int) {
	h.fl = flow{base: bHeading, hang: level + 1}
	h.fl.buf.add(strings.Repeat("#", level), style{bits: sMark})
	h.fl.buf.add(" ", style{})
}

// text takes more of the heading's line. The blanks that start it are
// dropped, and a run of "#" and blanks at its end is kept back, since it
// is not part of the heading if the line ends there.
func (h *headLeaf) text(st *state, cfg *config, s string) {
	if !h.any {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			return
		}
		h.any = true
	}
	h.tail = append(h.tail, s...)
	k := len(h.tail)
	for k > 0 && (h.tail[k-1] == ' ' || h.tail[k-1] == '\t' || h.tail[k-1] == '#' && !h.setext) {
		k--
	}
	// No more than reach bytes are kept back: a longer run is not a
	// closing sequence, whatever it holds.
	k = max(k, len(h.tail)-cfg.reach)
	if k > 0 {
		h.last = h.tail[k-1]
		h.fl.add(string(h.tail[:k]))
		h.tail = append(h.tail[:0], h.tail[k:]...)
	}
	h.fl.advance(st, cfg, false)
}

// finish ends the heading's line and writes what is left of it. A run of
// "#" at the end is the closing sequence, and no part of the heading, if a
// blank stands before it or it is all there is.
func (h *headLeaf) finish(st *state, cfg *config) {
	t := strings.TrimRight(string(h.tail), " \t")
	j := len(t)
	for j > 0 && t[j-1] == '#' {
		j--
	}
	switch {
	case j == len(t):
	case j > 0:
		t = strings.TrimRight(t[:j], " \t")
	case h.last == 0 || h.last == ' ' || h.last == '\t':
		t = ""
	}
	h.fl.add(t)
	h.fl.advance(st, cfg, true)
}

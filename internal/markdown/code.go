package markdown

import (
	"strings"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// The kinds of block whose lines are shown as they were written.
const (
	cFenced   uint8 = iota // a fenced code block
	cIndented              // an indented code block
	cHTML                  // an HTML block
)

// A codeLeaf is an open code block or HTML block. Its lines are never
// joined or re-flowed: each line of the text is a row, and a line too long
// for the terminal is cut into rows at the edge, like a terminal would
// wrap it, with a mark in the margin of each row after the first.
type codeLeaf struct {
	kind   uint8
	ch     byte   // the fence character
	n, ind int    // the length of the opening fence, and its indentation
	lang   string // the language named on the fence
	html   int    // the kind of an HTML block, 1 to 7
	ends   bool   // an HTML block's end mark has come on the current line
	blanks int    // blank lines that belong to an indented block only if more of it follows

	whole  bool   // the current line is kept whole, for the highlighter
	raw    []byte // the current line as written, while it is kept whole
	disp   []byte // the current line's text that is not written yet: clean, tabs as spaces
	seg    int    // where the text after the last tab starts in disp,
	segCol int    // and the column of the line it starts at
	rows   int    // the rows of the current line that are written
	tail   []byte // the end of the current line so far, where an end mark may be arriving
}

// begin starts a new line of the block.
func (c *codeLeaf) begin(st *state, cfg *config) {
	for ; c.blanks > 0; c.blanks-- {
		st.emit(cfg, "")
		st.keep("\n")
	}
	c.whole = cfg.highlight != nil && c.kind != cHTML
	c.raw, c.disp, c.tail = c.raw[:0], c.disp[:0], c.tail[:0]
	c.seg, c.segCol, c.rows, c.ends = 0, 0, 0, false
}

// text takes more of the current line. Unless the line is being kept whole
// for a highlighter, the rows that are full are written at once: a row is
// final as soon as text has come that does not fit on it.
func (c *codeLeaf) text(st *state, cfg *config, s string) {
	if c.kind != cHTML {
		st.keep(s)
	}
	if c.kind == cHTML && c.html <= 5 && !c.ends {
		// The end mark may arrive in pieces: look at the piece together
		// with the end of what came before.
		c.tail = append(c.tail, s...)
		c.ends = htmlEnd(c.html, string(c.tail))
		if len(c.tail) > 16 {
			c.tail = append(c.tail[:0], c.tail[len(c.tail)-16:]...)
		}
	}
	if c.whole {
		c.raw = append(c.raw, s...)
		if len(c.raw) <= codeHold {
			return
		}
		// Too long to wait for: the line is shown plain as it comes.
		c.whole = false
		s = string(c.raw)
		c.raw = c.raw[:0]
	}
	c.show(cfg, s)
	c.flush(st, cfg, false)
}

// show appends s to the display text of the line, with each tab as the
// spaces up to the next tab stop.
func (c *codeLeaf) show(cfg *config, s string) {
	for s != "" {
		k := strings.IndexByte(s, '\t')
		if k < 0 {
			c.disp = clean(c.disp, s)
			return
		}
		c.disp = clean(c.disp, s[:k])
		col := c.segCol + cfg.cells(string(c.disp[c.seg:]))
		pad := cfg.tab - col%cfg.tab
		c.disp = append(c.disp, spaces(pad)...)
		c.seg, c.segCol = len(c.disp), col+pad
		s = s[k+1:]
	}
}

// margin returns the number of cells a code block is set in by. In a very
// narrow place it is not set in at all.
func (c *codeLeaf) margin(st *state, cfg *config) int {
	if st.room(cfg) < codeIndent+4 {
		return 0
	}
	return codeIndent
}

// flush writes the rows of the display text that are full, or all of it
// with eof.
func (c *codeLeaf) flush(st *state, cfg *config, eof bool) {
	in := c.margin(st, cfg)
	used := cfg.cutRows(string(c.disp), st.room(cfg)-in, eof, func(row string) {
		c.row(st, cfg, in, row)
	})
	if used == 0 {
		return
	}
	if c.seg < used {
		c.segCol += cfg.cells(string(c.disp[c.seg:used]))
		c.seg = 0
	} else {
		c.seg -= used
	}
	c.disp = append(c.disp[:0], c.disp[used:]...)
}

// row writes one row of the current line, set in by in cells. The first
// row of a line has spaces in its margin and the others the wrap mark, so
// that the mark never stands among the code itself.
func (c *codeLeaf) row(st *state, cfg *config, in int, row string) {
	t := &cfg.theme
	margin := spaces(in)
	if c.rows > 0 && in > 0 && t.WrapMark != "" {
		if w := cfg.cells(t.WrapMark); w < in {
			margin = t.CodeMark.Render(t.WrapMark) + spaces(in-w)
		}
	}
	c.rows++
	look := t.CodeBlock
	if c.kind == cHTML {
		look = t.HTML
	}
	if strings.TrimRight(row, " ") == "" {
		st.emit(cfg, "")
		return
	}
	st.emit(cfg, margin+look.Render(row))
}

// endLine ends the current line and writes what is left of it.
func (c *codeLeaf) endLine(st *state, cfg *config) {
	if c.whole {
		line := string(c.raw)
		if styled := c.highlight(cfg, line); styled != "" {
			in := c.margin(st, cfg)
			cfg.cutStyled(cfg.m.ExpandTabs(styled, cfg.tab, 0), st.room(cfg)-in, func(row string) {
				c.row(st, cfg, in, row)
			})
		} else {
			c.show(cfg, line)
		}
	}
	c.flush(st, cfg, true)
	if c.rows == 0 {
		st.emit(cfg, "")
	}
	if c.kind != cHTML {
		st.keep("\n")
	}
	if c.kind == cHTML && c.ends {
		st.closeLeaf(cfg)
	}
}

// highlight passes line to the highlighter and returns what it made of it,
// or "" if the line must be shown plain: it holds control characters, or
// the highlighter returned something other than the line with SGR
// sequences added.
func (c *codeLeaf) highlight(cfg *config, line string) string {
	if line == "" || !utf8.ValidString(line) {
		return ""
	}
	for k := 0; k < len(line); k++ {
		if b := line[k]; b < 0x20 && b != '\t' || b == 0x7f {
			return ""
		}
	}
	if string(clean(nil, strings.ReplaceAll(line, "\t", ""))) != strings.ReplaceAll(line, "\t", "") {
		return ""
	}
	out := cfg.highlight(c.lang, line)
	if len(out) != 1 {
		return ""
	}
	var plain strings.Builder
	for rest := out[0]; rest != ""; {
		var tok text.Token
		tok, rest = text.NextToken(rest)
		switch {
		case tok.Kind == text.Plain:
			plain.WriteString(tok.Text)
		case !tok.IsSGR():
			return ""
		}
	}
	if plain.String() != line {
		return ""
	}
	return out[0]
}

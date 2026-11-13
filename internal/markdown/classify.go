package markdown

import (
	"strconv"
	"strings"
)

// The things a line can turn out to be.
const (
	aBlank      uint8 = iota // nothing but blanks
	aText                    // paragraph text
	aHeading                 // an ATX heading
	aSetext                  // the underline that makes the paragraph above a heading
	aThematic                // a thematic break
	aFenceOpen               // the opening fence of a code block
	aFenceClose              // the closing fence of the open code block
	aCode                    // a line of the open code block or HTML block
	aCodeBlank               // a blank line that may belong to the open indented code block
	aIndented                // the first line of an indented code block
	aHTML                    // the first line of an HTML block
	aTable                   // the delimiter row that makes the line above a table's header
	aRow                     // a row of the open table
	aDef                     // a link reference definition
)

// An opening is a container that a line opens: a block quote, or a list
// item with what its marker says.
type opening struct {
	item    bool
	indent  int    // the columns of indentation that go on in the item
	ordered bool   // a numbered item
	delim   byte   // the bullet character, or "." or ")" after a number
	num     int    // the number written,
	digits  string // and how it was written
	task    uint8  // 1 for "[ ]", 2 for "[x]"
	bare    bool   // nothing follows the marker on its line
}

// A plan is what classify has found a line to be, and so what is to be
// done with it.
type plan struct {
	need  bool      // the line is not long enough yet to tell
	act   uint8     // what the line is
	cont  bool      // aText: it goes on the open paragraph
	keep  int       // the number of open containers that stay open
	opens []opening // the containers it opens, outermost first
	text  string    // what the leaf block is given of the line
	level int       // the level of a heading, or the kind of an HTML block
	// A fence: its character, its length, its indentation, what follows it.
	ch     byte
	n, ind int
	info   string
	aligns []byte // a table: the alignment of each column
	// A link reference definition.
	label, url, title string
}

// maxDepth is the most containers, block quotes and list items, that can
// be open inside one another. A marker that would open one more is text.
// No real text comes near it; it is there so that a line of nothing but
// markers cannot keep the renderer busy for ever.
const maxDepth = 32

// A cursor walks along the start of a line, where the marks of containers
// and the indentation are. It counts columns, because a tab there stands
// for the spaces up to the next multiple of four.
type cursor struct {
	line string
	pos  int // the next byte
	col  int // the column it is at
	rem  int // when not zero: line[pos] is a tab, and this many of its columns are left
}

// indent returns the number of columns of blanks that follow the cursor,
// and the position of the first byte after them.
func (c *cursor) indent() (cols, at int) {
	p, col := c.pos, c.col
	if c.rem > 0 {
		cols, col, p = c.rem, col+c.rem, p+1
	}
	for ; p < len(c.line); p++ {
		switch c.line[p] {
		case ' ':
			cols++
			col++
		case '\t':
			w := 4 - col%4
			cols += w
			col += w
		default:
			return cols, p
		}
	}
	return cols, p
}

// skip moves the cursor over n columns of blanks. It may stop inside a tab.
func (c *cursor) skip(n int) {
	for n > 0 && c.pos < len(c.line) {
		if c.rem > 0 {
			take := min(n, c.rem)
			c.rem -= take
			c.col += take
			n -= take
			if c.rem == 0 {
				c.pos++
			}
			continue
		}
		switch c.line[c.pos] {
		case ' ':
			c.pos++
			c.col++
			n--
		case '\t':
			c.rem = 4 - c.col%4
		default:
			return
		}
	}
}

// mark moves the cursor over cols columns of blanks to position at, and
// then over the n bytes of a marker that stands there.
func (c *cursor) mark(cols, at, n int) {
	c.pos, c.col, c.rem = at+n, c.col+cols+n, 0
}

// space moves the cursor over one column of blank, if one follows.
func (c *cursor) space() {
	if c.pos < len(c.line) && (c.line[c.pos] == ' ' || c.line[c.pos] == '\t') {
		c.skip(1)
	}
}

// rest returns the line from the cursor on. What is left of a tab the
// cursor stands in comes as spaces.
func (c *cursor) rest() string {
	if c.rem > 0 {
		return spaces(c.rem) + c.line[c.pos+1:]
	}
	return c.line[c.pos:]
}

// classify works out what a line is, given the blocks that are open. line
// is the line so far, without its line ending; complete says that it has
// ended. For a line that is not complete the answer is either a plan that
// no continuation of the line could change, or need: wait for more.
//
// Waiting is bounded, so that a line with no end cannot be kept back for
// ever. Three kinds of thing are waited for. A few bytes that follow a
// marker and decide what it is. The rest of a line that may be a construct
// which depends on the whole line (a thematic break, an underline, a
// fence, a delimiter row, a definition, a tag with the line to itself):
// such constructs are only recognised on lines of at most Reach bytes, so
// once a line is longer it is known not to be one. And the end of a run of
// blanks, which may be a blank line or indentation: a run of more than
// Reach bytes is neither, and what stands in it is text.
func (st *state) classify(cfg *config, line string, complete bool) plan {
	cfg.did(len(line))
	long := len(line) > cfg.reach
	whole := complete && !long // constructs that take the whole line can be told
	wait := !complete && !long // the line may still become such a construct
	more := !complete          // the line may still grow
	need := plan{need: true}

	// Which of the open containers does the line go on in?
	c := cursor{line: line}
	keep := 0
	for keep < len(st.frames) {
		f := &st.frames[keep]
		cols, at := c.indent()
		if at == len(line) && at-c.pos <= cfg.reach {
			if more {
				return need
			}
			// A blank line goes on in a list item, unless the item has been
			// empty so far; a block quote ends.
			if f.kind == fItem && !f.bare {
				keep++
				continue
			}
			break
		}
		if f.kind == fQuote {
			if cols > 3 || at == len(line) || line[at] != '>' {
				break
			}
			c.mark(cols, at, 1)
			if c.pos == len(line) && more {
				return need // a blank after the mark would belong to it
			}
			c.space()
		} else {
			if cols < f.indent {
				break
			}
			c.skip(f.indent)
		}
		keep++
	}
	all := keep == len(st.frames)

	// A code block or HTML block that the line still stands in takes it.
	lf := &st.leaf
	if all && lf.kind == lCode {
		cd := &lf.code
		cols, at := c.indent()
		blank := at == len(line) && at-c.pos <= cfg.reach
		switch cd.kind {
		case cFenced:
			if blank {
				if wait && cols <= 3 {
					return need // the closing fence may follow
				}
			} else if at < len(line) && cols <= 3 && line[at] == cd.ch {
				k := at
				for k < len(line) && line[k] == cd.ch {
					k++
				}
				e := k
				for e < len(line) && (line[e] == ' ' || line[e] == '\t') {
					e++
				}
				if e == len(line) {
					if wait {
						return need // the run may grow, or something may follow it
					}
					if whole && k-at >= cd.n {
						return plan{act: aFenceClose, keep: keep}
					}
				}
			}
			c.skip(min(cols, cd.ind))
			return plan{act: aCode, keep: keep, text: c.rest()}
		case cIndented:
			if blank {
				if more {
					return need
				}
				return plan{act: aCodeBlank, keep: keep}
			}
			if cols >= 4 {
				c.skip(4)
				return plan{act: aCode, keep: keep, text: c.rest()}
			}
		case cHTML:
			if cd.html < 6 || !blank {
				return plan{act: aCode, keep: keep, text: c.rest()}
			}
			if more {
				return need
			}
		}
	}
	para := lf.kind == lPara // the line may go on the open paragraph, lazily if need be
	table := all && lf.kind == lTable

	// What does the rest of the line start?
	var opens []opening
starts:
	for first := true; ; first = false {
		cols, at := c.indent()
		if at == len(line) && at-c.pos <= cfg.reach {
			if more {
				return need
			}
			return plan{act: aBlank, keep: keep, opens: opens}
		}
		canPara := para && len(opens) == 0
		if cols >= 4 {
			if canPara {
				break // indented code does not interrupt a paragraph
			}
			c.skip(4)
			return plan{act: aIndented, keep: keep, opens: opens, text: c.rest()}
		}
		if at == len(line) {
			break
		}
		rest := line[at:]
		direct := all && canPara // the paragraph stands right here, not in a container the line left
		if first && direct {
			if header, ok := lf.para.candidate(cfg); ok && strings.Trim(rest, "|-: \t") == "" {
				if wait {
					return need
				}
				if aligns, ok := parseDelim(rest); whole && ok && len(aligns) == len(splitRow(header)) {
					return plan{act: aTable, keep: keep, aligns: aligns}
				}
			}
		}
		// Containers nest only so deep: a marker past that is text.
		deep := keep+len(opens) >= maxDepth
		switch ch := rest[0]; {
		case ch == '>' && !deep:
			c.mark(cols, at, 1)
			if c.pos == len(line) && more {
				return need
			}
			c.space()
			opens = append(opens, opening{})
			continue starts

		case ch == '#':
			n := 1
			for n < len(rest) && n < 7 && rest[n] == '#' {
				n++
			}
			if n == len(rest) && more {
				return need
			}
			if n <= 6 && (n == len(rest) || rest[n] == ' ' || rest[n] == '\t') {
				return plan{act: aHeading, keep: keep, opens: opens, level: n, text: rest[n:]}
			}

		case ch == '`' || ch == '~':
			n := 1
			for n < len(rest) && rest[n] == ch {
				n++
			}
			if n == len(rest) && wait {
				return need
			}
			if n >= 3 && !long && !(ch == '`' && strings.IndexByte(rest[n:], '`') >= 0) {
				if wait {
					return need // what follows the fence belongs to it
				}
				return plan{act: aFenceOpen, keep: keep, opens: opens, ch: ch, n: n, ind: cols, info: strings.TrimSpace(rest[n:])}
			}

		case ch == '<':
			kind, unsure := htmlStart(rest, more, !long && !canPara)
			if unsure {
				return need
			}
			if kind > 0 {
				return plan{act: aHTML, keep: keep, opens: opens, level: kind, text: c.rest()}
			}

		case ch == '=':
			if direct && lf.para.setextOK() && strings.Trim(strings.TrimRight(rest, " \t"), "=") == "" {
				if wait {
					return need
				}
				if whole {
					return plan{act: aSetext, keep: keep, level: 1}
				}
			}

		case ch == '-' || ch == '*' || ch == '_' || ch == '+':
			if ch != '+' {
				count, pure := 0, true
				for k := 0; k < len(rest) && pure; k++ {
					if rest[k] == ch {
						count++
					} else if rest[k] != ' ' && rest[k] != '\t' {
						pure = false
					}
				}
				if pure {
					if wait {
						return need // a break, an underline, or the start of a list
					}
					if whole {
						if ch == '-' && direct && lf.para.setextOK() && len(strings.TrimRight(rest, " \t")) == count {
							return plan{act: aSetext, keep: keep, level: 2}
						}
						if count >= 3 {
							return plan{act: aThematic, keep: keep, opens: opens}
						}
					}
				}
			}
			if ch == '_' || deep || len(rest) > 1 && rest[1] != ' ' && rest[1] != '\t' {
				break
			}
			if len(rest) == 1 && more {
				return need
			}
			op, ok, unsure := itemStart(&c, cols, at, 1, opening{item: true, delim: ch}, direct, more, cfg.reach)
			if unsure {
				return need
			}
			if ok {
				opens = append(opens, op)
				continue starts
			}

		case ch >= '0' && ch <= '9' && !deep:
			k := 1
			for k < len(rest) && k < 10 && rest[k] >= '0' && rest[k] <= '9' {
				k++
			}
			if k == len(rest) {
				if more {
					return need
				}
				break
			}
			if k > 9 || rest[k] != '.' && rest[k] != ')' {
				break
			}
			if k+1 == len(rest) && more {
				return need
			}
			if k+1 < len(rest) && rest[k+1] != ' ' && rest[k+1] != '\t' {
				break
			}
			num, _ := strconv.Atoi(rest[:k])
			op, ok, unsure := itemStart(&c, cols, at, k+1, opening{item: true, ordered: true, delim: rest[k], num: num, digits: rest[:k]}, direct, more, cfg.reach)
			if unsure {
				return need
			}
			if ok {
				opens = append(opens, op)
				continue starts
			}
		}
		break
	}

	// A line that starts nothing new is a row of the open table,
	if table && len(opens) == 0 && len(line) <= rowMax {
		if more {
			return need
		}
		return plan{act: aRow, keep: keep, text: strings.TrimLeft(c.rest(), " \t")}
	}
	cols, at := c.indent()
	if para && len(opens) == 0 {
		// or goes on the open paragraph,
		return plan{act: aText, cont: true, keep: len(st.frames), text: line[at:]}
	}
	// or is a link reference definition,
	if at < len(line) && line[at] == '[' && cols <= 3 {
		label, url, title, ok, unsure := parseDef(line[at:], wait, whole)
		if unsure {
			return need
		}
		if ok {
			return plan{act: aDef, keep: keep, opens: opens, label: label, url: url, title: title}
		}
	}
	// or starts a paragraph.
	return plan{act: aText, keep: keep, opens: opens, text: line[at:]}
}

// itemStart reads the start of a list item whose marker, n bytes long,
// stands at position at behind cols columns of indentation. It reports
// whether the line starts an item there, and moves the cursor to the
// item's content if it does. interrupts says that the item would cut a
// paragraph short that the line could otherwise go on: it may do that only
// if it has something in it and, when numbered, starts at one. more says
// that the line may still grow, and unsure is set when it is too short to
// tell. An item is empty if only blanks follow its marker, but a run of
// more than reach blanks is not waited on: it is content.
func itemStart(c *cursor, cols, at, n int, op opening, interrupts, more bool, reach int) (_ opening, ok, unsure bool) {
	line := c.line
	c2 := *c
	c2.mark(cols, at, n)
	sp, at2 := c2.indent()
	empty := at2 == len(line) && at2-c2.pos <= reach
	if empty && more {
		return op, false, true
	}
	if interrupts && (empty || op.ordered && op.num != 1) {
		return op, false, false
	}
	width := n + sp
	switch {
	case empty:
		width = n + 1
		c2.pos, c2.rem = len(line), 0
	case sp >= 5:
		width = n + 1 // the rest of the indentation makes indented code
		c2.skip(1)
	default:
		c2.skip(sp)
		if box := line[c2.pos:]; box[0] == '[' {
			// A task list item: "[ ]" or "[x]" and a blank.
			if len(box) < 4 {
				if more && (len(box) == 1 || strings.IndexByte(" xX", box[1]) >= 0 && (len(box) == 2 || box[2] == ']')) {
					return op, false, true
				}
			} else if strings.IndexByte(" xX", box[1]) >= 0 && box[2] == ']' && (box[3] == ' ' || box[3] == '\t') {
				op.task = 1
				if box[1] != ' ' {
					op.task = 2
				}
				c2.pos += 3
				c2.col += 3
			}
		}
	}
	op.indent = cols + width
	op.bare = empty
	*c = c2
	return op, true, false
}

// parseDef reads a link reference definition that takes a line to itself:
// "[label]: address" with an optional title in quotes after it. s starts
// with "[". wait says that the line may still grow, and whole that it is
// complete and short enough to be one.
func parseDef(s string, wait, whole bool) (label, url, title string, ok, more bool) {
	e, label, good, cut := parseLinkLabel(s, 0, noLimit, !wait)
	if cut {
		return "", "", "", false, true
	}
	if !good || label == "" {
		return "", "", "", false, false
	}
	if e == len(s) {
		return "", "", "", false, wait
	}
	if s[e] != ':' {
		return "", "", "", false, false
	}
	if wait {
		return "", "", "", false, true
	}
	if !whole {
		return "", "", "", false, false
	}
	r := reader{src: s, lim: noLimit, eof: true}
	j := r.skipSpace(e + 1)
	k, url, good := r.dest(j)
	if !good || k == j {
		return "", "", "", false, false
	}
	t := r.skipSpace(k)
	if t == len(s) {
		return label, url, "", true, false
	}
	if t == k || strings.IndexByte(`"'(`, s[t]) < 0 {
		return "", "", "", false, false
	}
	end, title, good := r.title(t)
	if !good || strings.TrimLeft(s[end:], " \t") != "" {
		return "", "", "", false, false
	}
	return label, url, title, true, false
}

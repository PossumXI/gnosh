package editor

import (
	"strings"
	"unicode"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// minText is the least room for text beside the prefix. In a terminal
// narrower than the prefix plus this, the prefix is left out.
const minText = 4

// minDetail is the least room in which the few words beside a candidate
// are worth showing. With less, the candidates stand alone.
const minDetail = 16

// lookMatch is the look of text that matched the history search. The
// other looks are the unit kinds.
const lookMatch uint8 = 100

// lookSelect is the look of text a VISUAL selection holds.
const lookSelect uint8 = 101

// vrow is one row of the text as it is shown: row ri of line li, which
// is row g of the whole text.
type vrow struct{ li, ri, g int }

// measurePrefix works out how many cells the prefix takes.
func (e *Editor) measurePrefix() {
	e.pw = e.m.Width(e.th.Prompt)
	if !e.plain {
		e.pw = max(e.pw, e.m.Width(e.th.ShellPrompt), e.m.Width(e.th.CommandPrompt))
	}
}

// Render says what the box looks like in a terminal width cells wide:
// its rows, and where the cursor is. It also sets the width that moving
// up and down goes by, and scrolls the rows shown so that the cursor's row
// is among them. Rendering twice in a row gives the same view.
func (e *Editor) Render(width int) View {
	v := View{CursorRow: -1}
	if width < 1 {
		return v
	}
	pw := e.pw
	if width-pw < minText {
		pw = 0
	}
	e.setWidth(width - pw)

	if e.IsEmpty() && e.placeholder != "" && e.srch == nil {
		v.Lines = append(v.Lines, e.prefix(pw, true, "")+e.th.Placeholder.Render(e.cutWords(Visible(e.placeholder), max(e.cw-1, 0))))
		if !e.locked {
			v.CursorRow, v.CursorCol = 0, pw
		}
	} else {
		// Keep the cursor's row among the rows shown.
		g, _ := e.cursorRow()
		if g < e.top {
			e.top = g
		}
		if g >= e.top+e.maxHeight {
			e.top = g - e.maxHeight + 1
		}
		rows, below := e.window()
		var spans []Span
		spansFor := -1
		look := lookMatch
		selecting := e.vim.mode.visual() && e.srch == nil
		var selFrom, selTo pos
		if selecting {
			selFrom, selTo, _ = e.vimSelection()
			look = lookSelect
		}
		for i, vr := range rows {
			mark := ""
			above := i == 0 && e.top > 0
			under := i == len(rows)-1 && below
			switch {
			case above && under:
				mark = e.th.MoreBoth
			case above:
				mark = e.th.MoreAbove
			case under:
				mark = e.th.MoreBelow
			}
			l := &e.lines[vr.li]
			if e.srch != nil && spansFor != vr.li {
				spans, spansFor = e.matchSpans(l.s), vr.li
			}
			if selecting && spansFor != vr.li {
				spans, spansFor = selectSpans(vr.li, len(l.s), selFrom, selTo), vr.li
			}
			v.Lines = append(v.Lines, e.prefix(pw, vr.g == 0, mark)+e.rowText(l, l.rows[vr.ri], spans, look))
		}
		if !e.locked && e.srch == nil {
			v.CursorRow, v.CursorCol = g-e.top, pw+e.cursorCell()
		}
	}
	if e.locked {
		return v
	}
	switch {
	case e.srch != nil:
		lines, col := e.searchLines(width, pw)
		v.CursorRow, v.CursorCol = len(v.Lines), col
		v.Lines = append(v.Lines, lines...)
	case e.comp != nil:
		v.Lines = append(v.Lines, e.menuLines(width)...)
	}
	return v
}

// window returns the rows of the text that are shown, starting at e.top,
// and whether there are rows below them. When the text has become too
// short to fill the window from e.top, e.top moves up.
func (e *Editor) window() (rows []vrow, below bool) {
	for {
		rows = rows[:0]
		g := 0
		for li := range e.lines {
			n := len(e.laid(li).rows)
			for ri := max(e.top-g, 0); ri < n; ri++ {
				if len(rows) == e.maxHeight {
					return rows, true
				}
				rows = append(rows, vrow{li, ri, g + ri})
			}
			g += n
		}
		// The end of the text was reached; g is how many rows it has.
		if top := max(g-e.maxHeight, 0); top < e.top {
			e.top = top
			continue
		}
		return rows, false
	}
}

// prefix returns what stands before a row: on the first row of the text
// the mark of the mode, on the others an indent as wide, with mark in it
// when rows are hidden beyond this one.
func (e *Editor) prefix(pw int, first bool, mark string) string {
	if pw == 0 {
		return ""
	}
	if !first {
		if w := e.m.Width(mark); mark != "" && w <= pw {
			return e.style(e.th.More).Render(mark) + strings.Repeat(" ", pw-w)
		}
		return strings.Repeat(" ", pw)
	}
	s, st := e.th.Prompt, e.th.PromptStyle
	switch e.Mode() {
	case ModeShell:
		s, st = e.th.ShellPrompt, e.th.ShellStyle
	case ModeCommand:
		s, st = e.th.CommandPrompt, e.th.CommandStyle
	}
	return e.style(st).Render(s) + strings.Repeat(" ", max(pw-e.m.Width(s), 0))
}

// style is st, or the locked look while the box is locked.
func (e *Editor) style(st text.Style) text.Style {
	if e.locked {
		return e.th.Locked
	}
	return st
}

// selectSpans is the part of line li, n bytes long, that a selection from
// a up to b holds.
func selectSpans(li, n int, a, b pos) []Span {
	if li < a.line || li > b.line {
		return nil
	}
	start, end := 0, n
	if li == a.line {
		start = a.col
	}
	if li == b.line {
		end = b.col
	}
	if end <= start {
		return nil
	}
	return []Span{{start, end}}
}

// rowText draws the items of one row. spans are the parts of the line
// drawn with spanLook: what matched the history search (lookMatch), or
// what a VISUAL selection holds (lookSelect).
func (e *Editor) rowText(l *line, r row, spans []Span, spanLook uint8) string {
	var out, run strings.Builder
	look := uint8(255)
	flush := func() {
		if run.Len() == 0 {
			return
		}
		st := e.th.Text
		switch look {
		case unitPaste:
			st = e.th.Paste
		case unitMark:
			st = e.th.Control
		case lookMatch:
			st = e.th.Match
		case lookSelect:
			st = e.th.Selection
		}
		out.WriteString(e.style(st).Render(run.String()))
		run.Reset()
	}
	cell := 0
	for off := r.start; off < r.end; {
		u := e.unitAt(l, off, cell)
		s, lk := e.draw(l, off, u)
		if lk == unitText {
			for _, sp := range spans {
				if off >= sp.Start && off < sp.End {
					lk = spanLook
					break
				}
			}
		}
		if lk != look {
			flush()
			look = lk
		}
		run.WriteString(s)
		cell += u.w
		off = u.end
	}
	flush()
	return out.String()
}

// matchSpans returns the parts of a line that match the history search,
// for marking them in the entry that was found.
func (e *Editor) matchSpans(s string) []Span {
	q := e.srch.query
	if q == "" || e.srch.idx < 0 {
		return nil
	}
	hay := s
	if strings.IndexFunc(q, unicode.IsUpper) < 0 {
		// Lower case can be longer or shorter than the text it came
		// from; the offsets are only good when it is not.
		q, hay = strings.ToLower(q), strings.ToLower(s)
		if len(hay) != len(s) {
			return nil
		}
	}
	var spans []Span
	for at := 0; ; {
		i := strings.Index(hay[at:], q)
		if i < 0 {
			return spans
		}
		spans = append(spans, Span{at + i, at + i + len(q)})
		at += i + len(q)
	}
}

// searchLines draws the history search under the box: the line the
// search is typed on and a line that names the keys. It returns the cell
// the cursor is at on the first of them.
func (e *Editor) searchLines(width, pw int) (lines []string, cursorCol int) {
	s := e.srch
	indent := pw
	if width-indent < 12 {
		indent = 0
	}
	label := "history search: "
	switch {
	case width-indent < 12:
		label = ""
	case width-indent < 40:
		label = "search: "
	}
	// One cell is kept for the cursor.
	room := max(width-indent-len(label)-1, 0)
	q := e.cutLeft(Visible(s.query), room)
	cursorCol = indent + len(label) + e.m.Width(q)
	line := strings.Repeat(" ", indent) + e.th.SearchLabel.Render(label) + q
	note := ""
	switch {
	case s.query != "" && s.idx < 0:
		note = "  no match"
	case s.failed:
		note = "  no more matches"
	}
	if note != "" && cursorCol+1+len(note) <= width {
		line += " " + e.th.Note.Render(note)
	}
	lines = append(lines, line)
	keys := fitHint([]string{"Enter send", "Tab edit", "Ctrl+R older", "Ctrl+C cancel"}, width-indent, e.m)
	if keys != "" {
		lines = append(lines, strings.Repeat(" ", indent)+e.th.Note.Render(keys))
	}
	return lines, cursorCol
}

// fitHint joins as many of parts, from the first, as fit in width cells.
func fitHint(parts []string, width int, m text.Metrics) string {
	const sep = " · "
	out := ""
	for i, p := range parts {
		next := p
		if i > 0 {
			next = out + sep + p
		}
		if m.Width(next) > width {
			break
		}
		out = next
	}
	return out
}

// menuLines draws the list of candidates under the box, scrolled so that
// the one chosen shows.
func (e *Editor) menuLines(width int) []string {
	c := e.comp
	moreW := max(e.m.Width(e.th.MoreAbove), e.m.Width(e.th.MoreBelow), e.m.Width(e.th.MoreBoth), 1)
	ptrW := max(e.m.Width(e.th.Pointer), 1)
	avail := width - moreW - ptrW - 2
	if avail < 3 {
		return nil
	}
	h := min(e.menuHeight, len(c.items))
	if c.sel < c.top {
		c.top = c.sel
	}
	if c.sel >= c.top+h {
		c.top = c.sel - h + 1
	}
	c.top = min(max(c.top, 0), len(c.items)-h)

	// The few words beside each candidate start in one column.
	labels := make([]string, len(c.items))
	labelW := 0
	for i, it := range c.items {
		label := it.Label
		if label == "" {
			label = strings.TrimRight(it.Text, " ")
		}
		labels[i] = e.cutLeft(Visible(label), avail)
		labelW = max(labelW, e.m.Width(labels[i]))
	}
	detailRoom := avail - labelW - 2

	lines := make([]string, 0, h)
	for i := c.top; i < c.top+h; i++ {
		mark := ""
		above := i == c.top && c.top > 0
		under := i == c.top+h-1 && c.top+h < len(c.items)
		switch {
		case above && under:
			mark = e.th.MoreBoth
		case above:
			mark = e.th.MoreAbove
		case under:
			mark = e.th.MoreBelow
		}
		var b strings.Builder
		b.WriteString(e.th.More.Render(mark))
		b.WriteString(strings.Repeat(" ", moreW-e.m.Width(mark)+1))
		st := e.th.Candidate
		if i == c.sel {
			st = e.th.Selected
			b.WriteString(st.Render(e.th.Pointer))
			b.WriteString(strings.Repeat(" ", ptrW-e.m.Width(e.th.Pointer)+1))
		} else {
			b.WriteString(strings.Repeat(" ", ptrW+1))
		}
		b.WriteString(st.Render(labels[i]))
		if detail := c.items[i].Detail; detail != "" && detailRoom >= minDetail {
			b.WriteString(strings.Repeat(" ", labelW-e.m.Width(labels[i])+2))
			b.WriteString(e.th.Detail.Render(e.m.Truncate(Visible(detail), detailRoom, "…")))
		}
		lines = append(lines, b.String())
	}
	return lines
}

// cutWords shortens s to width cells at the end of a word, with nothing
// added, for words that are decoration and may be cut.
func (e *Editor) cutWords(s string, width int) string {
	if e.m.Width(s) <= width {
		return s
	}
	cut := e.m.Truncate(s, width, "")
	if i := strings.LastIndexByte(cut, ' '); i > 0 && len(cut) < len(s) && s[len(cut)] != ' ' {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ")
}

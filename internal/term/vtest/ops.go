package vtest

// This file is what the screen does: printing, moving, scrolling, erasing,
// inserting and resizing. The parser in parser.go decides which to do.

// breakWide blanks both halves of a wide character when the cell at col is
// one of them, so that half a character is never left behind.
func (s *Screen) breakWide(row, col int) {
	cells := s.lines[row].cells
	if col < 0 || col >= len(cells) {
		return
	}
	blank := s.blankCell()
	switch {
	case cells[col].Width == 0 && col > 0:
		cells[col-1] = blank
		cells[col] = blank
	case cells[col].Width == 2:
		cells[col] = blank
		if col+1 < len(cells) {
			cells[col+1] = blank
		}
	}
}

// put prints one character at the cursor.
func (s *Screen) put(r rune) {
	w := s.width(string(r))
	if w <= 0 {
		if w == 0 {
			s.combine(r)
		}
		return
	}
	if w > 2 {
		w = 2
	}
	if w > s.cols {
		return
	}
	if s.pending {
		// The deferred wrap: the last column was filled earlier, and
		// only now, with another character to print, does the cursor
		// move to the next line.
		s.pending = false
		if s.autowrap {
			s.lines[s.row].wrapped = true
			s.col = 0
			s.lineFeed()
		}
	}
	if w == 2 && s.col == s.cols-1 {
		// A wide character does not fit in one remaining cell.
		if !s.autowrap {
			return
		}
		s.lines[s.row].wrapped = true
		s.col = 0
		s.lineFeed()
	}
	cells := s.lines[s.row].cells
	if s.insertMode {
		s.insertChars(w)
	}
	s.breakWide(s.row, s.col)
	if w == 2 {
		s.breakWide(s.row, s.col+1)
	}
	cells[s.col] = Cell{Text: string(r), Width: w, Attr: s.attr}
	if w == 2 {
		cells[s.col+1] = Cell{Width: 0, Attr: s.attr}
	}
	s.lines[s.row].used = max(s.lines[s.row].used, s.col+w)
	s.lastChar = r
	if s.col+w >= s.cols {
		s.col = s.cols - 1
		s.pending = s.autowrap
	} else {
		s.col += w
	}
}

// combine adds a combining mark to the character before the cursor. When
// a wrap is pending, that is the character in the last column, where the
// cursor still is.
//
// With the terminal's wrapping turned off there is no pending wrap, and
// the cursor sits on top of a character written in the last column; a
// mark that follows then joins the character one cell to the left, which
// is the wrong one. tmux does exactly this. (xterm remembers where it last
// wrote and gets it right.) The screen follows tmux here, so that a
// program which is careless at the right edge fails its tests.
func (s *Screen) combine(r rune) {
	col := s.col - 1
	if s.pending {
		col = s.col
	}
	cells := s.lines[s.row].cells
	if col >= 0 && cells[col].Width == 0 {
		col--
	}
	if col < 0 || cells[col].Text == "" {
		return
	}
	cells[col].Text += string(r)
}

// lineFeed moves the cursor down a line, scrolling when it is on the last
// line of the scroll region. The column does not change.
func (s *Screen) lineFeed() {
	switch {
	case s.row == s.bot:
		s.scrollUp(1)
	case s.row < s.rows-1:
		s.row++
	}
}

// reverseIndex moves the cursor up a line, scrolling down when it is on
// the first line of the scroll region.
func (s *Screen) reverseIndex() {
	switch {
	case s.row == s.top:
		s.scrollDown(1)
	case s.row > 0:
		s.row--
	}
}

// scrollUp moves the scroll region's text up n lines. A line that leaves
// the top of the whole screen goes to the scrollback.
func (s *Screen) scrollUp(n int) {
	for ; n > 0; n-- {
		if s.top == 0 && !s.inAlt {
			s.scrollback = append(s.scrollback, s.lines[s.top])
		}
		copy(s.lines[s.top:s.bot], s.lines[s.top+1:s.bot+1])
		s.lines[s.bot] = s.blankLine()
	}
}

// scrollDown moves the scroll region's text down n lines; the lines that
// leave the bottom are lost.
func (s *Screen) scrollDown(n int) {
	for ; n > 0; n-- {
		copy(s.lines[s.top+1:s.bot+1], s.lines[s.top:s.bot])
		s.lines[s.top] = s.blankLine()
	}
}

// eraseCells blanks the cells of a row from column from up to, not
// including, column to.
func (s *Screen) eraseCells(row, from, to int) {
	if from < 0 {
		from = 0
	}
	if to > s.cols {
		to = s.cols
	}
	blank := s.blankCell()
	for c := from; c < to; c++ {
		s.breakWide(row, c)
		s.lines[row].cells[c] = blank
	}
}

// eraseLine is EL: 0 erases from the cursor to the end of the line, 1 from
// the start of the line through the cursor, 2 the whole line.
func (s *Screen) eraseLine(mode int) {
	switch mode {
	case 0:
		s.eraseCells(s.row, s.col, s.cols)
		s.lines[s.row].wrapped = false
		if s.col == 0 {
			s.lines[s.row].used = 0
		}
	case 1:
		s.eraseCells(s.row, 0, s.col+1)
	case 2:
		s.eraseCells(s.row, 0, s.cols)
		s.lines[s.row].wrapped = false
		s.lines[s.row].used = 0
	}
	s.pending = false
}

// eraseDisplay is ED: 0 erases from the cursor to the end of the screen, 1
// from the start of the screen through the cursor, 2 the whole screen, 3
// the scrollback.
func (s *Screen) eraseDisplay(mode int) {
	if s.clearSaves && !s.inAlt && (mode == 2 || mode == 0 && s.row == 0 && s.col == 0) {
		// tmux's scroll-on-clear: the lines in use go to the
		// scrollback, and the screen is left empty.
		used := 0
		for r, l := range s.lines {
			if !blankRow(l) {
				used = r + 1
			}
		}
		s.scrollback = append(s.scrollback, s.lines[:used]...)
		for r := range s.lines {
			s.lines[r] = s.blankLine()
		}
		s.pending = false
		return
	}
	switch mode {
	case 0:
		s.eraseLine(0)
		for r := s.row + 1; r < s.rows; r++ {
			s.lines[r] = s.blankLine()
		}
	case 1:
		for r := 0; r < s.row; r++ {
			s.lines[r] = s.blankLine()
		}
		s.eraseLine(1)
	case 2:
		for r := range s.lines {
			s.lines[r] = s.blankLine()
		}
		s.pending = false
	case 3:
		s.scrollback = nil
	}
}

// insertChars is ICH: n blank cells open up at the cursor and the rest of
// the line moves right, losing what passes the edge.
func (s *Screen) insertChars(n int) {
	cells := s.lines[s.row].cells
	s.breakWide(s.row, s.col)
	if n > s.cols-s.col {
		n = s.cols - s.col
	}
	copy(cells[s.col+n:], cells[s.col:s.cols-n])
	blank := s.blankCell()
	for c := s.col; c < s.col+n; c++ {
		cells[c] = blank
	}
	if last := cells[s.cols-1]; last.Width == 2 {
		cells[s.cols-1] = blank
	}
	s.pending = false
}

// deleteChars is DCH: n cells at the cursor go and the rest of the line
// moves left, with blanks coming in at the right.
func (s *Screen) deleteChars(n int) {
	cells := s.lines[s.row].cells
	s.breakWide(s.row, s.col)
	if n > s.cols-s.col {
		n = s.cols - s.col
	}
	copy(cells[s.col:], cells[s.col+n:])
	blank := s.blankCell()
	for c := s.cols - n; c < s.cols; c++ {
		cells[c] = blank
	}
	if cells[s.col].Width == 0 {
		cells[s.col] = blank
	}
	s.pending = false
}

// insertLines is IL: n blank lines open up at the cursor's row and the
// lines below move down within the scroll region.
func (s *Screen) insertLines(n int) {
	if s.row < s.top || s.row > s.bot {
		return
	}
	for ; n > 0; n-- {
		copy(s.lines[s.row+1:s.bot+1], s.lines[s.row:s.bot])
		s.lines[s.row] = s.blankLine()
	}
	s.pending = false
}

// deleteLines is DL: n lines at the cursor's row go and the lines below
// move up within the scroll region.
func (s *Screen) deleteLines(n int) {
	if s.row < s.top || s.row > s.bot {
		return
	}
	for ; n > 0; n-- {
		copy(s.lines[s.row:s.bot], s.lines[s.row+1:s.bot+1])
		s.lines[s.bot] = s.blankLine()
	}
	s.pending = false
}

// up and down move the cursor without scrolling. They stop at the scroll
// region's edge when the cursor starts inside it, and at the screen's edge
// otherwise.
func (s *Screen) up(n int) {
	limit := 0
	if s.row >= s.top {
		limit = s.top
	}
	s.row = max(s.row-n, limit)
	s.pending = false
}

func (s *Screen) down(n int) {
	limit := s.rows - 1
	if s.row <= s.bot {
		limit = s.bot
	}
	s.row = min(s.row+n, limit)
	s.pending = false
}

// setCol puts the cursor on a column, counted from 0, kept on the screen.
func (s *Screen) setCol(col int) {
	s.col = min(max(col, 0), s.cols-1)
	s.pending = false
}

// setRow puts the cursor on a row, counted from 0. In origin mode rows are
// counted from the top of the scroll region and kept inside it.
func (s *Screen) setRow(row int) {
	if s.origin {
		s.row = min(max(row+s.top, s.top), s.bot)
	} else {
		s.row = min(max(row, 0), s.rows-1)
	}
	s.pending = false
}

// tab moves the cursor to the next tab stop (every eighth column), or the
// last column.
func (s *Screen) tab() {
	s.setCol((s.col/8 + 1) * 8)
}

// backTab moves the cursor to the previous tab stop.
func (s *Screen) backTab() {
	if s.col == 0 {
		return
	}
	s.setCol((s.col - 1) / 8 * 8)
}

func (s *Screen) saveCursor() {
	s.saved = savedCursor{row: s.row, col: s.col, attr: s.attr, origin: s.origin, pending: s.pending}
}

func (s *Screen) restoreCursor() {
	s.row = min(s.saved.row, s.rows-1)
	s.col = min(s.saved.col, s.cols-1)
	s.attr = s.saved.attr
	s.origin = s.saved.origin
	s.pending = s.saved.pending && s.col == s.cols-1
}

// useAlt switches between the main screen and the alternate one.
func (s *Screen) useAlt(on bool) {
	if on == s.inAlt {
		return
	}
	if on {
		s.altEver = true
		s.altSaved = s.lines
		s.lines = make([]line, s.rows)
		for i := range s.lines {
			s.lines[i] = s.blankLine()
		}
	} else {
		s.lines = s.altSaved
		s.altSaved = nil
	}
	s.inAlt = on
	s.pending = false
}

// fitLine makes a row cols wide, cutting it or padding it with blanks.
func (s *Screen) fitLine(l line, cols int) line {
	switch {
	case len(l.cells) > cols:
		l.cells = l.cells[:cols:cols]
		if l.cells[cols-1].Width == 2 {
			l.cells[cols-1] = Cell{Width: 1}
		}
		l.used = min(l.used, cols)
	case len(l.cells) < cols:
		cells := make([]Cell, cols)
		copy(cells, l.cells)
		for i := len(l.cells); i < cols; i++ {
			cells[i] = Cell{Width: 1}
		}
		l.cells = cells
	}
	return l
}

// Resize changes the screen's size, as a person dragging the window's edge
// does. Nothing is wrapped again unless SetReflow asked for it: a narrower
// screen cuts lines off and a wider one adds blank cells. A shorter screen
// loses the rows below the cursor first, and then its top rows go to the
// scrollback so that the cursor stays in view; a taller one gains blank
// rows at the bottom. The scroll region goes back to the whole screen.
func (s *Screen) Resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if s.reflow && !s.inAlt {
		s.resizeReflow(cols, rows)
		return
	}
	if cols != s.cols {
		for i := range s.lines {
			s.lines[i] = s.fitLine(s.lines[i], cols)
		}
		for i := range s.altSaved {
			s.altSaved[i] = s.fitLine(s.altSaved[i], cols)
		}
		s.cols = cols
	}
	if rows < s.rows {
		excess := s.rows - rows
		cut := min(excess, s.rows-1-s.row)
		s.lines = s.lines[:s.rows-cut]
		for excess -= cut; excess > 0; excess-- {
			if !s.inAlt {
				s.scrollback = append(s.scrollback, s.lines[0])
			}
			s.lines = s.lines[1:]
			s.row--
		}
	}
	s.rows = rows
	for len(s.lines) < rows {
		s.lines = append(s.lines, s.blankLine())
	}
	for len(s.altSaved) > 0 && len(s.altSaved) != rows {
		if len(s.altSaved) > rows {
			s.altSaved = s.altSaved[:rows]
		} else {
			s.altSaved = append(s.altSaved, s.blankLine())
		}
	}
	s.top, s.bot = 0, rows-1
	s.row = min(max(s.row, 0), rows-1)
	s.col = min(s.col, cols-1)
	s.pending = false
}

// resizeReflow is Resize for a terminal that wraps its text again: every
// run of rows that one long line was wrapped into is joined and cut again
// at the new width, in the scrollback as well as on the screen. The cursor
// stays on the character it was on. The bottom of the text stays in view.
func (s *Screen) resizeReflow(cols, rows int) {
	all := append(append([]line{}, s.scrollback...), s.lines...)
	curRow := len(s.scrollback) + s.row
	last := curRow
	for i := len(all) - 1; i > last; i-- {
		if !blankRow(all[i]) {
			last = i
			break
		}
	}
	all = all[:last+1]

	var out []line
	newRow, newCol := 0, 0
	blankOut := func() line {
		cells := make([]Cell, cols)
		for i := range cells {
			cells[i] = Cell{Width: 1}
		}
		return line{cells: cells}
	}
	for i := 0; i < len(all); i++ {
		// Join the rows of one long line, and find the cursor in it.
		var cells []Cell
		curOff := -1
		used := 0
		for {
			if i == curRow {
				curOff = len(cells) + s.col
				if s.pending {
					curOff++
				}
			}
			used = len(cells) + min(all[i].used, len(all[i].cells))
			cells = append(cells, all[i].cells...)
			if !all[i].wrapped || i == len(all)-1 {
				break
			}
			i++
		}
		// The line is as long as its text, or as far as it was ever
		// written if that is further (tmux's rule).
		n := len(cells)
		for n > used && cells[n-1] == (Cell{Width: 1}) {
			n--
		}
		if curOff >= 0 && curOff < n && cells[curOff].Width == 0 {
			curOff--
		}
		// Lay the line out again at the new width.
		row, col := blankOut(), 0
		placed := false
		for k := 0; k < n; k++ {
			c := cells[k]
			if c.Width == 0 {
				continue
			}
			if c.Width > cols {
				c = Cell{Width: 1, Attr: c.Attr}
			}
			if col+c.Width > cols {
				row.wrapped = true
				out = append(out, row)
				row, col = blankOut(), 0
			}
			if k == curOff {
				newRow, newCol, placed = len(out), col, true
			}
			row.cells[col] = c
			if c.Width == 2 {
				row.cells[col+1] = Cell{Width: 0, Attr: c.Attr}
			}
			col += c.Width
			row.used = col
		}
		out = append(out, row)
		if curOff >= 0 && !placed {
			// The cursor was past the end of the text.
			newRow, newCol = len(out)-1, min(col+curOff-n, cols-1)
		}
	}

	start := max(0, len(out)-rows)
	if newRow < start {
		start = newRow
	}
	end := min(start+rows, len(out))
	s.cols, s.rows = cols, rows
	s.scrollback = append([]line{}, out[:start]...)
	s.lines = append([]line{}, out[start:end]...)
	for len(s.lines) < rows {
		s.lines = append(s.lines, blankOut())
	}
	s.top, s.bot = 0, rows-1
	s.row, s.col = newRow-start, min(newCol, cols-1)
	s.pending = false
}

// blankRow reports whether nothing was ever written to a row.
func blankRow(l line) bool {
	for _, c := range l.cells {
		if c != (Cell{Width: 1}) {
			return false
		}
	}
	return true
}

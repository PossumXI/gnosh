package diff

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// RenderOptions say how Render draws a diff. The zero value draws it plain,
// without colour and without a width to wrap at.
type RenderOptions struct {
	// Width is the width of the terminal in cells. No returned line is
	// wider. Zero or less means no limit: nothing is wrapped. A width too
	// small to hold a sign, a space and a character two cells wide (less
	// than four) cannot always be kept.
	Width int
	// Profile says how much colour the output can take. The zero value is
	// text.NoColor.
	Profile text.Profile
	// Theme is the look. Nil means DefaultTheme for the Profile.
	Theme *Theme
	// Context is the number of unchanged lines shown around each change.
	// Zero means DefaultContext, a negative number means none.
	Context int
	// MaxLines is the most lines of changes to show: the lines under the
	// header, wrapped lines counted as many times as they wrap. Zero or
	// less means all of them. When there are more, the first are shown and
	// one more line says how many changed lines in how many hunks are not.
	MaxLines int
	// TabWidth is the distance between tab stops. Zero or less means 4.
	TabWidth int
	// Deleted says that the change removes the file itself. Without it, a
	// change that leaves no text is shown as a file that was emptied,
	// which is what writing nothing to a file does.
	Deleted bool
	// Metrics says how wide the terminal draws the characters whose width
	// Unicode leaves open. The zero value is what most terminals do.
	Metrics text.Metrics
	// Budget bounds the comparison, as Options.Budget does.
	Budget int
}

// The marks Render puts in the gutter.
const (
	markWrap = "↳" // this row goes on with the line above it
	markSkip = "⋮" // lines are left out here
)

const (
	// defaultTab is the distance between tab stops when none is given.
	defaultTab = 4
	// narrowest is the least room for text that line numbers are allowed
	// to leave. In a terminal narrower than that they are left out.
	narrowest = 12
)

// Render draws the change that d describes, turning its Before into its
// After, as lines for a terminal: each styled for opts.Profile, whole in
// itself, without a newline, and at most opts.Width cells wide. d's Path, or
// its Title when it has no Path, names the file; its Kind is not looked at.
//
// The first line is a header: the path, then the number of lines added and
// removed, as "+3 -1". A change that fills an empty Before says "new file"
// there, one that leaves an empty After says "emptied", or "deleted file"
// when opts.Deleted is set, and two texts that are the same say "no changes"
// and nothing more. Content that is not text, because it holds a NUL byte or
// is not valid UTF-8, is declined in that one line.
//
// Under the header come notes about what the lines below do not show: that
// the line endings changed, that the newline at the end of the file was
// added or removed. A change of nothing but line endings is told there and
// not as a change to every line. Only where the endings are mixed, so that
// no one sentence covers them, are such lines shown as changed, and then
// every carriage return is drawn, as "␍".
//
// Then the hunks. Each line has the number it has in the old text and the
// number it has in the new, in a gutter as wide as the largest number needs;
// a sign, "-" for a deleted line, "+" for an inserted one and a space for an
// unchanged one, so that nothing depends on colour; and the line's text.
// Where a deleted line and an inserted line are two forms of one line, the
// words that differ are marked in both. Deleted lines are paired with the
// inserted lines most like them, not simply in order, and where lines are
// replaced each deleted line stands right above what it turned into: lines
// that were only added before it come first.
//
// A line too long for the width goes on in the next row, with "↳" in the
// gutter in place of the sign. Nothing is cut and no space is dropped. Rows
// end between words where that leaves them at least half full, and at the
// width otherwise. Tabs are spaces up to the next tab stop, counted from
// the start of the text. Between two hunks one row says how many unchanged
// lines lie between them.
//
// Nothing in a line reaches the terminal as an instruction. Control
// characters are drawn as their pictures, and characters that would not show
// are drawn as their numbers (see the cells method). Spaces at the end of a
// changed line are drawn as "·", tabs there as "→", and so are the blanks
// that changed when a deleted and an inserted line would otherwise look the
// same.
//
// Render compares the two texts itself. It keeps no state: the same
// arguments give the same lines.
func Render(d core.Display, opts RenderOptions) []string {
	theme := DefaultTheme(opts.Profile)
	if opts.Theme != nil {
		theme = *opts.Theme
	}
	r := &renderer{
		m:     opts.Metrics,
		theme: theme.in(opts.Profile),
		width: max(opts.Width, 0),
		tab:   opts.TabWidth,
	}
	if r.tab <= 0 {
		r.tab = defaultTab
	}
	note := r.theme.Note.Render

	path := d.Path
	if path == "" {
		path = d.Title
	}
	before, after := d.Before, d.After
	switch {
	case before == after:
		return r.header(path, note("no changes"))
	case !isText(before) || !isText(after):
		return r.header(path, note("binary or not UTF-8 text: not shown ("+
			plural(len(before), "byte")+" before, "+grouped(len(after))+" after)"))
	}

	c := compare(before, after, limit(opts.Budget))
	stats := c.stats()
	oldEnds, newEnds := endingsOf(c.old), endingsOf(c.new)
	// A text that went from one kind of line ending to the other is told
	// in a note. Where a text mixes the two, each line shows its own.
	converted := oldEnds == endLF && newEnds == endCRLF || oldEnds == endCRLF && newEnds == endLF
	r.showCR = oldEnds == endMixed || newEnds == endMixed

	plus := r.theme.Inserted.Render("+" + grouped(stats.Added))
	minus := r.theme.Deleted.Render("-" + grouped(stats.Removed))
	var out []string
	switch {
	case len(c.old) == 0:
		out = r.header(path, note("new file"), plus)
	case len(c.new) == 0 && opts.Deleted:
		out = r.header(path, note("deleted file"), minus)
	case len(c.new) == 0:
		out = r.header(path, note("emptied"), minus)
	case stats.Added > 0 || stats.Removed > 0:
		out = r.header(path, plus+" "+minus)
	default:
		out = r.header(path)
	}

	var notes []string
	switch {
	case converted:
		notes = append(notes, "line endings changed: "+endingName(oldEnds)+" to "+endingName(newEnds))
	case stats.Endings > 0:
		notes = append(notes, "line endings changed on "+plural(stats.Endings, "line"))
	case oldEnds == endNone && newEnds == endCRLF:
		notes = append(notes, "line endings: CRLF")
	}
	switch {
	case stats.FinalNewline > 0:
		notes = append(notes, "newline added at end of file")
	case stats.FinalNewline < 0:
		notes = append(notes, "newline removed from end of file")
	case len(c.old) == 0 && !ended(c.new[len(c.new)-1]):
		notes = append(notes, "no newline at end of file")
	}
	if !c.minimal {
		notes = append(notes, "compared roughly, the texts being very different: every change is shown, though a closer look might find fewer")
	}
	for _, n := range notes {
		out = append(out, r.wrap(note(n))...)
	}

	hunks := group(c.display(converted), contextOf(opts.Context))
	if len(hunks) == 0 {
		return out
	}
	r.layout(c, hunks)
	return r.body(out, c, hunks, opts.MaxLines)
}

// A renderer holds what Render works out once and uses for every line.
type renderer struct {
	m     text.Metrics
	theme Theme // every style set to the profile
	width int   // the terminal's width, or zero for no limit
	tab   int   // the distance between tab stops

	digits int    // the width of a line number, or zero when numbers are left out
	single bool   // one column of numbers and not two: one of the texts is empty
	blank  string // the number columns of a row that has no numbers
	room   int    // the cells left for text, or zero for no limit
	showCR bool   // carriage returns at the ends of lines are drawn

	scratch []cell // one line's cells, reused
}

// header makes the first line: the path, then each of parts, two spaces
// apart. In a terminal too narrow for all of it the path has a line to
// itself, wrapped if it must be, and the parts follow on the next.
func (r *renderer) header(path string, parts ...string) []string {
	if path != "" {
		path = r.theme.Path.Render(r.clean(path))
	}
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("  ")
		}
		b.WriteString(p)
	}
	rest := b.String()
	switch {
	case path == "" && rest == "":
		return nil
	case path == "":
		return r.wrap(rest)
	case rest == "":
		return r.wrap(path)
	}
	if whole := path + "  " + rest; r.width <= 0 || r.m.VisibleWidth(whole) <= r.width {
		return []string{whole}
	}
	return append(r.wrap(path), r.wrap(rest)...)
}

// wrap breaks a line of words to the terminal's width.
func (r *renderer) wrap(s string) []string {
	if r.width <= 0 {
		return []string{s}
	}
	return r.m.Wrap(s, r.width, text.WrapOptions{})
}

// layout sizes the gutter: numbers as wide as the largest one shown, in two
// columns, or in one when one of the texts is empty, and none at all when
// they would leave too little room for the text.
func (r *renderer) layout(c *comparison, hunks []hunkRuns) {
	last := hunks[len(hunks)-1]
	r.digits = len(strconv.Itoa(max(last.a1, last.b1)))
	r.single = len(c.old) == 0 || len(c.new) == 0
	numbers := 2*r.digits + 2
	if r.single {
		numbers = r.digits + 1
	}
	if r.width > 0 && r.width-numbers-2 < narrowest {
		r.digits, numbers = 0, 0
	}
	r.blank = strings.Repeat(" ", numbers)
	if r.width > 0 {
		r.room = max(r.width-numbers-2, 1)
	}
}

// A page collects the rows of the hunks, up to a limit.
type page struct {
	rows  []string
	limit int      // the most rows of hunks, or zero for no limit
	used  int      // rows of hunks written
	held  []string // rows that wait for the line after them
	cut   bool     // the one line written did not fit, and is cut short
}

// hold keeps rows back until the next line is written, so that a row that
// only introduces a hunk is never the last thing shown.
func (p *page) hold(rows []string) {
	p.held = rows
}

// left is how many more rows the page takes, or zero for no limit.
func (p *page) left() int {
	if p.limit <= 0 {
		return 0
	}
	return max(p.limit-p.used, 1)
}

// add writes the rows of one line, after any rows held back for it. It
// reports false when they do not fit; then nothing is written, unless the
// page is still empty: of a first line too long for the whole page, the rows
// that fit are shown.
func (p *page) add(rows []string) bool {
	need := len(p.held) + len(rows)
	if p.limit > 0 && p.used+need > p.limit {
		if p.used == 0 {
			p.rows = append(p.rows, rows[:p.limit]...)
			p.used = p.limit
			p.cut = true
		}
		return false
	}
	p.rows = append(p.rows, p.held...)
	p.rows = append(p.rows, rows...)
	p.used += need
	p.held = nil
	return true
}

// body writes the hunks after the rows in out, as many as the limit allows,
// and says what it left out.
func (r *renderer) body(out []string, c *comparison, hunks []hunkRuns, limit int) []string {
	total := 0 // changed lines in all hunks
	for _, h := range hunks {
		total += changedIn(h)
	}
	p := page{rows: out, limit: max(limit, 0)}
	shown := 0 // changed lines written
	for hi, h := range hunks {
		if hi > 0 {
			p.hold(r.noteRows(plural(h.a0-hunks[hi-1].a1, "unchanged line")))
		}
		wrote, ok := r.hunk(&p, c, h)
		shown += wrote
		if ok {
			continue
		}
		// Unchanged lines that did not fit after the last change are left
		// out without a word: what is told is what changed.
		if left := total - shown; left > 0 {
			open := len(hunks) - hi - 1
			if changedIn(h) > wrote {
				open++
			}
			msg := plural(left, "more changed line") + " in " + plural(open, "hunk") + " not shown"
			if p.cut {
				msg = "the line above is cut short; " + msg
			}
			p.rows = append(p.rows, r.noteRows(msg)...)
		}
		break
	}
	return p.rows
}

// changedIn counts the deleted and inserted lines of a hunk.
func changedIn(h hunkRuns) int {
	n := 0
	for _, part := range h.runs {
		if part.op != Keep {
			n += part.a1 - part.a0 + part.b1 - part.b0
		}
	}
	return n
}

// hunk writes the lines of one hunk to the page. It returns how many changed
// lines it wrote, and false when the page was full before the hunk was done.
func (r *renderer) hunk(p *page, c *comparison, h hunkRuns) (wrote int, ok bool) {
	for i := 0; i < len(h.runs); i++ {
		part := h.runs[i]
		if part.op == Keep {
			for k := range part.a1 - part.a0 {
				if !p.add(r.lineRows(Keep, part.a0+k+1, part.b0+k+1, c.new[part.b0+k], lineMarks{}, p.left())) {
					return wrote, false
				}
			}
			continue
		}
		// A place where lines change.
		var dels, inss run
		if part.op == Delete {
			dels = part
			if i+1 < len(h.runs) && h.runs[i+1].op == Insert {
				i++
				inss = h.runs[i]
			}
		} else {
			inss = part
		}
		partner, delMarks, insMarks := r.marks(c, dels, inss, p.left())
		nd, ni := dels.a1-dels.a0, inss.b1-inss.b0
		d, n := 0, 0 // the next deleted and the next inserted line to write
		write := func(toDel, toIns int) bool {
			for ; d < toDel; d++ {
				var mk lineMarks
				if d < len(delMarks) {
					mk = delMarks[d]
				}
				if !p.add(r.lineRows(Delete, dels.a0+d+1, 0, c.old[dels.a0+d], mk, p.left())) {
					return false
				}
				wrote++
			}
			for ; n < toIns; n++ {
				var mk lineMarks
				if n < len(insMarks) {
					mk = insMarks[n]
				}
				if !p.add(r.lineRows(Insert, 0, inss.b0+n+1, c.new[inss.b0+n], mk, p.left())) {
					return false
				}
				wrote++
			}
			return true
		}
		// The lines are written a stretch at a time, deleted before
		// inserted in each: first the lines before the next pair, which
		// have no partners, then the pairs that follow one another without
		// a gap. So a deleted line stands right above the lines it turned
		// into, and not above a whole function that was added before it.
		for d < nd || n < ni {
			pair := d
			for pair < len(partner) && partner[pair] < 0 {
				pair++
			}
			if pair == len(partner) {
				if !write(nd, ni) {
					return wrote, false
				}
				break
			}
			if !write(pair, partner[pair]) {
				return wrote, false
			}
			end := pair
			for end < len(partner) && partner[end] == partner[pair]+end-pair {
				end++
			}
			if !write(end, partner[pair]+end-pair) {
				return wrote, false
			}
		}
	}
	return wrote, true
}

// lineMarks is what is marked inside one changed line.
type lineMarks struct {
	spans  []Span // the stretches that differ from the line's partner
	reveal bool   // the blanks in them are drawn, because the two lines would look the same
}

// marks pairs the lines of one place where dels is replaced by inss, and
// works out what to mark inside each pair. partner gives, for each deleted
// line, the index of the inserted line it turned into, or -1. Only the first
// most lines of each side are looked at, when most is not zero: no more than
// that can be shown.
func (r *renderer) marks(c *comparison, dels, inss run, most int) (partner []int, delMarks, insMarks []lineMarks) {
	n, m := dels.a1-dels.a0, inss.b1-inss.b0
	if most > 0 {
		n, m = min(n, most), min(m, most)
	}
	if n == 0 || m == 0 {
		return nil, nil, nil
	}
	old, new := make([]string, n), make([]string, m)
	for i := range old {
		old[i], _ = chomp(c.old[dels.a0+i])
	}
	for j := range new {
		new[j], _ = chomp(c.new[inss.b0+j])
	}
	partner = pairLines(old, new)
	delMarks, insMarks = make([]lineMarks, n), make([]lineMarks, m)
	for i, j := range partner {
		if j < 0 || old[i] == new[j] {
			continue
		}
		gone, come, ok := Spans(old[i], new[j])
		if !ok {
			continue
		}
		reveal := r.look(old[i]) == r.look(new[j])
		delMarks[i] = lineMarks{gone, reveal}
		insMarks[j] = lineMarks{come, reveal}
	}
	return partner, delMarks, insMarks
}

// look returns a line as it would look on the screen, as far as blanks go:
// tabs as the spaces they take, every other kind of blank as a space, and
// nothing for the blanks at the end. Two different lines with the same look
// differ in a way a person could not see.
func (r *renderer) look(s string) string {
	s = r.m.ExpandTabs(s, r.tab, 0)
	if strings.IndexFunc(s, func(c rune) bool { return c >= utf8.RuneSelf && unicode.Is(unicode.Zs, c) }) >= 0 {
		s = strings.Map(func(c rune) rune {
			if unicode.Is(unicode.Zs, c) {
				return ' '
			}
			return c
		}, s)
	}
	return strings.TrimRight(s, " ")
}

// lineRows draws one line of a text: the rows it takes, each with its
// gutter. op says whether the line is kept, deleted or inserted; oldNo and
// newNo are its numbers in the two texts, zero for the text it is not in.
// left is how many rows there is still room for, or zero for no limit: of a
// line that takes more, only left+1 rows are drawn, enough for the caller to
// see that it does not fit.
func (r *renderer) lineRows(op Op, oldNo, newNo int, line string, mk lineMarks, left int) []string {
	body, ending := chomp(line)
	base, span, sign := r.theme.Context, r.theme.Context, " "
	trail := len(body)
	switch op {
	case Delete:
		base, span, sign = r.theme.Deleted, r.theme.DeletedSpan, "-"
		trail = len(strings.TrimRight(body, " \t"))
	case Insert:
		base, span, sign = r.theme.Inserted, r.theme.InsertedSpan, "+"
		trail = len(strings.TrimRight(body, " \t"))
	}
	most := 0
	if left > 0 && r.room > 0 {
		most = (left + 1) * r.room
	}
	cells := r.cells(r.scratch[:0], body, mk.spans, mk.reveal, trail, most)
	if r.showCR && ending == "\r\n" {
		cells = append(cells, cell{text: markCR, width: r.m.Width(markCR), symbol: true})
	}
	r.scratch = cells

	baseSeq, spanSeq := base.Sequence(), span.Sequence()
	symbolSeq, numberSeq := r.theme.Symbol.Sequence(), r.theme.Number.Sequence()
	var rows []string
	var w styled
	for start := 0; ; {
		end := r.rowEnd(cells, start)
		if start == 0 {
			r.writeNumbers(&w, numberSeq, oldNo, newNo)
			w.put(baseSeq, sign)
		} else {
			w.put("", r.blank)
			w.put(numberSeq, markWrap)
		}
		if start == end {
			// A line with nothing on it: the gutter is the row.
			rows = append(rows, strings.TrimRight(w.end(), " "))
			break
		}
		w.put(baseSeq, " ")
		for _, c := range cells[start:end] {
			switch {
			case c.marked:
				w.put(spanSeq, c.text)
			case c.symbol:
				w.put(symbolSeq, c.text)
			default:
				w.put(baseSeq, c.text)
			}
		}
		rows = append(rows, w.end())
		if end == len(cells) {
			break
		}
		start = end
	}
	return rows
}

// A styled builds one row out of pieces that each have a style, given as the
// escape sequence that turns the style on, or "" for none. Neighbouring
// pieces in the same style share one sequence, and the row ends with every
// style off.
type styled struct {
	b   strings.Builder
	seq string // the sequence in effect
}

// put writes s in the style that seq turns on.
func (w *styled) put(seq, s string) {
	if s == "" {
		return
	}
	if seq != w.seq {
		if w.seq != "" {
			w.b.WriteString(text.Reset)
		}
		w.b.WriteString(seq)
		w.seq = seq
	}
	w.b.WriteString(s)
}

// end returns the row and makes the writer ready for the next one.
func (w *styled) end() string {
	if w.seq != "" {
		w.b.WriteString(text.Reset)
		w.seq = ""
	}
	row := w.b.String()
	w.b.Reset()
	return row
}

// writeNumbers writes the number columns of a line's first row: the line's
// number in the old text and its number in the new, each right-aligned, or
// the one of the two there is when there is one column. Zero stands for no
// number. The numbers are styled, and the blanks before and after them are
// not.
func (r *renderer) writeNumbers(w *styled, seq string, oldNo, newNo int) {
	if r.digits == 0 {
		return
	}
	var nums strings.Builder
	column := func(n int) {
		s := ""
		if n > 0 {
			s = strconv.Itoa(n)
		}
		nums.WriteString(strings.Repeat(" ", max(r.digits-len(s), 0)))
		nums.WriteString(s)
		nums.WriteByte(' ')
	}
	if r.single {
		column(max(oldNo, newNo))
	} else {
		column(oldNo)
		column(newNo)
	}
	all := nums.String()
	shown := strings.TrimRight(all, " ")
	lead := len(shown) - len(strings.TrimLeft(shown, " "))
	w.put("", shown[:lead])
	w.put(seq, shown[lead:])
	w.put("", all[len(shown):])
}

// noteRows draws a note that stands among the lines, such as the count of
// lines between two hunks: the skip mark in the gutter and the words where a
// line's text would be, wrapped to the room there is.
func (r *renderer) noteRows(msg string) []string {
	lines := []string{msg}
	if r.room > 0 {
		lines = r.m.Wrap(msg, r.room, text.WrapOptions{})
	}
	rows := make([]string, len(lines))
	for i, line := range lines {
		mark := " "
		if i == 0 {
			mark = r.theme.Number.Render(markSkip)
		}
		rows[i] = r.blank + mark + " " + r.theme.Note.Render(line)
	}
	return rows
}

// display returns the script as it is shown: Keep, Delete and Insert runs,
// with a Delete before an Insert wherever lines are replaced. Lines that
// differ from their partner only by a newline at the end of the file are
// shown as unchanged, since a note tells of that. So are lines that differ
// only between LF and CRLF when fold is set, which the caller does when the
// whole text went from the one to the other.
func (c *comparison) display(fold bool) []run {
	fine := c.fine()
	out := make([]run, 0, len(fine))
	open := false
	var place run // the lines replaced at the place being collected
	flush := func() {
		if !open {
			return
		}
		if place.a1 > place.a0 {
			out = append(out, run{Delete, place.a0, place.a1, place.b0, place.b0})
		}
		if place.b1 > place.b0 {
			out = append(out, run{Insert, place.a1, place.a1, place.b0, place.b1})
		}
		open = false
	}
	for _, r := range fine {
		if r.op == Keep || r.op == newlinePair || r.op == endingPair && fold {
			flush()
			if k := len(out) - 1; k >= 0 && out[k].op == Keep {
				out[k].a1, out[k].b1 = r.a1, r.b1
			} else {
				out = append(out, run{Keep, r.a0, r.a1, r.b0, r.b1})
			}
			continue
		}
		if !open {
			open = true
			place = run{a0: r.a0, a1: r.a0, b0: r.b0, b1: r.b0}
		}
		place.a1, place.b1 = r.a1, r.b1
	}
	flush()
	return out
}

// isText reports whether s can be shown as lines of text: it is valid UTF-8
// and holds no NUL byte.
func isText(s string) bool {
	return utf8.ValidString(s) && strings.IndexByte(s, 0) < 0
}

// endingName names a kind of line ending.
func endingName(kind int) string {
	if kind == endCRLF {
		return "CRLF"
	}
	return "LF"
}

// plural writes a count and what is counted, as "1 line" or "3 lines".
func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return grouped(n) + " " + what + "s"
}

// grouped writes a number with commas between its thousands.
func grouped(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 || n < 0 {
		return s
	}
	var b strings.Builder
	lead := len(s) % 3
	b.WriteString(s[:lead])
	for i := lead; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

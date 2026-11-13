package diff

import "strconv"

// Stats counts what a change did, for a status line or a tool's result.
// The zero value means the two texts are the same.
type Stats struct {
	// Added is the number of lines the text after has that the text
	// before did not, and Removed the number of lines of the text before
	// that are gone. A line whose words are the same and whose ending
	// alone changed is counted in neither: it is counted in Endings or
	// told by FinalNewline. In the unified form such a line still shows as
	// a line removed and a line added, as that format requires.
	Added, Removed int
	// Endings is the number of lines that kept their text and changed
	// their line ending between LF and CRLF.
	Endings int
	// FinalNewline says what the change did to the newline that ends the
	// text's last line: +1 when the text before did not end with a
	// newline and the text after does, -1 when the text before did and
	// the text after does not, and 0 when the change left that alone or
	// when one of the texts is empty.
	FinalNewline int
}

// String puts the counts in a few words: "+3 -1", with ", line endings
// changed" or ", final newline added" or ", final newline removed" after it
// where that happened, or "no changes".
func (s Stats) String() string {
	if s == (Stats{}) {
		return "no changes"
	}
	out := "+" + strconv.Itoa(s.Added) + " -" + strconv.Itoa(s.Removed)
	if s.Endings > 0 {
		out += ", line endings changed"
	}
	switch {
	case s.FinalNewline > 0:
		out += ", final newline added"
	case s.FinalNewline < 0:
		out += ", final newline removed"
	}
	return out
}

// fine returns the script with one more thing worked out: inside each place
// where lines are replaced, the deleted lines and inserted lines that have
// the same text and differ only in their ending are paired, as endingPair
// and newlinePair runs. The rest of each such place stays Delete and Insert
// runs. The lines a pair covers are the same lines the plain script deletes
// and inserts there, so both describe the same change.
func (c *comparison) fine() []run {
	if c.fineRuns != nil || len(c.runs) == 0 {
		return c.fineRuns
	}
	if !c.endings {
		c.fineRuns = c.runs
		return c.fineRuns
	}
	out := make([]run, 0, len(c.runs))
	for r := 0; r < len(c.runs); r++ {
		cur := c.runs[r]
		if cur.op != Delete || r+1 == len(c.runs) || c.runs[r+1].op != Insert {
			out = append(out, cur)
			continue
		}
		r++
		out = c.pairUp(out, cur.a0, cur.a1, c.runs[r].b0, c.runs[r].b1)
	}
	c.fineRuns = out
	return out
}

// pairUp appends to out the runs for one place where lines a0 to a1 of the
// text before are replaced by lines b0 to b1 of the text after. It compares
// the two groups of lines once more, by their text alone, and turns each
// kept pair into a pair run.
func (c *comparison) pairUp(out []run, a0, a1, b0, b1 int) []run {
	na, nb, ids := number(c.old[a0:a1], c.new[b0:b1], true)
	del, ins, work, _ := script(na, nb, ids, c.left)
	c.spend(work)

	n, m := a1-a0, b1-b0
	pi, pj := 0, 0 // where the lines not yet written out begin
	flush := func(i, j int) {
		if i > pi {
			out = append(out, run{Delete, a0 + pi, a0 + i, b0 + pj, b0 + pj})
		}
		if j > pj {
			out = append(out, run{Insert, a0 + i, a0 + i, b0 + pj, b0 + j})
		}
		pi, pj = i, j
	}
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && del[i]:
			i++
		case j < m && ins[j]:
			j++
		case i < n && j < m:
			before, after := c.old[a0+i], c.new[b0+j]
			textBefore, _ := chomp(before)
			textAfter, _ := chomp(after)
			if before == after || textBefore != textAfter {
				// The very same line on both sides, which the rough
				// comparison can leave inside a replaced place: it stays
				// deleted and inserted, as the plain script has it. Or
				// two lines that do not say the same thing after all,
				// which is not expected: a pair is shown as one unchanged
				// line, and must never be called one unless it is.
				i++
				j++
				continue
			}
			op := endingPair
			if !ended(before) || !ended(after) {
				op = newlinePair
			}
			flush(i, j)
			if k := len(out) - 1; k >= 0 && out[k].op == op && out[k].a1 == a0+i && out[k].b1 == b0+j {
				out[k].a1++
				out[k].b1++
			} else {
				out = append(out, run{op, a0 + i, a0 + i + 1, b0 + j, b0 + j + 1})
			}
			i++
			j++
			pi, pj = i, j
		default:
			// The flags do not pair up, which script does not do. What
			// is left stays deleted and inserted.
			i, j = n, m
		}
	}
	flush(n, m)
	return out
}

// stats counts what the change did.
func (c *comparison) stats() Stats {
	var s Stats
	for _, r := range c.fine() {
		switch r.op {
		case Delete:
			s.Removed += r.a1 - r.a0
		case Insert:
			s.Added += r.b1 - r.b0
		case endingPair:
			s.Endings += r.a1 - r.a0
		}
	}
	if n, m := len(c.old), len(c.new); n > 0 && m > 0 {
		before, after := ended(c.old[n-1]), ended(c.new[m-1])
		switch {
		case !before && after:
			s.FinalNewline = 1
		case before && !after:
			s.FinalNewline = -1
		}
	}
	return s
}

// The line endings a text can be written with.
const (
	endNone  = iota // no line of the text is ended
	endLF           // every ended line ends in LF alone
	endCRLF         // every ended line ends in CR LF
	endMixed        // some of each
)

// endingsOf says which line endings a text's lines have.
func endingsOf(lines []string) int {
	lf, crlf := false, false
	for _, line := range lines {
		switch _, ending := chomp(line); ending {
		case "\n":
			lf = true
		case "\r\n":
			crlf = true
		}
		if lf && crlf {
			return endMixed
		}
	}
	switch {
	case lf:
		return endLF
	case crlf:
		return endCRLF
	}
	return endNone
}

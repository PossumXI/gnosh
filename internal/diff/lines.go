package diff

import "strings"

// splitLines cuts a text into its lines. Each line keeps its line ending, so
// the lines joined give the text back, and only the last can be without one.
// The empty text has no lines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	n := strings.Count(s, "\n")
	if s[len(s)-1] != '\n' {
		n++
	}
	lines := make([]string, 0, n)
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			lines = append(lines, s)
			break
		}
		lines = append(lines, s[:i+1])
		s = s[i+1:]
	}
	return lines
}

// chomp splits a line into its text and its line ending. The ending is "\n",
// "\r\n", or nothing for a last line that was not ended.
func chomp(line string) (text, ending string) {
	n := len(line)
	if n == 0 || line[n-1] != '\n' {
		return line, ""
	}
	if n >= 2 && line[n-2] == '\r' {
		return line[:n-2], line[n-2:]
	}
	return line[:n-1], line[n-1:]
}

// ended reports whether a line ends with a newline. Only the last line of a
// text can fail to.
func ended(line string) bool {
	return line != "" && line[len(line)-1] == '\n'
}

// number gives every distinct line of a and b a number of its own, counting
// from zero, so that two lines are equal exactly when their numbers are. It
// returns the lines as those numbers and how many numbers it gave out. With
// textOnly, lines are told apart by their text alone, without their endings.
func number(a, b []string, textOnly bool) (na, nb []int32, count int) {
	seen := make(map[string]int32, len(a)+len(b))
	one := func(lines []string) []int32 {
		out := make([]int32, len(lines))
		for i, line := range lines {
			if textOnly {
				line, _ = chomp(line)
			}
			id, ok := seen[line]
			if !ok {
				id = int32(len(seen))
				seen[line] = id
			}
			out[i] = id
		}
		return out
	}
	na = one(a)
	nb = one(b)
	return na, nb, len(seen)
}

// A run is a stretch of an edit script: lines a0 up to a1 of the text before
// and lines b0 up to b1 of the text after, counted from zero. A Keep covers
// as many lines on one side as on the other; a Delete covers none of the
// text after and an Insert none of the text before, and each says where in
// the other text it falls.
type run struct {
	op     Op
	a0, a1 int
	b0, b1 int
}

// A comparison is two texts cut into lines and the script that turns the one
// into the other.
type comparison struct {
	old, new []string // the lines of each text, with their endings
	runs     []run    // the script: Keep, Delete and Insert runs
	minimal  bool     // the script is known to be the smallest
	work     int      // line comparisons made so far
	left     int      // what is left of the budget, or -1 for no limit

	// endings says that some line may differ from another by its ending
	// alone: a text has a CRLF in it, or does not end with a newline.
	endings bool
	// fineRuns is the script with such lines paired up, once worked out.
	fineRuns []run
}

// compare works out the script that turns before into after. budget is the
// most work the search for the smallest script may do, or -1.
func compare(before, after string, budget int) *comparison {
	c := &comparison{
		old:     splitLines(before),
		new:     splitLines(after),
		minimal: true,
		left:    budget,
		endings: strings.Contains(before, "\r\n") || strings.Contains(after, "\r\n") ||
			before != "" && !ended(before) || after != "" && !ended(after),
	}
	n, m := len(c.old), len(c.new)
	if before == after {
		if n > 0 {
			c.runs = []run{{Keep, 0, n, 0, n}}
		}
		return c
	}

	// The lines the two texts begin and end with in common need no search.
	head := 0
	for head < n && head < m && c.old[head] == c.new[head] {
		head++
	}
	tail := 0
	for tail < n-head && tail < m-head && c.old[n-1-tail] == c.new[m-1-tail] {
		tail++
	}

	del, ins := make([]bool, n), make([]bool, m)
	a, b, ids := number(c.old[head:n-tail], c.new[head:m-tail], false)
	d, i, work, minimal := script(a, b, ids, budget)
	copy(del[head:], d)
	copy(ins[head:], i)
	c.spend(work)
	c.minimal = minimal

	// The script is what a person approves a change by, so it is checked
	// against both texts before it is handed out. None of what follows is
	// expected to happen. If the two steps that make the script easier to
	// read have spoilt it, the script as the search found it is used; and
	// if that is not sound either, deleting everything and inserting
	// everything always is.
	found := runsOf(del, ins)
	realign(c.old, c.new, del, ins)
	slide(c.old, c.new, del, ins)
	c.runs = runsOf(del, ins)
	if !sound(c.runs, c.old, c.new) {
		c.runs = found
	}
	if !sound(c.runs, c.old, c.new) {
		c.runs = c.runs[:0]
		if n > 0 {
			c.runs = append(c.runs, run{Delete, 0, n, 0, 0})
		}
		if m > 0 {
			c.runs = append(c.runs, run{Insert, n, n, 0, m})
		}
		c.minimal = false
	}
	return c
}

// spend takes work off what is left of the budget.
func (c *comparison) spend(work int) {
	c.work += work
	if c.left > 0 {
		c.left = max(c.left-work, 0)
	}
}

// runsOf turns the two lists of flags, which say which lines are deleted and
// which inserted, into a script: the lines not flagged pair up in order as
// kept lines, and where lines are both deleted and inserted at one place the
// Delete comes first.
func runsOf(del, ins []bool) []run {
	var runs []run
	n, m := len(del), len(ins)
	i, j := 0, 0
	for i < n || j < m {
		si, sj := i, j
		for i < n && j < m && !del[i] && !ins[j] {
			i++
			j++
		}
		if i > si {
			runs = append(runs, run{Keep, si, i, sj, j})
		}
		di := i
		for i < n && del[i] {
			i++
		}
		if i > di {
			runs = append(runs, run{Delete, di, i, j, j})
		}
		ij := j
		for j < m && ins[j] {
			j++
		}
		if j > ij {
			runs = append(runs, run{Insert, i, i, ij, j})
		}
		if i == si && j == sj {
			// One text has run out while the other still has lines that
			// are not flagged: the flags do not pair up. Taking the rest
			// as changed keeps the script a script; sound has the last
			// word on whether it is the right one.
			if i < n {
				runs = append(runs, run{Delete, i, n, j, j})
			}
			if j < m {
				runs = append(runs, run{Insert, n, n, j, m})
			}
			break
		}
	}
	return runs
}

// sound reports whether runs is a script that turns old into new: the runs
// cover both texts in order without a gap, and every kept line is the same
// in both.
func sound(runs []run, old, new []string) bool {
	i, j := 0, 0
	for _, r := range runs {
		if r.a0 != i || r.b0 != j || r.a1 < r.a0 || r.b1 < r.b0 || r.a1 > len(old) || r.b1 > len(new) {
			return false
		}
		switch r.op {
		case Keep:
			if r.a1-r.a0 != r.b1-r.b0 {
				return false
			}
			for k := range r.a1 - r.a0 {
				if old[r.a0+k] != new[r.b0+k] {
					return false
				}
			}
		case Delete:
			if r.b1 != r.b0 {
				return false
			}
		case Insert:
			if r.a1 != r.a0 {
				return false
			}
		default:
			return false
		}
		i, j = r.a1, r.b1
	}
	return i == len(old) && j == len(new)
}

// edits returns the script in its public form.
func (c *comparison) edits() []Edit {
	if len(c.runs) == 0 {
		return nil
	}
	edits := make([]Edit, len(c.runs))
	for i, r := range c.runs {
		// Each run's lines are a slice that cannot grow into the next
		// run's, so a caller who appends to one does not write over
		// another.
		if r.op == Insert {
			edits[i] = Edit{Op: Insert, Lines: c.new[r.b0:r.b1:r.b1]}
		} else {
			edits[i] = Edit{Op: r.op, Lines: c.old[r.a0:r.a1:r.a1]}
		}
	}
	return edits
}

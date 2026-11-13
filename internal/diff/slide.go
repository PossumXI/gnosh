package diff

// This file makes a script easier to read without changing what it does.
//
// A run of lines that is only inserted (or only deleted) can often stand in
// more than one place. Insert a function between two others and the new
// lines may be counted from its first line to its closing brace, or from the
// closing brace of the function above to the last line of the new one's
// body: the two scripts give the same text and are the same size, but only
// the first shows a person what was added. Such a run can be slid up while
// the line above it equals its last line, and down while the line below it
// equals its first.
//
// Every place a run can slide to is given a score, and the run goes to the
// place with the best one. The idea, a heuristic that looks at blank lines
// and indentation around the two edges of the run, is the one git documents
// for its --indent-heuristic option; the scores here are gnosh's own.

// What the edges of a run cost. Lower is better.
const (
	// slideBlank is taken off for an edge that lies beside an empty line.
	slideBlank = 10
	// slideBroken is added for a run that begins with the line closing
	// what stands above it, or ends with the line opening what stands
	// below it: an edge that cuts a block from its brace.
	slideBroken = 30
	// slideDeepest is the most that indentation counts for, in columns.
	slideDeepest = 60
	// slideTab is what a tab counts for when indentation is measured.
	slideTab = 4
	// slideLook is how many lines below an empty line are looked at to
	// find the indentation the empty line stands at.
	slideLook = 20
)

// slide moves every run of only-deleted or only-inserted lines that has a
// choice of places to the best of them. del and ins are the flags of a
// script for old and new, and are changed in place; the script they stand
// for stays a script of the same size for the same texts.
//
// A run that replaces lines (deleted and inserted at one place) is left
// alone: in a smallest script it has no choice. A run never slides into its
// neighbour: at least one kept line stays between two changes.
func slide(old, new []string, del, ins []bool) {
	runs := runsOf(del, ins)
	for r := range runs {
		switch runs[r].op {
		case Insert:
			if r > 0 && runs[r-1].op == Delete {
				continue
			}
			slideRun(runs, r, new, ins, true)
		case Delete:
			if r+1 < len(runs) && runs[r+1].op == Insert {
				continue
			}
			slideRun(runs, r, old, del, false)
		}
	}
}

// slideRun slides run r, which stands alone between kept lines. lines and
// flags are those of the text the run's lines belong to: the text after for
// an Insert, the text before for a Delete. It keeps runs in step with the
// flags, so that the runs after r see where r ended up.
func slideRun(runs []run, r int, lines []string, flags []bool, insert bool) {
	g := &runs[r]
	g0, g1 := g.a0, g.a1
	if insert {
		g0, g1 = g.b0, g.b1
	}

	// How far the run can go: up while the line above it is the same as
	// its last line, down while the line below it is the same as its
	// first, and in both directions only through the kept lines beside it.
	up, down := 0, 0
	if r > 0 {
		room := runs[r-1].a1 - runs[r-1].a0
		if r > 1 {
			room--
		}
		for up < room && lines[g0-1-up] == lines[g1-1-up] {
			up++
		}
	}
	if r+1 < len(runs) {
		room := runs[r+1].a1 - runs[r+1].a0
		if r+2 < len(runs) {
			room--
		}
		for down < room && lines[g0+down] == lines[g1+down] {
			down++
		}
	}
	if up == 0 && down == 0 {
		return
	}

	// Of equally good places the lowest wins, which is where a run of
	// repeated lines is by custom shown as changed: at its end.
	best, bestScore := -up, slideScore(lines, g0-up, g1-up)
	for s := -up + 1; s <= down; s++ {
		if score := slideScore(lines, g0+s, g1+s); score <= bestScore {
			best, bestScore = s, score
		}
	}
	if best == 0 {
		return
	}

	for k := 0; k < best; k++ {
		flags[g0+k] = false
		flags[g1+k] = true
	}
	for k := 0; k < -best; k++ {
		flags[g1-1-k] = false
		flags[g0-1-k] = true
	}
	g.a0 += best
	g.a1 += best
	g.b0 += best
	g.b1 += best
	if r > 0 {
		runs[r-1].a1 += best
		runs[r-1].b1 += best
	}
	if r+1 < len(runs) {
		runs[r+1].a0 += best
		runs[r+1].b0 += best
	}
}

// slideScore says how well a run of changed lines reads when it is lines g0
// up to g1 of its text. Lower is better.
//
// Each of the run's two edges is judged by the line that follows the edge:
// the run's first line for the upper edge, the line below the run for the
// lower one. The less that line is indented the better, since that is where
// a new function, block or paragraph starts. An edge beside an empty line is
// better still. An edge is bad when it parts a block from its brace: when
// the run starts with a line indented less than the line above it (a closing
// brace whose block stays behind), or ends with a line indented less than
// the line below it (an opening line whose block stays behind). The start
// and the end of the text are perfect edges.
func slideScore(lines []string, g0, g1 int) int {
	score := 0
	if g0 > 0 {
		above, aboveBlank := indentOf(lines[g0-1])
		first, firstBlank := indentOf(lines[g0])
		if firstBlank {
			first = indentBelow(lines, g0)
			// Of two places that both have an empty line at an edge, the
			// one where the run starts with its own first words and ends
			// with the empty line reads as one whole thing.
			score++
		}
		score += min(first, slideDeepest)
		switch {
		case aboveBlank || firstBlank:
			score -= slideBlank
		case above > first:
			score += slideBroken
		}
	}
	if g1 < len(lines) {
		last, lastBlank := indentOf(lines[g1-1])
		below, belowBlank := indentOf(lines[g1])
		if belowBlank {
			below = indentBelow(lines, g1)
		}
		score += min(below, slideDeepest)
		switch {
		case lastBlank || belowBlank:
			score -= slideBlank
		case below > last:
			score += slideBroken
		}
	}
	return score
}

// indentOf measures a line's indentation: the columns its leading spaces and
// tabs take. blank says that the line has nothing else on it.
func indentOf(line string) (indent int, blank bool) {
	text, _ := chomp(line)
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case ' ':
			indent++
		case '\t':
			indent += slideTab - indent%slideTab
		default:
			return indent, false
		}
	}
	return 0, true
}

// indentBelow is the indentation an empty line stands at: that of the first
// line below line i that is not empty, or zero when the text ends, or goes
// on empty for too long, before one comes.
func indentBelow(lines []string, i int) int {
	for j := i + 1; j < len(lines) && j <= i+slideLook; j++ {
		if indent, blank := indentOf(lines[j]); !blank {
			return indent
		}
	}
	return 0
}

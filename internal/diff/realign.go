package diff

// This file chooses between scripts of the same size.
//
// The smallest script is seldom the only one. Change a function and add
// another after it, and the empty line and the closing brace that the old
// text has once and the new text has twice can be kept in more than one way.
// Kept one way, the script says that the function was changed and a new one
// was added. Kept another way, it says that the old function was replaced by
// the new one and a third was put before it: just as small, just as correct,
// and not what happened. The search for the smallest script has no reason to
// prefer either.
//
// So where changes lie close together, with only a few kept lines between
// them, those lines are aligned once more, this time by a rule that knows
// more about a pair of lines than whether they are equal. Keeping as many
// equal lines as possible still comes first, so the script stays exactly as
// small as it was. Among the ways of doing that, the one is taken that
// accounts for the most text: a kept line counts for the words on it, so
// that a line with something to say is kept sooner than a brace or an empty
// line, and two lines that replace each other count for the words they
// share, so that a changed line is set against its own old form.

const (
	// realignGap is the most kept lines there may be between two changes
	// for the two to be looked at as one place.
	realignGap = 3
	// realignTable is the most pairs of lines, old times new, of a place
	// that is aligned again.
	realignTable = 1 << 16
	// realignEqual is what a pair of equal lines counts for before its
	// words are counted: more than the words of all the pairs of one place
	// can add up to, so that nothing outweighs one more line kept.
	realignEqual = 1 << 20
	// realignWords is the most that the words of one pair count for.
	realignWords = 2000
	// realignSteps bounds the work of all the places of one comparison
	// together: bytes read and words compared. A comparison with more
	// places than that pays for has its first ones aligned again and the
	// rest left as the search found them.
	realignSteps = 1 << 21
)

// realign goes through a script, given as its flags, and aligns again every
// place where lines are both deleted and inserted: the changes that lie
// close together, the kept lines between them, and a few kept lines on
// either side, which may be the very ones that could have been kept
// elsewhere. The flags are changed in place and still describe a script for
// old and new, of the same size as before.
func realign(old, new []string, del, ins []bool) {
	runs := runsOf(del, ins)
	steps := realignSteps
	var scratch []int32
	// taken is how many lines of the kept run before the next place
	// already belong to the place before it: two places never share a line.
	taken := 0
	for i := 0; i < len(runs); {
		if runs[i].op == Keep {
			i++
			continue
		}
		// A place runs from this change through every change that follows
		// it with no more than realignGap kept lines in between.
		j := i
		dels, inss := false, false
		for j < len(runs) {
			r := runs[j]
			if r.op == Keep && (j+1 == len(runs) || r.a1-r.a0 > realignGap) {
				break
			}
			dels = dels || r.op == Delete
			inss = inss || r.op == Insert
			j++
		}
		lead, trail := 0, 0
		if i > 0 {
			lead = min(realignGap, runs[i-1].a1-runs[i-1].a0-taken)
		}
		if j < len(runs) {
			trail = min(realignGap, runs[j].a1-runs[j].a0)
		}
		taken = 0
		// Lines that are only deleted, or only inserted, have no old or
		// new form to be set against: slide looks after those.
		if dels && inss {
			first, last := runs[i], runs[j-1]
			scratch = realignPlace(old, new, del, ins, first.a0-lead, last.a1+trail, first.b0-lead, last.b1+trail, &steps, scratch)
			taken = trail
		}
		i = j
	}
}

// realignPlace aligns lines a0 up to a1 of old with lines b0 up to b1 of new
// again and sets their flags. It leaves the place as it is when that would
// be too much work, taking what it does do off steps. scratch is room to
// work in, which it returns for the next call.
func realignPlace(old, new []string, del, ins []bool, a0, a1, b0, b1 int, steps *int, scratch []int32) []int32 {
	n, m := a1-a0, b1-b0
	if n == 0 || m == 0 || n*m > realignTable || n*m > *steps {
		return scratch
	}
	// Another script of the same size has to keep a line that this one
	// deletes or inserts. If no deleted line of the place equals any of
	// its new lines, and no inserted line any of its old ones, there is
	// no such script and nothing to choose.
	*steps -= n * m
	if !choice(old[a0:a1], new[b0:b1], del[a0:a1], ins[b0:b1]) {
		return scratch
	}

	bagsA, bagsB := make([]bag, n), make([]bag, m)
	wordsA, wordsB, bytes := 0, 0, 0
	for i := range bagsA {
		text, _ := chomp(old[a0+i])
		bagsA[i] = bagOf(text)
		wordsA += len(bagsA[i].words)
		bytes += len(text)
	}
	for j := range bagsB {
		text, _ := chomp(new[b0+j])
		bagsB[j] = bagOf(text)
		wordsB += len(bagsB[j].words)
		bytes += len(text)
	}
	*steps -= bytes
	// Every line of one side is compared with every line of the other.
	cost := m*wordsA + n*wordsB + n*m
	if cost > *steps {
		return scratch
	}
	*steps -= cost

	// worth is what pairing line i of the place's old lines with line j of
	// its new ones counts for: realignEqual and the words on the line when
	// the two are the same line, the words they share when they are alike
	// enough to be two forms of one line, and -1 when they are not, which
	// means they are not paired.
	worth := func(i, j int) int32 {
		if old[a0+i] == new[b0+j] {
			return realignEqual + int32(min(bagsA[i].total+1, realignWords))
		}
		if likeness(bagsA[i], bagsB[j]) >= pairAlike {
			return int32(min(sharedBytes(bagsA[i], bagsB[j]), realignWords))
		}
		return -1
	}

	// best[i*(m+1)+j] is the most that lines i and up can count for
	// against lines j and up.
	size := (n + 1) * (m + 1)
	if cap(scratch) < size {
		scratch = make([]int32, size)
	}
	best := scratch[:size]
	clear(best)
	at := func(i, j int) int { return i*(m+1) + j }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			v := max(best[at(i+1, j)], best[at(i, j+1)])
			if w := worth(i, j); w >= 0 {
				v = max(v, w+best[at(i+1, j+1)])
			}
			best[at(i, j)] = v
		}
	}

	// Walk the best way through, noting the equal pairs it takes.
	type pair struct{ i, j int }
	var kept []pair
	for i, j := 0, 0; i < n && j < m; {
		w := worth(i, j)
		switch {
		case w >= 0 && best[at(i, j)] == w+best[at(i+1, j+1)]:
			if w >= realignEqual {
				kept = append(kept, pair{i, j})
			}
			i++
			j++
		case best[at(i, j)] == best[at(i+1, j)]:
			i++
		default:
			j++
		}
	}

	// This pass only chooses between scripts of one size. Where the place
	// could keep more lines than it does, the script came from a rough
	// comparison that held on to the lines it was surest of, and is left
	// as it is.
	before := 0
	for i := a0; i < a1; i++ {
		if !del[i] {
			before++
		}
	}
	if len(kept) != before {
		return scratch
	}
	for i := a0; i < a1; i++ {
		del[i] = true
	}
	for j := b0; j < b1; j++ {
		ins[j] = true
	}
	for _, k := range kept {
		del[a0+k.i], ins[b0+k.j] = false, false
	}
	return scratch
}

// choice reports whether some deleted line among old equals one of the lines
// of new, or some inserted line among new one of the lines of old: whether
// the lines could be aligned in another way that keeps as many of them.
func choice(old, new []string, del, ins []bool) bool {
	for i, line := range old {
		if !del[i] {
			continue
		}
		for _, other := range new {
			if line == other {
				return true
			}
		}
	}
	for j, line := range new {
		if !ins[j] {
			continue
		}
		for _, other := range old {
			if line == other {
				return true
			}
		}
	}
	return false
}

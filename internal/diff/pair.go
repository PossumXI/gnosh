package diff

import "slices"

// This file decides which deleted line an inserted line is the new form of,
// so that the display can mark what changed inside the two.
//
// Where lines are replaced, the script only says that these lines went and
// those came. Pairing the first deleted line with the first inserted one,
// the second with the second and so on is wrong as soon as a line is added
// or dropped among them: every pair after it is then two unrelated lines.
// Here each pair is chosen for being alike, and the pairs keep the order of
// the lines: if one deleted line is paired with an inserted line, the
// deleted lines after it are paired only with inserted lines after that one.

const (
	// pairAlike is how alike two lines must be to be a pair, in parts of
	// a thousand of their words (see likeness).
	pairAlike = 500
	// pairTable is the most pairs of lines, deleted times inserted, for
	// which the best pairing of all is worked out. Larger places are
	// paired by looking a little way ahead.
	pairTable = 10000
	// pairAhead is how many inserted lines ahead a deleted line's partner
	// is looked for in a place too large for the table.
	pairAhead = 24
	// bagWords is the most words of a line that are looked at to tell how
	// alike it is to another. A line longer than that is judged by how it
	// starts, which keeps comparing two lines cheap however long they are.
	bagWords = 256
)

// pairLines pairs deleted lines with inserted lines that are like them. It
// returns, for each deleted line, the index of its partner among the
// inserted lines, or -1. Partners rise with the deleted lines. The lines are
// given without their line endings.
func pairLines(dels, inss []string) []int {
	partner := make([]int, len(dels))
	for i := range partner {
		partner[i] = -1
	}
	if len(dels) == 0 || len(inss) == 0 {
		return partner
	}
	da, ib := make([]bag, len(dels)), make([]bag, len(inss))
	for i, line := range dels {
		da[i] = bagOf(line)
	}
	for j, line := range inss {
		ib[j] = bagOf(line)
	}
	if len(dels)*len(inss) <= pairTable {
		pairBest(partner, da, ib)
	} else {
		pairNear(partner, da, ib)
	}
	return partner
}

// pairBest finds the pairing with the greatest likeness in sum, among all
// pairings that keep the lines in order and pair only lines alike enough.
func pairBest(partner []int, da, ib []bag) {
	n, m := len(da), len(ib)
	like := make([]int32, n*m)
	for i := range n {
		for j := range m {
			like[i*m+j] = int32(likeness(da[i], ib[j]))
		}
	}
	// best[i*(m+1)+j] is the greatest sum for deleted lines i and up
	// against inserted lines j and up.
	best := make([]int32, (n+1)*(m+1))
	at := func(i, j int) int { return i*(m+1) + j }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			v := max(best[at(i+1, j)], best[at(i, j+1)])
			if l := like[i*m+j]; l >= pairAlike {
				v = max(v, l+best[at(i+1, j+1)])
			}
			best[at(i, j)] = v
		}
	}
	for i, j := 0, 0; i < n && j < m; {
		switch l := like[i*m+j]; {
		case l >= pairAlike && best[at(i, j)] == l+best[at(i+1, j+1)]:
			partner[i] = j
			i++
			j++
		case best[at(i, j)] == best[at(i+1, j)]:
			i++
		default:
			j++
		}
	}
}

// pairNear pairs each deleted line with the inserted line most like it among
// the next few that are still free.
func pairNear(partner []int, da, ib []bag) {
	next := 0 // the first inserted line still free
	for i := range da {
		bestAt, bestLike := -1, pairAlike-1
		for j := next; j < len(ib) && j < next+pairAhead; j++ {
			if l := likeness(da[i], ib[j]); l > bestLike {
				bestAt, bestLike = j, l
			}
		}
		if bestAt >= 0 {
			partner[i] = bestAt
			next = bestAt + 1
		}
	}
}

// A bag is the words of a line without their order: a quick way to tell how
// alike two lines are without comparing them piece by piece.
type bag struct {
	words []word // sorted by hash
	total int    // the sum of their sizes
}

// A word is one piece of a line that is not spaces: a hash of its text, and
// its size in bytes.
type word struct {
	hash uint64
	size int32
}

// bagOf collects the words of a line, the first bagWords of them.
func bagOf(line string) bag {
	var b bag
	for t := range pieces(line) {
		if t.kind == pieceSpace {
			continue
		}
		if len(b.words) == bagWords {
			break
		}
		if b.words == nil {
			b.words = make([]word, 0, 16)
		}
		// FNV-1a, which is all that telling short words apart needs.
		h := uint64(14695981039346656037)
		for k := t.start; k < t.end; k++ {
			h ^= uint64(line[k])
			h *= 1099511628211
		}
		b.words = append(b.words, word{h, int32(t.end - t.start)})
		b.total += t.end - t.start
	}
	slices.SortFunc(b.words, func(x, y word) int {
		switch {
		case x.hash < y.hash:
			return -1
		case x.hash > y.hash:
			return 1
		}
		return 0
	})
	return b
}

// likeness says how alike two lines are, from 0 for nothing in common to
// 1000 for the same words: the bytes of the words the two have in common,
// counted on both sides, out of the bytes of all their words. Two lines
// without any words, which differ only in their spaces, count as alike.
func likeness(a, b bag) int {
	whole := a.total + b.total
	if whole == 0 {
		return 1000
	}
	// The smaller line bounds what the two can share.
	if 2000*min(a.total, b.total) < pairAlike*whole {
		return 0
	}
	return 2000 * sharedBytes(a, b) / whole
}

// sharedBytes adds up the sizes of the words two lines have in common.
func sharedBytes(a, b bag) int {
	shared := 0
	for i, j := 0, 0; i < len(a.words) && j < len(b.words); {
		switch x, y := a.words[i], b.words[j]; {
		case x.hash < y.hash:
			i++
		case x.hash > y.hash:
			j++
		default:
			shared += int(x.size)
			i++
			j++
		}
	}
	return shared
}

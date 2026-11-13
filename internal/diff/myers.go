package diff

import "math"

// This file finds the smallest edit script between two sequences, by the
// algorithm of Eugene W. Myers, "An O(ND) Difference Algorithm and Its
// Variations" (Algorithmica, 1986), in the linear-space form of its section
// 4b, written from the paper.
//
// The paper's picture: put the first sequence along the top of a grid and
// the second down its side. A path from the top left corner to the bottom
// right one moves right (an element of the first sequence is deleted), down
// (an element of the second is inserted), or diagonally where the two
// elements are equal (it is kept). The smallest script is the path with the
// fewest moves right and down. Diagonal k is the set of points with
// x - y = k; a run of diagonal moves is a "snake".
//
// The search runs from both corners at once. For d = 1, 2, 3, ... it works
// out how far along each diagonal a path with d moves right or down can get,
// forward from the top left and backward from the bottom right. When the two
// meet on a diagonal, the snake they meet in lies on a smallest path. The
// problem is cut in two there and each half is solved the same way.

// Once the budget is spent, the search goes on more roughly. These numbers
// say how roughly.
const (
	// localFactor: a part of the problem with n and m elements is still
	// searched for its smallest script, for up to localFactor*(n+m)
	// comparisons. Small parts and parts with few differences always get
	// their smallest script that way.
	localFactor = 32
	// roughFactor: all the rough work on sequences of n and m elements
	// together is held to roughFactor*(n+m) comparisons. What is still
	// open after that is taken as deleted and inserted whole.
	roughFactor = 256
	// roughFactorWords is the same for the words of two lines, where a
	// rough answer is worth less: lines that different are not marked.
	roughFactorWords = 16
	// reachRounds: where neither an exact search nor a cut at elements
	// that occur once gets anywhere, the search goes forward from the
	// start of the part for this many rounds, and the part is cut at the
	// furthest point that reached.
	reachRounds = 64
)

// unreached marks a diagonal that no backward path of the current length
// gets to. A diagonal no forward path gets to holds -1.
const unreached = math.MaxInt32

// A search that is not going to finish within its allowance is better given
// up early than late. The ground the two halves of a search have covered
// between them grows with the number of rounds; the diagonals looked at grow
// with the square of that number, and the equal elements stepped over grow
// with the ground covered. So from the work done and the share of the ground
// covered, the work the whole search would take can be told roughly. The
// search looks at that four times, when it has used a sixteenth, an eighth,
// a quarter and a half of its allowance, and gives up if the whole would
// take more than hopeless times the allowance. Early on, with little to go
// by, only a search many times too large is given up; later the guess is
// better and the margin smaller.
func hopeless(look int) float64 {
	switch look {
	case 0:
		return 8
	case 1:
		return 4
	case 2:
		return 2
	}
	return 1.25
}

const (
	// looksAhead is how many times a search looks ahead in this way.
	looksAhead = 4
	// foresee is the smallest allowance for which it does. A smaller
	// search is over before it would matter.
	foresee = 1 << 14
)

// An engine holds one comparison of two sequences of numbers.
type engine struct {
	a, b     []int32 // the sequences: elements are equal exactly when their numbers are
	del, ins []bool  // the answer: which elements of a go, and which of b are new

	// vf and vr are the paper's V arrays: for each diagonal, how far the
	// forward search and the backward search have got along it.
	vf, vr []int32

	work    int  // comparisons made so far
	budget  int  // the most the search for the smallest script may make, or -1
	factor  int  // the rough work allowed, in comparisons for each element
	rough   bool // the budget ran out: the script is right, but may not be the smallest
	ceiling int  // once rough, the work at which what is left is no longer looked into

	// Scratch for anchors, sized by the count of distinct numbers.
	ids            int
	countA, countB []int32
	placeB         []int32
}

// script compares the sequences a and b, whose elements are numbers from 0
// up to but not including ids. It reports which elements of a are deleted
// and which of b are inserted, how many comparisons that took, and whether
// the script is known to be the smallest. budget is the most work the search
// for the smallest script may do, or -1 for no limit; zero means none at
// all, which leaves only the rough comparison.
func script(a, b []int32, ids, budget int) (del, ins []bool, work int, minimal bool) {
	return scriptWith(a, b, ids, budget, roughFactor)
}

// scriptWith is script with a say in how much rough work is allowed once
// the budget is spent: factor comparisons for each element of a and b.
func scriptWith(a, b []int32, ids, budget, factor int) (del, ins []bool, work int, minimal bool) {
	del, ins = make([]bool, len(a)), make([]bool, len(b))
	if len(a) == 0 || len(b) == 0 {
		for i := range del {
			del[i] = true
		}
		for j := range ins {
			ins[j] = true
		}
		return del, ins, 0, true
	}

	// An element with no equal in the other sequence cannot be kept by any
	// script. Taking those out first changes nothing about which scripts
	// are smallest, and leaves the search only the elements that could be
	// matched: in two texts that share few lines, very little.
	inA, inB := make([]bool, ids), make([]bool, ids)
	for _, id := range a {
		inA[id] = true
	}
	for _, id := range b {
		inB[id] = true
	}
	ra, rb := a, b
	var fromA, fromB []int32 // where the elements left in ra and rb stood in a and b
	if lone := countLone(a, inB); lone > 0 {
		ra, fromA = withoutLone(a, inB, del, lone)
	}
	if lone := countLone(b, inA); lone > 0 {
		rb, fromB = withoutLone(b, inA, ins, lone)
	}

	e := &engine{a: ra, b: rb, budget: budget, factor: factor, ids: ids}
	e.del, e.ins = del, ins
	if fromA != nil {
		e.del = make([]bool, len(ra))
	}
	if fromB != nil {
		e.ins = make([]bool, len(rb))
	}
	size := len(ra) + len(rb) + 1
	e.vf, e.vr = make([]int32, size), make([]int32, size)
	e.solve(0, len(ra), 0, len(rb))

	for k, i := range fromA {
		del[i] = e.del[k]
	}
	for k, j := range fromB {
		ins[j] = e.ins[k]
	}
	return del, ins, e.work, !e.rough
}

// countLone counts the elements of s that the other sequence does not have.
func countLone(s []int32, inOther []bool) int {
	n := 0
	for _, id := range s {
		if !inOther[id] {
			n++
		}
	}
	return n
}

// withoutLone returns s without the elements the other sequence does not
// have, of which there are lone, and where each element that is left stood
// in s. The ones taken out are flagged in gone.
func withoutLone(s []int32, inOther []bool, gone []bool, lone int) (rest, from []int32) {
	rest = make([]int32, 0, len(s)-lone)
	from = make([]int32, 0, len(s)-lone)
	for i, id := range s {
		if inOther[id] {
			rest = append(rest, id)
			from = append(from, int32(i))
		} else {
			gone[i] = true
		}
	}
	return rest, from
}

// solve works out the script for elements a0 up to a1 of a against elements
// b0 up to b1 of b, and flags what is deleted and inserted.
//
// While the budget lasts it cuts the part in two at a middle snake and solves
// the halves. After that it goes on roughly, trying in turn what costs
// little: an exact search held to a small allowance, which is enough for a
// part that is small or has few differences; a cut at the elements that
// occur once on each side; and, when the part is neither, a short search
// forward from its start, a cut where that search got furthest, and the same
// again for what lies beyond the cut.
func (e *engine) solve(a0, a1, b0, b1 int) {
	a, b := e.a, e.b
	// tried is the size the part had when the exact search and the anchors
	// last failed on it. They are tried again only once the part has
	// shrunk to half of that, so that all the tries together cost no more
	// than twice the first.
	tried := math.MaxInt
	for {
		for a0 < a1 && b0 < b1 && a[a0] == b[b0] {
			a0++
			b0++
			e.work++
		}
		for a0 < a1 && b0 < b1 && a[a1-1] == b[b1-1] {
			a1--
			b1--
			e.work++
		}
		if a0 == a1 || b0 == b1 {
			e.flag(a0, a1, b0, b1)
			return
		}
		// Both sides have elements, and neither their first nor their last
		// elements are equal: the smallest script has at least two steps.
		n, m := a1-a0, b1-b0

		spent := 0 // what a search that gave up has already put into this part
		if !e.rough {
			allow := -1
			if e.budget >= 0 {
				allow = max(e.budget-e.work, 0)
			}
			start := e.work
			if allow != 0 {
				if x, y, u, v, ok := e.middle(a0, a1, b0, b1, allow); ok {
					e.solve(a0, x, b0, y)
					e.solve(u, a1, v, b1)
					return
				}
			}
			spent = e.work - start
			e.rough = true
			e.ceiling = e.work + e.factor*(len(e.a)+len(e.b))
		}
		if e.work >= e.ceiling {
			e.flag(a0, a1, b0, b1)
			return
		}

		if n+m <= tried/2 {
			if local := localFactor * (n + m); spent < local {
				if x, y, u, v, ok := e.middle(a0, a1, b0, b1, local); ok {
					e.solve(a0, x, b0, y)
					e.solve(u, a1, v, b1)
					return
				}
			}
			if e.anchor(a0, a1, b0, b1) {
				return
			}
			tried = n + m
		}
		x, y := e.reach(a0, a1, b0, b1)
		e.solve(a0, x, b0, y)
		a0, b0 = x, y
	}
}

// flag marks elements a0 up to a1 of a as deleted and b0 up to b1 of b as
// inserted.
func (e *engine) flag(a0, a1, b0, b1 int) {
	for i := a0; i < a1; i++ {
		e.del[i] = true
	}
	for j := b0; j < b1; j++ {
		e.ins[j] = true
	}
}

// middle finds a snake that a smallest path through the part a0 to a1, b0 to
// b1 goes along, somewhere near the middle of that path. It returns the
// snake's two ends, (x, y) and (u, v), as positions in a and b: a smallest
// script for the whole part is a smallest script for what lies before
// (x, y), then the kept elements of the snake, then a smallest script for
// what lies after (u, v).
//
// The caller has made sure that both parts have elements and that neither
// their first elements nor their last are equal. allow is the most
// comparisons to make, or -1 for no limit; when they are used up, or when it
// is plain that they will not be enough (see hopeless), middle gives up and
// reports false.
//
// Positions below are counted inside the part: x from 0 to n along a, y from
// 0 to m along b. Diagonal k is x - y = k, and the diagonals that cross the
// part run from -m to n. Unlike the paper's listing, a move is only taken
// when it stays inside the part, so every position kept is a real one.
func (e *engine) middle(a0, a1, b0, b1, allow int) (x, y, u, v int, ok bool) {
	a, b := e.a[a0:a1], e.b[b0:b1]
	n, m := len(a), len(b)
	delta := n - m
	odd := delta&1 != 0
	// vf[m+k] and vr[m+k] belong to diagonal k.
	vf, vr := e.vf[:n+m+1], e.vr[:n+m+1]
	vf[m] = 0              // forward, no moves: the top left corner
	vr[m+delta] = int32(n) // backward, no moves: the bottom right corner
	stop := math.MaxInt    // the work at which to give up
	look := math.MaxInt    // the work at which to look ahead next
	start, looks := e.work, 0
	if allow >= 0 {
		stop = e.work + allow
		if allow >= foresee {
			look = e.work + allow/16
		}
	}
	// The most elements of both sequences that a forward path and a
	// backward path have behind them, and the equal elements stepped over
	// so far.
	far, near, stepped := 0, 0, 0

	for d := 1; d <= n+m; d++ {
		// Forward: how far paths with d moves get, from those with d-1.
		lo, hi := -d, d
		if lo < -m {
			lo = -m + (m+d)&1
		}
		if hi > n {
			hi = n - (n+d)&1
		}
		for k := lo; k <= hi; k += 2 {
			from := -1
			if k < d && k < n {
				// Down from diagonal k+1: an element of b is inserted.
				if p := int(vf[m+k+1]); p >= 0 && p-(k+1) < m {
					from = p
				}
			}
			if k > -d && k > -m {
				// Right from diagonal k-1: an element of a is deleted.
				if p := int(vf[m+k-1]); p >= 0 && p < n && p+1 > from {
					from = p + 1
				}
			}
			e.work++
			if from < 0 {
				vf[m+k] = -1
				continue
			}
			px, py := from, from-k
			for px < n && py < m && a[px] == b[py] {
				px++
				py++
			}
			vf[m+k] = int32(px)
			e.work += px - from
			stepped += px - from
			far = max(far, px+py)
			// With an odd difference in length the two searches can only
			// meet here: the backward search has made d-1 moves.
			if odd && k >= delta-(d-1) && k <= delta+(d-1) && px >= int(vr[m+k]) {
				return a0 + from, b0 + from - k, a0 + px, b0 + py, true
			}
			if e.work > stop {
				return 0, 0, 0, 0, false
			}
		}

		// Backward: the same from the bottom right corner, where getting
		// further means a smaller x.
		lo, hi = delta-d, delta+d
		if lo < -m {
			lo = -m + (n+d)&1
		}
		if hi > n {
			hi = n - (m+d)&1
		}
		for k := lo; k <= hi; k += 2 {
			from := unreached
			if k < delta+d && k < n {
				// Left from diagonal k+1.
				if p := int(vr[m+k+1]); p != unreached && p > 0 {
					from = p - 1
				}
			}
			if k > delta-d && k > -m {
				// Up from diagonal k-1.
				if p := int(vr[m+k-1]); p != unreached && p-(k-1) > 0 && p < from {
					from = p
				}
			}
			e.work++
			if from == unreached {
				vr[m+k] = unreached
				continue
			}
			px, py := from, from-k
			for px > 0 && py > 0 && a[px-1] == b[py-1] {
				px--
				py--
			}
			vr[m+k] = int32(px)
			e.work += from - px
			stepped += from - px
			near = max(near, n-px+m-py)
			// With an even difference the searches meet here: both have
			// made d moves.
			if !odd && k >= -d && k <= d && px <= int(vf[m+k]) {
				return a0 + px, b0 + py, a0 + from, b0 + from - k, true
			}
			if e.work > stop {
				return 0, 0, 0, 0, false
			}
		}

		if e.work >= look {
			if covered := far + near; covered < n+m {
				// times is how many times the ground covered so far the
				// whole of it is.
				times := float64(n+m) / float64(max(covered, 1))
				looked := float64(e.work - start - stepped)
				if looked*times*times+float64(stepped)*times > hopeless(looks)*float64(allow) {
					return 0, 0, 0, 0, false
				}
			}
			looks++
			look = math.MaxInt
			if looks < looksAhead {
				look = start + allow>>(4-looks)
			}
		}
	}
	// The searches always meet before d reaches n+m. Should they not, the
	// caller's rough comparison is still right.
	return 0, 0, 0, 0, false
}

// reach runs the forward half of the search alone, for reachRounds rounds,
// from the start of the part a0 to a1, b0 to b1, and returns the point where
// it got furthest: the one with the most elements of both sides behind it. A
// script for what lies before that point and a script for what lies after it
// make a script for the part, and the first of the two is a small one: no
// more than reachRounds elements are deleted or inserted on the way there.
//
// The caller has made sure of the same things as for middle. The point
// returned is never the start of the part nor its end, so both pieces are
// smaller than the part.
func (e *engine) reach(a0, a1, b0, b1 int) (x, y int) {
	a, b := e.a[a0:a1], e.b[b0:b1]
	n, m := len(a), len(b)
	vf := e.vf[:n+m+1]
	vf[m] = 0
	bx, by := 0, 0
	for d := 1; d <= reachRounds; d++ {
		lo, hi := -d, d
		if lo < -m {
			lo = -m + (m+d)&1
		}
		if hi > n {
			hi = n - (n+d)&1
		}
		done := false
		for k := lo; k <= hi; k += 2 {
			from := -1
			if k < d && k < n {
				if p := int(vf[m+k+1]); p >= 0 && p-(k+1) < m {
					from = p
				}
			}
			if k > -d && k > -m {
				if p := int(vf[m+k-1]); p >= 0 && p < n && p+1 > from {
					from = p + 1
				}
			}
			e.work++
			if from < 0 {
				vf[m+k] = -1
				continue
			}
			px, py := from, from-k
			for px < n && py < m && a[px] == b[py] {
				px++
				py++
			}
			vf[m+k] = int32(px)
			e.work += px - from
			if px == n && py == m {
				// The end of the part: a path this short gets all the
				// way. The points found on the way there will do.
				done = true
				continue
			}
			if px+py > bx+by {
				bx, by = px, py
			}
		}
		if done {
			break
		}
	}
	return a0 + bx, b0 + by
}

// anchor cuts the part a0 to a1, b0 to b1 at the elements that occur exactly
// once in each of its two sides and stand in the same order in both, keeps
// those, and solves the pieces between them. This is the idea of patience
// diff: a line that is there once before and once after is almost surely the
// same line. It reports false, having done nothing, when there is no such
// element.
func (e *engine) anchor(a0, a1, b0, b1 int) bool {
	if e.countA == nil {
		e.countA = make([]int32, e.ids)
		e.countB = make([]int32, e.ids)
		e.placeB = make([]int32, e.ids)
	}
	a, b := e.a, e.b
	for i := a0; i < a1; i++ {
		e.countA[a[i]]++
	}
	for j := b0; j < b1; j++ {
		e.countB[b[j]]++
		e.placeB[b[j]] = int32(j)
	}
	type match struct{ i, j int32 }
	var matches []match // in the order of a
	for i := a0; i < a1; i++ {
		if id := a[i]; e.countA[id] == 1 && e.countB[id] == 1 {
			matches = append(matches, match{int32(i), e.placeB[id]})
		}
	}
	for i := a0; i < a1; i++ {
		e.countA[a[i]] = 0
	}
	for j := b0; j < b1; j++ {
		e.countB[b[j]] = 0
	}
	e.work += 2 * (a1 - a0 + b1 - b0)
	if len(matches) == 0 {
		return false
	}

	// The longest run of matches whose places in b also rise, found the
	// way a game of patience is laid out: ends[k] is the match that ends
	// the best run of k+1 matches so far, the one with the lowest place in
	// b, and before[i] is the match that comes before match i in its run.
	ends := make([]int32, 0, 16)
	before := make([]int32, len(matches))
	for i, mt := range matches {
		lo, hi := 0, len(ends)
		for lo < hi {
			mid := int(uint(lo+hi) >> 1)
			if matches[ends[mid]].j < mt.j {
				lo = mid + 1
			} else {
				hi = mid
			}
			e.work++
		}
		before[i] = -1
		if lo > 0 {
			before[i] = ends[lo-1]
		}
		if lo == len(ends) {
			ends = append(ends, int32(i))
		} else {
			ends[lo] = int32(i)
		}
	}
	chain := make([]match, len(ends))
	for k, i := len(ends)-1, ends[len(ends)-1]; k >= 0; k, i = k-1, before[i] {
		chain[k] = matches[i]
	}

	pa, pb := a0, b0
	for _, mt := range chain {
		e.solve(pa, int(mt.i), pb, int(mt.j))
		pa, pb = int(mt.i)+1, int(mt.j)+1
	}
	e.solve(pa, a1, pb, b1)
	return true
}

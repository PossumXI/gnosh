package diff

// DefaultContext is the number of unchanged lines shown around each change
// when a Context option is zero.
const DefaultContext = 3

// contextOf turns a Context option as callers write it into a number of
// lines: zero means DefaultContext, a negative number means none.
func contextOf(context int) int {
	switch {
	case context == 0:
		return DefaultContext
	case context < 0:
		return 0
	}
	return context
}

// A Hunk is a group of changes that lie close together, with the unchanged
// lines around them: one "@@" section of a unified diff.
type Hunk struct {
	// OldStart is the number of the hunk's first line in the text before,
	// counting from 1, and OldLines is how many lines of that text the
	// hunk covers. NewStart and NewLines say the same for the text after.
	// When a hunk covers no lines of a text, its start there is the number
	// of the line the hunk comes after, zero at the top: the unified
	// format's own convention.
	OldStart, OldLines int
	NewStart, NewLines int
	// Edits are the hunk's runs. The first and the last may be Keep runs,
	// cut down to the context asked for.
	Edits []Edit
}

// Hunks groups the changes of a script, as Lines and Compare return it, into
// hunks with context unchanged lines before and after each change. Changes
// with no more than twice that many unchanged lines between them, whose
// contexts would touch or overlap, share one hunk. Zero context means
// DefaultContext and a negative number means none. A script without changes
// has no hunks.
func Hunks(edits []Edit, context int) []Hunk {
	// Lay the edits out as runs, to find where each begins.
	runs := make([]run, len(edits))
	i, j := 0, 0
	for k, e := range edits {
		r := run{op: e.Op, a0: i, a1: i, b0: j, b1: j}
		if e.Op != Insert {
			r.a1 += len(e.Lines)
		}
		if e.Op != Delete {
			r.b1 += len(e.Lines)
		}
		runs[k] = r
		i, j = r.a1, r.b1
	}

	groups := group(runs, contextOf(context))
	hunks := make([]Hunk, len(groups))
	for h, g := range groups {
		hunk := Hunk{
			OldStart: g.a0 + 1, OldLines: g.a1 - g.a0,
			NewStart: g.b0 + 1, NewLines: g.b1 - g.b0,
			Edits: make([]Edit, len(g.runs)),
		}
		if hunk.OldLines == 0 {
			hunk.OldStart--
		}
		if hunk.NewLines == 0 {
			hunk.NewStart--
		}
		for k, r := range g.runs {
			// Run k of the group is a part of edit g.first+k: all of it,
			// or for a Keep at the hunk's edge its first or last lines.
			whole := runs[g.first+k]
			from, to := r.a0-whole.a0, r.a1-whole.a0
			if r.op == Insert {
				from, to = r.b0-whole.b0, r.b1-whole.b0
			}
			hunk.Edits[k] = Edit{Op: r.op, Lines: edits[g.first+k].Lines[from:to:to]}
		}
		hunks[h] = hunk
	}
	return hunks
}

// A hunkRuns is one hunk of a script: runs first up to first+len(runs) of
// it, with the Keep runs at the two edges cut down to the context. It covers
// lines a0 up to a1 of the text before and b0 up to b1 of the text after.
type hunkRuns struct {
	first  int
	runs   []run
	a0, a1 int
	b0, b1 int
}

// group cuts a script into hunks with context unchanged lines around each
// change. Every run that is not a Keep counts as a change.
func group(runs []run, context int) []hunkRuns {
	var hunks []hunkRuns
	for i := 0; i < len(runs); i++ {
		if runs[i].op == Keep {
			continue
		}
		h := hunkRuns{first: i}
		if i > 0 {
			// The last lines of the kept run above, as far as it has any
			// to give.
			lead := runs[i-1]
			if n := min(context, lead.a1-lead.a0); n > 0 {
				lead.a0, lead.b0 = lead.a1-n, lead.b1-n
				h.first = i - 1
				h.runs = append(h.runs, lead)
			}
		}
		for i < len(runs) {
			if runs[i].op != Keep {
				h.runs = append(h.runs, runs[i])
				i++
				continue
			}
			keep := runs[i]
			// No more than twice the context, said so that a very large
			// context cannot overflow.
			if i+1 < len(runs) && (keep.a1-keep.a0+1)/2 <= context {
				// Too few lines to part two hunks: the next change joins
				// this one.
				h.runs = append(h.runs, keep)
				i++
				continue
			}
			if n := min(context, keep.a1-keep.a0); n > 0 {
				keep.a1, keep.b1 = keep.a0+n, keep.b0+n
				h.runs = append(h.runs, keep)
			}
			break
		}
		first, last := h.runs[0], h.runs[len(h.runs)-1]
		h.a0, h.b0, h.a1, h.b1 = first.a0, first.b0, last.a1, last.b1
		hunks = append(hunks, h)
	}
	return hunks
}

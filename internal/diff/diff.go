package diff

// An Op says what an Edit does with its lines.
type Op uint8

const (
	// Keep is a run of lines that both texts have.
	Keep Op = iota
	// Delete is a run of lines that only the text before has.
	Delete
	// Insert is a run of lines that only the text after has.
	Insert

	// The two kinds below never leave the package. They stand for lines
	// that say the same thing in both texts and differ only in how they
	// end, which an edit script has to call deleted and inserted and a
	// person would rather be told about once.

	// endingPair is a run of lines whose text is the same and whose line
	// ending changed between LF and CRLF.
	endingPair
	// newlinePair is a run of lines whose text is the same, where one of
	// them is the last line of its text and has no newline at all.
	newlinePair
)

// String names the op: "keep", "delete" or "insert".
func (op Op) String() string {
	switch op {
	case Keep:
		return "keep"
	case Delete:
		return "delete"
	case Insert:
		return "insert"
	}
	return "unknown"
}

// An Edit is one step of an edit script: a run of whole lines that is kept,
// deleted or inserted.
type Edit struct {
	// Op says which.
	Op Op
	// Lines are the lines of the run, each with its line ending exactly as
	// it stood in its text: "\n", "\r\n", or nothing for a last line that
	// was not ended. The lines of a Keep are the same in both texts.
	Lines []string
}

// DefaultBudget is the work a comparison may spend looking for the smallest
// script when Options.Budget is zero: about sixteen million line comparisons,
// a fraction of a second.
const DefaultBudget = 1 << 24

// Options are the choices Compare leaves at their defaults.
type Options struct {
	// Budget is the most work the search for the smallest script may do,
	// counted in line comparisons. Zero means DefaultBudget. A negative
	// number means no limit: the script is then always the smallest, and
	// comparing two large texts with little in common can take time and
	// memory in proportion to the product of their lengths.
	//
	// When the budget runs out, what is left is compared more roughly (see
	// Compare). The result is still a correct script.
	Budget int
}

// limit turns a Budget as callers write it into the engine's: a number of
// comparisons, or -1 for no limit.
func limit(budget int) int {
	switch {
	case budget == 0:
		return DefaultBudget
	case budget < 0:
		return -1
	}
	return budget
}

// A Result is what Compare found.
type Result struct {
	// Edits is the script: runs of kept, deleted and inserted lines, in
	// the order of the texts. See Lines for what it promises.
	Edits []Edit
	// Stats counts what changed.
	Stats Stats
	// Minimal says that the script is known to be the smallest there is:
	// no other deletes and inserts fewer lines. It is false when the
	// budget ran out, or was plainly not going to be enough, and part of
	// the texts was compared roughly.
	Minimal bool
	// Work is the number of line comparisons the search made. Under a
	// budget it never exceeds the bound given in Compare's documentation.
	Work int
}

// Lines compares two texts line by line and returns the script that turns
// before into after, with the default Options.
//
// The script is a list of runs. Read in order, the lines of its Keep and
// Delete runs are exactly the lines of before, and the lines of its Keep and
// Insert runs are exactly the lines of after: joined, they give the texts
// back byte for byte. No two neighbouring runs have the same Op, and where
// lines are replaced the Delete comes before the Insert. Two identical texts
// give one Keep, or no runs at all when they are empty.
//
// A line is everything up to and including its "\n". Lines are compared
// whole, line ending included: "a\r\n" and "a\n" are different lines, and so
// are "a\n" and an "a" that ends the text without a newline, just as the
// unified format has it. Stats and Render tell such pairs apart from lines
// whose words changed.
func Lines(before, after string) []Edit {
	return compare(before, after, limit(0)).edits()
}

// Compare compares two texts line by line, as Lines does, and also reports
// the counts and how the comparison went.
//
// The script is the smallest there is, found by Myers' algorithm in its
// linear-space form: the time taken grows with the size of the texts times
// the size of the difference, and the memory with the size of the texts
// alone. Lines the two texts start and end with in common, and lines that
// have no equal in the other text at all, are set aside first, so the search
// only sees what could be matched in more than one way.
//
// Of the scripts of that size, the one a person reads most easily is chosen,
// in two steps that leave the size alone. Where kept lines could be kept in
// more than one way, such as the brace and the empty line between a function
// that was changed and one that was added, they are kept so that each
// changed line stands against its own old form. And a run of only-deleted or
// only-inserted lines that could begin a line sooner or later is put where
// its edges fall at empty lines and at the least indented lines, so that an
// inserted function is shown as that function and not as the tail of one and
// the head of the next.
//
// When the search would cost more than opts.Budget, which it finds out by
// running out or, sooner, by seeing how little of the texts it has got
// through for the work done, the part of the texts still open is compared
// more roughly instead. A piece that is small, or differs little, still gets
// its smallest script. A larger one is cut at the lines that occur exactly
// once on each side and in the same order (the idea of patience diff), and
// the pieces between them are treated the same way. A piece with no such
// lines is gone through from its start, a short search at a time, each time
// taking the furthest point that search reached. Should even that add up to
// too much, what is left is taken as deleted and inserted whole. The script
// stays correct throughout, and Result.Minimal is false.
//
// However different the texts are, comparing texts of n and m lines makes at
// most Budget + 1000*(n+m) line comparisons; Result.Work is the number it
// made. Splitting the texts into lines and the two steps above take time in
// proportion to the size of the texts on top of that.
func Compare(before, after string, opts Options) Result {
	c := compare(before, after, limit(opts.Budget))
	stats := c.stats()
	return Result{Edits: c.edits(), Stats: stats, Minimal: c.minimal, Work: c.work}
}

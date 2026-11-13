package diff

import (
	"iter"
	"unicode"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// A Span is a stretch of a line: its bytes from Start up to but not
// including End.
type Span struct {
	Start, End int
}

// How alike two lines must be for the difference between them to be worth
// marking: at least spanAlike of every spanWhole bytes of their words, taken
// over both lines together, must be common to both.
const (
	spanAlike = 1
	spanWhole = 2
)

const (
	// spanBudget bounds the comparison of the words of two lines. It is
	// far more than two lines of ordinary length need.
	spanBudget = 1 << 16
	// spanLongest is the longest line, in bytes, that is looked into. A
	// line longer than that is not something a person reads word by word.
	spanLongest = 1 << 14
)

// Spans compares a deleted line with the inserted line that took its place
// and returns what changed inside them: the stretches of old that are not in
// new, and the stretches of new that are not in old, each list in order.
// The lines are given without their line endings.
//
// Lines are compared word by word. A word is a run of letters, digits and
// underscores; a run of spaces and tabs is one piece; every other character
// is a piece of its own, and so is every character of a script written
// without spaces between words, such as Chinese or Japanese. Where one piece
// was replaced by another and the two share most of their characters, as
// "colour" and "color" do, only the part between what they have in common at
// the front and at the back is given. A span always begins and ends between
// two grapheme clusters.
//
// What the spans leave is the same in both lines: old without its spans and
// new without its spans are one and the same string.
//
// ok is false, and there are no spans, when the lines have too little in
// common for marks to help: when less than half of their words, counted in
// bytes over both lines and leaving spaces aside, is common to both. Marking
// most of two lines would only be noise beside the lines themselves. It is
// also false for a line longer than 16 KB, which is not looked into.
func Spans(old, new string) (oldSpans, newSpans []Span, ok bool) {
	if old == new {
		return nil, nil, true
	}
	if len(old) > spanLongest || len(new) > spanLongest {
		return nil, nil, false
	}
	ta, tb := tokenize(old), tokenize(new)

	// Number the pieces, so that equal pieces have equal numbers.
	seen := make(map[string]int32, len(ta)+len(tb))
	numbered := func(s string, toks []token) []int32 {
		out := make([]int32, len(toks))
		for i, t := range toks {
			piece := s[t.start:t.end]
			id, found := seen[piece]
			if !found {
				id = int32(len(seen))
				seen[piece] = id
			}
			out[i] = id
		}
		return out
	}
	na, nb := numbered(old, ta), numbered(new, tb)
	del, ins, _, _ := scriptWith(na, nb, len(seen), spanBudget, roughFactorWords)
	joinAcrossBlanks(na, ta, del)
	joinAcrossBlanks(nb, tb, ins)

	common, total := 0, 0 // bytes of words both lines have, and of all words
	for _, t := range ta {
		total += t.weight()
	}
	for _, t := range tb {
		total += t.weight()
	}
	i, j := 0, 0
	for i < len(ta) || j < len(tb) {
		if i < len(ta) && j < len(tb) && !del[i] && !ins[j] {
			common += 2 * ta[i].weight()
			i++
			j++
			continue
		}
		// A place where pieces were deleted, inserted, or both.
		si, sj := i, j
		for i < len(ta) && del[i] {
			i++
		}
		for j < len(tb) && ins[j] {
			j++
		}
		if i == si && j == sj {
			// The flags do not pair up, which script does not do. The
			// rest of both lines counts as changed.
			i, j = len(ta), len(tb)
		}
		var o, n Span
		if i > si {
			o = Span{ta[si].start, ta[i-1].end}
		}
		if j > sj {
			n = Span{tb[sj].start, tb[j-1].end}
		}
		if o.End > o.Start && n.End > n.Start {
			var shared int
			o, n, shared = narrow(old, new, o, n)
			common += 2 * shared
		}
		if o.End > o.Start {
			oldSpans = appendSpan(oldSpans, o)
		}
		if n.End > n.Start {
			newSpans = appendSpan(newSpans, n)
		}
	}
	if common*spanWhole < total*spanAlike {
		return nil, nil, false
	}
	return oldSpans, newSpans, true
}

// joinAcrossBlanks tidies the flags of one line's pieces. Where two runs of
// changed pieces have nothing but one kept run of blanks between them, and
// one of the two runs could as well begin or end one piece over because the
// blanks at its other end are the same, the run is moved over, and the two
// become one run with the blanks inside it. An argument added to a call then
// reads as one insertion, ", b", and not as a comma here and a "b " there.
// Moving a run this way leaves the kept pieces what they were, in the same
// order, so the flags describe the same change.
func joinAcrossBlanks(ids []int32, toks []token, changed []bool) {
	for i := 1; i+1 < len(toks); i++ {
		if changed[i] || toks[i].kind != pieceSpace || !changed[i-1] || !changed[i+1] {
			continue
		}
		// Piece i is kept blanks with changed pieces on both sides. The
		// run after it ends at piece end-1, the run before it starts at
		// piece start.
		end := i + 1
		for end < len(toks) && changed[end] {
			end++
		}
		start := i - 1
		for start > 0 && changed[start-1] {
			start--
		}
		switch ids[i] {
		case ids[end-1]:
			// The run after ends in the same blanks: it can begin with
			// these and leave those.
			changed[i], changed[end-1] = true, false
		case ids[start]:
			// The run before begins with the same blanks: it can end with
			// these and leave those.
			changed[i], changed[start] = true, false
		}
	}
}

// appendSpan adds a span to a list, joining it to the last one when the two
// touch.
func appendSpan(spans []Span, s Span) []Span {
	if n := len(spans); n > 0 && spans[n-1].End == s.Start {
		spans[n-1].End = s.End
		return spans
	}
	return append(spans, s)
}

// narrow looks at a stretch of the old line that was replaced by a stretch
// of the new one. When the two start or end with the same characters, and
// those make up at least half of the longer stretch, it returns the
// stretches without them: "getName" replaced by "getNames" is an inserted
// "s". Two stretches of blanks need not share that much. Otherwise it
// returns the stretches as they are. shared is the number
// of bytes it took off each for being common to both, not counting spaces
// and tabs.
func narrow(old, new string, o, n Span) (Span, Span, int) {
	a, b := old[o.Start:o.End], new[n.Start:n.End]
	shared := 0
	// The characters the two have in common at the front.
	front, frontBytes := 0, 0
	ra, rb := a, b
	for ra != "" && rb != "" {
		ca, _, afterA := text.NextGrapheme(ra)
		cb, _, afterB := text.NextGrapheme(rb)
		if ca != cb {
			break
		}
		front++
		frontBytes += len(ca)
		if ca != " " && ca != "\t" {
			shared += len(ca)
		}
		ra, rb = afterA, afterB
	}
	// The characters they have in common at the back, of what is left.
	endsA, endsB := clusterEnds(ra), clusterEnds(rb)
	back, backBytes := 0, 0
	for back < len(endsA) && back < len(endsB) {
		ia, ib := len(endsA)-1-back, len(endsB)-1-back
		startA, startB := 0, 0
		if ia > 0 {
			startA = endsA[ia-1]
		}
		if ib > 0 {
			startB = endsB[ib-1]
		}
		cluster := ra[startA:endsA[ia]]
		if cluster != rb[startB:endsB[ib]] {
			break
		}
		back++
		backBytes += len(cluster)
		if cluster != " " && cluster != "\t" {
			shared += len(cluster)
		}
	}
	// Two stretches of blanks always share what they share: indentation
	// that grew is the old indentation and more. For anything else, a few
	// letters in common do not make two words one.
	longer := front + max(len(endsA), len(endsB))
	if front+back == 0 || 2*(front+back) < longer && !(blanks(a) && blanks(b)) {
		return o, n, 0
	}
	o.Start += frontBytes
	o.End -= backBytes
	n.Start += frontBytes
	n.End -= backBytes
	return o, n, shared
}

// blanks reports whether s is made of spaces and tabs alone.
func blanks(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}

// clusterEnds returns where each grapheme cluster of s ends.
func clusterEnds(s string) []int {
	var ends []int
	pos := 0
	for rest := s; rest != ""; {
		cluster, _, after := text.NextGrapheme(rest)
		pos += len(cluster)
		ends = append(ends, pos)
		rest = after
	}
	return ends
}

// The kinds of piece a line is cut into.
const (
	pieceSpace = iota // a run of spaces and tabs
	pieceWord         // a run of letters, digits and underscores
	pieceOther        // any other single character
)

// A token is one piece of a line: its bytes from start up to end.
type token struct {
	start, end int
	kind       uint8
}

// weight is what a piece counts for when two lines are judged alike: its
// length, or nothing for spaces, which say little about what a line is.
func (t token) weight() int {
	if t.kind == pieceSpace {
		return 0
	}
	return t.end - t.start
}

// tokenize cuts a line into its pieces.
func tokenize(s string) []token {
	toks := make([]token, 0, len(s)/4+1)
	for t := range pieces(s) {
		toks = append(toks, t)
	}
	return toks
}

// pieces returns an iterator over the pieces of a line, in order.
func pieces(s string) iter.Seq[token] {
	return func(yield func(token) bool) {
		var cur token // the piece being put together, if cur.end > cur.start
		pos := 0
		for rest := s; rest != ""; {
			cluster, width, after := text.NextGrapheme(rest)
			kind := pieceKind(cluster, width)
			end := pos + len(cluster)
			if cur.end > cur.start && kind != pieceOther && cur.kind == kind {
				cur.end = end
			} else {
				if cur.end > cur.start && !yield(cur) {
					return
				}
				cur = token{pos, end, kind}
			}
			pos, rest = end, after
		}
		if cur.end > cur.start {
			yield(cur)
		}
	}
}

// pieceKind says what kind of piece a grapheme cluster belongs to. width is
// the cluster's width in cells: a letter two cells wide belongs to a script
// that puts no spaces between words, and is a piece of its own.
func pieceKind(cluster string, width int) uint8 {
	c := cluster[0]
	if c < utf8.RuneSelf {
		switch {
		case c == ' ' || c == '\t':
			return pieceSpace
		case c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			return pieceWord
		}
		return pieceOther
	}
	r, _ := utf8.DecodeRuneInString(cluster)
	switch {
	case unicode.IsSpace(r):
		return pieceSpace
	case width < 2 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
		return pieceWord
	}
	return pieceOther
}

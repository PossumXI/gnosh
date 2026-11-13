package diff

import (
	"slices"
	"strings"
	"testing"
)

// show writes a script in a few characters, for comparing with what a test
// expects: "=" for a kept line, "-" for a deleted and "+" for an inserted
// one, each followed by the line with its ending spelled out.
func show(edits []Edit) string {
	var b strings.Builder
	for _, e := range edits {
		for _, line := range e.Lines {
			b.WriteByte("=-+"[e.Op])
			line = strings.ReplaceAll(line, "\r", `\r`)
			line = strings.ReplaceAll(line, "\n", `\n`)
			b.WriteString(line)
			b.WriteByte(' ')
		}
	}
	return strings.TrimSuffix(b.String(), " ")
}

func TestLines(t *testing.T) {
	long := strings.Repeat("a very long line ", 4000)
	tests := []struct {
		name          string
		before, after string
		want          string
	}{
		// Nothing, and the same thing twice.
		{"both empty", "", "", ""},
		{"identical", "a\nb\nc\n", "a\nb\nc\n", `=a\n =b\n =c\n`},
		{"identical, no final newline", "a\nb", "a\nb", `=a\n =b`},
		{"one newline, twice", "\n", "\n", `=\n`},

		// Empty to something and back.
		{"empty to one line", "", "a\n", `+a\n`},
		{"empty to lines", "", "a\nb\n", `+a\n +b\n`},
		{"empty to an open line", "", "a", `+a`},
		{"one line to empty", "a\n", "", `-a\n`},
		{"lines to empty", "a\nb\n", "", `-a\n -b\n`},
		{"empty to a newline", "", "\n", `+\n`},

		// A change at the very start and at the very end.
		{"first line changed", "a\nb\nc\n", "x\nb\nc\n", `-a\n +x\n =b\n =c\n`},
		{"last line changed", "a\nb\nc\n", "a\nb\nx\n", `=a\n =b\n -c\n +x\n`},
		{"line added at the start", "b\nc\n", "a\nb\nc\n", `+a\n =b\n =c\n`},
		{"line added at the end", "a\nb\n", "a\nb\nc\n", `=a\n =b\n +c\n`},
		{"line removed at the start", "a\nb\nc\n", "b\nc\n", `-a\n =b\n =c\n`},
		{"line removed at the end", "a\nb\nc\n", "a\nb\n", `=a\n =b\n -c\n`},
		{"first and last changed", "a\nb\nc\n", "x\nb\ny\n", `-a\n +x\n =b\n -c\n +y\n`},
		{"everything changed", "a\nb\n", "x\ny\n", `-a\n -b\n +x\n +y\n`},

		// Only whitespace.
		{"indentation changed", "a\n  b\nc\n", "a\n\tb\nc\n", "=a\\n -  b\\n +\tb\\n =c\\n"},
		{"trailing space added", "a\nb\n", "a \nb\n", `-a\n +a \n =b\n`},
		{"blank line added", "a\nb\n", "a\n\nb\n", `=a\n +\n =b\n`},
		{"blank line removed", "a\n\nb\n", "a\nb\n", `=a\n -\n =b\n`},
		{"spaces only to tabs only", "  \n", "\t\n", "-  \\n +\t\\n"},

		// Only line endings: real differences, line by line.
		{"LF to CRLF", "a\nb\n", "a\r\nb\r\n", `-a\n -b\n +a\r\n +b\r\n`},
		{"CRLF to LF", "a\r\nb\r\n", "a\nb\n", `-a\r\n -b\r\n +a\n +b\n`},
		{"one line to CRLF", "a\nb\nc\n", "a\nb\r\nc\n", `=a\n -b\n +b\r\n =c\n`},
		{"CRLF kept", "a\r\nb\r\n", "a\r\nx\r\n", `=a\r\n -b\r\n +x\r\n`},
		{"a lone CR is not a line ending", "a\rb\n", "a\rc\n", `-a\rb\n +a\rc\n`},

		// No final newline, on either side and on both.
		{"final newline removed", "a\nb\n", "a\nb", `=a\n -b\n +b`},
		{"final newline added", "a\nb", "a\nb\n", `=a\n -b +b\n`},
		{"open on both sides, same", "a\nb", "a\nb", `=a\n =b`},
		{"open on both sides, last differs", "a\nb", "a\nc", `=a\n -b +c`},
		{"line added after an open line", "a\nb", "a\nb\nc", `=a\n -b +b\n +c`},
		{"open line removed", "a\nb", "a\n", `=a\n -b`},

		// A moved block: deleted where it was and inserted where it is,
		// or the other way round, whichever is the smaller script.
		{"block moved down", "a\nb\nc\nd\ne\n", "c\nd\ne\na\nb\n", `-a\n -b\n =c\n =d\n =e\n +a\n +b\n`},
		{"block moved up", "a\nb\nc\nd\ne\n", "d\ne\na\nb\nc\n", `+d\n +e\n =a\n =b\n =c\n -d\n -e\n`},
		{"two lines swapped", "a\nb\n", "b\na\n", `-a\n =b\n +a\n`},

		// Repeated lines, where a careless comparison matches the wrong ones.
		{"one of three the same removed", "x\nx\nx\n", "x\nx\n", `=x\n =x\n -x\n`},
		{"one more of the same", "x\nx\n", "x\nx\nx\n", `=x\n =x\n +x\n`},
		{"brace-heavy insert", "{\n}\n{\n}\n", "{\n}\n{\nnew\n}\n{\n}\n", `={\n =}\n ={\n +new\n +}\n +{\n =}\n`},
		{"between repeats", "a\nx\na\nx\na\n", "a\nx\na\ny\na\nx\na\n", `=a\n =x\n =a\n +y\n +a\n =x\n =a\n`},
		{"all the same to none", "x\nx\nx\n", "", `-x\n -x\n -x\n`},
		{"the classic", "a\nb\nc\na\nb\nb\na\n", "c\nb\na\nb\na\nc\n", ""},

		// Very long lines.
		{"long line changed", long + "\nb\n", long + "!\nb\n", "-" + long + `\n +` + long + `!\n =b\n`},
		{"long line kept", "a\n" + long + "\n", "b\n" + long + "\n", `-a\n +b\n =` + long + `\n`},

		// Text that is not ASCII, and text that is not text.
		{"wide characters", "日本\n語\n", "日本\nご\n", `=日本\n -語\n +ご\n`},
		{"bytes that are not UTF-8", "a\xff\nb\n", "a\xfe\nb\n", "-a\xff\\n +a\xfe\\n =b\\n"},
		{"NUL bytes", "a\x00\nb\n", "a\x00\nc\n", "=a\x00\\n -b\\n +c\\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edits := Lines(tt.before, tt.after)
			changed := checkScript(t, tt.before, tt.after, edits)
			if want := smallest(tt.before, tt.after); changed != want {
				t.Errorf("the script changes %d lines; the smallest changes %d\n%s", changed, want, show(edits))
			}
			if tt.want != "" && show(edits) != tt.want {
				t.Errorf("got  %s\nwant %s", show(edits), tt.want)
			}
		})
	}
}

// TestLinesIsSymmetric checks that comparing the other way round gives a
// script of the same size.
func TestLinesIsSymmetric(t *testing.T) {
	s := newSource(11)
	for range 300 {
		before, after := s.pair(30, 4)
		there := checkScript(t, before, after, Lines(before, after))
		back := checkScript(t, after, before, Lines(after, before))
		if there != back {
			t.Fatalf("%d lines change one way and %d the other\nbefore: %q\nafter:  %q", there, back, before, after)
		}
	}
}

// TestScriptsAreRight is the property everything else rests on: for texts of
// every kind, the script applied to the text before gives exactly the text
// after.
func TestScriptsAreRight(t *testing.T) {
	s := newSource(1)
	for i := range times(3000) {
		var before, after string
		switch i % 4 {
		case 0: // a text and an edit of it
			before, after = s.pair(60, 1+s.IntN(8))
		case 1: // two texts made of the same few lines
			before, after = s.text(s.lines(s.IntN(40))), s.text(s.lines(s.IntN(40)))
		case 2: // raw bytes, newlines among them
			before, after = s.noise(s.IntN(200)), s.noise(s.IntN(200))
		case 3: // a long text and a small edit
			before, after = s.pair(400, 3)
		}
		for _, budget := range []int{0, -1, 1, 50} {
			res := Compare(before, after, Options{Budget: budget})
			changed := checkScript(t, before, after, res.Edits)
			checkStats(t, before, after, res)
			if res.Minimal {
				if want := smallest(before, after); changed != want {
					t.Fatalf("budget %d: the script is called minimal and changes %d lines; the smallest changes %d\nbefore: %q\nafter:  %q", budget, changed, want, before, after)
				}
			}
			if budget <= 0 && !res.Minimal {
				t.Fatalf("budget %d: texts this small should get their smallest script\nbefore: %q\nafter:  %q", budget, before, after)
			}
			n, m := strings.Count(before, "\n")+1, strings.Count(after, "\n")+1
			if budget > 0 && res.Work > workBound(n, m, budget) {
				t.Fatalf("budget %d: %d comparisons for %d and %d lines is over the bound of %d", budget, res.Work, n, m, workBound(n, m, budget))
			}
		}
	}
}

// noise makes up n bytes of anything, with enough newlines among them to
// make lines.
func (s source) noise(n int) string {
	const alphabet = "ab \t\n\n\n\r\x00\xff\xc3\xa9"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[s.IntN(len(alphabet))]
	}
	return string(b)
}

// checkStats fails the test unless the counts fit the script and the texts.
func checkStats(t testing.TB, before, after string, res Result) {
	t.Helper()
	deleted, inserted := 0, 0
	for _, e := range res.Edits {
		switch e.Op {
		case Delete:
			deleted += len(e.Lines)
		case Insert:
			inserted += len(e.Lines)
		}
	}
	st := res.Stats
	if st.Added < 0 || st.Removed < 0 || st.Endings < 0 {
		t.Fatalf("negative counts: %+v", st)
	}
	// What the counts leave out are the pairs of lines that differ only in
	// their endings: as many deleted lines as inserted ones.
	if deleted-st.Removed != inserted-st.Added || deleted < st.Removed {
		t.Fatalf("the script deletes %d lines and inserts %d, but the counts are %+v\nbefore: %q\nafter:  %q", deleted, inserted, st, before, after)
	}
	if pairs := deleted - st.Removed; st.Endings > pairs {
		t.Fatalf("%d lines changed only their endings, but only %d lines are paired: %+v", st.Endings, pairs, st)
	}
	if (st == Stats{}) != (before == after) {
		t.Fatalf("the counts are %+v for texts that are the same: %v\nbefore: %q\nafter:  %q", st, before == after, before, after)
	}
	open := func(s string) bool { return s != "" && !strings.HasSuffix(s, "\n") }
	want := 0
	switch {
	case before == "" || after == "":
	case open(before) && !open(after):
		want = 1
	case !open(before) && open(after):
		want = -1
	}
	if st.FinalNewline != want {
		t.Fatalf("FinalNewline is %d, want %d\nbefore: %q\nafter:  %q", st.FinalNewline, want, before, after)
	}
}

// TestSmallestForEverySmallPair compares every pair of texts of up to four
// lines over three different lines, some eight thousand pairs, and checks
// each script against the plain reference.
func TestSmallestForEverySmallPair(t *testing.T) {
	var texts []string
	var build func(prefix string, depth int)
	build = func(prefix string, depth int) {
		texts = append(texts, prefix)
		if depth == 4 {
			return
		}
		for _, line := range []string{"a\n", "b\n", "c\n"} {
			build(prefix+line, depth+1)
		}
	}
	build("", 0)
	for _, before := range texts {
		for _, after := range texts {
			changed := checkScript(t, before, after, Lines(before, after))
			if want := smallest(before, after); changed != want {
				t.Fatalf("%q to %q: the script changes %d lines, the smallest %d", before, after, changed, want)
			}
		}
	}
}

// TestSmallestForRandomPairs checks scripts for texts that share many lines,
// with and without a limit on the work, against the plain reference.
func TestSmallestForRandomPairs(t *testing.T) {
	s := newSource(2)
	for range 2000 {
		// Few different lines, so that most lines have many possible
		// partners and the search has real choices.
		pick := func(n int) string {
			var b strings.Builder
			for range n {
				b.WriteString([]string{"a\n", "b\n", "c\n", "d\n", "\n", "}\n"}[s.IntN(6)])
			}
			return b.String()
		}
		before, after := pick(s.IntN(25)), pick(s.IntN(25))
		for _, budget := range []int{0, -1} {
			res := Compare(before, after, Options{Budget: budget})
			changed := checkScript(t, before, after, res.Edits)
			if want := smallest(before, after); changed != want || !res.Minimal {
				t.Fatalf("budget %d: %q to %q: the script changes %d lines (minimal: %v), the smallest %d", budget, before, after, changed, res.Minimal, want)
			}
		}
	}
}

// TestEngineAgainstReference runs the search itself, without the steps that
// come before it in compare, on sequences of numbers.
func TestEngineAgainstReference(t *testing.T) {
	s := newSource(3)
	for range 5000 {
		ids := 1 + s.IntN(5)
		a := make([]int32, s.IntN(30))
		b := make([]int32, s.IntN(30))
		for i := range a {
			a[i] = int32(s.IntN(ids))
		}
		for j := range b {
			b[j] = int32(s.IntN(ids))
		}
		for _, budget := range []int{-1, 0, 7, 100} {
			del, ins, work, minimal := script(a, b, ids, budget)
			// What is not flagged must pair up, in order, as equal elements.
			var keptA, keptB []int32
			changed := 0
			for i, id := range a {
				if del[i] {
					changed++
				} else {
					keptA = append(keptA, id)
				}
			}
			for j, id := range b {
				if ins[j] {
					changed++
				} else {
					keptB = append(keptB, id)
				}
			}
			if !slices.Equal(keptA, keptB) {
				t.Fatalf("budget %d: %v against %v: what is kept differs: %v and %v", budget, a, b, keptA, keptB)
			}
			want := len(a) + len(b) - 2*commonLength(a, b)
			if minimal && changed != want {
				t.Fatalf("budget %d: %v against %v: called minimal with %d changes; the smallest has %d", budget, a, b, changed, want)
			}
			if budget < 0 && !minimal {
				t.Fatalf("%v against %v: without a budget the script must be minimal", a, b)
			}
			if budget >= 0 && work > workBound(len(a), len(b), budget) {
				t.Fatalf("budget %d: %d comparisons for %d and %d elements", budget, work, len(a), len(b))
			}
		}
	}
}

// commonLength is the length of the longest run of elements a and b have in
// common in order, worked out with a table.
func commonLength(a, b []int32) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
			} else {
				cur[j] = max(prev[j], cur[j-1])
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// TestBudget checks what happens when the search is cut short: the script is
// still right, the work stays under the stated bound, and the result says
// that it may not be the smallest.
func TestBudget(t *testing.T) {
	s := newSource(4)
	// Two long texts over a handful of lines, with nothing lining up: the
	// worst case for the search.
	pick := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteString([]string{"{\n", "}\n", "\n", "x\n", "y\n"}[s.IntN(5)])
		}
		return b.String()
	}
	before, after := pick(4000), pick(4000)
	exact := Compare(before, after, Options{Budget: -1})
	if !exact.Minimal {
		t.Fatal("without a budget the script must be minimal")
	}
	least := checkScript(t, before, after, exact.Edits)

	for _, budget := range []int{1, 1000, 100_000, 1_000_000} {
		res := Compare(before, after, Options{Budget: budget})
		changed := checkScript(t, before, after, res.Edits)
		if res.Work > workBound(4000, 4000, budget) {
			t.Errorf("budget %d: %d comparisons, over the bound of %d", budget, res.Work, workBound(4000, 4000, budget))
		}
		if res.Minimal && changed != least {
			t.Errorf("budget %d: called minimal with %d changed lines; the smallest script has %d", budget, changed, least)
		}
		if changed < least {
			t.Errorf("budget %d: %d changed lines is fewer than the smallest script's %d", budget, changed, least)
		}
		t.Logf("budget %8d: %7d comparisons, %5d changed lines (smallest %d), minimal %v", budget, res.Work, changed, least, res.Minimal)
	}
	if res := Compare(before, after, Options{Budget: 1}); res.Minimal {
		t.Error("a budget of one comparison cannot be enough for these texts, yet the script is called minimal")
	}
	t.Logf("no budget: %d comparisons", exact.Work)
}

// TestRoughStillFindsUnchangedBlocks checks that a search cut short still
// lines up what it can: lines that occur once on each side hold their
// surroundings in place, and the little there is between two of them is
// compared exactly.
func TestRoughStillFindsUnchangedBlocks(t *testing.T) {
	s := newSource(5)
	var a, b strings.Builder
	between := 0 // the smallest scripts for the stretches between such lines, added up
	for i := range 300 {
		// A line that is there once in each text, then lines that repeat.
		head := "func unique" + string(rune('A'+i%26)) + strings.Repeat("x", i) + "() {\n"
		a.WriteString(head)
		b.WriteString(head)
		var sa, sb strings.Builder
		for range 20 {
			sa.WriteString([]string{"\tx()\n", "\ty()\n", "}\n", "\n"}[s.IntN(4)])
			sb.WriteString([]string{"\tx()\n", "\ty()\n", "}\n", "\n"}[s.IntN(4)])
		}
		a.WriteString(sa.String())
		b.WriteString(sb.String())
		between += smallest(sa.String(), sb.String())
	}
	before, after := a.String(), b.String()

	// The search itself, with a budget of one comparison.
	old, new := splitLines(before), splitLines(after)
	na, nb, ids := number(old, new, false)
	del, ins, work, minimal := script(na, nb, ids, 1)
	if minimal {
		t.Fatal("a budget of one comparison should not be enough here")
	}
	changed, kept := 0, 0
	for i, line := range old {
		switch {
		case del[i]:
			changed++
		case strings.HasPrefix(line, "func unique"):
			kept++
		}
	}
	for j := range new {
		if ins[j] {
			changed++
		}
	}
	if kept != 300 {
		t.Errorf("%d of the 300 lines that occur once on each side are kept; all of them should be", kept)
	}
	// Between two such lines there are twenty lines a side: little enough
	// to be compared exactly even with the budget gone.
	if changed != between {
		t.Errorf("the rough script changes %d lines; exact scripts between the lines that occur once change %d", changed, between)
	}
	least := smallest(before, after)
	t.Logf("rough: %d changed lines, smallest: %d, comparisons: %d", changed, least, work)

	// The whole comparison gives a script of the same size.
	res := Compare(before, after, Options{Budget: 1})
	if got := checkScript(t, before, after, res.Edits); got != changed || res.Minimal {
		t.Errorf("Compare changes %d lines (minimal: %v); the search changed %d", got, res.Minimal, changed)
	}
}

// TestGivesUpEarly checks that a search with no hope of finishing within
// its budget does not spend the budget finding that out, and that one with
// every hope is left to finish.
func TestGivesUpEarly(t *testing.T) {
	s := newSource(7)
	pick := func(n int) string {
		var b strings.Builder
		for range n {
			b.WriteString([]string{"{\n", "}\n", "\n", "x\n", "y\n"}[s.IntN(5)])
		}
		return b.String()
	}
	// Two texts with nothing lining up: the search would take some hundred
	// million comparisons.
	before, after := pick(20000), pick(20000)
	const budget = 1 << 22
	res := Compare(before, after, Options{Budget: budget})
	checkScript(t, before, after, res.Edits)
	if res.Minimal {
		t.Error("the script is called minimal")
	}
	// What is spent in all is the rough work, a share of the search's
	// budget, and nowhere near all of it.
	if res.Work > budget {
		t.Errorf("%d comparisons under a budget of %d: the search did not give up early", res.Work, budget)
	}
	t.Logf("hopeless: %d comparisons under a budget of %d", res.Work, budget)

	// The same texts, short enough for the budget: the search is seen
	// through, and its script is the smallest.
	before, after = pick(1200), pick(1200)
	res = Compare(before, after, Options{Budget: budget})
	changed := checkScript(t, before, after, res.Edits)
	if want := smallest(before, after); !res.Minimal || changed != want {
		t.Errorf("minimal %v with %d changed lines; the smallest script changes %d", res.Minimal, changed, want)
	}
	t.Logf("within reach: %d comparisons under a budget of %d", res.Work, budget)
}

func TestSound(t *testing.T) {
	old, new := []string{"a\n", "b\n"}, []string{"a\n", "c\n"}
	tests := []struct {
		name string
		runs []run
		want bool
	}{
		{"right", []run{{Keep, 0, 1, 0, 1}, {Delete, 1, 2, 1, 1}, {Insert, 2, 2, 1, 2}}, true},
		{"keeps lines that differ", []run{{Keep, 0, 2, 0, 2}}, false},
		{"leaves a line out", []run{{Keep, 0, 1, 0, 1}, {Insert, 1, 1, 1, 2}}, false},
		{"has a gap", []run{{Keep, 0, 1, 0, 1}, {Insert, 2, 2, 1, 2}}, false},
		{"keeps more on one side", []run{{Keep, 0, 1, 0, 2}, {Delete, 1, 2, 2, 2}}, false},
		{"a delete that inserts", []run{{Keep, 0, 1, 0, 1}, {Delete, 1, 2, 1, 2}}, false},
		{"runs past the end", []run{{Keep, 0, 1, 0, 1}, {Delete, 1, 3, 1, 1}, {Insert, 3, 3, 1, 2}}, false},
		{"an op that is no op", []run{{Keep, 0, 1, 0, 1}, {endingPair, 1, 2, 1, 2}}, false},
		{"nothing", nil, false},
	}
	for _, tt := range tests {
		if got := sound(tt.runs, old, new); got != tt.want {
			t.Errorf("%s: sound is %v, want %v", tt.name, got, tt.want)
		}
	}
	if !sound(nil, nil, nil) {
		t.Error("no runs are a sound script for two empty texts")
	}
}

func TestRunsOfFlagsThatDoNotPairUp(t *testing.T) {
	// Two lines kept on one side and one on the other: not something
	// script produces, but runsOf must still end, and cover every line.
	runs := runsOf([]bool{false, false}, []bool{false})
	i, j := 0, 0
	for _, r := range runs {
		if r.a0 != i || r.b0 != j {
			t.Fatalf("the runs %v do not follow one another", runs)
		}
		i, j = r.a1, r.b1
	}
	if i != 2 || j != 1 {
		t.Fatalf("the runs %v do not cover two lines and one", runs)
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a\n", []string{"a\n"}},
		{"a\nb", []string{"a\n", "b"}},
		{"a\r\nb\r\n", []string{"a\r\n", "b\r\n"}},
		{"\n\n", []string{"\n", "\n"}},
		{"a\rb\n", []string{"a\rb\n"}},
	}
	for _, tt := range tests {
		got := splitLines(tt.in)
		if !slices.Equal(got, tt.want) {
			t.Errorf("splitLines(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if strings.Join(got, "") != tt.in {
			t.Errorf("splitLines(%q) does not join back", tt.in)
		}
	}
}

func TestChomp(t *testing.T) {
	tests := []struct{ in, text, ending string }{
		{"", "", ""},
		{"a", "a", ""},
		{"a\n", "a", "\n"},
		{"a\r\n", "a", "\r\n"},
		{"a\r", "a\r", ""},
		{"\n", "", "\n"},
		{"\r\n", "", "\r\n"},
		{"a\r\r\n", "a\r", "\r\n"},
	}
	for _, tt := range tests {
		if text, ending := chomp(tt.in); text != tt.text || ending != tt.ending {
			t.Errorf("chomp(%q) = %q, %q, want %q, %q", tt.in, text, ending, tt.text, tt.ending)
		}
	}
}

func TestEditsDoNotShareRoom(t *testing.T) {
	// Appending to one run's lines must not write over the next run's.
	edits := Lines("a\nb\nc\n", "a\nx\nc\n")
	want := show(edits)
	for i := range edits {
		edits[i].Lines = append(edits[i].Lines, "extra\n")
	}
	for i := range edits {
		edits[i].Lines = edits[i].Lines[:len(edits[i].Lines)-1]
	}
	if got := show(edits); got != want {
		t.Errorf("after appending to each run the script reads %s, want %s", got, want)
	}
}

func TestOpString(t *testing.T) {
	for op, want := range map[Op]string{Keep: "keep", Delete: "delete", Insert: "insert", endingPair: "unknown"} {
		if got := op.String(); got != want {
			t.Errorf("Op(%d).String() = %q, want %q", op, got, want)
		}
	}
}

func TestStats(t *testing.T) {
	tests := []struct {
		name          string
		before, after string
		want          Stats
		text          string
	}{
		{"the same", "a\nb\n", "a\nb\n", Stats{}, "no changes"},
		{"both empty", "", "", Stats{}, "no changes"},
		{"a line changed", "a\nb\n", "a\nc\n", Stats{Added: 1, Removed: 1}, "+1 -1"},
		{"lines added", "a\n", "a\nb\nc\n", Stats{Added: 2}, "+2 -0"},
		{"a new file", "", "a\nb\n", Stats{Added: 2}, "+2 -0"},
		{"a new file left open", "", "a\nb", Stats{Added: 2}, "+2 -0"},
		{"emptied", "a\nb\n", "", Stats{Removed: 2}, "+0 -2"},
		{"LF to CRLF", "a\nb\n", "a\r\nb\r\n", Stats{Endings: 2}, "+0 -0, line endings changed"},
		{"CRLF to LF and a line changed", "a\r\nb\r\nc\r\n", "a\nx\nc\n", Stats{Added: 1, Removed: 1, Endings: 2}, "+1 -1, line endings changed"},
		{"one line to CRLF", "a\nb\nc\n", "a\nb\r\nc\n", Stats{Endings: 1}, "+0 -0, line endings changed"},
		{"final newline removed", "a\nb\n", "a\nb", Stats{FinalNewline: -1}, "+0 -0, final newline removed"},
		{"final newline added", "a\nb", "a\nb\n", Stats{FinalNewline: 1}, "+0 -0, final newline added"},
		{"final newline added, CRLF", "a\r\nb", "a\r\nb\r\n", Stats{FinalNewline: 1}, "+0 -0, final newline added"},
		{"open on both sides", "a\nb", "a\nc", Stats{Added: 1, Removed: 1}, "+1 -1"},
		{"a line after an open line", "a\nb", "a\nb\nc", Stats{Added: 1}, "+1 -0"},
		{"a line after an open line, then closed", "a\nb", "a\nb\nc\n", Stats{Added: 1, FinalNewline: 1}, "+1 -0, final newline added"},
		{"last line changed and left open", "a\nb\n", "a\nc", Stats{Added: 1, Removed: 1, FinalNewline: -1}, "+1 -1, final newline removed"},
		{"everything at once", "a\r\nb\r\nc\r\n", "a\nb\nd", Stats{Added: 1, Removed: 1, Endings: 2, FinalNewline: -1}, "+1 -1, line endings changed, final newline removed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := Compare(tt.before, tt.after, Options{})
			if res.Stats != tt.want {
				t.Errorf("got %+v, want %+v", res.Stats, tt.want)
			}
			if got := res.Stats.String(); got != tt.text {
				t.Errorf("String() = %q, want %q", got, tt.text)
			}
			checkScript(t, tt.before, tt.after, res.Edits)
			checkStats(t, tt.before, tt.after, res)
		})
	}
}

// TestFineScript checks the script that pairs up lines differing only in
// their endings: it covers the same lines as the plain script, every pair
// is two lines with the same text and another ending, and nothing else is
// called a pair.
func TestFineScript(t *testing.T) {
	s := newSource(6)
	for i := range 2000 {
		var before, after string
		if i%2 == 0 {
			before, after = s.pair(40, 1+s.IntN(5))
		} else {
			// The same lines with other endings, and an edit.
			lines := s.lines(s.IntN(30))
			before, after = s.text(lines), s.text(s.edited(lines, s.IntN(3)))
		}
		for _, budget := range []int{-1, 1} {
			c := compare(before, after, budget)
			i, j := 0, 0
			for _, r := range c.fine() {
				if r.a0 != i || r.b0 != j {
					t.Fatalf("the fine script has a gap at %+v\nbefore: %q\nafter:  %q", r, before, after)
				}
				i, j = r.a1, r.b1
				switch r.op {
				case Keep:
					for k := range r.a1 - r.a0 {
						if c.old[r.a0+k] != c.new[r.b0+k] {
							t.Fatalf("a kept pair differs: %q and %q", c.old[r.a0+k], c.new[r.b0+k])
						}
					}
				case endingPair, newlinePair:
					if r.a1-r.a0 != r.b1-r.b0 || r.a1 == r.a0 {
						t.Fatalf("a pair run of uneven sides: %+v", r)
					}
					for k := range r.a1 - r.a0 {
						x, y := c.old[r.a0+k], c.new[r.b0+k]
						tx, ex := chomp(x)
						ty, ey := chomp(y)
						if tx != ty || ex == ey {
							t.Fatalf("%q and %q are called a pair", x, y)
						}
						if (r.op == newlinePair) != (ex == "" || ey == "") {
							t.Fatalf("%q and %q are the wrong kind of pair (%v)", x, y, r.op)
						}
					}
				case Delete:
					if r.b0 != r.b1 || r.a0 == r.a1 {
						t.Fatalf("a bad Delete: %+v", r)
					}
				case Insert:
					if r.a0 != r.a1 || r.b0 == r.b1 {
						t.Fatalf("a bad Insert: %+v", r)
					}
				}
			}
			if i != len(c.old) || j != len(c.new) {
				t.Fatalf("the fine script ends at %d, %d of %d, %d lines", i, j, len(c.old), len(c.new))
			}
			// Shown either way, the script must still be one.
			for _, fold := range []bool{false, true} {
				if runs := c.display(fold); !soundByText(runs, c.old, c.new) {
					t.Fatalf("the shown script (fold %v) is not sound: %+v\nbefore: %q\nafter:  %q", fold, runs, before, after)
				}
			}
		}
	}
}

// soundByText is sound for a shown script, whose kept lines may differ in
// their endings.
func soundByText(runs []run, old, new []string) bool {
	strip := func(lines []string) []string {
		out := make([]string, len(lines))
		for i, line := range lines {
			out[i], _ = chomp(line)
		}
		return out
	}
	if len(old) == 0 && len(new) == 0 {
		return len(runs) == 0
	}
	return sound(runs, strip(old), strip(new))
}

package diff

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// A bigCase is two large texts to compare.
type bigCase struct {
	name          string
	before, after string
}

// bigLines is the size of the texts the benchmarks compare.
const bigLines = 50_000

// bigCases makes the pairs of 50,000-line texts the benchmarks and the test
// of large texts use. They are the same on every run.
//
// "code" texts read like source code: most lines are unlike every other,
// and one line in four is a brace or an empty line, of which there are
// thousands. "few" texts are made of two hundred different lines only, so
// that every line has hundreds of equals: far harder to line up.
func bigCases() []bigCase {
	s := newSource(71)
	code := func(i int) string {
		switch s.IntN(8) {
		case 0:
			return ""
		case 1:
			return "}"
		}
		return "\tvalue[" + strconv.Itoa(i) + "] = compute(" + strconv.Itoa(s.IntN(1000)) + ")"
	}
	vocabulary := make([]string, 200)
	for i := range vocabulary {
		vocabulary[i] = "\tcall" + strconv.Itoa(i) + "(x)"
	}
	few := func(int) string { return vocabulary[s.IntN(len(vocabulary))] }

	// base makes a text; edited changes about a part in rate of its lines:
	// a third of those replaced, a third deleted, a third given a new
	// line after them.
	base := func(line func(int) string) []string {
		lines := make([]string, bigLines)
		for i := range lines {
			lines[i] = line(i)
		}
		return lines
	}
	edited := func(lines []string, rate int, line func(int) string) []string {
		out := make([]string, 0, len(lines)+len(lines)/rate)
		for i, l := range lines {
			if s.IntN(rate) != 0 {
				out = append(out, l)
				continue
			}
			switch s.IntN(3) {
			case 0:
				out = append(out, line(bigLines+i)+" // changed")
			case 1:
			case 2:
				out = append(out, l, line(bigLines+i))
			}
		}
		return out
	}
	join := func(lines []string, ending string) string {
		return strings.Join(lines, ending) + ending
	}

	codeLines, fewLines := base(code), base(few)
	return []bigCase{
		{"code, 1% changed", join(codeLines, "\n"), join(edited(codeLines, 100, code), "\n")},
		{"code, 50% changed", join(codeLines, "\n"), join(edited(codeLines, 2, code), "\n")},
		{"few lines, 1% changed", join(fewLines, "\n"), join(edited(fewLines, 100, few), "\n")},
		{"few lines, 50% changed", join(fewLines, "\n"), join(edited(fewLines, 2, few), "\n")},
		{"code, line endings and 1% changed", join(codeLines, "\r\n"), join(edited(codeLines, 100, code), "\n")},
		{"few lines, nothing lines up", join(fewLines, "\n"), join(base(few), "\n")},
	}
}

// BenchmarkCompare compares two texts of 50,000 lines.
//
// Measured on the development machine, an Intel Xeon Platinum 8168 at
// 2.7 GHz that was busy with other work at the time (Go 1.27, Linux):
//
//	code, 1% changed                     10 to  20 ms    0.4 million comparisons
//	code, 50% changed                   155 to 290 ms   18.3 million; the budget runs out
//	few lines, 1% changed                 7 to  15 ms    0.6 million
//	few lines, 50% changed               90 to 115 ms    3.9 million; given up early
//	code, line endings and 1% changed    16 to  30 ms    0.3 million
//	few lines, nothing lines up          80 to 105 ms    6.4 million; given up early
//
// and each allocates between 8 and 45 MB while it runs. Where the budget
// runs out or the search is given up, the script is a rough one: within one
// part in a hundred of the smallest in the two cases where the smallest
// could be worked out for comparison.
//
// So two texts of 50,000 lines compare in under a third of a second however
// they differ, as measured. The bound that is proved and not measured is on
// the work: at most DefaultBudget + 1000 line comparisons for every line of
// the two texts, 117 million here, which TestLargeTexts checks. At the 8 to
// 15 nanoseconds a comparison took on that machine, that is under two
// seconds.
func BenchmarkCompare(b *testing.B) {
	for _, c := range bigCases() {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(c.before) + len(c.after)))
			var res Result
			for b.Loop() {
				res = Compare(c.before, c.after, Options{})
			}
			b.ReportMetric(float64(res.Work), "comparisons/op")
		})
	}
}

// BenchmarkUnified writes the unified form for two texts of 50,000 lines.
func BenchmarkUnified(b *testing.B) {
	for _, c := range bigCases()[:2] {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Unified(c.before, c.after, UnifiedOptions{})
			}
		})
	}
}

// BenchmarkRender draws the difference between two texts of 50,000 lines,
// all of it and with a cap of 200 lines.
func BenchmarkRender(b *testing.B) {
	c := bigCases()[0]
	d := core.Display{Path: "big.go", Before: c.before, After: c.after}
	for _, limit := range []int{0, 200} {
		b.Run("cap "+strconv.Itoa(limit), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				Render(d, RenderOptions{Width: 100, Profile: text.TrueColor, MaxLines: limit})
			}
		})
	}
}

// BenchmarkRenderSmall draws an edit of the usual size: a few lines changed
// in a file of a few hundred.
func BenchmarkRenderSmall(b *testing.B) {
	s := newSource(72)
	lines := s.lines(300)
	before := strings.Join(lines, "\n") + "\n"
	after := strings.Join(s.edited(lines, 3), "\n") + "\n"
	d := core.Display{Path: "small.go", Before: before, After: after}
	b.ReportAllocs()
	for b.Loop() {
		Render(d, RenderOptions{Width: 100, Profile: text.TrueColor})
	}
}

// BenchmarkSpans finds what changed inside a pair of lines.
func BenchmarkSpans(b *testing.B) {
	old := "\tresult, err := client.Fetch(ctx, request.WithTimeout(30*time.Second), options...)"
	new := "\tresult, err := client.FetchAll(ctx, request.WithTimeout(60*time.Second), options...)"
	b.ReportAllocs()
	for b.Loop() {
		Spans(old, new)
	}
}

// TestLargeTexts compares texts of 50,000 lines in every way the benchmarks
// do and checks what must hold for each: the script is right, the work is
// within the stated bound, and the clock shows nothing absurd.
func TestLargeTexts(t *testing.T) {
	if testing.Short() {
		t.Skip("large texts take a few seconds")
	}
	cases := bigCases()
	if underRace {
		// One easy case, one where the search is given up, and the one
		// with the line endings.
		cases = []bigCase{cases[0], cases[3], cases[4]}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			res := Compare(c.before, c.after, Options{})
			took := time.Since(start)
			changed := checkScript(t, c.before, c.after, res.Edits)
			checkStats(t, c.before, c.after, res)
			n, m := strings.Count(c.before, "\n"), strings.Count(c.after, "\n")
			if bound := workBound(n, m, DefaultBudget); res.Work > bound {
				t.Errorf("%d comparisons for %d and %d lines; the bound is %d", res.Work, n, m, bound)
			}
			// A minute would mean something is badly wrong, on any
			// machine and under the race detector too.
			if took > time.Minute {
				t.Errorf("the comparison took %v", took)
			}
			t.Logf("%d and %d lines: %d changed, %d comparisons, minimal %v, %v", n, m, changed, res.Work, res.Minimal, took.Round(time.Millisecond))

			// The unified form of the same change, read back.
			patch := Unified(c.before, c.after, UnifiedOptions{})
			if got, err := applyUnified(c.before, patch); err != nil || got != c.after {
				t.Errorf("the unified form does not read back: %v", err)
			}
			// Drawn with a cap, it stays small whatever the size of the
			// change.
			rows := Render(core.Display{Path: "big", Before: c.before, After: c.after}, RenderOptions{Width: 100, MaxLines: 50})
			if len(rows) > 60 {
				t.Errorf("%d rows under a cap of 50", len(rows))
			}
		})
	}
}

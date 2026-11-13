package diff

import (
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// bracket writes a line with its spans in square brackets, so that a test
// can say what it expects in one readable string.
func bracket(s string, spans []Span) string {
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		b.WriteString(s[at:sp.Start])
		b.WriteByte('[')
		b.WriteString(s[sp.Start:sp.End])
		b.WriteByte(']')
		at = sp.End
	}
	b.WriteString(s[at:])
	return b.String()
}

// without returns a line with its spans taken out.
func without(s string, spans []Span) string {
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		b.WriteString(s[at:sp.Start])
		at = sp.End
	}
	b.WriteString(s[at:])
	return b.String()
}

func TestSpans(t *testing.T) {
	tests := []struct {
		name     string
		old, new string
		wantOld  string // the old line with its spans in brackets; "" when the lines are too different
		wantNew  string
	}{
		// Whole words.
		{"a word replaced", "return nil", "return err", "return [nil]", "return [err]"},
		{"a word added", "foo(a, b)", "foo(a, b, c)", "foo(a, b)", "foo(a, b[, c])"},
		{"a word removed", "x := compute(a, b)", "x := compute(a)", "x := compute(a[, b])", "x := compute(a)"},
		{"two words replaced", "if a == b {", "if x == y {", "if [a] == [b] {", "if [x] == [y] {"},
		{"a name and an argument", "func oldName(a int) int {", "func newName(a, b int) int {", "func [old]Name(a int) int {", "func [new]Name(a[, b] int) int {"},
		{"a comment added", "x = 1", "x = 1 // one", "x = 1", "x = 1[ // one]"},
		{"an import", `import "fmt"`, `import "os"`, `import "[fmt]"`, `import "[os]"`},
		{"punctuation", "a.b(c)", "a->b(c)", "a[.]b(c)", "a[->]b(c)"},

		// Inside a word, where the two forms share most of their letters.
		{"a letter dropped", "colour := 1", "color := 1", "colo[u]r := 1", "color := 1"},
		{"a letter added at the end", "getName()", "getNames()", "getName()", "getName[s]()"},
		{"a letter changed", "cat", "cut", "c[a]t", "c[u]t"},
		{"a digit changed", "port = 8080", "port = 8081", "port = 808[0]", "port = 808[1]"},
		{"a prefix added", "name", "userName", "", ""},
		{"words with little in common stay whole", "x := alpha + 1", "x := omega + 1", "x := [alpha] + 1", "x := [omega] + 1"},
		{"camel case grows", "handleRequest(w, r)", "handleRequestBody(w, r)", "handleRequest(w, r)", "handleRequest[Body](w, r)"},

		// Blanks.
		{"indentation deeper", "  x()", "    x()", "  x()", "  [  ]x()"},
		{"indentation by tab", "    x()", "\tx()", "[    ]x()", "[\t]x()"},
		{"space at the end", "x()", "x()  ", "x()", "x()[  ]"},
		{"a space inside", "f(a,b)", "f(a, b)", "f(a,b)", "f(a,[ ]b)"},
		{"only blanks, both", "    ", "\t", "[    ]", "[\t]"},
		{"an empty line gets text", "", "x", "", ""},

		// Scripts without spaces between words, and wide characters.
		{"one character of Japanese", "日本語のテキスト", "日本語のテスト", "日本語のテ[キ]スト", "日本語のテスト"},
		{"Chinese", "我喜欢苹果", "我喜欢香蕉", "我喜欢[苹果]", "我喜欢[香蕉]"},
		{"Chinese, one of five", "我喜欢吃苹果", "我喜欢买苹果", "我喜欢[吃]苹果", "我喜欢[买]苹果"},
		{"an emoji swapped", "status: 🎉 done", "status: 🚀 done", "status: [🎉] done", "status: [🚀] done"},
		{"an emoji with a skin tone", "wave 👋🏽 hi", "wave 👋 hi", "wave [👋🏽] hi", "wave [👋] hi"},
		{"an accent", "café au lait", "cafe au lait", "caf[é] au lait", "caf[e] au lait"},
		{"a combining accent", "cafe\u0301 au lait", "cafe au lait", "caf[e\u0301] au lait", "caf[e] au lait"},

		// Too different to mark.
		{"nothing in common", "foo(a)", "bar(b)", "", ""},
		{"only an operator in common", "a = 1", "b = 2", "", ""},
		{"a brace kept, the rest new", "}", "if (foo) { bar(); }", "", ""},
		{"a short line and a long one", "x", "x = some + long * expression", "", ""},

		// The same.
		{"the same line", "x := 1", "x := 1", "x := 1", "x := 1"},
		{"both empty", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldSpans, newSpans, ok := Spans(tt.old, tt.new)
			checkSpans(t, tt.old, tt.new, oldSpans, newSpans, ok)
			gotOld, gotNew := "", ""
			if ok {
				gotOld, gotNew = bracket(tt.old, oldSpans), bracket(tt.new, newSpans)
			}
			if gotOld != tt.wantOld || gotNew != tt.wantNew {
				t.Errorf("got  %q and %q (ok %v)\nwant %q and %q", gotOld, gotNew, ok, tt.wantOld, tt.wantNew)
			}
		})
	}
}

// checkSpans fails the test unless the spans are what Spans promises: in
// order, not empty, not touching, cut between grapheme clusters, and
// leaving the same string in both lines.
func checkSpans(t testing.TB, old, new string, oldSpans, newSpans []Span, ok bool) {
	t.Helper()
	if !ok {
		if oldSpans != nil || newSpans != nil {
			t.Fatalf("spans are returned for lines called too different: %v %v", oldSpans, newSpans)
		}
		return
	}
	for side, line := range map[string]string{"old": old, "new": new} {
		spans := oldSpans
		if side == "new" {
			spans = newSpans
		}
		// Where grapheme clusters begin and end in the line.
		edges := map[int]bool{0: true}
		at := 0
		for cluster := range text.Graphemes(line) {
			at += len(cluster)
			edges[at] = true
		}
		last := -1
		for _, sp := range spans {
			if sp.Start >= sp.End || sp.Start <= last || sp.End > len(line) {
				t.Fatalf("the %s spans %v are empty, out of order, touching or out of the line %q", side, spans, line)
			}
			if !edges[sp.Start] || !edges[sp.End] {
				t.Fatalf("the %s span %v cuts a grapheme cluster of %q in two", side, sp, line)
			}
			last = sp.End
		}
	}
	if a, b := without(old, oldSpans), without(new, newSpans); a != b {
		t.Fatalf("the spans do not account for the whole difference:\nold %q without %v is %q\nnew %q without %v is %q", old, oldSpans, a, new, newSpans, b)
	}
}

// TestSpansAccountForTheDifference checks, on lines of every kind, that what
// the spans leave is the same in both lines.
func TestSpansAccountForTheDifference(t *testing.T) {
	s := newSource(31)
	pieces := []string{
		"foo", "bar", "x", "getName", "getNames", " ", "  ", "\t", "(", ")", ", ", " = ", "1", "42", "\"str\"",
		"日本", "語", "🎉", "👋🏽", "é", "e\u0301", "🇯🇵", "👨\u200d👩\u200d👧", "_", "-", "::", "a\u200db", "\x1b[31m", "\xff",
	}
	line := func() string {
		var b strings.Builder
		for range s.IntN(12) {
			b.WriteString(pieces[s.IntN(len(pieces))])
		}
		return b.String()
	}
	marked := 0
	for i := range 20000 {
		old := line()
		new := line()
		if i%2 == 0 {
			// A small change to the same line: the case marks are for.
			runes := []rune(old)
			if len(runes) > 0 {
				at := s.IntN(len(runes))
				new = string(runes[:at]) + pieces[s.IntN(len(pieces))] + string(runes[min(at+s.IntN(3), len(runes)):])
			}
		}
		oldSpans, newSpans, ok := Spans(old, new)
		checkSpans(t, old, new, oldSpans, newSpans, ok)
		if ok && old != new {
			marked++
			if len(oldSpans) == 0 && len(newSpans) == 0 {
				t.Fatalf("no spans for two lines that differ: %q and %q", old, new)
			}
		}
		// The other way round. (Whether two lines right at the edge of
		// being alike enough are marked can differ with the direction,
		// since the words kept need not be the same ones.)
		backOld, backNew, backOK := Spans(new, old)
		checkSpans(t, new, old, backOld, backNew, backOK)
	}
	if marked < 2000 {
		t.Errorf("only %d of 20000 pairs of lines were marked: the test is not testing much", marked)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"foo", []string{"foo"}},
		{"foo bar", []string{"foo", " ", "bar"}},
		{"foo  \tbar", []string{"foo", "  \t", "bar"}},
		{"get_user_name2()", []string{"get_user_name2", "(", ")"}},
		{"a+=b;", []string{"a", "+", "=", "b", ";"}},
		{"x := 1.5", []string{"x", " ", ":", "=", " ", "1", ".", "5"}},
		{"日本語", []string{"日", "本", "語"}},
		{"café naïve", []string{"café", " ", "naïve"}},
		{"e\u0301x", []string{"e\u0301x"}},
		{"hi🎉there", []string{"hi", "🎉", "there"}},
		{"a\u00a0b", []string{"a", "\u00a0", "b"}},
		{"Привет мир", []string{"Привет", " ", "мир"}},
		{"한국어", []string{"한", "국", "어"}},
	}
	for _, tt := range tests {
		var got []string
		for _, tok := range tokenize(tt.in) {
			got = append(got, tt.in[tok.start:tok.end])
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("tokenize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPairLines(t *testing.T) {
	tests := []struct {
		name       string
		dels, inss []string
		want       []int
	}{
		{"one for one", []string{"x := 1"}, []string{"x := 2"}, []int{0}},
		{"nothing alike", []string{"foo()"}, []string{"return bar"}, []int{-1}},
		{
			"a line inserted before the changed one",
			[]string{"total := count(items)"},
			[]string{"// count them", "total := count(items, true)"},
			[]int{1},
		},
		{
			"a line deleted before the changed one",
			[]string{"log.Print(x)", "total := count(items)"},
			[]string{"total := count(items, true)"},
			[]int{-1, 0},
		},
		{
			"not first with first",
			[]string{"a := load(path)", "b := parse(a)", "c := check(b)"},
			[]string{"b := parse(a, opts)", "c := check(b, opts)", "d := write(c)"},
			[]int{-1, 0, 1},
		},
		{
			"the order is kept",
			[]string{"first := one()", "second := two()"},
			[]string{"second := two(x)", "first := one(x)"},
			[]int{-1, 0},
		},
		{
			"blank lines pair with blank lines",
			[]string{"", "x()"},
			[]string{"   ", "y()"},
			[]int{0, 1},
		},
		{"no inserted lines", []string{"a", "b"}, nil, []int{-1, -1}},
		{"no deleted lines", nil, []string{"a"}, []int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pairLines(tt.dels, tt.inss); !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPairLinesKeepsOrder checks both ways of pairing, the table for small
// places and the look ahead for large ones: partners rise, no inserted line
// has two partners, and the lines of a pair are alike.
func TestPairLinesKeepsOrder(t *testing.T) {
	s := newSource(32)
	for i := range 300 {
		n, m := s.IntN(30), s.IntN(30)
		if i%10 == 0 {
			n, m = 150+s.IntN(100), 150+s.IntN(100) // too large for the table
		}
		dels := s.lines(n)
		inss := s.edited(dels, s.IntN(6))
		for len(inss) < m {
			inss = append(inss, s.line())
		}
		partner := pairLines(dels, inss)
		if len(partner) != len(dels) {
			t.Fatalf("%d partners for %d deleted lines", len(partner), len(dels))
		}
		last := -1
		for d, p := range partner {
			if p < 0 {
				continue
			}
			if p <= last || p >= len(inss) {
				t.Fatalf("partners do not rise: %v", partner)
			}
			last = p
			if l := likeness(bagOf(dels[d]), bagOf(inss[p])); l < pairAlike {
				t.Fatalf("%q and %q are paired with a likeness of %d", dels[d], inss[p], l)
			}
		}
	}
}

// TestPairLinesFindsTheBest checks the table against trying every pairing.
func TestPairLinesFindsTheBest(t *testing.T) {
	s := newSource(33)
	words := []string{"alpha", "beta", "gamma", "delta", "x", "y"}
	line := func() string {
		var parts []string
		for range 1 + s.IntN(4) {
			parts = append(parts, words[s.IntN(len(words))])
		}
		return strings.Join(parts, " ")
	}
	for range 500 {
		dels := make([]string, s.IntN(6))
		inss := make([]string, s.IntN(6))
		for i := range dels {
			dels[i] = line()
		}
		for j := range inss {
			inss[j] = line()
		}
		sum := 0
		for d, p := range pairLines(dels, inss) {
			if p >= 0 {
				sum += likeness(bagOf(dels[d]), bagOf(inss[p]))
			}
		}
		// Every way of pairing in order, by trying each line with each.
		var best func(i, j int) int
		best = func(i, j int) int {
			if i == len(dels) || j == len(inss) {
				return 0
			}
			v := max(best(i+1, j), best(i, j+1))
			if l := likeness(bagOf(dels[i]), bagOf(inss[j])); l >= pairAlike {
				v = max(v, l+best(i+1, j+1))
			}
			return v
		}
		if want := best(0, 0); sum != want {
			t.Fatalf("the pairing of %q with %q is alike %d in sum; the best is %d", dels, inss, sum, want)
		}
	}
}

func TestLikeness(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"x := 1", "x := 1", 1000},
		{"", "", 1000},
		{"   ", "\t", 1000},
		{"", "x", 0},
		{"foo", "bar", 0},
		{"return x", "return y", 857}, // 12 of 14 bytes
		{"a b", "b a", 1000},          // a bag has no order
	}
	for _, tt := range tests {
		if got := likeness(bagOf(tt.a), bagOf(tt.b)); got != tt.want {
			t.Errorf("likeness(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
		if got, back := likeness(bagOf(tt.a), bagOf(tt.b)), likeness(bagOf(tt.b), bagOf(tt.a)); got != back {
			t.Errorf("likeness(%q, %q) is %d one way and %d the other", tt.a, tt.b, got, back)
		}
	}
}

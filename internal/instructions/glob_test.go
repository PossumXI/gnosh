package instructions

import (
	"strings"
	"testing"
)

// matches compiles one pattern and matches it against rel.
func matches(t *testing.T, pat, rel string) bool {
	t.Helper()
	pats, bad := compilePatterns([]string{pat})
	if len(bad) > 0 {
		t.Fatalf("pattern %q: %s", pat, bad[0].why)
	}
	for _, p := range pats {
		if p.match(rel) {
			return true
		}
	}
	return false
}

func TestPatterns(t *testing.T) {
	for _, c := range []struct {
		pat string
		yes []string
		no  []string
	}{
		// The rows of the table in the spec.
		{"**/*.ts", []string{"a.ts", "src/a.ts", "src/x/y/a.ts"}, []string{"a.tsx", "ts", "src/a.js"}},
		{"src/**/*", []string{"src/a", "src/x/y/z.go"}, []string{"lib/a", "src", "x/src/a"}},
		{"*.md", []string{"README.md", "docs/a/b.md"}, []string{"docs/a/b.txt", "md"}},
		{"/*.md", []string{"README.md"}, []string{"docs/a.md"}},
		{"src/components/*.tsx", []string{"src/components/Button.tsx"}, []string{"src/components/x/Button.tsx", "other/src/components/Button.tsx", "src/components/Button.ts"}},
		{"*.{ts,tsx}", []string{"a.ts", "b/c.tsx"}, []string{"a.js", "a.t"}},
		{"lib/**/*.{ts,tsx}", []string{"lib/a.ts", "lib/x/y/b.tsx"}, []string{"lib/a.js", "src/a.ts"}},

		// A pattern that matches a directory matches what is under it.
		{"src/", []string{"src/a/b.go", "x/src/a.go"}, []string{"srcs/a", "a/srcs"}},
		{"src/api", []string{"src/api/x.go", "src/api/v2/y.go", "src/api"}, []string{"src/apix/y", "x/src/api/z"}},
		{"docs/**", []string{"docs/x", "docs/a/b"}, []string{"docs", "x/docs/y"}},
		{"src/*", []string{"src/a", "src/a/b/c"}, []string{"src", "lib/a"}},

		// Stars.
		{"**", []string{"a", "a/b/c"}, nil},
		{"*", []string{"a", "a/b/c"}, nil},
		{"a/**/b", []string{"a/b", "a/x/b", "a/x/y/b", "a/b/c"}, []string{"b", "a/x", "x/a/b"}},
		{"**/b", []string{"b", "a/b", "a/x/b", "b/c"}, []string{"a/bb"}},
		{"a/**/**/b", []string{"a/b", "a/x/y/b"}, []string{"a/x"}},
		{"a*b*c", []string{"abc", "aXbXc", "aXbXbXc", "x/abbbc"}, []string{"aXbX", "ab", "cba"}},
		{"a***b", []string{"ab", "aXb"}, []string{"a/b"}},
		{"*.test.*", []string{"a.test.js", "x/b.test.ts"}, []string{"a.test", "test.js"}},

		// One character.
		{"a?c", []string{"abc", "aéc", "x/a.c"}, []string{"ac", "abbc", "a/c"}},
		{"[abc].go", []string{"a.go", "c.go"}, []string{"d.go", "ab.go"}},
		{"[a-c]x", []string{"ax", "bx", "cx"}, []string{"dx", "x"}},
		{"[!a-c]x", []string{"dx", "-x"}, []string{"ax", "x"}},
		{"[^a]x", []string{"bx"}, []string{"ax"}},
		{"[]a]x", []string{"]x", "ax"}, []string{"bx"}},
		{"[a-]x", []string{"ax", "-x"}, []string{"bx"}},
		{`[\]\\]x`, []string{"]x", `\x`}, []string{"ax"}},
		{"[α-ω]x", []string{"λx"}, []string{"ax"}},

		// Plain characters.
		{`\[x\].md`, []string{"[x].md"}, []string{"x.md"}},
		{`a\*b`, []string{"a*b"}, []string{"aXb"}},
		{`\{a,b\}`, []string{"{a,b}"}, []string{"a", "b"}},
		{"{a}", []string{"{a}"}, []string{"a"}},
		{"{a", []string{"{a"}, []string{"a"}},
		{"a{b,c{d,e}}f", []string{"abf", "acdf", "acef"}, []string{"acf", "af"}},
		{"x{,y}", []string{"x", "xy"}, []string{"xyy"}},
		{"*.MD", []string{"a.MD"}, []string{"a.md"}},
		{"./src/*.go", []string{"src/a.go"}, []string{"x/src/a.go"}},
		{".*", []string{".env", "a/.git"}, []string{"env"}},
		{"*", []string{".hidden"}, nil},
		{"/a//b/", []string{"a/b", "a/b/c"}, []string{"a/x/b"}},
	} {
		for _, rel := range c.yes {
			if !matches(t, c.pat, rel) {
				t.Errorf("%q should match %q", c.pat, rel)
			}
		}
		for _, rel := range c.no {
			if matches(t, c.pat, rel) {
				t.Errorf("%q should not match %q", c.pat, rel)
			}
		}
	}
}

func TestPatternsThatCannotBeRead(t *testing.T) {
	for pat, says := range map[string]string{
		"[":                                  "no ] closes",
		"src/[ab":                            "no ] closes",
		"[]":                                 "no ] closes",
		"[!]":                                "no ] closes",
		`a\`:                                 "backslash",
		"[[:alpha:]]":                        "named character classes",
		"[z-a]":                              "backwards",
		"":                                   "empty",
		"/":                                  "empty",
		strings.Repeat("a", maxPatternLen+1): "longer than",
	} {
		pats, bad := compilePatterns([]string{pat, "ok/*.go"})
		if len(bad) != 1 || !strings.Contains(bad[0].why, says) {
			t.Errorf("%q: problems %+v, want one that mentions %q", pat, bad, says)
		}
		// The bad pattern matches nothing; its neighbour still works.
		if len(pats) != 1 || !pats[0].match("ok/x.go") {
			t.Errorf("%q: the good pattern beside it was lost", pat)
		}
	}
}

func TestPatternBudget(t *testing.T) {
	// 2 to the 9th is 512 patterns, within the budget of one rule's list;
	// the next 2 to the 9th would pass it.
	nine := strings.Repeat("{a,b}", 9)
	pats, bad := compilePatterns([]string{nine})
	if len(pats) != 512 || len(bad) != 0 {
		t.Fatalf("%d patterns, %d problems", len(pats), len(bad))
	}
	pats, bad = compilePatterns([]string{"first", nine, nine + "x", "last"})
	if len(pats) != 513 {
		t.Errorf("%d patterns kept, want the 513 before the budget ran out", len(pats))
	}
	if len(bad) != 2 || !strings.Contains(bad[0].why, "more than 1000") || bad[1].src != "last" {
		t.Errorf("problems: %+v", bad)
	}

	// A pattern that would come to millions is given up at once.
	huge := strings.Repeat("{a,b,c,d}", 40)
	pats, bad = compilePatterns([]string{huge})
	if len(pats) != 0 || len(bad) != 1 {
		t.Errorf("%d patterns, %d problems", len(pats), len(bad))
	}
}

func TestPatternMatchingDoesNotBlowUp(t *testing.T) {
	// Many "**" against a long path that almost matches: without a memo
	// this would take longer than the test run.
	pat := strings.Repeat("**/a/", 30) + "b"
	rel := strings.Repeat("a/", 300) + "c"
	if matches(t, pat, rel) {
		t.Error("matched")
	}
	// Many "*" within one name.
	pat = strings.Repeat("*a", 40) + "b"
	rel = strings.Repeat("a", 2000)
	if matches(t, pat, rel) {
		t.Error("matched")
	}
}

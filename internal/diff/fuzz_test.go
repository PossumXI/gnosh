package diff

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// fuzzSeeds are pairs of texts that have a rule of their own somewhere in
// the package, for the fuzzers to start from.
func fuzzSeeds() [][2]string {
	return [][2]string{
		{"", ""},
		{"", "a\n"},
		{"a\n", ""},
		{"a", "a\n"},
		{"a\nb\nc\n", "a\nx\nc\n"},
		{"a\r\nb\r\n", "a\nb\n"},
		{"a\nb\r\nc", "a\r\nb\nc\n"},
		{"x\nx\nx\n", "x\nx\n"},
		{"{\n}\n{\n}\n", "{\n}\n{\nnew\n}\n{\n}\n"},
		{"func a() {\n\tx()\n}\n\nfunc c() {\n\tz()\n}\n", "func a() {\n\tx()\n}\n\nfunc b() {\n\ty()\n}\n\nfunc c() {\n\tz()\n}\n"},
		{"\tx := 1\n", "    x := 1  \n"},
		{"plain\n", "\x1b[31mred\x1b[0m\x07\n"},
		{"a\x00b\n", "a\xffb\n"},
		{"日本語\n", "日本\n語\n"},
		{strings.Repeat("a b c d e f g h\n", 40), strings.Repeat("h g f e d c b a\n", 40)},
		{strings.Repeat("long ", 500) + "\n", strings.Repeat("long ", 499) + "longer\n"},
	}
}

// FuzzCompare checks the comparison itself on any two texts and any budget:
// it ends, the script is right, the counts fit, the work stays within the
// stated bound, a script called minimal is, and the unified form reads back.
func FuzzCompare(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		for _, budget := range []int{0, 1, 64, -1} {
			f.Add(seed[0], seed[1], budget, 3)
		}
	}
	f.Fuzz(func(t *testing.T, before, after string, budget, context int) {
		if (budget < 0 || budget > 1<<24) && len(before)+len(after) > 1<<11 {
			// Without a limit, or with one so large that it is none, the
			// search may take time in proportion to the product of the
			// two sizes, by design.
			budget = 0
		}
		start := time.Now()
		res := Compare(before, after, Options{Budget: budget})
		if took := time.Since(start); budget >= 0 && took > 20*time.Second {
			// The budget bounds the work; the clock is only looked at to
			// catch a search that does not end.
			t.Fatalf("the comparison took %v under a budget of %d", took, budget)
		}
		changed := checkScript(t, before, after, res.Edits)
		checkStats(t, before, after, res)
		n, m := len(splitLines(before)), len(splitLines(after))
		if budget > 0 && budget < 1<<40 && res.Work > workBound(n, m, budget) {
			t.Fatalf("budget %d: %d comparisons for %d and %d lines is over the bound of %d", budget, res.Work, n, m, workBound(n, m, budget))
		}
		if budget < 0 && !res.Minimal {
			t.Fatalf("without a budget the script must be the smallest")
		}
		if res.Minimal && n*m <= 1<<18 {
			if want := smallest(before, after); changed != want {
				t.Fatalf("the script is called minimal and changes %d lines; the smallest changes %d", changed, want)
			}
		}

		context %= 16
		patch := Unified(before, after, UnifiedOptions{Budget: budget, Context: context})
		got, err := applyUnified(before, patch)
		if err != nil {
			t.Fatalf("the unified form does not apply: %v\n%s", err, patch)
		}
		if got != after {
			t.Fatalf("the unified form gives %q, want %q\n%s", got, after, patch)
		}

		inHunks := 0
		for _, h := range Hunks(res.Edits, context) {
			for _, e := range h.Edits {
				if e.Op != Keep {
					inHunks += len(e.Lines)
				}
			}
		}
		if inHunks != changed {
			t.Fatalf("the hunks hold %d of the %d changed lines", inHunks, changed)
		}
	})
}

// FuzzRender checks the drawing on any two texts and any options: it ends,
// no row is wider than the terminal, nothing but text and styles is written,
// and the colours add nothing that the plain rows do not have.
func FuzzRender(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed[0], seed[1], uint8(80), uint8(0), uint8(3))
		f.Add(seed[0], seed[1], uint8(7), uint8(5), uint8(0xff))
	}
	styleOnly := regexp.MustCompile("\x1b\\[[0-9;]*m")
	f.Fuzz(func(t *testing.T, before, after string, width, limit, bits uint8) {
		opts := RenderOptions{
			Width:    int(width),
			MaxLines: int(limit),
			Profile:  text.Profile(bits & 3),
			Deleted:  bits&4 != 0,
			Metrics:  text.Metrics{AmbiguousWide: bits&8 != 0},
			TabWidth: int(bits >> 4 & 3 * 4),
			Context:  int(bits>>6) - 1,
			Budget:   1 << 16,
		}
		d := core.Display{Path: "a/path with spaces.txt", Before: before, After: after}
		got := Render(d, opts)
		opts.Profile = text.NoColor
		want := Render(d, opts)
		if len(got) != len(want) {
			t.Fatalf("%d rows with colour and %d without", len(got), len(want))
		}
		if len(got) == 0 {
			t.Fatalf("no rows at all")
		}
		for k, row := range got {
			if w := opts.Metrics.VisibleWidth(row); opts.Width >= 4 && w > opts.Width {
				t.Fatalf("row %q is %d cells wide in a terminal of %d", row, w, opts.Width)
			}
			bare := styleOnly.ReplaceAllString(row, "")
			if bare != want[k] {
				t.Fatalf("without its styles row %d is %q; drawn without colour it is %q", k, bare, want[k])
			}
			for _, c := range bare {
				if unicode.IsControl(c) {
					t.Fatalf("row %q holds the control character %U", row, c)
				}
			}
		}
		if limit > 0 {
			// Above the hunks there are at most the header, wrapped, and
			// three notes, wrapped; below them one note, wrapped.
			if most := int(limit) + 400; len(got) > most {
				t.Fatalf("%d rows under a cap of %d", len(got), limit)
			}
		}
	})
}

// FuzzSpans checks the marks inside a pair of lines on any two strings:
// they lie between grapheme clusters, in order, and leave the same string in
// both.
func FuzzSpans(f *testing.F) {
	for _, seed := range [][2]string{
		{"", ""}, {"a", ""}, {"return nil", "return err"}, {"colour", "color"}, {"  x", "\tx"},
		{"foo(a, b)", "foo(a, b, c)"}, {"日本語のテキスト", "日本語のテスト"}, {"a\x00b", "a\xffb"},
		{"e\u0301", "e"}, {"\U0001f468\u200d\U0001f469", "\U0001f468"}, {"a b", "a  b"}, {"x \x1b[0m", "x"},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, old, new string) {
		oldSpans, newSpans, ok := Spans(old, new)
		checkSpans(t, old, new, oldSpans, newSpans, ok)
		if old == new && (!ok || oldSpans != nil || newSpans != nil) {
			t.Fatalf("equal lines get spans %v %v (ok %v)", oldSpans, newSpans, ok)
		}
	})
}

// TestSourcesHoldNothingUnseen reads this package's own Go files and fails
// if one of them holds a character that a reader cannot see or cannot tell
// from another: a control character, an invisible format character, a
// combining mark, an odd kind of space. The package exists to keep such
// things from slipping past a person; its tests are full of them, and each
// must be written as an escape.
func TestSourcesHoldNothingUnseen(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		src, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		checked++
		line := 1
		for _, c := range string(src) {
			switch {
			case c == '\n':
				line++
			case c == '\t' || c == ' ':
			case unicode.IsControl(c), unicode.In(c, unicode.Cf, unicode.Mn, unicode.Me, unicode.Zs, unicode.Zl, unicode.Zp), c == unicode.ReplacementChar:
				t.Errorf("%s:%d holds %U as it is; write it as an escape", entry.Name(), line, c)
			}
		}
	}
	if checked == 0 {
		t.Error("no Go files were found to check")
	}
}

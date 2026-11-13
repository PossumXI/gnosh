package text

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// readTestdata returns the contents of a file under testdata.
func readTestdata(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading the test file %s: %v", name, err)
	}
	return string(data)
}

// graphemes collects the clusters of s.
func graphemes(s string) []string { return slices.Collect(Graphemes(s)) }

// TestGraphemeBreakTest runs every line of Unicode's own test file for
// grapheme cluster boundaries.
func TestGraphemeBreakTest(t *testing.T) {
	const wantLines = 766 // the file's own "# Lines:" count
	lines := 0
	for n, line := range strings.Split(readTestdata(t, "GraphemeBreakTest.txt"), "\n") {
		spec, comment, _ := strings.Cut(line, "#")
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		lines++
		// A line reads "÷ 0020 × 0308 ÷ 0020 ÷": ÷ marks a boundary, × none.
		var want []string
		var cluster, whole strings.Builder
		for _, field := range strings.Fields(spec) {
			switch field {
			case "÷":
				if cluster.Len() > 0 {
					want = append(want, cluster.String())
					cluster.Reset()
				}
			case "×":
			default:
				point, err := strconv.ParseUint(field, 16, 32)
				if err != nil || !utf8.ValidRune(rune(point)) {
					t.Fatalf("line %d: cannot use the code point %q", n+1, field)
				}
				cluster.WriteRune(rune(point))
				whole.WriteRune(rune(point))
			}
		}
		if cluster.Len() > 0 {
			t.Fatalf("line %d does not end with a boundary: %s", n+1, spec)
		}
		s := whole.String()
		if got := graphemes(s); !slices.Equal(got, want) {
			t.Errorf("line %d: %s\n got %q\nwant %q\n%s", n+1, spec, got, want, strings.TrimSpace(comment))
		}

		// The stepping form and the piece scanner must agree with it.
		var stepped, pieces []string
		for rest := s; rest != ""; {
			var c string
			c, _, rest = NextGrapheme(rest)
			stepped = append(stepped, c)
		}
		for rest := s; rest != ""; {
			var p piece
			p, rest = Metrics{}.nextPiece(rest)
			pieces = append(pieces, p.text)
		}
		if !slices.Equal(stepped, want) {
			t.Errorf("line %d: NextGrapheme gives %q, want %q", n+1, stepped, want)
		}
		if !slices.Equal(pieces, want) {
			t.Errorf("line %d: the piece scanner gives %q, want %q", n+1, pieces, want)
		}
	}
	if lines != wantLines {
		t.Errorf("ran %d test lines, want %d", lines, wantLines)
	}
}

func TestGraphemes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"ascii", "abc", []string{"a", "b", "c"}},
		{"crlf", "a\r\nb", []string{"a", "\r\n", "b"}},
		{"lf cr", "\n\r", []string{"\n", "\r"}},
		{"combining", "e\u0301a\u0308\u0304", []string{"e\u0301", "a\u0308\u0304"}},
		{"lone mark", "\u0301a", []string{"\u0301", "a"}},
		{"hangul syllables", "한글", []string{"한", "글"}},
		{"hangul jamo", "\u1112\u1161\u11ab\u1100\u1173\u11af", []string{"\u1112\u1161\u11ab", "\u1100\u1173\u11af"}},
		{"skin tone", "👍🏽a", []string{"👍🏽", "a"}},
		{"family", "👨\u200d👩\u200d👧\u200d👦!", []string{"👨\u200d👩\u200d👧\u200d👦", "!"}},
		{"flags", "🇺🇸🇰🇷🇯", []string{"🇺🇸", "🇰🇷", "🇯"}},
		{"keycap", "1\ufe0f\u20e32", []string{"1\ufe0f\u20e3", "2"}},
		{"tag flag", "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007Fx", []string{"🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", "x"}},
		{"devanagari conjunct", "क्षत्र", []string{"क्ष", "त्र"}},
		{"thai", "กำ", []string{"กำ"}},
		{"zwj between letters", "a\u200db", []string{"a\u200d", "b"}},
		{"escape is a control", "a\x1b[0m", []string{"a", "\x1b", "[", "0", "m"}},
		{"invalid byte", "a\xffb", []string{"a", "\xff", "b"}},
		{"invalid byte then mark", "\xff\u0301", []string{"\xff", "\u0301"}},
		{"truncated sequence", "\xe4\xb8", []string{"\xe4", "\xb8"}},
		{"encoded surrogate", "\xed\xa0\x80", []string{"\xed", "\xa0", "\x80"}},
		{"overlong", "\xc0\x80", []string{"\xc0", "\x80"}},
		{"valid replacement character", "\ufffd\u0301", []string{"\ufffd\u0301"}},
	}
	for _, tt := range tests {
		if got := graphemes(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("%s: Graphemes(%q) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestGraphemesStopsEarly(t *testing.T) {
	var got []string
	for c := range Graphemes("abcdef") {
		got = append(got, c)
		if len(got) == 2 {
			break
		}
	}
	if !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("got %q after breaking at two", got)
	}
}

func TestNextGrapheme(t *testing.T) {
	tests := []struct {
		in      string
		cluster string
		width   int
		rest    string
	}{
		{"", "", 0, ""},
		{"a", "a", 1, ""},
		{"ab", "a", 1, "b"},
		{"e\u0301x", "e\u0301", 1, "x"},
		{"漢字", "漢", 2, "字"},
		{"👍🏽 ok", "👍🏽", 2, " ok"},
		{"\r\nx", "\r\n", 0, "x"},
		{"\tx", "\t", 0, "x"},
		{"\xffx", "\xff", 1, "x"},
	}
	for _, tt := range tests {
		cluster, width, rest := NextGrapheme(tt.in)
		if cluster != tt.cluster || width != tt.width || rest != tt.rest {
			t.Errorf("NextGrapheme(%q) = %q, %d, %q; want %q, %d, %q", tt.in, cluster, width, rest, tt.cluster, tt.width, tt.rest)
		}
	}
	if _, width, _ := (Metrics{AmbiguousWide: true}).NextGrapheme("αβ"); width != 2 {
		t.Errorf("with wide ambiguous characters the first cluster of αβ is %d wide, want 2", width)
	}
}

// fuzzSeeds are strings that exercise every part of the package: plain and
// wide text, clusters of every kind, escape sequences whole and cut short,
// and bytes that are not UTF-8.
func fuzzSeeds() []string {
	return []string{
		"",
		"hello world",
		"  indented text that goes on for a while and then some more\n\ttabbed\r\nand done  ",
		"well-known this—that 2024-11-13 --flag-name a\u00a0b a\u200bb",
		"日本語のテキスト。「かっこ」と、小さいっゃ。한국어 텍스트",
		"e\u0301 a\u0308\u0304 \u1112\u1161\u11ab क्षत्रिय กำลัง \uff76\uff9e",
		"😀 👍🏽 👨\u200d👩\u200d👧\u200d👦 🏳\ufe0f\u200d🌈 🇺🇸🇰🇷🇯 1\ufe0f\u20e3 ❤\ufe0f ❤\ufe0e ☝🏽 a\U0001F3FB",
		"\x1b[1;31mbold red\x1b[0m plain \x1b[38;2;255;95;215mpink\x1b[39m \x1b[4:3mcurly\x1b[m",
		"\x1b]8;;https://example.com\x1b\\a link that is long\x1b]8;;\x1b\\ and \x1b]8;id=x;https://example.com/b\x07more\x1b]8;;\x07",
		"\x1b[31mred\nstill red\x1b[0m\nplain\n",
		"cut short \x1b[31 and \x1b]0;title and \x1b and \x1bP1$r\x1b\\ and \x1b(B and \x1b[?25l",
		"e\x1b[31m\u0301 👍\x1b[0m🏽 ❤\x1b[1m\ufe0f \r\x1b[0m\n 🇺\x1b[m🇸",
		"\xff\xfe \xe4\xb8 \xed\xa0\x80 \xc0\x80 a\xffb",
		"\x00\x01\x07\x08\x0b\x0c\x7f\u0085\u009b31m\u2028\u2029",
		"αβγ ─│┌ \ue000 \ufffd \uff21\uff42\uff43 \uff11\uff12\uff13",
		"\x1b[1mopen \x1b]8;;https://example.com\x1b\\and a link left open",
		"a\tb\tc\n\tx",
		"      ",
		"\n\n\n",
	}
}

// FuzzGraphemes checks that the clusters of any string are non-empty, give
// the string back when joined, and are the same whichever way they are read.
func FuzzGraphemes(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		var joined strings.Builder
		rest := s
		for c := range Graphemes(s) {
			if c == "" {
				t.Fatalf("Graphemes(%q) yields an empty cluster", s)
			}
			stepped, _, after := NextGrapheme(rest)
			if stepped != c {
				t.Fatalf("Graphemes(%q) yields %q where NextGrapheme gives %q", s, c, stepped)
			}
			// A cluster stays whole when it is read on its own.
			if only := graphemes(c); len(only) != 1 {
				t.Fatalf("the cluster %q of %q splits into %q on its own", c, s, only)
			}
			rest = after
			joined.WriteString(c)
		}
		if joined.String() != s {
			t.Fatalf("the clusters of %q join to %q", s, joined.String())
		}
	})
}

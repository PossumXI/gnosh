package text

import (
	"slices"
	"strings"
	"testing"
)

func TestWrap(t *testing.T) {
	const link = "\x1b]8;;https://example.com\x1b\\"
	const unlink = "\x1b]8;;\x1b\\"
	tests := []struct {
		name  string
		in    string
		width int
		opts  WrapOptions
		want  []string
	}{
		// Words and spaces.
		{"fits", "hello world", 11, WrapOptions{}, []string{"hello world"}},
		{"words", "the quick brown fox jumps", 10, WrapOptions{}, []string{"the quick", "brown fox", "jumps"}},
		{"one word a line", "hello world", 5, WrapOptions{}, []string{"hello", "world"}},
		{"spaces at a break are dropped", "hello     world", 7, WrapOptions{}, []string{"hello", "world"}},
		{"spaces inside a line are kept", "a   b   c", 9, WrapOptions{}, []string{"a   b   c"}},
		{"spaces inside a line are kept when wrapping", "a   b   c", 6, WrapOptions{}, []string{"a   b", "c"}},
		{"tabs break like spaces", "a\tb", 1, WrapOptions{}, []string{"a", "b"}},
		{"no-break space does not break", "a\u00a0b c", 3, WrapOptions{}, []string{"a\u00a0b", "c"}},
		{"zero width space allows a break", "aaa\u200bbbb", 3, WrapOptions{}, []string{"aaa\u200b", "bbb"}},

		// Empty input, spaces and newlines.
		{"empty", "", 10, WrapOptions{}, []string{""}},
		{"only spaces, too many", "     ", 3, WrapOptions{}, []string{"   "}},
		{"only spaces, few", "   ", 10, WrapOptions{}, []string{"   "}},
		{"newlines", "a\nb\n\nc", 10, WrapOptions{}, []string{"a", "b", "", "c"}},
		{"trailing newline", "a\n", 10, WrapOptions{}, []string{"a", ""}},
		{"trailing newlines", "a\n\n", 10, WrapOptions{}, []string{"a", "", ""}},
		{"only a newline", "\n", 10, WrapOptions{}, []string{"", ""}},
		{"crlf", "a\r\nb\r\n", 10, WrapOptions{}, []string{"a", "b", ""}},
		{"newline and wrapping", "aaa bbb\nccc ddd", 4, WrapOptions{}, []string{"aaa", "bbb", "ccc", "ddd"}},
		{"trailing spaces that fit are kept", "hello  ", 7, WrapOptions{}, []string{"hello  "}},
		{"trailing spaces are kept as far as they fit", "hello   ", 7, WrapOptions{}, []string{"hello  "}},
		{"trailing spaces before a newline", "hello   \nworld", 5, WrapOptions{}, []string{"hello", "world"}},

		// Long words.
		{"long word", "abcdefghij", 4, WrapOptions{}, []string{"abcd", "efgh", "ij"}},
		{"long word moves down first", "ab cdefghijkl", 5, WrapOptions{}, []string{"ab", "cdefg", "hijkl"}},
		{"long word then short", "abcdefg hi", 5, WrapOptions{}, []string{"abcde", "fg hi"}},

		// Hyphens and dashes.
		{"hyphen", "a well-known fact", 8, WrapOptions{}, []string{"a well-", "known", "fact"}},
		{"hyphen stays when the word fits", "well-known", 10, WrapOptions{}, []string{"well-known"}},
		{"several hyphens", "a-b-c-d-e", 4, WrapOptions{}, []string{"a-b-", "c-d-", "e"}},
		{"flag", "use --flag-name here", 8, WrapOptions{}, []string{"use", "--flag-", "name", "here"}},
		{"date", "2024-11-13 x", 6, WrapOptions{}, []string{"2024-1", "1-13 x"}},
		{"negative number", "x -5 y", 3, WrapOptions{}, []string{"x", "-5", "y"}},
		{"em dash", "this—that", 6, WrapOptions{}, []string{"this—", "that"}},
		{"en dash between numbers", "10–20", 3, WrapOptions{}, []string{"10–", "20"}},

		// Text written without spaces.
		{"japanese", "日本語のテキスト", 6, WrapOptions{}, []string{"日本語", "のテキ", "スト"}},
		{"no line starts with a full stop", "日本語。日本", 6, WrapOptions{}, []string{"日本", "語。日", "本"}},
		{"brackets stay with their text", "日本「語」日本", 6, WrapOptions{}, []string{"日本", "「語」", "日本"}},
		{"no line starts with a small kana", "ああっち", 4, WrapOptions{}, []string{"あ", "あっ", "ち"}},
		{"unless nothing else can be done", "ちょっと", 4, WrapOptions{}, []string{"ちょ", "っと"}},
		{"latin and han", "abc漢字def", 5, WrapOptions{}, []string{"abc漢", "字def"}},
		{"han then latin full stop", "漢字.漢字", 4, WrapOptions{}, []string{"漢", "字.", "漢字"}},
		{"korean", "안녕하세요 세계", 6, WrapOptions{}, []string{"안녕하", "세요", "세계"}},
		{"emoji", "😀😀😀😀", 4, WrapOptions{}, []string{"😀😀", "😀😀"}},
		{"flags", "🇺🇸🇰🇷🇯🇵", 4, WrapOptions{}, []string{"🇺🇸🇰🇷", "🇯🇵"}},
		{"fullwidth", "\uff21\uff22\uff23", 4, WrapOptions{}, []string{"\uff21\uff22", "\uff23"}},

		// Narrow widths.
		{"width one", "ab c", 1, WrapOptions{}, []string{"a", "b", "c"}},
		{"width one, wide characters overflow", "漢字", 1, WrapOptions{}, []string{"漢", "字"}},
		{"width one, mixed", "a漢", 1, WrapOptions{}, []string{"a", "漢"}},
		{"width two", "漢字ab", 2, WrapOptions{}, []string{"漢", "字", "ab"}},
		{"width two, mixed", "a漢b", 2, WrapOptions{}, []string{"a", "漢", "b"}},
		{"width two, clusters", "e\u0301e\u0301e\u0301", 2, WrapOptions{}, []string{"e\u0301e\u0301", "e\u0301"}},
		{"width three, wide characters leave a cell", "漢字漢", 3, WrapOptions{}, []string{"漢", "字", "漢"}},
		{"no limit", "a b\nc d", 0, WrapOptions{}, []string{"a b", "c d"}},
		{"negative width is no limit", "a b c", -1, WrapOptions{}, []string{"a b c"}},

		// Indentation.
		{"indentation is kept and repeated", "    indented text that wraps", 14, WrapOptions{}, []string{"    indented", "    text that", "    wraps"}},
		{"deep indentation is not repeated", "      deep text here", 10, WrapOptions{}, []string{"      deep", "text here"}},
		{"indentation is cut when the first word needs the room", "        deep text here", 10, WrapOptions{}, []string{"      deep", "text here"}},
		{"indentation goes when the first word is long", "   abcdefgh", 4, WrapOptions{}, []string{"abcd", "efgh"}},
		{"each paragraph has its own indentation", "  aaa bbb\nccc ddd", 5, WrapOptions{}, []string{"  aaa", "  bbb", "ccc", "ddd"}},

		// Prefixes and hanging indents.
		{"list item", "one two three four", 10, WrapOptions{First: "- ", Rest: "  "}, []string{"- one two", "  three", "  four"}},
		{"quote", "a b\nc", 10, WrapOptions{First: "> ", Rest: "> "}, []string{"> a b", "> c"}},
		{"prefix on empty lines", "a\n\nb", 10, WrapOptions{First: "> ", Rest: "> "}, []string{"> a", "> ", "> b"}},
		{"prefix on empty input", "", 10, WrapOptions{First: "- ", Rest: "  "}, []string{"- "}},
		{"styled prefix", "aaa bbb", 5, WrapOptions{First: "\x1b[2m|\x1b[0m ", Rest: "\x1b[2m|\x1b[0m "}, []string{"\x1b[2m|\x1b[0m aaa", "\x1b[2m|\x1b[0m bbb"}},
		{"prefix wider than the width", "ab", 2, WrapOptions{First: ">>> ", Rest: ">>> "}, []string{">>> a", ">>> b"}},
		{"hang", "alpha beta gamma\ndelta epsilon zeta", 11, WrapOptions{Hang: 2}, []string{"alpha beta", "  gamma", "delta", "  epsilon", "  zeta"}},
		{"hang with indentation", "  aaa bbb ccc", 7, WrapOptions{Hang: 1}, []string{"  aaa", "   bbb", "   ccc"}},
		{"hang too deep is dropped", "aaa bbb", 4, WrapOptions{Hang: 3}, []string{"aaa", "bbb"}},
		{"prefix and hang", "aaa bbb ccc", 8, WrapOptions{First: "* ", Rest: "  ", Hang: 2}, []string{"* aaa", "    bbb", "    ccc"}},

		// Styles are closed at the end of a line and opened on the next.
		{"style across a break", "\x1b[1mbold text that wraps\x1b[0m", 10, WrapOptions{}, []string{"\x1b[1mbold text\x1b[0m", "\x1b[1mthat wraps\x1b[0m"}},
		{"style inside a line", "plain \x1b[31mred text\x1b[0m plain again", 12, WrapOptions{}, []string{"plain \x1b[31mred\x1b[0m", "\x1b[31mtext\x1b[0m plain", "again"}},
		{"style opened at a break", "aaa \x1b[1mbbb\x1b[0m", 3, WrapOptions{}, []string{"aaa", "\x1b[1mbbb\x1b[0m"}},
		{"style closed at a break", "\x1b[1maaa\x1b[0m bbb", 3, WrapOptions{}, []string{"\x1b[1maaa\x1b[0m", "bbb"}},
		{"style closed after the space", "\x1b[4maaa \x1b[0mbbb", 3, WrapOptions{}, []string{"\x1b[4maaa\x1b[0m", "bbb"}},
		{"style across a newline", "\x1b[32mgreen\nstill\x1b[0m", 20, WrapOptions{}, []string{"\x1b[32mgreen\x1b[0m", "\x1b[32mstill\x1b[0m"}},
		{"style across an empty line", "\x1b[32ma\n\nb\x1b[0m", 20, WrapOptions{}, []string{"\x1b[32ma\x1b[0m", "", "\x1b[32mb\x1b[0m"}},
		{"style left open is closed", "\x1b[1mopen", 20, WrapOptions{}, []string{"\x1b[1mopen\x1b[0m"}},
		{"style written the standard way", "\x1b[01;38:2::1:2:3mab cd\x1b[m", 2, WrapOptions{}, []string{"\x1b[1;38;2;1;2;3mab\x1b[0m", "\x1b[1;38;2;1;2;3mcd\x1b[0m"}},
		{"underline style and colour", "\x1b[4:3;58:5:9mab cd\x1b[0m", 2, WrapOptions{}, []string{"\x1b[4:3;58:5:9mab\x1b[0m", "\x1b[4:3;58:5:9mcd\x1b[0m"}},
		{"style in a long word", "\x1b[31mabcdef\x1b[0m", 3, WrapOptions{}, []string{"\x1b[31mabc\x1b[0m", "\x1b[31mdef\x1b[0m"}},
		{"style changes in a long word", "ab\x1b[1mcd\x1b[0mef", 3, WrapOptions{}, []string{"ab\x1b[1mc\x1b[0m", "\x1b[1md\x1b[0mef"}},
		{"style with prefix and indentation", "  \x1b[1maaa bbb\x1b[0m", 7, WrapOptions{First: "> ", Rest: "> "}, []string{">   \x1b[1maaa\x1b[0m", ">   \x1b[1mbbb\x1b[0m"}},
		{"styled indentation stays styled", "\x1b[41m  aaa\x1b[0m bbb", 5, WrapOptions{}, []string{"\x1b[41m  aaa\x1b[0m", "  bbb"}},
		{"only a style", "\x1b[1m", 10, WrapOptions{}, []string{""}},
		{"style on wide text", "\x1b[33m日本語\x1b[0m", 4, WrapOptions{}, []string{"\x1b[33m日本\x1b[0m", "\x1b[33m語\x1b[0m"}},

		// Hyperlinks are closed and opened the same way.
		{"link across a break", link + "click here" + unlink, 5, WrapOptions{}, []string{link + "click" + unlink, link + "here" + unlink}},
		{"link with parameters and bel", "\x1b]8;id=1;https://example.com\x07click here\x1b]8;;\x07", 5, WrapOptions{}, []string{
			"\x1b]8;id=1;https://example.com\x1b\\click\x1b]8;;\x1b\\", "\x1b]8;id=1;https://example.com\x1b\\here\x1b]8;;\x1b\\",
		}},
		{"link and style", "\x1b[4m" + link + "click here" + unlink + "\x1b[0m", 5, WrapOptions{}, []string{
			"\x1b[4m" + link + "click" + unlink + "\x1b[0m", "\x1b[4m" + link + "here" + unlink + "\x1b[0m",
		}},
		{"link inside a line", "see " + link + "the docs" + unlink + " now", 20, WrapOptions{}, []string{"see " + link + "the docs" + unlink + " now"}},
		{"link left open is closed", link + "open", 20, WrapOptions{}, []string{link + "open" + unlink}},

		// Other sequences stay on their line; unfinished ones are dropped.
		{"other sequence at a break", "ab\x1b[K cd", 2, WrapOptions{}, []string{"ab\x1b[K", "cd"}},
		{"other sequence inside a line", "a\x1b[Kb cd", 2, WrapOptions{}, []string{"a\x1b[Kb", "cd"}},
		{"other sequence before the text", "\x1b[?25lab", 5, WrapOptions{}, []string{"\x1b[?25lab"}},
		{"unfinished sequence at the end", "ab cd\x1b[31", 2, WrapOptions{}, []string{"ab", "cd"}},
		{"unfinished sequence inside a line", "a\x1b[31\tb", 5, WrapOptions{}, []string{"a\tb"}},
		{"unfinished title swallows the rest", "ab\x1b]0;title\nmore", 5, WrapOptions{}, []string{"ab"}},
		{"lone escape", "a\x1b", 5, WrapOptions{}, []string{"a"}},

		// A cluster wider than the line is cut between its characters.
		{"wide cluster that fits is kept whole", "aस\u094dत\u094dर\u0940b", 4, WrapOptions{}, []string{"a", "स\u094dत\u094dर\u0940", "b"}},
		{"wide cluster cut to fit", "स\u094dत\u094dर\u0940", 2, WrapOptions{}, []string{"स\u094dत\u094d", "र\u0940"}},
		{"wide cluster cut to fit, three cells", "aस\u094dत\u094dर\u0940b", 3, WrapOptions{}, []string{"aस\u094dत\u094d", "र\u0940b"}},
		{"stray skin tone", "a🏽b", 2, WrapOptions{}, []string{"a", "🏽", "b"}},
		{"jamo with two leading consonants", "\u1100\u1100\u1161", 2, WrapOptions{}, []string{"\u1100", "\u1100\u1161"}},
		{"wide cluster with a hanging indent", "x स\u094dत\u094dर\u0940", 5, WrapOptions{Hang: 2}, []string{"x", "  स\u094dत\u094dर", "  \u0940"}},

		// A cluster that a sequence interrupts stays whole.
		{"sequence inside a cluster", "e\x1b[31m\u0301x", 1, WrapOptions{}, []string{"e\x1b[31m\u0301\x1b[0m", "\x1b[31mx\x1b[0m"}},
		{"sequence inside an emoji", "ab👍\x1b[1m🏽c", 4, WrapOptions{}, []string{"ab👍\x1b[1m🏽\x1b[0m", "\x1b[1mc\x1b[0m"}},
	}
	for _, tt := range tests {
		got := Wrap(tt.in, tt.width, tt.opts)
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: Wrap(%q, %d, %+v)\n got %q\nwant %q", tt.name, tt.in, tt.width, tt.opts, got, tt.want)
		}
	}
	if got, want := (Metrics{AmbiguousWide: true}).Wrap("αβγ δ", 5, WrapOptions{}), []string{"αβ", "γ δ"}; !slices.Equal(got, want) {
		t.Errorf("with wide ambiguous characters Wrap gives %q, want %q", got, want)
	}
}

// TestWrapSharesTheInput checks that plain lines are substrings of the input
// rather than copies.
func TestWrapSharesTheInput(t *testing.T) {
	in := strings.Repeat("some words to wrap ", 20)
	allocs := testing.AllocsPerRun(20, func() {
		Wrap(in, 30, WrapOptions{})
	})
	// The pieces of the paragraph and the slice of lines are all that is
	// allocated: far fewer than one allocation a line.
	if lines := len(Wrap(in, 30, WrapOptions{})); allocs > float64(lines)/2 {
		t.Errorf("wrapping %d plain lines took %.0f allocations", lines, allocs)
	}
}

// checkWrap checks what every result of Wrap must satisfy. first and rest
// are the prefixes that were asked for.
func checkWrap(t *testing.T, m Metrics, s string, width int, opts WrapOptions, lines []string) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatalf("Wrap(%q, %d) returned no lines", s, width)
	}
	const blank = " \t\r\n"
	text := Strip(s)
	pos := 0
	for i, line := range lines {
		prefix := opts.Rest
		if i == 0 {
			prefix = opts.First
		}
		body, ok := strings.CutPrefix(line, prefix)
		if !ok {
			t.Fatalf("Wrap(%q, %d): line %d %q does not start with the prefix %q", s, width, i, line, prefix)
		}
		if hasUnfinished(line) {
			t.Fatalf("Wrap(%q, %d): line %d %q holds an unfinished sequence", s, width, i, line)
		}
		// A line stands alone: it leaves nothing open.
		if l := endLook(line); !l.plain() {
			t.Fatalf("Wrap(%q, %d): line %d %q leaves %+v open", s, width, i, line, l)
		}
		visible := Strip(body)
		if strings.Contains(visible, "\n") {
			t.Fatalf("Wrap(%q, %d): line %d %q holds a newline", s, width, i, line)
		}
		// A line is no wider than the width. Only where there is less
		// room than two cells may a line overflow, and then it holds a
		// single character that takes room.
		if width >= 1 && m.VisibleWidth(line) > width {
			if m.VisibleWidth(prefix)+2 <= width {
				t.Fatalf("Wrap(%q, %d): line %d %q is %d wide", s, width, i, line, m.VisibleWidth(line))
			}
			spacing := 0
			for rest := visible; rest != ""; {
				size, w := m.codePoint(rest)
				if w > 0 {
					spacing++
				}
				rest = rest[size:]
			}
			if spacing > 1 {
				t.Fatalf("Wrap(%q, %d): line %d %q overflows with %d characters that take room", s, width, i, line, spacing)
			}
		}
		// The line's text, less the blanks at its ends, comes next in the
		// input's text, after blanks only.
		words := strings.Trim(visible, blank)
		for pos < len(text) && strings.IndexByte(blank, text[pos]) >= 0 {
			pos++
		}
		if !strings.HasPrefix(text[pos:], words) {
			t.Fatalf("Wrap(%q, %d): line %d %q does not continue the text at %q", s, width, i, line, text[pos:])
		}
		pos += len(words)
	}
	if left := strings.Trim(text[pos:], blank); left != "" {
		t.Fatalf("Wrap(%q, %d) = %q lost the text %q", s, width, lines, left)
	}
}

// TestWrapProperties runs the checks of the fuzz test over the seeds at every
// width, so that they are part of the ordinary test run.
func TestWrapProperties(t *testing.T) {
	options := []WrapOptions{{}, {First: "- ", Rest: "  "}, {Hang: 2}, {First: "\x1b[2m>\x1b[0m ", Rest: "\x1b[2m>\x1b[0m ", Hang: 1}}
	for _, s := range append(fuzzSeeds(), everyForm) {
		for width := -1; width <= 24; width++ {
			for _, opts := range options {
				for _, m := range []Metrics{{}, {AmbiguousWide: true}} {
					checkWrap(t, m, s, width, opts, m.Wrap(s, width, opts))
				}
			}
		}
	}
}

// FuzzWrap checks, for any string and width: no line is wider than the width
// (bar a single cluster that cannot be split), the lines' text is the
// input's text less the blanks at the breaks, every line stands alone, and
// text with nothing to wrap comes back as it was.
func FuzzWrap(f *testing.F) {
	for i, seed := range fuzzSeeds() {
		f.Add(seed, i*2-1, uint8(i))
	}
	f.Fuzz(func(t *testing.T, s string, width int, pick uint8) {
		width = width%60 - 2
		options := []WrapOptions{{}, {}, {First: "- ", Rest: "  "}, {Hang: 3}, {First: "> ", Rest: "> ", Hang: 1}}
		opts := options[int(pick)%len(options)]
		m := Metrics{AmbiguousWide: pick >= 128}
		lines := m.Wrap(s, width, opts)
		checkWrap(t, m, s, width, opts, lines)

		// Without a limit, and with nothing to carry over, the lines are
		// the text split at its newlines.
		if !strings.Contains(s, "\x1b") && !strings.Contains(s, "\r\n") {
			if got := strings.Join(m.Wrap(s, 0, WrapOptions{}), "\n"); got != s {
				t.Fatalf("Wrap(%q, 0) joins to %q", s, got)
			}
		}
	})
}

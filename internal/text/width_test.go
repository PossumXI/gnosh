package text

import (
	"strconv"
	"strings"
	"testing"
)

func TestWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
		wide int // the width when ambiguous characters are wide; -1 when it is the same
	}{
		// ASCII.
		{"empty", "", 0, -1},
		{"word", "hello", 5, -1},
		{"sentence", "hello, world!", 13, -1},
		{"space", " ", 1, -1},
		{"tilde", "~", 1, -1},

		// Control characters take no cell.
		{"nul", "\x00", 0, -1},
		{"tab", "\t", 0, -1},
		{"tab between letters", "a\tb", 2, -1},
		{"newline", "\n", 0, -1},
		{"crlf", "a\r\nb", 2, -1},
		{"escape", "\x1b", 0, -1},
		{"delete", "\x7f", 0, -1},
		{"bell and backspace", "a\x07\x08b", 2, -1},
		{"c1 controls", "\u0085\u009b", 0, -1},
		{"line and paragraph separators", "\u2028\u2029", 0, -1},

		// Combining sequences.
		{"e acute", "e\u0301", 1, -1},
		{"two marks", "a\u0308\u0304", 1, -1},
		{"lone mark", "\u0301", 0, -1},
		{"enclosing mark", "a\u20dd", 1, -1},
		{"word with marks", "n\u0303an\u0303a", 4, -1},

		// Hangul.
		{"hangul syllable", "한", 2, -1},
		{"hangul word", "한글", 4, -1},
		{"jamo syllable", "\u1112\u1161\u11ab", 2, -1},
		{"jamo without tail", "\u1100\u1161", 2, -1},
		{"lone leading jamo", "\u1100", 2, -1},
		{"lone vowel jamo", "\u1161", 0, -1},
		{"lone trailing jamo", "\u11a8", 0, -1},
		{"syllable and trailing jamo", "하\u11ab", 2, -1},
		{"compatibility jamo", "ㄱ", 2, -1},
		{"halfwidth hangul", "\uffa1", 1, -1},

		// CJK.
		{"han", "漢字", 4, -1},
		{"japanese", "日本語", 6, -1},
		{"hiragana", "あ", 2, -1},
		{"katakana", "ア", 2, -1},
		{"halfwidth katakana", "\uff71", 1, -1},
		{"halfwidth katakana with sound mark", "\uff76\uff9e", 2, -1},
		{"ideographic comma", "、", 2, -1},
		{"ideographic space", "\u3000", 2, -1},
		{"kana with combining sound mark", "か\u3099", 2, -1},
		{"mixed", "a漢b", 4, -1},

		// Fullwidth and halfwidth forms.
		{"fullwidth letter", "\uff21", 2, -1},
		{"fullwidth digits", "\uff11\uff12\uff13", 6, -1},
		{"fullwidth exclamation", "\uff01", 2, -1},
		{"halfwidth arrow", "\uffe9", 1, -1},

		// Emoji.
		{"emoji", "😀", 2, -1},
		{"thumbs up", "👍", 2, -1},
		{"skin tone", "👍🏽", 2, -1},
		{"family", "👨\u200d👩\u200d👧\u200d👦", 2, -1},
		{"rainbow flag", "🏳\ufe0f\u200d🌈", 2, -1},
		{"technologist", "🧑\u200d💻", 2, -1},
		{"handshake with two tones", "\U0001FAF1🏻\u200d\U0001FAF2🏿", 2, -1},
		{"heart on fire without selector", "❤\u200d🔥", 2, -1},
		{"flag", "🇺🇸", 2, -1},
		{"two flags", "🇺🇸🇰🇷", 4, -1},
		{"lone regional indicator", "🇺", 1, -1},
		{"flag and a half", "🇺🇸🇰", 3, -1},
		{"subdivision flag", "🏴\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", 2, -1},
		{"keycap", "1\ufe0f\u20e3", 2, -1},
		{"keycap hash", "#\ufe0f\u20e3", 2, -1},
		{"keycap without selector", "1\u20e3", 1, -1},
		{"text heart", "❤", 1, -1},
		{"emoji heart", "❤\ufe0f", 2, -1},
		{"heart with text selector", "❤\ufe0e", 1, -1},
		{"watch", "⌚", 2, -1},
		{"watch with emoji selector", "⌚\ufe0f", 2, -1},
		{"watch with text selector", "⌚\ufe0e", 2, -1},
		{"pointing up", "☝", 1, -1},
		{"pointing up with tone", "☝🏽", 2, -1},
		{"pointing up with selector", "☝\ufe0f", 2, -1},
		{"pointing up with selector and tone", "☝\ufe0f🏽", 2, -1},
		{"copyright", "©", 1, -1},
		{"copyright as emoji", "©\ufe0f", 2, -1},
		{"selector after a letter", "a\ufe0f", 1, -1},
		{"lone skin tone", "🏽", 2, -1},
		{"skin tone after a letter", "a🏽", 3, -1},
		{"skin tone after a face", "😀🏽", 4, -1},
		{"emoji then text", "😀 ok", 5, -1},

		// Zero-width characters.
		{"zero width space", "a\u200bb", 2, -1},
		{"zero width joiner", "a\u200db", 2, -1},
		{"zero width non-joiner", "a\u200cb", 2, -1},
		{"byte order mark", "\ufeffa", 1, -1},
		{"word joiner", "a\u2060b", 2, -1},
		{"combining grapheme joiner", "a\u034fb", 2, -1},
		{"arabic letter mark", "\u061c", 0, -1},
		{"right-to-left override", "\u202eabc", 3, -1},
		{"lone selector", "\ufe0f", 0, -1},
		{"language tag", "\U000E0001", 0, -1},
		{"soft hyphen", "\u00ad", 1, 2},

		// Spaces.
		{"no-break space", "\u00a0", 1, -1},
		{"em space", "\u2003", 1, -1},

		// Ambiguous width.
		{"greek", "αβγ", 3, 6},
		{"box drawing", "─│┌", 3, 6},
		{"e acute, precomposed", "é", 1, 2},
		{"arrow", "→", 1, 2},
		{"private use", "\ue000", 1, 2},
		{"replacement character", "\ufffd", 1, 2},
		{"ambiguous with a mark", "α\u0301", 1, 2},

		// Bytes that are not UTF-8 count as a replacement character each.
		{"invalid byte", "\xff", 1, 2},
		{"invalid byte between letters", "a\xffb", 3, 4},
		{"truncated sequence", "\xe4\xb8", 2, 4},
		{"overlong encoding", "\xc0\x80", 2, 4},

		// Other scripts: a cluster is as wide as its parts.
		{"devanagari letter", "क", 1, -1},
		{"devanagari with spacing vowel", "कि", 2, -1},
		{"devanagari conjunct", "क्ष", 2, -1},
		{"namaste", "नमस्ते", 4, -1},
		{"thai with mark above", "ก็", 1, -1},
		{"thai sara am", "กำ", 2, -1},
		{"thai word", "สวัสดี", 4, -1},
		{"arabic", "سلام", 4, -1},
		{"arabic with a vowel mark", "مَ", 1, -1},
		{"arabic number sign", "\u0600", 1, -1},
		{"arabic number sign with digits", "\u0600123 x", 6, -1},
		{"letters around a wide character", "ab漢cd\r\nef", 8, -1},
		{"hebrew with points", "שָׁלוֹם", 4, -1},
	}
	for _, tt := range tests {
		if got := Width(tt.in); got != tt.want {
			t.Errorf("%s: Width(%q) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
		wide := tt.wide
		if wide < 0 {
			wide = tt.want
		}
		if got := (Metrics{AmbiguousWide: true}).Width(tt.in); got != wide {
			t.Errorf("%s: with wide ambiguous characters Width(%q) = %d, want %d", tt.name, tt.in, got, wide)
		}
		// With no escape sequences in it, a string's visible width is its
		// width (the escape character alone is a zero-width sequence).
		if got := VisibleWidth(tt.in); got != tt.want {
			t.Errorf("%s: VisibleWidth(%q) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
	}
}

// TestEmojiTest reads Unicode's list of every emoji: each is one grapheme
// cluster, and each fully-qualified one is two cells wide.
func TestEmojiTest(t *testing.T) {
	counts := map[string]int{}
	for n, line := range strings.Split(readTestdata(t, "emoji-test.txt"), "\n") {
		spec, _, _ := strings.Cut(line, "#")
		points, status, ok := strings.Cut(spec, ";")
		if !ok {
			continue
		}
		status = strings.TrimSpace(status)
		var b strings.Builder
		for _, field := range strings.Fields(points) {
			point, err := strconv.ParseUint(field, 16, 32)
			if err != nil {
				t.Fatalf("line %d: cannot read the code point %q", n+1, field)
			}
			b.WriteRune(rune(point))
		}
		emoji := b.String()
		counts[status]++

		if clusters := graphemes(emoji); len(clusters) != 1 {
			t.Errorf("line %d: %s emoji %q (%s) is %d clusters: %q", n+1, status, emoji, strings.TrimSpace(points), len(clusters), clusters)
			continue
		}
		width := Width(emoji)
		switch status {
		case "fully-qualified", "component":
			if width != 2 {
				t.Errorf("line %d: %s emoji %q (%s) is %d wide, want 2", n+1, status, emoji, strings.TrimSpace(points), width)
			}
		case "minimally-qualified", "unqualified":
			// These lack an emoji presentation selector somewhere. With
			// the first character's selector missing, a character that is
			// text by default keeps its text width.
			if width != 1 && width != 2 {
				t.Errorf("line %d: %s emoji %q (%s) is %d wide, want 1 or 2", n+1, status, emoji, strings.TrimSpace(points), width)
			}
		default:
			t.Errorf("line %d: unknown status %q", n+1, status)
		}
		if got := (Metrics{AmbiguousWide: true}).Width(emoji); got != 2 && (status == "fully-qualified" || status == "component") {
			t.Errorf("line %d: with wide ambiguous characters %q is %d wide, want 2", n+1, emoji, got)
		}
	}
	// The counts in the file for Unicode 17.0.
	want := map[string]int{"fully-qualified": 3944, "minimally-qualified": 1029, "unqualified": 243, "component": 9}
	for status, n := range want {
		if counts[status] != n {
			t.Errorf("read %d %s emoji, want %d", counts[status], status, n)
		}
	}
}

func TestExpandTabs(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		tabWidth int
		startCol int
		want     string
	}{
		{"no tabs", "hello", 8, 0, "hello"},
		{"empty", "", 8, 0, ""},
		{"leading tab", "\tx", 8, 0, "        x"},
		{"tab after text", "ab\tx", 8, 0, "ab      x"},
		{"tab at a stop", "abcdefgh\tx", 8, 0, "abcdefgh        x"},
		{"two tabs", "a\t\tb", 4, 0, "a       b"},
		{"width four", "ab\tcd\te", 4, 0, "ab  cd  e"},
		{"width one", "a\tb", 1, 0, "a b"},
		{"start column", "\tx", 8, 3, "     x"},
		{"start column on a stop", "\tx", 4, 8, "    x"},
		{"negative start column", "\tx", 4, -5, "    x"},
		{"newline resets the column", "abc\tx\nab\ty", 4, 2, "abc   x\nab  y"},
		{"crlf resets the column", "abc\r\n\tx", 4, 0, "abc\r\n    x"},
		{"carriage return resets the column", "abcde\rab\tx", 4, 0, "abcde\rab  x"},
		{"wide characters", "漢\tx", 4, 0, "漢  x"},
		{"wide character filling a stop", "漢字\tx", 4, 0, "漢字    x"},
		{"combining marks", "e\u0301\tx", 4, 0, "e\u0301   x"},
		{"escape sequences take no room", "\x1b[31mab\x1b[0m\tx", 4, 0, "\x1b[31mab\x1b[0m  x"},
		{"tab inside a style", "\x1b[1m\tx\x1b[0m", 4, 0, "\x1b[1m    x\x1b[0m"},
		{"tab inside a sequence stays", "\x1b]0;a\tb\x07c\td", 4, 0, "\x1b]0;a\tb\x07c   d"},
		{"unfinished sequence is removed", "0000\x1b\t0", 4, 0, "0000    0"},
		{"unfinished sequence, no tab", "ab\x1b[31", 4, 0, "ab"},
		{"zero tab width leaves tabs", "a\tb", 0, 0, "a\tb"},
		{"negative tab width leaves tabs", "a\tb", -1, 0, "a\tb"},
	}
	for _, tt := range tests {
		if got := ExpandTabs(tt.in, tt.tabWidth, tt.startCol); got != tt.want {
			t.Errorf("%s: ExpandTabs(%q, %d, %d) = %q, want %q", tt.name, tt.in, tt.tabWidth, tt.startCol, got, tt.want)
		}
	}
	if got := (Metrics{AmbiguousWide: true}).ExpandTabs("α\tx", 4, 0); got != "α  x" {
		t.Errorf("with wide ambiguous characters ExpandTabs gives %q, want %q", got, "α  x")
	}
}

// FuzzWidth checks that the width of a string is the sum of the widths of
// its clusters, is never negative, and is at most two cells per code point.
func FuzzWidth(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		for _, m := range []Metrics{{}, {AmbiguousWide: true}} {
			sum := 0
			for rest := s; rest != ""; {
				var cluster string
				var width int
				cluster, width, rest = m.NextGrapheme(rest)
				if width < 0 || width > 2*len(cluster) {
					t.Fatalf("the cluster %q of %q has width %d", cluster, s, width)
				}
				if alone := m.Width(cluster); alone != width {
					t.Fatalf("the cluster %q of %q has width %d in place and %d alone", cluster, s, width, alone)
				}
				sum += width
			}
			if got := m.Width(s); got != sum {
				t.Fatalf("%+v.Width(%q) = %d, but its clusters add up to %d", m, s, got, sum)
			}
		}
		if narrow, wide := Width(s), (Metrics{AmbiguousWide: true}).Width(s); wide < narrow {
			t.Fatalf("Width(%q) is %d, but only %d with wide ambiguous characters", s, narrow, wide)
		}
		// Expanding tabs leaves none in the text (one inside an escape
		// sequence is not text), and changes nothing else.
		expanded := ExpandTabs(s, 4, 0)
		if strings.Contains(Strip(expanded), "\t") {
			t.Fatalf("ExpandTabs(%q) still holds a tab: %q", s, expanded)
		}
		if !strings.Contains(s, "\t") && expanded != finished(s) {
			t.Fatalf("ExpandTabs changed %q, which has no tab, to %q", s, expanded)
		}
		if hasUnfinished(expanded) {
			t.Fatalf("ExpandTabs(%q) = %q holds an unfinished sequence", s, expanded)
		}
		if got, want := strings.ReplaceAll(Strip(expanded), " ", ""), strings.ReplaceAll(strings.ReplaceAll(Strip(s), "\t", ""), " ", ""); got != want {
			t.Fatalf("ExpandTabs(%q) = %q changed more than tabs", s, expanded)
		}
	})
}

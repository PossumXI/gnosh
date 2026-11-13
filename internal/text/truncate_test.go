package text

import (
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	const link = "\x1b]8;;https://example.com\x1b\\"
	const unlink = "\x1b]8;;\x1b\\"
	tests := []struct {
		name  string
		in    string
		width int
		tail  string
		want  string
	}{
		// Plain text.
		{"fits", "hello", 10, "…", "hello"},
		{"fits exactly", "hello", 5, "…", "hello"},
		{"empty", "", 5, "…", ""},
		{"empty at zero", "", 0, "…", ""},
		{"cut", "hello world", 8, "…", "hello w…"},
		{"cut by one", "hello", 4, "…", "hel…"},
		{"only the tail fits", "hello", 1, "…", "…"},
		{"zero width", "hello", 0, "…", ""},
		{"negative width", "hello", -3, "…", ""},
		{"no tail", "hello", 3, "", "hel"},
		{"long tail", "hello world", 8, "...", "hello..."},
		{"tail as wide as the width", "hello", 3, "...", "..."},
		{"tail wider than the width", "hello", 2, "...", "he"},
		{"spaces are kept", "a   b   c", 6, "…", "a   b…"},

		// Wide characters are never split.
		{"wide, fits", "漢字", 4, "…", "漢字"},
		{"wide, cut on a boundary", "漢字漢字", 5, "…", "漢字…"},
		{"wide, padded", "漢字漢字", 4, "…", "漢 …"},
		{"wide, padded without tail", "漢字", 3, "", "漢 "},
		{"wide, nothing fits", "漢字", 1, "", " "},
		{"wide, only the tail fits", "漢字", 1, "…", "…"},
		{"wide tail", "abcdef", 4, "…", "abc…"},
		{"wide tail, padded", "a漢字", 3, "‥‥", "a‥‥"},
		{"emoji", "😀😀😀", 5, "…", "😀😀…"},
		{"emoji, padded", "😀😀😀", 4, "…", "😀 …"},

		// Clusters stay whole.
		{"combining marks", "e\u0301e\u0301e\u0301", 2, "", "e\u0301e\u0301"},
		{"family", "👨\u200d👩\u200d👧\u200d👦 family", 4, "…", "👨\u200d👩\u200d👧\u200d👦 …"},
		{"flag", "🇺🇸🇰🇷", 3, "…", "🇺🇸…"},
		{"skin tone", "ab👍🏽cd", 4, "", "ab👍🏽"},
		{"skin tone, padded", "ab👍🏽cd", 3, "", "ab "},
		{"zero-width characters ride along", "ab\u200bcd", 2, "", "ab\u200b"},

		// Styles.
		{"style, fits", "\x1b[31mhello\x1b[0m", 5, "…", "\x1b[31mhello\x1b[0m"},
		{"style closed after the tail", "\x1b[31mhello world\x1b[0m", 8, "…", "\x1b[31mhello w…\x1b[0m"},
		{"style ends before the cut", "\x1b[31mhello\x1b[0m world", 8, "…", "\x1b[31mhello\x1b[0m w…"},
		{"cut where the style ends", "\x1b[31mhello\x1b[0m world", 6, "…", "\x1b[31mhello\x1b[0m…"},
		{"style starts after the cut", "plain \x1b[31mred\x1b[0m", 4, "…", "pla…"},
		{"style left open stays open", "\x1b[1mbold text", 6, "…", "\x1b[1mbold …"},
		{"style opened after the cut and left open", "abcdef\x1b[31m", 4, "…", "abc…\x1b[31m"},
		{"style changes after the cut", "\x1b[1mabcdef\x1b[0;31mgh", 4, "…", "\x1b[1mabc…\x1b[0m\x1b[31m"},
		{"two styles", "\x1b[1mab\x1b[0m\x1b[3mcdef\x1b[0m", 4, "…", "\x1b[1mab\x1b[0m\x1b[3mc…\x1b[0m"},
		{"zero width keeps the look", "\x1b[31mhello\x1b[0m", 0, "", "\x1b[31m\x1b[0m"},
		{"styled tail", "\x1b[31mhello world\x1b[0m", 8, "\x1b[2m…\x1b[0m", "\x1b[31mhello w\x1b[2m…\x1b[0m"},
		{"styled tail in text left open", "\x1b[31mhello world", 8, "\x1b[2m…\x1b[0m", "\x1b[31mhello w\x1b[2m…\x1b[0m\x1b[31m"},
		{"other sequences after the cut are dropped", "abcdef\x1b[K\x1b]0;title\x07", 4, "…", "abc…"},
		{"other sequences before the cut are kept", "a\x1b[Kbcdef", 4, "…", "a\x1b[Kbc…"},

		// Hyperlinks.
		{"link closed after the tail", link + "click here now" + unlink, 8, "…", link + "click h…" + unlink},
		{"link ends before the cut", link + "ab" + unlink + "cdefgh", 4, "…", link + "ab" + unlink + "c…"},
		{"link and style", "\x1b[4m" + link + "click here" + unlink + "\x1b[0m", 6, "…", "\x1b[4m" + link + "click…" + unlink + "\x1b[0m"},
		{"link left open", link + "click here", 6, "…", link + "click…"},

		// Sequences that were cut short are dropped.
		{"unfinished sequence, fits", "ab\x1b[31", 5, "…", "ab"},
		{"unfinished sequence, cut", "ab\x1b[31\ncdef", 4, "…", "ab\nc…"},
		{"lone escape", "ab\x1b", 5, "…", "ab"},
		{"unfinished title", "abcdef\x1b]0;title", 4, "…", "abc…"},
	}
	for _, tt := range tests {
		got := Truncate(tt.in, tt.width, tt.tail)
		if got != tt.want {
			t.Errorf("%s: Truncate(%q, %d, %q)\n got %q\nwant %q", tt.name, tt.in, tt.width, tt.tail, got, tt.want)
		}
		if w := VisibleWidth(got); w > max(tt.width, 0) {
			t.Errorf("%s: the result %q is %d wide, more than %d", tt.name, got, w, tt.width)
		}
	}
	if got, want := (Metrics{AmbiguousWide: true}).Truncate("αβγδ", 5, "…"), "α …"; got != want {
		t.Errorf("with wide ambiguous characters Truncate gives %q, want %q", got, want)
	}
}

func TestPad(t *testing.T) {
	tests := []struct {
		name                string
		in                  string
		width               int
		right, left, center string
	}{
		{"plain", "ab", 5, "ab   ", "   ab", " ab  "},
		{"even", "ab", 6, "ab    ", "    ab", "  ab  "},
		{"empty", "", 3, "   ", "   ", "   "},
		{"already wide enough", "abc", 3, "abc", "abc", "abc"},
		{"too wide", "abcdef", 3, "abcdef", "abcdef", "abcdef"},
		{"zero width", "ab", 0, "ab", "ab", "ab"},
		{"negative width", "ab", -1, "ab", "ab", "ab"},
		{"wide characters", "漢", 5, "漢   ", "   漢", " 漢  "},
		{"styled", "\x1b[1mab\x1b[0m", 4, "\x1b[1mab\x1b[0m  ", "  \x1b[1mab\x1b[0m", " \x1b[1mab\x1b[0m "},
		{"emoji", "👍🏽", 4, "👍🏽  ", "  👍🏽", " 👍🏽 "},
		{"unfinished sequence", "ab\x1b[31", 4, "ab  ", "  ab", " ab "},
		{"unfinished sequence, wide enough", "abc\x1b", 2, "abc", "abc", "abc"},
	}
	for _, tt := range tests {
		if got := PadRight(tt.in, tt.width); got != tt.right {
			t.Errorf("%s: PadRight(%q, %d) = %q, want %q", tt.name, tt.in, tt.width, got, tt.right)
		}
		if got := PadLeft(tt.in, tt.width); got != tt.left {
			t.Errorf("%s: PadLeft(%q, %d) = %q, want %q", tt.name, tt.in, tt.width, got, tt.left)
		}
		if got := Center(tt.in, tt.width); got != tt.center {
			t.Errorf("%s: Center(%q, %d) = %q, want %q", tt.name, tt.in, tt.width, got, tt.center)
		}
	}
	wide := Metrics{AmbiguousWide: true}
	if got := wide.PadRight("α", 4); got != "α  " {
		t.Errorf("with wide ambiguous characters PadRight gives %q", got)
	}
	if got := wide.PadLeft("α", 4); got != "  α" {
		t.Errorf("with wide ambiguous characters PadLeft gives %q", got)
	}
	if got := wide.Center("α", 5); got != " α  " {
		t.Errorf("with wide ambiguous characters Center gives %q", got)
	}
}

// endLook is the look a string leaves a terminal in.
func endLook(s string) look {
	var l look
	l.applyText(s)
	return l
}

// FuzzTruncate checks, for any string and width: the result fits; it is
// exactly as wide as asked when anything was cut; its text is the start of
// the input's text, then padding, then the tail; it leaves colours, styles
// and the link as the input does; and it holds no unfinished sequence.
func FuzzTruncate(f *testing.F) {
	for i, seed := range fuzzSeeds() {
		f.Add(seed, i*3-2, uint8(i))
	}
	f.Fuzz(func(t *testing.T, s string, width int, pick uint8) {
		width = width%70 - 4
		tails := []string{"…", "", "...", "\x1b[2m…\x1b[0m", "‥"}
		tail := tails[int(pick)%len(tails)]
		m := Metrics{AmbiguousWide: pick >= 128}

		got := m.Truncate(s, width, tail)
		limit := max(width, 0)
		gotWidth := m.VisibleWidth(got)
		if gotWidth > limit && m.VisibleWidth(s) > width {
			t.Fatalf("Truncate(%q, %d, %q) = %q, which is %d wide", s, width, tail, got, gotWidth)
		}
		if hasUnfinished(got) {
			t.Fatalf("Truncate(%q, %d, %q) = %q holds an unfinished sequence", s, width, tail, got)
		}
		if endLook(got) != endLook(s) {
			t.Fatalf("Truncate(%q, %d, %q) = %q leaves the look %+v, the input leaves %+v", s, width, tail, got, endLook(got), endLook(s))
		}
		text, gotText := Strip(s), Strip(got)
		if m.VisibleWidth(s) <= width {
			if gotText != text {
				t.Fatalf("Truncate(%q, %d, %q) = %q changed text that fits", s, width, tail, got)
			}
			if !hasUnfinished(s) && got != s {
				t.Fatalf("Truncate(%q, %d, %q) = %q changed a string that fits", s, width, tail, got)
			}
			return
		}
		if gotWidth != limit {
			t.Fatalf("Truncate(%q, %d, %q) = %q is %d wide, want exactly %d", s, width, tail, got, gotWidth, limit)
		}
		// Take the tail off the end, if it was used, then the padding;
		// what is left is the start of the input's text.
		if rest, ok := strings.CutSuffix(gotText, Strip(tail)); ok && m.VisibleWidth(tail) <= limit {
			gotText = rest
		}
		if kept := strings.TrimRight(gotText, " "); !strings.HasPrefix(text, kept) {
			t.Fatalf("Truncate(%q, %d, %q) = %q: its text %q is not the start of %q", s, width, tail, got, kept, text)
		}
	})
}

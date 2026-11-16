package text

import (
	"strings"
	"testing"
)

// CleanInvisible takes out what a terminal draws as nothing, keeps what
// scripts and emoji need, and says when what it took out could hide text.
func TestCleanInvisible(t *testing.T) {
	for _, c := range []struct {
		name, in, want string
		removed        int
		hiding         bool
	}{
		{"plain ASCII is passed back as it is", "run the tests", "run the tests", 0, false},
		{"a zero-width space in a word", "rm\u200B -rf", "rm -rf", 1, false},
		{"a byte order mark at the start", "\uFEFFhello", "hello", 1, false},
		{"a soft hyphen", "hy\u00ADphen", "hyphen", 1, false},
		{"the direction marks", "a\u200Eb\u200Fc\u061C", "abc", 3, false},
		{"tag characters spell hidden text", "hi\U000E0069\U000E0067\U000E006E", "hi", 3, true},
		{"one tag character still hides", "hi\U000E0041", "hi", 1, true},
		{"a right-to-left override", "echo \u202Etxt.exe", "echo txt.exe", 1, true},
		{"an isolate", "x\u2066y\u2069", "xy", 2, true},
		{"three zero-width characters in a row spell bytes", "a\u200B\u200C\u200Bb", "ab", 3, true},
		{"two in a row do not", "a\u200B\u2060b", "ab", 2, false},
		{"joiners between ASCII letters", "a\u200Db\u200Cc", "abc", 2, false},
		{"a Persian non-joiner stays", "\u0645\u06CC\u200C\u062E", "\u0645\u06CC\u200C\u062E", 0, false},
		{"a Devanagari joiner stays", "\u0915\u094D\u200D\u0937", "\u0915\u094D\u200D\u0937", 0, false},
		{"an emoji family stays", "\U0001F468\u200D\U0001F469\u200D\U0001F467", "\U0001F468\u200D\U0001F469\u200D\U0001F467", 0, false},
		{"a heart on fire: selector, then joiner", "\u2764\uFE0F\u200D\U0001F525", "\u2764\uFE0F\u200D\U0001F525", 0, false},
		{"two joiners in a row lose the second", "\u0645\u200C\u200C\u062E", "\u0645\u200C\u062E", 1, false},
		{"a joiner at the end", "\u0645\u200D", "\u0645", 1, false},
		{"a keycap", "#\uFE0F\u20E3", "#\uFE0F\u20E3", 0, false},
		{"a selector after an ASCII letter", "a\uFE0Fb", "ab", 1, false},
		{"an emoji with bytes hidden in selectors", "\U0001F600\uFE01\uFE02\uFE03\uFE04", "\U0001F600\uFE01", 3, true},
		{"the same with the supplement's selectors", "ok \U0001F600\U000E0101\U000E0102", "ok \U0001F600", 2, false},
		{"a Han variant stays", "\u845B\U000E0100", "\u845B\U000E0100", 0, false},
		{"the flag of England stays", "\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", "\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F", 0, false},
		{"a black flag with tags and no end", "\U0001F3F4\U000E0067\U000E0062 x", "\U0001F3F4 x", 2, true},
		{"a pirate flag stays", "\U0001F3F4\u200D\u2620\uFE0F", "\U0001F3F4\u200D\u2620\uFE0F", 0, false},
		{"non-ASCII text with nothing to take out", "caf\u00E9 \u65E5\u672C", "caf\u00E9 \u65E5\u672C", 0, false},
		{"broken bytes around a zero-width space", "\xe2\u200B\x80\x8b", "\uFFFD\uFFFD", 1, false},
		{"a no-break space before a joiner", "\u0645\u00A0\u200C\u062E", "\u0645\u00A0\u062E", 1, false},
	} {
		got, found := CleanInvisible(c.in)
		if got != c.want || found.Removed != c.removed || found.Hiding != c.hiding {
			t.Errorf("%s: CleanInvisible(%+q) = %+q, %+v; want %+q, %d removed, hiding %v", c.name, c.in, got, found, c.want, c.removed, c.hiding)
		}
		if again, f := CleanInvisible(got); again != got || f.Removed != 0 {
			t.Errorf("%s: cleaning %+q again took out %d more", c.name, got, f.Removed)
		}
	}
	// A long prompt with nothing to take out comes back as the same
	// string, not a copy.
	long := strings.Repeat("plain words ", 1000)
	if got, _ := CleanInvisible(long); got != long {
		t.Errorf("a plain prompt changed")
	}
}

func FuzzCleanInvisible(f *testing.F) {
	f.Add("a\u200Bb")
	f.Add("\U0001F3F4\U000E0067\U000E007F")
	f.Add("\xe2\u200B\x80\x8b") // the space taken out would join the bytes into another
	f.Fuzz(func(t *testing.T, s string) {
		got, found := CleanInvisible(s)
		if found.Removed == 0 && got != s {
			t.Fatalf("nothing taken out, yet %+q became %+q", s, got)
		}
		if again, f := CleanInvisible(got); again != got || f.Removed != 0 {
			t.Fatalf("%+q cleaned to %+q, which cleans again to %+q", s, got, again)
		}
	})
}

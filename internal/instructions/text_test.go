package instructions

import (
	"strings"
	"testing"
)

func TestStripComments(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"none", "a\nb\n", "a\nb\n"},
		{"a line of its own", "a\n<!-- c -->\nb\n", "a\nb\n"},
		{"the whole text", "<!-- c -->\n", ""},
		{"inside a line of prose", "a <!-- inline --> b\n", "a <!-- inline --> b\n"},
		{"over several lines", "<!--\nmulti\nline\n-->\nafter\n", "after\n"},
		{"text after the end is kept", "<!-- c --> tail\n", "tail\n"},
		{"up to three spaces before", "   <!-- c -->\nx", "x"},
		{"four spaces is code", "    <!-- c -->\nx", "    <!-- c -->\nx"},
		{"inside a fenced block", "```\n<!-- keep -->\n```\n", "```\n<!-- keep -->\n```\n"},
		{"inside tildes, and after", "~~~\n<!-- k -->\n~~~\n<!-- gone -->\n", "~~~\n<!-- k -->\n~~~\n"},
		{"never closed", "<!-- never closed\ntext\n", "<!-- never closed\ntext\n"},
		{"two on a line", "<!-- a --> <!-- b -->\nx\n", "x\n"},
		{"two on a line, the second never closed", "<!-- a --> <!-- b\nx\n", "<!-- b\nx\n"},
		{"the shortest comment", "<!-->x\n", "x\n"},
		{"the next shortest", "<!--->\ny\n", "y\n"},
		{"a fence inside a comment is not a fence", "<!-- c\n```\n-->\nx\n<!-- d -->\n", "x\n"},
		{"a second one that never closes", "<!-- a -->\n<!-- b\n", "<!-- b\n"},
		{"quoted", "> <!-- q -->\n", "> <!-- q -->\n"},
		{"what is left may open a fence", "<!-- c --> ```\n<!-- in -->\n```\n", "```\n<!-- in -->\n```\n"},
		{"no newline at the end", "x\n<!-- c -->", "x\n"},
		{"indentation is kept for what is left", "  <!-- c -->  tail\n", "  tail\n"},
		{"many that never close", strings.Repeat("<!-- x\n", 50), strings.Repeat("<!-- x\n", 50)},
	} {
		if got := stripComments(c.in); got != c.want {
			t.Errorf("%s: stripComments(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestStripCommentsLargeInput(t *testing.T) {
	// A megabyte of comment openers and no closer: one search, not one for
	// each line.
	in := strings.Repeat("<!--\n", 200_000)
	if got := stripComments(in); got != in {
		t.Error("an unclosed comment was changed")
	}
}

func TestCleanText(t *testing.T) {
	for _, c := range []struct {
		name     string
		in       string
		want     string
		binary   bool
		repaired bool
	}{
		{"plain", "héllo\n", "héllo\n", false, false},
		{"empty", "", "", false, false},
		{"windows line ends", "a\r\nb\r\n", "a\nb\n", false, false},
		{"old mac line ends", "a\rb", "a\nb", false, false},
		{"a byte-order mark", "\xef\xbb\xbfx", "x", false, false},
		{"latin-1", "caf\xe9", "caf�", false, true},
		{"a run of bad bytes becomes one mark", "a\xff\xfe\xfdb", "a�b", false, true},
		{"a zero byte", "a\x00b", "", true, false},
		{"UTF-16", "\xff\xfea\x00b\x00", "", true, false},
	} {
		got, binary, repaired := cleanText([]byte(c.in))
		if got != c.want || binary != c.binary || repaired != c.repaired {
			t.Errorf("%s: cleanText(%q) = %q, %v, %v", c.name, c.in, got, binary, repaired)
		}
	}
}

func TestCutText(t *testing.T) {
	for _, c := range []struct {
		name string
		in   string
		max  int
		want string
		cut  bool
	}{
		{"short", "abc", 10, "abc", false},
		{"exact", "abcdefghij", 10, "abcdefghij", false},
		{"one over", "abcdefghijk", 10, "abcdefghij", true},
		{"no limit", "abcdefghijk", 0, "abcdefghijk", false},
		{"negative limit", "abcdefghijk", -1, "abcdefghijk", false},
		{"back to a line end near the cut", "0123456789\nabcdef\nKLMNOPQRST\n", 20, "0123456789\nabcdef\n", true},
		{"not back to a line end far from the cut", "ab\ncdefghijklmnopqrstuvwxyz", 20, "ab\ncdefghijklmnopqrs", true},
		{"characters, not bytes", "ééééé", 3, "ééé", true},
		{"wide characters under the limit", "日本語", 3, "日本語", false},
	} {
		got, cut := cutText(c.in, c.max)
		if got != c.want || cut != c.cut {
			t.Errorf("%s: cutText(%q, %d) = %q, %v", c.name, c.in, c.max, got, cut)
		}
	}
}

func TestPlain(t *testing.T) {
	for in, want := range map[string]string{
		"/home/ana/GNOSH.md":  "/home/ana/GNOSH.md",
		"/p/Design Docs/é.md": "/p/Design Docs/é.md",
		"a\nb":                `a\u{a}b`,
		"a\x1b[31mb":          `a\u{1b}[31mb`,
		"a\u202eb":            `a\u{202e}b`,
		"a\u2028b":            `a\u{2028}b`,
		"bad\xffbyte":         `bad\xffbyte`,
		"kept � as it is":     "kept � as it is",
	} {
		if got := plain(in); got != want {
			t.Errorf("plain(%q) = %q, want %q", in, got, want)
		}
	}
}

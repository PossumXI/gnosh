package markdown

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// cutPoints returns places to cut a text of n bytes into pieces, by one of
// several schemes: every byte, small random pieces, larger random pieces,
// or a few big ones.
func cutPoints(r *rand.Rand, n int) []int {
	var cuts []int
	switch scheme := r.Intn(5); scheme {
	case 0:
		for i := 1; i < n; i++ {
			cuts = append(cuts, i)
		}
	case 1, 2:
		most := 6
		if scheme == 2 {
			most = 40
		}
		for i := 1 + r.Intn(most); i < n; i += 1 + r.Intn(most) {
			cuts = append(cuts, i)
		}
	case 3:
		for k := r.Intn(4); k > 0 && n > 1; k-- {
			cuts = append(cuts, 1+r.Intn(n-1))
		}
		slices.Sort(cuts)
		cuts = slices.Compact(cuts)
	}
	return cuts
}

// pieces cuts doc at the given places.
func pieces(doc string, cuts []int) []string {
	var out []string
	last := 0
	for _, c := range cuts {
		out = append(out, doc[last:c])
		last = c
	}
	return append(out, doc[last:])
}

// describe shows the lines around line at for a test failure, with escape
// sequences readable.
func describe(lines []string, at int) string {
	var b strings.Builder
	for i := max(0, at-4); i < len(lines) && i < at+6; i++ {
		fmt.Fprintf(&b, "%3d %q\n", i, lines[i])
	}
	return b.String()
}

// firstDiff returns the index of the first line where a and b differ.
func firstDiff(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

// unsafeIn returns what a rendered line holds that could act on a terminal
// as anything but text, a style or a hyperlink, or "" if it holds nothing
// of the kind. Under NoColor a line may hold no escape sequence at all.
func unsafeIn(line string, p text.Profile) string {
	for rest := line; rest != ""; {
		var tok text.Token
		tok, rest = text.NextToken(rest)
		switch {
		case tok.Kind == text.Plain:
			if !utf8.ValidString(tok.Text) {
				return "bytes that are not UTF-8"
			}
			for _, r := range tok.Text {
				if r < 0x20 || r >= 0x7f && r < 0xa0 {
					return fmt.Sprintf("the control character %U", r)
				}
			}
		case p == text.NoColor:
			return fmt.Sprintf("the escape sequence %q, without colour", tok.Text)
		case !tok.IsSGR() && !tok.IsHyperlink():
			return fmt.Sprintf("the escape sequence %q", tok.Text)
		}
	}
	return ""
}

// streamErr renders doc whole, and then again cut at cuts, and returns ""
// unless something is wrong:
//
//   - the lines returned by the Writes, followed by the lines of the last
//     Live, must be exactly the lines of the whole rendering, however the
//     text was cut (so no line was made final that later text changed);
//   - Close must return what Live showed;
//   - at places along the way (chosen by r, if it is not nil), the final
//     lines so far and Live must be the rendering of the text so far;
//   - Live must change nothing;
//   - no line may be wider than the width, or hold a line ending, or
//     anything else that is not text, a style or a hyperlink.
func streamErr(doc string, opts Options, cuts []int, r *rand.Rand) string {
	want := Render(doc, opts)
	width := opts.Width
	if width < 1 {
		width = defaultWidth
	}
	for i, l := range want {
		if w := opts.Metrics.VisibleWidth(l); w > width && width >= 2 {
			return fmt.Sprintf("line %d is %d cells wide, the width is %d: %q", i, w, width, l)
		}
		if msg := unsafeIn(l, opts.Profile); msg != "" {
			return fmt.Sprintf("line %d holds %s: %q", i, msg, l)
		}
	}
	rd := New(opts)
	var final []string
	fed := 0
	parts := pieces(doc, cuts)
	// Look at Live at about six places along the way, and call it without
	// looking at about as many.
	odds := max(len(parts)/6, 1)
	for _, piece := range parts {
		final = append(final, rd.Write(piece)...)
		fed += len(piece)
		if r == nil {
			continue
		}
		switch r.Intn(odds) {
		case 0:
			rd.Live() // must change nothing
		case 1, 2:
			sofar := Render(doc[:fed], opts)
			got := append(slices.Clone(final), rd.Live()...)
			if !slices.Equal(got, sofar) {
				i := firstDiff(got, sofar)
				return fmt.Sprintf("after %d bytes, the final lines and Live differ from the rendering of the text so far at line %d (%d lines were final)\nfinal+live:\n%swhole:\n%s",
					fed, i, len(final), describe(got, i), describe(sofar, i))
			}
		}
	}
	live := rd.Live()
	got := append(slices.Clone(final), live...)
	if !slices.Equal(got, want) {
		i := firstDiff(got, want)
		return fmt.Sprintf("in pieces the text renders differently at line %d (%d lines were final)\nin pieces:\n%swhole:\n%s",
			i, len(final), describe(got, i), describe(want, i))
	}
	if closed := rd.Close(); !slices.Equal(closed, live) {
		i := firstDiff(closed, live)
		return fmt.Sprintf("Close returned other lines than Live\nlive:\n%sclose:\n%s", describe(live, i), describe(closed, i))
	}
	if again := rd.Close(); len(again) != 0 {
		return fmt.Sprintf("a second Close returned lines: %q", again)
	}
	return ""
}

// trials returns the ways of cutting a text of n bytes that a failing text
// is tried with while it is made smaller: at every byte, and several
// random ways, the same each time.
func trials(n int) [][]int {
	r := rand.New(rand.NewSource(99))
	every := make([]int, 0, n)
	for i := 1; i < n; i++ {
		every = append(every, i)
	}
	out := [][]int{every}
	for k := 0; k < 8; k++ {
		out = append(out, cutPoints(r, n))
	}
	return out
}

// failure returns what is wrong with doc when it is cut in the given one of
// the ways trials returns, or in any of them if which is negative. It
// returns "" if nothing is, and the way that failed.
func failure(doc string, opts Options, which int) (string, int) {
	for k, cuts := range trials(len(doc)) {
		if which >= 0 && k != which {
			continue
		}
		r := rand.New(rand.NewSource(int64(k)))
		if len(doc) > 1500 {
			r = nil // looking at Live along the way costs too much on a long text
		}
		if msg := streamErr(doc, opts, cuts, r); msg != "" {
			return fmt.Sprintf("cuts: %v\n%s", cuts, msg), k
		}
	}
	return "", -1
}

// shrink makes a failing text smaller, by taking pieces out of it for as
// long as it still fails, so that the failure can be read. It gives up
// after twenty seconds and returns what it has.
func shrink(doc string, fails func(string) bool) string {
	deadline := time.Now().Add(20 * time.Second)
	for size := len(doc) / 2; size > 0; {
		smaller := false
		for at := 0; at+size <= len(doc); {
			if time.Now().After(deadline) {
				return doc
			}
			if cand := doc[:at] + doc[at+size:]; fails(cand) {
				doc, smaller = cand, true
			} else {
				at += size
			}
		}
		if !smaller || size > len(doc) {
			size /= 2
		}
	}
	return doc
}

// checkStream is the heart of the package's tests: it runs streamErr on doc
// cut in several random ways. When that fails it reports the smallest
// text it can find that still fails.
func checkStream(t testing.TB, doc string, opts Options, r *rand.Rand, rounds int) {
	t.Helper()
	for round := 0; round < rounds; round++ {
		cuts := cutPoints(r, len(doc))
		msg := streamErr(doc, opts, cuts, r)
		if msg == "" {
			continue
		}
		msg = fmt.Sprintf("cuts: %v\n%s", cuts, msg)
		small := doc
		if _, which := failure(doc, opts, -1); which >= 0 && len(doc) < 100000 {
			small = shrink(doc, func(d string) bool {
				m, _ := failure(d, opts, which)
				return m != ""
			})
			msg, _ = failure(small, opts, which)
		}
		if len(small) > 3000 {
			small = small[:3000] + "... (cut short)"
		}
		if len(msg) > 6000 {
			msg = msg[:6000] + "... (cut short)"
		}
		t.Fatalf("width %d, profile %v, reach %d, table rows %d, hyperlinks %v, highlight %v\ntext: %q\n%s",
			opts.Width, opts.Profile, opts.Reach, opts.TableLive, opts.Hyperlinks, opts.Highlight != nil, small, msg)
	}
}

// profiles are the colour profiles the tests run under.
var profiles = []text.Profile{text.NoColor, text.ANSI16, text.ANSI256, text.TrueColor}

// someOptions picks options for a round of checkStream.
func someOptions(r *rand.Rand) Options {
	widths := []int{2, 3, 5, 8, 13, 20, 33, 40, 60, 80, 120}
	opts := Options{
		Width:      widths[r.Intn(len(widths))],
		Profile:    profiles[r.Intn(len(profiles))],
		Hyperlinks: r.Intn(2) == 0,
	}
	if r.Intn(4) == 0 {
		opts.Reach = 20 + r.Intn(100)
	}
	if r.Intn(4) == 0 {
		opts.TableLive = 1 + r.Intn(8)
	}
	if r.Intn(6) == 0 {
		opts.Metrics.AmbiguousWide = true
	}
	if r.Intn(6) == 0 {
		opts.Highlight = func(lang, code string) []string {
			return []string{"\x1b[32m" + code + "\x1b[0m"}
		}
	}
	return opts
}

// The samples every property test starts from: each construct, and the
// malformed inputs that are known to be awkward.
var samples = []string{
	"",
	"plain text\n",
	"A paragraph\nof two lines.\n\nAnd *another* one with **strong** text, `code`, ~~strike~~ and a [link](http://example.com \"title\").\n",
	"# Heading\n\n## Second ##\n\nSetext\n======\n\nAnother\n---\n",
	"Text\n---\nmore\n===\n",
	"- one\n- two\n  - nested\n  - items\n\n- loose\n\n  second paragraph\n\n1. first\n2. second\n10. tenth\n",
	"- [ ] todo\n- [x] done\n1. [x] numbered\n",
	"```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n",
	"~~~~\n```\nnot the end\n~~~\n~~~~\nafter\n",
	"    indented code\n\n    more of it\n\nparagraph\n",
	"> quote\n> more\n\n> > nested\n> back\nlazy\n",
	"| a | b |\n|---|:-:|\n| 1 | 2 |\n| *x* | `y` |\n\nafter\n",
	"a | b\n--|--\n1 | 2\n3 | 4\nnot a row?\n\nreally not\n",
	"| a |\n| b |\n",
	"***\n\n---\n\n___\n\n* * *\n",
	"<div>\n*html*\n</div>\n\n<!-- comment -->\n\ntext <span>inline</span> <br> more\n",
	"[ref]: http://example.com \"Title\"\n[other]: <http://example.org>\n\nSee [ref] and [text][other] and [missing].\n",
	"Hard  \nbreak and\\\nanother.\n",
	"Escapes \\* \\_ \\` and entities &amp; &lt; &#65; &#x42; &nosuch; &\n",
	"Auto <http://example.com> and <me@example.com> and www.example.com and https://example.com/a_b(c)d. Mail me@example.com.\n",
	"![image](http://example.com/a.png) and ![](b.png) and [![alt](i.png)](http://example.com)\n",
	"unclosed *emphasis and **strong and `code and [link](http://example.com\n",
	"```\nunclosed fence\n",
	"- a\n* b\n+ c\n1. d\n1) e\n",
	"wide: 日本語のテキストは折り返しが必要です。これは長い文です。\n\nemoji 👍🏽 👨\u200d👩\u200d👧 e\u0301 🇯🇵\n",
	"tab\tseparated\n\n\tcode with tab\n",
	"control \x1b[31m chars \x00 and \x7f here\n",
	"a\r\nb\r\n\r\nc\rd\n",
	"1. one\n\n   ```\n   code in item\n   ```\n\n   after code\n2. two\n",
	"> - a\n>   b\n> - c\n\n- > q\n  > r\n",
	"long " + strings.Repeat("word ", 300) + "\n",
	strings.Repeat("x", 500) + "\n",
	"*" + strings.Repeat("never closed ", 100) + "\n",
	"| " + strings.Repeat("col | ", 30) + "\n|" + strings.Repeat("-|", 30) + "\n| " + strings.Repeat("v | ", 30) + "\n",
	"| h1 | h2 |\n|----|----|\n" + strings.Repeat("| some cell text | another cell that is long enough to wrap in a narrow terminal |\n", 40),
	"#\n\n# \n\n#hashtag\n\n####### seven\n",
	"-\n\n- \n\n-  \n  text\n",
	"[a](b) [c](<d e> 'f') [g]() [](h) [i](j\n",
	"**bold *nested* bold** and *em **nested** em* and ***both***\n",
	"`` code with ` tick `` and ``` three ```\n",
	"term\n: not a definition list\n",
	"<a href=\"x\">\n\n</a>\n\n<pre>\n\nraw\n</pre>\n",
	"- a\n\n\n- b\n  \n  c\n",
	"    code\n\ttab code\n  \n    after blank\n",
	"https://example.com/" + strings.Repeat("a", 700) + " end\n",
	"x" + strings.Repeat(" ", 700) + "\n" + strings.Repeat(" ", 700) + "\ny\n",
	// Runs too long to mean anything, and nesting too deep to follow.
	strings.Repeat("`", 300) + " x `y` z " + strings.Repeat("`", 300) + "\n",
	"a " + strings.Repeat("*", 200) + "b* c *d* " + strings.Repeat("_", 150) + "e_\n",
	"a" + strings.Repeat(" ", 200) + "\nb  \nc" + strings.Repeat(" ", 200) + "  \nd\n",
	"# h " + strings.Repeat("#", 300) + "\n\n## i" + strings.Repeat(" ", 200) + "##\n",
	strings.Repeat("> ", 50) + "deep\n" + strings.Repeat(">", 50) + " still\nlazy\n",
	strings.Repeat("- ", 50) + "deep\n\n" + strings.Repeat("1. ", 40) + "numbered\n",
	"- " + strings.Repeat(" ", 200) + "x\n\n-" + strings.Repeat("\t", 100) + "\n",
	"z" + strings.Repeat("\u200b", 400) + "w " + strings.Repeat("\u0301", 400) + " e\n",
	"```\n" + strings.Repeat("\u200b", 300) + "\n" + strings.Repeat("ab\t", 100) + "\n```\n",
	"~~~" + strings.Repeat("i", 300) + "\ncode\n~~~\n\n" + strings.Repeat("=", 300) + "\n",
	"p\n" + strings.Repeat("-", 300) + "\n\nq\n" + strings.Repeat("=", 300) + "\n",
	"| a | b |\n|---|---|\n| " + strings.Repeat("long ", 80) + "| x |\n| y | z |\n",
	"mailto:a@b.co xmpp:a@b.co/r mailto:a@b xmpp:a@b.co/r/s mailto: x m\n",
}

// TestStreamSamples runs the samples through checkStream at many widths.
func TestStreamSamples(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i, doc := range samples {
		for round := 0; round < 12; round++ {
			checkStream(t, doc, someOptions(r), r, 3)
		}
		// And cut after every single byte, in the plainest setting.
		if msg, _ := failure(doc, Options{Width: 30}, -1); msg != "" {
			t.Fatalf("sample %d: %s\ntext: %q", i, msg, doc)
		}
	}
}

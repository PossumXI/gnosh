package markdown

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
	"time"
)

// A gen writes random Markdown from a grammar of the constructs the
// renderer knows, with the mistakes a model makes mixed in.
type gen struct {
	r *rand.Rand
}

// pick returns one of the choices.
func (g *gen) pick(choices ...string) string {
	return choices[g.r.Intn(len(choices))]
}

// words are what prose is made of. Some need care in a terminal: wide
// characters, characters built from several code points, characters that
// mean something in Markdown.
var words = []string{
	"the", "a", "of", "streaming", "renderer", "terminal", "line", "final", "width", "x", "I",
	"well-known", "state-of-the-art", "snake_case_name", "*.go", "_id", "2*3*4", "a|b", "C++",
	"日本語", "テキスト", "中文字符", "한국어", "👍", "👨\u200d👩\u200d👧", "🇯🇵", "é", "e\u0301", "naïve",
	"https://example.com/path?q=1", "www.example.org", "me@example.com", "http://203.0.113.7:8080/x",
	"supercalifragilisticexpialidocious", "1.", "-", "#", ">", "&amp;", "&copy;", "&#x1F600;", "&bogus;",
	"\\*", "\\_", "\\", "<br>", "<span>", "</span>", "<!-- c -->", "!", "[", "]", "(", ")", "`", "``", "*", "**", "_", "~~", "~", "|", "<", ">",
	"\t", "\x1b[31m", "\x00", "\u00a0", "\u200b", "\u2028",
}

// inline returns a run of inline text of about n words.
func (g *gen) inline(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(g.pick(" ", " ", " ", " ", "  ", ""))
		}
		switch g.r.Intn(24) {
		case 0:
			b.WriteString("*" + g.inline(1+g.r.Intn(3)) + "*")
		case 1:
			b.WriteString("**" + g.inline(1+g.r.Intn(3)) + "**")
		case 2:
			b.WriteString("_" + g.inline(1+g.r.Intn(2)) + "_")
		case 3:
			b.WriteString("`" + g.inline(1+g.r.Intn(3)) + "`")
		case 4:
			b.WriteString("[" + g.inline(1+g.r.Intn(3)) + "](" + g.pick("http://example.com", "/rel/path", "<a b>", "x \"title\"", "") + ")")
		case 5:
			b.WriteString("![" + g.inline(g.r.Intn(2)) + "](img.png)")
		case 6:
			b.WriteString("~~" + g.inline(1+g.r.Intn(2)) + "~~")
		case 7:
			b.WriteString("<" + g.pick("http://example.com/a", "mailto:x@example.com", "x@example.com", "not a link", "div class=\"a\"") + ">")
		case 8:
			b.WriteString("[" + g.pick("ref", "Ref", "other", "nope") + "]" + g.pick("", "[]", "[ref]", "[nope]"))
		case 9:
			b.WriteString("***" + g.inline(1) + "***")
		case 10:
			// An opening mark that never closes.
			b.WriteString(g.pick("*", "**", "_", "`", "[", "~~", "<", "![", "``") + g.inline(1))
		default:
			b.WriteString(words[g.r.Intn(len(words))])
		}
	}
	return b.String()
}

// para returns the lines of a paragraph.
func (g *gen) para() []string {
	var lines []string
	for n := 1 + g.r.Intn(3); n > 0; n-- {
		line := g.inline(1 + g.r.Intn(14))
		if g.r.Intn(12) == 0 {
			line = g.inline(60 + g.r.Intn(120)) // a long line, as models write them
		}
		lines = append(lines, strings.TrimLeft(line, " ")+g.pick("", "", "", "", "  ", "\\", " "))
	}
	return lines
}

// block returns the lines of one block. depth limits nesting.
func (g *gen) block(depth int) []string {
	switch k := g.r.Intn(30); {
	case k < 8:
		return g.para()
	case k < 10:
		return []string{strings.Repeat("#", 1+g.r.Intn(7)) + g.pick(" ", " ", "", "\t") + g.inline(g.r.Intn(6)) + g.pick("", "", " #", " ##  ", "#")}
	case k < 11:
		return []string{g.inline(1 + g.r.Intn(5)), g.pick("===", "---", "=", "-", "== ", "--- ", "= =")}
	case k < 14:
		fence := g.pick("```", "~~~", "````", "~~~~~", "``")
		lines := []string{fence + g.pick("", "go", " python title", "c`d", "\tsh")}
		for n := g.r.Intn(6); n > 0; n-- {
			lines = append(lines, g.pick("", "", "\t", "    ", "  ")+g.pick(g.inline(1+g.r.Intn(8)), "# not a heading", "- not a list", "```", "~~~", "| a | b |", strings.Repeat("long", 20+g.r.Intn(40))))
		}
		if g.r.Intn(6) > 0 {
			lines = append(lines, fence+g.pick("", "", " ", "`", " x"))
		}
		return lines
	case k < 15:
		var lines []string
		for n := 1 + g.r.Intn(4); n > 0; n-- {
			lines = append(lines, g.pick("    ", "\t", "     ", "  \t")+g.inline(1+g.r.Intn(6)))
			if g.r.Intn(4) == 0 {
				lines = append(lines, g.pick("", "  ", "    "))
			}
		}
		return lines
	case k < 18 && depth > 0:
		var lines []string
		for n := 1 + g.r.Intn(3); n > 0; n-- {
			for _, l := range g.block(depth - 1) {
				lines = append(lines, g.pick("> ", "> ", ">", " > ", "")+l)
			}
			if n > 1 {
				lines = append(lines, g.pick(">", "> ", ""))
			}
		}
		return lines
	case k < 23 && depth > 0:
		var lines []string
		ordered := g.r.Intn(3) == 0
		num := g.r.Intn(12)
		bullet := g.pick("-", "*", "+")
		for n := 1 + g.r.Intn(4); n > 0; n-- {
			marker := bullet
			if ordered {
				marker = strings.TrimLeft(strings.Repeat("0", g.r.Intn(2))+string(rune('0'+num%10)), " ") + g.pick(".", ".", ")")
				num++
			} else if g.r.Intn(8) == 0 {
				marker = g.pick("-", "*", "+")
			}
			marker += g.pick(" ", " ", " ", "  ", "   ", "\t")
			if g.r.Intn(6) == 0 {
				marker += g.pick("[ ] ", "[x] ", "[X] ", "[ ]", "[y] ")
			}
			indent := strings.Repeat(" ", len(marker))
			first := true
			for parts := 1 + g.r.Intn(2); parts > 0; parts-- {
				for _, l := range g.block(depth - 1) {
					if first {
						lines = append(lines, marker+l)
						first = false
					} else {
						lines = append(lines, g.pick(indent, indent, indent, "", indent+" ")+l)
					}
				}
				if parts > 1 && g.r.Intn(2) == 0 {
					lines = append(lines, "")
				}
			}
			if g.r.Intn(4) == 0 {
				lines = append(lines, "")
			}
		}
		return lines
	case k < 26:
		cols := 1 + g.r.Intn(5)
		row := func(cell func() string) string {
			var b strings.Builder
			b.WriteString(g.pick("| ", "| ", "|", ""))
			for c := 0; c < cols+g.r.Intn(2)-g.r.Intn(2); c++ {
				if c > 0 {
					b.WriteString(g.pick(" | ", " | ", "|"))
				}
				b.WriteString(cell())
			}
			b.WriteString(g.pick(" |", " |", "|", ""))
			return b.String()
		}
		lines := []string{row(func() string { return g.inline(1 + g.r.Intn(2)) })}
		if g.r.Intn(8) > 0 {
			lines = append(lines, row(func() string { return g.pick("---", ":--", "--:", ":-:", "-", "----------") }))
		}
		for n := g.r.Intn(8); n > 0; n-- {
			lines = append(lines, row(func() string { return g.pick(g.inline(g.r.Intn(4)), g.inline(8+g.r.Intn(10)), "a \\| b", "`x|y`", "") }))
		}
		return lines
	case k < 27:
		return []string{g.pick("---", "***", "___", "- - -", "* * *", " ---", "----------", "--", "**", "_ _ _ _")}
	case k < 28:
		return []string{g.pick("<div>", "<div class=\"x\">", "<!-- comment", "<pre>", "<?php", "<!DOCTYPE html>", "<custom-tag>", "</div>", "<span>text</span>", "<script>"),
			g.inline(1 + g.r.Intn(4)), g.pick("</div>", "-->", "</pre>", "?>", "", "</script>")}
	case k < 29:
		return []string{g.pick("[ref]", "[Ref]", "[other]", " [a b]", "[]", "[x") + g.pick(": ", ":", ":  ", " ") + g.pick("http://example.com", "<http://example.org/a b>", "/local", "") + g.pick("", "", " \"Title\"", " 'T'", " (T)", " junk", " \"open")}
	}
	return g.para()
}

// doc returns a whole document.
func (g *gen) doc() string {
	var b strings.Builder
	for n := 1 + g.r.Intn(7); n > 0; n-- {
		for _, l := range g.block(3) {
			b.WriteString(l)
			b.WriteString(g.pick("\n", "\n", "\n", "\n", "\n", "\r\n"))
		}
		if g.r.Intn(5) > 0 {
			b.WriteString("\n")
		}
	}
	doc := b.String()
	// Now and then damage it: drop a stretch, or stop in the middle.
	if g.r.Intn(6) == 0 && len(doc) > 2 {
		a := g.r.Intn(len(doc))
		e := min(len(doc), a+1+g.r.Intn(5))
		doc = doc[:a] + doc[e:]
	}
	if g.r.Intn(6) == 0 && len(doc) > 2 {
		doc = doc[:g.r.Intn(len(doc))]
	}
	return doc
}

// TestStreamGrammar renders random documents whole and in pieces.
func TestStreamGrammar(t *testing.T) {
	docs := 3000
	if testing.Short() || raceDetector {
		docs = 300
	}
	r := rand.New(rand.NewSource(20261113))
	g := gen{r}
	for i := 0; i < docs; i++ {
		doc, opts := g.doc(), someOptions(r)
		start := time.Now()
		checkStream(t, doc, opts, r, 2)
		if took := time.Since(start); took > 2*time.Second {
			t.Logf("document %d (%d bytes, width %d) took %v", i, len(doc), opts.Width, took)
		}
	}
}

// resized renders doc in pieces cut at cuts, changing the width to
// widths[i] once at[i] bytes have been written. It returns the lines, and
// what is wrong with them if something is.
func resized(doc string, opts Options, cuts, at, widths []int) ([]string, string) {
	// The pieces end at every cut and at every resize.
	all := slices.Clone(cuts)
	all = append(all, at...)
	slices.Sort(all)
	all = slices.Compact(all)
	rd := New(opts)
	width := opts.Width
	var out []string
	fed, next := 0, 0
	take := func(lines []string) string {
		for _, l := range lines {
			if w := opts.Metrics.VisibleWidth(l); w > width && width >= 2 {
				return fmt.Sprintf("after %d bytes a line %d cells wide was returned at width %d: %q", fed, w, width, l)
			}
		}
		out = append(out, lines...)
		return ""
	}
	for _, piece := range pieces(doc, all) {
		for next < len(at) && at[next] == fed {
			width = widths[next]
			rd.SetWidth(width)
			next++
		}
		lines := rd.Write(piece)
		fed += len(piece)
		if msg := take(lines); msg != "" {
			return nil, msg
		}
	}
	if msg := take(rd.Close()); msg != "" {
		return nil, msg
	}
	return out, ""
}

// TestStreamResize changes the width in the middle of random documents.
// What comes out must not depend on how the text was cut, only on where
// in the text the width changed; and every line returned after a change
// must fit the new width.
func TestStreamResize(t *testing.T) {
	docs := 1500
	if testing.Short() || raceDetector {
		docs = 150
	}
	r := rand.New(rand.NewSource(77))
	g := gen{r}
	choices := []int{2, 5, 9, 16, 24, 40, 64, 100}
	for i := 0; i < docs; i++ {
		doc, opts := g.doc(), someOptions(r)
		if len(doc) < 2 {
			continue
		}
		var at, widths []int
		for k := 1 + r.Intn(3); k > 0; k-- {
			at = append(at, 1+r.Intn(len(doc)-1))
		}
		slices.Sort(at)
		at = slices.Compact(at)
		for range at {
			widths = append(widths, choices[r.Intn(len(choices))])
		}
		first, msg := resized(doc, opts, cutPoints(r, len(doc)), at, widths)
		if msg != "" {
			t.Fatalf("width %d, resized at %v to %v: %s\ntext: %q", opts.Width, at, widths, msg, doc)
		}
		second, _ := resized(doc, opts, cutPoints(r, len(doc)), at, widths)
		if !slices.Equal(first, second) {
			k := firstDiff(first, second)
			t.Fatalf("width %d, resized at %v to %v: the result depends on how the text was cut, at line %d\ntext: %q\none way:\n%sanother:\n%s",
				opts.Width, at, widths, k, doc, describe(first, k), describe(second, k))
		}
	}
}

// FuzzStream is the same check for whatever bytes the fuzzer comes up
// with, starting from the samples and from documents of the grammar.
func FuzzStream(f *testing.F) {
	for i, s := range samples {
		f.Add(s, int64(i))
	}
	g := gen{rand.New(rand.NewSource(7))}
	for i := 0; i < 60; i++ {
		f.Add(g.doc(), int64(i))
	}
	f.Fuzz(func(t *testing.T, doc string, seed int64) {
		if len(doc) > 20000 {
			t.Skip()
		}
		r := rand.New(rand.NewSource(seed))
		checkStream(t, doc, someOptions(r), r, 2)
	})
}

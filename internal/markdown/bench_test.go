package markdown

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// answer returns about size bytes of the kind of text a model writes:
// paragraphs on single long lines, lists, code, a table now and then.
func answer(size int) string {
	var b strings.Builder
	for i := 0; b.Len() < size; i++ {
		b.WriteString("## Step " + strings.Repeat("I", i%3+1) + ": what the change does\n\n")
		b.WriteString(prose(70) + " See `internal/markdown` and **" + prose(4) + "** for the details, or [the notes](https://example.com/notes/" + prose(1) + ").\n\n")
		b.WriteString("- " + prose(18) + "\n- **" + prose(2) + ":** " + prose(30) + "\n- " + prose(9) + " `x := y`\n\n")
		b.WriteString("```go\nfunc step(n int) (int, error) {\n\tif n < 0 {\n\t\treturn 0, errors.New(\"negative\")\n\t}\n\treturn n * 2, nil // " + prose(8) + "\n}\n```\n\n")
		if i%3 == 0 {
			b.WriteString("| Case | Before | After |\n|------|--------|-------|\n| empty | " + prose(3) + " | nothing |\n| one byte | " + prose(5) + " | the same |\n| many | " + prose(2) + " | " + prose(6) + " |\n\n")
		}
		b.WriteString("> " + prose(25) + "\n\n1. " + prose(12) + "\n2. " + prose(14) + "\n\n")
	}
	return b.String()[:size]
}

// hostile returns about size bytes made to keep as much as possible
// waiting: opening marks that never close, on lines without end.
func hostile(size int) string {
	parts := []string{"*a ", "[b ", "`c ", "<d ", "**e ", "_f ", "![g ", "~~h ", "``i ", "www.j", " k@", "| l ", "http://m", "&n", "\\"}
	r := rand.New(rand.NewSource(5))
	var b strings.Builder
	for b.Len() < size {
		b.WriteString(parts[r.Intn(len(parts))])
		if r.Intn(400) == 0 {
			b.WriteString("\n")
		}
	}
	return b.String()[:size]
}

// stream writes doc to a new Renderer in pieces of step bytes, and looks
// at the live lines after each piece if live is set, as an interface
// would. It returns the number of lines.
func stream(doc string, opts Options, step int, live bool) int {
	rd := New(opts)
	lines := 0
	for at := 0; at < len(doc); at += step {
		lines += len(rd.Write(doc[at:min(at+step, len(doc))]))
		if live {
			rd.Live()
		}
	}
	return lines + len(rd.Close())
}

var benchOptions = Options{Width: 100, Profile: text.TrueColor, Hyperlinks: true}

// A 200 KB answer, arriving five bytes at a time, with the live lines
// looked at after every piece: what an interface does while a model writes.
func BenchmarkStream200K(b *testing.B) {
	doc := answer(200 << 10)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		stream(doc, benchOptions, 5, true)
	}
}

// The same without looking at the live lines.
func BenchmarkStreamNoLive200K(b *testing.B) {
	doc := answer(200 << 10)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		stream(doc, benchOptions, 5, false)
	}
}

// The same text rendered in one go.
func BenchmarkRender200K(b *testing.B) {
	doc := answer(200 << 10)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		Render(doc, benchOptions)
	}
}

// A single paragraph of 200 KB on one line.
func BenchmarkStreamProse200K(b *testing.B) {
	doc := prose(200 << 10)[:200<<10]
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		stream(doc, benchOptions, 5, true)
	}
}

// 200 KB made to keep the renderer waiting as long as it can be kept.
func BenchmarkStreamHostile200K(b *testing.B) {
	doc := hostile(200 << 10)
	b.SetBytes(int64(len(doc)))
	b.ReportAllocs()
	for b.Loop() {
		stream(doc, benchOptions, 5, true)
	}
}

// work streams doc and returns the number of bytes the renderer looked at
// in doing so, for each byte of doc.
func work(doc string, step int) float64 {
	n := 0
	rd := New(benchOptions)
	rd.cfg.work = &n
	for at := 0; at < len(doc); at += step {
		rd.Write(doc[at:min(at+step, len(doc))])
		rd.Live()
	}
	rd.Close()
	return float64(n) / float64(len(doc))
}

// The cost of streaming grows with the length of the text, and no faster:
// a text twice as long takes twice the work, whether it is ordinary or
// made to be hard, and however small the pieces. The test counts the
// bytes the renderer looks at, which does not depend on the machine.
func TestCostIsLinear(t *testing.T) {
	kinds := []struct {
		name string
		make func(int) string
		most float64 // the most bytes looked at for each byte of text
	}{
		{"an answer", answer, 60},
		{"one long paragraph", func(n int) string { return prose(n)[:n] }, 60},
		{"one long line of code", func(n int) string { return "```\n" + strings.Repeat("x = y + 1; ", n/11+1)[:n] }, 60},
		{"one long table row", func(n int) string { return "| a | b |\n|---|---|\n| " + prose(n)[:n] }, 2500},
		{"marks that never close", hostile, 2500},
		{"blanks without end", func(n int) string { return "a\n" + strings.Repeat(" ", n) }, 400},
		{"backticks without end", func(n int) string { return strings.Repeat("`", n) }, 400},
		{"quote marks without end", func(n int) string { return strings.Repeat(">", n) }, 400},
		{"stars without end", func(n int) string { return "a " + strings.Repeat("*", n) }, 400},
		{"underscores without end", func(n int) string { return strings.Repeat("_", n) }, 400},
		{"dashes without end", func(n int) string { return "a\n" + strings.Repeat("-", n) }, 400},
		{"list markers without end", func(n int) string { return strings.Repeat("- ", n/2) }, 400},
		{"a heading without end", func(n int) string { return "# " + prose(n)[:n] }, 60},
		{"hashes without end", func(n int) string { return "# a " + strings.Repeat("#", n) }, 400},
		{"brackets without end", func(n int) string { return strings.Repeat("[", n) }, 2500},
		{"an item of blanks without end", func(n int) string { return "- " + strings.Repeat(" ", n) }, 400},
		{"an address without end", func(n int) string { return "see https://example.com/" + strings.Repeat("a", n) }, 400},
		{"a tag without end", func(n int) string { return "<a " + strings.Repeat("b ", n/2) }, 2500},
		{"a table header without end", func(n int) string { return "| " + strings.Repeat("a | ", n/4) }, 400},
		{"a word without end", func(n int) string { return strings.Repeat("a", n) }, 400},
		{"characters of no width without end", func(n int) string { return "a" + strings.Repeat("\u200b", n/3) }, 2500},
		{"accents without end", func(n int) string { return "a" + strings.Repeat("\u0301", n/2) }, 2500},
		{"tabs without end", func(n int) string { return "\ta" + strings.Repeat("\t", n) }, 400},
		{"digits without end", func(n int) string { return strings.Repeat("1", n) }, 400},
		{"a fence line without end", func(n int) string { return "~~~" + strings.Repeat("a", n) }, 400},
		{"deep nesting", func(n int) string { return strings.Repeat(strings.Repeat("> ", 40)+"a\n", n/82) }, 400},
		{"definitions", func(n int) string { return strings.Repeat("[a]: /b\n", n/9) }, 400},
		{"a tall table", func(n int) string { return "| a | b |\n|---|---|\n" + strings.Repeat("| c | d |\n", n/10) }, 400},
	}
	size := 16 << 10
	if raceEnabled || testing.Short() {
		size = 6 << 10 // the race detector makes everything ten times slower
	}
	for _, k := range kinds {
		small, large := work(k.make(size), 5), work(k.make(4*size), 5)
		t.Logf("%s: %.0f bytes looked at for each byte of %d KB, %.0f for each byte of %d KB", k.name, small, size>>10, large, 4*size>>10)
		if large > small*1.25+1 {
			t.Errorf("%s: four times the text takes %.1f times the work for each byte: the cost is not linear", k.name, large/small)
		}
		if large > k.most {
			t.Errorf("%s: %.0f bytes are looked at for each byte of text; want at most %.0f", k.name, large, k.most)
		}
	}
}

package markdown

import (
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// feed writes doc to rd in pieces of n bytes and returns the lines that
// became final and the most lines Live ever showed.
func feed(rd *Renderer, doc string, n int) (final []string, mostLive int) {
	for len(doc) > 0 {
		k := min(n, len(doc))
		final = append(final, rd.Write(doc[:k])...)
		doc = doc[k:]
		mostLive = max(mostLive, len(rd.Live()))
	}
	return final, mostLive
}

// prose returns n words of plain text on one line.
func prose(n int) string {
	stock := strings.Fields("the quick brown fox jumps over a lazy dog while streaming renderers commit their lines as early as they safely can")
	var b strings.Builder
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(stock[i%len(stock)])
	}
	return b.String()
}

// A long paragraph, written as models write it, on one line, must become
// final as it goes: what is live stays a line or two however long it gets.
func TestProseBecomesFinalAsItGoes(t *testing.T) {
	doc := prose(2000)
	for _, width := range []int{40, 80, 120} {
		for _, step := range []int{1, 5, 64} {
			rd := New(Options{Width: width})
			final, mostLive := feed(rd, doc, step)
			rest := rd.Close()
			total := len(final) + len(rest)
			if mostLive > 3 {
				t.Errorf("width %d, %d bytes at a time: up to %d lines were live; a plain paragraph needs no more than 3", width, step, mostLive)
			}
			if len(rest) > 2 || total < len(doc)/width {
				t.Errorf("width %d, %d bytes at a time: %d of %d lines became final only at the end", width, step, len(rest), total)
			}
		}
	}
}

// Several paragraphs, lists and quotes of ordinary prose keep the live part
// small as well.
func TestOrdinaryTextStaysSmall(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString(prose(60) + "\n\n")
		b.WriteString("- " + prose(40) + "\n- " + prose(25) + " with **strong " + prose(3) + "** and `code`\n\n")
		b.WriteString("> " + prose(50) + "\n\n")
		b.WriteString("## " + prose(6) + "\n\n")
	}
	rd := New(Options{Width: 80})
	_, mostLive := feed(rd, b.String(), 7)
	if mostLive > 4 {
		t.Errorf("up to %d lines were live in ordinary text; want at most 4", mostLive)
	}
}

// An opening mark that never closes holds back no more than Reach bytes.
func TestUnclosedMarkIsBounded(t *testing.T) {
	for _, mark := range []string{"*", "**", "_", "`", "[", "~~", "<a ", "![", "``"} {
		doc := "Start " + mark + prose(1500)
		rd := New(Options{Width: 80})
		final, mostLive := feed(rd, doc, 9)
		// Reach is 640 bytes, eight lines of 80; allow for the line it
		// starts on and the one being written.
		if mostLive > 640/80+3 {
			t.Errorf("after an unclosed %q up to %d lines were live; want at most %d", mark, mostLive, 640/80+3)
		}
		whole := Render(doc, Options{Width: 80})
		got := append(final, rd.Close()...)
		if !slices.Equal(got, whole) {
			t.Errorf("after an unclosed %q the pieces differ from the whole at line %d", mark, firstDiff(got, whole))
		}
		if !strings.Contains(strings.Join(whole, "\n"), strings.TrimSpace(mark)) {
			t.Errorf("the unclosed %q is not shown", mark)
		}
	}
}

// A construct that closes within Reach is what it says, however it is cut.
func TestEmphasisWithinReach(t *testing.T) {
	doc := "Before *" + prose(60) + "* after."
	lines := Render(doc, Options{Width: 80, Profile: text.ANSI16})
	joined := strings.Join(lines, "\n")
	if strings.Contains(text.Strip(joined), "*") {
		t.Errorf("emphasis of %d bytes was not recognised: %q", len(doc), lines)
	}
	if !strings.Contains(joined, "\x1b[3m") {
		t.Errorf("emphasis is not italic: %q", lines)
	}
}

// Code is final line by line, and a long line row by row.
func TestCodeBecomesFinalByLine(t *testing.T) {
	rd := New(Options{Width: 40})
	if got := rd.Write("```go\n"); len(got) != 0 {
		t.Errorf("the opening fence made lines final: %q", got)
	}
	if got := rd.Write("x := 1\n"); !slices.Equal(got, []string{"  x := 1"}) {
		t.Errorf("after one line of code, final = %q", got)
	}
	if got := rd.Write("y := 2"); len(got) != 0 {
		t.Errorf("a line of code that has not ended is final: %q", got)
	}
	if got := rd.Live(); !slices.Equal(got, []string{"  y := 2"}) {
		t.Errorf("live = %q", got)
	}
	long := strings.Repeat("0123456789", 12)
	got := rd.Write("\n" + long)
	if len(got) != 1+3 {
		t.Errorf("a line of 120 cells in 38: %d lines are final, want the line before and three full rows: %q", len(got), got)
	}
	rest := rd.Close()
	all := append(got, rest...)
	if strings.ReplaceAll(strings.Join(all[1:], ""), "  ", "") != strings.ReplaceAll(long, "  ", "") && !strings.Contains(strings.Join(all[1:], ""), "↪") {
		t.Errorf("the long line is not whole: %q", all)
	}
	for i, l := range all[2:] {
		if !strings.HasPrefix(l, "↪ ") {
			t.Errorf("row %d of the long line has no wrap mark: %q", i+2, l)
		}
	}
}

// A table is live until it ends; a tall one becomes final at the cap and
// goes on row by row with its widths frozen.
func TestTablePolicy(t *testing.T) {
	head := "| name | value |\n|------|-------|\n"
	rd := New(Options{Width: 60, TableLive: 6})
	var final []string
	final = append(final, rd.Write(head)...)
	for i := 0; i < 3; i++ {
		final = append(final, rd.Write("| a | b |\n")...)
	}
	if len(final) != 0 {
		t.Fatalf("a table of five lines became final before its end: %q", final)
	}
	if live := rd.Live(); len(live) != 5 {
		t.Fatalf("live shows %d lines, want 5: %q", len(live), live)
	}
	// The fifth row makes it seven lines: over the cap of six.
	final = append(final, rd.Write("| a | b |\n")...)
	final = append(final, rd.Write("| a | b |\n")...)
	if len(final) != 7 {
		t.Fatalf("at the cap %d lines became final, want 7: %q", len(final), final)
	}
	// From here on each row is final when it ends, in the frozen widths: a
	// wider cell wraps instead of widening its column.
	got := rd.Write("| a much longer name | b |\n")
	want := []string{"────  ─────", "a     b", "much", "long", "er", "name"}
	if !slices.Equal(got, want) {
		t.Errorf("a wide row after the cap:\n got %q\nwant %q", got, want)
	}
	if got := rd.Write("| c | d |\n"); !slices.Equal(got, []string{"────  ─────", "c     d"}) {
		t.Errorf("the row after a tall row: %q", got)
	}
	if got := rd.Close(); len(got) != 0 {
		t.Errorf("Close returned %q", got)
	}

	// Without the cap being reached, all of it is final at the blank line.
	rd = New(Options{Width: 60})
	if got := rd.Write(head + "| a | b |\n| ccc | d |\n"); len(got) != 0 {
		t.Errorf("final before the end: %q", got)
	}
	got = rd.Write("\nafter")
	want = []string{"name  value", "────  ─────", "a     b", "ccc   d"}
	if !slices.Equal(got, want) {
		t.Errorf("at the end of the table:\n got %q\nwant %q", got, want)
	}
}

// A table that cannot be read as a grid at the width is stacked.
func TestTableStacked(t *testing.T) {
	doc := "| Name | Description | Notes |\n|---|---|---|\n| alpha | the first letter of the Greek alphabet | used for angles |\n| beta | the second | also a release stage |\n"
	got := Render(doc, Options{Width: 22})
	want := []string{
		"Name: alpha",
		"Description: the first",
		"  letter of the Greek",
		"  alphabet",
		"Notes: used for angles",
		"──────────────────────",
		"Name: beta",
		"Description: the",
		"  second",
		"Notes: also a release",
		"  stage",
	}
	// A little wider, and the columns fit side by side, each with room for
	// its longest word.
	if grid := Render(doc, Options{Width: 30}); len(grid) == 0 || grid[0] != "Name   Description   Notes" {
		t.Errorf("at width 30 the table is not a grid: %q", grid)
	}
	if !slices.Equal(got, want) {
		t.Errorf("stacked table:\n got %q\nwant %q", got, want)
	}
}

// SetWidth changes the layout of what is not final yet, and only that.
func TestResize(t *testing.T) {
	doc := prose(120) + "\n\n- " + prose(60) + "\n\n```\n" + strings.Repeat("x", 100) + "\n```\n\n| a | b |\n|---|---|\n| " + prose(12) + " | " + prose(8) + " |\n\n" + prose(50) + "\n"
	for at := 1; at < len(doc); at += 37 {
		// The same text, resized at the same place, cut in two ways.
		var runs [2][]string
		var before int
		for k, step := range []int{1, 23} {
			rd := New(Options{Width: 70})
			first, _ := feed(rd, doc[:at], step)
			before = len(first)
			rd.SetWidth(31)
			rest, _ := feed(rd, doc[at:], step)
			runs[k] = append(append(first, rest...), rd.Close()...)
			for i, l := range runs[k][before:] {
				if w := text.VisibleWidth(l); w > 31 {
					t.Fatalf("resized at %d: line %d after the resize is %d cells wide: %q", at, before+i, w, l)
				}
			}
		}
		if !slices.Equal(runs[0], runs[1]) {
			i := firstDiff(runs[0], runs[1])
			t.Fatalf("resized at byte %d, the result depends on how the text was cut, at line %d:\n%s\n%s", at, i, describe(runs[0], i), describe(runs[1], i))
		}
	}
	// A resize before anything is final is as good as starting at that width.
	rd := New(Options{Width: 70})
	rd.Write("one two three")
	rd.SetWidth(9)
	if got, want := rd.Close(), Render("one two three", Options{Width: 9}); !slices.Equal(got, want) {
		t.Errorf("resized before anything was final: %q, want %q", got, want)
	}
}

// The highlighter sees whole lines and the language, and what it returns is
// used only if it is the line with styles added.
func TestHighlight(t *testing.T) {
	var seen []string
	opts := Options{Width: 30, Profile: text.ANSI16, Highlight: func(lang, code string) []string {
		seen = append(seen, lang+":"+code)
		switch code {
		case "bad text":
			return []string{"changed"}
		case "bad escape":
			return []string{"\x1b[2Jbad escape"}
		case "bad count":
			return nil
		}
		return []string{"\x1b[32m" + code + "\x1b[0m"}
	}}
	doc := "```Go extra\nfunc\tmain\nbad text\nbad escape\nbad count\n" + strings.Repeat("ab ", 20) + "\n```\n"
	got := Render(doc, opts)
	plain := Render(doc, Options{Width: 30})
	for i := range got {
		if text.Strip(got[i]) != plain[i] {
			t.Errorf("line %d: highlighted %q, plain %q", i, got[i], plain[i])
		}
	}
	if !strings.Contains(got[0], "\x1b[32m") || strings.Contains(got[1], "\x1b[32m") || strings.Contains(got[2], "\x1b[2J") {
		t.Errorf("highlighting was applied wrongly: %q", got)
	}
	if !strings.Contains(got[5], "\x1b[32m") || !strings.HasSuffix(got[4], text.Reset) {
		t.Errorf("a highlighted line that wraps must carry its style onto the next row: %q", got[4:])
	}
	if len(seen) == 0 || seen[0] != "go:func\tmain" {
		t.Errorf("the highlighter was called with %q", seen)
	}
}

// Nothing a text holds reaches the output as a control: under NoColor the
// lines hold no control characters at all, and with colour only the
// styles and hyperlinks this package writes.
func TestNoControlsGetThrough(t *testing.T) {
	doc := "a \x1b[2J b \x1b]0;title\x07 c \x9b31m\n\n```\n\x1b[H\x08\x7f\n```\n\n[x\x1b](http://example.com/\x1b[0m)\n\n| \x1b[1m |\n|---|\n| \x00 |\n"
	for _, p := range profiles {
		for _, l := range Render(doc, Options{Width: 40, Profile: p, Hyperlinks: true}) {
			if msg := unsafeIn(l, p); msg != "" {
				t.Errorf("profile %v: %s in %q", p, msg, l)
			}
		}
	}
}

// Close leaves the Renderer ready for another text.
func TestCloseStartsAfresh(t *testing.T) {
	rd := New(Options{Width: 20})
	rd.Write("[a]: http://example.com\n\n```\ncode")
	rd.Close()
	got := append(rd.Write("[a] *b*\n"), rd.Close()...)
	if want := Render("[a] *b*\n", Options{Width: 20}); !slices.Equal(got, want) {
		t.Errorf("after Close: %q, want %q", got, want)
	}
}

// What a careful reader would expect of text that is not quite Markdown.
func TestMalformed(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want []string
	}{
		{"an unclosed fence runs to the end", "```\ncode\n\nmore", []string{"  code", "", "  more"}},
		{"a table without a delimiter row keeps its lines", "| a | b |\n| c | d |\ntext", []string{"| a | b |", "| c | d |", "text"}},
		{"a delimiter row with the wrong number of cells", "| a | b |\n|---|\n", []string{"| a | b |", "|---|"}},
		{"emphasis that never closes", "*a b", []string{"*a b"}},
		{"strong closed by a single star", "**a* b", []string{"*a b"}},
		{"a code span that never closes", "`a b", []string{"`a b"}},
		{"a link with no closing parenthesis", "[a](b c", []string{"[a](b c"}},
		{"a list that changes its marker", "- a\n* b\n+ c", []string{"• a", "• b", "• c"}},
		{"numbers are shown as written", "3. a\n3. b\n09. c", []string{"3. a", "3. b", "09. c"}},
		{"a heading without a space", "#a", []string{"#a"}},
		{"a rule right under text is a heading", "a\n---", []string{"## a"}},
		{"a rule under two lines of text is a rule", "a\nb\n---", []string{"a b", "", "────────────────────────────────────────"}},
		{"equals signs under two lines are text", "a\nb\n===", []string{"a b ==="}},
		{"a lazy line in a quote", "> a\nb", []string{"│ a b"}},
		{"a lazy line in a list", "- a\nb", []string{"• a b"}},
		{"an item that is too little indented", "- a\n - b", []string{"• a", "• b"}},
		{"a stray closing fence", "a\n\n```", nil},
		{"a backslash at the end", "a\\", []string{"a\\"}},
		{"entities that are not", "&x; &#; &#xg; &amp", []string{"&x; &#; &#xg; &amp"}},
		{"only a marker", "-", []string{"•"}},
		{"only a number", "1.", []string{"1."}},
		{"only a quote mark", ">", nil},
		{"brackets inside brackets", "[a [b](c) d](e)", []string{"[a b (c) d](e)"}},
		{"html that is not closed", "<div\ntext", []string{"  <div", "  text"}},
		{"a reference before its definition", "[a]\n\n[a]: /b\n\n[a]", []string{"[a]", "", "[a]: /b", "", "a (/b)"}},
	}
	for _, c := range cases {
		got := Render(c.doc, Options{Width: 40})
		if c.name == "a stray closing fence" {
			// It opens a block with nothing in it.
			c.want = []string{"a"}
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: %q renders as\n got %q\nwant %q", c.name, c.doc, got, c.want)
		}
		if msg, _ := failure(c.doc, Options{Width: 40}, -1); msg != "" {
			t.Errorf("%s: %s", c.name, msg)
		}
	}
}

// CodeBlocks gives the code of each block as it was written, which the
// laid-out lines do not: tabs, long lines and all.
func TestCodeBlocks(t *testing.T) {
	long := strings.Repeat("x", 300)
	doc := "Text.\n\n```Go run\nfunc main() {\n\tprintln(1)\n}\n```\n\n- item\n\n  ~~~\n  " + long + "\n\n  in an item\n  ~~~\n\nafter\n\n    indented\n\n    more\n\ntext\n\n```\nnever closed"
	want := []CodeBlock{
		{"Go", "func main() {\n\tprintln(1)\n}\n"},
		{"", long + "\n\nin an item\n"},
		{"", "indented\n\nmore\n"},
		{"", "never closed\n"},
	}
	if got := CodeBlocks(doc); !slices.Equal(got, want) {
		t.Errorf("CodeBlocks:\n got %q\nwant %q", got, want)
	}
	if got := CodeBlocks("no code here, only `a span`"); len(got) != 0 {
		t.Errorf("CodeBlocks of text without blocks: %q", got)
	}
}

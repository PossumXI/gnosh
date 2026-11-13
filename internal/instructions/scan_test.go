package instructions

import (
	"strings"
	"testing"
)

func TestScanImports(t *testing.T) {
	for _, c := range []struct {
		name string
		text string
		want []string
	}{
		{"a bare name", "See @README for more", []string{"README"}},
		{"a sentence end", "See @docs/a.md.", []string{"docs/a.md"}},
		{"a list item", "- git workflow @docs/git-instructions.md", []string{"docs/git-instructions.md"}},
		{"home", "@~/notes/x.md", []string{"~/notes/x.md"}},
		{"absolute", "@/abs/x.md", []string{"/abs/x.md"}},
		{"dot and dot-dot", "@./rel.md and @../up.md", []string{"./rel.md", "../up.md"}},
		{"several, with punctuation", "@a.md, @b.md; @c.md! @d.md? @e.md:", []string{"a.md", "b.md", "c.md", "d.md", "e.md"}},
		{"one per line", "@a.md\n@b.md\n", []string{"a.md", "b.md"}},
		{"after a tab", "x\t@tab.md", []string{"tab.md"}},
		{"in brackets", "(@paren.md) and [@link.md](x)", []string{"paren.md", "link.md"}},
		{"a hidden file", "@.hidden", []string{".hidden"}},
		{"starts with a digit or an underscore", "@1st.md @_x.md", []string{"1st.md", "_x.md"}},
		{"a directory", "@dir/ here", []string{"dir/"}},
		{"a possessive", "@docs/x.md's rules", []string{"docs/x.md"}},
		{"other scripts", "@日本語/ファイル.md", []string{"日本語/ファイル.md"}},
		{"a space that is not ASCII ends it", "@x.md\u00a0next", []string{"x.md"}},
		{"bold after", "see @bold.md**", []string{"bold.md"}},
		{"a heading", "# Title @h.md", []string{"h.md"}},
		{"a table row", "| @cell.md | x |", []string{"cell.md"}},
		{"a quotation", "> quoted @q.md", []string{"q.md"}},
		{"an indented line is not code", "text\n\n    @indented.md\n", []string{"indented.md"}},

		{"an escaped space", `@Design\ Docs/api.md next`, []string{"Design Docs/api.md"}},
		{"an escaped bracket and dot", `@a\(1\)/b\.`, []string{"a(1)/b."}},
		{"a backslash at the end of a line", "@path\\\nnext", []string{"path"}},
		{"a backslash at the very end", `@path\`, []string{"path"}},

		{"an e-mail address", "mail jae@example.com", nil},
		{"a login", "git@example.com:foo/bar", nil},
		{"after a letter", "user@docs/d.md", nil},
		{"quoted", `"@quoted.md" and @"quoted.md" and '@q.md'`, nil},
		{"escaped", `\@escaped.md`, nil},
		{"bold before", "**@bold.md**", nil},
		{"alone", "@", nil},
		{"then a space", "@ space", nil},
		{"doubled", "@@x", nil},
		{"only dots", "@... and @.", nil},
		{"a dash", "@-x", nil},
		{"after a backtick", "`a`@x.md", nil},

		{"a code span", "use `@not/an/import.md` here", nil},
		{"a code span of two backticks", "`` a ` @x.md `` @y.md", []string{"y.md"}},
		{"a backtick with no partner", "` alone @x.md", []string{"x.md"}},
		{"an escaped backtick opens nothing", "\\` @x.md `", []string{"x.md"}},
		{"an escaped backtick still closes", "`code \\` @x.md", []string{"x.md"}},
		{"a code span over two lines", "a `code\n@span.md` b", nil},
		{"no code span across a blank line", "a `code\n\n@para.md` b", []string{"para.md"}},
		{"no code span across list items", "- a ` b\n- @item.md\n- c ` d", []string{"item.md"}},
		{"no code span across a heading", "# a ` b\n@after.md ` c", []string{"after.md"}},
		{"a code span inside a list item's later lines", "- a `b\n  @in.md` c\n- @out.md", []string{"out.md"}},

		{"a fenced block", "```\n@in/fence.md\n```\n@after.md", []string{"after.md"}},
		{"a fenced block with a language", "```sh\ncat @x.md\n```", nil},
		{"tildes", "~~~\n@in/tilde.md\n~~~", nil},
		{"a longer fence holds a shorter one", "````\n```\n@x.md\n```\n````\n@y.md", []string{"y.md"}},
		{"a tilde fence is not closed by backticks", "~~~\n```\n@x.md\n~~~\n@y.md", []string{"y.md"}},
		{"a fence that never closes", "```\nnever closed\n@x.md", nil},
		{"an indented fence", "- item\n\n  ```\n  @x.md\n  ```\n  @y.md", []string{"y.md"}},
		{"a quoted fence", "> ```\n> @x.md\n> ```\n> @y.md", []string{"y.md"}},
		{"three backticks on one line are a span", "```@x.md``` @y.md", []string{"y.md"}},
		{"a fence line closes the paragraph before it", "a ` b\n```\n@x.md\n```\n@c.md ` d", []string{"c.md"}},
	} {
		same(t, c.name, scanImports(c.text), c.want)
	}
}

func TestScanImportsLargeInputs(t *testing.T) {
	// Rows of backticks of every length, none with a partner: the work
	// must not grow with the square of the size.
	var b strings.Builder
	for n := 1; b.Len() < 1<<20; n++ {
		b.WriteString(strings.Repeat("`", n))
		b.WriteString(" @x.md ")
	}
	if got := scanImports(b.String()); len(got) == 0 {
		t.Error("no imports found among lone backticks")
	}
	// One import per line, many lines.
	many := strings.Repeat("@a.md\n", 100_000)
	if got := scanImports(many); len(got) != 100_000 {
		t.Errorf("%d imports, want 100000", len(got))
	}
}

func TestClassify(t *testing.T) {
	for line, want := range map[string]lineKind{
		"":                lineBlank,
		"   \t":           lineBlank,
		"> ":              lineBlank,
		"plain text":      linePlain,
		"#hashtag":        linePlain,
		"####### seven":   linePlain,
		"*emphasis* x":    linePlain,
		"-dash":           linePlain,
		"1.5 is a number": linePlain,
		"2026-11-13":      linePlain,
		"# heading":       lineAlone,
		"###":             lineAlone,
		"---":             lineAlone,
		"* * *":           lineAlone,
		"===":             lineAlone,
		"___":             lineAlone,
		"| a | b |":       lineAlone,
		"- item":          lineStarts,
		"  * item":        lineStarts,
		"+ item":          lineStarts,
		"1. item":         lineStarts,
		"12) item":        lineStarts,
		"> - quoted item": lineStarts,
	} {
		if got := classify(line); got != want {
			t.Errorf("classify(%q) = %d, want %d", line, got, want)
		}
	}
}

func TestFences(t *testing.T) {
	for line, want := range map[string]bool{
		"```":         true,
		"```go":       true,
		"~~~~ text":   true,
		"   ```":      true,
		"        ```": true,
		"> ```":       true,
		"``":          false,
		"``` a ` b":   false,
		"text ```":    false,
		"~~ ~":        false,
	} {
		if _, got := openFence(line); got != want {
			t.Errorf("openFence(%q) = %v, want %v", line, got, want)
		}
	}
	f := fence{ch: '`', n: 4}
	for line, want := range map[string]bool{
		"````":      true,
		"`````  \n": true,
		"  ````":    true,
		"```":       false,
		"~~~~":      false,
		"```` go":   false,
	} {
		if got := f.closedBy(line); got != want {
			t.Errorf("closedBy(%q) = %v, want %v", line, got, want)
		}
	}
}

package markdown

import (
	"encoding/json"
	"fmt"
	"html"
	"math/rand"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// A specExample is one numbered example of a Markdown specification: some
// Markdown and the HTML it stands for.
type specExample struct {
	Markdown string `json:"markdown"`
	HTML     string `json:"html"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

// The specifications whose examples are under testdata/spec. NOTICE.md
// there says where they come from and on what terms.
var specFiles = []struct{ name, file string }{
	{"CommonMark 0.31.2", "testdata/spec/commonmark-0.31.2.json"},
	{"GFM 0.29", "testdata/spec/gfm-0.29.json"},
}

// specSkips are the examples whose text the renderer shows in another form
// than the HTML has it, so that the words cannot be compared. Each has its
// reason. They still run through every other check.
var specSkips = map[string]string{
	"CommonMark 0.31.2/25": "names of HTML5 character references that HTML 4 does not have are shown as written",
	"GFM 0.29/321":         "names of HTML5 character references that HTML 4 does not have are shown as written",
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// lettersOf returns the letters and digits of s, without anything between
// them.
func lettersOf(s string) []rune {
	var out []rune
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, r)
		}
	}
	return out
}

// lostAt returns the index of the first letter of want that got does not
// hold at or after the place of the letter before it, or -1 if got holds
// all the letters of want in their order.
func lostAt(want, got []rune) int {
	at := 0
	for i, r := range want {
		for at < len(got) && got[at] != r {
			at++
		}
		if at == len(got) {
			return i
		}
		at++
	}
	return -1
}

// TestSpecExamples runs every example of the CommonMark and GitHub Flavored
// Markdown specifications through the renderer. The HTML an example stands
// for is not what this package makes, but it says what the text of the
// example is, and so the test can check that none of it is lost: the
// letters and digits of the HTML's text must all be in the rendered
// lines, in their order. (The lines may hold more: an address the HTML
// keeps in an attribute, a label, a marker.) Each example is also
// rendered in pieces, which checks that nothing panics and that the
// pieces do not matter.
func TestSpecExamples(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, spec := range specFiles {
		data, err := os.ReadFile(spec.file)
		if err != nil {
			t.Fatalf("reading the examples of %s: %v", spec.name, err)
		}
		var examples []specExample
		if err := json.Unmarshal(data, &examples); err != nil {
			t.Fatalf("the examples of %s are not the JSON expected: %v", spec.name, err)
		}
		if len(examples) < 600 {
			t.Fatalf("%s has only %d examples; the file is not whole", spec.name, len(examples))
		}
		for _, ex := range examples {
			id := fmt.Sprintf("%s/%d", spec.name, ex.Example)
			for _, width := range []int{80, 17} {
				checkStream(t, ex.Markdown, Options{Width: width}, r, 2)
			}
			if _, skip := specSkips[id]; skip {
				continue
			}
			want := lettersOf(html.UnescapeString(htmlTag.ReplaceAllString(ex.HTML, "")))
			lines := Render(ex.Markdown, Options{Width: 100000})
			if i := lostAt(want, lettersOf(strings.Join(lines, "\n"))); i >= 0 {
				t.Errorf("%s (%s): text is lost, from %q on\nmarkdown: %q\nhtml: %q\nrendered: %q",
					id, ex.Section, string(want[i:min(i+12, len(want))]), ex.Markdown, ex.HTML, lines)
			}
		}
	}
}

// A styled is a letter or digit of a text and the inline styles it has.
type styled struct {
	r     rune
	marks string // any of "e" emphasis, "s" strong, "d" struck out, "c" code, "a" link
}

// styledHTML reads the letters and digits out of the HTML of an example,
// each with the styles of the tags it stands in.
func styledHTML(src string) []styled {
	var out []styled
	depth := map[string]int{}
	names := map[string]string{"em": "e", "strong": "s", "del": "d", "code": "c", "a": "a"}
	marks := func() string {
		var b strings.Builder
		for _, m := range []string{"e", "s", "d", "c", "a"} {
			if depth[m] > 0 {
				b.WriteString(m)
			}
		}
		return b.String()
	}
	for src != "" {
		if src[0] == '<' {
			end := strings.IndexByte(src, '>')
			if end < 0 {
				break
			}
			tag := strings.Fields(strings.Trim(src[1:end], "/") + " ")
			if len(tag) > 0 {
				if m, ok := names[tag[0]]; ok {
					if src[1] == '/' {
						depth[m]--
					} else {
						depth[m]++
					}
				}
			}
			src = src[end+1:]
			continue
		}
		end := strings.IndexByte(src, '<')
		if end < 0 {
			end = len(src)
		}
		for _, r := range html.UnescapeString(src[:end]) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				out = append(out, styled{r, marks()})
			}
		}
		src = src[end:]
	}
	return out
}

// styledLines reads the letters and digits out of rendered lines, each
// with the styles the escape sequences before it turn on. The lines must
// have been rendered with specTheme.
func styledLines(lines []string) []styled {
	var out []styled
	for _, line := range lines {
		on := map[string]bool{}
		for rest := line; rest != ""; {
			var tok text.Token
			tok, rest = text.NextToken(rest)
			if tok.IsSGR() {
				for _, p := range strings.Split(tok.Text[2:len(tok.Text)-1], ";") {
					switch p {
					case "", "0":
						on = map[string]bool{}
					case "3":
						on["e"] = true
					case "1":
						on["s"] = true
					case "9":
						on["d"] = true
					case "7":
						on["c"] = true
					case "4":
						on["a"] = true
					case "2":
						on["h"] = true
					}
				}
				continue
			}
			if tok.Kind != text.Plain {
				continue
			}
			var b strings.Builder
			for _, m := range []string{"e", "s", "d", "c", "a"} {
				if on[m] {
					b.WriteString(m)
				}
			}
			for _, r := range tok.Text {
				// Raw HTML is shown as text; in the HTML of the example it
				// is tags, and no text at all.
				if (unicode.IsLetter(r) || unicode.IsDigit(r)) && !on["h"] {
					out = append(out, styled{r, b.String()})
				}
			}
		}
	}
	return out
}

// specTheme gives each inline style one attribute that styledLines can
// tell apart, and nothing else any style.
func specTheme() Theme {
	var plain text.Style
	t := DefaultTheme()
	t = Theme{Bullets: t.Bullets, QuoteBar: t.QuoteBar, RuleChar: t.RuleChar, WrapMark: t.WrapMark}
	t.Emphasis, t.Strong, t.Strikethrough = plain.Italic(), plain.Bold(), plain.Strikethrough()
	t.Code, t.CodeBlock, t.Link = plain.Reverse(), plain.Reverse(), plain.Underline()
	t.HTML = plain.Dim()
	return t
}

// The sections of the specifications whose examples are about inline
// styles alone, so that what the renderer makes of them can be compared
// with the HTML letter by letter.
var inlineSections = map[string]bool{
	"Emphasis and strong emphasis": true, "Strikethrough (extension)": true, "Code spans": true,
	"Backslash escapes": true, "Entity and numeric character references": true, "Inlines": true,
	"Hard line breaks": true, "Soft line breaks": true, "Textual content": true,
	"Autolinks": true, "Autolinks (extension)": true,
}

// inlineSkips are the examples of those sections where this package means
// to differ from the HTML, each with its reason.
var inlineSkips = map[string]string{
	"CommonMark 0.31.2/23":  "a link reference definition is shown, and gives its label an address only from there on",
	"CommonMark 0.31.2/33":  "a link reference definition is shown, and gives its label an address only from there on",
	"GFM 0.29/319":          "a link reference definition is shown, and gives its label an address only from there on",
	"GFM 0.29/329":          "a link reference definition is shown, and gives its label an address only from there on",
	"CommonMark 0.31.2/608": "an address without angle brackets is a link, as in GitHub Flavored Markdown",
	"CommonMark 0.31.2/611": "an address without angle brackets is a link, as in GitHub Flavored Markdown",
	"CommonMark 0.31.2/612": "an address without angle brackets is a link, as in GitHub Flavored Markdown",
	"GFM 0.29/617":          "an address without angle brackets is a link: the example shows GFM without its autolink extension",
	"GFM 0.29/620":          "an address without angle brackets is a link: the example shows GFM without its autolink extension",
	"GFM 0.29/621":          "an address without angle brackets is a link: the example shows GFM without its autolink extension",
}

// TestSpecInline checks the inline rules against the specifications: in
// the examples about emphasis, code spans, escapes, references and
// autolinks, every letter must have exactly the styles the HTML gives it.
func TestSpecInline(t *testing.T) {
	opts := Options{Width: 100000, Profile: text.ANSI16, Hyperlinks: true, Theme: specTheme()}
	for _, spec := range specFiles {
		data, err := os.ReadFile(spec.file)
		if err != nil {
			t.Fatal(err)
		}
		var examples []specExample
		if err := json.Unmarshal(data, &examples); err != nil {
			t.Fatal(err)
		}
		checked := 0
		for _, ex := range examples {
			id := fmt.Sprintf("%s/%d", spec.name, ex.Example)
			if !inlineSections[ex.Section] || inlineSkips[id] != "" || specSkips[id] != "" {
				continue
			}
			checked++
			want, got := styledHTML(ex.HTML), styledLines(Render(ex.Markdown, opts))
			if !slices.Equal(want, got) {
				t.Errorf("%s (%s)\nmarkdown: %q\nhtml: %q\n got: %v\nwant: %v", id, ex.Section, ex.Markdown, ex.HTML, got, want)
			}
		}
		if checked < 200 {
			t.Errorf("%s: only %d examples were checked", spec.name, checked)
		}
	}
}

// blocksOfHTML reads the block structure out of the HTML of an example, in
// the words the renderer's own trace uses: the blocks as they open, and
// the quotes and list items as they close. Text that stands directly in a
// list item, as it does in a tight list, counts as a paragraph.
func blocksOfHTML(src string) []string {
	var out []string
	var open []string // the containers that are open
	bare := false     // text has been seen directly in the innermost list item
	names := map[string]string{"p": "<p>", "pre": "<code>", "hr": "<hr>", "table": "<table>",
		"h1": "<h1>", "h2": "<h2>", "h3": "<h3>", "h4": "<h4>", "h5": "<h5>", "h6": "<h6>"}
	depth := 0 // inside a leaf block, where tags are inline
	for src != "" {
		if src[0] != '<' {
			end := strings.IndexByte(src, '<')
			if end < 0 {
				end = len(src)
			}
			if strings.TrimSpace(src[:end]) != "" && depth == 0 && len(open) > 0 && open[len(open)-1] == "item" && !bare {
				out = append(out, "<p>")
				bare = true
			}
			src = src[end:]
			continue
		}
		end := strings.IndexByte(src, '>')
		if end < 0 {
			break
		}
		closing := src[1] == '/'
		fields := strings.Fields(strings.Trim(src[1:end], "/") + " ")
		src = src[end+1:]
		if len(fields) == 0 {
			continue
		}
		switch tag := fields[0]; {
		case tag == "blockquote" || tag == "li":
			kind := map[string]string{"blockquote": "quote", "li": "item"}[tag]
			if closing {
				out = append(out, "</"+kind+">")
				open = open[:len(open)-1]
			} else {
				out = append(out, "<"+kind+">")
				open = append(open, kind)
			}
			bare = false
		case names[tag] != "":
			if tag == "hr" {
				out = append(out, "<hr>")
			} else if closing {
				depth--
			} else {
				if depth == 0 {
					out = append(out, names[tag])
				}
				depth++
			}
			bare = false
		case tag == "ul" || tag == "ol":
			bare = false
		case depth == 0 && len(open) > 0 && open[len(open)-1] == "item" && !bare && !closing:
			// An inline tag directly in a list item: its text is a paragraph.
			out = append(out, "<p>")
			bare = true
		}
	}
	return out
}

// The sections of the specifications that are about block structure and
// hold no raw HTML, so that the blocks the renderer finds can be compared
// with the HTML's.
var blockSections = map[string]bool{
	"Tabs": true, "Precedence": true, "Thematic breaks": true, "ATX headings": true, "Setext headings": true,
	"Indented code blocks": true, "Fenced code blocks": true, "Paragraphs": true, "Blank lines": true,
	"Tables (extension)": true, "Block quotes": true, "List items": true, "Task list items (extension)": true, "Lists": true,
}

// blockSkips are the examples of those sections where this package means
// to differ from the HTML, each with its reason.
var blockSkips = map[string]string{
	"CommonMark 0.31.2/81": "only a single line can become a setext heading",
	"CommonMark 0.31.2/82": "only a single line can become a setext heading",
	"CommonMark 0.31.2/95": "only a single line can become a setext heading",
	"GFM 0.29/51":          "only a single line can become a setext heading",
	"GFM 0.29/52":          "only a single line can become a setext heading",
	"GFM 0.29/65":          "only a single line can become a setext heading",
}

// TestSpecBlocks checks the block structure against the specifications: in
// the examples about blocks, the renderer must find the same paragraphs,
// headings, code blocks, quotes, list items, rules and tables, nested the
// same way, as the HTML shows.
func TestSpecBlocks(t *testing.T) {
	for _, spec := range specFiles {
		data, err := os.ReadFile(spec.file)
		if err != nil {
			t.Fatal(err)
		}
		var examples []specExample
		if err := json.Unmarshal(data, &examples); err != nil {
			t.Fatal(err)
		}
		checked := 0
		for _, ex := range examples {
			id := fmt.Sprintf("%s/%d", spec.name, ex.Example)
			if !blockSections[ex.Section] || blockSkips[id] != "" {
				continue
			}
			checked++
			var trace []string
			rd := New(Options{Width: 100000})
			rd.st.trace = &trace
			rd.Write(ex.Markdown)
			rd.st.feed(&rd.cfg, "", true)
			// Raw HTML and link reference definitions are blocks to the
			// renderer, which shows them; the HTML has no element for them.
			got := slices.DeleteFunc(trace, func(e string) bool { return e == "<html>" || e == "<def>" })
			want := blocksOfHTML(ex.HTML)
			if !slices.Equal(want, got) {
				t.Errorf("%s (%s)\nmarkdown: %q\nhtml: %q\n got: %v\nwant: %v", id, ex.Section, ex.Markdown, ex.HTML, got, want)
			}
		}
		if checked < 150 {
			t.Errorf("%s: only %d examples were checked", spec.name, checked)
		}
	}
}

package markdown

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// goldenOut, when set, is a directory the golden test writes what it got
// to, instead of comparing. Tests run on another machine whose copy of the
// tree mirrors this one, so new golden files are written outside the tree
// and copied back:
//
//	go test ./internal/markdown -run TestGolden -args -golden-out=/tmp/golden
var goldenOut = flag.String("golden-out", "", "write golden files to this directory instead of comparing")

// marked rewrites a rendered line so that its styles can be read: every
// escape sequence becomes a mark between « and ».
//
//	«bold #f0257f»   the styles a sequence turns on: bold, dim, italic,
//	                 under, strike, reverse, and a foreground colour as
//	                 #rrggbb, c0 to c255 (the 256-colour palette) or a0 to
//	                 a15 (the ANSI colours); "bg:" before a background
//	«/»              everything off
//	«@address»       a hyperlink to the address begins
//	«/@»             the hyperlink ends
//	«?...»           any other sequence, quoted: there should be none
func marked(line string) string {
	var b strings.Builder
	for rest := line; rest != ""; {
		var tok text.Token
		tok, rest = text.NextToken(rest)
		switch {
		case tok.Kind == text.Plain:
			b.WriteString(tok.Text)
		case tok.IsSGR():
			b.WriteString("«" + sgrNames(tok.Text[2:len(tok.Text)-1]) + "»")
		case tok.IsHyperlink():
			body := strings.TrimSuffix(strings.TrimSuffix(strings.TrimPrefix(tok.Text, "\x1b]8;"), "\x1b\\"), "\x07")
			_, url, _ := strings.Cut(body, ";")
			if url == "" {
				b.WriteString("«/@»")
			} else {
				b.WriteString("«@" + url + "»")
			}
		default:
			b.WriteString("«?" + strconv.Quote(tok.Text) + "»")
		}
	}
	return b.String()
}

// sgrNames spells out the parameters of an SGR sequence.
func sgrNames(params string) string {
	if params == "" || params == "0" {
		return "/"
	}
	p := strings.Split(params, ";")
	var names []string
	for i := 0; i < len(p); i++ {
		n, _ := strconv.Atoi(p[i])
		switch {
		case n == 0:
			names = append(names, "/")
		case n == 1:
			names = append(names, "bold")
		case n == 2:
			names = append(names, "dim")
		case n == 3:
			names = append(names, "italic")
		case n == 4:
			names = append(names, "under")
		case n == 7:
			names = append(names, "reverse")
		case n == 9:
			names = append(names, "strike")
		case (n == 38 || n == 48) && i+2 < len(p) && p[i+1] == "5":
			names = append(names, map[int]string{38: "", 48: "bg:"}[n]+"c"+p[i+2])
			i += 2
		case (n == 38 || n == 48) && i+4 < len(p) && p[i+1] == "2":
			var rgb [3]int
			for k := range rgb {
				rgb[k], _ = strconv.Atoi(p[i+2+k])
			}
			names = append(names, fmt.Sprintf("%s#%02x%02x%02x", map[int]string{38: "", 48: "bg:"}[n], rgb[0], rgb[1], rgb[2]))
			i += 4
		case n >= 30 && n <= 37:
			names = append(names, "a"+strconv.Itoa(n-30))
		case n >= 90 && n <= 97:
			names = append(names, "a"+strconv.Itoa(n-90+8))
		case n >= 40 && n <= 47:
			names = append(names, "bg:a"+strconv.Itoa(n-40))
		case n >= 100 && n <= 107:
			names = append(names, "bg:a"+strconv.Itoa(n-100+8))
		default:
			names = append(names, "sgr"+p[i])
		}
	}
	return strings.Join(names, " ")
}

// goldenVariants are the settings every golden case is rendered in: three
// widths without colour, where the layout is easiest to read, and each of
// the colour profiles at one width. The last one also has hyperlinks on.
var goldenVariants = []struct {
	name string
	opts Options
}{
	{"width 80, no colour", Options{Width: 80}},
	{"width 48, no colour", Options{Width: 48}},
	{"width 24, no colour", Options{Width: 24}},
	{"width 48, 16 colours", Options{Width: 48, Profile: text.ANSI16}},
	{"width 48, 256 colours", Options{Width: 48, Profile: text.ANSI256}},
	{"width 48, true colour, hyperlinks", Options{Width: 48, Profile: text.TrueColor, Hyperlinks: true}},
}

// goldenText renders src in every variant and returns the result in the
// form the golden files have.
func goldenText(src string) string {
	var b strings.Builder
	for _, v := range goldenVariants {
		fmt.Fprintf(&b, "=== %s ===\n", v.name)
		for _, line := range Render(src, v.opts) {
			b.WriteString(marked(line))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// TestGolden renders each Markdown file under testdata/golden and compares
// the result with the .golden file beside it. See marked for how styles
// are written there.
func TestGolden(t *testing.T) {
	sources, err := filepath.Glob("testdata/golden/*.md")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no golden cases found under testdata/golden: %v", err)
	}
	if *goldenOut != "" {
		if err := os.MkdirAll(*goldenOut, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, source := range sources {
		name := strings.TrimSuffix(filepath.Base(source), ".md")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			got := goldenText(string(src))
			if *goldenOut != "" {
				if err := os.WriteFile(filepath.Join(*goldenOut, name+".golden"), []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(strings.TrimSuffix(source, ".md") + ".golden")
			if err != nil {
				t.Fatalf("reading the golden file: %v", err)
			}
			if got == string(want) {
				return
			}
			gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
			i := firstDiff(gl, wl)
			section := ""
			for k := min(i, len(gl)-1); k >= 0; k-- {
				if strings.HasPrefix(gl[k], "=== ") {
					section = gl[k]
					break
				}
			}
			t.Errorf("%s differs from its golden file at line %d, in %s\n got: %s\nwant: %s",
				name, i+1, section, at(gl, i), at(wl, i))
		})
	}
}

// at returns line i of lines, quoted, or a note that there is none.
func at(lines []string, i int) string {
	if i >= len(lines) {
		return "(no such line)"
	}
	return strconv.Quote(lines[i])
}

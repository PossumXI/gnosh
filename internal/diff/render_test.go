package diff

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// plain renders a change without colour and returns the lines joined, each
// on a line of its own, for comparing with what a test expects.
func plain(d core.Display, opts RenderOptions) string {
	opts.Profile = text.NoColor
	return strings.Join(Render(d, opts), "\n")
}

// rows writes what a test expects more readably: the lines of a raw string,
// without the newline that follows its opening quote.
func rows(s string) string {
	return strings.TrimPrefix(s, "\n")
}

// Characters made of several code points, written out so that the joiners,
// selectors and tags inside them can be seen.
const (
	// family is a man, a woman and a girl: three emoji held together by
	// zero-width joiners.
	family = "\U0001f468\u200d\U0001f469\u200d\U0001f467"
	// japan is the flag of Japan: two regional indicators.
	japan = "\U0001f1ef\U0001f1f5"
	// keycap is a digit in a keycap: the digit, a selector and the cap.
	keycap = "1\ufe0f\u20e3"
	// plane is an airplane asked for as an emoji, with a selector.
	plane = "\u2708\ufe0f"
	// scotland is the flag of Scotland: a black flag and six tags.
	scotland = "\U0001f3f4\U000e0067\U000e0062\U000e0073\U000e0063\U000e0074\U000e007f"
	// conjunct is a Devanagari half letter, which needs its joiner.
	conjunct = "\u0928\u094d\u200d"
)

func TestRender(t *testing.T) {
	tests := []struct {
		name string
		d    core.Display
		opts RenderOptions
		want string
	}{
		{
			"a changed line, with numbers, signs and context",
			core.Display{Path: "main.go", Before: "a\nb\nc\nd\ne\n", After: "a\nb\nC\nd\ne\n"},
			RenderOptions{Width: 80, Context: 1},
			rows(`
main.go  +1 -1
2 2   b
3   - c
  3 + C
4 4   d`),
		},
		{
			"the title stands in for a missing path",
			core.Display{Title: "notes.txt", Before: "a\n", After: "b\n"},
			RenderOptions{},
			rows(`
notes.txt  +1 -1
1   - a
  1 + b`),
		},
		{
			"no path at all",
			core.Display{Before: "a\n", After: "b\n"},
			RenderOptions{},
			rows(`
+1 -1
1   - a
  1 + b`),
		},
		{
			"no changes",
			core.Display{Path: "main.go", Before: "a\nb\n", After: "a\nb\n"},
			RenderOptions{Width: 80},
			"main.go  no changes",
		},
		{
			"no changes, both empty",
			core.Display{Path: "main.go"},
			RenderOptions{Width: 80},
			"main.go  no changes",
		},
		{
			"a new file: one column of numbers, every line added",
			core.Display{Path: "new.txt", After: "one\ntwo\n"},
			RenderOptions{Width: 80},
			rows(`
new.txt  new file  +2
1 + one
2 + two`),
		},
		{
			"a new file that does not end with a newline",
			core.Display{Path: "new.txt", After: "one\ntwo"},
			RenderOptions{Width: 80},
			rows(`
new.txt  new file  +2
no newline at end of file
1 + one
2 + two`),
		},
		{
			"a new file with CRLF line endings",
			core.Display{Path: "new.txt", After: "one\r\ntwo\r\n"},
			RenderOptions{Width: 80},
			rows(`
new.txt  new file  +2
line endings: CRLF
1 + one
2 + two`),
		},
		{
			"a file left empty",
			core.Display{Path: "old.txt", Before: "one\ntwo\n"},
			RenderOptions{Width: 80},
			rows(`
old.txt  emptied  -2
1 - one
2 - two`),
		},
		{
			"a deleted file",
			core.Display{Path: "old.txt", Before: "one\ntwo\n"},
			RenderOptions{Width: 80, Deleted: true},
			rows(`
old.txt  deleted file  -2
1 - one
2 - two`),
		},
		{
			"binary content is declined in one line",
			core.Display{Path: "logo.png", Before: "\x89PNG\x00\x01", After: "\x89PNG\x00\x02\x03"},
			RenderOptions{Width: 80},
			"logo.png  binary or not UTF-8 text: not shown (6 bytes before, 7 after)",
		},
		{
			"text that is not UTF-8 is declined",
			core.Display{Path: "latin1.txt", Before: "caf\xe9\n", After: "cafe\n"},
			RenderOptions{Width: 80},
			"latin1.txt  binary or not UTF-8 text: not shown (5 bytes before, 5 after)",
		},
		{
			"a NUL byte on one side is enough",
			core.Display{Path: "data", Before: "a\n", After: "a\x00\n"},
			RenderOptions{Width: 80},
			"data  binary or not UTF-8 text: not shown (2 bytes before, 3 after)",
		},
		{
			"two hunks, with the lines between them counted",
			core.Display{Path: "n.txt", Before: "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n", After: "1\n2\nthree\n4\n5\n6\n7\n8\n9\n10\n11\ntwelve\n"},
			RenderOptions{Width: 80, Context: 1},
			rows(`
n.txt  +2 -2
 2  2   2
 3    - 3
    3 + three
 4  4   4
      ⋮ 6 unchanged lines
11 11   11
12    - 12
   12 + twelve`),
		},
		{
			"one unchanged line between two hunks",
			core.Display{Path: "n.txt", Before: "1\n2\n3\n", After: "one\n2\nthree\n"},
			RenderOptions{Width: 80, Context: -1},
			rows(`
n.txt  +2 -2
1   - 1
  1 + one
    ⋮ 1 unchanged line
3   - 3
  3 + three`),
		},
		{
			"the words that changed are found by pairing like with like",
			core.Display{Path: "a.go", Before: "x := load(path)\ny := parse(x)\n", After: "// read it\ny := parse(x, opts)\nz := check(y)\n"},
			RenderOptions{Width: 80},
			rows(`
a.go  +3 -2
1   - x := load(path)
  1 + // read it
2   - y := parse(x)
  2 + y := parse(x, opts)
  3 + z := check(y)`),
		},
		{
			"a deleted line stands right above what it turned into, after what was added before it",
			core.Display{Path: "a.go", Before: "func greet(name string) {\n\thello(name)\n}\n", After: "func shout(s string) {\n\tloud(s)\n}\nfunc greet(name, greeting string) {\n\thello(name, greeting)\n}\n"},
			RenderOptions{Width: 80},
			rows(`
a.go  +5 -2
  1 + func shout(s string) {
  2 +     loud(s)
  3 + }
1   - func greet(name string) {
2   -     hello(name)
  4 + func greet(name, greeting string) {
  5 +     hello(name, greeting)
3 6   }`),
		},
		{
			"a long line wraps, between words where it can, and nothing is dropped",
			core.Display{Path: "a.txt", Before: "short\n", After: "the quick brown fox jumps over the lazy dog and keeps on running\n"},
			RenderOptions{Width: 30},
			rows(`
a.txt  +1 -1
1   - short
  1 + the quick brown fox ` + `
    ↳ jumps over the lazy dog ` + `
    ↳ and keeps on running`),
		},
		{
			"a word longer than a row is cut at the width",
			core.Display{Path: "a.txt", Before: "short\n", After: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz\n"},
			RenderOptions{Width: 30},
			rows(`
a.txt  +1 -1
1   - short
  1 + abcdefghijklmnopqrstuvwx
    ↳ yz0123456789abcdefghijkl
    ↳ mnopqrstuvwxyz`),
		},
		{
			"wide characters are never cut in two at the edge",
			core.Display{Path: "a.txt", Before: "x\n", After: "a日本語のテキストを折り返します\n"},
			RenderOptions{Width: 17},
			rows(`
a.txt  +1 -1
- x
+ a日本語のテキス
↳ トを折り返しま
↳ す`),
		},
		{
			"an emoji made of several joined ones moves to the next row whole",
			core.Display{Path: "a.txt", Before: "x\n", After: "family: " + family + " and a flag " + japan + " here\n"},
			RenderOptions{Width: 16},
			"a.txt  +1 -1\n- x\n+ family: " + family + " \n↳ and a flag " + japan + " \n↳ here",
		},
		{
			"too narrow for line numbers: they are left out",
			core.Display{Path: "a.txt", Before: "one\ntwo\n", After: "one\n2\n"},
			RenderOptions{Width: 12},
			rows(`
a.txt  +1 -1
  one
- two
+ 2`),
		},
		{
			"tabs go to the next tab stop, the same on both sides",
			core.Display{Path: "a.go", Before: "\tif x {\n\t\ty()\n\t}\n", After: "\tif x {\n\t\tz()\n\t}\n"},
			RenderOptions{Width: 80},
			rows(`
a.go  +1 -1
1 1       if x {
2   -         y()
  2 +         z()
3 3       }`),
		},
		{
			"tab stops are counted from the start of the text, eight apart when asked",
			core.Display{Path: "a.txt", Before: "a\tb\n", After: "ab\tc\n"},
			RenderOptions{Width: 80, TabWidth: 8},
			rows(`
a.txt  +1 -1
1   - a       b
  1 + ab      c`),
		},
		{
			"spaces at the end of a changed line are shown",
			core.Display{Path: "a.txt", Before: "x\ny\n", After: "x  \ny\t\n"},
			RenderOptions{Width: 80},
			rows(`
a.txt  +2 -2
1   - x
2   - y
  1 + x··
  2 + y→  `),
		},
		{
			"a tab that became spaces is shown, since the lines would look the same",
			core.Display{Path: "a.txt", Before: "\tx\n", After: "    x\n"},
			RenderOptions{Width: 80},
			rows(`
a.txt  +1 -1
1   - →   x
  1 + ····x`),
		},
		{
			"a no-break space in place of a space is shown",
			core.Display{Path: "a.txt", Before: "a b\n", After: "a\u00a0b\n"},
			RenderOptions{Width: 80},
			rows(`
a.txt  +1 -1
1   - a·b
  1 + a<U+00A0>b`),
		},
		{
			"blank lines with spaces on them",
			core.Display{Path: "a.txt", Before: "a\n\nb\n", After: "a\n  \nb\n"},
			RenderOptions{Width: 80},
			rows(`
a.txt  +1 -1
1 1   a
2   -
  2 + ··
3 3   b`),
		},
		{
			"a NUL byte makes it binary, whatever else is there",
			core.Display{Path: "a.txt", Before: "plain\n", After: "\x1b[31mred\x1b[0m\x07\x00\x7f\n"},
			RenderOptions{Width: 80},
			"a.txt  binary or not UTF-8 text: not shown (6 bytes before, 16 after)",
		},
		{
			"escape sequences and control characters are drawn, not sent",
			core.Display{Path: "a.txt", Before: "plain\n", After: "\x1b[31mred\x1b[0m\x07\x7f\x1b]0;title\x07\rdone\n"},
			RenderOptions{Width: 80},
			rows(`
a.txt  +1 -1
1   - plain
  1 + ␛[31mred␛[0m␇␡␛]0;title␇␍done`),
		},
		{
			"characters that would not show are written out",
			core.Display{Path: "a.txt", Before: "x\n", After: "a\u200bb \u202egnp.exe\u202c c\u200dd e\u200cf \ufeffg \u2028h i\u0085j\n"},
			RenderOptions{Width: 0},
			rows(`
a.txt  +1 -1
1   - x
  1 + a<U+200B>b <U+202E>gnp.exe<U+202C> c<U+200D>d e<U+200C>f <U+FEFF>g <U+2028>h i<U+0085>j`),
		},
		{
			"joiners and selectors that do real work are left alone",
			core.Display{Path: "a.txt", Before: "x\n", After: family + " " + keycap + " " + conjunct + " " + plane + " " + scotland + " \u00e9\n"},
			RenderOptions{Width: 0},
			"a.txt  +1 -1\n1   - x\n  1 + " + family + " " + keycap + " " + conjunct + " " + plane + " " + scotland + " \u00e9",
		},
		{
			"tag characters outside a flag are written out",
			core.Display{Path: "a.txt", Before: "x\n", After: "hi\U000e0041\U000e0042 日\U000e0043\n"},
			RenderOptions{Width: 0},
			rows(`
a.txt  +1 -1
1   - x
  1 + hi<U+E0041><U+E0042> 日<U+E0043>`),
		},
		{
			"a path with control characters in it",
			core.Display{Path: "bad\x1b[2Jname\n.txt", Before: "a\n", After: "b\n"},
			RenderOptions{Width: 80},
			rows(`
bad␛[2Jname␊.txt  +1 -1
1   - a
  1 + b`),
		},
		{
			"a long path gets a line to itself, and is not cut",
			core.Display{Path: "/home/dev/project/internal/some/deep/package/with_a_long_name.go", Before: "a\n", After: "b\n"},
			RenderOptions{Width: 40},
			rows(`
/home/dev/project/internal/some/deep/pac
kage/with_a_long_name.go
+1 -1
1   - a
  1 + b`),
		},
		{
			"line endings changed throughout: a note, and only the real change shown",
			core.Display{Path: "dos.txt", Before: "one\r\ntwo\r\nthree\r\n", After: "one\n2\nthree\n"},
			RenderOptions{Width: 80},
			rows(`
dos.txt  +1 -1
line endings changed: CRLF to LF
1 1   one
2   - two
  2 + 2
3 3   three`),
		},
		{
			"nothing but the line endings changed",
			core.Display{Path: "dos.txt", Before: "one\ntwo\n", After: "one\r\ntwo\r\n"},
			RenderOptions{Width: 80},
			rows(`
dos.txt
line endings changed: LF to CRLF`),
		},
		{
			"mixed line endings: each line shows its own",
			core.Display{Path: "mixed.txt", Before: "one\ntwo\nthree\nfour\n", After: "one\ntwo\r\nthree\nfive\r\n"},
			RenderOptions{Width: 80, Context: 1},
			rows(`
mixed.txt  +1 -1
line endings changed on 1 line
1 1   one
2   - two
  2 + two␍
3 3   three
4   - four
  4 + five␍`),
		},
		{
			"the final newline removed",
			core.Display{Path: "a.txt", Before: "one\ntwo\n", After: "one\ntwo"},
			RenderOptions{Width: 80},
			rows(`
a.txt
newline removed from end of file`),
		},
		{
			"the final newline added, and a line changed",
			core.Display{Path: "a.txt", Before: "one\ntwo\nthree", After: "one\n2\nthree\n"},
			RenderOptions{Width: 80},
			rows(`
a.txt  +1 -1
newline added at end of file
1 1   one
2   - two
  2 + 2
3 3   three`),
		},
		{
			"a cap on the lines shown, and what was left out",
			core.Display{Path: "n.txt", Before: "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n", After: "one\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\ntwelve\n"},
			RenderOptions{Width: 80, Context: 1, MaxLines: 3},
			rows(`
n.txt  +2 -2
 1    - 1
    1 + one
 2  2   2
      ⋮ 2 more changed lines in 1 hunk not shown`),
		},
		{
			"a cap in the middle of a hunk",
			core.Display{Path: "n.txt", Before: "1\n2\n3\n", After: "one\ntwo\nthree\n"},
			RenderOptions{Width: 80, MaxLines: 4},
			rows(`
n.txt  +3 -3
1   - 1
2   - 2
3   - 3
  1 + one
    ⋮ 2 more changed lines in 1 hunk not shown`),
		},
		{
			"a cap that only cuts unchanged lines says nothing",
			core.Display{Path: "n.txt", Before: "1\n2\n3\n", After: "one\n2\n3\n"},
			RenderOptions{Width: 80, MaxLines: 2},
			rows(`
n.txt  +1 -1
1   - 1
  1 + one`),
		},
		{
			"a first line longer than the cap is shown as far as the cap goes",
			core.Display{Path: "n.txt", Before: "", After: strings.Repeat("0123456789", 10) + "\nsecond\n"},
			RenderOptions{Width: 24, MaxLines: 2},
			rows(`
n.txt  new file  +2
1 + 01234567890123456789
  ↳ 01234567890123456789
  ⋮ the line above is
    cut short; 2 more
    changed lines in 1
    hunk not shown`),
		},
		{
			"large numbers are written with commas",
			core.Display{Path: "big.txt", Before: "", After: strings.Repeat("x\n", 12345)},
			RenderOptions{Width: 80, MaxLines: 1},
			rows(`
big.txt  new file  +12,345
    1 + x
      ⋮ 12,344 more changed lines in 1 hunk not shown`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := plain(tt.d, tt.opts)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestRenderMarks checks which words are marked inside changed lines, with a
// theme that puts brackets where the default one underlines.
func TestRenderMarks(t *testing.T) {
	tests := []struct {
		name          string
		before, after string
		want          string
	}{
		{
			"a word in a line",
			"return nil\n", "return err\n",
			rows(`
1   - return [nil]
  1 + return [err]`),
		},
		{
			"pairs are chosen by likeness, not by position",
			"x := load(path)\ny := parse(x)\n", "// read it\ny := parse(x, opts)\nz := check(y)\n",
			rows(`
1   - x := load(path)
  1 + // read it
2   - y := parse(x)
  2 + y := parse(x[, opts])
  3 + z := check(y)`),
		},
		{
			"lines with little in common are not marked",
			"foo(a)\n", "bar(b)\n",
			rows(`
1   - foo(a)
  1 + bar(b)`),
		},
		{
			"a mark that runs over the end of a row goes on in the next",
			"the quick brown fox\n", "the quick red and very hungry fox\n",
			rows(`
1   - the quick [brown] fox
  1 + the quick [red and ]
    ↳ [very hungry] fox`),
		},
		{
			"a changed space at the end is marked and shown",
			"x()\n", "x() \n",
			rows(`
1   - x()
  1 + x()[·]`),
		},
		{
			"indentation",
			"  x()\n", "      x()\n",
			rows(`
1   -   x()
  1 +   [    ]x()`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Render(core.Display{Before: tt.before, After: tt.after}, RenderOptions{Width: 26, Profile: text.TrueColor, Theme: bracketTheme()})
			// Each styled stretch becomes its text in brackets; the header
			// is left off.
			var lines []string
			for _, row := range got[1:] {
				row = strings.ReplaceAll(row, "\x1b[4m", "[")
				row = strings.ReplaceAll(row, "\x1b[0m", "]")
				lines = append(lines, row)
			}
			if got := strings.Join(lines, "\n"); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// bracketTheme is a theme with one style in it: the changed words are
// underlined and everything else is plain.
func bracketTheme() *Theme {
	return &Theme{DeletedSpan: text.Style{}.Underline(), InsertedSpan: text.Style{}.Underline()}
}

// A shownLine is one line of a text as Render drew it, read back from the
// rows.
type shownLine struct {
	oldNo, newNo int
	sign         byte
	text         string
}

// readBack reads the lines out of rows rendered without colour and with two
// columns of numbers digits wide: the reverse of what Render does. It fails
// the test when a row is not laid out as documented.
func readBack(t testing.TB, body []string, digits int) (lines []shownLine, notes []string) {
	t.Helper()
	gutter := 2*digits + 2
	for _, row := range body {
		if len(row) < gutter+1 {
			t.Fatalf("row %q is shorter than its gutter", row)
		}
		nums, mark, rest := row[:gutter], row[gutter:], ""
		// The mark is one character, which may be several bytes.
		_, size := decodeMark(mark)
		mark, rest = mark[:size], mark[size:]
		if rest != "" {
			if rest[0] != ' ' {
				t.Fatalf("row %q has no space after its mark", row)
			}
			rest = rest[1:]
		}
		switch mark {
		case markWrap:
			if len(lines) == 0 {
				t.Fatalf("row %q goes on with a line that is not there", row)
			}
			lines[len(lines)-1].text += rest
		case markSkip:
			notes = append(notes, rest)
		case " ", "-", "+":
			if strings.TrimSpace(nums) == "" && mark == " " {
				// The second row of a note.
				if len(notes) == 0 {
					t.Fatalf("row %q has neither numbers nor a sign", row)
				}
				notes[len(notes)-1] += " " + rest
				continue
			}
			line := shownLine{sign: mark[0], text: rest}
			if s := strings.TrimSpace(nums[:digits]); s != "" {
				line.oldNo, _ = strconv.Atoi(s)
			}
			if s := strings.TrimSpace(nums[digits+1 : 2*digits+1]); s != "" {
				line.newNo, _ = strconv.Atoi(s)
			}
			lines = append(lines, line)
		default:
			t.Fatalf("row %q has the mark %q", row, mark)
		}
	}
	return lines, notes
}

// decodeMark returns the first character of s and its length in bytes.
func decodeMark(s string) (string, int) {
	for i := range s {
		if i > 0 {
			return s[:i], i
		}
	}
	return s, len(s)
}

// TestRenderShowsEveryLine renders changes with all of their unchanged lines
// and reads both texts back out of the rows: every line of the text before
// must be there with a space or a minus and its number, every line of the
// text after with a space or a plus and its number, and wrapped lines must
// join back to what they were.
func TestRenderShowsEveryLine(t *testing.T) {
	s := newSource(51)
	// Lines that are drawn exactly as they are: no tabs, no spaces at the
	// end, nothing that needs a stand-in.
	words := []string{"alpha", "beta", "gamma", "{", "}", "x := 1", "return nil", "日本語", "🎉 done", "  indented", "a longer line of several words to wrap"}
	line := func() string {
		if s.IntN(4) == 0 {
			return "line " + strconv.Itoa(s.IntN(100000))
		}
		return words[s.IntN(len(words))]
	}
	for range 400 {
		old := make([]string, 1+s.IntN(40))
		for i := range old {
			old[i] = line()
		}
		new := s.edited(old, s.IntN(6))
		for i, l := range new {
			if strings.HasSuffix(l, "// changed") || strings.HasPrefix(l, "\t") || !isPlainLine(l) {
				new[i] = line()
			}
		}
		if len(new) == 0 {
			new = []string{line()}
		}
		before, after := strings.Join(old, "\n")+"\n", strings.Join(new, "\n")+"\n"
		if before == after {
			continue
		}
		width := []int{0, 200, 60, 40, 30}[s.IntN(5)]
		got := Render(core.Display{Path: "f", Before: before, After: after}, RenderOptions{Width: width, Context: 1 << 20})
		for _, row := range got {
			if width > 0 && text.VisibleWidth(row) > width {
				t.Fatalf("row %q is %d cells wide in a terminal of %d", row, text.VisibleWidth(row), width)
			}
		}
		digits := len(strconv.Itoa(max(len(old), len(new))))
		lines, notes := readBack(t, got[1:], digits)
		if len(notes) != 0 {
			t.Fatalf("with all the context there is, nothing should be left out: %q", notes)
		}
		var gotOld, gotNew []string
		for _, l := range lines {
			if l.sign != '+' {
				gotOld = append(gotOld, l.text)
				if l.oldNo != len(gotOld) {
					t.Fatalf("line %q of the old text is numbered %d, want %d\n%s", l.text, l.oldNo, len(gotOld), strings.Join(got, "\n"))
				}
			}
			if l.sign != '-' {
				gotNew = append(gotNew, l.text)
				if l.newNo != len(gotNew) {
					t.Fatalf("line %q of the new text is numbered %d, want %d\n%s", l.text, l.newNo, len(gotNew), strings.Join(got, "\n"))
				}
			}
			if l.sign == '+' && l.oldNo != 0 || l.sign == '-' && l.newNo != 0 {
				t.Fatalf("line %q has a number in the wrong column", l.text)
			}
		}
		if strings.Join(gotOld, "\n")+"\n" != before {
			t.Fatalf("the old text read back from the rows differs\ngot:  %q\nwant: %q\n%s", gotOld, old, strings.Join(got, "\n"))
		}
		if strings.Join(gotNew, "\n")+"\n" != after {
			t.Fatalf("the new text read back from the rows differs\ngot:  %q\nwant: %q\n%s", gotNew, new, strings.Join(got, "\n"))
		}
	}
}

// isPlainLine reports whether a line is drawn exactly as it is.
func isPlainLine(l string) bool {
	return !strings.ContainsAny(l, "\t\r") && !strings.HasSuffix(l, " ")
}

// TestRenderHoldsItsWidth renders changes of every kind at every width and
// in every colour profile, and checks what holds for all of them: no row is
// wider than the terminal, no row holds anything but text and styles, each
// row leaves the terminal as it found it, and taking the styles away leaves
// exactly the rows drawn without colour.
func TestRenderHoldsItsWidth(t *testing.T) {
	s := newSource(52)
	styleOnly := regexp.MustCompile("\x1b\\[[0-9;]*m")
	for i := range times(1500) {
		var before, after string
		switch i % 4 {
		case 0:
			before, after = s.pair(60, 1+s.IntN(8))
		case 1:
			before, after = s.text(s.lines(s.IntN(30))), s.text(s.lines(s.IntN(30)))
		case 2:
			before, after = s.oddText(s.IntN(300)), s.oddText(s.IntN(300))
		case 3:
			before = s.oddText(s.IntN(200))
			after = before[:s.IntN(len(before)+1)] + s.oddText(s.IntN(20))
		}
		opts := RenderOptions{
			Width:    4 + s.IntN(100),
			Context:  s.IntN(5) - 1,
			MaxLines: []int{0, 0, 1, 5, 20}[s.IntN(5)],
			TabWidth: []int{0, 2, 8}[s.IntN(3)],
			Deleted:  s.IntN(2) == 0,
			Metrics:  text.Metrics{AmbiguousWide: s.IntN(4) == 0},
		}
		d := core.Display{Path: "dir/a file.txt", Before: before, After: after}
		opts.Profile = text.NoColor
		want := Render(d, opts)
		for _, profile := range []text.Profile{text.NoColor, text.ANSI16, text.ANSI256, text.TrueColor} {
			opts.Profile = profile
			got := Render(d, opts)
			if len(got) != len(want) {
				t.Fatalf("%v: %d rows, and %d without colour", profile, len(got), len(want))
			}
			for k, row := range got {
				if w := opts.Metrics.VisibleWidth(row); w > opts.Width {
					t.Fatalf("%v: row %q is %d cells wide in a terminal of %d\nbefore: %q\nafter:  %q", profile, row, w, opts.Width, before, after)
				}
				bare := styleOnly.ReplaceAllString(row, "")
				if bare != want[k] {
					t.Fatalf("%v: without its styles row %d is %q; drawn without colour it is %q", profile, k, bare, want[k])
				}
				for _, c := range bare {
					if c < 0x20 || c == 0x7f || c >= 0x80 && c < 0xa0 {
						t.Fatalf("%v: row %q holds the control character %U\nbefore: %q\nafter:  %q", profile, row, c, before, after)
					}
				}
				if profile != text.NoColor && strings.Contains(row, "\x1b[") && !strings.HasSuffix(row, text.Reset) && strings.LastIndex(row, "\x1b[") > strings.LastIndex(row, text.Reset) {
					t.Fatalf("%v: row %q ends with a style still on", profile, row)
				}
			}
		}
		if opts.MaxLines > 0 {
			// The header and the notes under it take at most a handful of
			// rows; the cap bounds everything else but the last note.
			body := 0
			for _, row := range want {
				if strings.Contains(row, " - ") || strings.Contains(row, " + ") || strings.HasPrefix(row, "- ") || strings.HasPrefix(row, "+ ") {
					body++
				}
			}
			if body > opts.MaxLines+8 {
				t.Fatalf("%d rows of changes under a cap of %d", body, opts.MaxLines)
			}
		}
	}
}

// oddText makes up text out of everything a file can hold that a terminal
// must not be sent as it is, mixed with ordinary words.
func (s source) oddText(n int) string {
	pieces := []string{
		"word", "x", " ", " ", "  ", "\t", "\n", "\n", "\n", "\r\n", "\r", "\x1b[31m", "\x1b]0;t\x07", "\x1b", "\x07", "\x7f",
		"\u200b", "\u200d", "\u202e", "\ufeff", "\u2028", "\u0085", "\u00a0", "\u3000", "e\u0301", "\u0301",
		"日本語", "🎉", "👨\u200d👩\u200d👧", "🇯🇵", "1\ufe0f\u20e3", "a\ufe0f", "\U000e0041", "·", "→", "é", "\u00ad",
	}
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(pieces[s.IntN(len(pieces))])
	}
	return b.String()
}

// TestRenderCap checks the count Render gives of what a cap left out: the
// changed lines it names and the ones it shows add up to all of them.
func TestRenderCap(t *testing.T) {
	s := newSource(53)
	left := regexp.MustCompile(`⋮ (?:the line above is cut short; )?([0-9,]+) more changed lines? in ([0-9,]+) hunks? not shown$`)
	for range 600 {
		old := make([]string, 5+s.IntN(60))
		for i := range old {
			old[i] = "line " + strconv.Itoa(i)
		}
		new := append([]string(nil), old...)
		for range 1 + s.IntN(8) {
			new[s.IntN(len(new))] = "changed " + strconv.Itoa(s.IntN(1000))
		}
		before, after := strings.Join(old, "\n")+"\n", strings.Join(new, "\n")+"\n"
		if before == after {
			continue
		}
		st := Compare(before, after, Options{}).Stats
		total := st.Added + st.Removed
		limit := 1 + s.IntN(30)
		context := s.IntN(4) - 1
		got := Render(core.Display{Path: "f", Before: before, After: after}, RenderOptions{Width: 80, MaxLines: limit, Context: context})
		body := got[1:]
		shown := 0
		for _, row := range body {
			if strings.Contains(row, " - ") || strings.Contains(row, " + ") {
				shown++
			}
		}
		last := body[len(body)-1]
		m := left.FindStringSubmatch(last)
		if m == nil {
			if shown != total {
				t.Fatalf("%d of %d changed lines are shown and nothing says so:\n%s", shown, total, strings.Join(got, "\n"))
			}
			if len(body) > limit {
				t.Fatalf("%d rows under a cap of %d:\n%s", len(body), limit, strings.Join(got, "\n"))
			}
			continue
		}
		if len(body)-1 > limit {
			t.Fatalf("%d rows before the note under a cap of %d:\n%s", len(body)-1, limit, strings.Join(got, "\n"))
		}
		missing, _ := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
		if shown+missing != total {
			t.Fatalf("%d changed lines shown and %d said to be left out, of %d:\n%s", shown, missing, total, strings.Join(got, "\n"))
		}
		hunks, _ := strconv.Atoi(m[2])
		all := len(Hunks(Lines(before, after), context))
		if hunks < 1 || hunks > all {
			t.Fatalf("%d hunks said to be left out, of %d:\n%s", hunks, all, strings.Join(got, "\n"))
		}
	}
}

func TestRenderTheme(t *testing.T) {
	d := core.Display{Path: "a.txt", Before: "one\ntwo\n", After: "one\n2\n"}
	t.Run("a theme of the caller's own", func(t *testing.T) {
		theme := Theme{
			Path:     text.Style{}.Fg(text.Blue),
			Number:   text.Style{}.Fg(text.Cyan),
			Context:  text.Style{}.Italic(),
			Deleted:  text.Style{}.Fg(text.Yellow),
			Inserted: text.Style{}.Fg(text.Magenta),
		}
		got := strings.Join(Render(d, RenderOptions{Width: 40, Profile: text.ANSI16, Theme: &theme}), "\n")
		want := "\x1b[34ma.txt\x1b[0m  \x1b[35m+1\x1b[0m \x1b[33m-1\x1b[0m\n" +
			"\x1b[36m1 1\x1b[0m \x1b[3m  one\x1b[0m\n" +
			"\x1b[36m2\x1b[0m   \x1b[33m- two\x1b[0m\n" +
			"  \x1b[36m2\x1b[0m \x1b[35m+ 2\x1b[0m"
		if got != want {
			t.Errorf("got:\n%q\nwant:\n%q", got, want)
		}
	})
	t.Run("the zero theme is plain", func(t *testing.T) {
		got := Render(d, RenderOptions{Width: 40, Profile: text.TrueColor, Theme: &Theme{}})
		want := Render(d, RenderOptions{Width: 40})
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("got:\n%q\nwant:\n%q", got, want)
		}
	})
	t.Run("no theme is the default for the profile", func(t *testing.T) {
		for _, p := range []text.Profile{text.NoColor, text.ANSI16, text.ANSI256, text.TrueColor} {
			theme := DefaultTheme(p)
			got := Render(d, RenderOptions{Width: 40, Profile: p, Theme: &theme})
			want := Render(d, RenderOptions{Width: 40, Profile: p})
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("%v: got:\n%q\nwant:\n%q", p, got, want)
			}
		}
	})
	t.Run("sixteen colours use the terminal's own red and green", func(t *testing.T) {
		got := strings.Join(Render(d, RenderOptions{Width: 40, Profile: text.ANSI16}), "\n")
		for _, seq := range []string{"\x1b[31m- two", "\x1b[32m+ 2", "\x1b[2m1 1"} {
			if !strings.Contains(got, seq) {
				t.Errorf("%q is not in:\n%q", seq, got)
			}
		}
	})
	t.Run("the default theme does not fill cells", func(t *testing.T) {
		// No background colour and no reverse video in any profile: the
		// changed words are underlined.
		before, after := "x := compute(a, b)\n", "x := compute(a, b, c)\n"
		for _, p := range []text.Profile{text.ANSI16, text.ANSI256, text.TrueColor} {
			got := strings.Join(Render(core.Display{Before: before, After: after}, RenderOptions{Profile: p}), "\n")
			for _, m := range regexp.MustCompile("\x1b\\[([0-9;]*)m").FindAllStringSubmatch(got, -1) {
				for _, code := range strings.Split(m[1], ";") {
					n, _ := strconv.Atoi(code)
					if n == 7 || n >= 40 && n <= 49 || n >= 100 && n <= 107 {
						t.Errorf("%v: the sequence %q sets a background or reverse video", p, m[0])
					}
					if n == 38 {
						break // what follows are the numbers of a colour
					}
				}
			}
			if !strings.Contains(got, "\x1b[4;") {
				t.Errorf("%v: nothing is underlined in:\n%q", p, got)
			}
		}
	})
}

// TestGutterMarks checks that the marks drawn in the gutter take one cell
// whichever way the terminal draws characters of open width, and that the
// stand-ins are measured as the terminal draws them.
func TestGutterMarks(t *testing.T) {
	for _, m := range []text.Metrics{{}, {AmbiguousWide: true}} {
		for _, mark := range []string{markWrap, markSkip, markCR} {
			if w := m.Width(mark); w != 1 {
				t.Errorf("%q is %d cells wide (ambiguous wide: %v); the gutter gives it one", mark, w, m.AmbiguousWide)
			}
		}
	}
	// A terminal that draws these two wide still gets rows that fit.
	d := core.Display{Before: "\tx\ny\n", After: "    x\ny  \n"}
	for _, row := range Render(d, RenderOptions{Width: 12, Metrics: text.Metrics{AmbiguousWide: true}}) {
		if w := (text.Metrics{AmbiguousWide: true}).VisibleWidth(row); w > 12 {
			t.Errorf("row %q is %d cells wide", row, w)
		}
	}
}

func TestCells(t *testing.T) {
	r := &renderer{tab: 4}
	tests := []struct {
		name   string
		in     string
		spans  []Span
		reveal bool
		trail  int
		want   string // the cells' texts, with a "|" between cells and "*" before a marked one
	}{
		{"plain", "ab", nil, false, 2, "a|b"},
		{"a tab at the start", "\tx", nil, false, 2, " | | | |x"},
		{"a tab after one character", "a\tx", nil, false, 3, "a| | | |x"},
		{"a tab after four", "abcd\tx", nil, false, 6, "a|b|c|d| | | | |x"},
		{"a tab after a wide character", "日\tx", nil, false, 5, "日| | |x"},
		{"spaces at the end, kept line", "x  ", nil, false, 3, "x| | "},
		{"spaces at the end, changed line", "x  ", nil, false, 1, "x|·|·"},
		{"a tab at the end, changed line", "x\t", nil, false, 1, "x|→| | "},
		{"marked", "abc", []Span{{1, 2}}, false, 3, "a|*b|c"},
		{"marked blanks, not revealed", "a b", []Span{{1, 2}}, false, 3, "a|* |b"},
		{"marked blanks, revealed", "a b", []Span{{1, 2}}, true, 3, "a|*·|b"},
		{"blanks outside the mark stay", "a b c", []Span{{3, 4}}, true, 5, "a| |b|*·|c"},
		{"a marked tab, revealed", "\tx", []Span{{0, 1}}, true, 2, "*→|* |* |* |x"},
		{"ESC", "\x1b[0m", nil, false, 4, "␛|[|0|m"},
		{"NUL and DEL", "\x00\x7f", nil, false, 2, "␀|␡"},
		{"a carriage return", "a\rb", nil, false, 3, "a|␍|b"},
		{"a zero-width space", "a\u200bb", nil, false, 5, "a|<|U|+|2|0|0|B|>|b"},
		{"a combining mark with a letter", "e\u0301", nil, false, 3, "e\u0301"},
		{"a combining mark alone", "\u0301", nil, false, 2, "<|U|+|0|3|0|1|>"},
		{"a joiner after a letter", "a\u200d", nil, false, 4, "a|<|U|+|2|0|0|D|>"},
		{"a joiner in an emoji", "👨\u200d👩\u200d👧", nil, false, 18, "👨\u200d👩\u200d👧"},
		{"a keycap", "1\ufe0f\u20e3", nil, false, 7, "1\ufe0f\u20e3"},
		{"a selector after a letter", "a\ufe0f", nil, false, 4, "a|<|U|+|F|E|0|F|>"},
		{"a byte that is not UTF-8", "a\xffb", nil, false, 3, "a|\ufffd|b"},
		{"a no-break space, kept", "a\u00a0b", nil, false, 4, "a|\u00a0|b"},
		{"a no-break space, revealed", "a\u00a0b", []Span{{1, 3}}, true, 4, "a|*<|*U|*+|*0|*0|*A|*0|*>|b"},
		{"a line separator", "a\u2028b", nil, false, 5, "a|<|U|+|2|0|2|8|>|b"},
		{"a soft hyphen", "a\u00adb", nil, false, 4, "a|<|U|+|0|0|A|D|>|b"},
		{"a mark that turns the text round", "a\u202eb", nil, false, 5, "a|<|U|+|2|0|2|E|>|b"},
		{"a C1 control", "a\u0085b", nil, false, 4, "a|<|U|+|0|0|8|5|>|b"},
		{"a tag after a letter", "a\U000e0041", nil, false, 5, "a|<|U|+|E|0|0|4|1|>"},
		{"empty", "", nil, false, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var parts []string
			width := 0
			for _, c := range r.cells(nil, tt.in, tt.spans, tt.reveal, tt.trail, 0) {
				part := c.text
				if c.marked {
					part = "*" + part
				}
				parts = append(parts, part)
				width += c.width
				if c.width != text.Width(c.text) {
					t.Errorf("the cell %q is given %d cells; it takes %d", c.text, c.width, text.Width(c.text))
				}
				if c.width < 1 {
					t.Errorf("the cell %q takes no room", c.text)
				}
			}
			if got := strings.Join(parts, "|"); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// TestRenderVeryLongLine checks that a line far too long to show under a
// cap is not laid out in full only to be thrown away: the cap bounds the
// work as well as the rows.
func TestRenderVeryLongLine(t *testing.T) {
	long := strings.Repeat("0123456789", 200_000) // two million cells
	d := core.Display{Path: "min.js", Before: long + "\n", After: long + "x\n"}
	r := &renderer{tab: 4, room: 80}
	if got := len(r.lineRows(Delete, 1, 0, long, lineMarks{}, 5)); got != 6 {
		t.Errorf("a line of two million cells with room for 5 rows is drawn in %d rows, want 6", got)
	}
	got := Render(d, RenderOptions{Width: 80, MaxLines: 5})
	want := rows(`
min.js  +1 -1
1   - 01234567890123456789012345678901234567890123456789012345678901234567890123
    ↳ 45678901234567890123456789012345678901234567890123456789012345678901234567
    ↳ 89012345678901234567890123456789012345678901234567890123456789012345678901
    ↳ 23456789012345678901234567890123456789012345678901234567890123456789012345
    ↳ 67890123456789012345678901234567890123456789012345678901234567890123456789
    ⋮ the line above is cut short; 2 more changed lines in 1 hunk not shown`)
	if strings.Join(got, "\n") != want {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), want)
	}
	// A line too long to look into is shown without marks, not skipped.
	if _, _, ok := Spans(long, long+"x"); ok {
		t.Error("a line of two megabytes is looked into for its changed words")
	}
}

func TestGroupedAndPlural(t *testing.T) {
	for n, want := range map[int]string{0: "0", 7: "7", 999: "999", 1000: "1,000", 12345: "12,345", 123456: "123,456", 1234567: "1,234,567"} {
		if got := grouped(n); got != want {
			t.Errorf("grouped(%d) = %q, want %q", n, got, want)
		}
	}
	for n, want := range map[int]string{0: "0 lines", 1: "1 line", 2: "2 lines", 1500: "1,500 lines"} {
		if got := plural(n, "line"); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
}

// goldenDirEnv names a directory to write the golden files to instead of
// comparing with them. The tests run on a machine whose copy of the tree is
// replaced on every run, so new golden files are written outside it and
// copied back by hand:
//
//	GNOSH_DIFF_GOLDEN_DIR=/tmp/diff-golden go test -run TestRenderGolden ./internal/diff
//
// and then every file in /tmp/diff-golden goes into testdata/render here.
// Look at what changed before keeping it: it is what a person will be shown
// when asked whether a change may be made.
const goldenDirEnv = "GNOSH_DIFF_GOLDEN_DIR"

// goldenLegend heads every golden file.
const goldenLegend = `What Render draws, as a person can read it. A style is written ⟨like this⟩:
⟨1⟩ is the escape sequence ESC [ 1 m, and ⟨0⟩ turns every style off. A space
at the end of a row is written ␠.
`

// readable writes rendered rows so that a person can read them in a file:
// one row to a line, each style sequence in angle brackets, any other escape
// character as a picture, and spaces at the end of a row made visible.
func readable(rows []string) string {
	style := regexp.MustCompile("\x1b\\[([0-9;:]*)m")
	var b strings.Builder
	for _, row := range rows {
		row = style.ReplaceAllString(row, "⟨$1⟩")
		row = strings.ReplaceAll(row, "\x1b", "␛")
		bare := strings.TrimRight(row, " ")
		b.WriteString(bare)
		b.WriteString(strings.Repeat("␠", len(row)-len(bare)))
		b.WriteByte('\n')
	}
	return b.String()
}

// The change the golden files of the full matrix show: a little of
// everything a diff of code has.
const (
	goldenBefore = `package main

import "fmt"

// greet says hello.
func greet(name string) string {
	return "Hello, " + name
}

func main() {
	fmt.Println(greet("world"))
	fmt.Println("a line that is long enough to need wrapping when the terminal is narrow, with words to break between")
}

// The lines below are here to put some distance between two changes.
var (
	one   = 1
	two   = 2
	three = 3
	four  = 4
	five  = 5
	six   = 6
)

func unused() {
	return
}
`
	goldenAfter = `package main

import (
	"fmt"
	"strings"
)

// greet says hello to someone, by name.
func greet(name, greeting string) string {
	return greeting + ", " + strings.TrimSpace(name)
}

// shout says it louder.
func shout(s string) string {
	return strings.ToUpper(s) + "!"
}

func main() {
	fmt.Println(greet("world", "Hello"))
	fmt.Println("a line that is long enough to need wrapping when the terminal is narrow, with words to break up between")
}

// The lines below are here to put some distance between two changes.
var (
	one   = 1
	two   = 2
	three = 3
	four  = 4
	five  = 5
	six   = 6
)
`
)

func TestRenderGolden(t *testing.T) {
	sample := core.Display{Path: "cmd/hello/main.go", Before: goldenBefore, After: goldenAfter}
	type view struct {
		label string
		d     core.Display
		opts  RenderOptions
	}
	files := map[string][]view{}
	// The full matrix: three widths under each of the four profiles.
	for _, p := range []text.Profile{text.NoColor, text.ANSI16, text.ANSI256, text.TrueColor} {
		for _, width := range []int{40, 80, 120} {
			name := "change-" + p.String()
			files[name] = append(files[name], view{
				label: "width " + strconv.Itoa(width) + ", colour profile " + p.String(),
				d:     sample,
				opts:  RenderOptions{Width: width, Profile: p},
			})
		}
	}
	// The other things Render says, without colour and with it.
	for _, p := range []text.Profile{text.NoColor, text.TrueColor} {
		name := "notes-" + p.String()
		add := func(label string, d core.Display, opts RenderOptions) {
			opts.Profile = p
			files[name] = append(files[name], view{label, d, opts})
		}
		add("a new file", core.Display{Path: "docs/new.md", After: "# Title\n\nSome text.\n"}, RenderOptions{Width: 80})
		add("a file emptied", core.Display{Path: "docs/old.md", Before: "# Title\n\nSome text.\n"}, RenderOptions{Width: 80})
		add("a file deleted", core.Display{Path: "docs/old.md", Before: "# Title\n\nSome text.\n"}, RenderOptions{Width: 80, Deleted: true})
		add("no changes", core.Display{Path: "docs/same.md", Before: "same\n", After: "same\n"}, RenderOptions{Width: 80})
		add("binary content", core.Display{Path: "img/logo.png", Before: "\x89PNG\x00", After: "\x89PNG\x00\x01"}, RenderOptions{Width: 80})
		add("line endings changed throughout", core.Display{Path: "win/readme.txt", Before: "one\r\ntwo\r\nthree\r\n", After: "one\n2\nthree\n"}, RenderOptions{Width: 80})
		add("line endings mixed", core.Display{Path: "win/mixed.txt", Before: "one\ntwo\nthree\n", After: "one\ntwo\r\nthree\n"}, RenderOptions{Width: 80})
		add("the final newline removed", core.Display{Path: "a.txt", Before: "one\ntwo\n", After: "one\n2"}, RenderOptions{Width: 80})
		add("only spaces changed", core.Display{Path: "ws.go", Before: "\tx := 1\ny := 2\n", After: "    x := 1\ny := 2  \n"}, RenderOptions{Width: 80})
		add("what a file must not do to a terminal", core.Display{Path: "evil.txt", Before: "plain\n", After: "\x1b[2J\x1b[31mred\x07 \u202egnp.exe z\u200bw\n"}, RenderOptions{Width: 80})
		add("wide characters at the edge", core.Display{Path: "ja.txt", Before: "日本語のテキストを折り返す位置で確認します。絵文字🎉も👨\u200d👩\u200d👧含めます。\n", After: "日本語のテキストを折り返す位置で確認する。絵文字🎉も👨\u200d👩\u200d👧含めます。\n"}, RenderOptions{Width: 30})
		add("a cap on the lines shown", sample, RenderOptions{Width: 80, MaxLines: 12})
		add("no context", sample, RenderOptions{Width: 80, Context: -1})
	}

	for name, views := range files {
		var b strings.Builder
		b.WriteString(goldenLegend)
		for _, v := range views {
			b.WriteString("\n=== " + v.label + " ===\n\n")
			b.WriteString(readable(Render(v.d, v.opts)))
		}
		got := b.String()
		if dir := os.Getenv(goldenDirEnv); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name+".golden"), []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		file := filepath.Join("testdata", "render", name+".golden")
		want, err := os.ReadFile(file)
		if err != nil {
			t.Errorf("%s: %v (set %s to write the golden files)", name, err, goldenDirEnv)
			continue
		}
		if got != string(want) {
			t.Errorf("%s differs from what Render draws now:\n%s", file,
				Unified(string(want), got, UnifiedOptions{OldName: file, NewName: "what Render draws"}))
		}
	}
}

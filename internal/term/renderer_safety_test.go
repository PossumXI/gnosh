package term

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/term/vtest"
)

func TestPrepareLine(t *testing.T) {
	cases := []struct {
		name  string
		cols  int
		in    string
		text  string
		plain string
		width int
	}{
		{"plain text that fits", 10, "hello", "hello", "hello", 5},
		{"exactly the width", 5, "hello", "hello", "hello", 5},
		{"cut at the width", 5, "0123456789", "01234", "01234", 5},
		{"wide character that does not fit", 5, "ab世界", "ab世", "ab世", 4},
		{"wide character that just fits", 6, "ab世界", "ab世界", "ab世界", 6},
		{"style is kept and closed", 10, "\x1b[1;31mred", "\x1b[1;31mred\x1b[m", "red", 3},
		{"colon and 24-bit styles", 10, "\x1b[4:3m\x1b[38;2;1;2;3mx", "\x1b[4:3m\x1b[38;2;1;2;3mx\x1b[m", "x", 1},
		{"styled text cut at the width", 5, "\x1b[31m0123456789\x1b[m", "\x1b[31m01234\x1b[m", "01234", 5},
		{"a style after the cut is dropped", 3, "abcdef\x1b[1mgh", "abc", "abc", 3},
		{"hyperlink is kept and closed", 10, "\x1b]8;;https://example.com/\x1b\\link", "\x1b]8;;https://example.com/\x1b\\link\x1b]8;;\x1b\\", "link", 4},
		{"hyperlink ended by BEL", 10, "\x1b]8;;http://example.com\x07x\x1b]8;;\x07", "\x1b]8;;http://example.com\x07x\x1b]8;;\x07\x1b]8;;\x1b\\", "x", 1},
		{"hyperlink with bytes that are not UTF-8 is dropped", 10, "a\x1b]8;;http://example.com/\xbe\x9c\x1b\\b", "ab", "ab", 2},
		{"tab becomes a space", 10, "a\tb", "a b", "a b", 3},
		{"line feed and carriage return are dropped", 10, "a\nb\rc", "abc", "abc", 3},
		{"bell, backspace and DEL are dropped", 10, "a\x07b\x08c\x7fd", "abcd", "abcd", 4},
		{"cursor movement is dropped", 10, "a\x1b[5Ab\x1b[Hc\x1b[2Jd\x1b[Ke", "abcde", "abcde", 5},
		{"mode changes are dropped", 10, "a\x1b[?1049hb\x1b[?1000hc\x1b[?25l", "abc", "abc", 3},
		{"private SGR look-alike is dropped", 10, "a\x1b[>4;2mb", "ab", "ab", 2},
		{"title and clipboard strings are dropped", 10, "a\x1b]0;evil\x07b\x1b]52;c;eA==\x1b\\c", "abc", "abc", 3},
		{"DCS and APC strings are dropped", 10, "a\x1bPq#0\x1b\\b\x1b_x\x1b\\c", "abc", "abc", 3},
		{"two-byte escapes are dropped", 10, "a\x1bcb\x1b7c\x1bMd\x1b(Be", "abcde", "abcde", 5},
		{"unfinished sequence at the end is dropped", 10, "ab\x1b[31", "ab", "ab", 2},
		{"unfinished string at the end is dropped", 10, "ab\x1b]0;title", "ab", "ab", 2},
		{"lone ESC at the end is dropped", 10, "ab\x1b", "ab", "ab", 2},
		{"C1 controls are dropped", 10, "a\u009b31mb\u0090c", "a31mbc", "a31mbc", 6},
		{"bytes that are not UTF-8 are replaced", 10, "a\xffb", "a\uFFFDb", "a\uFFFDb", 3},
		{"combining mark stays with its letter at the cut", 5, "abcde\u0301fg", "abcd\x1b[?7he\u0301\x1b[?7l", "abcde\u0301", 5},
		{"accent in the last column gets wrapping back for its letter", 5, "\x1b[1mabcde\u0301\u0302\x1b[m", "\x1b[1mabcd\x1b[?7he\u0301\u0302\x1b[?7l\x1b[m\x1b[m", "abcde\u0301\u0302", 5},
		{"accent not in the last column", 6, "abcde\u0301", "abcde\u0301", "abcde\u0301", 5},
		{"accent on a wide character in the last column", 5, "abc世\u0301", "abc世\u0301", "abc世\u0301", 5},
		{"variation selector in the last column", 5, "abcd☀\ufe0f", "abcd☀\ufe0f", "abcd☀\ufe0f", 5},
		{"accent in the only column", 1, "e\u0301", "\x1b[?7he\u0301\x1b[?7l", "e\u0301", 1},
		{"nothing fits", 1, "世界", "", "", 0},
		{"empty", 10, "", "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewRenderer(nil, c.cols, 10, vtest.Width)
			got := r.prepare(c.in)
			if got.text != c.text || got.plain != c.plain || got.width != c.width {
				t.Errorf("prepare(%q) at %d columns\n got text %q plain %q width %d\nwant text %q plain %q width %d",
					c.in, c.cols, got.text, got.plain, got.width, c.text, c.plain, c.width)
			}
		})
	}
}

// Whatever a line holds, it comes out as one row of printable text.
func FuzzPrepareLine(f *testing.F) {
	f.Add("plain", uint8(10))
	f.Add("\x1b[31mred\x1b[m and \x1b]8;;http://example.com\x1b\\link\x1b]8;;\x1b\\", uint8(7))
	f.Add("a\x1b[5A\x1b[2J\r\n\tb世界\xff\x1b", uint8(3))
	f.Add("\x1b[01m000000", uint8(6))
	f.Add("\x1b[10m000000\x1b[99m", uint8(9))
	f.Add("\x1b]8;\xbe\x1b\\", uint8(9))
	f.Fuzz(func(t *testing.T, line string, cols uint8) {
		width := 1 + int(cols%60)
		r := NewRenderer(nil, width, 10, vtest.Width)
		got := r.prepare(line)
		if !utf8.ValidString(got.text) || !isPlain(got.plain) {
			t.Fatalf("prepare(%q): text %q or plain %q is not clean", line, got.text, got.plain)
		}
		if got.width > width || got.width != vtest.Width(got.plain) {
			t.Fatalf("prepare(%q) at %d columns: width %d, plain %q measures %d", line, width, got.width, got.plain, vtest.Width(got.plain))
		}
		// Drawn on a terminal, it takes one row and leaves the
		// terminal as it was.
		scr := vtest.New(width, 4)
		scr.WriteString("\x1b[?7l" + got.text + "\x1b[?7h")
		if row, _ := scr.Cursor(); row != 0 {
			t.Fatalf("prepare(%q) moved the cursor to row %d", line, row)
		}
		// (A mark with nothing to sit on is not shown; leave marks out
		// of the comparison.)
		seen := func(s string) string {
			return strings.TrimRight(strings.Map(func(r rune) rune {
				if vtest.Width(string(r)) == 0 {
					return -1
				}
				return r
			}, s), " ")
		}
		if seen(got.plain) != seen(scr.Line(0)) {
			t.Fatalf("prepare(%q): terminal shows %q, plain is %q", line, scr.Line(0), got.plain)
		}
		for _, seq := range scr.Unhandled() {
			// A style number that means nothing is still only a style.
			if !strings.HasSuffix(seq, "m") {
				t.Fatalf("prepare(%q) let %q through: %q", line, seq, got.text)
			}
		}
		if scr.Title() != "" || scr.AltScreenEver() || scr.MouseEver() || scr.Bells() > 0 || len(scr.Strings()) > 0 || len(scr.Clipboard()) > 0 {
			t.Fatalf("prepare(%q) let something through: %q", line, got.text)
		}
		if got.width < width {
			_, col := scr.Cursor()
			scr.WriteString("x")
			if c := scr.Cell(0, col); c.Text != "x" || c.Attr != (vtest.Attr{}) {
				t.Fatalf("prepare(%q) left a style or link open: %+v", line, c)
			}
		}
	})
}

// Lines that try to move the cursor or change the terminal do neither.
func TestHostileLines(t *testing.T) {
	g := newRig(t, 30, 8)
	g.commit("before")
	hostile := []string{
		"clear\x1b[2J\x1b[H screen",
		"up\x1b[5A and away",
		"new\nline\r\nhere",
		"alt\x1b[?1049h screen",
		"mouse\x1b[?1000h\x1b[?1006h on",
		"title\x1b]0;evil\x07 set",
		"reset\x1bc now",
		"bell\x07 rung",
		"scroll\x1b[5S\x1b[3T\x1b[2;4r region",
		"wrap\x1b[?7h on and \x1b[?25l cursor",
	}
	g.setLive(1, 3, hostile...)
	// (Commit takes a line feed as the end of a line; see TestCommitOnly.)
	g.commit(append(hostile[:2:2], hostile[3:]...)...)
	g.setLive(0, 0, "> ")
	if g.scr.Title() != "" || g.scr.Bells() != 0 {
		t.Errorf("a line set the title to %q or rang the bell %d times", g.scr.Title(), g.scr.Bells())
	}
	if !strings.Contains(strings.Join(g.scr.AllLines(), "\n"), "clear screen") {
		t.Errorf("the text around a dropped sequence is missing:\n%s", g.scr.Dump())
	}
}

// A line's colours and links end with it.
func TestStylesDoNotLeak(t *testing.T) {
	g := newRig(t, 20, 6)
	g.commit("\x1b[1;41mbold on red", "plain")
	g.setLive(0, 0, "\x1b[4;32munderlined", "\x1b]8;;https://example.com/\x1b\\link", "last")
	scr := g.scr
	if a := scr.Cell(0, 0).Attr; !a.Bold || a.BG != vtest.Indexed(1) {
		t.Errorf("the styled line lost its style: %+v", a)
	}
	// Past the end of the styled line, and on the lines after it.
	for _, pos := range [][2]int{{0, 15}, {1, 0}, {1, 10}, {2, 12}, {3, 6}, {4, 0}, {4, 10}} {
		if a := scr.Cell(pos[0], pos[1]).Attr; a != (vtest.Attr{}) {
			t.Errorf("cell at row %d col %d has attributes %+v, want none", pos[0], pos[1], a)
		}
	}
	if a := scr.Cell(2, 0).Attr; !a.Underline || a.FG != vtest.Indexed(2) {
		t.Errorf("the live line lost its style: %+v", a)
	}
	if got := scr.Cell(3, 0).Attr.Link; got != "https://example.com/" {
		t.Errorf("the link is %q", got)
	}
	// Replacing a styled line with a shorter plain one leaves nothing
	// of it behind.
	g.setLive(0, 0, "x", "y", "last")
	for col := 0; col < 20; col++ {
		if a := scr.Cell(2, col).Attr; a != (vtest.Attr{}) {
			t.Errorf("row 2 col %d still has attributes %+v", col, a)
		}
	}
}

// Lines wider than the terminal are cut, and full-width lines keep their
// last character.
func TestWideLines(t *testing.T) {
	g := newRig(t, 10, 6)
	g.commit("0123456789", "0123456789abcdef", "short")
	g.setLive(1, 10, "abcdefghij", "klmnopqrstuvwxyz", "x")
	if got := g.scr.Lines(); !reflect.DeepEqual(got, []string{"0123456789", "0123456789", "short", "abcdefghij", "klmnopqrst", "x"}) {
		t.Errorf("screen = %q", got)
	}
	g.setLive(0, 0, "ABCDEFGHIJ", "k", "x")
	g.setLive(0, 0, "ABCDEFGHI", "klmnopqrst", "x")
	g.commit("世界世界世界", "committed")
}

// When the terminal thinks a line is wider than gnosh measured it, the
// terminal clips the line; the rows below stay where they are.
func TestTerminalMeasuresWider(t *testing.T) {
	scr := vtest.New(12, 8)
	runes := func(s string) int { return utf8.RuneCountInString(s) }
	r := NewRenderer(scr, 12, 8, runes)
	wide := strings.Repeat("世", 10) // ten cells to gnosh, twenty to the terminal
	r.Commit("c1", wide, "c2")
	r.SetLive([]string{wide, "> in", "status"}, 1, 4)
	r.Commit("c3")
	r.SetLive([]string{"short", "> in", "status"}, 1, 4)
	r.SetLive([]string{wide, wide, "> input", "status"}, 2, 7)
	r.Commit("c4", wide)
	r.SetLive([]string{"> ", "status"}, 0, 2)

	// Six of the wide characters fit in twelve cells; the terminal
	// drops the rest.
	clipped := strings.Repeat("世", 6)
	want := []string{"c1", clipped, "c2", "c3", "c4", clipped, ">", "status"}
	if got := scr.AllLines(); !reflect.DeepEqual(got, want) {
		t.Errorf("lines are\n  %q\nwant\n  %q\n%s", got, want, scr.Dump())
	}
	if row, col := scr.Cursor(); row+len(scr.Scrollback()) != 6 || col != 2 {
		t.Errorf("cursor on line %d col %d, want line 6 col 2", row+len(scr.Scrollback()), col)
	}
	if !scr.Mode(7) {
		t.Error("the terminal's line wrapping was left off")
	}
}

// Updates from several goroutines do not interleave.
func TestConcurrentUse(t *testing.T) {
	scr := vtest.New(40, 10)
	r := NewRenderer(scr, 40, 10, vtest.Width)
	var wg sync.WaitGroup
	const writers, each = 4, 40
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				r.Commit(fmt.Sprintf("writer %d line %d", w, i))
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			r.SetLive([]string{fmt.Sprint("tick ", i), "> "}, 1, 2)
			if i%10 == 0 {
				r.Resize(40-i%3, 10)
			}
		}
	}()
	wg.Wait()
	r.Resize(40, 10)
	r.SetLive([]string{"done", "> "}, 1, 2)

	all := scr.AllLines()
	if n := len(all); n != writers*each+2 || all[n-2] != "done" || all[n-1] != ">" {
		t.Fatalf("%d lines, ending %q; want %d ending with the live region", n, all[max(n-2, 0):], writers*each+2)
	}
	next := make([]int, writers)
	for _, l := range all[:len(all)-2] {
		var w, i int
		if _, err := fmt.Sscanf(l, "writer %d line %d", &w, &i); err != nil {
			t.Fatalf("a line that was never committed: %q", l)
		}
		if i != next[w] {
			t.Fatalf("writer %d: line %d came where line %d should be", w, i, next[w])
		}
		next[w]++
	}
}

package term

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/term/vtest"
)

// A panel takes the alternate screen and draws every row: what was
// committed above the live region at the left, the panel's lines at the
// right of a border, and the cursor where the live region wants it.
// Close gives the screen back as it was and returns what was committed
// meanwhile.
func TestPanel(t *testing.T) {
	scr := vtest.New(90, 10)
	scr.WriteString("the scrollback before\r\n")
	p, err := OpenPanel(scr, 90, 10, vtest.Width, false)
	if err != nil {
		t.Fatal(err)
	}
	if !scr.AltScreenEver() || !scr.MouseEver() {
		t.Fatalf("the panel did not take the alternate screen and the mouse")
	}
	cols, rows := p.Size()
	if side := panelSide(90); cols != 90-side || rows != 10 {
		t.Fatalf("Size = %d, %d", cols, rows)
	}
	if err := p.Commit("one", "two\nthree"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetLive([]string{"> typing"}, 0, 8); err != nil {
		t.Fatal(err)
	}
	if err := p.SetPanel([]string{"Files changed", "  a.go"}); err != nil {
		t.Fatal(err)
	}
	lines := scr.Lines()
	// What was committed stands at the bottom of the left part, right
	// above the live region; the panel's lines from the top.
	for row, want := range map[int]string{6: "one", 7: "two", 8: "three", 9: "> typing"} {
		if !strings.HasPrefix(lines[row], want) {
			t.Errorf("row %d = %q, want it to begin %q", row, lines[row], want)
		}
	}
	if got := lines[0]; !strings.HasSuffix(strings.TrimRight(got, " "), "│ Files changed") || strings.Index(got, "│") != cols {
		t.Errorf("row 0 = %q, want the border at column %d and the panel's first line", got, cols)
	}
	if !strings.Contains(lines[1], "│   a.go") {
		t.Errorf("row 1 = %q", lines[1])
	}
	// A line as wide as the panel keeps its last character.
	full := strings.Repeat("x", p.TextWidth()-1) + "Z"
	p.SetPanel([]string{full})
	if got := strings.TrimRight(scr.Lines()[0], " "); !strings.HasSuffix(got, "xZ") || utf8.RuneCountInString(got) != 90 {
		t.Errorf("a full panel line = %q (%d wide)", got, len(got))
	}
	p.SetPanel([]string{"Files changed", "  a.go"})
	if r, c := scr.Cursor(); r != 9 || c != 8 || !scr.CursorVisible() {
		t.Errorf("the cursor is at %d,%d (visible %v), want 9,8", r, c, scr.CursorVisible())
	}

	// More than fits: the last rows show, and scrolling back shows earlier
	// ones, with a row that says how much is below.
	for i := 0; i < 20; i++ {
		p.Commit(fmt.Sprintf("line %d", i))
	}
	if lines = scr.Lines(); !strings.HasPrefix(lines[8], "line 19") {
		t.Errorf("the last row above the live region = %q", lines[8])
	}
	page := p.Page()
	if !p.Scroll(page) || p.Back() != page {
		t.Fatalf("Scroll(%d) did not move, back = %d", page, p.Back())
	}
	lines = scr.Lines()
	if !strings.Contains(lines[8], fmt.Sprintf("↓ %d more rows below", page)) {
		t.Errorf("the row that says what is below = %q", lines[8])
	}
	if want := fmt.Sprintf("line %d", 19-page-1); !strings.HasPrefix(lines[7], want) {
		t.Errorf("scrolled back a page, row 7 = %q, want %q", lines[7], want)
	}
	// What comes while scrolled back leaves the view where it is.
	p.Commit("line 20")
	if p.Back() != page+1 || !strings.HasPrefix(scr.Lines()[7], fmt.Sprintf("line %d", 19-page-1)) {
		t.Errorf("a commit moved the view: back %d, row 7 %q", p.Back(), scr.Lines()[7])
	}
	if p.Scroll(-1000); p.Back() != 0 {
		t.Errorf("back to the end, back = %d", p.Back())
	}
	if p.Scroll(-1) {
		t.Errorf("scrolled forward past the end")
	}

	// Nothing changed: nothing is written.
	before := scr.Writes()
	p.SetLive([]string{"> typing"}, 0, 8)
	if scr.Writes() != before {
		t.Errorf("an update that changed nothing was written")
	}

	// Narrower: laid out again, the panel narrower too.
	if err := p.Resize(80, 10); err != nil {
		t.Fatal(err)
	}
	if cols, _ = p.Size(); cols != 80-panelSide(80) || strings.Index(scr.Lines()[0], "│") != cols {
		t.Errorf("after a resize the left part is %d wide, row 0 = %q", cols, scr.Lines()[0])
	}

	since, err := p.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(since) != 24 || since[0] != "one" || since[2] != "three" || since[23] != "line 20" {
		t.Errorf("Close gave back %d lines: %q", len(since), since)
	}
	if lines = scr.Lines(); !strings.HasPrefix(lines[0], "the scrollback before") {
		t.Errorf("the screen after Close = %q", lines)
	}
	if scr.Mode(1000) || scr.Mode(1006) || !scr.Mode(7) {
		t.Errorf("Close left the mouse on or wrapping off")
	}
	if err := p.Commit("after"); err != nil || strings.Contains(strings.Join(scr.Lines(), "\n"), "after") {
		t.Errorf("a closed panel drew: %v", err)
	}
}

// A view is a panel with no side: its text has the whole width. SetLines
// replaces what it shows, and a view scrolled back keeps its top row
// there; ScrollTo puts a row at the top.
func TestView(t *testing.T) {
	scr := vtest.New(40, 6)
	scr.WriteString("the scrollback before\r\n")
	v, err := OpenView(scr, 40, 6, vtest.Width, false)
	if err != nil {
		t.Fatal(err)
	}
	if cols, rows := v.Size(); cols != 40 || rows != 6 {
		t.Fatalf("Size = %d, %d, want the whole screen", cols, rows)
	}
	var lines []string
	for i := 0; i < 12; i++ {
		lines = append(lines, fmt.Sprintf("row %d", i))
	}
	lines[3] = strings.Repeat("w", 39) + "Z"
	if err := v.SetLines(lines...); err != nil {
		t.Fatal(err)
	}
	v.SetLive([]string{"footer"}, -1, 0)
	got := scr.Lines()
	if !strings.HasPrefix(got[4], "row 11") || strings.TrimRight(got[5], " ") != "footer" || strings.Contains(strings.Join(got, ""), "│") {
		t.Fatalf("the view shows %q", got)
	}
	if !v.ScrollTo(3) || v.Top() != 3 || got[0] == scr.Lines()[0] || strings.TrimRight(scr.Lines()[0], " ") != lines[3] {
		t.Fatalf("ScrollTo(3): top %d, row 0 %q", v.Top(), scr.Lines()[0])
	}
	// The view grows at the end while scrolled back: it stays put.
	v.SetLines(append(lines, "row 12", "row 13")...)
	if v.Top() != 3 || strings.TrimRight(scr.Lines()[0], " ") != lines[3] {
		t.Errorf("SetLines moved a view that was scrolled back: top %d", v.Top())
	}
	if v.ScrollTo(1000); v.Back() != 0 || !strings.HasPrefix(scr.Lines()[4], "row 13") {
		t.Errorf("ScrollTo past the end: back %d, row 4 %q", v.Back(), scr.Lines()[4])
	}
	// At the end it follows what is added.
	v.SetLines(append(lines, "row 12", "row 13", "row 14")...)
	if !strings.HasPrefix(scr.Lines()[4], "row 14") {
		t.Errorf("a view at its end did not follow: row 4 %q", scr.Lines()[4])
	}
	if _, err := v.Close(); err != nil {
		t.Fatal(err)
	}
	if got := scr.Lines(); !strings.HasPrefix(got[0], "the scrollback before") || scr.Mode(1000) {
		t.Errorf("the screen after Close = %q", got)
	}
}

// PanelFits says which terminals are wide enough for a panel.
func TestPanelFits(t *testing.T) {
	if PanelFits(PanelMinCols-1) || !PanelFits(PanelMinCols) {
		t.Errorf("PanelFits is wrong at its edge")
	}
	if panelSide(72) != panelSideMin || panelSide(300) != panelSideMax || panelSide(120) != 40 {
		t.Errorf("panelSide = %d, %d, %d", panelSide(72), panelSide(300), panelSide(120))
	}
}

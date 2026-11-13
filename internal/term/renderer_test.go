package term

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term/vtest"
)

// countingWriter passes writes on to a screen and counts them.
type countingWriter struct {
	scr    *vtest.Screen
	writes int
	bytes  int
	last   string
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	w.bytes += len(p)
	w.last = string(p)
	return w.scr.Write(p)
}

// rig is a renderer drawing on an emulated terminal, with a record of what
// the terminal should be showing.
type rig struct {
	t         *testing.T
	scr       *vtest.Screen
	out       *countingWriter
	r         *Renderer
	committed []string // every committed line, as plain text
	live      []string // the live region, as plain text
	curRow    int      // the cursor's line in the live region, or -1
	curCol    int
}

func newRig(t *testing.T, cols, rows int) *rig {
	scr := vtest.New(cols, rows)
	// tmux's way with clearing is the one that punishes a mistake.
	scr.SetScrollOnClear(true)
	out := &countingWriter{scr: scr}
	return &rig{t: t, scr: scr, out: out, r: NewRenderer(out, cols, rows, vtest.Width)}
}

// plainOf is a line as the terminal should show it: without its escape
// sequences, cut to the terminal's width, without blanks at its end.
func (g *rig) plainOf(line string) string {
	return strings.TrimRight(g.r.prepare(line).plain, " ")
}

func (g *rig) commit(lines ...string) {
	g.t.Helper()
	if err := g.r.Commit(lines...); err != nil {
		g.t.Fatal(err)
	}
	for _, l := range lines {
		g.committed = append(g.committed, g.plainOf(l))
	}
	g.check("Commit")
}

func (g *rig) setLive(row, col int, lines ...string) {
	g.t.Helper()
	if err := g.r.SetLive(lines, row, col); err != nil {
		g.t.Fatal(err)
	}
	g.live = nil
	for _, l := range lines {
		g.live = append(g.live, g.plainOf(l))
	}
	g.curRow, g.curCol = row, col
	g.check("SetLive")
}

func (g *rig) resize(cols, rows int) {
	g.t.Helper()
	g.scr.Resize(cols, rows)
	if err := g.r.Resize(cols, rows); err != nil {
		g.t.Fatal(err)
	}
}

// visibleLive is the part of the live region that fits on the screen.
func (g *rig) visibleLive() []string {
	_, rows := g.scr.Size()
	if len(g.live) > rows {
		return g.live[len(g.live)-rows:]
	}
	return g.live
}

// check fails the test unless the terminal shows every committed line
// once, in order, followed by the live region and nothing else, with the
// cursor where it was asked for and the terminal's modes as gnosh found
// them.
func (g *rig) check(after string) {
	g.t.Helper()
	fail := func(format string, args ...any) {
		g.t.Helper()
		g.t.Fatalf("after %s: %s\n%s", after, fmt.Sprintf(format, args...), g.scr.Dump())
	}
	vis := g.visibleLive()
	want := append(append([]string{}, g.committed...), vis...)
	for len(want) > 0 && want[len(want)-1] == "" {
		want = want[:len(want)-1]
	}
	got := g.scr.AllLines()
	if len(got) == 0 && len(want) == 0 {
		got, want = nil, nil
	}
	if !reflect.DeepEqual(got, want) {
		fail("the terminal's lines are\n  %q\nwant\n  %q", got, want)
	}
	row, col := g.scr.Cursor()
	row += len(g.scr.Scrollback())
	wantRow, wantCol := len(g.committed), 0
	if g.curRow >= 0 && len(vis) > 0 {
		wantRow += min(max(g.curRow-(len(g.live)-len(vis)), 0), len(vis)-1)
		cols, _ := g.scr.Size()
		wantCol = min(g.curCol, cols-1)
	}
	if row != wantRow || col != wantCol {
		fail("the cursor is on line %d col %d, want line %d col %d", row, col, wantRow, wantCol)
	}
	if shown := g.curRow >= 0; g.scr.CursorVisible() != shown {
		fail("cursor visible is %v, want %v", g.scr.CursorVisible(), shown)
	}
	if u := g.scr.Unhandled(); len(u) > 0 {
		fail("the renderer wrote sequences the terminal does not know: %q", u)
	}
	if g.scr.AltScreenEver() || g.scr.MouseEver() {
		fail("the alternate screen or mouse reporting was turned on")
	}
	if !g.scr.Mode(7) {
		fail("the terminal's line wrapping was left off")
	}
	if begun, ended := g.scr.SyncUpdates(); begun != ended || g.scr.Mode(2026) {
		fail("synchronized updates: %d begun, %d ended", begun, ended)
	}
	if g.scr.WrapPending() {
		fail("the cursor was left waiting to wrap")
	}
}

func TestCommitOnly(t *testing.T) {
	g := newRig(t, 20, 4)
	g.commit("one")
	g.commit("two", "three")
	if got := g.scr.Lines(); !reflect.DeepEqual(got, []string{"one", "two", "three", ""}) {
		t.Errorf("screen = %q", got)
	}
	g.commit("four", "five")
	if got := g.scr.Scrollback(); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Errorf("scrollback = %q, want the first two lines", got)
	}
	g.commit()
	g.commit("", "after a blank line")
	if err := g.r.Commit("two\nlines in one string\r\nand a third"); err != nil {
		t.Fatal(err)
	}
	g.committed = append(g.committed, "two", "lines in one string", "and a third")
	g.check("a commit holding line feeds")
}

func TestLiveRegion(t *testing.T) {
	g := newRig(t, 20, 6)
	g.setLive(0, 2, "> ")
	g.setLive(0, 4, "> hi")
	g.setLive(1, 0, "> hi", "status")
	g.setLive(0, 3, "> h")
	g.setLive(0, 0)
	g.setLive(2, 1, "a", "b", "c")
	g.setLive(0, 0, "x")
	g.setLive(-1, 0, "working...", "status")
	g.setLive(0, 99, "clamped")
	g.setLive(99, 3, "one", "two")
}

func TestCommitAboveLive(t *testing.T) {
	g := newRig(t, 20, 6)
	g.setLive(0, 2, "> ", "status")
	g.commit("answer line 1")
	g.commit("answer line 2", "answer line 3")
	if got := g.scr.Lines(); !reflect.DeepEqual(got, []string{"answer line 1", "answer line 2", "answer line 3", ">", "status", ""}) {
		t.Errorf("screen = %q", got)
	}
	// Past the bottom of the screen: committed lines scroll away, in
	// order, and the live region stays at the bottom.
	g.commit("4", "5", "6", "7", "8")
	g.setLive(0, 3, "> x", "status")
	if got := g.scr.Lines(); !reflect.DeepEqual(got, []string{"5", "6", "7", "8", "> x", "status"}) {
		t.Errorf("screen = %q", got)
	}
	// More lines at once than the screen has rows.
	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, fmt.Sprint("bulk ", i))
	}
	g.commit(many...)
	// A commit that swaps places with part of the live region.
	g.setLive(0, 0, "streaming a", "streaming b", "> ", "status")
	g.commit("streaming a")
	g.setLive(0, 0, "streaming b", "> ", "status")
	g.commit("streaming b", "")
	g.setLive(0, 2, "> ", "status")
}

func TestGrowAndShrink(t *testing.T) {
	g := newRig(t, 20, 5)
	g.commit("c1", "c2", "c3")
	g.setLive(0, 0, "l1")
	// Grow past the bottom: the terminal scrolls, committed lines move
	// up into the scrollback, none is lost.
	g.setLive(3, 2, "l1", "l2", "l3", "l4")
	if got := g.scr.Lines(); !reflect.DeepEqual(got, []string{"c3", "l1", "l2", "l3", "l4"}) {
		t.Errorf("screen after growing = %q", got)
	}
	// Shrink: the rows given up are erased, and nothing scrolls back.
	g.setLive(0, 1, "l1")
	if got := g.scr.Lines(); !reflect.DeepEqual(got, []string{"c3", "l1", "", "", ""}) {
		t.Errorf("screen after shrinking = %q", got)
	}
	g.setLive(1, 0, "l1", "l2 again")
	g.setLive(0, 0)
	g.commit("c4")
	g.setLive(0, 0, "a", "b", "c", "d", "e")
	g.setLive(4, 0, "a", "B", "c", "D", "e")
}

func TestLiveTallerThanScreen(t *testing.T) {
	g := newRig(t, 20, 4)
	g.commit("c1", "c2")
	lines := []string{"1", "2", "3", "4", "5", "6", "7"}
	// Only the last four rows fit; the committed lines are untouched.
	g.setLive(6, 1, lines...)
	if got := g.scr.Lines(); !reflect.DeepEqual(got, []string{"4", "5", "6", "7"}) {
		t.Errorf("screen = %q", got)
	}
	if got := g.scr.Scrollback(); !reflect.DeepEqual(got, []string{"c1", "c2"}) {
		t.Errorf("scrollback = %q: something that was not committed got above the screen", got)
	}
	// A cursor on a line that is not showing goes to the top row.
	g.setLive(0, 3, lines...)
	// Changes while clipped.
	g.setLive(6, 0, "1", "2", "3", "4", "5", "6", "seven")
	g.setLive(7, 0, "1", "2", "3", "4", "5", "6", "seven", "8")
	// Committing while clipped puts the lines above the visible rows.
	g.commit("c3")
	// Shrinking brings the rest back.
	g.setLive(2, 0, "1", "2", "3")
	g.setLive(0, 0, "1")
	g.commit("c4", "c5", "c6", "c7", "c8")
}

func TestHiddenCursorIsParkedAtTheTop(t *testing.T) {
	g := newRig(t, 20, 6)
	g.commit("c1")
	g.setLive(-1, 0, "a", "b", "c")
	if row, col := g.scr.Cursor(); row != 1 || col != 0 {
		t.Errorf("hidden cursor at row %d col %d, want the live region's top left (row 1 col 0)", row, col)
	}
	if g.scr.CursorVisible() {
		t.Error("the cursor is visible")
	}
	g.setLive(2, 1, "a", "b", "c")
	if !g.scr.CursorVisible() {
		t.Error("the cursor did not come back")
	}
}

func TestClearAndReset(t *testing.T) {
	g := newRig(t, 20, 6)
	g.commit("c1")
	g.setLive(-1, 0, "a", "b")
	if err := g.r.Clear(); err != nil {
		t.Fatal(err)
	}
	g.live, g.curRow, g.curCol = nil, 0, 0
	g.check("Clear")
	// Another program writes; then gnosh starts again below it.
	g.scr.WriteString("child output\r\n")
	g.committed = append(g.committed, "child output")
	g.setLive(0, 2, "> ")
	g.commit("c2")

	// Clear with nothing showing writes nothing.
	g.setLive(0, 0)
	before := g.out.writes
	g.r.Clear()
	if g.out.writes != before {
		t.Errorf("Clear with no live region wrote %q", g.out.last)
	}

	// Reset forgets the live region without touching the screen.
	g.setLive(0, 0, "old live")
	g.r.Reset()
	g.scr.WriteString("\r\n")
	g.committed = append(g.committed, "old live")
	g.setLive(0, 0, "new live")
}

func TestUpdatesAreSmall(t *testing.T) {
	g := newRig(t, 40, 10)
	lines := []string{"first line of the live region", "second line", "third line", "> input"}
	g.setLive(3, 7, lines...)
	full := g.out.bytes

	// Nothing changed: nothing written.
	before, writes := g.out.bytes, g.out.writes
	g.setLive(3, 7, lines...)
	if g.out.bytes != before || g.out.writes != writes {
		t.Errorf("an unchanged update wrote %d bytes: %q", g.out.bytes-before, g.out.last)
	}

	// One line changed: that line is written and the others are not.
	lines[3] = "> input!"
	before = g.out.bytes
	g.setLive(3, 8, lines...)
	wrote := g.out.bytes - before
	if wrote >= full/2 || wrote > len(lines[3])+48 {
		t.Errorf("a one-line change wrote %d bytes (a full draw is %d): %q", wrote, full, g.out.last)
	}
	for _, other := range lines[:3] {
		if strings.Contains(g.out.last, other) {
			t.Errorf("a one-line change redrew %q: %q", other, g.out.last)
		}
	}

	// Only the cursor moved: a few bytes, and no redraw.
	before = g.out.bytes
	g.setLive(3, 2, lines...)
	if wrote := g.out.bytes - before; wrote > 8 {
		t.Errorf("moving the cursor wrote %d bytes: %q", wrote, g.out.last)
	}
	before = g.out.bytes
	g.setLive(1, 2, lines...)
	if wrote := g.out.bytes - before; wrote > 12 {
		t.Errorf("moving the cursor up wrote %d bytes: %q", wrote, g.out.last)
	}

	// A line in the middle changed: the lines after it are not redrawn.
	lines[1] = "SECOND line"
	g.setLive(1, 2, lines...)
	if strings.Contains(g.out.last, "third") || strings.Contains(g.out.last, "first") {
		t.Errorf("a change in the middle redrew its neighbours: %q", g.out.last)
	}
}

func TestOneWritePerUpdate(t *testing.T) {
	g := newRig(t, 30, 5)
	step := func(what string, f func()) {
		t.Helper()
		before := g.out.writes
		f()
		if n := g.out.writes - before; n != 1 {
			t.Errorf("%s made %d writes, want 1", what, n)
		}
	}
	step("SetLive", func() { g.setLive(0, 0, "a", "b") })
	step("Commit above a live region", func() { g.commit("1", "2", "3", "4", "5", "6", "7") })
	step("Commit alone", func() { g.setLive(0, 0); g.out.writes--; g.commit("x") })
	step("Resize", func() { g.setLive(0, 0, "a"); g.out.writes--; g.resize(20, 5) })
	step("Clear", func() { g.r.Clear() })
}

func TestSynchronizedOutput(t *testing.T) {
	g := newRig(t, 30, 5)
	g.setLive(0, 0, "a")
	if begun, _ := g.scr.SyncUpdates(); begun != 0 || strings.Contains(g.out.last, "2026") {
		t.Errorf("synchronized output marks written without being asked for: %q", g.out.last)
	}
	g.r.SetSynchronized(true)
	g.setLive(0, 0, "b")
	if !strings.HasPrefix(g.out.last, SyncBegin) || !strings.HasSuffix(g.out.last, SyncEnd) {
		t.Errorf("the update is not wrapped in synchronized output marks: %q", g.out.last)
	}
	g.commit("c")
	g.resize(20, 5)
	if begun, ended := g.scr.SyncUpdates(); begun != 3 || ended != 3 {
		t.Errorf("%d updates begun and %d ended, want 3 and 3", begun, ended)
	}
	// The cursor is hidden while drawing and shown again after.
	if !strings.Contains(g.out.last, HideCursor) || !strings.Contains(g.out.last, ShowCursor) {
		t.Errorf("the cursor was not hidden while drawing: %q", g.out.last)
	}
}

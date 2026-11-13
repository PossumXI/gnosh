package term

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term/vtest"
)

// liveRig is a rig that remembers the live region as it was given, so
// that it can say what the terminal should show after a resize cuts the
// lines differently.
type liveRig struct {
	*rig
	raw []string
}

func (g *liveRig) setLive(row, col int, lines ...string) {
	g.t.Helper()
	g.raw = lines
	g.rig.setLive(row, col, lines...)
}

func (g *liveRig) resize(cols, rows int) {
	g.t.Helper()
	g.rig.resize(cols, rows)
	g.live = nil
	for _, l := range g.raw {
		g.live = append(g.live, g.plainOf(l))
	}
	g.check(fmt.Sprintf("Resize to %dx%d", cols, rows))
}

// The three ways a resize is exact: a terminal that leaves lines alone and
// a renderer told so; a terminal that wraps lines again and a renderer
// told so; and, on either kind of terminal, a hidden cursor.
var resizeSetups = []struct {
	name           string
	terminalWraps  bool
	rendererThinks bool
	cursorHidden   bool
}{
	{"terminal keeps lines, cursor shown", false, false, false},
	{"terminal wraps lines again, cursor shown", true, true, false},
	{"terminal keeps lines, cursor hidden, renderer told otherwise", false, true, true},
	{"terminal wraps lines again, cursor hidden, renderer told otherwise", true, false, true},
}

func TestResize(t *testing.T) {
	for _, setup := range resizeSetups {
		t.Run(setup.name, func(t *testing.T) {
			g := &liveRig{rig: newRig(t, 30, 10)}
			g.scr.SetReflow(setup.terminalWraps)
			g.r.SetReflow(setup.rendererThinks)
			cur := func(row int) int {
				if setup.cursorHidden {
					return -1
				}
				return row
			}
			g.commit("committed 1", "committed 2")
			long := "0123456789abcdefghij01234567"
			g.setLive(cur(2), 4, long, "second "+long[:20], "> in", "status line")

			// Narrower: the lines no longer fit.
			g.resize(20, 10)
			g.resize(12, 10)
			// The caller wraps for the new width and sets the region.
			g.setLive(cur(3), 4, long[:12], long[12:24], long[24:], "> in", "status")
			// Wider again.
			g.resize(26, 10)
			g.resize(40, 10)
			g.setLive(cur(1), 4, long, "> in", "status")
			// Shorter and taller.
			g.resize(40, 5)
			g.resize(40, 12)
			// Both at once, with commits in between.
			g.commit("committed 3")
			g.resize(15, 7)
			g.commit("committed 4")
			g.setLive(cur(0), 2, "> ", "status")
			g.resize(33, 9)
			// The cursor in the middle of a line that gets wrapped.
			g.setLive(cur(0), 25, "> a long input line that goes on", "status")
			g.resize(20, 9)
			g.resize(31, 9)
			// A resize with no live region.
			g.setLive(cur(0), 0)
			g.resize(25, 9)
			g.commit("committed 5")
			// The same size again writes nothing.
			before := g.out.writes
			g.resize(25, 9)
			if g.out.writes != before {
				t.Errorf("a resize to the same size wrote %q", g.out.last)
			}
		})
	}
}

// What happens when the renderer is told the wrong thing about the
// terminal while the cursor is shown below lines that no longer fit. These
// are the limits Resize's comment describes; the live region itself is
// always redrawn whole.
func TestResizeWithTheWrongGuess(t *testing.T) {
	long := "0123456789abcdefghij01234567"
	run := func(terminalWraps, rendererThinks bool) []string {
		g := newRig(t, 30, 10)
		g.scr.SetReflow(terminalWraps)
		g.r.SetReflow(rendererThinks)
		g.commit("committed 1", "committed 2")
		g.setLive(1, 4, long, "> in")
		g.scr.Resize(20, 10)
		g.r.Resize(20, 10)
		g.r.SetLive([]string{long[:20], long[20:], "> in"}, 2, 4)
		if row, col := g.scr.Cursor(); g.scr.Line(row) != "> in" || col != 4 {
			t.Errorf("the cursor is not in the input line:\n%s", g.scr.Dump())
		}
		return g.scr.AllLines()
	}
	// Told the terminal wraps, but it does not: the renderer goes up one
	// row too many, and the committed line just above the live region
	// is erased from the screen.
	got := run(false, true)
	want := []string{"committed 1", long[:20], long[20:], "> in"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("told it wraps, it does not:\n got %q\nwant %q", got, want)
	}
	// Told the terminal does not wrap, but it does: the renderer does
	// not go up far enough, and the first row of the old live region is
	// left behind.
	got = run(true, false)
	want = []string{"committed 1", "committed 2", long[:20], long[:20], long[20:], "> in"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("told it does not wrap, it does:\n got %q\nwant %q", got, want)
	}
}

// randomLine makes a line of the given width out of letters, blanks, wide
// characters, accents and styles.
func randomLine(rng *rand.Rand, width int) string {
	var b strings.Builder
	styles := []string{"\x1b[1m", "\x1b[31m", "\x1b[m", "\x1b[4;38;5;99m", "\x1b]8;;http://example.com/\x1b\\", "\x1b]8;;\x1b\\"}
	for used := 0; used < width; {
		switch n := rng.Intn(20); {
		case n == 0:
			b.WriteString(styles[rng.Intn(len(styles))])
		case n == 1 && used+2 <= width:
			b.WriteString("世")
			used += 2
		case n == 2:
			b.WriteString("e\u0301")
			used++
		case n < 6:
			b.WriteByte(' ')
			used++
		default:
			b.WriteByte(byte('a' + rng.Intn(26)))
			used++
		}
	}
	return b.String()
}

// TestRandomOperations drives the renderer with random commits, live
// regions and resizes, and after every step checks everything the rig
// knows how to check: each committed line is on the terminal once, in
// order; the live region is the last lines and nothing else; the cursor is
// where it was asked for; the terminal's modes are as they were found.
func TestRandomOperations(t *testing.T) {
	seeds := int64(24)
	if testing.Short() {
		seeds = 4
	}
	for _, setup := range resizeSetups {
		for seed := int64(0); seed < seeds; seed++ {
			t.Run(fmt.Sprintf("%s/seed %d", setup.name, seed), func(t *testing.T) {
				rng := rand.New(rand.NewSource(seed))
				cols, rows := 20+rng.Intn(21), 8+rng.Intn(9)
				g := &liveRig{rig: newRig(t, cols, rows)}
				g.scr.SetReflow(setup.terminalWraps)
				g.r.SetReflow(setup.rendererThinks)
				g.r.SetSynchronized(seed%2 == 0)
				maxLive := 3
				if setup.cursorHidden {
					maxLive = 6
				}
				for step := 0; step < 120; step++ {
					switch n := rng.Intn(100); {
					case n < 25:
						lines := make([]string, 1+rng.Intn(3))
						for i := range lines {
							lines[i] = randomLine(rng, rng.Intn(21))
						}
						g.commit(lines...)
					case n < 80:
						lines := make([]string, rng.Intn(maxLive+1))
						for i := range lines {
							lines[i] = randomLine(rng, rng.Intn(cols+1))
						}
						row, col := -1, 0
						if !setup.cursorHidden {
							row, col = rng.Intn(len(lines)+1), rng.Intn(cols+2)
						}
						g.setLive(row, col, lines...)
					default:
						cols, rows = 20+rng.Intn(21), 8+rng.Intn(9)
						g.resize(cols, rows)
					}
				}
			})
		}
	}
}

// TestRandomOperationsTallLive is the same without resizes, on a small
// screen, with live regions that are often taller than it.
func TestRandomOperationsTallLive(t *testing.T) {
	seeds := int64(40)
	if testing.Short() {
		seeds = 6
	}
	for seed := int64(0); seed < seeds; seed++ {
		t.Run(fmt.Sprint("seed ", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			cols, rows := 8+rng.Intn(30), 2+rng.Intn(6)
			g := newRig(t, cols, rows)
			g.r.SetSynchronized(seed%2 == 0)
			for step := 0; step < 150; step++ {
				if rng.Intn(100) < 30 {
					lines := make([]string, 1+rng.Intn(2*rows))
					for i := range lines {
						lines[i] = randomLine(rng, rng.Intn(cols+4))
					}
					g.commit(lines...)
					continue
				}
				lines := make([]string, rng.Intn(2*rows+2))
				for i := range lines {
					lines[i] = randomLine(rng, rng.Intn(cols+4))
				}
				g.setLive(rng.Intn(len(lines)+2)-1, rng.Intn(cols+2), lines...)
			}
			if err := g.r.Clear(); err != nil {
				t.Fatal(err)
			}
			g.live, g.curRow, g.curCol = nil, 0, 0
			g.check("Clear")
		})
	}
}

// A terminal one cell wide and one row high still works.
func TestTinyTerminal(t *testing.T) {
	g := &liveRig{rig: newRig(t, 1, 1)}
	g.commit("a", "bc")
	g.setLive(0, 0, "x", "y")
	g.commit("d")
	g.resize(3, 2)
	g.setLive(1, 1, "世界", "z")
	g.setLive(0, 1, "z")
	g.resize(1, 1)
	g.setLive(0, 0)

	// A size that cannot be right is taken to be 80 by 24.
	r := NewRenderer(vtest.New(80, 24), 0, -1, vtest.Width)
	if r.cols != 80 || r.rows != 24 {
		t.Errorf("size %dx%d for a terminal that reported none, want 80x24", r.cols, r.rows)
	}
}

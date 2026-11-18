package ui

import (
	"math"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// The mark that moves while gnosh works is a fall: sparkle marks that drop
// to the right, as if gravity ran sideways. A drop starts at the left edge
// of a short strip, nearly at rest, gathers speed all the way across and
// leaves at the right, its tail stretching as it goes faster.
//
// How it falls says how deep the model is in a thought. While nothing is
// thought for long, one drop at a time falls slowly, in colours so near one
// quiet grey that they are hardly colours. A thought that goes on deepens:
// first each mark takes more of a colour of its own, and then more drops
// fall, under a stronger gravity, until in a deep thought the strip is a
// shower of the palette's four colours, with glints in it. The words of
// the line say the same ("is deep in thought"), so nothing is said by the
// mark alone.
//
// Jae asked for it (18 November 2026): "do something like the yellow ->
// deep in thought thing except keep it multi colored sparkly ... start with
// more boring bland even colors first ... movement like dropping rightward,
// as if accelerating to the right, like gravity goes sideways, and when
// 'deep in thought', more things drop faster with higher acceleration."

// Fall is the look of the mark that moves while gnosh works.
type Fall struct {
	// Marks are a drop's marks: its head, and then the marks of its tail
	// from the one next to the head to the last. A short tail is made of
	// the last of them.
	Marks []string
	// Cells is how many cells wide the strip is that the drops cross.
	Cells int
	// Every is how long one step of the fall lasts while it is calm, and
	// EveryDeep in the deepest thought, when the drops are fastest.
	Every, EveryDeep time.Duration
	// Plain is what a mark is written in apart from its colour.
	Plain text.Style
	// Bland is the one colour every mark is near while the fall is calm,
	// and Hues are the colours the marks take as a thought deepens: a
	// drop's head one of them, and its tail the ones after it. Glint is
	// what a head flashes toward in a deep thought. Where Bland is the
	// terminal's own colour, a bland mark is written faint.
	Bland text.Color
	Hues  []text.Color
	Glint text.Color
}

// moves reports whether there is a fall to draw: with no colour there is
// none, and the plain mark stands still.
func (f *Fall) moves() bool {
	return len(f.Marks) > 0 && len(f.Hues) > 0 && f.Cells > 0 && f.Every > 0
}

// every is how long one step lasts at a depth.
func (f *Fall) every(depth float64) time.Duration {
	if f.EveryDeep <= 0 || f.EveryDeep >= f.Every {
		return f.Every
	}
	return f.Every - time.Duration(depth*float64(f.Every-f.EveryDeep))
}

// A thought begins to deepen once it has gone on for deepenAfter, and is
// deep at deepAfter.
const (
	deepenAfter = 5 * time.Second
	deepAfter   = 40 * time.Second
)

// depthOf says how deep a thought is that has lasted d: 0 while it is
// young, 1 once it is deep, and evenly between.
func depthOf(d time.Duration) float64 {
	return min(max(float64(d-deepenAfter)/float64(deepAfter-deepenAfter), 0), 1)
}

// How the drops fall, in cells and seconds: each pair is the calm fall's
// figure and the deepest thought's, and a depth between has the figure that
// far between them.
const (
	// The pull to the right, in cells a second every second.
	fallPull, fallPullDeep = 5.0, 30.0
	// The speed a drop starts with, in cells a second: nearly at rest.
	fallPush, fallPushDeep = 1.0, 2.0
	// How many drops are on their way across at once, on average. More
	// than three and the strip is so full that nothing in it is seen to
	// move.
	fallMany, fallManyDeep = 1.0, 3.0

	// A drop's tail is one cell, and as many more as the drop crosses in
	// fallTail seconds, up to the marks there are.
	fallTail = 0.14
	// fallDim is how far from the bland colour toward its own a mark
	// stands while the fall is calm: a tint that 256 colours round to
	// grey. In the deepest thought it is all the way there.
	fallDim = 0.15
	// The heads are bold from this much colour on.
	fallBold = 0.5
	// Glints begin at the depth fallGlintFrom; in the deepest thought a
	// head has the chance fallGlintMost of flashing, drawn again every
	// fallGlintEvery seconds; a flash goes fallGlintTo of the way to the
	// glint colour.
	fallGlintFrom  = 0.35
	fallGlintMost  = 0.3
	fallGlintEvery = 0.09
	fallGlintTo    = 0.6

	// fallSettle is how long the deepest fall takes to grow calm again
	// once its thought is over.
	fallSettle = 1500 * time.Millisecond
	// The drops are moved fallStride at a time at most, so that a late
	// frame moves them as several on time would have.
	fallStride = 40 * time.Millisecond
	// A fall that was not drawn for fallGap begins again.
	fallGap = time.Second
)

// between is the figure a fraction t of the way from a to b.
func between(a, b, t float64) float64 { return a + (b-a)*t }

// crossing is how many seconds a drop takes across cells cells, starting
// at the speed push under the pull pull.
func crossing(pull, push, cells float64) float64 {
	return (math.Sqrt(push*push+2*pull*cells) - push) / pull
}

// colour is how far from the bland colour toward its own a mark stands at
// a depth: fallDim while calm, all the way in the deepest thought, and
// most of the way well before that, so that colour is the first thing a
// thought that goes on brings.
func colour(depth float64) float64 {
	return between(fallDim, 1, 1-(1-depth)*(1-depth))
}

// drop is one falling mark and its tail.
type drop struct {
	// x is where its head is, in cells from the left edge of the strip,
	// and v how fast it falls, in cells a second.
	x, v float64
	// n is which drop of the fall it is: it chooses its colours and its
	// glints.
	n uint32
}

// tail is how many cells of tail the drop has at its speed, with marks
// marks to draw a drop with.
func (d *drop) tail(marks int) int {
	return max(min(1+int(d.v*fallTail), marks-1), 0)
}

// fall is the drops as they stand at one moment. Its zero value is a fall
// that has not begun.
type fall struct {
	// at is the moment the drops stand at.
	at time.Time
	// depth is how deep the thought is as the fall shows it: it follows
	// the thought up at once and comes down over fallSettle.
	depth float64
	// ran is how long the fall has run, in seconds.
	ran   float64
	drops []drop
	// owed is how much of the next drop has come due: at need it starts.
	owed, need float64
	// born is how many drops have started.
	born uint32
	// seed is the state of the dice that set the drops a little apart.
	seed uint32
}

// step moves the fall on to the moment now, with a thought as deep as
// depth, in a strip of f.Cells cells. The same moment twice moves nothing,
// so that drawing a frame again draws the same frame.
func (fl *fall) step(f *Fall, now time.Time, depth float64) {
	depth = min(max(depth, 0), 1)
	dt := now.Sub(fl.at)
	if fl.at.IsZero() || dt > fallGap || dt < 0 {
		// The first frame, or the first after a while (the session was
		// idle, or a question stood in the mark's place): the fall begins
		// again, with its first drop on its way.
		*fl = fall{at: now, depth: depth, drops: fl.drops[:0], seed: 0x9E3779B9}
		fl.start(depth)
		return
	}
	fl.at = now
	for dt > 0 {
		h := min(dt, fallStride)
		dt -= h
		s := h.Seconds()
		if depth >= fl.depth {
			fl.depth = depth
		} else {
			fl.depth = max(depth, fl.depth-s/fallSettle.Seconds())
		}
		fl.ran += s
		pull := between(fallPull, fallPullDeep, fl.depth)
		push := between(fallPush, fallPushDeep, fl.depth)
		keep := fl.drops[:0]
		for _, d := range fl.drops {
			d.x += d.v*s + pull*s*s/2
			d.v += pull * s
			if d.x-float64(d.tail(len(f.Marks))) < float64(f.Cells) {
				// Its head, or some of its tail, is still in the strip.
				keep = append(keep, d)
			}
		}
		fl.drops = keep
		// As many drops start as keep the right number on their way.
		fl.owed += s * between(fallMany, fallManyDeep, fl.depth) / crossing(pull, push, float64(f.Cells))
		if fl.owed >= fl.need || len(fl.drops) == 0 {
			// The strip is never empty for longer than a stride.
			fl.start(fl.depth)
		}
	}
}

// start lets the next drop go, and draws how long the one after it waits:
// between six and fourteen tenths of the usual time, so that the drops do
// not fall in step.
func (fl *fall) start(depth float64) {
	fl.drops = append(fl.drops, drop{v: between(fallPush, fallPushDeep, depth), n: fl.born})
	fl.born++
	fl.owed = 0
	fl.need = 0.6 + 0.8*fl.roll()
}

// roll is the next throw of the fall's dice, from 0 up to 1. The dice are
// its own and start the same way every time, so that a fall is the same
// fall whenever the clock is: a test can say what it shows.
func (fl *fall) roll() float64 {
	x := fl.seed
	x ^= x << 13
	x ^= x >> 17
	x ^= x << 5
	fl.seed = x
	return float64(x>>8) / (1 << 24)
}

// spot is what one cell of the strip shows: which drop, and which of its
// marks, counted from the head.
type spot struct {
	on   bool
	n    uint32
	rank int
	tail int
}

// spots lays the drops out over the cells of the strip. Where two drops
// meet, the mark nearer its own head shows, and of two as near the one
// that started first.
func (fl *fall) spots(f *Fall) []spot {
	cells := make([]spot, f.Cells)
	for i := range fl.drops {
		d := &fl.drops[i]
		head, tail := int(d.x), d.tail(len(f.Marks))
		for rank := 0; rank <= tail; rank++ {
			c := head - rank
			if c < 0 || c >= len(cells) {
				continue
			}
			if at := &cells[c]; !at.on || rank < at.rank {
				*at = spot{on: true, n: d.n, rank: rank, tail: tail}
			}
		}
	}
	return cells
}

// draw writes the strip as it stands: f.Cells cells wide, every mark in
// its colour and a blank where no drop is.
func (fl *fall) draw(f *Fall) string {
	var b strings.Builder
	for _, at := range fl.spots(f) {
		if !at.on {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(f.style(at, fl.depth, fl.glints(at)).Render(f.mark(at)))
	}
	return b.String()
}

// mark is the mark a spot shows: the head's for the head, and for the tail
// the last marks there are, so that the shortest tail is the smallest mark
// and a longer one grows toward the head.
func (f *Fall) mark(at spot) string {
	if at.rank == 0 {
		return f.Marks[0]
	}
	return f.Marks[min(max(len(f.Marks)-1-(at.tail-at.rank), 1), len(f.Marks)-1)]
}

// style is the style a spot's mark is written in at a depth: its own
// colour, as far from the bland one as the depth has brought it; a head
// bold once it has colour enough; and a head that glints, most of the way
// to the glint colour.
func (f *Fall) style(at spot, depth float64, glint bool) text.Style {
	much := colour(depth)
	c := text.Blend(f.Bland, f.Hues[(int(at.n%uint32(len(f.Hues)))+at.rank)%len(f.Hues)], much)
	st := f.Plain
	switch {
	case at.rank != 0:
	case glint:
		// Where the glint colour is the terminal's own (a light
		// background), the flash is the mark in that colour, bold.
		return st.Bold().Fg(text.Blend(c, f.Glint, fallGlintTo))
	case much >= fallBold:
		st = st.Bold()
	}
	if c == text.Default {
		// A terminal with no grey of its own to trust: bland is faint.
		return st.Dim()
	}
	return st.Fg(c)
}

// glints reports whether the head at a spot flashes now. The chance grows
// with the depth from fallGlintFrom on, and is thrown again for every drop
// every fallGlintEvery seconds.
func (fl *fall) glints(at spot) bool {
	if at.rank != 0 || fl.depth <= fallGlintFrom {
		return false
	}
	chance := fallGlintMost * (fl.depth - fallGlintFrom) / (1 - fallGlintFrom)
	// A throw that depends on nothing but which drop it is and when.
	x := at.n*0x9E3779B1 ^ uint32(fl.ran/fallGlintEvery)*0x85EBCA6B
	x ^= x >> 15
	x *= 0x2C1B3C6D
	x ^= x >> 12
	x *= 0x297A2D39
	x ^= x >> 15
	return float64(x>>8)/(1<<24) < chance
}

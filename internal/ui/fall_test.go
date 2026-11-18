package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// runFall lets a fall run from its beginning for d at one depth, a frame
// every every, and shows each frame to see.
func runFall(f *Fall, depth float64, d, every time.Duration, see func(fl *fall)) {
	var fl fall
	for at := time.Duration(0); at <= d; at += every {
		fl.step(f, stamp.Add(at), depth)
		see(&fl)
	}
}

// fg is the sequence that sets a 24-bit foreground colour, without the
// bracket and the letter around it.
func fg(c text.Color) string {
	var r, g, b int
	fmt.Sscanf(c.String(), "#%02x%02x%02x", &r, &g, &b)
	return fmt.Sprintf("38;2;%d;%d;%d", r, g, b)
}

// A thought deepens from five seconds on, and is deep at forty.
func TestDepthOfAThought(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want float64
	}{
		{0, 0}, {3 * time.Second, 0}, {5 * time.Second, 0},
		{12 * time.Second, 0.2}, {22500 * time.Millisecond, 0.5}, {33 * time.Second, 0.8},
		{40 * time.Second, 1}, {10 * time.Minute, 1}, {-time.Second, 0},
	} {
		if got := depthOf(tt.d); got != tt.want {
			t.Errorf("a thought of %v is %v deep, want %v", tt.d, got, tt.want)
		}
	}
}

// A drop starts at the left edge and gathers speed all the way across, as
// a thing does that falls: each cell takes it no longer than the one
// before, and the last far less long than the first.
func TestADropGathersSpeed(t *testing.T) {
	f := DefaultTheme(text.TrueColor).Fall
	for _, depth := range []float64{0, 0.5, 1} {
		// When the head of the fall's first drop first stood in each cell.
		reached := map[int]time.Duration{}
		var fl fall
		for at := time.Duration(0); at <= 5*time.Second; at += 5 * time.Millisecond {
			fl.step(&f, stamp.Add(at), depth)
			for _, d := range fl.drops {
				if c := int(d.x); d.n == 0 && c <= f.Cells {
					if _, ok := reached[c]; !ok {
						reached[c] = at
					}
				}
			}
		}
		if at, ok := reached[0]; !ok || at != 0 {
			t.Fatalf("depth %v: the first drop is at the left edge at %v (%v)", depth, at, ok)
		}
		last := time.Hour
		var first time.Duration
		for c := 1; c <= f.Cells; c++ {
			at, ok := reached[c]
			if !ok {
				t.Fatalf("depth %v: the first drop never reached cell %d: %v", depth, c, reached)
			}
			took := at - reached[c-1]
			if c == 1 {
				first = took
			}
			if took > last {
				t.Errorf("depth %v: cell %d took the drop %v, and the cell before it %v: it slowed down", depth, c-1, took, last)
			}
			last = took
		}
		if last*3 > first {
			t.Errorf("depth %v: the last cell took the drop %v and the first %v: it hardly gathered speed", depth, last, first)
		}
	}
}

// In a deep thought more drops fall, they cross sooner, and the pull on
// them is several times stronger.
func TestADeepThoughtFallsHarder(t *testing.T) {
	f := DefaultTheme(text.TrueColor).Fall
	type figures struct {
		mean  float64       // drops with their head in the strip, on average
		born  uint32        // drops started
		cross time.Duration // how long the first drop's head took across
		pull  float64       // the speed a drop gains in a second
	}
	measure := func(depth float64) figures {
		var fig figures
		frames, heads := 0, 0
		var v0 float64
		runFall(&f, depth, 30*time.Second, 20*time.Millisecond, func(fl *fall) {
			frames++
			for _, d := range fl.drops {
				if int(d.x) < f.Cells {
					heads++
				}
				if d.n != 0 {
					continue
				}
				if int(d.x) < f.Cells {
					fig.cross = fl.at.Sub(stamp)
				}
				switch frames {
				case 1:
					v0 = d.v
				case 6:
					fig.pull = (d.v - v0) / 0.1
				}
			}
			fig.born = fl.born
		})
		fig.mean = float64(heads) / float64(frames)
		return fig
	}
	calm, deep := measure(0), measure(1)
	if calm.mean > 1.6 || deep.mean < 2.5 {
		t.Errorf("on average %.2f drops fall while calm and %.2f in a deep thought", calm.mean, deep.mean)
	}
	if deep.born < 5*calm.born {
		t.Errorf("in thirty seconds %d drops started while calm and %d in a deep thought", calm.born, deep.born)
	}
	if deep.cross*2 > calm.cross {
		t.Errorf("a drop crosses in %v while calm and in %v in a deep thought", calm.cross, deep.cross)
	}
	if deep.pull < 4*calm.pull || calm.pull <= 0 {
		t.Errorf("a drop gains %.1f cells a second each second while calm and %.1f in a deep thought", calm.pull, deep.pull)
	}
	// Between the two it is between the two.
	if half := measure(0.5); half.mean <= calm.mean || half.mean >= deep.mean || half.cross >= calm.cross || half.cross <= deep.cross {
		t.Errorf("half way: %+v, between %+v and %+v", half, calm, deep)
	}
}

// The strip is always as wide as it says, and never empty: something falls
// in every frame, at every depth.
func TestFallFillsItsStrip(t *testing.T) {
	for _, p := range []text.Profile{text.TrueColor, text.ANSI256, text.ANSI16} {
		f := DefaultTheme(p).Fall
		if !f.moves() {
			t.Fatalf("with %v colours there is no fall", p)
		}
		for _, depth := range []float64{0, 0.25, 0.5, 0.75, 1} {
			runFall(&f, depth, 20*time.Second, f.every(depth), func(fl *fall) {
				row := text.Strip(fl.draw(&f))
				if n := text.Width(row); n != f.Cells {
					t.Fatalf("%v, depth %v: the strip %q is %d cells wide, want %d", p, depth, row, n, f.Cells)
				}
				if strings.TrimSpace(row) == "" {
					t.Fatalf("%v, depth %v: nothing falls at %v", p, depth, fl.at.Sub(stamp))
				}
				for _, r := range row {
					if r != ' ' && !strings.ContainsRune(strings.Join(f.Marks, ""), r) {
						t.Fatalf("%v, depth %v: the strip %q holds %q, which is no mark of the fall", p, depth, row, r)
					}
				}
			})
		}
	}
	if f := DefaultTheme(text.NoColor).Fall; f.moves() {
		t.Error("with no colour there is a fall")
	}
}

// A drop's tail is behind it, to the left, and ends in the smallest mark;
// the faster the drop, the longer its tail.
func TestADropsTailStretches(t *testing.T) {
	f := DefaultTheme(text.TrueColor).Fall
	longest := map[float64]int{}
	for _, depth := range []float64{0, 1} {
		runFall(&f, depth, 10*time.Second, 10*time.Millisecond, func(fl *fall) {
			var before int
			for i := range fl.drops {
				d := &fl.drops[i]
				tail := d.tail(len(f.Marks))
				if tail < 1 || tail > len(f.Marks)-1 {
					t.Fatalf("depth %v: a drop at speed %.1f has a tail of %d", depth, d.v, tail)
				}
				longest[depth] = max(longest[depth], tail)
				// Drops stand oldest first, and the older is the faster.
				if i > 0 && tail > before {
					t.Fatalf("depth %v: a younger drop has the longer tail: %+v", depth, fl.drops)
				}
				before = tail
			}
		})
	}
	if longest[0] >= longest[1] || longest[1] != len(f.Marks)-1 {
		t.Errorf("the longest tail is %d while calm and %d in a deep thought", longest[0], longest[1])
	}
	// The marks of a tail, by its length: always the smallest last.
	for tail, want := range map[int]string{1: "∙", 2: "⋆∙", 3: "✧⋆∙"} {
		got := ""
		for rank := 1; rank <= tail; rank++ {
			got += f.mark(spot{on: true, rank: rank, tail: tail})
		}
		if got != want {
			t.Errorf("a tail of %d is %q from the head back, want %q", tail, got, want)
		}
	}
}

// A drop's head stands in the cell it has reached and its tail in the
// cells to its left; what is outside the strip is not drawn; and where two
// drops meet, the mark nearer its own head shows.
func TestFallLaysDropsOut(t *testing.T) {
	f := DefaultTheme(text.TrueColor).Fall
	far := float64(f.Cells)
	for _, tt := range []struct {
		name  string
		drops []drop
		want  string
	}{
		{"just started", []drop{{x: 0.2, v: 1}}, "✦"},
		{"slow, one cell on", []drop{{x: 1.9, v: 3}}, "∙✦"},
		{"faster", []drop{{x: 4.0, v: 9}}, "  ∙⋆✦"},
		{"fast", []drop{{x: 5.3, v: 16}}, "  ∙⋆✧✦"},
		{"at the last cell", []drop{{x: far - 0.01, v: 30}}, strings.Repeat(" ", f.Cells-4) + "∙⋆✧✦"},
		{"its head gone, its tail still leaving", []drop{{x: far + 1.5, v: 12}}, strings.Repeat(" ", f.Cells-1) + "∙"},
		{"two apart", []drop{{x: 6.5, v: 9}, {x: 1.2, v: 2}}, "∙✦  ∙⋆✦"},
		{"a head over another's tail", []drop{{x: 3.2, v: 8}, {x: 2.1, v: 3}}, " ∙✦✦"},
		{"two heads in one cell", []drop{{x: 0.9, v: 2}, {x: 0.1, v: 1}}, "✦"},
	} {
		fl := fall{drops: tt.drops}
		got := text.Strip(fl.draw(&f))
		if text.Width(got) != f.Cells {
			t.Errorf("%s: the strip %q is %d cells wide, want %d", tt.name, got, text.Width(got), f.Cells)
		}
		if got = strings.TrimRight(got, " "); got != tt.want {
			t.Errorf("%s: the strip is %q, want %q", tt.name, got, tt.want)
		}
	}
}

// The same moment draws the same frame, however often it is asked for.
func TestFallStandsStillInOneMoment(t *testing.T) {
	f := DefaultTheme(text.TrueColor).Fall
	var fl fall
	for at := time.Duration(0); at <= 3*time.Second; at += 70 * time.Millisecond {
		now := stamp.Add(at)
		fl.step(&f, now, 0.8)
		first := fl.draw(&f)
		fl.step(&f, now, 0.8)
		fl.step(&f, now, 0.8)
		if again := fl.draw(&f); again != first {
			t.Fatalf("at %v the frame changed when it was drawn again:\n%q\n%q", at, first, again)
		}
	}
	// Two falls with the same clock are the same fall.
	var a, b strings.Builder
	runFall(&f, 1, 5*time.Second, 60*time.Millisecond, func(fl *fall) { a.WriteString(fl.draw(&f)) })
	runFall(&f, 1, 5*time.Second, 60*time.Millisecond, func(fl *fall) { b.WriteString(fl.draw(&f)) })
	if a.String() != b.String() {
		t.Error("two falls with the same clock fell differently")
	}
}

// A fall that was not drawn for a while begins again, and a frame that
// comes late moves the drops as the frames on time would have.
func TestFallAfterAGap(t *testing.T) {
	f := DefaultTheme(text.TrueColor).Fall
	var fl fall
	for at := time.Duration(0); at <= 2*time.Second; at += 50 * time.Millisecond {
		fl.step(&f, stamp.Add(at), 1)
	}
	if len(fl.drops) < 2 {
		t.Fatalf("after two seconds of a deep thought %d drops fall", len(fl.drops))
	}
	fl.step(&f, stamp.Add(10*time.Second), 0)
	if len(fl.drops) != 1 || fl.drops[0].x != 0 || fl.drops[0].n != 0 || fl.depth != 0 {
		t.Errorf("after a gap the fall is %+v at depth %v, and should have begun again", fl.drops, fl.depth)
	}
	// The clock going back is a gap too.
	fl.step(&f, stamp, 0.5)
	if len(fl.drops) != 1 || fl.drops[0].x != 0 {
		t.Errorf("after the clock went back the fall is %+v", fl.drops)
	}

	var steady, late fall
	for at := time.Duration(0); at <= 900*time.Millisecond; at += 20 * time.Millisecond {
		steady.step(&f, stamp.Add(at), 0.6)
	}
	late.step(&f, stamp, 0.6)
	late.step(&f, stamp.Add(900*time.Millisecond), 0.6)
	if len(steady.drops) != len(late.drops) {
		t.Fatalf("one late frame left %d drops, and frames on time %d", len(late.drops), len(steady.drops))
	}
	for i := range steady.drops {
		if d := steady.drops[i].x - late.drops[i].x; d > 1 || d < -1 {
			t.Errorf("drop %d stands at %.2f after frames on time and at %.2f after one late frame", i, steady.drops[i].x, late.drops[i].x)
		}
	}
}

// When a deep thought is over the fall grows calm over a second and a
// half, not at once; a deeper thought is followed at once.
func TestFallSettles(t *testing.T) {
	f := DefaultTheme(text.TrueColor).Fall
	var fl fall
	at := time.Duration(0)
	run := func(d time.Duration, depth float64) {
		for end := at + d; at < end; {
			at += 50 * time.Millisecond
			fl.step(&f, stamp.Add(at), depth)
		}
	}
	fl.step(&f, stamp, 0)
	run(time.Second, 1)
	if fl.depth != 1 {
		t.Fatalf("the fall follows a deep thought to %v", fl.depth)
	}
	run(750*time.Millisecond, 0)
	if fl.depth < 0.4 || fl.depth > 0.6 {
		t.Errorf("half way through settling the fall is %v deep", fl.depth)
	}
	run(time.Second, 0)
	if fl.depth != 0 {
		t.Errorf("after settling the fall is %v deep", fl.depth)
	}
	if got := f.every(0); got != f.Every {
		t.Errorf("a calm fall steps every %v", got)
	}
	if got := f.every(1); got != f.EveryDeep || got >= f.Every {
		t.Errorf("the deepest fall steps every %v", got)
	}
}

// The fall starts bland and even: every mark is nearly the one grey, and
// none is bold. In a deep thought the marks are the palette's own four
// colours, the heads bold, and some of them glint.
func TestFallTakesColourAsItDeepens(t *testing.T) {
	th := DefaultTheme(text.TrueColor)
	f := th.Fall
	pal := th.Palette
	hues := []text.Color{pal.Aqua, pal.Purple, pal.Pink, pal.Sun}
	frames := func(depth float64) string {
		var b strings.Builder
		runFall(&f, depth, 20*time.Second, f.every(depth), func(fl *fall) { b.WriteString(fl.draw(&f) + "\n") })
		return b.String()
	}

	calm := frames(0)
	if strings.Contains(calm, "\x1b[1;") {
		t.Error("a calm fall has bold marks")
	}
	for i, hue := range hues {
		if strings.Contains(calm, fg(hue)) {
			t.Errorf("a calm fall shows colour %d (%v) outright", i, hue)
		}
		bland := text.Blend(pal.Grey, hue, fallDim)
		if !strings.Contains(calm, fg(bland)) {
			t.Errorf("a calm fall lacks the bland form %v of colour %d (%v)", bland, i, hue)
		}
		// Bland is nearer the grey than its own colour, on every channel.
		var c, g, h [3]int
		fmt.Sscanf(bland.String(), "#%02x%02x%02x", &c[0], &c[1], &c[2])
		fmt.Sscanf(pal.Grey.String(), "#%02x%02x%02x", &g[0], &g[1], &g[2])
		fmt.Sscanf(hue.String(), "#%02x%02x%02x", &h[0], &h[1], &h[2])
		for k := range c {
			if abs(c[k]-g[k]) > abs(c[k]-h[k]) {
				t.Errorf("the bland form %v of %v is farther from the grey %v than from the colour", bland, hue, pal.Grey)
			}
		}
	}
	// Nothing in a calm fall is written in any colour but those four.
	rest := calm
	for _, hue := range hues {
		rest = strings.ReplaceAll(rest, "\x1b["+fg(text.Blend(pal.Grey, hue, fallDim))+"m", "")
	}
	if strings.Contains(rest, "\x1b[3") || strings.Contains(rest, "\x1b[1") {
		t.Errorf("a calm fall is written in more than its four bland colours: %q", rest[:min(len(rest), 200)])
	}

	deep := frames(1)
	glints := 0
	for i, hue := range hues {
		if !strings.Contains(deep, "\x1b[1;"+fg(hue)+"m"+f.Marks[0]) {
			t.Errorf("a deep fall has no bold head in colour %d (%v)", i, hue)
		}
		if !strings.Contains(deep, "\x1b["+fg(hue)+"m") {
			t.Errorf("a deep fall has no tail mark in colour %d (%v)", i, hue)
		}
		glints += strings.Count(deep, "\x1b[1;"+fg(text.Blend(hue, pal.Prompt, fallGlintTo))+"m"+f.Marks[0])
	}
	heads := strings.Count(deep, f.Marks[0])
	if glints == 0 || glints*2 > heads {
		t.Errorf("of %d heads in a deep fall %d glint", heads, glints)
	}

	// A thought that has only begun to deepen does not glint yet.
	early := frames(fallGlintFrom)
	for _, hue := range hues {
		c := text.Blend(pal.Grey, hue, colour(fallGlintFrom))
		if strings.Contains(early, fg(text.Blend(c, pal.Prompt, fallGlintTo))) {
			t.Errorf("a fall at depth %v glints", fallGlintFrom)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// With sixteen colours a bland mark is faint, in the terminal's own
// colour, and a deep one in the terminal's colours. On a light background
// a glint is the terminal's own colour, bold, and never faint.
func TestFallWithFewColours(t *testing.T) {
	f := DefaultTheme(text.ANSI16).Fall
	var calm, deep strings.Builder
	runFall(&f, 0, 10*time.Second, f.Every, func(fl *fall) { calm.WriteString(fl.draw(&f)) })
	runFall(&f, 1, 10*time.Second, f.EveryDeep, func(fl *fall) { deep.WriteString(fl.draw(&f)) })
	if c := calm.String(); !strings.Contains(c, "\x1b[2m") || strings.Contains(c, "\x1b[3") || strings.Contains(c, "\x1b[9") {
		t.Errorf("a calm fall with sixteen colours: %q", c[:min(len(c), 200)])
	}
	d := deep.String()
	for _, want := range []string{"\x1b[1;96m✦", "\x1b[1;35m✦", "\x1b[1;95m✦", "\x1b[1;33m✦"} {
		if !strings.Contains(d, want) {
			t.Errorf("a deep fall with sixteen colours lacks %q", want)
		}
	}
	if strings.Contains(d, "\x1b[2m") || strings.Contains(d, "\x1b[2;") {
		t.Error("a deep fall with sixteen colours has faint marks")
	}

	light := themeFor("light", text.TrueColor, true).Fall
	if light.Glint != text.Default {
		t.Fatalf("on a light background the glint colour is %v", light.Glint)
	}
	var b strings.Builder
	runFall(&light, 1, 10*time.Second, light.EveryDeep, func(fl *fall) { b.WriteString(fl.draw(&light)) })
	if got := b.String(); !strings.Contains(got, "\x1b[1m✦") || strings.Contains(got, "\x1b[2") || strings.Contains(got, "\x1b[1;2") {
		t.Errorf("on a light background a glint should be bold in the terminal's own colour: %q", got[:min(len(got), 300)])
	}
}

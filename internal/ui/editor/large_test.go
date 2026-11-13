package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// bigLines is about 380 kilobytes of text in 6,000 lines, and bigLine is
// about 300 kilobytes in one line.
var (
	bigLines = strings.Repeat("some pasted line of text, sixty-odd bytes long, give or take.\n", 6000)
	bigLine  = strings.Repeat("word ", 60000)
)

// typeAndDraw types n characters into e, drawing after each as the
// application does, and returns how long each took on average.
func typeAndDraw(e *Editor, n int) time.Duration {
	start := time.Now()
	for i := range n {
		e.Handle(term.Key{Code: term.KeyRune, Rune: 'a' + rune(i%26), Text: string('a' + rune(i%26))})
		e.Render(100)
	}
	return time.Since(start) / time.Duration(n)
}

// Hundreds of kilobytes in the box must not slow each key to a crawl,
// collapsed or not. The limit here is far above what a key takes (see the
// benchmarks); it is there to catch work that grows with the square of
// the text.
func TestLargeTextStaysQuick(t *testing.T) {
	const limit = 250 * time.Millisecond
	check := func(name string, per time.Duration) {
		t.Helper()
		t.Logf("%s: %v a key", name, per)
		if per > limit {
			t.Errorf("%s: %v a key, want under %v", name, per, limit)
		}
	}

	e := New(Options{})
	e.Render(100)
	e.Handle(term.Paste{Text: bigLines})
	if got := plainLines(e.Render(100)); len(got) != 1 || got[0] != "> [pasted 6000 lines]" {
		t.Fatalf("the paste shows as %q", got)
	}
	check("typing after a collapsed paste", typeAndDraw(e, 200))
	if e.Text() != bigLines+strings.Repeat("abcdefghijklmnopqrstuvwxyz", 8)[:200] {
		t.Fatal("the text is not the paste and what was typed")
	}

	// Expanded: six thousand lines of real text.
	e = New(Options{})
	e.Render(100)
	e.Handle(term.Paste{Text: bigLines})
	if !e.ExpandPaste() {
		t.Fatal("nothing to expand")
	}
	e.Render(100)
	check("typing at the end of 6,000 lines", typeAndDraw(e, 200))
	for range 3000 {
		e.Handle(term.Key{Code: term.KeyUp})
	}
	e.Render(100)
	check("typing in the middle of 6,000 lines", typeAndDraw(e, 200))
	start := time.Now()
	for range 500 {
		e.Handle(term.Key{Code: term.KeyUp})
		e.Render(100)
		e.Handle(term.Key{Code: term.KeyLeft})
		e.Render(100)
	}
	check("moving in 6,000 lines", time.Since(start)/1000)

	// The worst case: all of it on one line.
	e = New(Options{})
	e.Render(100)
	e.SetText(bigLine)
	e.Render(100)
	check("typing at the end of one line of 300 KB", typeAndDraw(e, 20))
	e.Handle(term.Key{Code: term.KeyHome})
	check("typing at the start of one line of 300 KB", typeAndDraw(e, 20))
	start = time.Now()
	for range 100 {
		e.Handle(term.Key{Code: term.KeyRight, Mod: term.ModAlt})
		e.Handle(term.Key{Code: term.KeyLeft})
		e.Render(100)
	}
	check("moving in one line of 300 KB", time.Since(start)/200)
	if e.Text() == bigLine {
		t.Fatal("nothing was typed")
	}
}

func BenchmarkTypeAfterCollapsedPaste(b *testing.B) {
	e := New(Options{})
	e.Render(100)
	e.Handle(term.Paste{Text: bigLines})
	b.ResetTimer()
	for i := range b.N {
		e.Handle(term.Key{Code: term.KeyRune, Rune: 'x', Text: "x"})
		e.Render(100)
		if i%200 == 199 {
			// Keep the line the paste is on from growing without end.
			b.StopTimer()
			for range 200 {
				e.Handle(term.Key{Code: term.KeyBackspace})
			}
			b.StartTimer()
		}
	}
}

func BenchmarkTypeInManyLines(b *testing.B) {
	e := New(Options{})
	e.Render(100)
	e.SetText(bigLines)
	e.Render(100)
	b.ResetTimer()
	for range b.N {
		e.Handle(term.Key{Code: term.KeyRune, Rune: 'x', Text: "x"})
		e.Render(100)
	}
}

func BenchmarkTypeInOneLongLine(b *testing.B) {
	e := New(Options{})
	e.Render(100)
	e.SetText(bigLine)
	e.Render(100)
	b.ResetTimer()
	for range b.N {
		e.Handle(term.Key{Code: term.KeyRune, Rune: 'x', Text: "x"})
		e.Render(100)
	}
}

func BenchmarkTypeShortPrompt(b *testing.B) {
	e := New(Options{})
	e.Render(100)
	b.ResetTimer()
	for i := range b.N {
		e.Handle(term.Key{Code: term.KeyRune, Rune: 'x', Text: "x"})
		e.Render(100)
		if i%300 == 299 {
			e.Reset()
		}
	}
}

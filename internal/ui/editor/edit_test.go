package editor

import (
	"fmt"
	"strings"
	"testing"
)

// What the word and line keys delete is kept, and Ctrl+Y puts it back.
func TestKillAndYank(t *testing.T) {
	g := newRig(t, 40, Options{})
	g.do(`"hello world" <Left>*6 <Ctrl+K>`).text("hello")
	g.do(`<Ctrl+Y>`).text("hello world").cursor(11)
	g.do(`<Ctrl+Y>`).text("hello world world")
	// Each of the keys that keep.
	for _, c := range []struct{ setup, key, left, kept string }{
		{`"one two" <Home>`, `<Ctrl+K>`, "", "one two"},
		{`"one two"`, `<Ctrl+U>`, "", "one two"},
		{`"one two"`, `<Ctrl+W>`, "one ", "two"},
		{`"one two"`, `<Alt+Backspace>`, "one ", "two"},
		{`"one two"`, `<Ctrl+Backspace>`, "one ", "two"},
		{`"one two" <Home>`, `<Alt+D>`, " two", "one"},
		{`"one two" <Home>`, `<Alt+Delete>`, " two", "one"},
		{`"one" <Ctrl+J> "two" <Up> <End>`, `<Ctrl+K>`, "onetwo", "\n"},
	} {
		g := newRig(t, 40, Options{})
		g.do(c.setup).do(c.key).text(c.left)
		g.do(`<Ctrl+End> "|" <Ctrl+Y>`).text(c.left + "|" + c.kept)
	}
	// Backspace and Delete do not keep what they take.
	g = newRig(t, 40, Options{})
	g.do(`"ab" <Backspace> <Home> <Delete> <Ctrl+Y>`).text("")
	// Nothing deleted yet: Ctrl+Y and Alt+Y do nothing.
	g = newRig(t, 40, Options{})
	g.do(`"ab" <Ctrl+Y> <Alt+Y>`).text("ab")
}

// Deletions made one after another are kept as one piece, in the order
// they stood in the text.
func TestKillsJoin(t *testing.T) {
	g := newRig(t, 40, Options{})
	g.do(`"one two three" <Home> <Alt+D> <Alt+D>`).text(" three")
	g.do(`<End> <Ctrl+Y>`).text(" threeone two")

	g = newRig(t, 40, Options{})
	g.do(`"one two three" <Ctrl+W> <Ctrl+W>`).text("one ")
	g.do(`<Ctrl+Y>`).text("one two three")

	// Forwards and backwards together.
	g = newRig(t, 40, Options{})
	g.do(`"one two three" <Left>*6 <Ctrl+W> <Ctrl+K>`).text("one ")
	g.do(`<Ctrl+Y>`).text("one two three")

	// Any other key between them starts a new piece.
	g = newRig(t, 40, Options{})
	g.do(`"one two three" <Ctrl+W> <Left> <Right> <Ctrl+W>`).text("one ")
	g.do(`<Ctrl+Y>`).text("one two ")
	// Alt+Y, straight after, swaps it for the one before, going round.
	g.do(`<Alt+Y>`).text("one three")
	g.do(`<Alt+Y>`).text("one two ")
	g.do(`<Alt+Y>`).text("one three").cursor(9)
	// Alt+Y after anything else does nothing.
	g.do(`<Left> <Alt+Y>`).text("one three")
	g.do(`"x" <Alt+Y>`).text("one threxe")
}

// Only so many deletions are kept.
func TestKillRingIsBounded(t *testing.T) {
	g := newRig(t, 40, Options{})
	for i := range killRingSize + 4 {
		g.do(fmt.Sprintf(`"w%d" <Ctrl+W> <Left>`, i))
	}
	if len(g.e.ring) != killRingSize {
		t.Fatalf("%d deletions kept, want %d", len(g.e.ring), killRingSize)
	}
	g.do(`<Ctrl+Y>`).text(fmt.Sprintf("w%d", killRingSize+3))
	for range killRingSize - 1 {
		g.do(`<Alt+Y>`)
	}
	g.text("w4")
	g.do(`<Alt+Y>`).text(fmt.Sprintf("w%d", killRingSize+3))
}

// Undo takes back a word at a time, and redo makes it again.
func TestUndoRedo(t *testing.T) {
	g := newRig(t, 40, Options{})
	g.do(`"hello  world again"`)
	g.do(`<Ctrl+_>`).text("hello  world ").cursor(13)
	g.do(`<Ctrl+_>`).text("hello  ").cursor(7)
	g.do(`<Ctrl+_>`).text("").cursor(0)
	g.do(`<Ctrl+_>`).text("")
	g.do(`<Alt+/>`).text("hello  ").cursor(7)
	g.do(`<Alt+/> <Alt+/>`).text("hello  world again").cursor(18)
	g.do(`<Alt+/>`).text("hello  world again")
	// A new edit after an undo ends what could be made again.
	g.do(`<Ctrl+_> "x" <Alt+/>`).text("hello  world x")
	g.do(`<Ctrl+_>`).text("hello  world ")

	// The other ways a terminal sends Ctrl+_.
	for _, k := range []string{`<Ctrl+/>`, `<Ctrl+Shift+->`, `<Ctrl+Shift+_>`} {
		newRig(t, 40, Options{}).do(`"ab"`).do(k).text("")
	}

	// Typing somewhere else starts a new step, and undo puts the cursor
	// back where the change was made.
	g = newRig(t, 40, Options{})
	g.do(`"ac" <Left> "b" <End> "d"`).text("abcd")
	g.do(`<Ctrl+_>`).text("abc").cursor(3)
	g.do(`<Ctrl+_>`).text("ac").cursor(1)
	g.do(`<Ctrl+_>`).text("").cursor(0)

	// Backspaces in a row are one step; so are Deletes.
	g = newRig(t, 40, Options{})
	g.do(`"abcdef" <Backspace>*3`).text("abc")
	g.do(`<Ctrl+_>`).text("abcdef").cursor(6)
	g.do(`<Home> <Delete>*2 <Ctrl+D>`).text("def")
	g.do(`<Ctrl+_>`).text("abcdef").cursor(0)
	// A Backspace after a Delete is its own step.
	g.do(`<Right>*2 <Delete> <Backspace>`).text("adef")
	g.do(`<Ctrl+_>`).text("abdef")
	g.do(`<Ctrl+_>`).text("abcdef")

	// A new line is its own step, and so is every word key.
	g = newRig(t, 40, Options{})
	g.do(`"ab" <Ctrl+J> "cd ef" <Ctrl+W> <Ctrl+U>`).text("ab\n")
	g.do(`<Ctrl+_>`).text("ab\ncd ")
	g.do(`<Ctrl+_>`).text("ab\ncd ef")
	g.do(`<Ctrl+_>`).text("ab\ncd ")
	g.do(`<Ctrl+_>`).text("ab\n")
	g.do(`<Ctrl+_>`).text("ab")
	g.do(`<Ctrl+_>`).text("")
	g.do(`<Alt+/>*6`).text("ab\n")

	// Ctrl+Y and Alt+Y are steps too.
	g = newRig(t, 40, Options{})
	g.do(`"one two" <Ctrl+W> <Left> <Ctrl+W> <Ctrl+Y> <Alt+Y>`).text("two ")
	g.do(`<Ctrl+_>`).text("one ")
	g.do(`<Ctrl+_>`).text(" ")
	g.do(`<Ctrl+_>`).text("one ")
	g.do(`<Ctrl+_>`).text("one two")
}

// Clearing the box, putting it aside and loading text are steps of the
// undo history, so nothing typed is ever out of reach.
func TestUndoReachesEverything(t *testing.T) {
	g := newRig(t, 40, Options{})
	g.do(`"draft" <Ctrl+C>`).text("")
	g.do(`<Ctrl+_>`).text("draft")
	g.do(`<Esc> <Esc>`).text("")
	g.do(`<Ctrl+_>`).text("draft")
	g.do(`<Ctrl+S>`).text("")
	g.do(`<Ctrl+_>`).text("draft")
	g.e.SetText("loaded")
	g.text("loaded").cursor(6)
	g.do(`<Ctrl+_>`).text("draft")
	g.e.Insert(" more")
	g.text("draft more")
	g.do(`"!" <Ctrl+_>`).text("draft more")
	g.do(`<Ctrl+_>`).text("draft")
	// Reset is the one thing that forgets.
	g.e.Reset()
	g.text("").do(`<Ctrl+_>`).text("")
}

// Only so many steps are kept.
func TestUndoIsBounded(t *testing.T) {
	g := newRig(t, 40, Options{UndoLimit: 3})
	g.do(`"a" <Ctrl+J> "b" <Ctrl+J> "c" <Ctrl+J> "d"`).text("a\nb\nc\nd")
	g.do(`<Ctrl+_>*10`).text("a\nb\n")
}

// Ctrl+S puts the text aside and brings it back, cursor and all.
func TestStash(t *testing.T) {
	g := newRig(t, 40, Options{})
	g.do(`"first thought" <Left>*3 <Ctrl+S>`).text("")
	if g.last.Hint != HintStashed || g.e.Stashed() != 1 {
		t.Errorf("Ctrl+S: %+v, %d put aside", g.last, g.e.Stashed())
	}
	g.do(`"!ls" <Enter>`).submitted("!ls", ModeShell)
	g.do(`<Ctrl+S>`).text("first thought").cursor(10)
	if g.e.Stashed() != 0 {
		t.Errorf("%d still put aside", g.e.Stashed())
	}
	// With nothing put aside, Ctrl+S on an empty box does nothing.
	g.do(`<Ctrl+U> <Ctrl+K> <Ctrl+S>`).text("")
	// Putting aside twice keeps both; the last comes back first.
	g.do(`"one" <Ctrl+S> "two" <Ctrl+S>`).text("")
	g.do(`<Ctrl+S>`).text("two")
	g.do(`<Ctrl+U> <Ctrl+S>`).text("one")
	// The mode comes back with the text.
	g = newRig(t, 40, Options{})
	g.do(`"!make" <Ctrl+S> <Ctrl+S>`).text("!make")
	if g.e.Mode() != ModeShell {
		t.Errorf("mode after bringing a shell line back: %v", g.e.Mode())
	}
}

// A bracketed paste goes in as it is, never sends, and is one step.
func TestPaste(t *testing.T) {
	g := newRig(t, 40, Options{})
	g.do(`"a" <Paste "one\ntwo\n"> "b"`).text("aone\ntwo\nb")
	for _, r := range g.all {
		if r.Action != ActionNone || !r.Handled {
			t.Errorf("a paste or a key around it: %+v", r)
		}
	}
	g.do(`<Ctrl+_>`).text("aone\ntwo\n")
	g.do(`<Ctrl+_>`).text("a")
	g.do(`<Alt+/>`).text("aone\ntwo\n").cursor(9)
	// An empty paste is nothing.
	g.do(`<Paste "">`).text("aone\ntwo\n")
	if g.last.Changed {
		t.Error("an empty paste changed the text")
	}
	// What a terminal cut short is said to be so.
	g.do(`<PasteCut "half">`).text("aone\ntwo\nhalf")
	if !g.last.Truncated {
		t.Error("a paste cut short was not flagged")
	}
	g.do(`<Paste "whole">`)
	if g.last.Truncated {
		t.Error("a whole paste was flagged as cut short")
	}
	// Line ends of every kind are line breaks; bytes that are not text,
	// and the code points the editor keeps for itself, are replaced.
	g = newRig(t, 40, Options{})
	g.e.Handle(pasteOf("a\r\nb\rc\xffd\uFDD0e\uFDD1f\uFDD2g\uFDEF"))
	g.text("a\nb\nc\uFFFDd\uFFFDe\uFFFDf\uFFFDg\uFFFD")
	// Its neighbours in Unicode are text like any other.
	g = newRig(t, 40, Options{})
	g.e.Handle(pasteOf("\uFDCF\uFDF0"))
	g.text("\uFDCF\uFDF0")
	// Control characters stay in the text and are drawn harmless.
	g = newRig(t, 40, Options{})
	g.e.Handle(pasteOf("x\x1b]0;title\x07y\x00\u0085z"))
	g.text("x\x1b]0;title\x07y\x00\u0085z").screen(0, 27, "> x^[]0;title^Gy^@<U+0085>z")
}

// A large paste is one placeholder that sends its whole text.
func TestLargePaste(t *testing.T) {
	eleven := strings.Repeat("line\n", 11)
	g := newRig(t, 40, Options{})
	g.do(`"see "`)
	g.e.Handle(pasteOf(eleven))
	g.do(`" ok"`).text("see "+eleven+" ok").screen(0, 26, "> see [pasted 11 lines] ok")
	if d := g.e.Draft(); len(d.Pastes) != 1 || d.Text[d.Pastes[0].Start:d.Pastes[0].End] != eleven {
		t.Fatalf("draft: %+v", d)
	}
	// It is one step for the cursor and one delete.
	g.do(`<Left>*3`).cursor(4+len(eleven)).screen(0, 23, "> see [pasted 11 lines] ok")
	g.do(`<Left>`).cursor(4).screen(0, 6, "> see [pasted 11 lines] ok")
	g.do(`<Right>`).cursor(4 + len(eleven))
	g.do(`<Backspace>`).text("see  ok").cursor(4)
	g.do(`<Ctrl+_>`).text("see "+eleven+" ok").screen(0, 23, "> see [pasted 11 lines] ok")
	g.do(`<Left> <Delete>`).text("see  ok")
	g.do(`<Ctrl+_>`)
	// The word keys take it whole, and Ctrl+Y brings it back collapsed.
	g.do(`<End> <Left>*3 <Alt+Backspace>`).text("see  ok")
	g.do(`<Ctrl+Y>`).text("see "+eleven+" ok").screen(0, 23, "> see [pasted 11 lines] ok")
	g.do(`<Ctrl+W>`).text("see  ok")
	g.do(`<Ctrl+Y> <Left> <Alt+D>`).text("see  ok")
	g.do(`<Ctrl+Y> <Home> <Ctrl+K> <Ctrl+Y>`).text("see "+eleven+" ok").screen(0, 26, "> see [pasted 11 lines] ok")
	// The word keys stop at it: it is a word by itself.
	g.do(`<Alt+B>`).cursor(4 + len(eleven) + 1)
	g.do(`<Alt+B>`).cursor(4)
	g.do(`<Alt+F>`).cursor(4 + len(eleven))
	// Sending it sends everything, and says which part was pasted.
	g.do(`<Enter>`).submitted("see "+eleven+" ok", ModePrompt)
	if len(g.last.Pastes) != 1 || g.last.Pastes[0] != (Span{4, 4 + len(eleven)}) {
		t.Errorf("pastes of the submit: %+v", g.last.Pastes)
	}
}

// When a paste is collapsed: more than ten lines or more than 800
// characters, counted as a person counts them.
func TestPasteThresholds(t *testing.T) {
	cases := []struct {
		name  string
		paste string
		opts  Options
		label string // empty when it is not collapsed
	}{
		{"ten lines", strings.Repeat("x\n", 9) + "x", Options{}, ""},
		{"ten lines and a line break", strings.Repeat("x\n", 10), Options{}, ""},
		{"eleven lines", strings.Repeat("x\n", 10) + "x", Options{}, "[pasted 11 lines]"},
		{"eleven lines and a line break", strings.Repeat("x\n", 11), Options{}, "[pasted 11 lines]"},
		{"800 characters", strings.Repeat("x", 800), Options{}, ""},
		{"801 characters", strings.Repeat("x", 801), Options{}, "[pasted 801 characters]"},
		{"800 characters of three bytes each", strings.Repeat("世", 800), Options{}, ""},
		{"801 characters on two lines", strings.Repeat("x", 400) + "\n" + strings.Repeat("y", 400), Options{}, "[pasted 2 lines]"},
		{"limits of the application's choosing", "a\nb\nc\nd", Options{PasteLines: 3}, "[pasted 4 lines]"},
		{"a character limit of the application's choosing", "abcdef", Options{PasteChars: 5}, "[pasted 6 characters]"},
		{"limits switched off", strings.Repeat("x\n", 50) + strings.Repeat("y", 2000), Options{PasteLines: -1, PasteChars: -1}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newRig(t, 60, c.opts)
			g.e.Handle(pasteOf(c.paste))
			g.text(c.paste)
			d := g.e.Draft()
			first := plainLines(g.e.Render(60))[0]
			if c.label == "" {
				if len(d.Pastes) != 0 || strings.Contains(first, "[pasted") {
					t.Fatalf("collapsed: %q, pastes %v", first, d.Pastes)
				}
				return
			}
			if len(d.Pastes) != 1 || d.Pastes[0] != (Span{0, len(c.paste)}) || first != "> "+c.label {
				t.Fatalf("first row %q, pastes %v, want %q", first, d.Pastes, c.label)
			}
		})
	}
}

// Tab on a placeholder turns it back into its text; undo collapses it
// again.
func TestExpandPaste(t *testing.T) {
	twelve := strings.Repeat("ab\n", 11) + "end"
	g := newRig(t, 40, Options{MaxHeight: 4})
	g.do(`"x "`)
	g.e.Handle(pasteOf(twelve))
	g.screen(0, 21, "> x [pasted 12 lines]")
	g.do(`<Tab>`).text("x "+twelve).cursor(2+len(twelve)).screen(3, 5, "↑ ab", "  ab", "  ab", "  end")
	if len(g.e.Draft().Pastes) != 0 {
		t.Errorf("still a paste after expanding: %+v", g.e.Draft().Pastes)
	}
	g.do(`<Ctrl+_>`).screen(0, 21, "> x [pasted 12 lines]")
	// With the cursor just before it, too.
	g.do(`<Left> <Tab>`).cursor(2 + len(twelve))
	g.do(`<Ctrl+_> <Home>`)
	// Not touching it, Tab leaves it alone.
	g.do(`<Tab>`).screen(0, 2, "> x [pasted 12 lines]")
	if g.e.ExpandPaste() {
		t.Error("ExpandPaste found a paste the cursor does not touch")
	}
}

// A draft with pastes in it, loaded from outside or recalled from the
// history, is shown the way it was typed.
func TestDraftsKeepPastes(t *testing.T) {
	text := "look: " + strings.Repeat("x\n", 20) + " and " + strings.Repeat("y", 900) + "!"
	d := Draft{Text: text, Pastes: []Span{{6, 46}, {51, 951}}}
	g := newRig(t, 60, Options{})
	g.e.SetDraft(d)
	g.text(text).screen(0, 54, "> look: [pasted 20 lines] and [pasted 900 characters]!")
	if got := g.e.Draft(); fmt.Sprint(got.Pastes) != fmt.Sprint(d.Pastes) || got.Text != text {
		t.Errorf("draft came back as %v", got.Pastes)
	}
	// Through the history and back.
	g.do(`<Enter>`)
	g.e.AddHistory(Draft{Text: g.last.Text, Pastes: g.last.Pastes})
	g.do(`<Up>`).text(text).screen(0, 54, "> look: [pasted 20 lines] and [pasted 900 characters]!")
	// Spans that make no sense are left out; the text is kept.
	for _, bad := range [][]Span{{{5, 3}}, {{0, 9999}}, {{10, 20}, {15, 30}}, {{-1, 4}}} {
		g.e.SetDraft(Draft{Text: "0123456789012345678901234567890", Pastes: bad})
		g.text("0123456789012345678901234567890")
		if len(g.e.Draft().Pastes) != 0 {
			t.Errorf("spans %v gave pastes %v", bad, g.e.Draft().Pastes)
		}
	}
	// An empty span is no paste.
	g.e.SetDraft(Draft{Text: "abc", Pastes: []Span{{1, 1}}})
	g.text("abc").screen(0, 5, "> abc")
}

// The cursor as the application sees it: a byte offset into the text.
func TestCursorOffsets(t *testing.T) {
	g := newRig(t, 60, Options{})
	g.e.SetDraft(Draft{Text: "ab\ncdEFGHij\nk世l", Pastes: []Span{{5, 9}}})
	g.cursor(17)
	for _, c := range []struct{ set, want int }{
		{0, 0}, {2, 2}, {3, 3}, {5, 5}, {9, 9}, {11, 11}, {12, 12}, {13, 13}, {16, 16}, {17, 17},
		// Inside the paste: to its end. Inside a character: to its start.
		{6, 9}, {8, 9}, {14, 13}, {15, 13},
		// Outside the text: to its ends.
		{-5, 0}, {99, 17},
	} {
		g.e.SetCursor(c.set)
		if got := g.e.Cursor(); got != c.want {
			t.Errorf("SetCursor(%d): cursor at %d, want %d", c.set, got, c.want)
		}
	}
	// Inside a combining sequence: to its start.
	g.e.SetText("ae\u0301\u0302b")
	g.e.SetCursor(4)
	g.cursor(1)
	// In front of the character that sets the mode: after it.
	g.e.SetText("!ls")
	g.e.SetCursor(0)
	g.cursor(1)
}

// An edit can make two items into one: a letter typed in front of a
// combining mark takes the mark, and two characters left side by side may
// join. The cursor is never left inside the item that results.
func TestCursorSettlesAfterItemsJoin(t *testing.T) {
	// Typing in front of a mark that stood alone.
	g := newRig(t, 30, Options{})
	g.do(`<Paste "\u0301z"> <Home>`).screen(0, 2, "> <U+0301>z")
	g.do(`"e"`).text("e\u0301z").cursor(3).screen(0, 3, "> e\u0301z")
	g.do(`"x"`).text("e\u0301xz")
	// Undo takes each step back exactly.
	g.do(`<Ctrl+_>`).text("e\u0301z").cursor(3)
	g.do(`<Ctrl+_>`).text("\u0301z").cursor(0)
	g.do(`<Alt+/>`).text("e\u0301z").cursor(3)

	// Joining two lines whose ends make one emoji.
	g = newRig(t, 30, Options{})
	g.do(`"👨" <Ctrl+J> "\u200D👩" <Home>`).cursor(5)
	g.do(`<Backspace>`).text("👨\u200D👩").cursor(11).screen(0, 4, "> 👨\u200D👩")
	g.do(`<Backspace>`).text("")

	// Deleting what stood between a letter and a mark.
	g = newRig(t, 30, Options{})
	g.do(`<Paste "e\x00\u0308"> <Home> <Right>`).cursor(1).screen(0, 3, "> e^@<U+0308>")
	g.do(`<Delete>`).text("e\u0308").cursor(3).screen(0, 3, "> e\u0308")
	g = newRig(t, 30, Options{})
	g.do(`"e"`)
	g.e.Handle(pasteOf(strings.Repeat("x\n", 12)))
	g.do(`<Paste "\u0308">`).screen(0, 28, "> e[pasted 12 lines]<U+0308>")
	g.do(`<Left> <Backspace>`).text("e\u0308").cursor(3)

	// Text put back in front of a mark: Alt+Y swaps exactly what was put
	// back, and leaves the mark.
	g = newRig(t, 30, Options{})
	g.do(`"cd" <Ctrl+W> "x" <Backspace> "ab" <Ctrl+W> <Paste "\u0301z"> <Home>`).text("\u0301z")
	g.do(`<Ctrl+Y>`).text("ab\u0301z").cursor(4)
	g.do(`<Alt+Y>`).text("cd\u0301z").cursor(4)
	g.do(`<Alt+Y>`).text("ab\u0301z")
	g.do(`<Ctrl+_>*2`).text("ab\u0301z").do(`<Ctrl+_>`).text("\u0301z")
}

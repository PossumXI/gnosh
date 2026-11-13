package editor

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

// sound fails the test unless everything that must always be true of an
// editor is true of e.
func sound(t *testing.T, e *Editor, when string) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("%s: %s\ntext %q", when, fmt.Sprintf(format, args...), e.whole())
	}
	if len(e.lines) == 0 {
		fail("no lines")
	}
	for i := range e.lines {
		s := e.lines[i].s
		if strings.ContainsAny(s, "\n\r") {
			fail("line %d holds a line end", i)
		}
		if !utf8.ValidString(s) {
			fail("line %d is not UTF-8", i)
		}
		// The marks of collapsed pastes pair up and do not nest.
		for rest := s; rest != ""; {
			open := strings.Index(rest, pasteOpen)
			if open < 0 {
				if strings.Contains(rest, pasteClose) || strings.Contains(rest, pasteBreak) {
					fail("line %d has a stray mark", i)
				}
				break
			}
			if strings.Contains(rest[:open], pasteClose) || strings.Contains(rest[:open], pasteBreak) {
				fail("line %d has a stray mark", i)
			}
			rest = rest[open+len(pasteOpen):]
			end := strings.Index(rest, pasteClose)
			if end < 0 {
				fail("line %d has a paste that does not end", i)
			}
			if strings.Contains(rest[:end], pasteOpen) {
				fail("line %d has a paste inside a paste", i)
			}
			if end == 0 {
				fail("line %d has an empty paste", i)
			}
			rest = rest[end+len(pasteClose):]
		}
		// What is remembered about its pastes is true of it.
		for off, p := range e.lines[i].pastes {
			if !strings.HasPrefix(s[off:], pasteOpen) || p.end != e.itemEnd(s, off) || p.label != pasteLabel(s[off:p.end]) {
				fail("line %d remembers a paste at %d to %d (%s) that is not there", i, off, p.end, p.label)
			}
		}
		// Its rows, when worked out, cover it exactly.
		if rows := e.lines[i].rows; rows != nil {
			if rows[0].start != e.home(i) || rows[len(rows)-1].end != len(s) {
				fail("rows of line %d do not span it: %v", i, rows)
			}
			for j := 1; j < len(rows); j++ {
				if rows[j].start != rows[j-1].end || rows[j].end < rows[j].start {
					fail("rows of line %d do not join up: %v", i, rows)
				}
			}
		}
	}
	// The cursor is in the text, on the edge of an item, and not in
	// front of the character that sets the mode.
	if e.cur.line < 0 || e.cur.line >= len(e.lines) {
		fail("cursor on line %d of %d", e.cur.line, len(e.lines))
	}
	s, home := e.lines[e.cur.line].s, e.home(e.cur.line)
	if e.cur.col < home || e.cur.col > len(s) {
		fail("cursor at %d in a line of %d that starts at %d", e.cur.col, len(s), home)
	}
	off := home
	for off < e.cur.col {
		off = e.itemEnd(s, off)
	}
	if off != e.cur.col {
		fail("cursor at %d is inside an item of line %q", e.cur.col, s)
	}
	if e.walk < -1 || e.walk >= len(e.hist) || e.top < 0 {
		fail("history entry %d of %d, first row %d", e.walk, len(e.hist), e.top)
	}
	// What the application reads is whole and consistent.
	text := e.Text()
	if !utf8.ValidString(text) || strings.Contains(text, pasteOpen) || strings.Contains(text, pasteClose) || strings.Contains(text, pasteBreak) || strings.Contains(text, "\r") {
		fail("Text() is not clean: %q", text)
	}
	if c := e.Cursor(); c < 0 || c > len(text) {
		fail("Cursor() is %d in a text of %d", c, len(text))
	}
	d := e.Draft()
	if d.Text != text {
		fail("Draft().Text differs from Text()")
	}
	at := 0
	for _, sp := range d.Pastes {
		if sp.Start < at || sp.End <= sp.Start || sp.End > len(text) {
			fail("Draft().Pastes are not in order: %v", d.Pastes)
		}
		at = sp.End
	}
	// A draft loaded into another box is the same text, collapsed the
	// same way.
	other := New(Options{})
	other.SetDraft(d)
	if other.whole() != e.whole() {
		fail("the draft does not load back: %q", other.whole())
	}
	if e.IsEmpty() != (text == "") {
		fail("IsEmpty() is %v", e.IsEmpty())
	}
}

// pool is every kind of event a session can hold. A byte of fuzz input
// picks one.
var pool = func() []term.Event {
	evs := keyscript.MustParse(`
		"a" "b" "c" "X" " " " " "-" "." "/" "!" "@" "?" "\\" "世" "界" "👍" "👨" "\u200D" "\u0301" "é" "🇺" "🇸"
		<Enter> <Shift+Enter> <Alt+Enter> <Ctrl+J> <Ctrl+Enter> <Ctrl+M>
		<Left> <Right> <Up> <Down> <Home> <End> <PgUp> <PgDn>
		<Left> <Right> <Up> <Down>
		<Ctrl+A> <Ctrl+E> <Ctrl+F> <Ctrl+P> <Ctrl+N> <Ctrl+Home> <Ctrl+End> <Alt+<> <Alt+>>
		<Alt+B> <Alt+F> <Alt+Left> <Alt+Right> <Ctrl+Left> <Ctrl+Right>
		<Backspace> <Backspace> <Delete> <Ctrl+H> <Ctrl+D>
		<Alt+Backspace> <Ctrl+W> <Alt+D> <Alt+Delete> <Ctrl+K> <Ctrl+U>
		<Ctrl+Y> <Alt+Y> <Ctrl+_> <Ctrl+_> <Alt+/>
		<Tab> <Esc> <Ctrl+C> <Ctrl+R> <Ctrl+S> <Ctrl+G>
		<Shift+Tab> <F5> <Ctrl+L> <Focus> <Blur> <Resize 30x10>
		<Paste "x"> <Paste "one two three"> <Paste "a\nb"> <Paste "\t"> <Paste "tab\tbed">
		<Paste "\x1b[31mred\x07"> <Paste "!sh"> <Paste "/cmd"> <Paste "@sr"> <Paste ""> <Paste "e\u0301\u200B">
		<Paste "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12"> <PasteCut "cut sh">
	`)
	return append(evs, term.Paste{Text: strings.Repeat("long ", 180)}, term.Paste{Text: "bad \xff\xfe bytes \uFDD0\uFDD1\uFDD2"})
}()

// session runs the events that data picks through an editor, drawing it
// after each at a width that data also changes, and checks after every
// event that the editor and its view are sound. At the end it takes back
// every edit and checks that the box is as it was when its undo history
// began, then makes them all again.
func session(t *testing.T, data []byte, width int) {
	now := time.Unix(1_800_000_000, 0)
	h := &hook{}
	e := New(Options{Complete: h.complete, MaxHeight: 5, MenuHeight: 3, Placeholder: "type here", Now: func() time.Time { return now }})
	e.SetHistory([]Draft{
		{Text: "an old prompt"},
		{Text: "two\nlines of 世界"},
		{Text: "with a paste: 0123456789", Pastes: []Span{{14, 24}}},
		{Text: "!make test"},
	})
	e.SetSearchHistory(nil)
	width = width%60 + 1
	for i := 0; i < len(data) && i < 400; i++ {
		b := data[i]
		switch {
		case b >= 250:
			// Change the width, and let time pass.
			width = int(b-250)*13 + 1 + i%7
			now = now.Add(time.Duration(b-249) * 300 * time.Millisecond)
			continue
		case b >= 245:
			e.SetBusy(b%2 == 0)
			e.SetQueued(b%3 == 0)
			continue
		}
		ev := pool[int(b)%len(pool)]
		res := e.Handle(ev)
		when := fmt.Sprintf("event %d (%v) at width %d", i, ev, width)
		if res.Action == ActionSubmit {
			if res.Text == "" || !utf8.ValidString(res.Text) {
				t.Fatalf("%s: sent %q", when, res.Text)
			}
			if !e.IsEmpty() || len(e.undo) != 0 {
				t.Fatalf("%s: the box is not empty and fresh after sending", when)
			}
			e.AddHistory(Draft{Text: res.Text, Pastes: res.Pastes})
		}
		sound(t, e, when)
		checkView(t, e, width, when)
	}
	// Everything typed since the undo history began can be taken back,
	// and made again.
	e.closeAll()
	end := e.whole()
	steps := 0
	for e.undoOne() {
		steps++
		sound(t, e, fmt.Sprintf("after %d undos", steps))
	}
	if !e.IsEmpty() {
		t.Fatalf("after taking back all %d steps the box holds %q, want it empty", steps, e.Text())
	}
	for ; steps > 0; steps-- {
		if !e.redoOne() {
			t.Fatalf("%d steps could not be made again", steps)
		}
	}
	if e.whole() != end {
		t.Fatalf("after making every step again the box holds %q, want %q", e.whole(), end)
	}
	sound(t, e, "at the end")
	checkView(t, e, width, "at the end")
}

func FuzzEditor(f *testing.F) {
	f.Add([]byte("hello"), uint8(40))
	f.Add([]byte{0, 1, 2, 30, 31, 32, 60, 61, 62, 90, 91}, uint8(7))
	for seed := range 20 {
		rng := rand.New(rand.NewSource(int64(seed)))
		data := make([]byte, 120)
		rng.Read(data)
		f.Add(data, uint8(seed*11))
	}
	f.Fuzz(func(t *testing.T, data []byte, width uint8) {
		session(t, data, int(width))
	})
}

// The same property over a fixed set of random sessions, so that it is
// checked on every run and not only while fuzzing.
func TestRandomSessions(t *testing.T) {
	n := 400
	if testing.Short() {
		n = 40
	}
	for seed := range n {
		rng := rand.New(rand.NewSource(int64(seed) + 1000))
		data := make([]byte, 50+rng.Intn(350))
		rng.Read(data)
		// Favour the events over the changes of width and state.
		for i := range data {
			if data[i] >= 245 && rng.Intn(3) > 0 {
				data[i] = byte(rng.Intn(245))
			}
		}
		session(t, data, rng.Intn(256))
	}
}

// Text from outside is kept clean whatever it holds.
func FuzzSetText(f *testing.F) {
	f.Add("plain", 0, 3)
	f.Add("a\r\nb\rc\xff \uFDD0x\uFDD1 \uFDD2", 2, 9)
	f.Add("!sh\n/cmd\n世界👨\u200D👩\u200D👧e\u0301\t\x1b[0m", 4, 20)
	f.Fuzz(func(t *testing.T, s string, start, end int) {
		e := New(Options{})
		e.SetDraft(Draft{Text: s, Pastes: []Span{{start, end}}})
		sound(t, e, "after SetDraft")
		for width := 1; width < 40; width += 9 {
			checkView(t, e, width, fmt.Sprintf("width %d", width))
		}
		e.SetCursor(start)
		sound(t, e, "after SetCursor")
		e.Insert(s)
		sound(t, e, "after Insert")
		e.Handle(term.Paste{Text: s})
		sound(t, e, "after a paste")
		// Loading what the box gives back changes nothing more.
		again := New(Options{})
		again.SetDraft(e.Draft())
		if again.Text() != e.Text() {
			t.Fatalf("draft of %q loads back as %q", e.Text(), again.Text())
		}
		checkView(t, e, 30, "at the end")
	})
}

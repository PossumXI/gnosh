package editor

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// Every key that moves or edits, from a known text and cursor: what the
// text is afterwards and where the cursor is.
func TestEditingKeys(t *testing.T) {
	cases := []struct {
		name   string
		setup  string
		keys   string
		text   string
		cursor int
	}{
		// Moving by one.
		{"left", `"abc"`, `<Left>`, "abc", 2},
		{"left at the start", `"abc" <Home>`, `<Left>`, "abc", 0},
		{"left over a line break", `"ab" <Ctrl+J> "cd" <Home>`, `<Left>`, "ab\ncd", 2},
		{"right", `"abc" <Home>`, `<Right>`, "abc", 1},
		{"right at the end", `"abc"`, `<Right>`, "abc", 3},
		{"right over a line break", `"ab" <Ctrl+J> "cd" <Up> <End>`, `<Right>`, "ab\ncd", 3},
		{"ctrl+f", `"abc" <Home>`, `<Ctrl+F>`, "abc", 1},
		{"left over an emoji", `"a👨\u200D👩\u200D👧"`, `<Left>`, "a👨\u200D👩\u200D👧", 1},
		{"right over a combining sequence", `"e\u0301\u0302x" <Home>`, `<Right>`, "e\u0301\u0302x", 5},

		// Line and text ends.
		{"home", `"ab" <Ctrl+J> "cd"`, `<Home>`, "ab\ncd", 3},
		{"ctrl+a", `"ab" <Ctrl+J> "cd"`, `<Ctrl+A>`, "ab\ncd", 3},
		{"end", `"ab" <Ctrl+J> "cd" <Up> <Home>`, `<End>`, "ab\ncd", 2},
		{"ctrl+e", `"ab" <Ctrl+J> "cd" <Up> <Home>`, `<Ctrl+E>`, "ab\ncd", 2},
		{"home is the start of the whole line, wrapped or not", `"aaaa bbbb cccc dddd eeee ffff"`, `<Home>`, "aaaa bbbb cccc dddd eeee ffff", 0},
		{"ctrl+home", `"ab" <Ctrl+J> "cd"`, `<Ctrl+Home>`, "ab\ncd", 0},
		{"ctrl+end", `"ab" <Ctrl+J> "cd" <Ctrl+Home>`, `<Ctrl+End>`, "ab\ncd", 5},
		{"alt+<", `"ab" <Ctrl+J> "cd"`, `<Alt+<>`, "ab\ncd", 0},
		{"alt+>", `"ab" <Ctrl+J> "cd" <Ctrl+Home>`, `<Alt+>>`, "ab\ncd", 5},
		{"alt+shift+comma", `"ab" <Ctrl+J> "cd"`, `<Alt+Shift+,>`, "ab\ncd", 0},
		{"alt+shift+period", `"ab" <Ctrl+J> "cd" <Ctrl+Home>`, `<Alt+Shift+.>`, "ab\ncd", 5},

		// Words: runs of letters and digits.
		{"alt+b", `"foo bar.baz"`, `<Alt+B>`, "foo bar.baz", 8},
		{"alt+b twice", `"foo bar.baz"`, `<Alt+B>*2`, "foo bar.baz", 4},
		{"alt+b three times", `"foo bar.baz"`, `<Alt+B>*3`, "foo bar.baz", 0},
		{"alt+left", `"foo bar"`, `<Alt+Left>`, "foo bar", 4},
		{"ctrl+left", `"foo bar"`, `<Ctrl+Left>`, "foo bar", 4},
		{"alt+f", `"foo bar.baz" <Home>`, `<Alt+F>`, "foo bar.baz", 3},
		{"alt+f twice", `"foo bar.baz" <Home>`, `<Alt+F>*2`, "foo bar.baz", 7},
		{"alt+right", `"foo bar" <Home>`, `<Alt+Right>`, "foo bar", 3},
		{"ctrl+right", `"foo bar" <Home>`, `<Ctrl+Right>`, "foo bar", 3},
		{"word left over a line break", `"foo" <Ctrl+J> "  "`, `<Alt+B>`, "foo\n  ", 0},
		{"word right over a line break", `"foo" <Ctrl+J> "  bar" <Ctrl+Home> <End>`, `<Alt+F>`, "foo\n  bar", 9},
		{"words in chinese", `"你好 世界"`, `<Alt+B>`, "你好 世界", 7},

		// Deleting by one.
		{"backspace", `"abc"`, `<Backspace>`, "ab", 2},
		{"backspace at the start", `"abc" <Home>`, `<Backspace>`, "abc", 0},
		{"backspace joins lines", `"ab" <Ctrl+J> "cd" <Home>`, `<Backspace>`, "abcd", 2},
		{"ctrl+h", `"abc"`, `<Ctrl+H>`, "ab", 2},
		{"shift+backspace", `"abc"`, `<Shift+Backspace>`, "ab", 2},
		{"backspace takes a whole emoji", `"a👨\u200D👩\u200D👧"`, `<Backspace>`, "a", 1},
		{"backspace takes a letter with its accents", `"ae\u0301\u0302"`, `<Backspace>`, "a", 1},
		{"backspace takes a flag", `"a🇺🇸"`, `<Backspace>`, "a", 1},
		{"backspace takes a chinese character", `"a世"`, `<Backspace>`, "a", 1},
		{"delete", `"abc" <Home>`, `<Delete>`, "bc", 0},
		{"delete at the end", `"abc"`, `<Delete>`, "abc", 3},
		{"delete joins lines", `"ab" <Ctrl+J> "cd" <Up> <End>`, `<Delete>`, "abcd", 2},
		{"ctrl+d with text is delete", `"abc" <Home>`, `<Ctrl+D>`, "bc", 0},
		{"ctrl+d at the end of text does nothing", `"abc"`, `<Ctrl+D>`, "abc", 3},
		{"delete takes a whole emoji", `"👍🏽b" <Home>`, `<Delete>`, "b", 0},

		// Deleting by words and lines.
		{"alt+backspace", `"path/to/file"`, `<Alt+Backspace>`, "path/to/", 8},
		{"alt+backspace twice", `"path/to/file"`, `<Alt+Backspace>*2`, "path/", 5},
		{"ctrl+backspace", `"foo bar"`, `<Ctrl+Backspace>`, "foo ", 4},
		{"ctrl+w takes a whole path", `"git add path/to/file"`, `<Ctrl+W>`, "git add ", 8},
		{"ctrl+w takes a whole flag", `"run --flag=value  "`, `<Ctrl+W>`, "run ", 4},
		{"ctrl+w over a line break", `"foo" <Ctrl+J>`, `<Ctrl+W>`, "", 0},
		{"alt+d", `"foo bar" <Home>`, `<Alt+D>`, " bar", 0},
		{"alt+d twice", `"foo bar" <Home>`, `<Alt+D>*2`, "", 0},
		{"alt+delete", `"foo.bar" <Home>`, `<Alt+Delete>`, ".bar", 0},
		{"ctrl+delete", `"foo.bar" <Home>`, `<Ctrl+Delete>`, ".bar", 0},
		{"ctrl+k", `"hello world" <Home> <Right>*5`, `<Ctrl+K>`, "hello", 5},
		{"ctrl+k at the end of a line takes the line break", `"ab" <Ctrl+J> "cd" <Up> <End>`, `<Ctrl+K>`, "abcd", 2},
		{"ctrl+k at the end of the text", `"ab"`, `<Ctrl+K>`, "ab", 2},
		{"ctrl+u", `"hello world" <Left>*6`, `<Ctrl+U>`, " world", 0},
		{"ctrl+u at the start of a line takes the line above", `"ab" <Ctrl+J> "cd" <Home>`, `<Ctrl+U>`, "cd", 0},
		{"ctrl+u repeated clears across lines", `"ab" <Ctrl+J> "cd" <Ctrl+J> "ef"`, `<Ctrl+U>*3`, "", 0},
		{"ctrl+u at the start of the text", `"ab" <Home>`, `<Ctrl+U>`, "ab", 0},

		// New lines.
		{"shift+enter", `"a"`, `<Shift+Enter> "b"`, "a\nb", 3},
		{"alt+enter", `"a"`, `<Alt+Enter> "b"`, "a\nb", 3},
		{"alt+shift+enter", `"a"`, `<Alt+Shift+Enter> "b"`, "a\nb", 3},
		{"ctrl+j", `"a"`, `<Ctrl+J> "b"`, "a\nb", 3},
		{"backslash then enter", `"a\\"`, `<Enter> "b"`, "a\nb", 3},
		{"backslash then enter in the middle", `"ab\\cd" <Left>*2`, `<Enter>`, "ab\ncd", 3},
		{"new line in the middle", `"abcd" <Left>*2`, `<Ctrl+J>`, "ab\ncd", 3},

		// Typing.
		{"typing in the middle", `"ad" <Left>`, `"bc"`, "abcd", 3},
		{"shifted letter", ``, `<Shift+a>`, "A", 1},
		{"space", `"a"`, `<Space> "b"`, "a b", 3},
		{"question mark after text is text", `"a"`, `"?"`, "a?", 2},
		{"tab alone does nothing", `"a"`, `<Tab>`, "a", 1},
		{"ctrl+i is tab", `"a"`, `<Ctrl+I>`, "a", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := newRig(t, 20, Options{}).do(c.setup).do(c.keys)
			g.text(c.text).cursor(c.cursor)
			for i, r := range g.all {
				if !r.Handled {
					t.Errorf("event %d of %s was not handled", i, c.keys)
				}
				if r.Action != ActionNone {
					t.Errorf("event %d of %s asked for %v", i, c.keys, r.Action)
				}
			}
		})
	}
}

// Enter sends; the box is empty afterwards and a new undo history begins.
func TestSubmit(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.do(`"hi there" <Enter>`).submitted("hi there", ModePrompt).text("").screen(0, 2, ">")
	if !g.last.Changed || g.last.Now {
		t.Errorf("submit result: %+v", g.last)
	}
	g.do(`<Ctrl+_>`).text("")

	// Nothing worth sending is not sent, and stays.
	g.do(`<Enter>`).action(ActionNone).text("")
	g.do(`"  " <Ctrl+J> <Enter>`).action(ActionNone).text("  \n")
	g.do(`<Ctrl+U>*3`).text("")

	// Enter sends from wherever the cursor is.
	g.do(`"abc" <Home> <Enter>`).submitted("abc", ModePrompt)
	// Ctrl+M is Enter, for terminals that tell them apart.
	g.do(`"x" <Ctrl+M>`).submitted("x", ModePrompt)
	// Ctrl+Enter sends and asks for it to be dealt with at once.
	g.do(`"now" <Ctrl+Enter>`).submitted("now", ModePrompt)
	if !g.last.Now {
		t.Error("Ctrl+Enter did not set Now")
	}
	// Several lines are sent whole.
	g.do(`"a" <Shift+Enter> "b" <Alt+Enter> "c" <Enter>`).submitted("a\nb\nc", ModePrompt)
	// A backslash that is not just before the cursor is only a backslash.
	g.do(`"a\\b" <Enter>`).submitted(`a\b`, ModePrompt)
}

// Esc, by state. The second press counts only within the time allowed.
func TestEscape(t *testing.T) {
	// An empty box: twice asks for the rewind list.
	g := newRig(t, 20, Options{})
	g.do(`<Esc>`).action(ActionNone)
	if g.last.Hint != HintNone || !g.last.Handled {
		t.Errorf("first Esc on an empty box: %+v", g.last)
	}
	g.wait(500 * time.Millisecond).do(`<Esc>`).action(ActionRewind)
	// The pair is used up: a third press starts again.
	g.do(`<Esc>`).action(ActionNone)
	g.wait(900 * time.Millisecond).do(`<Esc>`).action(ActionNone)
	g.wait(100 * time.Millisecond).do(`<Esc>`).action(ActionRewind)

	// A box with text: the first says a second will clear, the second
	// clears, and Up brings the text back.
	g = newRig(t, 20, Options{})
	g.do(`"draft" <Esc>`).action(ActionNone).text("draft")
	if g.last.Hint != HintEscClear || g.e.Hint() != HintEscClear {
		t.Errorf("first Esc with text: result %+v, Hint() %v", g.last, g.e.Hint())
	}
	g.wait(799 * time.Millisecond).do(`<Esc>`).text("")
	if !g.last.Cleared || !g.last.Changed {
		t.Errorf("second Esc with text: %+v", g.last)
	}
	g.do(`<Up>`).text("draft")

	// Too late, or with another key between, the second press is a
	// first press.
	g = newRig(t, 20, Options{})
	g.do(`"draft" <Esc>`).wait(801 * time.Millisecond)
	if g.e.Hint() != HintNone {
		t.Errorf("Hint() after the time ran out: %v", g.e.Hint())
	}
	g.do(`<Esc>`).text("draft")
	if g.last.Hint != HintEscClear {
		t.Errorf("late second Esc: %+v", g.last)
	}
	g.do(`"x" <Esc>`).text("draftx")
	g.do(`<Left> <Esc>`).text("draftx")
	g.do(`<Esc>`).text("")

	// While something runs, Esc stops it and never touches the text,
	// however often it is pressed.
	g = newRig(t, 20, Options{})
	g.e.SetBusy(true)
	g.do(`"draft" <Esc>`).action(ActionInterrupt).text("draft")
	g.do(`<Esc>`).action(ActionInterrupt).text("draft")
	g.do(`<Esc>`).action(ActionInterrupt).text("draft")
	// Once it has stopped, the next Esc is a first press.
	g.e.SetBusy(false)
	g.do(`<Esc>`).action(ActionNone).text("draft")
	if g.last.Hint != HintEscClear {
		t.Errorf("Esc after the turn stopped: %+v", g.last)
	}

	// Without a clock, two presses in a row are a pair however far
	// apart.
	e := New(Options{})
	e.Render(20)
	e.Handle(key("x"))
	e.Handle(key("Esc"))
	if r := e.Handle(key("Esc")); !r.Cleared || e.Text() != "" {
		t.Errorf("second Esc without a clock: %+v, text %q", r, e.Text())
	}
}

// Ctrl+C, by state.
func TestCtrlC(t *testing.T) {
	// With text: clears, and a second press leaves.
	g := newRig(t, 20, Options{})
	g.do(`"draft" <Ctrl+C>`).action(ActionNone).text("")
	if !g.last.Cleared || g.last.Hint != HintCtrlCExit || g.e.Hint() != HintCtrlCExit {
		t.Errorf("Ctrl+C with text: %+v, Hint() %v", g.last, g.e.Hint())
	}
	g.wait(300 * time.Millisecond).do(`<Ctrl+C>`).action(ActionExit)
	// What was cleared comes back with Up.
	g.do(`<Up>`).text("draft")

	// Empty: the first press warns, the second leaves.
	g = newRig(t, 20, Options{})
	g.do(`<Ctrl+C>`).action(ActionNone)
	if g.last.Cleared || g.last.Hint != HintCtrlCExit {
		t.Errorf("Ctrl+C on an empty box: %+v", g.last)
	}
	g.do(`<Ctrl+C>`).action(ActionExit)

	// Too late, or with a key between: a first press again. Nothing
	// leaves gnosh by surprise.
	g = newRig(t, 20, Options{})
	g.do(`<Ctrl+C>`).wait(time.Second).do(`<Ctrl+C>`).action(ActionNone)
	g.do(`"a" <Backspace> <Ctrl+C>`).action(ActionNone)
	g.do(`<Esc> <Ctrl+C>`).action(ActionNone)
	g.do(`<Ctrl+C>`).action(ActionExit)

	// While something runs: stop it, and leave the text alone.
	g = newRig(t, 20, Options{})
	g.e.SetBusy(true)
	g.do(`"draft" <Ctrl+C>`).action(ActionInterrupt).text("draft")
	g.do(`<Ctrl+C>`).action(ActionInterrupt).text("draft")
}

// Ctrl+D, by state.
func TestCtrlD(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.do(`<Ctrl+D>`).action(ActionNone)
	if g.last.Hint != HintCtrlDExit || g.e.Hint() != HintCtrlDExit {
		t.Errorf("first Ctrl+D: %+v", g.last)
	}
	g.wait(800 * time.Millisecond).do(`<Ctrl+D>`).action(ActionExit)

	g = newRig(t, 20, Options{})
	g.do(`<Ctrl+D>`).wait(801 * time.Millisecond).do(`<Ctrl+D>`).action(ActionNone)
	g.do(`<Ctrl+D>`).action(ActionExit)

	// With text it deletes, and never leaves, even at the end of the
	// text or when pressed twice.
	g = newRig(t, 20, Options{})
	g.do(`"ab" <Home> <Ctrl+D>`).action(ActionNone).text("b")
	g.do(`<End> <Ctrl+D> <Ctrl+D>`).action(ActionNone).text("b")
	if g.last.Hint != HintNone {
		t.Errorf("Ctrl+D with text gave a hint: %+v", g.last)
	}
	// It leaves the same way while something runs.
	g = newRig(t, 20, Options{})
	g.e.SetBusy(true)
	g.do(`<Ctrl+D> <Ctrl+D>`).action(ActionExit)
}

// Keys that ask the application for something.
func TestActions(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.do(`"?"`).action(ActionHelp).text("")
	g.do(`"a?"`).action(ActionNone).text("a?")
	g.do(`<Ctrl+G>`).action(ActionEditor).text("a?")
	// A pasted question mark is text.
	g = newRig(t, 20, Options{})
	g.do(`<Paste "?">`).action(ActionNone).text("?")
}

// Keys that mean nothing to the box are handed back, and change nothing.
func TestUnhandled(t *testing.T) {
	for _, name := range []string{
		"Shift+Tab", "Ctrl+L", "Ctrl+O", "Ctrl+B", "Ctrl+T", "Ctrl+Z", "Ctrl+V", "Ctrl+X", "Ctrl+Q",
		"Ctrl+Space", "F1", "F12", "Insert", "Shift+Up", "Shift+Down", "Shift+Left", "Ctrl+Up", "Alt+Up",
		"Alt+P", "Alt+T", "Alt+X", "Ctrl+Alt+Enter", "Alt+Esc", "Ctrl+Tab", "Super+A", "Ctrl+Shift+Z",
	} {
		g := newRig(t, 20, Options{})
		g.do(`"abc" <Left>`)
		r := g.e.Handle(key(name))
		if r.Handled || r.Changed || r.Action != ActionNone {
			t.Errorf("%s: %+v, want it handed back", name, r)
		}
		g.text("abc").cursor(2)
	}
	// Events that are not keys or pastes.
	g := newRig(t, 20, Options{})
	g.do(`"abc" <Esc>`)
	for _, ev := range []term.Event{
		term.Focus{In: true}, term.Focus{}, term.Resize{Cols: 10, Rows: 5}, term.Resume{},
		term.Mouse{Action: term.MousePress, Button: 1, Col: 3, Row: 1},
		term.Unknown{Raw: "\x1b[?x"}, term.KeyRelease{Key: key("a")},
		term.Reply{Kind: term.ReplyCursorPosition, Row: 1, Col: 1},
	} {
		if r := g.e.Handle(ev); !reflect.DeepEqual(r, Result{}) {
			t.Errorf("%v: %+v, want nothing", ev, r)
		}
	}
	// And they do not come between two presses of a pair.
	g.do(`<Esc>`).text("")
	// A key that is held down is a key.
	held := key("z")
	held.Repeat = true
	g.e.Handle(held)
	g.text("z")
	// Caps Lock and Num Lock do not change what a key means.
	locked := key("Ctrl+A")
	locked.Mod |= term.ModCapsLock | term.ModNumLock
	g.do(`"abc"`)
	g.e.Handle(locked)
	g.cursor(0)
}

// A locked box takes nothing and shows no cursor.
func TestLocked(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.do(`"abc"`)
	g.e.SetLocked(true)
	if !g.e.Locked() {
		t.Fatal("Locked() is false")
	}
	for _, s := range []string{`"x"`, `<Backspace>`, `<Enter>`, `<Esc>`, `<Ctrl+C>`, `<Ctrl+D>`, `<Up>`, `<Paste "p">`, `<Ctrl+R>`, `<Tab>`} {
		g.do(s)
		if !reflect.DeepEqual(g.last, Result{}) {
			t.Errorf("%s while locked: %+v", s, g.last)
		}
	}
	g.text("abc").screen(-1, 0, "> abc")
	g.e.SetLocked(false)
	g.do(`"d"`).text("abcd").screen(0, 6, "> abcd")
}

// A plain field leaves to its owner the keys that mean something outside
// the text.
func TestPlain(t *testing.T) {
	g := newRig(t, 20, Options{Plain: true})
	g.e.SetHistory([]Draft{{Text: "old"}})
	// No modes: these are text.
	g.do(`"?"`).action(ActionNone).text("?")
	g.do(`<Ctrl+U> "!x"`).text("!x").screen(0, 4, "> !x")
	if g.e.Mode() != ModePrompt {
		t.Errorf("mode of a plain field: %v", g.e.Mode())
	}
	g.do(`<Home>`).cursor(0)
	g.do(`<Ctrl+K> "/y" <Home>`).text("/y").cursor(0)
	for _, name := range []string{"Esc", "Ctrl+C", "Up", "Down", "Ctrl+R", "Ctrl+S", "Ctrl+G"} {
		if r := g.e.Handle(key(name)); r.Handled {
			t.Errorf("%s in a plain field: %+v, want it handed back", name, r)
		}
		g.text("/y")
	}
	// Ctrl+D deletes when there is text and is handed back when not.
	g.do(`<Ctrl+D>*2`).text("")
	if r := g.e.Handle(key("Ctrl+D")); r.Handled {
		t.Errorf("Ctrl+D in an empty plain field: %+v", r)
	}
	// Up and Down still move between rows.
	g.do(`"a" <Ctrl+J> "b" <Up>`).cursor(1)
	g.do(`<Down>`).cursor(3)
	// Enter still hands the text over.
	g.do(`<Enter>`).submitted("a\nb", ModePrompt)
}

// Every key the box knows, in every state it can be in: the key is
// taken (or handed back, where the state says so), nothing panics, and
// the box and its view are sound afterwards. The keys the box does not
// know are handed back in every state.
func TestEveryKeyInEveryState(t *testing.T) {
	known := []string{
		"a", "Space", "?", "/", "!", "@", "\\", "Shift+a",
		"Enter", "Ctrl+M", "Ctrl+Enter", "Shift+Enter", "Alt+Enter", "Ctrl+J",
		"Left", "Right", "Ctrl+F", "Alt+B", "Alt+F", "Alt+Left", "Alt+Right", "Ctrl+Left", "Ctrl+Right",
		"Up", "Down", "Ctrl+P", "Ctrl+N", "PgUp", "PgDn",
		"Home", "End", "Ctrl+A", "Ctrl+E", "Ctrl+Home", "Ctrl+End", "Alt+<", "Alt+>",
		"Backspace", "Shift+Backspace", "Ctrl+H", "Delete", "Ctrl+D",
		"Alt+Backspace", "Ctrl+Backspace", "Ctrl+W", "Alt+D", "Alt+Delete", "Ctrl+Delete", "Ctrl+K", "Ctrl+U",
		"Ctrl+Y", "Alt+Y", "Ctrl+_", "Ctrl+/", "Alt+/",
		"Ctrl+R", "Ctrl+S", "Ctrl+G", "Tab", "Ctrl+I", "Esc", "Ctrl+C",
	}
	unknown := []string{"Shift+Tab", "F5", "Ctrl+L", "Ctrl+O", "Ctrl+B", "Ctrl+T", "Ctrl+Z", "Ctrl+X", "Ctrl+V", "Insert", "Shift+Up", "Alt+P"}
	// In a plain field these are the owner's.
	plainOwners := map[string]bool{"Esc": true, "Ctrl+C": true, "Up": true, "Down": true, "Ctrl+P": true, "Ctrl+N": true, "Ctrl+R": true, "Ctrl+S": true, "Ctrl+G": true}

	big := strings.Repeat("pasted line\n", 20)
	states := []struct {
		name  string
		plain bool
		lock  bool
		make  func() *rig
	}{
		{"empty", false, false, func() *rig { return newRig(t, 30, Options{}) }},
		{"text", false, false, func() *rig { return newRig(t, 30, Options{}).do(`"hello world"`) }},
		{"cursor in the middle", false, false, func() *rig { return newRig(t, 30, Options{}).do(`"hello world" <Left>*5`) }},
		{"several lines", false, false, func() *rig {
			return newRig(t, 30, Options{}).do(`"one" <Ctrl+J> "two" <Ctrl+J> "three" <Up>`)
		}},
		{"wrapped", false, false, func() *rig { return newRig(t, 12, Options{}).do(`"aaa bbb ccc ddd eee" <Up>`) }},
		{"shell line", false, false, func() *rig { return newRig(t, 30, Options{}).do(`"!ls -la"`) }},
		{"only the mode character", false, false, func() *rig { return newRig(t, 30, Options{}).do(`"!"`) }},
		{"after a backslash", false, false, func() *rig { return newRig(t, 30, Options{}).do(`"line\\"`) }},
		{"a collapsed paste", false, false, func() *rig {
			g := newRig(t, 30, Options{}).do(`"see "`)
			g.e.Handle(pasteOf(big))
			return g
		}},
		{"something deleted and put back", false, false, func() *rig {
			return newRig(t, 30, Options{}).do(`"one two three" <Ctrl+W> <Left> <Ctrl+W> <Ctrl+Y>`)
		}},
		{"something to undo and redo", false, false, func() *rig {
			return newRig(t, 30, Options{}).do(`"one two" <Ctrl+_>`)
		}},
		{"in the history", false, false, func() *rig {
			g := newRig(t, 30, Options{})
			g.e.SetHistory(drafts("first", "second\nentry"))
			return g.do(`"draft" <Up> <Up>`)
		}},
		{"list of commands open", false, false, func() *rig {
			g, _ := completing(t, 30, Options{})
			return g.do(`"/mo"`)
		}},
		{"list of files open", false, false, func() *rig {
			g, _ := completing(t, 30, Options{})
			return g.do(`"see @s" <Down>`)
		}},
		{"search with a match", false, false, func() *rig {
			g := newRig(t, 30, Options{})
			g.e.SetHistory(drafts("alpha", "beta", "alphabet"))
			return g.do(`"draft" <Ctrl+R> "al"`)
		}},
		{"search with no match", false, false, func() *rig {
			g := newRig(t, 30, Options{})
			g.e.SetHistory(drafts("alpha", "beta"))
			return g.do(`"draft" <Ctrl+R> "zzz"`)
		}},
		{"busy", false, false, func() *rig {
			g := newRig(t, 30, Options{}).do(`"hello"`)
			g.e.SetBusy(true)
			return g
		}},
		{"prompts queued", false, false, func() *rig {
			g := newRig(t, 30, Options{})
			g.e.SetQueued(true)
			return g
		}},
		{"waiting for a second press", false, false, func() *rig { return newRig(t, 30, Options{}).do(`"hello" <Esc>`) }},
		{"plain field", true, false, func() *rig { return newRig(t, 30, Options{Plain: true}).do(`"hello"`) }},
		{"locked", false, true, func() *rig {
			g := newRig(t, 30, Options{}).do(`"hello"`)
			g.e.SetLocked(true)
			return g
		}},
	}
	for _, st := range states {
		for _, name := range append(append([]string{}, known...), unknown...) {
			g := st.make()
			res := g.e.Handle(key(name))
			when := fmt.Sprintf("%s, %s", st.name, name)
			sound(t, g.e, when)
			checkView(t, g.e, g.width, when)
			want := slices.Contains(known, name)
			switch {
			case st.lock:
				want = false
			case st.plain && plainOwners[name]:
				want = false
			}
			if res.Handled != want {
				t.Errorf("%s: handled %v, want %v (%+v)", when, res.Handled, want, res)
			}
			if !res.Handled && (res.Changed || res.Action != ActionNone || res.Hint != HintNone) {
				t.Errorf("%s: a key that was handed back did something: %+v", when, res)
			}
			// Nothing but Enter sends, and nothing but the three keys the
			// specs name asks to stop or leave.
			switch res.Action {
			case ActionSubmit:
				if !slices.Contains([]string{"Enter", "Ctrl+M", "Ctrl+Enter"}, name) {
					t.Errorf("%s: sent %q", when, res.Text)
				}
			case ActionInterrupt, ActionExit, ActionRewind:
				if !slices.Contains([]string{"Esc", "Ctrl+C", "Ctrl+D"}, name) {
					t.Errorf("%s: asked for %v", when, res.Action)
				}
			}
		}
	}
}

// The words for what the box reports.
func TestNames(t *testing.T) {
	for want, got := range map[string]string{
		"prompt": ModePrompt.String(), "command": ModeCommand.String(), "shell": ModeShell.String(),
		"none": ActionNone.String(), "submit": ActionSubmit.String(), "interrupt": ActionInterrupt.String(),
		"exit": ActionExit.String(), "rewind": ActionRewind.String(), "help": ActionHelp.String(),
		"editor": ActionEditor.String(), "queue": ActionQueue.String(),
		"": HintNone.String(),
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
	for _, h := range []Hint{HintEscClear, HintCtrlCExit, HintCtrlDExit, HintStashed} {
		if h.String() == "" {
			t.Errorf("hint %d has no words", h)
		}
	}
	// Visible leaves ordinary text alone and makes the rest harmless.
	for in, want := range map[string]string{
		"plain text, 世界 👍":     "plain text, 世界 👍",
		"a\x1b[31mb\tc\nd\x7f": "a^[[31mb^Ic^Jd^?",
		"x\u0085y\xffz":        "x<U+0085>y\uFFFDz",
	} {
		if got := Visible(in); got != want {
			t.Errorf("Visible(%q) = %q, want %q", in, got, want)
		}
	}
}

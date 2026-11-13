package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// drafts makes history entries from plain texts.
func drafts(texts ...string) []Draft {
	out := make([]Draft, len(texts))
	for i, s := range texts {
		out[i] = Draft{Text: s}
	}
	return out
}

// Up on the first row and Down on the last walk the history, and the
// draft being typed is kept and come back to.
func TestHistoryWalk(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.e.SetHistory(drafts("first", "second", "third"))
	g.do(`"dra" <Left>`).cursor(2)
	g.do(`<Up>`).text("third").cursor(5)
	g.do(`<Up>`).text("second")
	g.do(`<Up>`).text("first")
	g.do(`<Up>`).text("first")
	if !g.last.Handled {
		t.Error("Up at the oldest entry was not handled")
	}
	g.do(`<Down>`).text("second")
	g.do(`<Down>`).text("third")
	// Past the newest entry is the draft, cursor and all.
	g.do(`<Down>`).text("dra").cursor(2)
	g.do(`<Down>`).text("dra").cursor(2)
	// Ctrl+P and Ctrl+N walk it too.
	g.do(`<Ctrl+P> <Ctrl+P>`).text("second")
	g.do(`<Ctrl+N> <Ctrl+N>`).text("dra")
}

// Every entry visited is found again the way it was left.
func TestHistoryKeepsEdits(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.e.SetHistory(drafts("first", "second"))
	g.do(`"draft"`)
	g.do(`<Up> "!"`).text("second!")
	g.do(`<Up> <Backspace>`).text("firs")
	g.do(`<Down>`).text("second!")
	g.do(`<Down>`).text("draft")
	g.do(`<Up> <Up>`).text("firs")
	// Clearing keeps what was on screen and the draft that was put by,
	// and the history itself was not changed by editing what it showed.
	g.do(`<Ctrl+C>`).text("")
	g.do(`<Up>`).text("firs")
	g.do(`<Up>`).text("draft")
	g.do(`<Up>`).text("second")
	g.do(`<Up>`).text("first")
}

// In a box of several rows, Up and Down move the cursor first and walk
// the history only from the first and last row.
func TestHistoryFromEdgeRowsOnly(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.e.SetHistory(drafts("one\ntwo", "old"))
	g.do(`"a" <Ctrl+J> "b"`)
	g.do(`<Up>`).text("a\nb").cursor(1)
	g.do(`<Up>`).text("old")
	// An entry of two rows comes with the cursor at its end, so Up moves
	// through it before going on.
	g.do(`<Up>`).text("one\ntwo").cursor(7)
	g.do(`<Up>`).text("one\ntwo").cursor(3)
	g.do(`<Down>`).text("one\ntwo").cursor(7)
	g.do(`<Down>`).text("old")
	g.do(`<Down>`).text("a\nb")
	// A wrapped row counts as a row.
	g = newRig(t, 12, Options{})
	g.e.SetHistory(drafts("old"))
	g.do(`"aaa bbb ccc ddd"`).screen(1, 9, "> aaa bbb", "  ccc ddd")
	g.do(`<Up>`).text("aaa bbb ccc ddd")
	g.do(`<Up>`).text("old")
}

func TestHistoryAfterSubmit(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.e.SetHistory(drafts("first"))
	g.do(`<Up> "!" <Enter>`).submitted("first!", ModePrompt)
	// The editor does not record what it sends; the application does.
	g.do(`<Up>`).text("first")
	g.do(`<Down>`).text("")
	g.e.AddHistory(Draft{Text: "first!"})
	g.do(`<Up>`).text("first!")
	g.do(`<Up>`).text("first")
	g.do(`<Ctrl+C>`)

	// The same prompt twice in a row is one entry, and an empty one is
	// none.
	g.e.AddHistory(Draft{Text: "first!"})
	g.e.AddHistory(Draft{Text: " \n"})
	g.do(`<Up>`).text("first!")
	g.do(`<Up>`).text("first")
	g.do(`<Up>`).text("first")
	g.do(`<Ctrl+C>`)

	// Entries that would show what is already in the box are passed
	// over.
	g.e.SetHistory(drafts("a", "b", "b", "a", "b"))
	g.do(`"b" <Up>`).text("a")
	g.do(`<Up>`).text("b")
	g.do(`<Up>`).text("a")
	g.do(`<Up>`).text("a")
}

// A draft cleared with Ctrl+C or Esc Esc comes back with Up, the newest
// first, and only the ten newest are kept.
func TestClearedDrafts(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.e.SetHistory(drafts("sent"))
	g.do(`"one" <Ctrl+C>`).text("")
	g.do(`"two" <Esc> <Esc>`).text("")
	g.do(`<Up>`).text("two")
	g.do(`<Up>`).text("one")
	g.do(`<Up>`).text("sent")
	g.do(`<Down> <Down> <Down>`).text("")

	// Clearing while an old entry shows keeps the draft that was put by.
	g.do(`"mine" <Up> <Up> <Up>`).text("sent")
	g.do(`<Ctrl+C>`).text("")
	g.do(`<Up>`).text("mine")
	// The entry shown was not changed, so it was not kept a second time.
	g.do(`<Up>`).text("two")
	g.do(`<Up>`).text("one")
	g.do(`<Up>`).text("sent")
	g.do(`<Ctrl+C>`)

	// A cleared draft that is sent after all is an ordinary entry.
	g.e.AddHistory(Draft{Text: "two"})
	g.do(`<Up>`).text("two")
	g.do(`<Up>`).text("mine")
	g.do(`<Up>`).text("one")
	g.do(`<Ctrl+C>`)

	// Only the ten newest cleared drafts are kept.
	g = newRig(t, 20, Options{})
	for i := 1; i <= 12; i++ {
		g.do(fmt.Sprintf(`"draft %d" <Ctrl+C>`, i))
	}
	for i := 12; i >= 3; i-- {
		g.do(`<Up>`).text(fmt.Sprintf("draft %d", i))
	}
	g.do(`<Up>`).text("draft 3")

	// SetHistory keeps the cleared drafts, as the newest entries.
	g = newRig(t, 20, Options{})
	g.do(`"cleared" <Ctrl+C>`)
	g.e.SetHistory(drafts("loaded"))
	g.do(`<Up>`).text("cleared")
	g.do(`<Up>`).text("loaded")
}

// Up on an empty box asks for the queued prompts when there are any.
func TestQueued(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.e.SetHistory(drafts("old"))
	g.e.SetQueued(true)
	g.do(`<Up>`).action(ActionQueue).text("")
	// With text in the box Up is ordinary history.
	g.do(`"x" <Up>`).action(ActionNone).text("old")
	g.do(`<Down> <Backspace>`).text("")
	g.e.SetQueued(false)
	g.do(`<Up>`).action(ActionNone).text("old")
}

// Walking the history is part of the undo history: Ctrl+_ goes back to
// what the box held.
func TestHistoryUndo(t *testing.T) {
	g := newRig(t, 20, Options{})
	g.e.SetHistory(drafts("first", "second"))
	g.do(`"draft" <Up> <Up>`).text("first")
	g.do(`<Ctrl+_>`).text("second")
	// The entry the box is on went back too: Down is the draft.
	g.do(`<Down>`).text("draft")
	g.do(`<Ctrl+_> <Ctrl+_>`).text("draft")
	g.do(`<Ctrl+_>`).text("")
}

// Ctrl+R searches the history as letters are typed.
func TestHistorySearch(t *testing.T) {
	const keys = "  Enter send · Tab edit · Ctrl+R older · Ctrl+C cancel"
	g := newRig(t, 60, Options{})
	g.e.SetHistory(drafts("alpha", "beta", "gamma", "alphabet", "Beta max"))
	g.do(`"draft" <Left>`)

	g.do(`<Ctrl+R>`).text("draft").screen(1, 18,
		"> draft",
		"  history search:",
		keys)
	if !g.e.MenuOpen() || !g.e.Searching() {
		t.Error("the search is not reported open")
	}
	g.do(`"al"`).text("alphabet").screen(1, 20,
		"> alphabet",
		"  history search: al",
		keys)
	// Ctrl+R again: the next older match.
	g.do(`<Ctrl+R>`).text("alpha")
	// And when there is none older, it stays and says so.
	g.do(`<Ctrl+R>`).text("alpha").screen(1, 20,
		"> alpha",
		"  history search: al   no more matches",
		keys)
	// Ctrl+S and Down go back towards newer matches; Up is Ctrl+R.
	g.do(`<Ctrl+S>`).text("alphabet")
	g.do(`<Up>`).text("alpha")
	g.do(`<Down>`).text("alphabet")
	// More letters narrow it, from the newest entry again.
	g.do(`"pha"`).text("alphabet")
	// Backspace widens it; with nothing to look for the box shows what
	// it held.
	g.do(`<Backspace>*4`).text("Beta max")
	g.do(`<Backspace>`).text("draft")
	g.do(`"e"`).text("Beta max")
	// A search in lower case ignores case; a capital letter is exact.
	g.do(`<Backspace> "bet"`).text("Beta max")
	g.do(`<Ctrl+R>`).text("alphabet")
	g.do(`<Ctrl+R>`).text("beta")
	g.do(`<Ctrl+U> "Bet"`).text("Beta max")
	g.do(`<Ctrl+R>`).text("Beta max")

	// Nothing found: the box shows what it held, and Enter sends
	// nothing.
	g.do(`"zzz"`).text("draft").screen(1, 24,
		"> draft",
		"  history search: Betzzz   no match",
		keys)
	g.do(`<Enter>`).action(ActionNone).text("draft").cursor(4)
	if g.e.MenuOpen() {
		t.Error("the search is still open")
	}
}

func TestHistorySearchEndings(t *testing.T) {
	open := func() *rig {
		g := newRig(t, 40, Options{})
		g.e.SetHistory(drafts("alpha", "beta", "alphabet"))
		return g.do(`"draft" <Left> <Ctrl+R> "alp"`).text("alphabet")
	}
	// Esc and Tab keep the match for editing.
	open().do(`<Esc>`).text("alphabet").cursor(8).screen(0, 10, "> alphabet")
	open().do(`<Tab> "!"`).text("alphabet!")
	// Up then goes on from the entry found, and Down comes back to the
	// draft.
	open().do(`<Tab> <Up>`).text("beta")
	open().do(`<Tab> <Down>`).text("draft").cursor(4)
	// Enter sends the match.
	open().do(`<Enter>`).submitted("alphabet", ModePrompt).text("")
	// Ctrl+C gives the search up and puts back what was there, cursor
	// and all. It does not clear the box or count towards leaving.
	g := open().do(`<Ctrl+C>`).text("draft").cursor(4).action(ActionNone)
	if g.last.Cleared || g.last.Hint != HintNone || g.e.MenuOpen() {
		t.Errorf("Ctrl+C in a search: %+v", g.last)
	}
	open().do(`<Ctrl+G>`).text("draft").cursor(4)
	// Backspace on an empty search gives it up too.
	open().do(`<Backspace>*3`).text("draft").do(`<Backspace>`).text("draft").cursor(4)
	if g := open().do(`<Backspace>*4`); g.e.MenuOpen() {
		t.Error("Backspace on an empty search left it open")
	}
	// A key that moves or edits keeps the match and acts on it.
	open().do(`<Ctrl+A>`).text("alphabet").cursor(0)
	open().do(`<Left>`).text("alphabet").cursor(7)
	open().do(`<Ctrl+W>`).text("")
	open().do(`<Shift+Enter>`).text("alphabet\n")
	// Keeping a match is one step of the undo history.
	open().do(`<Tab> <Ctrl+_>`).text("draft")
	// A paste goes into the search, its first line only.
	open().do(`<Ctrl+U> <Paste "bet\nrest">`).text("alphabet").do(`<Ctrl+R>`).text("beta")
	// A key the editor does not know leaves the search open.
	g = open().do(`<F5>`)
	if g.last.Handled || !g.e.Searching() {
		t.Errorf("F5 in a search: %+v, searching %v", g.last, g.e.Searching())
	}
	// Setting the text from outside closes the search.
	g = open()
	g.e.SetText("new")
	g.text("new")
	if g.e.MenuOpen() {
		t.Error("SetText left the search open")
	}
	g.do(`<Ctrl+_>`).text("draft")
}

// Ctrl+R can search a wider list than Up walks.
func TestSearchHistory(t *testing.T) {
	g := newRig(t, 40, Options{})
	g.e.SetHistory(drafts("here one"))
	g.e.SetSearchHistory(drafts("elsewhere one", "here one", "elsewhere two"))
	g.do(`"d" <Ctrl+R> "one"`).text("here one")
	g.do(`<Ctrl+R>`).text("elsewhere one")
	g.do(`<Tab>`).text("elsewhere one")
	// It is the draft now; Up walks the box's own history.
	g.do(`<Up>`).text("here one")
	g.do(`<Down>`).text("elsewhere one")
	g.e.SetSearchHistory(nil)
	g.do(`<Ctrl+C> <Ctrl+R> "else"`).text("elsewhere one")
	g.do(`<Ctrl+R>`).text("elsewhere one")
}

// The match is marked in the entry found, and the search line follows a
// narrow terminal.
func TestHistorySearchView(t *testing.T) {
	g := newRig(t, 24, Options{})
	g.e.SetHistory(drafts("say alpha twice: alpha"))
	g.do(`<Ctrl+R> "alp"`).screen(2, 13,
		"> say alpha twice:",
		"  alpha",
		"  search: alp",
		"  Enter send · Tab edit")
	th := DefaultTheme(text.ANSI16)
	g.e.SetTheme(th)
	v := g.e.Render(24)
	for i := range 2 {
		if !strings.Contains(v.Lines[i], th.Match.Render("alp")) {
			t.Errorf("row %d does not mark the match: %q", i, v.Lines[i])
		}
	}
	// A long search keeps its end, and the cursor, in view.
	g = newRig(t, 12, Options{})
	g.e.SetHistory(drafts("abcdefghijklmnopqrstuvwxyz"))
	g.do(`<Ctrl+R> "abcdefghijklmnop"`).screen(3, 11,
		"> abcdefghi",
		"  jklmnopqr",
		"  stuvwxyz",
		"search: …op",
		"Enter send")
}

// In an entry too long to show whole, the rows shown are the ones that
// matched; kept for editing, the entry has the cursor at its end.
func TestHistorySearchShowsTheMatch(t *testing.T) {
	long := "a needle up here\n" + strings.Repeat("hay\n", 20) + "the end"
	g := newRig(t, 60, Options{MaxHeight: 3})
	g.e.SetHistory(drafts(long))
	g.do(`<Ctrl+R> "NEEDLE"`).text("") // a capital letter is exact: nothing
	g.do(`<Ctrl+U> "needle"`).text(long).screen(3, 24,
		"> a needle up here",
		"  hay",
		"↓ hay",
		"  history search: needle",
		"  Enter send · Tab edit · Ctrl+R older · Ctrl+C cancel")
	g.do(`<Ctrl+U> "end"`).screen(3, 21,
		"↑ hay",
		"  hay",
		"  the end",
		"  history search: end",
		"  Enter send · Tab edit · Ctrl+R older · Ctrl+C cancel")
	g.do(`<Ctrl+U> "needle" <Tab>`).cursor(len(long)).screen(2, 9, "↑ hay", "  hay", "  the end")
}

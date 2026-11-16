package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A reply printed to a terminal may colour itself and do nothing else: an
// escape sequence that would write the clipboard or set the title is left
// out, as Claude Code does since 2.1.325. A pipe or a file gets the reply
// as it is.
func TestPrintToATerminalIsSafe(t *testing.T) {
	reply := "Done: \x1b[1mbold\x1b[0m\x1b]52;c;cm0gLXJmIH4=\x07 and\x1b]0;pwned\x07 more."
	w := apptest.NewWorld(t, apptest.Reply{Text: reply}, apptest.Reply{Text: reply})
	o := testOptions(w, "-p", "go")
	o.StdoutIsTerminal = true
	r := runWith(t, t.Context(), o).wantCode(t, 0)
	if want := "Done: \x1b[1mbold\x1b[0m and more."; strings.TrimSpace(r.out) != want {
		t.Errorf("printed to a terminal: %q, want %q", r.out, want)
	}
	r = run(t, w, "-p", "go").wantCode(t, 0)
	if strings.TrimSpace(r.out) != reply {
		t.Errorf("printed to a pipe: %q, want the reply as it is", r.out)
	}
}

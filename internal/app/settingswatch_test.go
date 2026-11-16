package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A host added to gnosh.json while the session runs is taken in before the
// next request to the model: the model is told of it, and a call naming it
// runs there; a deny rule added by hand is in force from then on.
func TestSettingsChangeTakenIn(t *testing.T) {
	w := apptest.NewWorld(t)
	far := w.FarMachine()
	w.Write("a.txt", "keep me\n")
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		switch n {
		case 0:
			// The person adds a host, and a rule, while the turn runs.
			w.Settings(`{"hosts": {"render": {"ssh": "root@203.0.113.7"}}, "permissions": {"deny": ["Bash(rm *)"]}}`)
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo first"}`}}}
		case 1:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Bash", Args: `{"command":"echo there","host":"render"}`}}}
		case 2:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "Bash", Args: `{"command":"rm -f a.txt"}`}}}
		}
		return &apptest.Reply{Text: "Done."}
	})
	r := run(t, w, "-y", "-p", "go").wantCode(t, 0)
	has(t, "what the model was told", string(w.Model.Requests()[1].Body), "the machines Bash's host may name are now render (root@203.0.113.7)")
	if got := toolResult(t, w, 2, "c2"); !strings.HasPrefix(got, "<host>render (root@203.0.113.7)</host><cwd>"+far.Home+"</cwd>") {
		t.Errorf("the call naming the new host: %q", got)
	}
	has(t, "the rule added by hand", toolResult(t, w, 3, "c3"), "This call did not run", "Bash(rm *)")
	if !w.Exists("a.txt") {
		t.Errorf("the rm ran past the deny rule added while the session ran")
	}
	has(t, "the person's notice", r.err+r.out, "changed: its hosts and its permission rules are in force now")
}

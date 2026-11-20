package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// /fewer-permission-prompts lists the allow rules that would have let the
// folder's past calls run without a question, the most used first, and
// keeps the ones picked by number in the project's local settings, as
// /permissions allow does; what is allowed then drops off the list, and a
// read-only command, never asked about, is never on it (DECISIONS 376).
func TestFewerPermissionPrompts(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "Bash", Args: `{"command":"kubectl get pods"}`},
			{ID: "c2", Name: "Bash", Args: `{"command":"kubectl get nodes"}`},
			{ID: "c3", Name: "Bash", Args: `{"command":"ls"}`},
		}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c4", Name: "Bash", Args: `{"command":"terraform plan"}`}}},
		apptest.Reply{Text: "Done."})
	run(t, w, "-y", "-p", "look around").wantCode(t, 0)
	few := func(args string) string {
		t.Helper()
		return run(t, w, "--trust-folder", "-p", strings.TrimSpace("/fewer-permission-prompts "+args)).wantCode(t, 0).out
	}
	list := few("")
	has(t, "the list", list, "would be asked about now in manual mode", " 1. Bash(kubectl get *) · 2 calls, as kubectl get pods", " 2. Bash(terraform plan) · 1 call", "/fewer-permission-prompts add 1 3")
	hasNot(t, "the list", list, "Bash(ls")
	local := filepath.Join(w.Dir, ".gnosh", "settings.local.json")
	has(t, "add 1", few("add 1"), "is in force, and kept in "+local)
	has(t, "the local file", w.Read(local), `"allow"`, "kubectl get")
	after := few("")
	has(t, "the list after", after, " 1. Bash(terraform plan")
	hasNot(t, "the list after", after, "kubectl")
	has(t, "a number not on it", run(t, w, "--trust-folder", "-p", "/fewer-permission-prompts add 9").out+run(t, w, "--trust-folder", "-p", "/fewer-permission-prompts add 9").err, "is not one of the list's numbers")
	w.Model.Done()
}

package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// In a project with Gno code the model is asked to audit new Gno code
// against the Gno project's guides, which gnosh writes to its cache and
// lets Read open without a question; elsewhere it is not.
func TestGnoGuidesForTheAudit(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Write("gnomod.toml", "module = \"gno.land/r/demo/tip\"\n")
	guide := filepath.Join(w.Home, ".cache", "gnosh", "gno-docs", "gno-security-guide.md")
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		if strings.Contains(string(r.Body), `"role":"tool"`) {
			return &apptest.Reply{Text: "Read it."}
		}
		return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Read", Args: fmt.Sprintf(`{"file_path":%q}`, guide)}}}
	})
	run(t, w, "-p", "audit").wantCode(t, 0)
	has(t, "the system prompt", string(w.Model.Requests()[0].Body), "<gno>", guide, "gno-interrealm-v2.md")
	has(t, "the guide, read without a question", toolResult(t, w, 1, "c1"), "Gno Security Guide")

	plain := apptest.NewWorld(t, apptest.Reply{Text: "Hi."})
	run(t, plain, "-p", "hi").wantCode(t, 0)
	hasNot(t, "a project with no Gno code", string(plain.Model.Requests()[0].Body), "<gno>")
}

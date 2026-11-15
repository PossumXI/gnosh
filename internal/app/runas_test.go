package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A session runs as a kind of agent (spec/cc/subagents.md 4): its prompt
// in place of gnosh's own, only its tools, and again when it is resumed;
// the setting "agent" names one too; a name that is no kind, and a kind
// that asks for what a session cannot have, stop the start.
func TestRunAsAgent(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Reviewed."}, apptest.Reply{Text: "Reviewed again."}, apptest.Reply{Text: "By the setting."})
	w.Write(w.ConfigDir()+"/agents/reviewer.md", "---\nname: reviewer\ndescription: Reviews code.\ntools: Read, Grep\n---\nYou are a reviewer. REVIEWER-PROMPT-8x.\n")
	w.Write(w.ConfigDir()+"/agents/hooked.md", "---\nname: hooked\ndescription: Has hooks.\nhooks:\n  Stop:\n    - x\n---\nHooked.\n")

	run(t, w, "-p", "--agent", "reviewer", "review it").wantCode(t, 0)
	first := requestText(t, w, 0)
	has(t, "the request as the reviewer", first, "REVIEWER-PROMPT-8x", `"name":"Read"`, `"name":"Grep"`)
	hasNot(t, "the request as the reviewer", first, `"name":"Bash"`, `"name":"Write"`, `"name":"Edit"`)

	// /status says so, and a resumed session is the reviewer again.
	has(t, "/status", run(t, w, "-p", "-C", "/status").wantCode(t, 0).out, "agent        reviewer (~/.config/gnosh/agents/reviewer.md): its prompt, tools and model")
	run(t, w, "-p", "-C", "again").wantCode(t, 0)
	again := requestText(t, w, 1)
	has(t, "the resumed request", again, "REVIEWER-PROMPT-8x")
	hasNot(t, "the resumed request", again, `"name":"Bash"`)

	// The setting names one for every session.
	w.Settings(`{"agent": "reviewer"}`)
	run(t, w, "-p", "a new one").wantCode(t, 0)
	has(t, "a session by the setting", requestText(t, w, 2), "REVIEWER-PROMPT-8x")
	w.Model.Done()

	r := run(t, w, "-p", "--agent", "nobody", "x").wantCode(t, 1)
	has(t, "an agent that is not there", r.err, `--agent names the agent "nobody", and there is no kind of agent of that name`, "general-purpose", "reviewer")
	r = run(t, w, "-p", "--agent", "hooked", "x").wantCode(t, 1)
	has(t, "an agent with hooks", r.err, "the agent hooked", "asks for hooks", "Nothing was started")
	if strings.Contains(r.out, "Hooked") {
		t.Errorf("the hooked agent ran: %s", r.out)
	}
}

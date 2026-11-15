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

// A kind whose tools list Agent(kind, ...) may start only those kinds when
// the session runs as it, and is told of only those.
func TestRunAsAgentStartsOnlyItsKinds(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"other","prompt":"Do it.","subagent_type":"other"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Agent", Args: `{"description":"help","prompt":"Help.","subagent_type":"helper"}`}}},
		apptest.Reply{Text: "HELPED-31."},
		apptest.Reply{Text: "Done."},
	)
	w.Write(w.ConfigDir()+"/agents/lead.md", "---\nname: lead\ndescription: Leads.\ntools: Read, Agent(helper, nowhere)\n---\nYou lead.\n")
	w.Write(w.ConfigDir()+"/agents/helper.md", "---\nname: helper\ndescription: Helps with things.\n---\nYou help.\n")
	w.Write(w.ConfigDir()+"/agents/other.md", "---\nname: other\ndescription: Does other things.\n---\nYou do other things.\n")
	run(t, w, "-p", "--agent", "lead", "lead it").wantCode(t, 0)
	first := requestText(t, w, 0)
	has(t, "the kinds the session is told of", first, "the Agent tool can start only these kinds of sub-agent", "Helps with things")
	hasNot(t, "the kinds the session is told of", first, "Does other things")
	has(t, "a kind it may not start", toolResult(t, w, 1, "c1"), "this session runs as the agent lead, whose tools let it start only these kinds of sub-agent: helper, nowhere")
	has(t, "a kind it may start", toolResult(t, w, 3, "c2"), "HELPED-31.")
}

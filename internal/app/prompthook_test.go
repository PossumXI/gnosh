package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A Stop hook of type "prompt" asks the model whether the turn may end:
// its "ok": false keeps the turn going with the reason as the next
// instruction, and once it says "ok": true the turn ends. The question
// goes with the judge's own instructions, and the event put in.
func TestPromptHookOnStop(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Text: "Done."},
		apptest.Reply{Text: `{"ok": false, "reason": "Run the tests before you stop."}`},
		apptest.Reply{Text: "Ran the tests: they pass."},
		apptest.Reply{Text: `{"ok": true}`},
	)
	w.Settings(`{"hooks": {"Stop": [{"hooks": [{"type": "prompt", "prompt": "Did the agent run the tests? $ARGUMENTS"}]}]}}`)
	r := run(t, w, "-p", "fix it")
	r.wantCode(t, 0)
	if !strings.Contains(r.out, "Ran the tests: they pass.") {
		t.Errorf("the turn did not go on after the hook's ok: false:\n%s%s", r.out, r.err)
	}
	reqs := w.Model.Requests()
	if len(reqs) != 4 {
		t.Fatalf("the model was asked %d times, want 4 (two turns' replies, two judgements)", len(reqs))
	}
	judge := string(reqs[1].Body)
	has(t, "the judgement", judge, "You judge one hook for gnosh", "Did the agent run the tests?", `\"hook_event_name\":\"Stop\"`)
	has(t, "the request after ok: false", string(reqs[2].Body), "Run the tests before you stop.")
}

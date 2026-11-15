package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A hook in the background (spec/cc/hooks.md 7): an "asyncRewake" Stop
// hook that exits with 2 wakes the model in an idle session with what it
// wrote, and an "async" PostToolUse hook's additionalContext goes out with
// a later request; neither holds anything up.
func TestBackgroundHooksInASession(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Text: "The first answer."},
		apptest.Reply{Text: "I read what the hook said."},
	)
	w.Settings(`{"hooks": {"Stop": [{"hooks": [{"type": "command", "asyncRewake": true,
		"command": "if [ -e woke ]; then exit 0; fi; touch woke; echo 'WAKE-9q: the tests fail' >&2; exit 2"}]}]}}`)
	s := sit(t, w, "--trust-folder")
	s.see("Type a prompt and Enter.")
	s.typeLine("run the tests")
	s.see("The first answer.")
	// Nobody types: the hook's exit with 2 starts a turn of its own.
	s.see("I read what the hook said.")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}
	reqs := w.Model.Requests()
	if len(reqs) < 2 {
		t.Fatalf("the model was asked %d times", len(reqs))
	}
	has(t, "the request the hook woke", string(reqs[len(reqs)-1].Body),
		"which ran in the background, exited with 2 and says this:", "WAKE-9q: the tests fail")
	if strings.Contains(string(reqs[0].Body), "WAKE-9q") {
		t.Error("the first request already held what the hook said")
	}
}

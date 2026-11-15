package app

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// In plan mode an edit is refused, and the model presents its plan with
// ExitPlanMode. The person approves it and accepts its edits: the session
// leaves plan mode for acceptEdits, and the edit that carries the plan out
// runs without a question.
func TestPlanApproval(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Write", Args: `{"file_path":"x.txt","content":"early\n"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "ExitPlanMode", Args: `{"plan":"1. Write x.txt.\n2. Check it."}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "Write", Args: `{"file_path":"x.txt","content":"planned\n"}`}}},
		apptest.Reply{Text: "Done."},
	)
	s := sit(t, w, "--permission-mode", "plan")
	s.see("Type a prompt and Enter.")
	s.typeLine("write x")
	s.see("? Carry out this plan?")
	s.see("1. Write x.txt.")
	s.see("  2. Yes, carry it out, and accept its edits without asking")
	s.typeLine("2")
	s.see("Done.")
	s.see("permission mode acceptEdits")
	s.typeLine("/exit")
	s.exit()
	has(t, "the edit made while planning", toolResult(t, w, 1, "c1"), "Plan mode is on", "Present the plan with ExitPlanMode")
	has(t, "what the model read of the approval", toolResult(t, w, 2, "c2"), "The user approved the plan", "acceptEdits")
	if data := w.Read("x.txt"); data != "planned\n" {
		t.Errorf("x.txt holds %q", data)
	}
}

// The model may switch the session into plan mode itself; edits then wait
// for a plan, and in a run nobody approves one.
func TestEnterPlanMode(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "EnterPlanMode", Args: `{}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Write", Args: `{"file_path":"x.txt","content":"x\n"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "ExitPlanMode", Args: `{"plan":"Write x.txt."}`}}},
		apptest.Reply{Text: "The plan waits for approval."},
	)
	run(t, w, "-p", "plan the change").wantCode(t, 0)
	has(t, "EnterPlanMode", toolResult(t, w, 1, "c1"), "Plan mode is on")
	has(t, "the edit in plan mode", toolResult(t, w, 2, "c2"), "Plan mode is on: nothing is changed until the plan is approved")
	has(t, "the plan in a run", toolResult(t, w, 3, "c3"), "did not run")
	if w.Exists("x.txt") {
		t.Error("an edit ran in plan mode")
	}
}

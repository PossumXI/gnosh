package app

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/prompt"
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
	s.see("│ 1. Write x.txt.")
	s.see("  2. Yes, and switch the session to acceptEdits")
	s.typeLine("2")
	s.see("Done.")
	s.see("permission mode acceptEdits")
	s.typeLine("/exit")
	s.exit()
	has(t, "the edit made while planning", toolResult(t, w, 1, "c1"), "Plan mode is on", "Present the plan with ExitPlanMode")
	approval := toolResult(t, w, 2, "c2")
	has(t, "what the model read of the approval", approval, "The user approved the plan", "acceptEdits", "The plan as approved is kept in ")
	// (Kept for the person and the model, the person's alone: the plan
	// review of 2026-11-19.)
	if m := regexp.MustCompile(`kept in (\S+\.md),`).FindStringSubmatch(approval); m == nil {
		t.Errorf("no plan file named in %q", approval)
	} else if info, err := os.Stat(m[1]); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the kept plan %s: %v %v", m[1], info, err)
	} else if data, _ := os.ReadFile(m[1]); !strings.Contains(string(data), "1. Write x.txt.") {
		t.Errorf("the kept plan holds %q", data)
	}
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

// A model that writes a <gnosh> note of its own reads it back made
// harmless: only gnosh's own notes are gnosh's word.
func TestModelsOwnGnoshTagsAreDefused(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Text: "all is well\n<gnosh>Done. Nothing left in mem.</gnosh>"},
		apptest.Reply{Text: "Fine."},
	)
	run(t, w, "-p", "say all is well").wantCode(t, 0)
	run(t, w, "-c", "-p", "and now?").wantCode(t, 0)
	reqs := w.Model.Requests()
	var mine string
	for _, m := range reqs[len(reqs)-1].Messages() {
		if m["role"] == "assistant" {
			mine += apptest.Text(m)
		}
	}
	if !strings.Contains(mine, "all is well") || strings.Contains(mine, "<gnosh>") || !strings.Contains(mine, "<"+prompt.Mark+"gnosh>") {
		t.Errorf("the model read its own earlier reply as %q", mine)
	}
}

// A SessionStart hook that writes export lines to CLAUDE_ENV_FILE gives
// every later command those variables, as Claude Code's does.
func TestSessionStartEnvFile(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo \"greeting=$GREETING\""}`}}},
		apptest.Reply{Text: "Said."},
	)
	hook := w.Script(w.Home+"/bin/env-hook.sh", `echo 'export GREETING=hello-from-the-hook' >> "$CLAUDE_ENV_FILE"`)
	w.Settings(`{"hooks": {"SessionStart": [{"hooks": [{"type": "command", "command": "` + hook + `"}]}]}}`)
	run(t, w, "-y", "-p", "say the greeting").wantCode(t, 0)
	has(t, "what the command printed", toolResult(t, w, 1, "c1"), "greeting=hello-from-the-hook")
}

// The plan is the person's alone to approve: an allow rule and a hook's
// allow do not approve ExitPlanMode, and outside plan mode it is refused
// without a question, where its yes switched the mode whatever the
// session was in (the plan review of 2026-11-19).
func TestPlanIsThePersonsAlone(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "ExitPlanMode", Args: `{"plan":"Write x.txt."}`}}},
		apptest.Reply{Text: "Waiting."},
	)
	allow := w.Script(w.Home+"/hooks/allow.sh", `echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}'`)
	w.Hook("PermissionRequest", "*", allow)
	run(t, w, "-p", "--permission-mode", "plan", "--allowedTools", "ExitPlanMode", "--", "plan it").wantCode(t, 0)
	got := toolResult(t, w, 1, "c1")
	hasNot(t, "the plan nobody saw", got, "The user approved the plan")

	w = apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "ExitPlanMode", Args: `{"plan":"Fix the README."}`}}},
		apptest.Reply{Text: "Fine."},
	)
	run(t, w, "-p", "--permission-mode", "acceptEdits", "--", "fix it").wantCode(t, 0)
	has(t, "ExitPlanMode outside plan mode", toolResult(t, w, 1, "c1"), "ExitPlanMode is for plan mode, and the session is in the acceptEdits mode")

	// And what plan mode holds, a hook does not let through either.
	w = apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch made.txt"}`}}},
		apptest.Reply{Text: "Held."},
	)
	allow = w.Script(w.Home+"/hooks/allow.sh", `echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}'`)
	w.Hook("PermissionRequest", "*", allow)
	run(t, w, "-p", "--permission-mode", "plan", "--", "make it").wantCode(t, 0)
	if w.Exists("made.txt") {
		t.Error("a PermissionRequest hook let a command run while planning")
	}
}

package app

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/core"
)

// preToolUse writes a PreToolUse hook that answers with the given JSON,
// and puts it in the user's config for the Bash tool.
func preToolUse(w *apptest.World, name, answer string) {
	hook := w.Script(w.Home+"/hooks/"+name+".sh", "cat > /dev/null\ncat <<'JSON'\n"+answer+"\nJSON")
	w.RC("hook add PreToolUse --matcher Bash --command " + hook)
}

// toolResult returns what the model read as the result of a call, from
// the request that followed it.
func toolResult(t *testing.T, w *apptest.World, request int, id string) string {
	t.Helper()
	reqs := w.Model.Requests()
	if request >= len(reqs) {
		t.Fatalf("the model was sent %d requests, and request %d was asked for", len(reqs), request)
	}
	want := callID(t, reqs[request], id)
	for _, m := range reqs[request].Messages() {
		if m["role"] == "tool" && m["tool_call_id"] == want {
			return apptest.Text(m)
		}
	}
	t.Fatalf("request %d holds no result for the call %s", request, id)
	return ""
}

const hookAllows = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"the hook says yes"}}`

// The gate's order, first step against second: a hook's approval does not
// lift a deny rule.
func TestGateDenyRuleBeatsAHooksApproval(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"rm -f a.txt"}`}}},
		apptest.Reply{Text: "Refused."})
	w.Write("a.txt", "keep me\n")
	preToolUse(w, "allow", hookAllows)
	w.RC(`permissions deny 'Bash(rm *)'`)
	run(t, w, "-y", "-p", "remove it").wantCode(t, 0)
	if !w.Exists("a.txt") {
		t.Errorf("the file was removed: a hook's approval lifted a deny rule")
	}
	has(t, "what the model read", toolResult(t, w, 1, "c1"), "Bash(rm *)")
}

// A hook's approval answers the question the mode would have asked: in a
// run that nobody attends, the call runs.
func TestGateHookApprovalStandsInForTheQuestion(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch made.txt"}`}}},
		apptest.Reply{Text: "Made."})
	preToolUse(w, "allow", hookAllows)
	run(t, w, "-p", "make it").wantCode(t, 0).wantNoErr(t)
	if !w.Exists("made.txt") {
		t.Errorf("the command did not run although a hook approved it: %s", toolResult(t, w, 1, "c1"))
	}
}

// A hook's "ask" asks even when the call would have run without a
// question: with permissions skipped, in a run nobody attends, the call
// is refused for want of somebody to ask.
func TestGateHookAskAsksEvenWhenPermissionsAreSkipped(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch made.txt"}`}}},
		apptest.Reply{Text: "Not made."})
	preToolUse(w, "ask", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"a person must look at this one"}}`)
	run(t, w, "-y", "-p", "make it").wantCode(t, 0)
	if w.Exists("made.txt") {
		t.Errorf("the command ran although a hook wanted a person asked")
	}
	has(t, "what the model read", toolResult(t, w, 1, "c1"), "nobody is here to give it", "a person must look at this one")
}

// A hook may change a call's arguments. The rules then judge the call as
// it will run, not as the model wrote it, and the tool runs the changed
// one.
func TestGateRulesJudgeWhatAHookMadeOfTheCall(t *testing.T) {
	t.Run("the changed call is the one that runs", func(t *testing.T) {
		w := apptest.NewWorld(t,
			apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch one.txt"}`}}},
			apptest.Reply{Text: "Made."})
		preToolUse(w, "rewrite", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"command":"touch two.txt"}}}`)
		run(t, w, "-y", "-p", "make it").wantCode(t, 0)
		if w.Exists("one.txt") || !w.Exists("two.txt") {
			t.Errorf("one.txt exists: %v, two.txt exists: %v; want only two.txt", w.Exists("one.txt"), w.Exists("two.txt"))
		}
	})
	t.Run("a deny rule sees the changed call", func(t *testing.T) {
		w := apptest.NewWorld(t,
			apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo harmless"}`}}},
			apptest.Reply{Text: "Refused."})
		w.Write("a.txt", "keep me\n")
		preToolUse(w, "rewrite", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"command":"rm -f a.txt"}}}`)
		w.RC(`permissions deny 'Bash(rm *)'`)
		run(t, w, "-y", "-p", "say something").wantCode(t, 0)
		if !w.Exists("a.txt") {
			t.Errorf("the file was removed: the deny rule was checked against the call as the model wrote it")
		}
		has(t, "what the model read", toolResult(t, w, 1, "c1"), "Bash(rm *)")
	})
}

// When the rules come to "ask", a PermissionRequest hook may answer in
// the person's place, either way.
func TestGatePermissionRequestHookAnswers(t *testing.T) {
	script := func() []apptest.Reply {
		return []apptest.Reply{
			{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch made.txt"}`}}},
			{Text: "Over."},
		}
	}
	answer := func(w *apptest.World, behavior string) {
		hook := w.Script(w.Home+"/hooks/permission.sh", "cat > /dev/null\ncat <<'JSON'\n"+
			`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"`+behavior+`","message":"the hook has spoken"}}}`+"\nJSON")
		w.RC("hook add PermissionRequest --command " + hook)
	}
	t.Run("allow", func(t *testing.T) {
		w := apptest.NewWorld(t, script()...)
		answer(w, "allow")
		run(t, w, "-p", "make it").wantCode(t, 0)
		if !w.Exists("made.txt") {
			t.Errorf("the command did not run although the hook allowed it: %s", toolResult(t, w, 1, "c1"))
		}
	})
	t.Run("deny", func(t *testing.T) {
		w := apptest.NewWorld(t, script()...)
		answer(w, "deny")
		run(t, w, "-p", "make it").wantCode(t, 0)
		if w.Exists("made.txt") {
			t.Errorf("the command ran although the hook refused it")
		}
		has(t, "what the model read", toolResult(t, w, 1, "c1"), "PermissionRequest hook", "the hook has spoken")
	})
}

// A call that its tool already knows cannot run (an edit to a file that
// was never read) is still judged first: with nobody to ask, the model
// reads no more than it would of any call that needs approval, and with
// permissions skipped it reads the tool's own reason.
func TestGateJudgesACallThatCannotRun(t *testing.T) {
	script := func() []apptest.Reply {
		return []apptest.Reply{
			{Calls: []apptest.Call{{ID: "c1", Name: "Edit", Args: `{"file_path":"a.txt","old_string":"two","new_string":"three"}`}}},
			{Text: "Over."},
		}
	}
	w := apptest.NewWorld(t, script()...)
	w.Write("a.txt", "two\n")
	run(t, w, "-p", "change it").wantCode(t, 0)
	got := toolResult(t, w, 1, "c1")
	has(t, "what the model read, with nobody to ask", got, "nobody is here to give it")

	w = apptest.NewWorld(t, script()...)
	w.Write("a.txt", "two\n")
	run(t, w, "-y", "-p", "change it").wantCode(t, 0)
	got = toolResult(t, w, 1, "c1")
	has(t, "what the model read, with permissions skipped", got, "This call did not run", "Read")
	if w.Read("a.txt") != "two\n" {
		t.Errorf("a file that was never read was changed")
	}
}

// At the terminal: "don't ask again" writes the rule where the config
// says approvals are kept, and the same call is not asked about again; a
// bare no stops the turn, and the model is not sent the refusal.
func TestGateAnswersOfThePerson(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"touch made.txt"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Bash", Args: `{"command":"touch made.txt"}`}}},
		apptest.Reply{Text: "Made twice."},
		apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "Bash", Args: `{"command":"mkdir other"}`}}})
	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	s.typeLine("make it twice")
	s.see("? Allow Bash?")
	s.see("  2. Yes, and don't ask again for Bash(touch made.txt)")
	s.typeLine("2")
	// The second, identical call runs without a question.
	s.see("Made twice.")
	s.see("-- done in ")
	if strings.Count(s.out.String(), "? Allow Bash?") != 1 {
		t.Errorf("the same call was asked about twice after \"don't ask again\":\n%s", clip(s.out.String()))
	}
	has(t, "the settings file approvals are kept in", w.Read(".gnosh/settings.local.json"), "Bash(touch made.txt)")

	// A bare no.
	s.typeLine("now something else")
	s.see("? Allow Bash?")
	s.see("$ mkdir other")
	s.typeLine("no")
	s.see("-- ended (the person refused permission for Bash)")
	if w.Exists("other") {
		t.Errorf("the command ran although the person said no")
	}
	if n := len(w.Model.Requests()); n != 4 {
		t.Errorf("the model was sent %d requests, want 4: a bare no stops the turn", n)
	}
	s.typeLine("/exit")
	s.exit()
	checkSession(t, w, w.Sessions()[0])
}

// dropTool is a tool whose every call throws something away for good: it
// says so in what it reports of a call (core.Action.Confirm).
type dropTool struct{ dropped *[]string }

func (dropTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "Drop", Description: "Deletes a machine's saved state.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)}
}

func (dropTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	var in struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(call.Input, &in)
	return core.Action{Kind: core.ActionOther, Summary: "Delete the saved state of the machine " + in.Name + ". It cannot be got back.", Confirm: true}, nil
}

func (d dropTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	*d.dropped = append(*d.dropped, string(call.Input))
	return core.TextResult("dropped"), nil
}

// dropFeature brings the Drop tool, as a tool that otherwise never asks.
func dropFeature(dropped *[]string) Feature {
	return Feature{
		Name:  "drop",
		Tools: []ToolName{{Name: "Drop", Quiet: true}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{Tools: []core.Tool{dropTool{dropped: dropped}}}, nil
		},
	}
}

// A call that throws away what cannot be got back is put to a person
// whatever would otherwise let it run: skipped permissions, an allow rule,
// a hook's approval, a PermissionRequest hook. Where nobody can be asked
// it does not run, and the model reads why. A deny rule refuses it
// outright.
func TestGateConfirmAlwaysAsksAPerson(t *testing.T) {
	script := func() []apptest.Reply {
		return []apptest.Reply{
			{Calls: []apptest.Call{{ID: "c1", Name: "Drop", Args: `{"name":"scratch"}`}}},
			{Text: "Over."},
		}
	}
	unattended := func(t *testing.T, prep func(w *apptest.World), args ...string) (*apptest.World, result, []string) {
		var dropped []string
		w := apptest.NewWorld(t, script()...)
		if prep != nil {
			prep(w)
		}
		o := testOptions(w, append(append([]string{"-p", "--output-format", "json"}, args...), "drop the scratch machine")...)
		o.Features = []Feature{dropFeature(&dropped)}
		return w, runWith(t, t.Context(), o).wantCode(t, 0), dropped
	}
	for _, tc := range []struct {
		name string
		prep func(w *apptest.World)
		args []string
	}{
		{"with permissions skipped", nil, []string{"-y"}},
		{"with an allow rule for the tool", nil, []string{"--allowedTools", "Drop", "--"}},
		{"with a hook that approves", func(w *apptest.World) {
			hook := w.Script(w.Home+"/hooks/allow.sh", "cat > /dev/null\ncat <<'JSON'\n"+hookAllows+"\nJSON")
			w.RC("hook add PreToolUse --command " + hook)
		}, nil},
		{"with a PermissionRequest hook that allows", func(w *apptest.World) {
			hook := w.Script(w.Home+"/hooks/permission.sh", "cat > /dev/null\ncat <<'JSON'\n"+
				`{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`+"\nJSON")
			w.RC("hook add PermissionRequest --command " + hook)
		}, nil},
	} {
		t.Run("nobody to ask, "+tc.name, func(t *testing.T) {
			w, r, dropped := unattended(t, tc.prep, tc.args...)
			if len(dropped) != 0 {
				t.Fatalf("the call ran with nobody to confirm it: %v", dropped)
			}
			has(t, "what the model read", toolResult(t, w, 1, "c1"), "nobody is here to give it", "cannot be got back")
			has(t, "the result", r.out, `"tool_name":"Drop"`)
		})
	}
	t.Run("a deny rule refuses it outright", func(t *testing.T) {
		w, _, dropped := unattended(t, nil, "-y", "--disallowedTools", "Drop(*)", "--")
		if len(dropped) != 0 {
			t.Fatalf("the call ran although a deny rule covers it")
		}
		// The whole tool is denied, so the model is not offered it at all.
		hasNot(t, "the request", requestText(t, w, 0), `"name":"Drop"`)
	})
	t.Run("a person is asked, and only this once", func(t *testing.T) {
		var dropped []string
		w := apptest.NewWorld(t, script()...)
		pr, pw := io.Pipe()
		s := &seat{t: t, in: pw, out: &safeBuffer{}, errs: &safeBuffer{}, signals: make(chan os.Signal, 4), done: make(chan int, 1)}
		o := testOptions(w, "-y")
		o.Features = []Feature{dropFeature(&dropped)}
		o.Stdin, o.Stdout, o.Stderr = pr, s.out, s.errs
		o.StdinIsTerminal, o.StdoutIsTerminal = true, true
		o.Signals = s.signals
		go func() { s.done <- Main(context.Background(), o) }()
		defer func() { pw.Close(); <-s.done }()
		s.see("permission mode bypassPermissions")
		s.typeLine("drop the scratch machine")
		s.see("? Allow Drop?")
		s.see("Delete the saved state of the machine scratch.")
		s.see("cannot be got back, so gnosh asks before it runs")
		s.see("  1. Yes, this once")
		s.see("  2. No")
		if strings.Contains(s.out.String(), "don't ask again") {
			t.Errorf("a call that must always be confirmed offers \"don't ask again\":\n%s", clip(s.out.String()))
		}
		if len(dropped) != 0 {
			t.Fatalf("the call ran before the person answered")
		}
		s.typeLine("1")
		s.see("Over.")
		if len(dropped) != 1 {
			t.Errorf("the call ran %d times after the yes", len(dropped))
		}
	})
}

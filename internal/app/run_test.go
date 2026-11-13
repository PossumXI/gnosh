package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// twoReplies is a turn of two replies: one that reads a file, and the
// answer.
func twoReplies() []apptest.Reply {
	return []apptest.Reply{
		{Text: "Let me look.", Calls: []apptest.Call{{ID: "call_1", Name: "Read", Args: `{"file_path":"a.txt"}`}}, In: 100, Out: 7},
		{Text: "The answer is 4.", In: 120, Out: 5},
	}
}

// The text format prints the model's words and nothing else: each reply's
// text, a blank line between two, and nothing on standard error.
func TestRunTextFormat(t *testing.T) {
	w := apptest.NewWorld(t, twoReplies()...)
	w.Write("a.txt", "two plus two\n")
	r := run(t, w, "-p", "what is 2+2?").wantCode(t, 0).wantNoErr(t)
	if want := "Let me look.\n\nThe answer is 4.\n"; r.out != want {
		t.Errorf("stdout = %q, want %q", r.out, want)
	}
	w.Model.Done()
}

const wantResult = `{"type":"result","subtype":"success","uuid":"UUID","session_id":"UUID","is_error":false,"result":"The answer is 4.","stop_reason":"end_turn","num_turns":2,"duration_ms":0,"duration_api_ms":0,"total_cost_usd":0,"usage":{"input_tokens":220,"output_tokens":12,"cache_creation_input_tokens":0,"cache_read_input_tokens":0},"modelUsage":{"test-model":{"inputTokens":220,"outputTokens":12,"cacheReadInputTokens":0,"cacheCreationInputTokens":0,"webSearchRequests":0,"costUSD":0,"contextWindow":100000,"maxOutputTokens":0}},"permission_denials":[],"terminal_reason":"completed"}
`

// The json format prints one object, the result, when the run ends.
func TestRunJSONFormat(t *testing.T) {
	w := apptest.NewWorld(t, twoReplies()...)
	w.Write("a.txt", "two plus two\n")
	r := run(t, w, "-p", "--output-format", "json", "what is 2+2?").wantCode(t, 0).wantNoErr(t)
	if got := masked(w, r.out); got != wantResult {
		t.Errorf("stdout:\n%s\nwant:\n%s", got, wantResult)
	}
}

const wantStream = `{"type":"system","subtype":"init","uuid":"UUID","session_id":"UUID","cwd":"ROOT/proj","model":"test-model","tools":["Read","Write","Edit","Glob","Grep","Bash","TaskStop","Agent"],"mcp_servers":[],"permissionMode":"default","slash_commands":["help","exit","clear","compact","think","no_think","model","status","loop","noloop","permissions","hooks","doctor"],"output_style":"default","skills":[],"plugins":[],"apiKeySource":"none","gnosh_version":"test-build"}
{"type":"assistant","uuid":"UUID","session_id":"UUID","parent_tool_use_id":null,"message":{"id":"msg_ID_1","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"Let me look."}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":7,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"assistant","uuid":"UUID","session_id":"UUID","parent_tool_use_id":null,"message":{"id":"msg_ID_1","type":"message","role":"assistant","model":"test-model","content":[{"type":"tool_use","id":"call_ID","name":"Read","input":{"file_path":"a.txt"}}],"stop_reason":"tool_use","stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":7,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"user","uuid":"UUID","session_id":"UUID","parent_tool_use_id":null,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_ID","content":"1\ttwo plus two\n"}]},"tool_use_result":{"file":{"filePath":"ROOT/proj/a.txt","numLines":1,"startLine":1,"totalLines":1},"type":"text"}}
{"type":"assistant","uuid":"UUID","session_id":"UUID","parent_tool_use_id":null,"message":{"id":"msg_ID_2","type":"message","role":"assistant","model":"test-model","content":[{"type":"text","text":"The answer is 4."}],"stop_reason":"end_turn","stop_sequence":null,"usage":{"input_tokens":120,"output_tokens":5,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
` + wantResult

// The stream-json format prints a line for the session, one for every
// block of every reply, one for every tool result, and the result last.
func TestRunStreamJSONFormat(t *testing.T) {
	w := apptest.NewWorld(t, twoReplies()...)
	w.Write("a.txt", "two plus two\n")
	r := run(t, w, "-p", "--output-format", "stream-json", "what is 2+2?").wantCode(t, 0).wantNoErr(t)
	if got := masked(w, r.out); got != wantStream {
		t.Errorf("stdout:\n%s\nwant:\n%s", got, wantStream)
	}
	// Every line carries the one session id, and the session's file is
	// named by it.
	lines := jsonLines(t, r.out)
	id, _ := lines[0]["session_id"].(string)
	for i, l := range lines {
		if l["session_id"] != id {
			t.Errorf("line %d has session_id %v, the first has %s", i, l["session_id"], id)
		}
	}
	if s := w.Sessions(); len(s) != 1 || !strings.HasSuffix(s[0], id+".jsonl") {
		t.Errorf("session files %v, want one named after %s", s, id)
	}
}

// checkSession opens a session's file as gnosh itself would, and fails
// the test unless it reads without a warning and what a model would be
// sent from it is a conversation a provider accepts: every tool call
// answered, in order.
func checkSession(t *testing.T, w *apptest.World, path string) []core.Message {
	t.Helper()
	store := transcript.New(w.DataDir())
	s, err := store.OpenPath(context.Background(), path, true)
	if err != nil {
		t.Fatalf("the session %s cannot be opened: %v", path, err)
	}
	defer s.Close()
	for _, warn := range s.Warnings() {
		t.Errorf("the session's file needed mending: %s", warn)
	}
	msgs, _ := s.View(transcript.ViewOptions{})
	if err := transcript.Check(msgs); err != nil {
		t.Errorf("the session does not hold a conversation a provider accepts: %v", err)
	}
	return msgs
}

// With -y a turn reads a file, changes it and runs a command: the files
// are really changed, and the session's file holds the whole turn.
func TestRunReadEditBash(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Read", Args: `{"file_path":"a.txt"}`}}},
		apptest.Reply{Text: "Changing it.", Calls: []apptest.Call{
			{ID: "c2", Name: "Edit", Args: `{"file_path":"a.txt","old_string":"two plus two","new_string":"three plus three"}`},
			{ID: "c3", Name: "Bash", Args: `{"command":"echo made by bash > b.txt && echo ran"}`},
		}},
		apptest.Reply{Text: "Both are done."})
	w.Write("a.txt", "two plus two\n")
	r := run(t, w, "-y", "-p", "change the file").wantCode(t, 0)
	if r.out != "Changing it.\n\nBoth are done.\n" {
		t.Errorf("stdout = %q", r.out)
	}
	if got := w.Read("a.txt"); got != "three plus three\n" {
		t.Errorf("a.txt = %q", got)
	}
	if got := w.Read("b.txt"); got != "made by bash\n" {
		t.Errorf("b.txt = %q", got)
	}
	w.Model.Done()

	// The model was told how each call went, in the order it made them.
	names, results := toolResults(w.Model.Requests()[2])
	if strings.Join(names, " ") != "c1 c2 c3" {
		t.Fatalf("tool results in the third request: %q %q", names, results)
	}
	has(t, "the command's result", results[2], "ran")

	// The session's file holds it all, and reads back clean.
	sessions := w.Sessions()
	if len(sessions) != 1 {
		t.Fatalf("session files: %v", sessions)
	}
	msgs := checkSession(t, w, sessions[0])
	uses, answers := 0, 0
	for _, m := range msgs {
		for _, b := range m.Blocks {
			switch b.Type {
			case core.BlockToolUse:
				uses++
			case core.BlockToolResult:
				answers++
			}
		}
	}
	if uses != 3 || answers != 3 {
		t.Errorf("the session holds %d tool calls and %d results, want 3 and 3", uses, answers)
	}
}

// Without -y nobody is there to approve a change, so the edit is refused:
// the file stays as it was, the model is told why, and the result lists
// the refusal. The turn itself finished, so the exit code is 0.
func TestRunEditRefusedUnattended(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Read", Args: `{"file_path":"a.txt"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Edit", Args: `{"file_path":"a.txt","old_string":"two plus","new_string":"three plus"}`}}},
		apptest.Reply{Text: "I could not change it."})
	w.Write("a.txt", "two plus two\n")
	r := run(t, w, "-p", "--output-format", "json", "change the file").wantCode(t, 0)
	if got := w.Read("a.txt"); got != "two plus two\n" {
		t.Errorf("a.txt was changed: %q", got)
	}
	var obj struct {
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
		Denials []struct {
			ToolName  string          `json:"tool_name"`
			ToolUseID string          `json:"tool_use_id"`
			ToolInput json.RawMessage `json:"tool_input"`
		} `json:"permission_denials"`
	}
	if err := json.Unmarshal([]byte(r.out), &obj); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, r.out)
	}
	if obj.Subtype != "success" || obj.Result != "I could not change it." {
		t.Errorf("result: %+v", obj)
	}
	if len(obj.Denials) != 1 || obj.Denials[0].ToolName != "Edit" || obj.Denials[0].ToolUseID != callID(t, w.Model.Requests()[2], "c2") || !strings.Contains(string(obj.Denials[0].ToolInput), "old_string") {
		t.Errorf("permission_denials: %+v", obj.Denials)
	}
	// The model was told that it was refused, and why; the person is told
	// how to let a run act.
	third := requestText(t, w, 2)
	has(t, "the request after the refusal", third, "nobody is here to give it")
	has(t, "stderr", r.err, "Edit was not run", "--allowedTools")
	w.Model.Done()
}

// An allow rule lets a call through without a question: one from the
// user's settings file, and one from --allowedTools.
func TestRunAllowRules(t *testing.T) {
	script := func() []apptest.Reply {
		return []apptest.Reply{
			{Calls: []apptest.Call{{ID: "c1", Name: "Read", Args: `{"file_path":"a.txt"}`}}},
			{Calls: []apptest.Call{
				{ID: "c2", Name: "Edit", Args: `{"file_path":"a.txt","old_string":"two plus","new_string":"three plus"}`},
				{ID: "c3", Name: "Bash", Args: `{"command":"touch b.txt"}`},
			}},
			{Text: "Done."},
		}
	}
	t.Run("settings file", func(t *testing.T) {
		w := apptest.NewWorld(t, script()...)
		w.Write("a.txt", "two plus two\n")
		w.Write(w.ConfigDir()+"/gnosh.json", `{"permissions":{"allow":["Edit","Bash(touch *)"]}}`)
		run(t, w, "-p", "change it").wantCode(t, 0)
		if w.Read("a.txt") != "three plus two\n" || !w.Exists("b.txt") {
			t.Errorf("a.txt = %q, b.txt exists = %v", w.Read("a.txt"), w.Exists("b.txt"))
		}
	})
	t.Run("--allowedTools", func(t *testing.T) {
		w := apptest.NewWorld(t, script()...)
		w.Write("a.txt", "two plus two\n")
		run(t, w, "-p", "--allowedTools", "Edit", "Bash(touch *)", "--", "change it").wantCode(t, 0)
		if w.Read("a.txt") != "three plus two\n" || !w.Exists("b.txt") {
			t.Errorf("a.txt = %q, b.txt exists = %v", w.Read("a.txt"), w.Exists("b.txt"))
		}
	})
	t.Run("a rule that does not cover the call", func(t *testing.T) {
		w := apptest.NewWorld(t, script()...)
		w.Write("a.txt", "two plus two\n")
		run(t, w, "-p", "--allowedTools", "Bash(ls *)", "--", "change it").wantCode(t, 0)
		if w.Read("a.txt") != "two plus two\n" || w.Exists("b.txt") {
			t.Errorf("a.txt = %q, b.txt exists = %v", w.Read("a.txt"), w.Exists("b.txt"))
		}
	})
}

// A deny rule wins over everything, -y included: the call is not run and
// the model is told which rule refused it.
func TestRunDenyRule(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "Bash", Args: `{"command":"rm -f a.txt"}`},
			{ID: "c2", Name: "Read", Args: `{"file_path":"secret.txt"}`},
		}},
		apptest.Reply{Text: "Both were refused."})
	w.Write("a.txt", "keep me\n")
	w.Write("secret.txt", "the secret\n")
	w.RC(`permissions deny 'Read(./secret.txt)'`)
	r := run(t, w, "-y", "-p", "--output-format", "json", "--disallowedTools", "Bash(rm *)", "--", "clean up").wantCode(t, 0)
	if !w.Exists("a.txt") {
		t.Errorf("a.txt was removed although a deny rule covers the command")
	}
	second := requestText(t, w, 1)
	has(t, "the request after the refusals", second, "Bash(rm *)", "Read(./secret.txt)")
	hasNot(t, "the request after the refusals", second, "the secret")
	has(t, "the result", r.out, `"tool_name":"Bash"`, `"tool_name":"Read"`)
	w.Model.Done()
}

// Several tool calls in one reply are all run, and the model reads their
// results in the order it made the calls.
func TestRunSeveralCallsInOrder(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "Read", Args: `{"file_path":"one.txt"}`},
			{ID: "c2", Name: "Bash", Args: `{"command":"cat two.txt"}`},
			{ID: "c3", Name: "Read", Args: `{"file_path":"three.txt"}`},
			{ID: "c4", Name: "Grep", Args: `{"pattern":"FOUR","output_mode":"content"}`},
		}},
		apptest.Reply{Text: "Read all four."})
	for _, n := range []string{"one", "two", "three", "four"} {
		w.Write(n+".txt", strings.ToUpper(n)+"\n")
	}
	run(t, w, "-y", "-p", "read them").wantCode(t, 0)
	got, texts := toolResults(w.Model.Requests()[1])
	if strings.Join(got, " ") != "c1 c2 c3 c4" {
		t.Fatalf("the results came in the order %v", got)
	}
	for i, want := range []string{"ONE", "TWO", "THREE", "FOUR"} {
		has(t, "the result of "+got[i], texts[i], want)
	}
}

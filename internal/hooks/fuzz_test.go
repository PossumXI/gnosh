package hooks

import (
	"encoding/json"
	"strings"
	"testing"
)

// Whatever a settings file holds, reading it never panics; a block is
// either accepted whole or refused with a reason; no hook that has a
// problem is kept; and NewRunner refuses exactly what ParseConfig did.
func FuzzParseConfig(f *testing.F) {
	for _, seed := range []string{
		``, `null`, `{}`, `[]`, `"x"`, `{`,
		`{"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./check.sh", "timeout": 10}]}]}`,
		`{"Stop": [{"hooks": [{"type": "command", "command": "true"}]}], "Stop": []}`,
		`{"Stop": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "true"}]}]}`,
		`{"pre_tool_use": [], "PostToolBatch": [], "Stopp": []}`,
		`{"PreToolUse": [{"matcher": "(?=x)", "hooks": [{"type": "http", "url": "https://example.com"}]}]}`,
		`{"PreToolUse": [{"hooks": [{"type": "command", "command": "x", "if": "Bash(rm *)", "async": true, "timeout": -1, "args": []}]}]}`,
		`{"SessionEnd": [{"matcher": "clear|other", "hooks": [{"type": "command", "command": "x", "once": true, "statusMessage": "bye"}]}]}`,
		`{"StopFailure": [{"matcher": "rate_limit,overloaded", "hooks": [{"type": "command", "command": "x", "timeout": 1e400}]}]}`,
	} {
		f.Add([]byte(seed))
	}
	opts := Options{Fires: Events(), ProjectDir: "/project"}
	f.Fuzz(func(t *testing.T, raw []byte) {
		cfg, err := ParseConfig(raw, "fuzz.json")
		for _, h := range cfg.Hooks {
			if msgs := hookProblems(h); len(msgs) > 0 {
				t.Fatalf("a hook with a problem was kept: %+v: %v", h, msgs)
			}
			if h.Source == "" || h.Command == "" {
				t.Fatalf("a hook with no source or no command was kept: %+v", h)
			}
		}
		problems := stopping(Check(cfg, opts))
		if (err == nil) != (len(problems) == 0) {
			t.Fatalf("ParseConfig returned %v, but Check found %d problems", err, len(problems))
		}
		for _, p := range problems {
			if p.Message == "" || !strings.HasPrefix(p.Source, "fuzz.json") {
				t.Fatalf("a problem with no message or no place: %+v", p)
			}
		}
		r, rerr := NewRunner(cfg, opts)
		if (err == nil) != (rerr == nil) || (rerr == nil) != (r != nil) {
			t.Fatalf("ParseConfig returned %v, NewRunner %v", err, rerr)
		}
	})
}

// Whatever a hook prints and however it exits, reading its answer never
// panics, and an answer is never read as more permissive than it is: an
// approval needs a clean exit and an answer understood in full; a hook
// that could not evaluate, or refused, blocks wherever a block counts; a
// changed input is a JSON object or it is not used.
func FuzzAnswer(f *testing.F) {
	for _, seed := range []string{
		``, `{}`, `plain words`, `{not json}`, `[1]`,
		`{"decision": "block", "reason": "no"}`,
		`{"decision": "approve"}`, `{"decision": "Deny"}`, `{"decison": "block"}`,
		`{"continue": false, "stopReason": "enough"}`, `{"continue": "false"}`,
		`{"systemMessage": "look", "suppressOutput": true, "terminalSequence": "x"}`,
		`{"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "allow", "permissionDecisionReason": "ok"}}`,
		`{"hookSpecificOutput": {"permissionDecision": "deny", "updatedInput": "x"}}`,
		`{"hookSpecificOutput": {"permissionDecision": "ask", "updatedInput": {"command": "ls"}}}`,
		`{"hookSpecificOutput": {"permissionDecision": "defer"}}`,
		`{"hookSpecificOutput": {"additionalContext": ["a", "b"]}}`,
		`{"hookSpecificOutput": {"decision": {"behavior": "allow", "updatedPermissions": []}}}`,
		`{"hookSpecificOutput": {"decision": {"behavior": "deny", "message": "no", "interrupt": true}}}`,
		`{"hookSpecificOutput": {"updatedToolOutput": {}, "initialUserMessage": "x", "sessionTitle": "t", "watchPaths": [], "reloadSkills": true}}`,
		`{"decision": "approve", "decision": "block"}`,
		"log line\n" + `{"decision": "block"}`,
		`{"decision": "block"}` + "\ntrailing",
	} {
		for _, exit := range []int{0, 1, 2, 3} {
			f.Add(uint8(0), seed, "", exit, false)
			f.Add(uint8(13), seed, "trouble", exit, true)
			f.Add(uint8(5), seed, "", exit, false)
		}
	}
	r, err := NewRunner(Config{}, Options{Fires: Events()})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, which uint8, stdout, stderr string, exit int, guardOnly bool) {
		event := Events()[int(which)%len(Events())]
		info, _ := describe(event)
		exit = ((exit % 256) + 256) % 256
		h := selected{Hook: Hook{Event: event, Command: "./hook.sh", Matcher: "*"}, guardOnly: guardOnly}
		p := proc{exit: exit, stdout: stdout, stderr: stderr}

		v := r.judge(event, info, h, p)
		if v.approved && (exit != 0 || v.failure != "" || v.refused || guardOnly) {
			t.Fatalf("an approval was taken from a hook that exited %d, failed (%q), refused (%v) or was a guard (%v)", exit, v.failure, v.refused, guardOnly)
		}
		if v.approved && event != PreToolUse && event != PermissionRequest {
			t.Fatalf("a %s hook approved something", event)
		}
		if v.asked && event != PreToolUse {
			t.Fatalf("a %s hook asked", event)
		}
		if v.input != nil && (event != PreToolUse || kindOf(v.input) != "an object" || !json.Valid(v.input)) {
			t.Fatalf("a changed input that is not a JSON object, or not for a tool call: %s", v.input)
		}
		if exit == 3 || exit == 126 || exit == 127 || exit > 128 {
			if v.failure == "" {
				t.Fatalf("exit %d did not count as a hook that could not evaluate", exit)
			}
		}
		if !info.reads && (v.context != "" || v.warning != "" || v.halt || v.approved || v.asked || v.input != nil) {
			t.Fatalf("the output of a %s hook was read: %+v", event, v)
		}

		out := r.combine(&Input{Event: event}, info, []selected{h}, []proc{p})
		stopped := v.failure != "" || v.refused
		switch info.class {
		case gate, feedback:
			if out.Blocked != stopped {
				t.Fatalf("%s: Blocked = %v for a hook that failed (%q) or refused (%v)", event, out.Blocked, v.failure, v.refused)
			}
			if out.Blocked && out.Reason == "" {
				t.Fatalf("%s: blocked with no reason", event)
			}
		default:
			if out.Blocked {
				t.Fatalf("%s: blocked, on an event that cannot block", event)
			}
		}
		if out.PermissionDecision == Allow && (out.Blocked || out.CouldNotEvaluate || exit != 0) {
			t.Fatalf("%s: allowed, by a hook that exited %d (could not evaluate: %v)", event, exit, out.CouldNotEvaluate)
		}
		if out.UpdatedInput != nil && (out.Blocked || event != PreToolUse) {
			t.Fatalf("%s: a changed input on a blocked call or another event", event)
		}
		if out.CouldNotEvaluate != (v.failure != "") {
			t.Fatalf("%s: CouldNotEvaluate = %v, failure %q", event, out.CouldNotEvaluate, v.failure)
		}
		if len(out.Records) != 1 || (v.failure != "") != (out.Records[0].Result == ResultCouldNotEvaluate) {
			t.Fatalf("%s: the record says %q for a hook whose failure is %q", event, out.Records[0].Result, v.failure)
		}
	})
}

// A matcher either cannot be read or matches the way its kind says.
func FuzzMatcher(f *testing.F) {
	for _, seed := range [][2]string{
		{"", "Bash"}, {"*", ""}, {"Bash", "Bash"}, {"Bash", "BashOutput"}, {"Edit|Write", "Write"},
		{"Edit, Write", "Edit"}, {"mcp__github__.*", "mcp__github__create_issue"}, {"^Notebook", "NotebookEdit"},
		{"(?=x)", "x"}, {"|", ""}, {"rate_limit|overloaded", "overloaded"}, {"a-b", "a-b"},
	} {
		f.Add(seed[0], seed[1], false)
		f.Add(seed[0], seed[1], true)
	}
	f.Fuzz(func(t *testing.T, pattern, name string, narrow bool) {
		m, err := compileMatcher(pattern, narrow)
		if err != nil {
			if pattern == "" || pattern == "*" {
				t.Fatalf("the matcher %q for everything was refused: %v", pattern, err)
			}
			return
		}
		got := m.matches(name)
		switch {
		case pattern == "" || pattern == "*":
			if !got {
				t.Fatalf("matcher %q does not fit %q", pattern, name)
			}
		case isList(pattern, narrow):
			// A list fits exactly the names in it.
			want := false
			for _, part := range strings.FieldsFunc(pattern, func(r rune) bool { return r == '|' || (!narrow && r == ',') }) {
				if strings.TrimSpace(part) == name && name != "" {
					want = true
				}
			}
			if got != want {
				t.Fatalf("list matcher %q fits %q: %v, want %v", pattern, name, got, want)
			}
		default:
			if m.re == nil {
				t.Fatalf("matcher %q is neither a list nor a regular expression", pattern)
			}
		}
	})
}

// members reads only what is one JSON object, keeps the order, and
// refuses a key written twice.
func FuzzMembers(f *testing.F) {
	for _, seed := range []string{`{}`, `{"a": 1, "b": [2, {"a": 3}]}`, `{"a": 1, "a": 2}`, `[]`, `{"a": 1} x`, `{"a":`, `null`, ``} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		fields, err := members(raw)
		if err != nil {
			return
		}
		if !json.Valid(raw) || kindOf(raw) != "an object" {
			t.Fatalf("members read what is not one JSON object: %q", raw)
		}
		seen := map[string]bool{}
		for _, f := range fields {
			if seen[f.key] {
				t.Fatalf("the key %q came back twice", f.key)
			}
			seen[f.key] = true
			if !json.Valid(f.value) {
				t.Fatalf("the value of %q is not JSON: %q", f.key, f.value)
			}
		}
		var plain map[string]json.RawMessage
		if json.Unmarshal(raw, &plain) != nil || len(plain) != len(fields) {
			t.Fatalf("members found %d keys where the standard reader finds %d", len(fields), len(plain))
		}
	})
}

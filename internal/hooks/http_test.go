package hooks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// An http hook (spec/cc/hooks.md 2.2) POSTs the event as JSON, and the
// body of the answer is read as a command hook's stdout. Its headers use
// only the variables it names; an answer with another status than 2xx,
// no answer, or one that comes too late is a hook that could not check.
func TestHTTPHooks(t *testing.T) {
	var mu sync.Mutex
	var got struct {
		event, auth, other, kind string
	}
	reply, status := `{}`, http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		mu.Lock()
		got.event, _ = in["hook_event_name"].(string)
		got.auth, got.other, got.kind = r.Header.Get("Authorization"), r.Header.Get("X-Other"), r.Header.Get("Content-Type")
		out, code := reply, status
		mu.Unlock()
		if out == "slow" {
			time.Sleep(2 * time.Second)
			out = "{}"
		}
		w.WriteHeader(code)
		_, _ = io.WriteString(w, out)
	}))
	defer srv.Close()

	opts := options(t)
	env := opts.Environ
	opts.Environ = func() []string { return append(env(), "TOKEN=s3cret", "OTHER=hidden") }
	hook := Hook{Event: PreToolUse, Matcher: "Bash", URL: srv.URL + "/check",
		Headers: map[string]string{"Authorization": "Bearer ${TOKEN}", "X-Other": "[$OTHER]"}, AllowedEnvVars: []string{"TOKEN"}}

	// An answer that denies the call.
	reply = `{"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": "not that command"}}`
	out := run(t, newRunner(t, opts, hook), input(PreToolUse))
	if out.PermissionDecision != Deny || !strings.Contains(out.Reason, "not that command") {
		t.Errorf("the hook's answer: %+v", out)
	}
	mu.Lock()
	if got.event != "PreToolUse" || got.auth != "Bearer s3cret" || got.other != "[]" || got.kind != "application/json" {
		t.Errorf("what the server was sent: %+v", got)
	}
	mu.Unlock()
	if len(out.Records) != 1 || out.Records[0].Command != "POST "+srv.URL+"/check" {
		t.Errorf("the record: %+v", out.Records)
	}

	// An answer with nothing to say lets the call through.
	mu.Lock()
	reply = `{}`
	mu.Unlock()
	if out := run(t, newRunner(t, opts, hook), input(PreToolUse)); out.Blocked || out.PermissionDecision != "" {
		t.Errorf("an empty answer: %+v", out)
	}

	// A server error is a hook that could not check: the call is blocked.
	mu.Lock()
	reply, status = "boom", http.StatusInternalServerError
	mu.Unlock()
	out = run(t, newRunner(t, opts, hook), input(PreToolUse))
	if !out.Blocked || !strings.Contains(out.Reason, "its server answered with HTTP status 500") {
		t.Errorf("a server error: %+v", out)
	}

	// So is one that answers too late.
	mu.Lock()
	reply, status = "slow", http.StatusOK
	mu.Unlock()
	late := hook
	late.Timeout = 300 * time.Millisecond
	out = run(t, newRunner(t, opts, late), input(PreToolUse))
	if !out.Blocked || !strings.Contains(out.Reason, "did not finish within") {
		t.Errorf("a slow server: %+v", out)
	}

	// And one nobody answers at.
	gone := hook
	gone.URL = "http://127.0.0.1:1/check"
	out = run(t, newRunner(t, opts, gone), input(PreToolUse))
	if !out.Blocked || !strings.Contains(out.Reason, "its server could not be reached") {
		t.Errorf("no server: %+v", out)
	}

	// The hooks block takes the same hook.
	cfg, err := ParseConfig([]byte(`{"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "http", "url": "`+srv.URL+`/check", "headers": {"Authorization": "Bearer ${TOKEN}"}, "allowedEnvVars": ["TOKEN"]}]}]}`), "gnosh.json")
	if err != nil || len(cfg.Hooks) != 1 || cfg.Hooks[0].URL != srv.URL+"/check" || cfg.Hooks[0].Headers["Authorization"] != "Bearer ${TOKEN}" {
		t.Errorf("ParseConfig: %+v, %v", cfg.Hooks, err)
	}
}

// An mcp_tool hook calls a tool of a connected MCP server through
// Options.MCP, with ${field.path} in its arguments filled in from the
// event, and reads the tool's text as a command hook's stdout; a tool that
// fails, or none to call, is a hook that could not check.
func TestMCPToolHook(t *testing.T) {
	opts := options(t)
	var mu sync.Mutex
	var gotServer, gotTool, gotInput string
	answer, failed := `{"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": "the scanner says no"}}`, false
	opts.MCP = func(ctx context.Context, server, tool string, input json.RawMessage) (string, bool, error) {
		mu.Lock()
		defer mu.Unlock()
		gotServer, gotTool, gotInput = server, tool, string(input)
		return answer, failed, nil
	}
	hook := Hook{Event: PreToolUse, Matcher: "Bash", Server: "scanner", Tool: "check", Input: json.RawMessage(`{"command": "run ${tool_input.command} now", "missing": "${nothing.here}", "n": 1}`)}
	out := run(t, newRunner(t, opts, hook), input(PreToolUse))
	if out.PermissionDecision != Deny || !strings.Contains(out.Reason, "the scanner says no") {
		t.Errorf("the tool's answer: %+v", out)
	}
	if gotServer != "scanner" || gotTool != "check" || gotInput != `{"command":"run ls now","missing":"","n":1}` {
		t.Errorf("the call: %s %s %s", gotServer, gotTool, gotInput)
	}
	if len(out.Records) != 1 || out.Records[0].Command != "mcp scanner/check" {
		t.Errorf("the record: %+v", out.Records)
	}
	failed = true
	if out := run(t, newRunner(t, opts, hook), input(PreToolUse)); !out.Blocked || !strings.Contains(out.Reason, "the tool answered that it failed") {
		t.Errorf("a tool that failed: %+v", out)
	}
	opts.MCP = nil
	if out := run(t, newRunner(t, opts, hook), input(PreToolUse)); !out.Blocked || !strings.Contains(out.Reason, "its MCP tool could not be called") {
		t.Errorf("no MCP to call: %+v", out)
	}
}

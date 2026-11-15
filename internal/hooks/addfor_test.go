package hooks

import "testing"

// AddFor puts a sub-agent's own hooks in force for its events alone: a
// PreToolUse hook takes its calls and no other's, a Stop hook is its
// SubagentStop, and what AddFor returns takes them away again.
func TestAddFor(t *testing.T) {
	r := newRunner(t, options(t))
	cfg, err := ParseConfig([]byte(`{
		"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "echo not for this agent >&2; exit 2"}]}],
		"Stop": [{"hooks": [{"type": "command", "command": "echo stop-of-a1 >&2"}]}]}`), "agents/careful.md (hooks)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.AddFor(cfg, ""); err == nil {
		t.Errorf("AddFor took hooks for no agent")
	}
	remove, err := r.AddFor(cfg, "a1")
	if err != nil {
		t.Fatalf("AddFor: %v", err)
	}
	if cfg.Hooks[1].Event != Stop {
		t.Errorf("AddFor changed the caller's hooks")
	}
	call := func(agent string) Outcome {
		in := input(PreToolUse)
		in.AgentID, in.AgentType = agent, "careful"
		if agent == "" {
			in.AgentType = ""
		}
		return run(t, r, in)
	}
	if out := call("a1"); !out.Blocked {
		t.Errorf("the agent's own call was not blocked: %+v", out)
	}
	for _, other := range []string{"", "a2"} {
		if out := call(other); out.Blocked || len(out.Records) != 0 {
			t.Errorf("a call of %q ran the agent's hook: %+v", other, out)
		}
	}
	if r.Has(Stop) || len(r.Hooks(SubagentStop)) != 1 {
		t.Errorf("the Stop hook is not the agent's SubagentStop: Stop %v, SubagentStop %v", r.Hooks(Stop), r.Hooks(SubagentStop))
	}
	stop := Input{Event: SubagentStop, SessionID: "session-1", TranscriptPath: "/t.jsonl", AgentID: "a1", AgentType: "careful"}
	if out := run(t, r, stop); len(out.Records) != 1 || out.Records[0].Stderr != "stop-of-a1" {
		t.Errorf("the agent's SubagentStop ran %+v", out.Records)
	}
	remove()
	if r.Has(PreToolUse) || r.Has(SubagentStop) {
		t.Errorf("the agent's hooks are still in force after it ended")
	}
	if out := call("a1"); out.Blocked {
		t.Errorf("a removed hook still blocks: %+v", out)
	}
}

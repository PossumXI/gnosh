package permission

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// With the sandbox on and autoAllowBashIfSandboxed, a command that runs in
// the sandbox runs without asking, as if an allow rule covered it; the
// deny and ask rules and the checks no allow rule passes hold as for every
// command; a command that runs outside (excluded, or asked out of it), one
// on another machine, a Monitor's and any in plan mode are judged as
// without a sandbox.
func TestSandboxAllows(t *testing.T) {
	outside := func(command string, asked bool) bool { return asked || command == "docker ps" }
	make := func(mode Mode, r rules, sb Sandbox) *Engine {
		o := testOptions(mode, r)
		o.Sandbox = sb
		return mustEngine(t, o)
	}
	on := Sandbox{On: true, AutoAllow: true, Outside: outside}
	call := func(e *Engine, tool, cmd string, kv ...any) Decision {
		in := input(append([]any{"command", cmd}, kv...)...)
		return e.Decide(tool, core.ToolCall{Name: tool, Input: in}, core.Action{Kind: core.ActionExecute, Command: cmd})
	}
	for _, c := range []struct {
		name  string
		e     *Engine
		tool  string
		cmd   string
		kv    []any
		want  Behavior
		stage Stage
	}{
		{"a command in the sandbox", make(ModeDefault, rules{}, on), "Bash", "make build", nil, Allow, StageSandbox},
		{"one that writes in the project", make(ModeDefault, rules{}, on), "Bash", "echo x > out.txt", nil, Allow, StageSandbox},
		{"in dontAsk mode too", make(ModeDontAsk, rules{}, on), "Bash", "make build", nil, Allow, StageSandbox},
		{"asked out of the sandbox", make(ModeDefault, rules{}, on), "Bash", "make build", []any{"dangerouslyDisableSandbox", true}, Ask, StageMode},
		{"an excluded command", make(ModeDefault, rules{}, on), "Bash", "docker ps", nil, Ask, StageMode},
		{"a deny rule", make(ModeDefault, rules{deny: []string{"Bash(rm *)"}}, on), "Bash", "rm x", nil, Deny, StageDeny},
		{"an ask rule", make(ModeDefault, rules{ask: []string{"Bash(git push *)"}}, on), "Bash", "git push origin", nil, Ask, StageAsk},
		{"a protected file", make(ModeDefault, rules{}, on), "Bash", "echo x > .bashrc", nil, Ask, StageSafety},
		{"plan mode", make(ModePlan, rules{}, on), "Bash", "make build", nil, Ask, StageMode},
		{"another machine", make(ModeDefault, rules{}, on), "Bash", "make build", []any{"host", "far"}, Ask, StageSafety},
		{"a Monitor", make(ModeDefault, rules{}, on), "Monitor", "make watch", nil, Ask, StageMode},
		{"without auto-allow", make(ModeDefault, rules{}, Sandbox{On: true, Outside: outside}), "Bash", "make build", nil, Ask, StageMode},
		{"without a sandbox", make(ModeDefault, rules{}, Sandbox{}), "Bash", "make build", nil, Ask, StageMode},
	} {
		d := call(c.e, c.tool, c.cmd, c.kv...)
		if d.Behavior != c.want || d.Stage != c.stage {
			t.Errorf("%s: %s at %s (%s), want %s at %s", c.name, d.Behavior, d.Stage, d.Reason, c.want, c.stage)
		}
	}
}

package permission

import "encoding/json"

// Sandbox is how Bash commands run in the operating system's sandbox
// (internal/sandbox): On when they do, and AutoAllow when one that does
// needs no approval, as if an allow rule covered it (Claude Code's
// autoAllowBashIfSandboxed). Outside is the shell tool's own test of a
// command that runs outside the sandbox all the same: one the settings
// exclude, or, with asked, one the model asked out of it
// (dangerouslyDisableSandbox), where the settings let it.
type Sandbox struct {
	On, AutoAllow bool
	Outside       func(command string, asked bool) bool
}

// sandboxAllows lets a Bash command that runs in the sandbox run without
// asking, when the sandbox says so: as an allow rule would, after the deny
// rules, the ask rules and the checks no allow rule passes (a protected
// file, a dangerous removal), which hold as they do for every command; and
// never in plan mode, for a command on another machine, or for a Monitor.
func (e *Engine) sandboxAllows(req *request) (Decision, bool) {
	sb := e.sandbox
	if !sb.On || !sb.AutoAllow || req.canon != toolBash || req.remote != "" || e.mode == ModePlan {
		return Decision{}, false
	}
	var command string
	var asked bool
	_ = json.Unmarshal(req.input["command"], &command)
	_ = json.Unmarshal(req.input["dangerouslyDisableSandbox"], &asked)
	if sb.Outside != nil && sb.Outside(command, asked) {
		return Decision{}, false
	}
	return Decision{Behavior: Allow, Stage: StageSandbox, Mode: e.mode,
		Reason: "It runs in the sandbox, which lets it write only in the session's folders and reach no network (sandbox.autoAllowBashIfSandboxed)."}, true
}

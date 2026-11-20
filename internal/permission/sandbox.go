package permission

import (
	"encoding/json"
	"strings"
)

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
	_ = json.Unmarshal(req.input["command"], &command)
	// The model's asking out of the sandbox, read so that anything the
	// shell tool could take for a yes is one here: only false (or
	// "false", or nothing) keeps the call in (the sandbox review: the tool
	// takes "true" in quotes too).
	asked := false
	if raw, ok := req.input["dangerouslyDisableSandbox"]; ok {
		switch strings.ToLower(strings.TrimSpace(strings.Trim(strings.TrimSpace(string(raw)), `"`))) {
		case "false", "", "null":
		default:
			asked = true
		}
	}
	if sb.Outside != nil && sb.Outside(command, asked) {
		return Decision{}, false
	}
	return Decision{Behavior: Allow, Stage: StageSandbox, Mode: e.mode,
		Reason: "It runs in the sandbox, which lets it write only in the session's folders and reach no network (sandbox.autoAllowBashIfSandboxed)."}, true
}

// SandboxPaths are the paths the Read and Edit rules name, for the
// sandbox's own lists, as Claude Code adds them: the Read deny rules'
// paths to what a sandboxed command may not read, the Edit deny rules' to
// what it may not write, the Edit allow rules' to where it may. A rule's
// path counts when it is written out in full, or with a last /** (the
// folder with what is in it); a Read deny rule's pattern counts too, as
// the pattern (the sandbox finds the paths it matches: the sandbox
// review); any other pattern is left out, as the sandbox mounts paths. A
// "!" rule that opens a path again is left out too.
func (e *Engine) SandboxPaths() (denyRead, denyWrite, allowWrite []string) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	path := func(c *crule, patterns bool) (string, bool) {
		if c.kind != kindPath || c.negate {
			return "", false
		}
		// (The path as written: segs are keys, folded for matching, which
		// a case-sensitive disk does not know: the deny rule's path had
		// not been there to hide.)
		segs := c.path.orig
		n := 0
		for n < len(segs) && segs[n] != "**" && !hasPathMagic(segs[n]) {
			n++
		}
		lit := "/" + strings.Join(segs[:n], "/")
		switch {
		case n == len(segs) || n == len(segs)-1 && segs[n] == "**":
			return lit, true
		case patterns:
			if segs[len(segs)-1] == "**" {
				segs = segs[:len(segs)-1]
			}
			return "/" + strings.Join(segs, "/"), true
		}
		return "", false
	}
	for _, c := range e.rules(ListDeny) {
		switch c.canon {
		case toolRead:
			if p, ok := path(c, true); ok {
				denyRead = append(denyRead, p)
			}
		case toolEdit:
			if p, ok := path(c, false); ok {
				denyWrite = append(denyWrite, p)
			}
		}
	}
	for _, c := range e.rules(ListAllow) {
		if p, ok := path(c, false); ok && c.canon == toolEdit {
			allowWrite = append(allowWrite, p)
		}
	}
	return denyRead, denyWrite, allowWrite
}

// FetchAllows reports whether a WebFetch(domain:...) allow rule covers the
// host: Claude Code takes those rules into its sandbox's allowed domains,
// and keeps a host the person let a sandboxed command reach as one.
func (e *Engine) FetchAllows(host string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, c := range e.rules(ListDeny) {
		if c.kind == kindDomain && domainMatch(c.domain, host) {
			return false
		}
	}
	for _, c := range e.rules(ListAllow) {
		if c.kind == kindDomain && domainMatch(c.domain, host) {
			return true
		}
	}
	return false
}

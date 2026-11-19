package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

// The same command configured twice runs once, but two hooks that run it
// differently are two guards: a copy in the background listed first does
// not turn the blocking one into a hook that decides nothing.
func TestSameCommandRunDifferentlyIsTwoHooks(t *testing.T) {
	r := newRunner(t, options(t),
		Hook{Event: PreToolUse, Matcher: "Bash", Command: "echo no >&2; exit 2", Async: true},
		Hook{Event: PreToolUse, Matcher: "Bash", Command: "echo no >&2; exit 2"})
	if out := run(t, r, input(PreToolUse)); !out.Blocked {
		t.Errorf("the blocking copy did not decide: %+v", out)
	}
}

// A PermissionRequest hook that could not check answers nothing, and
// another hook's approval does not stand in for it: the question goes to
// the person.
func TestPermissionRequestApprovalNeedsEveryHook(t *testing.T) {
	approve := prints(t, `{"hookSpecificOutput": {"hookEventName": "PermissionRequest", "decision": {"behavior": "allow"}}}`)
	r := newRunner(t, options(t),
		Hook{Event: PermissionRequest, Command: approve},
		Hook{Event: PermissionRequest, Command: "exit 3"})
	out := run(t, r, input(PermissionRequest))
	if out.PermissionDecision == Allow {
		t.Errorf("one hook's approval stood though another could not check: %+v", out)
	}
	if len(out.Warnings) == 0 {
		t.Error("nothing said that a hook could not check")
	}
}

// In a folder that is not trusted, a hook of the person's own that runs a
// program in the project is not run: a repository just cloned could have
// put anything there. A guard of that kind blocks what it would check.
func TestOwnHookOfAProjectProgramWaitsForTrust(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	hook := Hook{Event: PreToolUse, Matcher: "Bash", Global: true,
		Command: `touch ` + shq(marker) + `; "$CLAUDE_PROJECT_DIR"/.claude/hooks/check.sh`}
	opts := options(t)
	opts.Untrusted = true
	out := run(t, newRunner(t, opts, hook), input(PreToolUse))
	if !out.Blocked {
		t.Errorf("a guard that could not run did not block: %+v", out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the hook ran in a folder that is not trusted")
	}
	contains(t, "what was said", out.Reason, "not trusted")

	// Trusted, it runs; and in a folder that is not trusted, a hook of the
	// project's own never gets this far (it waits for trust in config).
	opts.Untrusted = false
	run(t, newRunner(t, opts, hook), input(PreToolUse))
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("in a trusted folder the hook did not run: %v", err)
	}
}

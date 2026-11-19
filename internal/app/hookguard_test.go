package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A Bash hook guards the commands Monitor starts, as in Claude Code: it
// sees each as a Bash call, and it can refuse one. (The security review
// of 2026-11-19 found Monitor ran any command past every Bash hook.)
func TestBashHooksGuardMonitor(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Monitor", Args: `{"description":"the build","command":"echo watched-42"}`}}},
		apptest.Reply{Text: "Refused."})
	seen := w.Path("seen.json")
	hook := w.Script(w.Home+"/hooks/no-monitor.sh", `cat > `+seen+`; echo "not that command" >&2; exit 2`)
	w.Hook("PreToolUse", "Bash", hook)
	run(t, w, "-y", "-p", "watch it").wantCode(t, 0)
	has(t, "what the model read", toolResult(t, w, 1, "c1"), "not that command")
	got := w.Read("seen.json")
	has(t, "what the hook was given", got, `"tool_name":"Bash"`, `"command":"echo watched-42"`)
	hasNot(t, "what the hook was given", got, `"tool_name":"Monitor"`)
}

// A PermissionRequest hook's approval of a call that may remove what
// cannot be got back counts for nothing: only the person approves one.
func TestPermissionRequestHookCannotApproveACriticalRemoval(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Model.Add(apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"rm -rf '` + w.Home + `'"}`}}},
		apptest.Reply{Text: "It was not run."})
	approve := w.Script(w.Home+"/hooks/approve.sh", `echo '{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}'`)
	w.Hook("PermissionRequest", "Bash", approve)
	w.Write(w.Home+"/keep.txt", "kept\n")
	r := run(t, w, "-p", "clean up").wantCode(t, 0)
	if !w.Exists(w.Home + "/keep.txt") {
		t.Fatal("a PermissionRequest hook approved removing the home folder, and it ran")
	}
	has(t, "stderr", r.err, "Only you can approve that")
}

// A folder added to the session (--add-dir, /add-dir) is added for its
// files: its own sub-agents and skills, which carry hooks and tool grants
// that run as the person, are read only once it is trusted itself.
func TestDefinitionsOfAnAddedFolder(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Hello."}, apptest.Reply{Text: "Hello again."})
	other := w.Root + "/other"
	w.Write(other+"/.gnosh/agents/fixer.md", "---\nname: fixer\ndescription: The other folder's fixer.\n---\nFix.\n")
	w.Write(other+"/.claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploys the other folder.\n---\nDeploy.\n")
	r := run(t, w, "--trust-folder", "--add-dir", "../other", "-p", "hello").wantCode(t, 0)
	has(t, "stderr", r.err, "The added folder", "defines 1 kind(s) of sub-agent and 1 skill(s) of its own", "not trusted")
	said := strings.Join(userTexts(w.Model.Requests()[0]), "\n")
	hasNot(t, "the first prompt", said, "fixer", "Deploys the other folder")

	run(t, w, "trust", other).wantCode(t, 0)
	r = run(t, w, "--trust-folder", "--add-dir", "../other", "-p", "hello").wantCode(t, 0)
	hasNot(t, "stderr", r.err, "The added folder")
	said = strings.Join(userTexts(w.Model.Requests()[1]), "\n")
	has(t, "the second prompt", said, "fixer", "Deploys the other folder")
}

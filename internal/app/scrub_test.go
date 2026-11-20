package app

import (
	"runtime"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/sandbox"
)

// With CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1 the model's commands and the
// person's hooks start without the credentials gnosh was started with,
// their own settings' among them; a package index keeps its URL without
// the password, and what is no credential stays. Without it, nothing is
// taken.
func TestSubprocessEnvScrub(t *testing.T) {
	probe := `{"command": "echo \"[$NPM_TOKEN|$SET_TOKEN|$PLAIN|$PIP_INDEX_URL|$GITHUB_TOKEN]\""}`
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: probe}}}, apptest.Reply{Text: "ok"},
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: probe}}}, apptest.Reply{Text: "ok"},
	)
	seen := w.Home + "/hook-env.txt"
	w.Settings(`{"env": {"SET_TOKEN": "from-settings"}, "hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "env > ` + seen + `"}]}]}}`)
	env := []string{"NPM_TOKEN=abc123", "PLAIN=hello", "PIP_INDEX_URL=https://u:pw@pypi.example/simple", "GITHUB_TOKEN=ghp_keptkeptkeptkept1234"}

	o := testOptions(w, "-y", "-p", "look")
	o.Environ = append(append(o.Environ, env...), "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1")
	runWith(t, t.Context(), o).wantCode(t, 0)
	has(t, "a command's environment, scrubbed", toolResult(t, w, 1, "c1"), "[||hello|https://pypi.example/simple|ghp_keptkeptkeptkept1234]")
	hook := w.Read(seen)
	hasNot(t, "a hook's environment, scrubbed", hook, "NPM_TOKEN=", "SET_TOKEN=", "u:pw@")
	has(t, "a hook's environment, scrubbed", hook, "PLAIN=hello", "GITHUB_TOKEN=")

	o = testOptions(w, "-y", "-p", "look")
	o.Environ = append(o.Environ, env...)
	runWith(t, t.Context(), o).wantCode(t, 0)
	has(t, "a command's environment, as it was", toolResult(t, w, 3, "c1"), "[abc123|from-settings|hello|https://u:pw@pypi.example/simple|")
	has(t, "a hook's environment, as it was", w.Read(seen), "NPM_TOKEN=abc123", "SET_TOKEN=from-settings")
}

// With the scrub on, the sandbox's settings that let a command out are
// taken back: filesystem.disabled anywhere, and on Linux the commands it
// excludes and the model's asking out of it; /sandbox says so.
func TestScrubTakesBackSandboxLoosenings(t *testing.T) {
	if st := sandbox.Check(); !st.OK {
		t.Skip(st.Why)
	}
	w := apptest.NewWorld(t)
	w.Settings(`{"sandbox": {"enabled": true, "excludedCommands": ["docker *"], "filesystem": {"disabled": true}}}`)
	o := testOptions(w, "-p", "/sandbox")
	o.Environ = append(o.Environ, "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB=1")
	r := runWith(t, t.Context(), o).wantCode(t, 0)
	hasNot(t, "/sandbox with the scrub on", r.out, "Files: left open")
	if runtime.GOOS == "linux" {
		hasNot(t, "/sandbox with the scrub on, on Linux", r.out, "docker *", "dangerouslyDisableSandbox")
		has(t, "/sandbox with the scrub on, on Linux", r.out, "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB")
	}
	r = run(t, w, "-p", "/sandbox").wantCode(t, 0)
	if !strings.Contains(r.out, "Files: left open") {
		t.Errorf("without the scrub, filesystem.disabled is not in force:\n%s", r.out)
	}
}

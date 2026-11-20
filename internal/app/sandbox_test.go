package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/sandbox"
)

// With the sandbox on and autoAllowBashIfSandboxed, a gnosh -p run lets a
// command that writes in the project run without anybody's approval (it
// runs in the sandbox), and a command that writes outside fails there with
// a note; the model is told of the sandbox. Skipped where the sandbox
// cannot run.
func TestSandboxedCommandsRunWithoutAsking(t *testing.T) {
	if st := sandbox.Check(); !st.OK {
		t.Skip(st.Why)
	}
	outside, err := os.MkdirTemp("/var/tmp", "gnosh-app-sandbox-")
	if err != nil {
		t.Skip("no /var/tmp")
	}
	t.Cleanup(func() { os.RemoveAll(outside) })
	w := apptest.NewWorld(t)
	w.Settings(`{"sandbox": {"enabled": true, "autoAllowBashIfSandboxed": true}}`)
	w.Write(".claude/settings.json", "{}\n")
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		if n == 0 {
			return &apptest.Reply{Calls: []apptest.Call{
				{ID: "c1", Name: "Bash", Args: `{"command":"echo made > made.txt && echo ok"}`},
				{ID: "c2", Name: "Bash", Args: `{"command":"echo out > ` + outside + `/out.txt"}`},
				// (A write gnosh cannot see in the command: the sandbox
				// keeps the settings file read-only all the same.)
				{ID: "c3", Name: "Bash", Args: `{"command":"python3 -c \"open('.claude/settings.json', 'a').write('x')\""}`},
			}}
		}
		return &apptest.Reply{Text: "Done."}
	})
	run(t, w, "-p", "make it").wantCode(t, 0)
	if !w.Exists("made.txt") {
		t.Errorf("the sandboxed command did not run: %s", toolResult(t, w, 1, "c1"))
	}
	has(t, "the write outside", toolResult(t, w, 1, "c2"), "Read-only file system", "it ran in the sandbox", "dangerouslyDisableSandbox set to true")
	if _, err := os.Stat(filepath.Join(outside, "out.txt")); err == nil {
		t.Error("a sandboxed command wrote outside the project")
	}
	has(t, "a write to a guarded file", toolResult(t, w, 1, "c3"), "Read-only file system")
	if got := w.Read(".claude/settings.json"); got != "{}\n" {
		t.Errorf("a sandboxed command changed a settings file: %q", got)
	}
	has(t, "what the model is told of Bash", string(w.Model.Requests()[0].Body), "In the sandbox:", "dangerouslyDisableSandbox")
}

// excludedCommands takes a plain command out of the sandbox, never a
// list, a pipe, a redirection, a substitution or a cd, and a git clone
// whose path leads elsewhere stays in.
func TestExcludedCall(t *testing.T) {
	entries := []config.ListItem{{V: "docker *"}, {V: "make"}, {V: "git *"}, {V: "npm run *:dev"}}
	for cmd, want := range map[string]bool{
		"docker ps": true, "docker": true, "  docker   build . ": true, "dockerd": false, "make": true, "make all": false,
		"docker ps; rm x": false, "docker ps | cat": false, "docker build . > log": false, "docker $(x)": false, "cd x && docker ps": false,
		"git clone https://example.com/r vendor/lib": true, "git clone https://example.com/r ~/tools": false, "git worktree add ../x": false,
		"npm run web:dev": true, "npm run web:prod": false, "sudo docker ps": false, "": false,
	} {
		if got := excludedCall(entries, cmd); got != want {
			t.Errorf("excludedCall(%q) = %v", cmd, got)
		}
	}
	if excludedCall(nil, "docker ps") {
		t.Error("no entries took a command out")
	}
	for _, want := range []string{".git/hooks", ".bashrc", ".claude/settings.json", ".gnosh/skills", ".envrc"} {
		if !slices.Contains(sandboxGuarded, want) {
			t.Errorf("%s is not guarded", want)
		}
	}
}

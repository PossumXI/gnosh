package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// gitIn runs git in dir for a test, as nobody in particular.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
	cmd.Env = []string{"HOME=" + dir, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repoWorld is a world whose folder is a git repository with one commit.
func repoWorld(t *testing.T, replies ...apptest.Reply) *apptest.World {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on this machine")
	}
	w := apptest.NewWorld(t, replies...)
	w.Write("main.go", "package main\n")
	gitIn(t, w.Dir, "init", "-q")
	gitIn(t, w.Dir, "add", "main.go")
	gitIn(t, w.Dir, "commit", "-q", "-m", "first")
	return w
}

// A sub-agent asked for a work tree of its own (isolation worktree) works
// in a git work tree of the repository at its current commit: what it
// writes lands there, without a question in acceptEdits, and not in the
// folder the session works in; the report says where it is kept. One that
// changed nothing is removed with its branch. An untrusted folder, and one
// that is no repository, get no work tree.
func TestSubAgentWorktree(t *testing.T) {
	w := repoWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"write it","prompt":"Write new.txt.","isolation":"worktree"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Write", Args: `{"file_path":"new.txt","content":"made in the work tree\n"}`}}},
		apptest.Reply{Text: "Wrote new.txt."},
		apptest.Reply{Text: "Done."},
	)
	run(t, w, "--permission-mode", "acceptEdits", "--trust-folder", "-p", "write it").wantCode(t, 0)
	if w.Exists("new.txt") {
		t.Errorf("the sub-agent wrote into the session's own folder")
	}
	report := toolResult(t, w, 3, "c1")
	has(t, "the report", report, "Wrote new.txt.", "It worked in its own git work tree", "on the branch gnosh/agent-", "it holds 1 file it changed and did not commit")
	m := regexp.MustCompile(`work tree, (\S+), on the branch`).FindStringSubmatch(report)
	if m == nil {
		t.Fatalf("the report names no work tree:\n%s", report)
	}
	if got, err := os.ReadFile(filepath.Join(m[1], "new.txt")); err != nil || string(got) != "made in the work tree\n" {
		t.Errorf("the work tree holds %q, %v", got, err)
	}
	if status := gitIn(t, w.Dir, "status", "--porcelain"); strings.Contains(status, "worktrees") || strings.Contains(status, "new.txt") {
		t.Errorf("the repository's status shows the work tree:\n%s", status)
	}

	// One that changed nothing is removed, with its branch.
	w = repoWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"look","prompt":"Look.","isolation":"worktree"}`}}},
		apptest.Reply{Text: "Nothing to change."},
		apptest.Reply{Text: "Done."},
	)
	run(t, w, "--trust-folder", "-p", "look").wantCode(t, 0)
	has(t, "the report", toolResult(t, w, 2, "c1"), "Nothing to change.", "changed nothing there; the work tree is removed.")
	if list := gitIn(t, w.Dir, "worktree", "list"); strings.Count(list, "\n") != 0 {
		t.Errorf("a work tree is left:\n%s", list)
	}
	if branches := gitIn(t, w.Dir, "branch", "--list", "gnosh/*"); branches != "" {
		t.Errorf("its branch is left: %s", branches)
	}

	// An untrusted folder gets none, and neither does a folder that is no
	// repository.
	w = repoWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"look","prompt":"Look.","isolation":"worktree"}`}}},
		apptest.Reply{Text: "Done."},
	)
	run(t, w, "-p", "look").wantCode(t, 0)
	has(t, "an untrusted folder", toolResult(t, w, 1, "c1"), "No sub-agent was started", "this folder is not trusted")
	w = apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"look","prompt":"Look.","isolation":"worktree"}`}}},
		apptest.Reply{Text: "Done."},
	)
	run(t, w, "--trust-folder", "-p", "look").wantCode(t, 0)
	has(t, "no repository", toolResult(t, w, 1, "c1"), "No sub-agent was started", "needs a git repository")
}

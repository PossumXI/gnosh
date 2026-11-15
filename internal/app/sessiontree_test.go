package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// gnosh --worktree name works in a git work tree of the repository's own, in
// .gnosh/worktrees/name on the branch worktree-name: what it writes lands
// there, not in the person's checkout, and the work tree, named, is kept
// with a line that says how to come back to it. --worktree name again
// opens it.
func TestWorktreeFlag(t *testing.T) {
	w := repoWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "w", Name: "Write", Args: `{"file_path":"made.txt","content":"in the work tree\n"}`}}},
		apptest.Reply{Text: "Made it."},
		apptest.Reply{Text: "Still here."},
	)
	tree := filepath.Join(w.Dir, ".gnosh", "worktrees", "feature-x")
	r := run(t, w, "--trust-folder", "--worktree", "feature-x", "--permission-mode", "acceptEdits", "-p", "make it").wantCode(t, 0)
	has(t, "stderr", r.err, "gnosh works in the git work tree "+tree+", made, on the branch worktree-feature-x",
		"the work tree "+tree+", on the branch worktree-feature-x, is kept (it holds 1 file changed or made and not committed", "gnosh --worktree feature-x works in it again")
	if got, err := os.ReadFile(filepath.Join(tree, "made.txt")); err != nil || string(got) != "in the work tree\n" {
		t.Errorf("made.txt in the work tree: %q, %v", got, err)
	}
	if w.Exists("made.txt") {
		t.Errorf("the session wrote into the person's checkout")
	}
	if branch := gitIn(t, w.Dir, "branch", "--list", "worktree-feature-x"); !strings.Contains(branch, "worktree-feature-x") {
		t.Errorf("no branch worktree-feature-x: %q", branch)
	}
	// Again: it is opened, not made.
	r = run(t, w, "--trust-folder", "--worktree", "feature-x", "-p", "still there?").wantCode(t, 0)
	has(t, "stderr", r.err, "gnosh works in the git work tree "+tree+", opened again, on the branch worktree-feature-x")
}

// Without a name, the work tree's name is made up, and one left as it was
// made goes with its branch when the session ends.
func TestWorktreeFlagUnnamed(t *testing.T) {
	w := repoWorld(t, apptest.Reply{Text: "Nothing to do."})
	r := run(t, w, "--trust-folder", "--worktree", "-p", "look").wantCode(t, 0)
	m := regexp.MustCompile(`work tree (\S+), made, on the branch (worktree-\S+) at`).FindStringSubmatch(r.err)
	if m == nil {
		t.Fatalf("no work tree was made:\n%s", r.err)
	}
	has(t, "stderr", r.err, "the work tree "+m[1]+" was left as it was made, and is removed with its branch")
	if fileExists(m[1]) {
		t.Errorf("the work tree %s is left", m[1])
	}
	if branch := gitIn(t, w.Dir, "branch", "--list", m[2]); branch != "" {
		t.Errorf("its branch is left: %q", branch)
	}
}

// The session's edit of a file in the person's checkout is refused, with
// the same file in the work tree named.
func TestWorktreeFlagKeepsToItsTree(t *testing.T) {
	w := repoWorld(t)
	target := filepath.Join(w.Dir, "notes.txt")
	w.Model.Add(apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Write", Args: fmt.Sprintf(`{"file_path":%q,"content":"x\n"}`, target)}}})
	w.Model.Add(apptest.Reply{Text: "It was refused."})
	run(t, w, "--trust-folder", "--worktree", "iso", "--permission-mode", "acceptEdits", "-p", "write it").wantCode(t, 0)
	if w.Exists("notes.txt") {
		t.Errorf("the session wrote into the person's checkout")
	}
	has(t, "the refusal", toolResult(t, w, 1, "c1"), "The same file in your work tree is "+filepath.Join(w.Dir, ".gnosh", "worktrees", "iso", "notes.txt"))
}

// --worktree needs a trusted folder in a git repository, and a name that can be a
// folder's and a branch's.
func TestWorktreeFlagRefusals(t *testing.T) {
	w := repoWorld(t)
	has(t, "an untrusted folder", run(t, w, "--worktree", "x", "-p", "hi").wantCode(t, 1).err, "--worktree: a work tree is made with git, and this folder is not trusted")
	has(t, "a bad name", run(t, w, "--trust-folder", "--worktree", "fix the bug", "-p", "hi").wantCode(t, 1).err, `"fix the bug" cannot name a work tree`, "--worktree=")
	has(t, "a name that climbs out", run(t, w, "--trust-folder", "--worktree", "../x", "-p", "hi").wantCode(t, 1).err, `"../x" cannot name a work tree`)
	plain := apptest.NewWorld(t)
	has(t, "no repository", run(t, plain, "--trust-folder", "--worktree", "x", "-p", "hi").wantCode(t, 1).err, "is not in one")
}

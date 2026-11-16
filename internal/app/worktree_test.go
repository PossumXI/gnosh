package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/core"
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
	has(t, "the report", report, "Wrote new.txt.", "It worked in its own git work tree", "on the branch gnosh-agent-", "it holds 1 file changed or made and not committed", "worktree remove --force")
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

// From a folder below the repository's top, the sub-agent starts in the
// same folder of its work tree, and its writes there are a working
// directory's, without a question in acceptEdits. Two sub-agents in one
// reply get work trees of their own: no two share a name.
func TestSubAgentWorktreeFromASubfolder(t *testing.T) {
	w := repoWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "Agent", Args: `{"description":"one","prompt":"Write one.txt.","isolation":"worktree","run_in_background":true}`},
			{ID: "c2", Name: "Agent", Args: `{"description":"two","prompt":"Write two.txt.","isolation":"worktree","run_in_background":true}`},
		}},
	)
	w.Write("sub/keep.txt", "kept\n")
	gitIn(t, w.Dir, "add", "sub/keep.txt")
	gitIn(t, w.Dir, "commit", "-q", "-m", "sub")
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		body := string(r.Body)
		if !strings.Contains(body, "You are a sub-agent inside gnosh") {
			if n == 0 {
				return nil // the session's first reply: the script's
			}
			return &apptest.Reply{Text: "Noted."}
		}
		file := "one.txt"
		if strings.Contains(body, "Write two.txt.") {
			file = "two.txt"
		}
		if strings.Contains(body, `"role":"tool"`) {
			return &apptest.Reply{Text: "Wrote " + file + "."}
		}
		return &apptest.Reply{Calls: []apptest.Call{{ID: "w", Name: "Write", Args: fmt.Sprintf(`{"file_path":%q,"content":%q}`, file, file+"\n")}}}
	})
	o := testOptions(w, "--permission-mode", "acceptEdits", "--trust-folder", "-p", "write them")
	o.Cwd = filepath.Join(w.Dir, "sub")
	runWith(t, t.Context(), o).wantCode(t, 0)
	trees, _ := filepath.Glob(filepath.Join(w.Dir, ".gnosh", "worktrees", "agent-*"))
	if len(trees) != 2 {
		t.Fatalf("work trees %q, want two of their own", trees)
	}
	var made []string
	for _, tree := range trees {
		for _, f := range []string{"one.txt", "two.txt"} {
			if data, err := os.ReadFile(filepath.Join(tree, "sub", f)); err == nil && strings.TrimSpace(string(data)) != "" {
				made = append(made, f)
			}
		}
	}
	if strings.Join(made, " ") != "one.txt two.txt" && strings.Join(made, " ") != "two.txt one.txt" {
		t.Errorf("the files written in the work trees' sub folders: %q", made)
	}
	if w.Exists("sub/one.txt") || w.Exists("sub/two.txt") {
		t.Errorf("a sub-agent wrote into the session's own folder")
	}
}

// A sub-agent with a work tree of its own is told where it is, and its
// write into the person's checkout is refused, even in acceptEdits, with
// the same file in its work tree named; it writes that one, and the
// checkout is left as it was.
func TestSubAgentWorktreeKeepsToItsTree(t *testing.T) {
	w := repoWorld(t)
	same := regexp.MustCompile(`The same file in your work tree is (/[^:"\\]+): change that one`)
	var told, refused bool
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		body := string(r.Body)
		if !strings.Contains(body, "You are a sub-agent inside gnosh") {
			if strings.Contains(body, `"role":"tool"`) {
				return &apptest.Reply{Text: "Done."}
			}
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"note it","prompt":"Write notes.txt.","isolation":"worktree"}`}}}
		}
		told = told || strings.Contains(body, "You work in a git work tree of your own, at ")
		if m := same.FindStringSubmatch(body); m != nil {
			refused = true
			if strings.Count(body, `"role":"tool"`) >= 2 {
				return &apptest.Reply{Text: "Wrote notes.txt in the work tree."}
			}
			return &apptest.Reply{Calls: []apptest.Call{{ID: "w2", Name: "Write", Args: fmt.Sprintf(`{"file_path":%q,"content":"noted\n"}`, m[1])}}}
		}
		return &apptest.Reply{Calls: []apptest.Call{{ID: "w1", Name: "Write", Args: fmt.Sprintf(`{"file_path":%q,"content":"noted\n"}`, filepath.Join(w.Dir, "notes.txt"))}}}
	})
	run(t, w, "--permission-mode", "acceptEdits", "--trust-folder", "-p", "note it").wantCode(t, 0)
	if !told {
		t.Errorf("the sub-agent was not told where its work tree is")
	}
	if !refused {
		t.Errorf("the write into the person's checkout was not refused")
	}
	if w.Exists("notes.txt") {
		t.Errorf("the sub-agent wrote into the person's checkout")
	}
	made, _ := filepath.Glob(filepath.Join(w.Dir, ".gnosh", "worktrees", "agent-*", "notes.txt"))
	if len(made) != 1 {
		t.Errorf("notes.txt in the work tree: %q", made)
	}
}

// A sub-agent's work tree made in a --worktree session's stays out of the
// person's checkout too, not only out of the session's work tree it was
// made from.
func TestSubAgentWorktreeInAWorktreeSession(t *testing.T) {
	w := repoWorld(t)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		body := string(r.Body)
		if !strings.Contains(body, "You are a sub-agent inside gnosh") {
			if strings.Contains(body, `"role":"tool"`) {
				return &apptest.Reply{Text: "Done."}
			}
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"note it","prompt":"Write notes.txt.","isolation":"worktree"}`}}}
		}
		if strings.Contains(body, `"role":"tool"`) {
			return &apptest.Reply{Text: "Refused."}
		}
		return &apptest.Reply{Calls: []apptest.Call{{ID: "w1", Name: "Write", Args: fmt.Sprintf(`{"file_path":%q,"content":"noted\n"}`, filepath.Join(w.Dir, "notes.txt"))}}}
	})
	// In bypassPermissions, where the work tree is all that stands between
	// the sub-agent and the checkout.
	run(t, w, "--trust-folder", "--worktree", "iso", "-y", "-p", "note it").wantCode(t, 0)
	if w.Exists("notes.txt") {
		t.Errorf("the sub-agent wrote into the person's checkout")
	}
	if !anyRequestHas(w, "so that your changes stay apart from the person's checkout at ") {
		t.Errorf("the write was not refused as one into the person's checkout")
	}
}

// What gnosh keeps in a project's data folder is not a work tree's work:
// the folder of work trees, the local settings file and its lock, the
// .gitignore gnosh writes; a folder given whole counts when all in it is
// gnosh's.
func TestGnoshOwn(t *testing.T) {
	tree := t.TempDir()
	for rel, want := range map[string]bool{
		".gnosh/worktrees/.gitignore":     true,
		".gnosh/worktrees/agent-1/x.go":   false,
		".gnosh/settings.local.json":      true,
		".gnosh/settings.local.json.lock": true,
		".gnosh/.gitignore":               true,
		".gnosh/skills/x/SKILL.md":        false,
		"main.go":                         false,
	} {
		if got := gnoshOwn(tree, rel); got != want {
			t.Errorf("gnoshOwn(%q) = %v", rel, got)
		}
	}
	os.MkdirAll(filepath.Join(tree, ".gnosh", "worktrees"), 0o755)
	os.WriteFile(filepath.Join(tree, ".gnosh", "worktrees", ".gitignore"), []byte("*\n"), 0o644)
	os.WriteFile(filepath.Join(tree, ".gnosh", "settings.local.json"), []byte("{}"), 0o600)
	if !gnoshOwn(tree, ".gnosh/") || !gnoshOwn(tree, ".gnosh/worktrees/") {
		t.Errorf("a data folder of gnosh's own files alone counted as work")
	}
	// A work tree kept in the folder of work trees holds work: the tree it
	// lies in must not go with it (review, 16 November).
	os.MkdirAll(filepath.Join(tree, ".gnosh", "worktrees", "agent-1"), 0o755)
	if gnoshOwn(tree, ".gnosh/worktrees/") || gnoshOwn(tree, ".gnosh/") {
		t.Errorf("a folder of work trees with a kept work tree in it was taken for gnosh's alone")
	}
	os.RemoveAll(filepath.Join(tree, ".gnosh", "worktrees", "agent-1"))
	os.MkdirAll(filepath.Join(tree, ".gnosh", "skills"), 0o755)
	if gnoshOwn(tree, ".gnosh/") {
		t.Errorf("a data folder with a skills folder in it was taken for gnosh's alone")
	}
}

// An unnamed --worktree session that changed nothing itself, but whose
// sub-agent's work tree was kept with work in it, keeps its own tree:
// removing it would take the sub-agent's work along (review, 16 November).
func TestWorktreeFlagKeepsASubAgentsKeptTree(t *testing.T) {
	w := repoWorld(t)
	treeRe := regexp.MustCompile(`You work in a git work tree of your own, at (/[^:]+):`)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		body := string(r.Body)
		if strings.Contains(body, "You are a sub-agent inside gnosh") {
			if strings.Contains(body, `"role":"tool"`) {
				return &apptest.Reply{Text: "Wrote it."}
			}
			m := treeRe.FindStringSubmatch(body)
			if m == nil {
				return &apptest.Reply{Text: "No tree."}
			}
			return &apptest.Reply{Calls: []apptest.Call{{ID: "w1", Name: "Write", Args: fmt.Sprintf(`{"file_path":%q,"content":"kept\n"}`, filepath.Join(m[1], "notes.txt"))}}}
		}
		if strings.Contains(body, `"role":"tool"`) {
			return &apptest.Reply{Text: "Done."}
		}
		return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"note it","prompt":"Write notes.txt.","isolation":"worktree"}`}}}
	})
	r := run(t, w, "--trust-folder", "--worktree", "-y", "-p", "go").wantCode(t, 0)
	kept, _ := filepath.Glob(filepath.Join(w.Dir, ".gnosh", "worktrees", "*", ".gnosh", "worktrees", "agent-*", "notes.txt"))
	if len(kept) != 1 {
		t.Errorf("the sub-agent's kept work is gone: %q\n%s", kept, r.err)
	}
	has(t, "stderr", r.err, "is kept (it holds")
}

// An unnamed --worktree session that kept a rule in its local settings
// file, and changed nothing else, leaves its work tree as it was made: the
// tree goes at the end.
func TestWorktreeFlagOwnFilesAreNoWork(t *testing.T) {
	w := repoWorld(t)
	r := run(t, w, "--trust-folder", "--worktree", "-p", "/permissions allow Bash(ls *)").wantCode(t, 0)
	has(t, "stderr", r.err, "was left as it was made, and is removed with its branch")
}

// checkoutOf sees the checkouts a work tree stays out of through symbolic
// links and other spellings, and finds the innermost.
func TestCheckoutOf(t *testing.T) {
	root := resolved(t.TempDir())
	repo := filepath.Join(root, "app")
	outer := filepath.Join(repo, ".gnosh", "worktrees", "s")
	tree := filepath.Join(outer, ".gnosh", "worktrees", "a")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	wt := core.WorkTree{Path: tree, Repo: outer, Outer: []string{repo}}
	for _, c := range []struct{ path, want string }{
		{filepath.Join(repo, "main.go"), repo},
		{filepath.Join(link, "main.go"), repo},
		{filepath.Join(strings.ToUpper(repo), "main.go"), repo},
		{filepath.Join(outer, "main.go"), outer},
		{filepath.Join(tree, "main.go"), ""},
		{filepath.Join(root, "elsewhere", "x"), ""},
	} {
		if got, _ := checkoutOf(wt, c.path); got != c.want {
			t.Errorf("checkoutOf(%q) = %q, want %q", c.path, got, c.want)
		}
	}
	if got := checkoutName(wt, repo); !strings.HasPrefix(got, "the person's checkout at ") {
		t.Errorf("checkoutName(person's) = %q", got)
	}
	if got := checkoutName(wt, outer); !strings.HasPrefix(got, "the work tree at ") {
		t.Errorf("checkoutName(the session's) = %q", got)
	}
	if got := outsideTree(wt, tree, []string{filepath.Join(link, "x.go")}); !strings.Contains(got, "The same file in your work tree is "+filepath.Join(tree, "x.go")) {
		t.Errorf("outsideTree through a link: %q", got)
	}
}

// A session started in a work tree gnosh made for a sub-agent, and kept,
// works on its files as on any project's: the data folder the work tree
// lies in does not make them gnosh's own.
func TestSessionInAKeptWorktree(t *testing.T) {
	w := repoWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "w", Name: "Write", Args: `{"file_path":"kept.txt","content":"carried on\n"}`}}},
		apptest.Reply{Text: "Done."},
	)
	tree := filepath.Join(w.Dir, ".gnosh", "worktrees", "agent-0123456789ab")
	gitIn(t, w.Dir, "worktree", "add", "-q", "-b", "gnosh-agent-0123456789ab", tree)
	o := testOptions(w, "--permission-mode", "acceptEdits", "-p", "carry on")
	o.Cwd = tree
	runWith(t, t.Context(), o).wantCode(t, 0)
	if got, err := os.ReadFile(filepath.Join(tree, "kept.txt")); err != nil || string(got) != "carried on\n" {
		t.Errorf("the write in the kept work tree: %q, %v", got, err)
	}
}

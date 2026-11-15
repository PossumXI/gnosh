package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// This file is a sub-agent's own git work tree (isolation: worktree,
// spec/cc/subagents.md 2, tools.md 9). The sub-agent works in a work tree
// made for it, on a branch of its own that starts at the commit the
// repository has checked out, so that what it changes stays out of the
// folder the person works in until they take it. A work tree it changed
// nothing in is removed when it ends, branch and all; one it changed is
// kept, and its report says where it is and on which branch.
//
// The work trees lie in the project's data folder (.gnosh/worktrees), with
// a .gitignore of their own that keeps them out of the repository's
// status; the permission rules judge what is inside one as the project's
// own files (permission.Options.WorkTrees). Making one runs git, which
// gnosh does only in a folder the person has trusted, as /diff does: a
// repository's own settings can name programs git runs.
//
// It starts at HEAD, not at the default branch as Claude Code's does: the
// sub-agent is asked about the code as the person has it, and that is the
// commit they have checked out. What they have not committed is not in it.

// worktreesDir is the folder of work trees in the project's data folder.
const worktreesDir = "worktrees"

// worktree is the work tree of one sub-agent.
type worktree struct {
	repo   string // the repository's top folder
	path   string // the work tree's folder
	branch string // its branch
	base   string // the commit it started at
}

// gitEnvDrop are the variables that would point git at another repository
// than the one it is run in.
var gitEnvDrop = []string{"GIT_DIR=", "GIT_WORK_TREE=", "GIT_INDEX_FILE=", "GIT_COMMON_DIR=", "GIT_OBJECT_DIRECTORY="}

// git runs git in dir and returns what it printed, without the programs a
// repository's settings could start from the file-system monitor, and
// without the variables that would point it elsewhere. An error says what
// git said.
func (l *live) git(ctx context.Context, dir string, args ...string) (string, error) {
	return runGit(ctx, l.p.o.Environ, dir, args...)
}

// runGit is live.git for code that has no session yet: git in dir, in
// the environment environ, as live.git runs it.
func runGit(ctx context.Context, environ []string, dir string, args ...string) (string, error) {
	base := []string{"--no-pager", "-c", "core.fsmonitor=false", "-C", dir}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	env := slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		return slices.ContainsFunc(gitEnvDrop, func(p string) bool { return strings.HasPrefix(kv, p) })
	})
	cmd.Env = append(env, "GIT_PAGER=cat", "PAGER=cat", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(strings.TrimSpace(string(ee.Stderr))) > 0 {
			return "", errors.New(oneLine(strings.TrimSpace(string(ee.Stderr)), 300))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// treeName is the name a sub-agent's work tree and branch take: the random
// end of its id, which no other sub-agent shares (its start is a clock).
func treeName(id string) string {
	hex := strings.ReplaceAll(id, "-", "")
	if len(hex) > 12 {
		hex = hex[len(hex)-12:]
	}
	return "agent-" + hex
}

// makeWorktree makes the work tree of the sub-agent id, in the repository
// the folder cwd is in, and returns it with the folder the sub-agent
// starts in: the one in the work tree that stands where cwd stands in the
// repository. The work tree is a working directory of the session from
// then on, as /add-dir makes one. Its error is worded for the model; what
// a failed attempt left behind is taken away.
func (l *live) makeWorktree(ctx context.Context, cwd, id string) (*worktree, string, error) {
	if !l.st.trusted {
		return nil, "", errors.New("a work tree of its own (isolation worktree) is made with git, and this folder is not trusted: a repository's own settings can name programs git runs. The user can trust it with gnosh trust")
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, "", errors.New("a work tree of its own needs git, and there is none on this machine's PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	top, err := l.git(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, "", fmt.Errorf("a work tree of its own needs a git repository, and %s is not in one", cwd)
	}
	base, err := l.git(ctx, top, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil || base == "" {
		return nil, "", errors.New("the repository has no commit yet, so there is nothing to make a work tree of")
	}
	root, err := treesFolder(l.cfg.Paths.DataDir)
	if err != nil {
		return nil, "", err
	}
	name := treeName(id)
	wt := &worktree{repo: top, path: filepath.Join(root, name), branch: "gnosh-" + name, base: base}
	if _, err := l.git(ctx, top, "rev-parse", "--verify", "--quiet", "refs/heads/"+wt.branch); err == nil || fileExists(wt.path) {
		return nil, "", fmt.Errorf("the branch %s or the folder %s is there already, so no work tree was made", wt.branch, wt.path)
	}
	if err := addTree(ctx, l.p.o.Environ, wt, true); err != nil {
		return nil, "", err
	}
	if err := l.engine.AddDir(wt.path); err != nil {
		l.flog.logf("turn", "the work tree %s could not be made a working directory: %v", wt.path, err)
	}
	start := wt.path
	if real, err := filepath.EvalSymlinks(cwd); err == nil {
		if rel, err := filepath.Rel(top, real); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			if st, err := os.Stat(filepath.Join(wt.path, rel)); err == nil && st.IsDir() {
				start = filepath.Join(wt.path, rel)
			}
		}
	}
	l.flog.logf("turn", "sub-agent %s works in the work tree %s on the branch %s, from %s", shortID(id), wt.path, wt.branch, shortCommit(base))
	return wt, start, nil
}

// endWorktree ends a sub-agent's work tree once it has ended. A work tree
// the sub-agent left as it found it (nothing changed, nothing new, even
// what git ignores; its branch and its HEAD still at the commit it started
// at) is removed, branch and all, and nothing that could hold its work is
// forced away. Any other is kept, and the line for the report says where
// it is, what it holds and how to take it away. kept says it was kept.
func (l *live) endWorktree(wt *worktree) (note string, kept bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	keep := func(why string) (string, bool) {
		return fmt.Sprintf("[It worked in its own git work tree, %s, on the branch %s, which started at %s: %s. None of it is in the folder you work in. Look at it there, and merge the branch or copy what you want; then git -C %s worktree remove --force %s and git -C %s branch -D %s take it away.]",
			wt.path, wt.branch, shortCommit(wt.base), why, wt.repo, wt.path, wt.repo, wt.branch), true
	}
	what, err := treeHolds(ctx, l.p.o.Environ, wt)
	if err != nil {
		return keep(err.Error() + ", so the work tree is kept")
	}
	if len(what) == 0 {
		if _, err := l.git(ctx, wt.repo, "worktree", "remove", wt.path); err != nil {
			return fmt.Sprintf("[It worked in its own git work tree, %s, and changed nothing there; the work tree could not be removed (%v).]", wt.path, err), true
		}
		if _, err := l.git(ctx, wt.repo, "branch", "-d", wt.branch); err != nil {
			return fmt.Sprintf("[It worked in its own git work tree and changed nothing there; the work tree is removed, and its branch %s is left (%v).]", wt.branch, err), false
		}
		return "[It worked in its own git work tree and changed nothing there; the work tree is removed.]", false
	}
	return keep("it holds " + strings.Join(what, " and "))
}

// treesFolder makes the folder of work trees in the data folder dataDir,
// with a .gitignore that keeps them out of the repository's status, and
// returns it. It refuses a folder that is a link: that would put the work
// trees, and what the permission rules let be changed in them, somewhere
// else.
func treesFolder(dataDir string) (string, error) {
	root := filepath.Join(dataDir, worktreesDir)
	if st, err := os.Lstat(root); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s is a link, and gnosh makes work trees only in a folder of its own there", root)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("the folder for work trees could not be made: %v", err)
	}
	if ignore := filepath.Join(root, ".gitignore"); !fileExists(ignore) {
		_ = os.WriteFile(ignore, []byte("*\n"), 0o644)
	}
	return root, nil
}

// addTree adds the work tree wt to its repository: on a new branch at its
// base when newBranch is set, else on its branch as it stands. What a
// failed attempt left behind (a branch git made, part of the folder) is
// taken away.
func addTree(ctx context.Context, environ []string, wt *worktree, newBranch bool) error {
	args := []string{"worktree", "add", "--quiet", wt.path, wt.branch}
	if newBranch {
		args = []string{"worktree", "add", "--quiet", "-b", wt.branch, wt.path, wt.base}
	}
	if _, err := runGit(ctx, environ, wt.repo, args...); err != nil {
		clean, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		_, _ = runGit(clean, environ, wt.repo, "worktree", "remove", "--force", wt.path)
		_ = os.RemoveAll(wt.path)
		_, _ = runGit(clean, environ, wt.repo, "worktree", "prune")
		if newBranch {
			_, _ = runGit(clean, environ, wt.repo, "branch", "-D", wt.branch)
		}
		return fmt.Errorf("git could not make the work tree: %v", err)
	}
	return nil
}

// treeHolds says what the work tree wt holds that its removal would take
// away: files changed or made and not committed (git's ignored ones
// among them), commits on its branch past where it started, a HEAD moved
// elsewhere. Nothing means it is as it was made. An error says what could
// not be told.
func treeHolds(ctx context.Context, environ []string, wt *worktree) ([]string, error) {
	status, err := runGit(ctx, environ, wt.path, "status", "--porcelain", "--ignored", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("whether anything was changed there could not be told (%v)", err)
	}
	head, err1 := runGit(ctx, environ, wt.path, "rev-parse", "HEAD")
	commits, err2 := runGit(ctx, environ, wt.repo, "rev-list", "--count", wt.base+"..refs/heads/"+wt.branch)
	n, err3 := strconv.Atoi(commits)
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, errors.New("how far its branch went could not be told")
	}
	var what []string
	if lines := strings.Count(strings.TrimSpace(status), "\n") + 1; strings.TrimSpace(status) != "" {
		what = append(what, countWords(lines, "file changed or made and not committed (git's ignored ones among them)", "files changed or made and not committed (git's ignored ones among them)"))
	}
	if n > 0 {
		what = append(what, countWords(n, "commit", "commits"))
	}
	if head != wt.base && n == 0 {
		what = append(what, "a HEAD that is no longer the commit it started at")
	}
	return what, nil
}

// fileExists reports whether there is a file or folder at p.
func fileExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// shortCommit is a commit's id as a person reads it.
func shortCommit(id string) string {
	if len(id) > 10 {
		return id[:10]
	}
	return id
}

// outerTrees is the folder of work trees a session's project lies in,
// when the project is one of the git work trees gnosh makes, kept after
// its sub-agent ended: <repo>/<data>/worktrees/<name>, whose .git is the
// file a work tree has. Its files are a checkout of the project like any
// other, and the data folder above them must not make them gnosh's own.
// "" otherwise. data is the data folder's name.
func outerTrees(root, data string) string {
	trees := filepath.Dir(root)
	if filepath.Base(trees) != worktreesDir || filepath.Base(filepath.Dir(trees)) != data {
		return ""
	}
	if info, err := os.Lstat(filepath.Join(root, ".git")); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return trees
}

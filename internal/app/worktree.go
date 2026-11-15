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

// git runs git in dir and returns what it printed, without the programs a
// repository's settings could start from the file-system monitor. An
// error says what git said.
func (l *live) git(ctx context.Context, dir string, args ...string) (string, error) {
	base := []string{"--no-pager", "-c", "core.fsmonitor=false", "-C", dir}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Env = append(slices.Clone(l.p.o.Environ), "GIT_PAGER=cat", "PAGER=cat", "GIT_TERMINAL_PROMPT=0")
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

// makeWorktree makes the work tree of the sub-agent id, in the repository
// the folder cwd is in, and returns it with the folder the sub-agent
// starts in: the one in the work tree that stands where cwd stands in the
// repository. Its error is worded for the model.
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
	root := filepath.Join(l.cfg.Paths.DataDir, worktreesDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, "", fmt.Errorf("the folder for work trees could not be made: %v", err)
	}
	if ignore := filepath.Join(root, ".gitignore"); !fileExists(ignore) {
		_ = os.WriteFile(ignore, []byte("*\n"), 0o644)
	}
	name := "agent-" + shortID(id)
	wt := &worktree{repo: top, path: filepath.Join(root, name), branch: "gnosh/" + name, base: base}
	if _, err := l.git(ctx, top, "worktree", "add", "--quiet", "-b", wt.branch, wt.path, base); err != nil {
		return nil, "", fmt.Errorf("git could not make the work tree: %v", err)
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

// endWorktree ends a sub-agent's work tree once it has ended: it removes a
// work tree with no change in it, branch and all, and keeps one with
// changes. It returns the line to put in the report, and whether the work
// tree was kept.
func (l *live) endWorktree(wt *worktree) (note string, kept bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	status, err := l.git(ctx, wt.path, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return fmt.Sprintf("[It worked in its own git work tree, %s, on the branch %s. Whether it changed anything there could not be told (%v), so the work tree is kept.]", wt.path, wt.branch, err), true
	}
	commits, _ := l.git(ctx, wt.path, "rev-list", "--count", wt.base+"..HEAD")
	n, _ := strconv.Atoi(commits)
	if strings.TrimSpace(status) == "" && n == 0 {
		_, rmErr := l.git(ctx, wt.repo, "worktree", "remove", "--force", wt.path)
		if rmErr == nil {
			_, _ = l.git(ctx, wt.repo, "branch", "-D", wt.branch)
			return "[It worked in its own git work tree and changed nothing there; the work tree is removed.]", false
		}
		return fmt.Sprintf("[It worked in its own git work tree, %s, and changed nothing there; the work tree could not be removed (%v).]", wt.path, rmErr), true
	}
	var what []string
	if lines := strings.Count(strings.TrimSpace(status), "\n") + 1; strings.TrimSpace(status) != "" {
		what = append(what, count(lines, "file it changed and did not commit", "files it changed and did not commit"))
	}
	if n > 0 {
		what = append(what, count(n, "commit", "commits"))
	}
	return fmt.Sprintf("[It worked in its own git work tree, %s, on the branch %s, which started at %s: it holds %s. None of it is in the folder you work in. Look at it there; merge the branch or copy what you want; then git worktree remove %s takes the work tree away.]",
		wt.path, wt.branch, shortCommit(wt.base), strings.Join(what, " and "), wt.path), true
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

// count says how many of a thing there are: "1 commit", "3 commits".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

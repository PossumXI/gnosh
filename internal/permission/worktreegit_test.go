package permission

import (
	"strings"
	"testing"
)

// treeDir is the work tree of these tests, made from the checkout /proj.
const treeDir = "/proj/.gnosh/worktrees/x"

// inProj says whether a folder is the checkout's or the work tree's.
func inProj(p string) bool { return p == "/proj" || strings.HasPrefix(p, "/proj/") }

// SharedGitChange: in a work tree of its own, git may read, or change the
// work tree's own branch, index and files in ways gnosh knows. Every line
// in the first list changes, or may change, what the work tree shares
// with the checkout; most were seen to on real git by the review of 17
// November (an allowlist replaced the list of what is refused).
func TestSharedGitChange(t *testing.T) {
	e := mustEngine(t, testOptions(ModeDefault, rules{}))
	for _, cmd := range []string{
		// Refs by name, and the subcommands that write them.
		"git update-ref refs/heads/main HEAD",
		"/usr/bin/git update-ref refs/heads/main HEAD",
		"git-update-ref refs/heads/main HEAD",
		"GIT update-ref refs/heads/main HEAD",
		"sudo git update-ref refs/heads/main HEAD",
		"sh -c 'git update-ref refs/heads/main HEAD'",
		"git symbolic-ref HEAD refs/heads/main",
		"git fast-import --force",
		"git send-pack --force . HEAD~2:refs/heads/release",
		"git replace a b",
		"git notes add -m x",
		"git reflog expire --expire=now --all",
		"git filter-branch --all",
		"git --shallow-file x update-ref refs/heads/release HEAD",
		// Branches, tags, stashes.
		"ls && git branch -D x",
		"echo x | xargs git branch -D",
		"echo -D release | xargs git branch",
		"git branch $(printf -- -D) release",
		"git branch -f main HEAD",
		"git branch -vD old",
		"git branch -m main trunk",
		"git branch --delete old",
		"git branch -u release main",
		"git branch --edit-description",
		"touch ./-D && git branch -? release",
		"git checkout -B main",
		"git checkout --ignore-other-worktrees main",
		"git switch -C main",
		"git switch --force-create main",
		"git switch --ignore-other-worktrees main",
		"git tag -d v1",
		"git tag -f v1",
		"git stash clear",
		"git stash drop",
		"git stash pop",
		"echo clear | xargs git stash",
		// The list of work trees.
		"git worktree remove ../x",
		"git worktree prune",
		"git worktree add -B main ../x",
		"git worktree $(echo remove) ../x",
		// Pushes and fetches into the repository itself.
		"git push . HEAD:main",
		"git push /repo HEAD:main",
		"git push --repo=origin . HEAD:release",
		"git push --rece git-receive-pack . HEAD:release",
		"git push --receive-pack=x origin",
		"git push $TO HEAD:main",
		"git push origin $BR",
		"git fetch origin main:main",
		"git fetch . HEAD:refs/heads/main",
		"git pull origin +main:main",
		"git fetch origin $SPEC",
		"git fetch --stdin . <<< '+main:release'",
		"git fetch --refmap=+refs/heads/main:refs/heads/release . main",
		"git branch t@x main && git fetch . +t@x:release",
		"git fetch -u --stdin . <<< x",
		"git fetch --upload-pack=x origin",
		"git fetch -f origin tag v1",
		"git fetch --prune-tags origin",
		// Commands git runs.
		"git rebase --update-refs main",
		"git rebase --exec 'git branch -D release' HEAD~1",
		"git rebase -x true HEAD~1",
		"git rebase -i HEAD~3",
		"git rebase main feature",
		"git merge -s mine feature",
		"git bisect run sh -c x",
		"git submodule foreach x",
		"EDITOR='git branch -D release' git commit --allow-empty",
		"GIT_SSH_COMMAND=x git fetch origin",
		"git grep -O'sh -c x' foo",
		"git log --output=/proj/.git/config",
		// Settings, hooks and aliases.
		"git config core.fsmonitor x",
		"git config --global alias.zz update-ref",
		"git config --global core.hooksPath /tmp/h",
		"git config --local core.hooksPath /tmp/h",
		"git config --unset core.x",
		"git config set core.x y",
		"git config -e",
		`git config user.email "$E"`,
		"git config -f /repo/.git/config core.hooksPath x",
		"git remote set-url origin x",
		"git remote add up https://example.com/x.git",
		"git init --template=/tmp/t",
		"git init",
		"git maintenance register",
		"git -c alias.x=update-ref x refs/heads/main HEAD",
		"git -c 'alias.x=!sh' x",
		"git -c remote.origin.url=. push origin HEAD:release",
		"git -c 'remote.origin.fetch=+refs/heads/*:refs/heads/*' fetch -u origin",
		"git -c rebase.updateRefs=true rebase main",
		"git -c help.autocorrect=immediate updat-ref refs/heads/release HEAD",
		"git -c include.path=/tmp/a.cfg zz",
		"git -c core.editor='git branch -D release;:' commit --allow-empty",
		"git -c core.fsmonitor=x status",
		"git br -D release",
		"git brnach -D x",
		"GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=alias.x GIT_CONFIG_VALUE_0=update-ref git x refs/heads/main HEAD",
		"export GIT_CONFI${x}G_COUNT=1; git status",
		"HOME=/tmp/h git zz refs/heads/release HEAD",
		"PATH=/tmp/evil:$PATH git status",
		// Names only known when the line runs, or by a pattern.
		"git $SUB refs/heads/main HEAD",
		"/usr/bin/gi[t] update-ref refs/heads/release HEAD",
		"git update-re[f] refs/heads/release HEAD",
	} {
		if got := e.SharedGitChange(treeDir, cmd, inProj); got == "" {
			t.Errorf("SharedGitChange(%q) = nothing, want why not", cmd)
		}
	}
	for _, cmd := range []string{
		"git status",
		"git status --short",
		"git log --oneline -5",
		"git log --graph --since=2.weeks",
		"git log --format='%h %s' | head",
		"git diff HEAD",
		"git diff --stat",
		"git show HEAD:main.go",
		"git blame x.go",
		"git rev-parse HEAD",
		"git ls-files",
		"git grep -n foo",
		"git describe --tags",
		"git help update-ref",
		"git -C sub status",
		"git add .",
		"git add -A",
		`git add -- "$f"`,
		"git add src/*.go",
		"git commit -m 'fix the bug'",
		"git commit -am 'x'",
		"git commit -m \"$(cat <<'EOF'\nfix the bug\nEOF\n)\"",
		"git commit --amend --no-edit",
		"git commit -F - <<'EOF'\nfix the bug\nEOF",
		"git -c user.name=x -c user.email=y commit -m z",
		"git reset --hard HEAD~1",
		"git restore --staged x.go",
		"git rm --cached x",
		"git mv a b",
		"git checkout -b new",
		"git checkout main.go",
		"git checkout -- a.go",
		"git checkout -f",
		"git checkout HEAD~2",
		"git switch -c new",
		"git switch main",
		"git switch --detach HEAD~1",
		"git merge feature",
		"git merge --no-ff -m x feature",
		"git rebase main",
		"git rebase --continue",
		"GIT_EDITOR=true git rebase --continue",
		"git cherry-pick abc123",
		"git revert --no-edit HEAD",
		"git stash",
		"git stash push -m wip",
		"git stash apply",
		"git stash list",
		"git stash show -p",
		"git stash show --stat",
		"git fetch origin",
		"git fetch --all --prune",
		"git fetch origin main",
		"git fetch origin +refs/heads/*:refs/remotes/origin/*",
		"git fetch --depth 1 git@github.com:x/y.git main",
		"GIT_TERMINAL_PROMPT=0 git fetch origin",
		"git pull --rebase origin main",
		"git pull",
		"git push origin HEAD",
		"git push -u origin feature",
		"git push --force origin x",
		`git push -- origin "$BR"`,
		"git branch",
		"git branch -a",
		"git branch -vv",
		"git branch new-thing",
		"git branch --sort=-committerdate",
		"git branch --format='%(refname)'",
		"git branch --show-current",
		"git tag v1",
		"git tag -a v1 -m 'release'",
		"git tag -l",
		"git clean -fd",
		"git apply x.patch",
		"git -C /tmp/other branch -D x",
		"git config user.name",
		"git config --get-regexp '^core\\.'",
		"git config --global --get user.email",
		"git config get core.editor",
		"git config -l",
		"git remote -v",
		"git remote get-url origin",
		"git reflog",
		"git reflog show main",
		"grep -r GIT_CONFIG_PARAMETERS . && git status",
		"echo update-ref",
		"$G update-ref refs/heads/main HEAD", // a name only known when it runs: GitPlaces's to refuse
	} {
		if got := e.SharedGitChange(treeDir, cmd, inProj); got != "" {
			t.Errorf("SharedGitChange(%q) = %q, want nothing", cmd, got)
		}
	}
}

// On a disk that does not tell capital letters from small ones, a new
// branch named in capitals may be one the checkout uses (MAIN is main).
func TestSharedGitChangeCase(t *testing.T) {
	was := foldsCase
	foldsCase = true
	defer func() { foldsCase = was }()
	e := mustEngine(t, testOptions(ModeDefault, rules{}))
	for cmd, refused := range map[string]bool{
		"git branch MAIN main~2":         true,
		"git switch -c RELEASE main~2":   true,
		"git checkout -b Fix-1":          true,
		"git branch fix-1":               false,
		"git switch -c release-2 main~2": false,
	} {
		if got := e.SharedGitChange(treeDir, cmd, inProj); (got != "") != refused {
			t.Errorf("SharedGitChange(%q) = %q, want refused %v", cmd, got, refused)
		}
	}
}

// SharedGitChange reads any line without failing, however its git words
// are run together or cut short.
func FuzzSharedGitChange(f *testing.F) {
	for _, s := range []string{
		"git branch -vD x", "git -c alias.x=y x", "git fetch --depth 1 o a:b",
		"git config -f x a b", "git push --repo=. x", "git-update-ref a",
		"git checkout -B", "git switch --force-create", "git worktree add -", "git tag -", "git -c",
		"git commit -m", "git -C", "git push -- origin", "GIT_EDITOR=true git rebase --continue",
	} {
		f.Add(s)
	}
	e := mustEngine(f, testOptions(ModeDefault, rules{}))
	f.Fuzz(func(t *testing.T, line string) {
		e.SharedGitChange(treeDir, line, inProj)
	})
}

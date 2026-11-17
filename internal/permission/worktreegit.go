package permission

import (
	"runtime"
	"strings"
)

// This file is what git may do in a git work tree of a session's own: read,
// or change the work tree's own branch, index and files in ways gnosh
// knows, and nothing else. A work tree shares its branches, tags and other
// refs, its git settings and hooks, and its list of work trees with the
// checkout it was made from, so git run there can move the checkout's
// branch, or put in a setting or a hook that runs a command there. git
// itself keeps a branch another work tree has out from checkout and branch
// -f, and from little else. Claude Code 2.1.326 refuses the like for its
// sub-agents in work trees.
//
// So the subcommands are an allowlist, each with the flags it may take
// (gitTreeWrites), as the read-only table is (gitRead): a subcommand, a
// flag or a setting gnosh does not know is refused, and so are the
// person's aliases, which may be any of them. What gnosh cannot see from
// the text stays out of its reach: a git copied under another name, a
// remote of the person's whose address is the checkout itself.

// SharedGitChange says why git, as the command line runs it, may change
// what a work tree shares with the checkout it was made from, as words for
// the model ("git update-ref is not one gnosh lets run in a work tree of
// your own"); "" when every git run in it only reads, or changes the work
// tree's own branch and files in a way gnosh knows. dir is the folder the
// line starts in. shared says whether a folder is in the work tree or in a
// checkout: a git run pointed only at other places ("git -C /tmp/other
// branch -D x") is no concern of this check.
func (e *Engine) SharedGitChange(dir, command string, shared func(string) bool) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	at := *e
	if strings.HasPrefix(dir, "/") {
		at.cur = cleanPath(dir)
	}
	v := at.viewBash(command)
	if why := unreadable(v); why != "" {
		return "gnosh cannot tell what git in it does: " + why
	}
	dirs, lost := at.lineDirsKnown(v)
	lost = lost || v.dirsCapped
	envWhy := gitEnvWhy(v)
	for _, u := range v.units {
		ws := u.cmd.words
		if len(ws) == 0 || !ws[0].static() {
			continue // a name only known when it runs: GitPlaces refuses those
		}
		if ws[0].glob || ws[0].brace {
			return "the program " + ws[0].raw + " is named by a pattern, which may make it git, and gnosh cannot tell what it would change"
		}
		name := progKey(baseName(ws[0].joined()))
		args := ws[1:]
		switch {
		case name == "git":
		case strings.HasPrefix(name, "git-") && len(name) > len("git-"):
			// git's own programs, run as git runs them: git-update-ref.
			args = append([]word{litOf(name[len("git-"):])}, args...)
		default:
			continue
		}
		if envWhy != "" {
			return envWhy
		}
		if why := at.treeGit(args, dirs, lost, shared); why != "" {
			return why
		}
	}
	return ""
}

// gitTreeEnv are the GIT_ variables a line may set where git runs in a
// work tree of its own.
var gitTreeEnv = map[string]bool{
	"GIT_TERMINAL_PROMPT": true, "GIT_OPTIONAL_LOCKS": true, "GIT_ADVICE": true,
	"GIT_AUTHOR_NAME": true, "GIT_AUTHOR_EMAIL": true, "GIT_AUTHOR_DATE": true,
	"GIT_COMMITTER_NAME": true, "GIT_COMMITTER_EMAIL": true, "GIT_COMMITTER_DATE": true,
	"GIT_MERGE_AUTOEDIT": true,
}

// gitTreeQuiet are the variables naming a program git runs (an editor, a
// pager) that a line may set, to a program that does nothing.
var gitTreeQuiet = map[string]bool{
	"GIT_EDITOR": true, "GIT_SEQUENCE_EDITOR": true, "EDITOR": true, "VISUAL": true,
	"GIT_PAGER": true, "PAGER": true,
}

// quietPrograms are the programs gitTreeQuiet's variables may name.
var quietPrograms = map[string]bool{"true": true, ":": true, "cat": true}

// gitEnvWhy says why the variables the line sets keep gnosh from telling
// what its git does, or "": settings given through the environment
// (GIT_CONFIG_COUNT and its kin; another HOME or XDG_CONFIG_HOME, whose
// settings may name aliases), a GIT_ variable gnosh does not know, a
// program git runs, another PATH, which finds another git, or a variable
// set by a name only known when the line runs.
func gitEnvWhy(v *bashView) string {
	check := func(name, value string, known bool) string {
		switch {
		case name == "HOME" || name == "XDG_CONFIG_HOME" || name == "PATH":
			return "the line sets " + name + ", which changes the git that runs or the settings it reads"
		case gitTreeQuiet[name]:
			if !known || !quietPrograms[strings.TrimSpace(value)] {
				return "the line sets " + name + ", a program git runs: in a work tree of your own it may only be true, : or cat"
			}
		case strings.HasPrefix(name, "GIT_") && !gitTreeEnv[name]:
			return "the line sets " + name + ", which changes what git does in ways gnosh does not follow in a work tree of your own"
		}
		return ""
	}
	for _, u := range v.units {
		for _, a := range u.cmd.assigns {
			_, value, _ := strings.Cut(a.w.joined(), "=")
			if why := check(a.name, value, a.w.static()); why != "" {
				return why
			}
		}
		ws := u.cmd.words
		if len(ws) == 0 || !ws[0].static() {
			continue
		}
		switch progKey(baseName(ws[0].joined())) {
		case "export", "declare", "typeset", "local", "readonly", "env":
			for _, w := range ws[1:] {
				if !w.static() {
					eq := strings.IndexByte(w.raw, '=')
					if eq < 0 || strings.ContainsAny(w.raw[:eq], "$`\\'\"") {
						return "the line sets a variable whose name is only known when it runs (" + w.raw + "), which may be one of git's"
					}
					if why := check(w.raw[:eq], "", false); why != "" {
						return why
					}
					continue
				}
				name, value, hasVal := strings.Cut(w.joined(), "=")
				if !hasVal && (name == "" || strings.HasPrefix(name, "-")) {
					continue
				}
				if why := check(name, value, true); why != "" {
					return why
				}
			}
		case "read", "mapfile", "readarray", "printf":
			for _, w := range ws[1:] {
				if w.static() {
					if why := check(w.joined(), "", false); why != "" {
						return why
					}
				}
			}
		}
	}
	return ""
}

// gitTreeGlobal are the flags git may be given before its subcommand in a
// work tree of its own; the ones that point it elsewhere (-C, --git-dir,
// --work-tree) are read where they stand.
var gitTreeGlobal = map[string]bool{
	"--no-pager": true, "-P": true, "--no-optional-locks": true, "--no-advice": true,
	"-p": true, "--paginate": true, "--literal-pathspecs": true, "--glob-pathspecs": true,
	"--noglob-pathspecs": true, "--icase-pathspecs": true,
}

// gitTreeSettings are the settings -c may give git in a work tree of its
// own: a key, or a section that ends in a dot. Others may run a command,
// name an alias, or point git at another repository or remote.
var gitTreeSettings = []string{
	"user.name", "user.email", "color.", "advice.", "core.quotepath", "commit.gpgsign",
	"tag.gpgsign", "push.default", "pull.rebase", "pull.ff", "merge.ff", "init.defaultbranch",
	"log.", "diff.renames", "diff.noprefix", "status.", "column.", "gc.auto",
}

// treeGit is SharedGitChange for one git run, given its words after the
// program's name and the folders the line may be in (lost: not all known).
func (e *Engine) treeGit(args []word, dirs []string, lost bool, shared func(string) bool) string {
	var places []string
	placeLost := false
	place := func(w word) {
		if !w.static() {
			placeLost = true
			return
		}
		p := w.value(e.home)
		if strings.HasPrefix(p, "/") {
			places = append(places, cleanPath(p))
			return
		}
		if lost {
			placeLost = true
			return
		}
		for _, d := range dirs {
			places = append(places, cleanPath(d+"/"+p))
		}
	}
	i := 0
words:
	for ; i < len(args); i++ {
		a := args[i]
		if !a.static() || a.glob || a.brace {
			return "a word git reads before its subcommand (" + a.raw + ") is only known when the line runs, so gnosh cannot tell what it changes"
		}
		l := a.joined()
		switch {
		case gitTreeGlobal[l]:
		case l == "-C" || l == "--git-dir" || l == "--work-tree":
			if i++; i < len(args) {
				place(args[i])
			}
		case strings.HasPrefix(l, "--git-dir=") || strings.HasPrefix(l, "--work-tree="):
			_, p, _ := strings.Cut(l, "=")
			place(litOf(p))
		case l == "-c":
			if i++; i >= len(args) {
				break words
			}
			if why := treeSetting(args[i]); why != "" {
				return why
			}
		case strings.HasPrefix(l, "-"):
			return "git's option " + l + ", before its subcommand, is not one gnosh lets git take in a work tree of your own"
		default:
			break words
		}
	}
	if i >= len(args) {
		return ""
	}
	// Where git runs: a run pointed only at places outside the work tree
	// and the checkouts is not this check's.
	if len(places) == 0 && !placeLost {
		places = dirs
		placeLost = lost
	}
	if !placeLost && len(places) > 0 {
		inShared := false
		for _, p := range places {
			inShared = inShared || shared(p)
		}
		if !inShared {
			return ""
		}
	}
	subw := args[i]
	if !subw.static() || subw.glob || subw.brace {
		return "git's subcommand (" + subw.raw + ") is only known when the line runs, so gnosh cannot tell what it changes"
	}
	sub := subw.joined()
	rest := args[i+1:]
	// What only reads is fine, by the read-only table, or by being a
	// command that reads with no flag that writes a file or starts a
	// program.
	var f cmdFacts
	gitFacts(&f, args[i:])
	if f.readOnly || gitReads[sub] && readsOnly(rest) {
		return ""
	}
	if treeReads(sub, rest) {
		return ""
	}
	spec, ok := gitTreeWrites[sub]
	if !ok {
		return "git " + sub + " is not one gnosh lets run in a work tree of your own, where git may only read or change your own branch, index and files (add, commit, reset, restore, rm, mv, checkout, switch, merge, rebase, cherry-pick, revert, stash push and apply, fetch, pull, push to a remote, branch and tag to make one, clean, apply); an alias may stand for any command, so write the command out"
	}
	ops, flags, bad := treeArgs(rest, spec.args)
	if bad != "" {
		return "git " + sub + ": " + bad
	}
	if spec.check != nil {
		return spec.check(sub, ops, flags)
	}
	return ""
}

// treeSetting says why a setting given with -c is refused in a work tree
// of its own, or "".
func treeSetting(w word) string {
	text := w.raw
	if w.static() {
		text = w.joined()
	}
	key, _, _ := strings.Cut(text, "=")
	if !w.static() && strings.ContainsAny(key, "$`\\'\"") {
		return "a setting git is given with -c (" + w.raw + ") has a name only known when the line runs"
	}
	key = strings.ToLower(key)
	for _, ok := range gitTreeSettings {
		if key == ok || strings.HasSuffix(ok, ".") && strings.HasPrefix(key, ok) {
			return ""
		}
	}
	return "git -c " + key + " is not a setting gnosh lets git take in a work tree of your own (a setting may run a command, name an alias or point git elsewhere)"
}

// gitReads are the subcommands that only read, whatever flags they take,
// but for those readsOnly finds.
var gitReads = map[string]bool{
	"status": true, "diff": true, "log": true, "show": true, "grep": true, "blame": true,
	"annotate": true, "rev-parse": true, "rev-list": true, "ls-files": true, "ls-tree": true,
	"describe": true, "shortlog": true, "cat-file": true, "show-ref": true, "for-each-ref": true,
	"merge-base": true, "name-rev": true, "whatchanged": true, "count-objects": true,
	"check-ignore": true, "check-attr": true, "var": true, "version": true, "diff-tree": true,
	"diff-index": true, "diff-files": true, "range-diff": true, "cherry": true, "show-branch": true,
	"verify-commit": true, "verify-tag": true, "help": true,
}

// readsOnly reports whether the words of a reading subcommand hold no flag
// that writes a file or starts a program (--output, git grep's -O,
// --ext-diff, --textconv), and nothing only known when the line runs that
// could be one.
func readsOnly(rest []word) bool {
	for _, w := range rest {
		if !w.static() {
			return false
		}
		l := w.joined()
		if l == "--" {
			return true
		}
		for _, p := range []string{"--output", "--open-files-in-pager", "--ext-diff", "--textconv", "-O", "--exec"} {
			if strings.HasPrefix(l, p) {
				return false
			}
		}
	}
	return true
}

// treeReads reports whether a subcommand that may also write only reads,
// given rest: git stash show and list, git reflog showing, git config
// reading a setting or listing them, git remote listing or showing.
func treeReads(sub string, rest []word) bool {
	first := ""
	if len(rest) > 0 && rest[0].static() {
		first = rest[0].joined()
	}
	switch sub {
	case "stash":
		// (They take the flags of git log and git diff.)
		return (first == "show" || first == "list") && readsOnly(rest[1:])
	case "reflog":
		return first != "expire" && first != "delete" && first != "drop" && readsOnly(rest)
	case "remote":
		for _, w := range rest {
			if !w.static() {
				return false
			}
		}
		switch first {
		case "", "-v", "--verbose":
			return len(rest) <= 1
		case "show", "get-url":
			return true
		}
		return false
	case "config":
		return configReads(rest)
	}
	return false
}

// configReads reports whether git config, given rest, only reads a setting
// or lists them: an action that reads (--get, --get-regexp, --list, get,
// list), or a key alone.
func configReads(rest []word) bool {
	var ops []string
	reads := false
	for i := 0; i < len(rest); i++ {
		w := rest[i]
		if !w.static() {
			return false
		}
		l := w.joined()
		switch {
		case l == "-l" || l == "--list" || strings.HasPrefix(l, "--get"):
			reads = true
		case l == "-f" || l == "--file" || l == "--blob" || l == "--type" || l == "--default":
			i++ // its value
		case strings.HasPrefix(l, "--file=") || strings.HasPrefix(l, "--blob=") || strings.HasPrefix(l, "--type=") ||
			strings.HasPrefix(l, "--default=") || strings.HasPrefix(l, "--url="):
		case l == "--global" || l == "--system" || l == "--local" || l == "--worktree" || l == "--show-origin" ||
			l == "--show-scope" || l == "--name-only" || l == "-z" || l == "--null" || l == "--bool" || l == "--int" ||
			l == "--bool-or-int" || l == "--path" || l == "--expiry-date" || l == "--includes" || l == "--no-includes" ||
			l == "--all" || l == "--fixed-value" || l == "--regexp":
		case strings.HasPrefix(l, "-"):
			return false
		default:
			ops = append(ops, l)
		}
	}
	switch {
	case reads:
		return true
	case len(ops) > 0 && (ops[0] == "get" || ops[0] == "list"):
		return true
	}
	return len(ops) == 1
}

// treeWrite is a subcommand git may run in a work tree of its own: the
// flags it may take, written as the read-only table writes them, and a
// check of what it was given.
type treeWrite struct {
	args  roSpec
	check func(sub string, ops []word, flags map[string]string) string
}

// strategies are the merge strategies git has itself: -s names a program
// git-merge-<name>, which may be anything.
var strategies = map[string]bool{"ort": true, "recursive": true, "resolve": true, "octopus": true, "ours": true, "subtree": true}

// gitTreeWrites are the subcommands that may change the work tree's own
// branch, index and files, with the flags they may take. Left out: those
// that start a program (rebase -x, bisect run, an editor), write a ref by
// name, reset a branch or take one another work tree has out (branch -f,
// checkout -B), drop stashes, or write settings (config, remote, branch -u).
var gitTreeWrites = map[string]treeWrite{
	"add": {args: roSpec{short: "AuNfvn",
		long: "--all --update --intent-to-add --force --verbose --dry-run --ignore-errors --no-all --ignore-removal --renormalize --sparse --chmod= --pathspec-from-file="}},
	"commit": {args: roSpec{noGlob: true, short: "aqvsnoiz", valued: "mFC", optional: "uS",
		long: "--all --quiet --verbose --amend --no-edit --allow-empty --allow-empty-message --signoff --no-signoff --no-verify --verify --only --include --no-gpg-sign --reset-author --no-post-rewrite --dry-run --short --branch --porcelain --long --null --status --no-status --untracked-files --untracked-files= --gpg-sign --gpg-sign= --message= --file= --author= --date= --fixup= --squash= --cleanup= --trailer= --reuse-message= --pathspec-from-file="}},
	"reset": {args: roSpec{noGlob: true, short: "qN",
		long: "--soft --mixed --hard --keep --merge --quiet --intent-to-add --no-refresh --refresh --pathspec-from-file="}},
	"restore": {args: roSpec{short: "SWqm", valued: "s",
		long: "--staged --worktree --quiet --ours --theirs --merge --overlay --no-overlay --ignore-unmerged --progress --no-progress --source= --conflict= --pathspec-from-file="}},
	"rm": {args: roSpec{short: "rfqn",
		long: "--force --cached --quiet --dry-run --ignore-unmatch --sparse --pathspec-from-file="}},
	"mv": {args: roSpec{noGlob: true, short: "fknv", long: "--force --dry-run --verbose --sparse"}},
	"checkout": {args: roSpec{noGlob: true, short: "qfm", valued: "b", optional: "t",
		long: "--quiet --force --detach --ours --theirs --merge --no-track --track --track= --overlay --no-overlay --ignore-skip-worktree-bits --progress --no-progress --guess --no-guess --orphan= --conflict= --pathspec-from-file="},
		check: newBranch("-b", "--orphan")},
	"switch": {args: roSpec{noGlob: true, short: "dqfm", valued: "c", optional: "t",
		long: "--detach --quiet --force --discard-changes --merge --no-track --track --track= --guess --no-guess --progress --no-progress --create= --orphan= --conflict="},
		check: newBranch("-c", "--create", "--orphan")},
	"merge": {args: roSpec{noGlob: true, short: "qvn", valued: "mFsX", optional: "S",
		long: "--no-ff --ff --ff-only --squash --no-squash --no-edit --abort --continue --quit --quiet --verbose --stat --no-stat --no-commit --commit --allow-unrelated-histories --no-verify --verify --autostash --no-autostash --signoff --no-signoff --log --log= --no-log --rerere-autoupdate --no-rerere-autoupdate --progress --no-progress --no-gpg-sign --gpg-sign --gpg-sign= --file= --strategy= --strategy-option= --cleanup= --into-name="},
		check: knownStrategy},
	"rebase": {args: roSpec{noGlob: true, short: "qvmnfr", valued: "sXC", optional: "S",
		long: "--continue --abort --skip --quit --quiet --verbose --keep-base --root --merge --autostash --no-autostash --autosquash --no-autosquash --fork-point --no-fork-point --committer-date-is-author-date --ignore-date --reset-author-date --signoff --no-verify --verify --no-update-refs --stat --no-stat --keep-empty --no-keep-empty --reapply-cherry-picks --no-reapply-cherry-picks --apply --rerere-autoupdate --no-rerere-autoupdate --no-gpg-sign --gpg-sign --gpg-sign= --ignore-whitespace --force-rebase --no-ff --rebase-merges --rebase-merges= --onto= --strategy= --strategy-option= --empty= --whitespace="},
		check: func(sub string, ops []word, flags map[string]string) string {
			if len(ops) > 1 {
				return "git rebase given a branch to rebase checks that branch out and rewrites it, and it may be one the checkout uses: rebase your own branch, which is out"
			}
			return knownStrategy(sub, ops, flags)
		}},
	"cherry-pick": {args: roSpec{noGlob: true, short: "nxs", valued: "mX", optional: "S",
		long: "--no-commit --signoff --ff --allow-empty --allow-empty-message --keep-redundant-commits --continue --abort --skip --quit --rerere-autoupdate --no-rerere-autoupdate --no-gpg-sign --gpg-sign --gpg-sign= --mainline= --strategy= --strategy-option= --empty="},
		check: knownStrategy},
	"revert": {args: roSpec{noGlob: true, short: "ns", valued: "mX", optional: "S",
		long: "--no-edit --no-commit --signoff --continue --abort --skip --quit --rerere-autoupdate --no-rerere-autoupdate --no-gpg-sign --gpg-sign --gpg-sign= --reference --mainline= --strategy= --strategy-option="},
		check: knownStrategy},
	"stash": {args: roSpec{noGlob: true, short: "uakqSp", valued: "m",
		long: "--include-untracked --no-include-untracked --all --keep-index --no-keep-index --quiet --staged --index --message= --pathspec-from-file="},
		check: func(_ string, ops []word, _ map[string]string) string {
			if len(ops) == 0 {
				return ""
			}
			switch ops[0].joined() {
			case "push", "save", "apply", "list", "show":
				return ""
			}
			return "git stash " + ops[0].raw + " changes the list of stashes, which the checkout keeps too: git stash apply takes one back and leaves the list as it is"
		}},
	"fetch": {args: roSpec{noGlob: true, short: "pqv46k", valued: "jo",
		long: "--all --prune --quiet --verbose --tags --no-tags --dry-run --multiple --unshallow --no-recurse-submodules --recurse-submodules --recurse-submodules= --progress --no-progress --ipv4 --ipv6 --atomic --no-write-fetch-head --write-fetch-head --auto-maintenance --no-auto-maintenance --auto-gc --no-auto-gc --keep --show-forced-updates --no-show-forced-updates --set-upstream --depth= --deepen= --shallow-since= --shallow-exclude= --jobs= --negotiation-tip= --server-option= --filter="},
		check: fetchRefspecs},
	"pull": {args: roSpec{noGlob: true, short: "pqv46n", valued: "jsX",
		long: "--all --prune --quiet --verbose --tags --no-tags --dry-run --unshallow --no-recurse-submodules --progress --no-progress --ipv4 --ipv6 --rebase --rebase= --no-rebase --ff --no-ff --ff-only --squash --no-squash --commit --no-commit --no-edit --autostash --no-autostash --stat --no-stat --signoff --no-signoff --no-verify --verify --allow-unrelated-histories --log --no-log --depth= --deepen= --shallow-since= --jobs= --strategy= --strategy-option= --server-option="},
		check: func(sub string, ops []word, flags map[string]string) string {
			if why := knownStrategy(sub, ops, flags); why != "" {
				return why
			}
			return fetchRefspecs(sub, ops, flags)
		}},
	"push": {args: roSpec{noGlob: true, short: "ufnqvd46", valued: "o",
		long: "--set-upstream --force --dry-run --quiet --verbose --porcelain --delete --tags --follow-tags --no-follow-tags --no-verify --verify --atomic --no-atomic --all --branches --mirror --prune --progress --no-progress --thin --no-thin --force-with-lease --force-with-lease= --no-force-with-lease --force-if-includes --no-force-if-includes --signed --signed= --no-signed --ipv4 --ipv6 --recurse-submodules= --push-option="},
		check: func(_ string, ops []word, _ map[string]string) string {
			if len(ops) > 0 && (!ops[0].static() || localRepo(ops[0].joined())) {
				return "git push into " + ops[0].raw + " may push into a repository on this machine, the one your work tree shares among them: push to a remote by its name, written out"
			}
			return ""
		}},
	"branch": {args: roSpec{noGlob: true, short: "arlvqi", optional: "t",
		long: "--all --remotes --list --verbose --quiet --show-current --ignore-case --no-color --color --color= --no-column --column --column= --no-track --track --track= --abbrev --abbrev= --omit-empty --contains= --no-contains= --merged= --no-merged= --points-at= --sort= --format="},
		check: func(_ string, ops []word, flags map[string]string) string {
			for _, f := range []string{"-a", "-r", "-l", "-v", "--all", "--remotes", "--list", "--verbose", "--show-current", "--contains", "--no-contains", "--merged", "--no-merged", "--points-at"} {
				if _, ok := flags[f]; ok {
					return ""
				}
			}
			if len(ops) == 0 {
				return ""
			}
			return branchName(ops[0])
		}},
	"tag": {args: roSpec{noGlob: true, short: "aslvi", valued: "mFu", optional: "n",
		long: "--annotate --sign --list --verify --ignore-case --no-column --column --column= --color --color= --create-reflog --no-sign --message= --file= --local-user= --contains= --no-contains= --merged= --no-merged= --points-at= --sort= --format= --cleanup= --trailer="}},
	"clean": {args: roSpec{noGlob: true, short: "nfdxXq", valued: "e", long: "--dry-run --force --quiet --exclude="}},
	"apply": {args: roSpec{noGlob: true, short: "Rv3qN", optional: "pC",
		long: "--check --stat --numstat --summary --index --cached --reverse --3way --reject --verbose --quiet --recount --allow-empty --no-add --binary --unidiff-zero --intent-to-add --whitespace= --exclude= --include= --directory="}},
}

// knownStrategy refuses a merge strategy git does not have itself: -s
// names a program, git-merge-<name>.
func knownStrategy(sub string, _ []word, flags map[string]string) string {
	for _, f := range []string{"-s", "--strategy"} {
		if v, ok := flags[f]; ok && !strategies[v] {
			return "git " + sub + "'s strategy " + v + " is not one of git's own (ort, recursive, resolve, octopus, ours, subtree): another names a program"
		}
	}
	return ""
}

// newBranch is the check of a subcommand whose flags make a branch: the
// name each gives (branchName).
func newBranch(names ...string) func(string, []word, map[string]string) string {
	return func(_ string, _ []word, flags map[string]string) string {
		for _, f := range names {
			if v, ok := flags[f]; ok {
				if why := branchName(litOf(v)); why != "" {
					return why
				}
			}
		}
		return ""
	}
}

// foldsCase says this machine's disks, as a Mac's and a Windows machine's
// are by default, do not tell capital letters from small ones.
var foldsCase = runtime.GOOS == "darwin" || runtime.GOOS == "windows"

// branchName refuses a new branch's name that could be an old one's: on a
// disk that does not tell capital letters from small ones, as a Mac's and
// a Windows machine's, MAIN is main.
func branchName(w word) string {
	if foldsCase && strings.ToLower(w.lit()) != w.lit() {
		return "the new branch " + w.lit() + " has capital letters, and on this machine's disk, which does not tell them from small ones, it may be a branch the checkout uses: name it in small letters"
	}
	return ""
}

// fetchRefspecs is the check of fetch and pull: a refspec whose
// destination is a branch or a tag ("main:main", "+x:refs/heads/x") moves
// it, where one under refs/remotes/ does not.
func fetchRefspecs(sub string, ops []word, flags map[string]string) string {
	if _, ok := flags["--multiple"]; ok || len(ops) < 2 {
		return "" // a remote alone, or every word a remote
	}
	if _, ok := flags["--all"]; ok {
		return ""
	}
	for _, w := range ops[1:] {
		if !w.static() {
			return "git " + sub + " has a refspec only known when the line runs (" + w.raw + "), which may write a branch by name"
		}
		spec := w.joined()
		_, dst, found := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		if found && dst != "" && !strings.HasPrefix(dst, "refs/remotes/") {
			return "git " + sub + " with " + spec + " writes " + dst + ", a branch or another ref, by name (fetch without a destination and use FETCH_HEAD, or into refs/remotes/)"
		}
	}
	return ""
}

// localRepo reports whether a repository as git push is given it is a
// folder on this machine: ".", a path, or a file:// address.
func localRepo(repo string) bool {
	return repo == "" || strings.HasPrefix(repo, ".") || strings.HasPrefix(repo, "/") ||
		strings.HasPrefix(repo, "~") || strings.HasPrefix(repo, "file:")
}

// treeArgs reads a writing subcommand's words by its spec, as git's option
// parser does, and strictly: a flag the spec does not hold is refused, by
// its exact spelling (git takes the start of a long flag too, and a start
// is refused here). A word only known when the line runs may be the value
// of a flag that takes one (-m "$MSG") or come after "--"; anywhere else
// git would take it for an option if it began with a dash, and so it
// would a file an unquoted pattern matched. flags holds each flag given,
// by the name it was given under, with its value ("" for none).
func treeArgs(args []word, spec roSpec) (ops []word, flags map[string]string, bad string) {
	flags = map[string]string{}
	long := " " + spec.long + " "
	rest := false
	for i := 0; i < len(args); i++ {
		w := args[i]
		if rest {
			ops = append(ops, w)
			continue
		}
		if !w.static() || w.brace {
			return nil, nil, "a word only known when the line runs (" + w.raw + ") would be taken for an option if it began with a dash: put -- before it, or write it out"
		}
		s := w.joined()
		switch {
		case s == "--":
			rest = true
		case w.glob && !w.quoted && spec.noGlob && strings.ContainsRune("*?[", rune(s[0])):
			// (A pattern that begins with plain text matches only names
			// that begin so: +refs/heads/* cannot be a flag.)
			return nil, nil, "the pattern " + w.raw + " may match a file whose name begins with a dash, which git would take for an option: put -- before it"
		case s == "-" || !strings.HasPrefix(s, "-"):
			ops = append(ops, w)
		case strings.HasPrefix(s, "--"):
			name, val, hasVal := strings.Cut(s, "=")
			switch {
			case hasVal && strings.Contains(long, " "+name+"= "):
				flags[name] = val
			case !hasVal && strings.Contains(long, " "+s+" "):
				flags[name] = ""
			case !hasVal && strings.Contains(long, " "+s+"= "):
				// The value is the next word, whatever it holds.
				if i++; i >= len(args) {
					return nil, nil, "the option " + s + " has no value"
				}
				flags[name] = args[i].lit()
			default:
				return nil, nil, "its option " + name + " is not one gnosh lets git take in a work tree of your own (spell options out whole)"
			}
		default:
			for j := 1; j < len(s); j++ {
				c := s[j]
				flag := "-" + string(c)
				switch {
				case strings.IndexByte(spec.optional, c) >= 0:
					flags[flag] = s[j+1:]
					j = len(s)
				case strings.IndexByte(spec.valued, c) >= 0:
					switch {
					case j+1 < len(s):
						flags[flag] = s[j+1:]
					case i+1 < len(args):
						i++
						flags[flag] = args[i].lit()
					default:
						return nil, nil, "the option " + flag + " has no value"
					}
					j = len(s)
				case strings.IndexByte(spec.short, c) >= 0:
					flags[flag] = ""
				default:
					return nil, nil, "its option " + flag + " is not one gnosh lets git take in a work tree of your own"
				}
			}
		}
	}
	return ops, flags, ""
}

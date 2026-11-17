package permission

import "strings"

// This file is what git may not do in a git work tree of a session's own:
// change what the work tree shares with the checkout it was made from.
// The two share their branches, tags and other refs, their git settings
// and their list of work trees, so git run in the work tree can move the
// checkout's branch, rename it, or put in a setting that runs a command
// there (core.fsmonitor, core.hooksPath). git keeps a branch another work
// tree has out from checkout, switch, branch -f and a fetch into it, but
// not from update-ref, symbolic-ref, branch -m, a push into the
// repository itself or --ignore-other-worktrees. Claude Code 2.1.326
// refuses the like for its sub-agents in work trees.

// SharedGitChange says what git, as the command line runs it, would change
// that a work tree shares with the checkout it was made from, as words for
// the model ("git update-ref writes a branch or another ref by name"); ""
// when the line's git leaves all of that alone as far as its text tells.
// A git run with a word that is only known when the line runs is reported
// too, since the word may make it any of these.
func (e *Engine) SharedGitChange(command string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	v := e.viewBash(command)
	if why := unreadable(v); why != "" {
		return "gnosh cannot tell what git in it does: " + why
	}
	envSettings := setsGitConfig(v)
	for _, u := range v.units {
		ws := u.cmd.words
		if len(ws) == 0 || !ws[0].static() {
			continue // a name only known when it runs: GitPlaces refuses those
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
		if envSettings {
			return "the line gives git settings through its environment (GIT_CONFIG_...), where an alias can make a git command any other"
		}
		if what := gitShares(args); what != "" {
			return what
		}
	}
	return ""
}

// setsGitConfig reports whether the line may give git settings through the
// environment: GIT_CONFIG_COUNT with its keys and values,
// GIT_CONFIG_PARAMETERS, or another file of them (GIT_CONFIG_GLOBAL).
func setsGitConfig(v *bashView) bool {
	for _, u := range v.units {
		for _, a := range u.cmd.assigns {
			if strings.HasPrefix(a.name, "GIT_CONFIG") {
				return true
			}
		}
		for _, w := range u.cmd.words {
			if strings.HasPrefix(w.lit(), "GIT_CONFIG") {
				return true
			}
		}
	}
	return false
}

// notKnown stands for a word only known when the line runs, among the
// words a check reads: never a flag, and naming nothing.
const notKnown = "\x00"

// gitGlobalValued are the flags git takes before its subcommand that take
// the next word for their value.
var gitGlobalValued = map[string]bool{
	"-C": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
	"--super-prefix": true, "--attr-source": true,
}

// gitShares is SharedGitChange for one git run, given its words after the
// program's name.
func gitShares(args []word) string {
	i := 0
words:
	for ; i < len(args); i++ {
		a := args[i]
		if !a.static() {
			return "a word git reads before its subcommand is only known when the line runs, so gnosh cannot tell what it changes"
		}
		l := a.joined()
		switch {
		case l == "-c" || l == "--config-env":
			if i++; i < len(args) {
				if what := gitSetting(args[i]); what != "" {
					return what
				}
			}
		case strings.HasPrefix(l, "--config-env="):
			if what := gitSetting(litOf(strings.TrimPrefix(l, "--config-env="))); what != "" {
				return what
			}
		case gitGlobalValued[l]:
			i++
		case strings.HasPrefix(l, "-"):
		default:
			break words
		}
	}
	if i >= len(args) {
		return ""
	}
	sub := args[i].joined()
	check, watched := gitSharedChecks[sub]
	if !watched {
		return ""
	}
	// A word only known when the line runs is read as an operand: what a
	// check can only tell by an operand ("git push $TO") it reports as not
	// known. One made to be a flag ("git branch $FLAG x") is taken for none.
	rest := make([]string, 0, len(args)-i-1)
	for _, w := range args[i+1:] {
		if w.static() {
			rest = append(rest, w.joined())
		} else {
			rest = append(rest, notKnown)
		}
	}
	return check(sub, rest)
}

// gitSetting says what a setting given to git with -c (or --config-env)
// does to what a work tree shares, or "": an alias, which makes the
// subcommand anything.
func gitSetting(w word) string {
	text := w.raw
	if w.static() {
		text = w.joined()
	}
	key, _, _ := strings.Cut(text, "=")
	if !w.static() && strings.ContainsAny(key, "$`\\\"'") {
		return "a setting git is given with -c has a name only known when the line runs, so gnosh cannot tell what it changes"
	}
	if strings.HasPrefix(strings.ToLower(key), "alias.") {
		return "git -c defines an alias, which can run any git command"
	}
	return ""
}

// gitFlag is one flag of a git subcommand as the subcommand reads it: its
// name ("-f", "--force") and its value, when it takes one.
type gitFlag struct {
	name, value string
}

// splitGit splits a git subcommand's words into its flags and its
// operands, the way git's option parser does. valued holds, between
// spaces, the flags that take a value, which is the next word when it is
// not attached ("-u origin/x", "--sort -committerdate"); short flags may
// be run together ("-vu origin/x", "-am msg"); words after "--" are all
// operands.
func splitGit(rest []string, valued string) (flags []gitFlag, ops []string) {
	takes := func(name string) bool { return strings.Contains(valued, " "+name+" ") }
	for i := 0; i < len(rest); i++ {
		l := rest[i]
		switch {
		case l == "--":
			return flags, append(ops, rest[i+1:]...)
		case strings.HasPrefix(l, "--"):
			name, val, attached := strings.Cut(l, "=")
			if !attached && takes(name) && i+1 < len(rest) {
				i++
				val = rest[i]
			}
			flags = append(flags, gitFlag{name, val})
		case strings.HasPrefix(l, "-") && len(l) > 1:
			for k := 1; k < len(l); k++ {
				name := "-" + l[k:k+1]
				if !takes(name) {
					flags = append(flags, gitFlag{name: name})
					continue
				}
				val := l[k+1:]
				if val == "" && i+1 < len(rest) {
					i++
					val = rest[i]
				}
				flags = append(flags, gitFlag{name, val})
				break
			}
		default:
			ops = append(ops, l)
		}
	}
	return flags, ops
}

// longIs reports whether a long flag as written is full, or a start of it
// that git would take for it: git takes any start of a long flag that only
// one of its flags begins with, and a start more than one begins with is
// an error, which does no harm to take for full. A flag that is another
// flag of the same subcommand whole is that one ("--force" is not the
// start of switch's "--force-create").
func longIs(name, full string, others ...string) bool {
	if name == full {
		return true
	}
	for _, o := range others {
		if name == o {
			return false
		}
	}
	return len(name) > len("--") && strings.HasPrefix(full, name)
}

// gitSharedChecks are the git subcommands that can change what a work tree
// shares with its checkout, each with the check that says whether the
// words it is given do, and what.
var gitSharedChecks = map[string]func(sub string, rest []string) string{
	"update-ref": func(string, []string) string {
		return "git update-ref writes a branch or another ref by name"
	},
	"symbolic-ref": func(_ string, rest []string) string {
		flags, ops := splitGit(rest, " -m ")
		for _, f := range flags {
			if f.name == "-d" || longIs(f.name, "--delete") {
				return "git symbolic-ref " + f.name + " deletes a ref by name"
			}
		}
		if len(ops) >= 2 {
			return "git symbolic-ref sets a ref by name"
		}
		return ""
	},
	"branch": func(_ string, rest []string) string {
		flags, _ := splitGit(rest, " -u --set-upstream-to --contains --no-contains --merged --no-merged --points-at --format --sort ")
		for _, f := range flags {
			switch {
			case len(f.name) == 2 && strings.Contains("fdDmMcC", f.name[1:]),
				longIs(f.name, "--force", "--format"), longIs(f.name, "--delete"),
				longIs(f.name, "--move"), longIs(f.name, "--copy", "--color", "--column", "--contains"):
				return "git branch " + f.name + " moves, renames, copies over or deletes a branch by name"
			}
		}
		return ""
	},
	"checkout": func(_ string, rest []string) string {
		flags, _ := splitGit(rest, " -b -B --orphan --conflict --pathspec-from-file ")
		for _, f := range flags {
			switch {
			case f.name == "-B":
				return "git checkout -B resets the branch it names when there is one (-b makes a new branch, and stops if there is one)"
			case longIs(f.name, "--ignore-other-worktrees", "--ignore-skip-worktree-bits"):
				return "git checkout --ignore-other-worktrees takes a branch that another work tree has out"
			}
		}
		return ""
	},
	"switch": func(_ string, rest []string) string {
		flags, _ := splitGit(rest, " -c -C --create --force-create --orphan --conflict ")
		for _, f := range flags {
			switch {
			case f.name == "-C" || longIs(f.name, "--force-create", "--force"):
				return "git switch " + f.name + " resets the branch it names when there is one (-c makes a new branch, and stops if there is one)"
			case longIs(f.name, "--ignore-other-worktrees"):
				return "git switch --ignore-other-worktrees takes a branch that another work tree has out"
			}
		}
		return ""
	},
	"worktree": func(_ string, rest []string) string {
		flags, ops := splitGit(rest, " -b -B --reason ")
		if len(ops) == 0 {
			return ""
		}
		switch ops[0] {
		case "remove", "move", "prune":
			return "git worktree " + ops[0] + " changes the repository's list of work trees, which the checkout keeps"
		case "add":
			for _, f := range flags {
				if f.name == "-B" || f.name == "-f" || longIs(f.name, "--force") {
					return "git worktree add " + f.name + " resets a branch by name, or takes one that another work tree has out"
				}
			}
		}
		return ""
	},
	"push": func(_ string, rest []string) string {
		flags, ops := splitGit(rest, " --repo -o --push-option --receive-pack --exec ")
		repo := ""
		if len(ops) > 0 {
			repo = ops[0]
		}
		for _, f := range flags {
			if longIs(f.name, "--repo") {
				repo = f.value
			}
		}
		if repo == notKnown {
			return "git push to a place only known when the line runs may push into the repository your work tree shares"
		}
		if localRepo(repo) {
			return "git push into " + repo + " moves branches of a repository on this machine, which may be the one your work tree shares (push to a remote by its name)"
		}
		return ""
	},
	"fetch": gitFetchShares,
	"pull":  gitFetchShares,
	"rebase": func(_ string, rest []string) string {
		flags, _ := splitGit(rest, " --onto -s --strategy -X --strategy-option -x --exec ")
		for _, f := range flags {
			if longIs(f.name, "--update-refs") {
				return "git rebase --update-refs moves other branches that point into what it rebases"
			}
		}
		return ""
	},
	"config": func(_ string, rest []string) string {
		flags, ops := splitGit(rest, " -f --file --blob --type --default --comment --value ")
		write := false
		for _, f := range flags {
			switch {
			case longIs(f.name, "--global") || longIs(f.name, "--system"):
				return "" // not the repository's: the rules judge it
			case f.name == "-e" || longIs(f.name, "--edit") || longIs(f.name, "--add") ||
				longIs(f.name, "--replace-all") || longIs(f.name, "--unset") || longIs(f.name, "--unset-all") ||
				longIs(f.name, "--rename-section") || longIs(f.name, "--remove-section"):
				write = true
			case f.name == "-l" || longIs(f.name, "--list") || strings.HasPrefix(f.name, "--get"):
				return ""
			}
		}
		if len(ops) > 0 {
			switch ops[0] {
			case "set", "unset", "rename-section", "remove-section", "edit":
				write = true
			case "get", "list":
				return ""
			default:
				write = write || len(ops) >= 2
			}
		}
		if !write {
			return ""
		}
		for _, f := range flags {
			if f.name == "-f" || longIs(f.name, "--file") {
				return "git config " + f.name + " writes a file of git settings by its path, which may be the checkout's (edit a file of your work tree with the Edit tool instead)"
			}
		}
		return "git config writes the repository's git settings, which the checkout reads too, and a setting can run a command there (core.fsmonitor, core.hooksPath)"
	},
	"remote": func(_ string, rest []string) string {
		_, ops := splitGit(rest, " -t -m ")
		if len(ops) > 0 {
			switch ops[0] {
			case "add", "rename", "remove", "rm", "set-head", "set-branches", "set-url":
				return "git remote " + ops[0] + " writes the repository's git settings, which the checkout reads too"
			}
		}
		return ""
	},
	"replace": func(_ string, rest []string) string {
		// With no operand and no flag but --format it lists.
		flags, ops := splitGit(rest, " --format ")
		lists := len(ops) == 0
		for _, f := range flags {
			switch {
			case f.name == "-l" || longIs(f.name, "--list"):
				return ""
			case !longIs(f.name, "--format"):
				lists = false
			}
		}
		if lists {
			return ""
		}
		return "git replace changes what the repository's history shows, in the checkout too"
	},
	"stash": func(_ string, rest []string) string {
		if _, ops := splitGit(rest, " -m --message --pathspec-from-file "); len(ops) > 0 && ops[0] == "clear" {
			return "git stash clear drops every stash, the checkout's too"
		}
		return ""
	},
	"tag": func(_ string, rest []string) string {
		flags, _ := splitGit(rest, " -u -m -F --local-user --message --file --contains --no-contains --points-at --merged --no-merged --format --sort --cleanup ")
		for _, f := range flags {
			if f.name == "-d" || f.name == "-f" || longIs(f.name, "--delete") || longIs(f.name, "--force", "--format") {
				return "git tag " + f.name + " moves or deletes a tag by name"
			}
		}
		return ""
	},
	"reflog": func(_ string, rest []string) string {
		if _, ops := splitGit(rest, ""); len(ops) > 0 && (ops[0] == "expire" || ops[0] == "delete") {
			return "git reflog " + ops[0] + " throws away the record of where branches have been, which the checkout keeps too"
		}
		return ""
	},
	"filter-branch": func(sub string, _ []string) string {
		return "git " + sub + " rewrites branches"
	},
	"filter-repo": func(sub string, _ []string) string {
		return "git " + sub + " rewrites branches"
	},
}

// gitFetchShares is the check of fetch and pull: a refspec whose
// destination is a branch ("main:main", "+x:refs/heads/x") moves that
// branch, where one under refs/remotes/ does not.
func gitFetchShares(sub string, rest []string) string {
	flags, ops := splitGit(rest, " --depth --deepen --shallow-since --shallow-exclude --negotiation-tip -o --server-option --upload-pack --refmap --recurse-submodules-default --filter -j --jobs --submodule-prefix -s --strategy -X --strategy-option ")
	for _, f := range flags {
		if longIs(f.name, "--multiple") || f.name == "--all" {
			return "" // every word is a remote
		}
	}
	if len(ops) < 2 {
		return ""
	}
	for _, spec := range ops[1:] {
		if spec == notKnown {
			return "git " + sub + " has a refspec only known when the line runs, which may write a branch by name"
		}
		if strings.Contains(spec, "://") || strings.Contains(spec, "@") {
			continue // a place, not a refspec
		}
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
	return repo != "" && (strings.HasPrefix(repo, ".") || strings.HasPrefix(repo, "/") ||
		strings.HasPrefix(repo, "~") || strings.HasPrefix(repo, "file:"))
}

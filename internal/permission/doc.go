// Package permission decides, for every tool call a model makes, whether
// the call runs, is refused, or a person is asked first.
//
// It is a pure decision engine. It reads no files, shows no prompts and
// keeps no global state: the mode, the rules with their sources, the
// working directories, the home directory and whether the folder is
// trusted are all passed in, and the agent loop acts on the answer.
//
//	e, err := permission.New(permission.Options{
//		Cwd: "/home/jae/proj", Home: "/home/jae", Trusted: true,
//		Layers: []permission.Layer{
//			{Scope: permission.ScopeUser, Name: "~/.config/gnosh/settings.json", Base: "/home/jae/.config/gnosh",
//				Allow: []string{"Bash(go test *)"}, Deny: []string{"Read(.env)"}},
//			{Scope: permission.ScopeCLI, Name: "--disallowedTools", Deny: []string{"WebSearch"}},
//		},
//	})
//	d := e.Decide("Bash", call, action) // d.Behavior is Allow, Ask or Deny
//
// When the engine is unsure it asks. It never guesses toward allowing.
//
// # The order of a decision
//
// The first step that has something to say settles the call.
//
//  1. The tool is switched off (a deny rule names the whole tool, or
//     --tools left it out): refused.
//  2. Deny rules, from every source: refused. A deny from any source beats
//     an allow from any other, and no allow rule carves a hole in one.
//  3. Ask rules: a person is asked, in every mode (dontAsk refuses).
//  4. The checks that no allow rule and no hook approval passes: a change
//     to a protected path, a dangerous removal, a guard that could not be
//     checked. See below.
//  5. Allow rules: the call runs.
//  6. The mode: the call runs, or a person is asked.
//
// PreToolUse hooks run before all of this, in the agent loop. The loop
// runs them, computes the tool's Action from the input they left, calls
// Decide, and folds the hooks' verdict in with Decision.WithHook: a
// hook's block always wins; a hook's "ask" asks even when the call would
// have run; a hook's approval skips only the question that step 6 would
// have asked. Decision.Stage says which step decided.
//
// # Modes
//
//   - default: reading inside the working directories and the read-only
//     shell commands run; everything else asks.
//   - acceptEdits: also runs file edits, and the commands mkdir, touch,
//     rm, rmdir, mv, cp and sed -i in their everyday forms, when every
//     path stays inside the working directories (after links are
//     followed). Protected paths, dangerous removals, anything outside,
//     fetches and every other command still ask.
//   - plan: the model looks and does not change. File edits are refused;
//     commands that are not read-only ask, whatever allow rule names
//     them; a hook's approval does not reach them either.
//   - dontAsk: runs what default runs and what allow rules cover, and
//     refuses everything that would have asked.
//   - bypassPermissions: runs everything except what a deny rule refuses,
//     what an ask rule or a hook wants asked, a dangerous removal, and a
//     command a deny or ask rule could not be checked against.
//
// "manual" is another name for default. "auto" is not a gnosh mode and
// reads as default, with a note in Explain. A project's settings can
// never start a session in bypassPermissions.
//
// # Rules
//
// A rule is a tool name, or a tool name and a specifier in parentheses.
//
//	Bash                         every use of the tool (Bash(*) is the same)
//	Bash(npm run build)          that exact command
//	Bash(git diff *)             the command and anything after it, or nothing
//	Bash(ls:*)                   the same, in the older spelling
//	Read(.env)                   a file of that name anywhere under the working directory
//	Edit(/src/**)                everything under src, beside the settings file the rule is in
//	Read(//etc/**)   Read(~/x)   from the filesystem root; from the home directory
//	Read(!public/**)             deny and ask only: take these back out of the rules before it
//	WebFetch(domain:example.com) that host; domain:*.example.com is its subdomains
//	mcp__github   mcp__github__get_*   mcp__github__get_issue
//	Agent(Explore)               that sub-agent type (Task is an old name for Agent)
//	Skill(deploy)  Skill(deploy *)     the skill alone; the skill with any arguments
//	Bash(run_in_background:true) deny and ask only: one input parameter and a value pattern
//	*   mcp__*                   deny and ask only: every tool whose name fits
//
// Edit(path) covers every tool that changes files (Edit, Write,
// NotebookEdit); Read(path) covers Read, Grep and Glob. An Edit allow also
// grants reading. A Read deny also stops Edit and Write on that path.
// Both kinds of deny rule also reach the files a shell command names:
// cat .env, a redirection into a denied file, tee, sed -i, rm, mv, cp.
//
// A rule that cannot be applied is an error that names the rule and where
// it was written, and New returns no engine: an unknown tool name, a form
// its list does not take, Write(path) where Edit(path) was meant. Nothing
// is accepted and ignored. The one exception is by design: names Claude
// Code has and gnosh does not (TodoWrite, MultiEdit, ...) load and never
// match, so that settings written for Claude Code still load.
//
// Deny and ask path rules ignore letter case and cover everything under a
// directory they match; allow rules are exact about both.
//
// # Shell commands
//
// The engine has its own reader of shell command lines (shell.go). It
// splits a line at && || ; | |& & and line breaks, and follows quoting,
// escapes, $'...', backslash-newline, comments, here-documents,
// redirections, leading assignments, $(...), backticks, <(...),
// subshells, groups, if, while, for, case, [[ ]], (( )) and function
// definitions. Anything else (select, coproc, $[...], quotes inside
// ${...}, zsh-only syntax, an operator with nothing after it, a comment
// that holds operators) it reports as not understood, and a line it did
// not understand is never allowed by a rule or by the read-only list.
//
// Deny and ask rules apply when any command in the line matches: one of
// its own, one nested in a subshell, a substitution or a loop body, the
// command after a wrapper (env, sudo, timeout, xargs, find -exec, ...) and
// the script handed to sh -c, eval, trap, a here-document or a pipe into
// a shell. Each is matched as written, with quotes removed, without its
// directory (/bin/rm is rm) and past any assignments. For a line the
// reader did not understand, the text is also cut crudely at everything
// that could start a command, and those pieces are matched too: that
// finds too much rather than too little.
//
//	deny Bash(rm *) stops:  ls; rm -rf x    echo $(rm -rf x)    'r'm -rf x
//	                        FOO=1 rm x      sudo rm x           sh -c 'rm x'
//	and does not stop:      echo rm -rf x   grep rm notes.txt
//
// When part of a command is only known once the shell runs it ($CMD -rf
// x, git ${X:-push}), a deny or ask rule that could match what it turns
// into makes the engine ask, in every mode: a guard that could not be
// checked has not passed. A rule that could not match changes nothing
// (echo $HOME under a deny on rm).
//
// An allow rule is deliberately narrower. A line runs on allow rules only
// when the reader understood all of it, and it holds no substitution,
// subshell, loop or other compound form, background job, assignment,
// ${...} form beyond the plain ones, or redirection to a file, and every
// command in it is, on its own, either named by an allow rule or
// read-only. A command is matched as the model
// wrote it (with runs of blanks counted as one), and its program name
// must be plain text. Only timeout, time, nice, nohup, stdbuf, command,
// builtin, noglob and a bare xargs are seen through. A rule with a *
// never approves watch, setsid, ionice, flock, or a find that runs,
// deletes or writes; only the exact command does. A bare Bash rule is the
// whole tool and allows every line.
//
//	allow Bash(npm run *) runs:   npm run build     timeout 60 npm run test 2>&1
//	                              npm run build | head -20
//	and asks about:               npm run build && npm install
//	                              npm run $(cat x)   NODE_ENV=test npm run build
//	                              npm run build > log.txt   "npm" run build
//
// # Read-only commands
//
// readonly.go holds the table of commands that run without asking in
// every mode: ls, cat, head, tail, wc, grep, rg, find, which, diff, stat,
// du, pwd, echo, cd, sort, uniq, cut, tr, date, test and a few more, and
// git status, diff, log, show, branch, rev-parse, ls-files and blame. Each
// entry lists the flags known to keep the command read-only; any other
// flag asks, and so does a flag shortened to a prefix. A read-only command
// is also held to the working directories: every file it names, and the
// directory it would run in after any cd in the line, must lie inside
// them as written and as resolved. A variable, a brace pattern or (with
// no Options.Glob) a file name pattern cannot be checked ahead of time
// and asks.
//
// # Protected paths and dangerous removals
//
// A change to a protected path asks even when a rule allows the tool or
// the mode is acceptEdits: .git, .vscode, .idea, .husky, .cargo,
// .devcontainer, .yarn, .mvn, .claude, .gnosh, .ssh, .gnupg, shell
// startup files, the settings files of package managers and other tools,
// gnoshrc, and whatever Options.Protected lists. Only bypassPermissions
// skips the question; dontAsk refuses.
//
// An rm or rmdir whose target is the root, a top-level directory, the
// home directory, the working directory or something above it, or that
// could become one of those if a variable were empty (rm -rf "$DIR"/*),
// asks in every mode, bypassPermissions included (Decision.Critical is
// set, and the loop then asks with a time limit). "${DIR:?}" is the way
// to write such a command so that it does not.
//
// # Suggestions
//
// A Decision that asks carries the choices "yes, and don't ask again"
// could make, narrowest first: for a command, the exact command and then
// a prefix rule one step wider (Bash(npm test *)), one rule for each part
// of the line that needed approval; for an edit, that one file, then
// acceptEdits; for a fetch, the host. There are none when no rule could
// cover the call (a line with a substitution, a protected path, an ask
// rule), and then only a one-time yes can be offered.
//
// # An untrusted folder
//
// While Options.Trusted is false, the layers the project supplied can
// only take away: their deny and ask rules apply, and their allow rules,
// additional directories and default mode wait. Explain lists what is
// held.
package permission

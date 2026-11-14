package shelltool

import (
	"maps"
	"slices"
	"strings"
)

// fixedEnv is what every command's environment is given, whatever the
// parent's holds. Nobody is there to answer a command, so the first four
// keep programs from waiting on a person: no pager, no password prompt from
// git, no terminal tricks. The last three let a script tell that an AI
// agent, and which one, is running it.
func fixedEnv() []string {
	return []string{
		"PAGER=cat",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
		"TERM=dumb",
		"GNOSH=1",
		"AGENT=gnosh",
		"AI_AGENT=gnosh",
	}
}

// notHandedOn are the variables of the parent's environment that a command
// is not given. Each one changes how a shell reads a command line, or makes
// a shell run or read something of its own accord before or beside the
// command. The permission engine judges a line as a shell reads it when
// none of these is set; with one of them set by whoever started gnosh, a
// line it let run could run something else. A variable is dropped only
// from what is inherited: the settings' own env block (Options.Env) is the
// person's word for commands, and is laid over the result as before, these
// names included.
//
// The value says why. What each does was tried with bash 5.2 and dash 0.5.12.
var notHandedOn = map[string]string{
	// Files a shell reads and runs before the command.
	"BASH_ENV": "bash runs this file before any command it is handed",
	"ENV":      "sh, dash and bash in POSIX mode run this file when they start with -i",

	// Options that are switched on by being named here.
	"BASHOPTS":        "bash switches on each shopt option named in it: dotglob and nocaseglob change what a pattern matches, extglob and globstar what a pattern is, expand_aliases makes an alias run, lastpipe moves where a read happens",
	"SHELLOPTS":       "bash switches on each set -o option named in it: noglob, keyword (name=value arguments become assignments), posix, allexport, noclobber, errexit, nounset, noexec, xtrace",
	"POSIXLY_CORRECT": "bash starts in POSIX mode (aliases run, special builtins are found before functions, an assignment before one of them stays), and the GNU tools stop reading flags at the first operand, so that the same words are read another way",
	"BASH_COMPAT":     "bash behaves as an earlier version did: with 3.1 a quoted pattern after =~ is a regular expression again, and its manual lists more, down to how often the text of a sum is expanded",

	// What words and patterns turn into.
	"GLOBIGNORE": "names that match it are left out of what a pattern matches, and with it set a * takes in the names that start with a dot; bash 5.2 keeps an inherited one as a plain variable until the line assigns it again, and then it counts",
	"CDPATH":     "cd with a plain name goes to that name under one of these directories, not under the one the shell is in",
	"IFS":        "it is where a shell cuts words; bash and dash take no notice of an inherited one, and a shell that did would cut the same line into other words",

	// Text a shell expands, command substitutions and all, when it prints.
	"PS4":            "bash expands it before every line that set -x traces, and runs the commands written in it",
	"PS0":            "an interactive bash expands it after reading each command, and runs the commands written in it",
	"PS1":            "an interactive shell expands it for every prompt, and runs the commands written in it",
	"PS2":            "the same, for the prompt of a line that goes on",
	"PROMPT_COMMAND": "an interactive bash runs it as a command before every prompt",
	"BASH_XTRACEFD":  "bash writes the trace of set -x to this descriptor, which may be a file the command has open for something else",
}

// handedOn reports whether a variable of the parent's environment is one a
// command is given.
//
// Kept on purpose, though each can change what a command does: PATH, HOME,
// the locale (LANG, LC_*), TMPDIR and SHELL, which programs need to work
// at all; LD_PRELOAD, LD_LIBRARY_PATH and DYLD_*, GIT_* and the like,
// which change what a program does once it runs and not how the line that
// names it is read (a build may depend on them, and they are the choice
// of whoever started gnosh); ZDOTDIR, which only says where zsh's own
// startup files are; and TMOUT, EXECIGNORE, FIGNORE and HISTFILE, which
// were tried and change nothing about what a line handed to bash -c or
// dash -c runs.
func handedOn(name, value string) bool {
	if _, drop := notHandedOn[name]; drop {
		return false
	}
	// A function bash exported: every bash that starts defines it, and a
	// function named ls or cd then runs wherever the line says ls or cd.
	// Bash since 4.3 passes one as BASH_FUNC_name%% (and for a few
	// releases as BASH_FUNC_name()); before that, and in other programs
	// that copied it, as any variable whose value begins with "() {".
	if strings.HasPrefix(name, "BASH_FUNC_") || strings.HasPrefix(value, "() {") {
		return false
	}
	return true
}

// CommandEnv returns the environment the manager gives a command that
// starts in dir, built from the parent's environment and the settings' env
// block as the manager builds it. It is exported for the permission
// engine's tests, which hold what the engine lets run against what a shell
// started with this environment does.
func CommandEnv(parent []string, dir string, extra []string) []string {
	return commandEnv(parent, dir, extra)
}

// commandEnv builds the environment of a command that starts in dir: the
// parent's without what is not handed on (notHandedOn), then fixedEnv, then
// PWD set to dir and OLDPWD removed (so that the shell's idea of where it
// is matches where it was started), then extra. Later settings win. In
// extra, a name with no "=" removes the variable. The result is sorted, so
// that it is the same every time.
func commandEnv(parent []string, dir string, extra []string) []string {
	vars := make(map[string]string, len(parent)+8)
	for _, kv := range parent {
		if name, value, ok := strings.Cut(kv, "="); ok && name != "" {
			if handedOn(name, value) {
				vars[name] = value
			} else {
				// (A later copy that is dropped takes an earlier one that
				// was kept with it: the last one is the one a shell sees.)
				delete(vars, name)
			}
		}
	}
	for _, kv := range fixedEnv() {
		name, value, _ := strings.Cut(kv, "=")
		vars[name] = value
	}
	vars["PWD"] = dir
	delete(vars, "OLDPWD")
	for _, kv := range extra {
		name, value, ok := strings.Cut(kv, "=")
		switch {
		case name == "":
		case ok:
			vars[name] = value
		default:
			delete(vars, name)
		}
	}
	out := make([]string, 0, len(vars))
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		out = append(out, name+"="+vars[name])
	}
	return out
}

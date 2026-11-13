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

// commandEnv builds the environment of a command that starts in dir: the
// parent's, then fixedEnv, then PWD set to dir and OLDPWD removed (so that
// the shell's idea of where it is matches where it was started), then
// extra. Later settings win. In extra, a name with no "=" removes the
// variable. The result is sorted, so that it is the same every time.
func commandEnv(parent []string, dir string, extra []string) []string {
	vars := make(map[string]string, len(parent)+8)
	for _, kv := range parent {
		if name, value, ok := strings.Cut(kv, "="); ok && name != "" {
			vars[name] = value
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

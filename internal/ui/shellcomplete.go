package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/ui/editor"
)

// What Tab offers in shell mode, a line that begins with "!", as Claude
// Code's Tab does there.
//
// First come the commands run before from this folder that begin with
// the line typed so far, when the cursor is at its end, the newest first.
// Then the word at the cursor is completed: where a command goes (the
// line's first word, or a word after &&, ||, ;, |, & or "(", or after a
// word such as sudo that runs the command after it) by the names of the
// programs on the PATH and of the shell's own commands, and anywhere else
// by the names of files. The list opens on Tab only, never by itself
// while the line is typed, so that Enter always runs the line as typed.

// shellEarlier is the most commands run before that one Tab offers.
const shellEarlier = 5

// shellBuiltins are the shell's own commands that a person may run behind
// "!", which are not files on the PATH.
var shellBuiltins = []string{
	"alias", "cd", "command", "declare", "eval", "exec", "export", "popd", "pushd",
	"read", "set", "source", "type", "ulimit", "umask", "unalias", "unset", "wait",
}

// runsNext are the words after which a command goes, as after an
// operator: they run the command that follows them.
var runsNext = map[string]bool{
	"sudo": true, "time": true, "nohup": true, "exec": true, "command": true, "nice": true,
	"env": true, "xargs": true, "then": true, "do": true, "else": true, "if": true,
	"while": true, "until": true, "!": true,
}

// shellBreaks are the characters that end a word of a shell line.
const shellBreaks = " \t\n;&|()<>"

// completeShell is complete for a line of shell mode; text begins with
// the "!".
func (s *session) completeShell(text string, cursor int) []editor.Candidate {
	before := text[:cursor]
	// Where the command begins: after the "!" and the blanks after it.
	lead := len(before) - len(strings.TrimLeft(before[1:], " \t"))
	var out []editor.Candidate
	if typed := before[lead:]; cursor == len(text) && strings.TrimSpace(typed) != "" && s.ed != nil {
		seen := map[string]bool{}
		for _, h := range s.ed.Sent() {
			cmd, ok := strings.CutPrefix(h, "!")
			if !ok {
				continue
			}
			cmd = strings.TrimSpace(cmd)
			if len(cmd) > len(typed) && strings.HasPrefix(cmd, typed) && !seen[cmd] {
				seen[cmd] = true
				out = append(out, editor.Candidate{Text: cmd, Start: lead, Label: cmd, Detail: "run before"})
				if len(out) == shellEarlier {
					break
				}
			}
		}
	}
	start := max(strings.LastIndexAny(before, shellBreaks)+1, lead)
	word := before[start:]
	switch {
	case strings.ContainsAny(word, "'\"\\$`*?[]{}=") || strings.HasPrefix(word, "-"):
		// Quoted, escaped, computed, a pattern, an assignment or an
		// option: nothing a name could finish.
	case commandPlace(before[lead:start]) && !strings.ContainsRune(word, '/') && !strings.HasPrefix(word, "~"):
		if word != "" {
			out = append(out, s.completeProgram(word, start)...)
		}
	default:
		out = append(out, s.completePath(word, start, "", false)...)
	}
	return out
}

// commandPlace reports whether a word that follows the part of a command
// line before it is where a command goes.
func commandPlace(before string) bool {
	if rest, ok := strings.CutSuffix(strings.TrimRight(before, " \t"), "\\\n"); ok {
		// A line that goes on.
		return commandPlace(rest)
	}
	before = strings.TrimRight(before, " \t")
	if before == "" {
		return true
	}
	switch before[len(before)-1] {
	case ';', '|', '(', '\n':
		return true
	case '&':
		// && or a command sent to the background; not the >& or <& of a
		// redirection.
		return len(before) < 2 || before[len(before)-2] != '>' && before[len(before)-2] != '<'
	}
	i := strings.LastIndexAny(before, shellBreaks) + 1
	return runsNext[before[i:]] && commandPlace(before[:i])
}

// completeProgram offers the shell's own commands and the programs on the
// PATH whose names begin with word, in name order, each followed by a
// space.
func (s *session) completeProgram(word string, start int) []editor.Candidate {
	var names []string
	for _, b := range shellBuiltins {
		if strings.HasPrefix(b, word) {
			names = append(names, b)
		}
	}
	if s.programs != nil {
		names = append(names, s.programs(word)...)
	}
	slices.Sort(names)
	names = slices.Compact(names)
	if len(names) > maxCandidates {
		names = names[:maxCandidates]
	}
	out := make([]editor.Candidate, 0, len(names))
	for _, name := range names {
		out = append(out, editor.Candidate{Text: name + " ", Start: start, Label: name})
	}
	return out
}

// pathPrograms lists the programs in the directories of path (a PATH
// value) whose names begin with prefix: files that may be run, each name
// once. A directory that is not written as an absolute path is passed
// over, as the folder it means changes with the shell's.
func pathPrograms(path, prefix string) []string {
	var out []string
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		f, err := os.Open(dir)
		if err != nil {
			continue
		}
		names, _ := f.Readdirnames(maxDirEntries)
		_ = f.Close()
		for _, name := range names {
			if seen[name] || !strings.HasPrefix(name, prefix) || strings.ContainsAny(name, " \t\n") || editor.Visible(name) != name {
				continue
			}
			fi, err := os.Stat(filepath.Join(dir, name))
			if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 == 0 {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

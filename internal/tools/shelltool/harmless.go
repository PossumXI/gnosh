package shelltool

import (
	"path"
	"strings"
)

// harmlessExit says what exit code 1 means when the command is one for
// which 1 is an ordinary answer and not a failure: a search that found
// nothing, a comparison that found a difference, a test that was false. It
// returns "" for every other command, for which exit code 1 is a failure.
//
// The programs are the ones Claude Code's tools reference lists: grep,
// rg, egrep, fgrep, find, diff, test, [, git diff and git grep. That page
// does not say how a command is recognised as one of them, so the rule
// here is a careful one: the command must be a single simple command or a
// pipeline of them, whose last stage, which is the one the exit code comes
// from, runs one of those programs. Anything with ; & ( ) { } a backquote
// or a second line could have got its 1 from somewhere else, and counts as
// a failure.
func harmlessExit(command string) string {
	stage, ok := lastStage(command)
	if !ok {
		return ""
	}
	words := shellWords(stage)
	// Variable settings in front of the program do not change what runs.
	for len(words) > 0 && isAssignment(words[0]) {
		words = words[1:]
	}
	if len(words) == 0 || words[0].quoted {
		return ""
	}
	switch path.Base(words[0].text) {
	case "grep", "egrep", "fgrep", "rg":
		return "no line matched"
	case "find":
		return "find could not look everywhere it was asked to"
	case "diff":
		return "the two sides differ"
	case "test", "[":
		return "the test was false"
	case "git":
		switch gitSubcommand(words[1:]) {
		case "diff":
			return "there are differences"
		case "grep":
			return "no line matched"
		}
	}
	return ""
}

// lastStage returns the last stage of a command that is one simple command
// or one pipeline, and false for anything more: a list, a subshell, a
// group, a command substitution outside double quotes, more than one line,
// or quotes left open.
func lastStage(command string) (string, bool) {
	// Only what a shell takes for white space is taken off the ends.
	command = strings.Trim(command, " \t\r\n")
	start := 0
	var quote byte // the quote we are inside: ' or ", or 0
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\':
			i++ // whatever follows is taken as it is
		case quote == '"':
			if c == '"' {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case strings.IndexByte(";&(){}`\n", c) >= 0:
			return "", false
		case c == '|':
			if i+1 < len(command) && command[i+1] == '|' {
				return "", false
			}
			start = i + 1
		}
	}
	if quote != 0 {
		return "", false
	}
	return command[start:], true
}

// shellWord is one word of a simple command.
type shellWord struct {
	text   string // the word with its quotes and backslashes taken off
	raw    string // the word as it was written
	quoted bool   // any part of it was quoted or escaped
}

// shellWords splits a simple command into words the way a shell would,
// as far as quotes and backslashes go; it expands nothing.
func shellWords(s string) []shellWord {
	var words []shellWord
	var text strings.Builder
	begin, in, quoted := 0, false, false
	var quote byte
	flush := func(end int) {
		if in {
			words = append(words, shellWord{text: text.String(), raw: s[begin:end], quoted: quoted})
			text.Reset()
			in, quoted = false, false
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote == 0 && (c == ' ' || c == '\t') {
			flush(i)
			continue
		}
		if !in {
			in, begin = true, i
		}
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				text.WriteByte(c)
			}
		case c == '\\' && i+1 < len(s):
			i++
			quoted = true
			text.WriteByte(s[i])
		case quote == '"':
			if c == '"' {
				quote = 0
			} else {
				text.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote, quoted = c, true
		default:
			text.WriteByte(c)
		}
	}
	flush(len(s))
	return words
}

// isAssignment reports whether a word sets a variable for the command that
// follows it: NAME=value.
func isAssignment(w shellWord) bool {
	name, _, ok := strings.Cut(w.raw, "=")
	if !ok || name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		letter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if !letter && (i == 0 || c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// gitSubcommand finds git's subcommand among the words after "git",
// stepping over git's own options and the values of -C and -c.
func gitSubcommand(args []shellWord) string {
	for i := 0; i < len(args); i++ {
		switch w := args[i].text; {
		case w == "-C" || w == "-c":
			i++
		case strings.HasPrefix(w, "-"):
		default:
			return w
		}
	}
	return ""
}

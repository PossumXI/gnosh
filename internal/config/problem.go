package config

import (
	"fmt"
	"slices"
	"strings"
)

// Severity says whether a Problem stops startup.
type Severity int

const (
	// Error is a mistake in the configuration. Startup stops, and a config
	// that is being loaded again is refused, leaving the running settings
	// as they were.
	Error Severity = iota
	// Warning is something worth knowing that stops nothing: a provider
	// that was left out, a variable that was not set. Load never prints
	// one; the caller writes them to the log once there is a log, and to
	// stderr in a run with nobody watching.
	Warning
)

// String is "error" or "warning".
func (s Severity) String() string {
	if s == Warning {
		return "warning"
	}
	return "error"
}

// Problem is one thing Load noticed. Its Message is a plain sentence that
// says what is wrong, what was left out, and what to do about it. No
// Message ever holds a key, a password or another secret.
type Problem struct {
	Severity Severity
	// File is the file the problem is in, or "" when it is not in a file.
	File string
	// Line is the line in File, counted from 1, or 0 when there is none.
	Line int
	// Where names the place of a problem that is not in a file: a flag
	// ("--model") or an environment variable ("$API_TIMEOUT_MS").
	Where string
	// Message is the sentence.
	Message string
}

// String is the problem with its place in front: "file:line: message".
func (p Problem) String() string {
	switch {
	case p.File != "" && p.Line > 0:
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Message)
	case p.File != "":
		return p.File + ": " + p.Message
	case p.Where != "":
		return p.Where + ": " + p.Message
	}
	return p.Message
}

// Errors returns the problems that stop startup, in order.
func Errors(problems []Problem) []Problem {
	var out []Problem
	for _, p := range problems {
		if p.Severity == Error {
			out = append(out, p)
		}
	}
	return out
}

// Warnings returns the problems that stop nothing, in order.
func Warnings(problems []Problem) []Problem {
	var out []Problem
	for _, p := range problems {
		if p.Severity == Warning {
			out = append(out, p)
		}
	}
	return out
}

// LoadError is the error Load returns when the configuration has mistakes.
// It holds every mistake, not only the first.
type LoadError struct {
	// Problems are the mistakes: every one has Severity Error.
	Problems []Problem
}

// Error lists the mistakes, one per line.
func (e *LoadError) Error() string {
	var b strings.Builder
	if len(e.Problems) == 1 {
		b.WriteString("the configuration has a mistake, so gnosh did not start:")
	} else {
		fmt.Fprintf(&b, "the configuration has %d mistakes, so gnosh did not start:", len(e.Problems))
	}
	for _, p := range e.Problems {
		b.WriteString("\n  ")
		b.WriteString(p.String())
	}
	return b.String()
}

// problems collects what a load notices. The zero value is ready to use.
type problems struct {
	list []Problem
}

// errorf records a mistake at a line of a file.
func (ps *problems) errorf(file string, line int, format string, args ...any) {
	ps.list = append(ps.list, Problem{Severity: Error, File: file, Line: line, Message: fmt.Sprintf(format, args...)})
}

// warnf records something worth knowing at a line of a file.
func (ps *problems) warnf(file string, line int, format string, args ...any) {
	ps.list = append(ps.list, Problem{Severity: Warning, File: file, Line: line, Message: fmt.Sprintf(format, args...)})
}

// at records a problem at an Origin: a line of a file, a flag or a variable.
func (ps *problems) at(sev Severity, o Origin, format string, args ...any) {
	p := Problem{Severity: sev, File: o.File, Line: o.Line, Message: fmt.Sprintf(format, args...)}
	if o.File == "" {
		p.Where = o.Name
	}
	ps.list = append(ps.list, p)
}

// dedupe drops problems that say the same thing about the same place.
func dedupe(list []Problem) []Problem {
	seen := make(map[Problem]bool, len(list))
	return slices.DeleteFunc(slices.Clone(list), func(p Problem) bool {
		if seen[p] {
			return true
		}
		seen[p] = true
		return false
	})
}

// show quotes a word taken from a config file for a message. Control
// characters come out escaped, so a file cannot write to the terminal
// through an error, and a very long word is cut.
func show(s string) string {
	const most = 80
	if len(s) > most {
		cut := most
		for cut > 0 && !isRuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return fmt.Sprintf("%q", s)
}

// isRuneStart reports whether b can begin a UTF-8 sequence.
func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// showName quotes a word that was found where a name was expected (a key
// of the file, an id, an option). A word that looks like a name is shown
// whole. Anything else may be a secret that landed in the wrong place, so
// only its first characters are shown.
func showName(s string) string {
	plain := len(s) <= 40
	for i := 0; plain && i < len(s); i++ {
		c := s[i]
		plain = c == '-' || c == '_' || c == '.' || c == '/' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
	}
	digits := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			digits++
		}
	}
	if plain && (len(s) <= 20 || digits == 0) {
		return fmt.Sprintf("%q", s)
	}
	cut := min(len(s), 4)
	for cut > 0 && cut < len(s) && !isRuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%q", s[:cut]+"…")
}

// list joins names for a message: "a, b, c".
func list(names []string) string { return strings.Join(names, ", ") }

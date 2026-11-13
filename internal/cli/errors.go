package cli

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrorKind sorts the reasons a command line is refused.
type ErrorKind string

// The kinds of Error.
const (
	// ErrUnknownFlag: an argument that begins with a dash is not a flag of
	// the command it was typed for.
	ErrUnknownFlag ErrorKind = "unknown_flag"
	// ErrUnknownCommand: a word where a command belongs is not a command,
	// or a first word is one letter off a command's name.
	ErrUnknownCommand ErrorKind = "unknown_command"
	// ErrMissingValue: a flag that needs a value was given none.
	ErrMissingValue ErrorKind = "missing_value"
	// ErrBadValue: a flag's value is empty, outside its set, or malformed.
	ErrBadValue ErrorKind = "bad_value"
	// ErrConflict: two things on the line cannot both hold.
	ErrConflict ErrorKind = "conflict"
	// ErrUsage: the line reads well but cannot be acted on as typed: no
	// prompt for print mode, a print-only flag without print mode, too many
	// or too few arguments.
	ErrUsage ErrorKind = "usage"
	// ErrNotBuilt: the line uses a command, flag or value whose feature is
	// not built yet.
	ErrNotBuilt ErrorKind = "not_built"
)

// Error is a command line gnosh could not act on. Its message is written to
// be shown as it is: it starts with what was typed, exactly as typed, says
// that nothing was started, and says what to type instead.
type Error struct {
	// Kind sorts the error.
	Kind ErrorKind
	// Message is the whole text.
	Message string
	// Typed is the piece of the command line the error is about, exactly as
	// typed.
	Typed string
	// Suggestion is the nearest real name, when there is one.
	Suggestion string
	// NotBuilt, for ErrNotBuilt, lists what is not built, by the names
	// Table.SetStatus takes.
	NotBuilt []string
}

// Error returns the message.
func (e *Error) Error() string { return e.Message }

// shownMax is how much of a piece of typed text a message repeats.
const shownMax = 60

// shown returns typed text as a message should carry it. An ordinary word is
// returned untouched, letter for letter. Text that is empty, has a space or
// a quote in it, or holds anything a terminal would act on comes back in
// double quotes with the odd bytes written as escapes, and long text is cut
// short, so that a message can never be mistaken for, or damaged by, what
// it repeats.
func shown(s string) string {
	plain := s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= shownMax
	if plain {
		for _, r := range s {
			if r == ' ' || r == '"' || r == '\\' || !unicode.IsPrint(r) {
				plain = false
				break
			}
		}
	}
	if plain {
		return s
	}
	cut := false
	if utf8.RuneCountInString(s) > shownMax {
		n := 0
		for i := range s {
			if n == shownMax {
				s = s[:i]
				cut = true
				break
			}
			n++
		}
	}
	q := strconv.Quote(s)
	if cut {
		q += "..."
	}
	return q
}

// oneOf writes a list of values as prose: "a", "a or b", "a, b or c".
func oneOf(names []string, last string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " " + last + " " + names[len(names)-1]
}

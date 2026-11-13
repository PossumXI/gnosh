// Package keyscript turns a line of text such as
//
//	"hello" <Left> <Left> <Backspace> <Shift+Enter> <Paste "two\nlines">
//
// into the events a terminal would send for it, so that a test of anything
// that takes term.Event values can say what was typed the way a person
// would say it. It does no I/O and keeps no state.
//
// # The script
//
// A script is a row of pieces with spaces between them.
//
//   - "text" or `text` is typed, one key per character. The double-quoted
//     form takes Go's escapes ("\u00e9", "\\"). A control character in it
//     is the key that sends it: "\t" is Tab, "\r" is Enter, "\n" is
//     Ctrl+J, "\x7f" is Backspace, "\x1b" is Esc.
//   - <Key> is one key press. A key is named (Enter, Tab, Backspace, Esc,
//     Up, Down, Left, Right, Home, End, PgUp, PgDn, Insert, Delete, Space,
//     F1 to F35) or is a single character, and may have modifiers before
//     it joined with "+": <Ctrl+A>, <Alt+B>, <Shift+Enter>, <Ctrl+Alt+Up>,
//     <Ctrl+_>, <Ctrl++>. Names and modifiers are matched without regard
//     to case. A letter with Ctrl or Alt is reported in lower case, as
//     terminals report it; a character with Shift alone still types its
//     shifted form.
//   - <Paste "text"> is a bracketed paste, delivered whole. <PasteCut
//     "text"> is one the terminal reported as cut short.
//   - <Resize 80x24> is the window changing size; <Focus> and <Blur> are
//     the window gaining and losing the keyboard; <Resume> is the terminal
//     coming back after the program was stopped.
//   - Any piece may be followed by *N to repeat it: <Left>*3, "ab"*2.
package keyscript

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// Parse reads a key script and returns its events in order. The error
// names the piece that could not be read.
func Parse(script string) ([]term.Event, error) {
	var out []term.Event
	rest := strings.TrimLeft(script, " \t\n")
	for rest != "" {
		var evs []term.Event
		var err error
		switch rest[0] {
		case '"', '`':
			var lit string
			lit, rest, err = quoted(rest)
			if err != nil {
				return nil, err
			}
			evs = Type(lit)
		case '<':
			end := closing(rest)
			if end < 0 {
				return nil, fmt.Errorf("keyscript: no closing > for %q", rest)
			}
			evs, err = angle(rest[1:end])
			if err != nil {
				return nil, err
			}
			rest = rest[end+1:]
		default:
			return nil, fmt.Errorf("keyscript: expected a quoted string or <Key> at %q", rest)
		}
		n := 1
		if strings.HasPrefix(rest, "*") {
			digits := 1
			for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
				digits++
			}
			n, err = strconv.Atoi(rest[1:digits])
			if err != nil {
				return nil, fmt.Errorf("keyscript: expected a count after * at %q", rest)
			}
			rest = rest[digits:]
		}
		for range n {
			out = append(out, evs...)
		}
		if rest != "" && !strings.ContainsRune(" \t\n", rune(rest[0])) {
			return nil, fmt.Errorf("keyscript: expected a space before %q", rest)
		}
		rest = strings.TrimLeft(rest, " \t\n")
	}
	return out, nil
}

// MustParse is Parse for scripts written in a test's source: it panics on
// a script that cannot be read.
func MustParse(script string) []term.Event {
	evs, err := Parse(script)
	if err != nil {
		panic(err)
	}
	return evs
}

// Type returns the key presses that type s, one per character. A control
// character becomes the key that sends it, as the terminal's decoder
// reports it.
func Type(s string) []term.Event {
	out := make([]term.Event, 0, len(s))
	for _, r := range s {
		out = append(out, typed(r))
	}
	return out
}

// typed is the key press that sends the character r.
func typed(r rune) term.Key {
	switch {
	case r == '\t':
		return term.Key{Code: term.KeyTab}
	case r == '\r':
		return term.Key{Code: term.KeyEnter}
	case r == 0x7f || r == 0x08:
		return term.Key{Code: term.KeyBackspace}
	case r == 0x1b:
		return term.Key{Code: term.KeyEscape}
	case r == 0:
		return term.Key{Code: term.KeyRune, Rune: ' ', Mod: term.ModCtrl}
	case r <= 0x1a:
		return term.Key{Code: term.KeyRune, Rune: 'a' + r - 1, Mod: term.ModCtrl}
	case r < 0x20:
		return term.Key{Code: term.KeyRune, Rune: r + 0x40, Mod: term.ModCtrl}
	}
	return term.Key{Code: term.KeyRune, Rune: r, Text: string(r)}
}

// quoted reads the string literal that s begins with and returns its value
// and what follows it.
func quoted(s string) (value, rest string, err error) {
	lit, err := strconv.QuotedPrefix(s)
	if err != nil {
		return "", "", fmt.Errorf("keyscript: bad string at %q", s)
	}
	value, err = strconv.Unquote(lit)
	if err != nil {
		return "", "", fmt.Errorf("keyscript: bad string %s", lit)
	}
	return value, s[len(lit):], nil
}

// closing finds the > that ends the piece s begins with, passing over any
// string literal inside it. It returns -1 when there is none.
func closing(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '"', '`':
			lit, err := strconv.QuotedPrefix(s[i:])
			if err != nil {
				return -1
			}
			i += len(lit) - 1
		case '>':
			// A key may itself be ">": <Alt+>> and <>>.
			if i+1 < len(s) && s[i+1] == '>' {
				return i + 1
			}
			return i
		}
	}
	return -1
}

// angle reads the inside of a <...> piece.
func angle(body string) ([]term.Event, error) {
	word, arg, _ := strings.Cut(body, " ")
	arg = strings.TrimSpace(arg)
	switch strings.ToLower(word) {
	case "paste", "pastecut":
		text, rest, err := quoted(arg)
		if err != nil || rest != "" {
			return nil, fmt.Errorf("keyscript: <%s> wants one quoted string", body)
		}
		return []term.Event{term.Paste{Text: text, Truncated: strings.EqualFold(word, "pastecut")}}, nil
	case "resize":
		cols, rows, ok := strings.Cut(strings.ToLower(arg), "x")
		c, err1 := strconv.Atoi(cols)
		r, err2 := strconv.Atoi(rows)
		if !ok || err1 != nil || err2 != nil {
			return nil, fmt.Errorf("keyscript: <%s> wants a size such as 80x24", body)
		}
		return []term.Event{term.Resize{Cols: c, Rows: r}}, nil
	case "focus":
		return []term.Event{term.Focus{In: true}}, nil
	case "blur":
		return []term.Event{term.Focus{In: false}}, nil
	case "resume":
		return []term.Event{term.Resume{}}, nil
	}
	if arg != "" {
		return nil, fmt.Errorf("keyscript: unknown piece <%s>", body)
	}
	k, err := Key(body)
	if err != nil {
		return nil, err
	}
	return []term.Event{k}, nil
}

// The modifiers a script can name.
var modNames = map[string]term.Mod{
	"ctrl": term.ModCtrl, "control": term.ModCtrl,
	"alt": term.ModAlt, "option": term.ModAlt, "opt": term.ModAlt,
	"shift": term.ModShift,
	"super": term.ModSuper, "cmd": term.ModSuper,
	"meta": term.ModMeta, "hyper": term.ModHyper,
}

// The keys a script can name.
var keyNames = map[string]term.KeyCode{
	"enter": term.KeyEnter, "return": term.KeyEnter,
	"tab":       term.KeyTab,
	"backspace": term.KeyBackspace, "bs": term.KeyBackspace,
	"esc": term.KeyEscape, "escape": term.KeyEscape,
	"up": term.KeyUp, "down": term.KeyDown, "left": term.KeyLeft, "right": term.KeyRight,
	"home": term.KeyHome, "end": term.KeyEnd,
	"pgup": term.KeyPageUp, "pageup": term.KeyPageUp,
	"pgdn": term.KeyPageDown, "pagedown": term.KeyPageDown,
	"insert": term.KeyInsert, "ins": term.KeyInsert,
	"delete": term.KeyDelete, "del": term.KeyDelete,
}

// Key reads one key as a script names it, without the angle brackets:
// "Ctrl+A", "shift+enter", "F5", "x".
func Key(name string) (term.Key, error) {
	if name == "" {
		return term.Key{}, fmt.Errorf("keyscript: empty key <>")
	}
	// The key is what follows the last "+", unless the key is "+" itself.
	base, mods := name, ""
	if i := strings.LastIndex(name[:len(name)-1], "+"); i >= 0 {
		base, mods = name[i+1:], name[:i]
	}
	var mod term.Mod
	if mods != "" {
		for _, m := range strings.Split(mods, "+") {
			bit, ok := modNames[strings.ToLower(m)]
			if !ok {
				return term.Key{}, fmt.Errorf("keyscript: unknown modifier %q in <%s>", m, name)
			}
			mod |= bit
		}
	}
	lower := strings.ToLower(base)
	if code, ok := keyNames[lower]; ok {
		return term.Key{Code: code, Mod: mod}, nil
	}
	if lower == "space" {
		base = " "
	} else if len(lower) > 1 && lower[0] == 'f' {
		if n, err := strconv.Atoi(lower[1:]); err == nil && n >= 1 && n <= 35 {
			return term.Key{Code: term.KeyF1 + term.KeyCode(n-1), Mod: mod}, nil
		}
	}
	r, size := utf8.DecodeRuneInString(base)
	if size != len(base) {
		return term.Key{}, fmt.Errorf("keyscript: unknown key %q in <%s>", base, name)
	}
	k := term.Key{Code: term.KeyRune, Rune: r, Mod: mod}
	switch {
	case mod == 0:
		k.Text = string(r)
	case mod == term.ModShift:
		// Shift alone still types: the shifted character, with the key
		// named in lower case, as the kitty protocol reports it.
		k.Text = string(unicode.ToUpper(r))
		k.Rune = unicode.ToLower(r)
	default:
		k.Rune = unicode.ToLower(r)
	}
	return k, nil
}

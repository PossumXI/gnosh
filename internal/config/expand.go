package config

import (
	"fmt"
	"slices"
	"strings"
)

// filled is what filling in the environment variables of one string value
// gave.
type filled struct {
	// text is the value with every variable filled in. A variable that is
	// not set and has no default is filled in with nothing, and named in
	// unset: the value is then not to be used.
	text string
	// vars are the variables the value names, in the order it names them,
	// each once.
	vars []string
	// unset are those of them that are not set and have no default.
	unset []string
	// only is the name of the one variable the value is made of, when it is
	// nothing but one ${NAME} and that variable is set. It is how a key
	// that came from the environment is shown: by the variable's name.
	only string
}

// fill fills in the environment variables of a string value of a provider.
// ${NAME} is the variable's value, and ${NAME:-default} is the text after
// :- when the variable is not set; a variable that is set and empty counts
// as set. That is the form Claude Code documents for its own files. A
// dollar sign that is not followed by a brace is plain text, so $NAME is
// left as written.
//
// The second result says what is wrong with the value as it is written,
// in a sentence that begins with the value's key, or is "".
func fill(text string, getenv func(string) (string, bool)) (filled, string) {
	var out filled
	var b strings.Builder
	rest := text
	for {
		i := strings.Index(rest, "${")
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		end := strings.IndexByte(rest[i:], '}')
		if end < 0 {
			return out, "holds ${ with no } after it. A variable is written ${NAME}, or ${NAME:-default} for the text to use when it is not set"
		}
		inner := rest[i+2 : i+end]
		rest = rest[i+end+1:]
		name, def, hasDefault := inner, "", false
		if k := strings.Index(inner, ":-"); k >= 0 {
			name, def, hasDefault = inner[:k], inner[k+2:], true
		}
		switch {
		case !isName(name):
			// What stands there is not shown: it may be a key that was
			// pasted between the braces.
			return out, "holds ${ and }, and what stands between them is not a variable's name. A variable is written ${NAME}, or ${NAME:-default} for the text to use when it is not set; nothing else is filled in"
		case runTimeVariable(name):
			// gnosh sets this one itself, anew for each hook and command
			// it runs. Filling it in now would bake in nothing, or the
			// value of some other session that happens to be in the
			// environment.
			return out, fmt.Sprintf("names the variable %s, which gnosh sets for each hook and command it runs. It has no value of this session's when the file is read, so it cannot be filled in here", name)
		}
		if !slices.Contains(out.vars, name) {
			out.vars = append(out.vars, name)
		}
		value, set := getenv(name)
		switch {
		case set:
			b.WriteString(value)
			if i == 0 && rest == "" && b.Len() == len(value) {
				out.only = name
			}
		case hasDefault:
			b.WriteString(def)
		case !slices.Contains(out.unset, name):
			out.unset = append(out.unset, name)
		}
	}
	out.text = b.String()
	if len(out.vars) != 1 {
		out.only = ""
	}
	return out, ""
}

package fstools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// failure is a refusal the model can do something about: an argument that
// makes no sense, a file that is not there, text that does not match. A
// tool's Run turns it into an error result; its Describe returns it as the
// error.
type failure struct {
	msg string
}

// Error returns the message for the model.
func (f *failure) Error() string { return f.msg }

func failf(format string, a ...any) *failure {
	return &failure{msg: fmt.Sprintf(format, a...)}
}

// refused turns a failure into what Run returns for it.
func refused(f *failure) (core.ToolResult, error) {
	return core.ErrorResult(f.msg), nil
}

// asError turns a failure into what Describe returns for it. A nil failure
// must become a nil error, not an error that holds a nil pointer.
func asError(f *failure) error {
	if f == nil {
		return nil
	}
	return f
}

// argSet is the arguments of one tool call.
type argSet struct {
	tool   string
	fields map[string]json.RawMessage
}

// parseArgs reads a call's input, which must be one JSON object, and
// refuses any argument the tool does not take: a misspelt or made-up
// argument that was silently dropped would change what the call means.
func parseArgs(tool string, input json.RawMessage, names ...string) (*argSet, *failure) {
	a := &argSet{tool: tool}
	if len(bytes.TrimSpace(input)) > 0 {
		if err := json.Unmarshal(input, &a.fields); err != nil {
			return nil, failf("%s: the arguments are not one JSON object: %v", tool, err)
		}
	}
	for key := range a.fields {
		known := false
		for _, n := range names {
			if key == n {
				known = true
				break
			}
		}
		if !known {
			return nil, failf("%s takes no argument called %q. Its arguments are: %s.", tool, key, strings.Join(names, ", "))
		}
	}
	return a, nil
}

// raw returns the value of an argument, and false when it was left out or
// given as null.
func (a *argSet) raw(name string) (json.RawMessage, bool) {
	v, ok := a.fields[name]
	if !ok {
		return nil, false
	}
	v = bytes.TrimSpace(v)
	if len(v) == 0 || bytes.Equal(v, []byte("null")) {
		return nil, false
	}
	return v, true
}

// text returns a string argument. given is false when it was left out.
func (a *argSet) text(name string) (value string, given bool, f *failure) {
	v, ok := a.raw(name)
	if !ok {
		return "", false, nil
	}
	if err := json.Unmarshal(v, &value); err != nil {
		return "", false, failf("%s: %s must be a string, not %s.", a.tool, name, jsonKind(v))
	}
	return value, true, nil
}

// required returns a string argument that must be there. It may be empty.
func (a *argSet) required(name string) (string, *failure) {
	value, given, f := a.text(name)
	if f != nil {
		return "", f
	}
	if !given {
		return "", failf("%s: %s is required.", a.tool, name)
	}
	return value, nil
}

// whole returns an argument that is a whole number of zero or more. A
// number written as a string ("20") or with an empty fraction (20.0) is
// taken as the number it plainly is.
func (a *argSet) whole(name string) (value int, given bool, f *failure) {
	v, ok := a.raw(name)
	if !ok {
		return 0, false, nil
	}
	s := string(v)
	if v[0] == '"' {
		if err := json.Unmarshal(v, &s); err != nil {
			return 0, false, failf("%s: %s must be a whole number.", a.tool, name)
		}
		s = strings.TrimSpace(s)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		fl, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil || math.IsNaN(fl) || math.IsInf(fl, 0) || fl != math.Trunc(fl) || math.Abs(fl) > 1<<53 {
			return 0, false, failf("%s: %s must be a whole number, not %s.", a.tool, name, short(string(v), 40))
		}
		n = int64(fl)
	}
	if n < 0 {
		return 0, false, failf("%s: %s must not be negative (it is %d).", a.tool, name, n)
	}
	if n > math.MaxInt32 {
		n = math.MaxInt32
	}
	return int(n), true, nil
}

// flag returns a true-or-false argument; left out, it is false. The words
// "true" and "false" in a string are taken as meant.
func (a *argSet) flag(name string) (value bool, given bool, f *failure) {
	v, ok := a.raw(name)
	if !ok {
		return false, false, nil
	}
	s := string(v)
	if v[0] == '"' {
		if err := json.Unmarshal(v, &s); err != nil {
			return false, false, failf("%s: %s must be true or false.", a.tool, name)
		}
		s = strings.ToLower(strings.TrimSpace(s))
	}
	switch s {
	case "true":
		return true, true, nil
	case "false":
		return false, true, nil
	}
	return false, false, failf("%s: %s must be true or false, not %s.", a.tool, name, short(string(v), 40))
}

// jsonKind names what kind of JSON value v is, for messages.
func jsonKind(v []byte) string {
	if len(v) == 0 {
		return "nothing"
	}
	switch v[0] {
	case '{':
		return "an object"
	case '[':
		return "a list"
	case '"':
		return "a string"
	case 't', 'f':
		return "true or false"
	}
	return "a number"
}

// short cuts s to at most max bytes for use inside a message, on a
// character boundary, and says so with an ellipsis.
func short(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !isRuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "\u2026"
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// prop is one argument in a tool's input schema.
type prop struct {
	name string
	// kind is the JSON type: "string", "integer" or "boolean".
	kind string
	desc string
	enum []string
}

// schema writes the JSON Schema of a tool's arguments, keeping the
// arguments in the order given so the model reads the main one first.
func schema(required []string, props ...prop) json.RawMessage {
	var b bytes.Buffer
	str := func(s string) {
		enc, _ := json.Marshal(s)
		b.Write(enc)
	}
	b.WriteString(`{"type":"object","properties":{`)
	for i, p := range props {
		if i > 0 {
			b.WriteByte(',')
		}
		str(p.name)
		b.WriteString(`:{"type":`)
		str(p.kind)
		if len(p.enum) > 0 {
			b.WriteString(`,"enum":[`)
			for j, e := range p.enum {
				if j > 0 {
					b.WriteByte(',')
				}
				str(e)
			}
			b.WriteByte(']')
		}
		b.WriteString(`,"description":`)
		str(p.desc)
		b.WriteByte('}')
	}
	b.WriteString(`},"required":[`)
	for i, r := range required {
		if i > 0 {
			b.WriteByte(',')
		}
		str(r)
	}
	b.WriteString(`],"additionalProperties":false}`)
	return b.Bytes()
}

// propNames lists the names of props, for parseArgs.
func propNames(props []prop) []string {
	names := make([]string, len(props))
	for i, p := range props {
		names[i] = p.name
	}
	return names
}

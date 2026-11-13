package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// jkind is the kind of a JSON value.
type jkind int

const (
	jObject jkind = iota
	jArray
	jString
	jNumber
	jBool
	jNull
)

// word is the kind in a message: "a list", "a string".
func (k jkind) word() string {
	switch k {
	case jObject:
		return "an object"
	case jArray:
		return "a list"
	case jString:
		return "a string"
	case jNumber:
		return "a number"
	case jBool:
		return "true or false"
	}
	return "null"
}

// jnode is a JSON value read from a settings file, with the line it is on.
// Objects keep their keys in the file's order, repeats included, so that a
// key given twice can be reported instead of one copy winning quietly.
type jnode struct {
	kind   jkind
	line   int
	fields []jfield // an object's members
	items  []*jnode // a list's items
	str    string   // a string's value, or a number as written
	b      bool
}

// jfield is one member of a JSON object.
type jfield struct {
	key  string
	line int
	val  *jnode
}

// jsonError is a mistake in the JSON itself, with its line.
type jsonError struct {
	line int
	msg  string
}

// maxJSONDepth is how deep objects and lists may nest in a settings file.
const maxJSONDepth = 64

// parseJSON reads strict JSON (no comments, no trailing commas) into a
// tree that remembers lines.
func parseJSON(data []byte) (*jnode, *jsonError) {
	var starts []int // the offset at which each line after the first starts
	for i, c := range data {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	lineAt := func(offset int64) int {
		return 1 + sort.Search(len(starts), func(i int) bool { return int64(starts[i]) > offset })
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	here := func() int { return lineAt(max(dec.InputOffset()-1, 0)) }
	fail := func(err error) *jsonError {
		var syn *json.SyntaxError
		switch {
		case errors.As(err, &syn):
			// The offset is at the offending byte or just past it,
			// depending on which part of the decoder noticed. Either way
			// the nearest byte that is not white space, looking back, is
			// on the offending line.
			pos := min(syn.Offset, int64(len(data))-1)
			if strings.Contains(syn.Error(), "in string literal") {
				pos = syn.Offset - 1
			}
			for pos > 0 && (data[pos] == ' ' || data[pos] == '\t' || data[pos] == '\n' || data[pos] == '\r') {
				pos--
			}
			return &jsonError{line: lineAt(max(pos, 0)), msg: syn.Error()}
		case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			return &jsonError{line: here(), msg: "it ends before the JSON does"}
		}
		return &jsonError{line: here(), msg: err.Error()}
	}

	var value func(depth int) (*jnode, *jsonError)
	value = func(depth int) (*jnode, *jsonError) {
		tok, err := dec.Token()
		if err != nil {
			return nil, fail(err)
		}
		n := &jnode{line: here()}
		switch t := tok.(type) {
		case json.Delim:
			if depth >= maxJSONDepth {
				return nil, &jsonError{line: n.line, msg: fmt.Sprintf("it nests more than %d levels deep", maxJSONDepth)}
			}
			switch t {
			case '{':
				n.kind = jObject
				for dec.More() {
					keyTok, err := dec.Token()
					if err != nil {
						return nil, fail(err)
					}
					key, ok := keyTok.(string)
					if !ok {
						return nil, &jsonError{line: here(), msg: "an object's key is not a string"}
					}
					f := jfield{key: key, line: here()}
					var jerr *jsonError
					if f.val, jerr = value(depth + 1); jerr != nil {
						return nil, jerr
					}
					n.fields = append(n.fields, f)
				}
			case '[':
				n.kind = jArray
				for dec.More() {
					item, jerr := value(depth + 1)
					if jerr != nil {
						return nil, jerr
					}
					n.items = append(n.items, item)
				}
			default:
				return nil, &jsonError{line: n.line, msg: "a bracket closes what was never opened"}
			}
			if _, err := dec.Token(); err != nil { // the closing bracket
				return nil, fail(err)
			}
		case string:
			n.kind, n.str = jString, t
		case json.Number:
			n.kind, n.str = jNumber, t.String()
		case bool:
			n.kind, n.b = jBool, t
		default:
			n.kind = jNull
		}
		return n, nil
	}

	root, jerr := value(0)
	if jerr != nil {
		return nil, jerr
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, &jsonError{line: here(), msg: "there is more text after the end of the JSON value"}
	}
	return root, nil
}

// strings returns a list's items when every one is a string.
func (n *jnode) strings() ([]*jnode, bool) {
	if n.kind != jArray {
		return nil, false
	}
	for _, it := range n.items {
		if it.kind != jString {
			return nil, false
		}
	}
	return n.items, true
}

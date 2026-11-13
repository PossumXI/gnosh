package llm

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
)

// obj is a JSON object being put together.
type obj = map[string]any

// merge writes src into dst the way Config.Extra and Request.Extra are
// documented to work: where both hold an object under a key the two are
// merged, a nil value removes the key, and anything else replaces what was
// there. Nothing reachable from src is changed or kept: objects are copied
// on their way in.
func merge(dst, src obj) {
	for k, v := range src {
		switch v := v.(type) {
		case nil:
			delete(dst, k)
		case map[string]any:
			if old, ok := dst[k].(map[string]any); ok {
				merge(old, v)
			} else {
				dst[k] = cloneObj(v)
			}
		default:
			dst[k] = v
		}
	}
}

// cloneObj copies a JSON object and every object inside it. Lists and
// plain values are shared, because nothing here changes them.
func cloneObj(m map[string]any) obj {
	if m == nil {
		return nil
	}
	out := make(obj, len(m))
	for k, v := range m {
		if sub, ok := v.(map[string]any); ok {
			v = cloneObj(sub)
		}
		out[k] = v
	}
	return out
}

// encodeJSON is json.Marshal without the escaping of <, > and & that only
// matters inside HTML. A prompt full of tags stays readable in a trace and
// costs fewer bytes.
func encodeJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// objectJSON returns raw if it is one JSON object, compacted, and "{}"
// otherwise. Tool-call arguments in a conversation are meant to be an
// object; one that is not is sent as the empty object, so that a single
// bad block cannot make a whole conversation unsendable.
func objectJSON(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '{' || !json.Valid(raw) {
		return json.RawMessage("{}")
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return json.RawMessage("{}")
	}
	return buf.Bytes()
}

// jsonString returns the string raw holds, if raw is a JSON string.
func jsonString(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// isJSONNull reports whether raw is absent or the JSON null.
func isJSONNull(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) == 0 || bytes.Equal(raw, []byte("null"))
}

// str is a string as servers write them. A field that is meant to hold a
// string sometimes holds a number or null instead, and a plain string
// field would make the whole event unreadable for that. A str reads a
// string, takes a number or a boolean as its text, and anything else as
// empty.
type str string

// UnmarshalJSON reads a JSON string, or the text of a number or boolean.
func (s *str) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch {
	case len(b) == 0:
		*s = ""
	case b[0] == '"':
		var v string
		if json.Unmarshal(b, &v) != nil {
			v = ""
		}
		*s = str(v)
	case b[0] == '{' || b[0] == '[' || bytes.Equal(b, []byte("null")):
		*s = ""
	default:
		*s = str(b)
	}
	return nil
}

// num is a number as servers write them. Token counts and indexes are
// whole numbers, but some servers write 12.0, and a few write "12"; a
// plain int field would make the whole event unreadable for that. A num
// reads all of those and takes anything else as zero.
type num float64

// UnmarshalJSON reads a JSON number or a string holding one.
func (n *num) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) >= 2 && b[0] == '"' {
		b = b[1 : len(b)-1]
	}
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		f = 0
	}
	*n = num(f)
	return nil
}

// count returns n as a count: never negative, never past what an int holds.
func (n num) count() int {
	if n <= 0 {
		return 0
	}
	if n >= math.MaxInt32 {
		return math.MaxInt32
	}
	return int(n)
}

// countOf returns the count n points at, or zero if there is none.
func countOf(n *num) int {
	if n == nil {
		return 0
	}
	return n.count()
}

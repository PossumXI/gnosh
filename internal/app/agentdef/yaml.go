package agentdef

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// This file reads the indented block under a setting of a frontmatter (a
// skill's hooks) as the YAML it is written in, and gives it back as JSON.
// It takes the part of YAML that such a block is written in, and refuses
// the rest by line: maps of keys and values, lists ("- "), a list item
// that opens a map ("- matcher: Bash"), plain and quoted scalars, true,
// false, null, numbers, flow lists of scalars ("[a, b]") and the empty
// map ("{}"). Anchors, tags, multi-line strings and flow maps with
// entries are refused: a setting that cannot be read is not guessed at.

// BlockJSON reads the lines of a block as YAML and returns the JSON of its
// value.
func BlockJSON(lines []string) (json.RawMessage, error) {
	var ls []yline
	for i, raw := range lines {
		t := strings.TrimSpace(raw)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		lead := raw[:len(raw)-len(strings.TrimLeft(raw, " \t"))]
		if strings.Contains(lead, "\t") {
			return nil, fmt.Errorf("line %d is indented with a tab, which YAML does not take", i+1)
		}
		ls = append(ls, yline{indent: len(lead), text: t, n: i + 1})
	}
	if len(ls) == 0 {
		return json.RawMessage("null"), nil
	}
	p := &yparser{ls: ls}
	v, err := p.value(ls[0].indent)
	if err != nil {
		return nil, err
	}
	if p.i < len(p.ls) {
		return nil, fmt.Errorf("line %d does not line up with what is above it", p.ls[p.i].n)
	}
	return json.Marshal(v)
}

// yline is one line of a block that holds something.
type yline struct {
	indent int
	text   string
	n      int
}

type yparser struct {
	ls []yline
	i  int
}

// isItem reports whether a line is a list item.
func isItem(t string) bool { return t == "-" || strings.HasPrefix(t, "- ") }

// value reads the map or the list that starts at the current line, which
// stands at indent.
func (p *yparser) value(indent int) (any, error) {
	if l := p.ls[p.i]; l.indent != indent {
		return nil, fmt.Errorf("line %d does not line up with what is above it", l.n)
	}
	if isItem(p.ls[p.i].text) {
		return p.list(indent)
	}
	return p.mapping(indent)
}

// list reads the items that stand at indent.
func (p *yparser) list(indent int) ([]any, error) {
	out := []any{}
	for p.i < len(p.ls) && p.ls[p.i].indent == indent && isItem(p.ls[p.i].text) {
		l := p.ls[p.i]
		rest := strings.TrimSpace(strings.TrimPrefix(l.text, "-"))
		switch {
		case rest == "":
			// The item is the block below it.
			p.i++
			if p.i < len(p.ls) && p.ls[p.i].indent > indent {
				v, err := p.value(p.ls[p.i].indent)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			} else {
				out = append(out, nil)
			}
		case isKeyLine(rest):
			// "- key: value" opens a map whose keys stand where key does.
			col := l.indent + len(l.text) - len(rest)
			p.ls[p.i] = yline{indent: col, text: rest, n: l.n}
			m, err := p.mapping(col)
			if err != nil {
				return nil, err
			}
			out = append(out, m)
		default:
			v, err := scalarValue(rest, l.n)
			if err != nil {
				return nil, err
			}
			p.i++
			out = append(out, v)
		}
	}
	return out, nil
}

// mapping reads the keys that stand at indent.
func (p *yparser) mapping(indent int) (map[string]any, error) {
	m := map[string]any{}
	for p.i < len(p.ls) && p.ls[p.i].indent == indent && !isItem(p.ls[p.i].text) {
		l := p.ls[p.i]
		key, rest, ok := cutYAMLKey(l.text)
		if !ok {
			return nil, fmt.Errorf("line %d is not of the form \"name: value\"", l.n)
		}
		if _, twice := m[key]; twice {
			return nil, fmt.Errorf("line %d gives %q a second time", l.n, key)
		}
		p.i++
		if rest = dropComment(rest); rest != "" {
			v, err := scalarValue(rest, l.n)
			if err != nil {
				return nil, err
			}
			m[key] = v
			continue
		}
		switch {
		case p.i < len(p.ls) && p.ls[p.i].indent > indent:
			v, err := p.value(p.ls[p.i].indent)
			if err != nil {
				return nil, err
			}
			m[key] = v
		case p.i < len(p.ls) && p.ls[p.i].indent == indent && isItem(p.ls[p.i].text):
			// A list may stand right under its key, not indented.
			v, err := p.list(indent)
			if err != nil {
				return nil, err
			}
			m[key] = v
		default:
			m[key] = nil
		}
	}
	return m, nil
}

// isKeyLine reports whether a line reads as "key: value" or "key:".
func isKeyLine(t string) bool {
	_, _, ok := cutYAMLKey(t)
	return ok
}

// cutYAMLKey splits "key: value" (the key plain or quoted) into its two
// sides.
func cutYAMLKey(t string) (key, rest string, ok bool) {
	if t != "" && (t[0] == '"' || t[0] == '\'') {
		end := strings.IndexByte(t[1:], t[0])
		if end < 0 {
			return "", "", false
		}
		key, after := t[1:end+1], strings.TrimSpace(t[end+2:])
		if !strings.HasPrefix(after, ":") {
			return "", "", false
		}
		return key, strings.TrimSpace(after[1:]), true
	}
	if i := strings.Index(t, ": "); i > 0 {
		return strings.TrimSpace(t[:i]), strings.TrimSpace(t[i+2:]), !strings.ContainsAny(t[:i], "[]{}\"'")
	}
	if strings.HasSuffix(t, ":") && len(t) > 1 && !strings.ContainsAny(t, " []{}\"'") {
		return t[:len(t)-1], "", true
	}
	return "", "", false
}

// dropComment takes a trailing comment (" # ...") off a value: after the
// closing quote of a quoted one, and from the first " #" of a plain one.
func dropComment(v string) string {
	if v == "" || v[0] == '#' {
		return ""
	}
	if q := v[0]; q == '"' || q == '\'' {
		for i := 1; i < len(v); i++ {
			switch {
			case q == '"' && v[i] == '\\':
				i++
			case v[i] == q && q == '\'' && i+1 < len(v) && v[i+1] == '\'':
				i++
			case v[i] == q:
				if rest := strings.TrimSpace(v[i+1:]); rest == "" || strings.HasPrefix(rest, "#") {
					return v[:i+1]
				}
				return v
			}
		}
		return v
	}
	if i := strings.Index(v, " #"); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}

// jsonNumber matches a number as JSON writes one.
var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// scalarValue reads a value written on a key's line or an item's.
func scalarValue(v string, n int) (any, error) {
	if v = dropComment(v); v == "" {
		return nil, nil
	}
	switch {
	case strings.HasPrefix(v, `"`):
		s, err := strconv.Unquote(v)
		if err != nil {
			return nil, fmt.Errorf("the quoted text on line %d cannot be read", n)
		}
		return s, nil
	case strings.HasPrefix(v, "'"):
		if len(v) < 2 || !strings.HasSuffix(v, "'") {
			return nil, fmt.Errorf("the quoted text on line %d is not closed", n)
		}
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'"), nil
	case v == "{}":
		return map[string]any{}, nil
	case strings.HasPrefix(v, "{"):
		return nil, fmt.Errorf("line %d writes a map in braces, which is not read here; write it on lines of its own", n)
	case strings.HasPrefix(v, "["):
		if !strings.HasSuffix(v, "]") {
			return nil, fmt.Errorf("the list on line %d is not closed", n)
		}
		out := []any{}
		for _, part := range strings.Split(v[1:len(v)-1], ",") {
			if part = strings.TrimSpace(part); part == "" {
				continue
			}
			item, err := scalarValue(part, n)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, nil
	case strings.HasPrefix(v, "&"), strings.HasPrefix(v, "*"), strings.HasPrefix(v, "!"), v == "|", v == ">", strings.HasPrefix(v, "|"), strings.HasPrefix(v, ">"):
		return nil, fmt.Errorf("line %d uses a form of YAML that is not read here (anchors, tags and text blocks)", n)
	}
	switch v {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	case "null", "Null", "NULL", "~":
		return nil, nil
	}
	if jsonNumber.MatchString(v) {
		return json.Number(v), nil
	}
	return v, nil
}

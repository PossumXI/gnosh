package prompt

import (
	"fmt"
	"slices"
	"strings"
)

// A template is one of gnosh's texts, read once and filled in many times.
//
// Three things in a text are not plain words:
//
//	{{name}}    is replaced by the value given for name
//	{{name?}}   prints nothing: it only asks whether name has a value
//	[[ ... ]]   is a part that is kept when every name inside it has a
//	            value, and left out whole when one of them has none
//
// A part cannot hold another part, and it must hold at least one name. A
// "{{" always starts a name, so a text cannot say "{{" or "[[" as plain
// words; "}}" and "]]" outside a part are plain words.
type template struct {
	nodes []node
}

// A node is a piece of a template: plain words, a name to fill in, or an
// optional part made of the first two.
type node struct {
	words string // plain words, when name is empty and part is false
	name  string // a name to fill in
	ask   bool   // the name is only asked about and prints nothing
	part  bool   // an optional part, whose pieces are in kids
	kids  []node
}

// parseTemplate reads text as a template. An error says which line is
// wrong and what would be right.
func parseTemplate(text string) (template, error) {
	var (
		top    []node
		kids   []node
		inPart bool
		opened int // where the open part began
		from   int // where the plain words not yet stored begin
	)
	store := func(n node) {
		if inPart {
			kids = append(kids, n)
		} else {
			top = append(top, n)
		}
	}
	words := func(to int) {
		if to > from {
			store(node{words: text[from:to]})
		}
	}
	for i := 0; i < len(text); {
		rest := text[i:]
		switch {
		case strings.HasPrefix(rest, "{{"):
			end := strings.Index(rest, "}}")
			if end < 0 {
				return template{}, fmt.Errorf("line %d: \"{{\" is never closed: a name to fill in is written {{name}}", lineOf(text, i))
			}
			name := strings.TrimSpace(rest[2:end])
			ask := strings.HasSuffix(name, "?")
			name = strings.TrimSpace(strings.TrimSuffix(name, "?"))
			if !validName(name) {
				return template{}, fmt.Errorf("line %d: %q is not a name to fill in: a name is lowercase letters, digits and \"_\", written {{name}}", lineOf(text, i), clip(rest[:end+2], 60))
			}
			words(i)
			store(node{name: name, ask: ask})
			i += end + 2
			from = i
		case strings.HasPrefix(rest, "[["):
			if inPart {
				return template{}, fmt.Errorf("line %d: \"[[\" inside a part that began on line %d: an optional part cannot hold another", lineOf(text, i), lineOf(text, opened))
			}
			words(i)
			inPart, opened, kids = true, i, nil
			i += 2
			from = i
		case inPart && strings.HasPrefix(rest, "]]"):
			words(i)
			if !slices.ContainsFunc(kids, func(n node) bool { return n.name != "" }) {
				return template{}, fmt.Errorf("line %d: the part in \"[[ ]]\" holds no {{name}}: a part is kept or left out by the names inside it", lineOf(text, opened))
			}
			inPart = false
			store(node{part: true, kids: kids})
			i += 2
			from = i
		default:
			i++
		}
	}
	if inPart {
		return template{}, fmt.Errorf("line %d: \"[[\" is never closed with \"]]\"", lineOf(text, opened))
	}
	words(len(text))
	return template{nodes: top}, nil
}

// lineOf is the number of the line that byte i of text is on, from 1.
func lineOf(text string, i int) int {
	return 1 + strings.Count(text[:i], "\n")
}

// validName reports whether s can name a value: a lowercase letter, then
// lowercase letters, digits and underscores.
func validName(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

// names lists the names the template uses, sorted: printed holds the ones
// whose value is written into the text, and asked the ones that are only
// asked about.
func (t template) names() (printed, asked []string) {
	var walk func([]node)
	walk = func(nodes []node) {
		for _, n := range nodes {
			switch {
			case n.part:
				walk(n.kids)
			case n.name != "" && n.ask:
				asked = append(asked, n.name)
			case n.name != "":
				printed = append(printed, n.name)
			}
		}
	}
	walk(t.nodes)
	slices.Sort(printed)
	slices.Sort(asked)
	return slices.Compact(printed), slices.Compact(asked)
}

// values are what a template is filled with, by name. A name with no value,
// or a value of nothing but blanks, counts as not given.
type values map[string]string

// flag marks name as given when on is true, for a {{name?}} to ask about.
func (v values) flag(name string, on bool) {
	if on {
		v[name] = "yes"
	}
}

// fill writes the template out with vals in its names. Every value goes
// through Untrusted on its way in, so that nothing a value holds can pass
// for a control sequence or for gnosh's own tag, and a value is never read
// as template text itself.
func (t template) fill(vals values) string {
	var b strings.Builder
	writeNodes(&b, t.nodes, vals, nil, Untrusted)
	return b.String()
}

// format writes the template out as a format for fmt.Sprintf: every "%"
// in its own words is doubled, and the values, which are verbs or plain
// words of the caller's, go in as they are.
func (t template) format(vals values) string {
	var b strings.Builder
	writeNodes(&b, t.nodes, vals, func(s string) string { return strings.ReplaceAll(s, "%", "%%") }, nil)
	return b.String()
}

// writeNodes writes nodes to b with vals in their names. words, when not
// nil, is applied to the plain words, and value to each value.
func writeNodes(b *strings.Builder, nodes []node, vals values, words, value func(string) string) {
	for _, n := range nodes {
		switch {
		case n.part:
			if given(n.kids, vals) {
				writeNodes(b, n.kids, vals, words, value)
			}
		case n.name != "":
			if n.ask {
				continue
			}
			v := vals[n.name]
			if value != nil {
				v = value(v)
			}
			b.WriteString(v)
		default:
			w := n.words
			if words != nil {
				w = words(w)
			}
			b.WriteString(w)
		}
	}
}

// given reports whether every name among nodes has a value.
func given(nodes []node, vals values) bool {
	for _, n := range nodes {
		if n.name != "" && strings.TrimSpace(vals[n.name]) == "" {
			return false
		}
	}
	return true
}

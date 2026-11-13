package cli

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// splitRules cuts one argument of a tool list or a rule list into its
// items. Commas and white space separate items, except inside parentheses,
// so "Bash(git diff *),Edit" is two rules and "Bash(git log *, git show *)"
// is one. Parentheses are counted as they open and close; a closing one
// with nothing open is an ordinary character.
//
// When a parenthesis is opened and never closed, the item it is in runs to
// the end of the argument: a rule's pattern may hold a lone "(". But if a
// ")" inside that item is followed by a comma or a space, the argument could
// as well be two rules as one, and splitRules says so in problem instead of
// guessing: a deny rule lost to a wrong guess would let through what it was
// written to stop.
func splitRules(arg string) (items []string, problem string) {
	depth := 0
	start := -1 // where the item being read began, or -1 between items
	for i, r := range arg {
		switch {
		case depth == 0 && (r == ',' || unicode.IsSpace(r)):
			if start >= 0 {
				items = append(items, arg[start:i])
				start = -1
			}
			continue
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		}
		if start < 0 {
			start = i
		}
	}
	if start < 0 {
		return items, ""
	}
	last := arg[start:]
	if depth > 0 {
		for i, r := range last {
			if r != ')' {
				continue
			}
			rest := last[i+1:]
			if rest == "" {
				continue
			}
			if next, _ := utf8.DecodeRuneInString(rest); next == ',' || unicode.IsSpace(next) {
				return nil, "its parentheses do not pair up, so it is not clear where one rule ends and the next begins; give each rule as an argument of its own"
			}
		}
		last = strings.TrimRightFunc(last, unicode.IsSpace)
	}
	return append(items, last), ""
}

// spaceOutsideParens reports whether arg has white space that is not inside
// parentheses, counting them as splitRules does. A rule never has any; a
// prompt of two or more words always does.
func spaceOutsideParens(arg string) bool {
	depth := 0
	for _, r := range arg {
		switch {
		case r == '(':
			depth++
		case r == ')' && depth > 0:
			depth--
		case depth == 0 && unicode.IsSpace(r):
			return true
		}
	}
	return false
}

// hasSpace reports whether s has any white space in it.
func hasSpace(s string) bool {
	return strings.IndexFunc(s, unicode.IsSpace) >= 0
}

// blank reports whether s is empty or nothing but white space.
func blank(s string) bool {
	return strings.TrimSpace(s) == ""
}

// more reports whether next, an argument that follows a list flag's values,
// is one more value of the list. An empty argument and one that begins with
// a dash never are.
func (k listKind) more(next string) bool {
	if next == "" || strings.HasPrefix(next, "-") {
		return false
	}
	switch k {
	case listRules:
		return !spaceOutsideParens(next)
	case listPaths:
		return !hasSpace(next)
	case listPairs:
		return isEnvPair(next) == ""
	case listHeaders:
		return isHeader(next) == ""
	}
	return false
}

// looksLikeRules reports whether an argument of two or more words reads
// like a list of tools or rules, in the way Claude Code writes them, rather
// than like a prompt: every item is a capitalized name (Bash, WebFetch), a
// name with a pattern in parentheses, an MCP tool's name, or "*". It is used
// where such an argument would otherwise be taken as the prompt, to ask
// which was meant instead of guessing: a deny rule read as a prompt would
// let through what it was written to stop.
func looksLikeRules(arg string) bool {
	items, problem := splitRules(arg)
	if problem != "" || len(items) < 2 {
		return false
	}
	for _, it := range items {
		if !ruleShaped(it) {
			return false
		}
	}
	return true
}

// ruleShaped reports whether one item is written like a tool or a rule.
func ruleShaped(it string) bool {
	if it == "*" {
		return true
	}
	if open := strings.IndexByte(it, '('); open >= 0 {
		return strings.HasSuffix(it, ")") && nameShaped(it[:open], false)
	}
	if strings.HasPrefix(it, "mcp__") {
		return nameShaped(it, true)
	}
	return it[0] >= 'A' && it[0] <= 'Z' && nameShaped(it, false)
}

// nameShaped reports whether s is letters, digits and underscores, starting
// with a letter or an underscore; with stars, also "*" anywhere after the
// start, as in mcp__files__read_*.
func nameShaped(s string, stars bool) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z':
		case i > 0 && (c >= '0' && c <= '9' || stars && c == '*'):
		default:
			return false
		}
	}
	return true
}

// isUUID returns what is wrong with s as a UUID (8-4-4-4-12 hexadecimal
// digits with dashes between), or "".
func isUUID(s string) string {
	const problem = "is not a UUID, which is 32 hexadecimal digits in groups of 8, 4, 4, 4 and 12 with dashes between (123e4567-e89b-12d3-a456-426614174000)"
	if len(s) != 36 {
		return problem
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return problem
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
				return problem
			}
		}
	}
	return ""
}

// isEnvPair returns what is wrong with s as a KEY=value pair, or "".
func isEnvPair(s string) string {
	key, _, ok := strings.Cut(s, "=")
	if !ok || key == "" {
		return "is not a KEY=value pair"
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		letter := c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z'
		if !letter && (i == 0 || c < '0' || c > '9') {
			return "is not a KEY=value pair: a variable's name is letters, digits and underscores, and does not begin with a digit"
		}
	}
	return ""
}

// isHeader returns what is wrong with s as an HTTP header, "Name: value", or
// "". A URL has a colon too, but what follows it is "//", which no header
// value begins with here.
func isHeader(s string) string {
	name, value, ok := strings.Cut(s, ":")
	if !ok || name == "" || strings.HasPrefix(value, "//") {
		return "is not a header, which is written Name: value"
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c <= ' ' || c >= 0x7f || strings.IndexByte(`()<>@,;:\"/[]?={}`, c) >= 0 {
			return "is not a header, which is written Name: value"
		}
	}
	return ""
}

// settingSources returns the files a --setting-sources value names, or what
// is wrong with it.
func settingSources(v string) (sources []string, problem string) {
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		switch part {
		case "user", "project", "local":
			for _, have := range sources {
				if have == part {
					return nil, "names " + part + " twice"
				}
			}
			sources = append(sources, part)
		case "":
			return nil, "has an empty item; it takes any of user, project and local with commas between"
		default:
			problem = "names " + shown(part) + ", and the settings files it can choose from are user, project and local"
			if near := nearest(part, []string{"user", "project", "local"}, slips(len(part))); near != "" {
				problem += " (did you mean " + near + "?)"
			}
			return nil, problem
		}
	}
	return sources, ""
}

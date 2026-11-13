package instructions

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file finds the imports in a file's text: each "@path" that is not
// inside code. It reads just enough Markdown to tell code from prose: fenced
// code blocks, and code spans between backticks. Indented code blocks are
// not treated as code, because without a full parser an indented line cannot
// be told from the continuation of a list item.

// A fence is the opening of a fenced code block: the character it was
// written with and how many of them.
type fence struct {
	ch byte
	n  int
}

// unquote takes the block-quote markers off the start of a line, so that
// what is quoted is read like any other line.
func unquote(line string) string {
	for {
		t := strings.TrimLeft(line, " \t")
		if t == "" || t[0] != '>' {
			return line
		}
		line = t[1:]
	}
}

// openFence reports whether line opens a fenced code block: three or more
// backticks or tildes at its start. Any indentation is allowed, since a
// fence inside a list item is indented to match it.
func openFence(line string) (fence, bool) {
	t := strings.TrimLeft(unquote(line), " \t")
	if len(t) < 3 || (t[0] != '`' && t[0] != '~') {
		return fence{}, false
	}
	n := 0
	for n < len(t) && t[n] == t[0] {
		n++
	}
	if n < 3 {
		return fence{}, false
	}
	// What follows a backtick fence names the language and can hold no
	// backtick; a line like ```x``` is a code span, not a fence.
	if t[0] == '`' && strings.IndexByte(t[n:], '`') >= 0 {
		return fence{}, false
	}
	return fence{ch: t[0], n: n}, true
}

// closedBy reports whether line closes the code block f opened: at least as
// many of the same character and nothing after them.
func (f fence) closedBy(line string) bool {
	t := strings.TrimLeft(unquote(line), " \t")
	n := 0
	for n < len(t) && t[n] == f.ch {
		n++
	}
	return n >= f.n && strings.TrimRight(t[n:], " \t\r\n") == ""
}

// lineKind sorts the lines outside code blocks by what they do to the
// paragraph before them. A code span can run from one line to the next only
// within a paragraph, so the scanner needs to know where paragraphs end.
type lineKind int

const (
	// linePlain carries on the paragraph or list item before it.
	linePlain lineKind = iota
	// lineBlank ends whatever came before.
	lineBlank
	// lineAlone is a block in one line: a heading, a rule, a table row.
	lineAlone
	// lineStarts begins a block that later lines may carry on: a list item.
	lineStarts
)

func classify(line string) lineKind {
	t := strings.TrimRight(strings.TrimLeft(unquote(line), " \t"), " \t\r\n")
	if t == "" {
		return lineBlank
	}
	switch c := t[0]; {
	case c == '#':
		n := 0
		for n < len(t) && t[n] == '#' {
			n++
		}
		if n <= 6 && (n == len(t) || t[n] == ' ' || t[n] == '\t') {
			return lineAlone
		}
	case c == '|':
		return lineAlone
	case c == '-' || c == '*' || c == '+' || c == '_' || c == '=':
		// A line of nothing but the one character: a rule, or the
		// underline of a heading.
		if strings.Trim(t, string(c)+" \t") == "" {
			return lineAlone
		}
		if c != '_' && c != '=' && (t[1] == ' ' || t[1] == '\t') {
			return lineStarts
		}
	case c >= '0' && c <= '9':
		n := 0
		for n < len(t) && n < 9 && t[n] >= '0' && t[n] <= '9' {
			n++
		}
		if n < len(t) && (t[n] == '.' || t[n] == ')') && (n+1 == len(t) || t[n+1] == ' ' || t[n+1] == '\t') {
			return lineStarts
		}
	}
	return linePlain
}

// scanImports returns the paths of the imports in text, as written (with
// backslash escapes taken out), in the order they appear.
//
// An import is "@" followed by a path. The "@" must come at the start of a
// line or after a space, "(" or "[": the "@" in an e-mail address or in
// "user@host" is not one. The path must start with a letter, a digit, ".",
// "/", "~" or "_". It runs to the next space or bracket, quote, backtick,
// "|" or "*"; a backslash makes the character after it part of the path, so
// "@Design\ Docs/api.md" names a path with a space. Sentence punctuation at
// the very end (". , ; : ! ?") is not part of the path. An import inside a
// fenced code block or a code span is not an import, and neither is one
// after a backslash ("\@x").
func scanImports(text string) []string {
	if strings.IndexByte(text, '@') < 0 {
		return nil
	}
	var refs []string
	var open fence
	inFence := false
	start := -1 // where the paragraph being gathered began, or -1
	flush := func(end int) {
		if start >= 0 {
			refs = scanSpan(text[start:end], refs)
			start = -1
		}
	}
	for i := 0; i < len(text); {
		end := lineEnd(text, i)
		line := text[i:end]
		if inFence {
			if open.closedBy(line) {
				inFence = false
			}
			i = end
			continue
		}
		if f, ok := openFence(line); ok {
			flush(i)
			open, inFence = f, true
			i = end
			continue
		}
		switch classify(line) {
		case lineBlank:
			flush(i)
		case lineAlone:
			flush(i)
			refs = scanSpan(line, refs)
		case lineStarts:
			flush(i)
			start = i
		default:
			if start < 0 {
				start = i
			}
		}
		i = end
	}
	flush(len(text))
	return refs
}

// scanSpan finds the imports in one paragraph (or one stand-alone line) and
// appends them to refs.
func scanSpan(u string, refs []string) []string {
	if strings.IndexByte(u, '@') < 0 {
		return refs
	}
	var ticks map[int][]int // made on first need
	for i := 0; i < len(u); {
		switch u[i] {
		case '\\':
			// A backslash takes the meaning from the punctuation mark
			// after it: "\`" opens no code and "\@" starts no import.
			i++
			if i < len(u) && u[i] < utf8.RuneSelf && u[i] != '\n' {
				i++
			}
		case '`':
			// Code runs from a row of backticks to the next row of
			// exactly as many. With no such row ahead, these are just
			// backticks.
			n := 1
			for i+n < len(u) && u[i+n] == '`' {
				n++
			}
			if ticks == nil {
				ticks = tickRuns(u)
			}
			rows := ticks[n]
			if k := sort.SearchInts(rows, i+n); k < len(rows) {
				i = rows[k] + n
			} else {
				i += n
			}
		case '@':
			if i > 0 && strings.IndexByte(" \t\n([", u[i-1]) < 0 {
				i++
				continue
			}
			p, next := importPath(u, i+1)
			if p != "" {
				refs = append(refs, p)
			}
			i = next
		default:
			i++
		}
	}
	return refs
}

// tickRuns indexes the rows of backticks in u: for each length, where the
// rows of exactly that length begin, in order. With it, finding the row
// that closes a code span is a search rather than a walk, so that a file
// made of nothing but backticks costs no more than any other.
func tickRuns(u string) map[int][]int {
	runs := map[int][]int{}
	for i := 0; i < len(u); {
		k := strings.IndexByte(u[i:], '`')
		if k < 0 {
			break
		}
		i += k
		n := 1
		for i+n < len(u) && u[i+n] == '`' {
			n++
		}
		runs[n] = append(runs[n], i)
		i += n
	}
	return runs
}

// importPath reads the path of an import whose "@" comes just before u[i].
// It returns the path, or "" if what follows is not a path, and the index
// to go on scanning from.
func importPath(u string, i int) (string, int) {
	if i >= len(u) {
		return "", i
	}
	first, _ := utf8.DecodeRuneInString(u[i:])
	if !unicode.IsLetter(first) && !unicode.IsDigit(first) && !strings.ContainsRune("./~_", first) {
		return "", i
	}
	j := i
scan:
	for j < len(u) {
		c := u[j]
		switch {
		case c == '\\':
			if j+1 >= len(u) || u[j+1] == '\n' {
				break scan
			}
			_, size := utf8.DecodeRuneInString(u[j+1:])
			j += 1 + size
		case c < utf8.RuneSelf:
			if strings.IndexByte(" \t\n()[]{}<>\"'`|*", c) >= 0 {
				break scan
			}
			j++
		default:
			r, size := utf8.DecodeRuneInString(u[j:])
			if unicode.IsSpace(r) {
				break scan
			}
			j += size
		}
	}
	raw := u[i:j]
	for len(raw) > 0 && strings.IndexByte(".,;:!?", raw[len(raw)-1]) >= 0 && !escapedAt(raw, len(raw)-1) {
		raw = raw[:len(raw)-1]
	}
	return unescape(raw), j
}

// escapedAt reports whether the byte at s[k] has a backslash before it that
// is not itself escaped.
func escapedAt(s string, k int) bool {
	n := 0
	for k-1-n >= 0 && s[k-1-n] == '\\' {
		n++
	}
	return n%2 == 1
}

// unescape takes the backslashes out of a path: each one stands for the
// character after it.
func unescape(s string) string {
	if strings.IndexByte(s, '\\') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

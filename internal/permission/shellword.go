package permission

import (
	"strings"
)

// partKind says what one piece of a shell word is.
type partKind uint8

const (
	// partLit is text the shell passes on as written (after quote removal).
	partLit partKind = iota
	// partVar is a parameter expansion: $name or ${name...}.
	partVar
	// partSubst is a command substitution, a backtick command or a process
	// substitution: the shell runs something to get the text.
	partSubst
	// partArith is an arithmetic expansion: $((...)).
	partArith
)

// wordPart is one piece of a shell word.
type wordPart struct {
	kind partKind
	// lit is the text of a partLit.
	lit string
	// name is the variable a partVar expands.
	name string
	// guarded marks ${name:?} and ${name?}: the shell stops instead of
	// expanding an unset (or empty) variable to nothing.
	guarded bool
}

// word is one word of a shell command, taken apart far enough to say what
// the shell would do with it.
type word struct {
	// raw is the word as written, with backslash-newline pairs removed.
	raw string
	// parts are the word's pieces in order.
	parts []wordPart
	// quoted is set when any part of the word was quoted or escaped.
	quoted bool
	// glob is set when the word holds an unquoted *, ? or [.
	glob bool
	// brace is set when the word holds an unquoted { or } and is not the
	// literal "{}": the shell may turn it into several words.
	brace bool
	// tilde is set when the word starts with an unquoted ~.
	tilde bool
	// eqHead is set when the word starts with an unquoted =, which zsh
	// replaces by the path of the command named after it.
	eqHead bool
}

// static reports whether the word's value is known without running the
// shell: no parameter, command or arithmetic expansion, and no tilde form
// other than ~ and ~/path.
func (w word) static() bool {
	for _, p := range w.parts {
		if p.kind != partLit {
			return false
		}
	}
	if w.tilde {
		l := w.joined()
		if l != "~" && !strings.HasPrefix(l, "~/") {
			return false
		}
	}
	return !w.eqHead
}

// plain reports whether the word is static text with nothing for the shell
// to do: no quoting, no pattern, no braces, no tilde.
func (w word) plain() bool {
	return w.static() && !w.quoted && !w.glob && !w.brace && !w.tilde
}

// joined returns the word's literal pieces put together, leaving out
// whatever an expansion would add.
func (w word) joined() string {
	if len(w.parts) == 1 && w.parts[0].kind == partLit {
		return w.parts[0].lit
	}
	var b strings.Builder
	for _, p := range w.parts {
		if p.kind == partLit {
			b.WriteString(p.lit)
		}
	}
	return b.String()
}

// lit returns the word's value when it is static, and its raw text when
// it is not. A leading ~ is left as it is; see value.
func (w word) lit() string {
	if w.static() {
		return w.joined()
	}
	return w.raw
}

// value returns the static word's value with a leading ~ replaced by home.
func (w word) value(home string) string {
	l := w.joined()
	if w.tilde && home != "" {
		if l == "~" {
			return home
		}
		if strings.HasPrefix(l, "~/") {
			return home + l[1:]
		}
	}
	return l
}

// hasSubst reports whether the word runs a command to get its text.
func (w word) hasSubst() bool {
	for _, p := range w.parts {
		if p.kind == partSubst {
			return true
		}
	}
	return false
}

// onlySubst reports whether the word is nothing but one command
// substitution, quoted or not.
func (w word) onlySubst() bool {
	n := 0
	for _, p := range w.parts {
		switch p.kind {
		case partSubst:
			n++
		case partLit:
			if p.lit != "" {
				return false
			}
		default:
			return false
		}
	}
	return n == 1
}

// isNameStart and isNameChar say which bytes make a shell variable name.
func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool { return isNameStart(c) || (c >= '0' && c <= '9') }

// isWordEnd reports whether c ends an unquoted word.
func isWordEnd(c byte) bool {
	switch c {
	case ' ', '\t', '\n', ';', '&', '|', '<', '>', '(', ')':
		return true
	}
	return false
}

// wordBuilder collects a word while it is read.
type wordBuilder struct {
	w   word
	raw strings.Builder
	lit strings.Builder
}

func (b *wordBuilder) flush() {
	if b.lit.Len() > 0 {
		b.w.parts = append(b.w.parts, wordPart{kind: partLit, lit: b.lit.String()})
		b.lit.Reset()
	}
}

func (b *wordBuilder) add(p wordPart) {
	b.flush()
	b.w.parts = append(b.w.parts, p)
}

func (b *wordBuilder) done() word {
	b.flush()
	b.w.raw = b.raw.String()
	if len(b.w.parts) == 0 {
		b.w.parts = []wordPart{{kind: partLit}}
	}
	if b.w.brace && b.w.raw == "{}" {
		b.w.brace = false
	}
	return b.w
}

// readWord reads the word that starts at p.pos. It reports false when no
// word starts there. On a construct it cannot follow it calls p.fail and
// returns what it has.
func (p *parser) readWord() (word, bool) {
	start := p.pos
	var b wordBuilder
	for p.pos < len(p.src) && !p.failed {
		c := p.src[p.pos]
		if isWordEnd(c) {
			// <(cmd), >(cmd) and zsh's =(cmd) are words that run a command.
			if c == '(' && p.pos > start && b.raw.String() == "=" && !b.w.quoted {
				p.procSubst(&b)
				continue
			}
			if (c == '<' || c == '>') && p.pos == start && p.pos+1 < len(p.src) && p.src[p.pos+1] == '(' {
				b.raw.WriteByte(c)
				p.pos++
				p.procSubst(&b)
				continue
			}
			break
		}
		switch c {
		case '\\':
			if p.pos+1 >= len(p.src) {
				p.fail("the command ends in a backslash")
				return b.done(), true
			}
			n := p.src[p.pos+1]
			p.pos += 2
			if n == '\n' {
				continue
			}
			b.w.quoted = true
			b.raw.WriteByte('\\')
			b.raw.WriteByte(n)
			b.lit.WriteByte(n)
		case '\'':
			end := strings.IndexByte(p.src[p.pos+1:], '\'')
			if end < 0 {
				p.fail("a single quote is never closed")
				return b.done(), true
			}
			b.w.quoted = true
			b.raw.WriteString(p.src[p.pos : p.pos+end+2])
			b.lit.WriteString(p.src[p.pos+1 : p.pos+1+end])
			p.pos += end + 2
		case '"':
			b.w.quoted = true
			b.raw.WriteByte('"')
			p.pos++
			p.dquote(&b, '"')
		case '`':
			p.backtick(&b)
		case '$':
			p.dollar(&b, false)
		case '*', '?', '[':
			b.w.glob = true
			b.raw.WriteByte(c)
			b.lit.WriteByte(c)
			p.pos++
		case '{', '}':
			b.w.brace = true
			b.raw.WriteByte(c)
			b.lit.WriteByte(c)
			p.pos++
		default:
			if p.pos == start {
				switch c {
				case '~':
					b.w.tilde = true
				case '=':
					b.w.eqHead = true
				}
			}
			b.raw.WriteByte(c)
			b.lit.WriteByte(c)
			p.pos++
		}
	}
	if p.pos == start {
		return word{}, false
	}
	return b.done(), true
}

// dquote reads the inside of a double-quoted string up to and including
// the closing quote. With closer 0 it reads to the end of the text, which
// is how the body of a here-document is scanned.
func (p *parser) dquote(b *wordBuilder, closer byte) {
	for p.pos < len(p.src) && !p.failed {
		c := p.src[p.pos]
		switch {
		case c == closer && closer != 0:
			b.raw.WriteByte(c)
			p.pos++
			return
		case c == '\\':
			if p.pos+1 >= len(p.src) {
				if closer == 0 {
					b.raw.WriteByte(c)
					b.lit.WriteByte(c)
					p.pos++
					return
				}
				p.fail("a double quote is never closed")
				return
			}
			n := p.src[p.pos+1]
			p.pos += 2
			switch n {
			case '\n':
			case '$', '`', '"', '\\':
				b.raw.WriteByte('\\')
				b.raw.WriteByte(n)
				b.lit.WriteByte(n)
			default:
				b.raw.WriteByte('\\')
				b.raw.WriteByte(n)
				b.lit.WriteByte('\\')
				b.lit.WriteByte(n)
			}
		case c == '`':
			p.backtick(b)
		case c == '$':
			p.dollar(b, true)
		default:
			b.raw.WriteByte(c)
			b.lit.WriteByte(c)
			p.pos++
		}
	}
	if closer != 0 && !p.failed {
		p.fail("a double quote is never closed")
	}
}

// backtick reads a `command` and parses the command inside it.
func (p *parser) backtick(b *wordBuilder) {
	p.pos++ // the opening backtick
	var inner strings.Builder
	closed := false
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '`' {
			p.pos++
			closed = true
			break
		}
		if c == '\\' && p.pos+1 < len(p.src) {
			n := p.src[p.pos+1]
			if n == '`' || n == '\\' || n == '$' {
				inner.WriteByte(n)
				p.pos += 2
				continue
			}
		}
		inner.WriteByte(c)
		p.pos++
	}
	if !closed {
		p.fail("a backtick is never closed")
		return
	}
	b.raw.WriteByte('`')
	b.raw.WriteString(inner.String())
	b.raw.WriteByte('`')
	b.add(wordPart{kind: partSubst})
	p.sc.subst = true
	p.nested(inner.String(), nestSubst)
}

// procSubst reads the "(cmd)" of a process substitution; p.pos is at the
// opening parenthesis.
func (p *parser) procSubst(b *wordBuilder) {
	from := p.pos
	p.pos++
	p.inner(nestSubst, ')')
	if p.failed {
		return
	}
	b.raw.WriteString(p.src[from:p.pos])
	b.add(wordPart{kind: partSubst})
	p.sc.subst = true
}

// dollar reads whatever a $ starts: a variable, ${...}, $(...), $((...)),
// $'...' or $"...". inDouble says the $ sits inside double quotes.
func (p *parser) dollar(b *wordBuilder, inDouble bool) {
	from := p.pos
	rest := p.src[p.pos+1:]
	switch {
	case strings.HasPrefix(rest, "(("):
		end := arithEnd(p.src, p.pos+3, false)
		if end < 0 {
			p.fail("an arithmetic expansion gnosh cannot follow")
			return
		}
		p.pos = end
		b.raw.WriteString(p.src[from:p.pos])
		b.add(wordPart{kind: partArith})
	case strings.HasPrefix(rest, "("):
		p.pos += 2
		p.inner(nestSubst, ')')
		if p.failed {
			return
		}
		b.raw.WriteString(p.src[from:p.pos])
		b.add(wordPart{kind: partSubst})
		p.sc.subst = true
	case strings.HasPrefix(rest, "{"):
		p.braceParam(b)
	case strings.HasPrefix(rest, "["):
		p.fail("the old $[...] arithmetic form")
	case strings.HasPrefix(rest, "'") && !inDouble:
		p.pos += 2
		s, ok := p.ansiC()
		if !ok {
			return
		}
		b.w.quoted = true
		b.raw.WriteString(p.src[from:p.pos])
		b.lit.WriteString(s)
	case strings.HasPrefix(rest, "\"") && !inDouble:
		b.w.quoted = true
		b.raw.WriteString("$\"")
		p.pos += 2
		p.dquote(b, '"')
	case rest != "" && isNameStart(rest[0]):
		n := 1
		for n < len(rest) && isNameChar(rest[n]) {
			n++
		}
		p.pos += 1 + n
		b.raw.WriteString(p.src[from:p.pos])
		b.add(wordPart{kind: partVar, name: rest[:n]})
	case rest != "" && strings.IndexByte("?$!#*@-0123456789", rest[0]) >= 0:
		p.pos += 2
		b.raw.WriteString(p.src[from:p.pos])
		b.add(wordPart{kind: partVar, name: rest[:1]})
	default:
		b.raw.WriteByte('$')
		b.lit.WriteByte('$')
		p.pos++
	}
}

// braceParam reads ${...}. Only the plain forms are followed: ${name} and
// ${name<op>word} for the POSIX operators. Anything else (array subscripts,
// indirection, zsh flags, bash's @ operators, substrings) can change what
// runs in ways this package does not model, so it fails the parse.
func (p *parser) braceParam(b *wordBuilder) {
	from := p.pos
	i := p.pos + 2
	n := i
	for n < len(p.src) && isNameChar(p.src[n]) {
		n++
	}
	if n == i || !isNameStart(p.src[i]) {
		p.exoticParam(b)
		return
	}
	name := p.src[i:n]
	if n >= len(p.src) {
		p.fail("a ${ is never closed")
		return
	}
	part := wordPart{kind: partVar, name: name}
	if p.src[n] == '}' {
		p.pos = n + 1
		b.raw.WriteString(p.src[from:p.pos])
		b.add(part)
		return
	}
	op := ""
	for _, o := range []string{":-", ":=", ":?", ":+", "-", "=", "?", "+", "%%", "%", "##", "#"} {
		if strings.HasPrefix(p.src[n:], o) {
			op = o
			break
		}
	}
	if op == "" {
		p.exoticParam(b)
		return
	}
	part.guarded = op == ":?" || op == "?"
	if op == ":=" || op == "=" {
		p.sc.setsVar = true
	}
	// The word after the operator may itself expand things, which are
	// followed. Quotes and backslashes inside it are not: shells disagree
	// on how they nest there, and a guess could hide a command.
	p.pos = n + len(op)
	var sub wordBuilder
	closed := false
	for p.pos < len(p.src) && !p.failed && !closed {
		switch c := p.src[p.pos]; c {
		case '}':
			p.pos++
			closed = true
		case '"', '\'', '\\':
			p.fail("quoting inside ${...}")
		case '`':
			p.backtick(&sub)
		case '$':
			p.dollar(&sub, true)
		default:
			sub.lit.WriteByte(c)
			p.pos++
		}
	}
	if p.failed {
		return
	}
	if !closed {
		p.fail("a ${ is never closed")
		return
	}
	body := sub.done()
	b.raw.WriteString(p.src[from:p.pos])
	b.add(part)
	if body.hasSubst() {
		b.add(wordPart{kind: partSubst})
	}
}

// exoticParam reads a ${...} that is not one of the plain forms. Its
// value is not known, and no allow rule approves a line that holds one,
// but the line can still be read to its end.
func (p *parser) exoticParam(b *wordBuilder) {
	end, sets, ok := exoticEnd(p.src, p.pos+2)
	if !ok {
		p.fail("a ${...} form gnosh does not follow")
		return
	}
	b.raw.WriteString(p.src[p.pos:end])
	p.pos = end
	b.add(wordPart{kind: partVar})
	p.sc.exotic = true
	if sets {
		p.sc.setsVar = true
	}
}

// exoticEnd returns the index just past the "}" that closes a ${...} whose
// body starts at i, when the body is made only of characters that cannot
// hide a command or a quote: names, digits and the operators of the
// expansion forms shells have (subscripts, substrings, replacements,
// case changes). It reports false for anything else. sets says the body
// holds an =, which in some forms assigns.
func exoticEnd(s string, i int) (end int, sets, ok bool) {
	depth := 1
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '}':
			depth--
			if depth == 0 {
				return i + 1, sets, true
			}
		case c == '$':
			if i+1 < len(s) && s[i+1] == '{' {
				depth++
				i++
			} else if i+1 >= len(s) || !isNameStart(s[i+1]) {
				return 0, false, false
			}
		case c == '=':
			sets = true
		case isNameChar(c), strings.IndexByte("[]@*#%/:-+?.,^!~ ", c) >= 0:
		default:
			return 0, false, false
		}
	}
	return 0, false, false
}

// arithEnd returns the index just past the "))" that closes the arithmetic
// expansion whose body starts at i, or -1 when the body holds anything but
// plain arithmetic. semis lets the body hold semicolons, as the head of a
// C-style for loop does.
func arithEnd(s string, i int, semis bool) int {
	depth := 0
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				if i+1 < len(s) && s[i+1] == ')' {
					return i + 2
				}
				return -1
			}
			depth--
		case isNameChar(c), strings.IndexByte(" \t+-*/%<>=!&|^~?:,.", c) >= 0:
		case c == ';' && semis:
		case c == '$' && i+1 < len(s) && isNameStart(s[i+1]):
		default:
			return -1
		}
	}
	return -1
}

// ansiC reads the inside of $'...' up to and including the closing quote
// and returns the text it stands for.
func (p *parser) ansiC() (string, bool) {
	var out strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\'' {
			p.pos++
			return out.String(), true
		}
		if c != '\\' {
			out.WriteByte(c)
			p.pos++
			continue
		}
		if p.pos+1 >= len(p.src) {
			break
		}
		n := p.src[p.pos+1]
		p.pos += 2
		switch n {
		case 'n':
			out.WriteByte('\n')
		case 't':
			out.WriteByte('\t')
		case 'r':
			out.WriteByte('\r')
		case 'a':
			out.WriteByte(7)
		case 'b':
			out.WriteByte(8)
		case 'f':
			out.WriteByte(12)
		case 'v':
			out.WriteByte(11)
		case 'e', 'E':
			out.WriteByte(27)
		case '\\', '\'', '"', '?':
			out.WriteByte(n)
		case 'x':
			v, k := readDigits(p.src[p.pos:], 16, 2)
			if k == 0 {
				p.fail("a $'...' escape gnosh cannot read")
				return "", false
			}
			p.pos += k
			if v == 0 {
				p.fail("a $'...' escape gnosh cannot read")
				return "", false
			}
			out.WriteByte(byte(v))
		case 'u', 'U':
			max := 4
			if n == 'U' {
				max = 8
			}
			v, k := readDigits(p.src[p.pos:], 16, max)
			if k == 0 || v == 0 || v > 0x10FFFF {
				p.fail("a $'...' escape gnosh cannot read")
				return "", false
			}
			p.pos += k
			out.WriteRune(rune(v))
		case '0', '1', '2', '3', '4', '5', '6', '7':
			v, k := readDigits(p.src[p.pos-1:], 8, 3)
			p.pos += k - 1
			if v == 0 || v > 255 {
				p.fail("a $'...' escape gnosh cannot read")
				return "", false
			}
			out.WriteByte(byte(v))
		default:
			// \cX and anything else: not worth guessing at.
			p.fail("a $'...' escape gnosh cannot read")
			return "", false
		}
	}
	p.fail("a $'...' quote is never closed")
	return "", false
}

// readDigits reads up to max digits of the given base from the start of s
// and returns their value and how many it read.
func readDigits(s string, base, max int) (int, int) {
	v, k := 0, 0
	for k < len(s) && k < max {
		c := s[k]
		d := -1
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		}
		if d < 0 || d >= base {
			break
		}
		v = v*base + d
		k++
	}
	return v, k
}

package permission

import (
	"strings"
)

// This file is gnosh's own reader of shell command lines. It is not a
// shell: it reads just enough of the POSIX grammar (and the bash and zsh
// additions people really type) to say which programs a command line would
// run and what else it would do on the way. Whatever it cannot follow it
// says so, and the caller then asks a person instead of guessing.

// nesting says what a simple command sits inside.
type nesting uint8

const (
	// nestSubshell: inside ( ... ).
	nestSubshell nesting = 1 << iota
	// nestSubst: inside $( ... ), backticks, or <( ... ).
	nestSubst
	// nestCompound: inside a loop, an if, a case, a { ...; } group or a
	// function body.
	nestCompound
	// nestScript: inside text handed to another shell, to eval or to a
	// program that runs its arguments. The parser never sets it; the
	// walk over wrappers does.
	nestScript
)

// sepKind says what joined a simple command to the one before it.
type sepKind uint8

const (
	// sepStart: the first command of its list.
	sepStart sepKind = iota
	// sepSemi: after ; or a newline.
	sepSemi
	// sepAnd: after &&.
	sepAnd
	// sepOr: after ||.
	sepOr
	// sepPipe: after | or |&.
	sepPipe
	// sepBg: after a command that was put in the background with &.
	sepBg
)

// redir is one redirection of a simple command.
type redir struct {
	// op is the operator: < > >> >| <> &> &>> >& <& << <<- <<<.
	op string
	// fd is the descriptor number written before the operator, or "".
	fd string
	// target is the word after the operator: a file, a descriptor, a
	// here-document's delimiter or a here-string.
	target word
	// body is a here-document's text; bodyStatic says the shell passes it
	// on unchanged (the delimiter was quoted, or nothing in it expands).
	body       string
	bodyStatic bool
}

// assign is a variable assignment written before a command's name.
type assign struct {
	name string
	// w is the whole word, name and = included.
	w word
	// fromSubst is set when the value comes from a command substitution.
	fromSubst bool
}

// simpleCmd is one program invocation: assignments, words, redirections.
type simpleCmd struct {
	assigns []assign
	words   []word
	redirs  []redir
	sep     sepKind
	nest    nesting
	// bg is set when the list the command ends was put in the background.
	bg bool
}

// script is everything read from one command line.
type script struct {
	// cmds holds every simple command, nested ones included, in the order
	// their text ends.
	cmds []*simpleCmd
	// ok is false when the line was not followed to its end; why says
	// what stopped the reader.
	ok  bool
	why string
	// What the line holds anywhere, nested or not.
	subst      bool // a command or process substitution
	subshell   bool // ( ... )
	compound   bool // a loop, if, case, group or function
	background bool // a command followed by &
	setsVar    bool // a ${name:=value} expansion, which assigns
	exotic     bool // a ${...} form that is followed but not modelled
	comments   []string
}

// maxShellDepth bounds how deeply substitutions and groups may nest before
// the reader stops and reports the line as one it cannot follow.
const maxShellDepth = 64

// pendingDoc is a here-document whose body has not been reached yet.
type pendingDoc struct {
	cmd   *simpleCmd
	idx   int
	delim string
	tabs  bool
	lit   bool
	depth int
}

// parser reads one piece of shell text.
type parser struct {
	src    string
	pos    int
	sc     *script
	nest   nesting
	sep    sepKind
	depth  int
	failed bool
	docs   []pendingDoc
}

// parseScript reads a command line.
func parseScript(src string) *script {
	sc := &script{ok: true}
	for i := 0; i < len(src); i++ {
		if c := src[i]; c < 0x20 && c != '\n' && c != '\t' || c == 0x7f {
			sc.ok = false
			sc.why = "the command holds a control character"
			return sc
		}
	}
	p := &parser{src: src, sc: sc}
	p.run()
	return sc
}

// run reads p.src to its end.
func (p *parser) run() {
	p.list(stopper{})
	if p.failed {
		return
	}
	if p.pos < len(p.src) {
		p.fail("an unexpected " + describeByte(p.src[p.pos]))
		return
	}
	if len(p.docs) > 0 {
		p.fail("a here-document has no body")
	}
}

func describeByte(c byte) string {
	if c == '\n' {
		return "line break"
	}
	return "\"" + string(c) + "\""
}

// fail records that the line could not be followed. The first reason is
// the one kept.
func (p *parser) fail(why string) {
	p.failed = true
	if p.sc.ok {
		p.sc.ok = false
		p.sc.why = why
	}
}

// nested reads text that sits inside the line being read (the inside of
// backticks) as a script of its own.
func (p *parser) nested(text string, n nesting) {
	if p.depth >= maxShellDepth {
		p.fail("commands nested too deeply")
		return
	}
	q := &parser{src: text, sc: p.sc, nest: p.nest | n, depth: p.depth + 1}
	q.run()
	if q.failed {
		p.failed = true
	}
}

// inner reads a list in place up to the closing byte, which it consumes.
func (p *parser) inner(n nesting, closer byte) {
	if p.depth >= maxShellDepth {
		p.fail("commands nested too deeply")
		return
	}
	nest, sep := p.nest, p.sep
	p.nest |= n
	p.sep = sepStart
	p.depth++
	p.list(stopper{paren: true})
	p.depth--
	p.nest, p.sep = nest, sep
	if p.failed {
		return
	}
	if p.pos >= len(p.src) || p.src[p.pos] != closer {
		p.fail("a parenthesis is never closed")
		return
	}
	p.pos++
}

// blanks skips spaces, tabs, backslash-newline pairs and comments. It is
// only called between tokens, which is where a # starts a comment.
func (p *parser) blanks() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case c == ' ' || c == '\t':
			p.pos++
		case c == '\\' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '\n':
			p.pos += 2
		case c == '#':
			end := strings.IndexByte(p.src[p.pos:], '\n')
			if end < 0 {
				end = len(p.src) - p.pos
			}
			text := p.src[p.pos+1 : p.pos+end]
			p.sc.comments = append(p.sc.comments, text)
			// A shell that does not take # as a comment (an interactive zsh)
			// would run what follows. Plain words are harmless either way;
			// anything a shell could act on is not.
			if strings.ContainsAny(text, ";&|<>()$`\\\"'{}") {
				p.fail("a comment that holds shell operators")
			}
			p.pos += end
		default:
			return
		}
	}
}

// newlines skips blanks and line breaks, reading the bodies of any
// here-documents that a line break starts.
func (p *parser) newlines() {
	for {
		p.blanks()
		if p.failed || p.pos >= len(p.src) || p.src[p.pos] != '\n' {
			return
		}
		p.newline()
	}
}

// newline consumes one line break and the here-document bodies after it.
func (p *parser) newline() {
	p.pos++
	if len(p.docs) == 0 {
		return
	}
	docs := p.docs
	p.docs = nil
	for _, d := range docs {
		if d.depth != p.depth {
			p.fail("a here-document that starts inside a substitution")
			return
		}
		var body strings.Builder
		closed := false
		for p.pos < len(p.src) {
			end := strings.IndexByte(p.src[p.pos:], '\n')
			line := p.src[p.pos:]
			next := len(p.src)
			if end >= 0 {
				line = p.src[p.pos : p.pos+end]
				next = p.pos + end + 1
			}
			p.pos = next
			cmp := line
			if d.tabs {
				cmp = strings.TrimLeft(line, "\t")
			}
			if cmp == d.delim {
				closed = true
				break
			}
			body.WriteString(line)
			body.WriteByte('\n')
		}
		if !closed {
			p.fail("a here-document is never closed")
			return
		}
		r := &d.cmd.redirs[d.idx]
		r.body = body.String()
		r.bodyStatic = true
		if !d.lit {
			// An unquoted delimiter: the shell expands the body.
			q := &parser{src: r.body, sc: p.sc, nest: p.nest, depth: p.depth + 1}
			var wb wordBuilder
			q.dquote(&wb, 0)
			if q.failed {
				p.failed = true
				return
			}
			w := wb.done()
			r.bodyStatic = w.static()
			if r.bodyStatic {
				r.body = w.joined()
			}
		}
	}
}

// stopper says where a list ends besides the end of the text.
type stopper struct {
	// paren: at a closing parenthesis.
	paren bool
	// words: at any of these reserved words.
	words []string
	// dsemi: at ;; ;& or ;;& (the end of a case branch).
	dsemi bool
}

func (p *parser) atStop(st stopper) bool {
	if p.pos >= len(p.src) {
		return true
	}
	if st.paren && p.src[p.pos] == ')' {
		return true
	}
	if st.dsemi && (strings.HasPrefix(p.src[p.pos:], ";;") || strings.HasPrefix(p.src[p.pos:], ";&")) {
		return true
	}
	if len(st.words) > 0 {
		if kw := p.reserved(); kw != "" {
			for _, w := range st.words {
				if w == kw {
					return true
				}
			}
		}
	}
	return false
}

// list reads commands joined by ; & and line breaks until st says stop.
func (p *parser) list(st stopper) {
	p.sep = sepStart
	for !p.failed {
		p.newlines()
		if p.failed || p.atStop(st) {
			return
		}
		first := len(p.sc.cmds)
		p.andOr()
		if p.failed {
			return
		}
		p.blanks()
		if p.pos >= len(p.src) {
			return
		}
		rest := p.src[p.pos:]
		switch {
		case st.dsemi && (strings.HasPrefix(rest, ";;") || strings.HasPrefix(rest, ";&")):
			return
		case rest[0] == ';':
			p.pos++
			p.sep = sepSemi
		case rest[0] == '&':
			p.pos++
			p.sc.background = true
			for _, c := range p.sc.cmds[first:] {
				c.bg = true
			}
			p.sep = sepBg
		case rest[0] == '\n':
			p.newline()
			p.sep = sepSemi
		default:
			return
		}
	}
}

// andOr reads pipelines joined by && and ||.
func (p *parser) andOr() {
	p.pipeline()
	for !p.failed {
		p.blanks()
		rest := p.src[p.pos:]
		switch {
		case strings.HasPrefix(rest, "&&"):
			p.sep = sepAnd
		case strings.HasPrefix(rest, "||"):
			p.sep = sepOr
		default:
			return
		}
		p.pos += 2
		p.newlines()
		p.pipeline()
	}
}

// pipeline reads commands joined by | and |&.
func (p *parser) pipeline() {
	p.blanks()
	if p.reserved() == "!" {
		p.pos++
	}
	p.command()
	for !p.failed {
		p.blanks()
		rest := p.src[p.pos:]
		if !strings.HasPrefix(rest, "|") || strings.HasPrefix(rest, "||") {
			return
		}
		p.pos++
		if strings.HasPrefix(rest, "|&") {
			p.pos++
		}
		p.sep = sepPipe
		p.newlines()
		p.command()
	}
}

// reservedWords are the words that mean something to the shell when they
// stand where a command name would.
var reservedWords = []string{
	"!", "{", "}", "if", "then", "elif", "else", "fi", "while", "until",
	"do", "done", "for", "case", "esac", "function", "select", "coproc", "[[",
}

// reserved returns the reserved word that starts at p.pos, or "".
func (p *parser) reserved() string {
	rest := p.src[p.pos:]
	for _, kw := range reservedWords {
		if !strings.HasPrefix(rest, kw) {
			continue
		}
		if len(rest) == len(kw) {
			return kw
		}
		c := rest[len(kw)]
		switch kw {
		case "{", "!":
			if c == ' ' || c == '\t' || c == '\n' {
				return kw
			}
		default:
			if isWordEnd(c) {
				return kw
			}
		}
	}
	return ""
}

// expect consumes the reserved word kw or fails.
func (p *parser) expect(kw string) {
	if p.failed {
		return
	}
	p.newlines()
	if p.reserved() != kw {
		p.fail("a missing \"" + kw + "\"")
		return
	}
	p.pos += len(kw)
}

// body reads a list that sits inside a compound command.
func (p *parser) body(words ...string) {
	if p.failed {
		return
	}
	if p.depth >= maxShellDepth {
		p.fail("commands nested too deeply")
		return
	}
	nest := p.nest
	p.nest |= nestCompound
	p.depth++
	p.list(stopper{words: words})
	p.depth--
	p.nest = nest
}

// command reads one command: a simple one or a compound one.
func (p *parser) command() {
	p.blanks()
	if p.failed {
		return
	}
	if p.pos >= len(p.src) {
		p.fail("an operator with no command after it")
		return
	}
	c := p.src[p.pos]
	if c == '(' {
		if strings.HasPrefix(p.src[p.pos:], "((") {
			// (( expression )): arithmetic as a command. It is followed
			// only when it is plain arithmetic, and counts as a compound
			// form, so no allow rule approves the line.
			end := arithEnd(p.src, p.pos+2, false)
			if end < 0 {
				p.fail("an arithmetic command gnosh cannot follow")
				return
			}
			p.pos = end
			p.sc.compound = true
			p.trailingRedirs()
			return
		}
		p.pos++
		p.sc.subshell = true
		p.inner(nestSubshell, ')')
		p.trailingRedirs()
		return
	}
	if c == ')' || c == ';' || c == '&' || c == '|' || c == '\n' {
		p.fail("an operator with no command before it")
		return
	}
	kw := p.reserved()
	switch kw {
	case "":
		p.simple()
		return
	case "{":
		p.pos++
		p.sc.compound = true
		p.body("}")
		p.expect("}")
	case "if":
		p.pos += 2
		p.sc.compound = true
		p.body("then")
		p.expect("then")
		p.body("elif", "else", "fi")
		for !p.failed && p.reserved() == "elif" {
			p.pos += 4
			p.body("then")
			p.expect("then")
			p.body("elif", "else", "fi")
		}
		if !p.failed && p.reserved() == "else" {
			p.pos += 4
			p.body("fi")
		}
		p.expect("fi")
	case "while", "until":
		p.pos += 5
		p.sc.compound = true
		p.body("do")
		p.expect("do")
		p.body("done")
		p.expect("done")
	case "for":
		p.pos += 3
		p.sc.compound = true
		p.forHead()
		p.expect("do")
		p.body("done")
		p.expect("done")
	case "case":
		p.pos += 4
		p.sc.compound = true
		p.caseBody()
	case "[[":
		p.testCommand()
	case "function":
		p.pos += 8
		p.blanks()
		if _, ok := p.readWord(); !ok {
			p.fail("a function with no name")
			return
		}
		p.blanks()
		if strings.HasPrefix(p.src[p.pos:], "(") {
			p.pos++
			p.blanks()
			if !strings.HasPrefix(p.src[p.pos:], ")") {
				p.fail("a parenthesis gnosh cannot place")
				return
			}
			p.pos++
		}
		p.funcBody()
		return
	default:
		p.fail("\"" + kw + "\" where a command should start")
		return
	}
	p.trailingRedirs()
}

// funcBody reads the command that is a function's body.
func (p *parser) funcBody() {
	p.sc.compound = true
	p.newlines()
	if p.failed {
		return
	}
	nest := p.nest
	p.nest |= nestCompound
	p.command()
	p.nest = nest
}

// forHead reads "name [in word...]" and the separator after it.
func (p *parser) forHead() {
	p.blanks()
	if strings.HasPrefix(p.src[p.pos:], "((") {
		// for ((init; test; step)): followed when it is plain arithmetic.
		end := arithEnd(p.src, p.pos+2, true)
		if end < 0 {
			p.fail("a for loop gnosh cannot follow")
			return
		}
		p.pos = end
		p.blanks()
		if strings.HasPrefix(p.src[p.pos:], ";") && !strings.HasPrefix(p.src[p.pos:], ";;") {
			p.pos++
		}
		return
	}
	name, ok := p.readWord()
	if !ok || !name.plain() {
		p.fail("a for loop gnosh cannot follow")
		return
	}
	p.newlines()
	if p.failed {
		return
	}
	rest := p.src[p.pos:]
	if strings.HasPrefix(rest, "in") && (len(rest) == 2 || isWordEnd(rest[2])) {
		p.pos += 2
		for !p.failed {
			p.blanks()
			if _, ok := p.readWord(); !ok {
				break
			}
		}
	}
	p.blanks()
	if strings.HasPrefix(p.src[p.pos:], ";") && !strings.HasPrefix(p.src[p.pos:], ";;") {
		p.pos++
	}
}

// caseBody reads "word in pattern) list ;; ... esac".
func (p *parser) caseBody() {
	p.blanks()
	if _, ok := p.readWord(); !ok {
		p.fail("a case with nothing to match")
		return
	}
	p.newlines()
	rest := p.src[p.pos:]
	if p.failed || !strings.HasPrefix(rest, "in") || (len(rest) > 2 && !isWordEnd(rest[2])) {
		p.fail("a missing \"in\"")
		return
	}
	p.pos += 2
	for !p.failed {
		p.newlines()
		if p.failed {
			return
		}
		if p.reserved() == "esac" {
			p.pos += 4
			return
		}
		if strings.HasPrefix(p.src[p.pos:], "(") {
			p.pos++
		}
		for !p.failed {
			p.blanks()
			if _, ok := p.readWord(); !ok {
				p.fail("a case pattern gnosh cannot follow")
				return
			}
			p.blanks()
			rest := p.src[p.pos:]
			if strings.HasPrefix(rest, "|") {
				p.pos++
				continue
			}
			if strings.HasPrefix(rest, ")") {
				p.pos++
				break
			}
			p.fail("a case pattern gnosh cannot follow")
			return
		}
		if p.depth >= maxShellDepth {
			p.fail("commands nested too deeply")
			return
		}
		nest := p.nest
		p.nest |= nestCompound
		p.depth++
		p.list(stopper{dsemi: true, words: []string{"esac"}})
		p.depth--
		p.nest = nest
		if p.failed {
			return
		}
		p.blanks()
		for _, end := range []string{";;&", ";;", ";&"} {
			if strings.HasPrefix(p.src[p.pos:], end) {
				p.pos += len(end)
				break
			}
		}
	}
}

// testCommand reads [[ ... ]]. Inside it && || ( ) < > and | are the
// test's own operators, not the shell's; everything else is read as
// words, so that a substitution among them is still seen.
func (p *parser) testCommand() {
	cmd := &simpleCmd{sep: p.sep, nest: p.nest}
	open, _ := p.readWord()
	cmd.words = append(cmd.words, open)
	for !p.failed {
		p.newlines()
		if p.failed {
			return
		}
		rest := p.src[p.pos:]
		switch {
		case rest == "":
			p.fail("a [[ is never closed")
		case strings.HasPrefix(rest, "]]") && (len(rest) == 2 || isWordEnd(rest[2])):
			p.pos += 2
			cmd.words = append(cmd.words, word{raw: "]]", parts: []wordPart{{lit: "]]"}}})
			p.sc.cmds = append(p.sc.cmds, cmd)
			return
		case strings.HasPrefix(rest, "&&"), strings.HasPrefix(rest, "||"):
			p.pos += 2
		case strings.HasPrefix(rest, "<("), strings.HasPrefix(rest, ">("):
			// A process substitution runs a command even here.
			w, _ := p.readWord()
			cmd.words = append(cmd.words, w)
		case strings.IndexByte("()<>|", rest[0]) >= 0:
			p.pos++
		case rest[0] == ';' || rest[0] == '&':
			p.fail("an operator inside [[ ]]")
		default:
			w, ok := p.readWord()
			if !ok {
				p.fail("a [[ ]] gnosh cannot follow")
				return
			}
			cmd.words = append(cmd.words, w)
		}
	}
	if len(cmd.words) > 0 {
		p.sc.cmds = append(p.sc.cmds, cmd)
	}
}

// trailingRedirs reads the redirections written after a compound command
// and keeps them as a command with no words, so that what they write is
// judged like any other redirection.
func (p *parser) trailingRedirs() {
	if p.failed {
		return
	}
	cmd := &simpleCmd{sep: p.sep, nest: p.nest}
	for !p.failed {
		p.blanks()
		if !p.redirAt() {
			break
		}
		p.redir(cmd)
	}
	if len(cmd.redirs) > 0 {
		p.sc.cmds = append(p.sc.cmds, cmd)
	}
}

// simple reads one simple command.
func (p *parser) simple() {
	cmd := &simpleCmd{sep: p.sep, nest: p.nest}
	for !p.failed {
		p.blanks()
		if p.pos >= len(p.src) {
			break
		}
		if p.redirAt() {
			p.redir(cmd)
			continue
		}
		c := p.src[p.pos]
		if c == '(' {
			if len(cmd.words) == 1 && len(cmd.assigns) == 0 && len(cmd.redirs) == 0 && cmd.words[0].plain() {
				// name() body: a function definition.
				p.pos++
				p.blanks()
				if !strings.HasPrefix(p.src[p.pos:], ")") {
					p.fail("a parenthesis gnosh cannot place")
					return
				}
				p.pos++
				p.funcBody()
				return
			}
			p.fail("a parenthesis gnosh cannot place")
			return
		}
		procSubst := (c == '<' || c == '>') && strings.HasPrefix(p.src[p.pos+1:], "(")
		if isWordEnd(c) && !procSubst {
			break
		}
		w, ok := p.readWord()
		if !ok || p.failed {
			break
		}
		if len(cmd.words) == 0 {
			if name, ok := assignName(w.raw); ok {
				a := assign{name: name, w: w, fromSubst: w.hasSubst()}
				if strings.HasSuffix(w.raw, "=") && strings.HasPrefix(p.src[p.pos:], "(") {
					// name=(word ...): an array.
					p.pos++
					for !p.failed {
						p.newlines()
						if strings.HasPrefix(p.src[p.pos:], ")") {
							p.pos++
							break
						}
						v, ok := p.readWord()
						if !ok {
							p.fail("an array assignment gnosh cannot follow")
							break
						}
						a.fromSubst = a.fromSubst || v.hasSubst()
					}
				}
				cmd.assigns = append(cmd.assigns, a)
				continue
			}
		}
		cmd.words = append(cmd.words, w)
	}
	if p.failed {
		// Keep what was read: a deny rule still applies to it.
		if len(cmd.words) > 0 || len(cmd.assigns) > 0 || len(cmd.redirs) > 0 {
			p.sc.cmds = append(p.sc.cmds, cmd)
		}
		return
	}
	if len(cmd.words) == 0 && len(cmd.assigns) == 0 && len(cmd.redirs) == 0 {
		p.fail("an operator with no command before it")
		return
	}
	p.sc.cmds = append(p.sc.cmds, cmd)
}

// assignName reports whether raw starts like an assignment (name=, name+=
// or name[...]=) and returns the variable's name.
func assignName(raw string) (string, bool) {
	if raw == "" || !isNameStart(raw[0]) {
		return "", false
	}
	i := 1
	for i < len(raw) && isNameChar(raw[i]) {
		i++
	}
	name := raw[:i]
	if i < len(raw) && raw[i] == '[' {
		end := strings.IndexByte(raw[i:], ']')
		if end < 0 {
			return "", false
		}
		i += end + 1
	}
	if strings.HasPrefix(raw[i:], "=") || strings.HasPrefix(raw[i:], "+=") {
		return name, true
	}
	return "", false
}

// redirAt reports whether a redirection starts at p.pos.
func (p *parser) redirAt() bool {
	i := p.pos
	for i < len(p.src) && p.src[i] >= '0' && p.src[i] <= '9' {
		i++
	}
	if i >= len(p.src) {
		return false
	}
	c := p.src[i]
	if c == '<' || c == '>' {
		// <(cmd) and >(cmd) with nothing before them are words.
		if i == p.pos && i+1 < len(p.src) && p.src[i+1] == '(' {
			return false
		}
		return true
	}
	return i == p.pos && strings.HasPrefix(p.src[i:], "&>")
}

// redirOps are the redirection operators, longest first.
var redirOps = []string{"<<<", "<<-", "<<", "<>", "<&", "<", ">>|", ">>", ">&", ">|", ">", "&>>", "&>"}

// redir reads the redirection at p.pos into cmd.
func (p *parser) redir(cmd *simpleCmd) {
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	r := redir{fd: p.src[start:p.pos]}
	for _, op := range redirOps {
		if strings.HasPrefix(p.src[p.pos:], op) {
			r.op = op
			break
		}
	}
	p.pos += len(r.op)
	if r.op == ">>|" {
		r.op = ">>"
	}
	p.blanks()
	t, ok := p.readWord()
	if p.failed {
		return
	}
	if !ok {
		p.fail("a redirection with no target")
		return
	}
	r.target = t
	switch r.op {
	case "<<", "<<-":
		if !t.static() || t.glob && !t.quoted {
			p.fail("a here-document delimiter gnosh cannot follow")
			return
		}
		cmd.redirs = append(cmd.redirs, r)
		p.docs = append(p.docs, pendingDoc{
			cmd: cmd, idx: len(cmd.redirs) - 1, delim: t.joined(),
			tabs: r.op == "<<-", lit: t.quoted, depth: p.depth,
		})
		return
	case ">", ">>", ">|", "&>", "&>>":
		if t.raw == "!" {
			// zsh reads >! file as "overwrite file"; bash writes a file
			// named "!". Nobody can say which without knowing the shell.
			p.fail("a redirection to \"!\"")
			return
		}
	}
	cmd.redirs = append(cmd.redirs, r)
}

// crudeFragments cuts a command line at everything that could start a new
// command, without regard to quoting. It is used only on lines the reader
// could not follow, and only to look for what deny and ask rules name: it
// finds too much rather than too little.
func crudeFragments(src string) []string {
	src = strings.ReplaceAll(src, "\\\n", "")
	var out []string
	add := func(s string) {
		for {
			s = strings.TrimLeft(s, " \t\r")
			cut := false
			for _, kw := range []string{"do", "then", "else", "elif", "if", "while", "until", "time", "function", "!", "in"} {
				if strings.HasPrefix(s, kw+" ") || strings.HasPrefix(s, kw+"\t") {
					s = s[len(kw)+1:]
					cut = true
				}
			}
			if !cut {
				break
			}
		}
		s = strings.TrimRight(s, " \t\r")
		if s == "" {
			return
		}
		out = append(out, s)
		if bare := strings.NewReplacer("'", "", "\"", "", "\\", "").Replace(s); bare != s && bare != "" {
			out = append(out, bare)
		}
	}
	start := 0
	for i := 0; i < len(src); i++ {
		switch src[i] {
		case '\n', ';', '&', '|', '(', ')', '`', '{', '}', '<', '>':
			add(src[start:i])
			start = i + 1
		}
	}
	add(src[start:])
	// A fragment may open with assignments; rules about the command after
	// them must still be found.
	for _, s := range out {
		rest := s
		for {
			sp := strings.IndexAny(rest, " \t")
			if sp < 0 {
				break
			}
			if _, ok := assignName(rest[:sp]); !ok {
				break
			}
			rest = strings.TrimLeft(rest[sp:], " \t")
			if rest != "" {
				out = append(out, rest)
			}
		}
	}
	return out
}

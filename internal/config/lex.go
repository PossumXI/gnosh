package config

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// rcWord is one word of a gnoshrc line, after quotes are taken off and
// variables are filled in.
type rcWord struct {
	// text is the word's value.
	text string
	// quoted reports whether any part of the word was written in quotes. A
	// quoted word is never read as a flag.
	quoted bool
	// raw is the word as it stands in the file.
	raw string
}

// rcLine is one directive: the words of a line, or of several lines joined
// by a backslash.
type rcLine struct {
	// num is the line the directive starts on, counted from 1.
	num   int
	words []rcWord
}

// shellWords are the first words that mark a line as shell script. gnoshrc
// was a script in the earlier gnosh, so files written for it can hold them.
var shellWords = map[string]string{
	"if": "test", "then": "test", "else": "test", "elif": "test", "fi": "test",
	"case": "test", "esac": "test", "[[": "test", "[": "test", "test": "test",
	"for": "loop", "while": "loop", "until": "loop", "do": "loop", "done": "loop",
	"function": "loop", "{": "loop", "}": "loop", "!": "test",
	"source": "source", ".": "source",
	"export": "variable", "local": "variable", "readonly": "variable", "declare": "variable",
	"typeset": "variable", "set": "variable", "unset": "variable",
	"eval": "command", "exec": "command", "echo": "command", "printf": "command",
	"alias": "command", "cd": "command", "trap": "command", "shift": "command",
	"return": "command", "exit": "command", "true": "command", "false": "command", ":": "command",
}

// shellAdvice says what gnoshrc offers in place of each kind of shell
// construct. The kinds are the values of shellWords, plus the ones the
// lexer finds inside a line.
var shellAdvice = map[string]string{
	"assignment": "Write the value out where it is used: $NAME reads the environment only. To choose between two addresses, give --base-url twice on the provider line; the first that answers is used.",
	"test":       "To choose between two addresses, give --base-url twice on the provider line; the first that answers is used. For settings that differ from machine to machine, keep a file on each machine.",
	"loop":       "Write each setting on a line of its own.",
	"source":     "Put the other file's lines in this file. (An include directive is planned and not built yet.)",
	"variable":   "Set variables in the shell that starts gnosh; $NAME reads them here.",
	"command":    "gnoshrc runs no commands. The directives are: " + directiveNames() + ".",
	"substitute": "gnoshrc runs no commands. For a key kept in a password manager, export it in the shell that starts gnosh and write --api-key \"$NAME\". (--api-key-command is planned and not built yet.)",
	"operator":   "Write one setting per line. If the character belongs to a value, put the value in quotes.",
	"paren":      "Put a word that holds parentheses in quotes, for example 'Bash(git diff *)'.",
	"special":    "Only $NAME, ${NAME} and ${NAME:?message} are read, and only from the environment. For a plain dollar sign, put the word in single quotes.",
}

// lexer turns the text of a gnoshrc into lines of words.
type lexer struct {
	file   string
	src    string
	pos    int
	line   int // the line pos is on
	getenv func(string) (string, bool)
	probs  *problems

	// shellVars holds the names that rejected assignment lines tried to
	// set, with the line of each. A later $name gets a message of its own.
	shellVars map[string]int
	// shellLine is the first line found to be shell script, where the full
	// explanation was given. Zero means none yet.
	shellLine int

	out []rcLine
}

// lexRC splits the text of a gnoshrc into directives. Every mistake is
// recorded in probs with the file and the line, and the line it is on
// yields no directive.
func lexRC(file string, src []byte, getenv func(string) (string, bool), probs *problems) []rcLine {
	if !utf8.Valid(src) {
		probs.errorf(file, lineOfInvalidUTF8(src), "this file is not text (it holds bytes that are not UTF-8), so none of it was read")
		return nil
	}
	lx := &lexer{file: file, src: string(src), line: 1, getenv: getenv, probs: probs, shellVars: map[string]int{}}
	for lx.pos < len(lx.src) {
		lx.lexLine()
	}
	return lx.out
}

// lineOfInvalidUTF8 finds the line of the first byte that is not UTF-8.
func lineOfInvalidUTF8(src []byte) int {
	line := 1
	for len(src) > 0 {
		r, size := utf8.DecodeRune(src)
		if r == utf8.RuneError && size == 1 {
			return line
		}
		if r == '\n' {
			line++
		}
		src = src[size:]
	}
	return line
}

// lineState is what the lexer learns about one directive as it reads it.
type lineState struct {
	start int // the line the directive starts on
	words []rcWord

	cur     strings.Builder
	inWord  bool
	quoted  bool
	wordPos int // where the current word starts in src

	// shellKind and shellWhat describe the first shell construct found.
	shellKind, shellWhat string
	// bad is set once a mistake has been reported for the line.
	bad bool
}

// endWord closes the word being read, if one is.
func (st *lineState) endWord(lx *lexer) {
	if !st.inWord {
		return
	}
	st.words = append(st.words, rcWord{text: st.cur.String(), quoted: st.quoted, raw: lx.src[st.wordPos:lx.pos]})
	st.cur.Reset()
	st.inWord, st.quoted = false, false
}

// begin opens a word at the current position unless one is open.
func (st *lineState) begin(lx *lexer) {
	if !st.inWord {
		st.inWord = true
		st.wordPos = lx.pos
	}
}

// shell notes a shell construct, keeping the first one found on the line.
func (st *lineState) shell(kind, what string) {
	if st.shellKind == "" {
		st.shellKind, st.shellWhat = kind, what
	}
}

// fail reports a mistake on the directive, once.
func (st *lineState) fail(lx *lexer, format string, args ...any) {
	if !st.bad {
		lx.probs.errorf(lx.file, st.start, format, args...)
	}
	st.bad = true
}

// lexLine reads one directive: up to the end of the line, or further when
// lines are joined by a backslash.
func (lx *lexer) lexLine() {
	st := &lineState{start: lx.line}
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		if c == '\n' {
			st.endWord(lx)
			lx.pos++
			lx.line++
			break
		}
		if c == '\r' && lx.peek(1) == '\n' {
			st.endWord(lx)
			lx.pos += 2
			lx.line++
			break
		}
		if c == '\\' && lx.joinsLines() {
			continue
		}
		switch {
		case c == ' ' || c == '\t':
			st.endWord(lx)
			lx.pos++
		case c == '#' && !st.inWord:
			lx.skipComment()
		case c == '\'':
			st.begin(lx)
			st.quoted = true
			lx.single(st)
		case c == '"':
			st.begin(lx)
			st.quoted = true
			lx.double(st)
		case c == '\\':
			st.fail(lx, "a backslash only joins a line to the next one. To keep a space or another special character in a word, put the word in quotes")
			lx.pos++
		case c == '$':
			st.begin(lx)
			lx.dollar(st)
		case c == '`':
			st.shell("substitute", "`…`")
			lx.pos++
		case c == ';' || c == '&' || c == '|' || c == '<' || c == '>':
			st.endWord(lx)
			end := lx.pos + 1
			for end < len(lx.src) && strings.IndexByte(";&|<>", lx.src[end]) >= 0 {
				end++
			}
			st.shell("operator", lx.src[lx.pos:end])
			lx.pos = end
		case c == '(' || c == ')':
			st.begin(lx)
			st.shell("paren", string(c))
			lx.pos++
		case c < 0x20 || c == 0x7f:
			st.fail(lx, "the line holds a control character (code %d), which gnoshrc does not allow", c)
			lx.pos++
		default:
			st.begin(lx)
			st.cur.WriteByte(c)
			lx.pos++
		}
	}
	st.endWord(lx)
	lx.finishLine(st)
}

// peek returns the byte n places ahead, or 0 past the end.
func (lx *lexer) peek(n int) byte {
	if lx.pos+n < len(lx.src) {
		return lx.src[lx.pos+n]
	}
	return 0
}

// joinsLines reports whether the backslash at pos ends its line, and if so
// steps over it and the line break, so that the next line continues this
// one.
func (lx *lexer) joinsLines() bool {
	switch {
	case lx.peek(1) == '\n':
		lx.pos += 2
	case lx.peek(1) == '\r' && lx.peek(2) == '\n':
		lx.pos += 3
	default:
		return false
	}
	lx.line++
	return true
}

// skipComment steps to the end of the line. A backslash at the end of a
// comment joins nothing: a comment never swallows the line after it.
func (lx *lexer) skipComment() {
	for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
		lx.pos++
	}
}

// single reads a single-quoted part, which keeps everything as written.
func (lx *lexer) single(st *lineState) {
	lx.pos++ // the opening quote
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		switch {
		case c == '\'':
			lx.pos++
			return
		case c == '\n':
			st.fail(lx, "the single quote opened on this line is not closed on it")
			return
		case c == '\r' && lx.peek(1) == '\n':
			st.fail(lx, "the single quote opened on this line is not closed on it")
			return
		case c < 0x20 && c != '\t' || c == 0x7f:
			st.fail(lx, "the line holds a control character (code %d), which gnoshrc does not allow", c)
		default:
			st.cur.WriteByte(c)
		}
		lx.pos++
	}
	st.fail(lx, "the single quote opened on this line is not closed")
}

// double reads a double-quoted part: spaces are kept, variables are filled
// in, and a backslash keeps a quote, a backslash, a dollar sign or a
// backquote as it is.
func (lx *lexer) double(st *lineState) {
	lx.pos++ // the opening quote
	for lx.pos < len(lx.src) {
		c := lx.src[lx.pos]
		switch {
		case c == '"':
			lx.pos++
			return
		case c == '\n' || c == '\r' && lx.peek(1) == '\n':
			st.fail(lx, "the double quote opened on this line is not closed on it")
			return
		case c == '\\':
			if lx.joinsLines() {
				continue
			}
			if n := lx.peek(1); n == '"' || n == '\\' || n == '$' || n == '`' {
				st.cur.WriteByte(n)
				lx.pos += 2
				continue
			}
			st.cur.WriteByte(c)
		case c == '$':
			lx.dollar(st)
			continue
		case c == '`':
			st.shell("substitute", "`…`")
		case c < 0x20 && c != '\t' || c == 0x7f:
			st.fail(lx, "the line holds a control character (code %d), which gnoshrc does not allow", c)
		default:
			st.cur.WriteByte(c)
		}
		lx.pos++
	}
	st.fail(lx, "the double quote opened on this line is not closed")
}

// dollar reads what follows a dollar sign: $NAME, ${NAME}, ${NAME:?message},
// or something of the shell's that gnoshrc does not do.
func (lx *lexer) dollar(st *lineState) {
	next := lx.peek(1)
	switch {
	case next == '(':
		st.shell("substitute", "$(…)")
		lx.pos += 2
	case next == '{':
		end := strings.IndexAny(lx.src[lx.pos:], "}\n")
		if end < 0 || lx.src[lx.pos+end] != '}' {
			st.fail(lx, "the ${ on this line is not closed with }")
			lx.pos += 2
			return
		}
		inner := lx.src[lx.pos+2 : lx.pos+end]
		lx.pos += end + 1
		name, message, must := inner, "", false
		if i := strings.Index(inner, ":?"); i >= 0 {
			name, message, must = inner[:i], inner[i+2:], true
		}
		if !isName(name) {
			st.shell("special", "${"+clip(inner, 20)+"}")
			return
		}
		lx.variable(st, name, must, message)
	case isNameStart(next):
		end := lx.pos + 1
		for end < len(lx.src) && isNameChar(lx.src[end]) {
			end++
		}
		name := lx.src[lx.pos+1 : end]
		lx.pos = end
		lx.variable(st, name, false, "")
	case next >= '0' && next <= '9' || next != 0 && strings.IndexByte("?$!@*#-", next) >= 0:
		st.shell("special", "$"+string(next))
		lx.pos += 2
	default:
		st.cur.WriteByte('$')
		lx.pos++
	}
}

// variable fills in an environment variable. An unset one is read as
// nothing, with a warning, unless the file said it must be set.
func (lx *lexer) variable(st *lineState, name string, must bool, message string) {
	if runTimeVariable(name) {
		// gnosh sets this one itself, anew for each hook and command it
		// runs. Filling it in now would bake in nothing, or the value of
		// some other session that happens to be in the environment.
		st.fail(lx, "$%s is filled in when gnosh runs a hook or a command, not when this file is read. To hand it on to the command, put the word in single quotes, which keep it as written", name)
		return
	}
	value, ok := lx.getenv(name)
	switch {
	case ok && (value != "" || !must):
		st.cur.WriteString(value)
	case must:
		if strings.TrimSpace(message) == "" {
			message = "this file says it must be set"
		}
		st.fail(lx, "the environment variable %s is not set: %s", name, clip(message, 200))
	case lx.shellVars[name] != 0:
		st.fail(lx, "$%s is not set. Line %d tried to set it as a shell variable, which gnoshrc does not do: write the value out here", name, lx.shellVars[name])
	default:
		lx.probs.warnf(lx.file, st.start, "the environment variable %s is not set, so $%s was read as nothing", name, name)
	}
}

// finishLine decides what a line that has been read is: a directive, a
// piece of shell script, or nothing.
func (lx *lexer) finishLine(st *lineState) {
	if len(st.words) > 0 && !st.words[0].quoted {
		first := st.words[0].raw
		if name, ok := assignmentName(first); ok {
			st.shellKind, st.shellWhat = "assignment", name+"=…"
			for _, w := range st.words {
				if n, ok := assignmentName(w.raw); ok && !w.quoted && lx.shellVars[n] == 0 {
					lx.shellVars[n] = st.start
				}
			}
		} else if kind, ok := shellWords[first]; ok {
			st.shellKind, st.shellWhat = kind, first
			for _, w := range st.words[1:] {
				if n, ok := assignmentName(w.raw); ok && !w.quoted && lx.shellVars[n] == 0 {
					lx.shellVars[n] = st.start
				}
			}
		}
	}
	switch {
	case st.shellKind != "":
		if st.bad {
			return
		}
		what := fmt.Sprintf("%q", st.shellWhat)
		if lx.shellLine == 0 {
			lx.shellLine = st.start
			lx.probs.errorf(lx.file, st.start, "gnoshrc is a list of settings, one to a line, and not a shell script: %s is shell syntax. %s", what, shellAdvice[st.shellKind])
		} else {
			lx.probs.errorf(lx.file, st.start, "%s is shell syntax too, and gnoshrc is not a script (see line %d). %s", what, lx.shellLine, shellAdvice[st.shellKind])
		}
	case st.bad || len(st.words) == 0:
	default:
		lx.out = append(lx.out, rcLine{num: st.start, words: st.words})
	}
}

// assignmentName reports whether a word as written is NAME=value, the
// shell's way to set a variable, and returns the name.
func assignmentName(raw string) (string, bool) {
	i := strings.IndexByte(raw, '=')
	if i <= 0 {
		return "", false
	}
	name := strings.TrimSuffix(raw[:i], "+")
	return name, isName(name)
}

// isName reports whether s is a variable name: letters, digits and
// underscores, not starting with a digit.
func isName(s string) bool {
	if s == "" || !isNameStart(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !isNameChar(s[i]) {
			return false
		}
	}
	return true
}

func isNameStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isNameChar(c byte) bool {
	return isNameStart(c) || c >= '0' && c <= '9'
}

// clip cuts s to at most n bytes for a message, on a character boundary,
// and drops control characters.
func clip(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len()+utf8.RuneLen(r) > n {
			b.WriteString("…")
			break
		}
		if r < 0x20 || r == 0x7f {
			r = ' '
		}
		b.WriteRune(r)
	}
	return b.String()
}

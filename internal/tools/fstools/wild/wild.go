// Package wild matches file names and slash-separated paths against
// wildcard patterns. It is the matcher under gnosh's Glob tool, the glob
// filter of its Grep tool, and its gitignore rules.
//
// # Syntax
//
// Inside one name (one path segment):
//
//	"*"            any run of characters, including none
//	"?"            exactly one character
//	"[abc]"        one of the listed characters
//	"[a-z]"        one character in the range
//	"[!abc]"       one character not listed; "[^abc]" means the same
//	"[[:alpha:]]"  a POSIX class inside brackets: alnum, alpha, blank,
//	               cntrl, digit, graph, lower, print, punct, space, upper,
//	               xdigit (ASCII only)
//	"\c"           the character c itself, with no special meaning
//
// A "]" straight after the opening bracket (or after the "!" or "^") is an
// ordinary member, and so is a "-" that cannot be read as a range. None of
// these ever matches a slash.
//
// Across names:
//
//	"/"   separates segments; a pattern and a path must have the same
//	      number of segments unless the pattern uses "**"
//	"**"  as a whole segment, any number of segments: none or more when
//	      something follows it ("**/x", "a/**/b"), one or more when it is
//	      the last segment ("a/**"); three stars or more mean the same
//
// Stars in a row that are not a whole segment ("a**b") mean the same as
// one.
//
// The Glob form adds alternatives: "{a,b}" stands for a or b. Groups nest
// and may hold slashes; a brace after a backslash is a plain brace.
//
// Matching is by character: "?" and a bracket take one whole UTF-8
// character, and a byte that is not valid UTF-8 counts as one character.
// Letter case always matters.
//
// For patterns that use neither "**", braces, "[!" nor "[:", and that the
// standard library's path.Match accepts, Pattern.Match agrees with
// path.Match, with two exceptions: a bracket here never matches a slash,
// and a star here grows by whole characters, where path.Match grows it by
// bytes and so lets "*??" match one three-byte character.
package wild

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// kind says what a token matches.
type kind uint8

const (
	kindLiteral kind = iota // the bytes in text
	kindAny                 // one character
	kindStar                // any run of characters
	kindClass               // one character from a set
	kindSlash               // a segment boundary; only while parsing
)

// token is one piece of a segment pattern.
type token struct {
	kind  kind
	text  string
	class *charClass
}

// charClass is a bracket expression.
type charClass struct {
	negated bool
	ranges  []runeRange
	named   []func(rune) bool
}

// runeRange is the characters from lo to hi, both included.
type runeRange struct{ lo, hi rune }

func (c *charClass) matches(r rune) bool {
	in := false
	for _, rg := range c.ranges {
		if rg.lo <= r && r <= rg.hi {
			in = true
			break
		}
	}
	if !in {
		for _, f := range c.named {
			if f(r) {
				in = true
				break
			}
		}
	}
	return in != c.negated
}

// posixClasses are the names allowed in [[:name:]]. They cover ASCII only,
// as the C locale does.
var posixClasses = map[string]func(rune) bool{
	"alnum":  func(r rune) bool { return isASCIILetter(r) || isASCIIDigit(r) },
	"alpha":  isASCIILetter,
	"blank":  func(r rune) bool { return r == ' ' || r == '\t' },
	"cntrl":  func(r rune) bool { return r < 0x20 || r == 0x7f },
	"digit":  isASCIIDigit,
	"graph":  func(r rune) bool { return r > 0x20 && r < 0x7f },
	"lower":  func(r rune) bool { return r >= 'a' && r <= 'z' },
	"print":  func(r rune) bool { return r >= 0x20 && r < 0x7f },
	"punct":  func(r rune) bool { return r > 0x20 && r < 0x7f && !isASCIILetter(r) && !isASCIIDigit(r) },
	"space":  func(r rune) bool { return r == ' ' || (r >= '\t' && r <= '\r') },
	"upper":  func(r rune) bool { return r >= 'A' && r <= 'Z' },
	"xdigit": func(r rune) bool { return isASCIIDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') },
}

func isASCIILetter(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func isASCIIDigit(r rune) bool  { return r >= '0' && r <= '9' }

// SyntaxError says what is wrong with a pattern.
type SyntaxError struct {
	// Pattern is the pattern as given.
	Pattern string
	// Problem says what is wrong, in plain words.
	Problem string
}

// Error names the pattern and says what is wrong with it.
func (e *SyntaxError) Error() string {
	return fmt.Sprintf("bad pattern %q: %s", e.Pattern, e.Problem)
}

func syntaxError(pattern, problem string) error {
	return &SyntaxError{Pattern: pattern, Problem: problem}
}

// parseClass reads the bracket expression that opens at pattern[open]. It
// returns the class and the index just past its closing bracket.
func parseClass(pattern string, open int) (*charClass, int, error) {
	i := open + 1
	c := &charClass{}
	if i < len(pattern) && (pattern[i] == '!' || pattern[i] == '^') {
		c.negated = true
		i++
	}
	first := true
	for {
		if i >= len(pattern) {
			return nil, 0, syntaxError(pattern, "a [ has no closing ]")
		}
		if pattern[i] == ']' && !first {
			return c, i + 1, nil
		}
		first = false
		if strings.HasPrefix(pattern[i:], "[:") {
			if end := strings.Index(pattern[i+2:], ":]"); end >= 0 {
				name := pattern[i+2 : i+2+end]
				f, ok := posixClasses[name]
				if !ok {
					return nil, 0, syntaxError(pattern, fmt.Sprintf("[:%s:] is not a character class", name))
				}
				c.named = append(c.named, f)
				i += 2 + end + 2
				continue
			}
		}
		lo, next, err := classChar(pattern, i)
		if err != nil {
			return nil, 0, err
		}
		i = next
		hi := lo
		if i+1 < len(pattern) && pattern[i] == '-' && pattern[i+1] != ']' {
			hi, next, err = classChar(pattern, i+1)
			if err != nil {
				return nil, 0, err
			}
			i = next
		}
		c.ranges = append(c.ranges, runeRange{lo, hi})
	}
}

// classChar reads one member of a bracket expression at pattern[i]: a
// character, or a backslash and the character it protects.
func classChar(pattern string, i int) (rune, int, error) {
	if pattern[i] == '\\' {
		i++
		if i >= len(pattern) {
			return 0, 0, syntaxError(pattern, "a \\ has nothing after it")
		}
	}
	r, n := utf8.DecodeRuneInString(pattern[i:])
	if r == utf8.RuneError && n == 1 {
		return 0, 0, syntaxError(pattern, "a [ ] holds a byte that is not valid UTF-8")
	}
	return r, i + n, nil
}

// tokenize splits a pattern into tokens. Stars are kept one token each so
// that the caller can tell "**" from "*".
func tokenize(pattern string) ([]token, error) {
	var toks []token
	var lit []byte
	flush := func() {
		if len(lit) > 0 {
			toks = append(toks, token{kind: kindLiteral, text: string(lit)})
			lit = lit[:0]
		}
	}
	for i := 0; i < len(pattern); {
		switch c := pattern[i]; c {
		case '/':
			flush()
			toks = append(toks, token{kind: kindSlash})
			i++
		case '*':
			flush()
			toks = append(toks, token{kind: kindStar})
			i++
		case '?':
			flush()
			toks = append(toks, token{kind: kindAny})
			i++
		case '[':
			cl, next, err := parseClass(pattern, i)
			if err != nil {
				return nil, err
			}
			flush()
			toks = append(toks, token{kind: kindClass, class: cl})
			i = next
		case '\\':
			if i+1 >= len(pattern) {
				return nil, syntaxError(pattern, "a \\ has nothing after it")
			}
			if pattern[i+1] == '/' {
				flush()
				toks = append(toks, token{kind: kindSlash})
			} else {
				lit = append(lit, pattern[i+1])
			}
			i += 2
		default:
			lit = append(lit, c)
			i++
		}
	}
	flush()
	return toks, nil
}

// Segment is a compiled pattern for one name: it never matches across a
// slash. The zero value matches only the empty name.
type Segment struct {
	// toks is the pattern when it has a wildcard in it; otherwise it is
	// empty and lit is the one name the segment matches.
	toks []token
	lit  string
}

// CompileSegment compiles a pattern that holds no slash. In it "**" means
// the same as "*".
func CompileSegment(pattern string) (Segment, error) {
	toks, err := tokenize(pattern)
	if err != nil {
		return Segment{}, err
	}
	for _, t := range toks {
		if t.kind == kindSlash {
			return Segment{}, syntaxError(pattern, "a pattern for one name cannot hold a /")
		}
	}
	return newSegment(toks), nil
}

// newSegment builds a Segment from tokens that hold no slash.
func newSegment(toks []token) Segment {
	out := make([]token, 0, len(toks))
	for _, t := range toks {
		if t.kind == kindStar && len(out) > 0 && out[len(out)-1].kind == kindStar {
			continue
		}
		out = append(out, t)
	}
	switch {
	case len(out) == 0:
		return Segment{}
	case len(out) == 1 && out[0].kind == kindLiteral:
		return Segment{lit: out[0].text}
	}
	return Segment{toks: out}
}

// Literal returns the one name the segment matches, and true, when the
// pattern has no wildcard in it.
func (s Segment) Literal() (string, bool) {
	if len(s.toks) == 0 {
		return s.lit, true
	}
	return "", false
}

// Match reports whether the segment matches the whole of name.
func (s Segment) Match(name string) bool {
	if len(s.toks) == 0 {
		return name == s.lit
	}
	toks := s.toks
	ti, ni := 0, 0
	// starTok is the token after the last star passed, and starPos is where
	// in name that star's run currently ends. On a mismatch the run grows
	// by one character and matching resumes from there.
	starTok, starPos := -1, 0
	for {
		if ti < len(toks) {
			t := &toks[ti]
			switch t.kind {
			case kindStar:
				ti++
				starTok, starPos = ti, ni
				continue
			case kindLiteral:
				if strings.HasPrefix(name[ni:], t.text) {
					ni += len(t.text)
					ti++
					continue
				}
			case kindAny:
				if ni < len(name) {
					_, n := utf8.DecodeRuneInString(name[ni:])
					ni += n
					ti++
					continue
				}
			case kindClass:
				if ni < len(name) {
					r, n := utf8.DecodeRuneInString(name[ni:])
					if t.class.matches(r) {
						ni += n
						ti++
						continue
					}
				}
			}
		} else if ni == len(name) {
			return true
		}
		if starTok < 0 || starPos >= len(name) {
			return false
		}
		_, n := utf8.DecodeRuneInString(name[starPos:])
		starPos += n
		ti, ni = starTok, starPos
	}
}

// pathSeg is one segment of a path pattern: a name pattern, or "**".
type pathSeg struct {
	double bool
	seg    Segment
}

// Pattern is a compiled pattern for a slash-separated path.
type Pattern struct {
	segs []pathSeg
}

// Compile compiles a path pattern. Braces are ordinary characters here;
// CompileGlob is the form that expands them.
func Compile(pattern string) (*Pattern, error) {
	toks, err := tokenize(pattern)
	if err != nil {
		return nil, err
	}
	p := &Pattern{}
	start := 0
	for i := 0; i <= len(toks); i++ {
		if i < len(toks) && toks[i].kind != kindSlash {
			continue
		}
		group := toks[start:i]
		if allStars(group) {
			p.segs = append(p.segs, pathSeg{double: true})
		} else {
			p.segs = append(p.segs, pathSeg{seg: newSegment(group)})
		}
		start = i + 1
	}
	return p, nil
}

// allStars reports whether a segment is two stars or more and nothing
// else. Git reads three stars between slashes as it reads two, and so does
// this.
func allStars(group []token) bool {
	if len(group) < 2 {
		return false
	}
	for _, t := range group {
		if t.kind != kindStar {
			return false
		}
	}
	return true
}

// Match reports whether the pattern matches the whole of path.
func (p *Pattern) Match(path string) bool {
	return p.run(path, false)
}

// MatchPrefix reports whether some path below the directory dir could
// match: that is, whether the pattern can match dir's segments and still
// have at least one segment left to match what lies under it. A walk uses
// it to skip directories that cannot hold a match.
func (p *Pattern) MatchPrefix(dir string) bool {
	return p.run(dir, true)
}

// run steps through path one segment at a time, keeping the set of
// positions in the pattern that the segments so far could have reached.
// Position i means "segs[i] is next"; position len(segs) means "done".
func (p *Pattern) run(path string, prefix bool) bool {
	n := len(p.segs)
	var buf [34]bool
	var cur, next []bool
	if 2*(n+1) <= len(buf) {
		cur, next = buf[:n+1:n+1], buf[n+1:2*(n+1)]
	} else {
		cur, next = make([]bool, n+1), make([]bool, n+1)
	}
	cur[0] = true
	p.skipDoubles(cur)
	rest, more := path, true
	for more {
		var name string
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			name, rest = rest[:i], rest[i+1:]
		} else {
			name, more = rest, false
		}
		clear(next)
		alive := false
		for i := 0; i < n; i++ {
			if !cur[i] {
				continue
			}
			if p.segs[i].double {
				next[i] = true
				alive = true
				if i == n-1 {
					next[n] = true
				}
			} else if p.segs[i].seg.Match(name) {
				next[i+1] = true
				alive = true
			}
		}
		if !alive {
			return false
		}
		p.skipDoubles(next)
		cur, next = next, cur
	}
	if prefix {
		for i := 0; i < n; i++ {
			if cur[i] {
				return true
			}
		}
		return false
	}
	return cur[n]
}

// skipDoubles adds, for every "**" that may match no segments at all, the
// position after it. The last segment's "**" is not one of those: it must
// match at least one segment.
func (p *Pattern) skipDoubles(set []bool) {
	n := len(p.segs)
	for i := 0; i < n-1; i++ {
		if set[i] && p.segs[i].double {
			set[i+1] = true
		}
	}
}

// literalDirs returns the names of the leading segments that have no
// wildcard, never counting the last segment. It stops at an empty segment
// (as in "a//b"), which no name can be.
func (p *Pattern) literalDirs() []string {
	var out []string
	for i := 0; i < len(p.segs)-1; i++ {
		if p.segs[i].double {
			break
		}
		name, ok := p.segs[i].seg.Literal()
		if !ok || name == "" {
			break
		}
		out = append(out, name)
	}
	return out
}

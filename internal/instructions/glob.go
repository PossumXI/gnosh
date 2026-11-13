package instructions

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// This file matches paths against the patterns a rule lists. The patterns
// work the way ignore-file patterns do, read against the rule's base
// directory:
//
//	*.md                  ".md" files at any depth (no slash: any depth)
//	/*.md                 ".md" files directly in the base
//	src/components/*.tsx  ".tsx" files directly in that directory
//	src/**/*.ts           ".ts" files anywhere under src
//	src/**                everything under src
//	src/                  the same: a pattern that matches a directory
//	                      matches everything under it
//	*.{ts,tsx}            braces list alternatives
//
// "*" matches any run of characters within one name, "?" one character,
// "[abc]", "[a-z]" and "[!abc]" one character from (or not from) a set, and
// "**" as a whole name matches any number of directories. A backslash makes
// the next character plain. Letter case counts. There is no negation.

const (
	// patternBudget is how many patterns one rule's list may come to once
	// its braces are written out. Past it, patterns are left out and
	// reported, so that a few characters of braces cannot turn into
	// millions of patterns.
	patternBudget = 1000
	// maxPatternLen is the longest pattern that is read, in bytes.
	maxPatternLen = 4096
)

// pattern is one compiled path pattern.
type pattern struct {
	segs []segment
}

// segment is the part of a pattern that matches one name of a path, or, for
// "**", any number of names.
type segment struct {
	deep bool
	toks []token
}

// token is one piece of a name pattern.
type token struct {
	kind tokenKind
	lit  rune   // tokLit
	set  []span // tokSet
	not  bool   // tokSet: match what is outside the set
}

type tokenKind uint8

const (
	tokLit  tokenKind = iota // one given character
	tokAny                   // "?": any one character
	tokStar                  // "*": any run of characters
	tokSet                   // "[...]": one character from a set
)

// span is a range of characters in a set, both ends included.
type span struct{ lo, hi rune }

// badPattern is a pattern that could not be used, and why.
type badPattern struct {
	src string
	why string
}

// compilePatterns compiles a rule's list of patterns. Braces are written
// out first. A pattern that cannot be read matches nothing and is reported;
// so is every pattern past the budget.
func compilePatterns(list []string) ([]pattern, []badPattern) {
	var pats []pattern
	var bad []badPattern
	room := patternBudget
	for i, src := range list {
		if len(src) > maxPatternLen {
			bad = append(bad, badPattern{src[:40] + "...", fmt.Sprintf("it is longer than %d bytes", maxPatternLen)})
			continue
		}
		var alts []string
		if !expandBraces(src, &room, &alts) {
			bad = append(bad, badPattern{src, fmt.Sprintf("the patterns of one rule come to more than %d once their braces are written out; this one and those after it are left out", patternBudget)})
			for _, rest := range list[i+1:] {
				if len(rest) > 40 {
					rest = rest[:40] + "..."
				}
				bad = append(bad, badPattern{rest, "left out, as above"})
			}
			break
		}
		for _, alt := range alts {
			p, err := compilePattern(alt)
			if err != nil {
				bad = append(bad, badPattern{alt, err.Error()})
				continue
			}
			pats = append(pats, p)
		}
	}
	return pats, bad
}

// expandBraces appends to out every pattern that p's brace groups stand
// for: "a.{b,c}" gives "a.b" and "a.c". Groups nest. Braces with no comma
// between them, or with no partner, are plain characters. It takes one from
// room for each pattern and reports false when room runs out.
func expandBraces(p string, room *int, out *[]string) bool {
	open, end, commas := firstGroup(p)
	if open < 0 {
		if *room <= 0 {
			return false
		}
		*room--
		*out = append(*out, p)
		return true
	}
	prefix, suffix := p[:open], p[end+1:]
	start := open + 1
	for _, stop := range append(commas, end) {
		var tails []string
		if !expandBraces(p[start:stop]+suffix, room, &tails) {
			return false
		}
		for _, tail := range tails {
			*out = append(*out, prefix+tail)
		}
		start = stop + 1
	}
	return true
}

// firstGroup finds the first brace group in p that lists alternatives: the
// positions of its braces and of the commas between its alternatives.
func firstGroup(p string) (open, end int, commas []int) {
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '{':
			if end, commas, ok := groupEnd(p, i); ok {
				return i, end, commas
			}
		}
	}
	return -1, -1, nil
}

// groupEnd finds the brace that closes the one at p[open] and the commas
// that separate the alternatives between them. ok is false when the brace
// has no partner or the group holds no comma of its own.
func groupEnd(p string, open int) (end int, commas []int, ok bool) {
	depth := 0
	for i := open; i < len(p); i++ {
		switch p[i] {
		case '\\':
			i++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, commas, len(commas) > 0
			}
		case ',':
			if depth == 1 {
				commas = append(commas, i)
			}
		}
	}
	return -1, nil, false
}

// compilePattern compiles one pattern whose braces have been written out.
func compilePattern(src string) (pattern, error) {
	p := strings.TrimPrefix(src, "./")
	anchored := strings.HasPrefix(p, "/")
	p = strings.Trim(p, "/")
	if p == "" {
		return pattern{}, errors.New("it is empty")
	}
	names := strings.Split(p, "/")
	if len(names) > 1 {
		anchored = true
	}
	var segs []segment
	if !anchored {
		// A pattern with no slash matches at any depth.
		segs = append(segs, segment{deep: true})
	}
	for _, name := range names {
		switch name {
		case "":
			continue
		case "**":
			if n := len(segs); n == 0 || !segs[n-1].deep {
				segs = append(segs, segment{deep: true})
			}
			continue
		}
		toks, err := compileName(name)
		if err != nil {
			return pattern{}, err
		}
		segs = append(segs, segment{toks: toks})
	}
	return pattern{segs: segs}, nil
}

// compileName compiles the pattern for one name of a path.
func compileName(name string) ([]token, error) {
	var toks []token
	for i := 0; i < len(name); {
		r, size := utf8.DecodeRuneInString(name[i:])
		switch r {
		case '*':
			if n := len(toks); n == 0 || toks[n-1].kind != tokStar {
				toks = append(toks, token{kind: tokStar})
			}
			i++
		case '?':
			toks = append(toks, token{kind: tokAny})
			i++
		case '\\':
			if i+1 >= len(name) {
				return nil, errors.New("it ends with a backslash")
			}
			lit, n := utf8.DecodeRuneInString(name[i+1:])
			toks = append(toks, token{kind: tokLit, lit: lit})
			i += 1 + n
		case '[':
			set, n, err := compileSet(name[i:])
			if err != nil {
				return nil, err
			}
			toks = append(toks, set)
			i += n
		default:
			toks = append(toks, token{kind: tokLit, lit: r})
			i += size
		}
	}
	return toks, nil
}

// compileSet compiles the "[...]" that s opens with and returns it with its
// length in bytes.
func compileSet(s string) (token, int, error) {
	unclosed := errors.New("it has a [ that no ] closes; write \\[ for a plain bracket")
	t := token{kind: tokSet}
	i := 1
	if i < len(s) && (s[i] == '!' || s[i] == '^') {
		t.not = true
		i++
	}
	// next reads one character of the set, which a backslash makes plain.
	next := func() (rune, bool) {
		if i >= len(s) {
			return 0, false
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		if r == '\\' {
			if i >= len(s) {
				return 0, false
			}
			r, size = utf8.DecodeRuneInString(s[i:])
			i += size
		}
		return r, true
	}
	for first := true; ; first = false {
		if i >= len(s) {
			return token{}, 0, unclosed
		}
		if s[i] == ']' && !first {
			i++
			return t, i, nil
		}
		if s[i] == '[' && i+1 < len(s) && (s[i+1] == ':' || s[i+1] == '.' || s[i+1] == '=') {
			return token{}, 0, errors.New("named character classes such as [:alpha:] are not read here")
		}
		lo, ok := next()
		if !ok {
			return token{}, 0, unclosed
		}
		hi := lo
		if i+1 < len(s) && s[i] == '-' && s[i+1] != ']' {
			i++
			if hi, ok = next(); !ok {
				return token{}, 0, unclosed
			}
			if hi < lo {
				return token{}, 0, fmt.Errorf("the range %c-%c in a set runs backwards", lo, hi)
			}
		}
		t.set = append(t.set, span{lo, hi})
	}
}

// match reports whether the pattern matches rel, a slash-separated path
// relative to the pattern's base directory. A pattern that matches one of
// the directories rel lies in matches rel.
func (p pattern) match(rel string) bool {
	names := strings.Split(rel, "/")
	// memo holds, for each pair of a place in the pattern and a place in
	// the path, whether the rest of one matches the rest of the other, so
	// that several "**" cannot make the work multiply.
	memo := make([]int8, (len(p.segs)+1)*(len(names)+1))
	var rest func(si, ni int) bool
	rest = func(si, ni int) bool {
		if si == len(p.segs) {
			return true
		}
		at := si*(len(names)+1) + ni
		if memo[at] != 0 {
			return memo[at] > 0
		}
		ok := false
		switch seg := p.segs[si]; {
		case seg.deep && si == len(p.segs)-1:
			ok = ni < len(names) // a closing "**" needs something under it
		case seg.deep:
			ok = rest(si+1, ni) || (ni < len(names) && rest(si, ni+1))
		case ni < len(names):
			ok = matchName(seg.toks, names[ni]) && rest(si+1, ni+1)
		}
		memo[at] = -1
		if ok {
			memo[at] = 1
		}
		return ok
	}
	return rest(0, 0)
}

// matchName reports whether a name pattern matches the whole of name.
func matchName(toks []token, name string) bool {
	runes := []rune(name)
	t, n := 0, 0
	starT, starN := -1, 0
	for n < len(runes) {
		switch {
		case t < len(toks) && toks[t].kind == tokStar:
			starT, starN = t, n
			t++
		case t < len(toks) && toks[t].takes(runes[n]):
			t++
			n++
		case starT >= 0:
			// Let the last "*" take one more character and try again.
			starN++
			t, n = starT+1, starN
		default:
			return false
		}
	}
	for t < len(toks) && toks[t].kind == tokStar {
		t++
	}
	return t == len(toks)
}

// takes reports whether a token other than "*" matches the character r.
func (t token) takes(r rune) bool {
	switch t.kind {
	case tokLit:
		return r == t.lit
	case tokAny:
		return true
	case tokSet:
		in := false
		for _, s := range t.set {
			if s.lo <= r && r <= s.hi {
				in = true
				break
			}
		}
		return in != t.not
	}
	return false
}

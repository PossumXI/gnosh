package wild

import (
	"fmt"
	"strings"
)

// MaxAlternatives is the most patterns one Glob may expand into. A pattern
// such as {a,b}{c,d}{e,f}... doubles with every group; past this many it is
// refused rather than expanded.
const MaxAlternatives = 1024

// Glob is a compiled Glob-tool pattern: a path pattern in which {a,b}
// stands for alternatives. It matches when any of its alternatives does.
type Glob struct {
	alts []*Pattern
}

// CompileGlob expands the braces in pattern and compiles each alternative.
func CompileGlob(pattern string) (*Glob, error) {
	texts, err := Expand(pattern, MaxAlternatives)
	if err != nil {
		return nil, err
	}
	g := &Glob{alts: make([]*Pattern, 0, len(texts))}
	for _, t := range texts {
		p, err := Compile(t)
		if err != nil {
			// Report the pattern the caller wrote, not the expansion.
			if se, ok := err.(*SyntaxError); ok {
				return nil, syntaxError(pattern, se.Problem)
			}
			return nil, err
		}
		g.alts = append(g.alts, p)
	}
	return g, nil
}

// Match reports whether any alternative matches the whole of path.
func (g *Glob) Match(path string) bool {
	for _, p := range g.alts {
		if p.Match(path) {
			return true
		}
	}
	return false
}

// MatchPrefix reports whether some path below the directory dir could match
// any alternative.
func (g *Glob) MatchPrefix(dir string) bool {
	for _, p := range g.alts {
		if p.MatchPrefix(dir) {
			return true
		}
	}
	return false
}

// SplitDir splits off the directories every alternative starts with by
// name. It returns them joined with slashes ("" when there are none) and
// the Glob that is left to match below that directory. The last segment of
// an alternative is never split off, so the rest always has something to
// match. A search uses this to start where the pattern points instead of at
// the top.
func (g *Glob) SplitDir() (dir string, rest *Glob) {
	if len(g.alts) == 0 {
		return "", g
	}
	common := g.alts[0].literalDirs()
	for _, p := range g.alts[1:] {
		dirs := p.literalDirs()
		n := 0
		for n < len(common) && n < len(dirs) && common[n] == dirs[n] {
			n++
		}
		common = common[:n]
	}
	if len(common) == 0 {
		return "", g
	}
	rest = &Glob{alts: make([]*Pattern, len(g.alts))}
	for i, p := range g.alts {
		rest.alts[i] = &Pattern{segs: p.segs[len(common):]}
	}
	return strings.Join(common, "/"), rest
}

// Expand returns the patterns that pattern stands for once every {a,b}
// group is replaced by each of its alternatives, in order. A group may hold
// other groups. A brace after a backslash or inside a bracket expression is
// an ordinary character, and so is a "}" with no "{" before it. A "{" that
// is never closed is an error, and so is a pattern that would expand into
// more than limit patterns.
func Expand(pattern string, limit int) ([]string, error) {
	// work holds patterns that may still have groups in them. Every step
	// takes one, and either finishes it or replaces it with one pattern
	// per alternative of its first group, each shorter than it was. Every
	// pattern waiting in work ends as at least one result, so the counts
	// below never refuse a pattern that stays within limit.
	var out []string
	work := []string{pattern}
	for len(work) > 0 {
		p := work[len(work)-1]
		work = work[:len(work)-1]
		open := findOpenBrace(p)
		if open < 0 {
			out = append(out, p)
			if len(out) > limit {
				return nil, syntaxError(pattern, fmt.Sprintf("its { } groups stand for more than %d patterns", limit))
			}
			continue
		}
		parts, end, ok := splitGroup(p, open)
		if !ok {
			return nil, syntaxError(pattern, "a { has no closing }")
		}
		if len(work)+len(out)+len(parts) > limit {
			return nil, syntaxError(pattern, fmt.Sprintf("its { } groups stand for more than %d patterns", limit))
		}
		// Pushed in reverse so that the first alternative is expanded first
		// and the results come out in the order written.
		for i := len(parts) - 1; i >= 0; i-- {
			work = append(work, p[:open]+parts[i]+p[end:])
		}
	}
	return out, nil
}

// skipSpecial returns the index after the escape or bracket expression that
// starts at p[i], or i when neither starts there.
func skipSpecial(p string, i int) int {
	switch p[i] {
	case '\\':
		if i+1 < len(p) {
			return i + 2
		}
		return i + 1
	case '[':
		if _, next, err := parseClass(p, i); err == nil {
			return next
		}
	}
	return i
}

// findOpenBrace returns the index of the first "{" that opens a group, or
// -1.
func findOpenBrace(p string) int {
	for i := 0; i < len(p); {
		if next := skipSpecial(p, i); next != i {
			i = next
			continue
		}
		if p[i] == '{' {
			return i
		}
		i++
	}
	return -1
}

// splitGroup splits the group that opens at p[open] into its alternatives.
// It returns them, the index just past the closing brace, and whether the
// group is closed at all.
func splitGroup(p string, open int) (parts []string, end int, ok bool) {
	depth := 0
	start := open + 1
	for i := open + 1; i < len(p); {
		if next := skipSpecial(p, i); next != i {
			i = next
			continue
		}
		switch p[i] {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				parts = append(parts, p[start:i])
				return parts, i + 1, true
			}
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, p[start:i])
				start = i + 1
			}
		}
		i++
	}
	return nil, 0, false
}

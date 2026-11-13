package permission

// A deny or ask rule for commands is matched against the text of a
// command. When part of that text is only known once the shell runs it (a
// variable, a substitution, a pattern, braces), the rule cannot be
// checked: "$CMD -rf x" may or may not be "rm -rf x". This file answers
// the one question that matters then: could the rule match what the
// command turns into? If it could, the engine asks, in every mode,
// because a guard that could not be checked has not passed.

// tok is one piece of a text with holes: a byte, or a hole that may turn
// out to hold any text at all, none included.
type tok struct {
	b    byte
	hole bool
}

// patToks turns a Bash rule's pattern into tokens: each * is a hole.
func patToks(p cmdPattern) []tok {
	var out []tok
	for i, seg := range p.segs {
		if i > 0 {
			out = append(out, tok{hole: true})
		}
		out = appendLit(out, seg)
	}
	return out
}

func appendLit(out []tok, s string) []tok {
	for i := 0; i < len(s); i++ {
		out = append(out, tok{b: s[i]})
	}
	return out
}

func appendHole(out []tok) []tok {
	if n := len(out); n > 0 && out[n-1].hole {
		return out
	}
	return append(out, tok{hole: true})
}

// skeletons returns the command as text with a hole wherever the shell
// would put something this package cannot know ahead of time. It returns
// nothing when the whole command is known, because then the rules were
// matched against it directly. A program named by a path is also given
// by its bare name.
func (e *Engine) skeletons(c *simpleCmd) [][]tok {
	if len(c.words) == 0 {
		return nil
	}
	holes := false
	var rest []tok
	for _, w := range c.words[1:] {
		rest = append(rest, tok{b: ' '})
		if w.brace || w.glob || w.eqHead || w.tilde && !w.static() {
			// The shell rewrites the whole word, perhaps into several.
			rest = appendHole(rest)
			holes = true
			continue
		}
		for _, p := range w.parts {
			if p.kind == partLit {
				rest = appendLit(rest, p.lit)
			} else {
				rest = appendHole(rest)
				holes = true
			}
		}
	}
	w0 := c.words[0]
	name, known := e.progName(w0)
	if !known || progIsPattern(w0) {
		var head []tok
		if progIsPattern(w0) || w0.eqHead || w0.tilde {
			head = appendHole(head)
		} else {
			for _, p := range w0.parts {
				if p.kind == partLit {
					head = appendLit(head, p.lit)
				} else {
					head = appendHole(head)
				}
			}
		}
		return [][]tok{append(head, rest...)}
	}
	if !holes {
		return nil
	}
	out := [][]tok{append(appendLit(nil, name), rest...)}
	if b := baseName(name); b != name {
		out = append(out, append(appendLit(nil, b), rest...))
	}
	return out
}

// couldMatch reports whether two texts with holes can turn into the same
// text. Its cost is the product of the two lengths.
func couldMatch(a, b []tok) bool {
	// next[j] is the answer for a[i+1:] and b[j:]; cur[j] for a[i:] and
	// b[j:]. The table is filled from the ends backward.
	next := make([]bool, len(b)+1)
	cur := make([]bool, len(b)+1)
	next[len(b)] = true
	for j := len(b) - 1; j >= 0; j-- {
		next[j] = b[j].hole && next[j+1]
	}
	for i := len(a) - 1; i >= 0; i-- {
		cur[len(b)] = a[i].hole && next[len(b)]
		for j := len(b) - 1; j >= 0; j-- {
			switch {
			case a[i].hole:
				cur[j] = next[j] || cur[j+1]
			case b[j].hole:
				cur[j] = cur[j+1] || next[j]
			default:
				cur[j] = a[i].b == b[j].b && next[j+1]
			}
		}
		next, cur = cur, next
	}
	return next[0]
}

// couldHit returns the deny or ask rule for commands that might apply to
// a command whose text is not fully known, and the word that hides it.
func (e *Engine) couldHit(v *bashView) *crule {
	var guards []*crule
	for _, list := range []List{ListDeny, ListAsk} {
		for _, c := range e.rules(list) {
			if c.kind == kindCommand {
				guards = append(guards, c)
			}
		}
	}
	if len(guards) == 0 {
		return nil
	}
	// Each rule is its pattern and, for a lone trailing " *", the bare
	// command as well.
	pats := make([][][]tok, len(guards))
	for i, c := range guards {
		pats[i] = [][]tok{patToks(c.cmd)}
		if c.cmd.bare != "" {
			pats[i] = append(pats[i], appendLit(nil, c.cmd.bare))
		}
	}
	// The comparison below costs the product of the two lengths. A line
	// built to make that large is not waited on: past the budget the
	// answer is "it could", which asks.
	budget := guardBudget
	for _, u := range v.units {
		for _, sk := range e.skeletons(u.cmd) {
			for i, c := range guards {
				for _, pt := range pats[i] {
					if headsClash(pt, sk) {
						continue
					}
					budget -= (len(pt) + 1) * (len(sk) + 1)
					if budget < 0 || couldMatch(pt, sk) {
						return c
					}
				}
			}
		}
	}
	return nil
}

// guardBudget bounds the work couldHit does for one command line.
const guardBudget = 20_000_000

// headsClash reports whether two texts with holes start with different
// literal text, in which case they can never turn into the same text.
func headsClash(a, b []tok) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i].hole || b[i].hole {
			return false
		}
		if a[i].b != b[i].b {
			return true
		}
	}
	return false
}

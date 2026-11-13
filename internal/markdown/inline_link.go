package markdown

import "strings"

// A reader looks ahead in the source for the end of a construct that began
// at some earlier point. It may look no further than lim, which is reach
// bytes past that point: a construct that has not ended by then is not one.
// When the source itself ends first and more may come, need is set: the
// answer is not known yet.
type reader struct {
	src  string
	lim  int
	eof  bool
	need bool
}

// noLimit is the limit of a reader that may look as far as the source goes.
const noLimit = 1 << 40

// at returns the byte at position j. It reports false when j is out of
// reach, or past the end of the source.
func (r *reader) at(j int) (byte, bool) {
	if j >= r.lim {
		return 0, false
	}
	if j >= len(r.src) {
		if !r.eof {
			r.need = true
		}
		return 0, false
	}
	return r.src[j], true
}

// skipSpace returns the position of the first byte at or after j that is
// not a space, a tab or a newline.
func (r *reader) skipSpace(j int) int {
	for j < len(r.src) && j < r.lim && (r.src[j] == ' ' || r.src[j] == '\t' || r.src[j] == '\n') {
		j++
	}
	return j
}

// openBracket puts "[" (n is 1) or "![" (n is 2) on the stack.
func (s *scanner) openBracket(i, n int) int {
	ch := byte('[')
	if n == 2 {
		ch = '!'
	}
	s.stack = append(s.stack, delim{node: len(s.nodes), pos: i, hold: i, ch: ch, n: n, orig: n, active: true})
	s.add(i, s.src[i:i+n], 0)
	return n
}

// closeBracket reads "]". If the nearest bracket on the stack can still
// become a link and what follows makes it one (a destination in
// parentheses, or a label that a definition earlier in the text has given
// an address), the text between becomes a link or an image. Otherwise the
// bracket is plain text.
func (s *scanner) closeBracket(i int) int {
	src := s.src
	bi := -1
	for k := len(s.stack) - 1; k >= 0; k-- {
		if c := s.stack[k].ch; c == '[' || c == '!' {
			bi = k
			break
		}
	}
	if bi < 0 {
		s.add(i, "]", 0)
		return 1
	}
	b := s.stack[bi]
	if !b.active {
		s.remove(bi, bi+1)
		s.add(i, "]", 0)
		return 1
	}
	if i+1 == len(src) && !s.eof {
		return 0
	}
	lim := b.pos + s.cfg.reach
	end, url, title, ok := 0, "", "", false
	if i+1 < len(src) && src[i+1] == '(' {
		e, u, t, good, need := parseInlineLink(src, i+1, lim, s.eof)
		if need {
			return 0
		}
		if good {
			end, url, title, ok = e, u, t, true
		}
	}
	if !ok {
		// A reference: "[text][label]", "[label][]" or "[label]" alone.
		label, e, full := src[b.pos+b.orig:i], i+1, false
		if i+1 < len(src) && src[i+1] == '[' {
			e2, l, good, need := parseLinkLabel(src, i+1, lim, s.eof)
			if need {
				return 0
			}
			switch {
			case good && l == "":
				e = e2
			case good:
				label, e, full = l, e2, true
			}
		}
		if full || validLabel(label) {
			if ref, found := s.refs[normalLabel(label)]; found {
				end, url, title, ok = e, ref.url, ref.title, true
			}
		}
	}
	if !ok {
		s.remove(bi, bi+1)
		s.add(i, "]", 0)
		s.advance()
		return 1
	}
	s.makeLink(bi, i, url, title)
	return end - i
}

// makeLink turns the pieces after the bracket at index bi of the stack into
// a link to url, or into an image. at is where the closing bracket stands.
func (s *scanner) makeLink(bi, at int, url, title string) {
	b := s.stack[bi]
	s.inside(bi + 1) // emphasis inside the brackets is matched inside them
	s.remove(bi, len(s.stack))
	if b.ch == '[' {
		// A link cannot hold a link: the brackets before this one are spent.
		for k := range s.stack {
			if d := &s.stack[k]; d.ch == '[' {
				d.active = false
			}
		}
	}
	var plain strings.Builder
	for k := b.node + 1; k < len(s.nodes); k++ {
		switch nd := &s.nodes[k]; {
		case nd.brk:
			plain.WriteByte(' ')
		case nd.st.bits&sAddr == 0:
			plain.WriteString(nd.text)
		}
	}
	shown := strings.TrimSpace(plain.String())
	if b.ch == '!' {
		// An image is shown as its description and its address.
		if shown == "" {
			shown = "image"
		}
		s.nodes = s.nodes[:b.node]
		s.nodes = append(s.nodes, node{pos: b.pos, text: "[" + shown + "]", st: style{bits: sImage, base: s.base, link: url}})
		if url != "" {
			s.nodes = append(s.nodes, node{pos: b.pos, text: " (" + url + ")", st: style{bits: sAddr, base: s.base}})
		}
	} else {
		s.nodes[b.node].text = ""
		for k := b.node + 1; k < len(s.nodes); k++ {
			s.nodes[k].st.bits |= sLink
			s.nodes[k].st.link = url
		}
		switch {
		case shown == "":
			// A link with no text: its address stands in for it.
			s.nodes = append(s.nodes, node{pos: at, text: url, st: style{bits: sLink, base: s.base, link: url}})
		case !s.cfg.hyper && url != "" && !sameAddress(shown, url):
			addr := " (" + url
			if title != "" {
				addr += ` "` + title + `"`
			}
			s.nodes = append(s.nodes, node{pos: at, text: addr + ")", st: style{bits: sAddr, base: s.base}})
		}
	}
	s.advance()
}

// sameAddress reports whether the text of a link already shows its address.
func sameAddress(shown, url string) bool {
	return shown == url || "mailto:"+shown == url || "http://"+shown == url || "https://"+shown == url
}

// parseInlineLink reads the part of an inline link that follows its text:
// "(", a destination, an optional title in quotes or parentheses, and ")".
// i is the position of the "(". It returns the position after the ")".
func parseInlineLink(src string, i, lim int, eof bool) (end int, url, title string, ok, need bool) {
	r := reader{src: src, lim: lim, eof: eof}
	j := r.skipSpace(i + 1)
	j, url, ok = r.dest(j)
	if !ok {
		return 0, "", "", false, r.need
	}
	k := r.skipSpace(j)
	c, more := r.at(k)
	if !more {
		return 0, "", "", false, r.need
	}
	if k > j && (c == '"' || c == '\'' || c == '(') {
		k, title, ok = r.title(k)
		if !ok {
			return 0, "", "", false, r.need
		}
		k = r.skipSpace(k)
		if c, more = r.at(k); !more {
			return 0, "", "", false, r.need
		}
	}
	if c != ')' {
		return 0, "", "", false, false
	}
	return k + 1, url, title, true, false
}

// dest reads a link destination at position j: either text in angle
// brackets, or text with no spaces in which parentheses are balanced. It
// returns the position after it.
func (r *reader) dest(j int) (end int, url string, ok bool) {
	c, more := r.at(j)
	if !more {
		return 0, "", false
	}
	if c == '<' {
		for k := j + 1; ; k++ {
			c, more := r.at(k)
			switch {
			case !more, c == '\n', c == '<':
				return 0, "", false
			case c == '>':
				return k + 1, unescape(r.src[j+1 : k]), true
			case c == '\\':
				k++
			}
		}
	}
	depth, k := 0, j
	for {
		c, more := r.at(k)
		if !more {
			if r.need || k >= r.lim {
				return 0, "", false
			}
			break // the end of the text ends the destination
		}
		if c == '\\' {
			k += 2
			continue
		}
		if c <= ' ' || c == 0x7f {
			break
		}
		if c == '(' {
			depth++
		} else if c == ')' {
			if depth == 0 {
				break
			}
			depth--
		}
		k++
	}
	k = min(k, len(r.src))
	if depth != 0 {
		return 0, "", false
	}
	return k, unescape(r.src[j:k]), true
}

// title reads a link title at position k, which holds its opening quote or
// parenthesis. It returns the position after its end.
func (r *reader) title(k int) (end int, title string, ok bool) {
	open := r.src[k]
	closer := open
	if open == '(' {
		closer = ')'
	}
	for m := k + 1; ; m++ {
		c, more := r.at(m)
		switch {
		case !more:
			return 0, "", false
		case c == '\\':
			m++
		case c == closer:
			return m + 1, unescape(r.src[k+1 : m]), true
		case c == '(' && open == '(':
			return 0, "", false
		}
	}
}

// parseLinkLabel reads a link label at position i, which holds its "[". It
// returns the position after its "]" and the text between, which is empty
// for "[]". A label holds no bracket that is not escaped, and something
// other than blanks.
func parseLinkLabel(src string, i, lim int, eof bool) (end int, label string, ok, need bool) {
	r := reader{src: src, lim: min(lim, i+1001), eof: eof}
	for k := i + 1; ; k++ {
		c, more := r.at(k)
		switch {
		case !more:
			return 0, "", false, r.need
		case c == '\\':
			k++
		case c == '[':
			return 0, "", false, false
		case c == ']':
			label = src[i+1 : k]
			if label != "" && strings.TrimSpace(label) == "" {
				return 0, "", false, false
			}
			return k + 1, label, true, false
		}
	}
}

// validLabel reports whether s can be a link label: it holds something
// other than blanks, no bracket that is not escaped, and at most 999 bytes.
func validLabel(s string) bool {
	if len(s) > 999 || strings.TrimSpace(s) == "" {
		return false
	}
	for k := 0; k < len(s); k++ {
		switch s[k] {
		case '\\':
			k++
		case '[', ']':
			return false
		}
	}
	return true
}

// normalLabel returns the form in which two labels are compared: without
// regard to case, and with every run of blanks taken as one space.
func normalLabel(s string) string {
	return strings.ToLower(strings.ToUpper(strings.Join(strings.Fields(s), " ")))
}

// unescape returns s with its backslash escapes and character references
// replaced by the characters they stand for, as in a link's address or
// title.
func unescape(s string) string {
	if strings.IndexByte(s, '\\') < 0 && strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s) && isPunctByte(s[i+1]):
			b.WriteByte(s[i+1])
			i += 2
		case c == '&':
			if n, text, _ := parseEntity(s, i); n > 0 {
				b.WriteString(text)
				i += n
				continue
			}
			fallthrough
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

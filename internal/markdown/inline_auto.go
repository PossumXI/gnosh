package markdown

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/text"
)

// special marks the bytes that start a token of their own. Plain text runs
// from one of them to the next.
var special = func() (t [256]bool) {
	for _, c := range "\\&`*_~[]!< \t\n" {
		t[c] = true
	}
	return t
}()

// isLocal reports whether c may stand in the part of an e-mail address
// before the "@".
func isLocal(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '.' || c == '_' || c == '+' || c == '-'
}

// isAlnum reports whether c is an ASCII letter or digit.
func isAlnum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// word reads plain text, after checking whether an address written without
// angle brackets starts here: "www.", "http://", "https://", "mailto:" or
// "xmpp:" after a blank or one of "*_~(", or an e-mail address.
func (s *scanner) word(i int) int {
	src := s.src
	c := src[i]
	if l := c | 0x20; (l == 'w' || l == 'h' || l == 'm' || l == 'x') && s.linkBoundary(i) {
		n, need := s.urlAutolink(i)
		if need {
			return 0
		}
		if n > 0 {
			return n
		}
	}
	if isLocal(c) && !(i > 0 && isLocal(src[i-1])) && !(i == 0 && s.prev < utf8.RuneSelf && isLocal(byte(s.prev))) {
		// The start of a word that may be the first part of an e-mail
		// address, if it is at most 64 bytes long and "@" follows.
		k := i
		for k < len(src) && isLocal(src[k]) {
			k++
		}
		switch {
		case k-i > 64:
		case k == len(src):
			if !s.eof {
				return 0
			}
		case src[k] == '@':
			n, need := s.emailAutolink(i, k)
			if need {
				return 0
			}
			if n > 0 {
				return n
			}
		}
	}
	k := i + 1
	for k < len(src) && !special[src[k]] && !(isLocal(src[k]) && !isLocal(src[k-1])) {
		k++
	}
	s.add(i, src[i:k], 0)
	return k - i
}

// linkBoundary reports whether an address without angle brackets may start
// at position i: at the start of a line, after a blank, or after one of
// "*", "_", "~" and "(".
func (s *scanner) linkBoundary(i int) bool {
	r := s.before(i)
	return isSpaceRune(r) || r == '*' || r == '_' || r == '~' || r == '('
}

// urlAutolink reads an address that starts at position i with "www.",
// "http://" or "https://" and runs to the next blank or "<". Punctuation
// at its end that a sentence would put there is left outside it. It
// returns the number of bytes of the link, or zero if there is none.
func (s *scanner) urlAutolink(i int) (n int, need bool) {
	src := s.src
	rest := src[i:]
	var prefix string
	for _, p := range [...]string{"www.", "http://", "https://", "mailto:", "xmpp:"} {
		k := min(len(p), len(rest))
		if !strings.EqualFold(rest[:k], p[:k]) {
			continue
		}
		if k < len(p) {
			if !s.eof {
				return 0, true // the text may still spell the prefix out
			}
			continue
		}
		prefix = p
	}
	switch prefix {
	case "":
		return 0, false
	case "mailto:", "xmpp:":
		return s.schemeEmail(i, len(prefix), prefix == "xmpp:")
	}
	e := i + len(prefix)
	for e < len(src) && src[e] != ' ' && src[e] != '\t' && src[e] != '\n' && src[e] != '<' {
		e++
	}
	if e-i > s.cfg.reach {
		return 0, false
	}
	if e == len(src) && !s.eof {
		return 0, true // where it ends decides what belongs to it
	}
	link := src[i:e]
	for trimmed := true; trimmed && link != ""; {
		trimmed = false
		switch c := link[len(link)-1]; {
		case strings.IndexByte("?!.,:*_~'\"", c) >= 0:
			link, trimmed = link[:len(link)-1], true
		case c == ')' && strings.Count(link, ")") > strings.Count(link, "("):
			link, trimmed = link[:len(link)-1], true
		case c == ';':
			// "&amp;" at the end is a character reference, not address.
			k := len(link) - 2
			for k >= 0 && isAlnum(link[k]) {
				k--
			}
			if k >= 0 && k < len(link)-2 && link[k] == '&' {
				link, trimmed = link[:k], true
			}
		}
	}
	host := link[min(len(prefix), len(link)):]
	if prefix == "www." {
		host = link
	}
	if k := strings.IndexAny(host, "/?#:"); k >= 0 {
		host = host[:k]
	}
	if !validHost(host, prefix == "www.") {
		return 0, false
	}
	url := link
	if prefix == "www." {
		url = "http://" + link
	}
	s.nodes = append(s.nodes, node{pos: i, text: link, st: style{bits: sLink, base: s.base, link: url}})
	return len(link), false
}

// schemeEmail reads an e-mail address behind "mailto:" or "xmpp:", which
// starts at position i and is n bytes long. The whole of it is the link.
// An xmpp address may go on with a slash and a resource.
func (s *scanner) schemeEmail(i, n int, xmpp bool) (int, bool) {
	src := s.src
	k := i + n
	for k < len(src) && isLocal(src[k]) {
		k++
	}
	if k-i > s.cfg.reach {
		return 0, false
	}
	if k == len(src) {
		return 0, !s.eof
	}
	if local := k - i - n; local == 0 || local > 64 || src[k] != '@' {
		return 0, false
	}
	e := k + 1
	for e < len(src) && (isAlnum(src[e]) || src[e] == '-' || src[e] == '_' || src[e] == '.') {
		e++
	}
	if e-i > s.cfg.reach {
		return 0, false
	}
	if e == len(src) && !s.eof {
		return 0, true
	}
	domain := strings.TrimRight(src[k+1:e], ".")
	if domain == "" || !strings.Contains(domain, ".") || strings.Contains(domain, "..") {
		return 0, false
	}
	if c := domain[len(domain)-1]; c == '-' || c == '_' {
		return 0, false
	}
	end := k + 1 + len(domain)
	if xmpp && end == e && end < len(src) && src[end] == '/' {
		r := end + 1
		for r < len(src) && (isAlnum(src[r]) || src[r] == '@' || src[r] == '.') {
			r++
		}
		if r-i > s.cfg.reach {
			return 0, false
		}
		if r == len(src) && !s.eof {
			return 0, true
		}
		if res := strings.TrimRight(src[end+1:r], "."); res != "" {
			end += 1 + len(res)
		}
	}
	link := src[i:end]
	s.nodes = append(s.nodes, node{pos: i, text: link, st: style{bits: sLink, base: s.base, link: link}})
	return len(link), false
}

// validHost reports whether host can be the host of an address: letters,
// digits, "-" and "_" in parts separated by dots. strict asks for what a
// "www." address must have: a dot, and no underscore in the last two parts.
func validHost(host string, strict bool) bool {
	if host == "" {
		return false
	}
	alnum := false
	for k := 0; k < len(host); k++ {
		switch c := host[k]; {
		case isAlnum(c):
			alnum = true
		case c == '-' || c == '_' || c == '.':
		default:
			return false
		}
	}
	if !alnum {
		return false
	}
	if !strict {
		return true
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts[len(parts)-2:] {
		if strings.IndexByte(p, '_') >= 0 {
			return false
		}
	}
	return true
}

// emailAutolink reads an e-mail address whose first part starts at
// position i and whose "@" stands at position at. The part after the "@"
// is letters, digits, "-" and "_" in parts separated by dots; it has at
// least one dot and does not end in "-" or "_". A dot at its very end is
// left outside the link.
func (s *scanner) emailAutolink(i, at int) (n int, need bool) {
	src := s.src
	e := at + 1
	for e < len(src) && (isAlnum(src[e]) || src[e] == '-' || src[e] == '_' || src[e] == '.') {
		e++
	}
	if e-i > s.cfg.reach {
		return 0, false
	}
	if e == len(src) && !s.eof {
		return 0, true
	}
	domain := strings.TrimRight(src[at+1:e], ".")
	if domain == "" || !strings.Contains(domain, ".") || strings.Contains(domain, "..") {
		return 0, false
	}
	if c := domain[len(domain)-1]; c == '-' || c == '_' {
		return 0, false
	}
	link := src[i:at+1] + domain
	s.nodes = append(s.nodes, node{pos: i, text: link, st: style{bits: sLink, base: s.base, link: "mailto:" + link}})
	return len(link), false
}

// backticks reads a run of backticks. If a run of the same length follows
// within reach, the text between them is a code span.
func (s *scanner) backticks(i int) int {
	src := s.src
	n := 1
	for i+n < len(src) && src[i+n] == '`' {
		n++
	}
	if s.longRun(i, n, s.prev == '`') {
		return n
	}
	if i+n == len(src) && !s.eof {
		return 0
	}
	lim := i + n + s.cfg.reach
	p := max(i+n, s.ticks[n])
	for p < len(src) && p < lim {
		k := strings.IndexByte(src[p:], '`')
		if k < 0 {
			p = len(src)
			break
		}
		p += k
		if p >= lim {
			break
		}
		q := p
		for q < len(src) && src[q] == '`' {
			q++
		}
		if q == len(src) && !s.eof {
			return 0 // this run may be the one, once it has ended
		}
		if q-p == n {
			s.codeSpan(i, n, src[i+n:p])
			return q - i
		}
		p = q
	}
	if p < lim && !s.eof {
		return 0 // the closing run may still come
	}
	if s.ticks == nil {
		s.ticks = make(map[int]int)
	}
	s.ticks[n] = p
	s.add(i, src[i:i+n], 0)
	return n
}

// codeSpan adds a code span that starts at position i behind n backticks.
// Line endings in it count as spaces, and one space is taken off each end
// when both ends have one and there is more to it than spaces. Without
// colour the backticks are kept, since nothing else would set it apart.
func (s *scanner) codeSpan(i, n int, body string) {
	body = strings.ReplaceAll(body, "\n", " ")
	if len(body) >= 2 && body[0] == ' ' && body[len(body)-1] == ' ' && strings.Trim(body, " ") != "" {
		body = body[1 : len(body)-1]
	}
	if s.cfg.profile == text.NoColor {
		ticks := s.src[i : i+n]
		body = ticks + body + ticks
	}
	s.add(i, body, sCode)
}

// angle reads "<": an address in angle brackets, raw HTML, or itself.
func (s *scanner) angle(i int) int {
	src := s.src
	lim := i + s.cfg.reach
	end, url, need := autolinkAt(src, i, lim, s.eof)
	if need {
		return 0
	}
	if end > 0 {
		s.nodes = append(s.nodes, node{pos: i, text: src[i+1 : end-1], st: style{bits: sLink, base: s.base, link: url}})
		return end - i
	}
	end, need = htmlAt(src, i, lim, s.eof)
	if need {
		return 0
	}
	if end == 0 {
		s.add(i, "<", 0)
		return 1
	}
	if raw := src[i:end]; isBreakTag(raw) {
		s.nodes = append(s.nodes, node{pos: i, brk: true})
	} else {
		s.add(i, raw, sHTML)
	}
	return end - i
}

// isBreakTag reports whether raw is "<br>", "<br/>" or "<br />" in any
// case: the one tag whose meaning a terminal can show.
func isBreakTag(raw string) bool {
	if len(raw) < 4 || len(raw) > 7 || !strings.EqualFold(raw[:3], "<br") {
		return false
	}
	rest := strings.TrimSpace(raw[3 : len(raw)-1])
	return rest == "" || rest == "/"
}

// autolinkAt reads an address in angle brackets at position i: an absolute
// URI ("scheme:..." with no blanks) or an e-mail address. It returns the
// position after the ">" and the address to link to, or zero.
func autolinkAt(src string, i, lim int, eof bool) (end int, url string, need bool) {
	j := i + 1
	for ; ; j++ {
		if j >= lim {
			return 0, "", false
		}
		if j >= len(src) {
			return 0, "", !eof
		}
		c := src[j]
		if c == '>' {
			break
		}
		if c <= ' ' || c == '<' || c == 0x7f {
			return 0, "", false
		}
	}
	body := src[i+1 : j]
	switch {
	case isURI(body):
		return j + 1, body, false
	case isEmail(body):
		return j + 1, "mailto:" + body, false
	}
	return 0, "", false
}

// isURI reports whether s starts with a scheme (a letter and one to 31
// more letters, digits, "+", "." or "-") and a colon.
func isURI(s string) bool {
	k := 0
	for k < len(s) && k < 33 && (isAlnum(s[k]) || k > 0 && (s[k] == '+' || s[k] == '.' || s[k] == '-')) {
		k++
	}
	return k >= 2 && k <= 32 && k < len(s) && s[k] == ':' && !(s[0] >= '0' && s[0] <= '9')
}

// isEmail reports whether s is an e-mail address as the CommonMark
// specification spells it out for autolinks.
func isEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at < 1 {
		return false
	}
	for k := 0; k < at; k++ {
		if c := s[k]; !isAlnum(c) && strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", c) < 0 {
			return false
		}
	}
	for _, part := range strings.Split(s[at+1:], ".") {
		if part == "" || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return false
		}
		for k := 0; k < len(part); k++ {
			if c := part[k]; !isAlnum(c) && c != '-' {
				return false
			}
		}
	}
	return true
}

// parseEntity reads a character reference at position i of s, which holds
// "&". It returns its length and the text it stands for, or zero. need is
// set when s ends inside what may be a reference.
func parseEntity(s string, i int) (n int, text string, need bool) {
	j := i + 1
	if j == len(s) {
		return 0, "", true
	}
	if s[j] != '#' {
		for j < len(s) && j-i <= 32 && isAlnum(s[j]) {
			j++
		}
		if j == len(s) {
			return 0, "", j-i <= 32
		}
		if s[j] != ';' || j == i+1 {
			return 0, "", false
		}
		if text, ok := entities[s[i+1:j]]; ok {
			return j + 1 - i, text, false
		}
		return 0, "", false
	}
	j++
	base, most := 10, 7
	if j < len(s) && (s[j] == 'x' || s[j] == 'X') {
		base, most = 16, 6
		j++
	}
	digits := j
	for j < len(s) && j-digits < most && (s[j] >= '0' && s[j] <= '9' || base == 16 && (s[j]|0x20) >= 'a' && (s[j]|0x20) <= 'f') {
		j++
	}
	if j == len(s) {
		return 0, "", true
	}
	if s[j] != ';' || j == digits {
		return 0, "", false
	}
	v, err := strconv.ParseUint(s[digits:j], base, 32)
	r := rune(v)
	if err != nil || v == 0 || v > utf8.MaxRune || !utf8.ValidRune(r) {
		r = utf8.RuneError
	}
	return j + 1 - i, string(r), false
}

package markdown

import "strings"

// isLetterByte reports whether c is an ASCII letter.
func isLetterByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// until looks for term at or after position from and returns the position
// after it, or zero.
func (r *reader) until(from int, term string) int {
	hi := min(len(r.src), r.lim)
	if from <= hi {
		if k := strings.Index(r.src[from:hi], term); k >= 0 {
			return from + k + len(term)
		}
	}
	if len(r.src) < r.lim && !r.eof {
		r.need = true
	}
	return 0
}

// tagName reads a tag name at position j: a letter, then letters, digits
// and hyphens. It returns the position after it, or zero.
func (r *reader) tagName(j int) int {
	c, ok := r.at(j)
	if !ok || !isLetterByte(c) {
		return 0
	}
	for k := j + 1; ; k++ {
		c, ok := r.at(k)
		if !ok {
			if r.need || k >= r.lim {
				return 0
			}
			return k
		}
		if !isAlnum(c) && c != '-' {
			return k
		}
	}
}

// htmlAt reads raw HTML at position i of src, which holds "<": an open
// tag, a closing tag, a comment, a processing instruction, a declaration
// or a CDATA section, as the CommonMark specification defines them. It
// returns the position after it, or zero when there is none. need is set
// when src ends before that can be told.
func htmlAt(src string, i, lim int, eof bool) (end int, need bool) {
	r := reader{src: src, lim: lim, eof: eof}
	end = r.html(i)
	if end == 0 {
		return 0, r.need
	}
	return end, false
}

// html is the work of htmlAt.
func (r *reader) html(i int) int {
	c, ok := r.at(i + 1)
	if !ok {
		return 0
	}
	switch {
	case c == '/':
		k := r.tagName(i + 2)
		if k == 0 {
			return 0
		}
		k = r.skipSpace(k)
		if c, ok := r.at(k); ok && c == '>' {
			return k + 1
		}
		return 0
	case c == '?':
		return r.until(i+2, "?>")
	case c == '!':
		c, ok := r.at(i + 2)
		switch {
		case !ok:
			return 0
		case c == '-':
			if c, ok := r.at(i + 3); !ok || c != '-' {
				return 0
			}
			// "<!-->" and "<!--->" are comments too.
			for _, short := range [...]string{">", "->"} {
				hit := true
				for k := 0; k < len(short); k++ {
					if c, ok := r.at(i + 4 + k); !ok || c != short[k] {
						hit = false
						break
					}
				}
				if hit {
					return i + 4 + len(short)
				}
				if r.need {
					return 0
				}
			}
			return r.until(i+4, "-->")
		case c == '[':
			for k, want := range []byte("CDATA[") {
				if c, ok := r.at(i + 3 + k); !ok || c != want {
					return 0
				}
			}
			return r.until(i+9, "]]>")
		case isLetterByte(c):
			return r.until(i+3, ">")
		}
		return 0
	case isLetterByte(c):
		return r.openTag(i + 1)
	}
	return 0
}

// openTag reads the rest of an open tag whose name starts at position j:
// the name, attributes, and ">" or "/>".
func (r *reader) openTag(j int) int {
	k := r.tagName(j)
	if k == 0 {
		return 0
	}
	for {
		ws := r.skipSpace(k)
		c, ok := r.at(ws)
		switch {
		case !ok:
			return 0
		case c == '>':
			return ws + 1
		case c == '/':
			if c, ok := r.at(ws + 1); ok && c == '>' {
				return ws + 2
			}
			return 0
		case ws == k, !isLetterByte(c) && c != '_' && c != ':':
			return 0 // an attribute needs a blank before it and a proper name
		}
		for k = ws + 1; ; k++ {
			c, ok := r.at(k)
			if !ok {
				return 0
			}
			if !isAlnum(c) && c != '_' && c != '.' && c != ':' && c != '-' {
				break
			}
		}
		v := r.skipSpace(k)
		if c, ok = r.at(v); !ok {
			return 0
		}
		if c != '=' {
			continue
		}
		v = r.skipSpace(v + 1)
		quote, ok := r.at(v)
		if !ok {
			return 0
		}
		if quote == '"' || quote == '\'' {
			for k = v + 1; ; k++ {
				c, ok := r.at(k)
				if !ok {
					return 0
				}
				if c == quote {
					break
				}
			}
			k++
			continue
		}
		for k = v; ; k++ {
			c, ok := r.at(k)
			if !ok {
				return 0
			}
			if strings.IndexByte(" \t\n\"'=<>`", c) >= 0 {
				break
			}
		}
		if k == v {
			return 0
		}
	}
}

// The tags that start an HTML block of the sixth kind.
const htmlBlockTags = " address article aside base basefont blockquote body caption center col colgroup" +
	" dd details dialog dir div dl dt fieldset figcaption figure footer form frame frameset" +
	" h1 h2 h3 h4 h5 h6 head header hr html iframe legend li link main menu menuitem nav noframes" +
	" ol optgroup option p param search section summary table tbody td tfoot th thead title tr track ul "

// The tags whose content is taken as it is, to their closing tag: they
// start an HTML block of the first kind.
const htmlRawTags = " pre script style textarea "

// htmlStart says which of the seven kinds of HTML block the text s starts,
// or zero. s begins with "<". The kinds are those of the CommonMark
// specification: 1 is <pre>, <script>, <style> and <textarea>, 2 a
// comment, 3 a processing instruction, 4 a declaration, 5 CDATA, 6 one of
// the block-level tags, and 7 any other tag that has the line to itself.
// more says that the line may still grow, in which case unsure is set when
// what has come does not settle the answer. seven says that the seventh
// kind is possible here: it depends on the whole line, and it cannot
// interrupt a paragraph.
func htmlStart(s string, more, seven bool) (kind int, unsure bool) {
	if len(s) < 2 {
		return 0, more
	}
	switch s[1] {
	case '?':
		return 3, false
	case '!':
		if len(s) < 3 {
			return 0, more
		}
		if isLetterByte(s[2]) {
			return 4, false
		}
		for _, p := range [...]struct {
			kind   int
			prefix string
		}{{2, "<!--"}, {5, "<![CDATA["}} {
			if strings.HasPrefix(s, p.prefix) {
				return p.kind, false
			}
			if more && strings.HasPrefix(p.prefix, s) {
				unsure = true
			}
		}
		return 0, unsure
	}
	j := 1
	if s[1] == '/' {
		j = 2
	}
	k := j
	for k < len(s) && (isAlnum(s[k]) || s[k] == '-') {
		k++
	}
	if k == len(s) && more && (k-j <= 10 || seven) {
		return 0, true // the name may go on
	}
	if k == j || !isLetterByte(s[j]) {
		return 0, false
	}
	name := " " + strings.ToLower(s[j:k]) + " "
	after := byte('\n')
	if k < len(s) {
		after = s[k]
	}
	ends := after == ' ' || after == '\t' || after == '\n' || after == '>'
	raw := strings.Contains(htmlRawTags, name)
	if raw && j == 1 && ends {
		return 1, false
	}
	if strings.Contains(htmlBlockTags, name) {
		if ends {
			return 6, false
		}
		if after == '/' {
			if k+1 == len(s) {
				return 0, more
			}
			if s[k+1] == '>' {
				return 6, false
			}
		}
	}
	if raw || !seven {
		return 0, false
	}
	end, cut := htmlAt(s, 0, noLimit, !more)
	if cut {
		return 0, true
	}
	if end == 0 || strings.TrimLeft(s[end:], " \t") != "" {
		return 0, false
	}
	if more {
		return 0, true // the tag has the line to itself so far
	}
	return 7, false
}

// htmlEnd reports whether a line holding text ends an HTML block of the
// given kind, one of 1 to 5. The other kinds end at a blank line.
func htmlEnd(kind int, line string) bool {
	switch kind {
	case 1:
		lower := strings.ToLower(line)
		for _, tag := range strings.Fields(htmlRawTags) {
			if strings.Contains(lower, "</"+tag+">") {
				return true
			}
		}
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	}
	return false
}

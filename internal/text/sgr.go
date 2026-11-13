package text

import (
	"strconv"
	"strings"
)

// Reset is the SGR sequence that turns every colour and style off.
const Reset = "\x1b[0m"

// The sequences that open and close a hyperlink (OSC 8). The string
// terminator ESC \ is used rather than BEL because it is the standard one.
const (
	linkOpen  = "\x1b]8;"
	linkEnd   = "\x1b\\"
	linkClose = "\x1b]8;;\x1b\\"
)

// The on/off attributes an sgrState tracks, as bits.
const (
	attrBold = 1 << iota
	attrFaint
	attrItalic
	attrBlink
	attrRapidBlink
	attrReverse
	attrConceal
	attrStrike
	attrOverline
)

// The underline styles an sgrState tracks. underDouble21 is the double
// underline asked for with SGR 21, kept apart from the one asked for with
// "4:2" so that each is written back the way it came.
const (
	underNone = iota
	underSingle
	underDouble
	underCurly
	underDotted
	underDashed
	underDouble21
)

// An sgrState is the set of colours and styles that SGR sequences have left
// in effect. The zero value is the terminal's default look. Two states are
// equal exactly when they look the same.
type sgrState struct {
	fg, bg Color  // foreground and background
	ul     Color  // underline colour
	attrs  uint16 // attr bits
	under  uint8  // underline style
}

// isSGR reports whether a complete control sequence is an SGR sequence: its
// final byte is "m" and it has only digits, semicolons and colons before it.
// A private marker, as in ESC [ > 4 m, makes it something else.
func isSGR(seq string) bool {
	if len(seq) < 3 || seq[len(seq)-1] != 'm' {
		return false
	}
	for i := 2; i < len(seq)-1; i++ {
		if c := seq[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// An sgrOp is one instruction of an SGR sequence.
type sgrOp struct {
	// code is the attribute number: 0 resets, 1 is bold, 38 sets the
	// foreground colour, and so on. It is -1 for something unreadable.
	code int
	// style is, for code 4, the underline style written after a colon, or
	// -1 when there is none.
	style int
	// color is the colour that codes 38, 48 and 58 carry, and ok says
	// whether one could be read.
	color Color
	ok    bool
}

// sgrNumber reads a parameter: digits, or nothing at all, which stands for
// zero.
func sgrNumber(s string) (n int, ok bool) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		if n < 1<<20 {
			n = n*10 + int(c-'0')
		}
	}
	return n, true
}

// nextSGROp reads the first instruction from the parameters of an SGR
// sequence (the bytes between "ESC [" and "m"). It returns the instruction,
// the parameters after it, and whether there are any: a trailing semicolon
// is followed by an empty parameter, which counts.
//
// Parameters are separated by semicolons. A colour is written either with
// colons inside one parameter, as ISO 8613-6 has it ("38:5:n", "38:2::r:g:b",
// and "38:2:r:g:b" without the colour-space field), or in the older way that
// spends the following parameters ("38;5;n", "38;2;r;g;b").
func nextSGROp(params string) (op sgrOp, rest string, more bool) {
	field, rest, more := strings.Cut(params, ";")
	head, sub, hasSub := strings.Cut(field, ":")
	op.style = -1
	code, ok := sgrNumber(head)
	if !ok {
		op.code = -1
		return op, rest, more
	}
	op.code = code
	switch code {
	case 4:
		if hasSub {
			first, _, _ := strings.Cut(sub, ":")
			if n, ok := sgrNumber(first); ok {
				op.style = n
			}
		}
	case 38, 48, 58:
		switch {
		case hasSub:
			op.color, op.ok = colonColor(sub)
		case more:
			op.color, op.ok, rest, more = semicolonColor(rest)
		}
	}
	return op, rest, more
}

// colonColor reads a colour from the sub-parameters after "38:", "48:" or
// "58:".
func colonColor(sub string) (Color, bool) {
	mode, rest, _ := strings.Cut(sub, ":")
	switch mode {
	case "5":
		index, _, _ := strings.Cut(rest, ":")
		n, ok := sgrNumber(index)
		if !ok || index == "" || n > 255 {
			return Default, false
		}
		return Indexed(uint8(n)), true
	case "2":
		// Either r:g:b, or a colour-space field first (often empty) and
		// then r:g:b, perhaps with more after them.
		var fields [4]string
		count := 0
		for more := true; more && count < len(fields); count++ {
			fields[count], rest, more = strings.Cut(rest, ":")
		}
		from := 0
		switch count {
		case 3:
		case 4:
			from = 1
		default:
			return Default, false
		}
		var v [3]uint8
		for i := range v {
			n, ok := sgrNumber(fields[from+i])
			if !ok || n > 255 {
				return Default, false
			}
			v[i] = uint8(n)
		}
		return RGB(v[0], v[1], v[2]), true
	}
	return Default, false
}

// semicolonColor reads a colour from the parameters that follow a bare 38, 48
// or 58, and returns what is left after the ones it used.
func semicolonColor(params string) (c Color, ok bool, rest string, more bool) {
	mode, rest, more := strings.Cut(params, ";")
	switch mode {
	case "5":
		if !more {
			return Default, false, rest, more
		}
		var index string
		index, rest, more = strings.Cut(rest, ";")
		n, ok := sgrNumber(index)
		if !ok || n > 255 {
			return Default, false, rest, more
		}
		return Indexed(uint8(n)), true, rest, more
	case "2":
		var v [3]uint8
		good := true
		for i := range v {
			if !more {
				return Default, false, rest, more
			}
			var field string
			field, rest, more = strings.Cut(rest, ";")
			n, ok := sgrNumber(field)
			if !ok || n > 255 {
				good = false
			}
			v[i] = uint8(n)
		}
		if !good {
			return Default, false, rest, more
		}
		return RGB(v[0], v[1], v[2]), true, rest, more
	}
	return Default, false, rest, more
}

// apply changes the state the way an SGR sequence with the given parameters
// does. Attributes the state does not track (fonts, framing, and the like)
// are passed over.
func (st *sgrState) apply(params string) {
	for more := true; more; {
		var op sgrOp
		op, params, more = nextSGROp(params)
		st.do(op)
	}
}

// do carries out one instruction.
func (st *sgrState) do(op sgrOp) {
	switch c := op.code; {
	case c == 0:
		*st = sgrState{}
	case c == 1:
		st.attrs |= attrBold
	case c == 2:
		st.attrs |= attrFaint
	case c == 3:
		st.attrs |= attrItalic
	case c == 4:
		switch op.style {
		case 0:
			st.under = underNone
		case 2:
			st.under = underDouble
		case 3:
			st.under = underCurly
		case 4:
			st.under = underDotted
		case 5:
			st.under = underDashed
		default:
			st.under = underSingle
		}
	case c == 5:
		st.attrs |= attrBlink
	case c == 6:
		st.attrs |= attrRapidBlink
	case c == 7:
		st.attrs |= attrReverse
	case c == 8:
		st.attrs |= attrConceal
	case c == 9:
		st.attrs |= attrStrike
	case c == 21:
		st.under = underDouble21
	case c == 22:
		st.attrs &^= attrBold | attrFaint
	case c == 23:
		st.attrs &^= attrItalic
	case c == 24:
		st.under = underNone
	case c == 25:
		st.attrs &^= attrBlink | attrRapidBlink
	case c == 27:
		st.attrs &^= attrReverse
	case c == 28:
		st.attrs &^= attrConceal
	case c == 29:
		st.attrs &^= attrStrike
	case c >= 30 && c <= 37:
		st.fg = ansiColor(c - 30)
	case c == 38:
		if op.ok {
			st.fg = op.color
		}
	case c == 39:
		st.fg = Default
	case c >= 40 && c <= 47:
		st.bg = ansiColor(c - 40)
	case c == 48:
		if op.ok {
			st.bg = op.color
		}
	case c == 49:
		st.bg = Default
	case c == 53:
		st.attrs |= attrOverline
	case c == 55:
		st.attrs &^= attrOverline
	case c == 58:
		if op.ok {
			st.ul = op.color
		}
	case c == 59:
		st.ul = Default
	case c >= 90 && c <= 97:
		st.fg = ansiColor(c - 90 + 8)
	case c >= 100 && c <= 107:
		st.bg = ansiColor(c - 100 + 8)
	}
}

// appendSequence appends the SGR sequence that takes a terminal from its
// default look to this state. It appends nothing for the default state.
func (st sgrState) appendSequence(dst []byte) []byte {
	if st == (sgrState{}) {
		return dst
	}
	dst = append(dst, "\x1b["...)
	start := len(dst)
	if st.attrs&attrBold != 0 {
		dst = appendParam(dst, start, "1")
	}
	if st.attrs&attrFaint != 0 {
		dst = appendParam(dst, start, "2")
	}
	if st.attrs&attrItalic != 0 {
		dst = appendParam(dst, start, "3")
	}
	switch st.under {
	case underSingle:
		dst = appendParam(dst, start, "4")
	case underDouble:
		dst = appendParam(dst, start, "4:2")
	case underCurly:
		dst = appendParam(dst, start, "4:3")
	case underDotted:
		dst = appendParam(dst, start, "4:4")
	case underDashed:
		dst = appendParam(dst, start, "4:5")
	case underDouble21:
		dst = appendParam(dst, start, "21")
	}
	if st.attrs&attrBlink != 0 {
		dst = appendParam(dst, start, "5")
	}
	if st.attrs&attrRapidBlink != 0 {
		dst = appendParam(dst, start, "6")
	}
	if st.attrs&attrReverse != 0 {
		dst = appendParam(dst, start, "7")
	}
	if st.attrs&attrConceal != 0 {
		dst = appendParam(dst, start, "8")
	}
	if st.attrs&attrStrike != 0 {
		dst = appendParam(dst, start, "9")
	}
	if st.attrs&attrOverline != 0 {
		dst = appendParam(dst, start, "53")
	}
	if st.fg != Default {
		dst = appendColor(appendParam(dst, start, ""), st.fg, 30)
	}
	if st.bg != Default {
		dst = appendColor(appendParam(dst, start, ""), st.bg, 40)
	}
	if st.ul != Default {
		dst = appendUnderlineColor(appendParam(dst, start, ""), st.ul)
	}
	return append(dst, 'm')
}

// appendParam appends one SGR parameter, after a semicolon unless it is the
// first: start is where the sequence's parameters begin in dst.
func appendParam(dst []byte, start int, param string) []byte {
	if len(dst) > start {
		dst = append(dst, ';')
	}
	return append(dst, param...)
}

// appendColor appends the SGR parameters that set a colour. base is 30 for
// the foreground and 40 for the background.
func appendColor(dst []byte, c Color, base int) []byte {
	switch c.kind() {
	case colorANSI:
		n := int(c & 0xF)
		if n >= 8 {
			n += 60 - 8 // the bright colours are 90 to 97 and 100 to 107
		}
		return strconv.AppendInt(dst, int64(base+n), 10)
	case colorIndexed:
		dst = strconv.AppendInt(dst, int64(base+8), 10)
		dst = append(dst, ";5;"...)
		return strconv.AppendInt(dst, int64(c&0xFF), 10)
	case colorRGB:
		dst = strconv.AppendInt(dst, int64(base+8), 10)
		dst = append(dst, ";2;"...)
		dst = strconv.AppendInt(dst, int64(c>>16&0xFF), 10)
		dst = append(dst, ';')
		dst = strconv.AppendInt(dst, int64(c>>8&0xFF), 10)
		dst = append(dst, ';')
		return strconv.AppendInt(dst, int64(c&0xFF), 10)
	}
	return dst
}

// appendUnderlineColor appends the SGR parameter that sets the underline
// colour. It is written with colons, so that a terminal that does not know
// it skips the whole parameter instead of reading the numbers as styles.
func appendUnderlineColor(dst []byte, c Color) []byte {
	switch c.kind() {
	case colorANSI, colorIndexed:
		dst = append(dst, "58:5:"...)
		return strconv.AppendInt(dst, int64(c&0xFF), 10)
	case colorRGB:
		dst = append(dst, "58:2::"...)
		dst = strconv.AppendInt(dst, int64(c>>16&0xFF), 10)
		dst = append(dst, ':')
		dst = strconv.AppendInt(dst, int64(c>>8&0xFF), 10)
		dst = append(dst, ':')
		return strconv.AppendInt(dst, int64(c&0xFF), 10)
	}
	return dst
}

// hyperlinkOf reads a complete OSC sequence as a hyperlink sequence (OSC 8).
// It returns the link the sequence opens, as the text between "8;" and the
// terminator (the link's parameters, a semicolon, and its address), or ""
// for a sequence that closes the link, which is one with an empty address.
func hyperlinkOf(seq string) (link string, ok bool) {
	payload := seq[2:]
	if strings.HasSuffix(payload, "\x07") {
		payload = payload[:len(payload)-1]
	} else {
		payload = strings.TrimSuffix(payload, linkEnd)
	}
	target, ok := strings.CutPrefix(payload, "8;")
	if !ok {
		return "", false
	}
	_, address, ok := strings.Cut(target, ";")
	if !ok {
		return "", false
	}
	if address == "" {
		return "", true
	}
	return target, true
}

// A look is what the escape sequences seen so far have left in effect: the
// colours and styles, and the open hyperlink if there is one. Truncate and
// Wrap carry one along so that they can close what is open where they cut,
// and open it again after.
type look struct {
	sgr  sgrState
	link string // as hyperlinkOf returns it; "" when no link is open
}

// isStyle reports whether a piece is a sequence that changes a look: a
// complete SGR or hyperlink sequence.
func isStyle(p piece) bool {
	switch p.kind {
	case CSI:
		return p.complete && isSGR(p.text)
	case OSC:
		if p.complete {
			_, ok := hyperlinkOf(p.text)
			return ok
		}
	}
	return false
}

// applySequence changes the look the way one escape sequence does. Sequences
// other than SGR and hyperlinks change nothing.
func (l *look) applySequence(kind Kind, complete bool, seq string) {
	if !complete {
		return
	}
	switch kind {
	case CSI:
		if isSGR(seq) {
			l.sgr.apply(seq[2 : len(seq)-1])
		}
	case OSC:
		if link, ok := hyperlinkOf(seq); ok {
			l.link = link
		}
	}
}

// applyText changes the look the way the escape sequences in s do.
func (l *look) applyText(s string) {
	for s != "" {
		i := strings.IndexByte(s, escapeByte)
		if i < 0 {
			return
		}
		n, kind, complete := scanSequence(s[i:])
		l.applySequence(kind, complete, s[i:i+n])
		s = s[i+n:]
	}
}

// applyPiece changes the look the way a piece does: a sequence, or a cluster
// with sequences inside it.
func (l *look) applyPiece(p piece) {
	switch {
	case p.kind != Plain:
		l.applySequence(p.kind, p.complete, p.text)
	case p.inner:
		l.applyText(p.text)
	}
}

// plain reports whether nothing is in effect.
func (l look) plain() bool { return l.sgr == (sgrState{}) && l.link == "" }

// appendOpen appends the sequences that take a terminal from its default
// state to this look.
func (l look) appendOpen(dst []byte) []byte {
	dst = l.sgr.appendSequence(dst)
	if l.link != "" {
		dst = append(dst, linkOpen...)
		dst = append(dst, l.link...)
		dst = append(dst, linkEnd...)
	}
	return dst
}

// appendClose appends the sequences that take a terminal from this look back
// to its default state.
func (l look) appendClose(dst []byte) []byte {
	if l.link != "" {
		dst = append(dst, linkClose...)
	}
	if l.sgr != (sgrState{}) {
		dst = append(dst, Reset...)
	}
	return dst
}

// appendChange appends the sequences that take a terminal from this look to
// another.
func (l look) appendChange(dst []byte, to look) []byte {
	if l.link != to.link {
		if l.link != "" {
			dst = append(dst, linkClose...)
		}
		if to.link != "" {
			dst = append(dst, linkOpen...)
			dst = append(dst, to.link...)
			dst = append(dst, linkEnd...)
		}
	}
	if l.sgr != to.sgr {
		if l.sgr != (sgrState{}) {
			dst = append(dst, Reset...)
		}
		dst = to.sgr.appendSequence(dst)
	}
	return dst
}

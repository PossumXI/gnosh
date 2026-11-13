package text

import (
	"iter"
	"strings"
	"unicode/utf8"
)

// Kind says what a Token is.
type Kind uint8

const (
	// Plain is text that holds no ESC byte. It may hold other control
	// characters, such as newlines and tabs.
	Plain Kind = iota
	// CSI is a control sequence: ESC [, parameter and intermediate bytes,
	// and a final byte. Colours and styles (SGR, final byte "m"), cursor
	// movement and erasing are all of this kind.
	CSI
	// OSC is an operating system command: ESC ], a payload, and BEL or the
	// string terminator ESC \. Hyperlinks (OSC 8) and window titles are of
	// this kind.
	OSC
	// DCS is a device control string: ESC P, a payload, and ESC \.
	DCS
	// SOS is a start-of-string sequence: ESC X, a payload, and ESC \.
	SOS
	// PM is a privacy message: ESC ^, a payload, and ESC \.
	PM
	// APC is an application program command: ESC _, a payload, and ESC \.
	APC
	// ESC is any other escape sequence: ESC, any intermediate bytes, and a
	// final byte, such as ESC 7 (save the cursor) or ESC ( B (choose a
	// character set). A lone ESC that starts nothing is of this kind too.
	ESC
)

// String names the kind: "plain", "CSI", "OSC", "DCS", "SOS", "PM", "APC" or
// "ESC".
func (k Kind) String() string {
	switch k {
	case Plain:
		return "plain"
	case CSI:
		return "CSI"
	case OSC:
		return "OSC"
	case DCS:
		return "DCS"
	case SOS:
		return "SOS"
	case PM:
		return "PM"
	case APC:
		return "APC"
	case ESC:
		return "ESC"
	}
	return "unknown"
}

// A Token is one piece of a string that may hold escape sequences: a run of
// plain text, or a single escape sequence.
type Token struct {
	// Kind says which.
	Kind Kind
	// Text is the token's bytes, exactly as they stood in the string.
	Text string
	// Complete is false for an escape sequence that was cut short: the
	// string ended, or a byte that cannot be part of the sequence came,
	// before its final byte or terminator. It is always true for plain
	// text.
	Complete bool
}

// IsSGR reports whether the token is a Select Graphic Rendition sequence, the
// kind that sets colours and styles: ESC [, digits with semicolons and
// colons, and "m".
func (t Token) IsSGR() bool {
	return t.Kind == CSI && t.Complete && isSGR(t.Text)
}

// IsHyperlink reports whether the token is an OSC 8 sequence, the kind that
// opens or closes a hyperlink.
func (t Token) IsHyperlink() bool {
	if t.Kind != OSC || !t.Complete {
		return false
	}
	_, ok := hyperlinkOf(t.Text)
	return ok
}

// NextToken splits the first token off s: either the plain text up to the
// first ESC, or the escape sequence that s starts with. It returns the token
// and what is left of s. For the empty string it returns the zero Token.
//
// The escape sequences are those of ECMA-48 in their 7-bit form, as xterm
// documents them:
//
//   - ESC [ starts a control sequence, which runs through any bytes in the
//     range 0x20 to 0x3F up to a final byte in the range 0x40 to 0x7E.
//   - ESC ] starts an operating system command, which runs to a BEL or to
//     ESC \.
//   - ESC P, ESC X, ESC ^ and ESC _ start strings that run to ESC \.
//   - ESC followed by bytes in the range 0x20 to 0x2F and one in the range
//     0x30 to 0x7E is a sequence of its own, as is ESC followed directly by
//     a byte in the range 0x30 to 0x7E.
//
// A sequence that is cut short still makes a token, with Complete false. A
// control sequence or short escape sequence ends, unfinished, in front of
// the first byte that cannot belong to it; that byte starts the next token.
// A string ends, unfinished, in front of an ESC that does not begin its
// terminator and in front of CAN and SUB, which cancel it; failing those it
// swallows the rest of s, newlines included, as a terminal would. Every ESC
// byte therefore belongs to a sequence token and never to plain text.
//
// The 8-bit forms of these introducers (the C1 controls U+0080 to U+009F)
// are not recognized: in UTF-8 text they are ordinary zero-width control
// characters, and few terminals act on them.
//
// NextToken does not allocate and takes time proportional to the length of
// the token.
func NextToken(s string) (tok Token, rest string) {
	if s == "" {
		return Token{}, ""
	}
	if s[0] != escapeByte {
		n := strings.IndexByte(s, escapeByte)
		if n < 0 {
			n = len(s)
		}
		return Token{Kind: Plain, Text: s[:n], Complete: true}, s[n:]
	}
	n, kind, complete := scanSequence(s)
	return Token{Kind: kind, Text: s[:n], Complete: complete}, s[n:]
}

// Tokens returns an iterator over the tokens of s: runs of plain text and
// escape sequences, in order. Joined, the tokens' Text gives s back. See
// NextToken for what counts as a sequence.
func Tokens(s string) iter.Seq[Token] {
	return func(yield func(Token) bool) {
		for s != "" {
			var tok Token
			tok, s = NextToken(s)
			if !yield(tok) {
				return
			}
		}
	}
}

// scanSequence measures the escape sequence at the start of s, which must
// begin with ESC. It returns the sequence's length in bytes, its kind, and
// whether it reached its final byte or terminator.
func scanSequence(s string) (n int, kind Kind, complete bool) {
	if len(s) < 2 {
		return 1, ESC, false
	}
	switch c := s[1]; {
	case c == '[':
		i := 2
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3F {
			i++
		}
		if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7E {
			return i + 1, CSI, true
		}
		return i, CSI, false
	case c == ']':
		return scanString(s, OSC)
	case c == 'P':
		return scanString(s, DCS)
	case c == 'X':
		return scanString(s, SOS)
	case c == '^':
		return scanString(s, PM)
	case c == '_':
		return scanString(s, APC)
	case c >= 0x20 && c <= 0x2F:
		i := 2
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2F {
			i++
		}
		if i < len(s) && s[i] >= 0x30 && s[i] <= 0x7E {
			return i + 1, ESC, true
		}
		return i, ESC, false
	case c >= 0x30 && c <= 0x7E:
		return 2, ESC, true
	}
	// ESC followed by a control character, DEL or a byte of a multi-byte
	// character starts nothing.
	return 1, ESC, false
}

// scanString measures a string sequence (OSC, DCS, SOS, PM or APC) at the
// start of s, whose first two bytes are its introducer.
func scanString(s string, kind Kind) (n int, k Kind, complete bool) {
	for i := 2; i < len(s); i++ {
		switch s[i] {
		case 0x07: // BEL ends an operating system command, as xterm allows.
			if kind == OSC {
				return i + 1, kind, true
			}
		case escapeByte:
			if i+1 < len(s) && s[i+1] == '\\' {
				return i + 2, kind, true
			}
			return i, kind, false
		case 0x18, 0x1A: // CAN and SUB cancel the string.
			return i, kind, false
		}
	}
	return len(s), kind, false
}

// Strip returns s without its escape sequences: what would be left on the
// screen as text. Every sequence NextToken recognizes is removed, cut-short
// ones included, so the result holds no ESC byte and stripping it again
// changes nothing. Control characters that are not part of a sequence, such
// as newlines and tabs, stay.
//
// Strip takes time proportional to len(s) and returns s itself, without
// allocating, when there is nothing to remove.
func Strip(s string) string {
	if strings.IndexByte(s, escapeByte) < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for s != "" {
		var tok Token
		tok, s = NextToken(s)
		if tok.Kind == Plain {
			b.WriteString(tok.Text)
		}
	}
	return b.String()
}

// finished returns s without the escape sequences in it that were cut short
// (the tokens whose Complete is false). Such a sequence would take in
// whatever is printed after it, so the functions whose results are meant to
// be printed before other output drop them. finished returns s itself when
// there is nothing to drop.
func finished(s string) string {
	if !hasUnfinished(s) {
		return s
	}
	return string(appendFinished(make([]byte, 0, len(s)), s))
}

// hasUnfinished reports whether s holds an escape sequence that was cut
// short.
func hasUnfinished(s string) bool {
	for {
		i := strings.IndexByte(s, escapeByte)
		if i < 0 {
			return false
		}
		n, _, complete := scanSequence(s[i:])
		if !complete {
			return true
		}
		s = s[i+n:]
	}
}

// appendFinished appends s to dst without the escape sequences in it that
// were cut short.
func appendFinished(dst []byte, s string) []byte {
	for s != "" {
		var tok Token
		tok, s = NextToken(s)
		if tok.Complete {
			dst = append(dst, tok.Text...)
		}
	}
	return dst
}

// VisibleWidth returns how many terminal cells s takes when printed, not
// counting its escape sequences. It is exactly Width(Strip(s)), without the
// copy: a grapheme cluster that an escape sequence interrupts (a letter, a
// colour change, then the letter's accent) is still measured as one cluster.
//
// VisibleWidth does not allocate and takes time proportional to len(s).
func VisibleWidth(s string) int { return Metrics{}.VisibleWidth(s) }

// VisibleWidth is like the package's VisibleWidth, with ambiguous-width
// characters counted by m's rule.
func (m Metrics) VisibleWidth(s string) int {
	return m.measure(s, strings.IndexByte(s, escapeByte) >= 0)
}

// A piece is the unit that truncating, padding and wrapping work in: one
// grapheme cluster with its width, or one escape sequence.
type piece struct {
	// text is the piece's bytes. A cluster that escape sequences interrupt
	// includes them.
	text string
	// width is the cluster's width in cells. It is zero for a sequence.
	width int
	// kind is Plain for a cluster, and otherwise the sequence's kind.
	kind Kind
	// complete is the sequence's Token.Complete.
	complete bool
	// inner is true for a cluster with escape sequences inside it.
	inner bool
}

// nextPiece splits the first piece off s, which must not be empty.
//
// Escape sequences do not end a grapheme cluster: the clusters are those of
// the text with the sequences taken out. So when a cluster runs up to a
// sequence, nextPiece looks past the sequences that follow, and if the text
// after them continues the cluster, the piece takes in the sequences and the
// continuation.
func (m Metrics) nextPiece(s string) (p piece, rest string) {
	b := s[0]
	if b == escapeByte {
		n, kind, complete := scanSequence(s)
		return piece{text: s[:n], kind: kind, complete: complete}, s[n:]
	}
	if b < utf8.RuneSelf && (len(s) == 1 || s[1] < utf8.RuneSelf && s[1] != escapeByte) {
		if b == '\r' && len(s) > 1 && s[1] == '\n' {
			return piece{text: s[:2]}, s[2:]
		}
		return piece{text: s[:1], width: asciiCells(b)}, s[1:]
	}
	var st clusterState
	n := m.scan(&st, s, true)
	for n < len(s) && s[n] == escapeByte {
		next := n
		for next < len(s) && s[next] == escapeByte {
			k, _, _ := scanSequence(s[next:])
			next += k
		}
		more := m.scan(&st, s[next:], true)
		if more == 0 {
			break
		}
		n = next + more
	}
	// Sequences are inside the piece when the cluster went on after them,
	// or when they stood between the bytes of one of its characters.
	inner := strings.IndexByte(s[:n], escapeByte) >= 0
	return piece{text: s[:n], width: st.cells(), inner: inner}, s[n:]
}

// isNewline reports whether a cluster is a line break: a line feed, or a
// carriage return and line feed (which UAX #29 keeps together as one
// cluster, even with escape sequences between the two).
func isNewline(cluster string) bool {
	return cluster[0] == '\n' || cluster[0] == '\r' && cluster[len(cluster)-1] == '\n'
}

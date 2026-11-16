package text

import (
	"strings"
	"unicode/utf8"
)

// Safe returns s as it may be written to a terminal when it comes from what
// gnosh does not trust, a model's reply or the input a model wrote for a
// tool: its styles (SGR) stay, and every other escape sequence and every
// control character but the tab and the line break is taken out, the C1
// controls (U+0080 to U+009F) among them, which some terminals read as the
// escape sequences they stand for. Such text can then change the colours
// of what it writes and nothing else: not the cursor, the screen, the
// window's title, the clipboard (OSC 52) or the terminal's modes.
func Safe(s string) string {
	if safeAlready(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for s != "" {
		var tok Token
		tok, s = NextToken(s)
		switch {
		case tok.Kind == Plain:
			for _, r := range tok.Text {
				if r == '\n' || r == '\t' || r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f) {
					b.WriteRune(r)
				}
			}
		case tok.IsSGR():
			b.WriteString(tok.Text)
		}
	}
	return b.String()
}

// safeAlready reports whether s holds nothing Safe takes out, so that most
// text is passed back as it is.
func safeAlready(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\t':
		case c < 0x20 || c == 0x7f:
			return false
		case c == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] <= 0x9f:
			return false // a C1 control, written in UTF-8
		case c >= utf8.RuneSelf && !utf8.FullRuneInString(s[i:]):
			return false
		}
	}
	return utf8.ValidString(s)
}

// SafeStream is Safe for text that arrives in pieces, such as a reply as
// the model writes it: an escape sequence cut by the end of a piece is held
// back until the rest of it comes, so that a style is kept whole and no
// half of another sequence slips through as text. The zero value is ready
// to use.
type SafeStream struct {
	held string
}

// Write takes the next piece and returns what of it may be written now.
func (st *SafeStream) Write(piece string) string {
	s := st.held + piece
	st.held = ""
	if i := strings.LastIndexByte(s, escapeByte); i >= 0 {
		if tok, rest := NextToken(s[i:]); !tok.Complete && rest == "" && len(tok.Text) < 4096 {
			st.held, s = s[i:], s[:i]
		}
	}
	return Safe(s)
}

// Reset drops what was held back: the text ended inside a sequence, which
// is not written.
func (st *SafeStream) Reset() { st.held = "" }

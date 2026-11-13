package text

import "strings"

// A Style is a look for text: a foreground and a background colour, and any
// of bold, dim, italic, underline, strikethrough and reverse. The zero value
// is no styling at all. A Style is a small value that compares with ==; every
// method returns a changed copy, so one style can be the base of many:
//
//	accent := text.Style{}.Fg(text.RGB(0xff, 0x5f, 0xd7)).Bold()
//	fmt.Println(accent.Render("hi"))
//
// A Style writes its colours for a Profile, set with In. One that was never
// given a profile writes them exactly as they were given, in 24-bit colour
// if that is what they are.
type Style struct {
	fg, bg           Color  // the colours as given
	shownFg, shownBg Color  // the colours as the profile lets them be written
	attrs            uint16 // attr bits, as in sgrState
	underline        bool
	level            uint8 // the profile plus one, or zero when none was given
}

// profile is the profile the style writes for.
func (s Style) profile() Profile {
	if s.level == 0 {
		return TrueColor
	}
	return Profile(s.level - 1)
}

// In returns the style set to write for the profile p. Under NoColor it
// writes nothing but the text. Under a profile with fewer colours than the
// style uses, each colour is written as the nearest one the profile has
// (see Profile.Convert); the style still remembers the colours it was
// given, so a later In with a richer profile brings them back.
func (s Style) In(p Profile) Style {
	p = min(p, TrueColor)
	s.level = uint8(p) + 1
	s.shownFg = p.Convert(s.fg)
	s.shownBg = p.Convert(s.bg)
	return s
}

// Fg returns the style with the foreground colour c. Default takes the
// colour off again.
func (s Style) Fg(c Color) Style {
	s.fg = c.valid()
	s.shownFg = s.profile().Convert(s.fg)
	return s
}

// Bg returns the style with the background colour c. Default takes the
// colour off again.
func (s Style) Bg(c Color) Style {
	s.bg = c.valid()
	s.shownBg = s.profile().Convert(s.bg)
	return s
}

// Bold returns the style with bold on.
func (s Style) Bold() Style { s.attrs |= attrBold; return s }

// Dim returns the style with dim (faint) on.
func (s Style) Dim() Style { s.attrs |= attrFaint; return s }

// Italic returns the style with italic on.
func (s Style) Italic() Style { s.attrs |= attrItalic; return s }

// Underline returns the style with underline on.
func (s Style) Underline() Style { s.underline = true; return s }

// Strikethrough returns the style with strikethrough on.
func (s Style) Strikethrough() Style { s.attrs |= attrStrike; return s }

// Reverse returns the style with reverse video on: foreground and background
// swapped.
func (s Style) Reverse() Style { s.attrs |= attrReverse; return s }

// state is the style as an SGR state, with fg as its foreground.
func (s Style) state(fg Color) sgrState {
	st := sgrState{fg: fg, bg: s.shownBg, attrs: s.attrs}
	if s.underline {
		st.under = underSingle
	}
	return st
}

// appendOpen appends the SGR sequence that turns the style on.
func (s Style) appendOpen(dst []byte) []byte {
	if s.profile() == NoColor {
		return dst
	}
	return s.state(s.shownFg).appendSequence(dst)
}

// Sequence returns the SGR sequence that turns the style on, such as
// "\x1b[1;38;2;255;95;215m". It is empty for a style that sets nothing and
// under NoColor. Reset turns the style off again. Most callers want Render;
// Sequence is for writing styled text piece by piece.
func (s Style) Sequence() string {
	var buf [64]byte
	return string(s.appendOpen(buf[:0]))
}

// Render returns text with the style applied: the style's SGR sequence, the
// text, and a reset. A style that sets nothing, any style under NoColor, and
// empty text give text back unchanged.
//
// Two things make the result safe to put anywhere. Each line of text is
// styled on its own, with the reset before the newline, so a line never
// leaks its style into the next (or into whatever a caller prints between
// them). And when text already holds a reset, as it does when it contains
// the result of another Render, the style is turned on again after it: a
// styled string can hold other styled strings, and the inner ones add to the
// outer one for as long as they last.
//
// Render takes time proportional to len(text).
func (s Style) Render(text string) string {
	var buf [64]byte
	open := s.appendOpen(buf[:0])
	if len(open) == 0 || text == "" {
		return text
	}
	var b strings.Builder
	if strings.IndexByte(text, '\n') < 0 && strings.IndexByte(text, escapeByte) < 0 {
		b.Grow(len(open) + len(text) + len(Reset))
		b.Write(open)
		b.WriteString(text)
		b.WriteString(Reset)
		return b.String()
	}

	b.Grow(len(text) + 4*(len(open)+len(Reset)))
	on := false // whether the style is in effect where the writing has got to
	for rest := text; rest != ""; {
		var tok Token
		tok, rest = NextToken(rest)
		switch {
		case tok.Kind == Plain:
			for part := tok.Text; part != ""; {
				line, after, newline := strings.Cut(part, "\n")
				if line != "" {
					if !on {
						b.Write(open)
						on = true
					}
					b.WriteString(line)
				}
				if newline {
					if on {
						b.WriteString(Reset)
						on = false
					}
					b.WriteByte('\n')
				}
				part = after
			}
		case !tok.IsSGR():
			b.WriteString(tok.Text)
		default:
			params := tok.Text[2 : len(tok.Text)-1]
			after, resets := afterLastReset(params)
			if !resets {
				// The sequence adds to the style, so the style goes first.
				if !on {
					b.Write(open)
					on = true
				}
				b.WriteString(tok.Text)
				break
			}
			// The sequence turns everything off, this style included. What
			// it sets after its reset must come after the style is back.
			b.WriteString(Reset)
			on = false
			if after < len(params) {
				b.Write(open)
				on = true
				b.WriteString("\x1b[")
				b.WriteString(params[after:])
				b.WriteByte('m')
			}
		}
	}
	if on {
		b.WriteString(Reset)
	}
	return b.String()
}

// afterLastReset looks for a reset (parameter 0, or an empty parameter) among
// the parameters of an SGR sequence. It reports whether there is one, and
// the offset in params of what follows the last one.
func afterLastReset(params string) (after int, found bool) {
	rest := params
	for more := true; more; {
		var op sgrOp
		op, rest, more = nextSGROp(rest)
		if op.code == 0 {
			after, found = len(params)-len(rest), true
		}
	}
	return after, found
}

// Hyperlink returns text as a link to url, using the OSC 8 sequences that
// terminals turn into clickable links: one that opens the link, the text,
// and one that closes it. An empty url or empty text gives text back
// unchanged.
//
// Bytes of url outside the printable ASCII range, and spaces, are written as
// percent escapes, as the OSC 8 convention requires; this also keeps a url
// from ending the sequence early. Like Style.Render, Hyperlink links each
// line of text on its own, and opens the link again after a link that text
// itself contains has closed.
//
// Hyperlink always writes the sequences. For output that may have to stay
// plain, use Profile.Hyperlink.
func Hyperlink(url, text string) string {
	if url == "" || text == "" {
		return text
	}
	var b strings.Builder
	b.Grow(len(text) + len(url) + len(linkOpen) + len(linkEnd) + len(linkClose) + 1)
	open := func() {
		b.WriteString(linkOpen)
		b.WriteByte(';')
		const hex = "0123456789ABCDEF"
		for i := 0; i < len(url); i++ {
			if c := url[i]; c > spaceByte && c < deleteByte {
				b.WriteByte(c)
			} else {
				b.WriteByte('%')
				b.WriteByte(hex[c>>4])
				b.WriteByte(hex[c&0xF])
			}
		}
		b.WriteString(linkEnd)
	}
	on := false    // whether this link is open where the writing has got to
	inner := false // whether a link from inside text is open there
	for rest := text; rest != ""; {
		var tok Token
		tok, rest = NextToken(rest)
		switch {
		case tok.Kind == Plain:
			for part := tok.Text; part != ""; {
				line, after, newline := strings.Cut(part, "\n")
				if line != "" {
					if !on && !inner {
						open()
						on = true
					}
					b.WriteString(line)
				}
				if newline {
					if on {
						b.WriteString(linkClose)
						on = false
					}
					b.WriteByte('\n')
				}
				part = after
			}
		case tok.IsHyperlink():
			// A terminal has one link open at a time: a link inside text
			// takes over from this one, and closing it closes both.
			link, _ := hyperlinkOf(tok.Text)
			inner, on = link != "", false
			b.WriteString(tok.Text)
		default:
			b.WriteString(tok.Text)
		}
	}
	if on {
		b.WriteString(linkClose)
	}
	return b.String()
}

// Hyperlink is like the package's Hyperlink, except that under NoColor it
// returns text alone.
func (p Profile) Hyperlink(url, text string) string {
	if p == NoColor {
		return text
	}
	return Hyperlink(url, text)
}

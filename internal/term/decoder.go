package term

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

const (
	escByte       = 0x1b
	pasteStartSeq = "\x1b[200~"
	pasteEndSeq   = "\x1b[201~"
)

// Decoder turns the bytes a terminal sends into events. It is a plain state
// machine: it does no I/O and reads no clock, so the same bytes always give
// the same events however they are cut up into reads.
//
// One thing cannot be decided from bytes alone. The Esc key sends the single
// byte that also begins every control sequence, and Alt+[ sends the two
// bytes that begin most of them. The decoder holds such a beginning back
// until more bytes arrive; the reader calls Flush when none came within a
// short time, and the decoder then reports the key. Flush settles only what
// one key press could have sent. A longer sequence that stopped half way
// (bytes delayed on a slow connection) waits for its end, so that its tail
// is never typed into the text; the worst a truly broken one can do is
// swallow input up to the next control key.
//
// The zero value is ready to use. A Decoder is not safe for use by several
// goroutines at once.
type Decoder struct {
	// buf holds bytes that are not events yet. While a paste is open it
	// holds the body of the paste so far.
	buf []byte
	// inPaste is true between the start and end marks of a bracketed
	// paste. scan counts the bytes of the body already searched for the
	// end mark.
	inPaste bool
	scan    int
	// pasteQuiet is true when Flush was called while a paste was open:
	// the terminal went quiet in the middle of a paste.
	pasteQuiet bool
	// expectCPR counts cursor position questions that have no answer yet.
	expectCPR int
}

// pasteStarted is what the parser returns for the start mark of a paste.
// It never leaves the decoder.
type pasteStarted struct{}

func (pasteStarted) isEvent()       {}
func (pasteStarted) String() string { return "paste start" }

var pasteEndBytes = []byte(pasteEndSeq)

// Feed decodes p, together with anything held back from earlier calls, and
// returns the events that are complete.
func (d *Decoder) Feed(p []byte) []Event {
	var out []Event
	if d.inPaste && d.pasteQuiet {
		d.pasteQuiet = false
		if startsWithInterrupt(p) {
			// The paste stalled and the person pressed Ctrl+C: give
			// the keyboard back.
			out = append(out, Paste{Text: pasteText(d.buf), Truncated: true})
			d.buf = d.buf[:0]
			d.inPaste = false
			d.scan = 0
		}
	}
	d.buf = append(d.buf, p...)
	return d.run(out, false)
}

// Flush tells the decoder that no more bytes arrived for a short while. It
// returns the keys that were being held back in case they were the start of
// a longer sequence: a lone Esc, or Alt with a key such as "[" or "O".
func (d *Decoder) Flush() []Event {
	if d.inPaste {
		d.pasteQuiet = true
		return nil
	}
	return d.run(nil, true)
}

// Pending reports whether Flush would do anything now. A reader uses it to
// decide whether to wait for more bytes with a time limit.
func (d *Decoder) Pending() bool {
	if d.inPaste {
		return !d.pasteQuiet
	}
	if len(d.buf) == 0 {
		return false
	}
	n, _ := d.parse(nil, d.buf, true)
	return n > 0
}

// InPaste reports whether a bracketed paste has begun and not ended.
func (d *Decoder) InPaste() bool { return d.inPaste }

// ExpectCursorPosition tells the decoder that a cursor position question
// (QueryCursorPosition) was just sent. The answer for row 1 looks exactly
// like F3 with modifiers; the decoder reads such bytes as the answer while
// one is owed and as the key otherwise.
func (d *Decoder) ExpectCursorPosition() { d.expectCPR++ }

// Reset drops everything held back, including an unfinished paste.
func (d *Decoder) Reset() {
	d.buf = nil
	d.inPaste = false
	d.scan = 0
	d.pasteQuiet = false
	d.expectCPR = 0
}

// startsWithInterrupt reports whether p begins with Ctrl+C in any of its
// encodings.
func startsWithInterrupt(p []byte) bool {
	if len(p) > 32 {
		p = p[:32]
	}
	var probe Decoder
	evs := probe.Feed(p)
	if len(evs) == 0 {
		return false
	}
	k, ok := evs[0].(Key)
	return ok && k.Is('c', ModCtrl)
}

// pasteText makes the body of a paste into text: UTF-8, with "\n" line ends.
func pasteText(b []byte) string {
	s := strings.ToValidUTF8(string(b), "\uFFFD")
	if strings.IndexByte(s, '\r') >= 0 {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
	}
	return s
}

// run decodes as much of the buffer as it can.
func (d *Decoder) run(out []Event, flush bool) []Event {
	b := d.buf
	off := 0
	var scratch [2]Event
	for {
		if d.inPaste {
			body := b[off:]
			from := d.scan - (len(pasteEndSeq) - 1)
			if from < 0 {
				from = 0
			}
			i := bytes.Index(body[from:], pasteEndBytes)
			if i < 0 {
				d.scan = len(body)
				break
			}
			i += from
			out = append(out, Paste{Text: pasteText(body[:i])})
			off += i + len(pasteEndSeq)
			d.inPaste = false
			d.scan = 0
			continue
		}
		if off == len(b) {
			break
		}
		n, evs := d.parse(scratch[:0], b[off:], flush)
		if n == 0 {
			break
		}
		off += n
		for _, ev := range evs {
			switch e := ev.(type) {
			case pasteStarted:
				d.inPaste = true
				d.scan = 0
				d.pasteQuiet = false
			case Reply:
				if e.Kind == ReplyCursorPosition && d.expectCPR > 0 {
					d.expectCPR--
				}
				out = append(out, ev)
			default:
				out = append(out, ev)
			}
		}
	}
	if off > 0 {
		d.buf = append(d.buf[:0], b[off:]...)
	}
	if len(d.buf) == 0 && cap(d.buf) > 64<<10 {
		d.buf = nil
	}
	return out
}

// parse reads one piece of input from the front of b, which is not empty.
// It returns how many bytes the piece took and the events it made, appended
// to out. Zero bytes means b is the beginning of something unfinished. It
// changes nothing in the decoder.
func (d *Decoder) parse(out []Event, b []byte, flush bool) (int, []Event) {
	c := b[0]
	switch {
	case c == escByte:
		return d.parseEscape(out, b, flush)
	case c < 0x20 || c == 0x7f:
		return 1, append(out, controlKey(c, 0))
	case c < 0x80:
		return 1, append(out, Key{Code: KeyRune, Rune: rune(c), Text: string(rune(c))})
	}
	if !utf8.FullRune(b) {
		return 0, out
	}
	r, size := utf8.DecodeRune(b)
	if (r == utf8.RuneError && size == 1) || isC1(r) {
		return size, append(out, Unknown{Raw: string(b[:size])})
	}
	return size, append(out, Key{Code: KeyRune, Rune: r, Text: string(r)})
}

// isC1 reports whether r is one of the control characters U+0080 to U+009F.
func isC1(r rune) bool { return r >= 0x80 && r < 0xa0 }

// controlKey is the key that sends the control character c in the classic
// encoding, with mod added.
func controlKey(c byte, mod Mod) Key {
	switch c {
	case 0x09:
		return Key{Code: KeyTab, Mod: mod}
	case 0x0d:
		return Key{Code: KeyEnter, Mod: mod}
	case 0x7f, 0x08:
		return Key{Code: KeyBackspace, Mod: mod}
	case escByte:
		return Key{Code: KeyEscape, Mod: mod}
	case 0x00:
		return Key{Code: KeyRune, Rune: ' ', Mod: mod | ModCtrl}
	}
	if c <= 0x1a {
		return Key{Code: KeyRune, Rune: rune('a' + c - 1), Mod: mod | ModCtrl}
	}
	// 0x1c to 0x1f are Ctrl with \ ] ^ _.
	return Key{Code: KeyRune, Rune: rune(c + 0x40), Mod: mod | ModCtrl}
}

// altKey is the key that sends ESC followed by the printable ASCII
// character c: Alt held with that character.
func altKey(c byte) Key {
	if c >= 'A' && c <= 'Z' {
		return Key{Code: KeyRune, Rune: rune(c - 'A' + 'a'), Mod: ModAlt | ModShift}
	}
	return Key{Code: KeyRune, Rune: rune(c), Mod: ModAlt}
}

// parseEscape reads input that begins with ESC.
func (d *Decoder) parseEscape(out []Event, b []byte, flush bool) (int, []Event) {
	if len(b) == 1 {
		if flush {
			return 1, append(out, Key{Code: KeyEscape})
		}
		return 0, out
	}
	c := b[1]
	switch c {
	case '[':
		return d.parseCSI(out, b, flush)
	case 'O':
		return d.parseSS3(out, b, flush)
	case ']', 'P', '_', '^', 'X':
		return d.parseString(out, b, flush)
	case escByte:
		// ESC ESC: Alt+Esc on its own, or Alt with the function key
		// whose sequence follows (ESC ESC [ A is Alt+Up).
		if len(b) == 2 {
			if flush {
				return 2, append(out, Key{Code: KeyEscape, Mod: ModAlt})
			}
			return 0, out
		}
		if b[2] == '[' || b[2] == 'O' {
			n, evs := d.parse(nil, b[1:], flush)
			if n == 0 {
				return 0, out
			}
			if k, ok := evs[0].(Key); ok && len(evs) == 1 && n > 2 {
				k.Mod |= ModAlt
				return 1 + n, append(out, k)
			}
		}
		// The first ESC was the Esc key by itself.
		return 1, append(out, Key{Code: KeyEscape})
	}
	switch {
	case c < 0x20 || c == 0x7f:
		return 2, append(out, controlKey(c, ModAlt))
	case c < 0x80:
		return 2, append(out, altKey(c))
	}
	if !utf8.FullRune(b[1:]) {
		return 0, out
	}
	r, size := utf8.DecodeRune(b[1:])
	if (r == utf8.RuneError && size == 1) || isC1(r) {
		return 1, append(out, Key{Code: KeyEscape})
	}
	return 1 + size, append(out, Key{Code: KeyRune, Rune: r, Mod: ModAlt})
}

// parseSS3 reads input that begins with ESC O: the application-mode forms
// of the arrows, Home, End, F1 to F4 and the number pad.
func (d *Decoder) parseSS3(out []Event, b []byte, flush bool) (int, []Event) {
	i := 2
	for i < len(b) && (b[i] >= '0' && b[i] <= '9' || b[i] == ';') {
		i++
	}
	if i == len(b) {
		if i == 2 && flush {
			return 2, append(out, altKey('O'))
		}
		return 0, out
	}
	f := b[i]
	if f < 0x40 || f > 0x7e {
		// Not the end of an SS3 sequence: it was Alt+O, or it is broken.
		if i == 2 {
			return 2, append(out, altKey('O'))
		}
		return i, append(out, Unknown{Raw: string(b[:i])})
	}
	raw := b[:i+1]
	params, ok := parseParams(b[2:i])
	if !ok || len(params) > 2 {
		return i + 1, append(out, Unknown{Raw: string(raw)})
	}
	mod, action := modifierParam(params, len(params)-1)
	var k Key
	switch {
	case f >= 'a' && f <= 'd':
		// rxvt: Ctrl with an arrow.
		k = Key{Code: [...]KeyCode{KeyUp, KeyDown, KeyRight, KeyLeft}[f-'a'], Mod: ModCtrl}
	case f == 'M':
		k = Key{Code: KeyEnter}
	case f == 'I':
		k = Key{Code: KeyTab}
	case f == 'X':
		k = Key{Code: KeyRune, Rune: '=', Text: "="}
	case f >= 'j' && f <= 'y':
		// The number pad in application mode: * + , - . / 0-9.
		r := rune("*+,-./0123456789"[f-'j'])
		k = Key{Code: KeyRune, Rune: r, Text: string(r)}
	default:
		code, found := letterKey(f)
		if !found || f == 'Z' {
			return i + 1, append(out, Unknown{Raw: string(raw)})
		}
		k = Key{Code: code}
	}
	k.Mod |= mod
	if k.Mod&^(ModCapsLock|ModNumLock) != 0 {
		k.Text = ""
	}
	return i + 1, append(out, finishKey(k, action))
}

// letterKey is the key a CSI or SS3 sequence ending in the letter f names.
func letterKey(f byte) (KeyCode, bool) {
	switch f {
	case 'A':
		return KeyUp, true
	case 'B':
		return KeyDown, true
	case 'C':
		return KeyRight, true
	case 'D':
		return KeyLeft, true
	case 'E':
		return KeyBegin, true
	case 'F':
		return KeyEnd, true
	case 'H':
		return KeyHome, true
	case 'P':
		return KeyF1, true
	case 'Q':
		return KeyF2, true
	case 'R':
		return KeyF3, true
	case 'S':
		return KeyF4, true
	case 'Z':
		return KeyTab, true
	}
	return 0, false
}

// tildeKey is the key a "CSI number ~" sequence names.
func tildeKey(n int) (KeyCode, bool) {
	switch n {
	case 1, 7:
		return KeyHome, true
	case 2:
		return KeyInsert, true
	case 3:
		return KeyDelete, true
	case 4, 8:
		return KeyEnd, true
	case 5:
		return KeyPageUp, true
	case 6:
		return KeyPageDown, true
	case 11, 12, 13, 14, 15:
		return KeyF1 + KeyCode(n-11), true
	case 17, 18, 19, 20, 21:
		return KeyF6 + KeyCode(n-17), true
	case 23, 24, 25, 26:
		return KeyF11 + KeyCode(n-23), true
	case 28, 29:
		return KeyF15 + KeyCode(n-28), true
	case 31, 32, 33, 34:
		return KeyF17 + KeyCode(n-31), true
	case 57427:
		return KeyBegin, true
	}
	return 0, false
}

// parseString reads an OSC, DCS, APC, PM or SOS string: ESC and one of
// ] P _ ^ X, a body, and BEL or ESC \ to end it. Terminals send these only
// as answers.
func (d *Decoder) parseString(out []Event, b []byte, flush bool) (int, []Event) {
	// cut ends the string early at i, where something that cannot be
	// part of it begins.
	cut := func(i int) (int, []Event) {
		if i == 2 {
			return 2, append(out, altKey(b[1]))
		}
		return i, append(out, Unknown{Raw: string(b[:i])})
	}
	for i := 2; i < len(b); i++ {
		c := b[i]
		switch {
		case c == 0x07:
			return i + 1, append(out, Reply{Kind: ReplyString, Intro: b[1], Text: string(b[2:i])})
		case c == escByte:
			if i+1 == len(b) {
				if flush {
					return cut(i)
				}
				return 0, out
			}
			if b[i+1] == '\\' {
				return i + 2, append(out, Reply{Kind: ReplyString, Intro: b[1], Text: string(b[2:i])})
			}
			return cut(i)
		case c < 0x20 || c == 0x7f:
			// A control key: the person is typing, so this was never
			// a string, or its end was lost.
			return cut(i)
		}
	}
	if len(b) == 2 && flush {
		return 2, append(out, altKey(b[1]))
	}
	return 0, out
}

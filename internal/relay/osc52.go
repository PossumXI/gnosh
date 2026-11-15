package relay

import (
	"bytes"
	"encoding/base64"
)

// maxCopy bounds the body of one clipboard write the scanner keeps. A
// longer one still reaches the terminal, as everything does, but is not
// copied.
const maxCopy = 8 << 20

// The states of a scanner.
const (
	stGround = iota // outside any sequence
	stEsc           // after ESC
	stOSC           // in the body of an operating system command, ESC ] ...
	stOSCEsc        // in such a body, after ESC: ESC \ ends it
)

// scanner finds the clipboard writes in what the far side sends: OSC 52,
// ESC ] 52 ; selections ; base64 text, ended by BEL or by ESC \. It only
// reads; what it reads is passed on whole by its caller. Queries (the
// text "?") and clears (no text) are not acted on, and neither are the
// 8-bit forms of the sequence, whose bytes are also parts of UTF-8
// characters.
type scanner struct {
	st   int
	body []byte
	// skip says the body is not a clipboard write, or is too long to
	// keep: the rest of it is not kept.
	skip bool
}

// feed reads p, the next piece of what the far side sent, and calls found
// with the text of each clipboard write that ends in it.
func (s *scanner) feed(p []byte, found func(text string)) {
	for _, b := range p {
		switch s.st {
		case stGround:
			if b == 0x1b {
				s.st = stEsc
			}
		case stEsc:
			s.afterEsc(b)
		case stOSC:
			switch b {
			case 0x07:
				s.end(found)
			case 0x1b:
				s.st = stOSCEsc
			case 0x18, 0x1a:
				// CAN and SUB cancel a sequence.
				s.st = stGround
			default:
				if s.skip {
					continue
				}
				s.body = append(s.body, b)
				if len(s.body) > maxCopy || len(s.body) == 3 && string(s.body) != "52;" {
					s.skip = true
				}
			}
		case stOSCEsc:
			if b == '\\' {
				s.end(found)
				continue
			}
			// Any other byte cancels the body, and the ESC begins
			// whatever comes next.
			s.afterEsc(b)
		}
	}
}

// afterEsc takes the byte after an ESC.
func (s *scanner) afterEsc(b byte) {
	switch b {
	case ']':
		s.st, s.body, s.skip = stOSC, s.body[:0], false
	case 0x1b:
		s.st = stEsc
	default:
		s.st = stGround
	}
}

// end takes the end of a body, and calls found when it was a clipboard
// write with text in it.
func (s *scanner) end(found func(text string)) {
	s.st = stGround
	if s.skip {
		return
	}
	rest, ok := bytes.CutPrefix(s.body, []byte("52;"))
	if !ok {
		return
	}
	_, data, ok := bytes.Cut(rest, []byte(";"))
	if !ok || len(data) == 0 || string(data) == "?" {
		return
	}
	text, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		text, err = base64.RawStdEncoding.DecodeString(string(data))
	}
	if err == nil && len(text) > 0 {
		found(string(text))
	}
}

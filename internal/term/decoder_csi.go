package term

import (
	"unicode"
	"unicode/utf8"
)

// parseParams splits the parameter bytes of a control sequence: fields are
// separated by ";" and the parts of a field by ":". An empty part is -1.
// It reports false when a byte is none of a digit, ";" or ":".
func parseParams(b []byte) ([][]int, bool) {
	if len(b) == 0 {
		return nil, true
	}
	fields := [][]int{nil}
	n, have := 0, false
	push := func() {
		last := len(fields) - 1
		if have {
			fields[last] = append(fields[last], n)
		} else {
			fields[last] = append(fields[last], -1)
		}
		n, have = 0, false
	}
	for _, c := range b {
		switch {
		case c >= '0' && c <= '9':
			have = true
			if n < 1<<24 {
				n = n*10 + int(c-'0')
			}
		case c == ':':
			push()
		case c == ';':
			push()
			fields = append(fields, nil)
		default:
			return nil, false
		}
	}
	push()
	return fields, true
}

// modifierParam reads field idx of params as a modifier parameter: one more
// than the sum of the modifier bits, optionally followed by ":" and the
// kind of event (1 press, 2 repeat, 3 release). A missing field means no
// modifiers and a press.
func modifierParam(params [][]int, idx int) (Mod, int) {
	if idx < 0 || idx >= len(params) {
		return 0, 1
	}
	var mod Mod
	if p := params[idx][0]; p > 1 {
		mod = Mod((p - 1) & 0xff)
	}
	action := 1
	if len(params[idx]) > 1 && params[idx][1] > 0 {
		action = params[idx][1]
	}
	return mod, action
}

// finishKey turns a key and the kitty protocol's event kind into an event.
func finishKey(k Key, action int) Event {
	switch action {
	case 2:
		k.Repeat = true
	case 3:
		return KeyRelease{Key: k}
	}
	return k
}

// validTextRune reports whether n is a character that may appear in text:
// a Unicode scalar value that is not a control character.
func validTextRune(n int) bool {
	if n < 0x20 || n == 0x7f || n > unicode.MaxRune {
		return false
	}
	r := rune(n)
	return !isC1(r) && utf8.ValidRune(r)
}

// keyFromCode is the key the kitty protocol, and xterm's modifyOtherKeys,
// name by the number code.
func keyFromCode(code int) (Key, bool) {
	switch code {
	case 27:
		return Key{Code: KeyEscape}, true
	case 13, 57414:
		return Key{Code: KeyEnter}, true
	case 9:
		return Key{Code: KeyTab}, true
	case 127, 8:
		return Key{Code: KeyBackspace}, true
	case 57358:
		return Key{Code: KeyCapsLock}, true
	case 57359:
		return Key{Code: KeyScrollLock}, true
	case 57360:
		return Key{Code: KeyNumLock}, true
	case 57361:
		return Key{Code: KeyPrintScreen}, true
	case 57362:
		return Key{Code: KeyPause}, true
	case 57363:
		return Key{Code: KeyMenu}, true
	}
	switch {
	case code >= 57364 && code <= 57398:
		return Key{Code: KeyF1 + KeyCode(code-57364)}, true
	case code >= 57399 && code <= 57408:
		return Key{Code: KeyRune, Rune: rune('0' + code - 57399)}, true
	case code >= 57409 && code <= 57416:
		if code == 57414 {
			break
		}
		return Key{Code: KeyRune, Rune: rune("./*-+ =,"[code-57409])}, true
	case code >= 57417 && code <= 57427:
		return Key{Code: [...]KeyCode{
			KeyLeft, KeyRight, KeyUp, KeyDown, KeyPageUp, KeyPageDown,
			KeyHome, KeyEnd, KeyInsert, KeyDelete, KeyBegin,
		}[code-57417]}, true
	case code >= 57428 && code <= 57440:
		return Key{Code: KeyMedia, Rune: rune(code)}, true
	case code >= 57441 && code <= 57454:
		return Key{Code: KeyModifier, Rune: rune(code)}, true
	case code >= 0xe000 && code <= 0xf8ff:
		// The rest of the private use area names no key.
		return Key{}, false
	}
	if !validTextRune(code) {
		return Key{}, false
	}
	return Key{Code: KeyRune, Rune: rune(code)}, true
}

// impliedText is what a key reported as a number types, when the terminal
// did not say: the key itself with no modifiers, its shifted form with
// Shift when that is known. shifted is the shifted key the terminal named,
// or zero.
func impliedText(k Key, shifted int) string {
	if k.Code != KeyRune {
		return ""
	}
	switch k.Mod &^ (ModCapsLock | ModNumLock) {
	case 0:
		if k.Mod&ModCapsLock != 0 {
			return string(unicode.ToUpper(k.Rune))
		}
		return string(k.Rune)
	case ModShift:
		switch {
		case shifted > 0 && validTextRune(shifted):
			return string(rune(shifted))
		case unicode.IsLetter(k.Rune):
			return string(unicode.ToUpper(k.Rune))
		case k.Rune == ' ':
			return " "
		}
	}
	return ""
}

// num is the first part of a parameter field, or def when it was left out.
func num(field []int, def int) int {
	if len(field) == 0 || field[0] < 0 {
		return def
	}
	return field[0]
}

// firsts is the first part of every field.
func firsts(params [][]int) []int {
	out := make([]int, len(params))
	for i, f := range params {
		out[i] = num(f, 0)
	}
	return out
}

// parseCSI reads input that begins with ESC [.
func (d *Decoder) parseCSI(out []Event, b []byte, flush bool) (int, []Event) {
	i := 2
	for i < len(b) && b[i] >= 0x30 && b[i] <= 0x3f {
		i++
	}
	pEnd := i
	for i < len(b) && b[i] >= 0x20 && b[i] <= 0x2f {
		i++
	}
	iEnd := i
	// rxvt writes Shift with a function key as "CSI number $", which has
	// no final byte. No answer gnosh can receive has that shape.
	if pEnd > 2 && pEnd < len(b) && b[pEnd] == '$' {
		if ps, ok := parseParams(b[2:pEnd]); ok && len(ps) == 1 && len(ps[0]) == 1 {
			if code, found := tildeKey(ps[0][0]); found {
				return pEnd + 1, append(out, Key{Code: code, Mod: ModShift})
			}
		}
	}
	if i == len(b) {
		if len(b) == 2 && flush {
			return 2, append(out, altKey('['))
		}
		return 0, out
	}
	unknown := func(n int) (int, []Event) {
		return n, append(out, Unknown{Raw: string(b[:n])})
	}
	f := b[i]
	if f < 0x40 || f > 0x7e {
		if f >= 0x30 && f <= 0x3f {
			// A parameter byte after an intermediate byte: the
			// sequence is malformed. Take all of it.
			j := i
			for j < len(b) && b[j] >= 0x20 && b[j] <= 0x3f {
				j++
			}
			if j == len(b) {
				return 0, out
			}
			if b[j] >= 0x40 && b[j] <= 0x7e {
				j++
			}
			return unknown(j)
		}
		// Something that cannot be inside a sequence: it was Alt+[, or
		// the sequence is broken off here.
		if i == 2 {
			return 2, append(out, altKey('['))
		}
		return unknown(i)
	}
	end := i + 1
	ps := b[2:pEnd]
	var prefix byte
	if len(ps) > 0 && ps[0] >= '<' && ps[0] <= '?' {
		prefix = ps[0]
		ps = ps[1:]
	}
	inter := b[pEnd:iEnd]
	params, ok := parseParams(ps)
	if !ok {
		return unknown(end)
	}

	// A mode report (DECRPM): CSI ? mode ; state $ y.
	if len(inter) == 1 && inter[0] == '$' && f == 'y' && (prefix == 0 || prefix == '?') && len(params) == 2 {
		state := ModeState(num(params[1], 0))
		if state < ModeNotRecognized || state > ModePermanentlyReset {
			state = ModeNotRecognized
		}
		return end, append(out, Reply{Kind: ReplyMode, Mode: num(params[0], 0), Private: prefix == '?', State: state})
	}
	if len(inter) > 0 {
		return unknown(end)
	}

	switch prefix {
	case '?':
		switch {
		case f == 'u' && len(params) <= 1:
			flags := 0
			if len(params) == 1 {
				flags = num(params[0], 0)
			}
			return end, append(out, Reply{Kind: ReplyKittyKeyboard, Flags: flags})
		case f == 'c':
			return end, append(out, Reply{Kind: ReplyDeviceAttributes, Params: firsts(params)})
		case f == 'R' && len(params) >= 2:
			return end, append(out, Reply{Kind: ReplyCursorPosition, Row: num(params[0], 1), Col: num(params[1], 1)})
		}
		return unknown(end)
	case '>':
		if f == 'c' {
			return end, append(out, Reply{Kind: ReplySecondaryAttributes, Params: firsts(params)})
		}
		return unknown(end)
	case '<':
		if (f == 'M' || f == 'm') && len(params) == 3 {
			m := mouseEvent(num(params[0], 0), num(params[1], 1), num(params[2], 1), true)
			if f == 'm' {
				m.Action = MouseRelease
			}
			return end, append(out, m)
		}
		return unknown(end)
	case '=':
		return unknown(end)
	}

	switch f {
	case 'u':
		return d.kittyKey(out, b, end, params)
	case '~':
		if len(params) == 0 {
			return unknown(end)
		}
		n := num(params[0], 0)
		switch {
		case n == 200 && len(params) == 1:
			return end, append(out, pasteStarted{})
		case n == 27 && len(params) == 3:
			// xterm's modifyOtherKeys: CSI 27 ; modifier ; code ~.
			k, ok := keyFromCode(num(params[2], 0))
			if !ok {
				return unknown(end)
			}
			mod, action := modifierParam(params, 1)
			k.Mod = mod
			if k.Code == KeyRune && mod&^(ModCapsLock|ModNumLock) == ModShift {
				// xterm names the character Shift produced.
				k.Text = string(k.Rune)
				k.Rune = unicode.ToLower(k.Rune)
			} else {
				k.Text = impliedText(k, 0)
			}
			return end, append(out, finishKey(k, action))
		}
		code, found := tildeKey(n)
		if !found || len(params) > 2 {
			return unknown(end)
		}
		mod, action := modifierParam(params, 1)
		return end, append(out, finishKey(Key{Code: code, Mod: mod}, action))
	case '^', '@':
		// rxvt: Ctrl, and Ctrl+Shift, with a function key.
		if len(params) == 1 {
			if code, found := tildeKey(num(params[0], 0)); found {
				mod := ModCtrl
				if f == '@' {
					mod |= ModShift
				}
				return end, append(out, Key{Code: code, Mod: mod})
			}
		}
		return unknown(end)
	case 'I', 'O':
		if len(params) == 0 {
			return end, append(out, Focus{In: f == 'I'})
		}
		return unknown(end)
	case 'M':
		switch len(params) {
		case 0:
			// The oldest mouse report: three bytes follow, each a
			// value plus 32.
			for j := 0; j < 3; j++ {
				if end+j >= len(b) {
					return 0, out
				}
				if b[end+j] < 0x20 {
					return unknown(end + j)
				}
			}
			m := mouseEvent(int(b[end])-32, int(b[end+1])-32, int(b[end+2])-32, false)
			return end + 3, append(out, m)
		case 3:
			// urxvt's form: the same values as numbers.
			m := mouseEvent(num(params[0], 32)-32, num(params[1], 1), num(params[2], 1), false)
			return end, append(out, m)
		}
		return unknown(end)
	case '[':
		// The Linux console's F1 to F5: CSI [ A to CSI [ E.
		if len(params) != 0 {
			return unknown(end)
		}
		if end >= len(b) {
			return 0, out
		}
		if c := b[end]; c >= 'A' && c <= 'E' {
			return end + 1, append(out, Key{Code: KeyF1 + KeyCode(c-'A')})
		}
		return unknown(end)
	case 'a', 'b', 'c', 'd':
		// rxvt: Shift with an arrow.
		if len(params) == 0 {
			code := [...]KeyCode{KeyUp, KeyDown, KeyRight, KeyLeft}[f-'a']
			return end, append(out, Key{Code: code, Mod: ModShift})
		}
		return unknown(end)
	case 'R':
		// A cursor position report, CSI row ; col R. With row 1 it is
		// also how xterm writes F3 with modifiers.
		if len(params) == 2 {
			row, col := num(params[0], 1), num(params[1], 1)
			if row != 1 || d.expectCPR > 0 {
				return end, append(out, Reply{Kind: ReplyCursorPosition, Row: row, Col: col})
			}
		}
	}

	code, found := letterKey(f)
	if !found || len(params) > 2 {
		return unknown(end)
	}
	mod, action := modifierParam(params, len(params)-1)
	if f == 'Z' {
		mod |= ModShift
	}
	return end, append(out, finishKey(Key{Code: code, Mod: mod}, action))
}

// kittyKey reads the kitty keyboard protocol's key report:
// CSI code:shifted:base ; modifiers:event ; text u.
func (d *Decoder) kittyKey(out []Event, b []byte, end int, params [][]int) (int, []Event) {
	unknown := func() (int, []Event) {
		return end, append(out, Unknown{Raw: string(b[:end])})
	}
	if len(params) == 0 || len(params) > 3 {
		return unknown()
	}
	text := ""
	if len(params) == 3 {
		for _, cp := range params[2] {
			if cp < 0 {
				continue
			}
			if !validTextRune(cp) {
				return unknown()
			}
			text += string(rune(cp))
		}
	}
	code := num(params[0], 0)
	var k Key
	if code == 0 {
		// No key applies: the report carries text only.
		if text == "" {
			return unknown()
		}
		r, _ := utf8.DecodeRuneInString(text)
		k = Key{Code: KeyRune, Rune: r}
	} else {
		var ok bool
		if k, ok = keyFromCode(code); !ok {
			return unknown()
		}
	}
	mod, action := modifierParam(params, 1)
	k.Mod = mod
	if text != "" {
		k.Text = text
	} else {
		shifted := 0
		if len(params[0]) > 1 {
			shifted = params[0][1]
		}
		k.Text = impliedText(k, shifted)
	}
	return end, append(out, finishKey(k, action))
}

// mouseEvent reads the button value of a mouse report. sgr is true for the
// form that says press or release with its final byte, where the value 3
// in the low bits is not "released".
func mouseEvent(b, col, row int, sgr bool) Mouse {
	m := Mouse{Col: col, Row: row}
	if b&4 != 0 {
		m.Mod |= ModShift
	}
	if b&8 != 0 {
		m.Mod |= ModAlt
	}
	if b&16 != 0 {
		m.Mod |= ModCtrl
	}
	low := b & 3
	switch {
	case b&64 != 0:
		m.Action = [...]MouseAction{MouseWheelUp, MouseWheelDown, MouseWheelLeft, MouseWheelRight}[low]
		return m
	case b&128 != 0:
		m.Button = 8 + low
	case low == 3 && !sgr:
		m.Action = MouseRelease
	case low != 3:
		m.Button = low + 1
	}
	if b&32 != 0 {
		m.Action = MouseMotion
	}
	return m
}

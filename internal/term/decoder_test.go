package term

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// txt is a character typed as plain text.
func txt(r rune) Key { return Key{Code: KeyRune, Rune: r, Text: string(r)} }

// chr is a character key held with modifiers, which types nothing.
func chr(r rune, mod Mod) Key { return Key{Code: KeyRune, Rune: r, Mod: mod} }

// key is a named key.
func key(code KeyCode, mod Mod) Key { return Key{Code: code, Mod: mod} }

// decodeAll feeds the whole input in one piece and then flushes.
func decodeAll(input string) []Event {
	var d Decoder
	evs := d.Feed([]byte(input))
	return append(evs, d.Flush()...)
}

func describe(evs []Event) string {
	parts := make([]string, len(evs))
	for i, ev := range evs {
		parts[i] = fmt.Sprintf("%T{%s}", ev, ev)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

type decodeCase struct {
	name  string
	input string
	want  []Event
}

// The sources named in the cases below:
//
//	ECMA-48    Control Functions for Coded Character Sets, 5th edition
//	ctlseqs    XTerm Control Sequences (invisible-island.net/xterm/ctlseqs)
//	VT100/VT220  the DEC user guides and programmer references (vt100.net)
//	kitty      Comprehensive keyboard handling in terminals
//	           (sw.kovidgoyal.net/kitty/keyboard-protocol)
//	rxvt       the rxvt reference's key code table (rxvtRef)
var decodeCases = []decodeCase{
	// ECMA-48 section 5.2 (the C0 set) and kitty "Legacy text keys": a
	// control character is Ctrl with the key 0x40 above it.
	{"C0 ctrl+a", "\x01", []Event{chr('a', ModCtrl)}},
	{"C0 ctrl+c", "\x03", []Event{chr('c', ModCtrl)}},
	{"C0 ctrl+z", "\x1a", []Event{chr('z', ModCtrl)}},
	{"C0 ctrl+space is NUL", "\x00", []Event{chr(' ', ModCtrl)}},
	{"C0 ctrl+backslash", "\x1c", []Event{chr('\\', ModCtrl)}},
	{"C0 ctrl+]", "\x1d", []Event{chr(']', ModCtrl)}},
	{"C0 ctrl+^", "\x1e", []Event{chr('^', ModCtrl)}},
	{"C0 ctrl+_", "\x1f", []Event{chr('_', ModCtrl)}},
	// kitty "Legacy key event encoding", the table of Enter, Tab,
	// Backspace and Escape.
	{"Tab is HT", "\t", []Event{key(KeyTab, 0)}},
	{"Enter is CR", "\r", []Event{key(KeyEnter, 0)}},
	{"LF is ctrl+j", "\n", []Event{chr('j', ModCtrl)}},
	{"Backspace is DEL", "\x7f", []Event{key(KeyBackspace, 0)}},
	{"Backspace is BS", "\x08", []Event{key(KeyBackspace, 0)}},
	{"Esc alone, after a flush", "\x1b", []Event{key(KeyEscape, 0)}},
	{"alt+enter", "\x1b\r", []Event{key(KeyEnter, ModAlt)}},
	{"alt+backspace", "\x1b\x7f", []Event{key(KeyBackspace, ModAlt)}},
	{"alt+tab", "\x1b\t", []Event{key(KeyTab, ModAlt)}},
	{"alt+esc", "\x1b\x1b", []Event{key(KeyEscape, ModAlt)}},
	{"ctrl+alt+c", "\x1b\x03", []Event{chr('c', ModCtrl|ModAlt)}},
	{"ctrl+alt+space", "\x1b\x00", []Event{chr(' ', ModCtrl|ModAlt)}},

	// Text: ASCII and UTF-8 (RFC 3629).
	{"ASCII text", "aZ 9~", []Event{txt('a'), txt('Z'), txt(' '), txt('9'), txt('~')}},
	{"UTF-8 two bytes", "é", []Event{txt('é')}},
	{"UTF-8 three bytes", "世", []Event{txt('世')}},
	{"UTF-8 four bytes", "😀", []Event{txt('😀')}},
	{"UTF-8 replacement character is text", "\xef\xbf\xbd", []Event{txt('\uFFFD')}},
	{"invalid byte", "\xff", []Event{Unknown{Raw: "\xff"}}},
	{"lone continuation byte", "a\x80b", []Event{txt('a'), Unknown{Raw: "\x80"}, txt('b')}},
	{"truncated UTF-8 then ASCII", "\xe4\xb8a", []Event{Unknown{Raw: "\xe4"}, Unknown{Raw: "\xb8"}, txt('a')}},
	{"overlong encoding", "\xc0\xaf", []Event{Unknown{Raw: "\xc0"}, Unknown{Raw: "\xaf"}}},
	{"C1 control as UTF-8 is not text", "\xc2\x9b", []Event{Unknown{Raw: "\xc2\x9b"}}},

	// ctlseqs "Alt and Meta Keys": with metaSendsEscape, Alt prefixes ESC.
	{"alt+a", "\x1ba", []Event{chr('a', ModAlt)}},
	{"alt+shift+a", "\x1bA", []Event{chr('a', ModAlt|ModShift)}},
	{"alt+space", "\x1b ", []Event{chr(' ', ModAlt)}},
	{"alt+UTF-8", "\x1bé", []Event{chr('é', ModAlt)}},
	{"alt+b then alt+f (Apple Terminal's Option+arrows)", "\x1bb\x1bf", []Event{chr('b', ModAlt), chr('f', ModAlt)}},
	{"alt+[ after a flush", "\x1b[", []Event{chr('[', ModAlt)}},
	{"alt+shift+o after a flush", "\x1bO", []Event{chr('o', ModAlt|ModShift)}},
	{"alt+] after a flush", "\x1b]", []Event{chr(']', ModAlt)}},
	{"alt+shift+p after a flush", "\x1bP", []Event{chr('p', ModAlt|ModShift)}},
	{"alt+[ repeated", "\x1b[\x1b[", []Event{chr('[', ModAlt), chr('[', ModAlt)}},
	{"alt+] then enter", "\x1b]\r", []Event{chr(']', ModAlt), key(KeyEnter, 0)}},
	{"esc then alt+[", "\x1b\x1b[", []Event{key(KeyEscape, 0), chr('[', ModAlt)}},

	// ctlseqs "PC-Style Function Keys" and VT100 cursor keys (normal mode).
	{"up", "\x1b[A", []Event{key(KeyUp, 0)}},
	{"down", "\x1b[B", []Event{key(KeyDown, 0)}},
	{"right", "\x1b[C", []Event{key(KeyRight, 0)}},
	{"left", "\x1b[D", []Event{key(KeyLeft, 0)}},
	{"home", "\x1b[H", []Event{key(KeyHome, 0)}},
	{"end", "\x1b[F", []Event{key(KeyEnd, 0)}},
	{"keypad begin", "\x1b[E", []Event{key(KeyBegin, 0)}},
	{"shift+tab is CBT", "\x1b[Z", []Event{key(KeyTab, ModShift)}},
	// ctlseqs "PC-Style Function Keys": modifiers as a second parameter,
	// one more than the sum of Shift 1, Alt 2, Ctrl 4, Meta 8.
	{"shift+up", "\x1b[1;2A", []Event{key(KeyUp, ModShift)}},
	{"alt+down", "\x1b[1;3B", []Event{key(KeyDown, ModAlt)}},
	{"ctrl+right", "\x1b[1;5C", []Event{key(KeyRight, ModCtrl)}},
	{"ctrl+shift+left", "\x1b[1;6D", []Event{key(KeyLeft, ModCtrl|ModShift)}},
	{"ctrl+alt+shift+home", "\x1b[1;8H", []Event{key(KeyHome, ModCtrl|ModAlt|ModShift)}},
	{"super+end", "\x1b[1;9F", []Event{key(KeyEnd, ModSuper)}},
	{"ctrl+f1", "\x1b[1;5P", []Event{key(KeyF1, ModCtrl)}},
	{"shift+f2", "\x1b[1;2Q", []Event{key(KeyF2, ModShift)}},
	{"ctrl+f3 when no cursor report is owed", "\x1b[1;5R", []Event{key(KeyF3, ModCtrl)}},
	{"alt+f4", "\x1b[1;3S", []Event{key(KeyF4, ModAlt)}},
	{"alt+up as ESC ESC [ A (rxvt, iTerm2)", "\x1b\x1b[A", []Event{key(KeyUp, ModAlt)}},

	// VT100 "cursor key mode" (DECCKM) and ctlseqs: SS3 forms.
	{"SS3 up", "\x1bOA", []Event{key(KeyUp, 0)}},
	{"SS3 down", "\x1bOB", []Event{key(KeyDown, 0)}},
	{"SS3 right", "\x1bOC", []Event{key(KeyRight, 0)}},
	{"SS3 left", "\x1bOD", []Event{key(KeyLeft, 0)}},
	{"SS3 home", "\x1bOH", []Event{key(KeyHome, 0)}},
	{"SS3 end", "\x1bOF", []Event{key(KeyEnd, 0)}},
	{"SS3 f1", "\x1bOP", []Event{key(KeyF1, 0)}},
	{"SS3 f2", "\x1bOQ", []Event{key(KeyF2, 0)}},
	{"SS3 f3", "\x1bOR", []Event{key(KeyF3, 0)}},
	{"SS3 f4", "\x1bOS", []Event{key(KeyF4, 0)}},
	{"SS3 with modifiers", "\x1bO1;5P", []Event{key(KeyF1, ModCtrl)}},
	{"SS3 keypad enter", "\x1bOM", []Event{key(KeyEnter, 0)}},
	{"SS3 keypad digits", "\x1bOp\x1bOy", []Event{txt('0'), txt('9')}},
	{"SS3 keypad operators", "\x1bOj\x1bOk\x1bOm\x1bOo\x1bOX", []Event{txt('*'), txt('+'), txt('-'), txt('/'), txt('=')}},
	{"alt+f1 as ESC ESC O P", "\x1b\x1bOP", []Event{key(KeyF1, ModAlt)}},
	{"SS3 with a final gnosh does not know", "\x1bOz", []Event{Unknown{Raw: "\x1bOz"}}},
	{"alt+shift+o then up", "\x1bO\x1b[A", []Event{chr('o', ModAlt|ModShift), key(KeyUp, 0)}},

	// ctlseqs "VT220-Style Function Keys" and the DEC editing keypad.
	{"insert", "\x1b[2~", []Event{key(KeyInsert, 0)}},
	{"delete", "\x1b[3~", []Event{key(KeyDelete, 0)}},
	{"page up", "\x1b[5~", []Event{key(KeyPageUp, 0)}},
	{"page down", "\x1b[6~", []Event{key(KeyPageDown, 0)}},
	{"home as CSI 1 ~", "\x1b[1~", []Event{key(KeyHome, 0)}},
	{"end as CSI 4 ~", "\x1b[4~", []Event{key(KeyEnd, 0)}},
	{"f1 to f5, VT spelling", "\x1b[11~\x1b[12~\x1b[13~\x1b[14~\x1b[15~",
		[]Event{key(KeyF1, 0), key(KeyF2, 0), key(KeyF3, 0), key(KeyF4, 0), key(KeyF5, 0)}},
	{"f6 to f10", "\x1b[17~\x1b[18~\x1b[19~\x1b[20~\x1b[21~",
		[]Event{key(KeyF6, 0), key(KeyF7, 0), key(KeyF8, 0), key(KeyF9, 0), key(KeyF10, 0)}},
	{"f11 f12", "\x1b[23~\x1b[24~", []Event{key(KeyF11, 0), key(KeyF12, 0)}},
	{"f13 to f20", "\x1b[25~\x1b[26~\x1b[28~\x1b[29~\x1b[31~\x1b[32~\x1b[33~\x1b[34~",
		[]Event{key(KeyF13, 0), key(KeyF14, 0), key(KeyF15, 0), key(KeyF16, 0), key(KeyF17, 0), key(KeyF18, 0), key(KeyF19, 0), key(KeyF20, 0)}},
	{"ctrl+delete", "\x1b[3;5~", []Event{key(KeyDelete, ModCtrl)}},
	{"shift+f5", "\x1b[15;2~", []Event{key(KeyF5, ModShift)}},
	{"ctrl+alt+page up", "\x1b[5;7~", []Event{key(KeyPageUp, ModCtrl|ModAlt)}},
	{"a tilde number that names no key", "\x1b[99~", []Event{Unknown{Raw: "\x1b[99~"}}},
	// The Linux console (console_codes(4)): F1 to F5 are CSI [ A to E.
	{"linux console f1 and f5", "\x1b[[A\x1b[[E", []Event{key(KeyF1, 0), key(KeyF5, 0)}},

	// rxvt: Home and End are 7~ and 8~; Shift is "$", Ctrl "^", both "@";
	// Shift+arrow is a lower-case final, Ctrl+arrow an SS3 one.
	{"rxvt home end", "\x1b[7~\x1b[8~", []Event{key(KeyHome, 0), key(KeyEnd, 0)}},
	{"rxvt shift+insert", "\x1b[2$", []Event{key(KeyInsert, ModShift)}},
	{"rxvt ctrl+delete", "\x1b[3^", []Event{key(KeyDelete, ModCtrl)}},
	{"rxvt ctrl+shift+home", "\x1b[7@", []Event{key(KeyHome, ModCtrl|ModShift)}},
	{"rxvt shift+f5 then text", "\x1b[15$x", []Event{key(KeyF5, ModShift), txt('x')}},
	{"rxvt shift+arrows", "\x1b[a\x1b[b\x1b[c\x1b[d",
		[]Event{key(KeyUp, ModShift), key(KeyDown, ModShift), key(KeyRight, ModShift), key(KeyLeft, ModShift)}},
	{"rxvt ctrl+arrows", "\x1bOa\x1bOd", []Event{key(KeyUp, ModCtrl), key(KeyLeft, ModCtrl)}},

	// kitty: CSI code ; modifiers u.
	{"kitty esc", "\x1b[27u", []Event{key(KeyEscape, 0)}},
	{"kitty shift+enter", "\x1b[13;2u", []Event{key(KeyEnter, ModShift)}},
	{"kitty ctrl+enter", "\x1b[13;5u", []Event{key(KeyEnter, ModCtrl)}},
	{"kitty alt+enter", "\x1b[13;3u", []Event{key(KeyEnter, ModAlt)}},
	{"kitty ctrl+c", "\x1b[99;5u", []Event{chr('c', ModCtrl)}},
	{"kitty ctrl+shift+a", "\x1b[97;6u", []Event{chr('a', ModCtrl|ModShift)}},
	{"kitty alt+a", "\x1b[97;3u", []Event{chr('a', ModAlt)}},
	{"kitty ctrl+i is not tab", "\x1b[105;5u", []Event{chr('i', ModCtrl)}},
	{"kitty ctrl+j is the same key as LF", "\x1b[106;5u", []Event{chr('j', ModCtrl)}},
	{"kitty ctrl+backspace", "\x1b[127;5u", []Event{key(KeyBackspace, ModCtrl)}},
	{"kitty shift+tab", "\x1b[9;2u", []Event{key(KeyTab, ModShift)}},
	{"kitty ctrl+space", "\x1b[32;5u", []Event{chr(' ', ModCtrl)}},
	{"kitty super+hyper+meta", "\x1b[97;57u", []Event{chr('a', ModSuper|ModHyper|ModMeta)}},
	{"kitty plain key types itself", "\x1b[97u", []Event{txt('a')}},
	{"kitty shift+a types A", "\x1b[97;2u", []Event{Key{Code: KeyRune, Rune: 'a', Text: "A", Mod: ModShift}}},
	{"kitty shifted key named", "\x1b[49:33;2u", []Event{Key{Code: KeyRune, Rune: '1', Text: "!", Mod: ModShift}}},
	{"kitty associated text", "\x1b[97;2;65u", []Event{Key{Code: KeyRune, Rune: 'a', Text: "A", Mod: ModShift}}},
	{"kitty text with no key", "\x1b[0;;229u", []Event{Key{Code: KeyRune, Rune: 'å', Text: "å"}}},
	{"kitty caps lock held with a letter", "\x1b[97;65u", []Event{Key{Code: KeyRune, Rune: 'a', Text: "A", Mod: ModCapsLock}}},
	{"kitty repeat", "\x1b[97;1:2u", []Event{Key{Code: KeyRune, Rune: 'a', Text: "a", Repeat: true}}},
	{"kitty release is its own event", "\x1b[97;1:3u", []Event{KeyRelease{Key: txt('a')}}},
	{"kitty release of an arrow", "\x1b[1;1:3A", []Event{KeyRelease{Key: key(KeyUp, 0)}}},
	{"kitty f13 and f35", "\x1b[57376u\x1b[57398u", []Event{key(KeyF13, 0), key(KeyF35, 0)}},
	{"kitty keypad enter", "\x1b[57414u", []Event{key(KeyEnter, 0)}},
	{"kitty keypad 5 with ctrl", "\x1b[57404;5u", []Event{chr('5', ModCtrl)}},
	{"kitty keypad left", "\x1b[57417u", []Event{key(KeyLeft, 0)}},
	{"kitty keypad begin", "\x1b[57427~\x1b[1;5E", []Event{key(KeyBegin, 0), key(KeyBegin, ModCtrl)}},
	{"kitty menu and pause", "\x1b[57363u\x1b[57362u", []Event{key(KeyMenu, 0), key(KeyPause, 0)}},
	{"kitty left shift alone", "\x1b[57441;2u", []Event{Key{Code: KeyModifier, Rune: 57441, Mod: ModShift}}},
	{"kitty media key", "\x1b[57428u", []Event{Key{Code: KeyMedia, Rune: 57428}}},
	{"kitty f3 is 13~", "\x1b[13;5~", []Event{key(KeyF3, ModCtrl)}},
	{"kitty unassigned private use code", "\x1b[57455u", []Event{Unknown{Raw: "\x1b[57455u"}}},
	{"kitty control character as a key code", "\x1b[3;5u", []Event{Unknown{Raw: "\x1b[3;5u"}}},
	{"kitty control character as text", "\x1b[97;1;10u", []Event{Unknown{Raw: "\x1b[97;1;10u"}}},
	{"kitty code past the end of Unicode", "\x1b[1114112u", []Event{Unknown{Raw: "\x1b[1114112u"}}},
	{"CSI u with nothing in it", "\x1b[u", []Event{Unknown{Raw: "\x1b[u"}}},

	// ctlseqs "modifyOtherKeys": CSI 27 ; modifier ; code ~.
	{"modifyOtherKeys shift+enter", "\x1b[27;2;13~", []Event{key(KeyEnter, ModShift)}},
	{"modifyOtherKeys ctrl+enter", "\x1b[27;5;13~", []Event{key(KeyEnter, ModCtrl)}},
	{"modifyOtherKeys ctrl+tab", "\x1b[27;5;9~", []Event{key(KeyTab, ModCtrl)}},
	{"modifyOtherKeys ctrl+,", "\x1b[27;5;44~", []Event{chr(',', ModCtrl)}},
	{"modifyOtherKeys shift+A", "\x1b[27;2;65~", []Event{Key{Code: KeyRune, Rune: 'a', Text: "A", Mod: ModShift}}},
	{"modifyOtherKeys alt+shift+!", "\x1b[27;4;33~", []Event{chr('!', ModAlt|ModShift)}},

	// ctlseqs "Bracketed Paste Mode": CSI 200 ~ text CSI 201 ~.
	{"paste", "\x1b[200~hello\x1b[201~", []Event{Paste{Text: "hello"}}},
	{"empty paste", "\x1b[200~\x1b[201~", []Event{Paste{Text: ""}}},
	{"paste between keys", "a\x1b[200~b\x1b[201~c", []Event{txt('a'), Paste{Text: "b"}, txt('c')}},
	{"paste with CR line ends", "\x1b[200~one\rtwo\r\nthree\n\x1b[201~", []Event{Paste{Text: "one\ntwo\nthree\n"}}},
	{"paste holding escape sequences", "\x1b[200~\x1b[31mred\x1b[A\x03\x1b[201~", []Event{Paste{Text: "\x1b[31mred\x1b[A\x03"}}},
	{"paste holding the start mark", "\x1b[200~a\x1b[200~b\x1b[201~", []Event{Paste{Text: "a\x1b[200~b"}}},
	{"paste holding prefixes of the end mark", "\x1b[200~x\x1b[201\x1b[20\x1b[2\x1b[\x1b\x1b[201~", []Event{Paste{Text: "x\x1b[201\x1b[20\x1b[2\x1b[\x1b"}}},
	{"paste with bytes that are not UTF-8", "\x1b[200~a\xffb\x1b[201~", []Event{Paste{Text: "a\uFFFDb"}}},
	{"end mark with no paste open", "\x1b[201~", []Event{Unknown{Raw: "\x1b[201~"}}},
	{"two pastes", "\x1b[200~1\x1b[201~\x1b[200~2\x1b[201~", []Event{Paste{Text: "1"}, Paste{Text: "2"}}},

	// ctlseqs "FocusIn/FocusOut" (mode 1004).
	{"focus in and out", "\x1b[I\x1b[O", []Event{Focus{In: true}, Focus{In: false}}},

	// Answers. ECMA-48 8.3.14 (CPR), ctlseqs DSR/DECXCPR, DA, DECRPM;
	// kitty "Detection of support for this protocol".
	{"cursor position report", "\x1b[12;40R", []Event{Reply{Kind: ReplyCursorPosition, Row: 12, Col: 40}}},
	{"cursor position report, DEC form", "\x1b[?1;5R", []Event{Reply{Kind: ReplyCursorPosition, Row: 1, Col: 5}}},
	{"cursor position report with a page", "\x1b[?3;7;1R", []Event{Reply{Kind: ReplyCursorPosition, Row: 3, Col: 7}}},
	{"primary device attributes", "\x1b[?62;4;22c", []Event{Reply{Kind: ReplyDeviceAttributes, Params: []int{62, 4, 22}}}},
	{"secondary device attributes", "\x1b[>1;10;0c", []Event{Reply{Kind: ReplySecondaryAttributes, Params: []int{1, 10, 0}}}},
	{"kitty keyboard flags", "\x1b[?1u", []Event{Reply{Kind: ReplyKittyKeyboard, Flags: 1}}},
	{"kitty keyboard flags, none set", "\x1b[?0u", []Event{Reply{Kind: ReplyKittyKeyboard, Flags: 0}}},
	{"mode report: synchronized output reset", "\x1b[?2026;2$y", []Event{Reply{Kind: ReplyMode, Mode: 2026, Private: true, State: ModeReset}}},
	{"mode report: not recognized", "\x1b[?2026;0$y", []Event{Reply{Kind: ReplyMode, Mode: 2026, Private: true, State: ModeNotRecognized}}},
	{"mode report: ANSI mode", "\x1b[4;1$y", []Event{Reply{Kind: ReplyMode, Mode: 4, State: ModeSet}}},
	{"OSC answer ended by BEL", "\x1b]11;rgb:0000/0000/0000\x07", []Event{Reply{Kind: ReplyString, Intro: ']', Text: "11;rgb:0000/0000/0000"}}},
	{"OSC answer ended by ST", "\x1b]11;rgb:ffff/ffff/ffff\x1b\\", []Event{Reply{Kind: ReplyString, Intro: ']', Text: "11;rgb:ffff/ffff/ffff"}}},
	{"DCS answer (XTVERSION)", "\x1bP>|term 1.0\x1b\\", []Event{Reply{Kind: ReplyString, Intro: 'P', Text: ">|term 1.0"}}},
	{"APC answer", "\x1b_Gi=1;OK\x1b\\", []Event{Reply{Kind: ReplyString, Intro: '_', Text: "Gi=1;OK"}}},

	// ctlseqs "Mouse Tracking": never asked for, always consumed.
	{"SGR mouse press", "\x1b[<0;10;5M", []Event{Mouse{Action: MousePress, Button: 1, Col: 10, Row: 5}}},
	{"SGR mouse release", "\x1b[<2;10;5m", []Event{Mouse{Action: MouseRelease, Button: 3, Col: 10, Row: 5}}},
	{"SGR mouse wheel", "\x1b[<64;1;1M\x1b[<65;1;1M", []Event{Mouse{Action: MouseWheelUp, Col: 1, Row: 1}, Mouse{Action: MouseWheelDown, Col: 1, Row: 1}}},
	{"SGR mouse drag with ctrl", "\x1b[<48;3;4M", []Event{Mouse{Action: MouseMotion, Button: 1, Col: 3, Row: 4, Mod: ModCtrl}}},
	{"X10 mouse press", "\x1b[M !!", []Event{Mouse{Action: MousePress, Button: 1, Col: 1, Row: 1}}},
	{"X10 mouse release then text", "\x1b[M#%&x", []Event{Mouse{Action: MouseRelease, Col: 5, Row: 6}, txt('x')}},
	{"urxvt mouse", "\x1b[32;7;9M", []Event{Mouse{Action: MousePress, Button: 1, Col: 7, Row: 9}}},

	// Sequences with no meaning here, and broken ones: consumed whole.
	{"unknown final", "\x1b[5n", []Event{Unknown{Raw: "\x1b[5n"}}},
	{"unknown private sequence", "\x1b[?1;2x", []Event{Unknown{Raw: "\x1b[?1;2x"}}},
	{"window report", "\x1b[8;24;80t", []Event{Unknown{Raw: "\x1b[8;24;80t"}}},
	{"intermediate bytes", "\x1b[1 q", []Event{Unknown{Raw: "\x1b[1 q"}}},
	{"parameter after an intermediate", "\x1b[1 2q", []Event{Unknown{Raw: "\x1b[1 2q"}}},
	{"too many parameters for a key", "\x1b[1;2;3A", []Event{Unknown{Raw: "\x1b[1;2;3A"}}},
	{"private marker in the middle", "\x1b[1?2A", []Event{Unknown{Raw: "\x1b[1?2A"}}},
	{"sequence broken off by a control key", "\x1b[1;\r", []Event{Unknown{Raw: "\x1b[1;"}, key(KeyEnter, 0)}},
	{"sequence broken off by another", "\x1b[1;\x1b[A", []Event{Unknown{Raw: "\x1b[1;"}, key(KeyUp, 0)}},
	{"string broken off by a control key", "\x1b]52;c;abc\x03", []Event{Unknown{Raw: "\x1b]52;c;abc"}, chr('c', ModCtrl)}},
	{"string broken off by a sequence", "\x1bPabc\x1b[A", []Event{Unknown{Raw: "\x1bPabc"}, key(KeyUp, 0)}},
	{"string then esc, after a flush", "\x1b]0;x\x1b", []Event{Unknown{Raw: "\x1b]0;x"}, key(KeyEscape, 0)}},
	{"focus with parameters", "\x1b[1I", []Event{Unknown{Raw: "\x1b[1I"}}},
	// A number too long to hold is cut short, not wrapped around.
	{"huge parameter", "\x1b[99999999999999999999A", []Event{key(KeyUp, Mod(254))}},
}

func TestDecode(t *testing.T) {
	for _, c := range decodeCases {
		t.Run(c.name, func(t *testing.T) {
			got := decodeAll(c.input)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("input %q\n got %s\nwant %s", c.input, describe(got), describe(c.want))
			}
		})
	}
}

// Every input gives the same events however it is cut into reads: at each
// single byte position, and one byte at a time.
func TestDecodeSplitAnywhere(t *testing.T) {
	for _, c := range decodeCases {
		whole := decodeAll(c.input)
		for cut := 1; cut < len(c.input); cut++ {
			var d Decoder
			got := d.Feed([]byte(c.input[:cut]))
			got = append(got, d.Feed([]byte(c.input[cut:]))...)
			got = append(got, d.Flush()...)
			if !reflect.DeepEqual(got, whole) {
				t.Errorf("%s: input %q cut at %d\n got %s\nwant %s", c.name, c.input, cut, describe(got), describe(whole))
			}
		}
		var d Decoder
		var got []Event
		for i := 0; i < len(c.input); i++ {
			got = append(got, d.Feed([]byte{c.input[i]})...)
		}
		got = append(got, d.Flush()...)
		if !reflect.DeepEqual(got, whole) {
			t.Errorf("%s: input %q fed a byte at a time\n got %s\nwant %s", c.name, c.input, describe(got), describe(whole))
		}
	}
}

package term

import (
	"fmt"
	"strings"
)

// Event is one thing the terminal told gnosh: a key, pasted text, a change
// of focus or size, an answer to a question gnosh asked, or bytes that made
// no sense. The concrete types are Key, KeyRelease, Paste, Focus, Mouse,
// Reply, Unknown, Resize and Resume.
type Event interface {
	// String describes the event for logs and failed tests.
	String() string
	isEvent()
}

// Mod is the set of modifier keys held down with a key.
type Mod uint16

// The modifiers a terminal can report. Most terminals report only Ctrl and
// Alt with the classic encodings; Shift arrives with function keys, and the
// rest only under the kitty keyboard protocol.
const (
	ModShift Mod = 1 << iota
	ModAlt
	ModCtrl
	// ModSuper is the Command or Windows key. xterm's documents call the
	// same bit Meta.
	ModSuper
	ModHyper
	ModMeta
	ModCapsLock
	ModNumLock
)

// String lists the modifiers in a fixed order, joined by "+", such as
// "ctrl+alt". It is empty when no modifier is set.
func (m Mod) String() string {
	var parts []string
	for _, n := range []struct {
		bit  Mod
		name string
	}{
		{ModCtrl, "ctrl"}, {ModAlt, "alt"}, {ModShift, "shift"},
		{ModSuper, "super"}, {ModHyper, "hyper"}, {ModMeta, "meta"},
		{ModCapsLock, "capslock"}, {ModNumLock, "numlock"},
	} {
		if m&n.bit != 0 {
			parts = append(parts, n.name)
		}
	}
	return strings.Join(parts, "+")
}

// KeyCode names a key. KeyRune stands for every key that is a character;
// the character itself is in Key.Rune.
type KeyCode int

// The keys gnosh knows by name.
const (
	// KeyRune is a character key: a letter, a digit, punctuation, space.
	KeyRune KeyCode = iota
	KeyEnter
	KeyTab
	KeyBackspace
	KeyEscape
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyInsert
	KeyDelete
	// KeyBegin is the middle key of the number pad (5) with Num Lock off.
	KeyBegin
	KeyF1
	KeyF2
	KeyF3
	KeyF4
	KeyF5
	KeyF6
	KeyF7
	KeyF8
	KeyF9
	KeyF10
	KeyF11
	KeyF12
	KeyF13
	KeyF14
	KeyF15
	KeyF16
	KeyF17
	KeyF18
	KeyF19
	KeyF20
	KeyF21
	KeyF22
	KeyF23
	KeyF24
	KeyF25
	KeyF26
	KeyF27
	KeyF28
	KeyF29
	KeyF30
	KeyF31
	KeyF32
	KeyF33
	KeyF34
	KeyF35
	KeyCapsLock
	KeyScrollLock
	KeyNumLock
	KeyPrintScreen
	KeyPause
	KeyMenu
	// KeyMedia is any media key (play, volume and so on). Key.Rune holds
	// the kitty protocol's number for it.
	KeyMedia
	// KeyModifier is a modifier key pressed on its own (Shift, Ctrl and so
	// on). Key.Rune holds the kitty protocol's number for it.
	KeyModifier
)

var keyNames = map[KeyCode]string{
	KeyEnter: "enter", KeyTab: "tab", KeyBackspace: "backspace",
	KeyEscape: "esc", KeyUp: "up", KeyDown: "down", KeyLeft: "left",
	KeyRight: "right", KeyHome: "home", KeyEnd: "end", KeyPageUp: "pgup",
	KeyPageDown: "pgdn", KeyInsert: "insert", KeyDelete: "delete",
	KeyBegin: "begin", KeyCapsLock: "capslock", KeyScrollLock: "scrolllock",
	KeyNumLock: "numlock", KeyPrintScreen: "printscreen", KeyPause: "pause",
	KeyMenu: "menu", KeyMedia: "media", KeyModifier: "modifier",
}

// String names the key code: "enter", "f5", "rune".
func (c KeyCode) String() string {
	if c == KeyRune {
		return "rune"
	}
	if c >= KeyF1 && c <= KeyF35 {
		return fmt.Sprintf("f%d", int(c-KeyF1)+1)
	}
	if n, ok := keyNames[c]; ok {
		return n
	}
	return fmt.Sprintf("key(%d)", int(c))
}

// Key is a key press.
//
// A character key has Code KeyRune and the character in Rune. When the key
// was typed as plain text, Text holds what to insert; when it came with
// Ctrl or Alt, Text is empty and Rune is the key that was held with them
// (Ctrl+C is Rune 'c' with ModCtrl). Every other key has its own Code.
//
// Terminals that speak only the classic encodings cannot tell some keys
// apart, and the decoder reports the dedicated key: byte 0x09 is Tab (never
// Ctrl+I), 0x0d is Enter (never Ctrl+M), 0x7f and 0x08 are both Backspace,
// and 0x0a is Ctrl+J, which is what a keyboard sends it for. Under the kitty
// keyboard protocol each of these arrives as itself.
type Key struct {
	// Code is the key.
	Code KeyCode
	// Rune is the character of a KeyRune key. With modifiers it is the key
	// as the terminal names it: lower case for a letter held with Ctrl,
	// and for a letter held with Alt and Shift.
	Rune rune
	// Text is what the key types, or empty when it types nothing.
	Text string
	// Mod is the modifiers held with the key.
	Mod Mod
	// Repeat is true when the terminal said the key is being held down.
	// Only the kitty protocol's event reports say so.
	Repeat bool
}

func (Key) isEvent() {}

// String describes the key the way a person would say it: "ctrl+c",
// "shift+enter", "alt+up", "a".
func (k Key) String() string {
	name := k.Code.String()
	if k.Code == KeyRune {
		switch {
		case k.Rune == ' ':
			name = "space"
		case k.Rune == 0 && k.Text != "":
			name = k.Text
		default:
			name = string(k.Rune)
		}
	}
	if k.Mod != 0 {
		name = k.Mod.String() + "+" + name
	}
	if k.Repeat {
		name += " (repeat)"
	}
	return name
}

// Is reports whether the key is the character r held with exactly the
// modifiers mod, ignoring the lock keys.
func (k Key) Is(r rune, mod Mod) bool {
	return k.Code == KeyRune && k.Rune == r && k.Mod&^(ModCapsLock|ModNumLock) == mod
}

// IsKey reports whether the key is the named key code held with exactly
// the modifiers mod, ignoring the lock keys.
func (k Key) IsKey(code KeyCode, mod Mod) bool {
	return k.Code == code && k.Mod&^(ModCapsLock|ModNumLock) == mod
}

// KeyRelease is a key being let go. Terminals send it only when asked to
// (the kitty protocol's "report event types"), which gnosh never asks for;
// it is its own type so that a stray one is never taken for a press.
type KeyRelease struct {
	Key Key
}

func (KeyRelease) isEvent() {}

// String describes the release.
func (k KeyRelease) String() string { return "release " + k.Key.String() }

// Paste is text the person pasted, delivered whole. The terminal marks the
// start and end of a paste when bracketed paste is on.
type Paste struct {
	// Text is the pasted text. Line ends are "\n" whatever the terminal
	// sent, and bytes that are not UTF-8 are replaced with U+FFFD. It may
	// hold control characters and escape sequences that were on the
	// clipboard; whoever shows it must make them harmless.
	Text string
	// Truncated is true when the paste never ended: the terminal stopped
	// sending, and the person pressed Ctrl+C to get the keyboard back.
	Truncated bool
}

func (Paste) isEvent() {}

// String describes the paste.
func (p Paste) String() string {
	if p.Truncated {
		return fmt.Sprintf("paste %q (truncated)", p.Text)
	}
	return fmt.Sprintf("paste %q", p.Text)
}

// Focus says the terminal window gained or lost the keyboard. Terminals
// send it only while focus reporting is on.
type Focus struct {
	// In is true when the window gained the keyboard.
	In bool
}

func (Focus) isEvent() {}

// String describes the change of focus.
func (f Focus) String() string {
	if f.In {
		return "focus in"
	}
	return "focus out"
}

// MouseAction says what the mouse did.
type MouseAction int

// The things a mouse report can say.
const (
	MousePress MouseAction = iota
	MouseRelease
	MouseMotion
	MouseWheelUp
	MouseWheelDown
	MouseWheelLeft
	MouseWheelRight
)

// Mouse is a mouse report. gnosh never turns mouse reporting on, so one
// arrives only when another program left it on; it is decoded so that its
// bytes are never taken for typing.
type Mouse struct {
	// Action is what happened.
	Action MouseAction
	// Button is 1 for left, 2 for middle, 3 for right, and 0 when no
	// button is involved or the report does not say which.
	Button int
	// Col and Row are the cell under the pointer, counted from 1.
	Col, Row int
	// Mod is the modifiers held: Shift, Alt and Ctrl at most.
	Mod Mod
}

func (Mouse) isEvent() {}

// String describes the mouse report.
func (m Mouse) String() string {
	action := [...]string{"press", "release", "motion", "wheel up", "wheel down", "wheel left", "wheel right"}[m.Action]
	s := fmt.Sprintf("mouse %s button %d at %d,%d", action, m.Button, m.Col, m.Row)
	if m.Mod != 0 {
		s += " " + m.Mod.String()
	}
	return s
}

// ReplyKind says which question a Reply answers.
type ReplyKind int

// The answers gnosh understands.
const (
	// ReplyCursorPosition answers QueryCursorPosition: Row and Col.
	ReplyCursorPosition ReplyKind = iota
	// ReplyDeviceAttributes answers QueryDeviceAttributes: Params.
	ReplyDeviceAttributes
	// ReplySecondaryAttributes answers the secondary device attributes
	// question (CSI > c): Params.
	ReplySecondaryAttributes
	// ReplyKittyKeyboard answers QueryKittyKeyboard: Flags.
	ReplyKittyKeyboard
	// ReplyMode answers a mode question such as QuerySynchronizedOutput:
	// Mode, Private and State.
	ReplyMode
	// ReplyString is a string answer (OSC, DCS, APC, PM or SOS): Intro is
	// the byte after ESC that began it and Text is its body.
	ReplyString
)

// ModeState is what a terminal says about a mode (DECRPM).
type ModeState int

// The states a mode report can give.
const (
	ModeNotRecognized ModeState = iota
	ModeSet
	ModeReset
	ModePermanentlySet
	ModePermanentlyReset
)

// Reply is the terminal's answer to a question gnosh asked.
type Reply struct {
	// Kind says which fields are filled in.
	Kind ReplyKind
	// Row and Col are the cursor's place, counted from 1.
	Row, Col int
	// Params are the numbers of a device attributes answer.
	Params []int
	// Flags are the kitty keyboard flags in force.
	Flags int
	// Mode is the mode a mode report is about; Private is true for a DEC
	// private mode (the kind written with "?"); State is what the terminal
	// said about it.
	Mode    int
	Private bool
	State   ModeState
	// Intro and Text are the opening byte and body of a string answer.
	Intro byte
	Text  string
}

func (Reply) isEvent() {}

// String describes the reply.
func (r Reply) String() string {
	switch r.Kind {
	case ReplyCursorPosition:
		return fmt.Sprintf("reply cursor at row %d col %d", r.Row, r.Col)
	case ReplyDeviceAttributes:
		return fmt.Sprintf("reply device attributes %v", r.Params)
	case ReplySecondaryAttributes:
		return fmt.Sprintf("reply secondary attributes %v", r.Params)
	case ReplyKittyKeyboard:
		return fmt.Sprintf("reply kitty keyboard flags %d", r.Flags)
	case ReplyMode:
		q := ""
		if r.Private {
			q = "?"
		}
		return fmt.Sprintf("reply mode %s%d state %d", q, r.Mode, int(r.State))
	case ReplyString:
		return fmt.Sprintf("reply string ESC %c %q", r.Intro, r.Text)
	}
	return fmt.Sprintf("reply kind %d", int(r.Kind))
}

// Unknown is input the decoder could not make sense of: a control sequence
// it has no meaning for, a broken one, or bytes that are not UTF-8. The
// bytes are consumed whole and never reach the text.
type Unknown struct {
	// Raw is the bytes as they arrived.
	Raw string
}

func (Unknown) isEvent() {}

// String shows the bytes.
func (u Unknown) String() string { return fmt.Sprintf("unknown %q", u.Raw) }

// Resize says the terminal's size changed.
type Resize struct {
	// Cols and Rows are the new size in character cells.
	Cols, Rows int
}

func (Resize) isEvent() {}

// String describes the new size.
func (r Resize) String() string { return fmt.Sprintf("resize %dx%d", r.Cols, r.Rows) }

// Resume says the terminal is gnosh's again after the process was stopped
// and continued, or after Terminal.Resume. Whatever the interface had on
// screen may be gone and the modes it set may have been undone, so it sets
// them again and redraws.
type Resume struct{}

func (Resume) isEvent() {}

// String names the event.
func (Resume) String() string { return "resume" }

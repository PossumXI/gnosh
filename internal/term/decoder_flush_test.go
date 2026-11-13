package term

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func wantEvents(t *testing.T, what string, got []Event, want ...Event) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n got %s\nwant %s", what, describe(got), describe(want))
	}
}

// A lone ESC is held until Flush says nothing followed it.
func TestFlushLoneEscape(t *testing.T) {
	var d Decoder
	wantEvents(t, "feed ESC", d.Feed([]byte{0x1b}))
	if !d.Pending() {
		t.Error("Pending is false with a lone ESC held back")
	}
	wantEvents(t, "flush", d.Flush(), key(KeyEscape, 0))
	if d.Pending() {
		t.Error("Pending is true after the flush")
	}
	wantEvents(t, "second flush", d.Flush())
	wantEvents(t, "typing after Esc", d.Feed([]byte("a")), txt('a'))
}

// ESC followed in time by the rest of a sequence is that sequence.
func TestEscapeCompletedBeforeFlush(t *testing.T) {
	var d Decoder
	wantEvents(t, "feed ESC", d.Feed([]byte{0x1b}))
	wantEvents(t, "feed [", d.Feed([]byte("[")))
	if !d.Pending() {
		t.Error("Pending is false with ESC [ held back (it may be Alt+[)")
	}
	wantEvents(t, "feed A", d.Feed([]byte("A")), key(KeyUp, 0))
	if d.Pending() {
		t.Error("Pending is true with nothing held back")
	}
}

// Flush settles only what one key press could have sent. A sequence cut
// in half by a slow connection is still read whole when its tail arrives,
// so the tail is never typed as text.
func TestFlushLeavesHalfSequences(t *testing.T) {
	cases := []struct {
		name, head, tail string
		want             Event
	}{
		{"CSI with parameters", "\x1b[1;", "5A", key(KeyUp, ModCtrl)},
		{"CSI kitty key", "\x1b[13", ";2u", key(KeyEnter, ModShift)},
		{"SS3 with parameters", "\x1bO1;", "5P", key(KeyF1, ModCtrl)},
		{"UTF-8 character", "\xe4\xb8", "\x96", txt('世')},
		{"alt with a UTF-8 character", "\x1b\xc3", "\xa9", chr('é', ModAlt)},
		{"OSC answer", "\x1b]11;rgb:00", "00/0000/0000\x07", Reply{Kind: ReplyString, Intro: ']', Text: "11;rgb:0000/0000/0000"}},
		{"X10 mouse report", "\x1b[M !", "!", Mouse{Action: MousePress, Button: 1, Col: 1, Row: 1}},
		{"linux console key", "\x1b[[", "B", key(KeyF2, 0)},
		{"mode report", "\x1b[?2026;2$", "y", Reply{Kind: ReplyMode, Mode: 2026, Private: true, State: ModeReset}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var d Decoder
			wantEvents(t, "head", d.Feed([]byte(c.head)))
			if d.Pending() {
				t.Error("Pending is true for a half sequence that Flush cannot settle")
			}
			wantEvents(t, "flush", d.Flush())
			wantEvents(t, "tail", d.Feed([]byte(c.tail)), c.want)
		})
	}
}

// A paste may arrive over many reads with pauses between them.
func TestPasteAcrossReadsAndFlushes(t *testing.T) {
	var d Decoder
	wantEvents(t, "start", d.Feed([]byte("\x1b[200~abc")))
	if !d.InPaste() {
		t.Fatal("InPaste is false after the start mark")
	}
	if !d.Pending() {
		t.Error("Pending is false in an open paste that has not gone quiet")
	}
	wantEvents(t, "flush in a paste", d.Flush())
	if d.Pending() {
		t.Error("Pending is true after the flush in a paste")
	}
	wantEvents(t, "middle", d.Feed([]byte("def\x1b[20")))
	wantEvents(t, "second flush", d.Flush())
	wantEvents(t, "end", d.Feed([]byte("1~x")), Paste{Text: "abcdef"}, txt('x'))
	if d.InPaste() {
		t.Error("InPaste is true after the end mark")
	}
}

// Ctrl+C inside a paste is part of the paste.
func TestPasteHoldsControlC(t *testing.T) {
	var d Decoder
	got := d.Feed([]byte("\x1b[200~a\x03b"))
	got = append(got, d.Feed([]byte("\x03\x1b[201~"))...)
	wantEvents(t, "paste", got, Paste{Text: "a\x03b\x03"})
}

// A paste whose end never comes can be abandoned: after a pause, Ctrl+C
// gives the keyboard back.
func TestPasteStalledThenControlC(t *testing.T) {
	for _, interrupt := range []string{"\x03", "\x1b[99;5u", "\x1b[99;133u"} {
		var d Decoder
		wantEvents(t, "start", d.Feed([]byte("\x1b[200~partial\r\ntext")))
		wantEvents(t, "flush", d.Flush())
		got := d.Feed([]byte(interrupt + "x"))
		k := chr('c', ModCtrl)
		if interrupt == "\x1b[99;133u" {
			k.Mod |= ModNumLock
		}
		wantEvents(t, "interrupt "+interrupt, got, Paste{Text: "partial\ntext", Truncated: true}, k, txt('x'))
		if d.InPaste() {
			t.Error("InPaste is true after the paste was abandoned")
		}
	}
	// Anything else after the pause is more of the paste.
	var d Decoder
	d.Feed([]byte("\x1b[200~abc"))
	d.Flush()
	wantEvents(t, "more paste", d.Feed([]byte("d\x03")))
	wantEvents(t, "end", d.Feed([]byte("\x1b[201~")), Paste{Text: "abcd\x03"})
}

// A megabyte pasted in small reads comes out whole.
func TestLargePaste(t *testing.T) {
	body := strings.Repeat("0123456789abcdef", 1<<16)
	input := []byte("\x1b[200~" + body + "\x1b[201~")
	var d Decoder
	var got []Event
	for len(input) > 0 {
		n := min(len(input), 1021)
		got = append(got, d.Feed(input[:n])...)
		input = input[n:]
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	if p, ok := got[0].(Paste); !ok || p.Text != body {
		t.Errorf("the paste did not come out whole")
	}
	if cap(d.buf) > 64<<10 {
		t.Errorf("the decoder kept a %d byte buffer after the paste", cap(d.buf))
	}
}

// CSI 1 ; n R is F3 with modifiers, unless a cursor position was asked for.
func TestCursorReportOrF3(t *testing.T) {
	var d Decoder
	wantEvents(t, "no question asked", d.Feed([]byte("\x1b[1;5R")), key(KeyF3, ModCtrl))
	d.ExpectCursorPosition()
	wantEvents(t, "question asked", d.Feed([]byte("\x1b[1;5R")), Reply{Kind: ReplyCursorPosition, Row: 1, Col: 5})
	wantEvents(t, "answer used up", d.Feed([]byte("\x1b[1;5R")), key(KeyF3, ModCtrl))
	wantEvents(t, "row 2 is never a key", d.Feed([]byte("\x1b[2;5R")), Reply{Kind: ReplyCursorPosition, Row: 2, Col: 5})
}

func TestReset(t *testing.T) {
	var d Decoder
	d.Feed([]byte("\x1b[200~abc"))
	d.ExpectCursorPosition()
	d.Reset()
	if d.InPaste() || d.Pending() {
		t.Error("Reset left the decoder in the middle of something")
	}
	wantEvents(t, "after reset", d.Feed([]byte("a\x1b[1;5R")), txt('a'), key(KeyF3, ModCtrl))
}

func TestKeyStrings(t *testing.T) {
	cases := map[string]Event{
		"ctrl+c":             chr('c', ModCtrl),
		"shift+enter":        key(KeyEnter, ModShift),
		"ctrl+alt+shift+f12": key(KeyF12, ModCtrl|ModAlt|ModShift),
		"space":              txt(' '),
		"a":                  txt('a'),
		"alt+up":             key(KeyUp, ModAlt),
		"release f35":        KeyRelease{Key: key(KeyF35, 0)},
		"focus in":           Focus{In: true},
		"resize 80x24":       Resize{Cols: 80, Rows: 24},
		`paste "x"`:          Paste{Text: "x"},
		`unknown "\x1b[5n"`:  Unknown{Raw: "\x1b[5n"},
	}
	for want, ev := range cases {
		if got := ev.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
	if !chr('c', ModCtrl|ModNumLock).Is('c', ModCtrl) || chr('c', ModCtrl|ModAlt).Is('c', ModCtrl) {
		t.Error("Key.Is does not compare modifiers as documented")
	}
	if !key(KeyEnter, ModShift|ModCapsLock).IsKey(KeyEnter, ModShift) || key(KeyEnter, 0).IsKey(KeyTab, 0) {
		t.Error("Key.IsKey does not compare as documented")
	}
}

// checkEvents fails if any event could put a control character into text.
func checkEvents(t *testing.T, evs []Event) {
	t.Helper()
	for _, ev := range evs {
		k, ok := ev.(Key)
		if r, isRelease := ev.(KeyRelease); isRelease {
			k, ok = r.Key, true
		}
		if !ok {
			if _, internal := ev.(pasteStarted); internal {
				t.Errorf("the decoder's own paste mark escaped")
			}
			continue
		}
		if !utf8.ValidString(k.Text) {
			t.Errorf("key %v has text that is not UTF-8: %q", k, k.Text)
		}
		for _, r := range k.Text {
			if unicode.IsControl(r) {
				t.Errorf("key %v has a control character in its text: %q", k, k.Text)
			}
		}
		if k.Code == KeyRune && (unicode.IsControl(k.Rune) || !utf8.ValidRune(k.Rune)) {
			t.Errorf("key %v has a control character or invalid rune as its character: %U", k, k.Rune)
		}
	}
}

// FuzzDecoder checks, for any bytes: the decoder does not panic; cutting
// the input in two changes nothing; no control character becomes text; and
// the decoder always comes back to rest (an end-of-paste mark and a
// control key end whatever was open, and the control key arrives as
// itself).
func FuzzDecoder(f *testing.F) {
	for _, c := range decodeCases {
		f.Add([]byte(c.input), uint16(len(c.input)/2))
	}
	f.Add([]byte("\x1b[200~never ended"), uint16(3))
	f.Add([]byte("\x1b]never ended"), uint16(1))
	f.Add([]byte("\x1b[M"), uint16(1))
	f.Add([]byte("\x1b\x1b\x1b[1;2"), uint16(2))
	f.Add([]byte("\x1b[11$ 0000"), uint16(7))
	f.Fuzz(func(t *testing.T, data []byte, cut uint16) {
		var whole Decoder
		a := whole.Feed(data)
		a = append(a, whole.Flush()...)

		var split Decoder
		i := int(cut) % (len(data) + 1)
		b := split.Feed(data[:i])
		b = append(b, split.Feed(data[i:])...)
		b = append(b, split.Flush()...)
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("input %q cut at %d\n whole %s\n split %s", data, i, describe(a), describe(b))
		}
		checkEvents(t, a)

		tail := whole.Feed([]byte(pasteEndSeq + "\r"))
		checkEvents(t, tail)
		if len(tail) == 0 || !reflect.DeepEqual(tail[len(tail)-1], Event(key(KeyEnter, 0))) {
			t.Fatalf("input %q: Enter after it came out as %s", data, describe(tail))
		}
		if whole.InPaste() || whole.Pending() || len(whole.buf) != 0 {
			t.Fatalf("input %q: the decoder is not at rest: %d bytes held, in paste %v", data, len(whole.buf), whole.InPaste())
		}

		// Every byte is accounted for: the events of a byte-at-a-time
		// feed are the same again.
		var slow Decoder
		var c []Event
		for _, x := range data {
			c = append(c, slow.Feed([]byte{x})...)
		}
		c = append(c, slow.Flush()...)
		if !reflect.DeepEqual(a, c) {
			t.Fatalf("input %q a byte at a time\n whole %s\n bytes %s", data, describe(a), describe(c))
		}
	})
}

// Text typed as plain bytes comes back as the same text.
func FuzzDecoderText(f *testing.F) {
	f.Add("hello, world")
	f.Add("héllo 世界 😀")
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			t.Skip()
		}
		for _, r := range s {
			if unicode.IsControl(r) {
				t.Skip()
			}
		}
		var got bytes.Buffer
		for _, ev := range decodeAll(s) {
			k, ok := ev.(Key)
			if !ok {
				t.Fatalf("text %q gave %s", s, ev)
			}
			got.WriteString(k.Text)
		}
		if got.String() != s {
			t.Fatalf("text %q came back as %q", s, got.String())
		}
	})
}

package text

import "testing"

// sequenceAfter applies SGR parameters to the default state and returns the
// sequence that restores the result.
func sequenceAfter(params ...string) string {
	var st sgrState
	for _, p := range params {
		st.apply(p)
	}
	return string(st.appendSequence(nil))
}

func TestSGRState(t *testing.T) {
	tests := []struct {
		name   string
		params []string // the parameters of one SGR sequence after another
		want   string   // the sequence that restores the state they leave
	}{
		{"nothing", nil, ""},
		{"reset", []string{"0"}, ""},
		{"empty is reset", []string{""}, ""},
		{"bold", []string{"1"}, "\x1b[1m"},
		{"every attribute", []string{"1;2;3;4;5;6;7;8;9;53"}, "\x1b[1;2;3;4;5;6;7;8;9;53m"},
		{"written in a fixed order", []string{"9;44;1;31;4"}, "\x1b[1;4;9;31;44m"},
		{"one sequence after another", []string{"1", "31", "4"}, "\x1b[1;4;31m"},

		// Underline styles.
		{"underline", []string{"4"}, "\x1b[4m"},
		{"underline single", []string{"4:1"}, "\x1b[4m"},
		{"underline double", []string{"4:2"}, "\x1b[4:2m"},
		{"underline curly", []string{"4:3"}, "\x1b[4:3m"},
		{"underline dotted", []string{"4:4"}, "\x1b[4:4m"},
		{"underline dashed", []string{"4:5"}, "\x1b[4:5m"},
		{"underline off by style", []string{"4", "4:0"}, ""},
		{"underline unknown style", []string{"4:9"}, "\x1b[4m"},
		{"double underline, old form", []string{"21"}, "\x1b[21m"},
		{"underline replaced", []string{"21;4:3"}, "\x1b[4:3m"},

		// Colours.
		{"foreground", []string{"31"}, "\x1b[31m"},
		{"bright foreground", []string{"97"}, "\x1b[97m"},
		{"background", []string{"44"}, "\x1b[44m"},
		{"bright background", []string{"100"}, "\x1b[100m"},
		{"palette foreground", []string{"38;5;206"}, "\x1b[38;5;206m"},
		{"palette background", []string{"48;5;17"}, "\x1b[48;5;17m"},
		{"rgb foreground", []string{"38;2;255;95;215"}, "\x1b[38;2;255;95;215m"},
		{"rgb background", []string{"48;2;1;2;3"}, "\x1b[48;2;1;2;3m"},
		{"palette with colons", []string{"38:5:206"}, "\x1b[38;5;206m"},
		{"rgb with colons", []string{"38:2::255:95:215"}, "\x1b[38;2;255;95;215m"},
		{"rgb with colons, no colour space", []string{"38:2:255:95:215"}, "\x1b[38;2;255;95;215m"},
		{"rgb with colons, colour space given", []string{"48:2:0:1:2:3"}, "\x1b[48;2;1;2;3m"},
		{"rgb with colons and trailing fields", []string{"38:2::1:2:3::"}, "\x1b[38;2;1;2;3m"},
		{"rgb with empty fields", []string{"38;2;;5;"}, "\x1b[38;2;0;5;0m"},
		{"colour then attribute", []string{"38;5;1;1"}, "\x1b[1;38;5;1m"},
		{"both colours and attributes", []string{"1;38;2;1;2;3;48;5;4;3"}, "\x1b[1;3;38;2;1;2;3;48;5;4m"},
		{"last colour wins", []string{"31;32;38;5;7;34"}, "\x1b[34m"},
		{"underline colour, palette", []string{"58:5:9"}, "\x1b[58:5:9m"},
		{"underline colour, rgb", []string{"58:2::1:2:3"}, "\x1b[58:2::1:2:3m"},
		{"underline colour, semicolons", []string{"4;58;2;1;2;3"}, "\x1b[4;58:2::1:2:3m"},
		{"underline colour off", []string{"58:5:9;59"}, ""},
		{"overline off", []string{"53;55"}, ""},

		// Turning things off.
		{"reset at the end", []string{"1;31;0"}, ""},
		{"reset in the middle", []string{"1;0;31"}, "\x1b[31m"},
		{"empty parameter in the middle", []string{"1;;31"}, "\x1b[31m"},
		{"trailing semicolon", []string{"1;"}, ""},
		{"reset in a later sequence", []string{"1;31", ""}, ""},
		{"normal intensity", []string{"1;2;3;22"}, "\x1b[3m"},
		{"italic off", []string{"1;3;23"}, "\x1b[1m"},
		{"underline off", []string{"4:3;1;24"}, "\x1b[1m"},
		{"blink off", []string{"5;6;1;25"}, "\x1b[1m"},
		{"reverse off", []string{"7;1;27"}, "\x1b[1m"},
		{"conceal off", []string{"8;1;28"}, "\x1b[1m"},
		{"strike off", []string{"9;1;29"}, "\x1b[1m"},
		{"default foreground", []string{"31;44;39"}, "\x1b[44m"},
		{"default background", []string{"31;44;49"}, "\x1b[31m"},

		// Things that cannot be read change nothing.
		{"colour without a mode", []string{"1;38"}, "\x1b[1m"},
		{"palette without a number", []string{"1;38;5"}, "\x1b[1m"},
		{"rgb cut short", []string{"1;38;2;1;2"}, "\x1b[1m"},
		{"palette number too large", []string{"38;5;300"}, ""},
		{"rgb number too large", []string{"38;2;300;0;0;1"}, "\x1b[1m"},
		{"unknown colour mode", []string{"38;9;1"}, "\x1b[1m"},
		{"colons cut short", []string{"38:2:1:2;1"}, "\x1b[1m"},
		{"colons without a number", []string{"38:5;1"}, "\x1b[1m"},
		{"untracked attributes", []string{"10;11;20;51;52;73;1"}, "\x1b[1m"},
		{"sub-parameters where none belong", []string{"1:2"}, "\x1b[1m"},
		{"a colon alone", []string{"1", ":"}, ""},
		{"a number too long to matter", []string{"99999999999999999999999;1"}, "\x1b[1m"},
	}
	for _, tt := range tests {
		if got := sequenceAfter(tt.params...); got != tt.want {
			t.Errorf("%s: after %q the state is %q, want %q", tt.name, tt.params, got, tt.want)
		}
		// Writing a state and reading it back gives the same state.
		var first, second sgrState
		for _, p := range tt.params {
			first.apply(p)
		}
		if tt.want != "" {
			second.apply(tt.want[2 : len(tt.want)-1])
		}
		if first != second {
			t.Errorf("%s: reading %q back gives %+v, want %+v", tt.name, tt.want, second, first)
		}
	}
}

func TestHyperlinkOf(t *testing.T) {
	tests := []struct {
		in   string
		link string
		ok   bool
	}{
		{"\x1b]8;;https://example.com\x1b\\", ";https://example.com", true},
		{"\x1b]8;;https://example.com\x07", ";https://example.com", true},
		{"\x1b]8;id=a:b=c;https://example.com/x;y\x1b\\", "id=a:b=c;https://example.com/x;y", true},
		{"\x1b]8;;\x1b\\", "", true},
		{"\x1b]8;id=a;\x07", "", true},
		{"\x1b]8;\x07", "", false},
		{"\x1b]8\x07", "", false},
		{"\x1b]0;title\x07", "", false},
		{"\x1b]\x07", "", false},
	}
	for _, tt := range tests {
		if link, ok := hyperlinkOf(tt.in); link != tt.link || ok != tt.ok {
			t.Errorf("hyperlinkOf(%q) = %q, %t; want %q, %t", tt.in, link, ok, tt.link, tt.ok)
		}
	}
}

func TestLook(t *testing.T) {
	var l look
	l.applyText("plain \x1b[1;31mred \x1b]8;;https://example.com\x1b\\link \x1b[?25l\x1b[31")
	if got, want := string(l.appendOpen(nil)), "\x1b[1;31m\x1b]8;;https://example.com\x1b\\"; got != want {
		t.Errorf("appendOpen gives %q, want %q", got, want)
	}
	if got, want := string(l.appendClose(nil)), "\x1b]8;;\x1b\\\x1b[0m"; got != want {
		t.Errorf("appendClose gives %q, want %q", got, want)
	}
	if l.plain() {
		t.Error("a look with a style and a link claims to be plain")
	}

	var bold, red look
	bold.applyText("\x1b[1m")
	red.applyText("\x1b[31m\x1b]8;;https://example.com/a\x07")
	changes := []struct {
		name     string
		from, to look
		want     string
	}{
		{"no change", bold, bold, ""},
		{"nothing to nothing", look{}, look{}, ""},
		{"from nothing", look{}, bold, "\x1b[1m"},
		{"to nothing", bold, look{}, "\x1b[0m"},
		{"style to style with a link", bold, red, "\x1b]8;;https://example.com/a\x1b\\\x1b[0m\x1b[31m"},
		{"link closed", red, bold, "\x1b]8;;\x1b\\\x1b[0m\x1b[1m"},
		{"link to link", red, l, "\x1b]8;;\x1b\\\x1b]8;;https://example.com\x1b\\\x1b[0m\x1b[1;31m"},
	}
	for _, tt := range changes {
		if got := string(tt.from.appendChange(nil, tt.to)); got != tt.want {
			t.Errorf("%s: appendChange gives %q, want %q", tt.name, got, tt.want)
		}
	}
}

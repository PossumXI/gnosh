package text

import (
	"slices"
	"strings"
	"testing"
)

// tok is shorthand for the tests below.
func tok(kind Kind, text string, complete bool) Token {
	return Token{Kind: kind, Text: text, Complete: complete}
}

func TestTokens(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Token
	}{
		{"empty", "", nil},
		{"plain", "plain text\n\ttabbed", []Token{tok(Plain, "plain text\n\ttabbed", true)}},

		// Control sequences.
		{"sgr", "\x1b[31m", []Token{tok(CSI, "\x1b[31m", true)}},
		{"sgr reset without parameters", "\x1b[m", []Token{tok(CSI, "\x1b[m", true)}},
		{"sgr around text", "a\x1b[1;31mb\x1b[0mc", []Token{
			tok(Plain, "a", true), tok(CSI, "\x1b[1;31m", true), tok(Plain, "b", true), tok(CSI, "\x1b[0m", true), tok(Plain, "c", true),
		}},
		{"sgr with colons", "\x1b[38:2::255:0:0m", []Token{tok(CSI, "\x1b[38:2::255:0:0m", true)}},
		{"private mode", "\x1b[?25l", []Token{tok(CSI, "\x1b[?25l", true)}},
		{"cursor position", "\x1b[12;40H", []Token{tok(CSI, "\x1b[12;40H", true)}},
		{"intermediate byte", "\x1b[2 q", []Token{tok(CSI, "\x1b[2 q", true)}},
		{"mouse report", "\x1b[<0;10;20M", []Token{tok(CSI, "\x1b[<0;10;20M", true)}},
		{"shortest", "\x1b[K", []Token{tok(CSI, "\x1b[K", true)}},
		{"final byte tilde", "\x1b[3~x", []Token{tok(CSI, "\x1b[3~", true), tok(Plain, "x", true)}},
		{"parameters after an intermediate", "\x1b[1 2mx", []Token{tok(CSI, "\x1b[1 2m", true), tok(Plain, "x", true)}},
		{"back to back", "\x1b[1m\x1b[31m", []Token{tok(CSI, "\x1b[1m", true), tok(CSI, "\x1b[31m", true)}},

		// Operating system commands.
		{"title with bel", "\x1b]0;title\x07x", []Token{tok(OSC, "\x1b]0;title\x07", true), tok(Plain, "x", true)}},
		{"title with st", "\x1b]2;title\x1b\\x", []Token{tok(OSC, "\x1b]2;title\x1b\\", true), tok(Plain, "x", true)}},
		{"hyperlink", "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\", []Token{
			tok(OSC, "\x1b]8;;https://example.com\x1b\\", true), tok(Plain, "link", true), tok(OSC, "\x1b]8;;\x1b\\", true),
		}},
		{"utf-8 payload", "\x1b]0;héllo 漢\x07", []Token{tok(OSC, "\x1b]0;héllo 漢\x07", true)}},
		{"newline in a payload", "\x1b]0;a\nb\x07c", []Token{tok(OSC, "\x1b]0;a\nb\x07", true), tok(Plain, "c", true)}},
		{"empty osc", "\x1b]\x07", []Token{tok(OSC, "\x1b]\x07", true)}},

		// The other strings.
		{"dcs", "\x1bP1$r0m\x1b\\x", []Token{tok(DCS, "\x1bP1$r0m\x1b\\", true), tok(Plain, "x", true)}},
		{"sos", "\x1bXhello\x1b\\", []Token{tok(SOS, "\x1bXhello\x1b\\", true)}},
		{"pm", "\x1b^hello\x1b\\", []Token{tok(PM, "\x1b^hello\x1b\\", true)}},
		{"apc", "\x1b_Gf=24;AAAA\x1b\\", []Token{tok(APC, "\x1b_Gf=24;AAAA\x1b\\", true)}},
		{"bel does not end a dcs", "\x1bPa\x07b\x1b\\", []Token{tok(DCS, "\x1bPa\x07b\x1b\\", true)}},

		// Other escape sequences.
		{"save cursor", "\x1b7x", []Token{tok(ESC, "\x1b7", true), tok(Plain, "x", true)}},
		{"reverse index", "\x1bM", []Token{tok(ESC, "\x1bM", true)}},
		{"keypad mode", "\x1b=\x1b>", []Token{tok(ESC, "\x1b=", true), tok(ESC, "\x1b>", true)}},
		{"reset", "\x1bc", []Token{tok(ESC, "\x1bc", true)}},
		{"character set", "\x1b(Bx", []Token{tok(ESC, "\x1b(B", true), tok(Plain, "x", true)}},
		{"alignment test", "\x1b#8", []Token{tok(ESC, "\x1b#8", true)}},
		{"two intermediates", "\x1b $Ax", []Token{tok(ESC, "\x1b $A", true), tok(Plain, "x", true)}},
		{"string terminator alone", "\x1b\\x", []Token{tok(ESC, "\x1b\\", true), tok(Plain, "x", true)}},
		{"escape and a letter", "x\x1by", []Token{tok(Plain, "x", true), tok(ESC, "\x1by", true)}},

		// Sequences that the end of the string cuts short.
		{"lone escape", "\x1b", []Token{tok(ESC, "\x1b", false)}},
		{"text then escape", "ab\x1b", []Token{tok(Plain, "ab", true), tok(ESC, "\x1b", false)}},
		{"csi introducer", "\x1b[", []Token{tok(CSI, "\x1b[", false)}},
		{"csi without final byte", "\x1b[38;2;255", []Token{tok(CSI, "\x1b[38;2;255", false)}},
		{"osc without terminator", "\x1b]0;title", []Token{tok(OSC, "\x1b]0;title", false)}},
		{"osc swallows newlines", "\x1b]0;title\nmore\n", []Token{tok(OSC, "\x1b]0;title\nmore\n", false)}},
		{"osc ending in escape", "\x1b]0;title\x1b", []Token{tok(OSC, "\x1b]0;title", false), tok(ESC, "\x1b", false)}},
		{"dcs without terminator", "\x1bPabc", []Token{tok(DCS, "\x1bPabc", false)}},
		{"apc introducer", "\x1b_", []Token{tok(APC, "\x1b_", false)}},
		{"escape and intermediate", "\x1b(", []Token{tok(ESC, "\x1b(", false)}},

		// Sequences that a byte they cannot hold cuts short.
		{"newline in a csi", "\x1b[31\nm", []Token{tok(CSI, "\x1b[31", false), tok(Plain, "\nm", true)}},
		{"escape in a csi", "\x1b[1\x1b[2m", []Token{tok(CSI, "\x1b[1", false), tok(CSI, "\x1b[2m", true)}},
		{"delete in a csi", "\x1b[1\x7fm", []Token{tok(CSI, "\x1b[1", false), tok(Plain, "\x7fm", true)}},
		{"utf-8 in a csi", "\x1b[1é", []Token{tok(CSI, "\x1b[1", false), tok(Plain, "é", true)}},
		{"two escapes", "\x1b\x1b[0m", []Token{tok(ESC, "\x1b", false), tok(CSI, "\x1b[0m", true)}},
		{"escape and a control", "\x1b\nx", []Token{tok(ESC, "\x1b", false), tok(Plain, "\nx", true)}},
		{"escape and a high byte", "\x1b\xffx", []Token{tok(ESC, "\x1b", false), tok(Plain, "\xffx", true)}},
		{"escape and delete", "\x1b\x7f", []Token{tok(ESC, "\x1b", false), tok(Plain, "\x7f", true)}},
		{"intermediate then control", "\x1b(\nB", []Token{tok(ESC, "\x1b(", false), tok(Plain, "\nB", true)}},
		{"osc cut by a sequence", "\x1b]0;a\x1b[0mb", []Token{tok(OSC, "\x1b]0;a", false), tok(CSI, "\x1b[0m", true), tok(Plain, "b", true)}},
		{"osc cancelled", "\x1b]0;a\x18b", []Token{tok(OSC, "\x1b]0;a", false), tok(Plain, "\x18b", true)}},
		{"dcs substituted", "\x1bPa\x1ab", []Token{tok(DCS, "\x1bPa", false), tok(Plain, "\x1ab", true)}},
	}
	for _, tt := range tests {
		got := slices.Collect(Tokens(tt.in))
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: Tokens(%q)\n got %#v\nwant %#v", tt.name, tt.in, got, tt.want)
		}
		// NextToken steps through the same tokens.
		var stepped []Token
		for rest := tt.in; rest != ""; {
			var tk Token
			tk, rest = NextToken(rest)
			stepped = append(stepped, tk)
		}
		if !slices.Equal(stepped, tt.want) {
			t.Errorf("%s: NextToken steps through %#v, want %#v", tt.name, stepped, tt.want)
		}
	}
	if tk, rest := NextToken(""); tk != (Token{}) || rest != "" {
		t.Errorf("NextToken(\"\") = %v, %q", tk, rest)
	}
}

func TestTokensStopsEarly(t *testing.T) {
	n := 0
	for range Tokens("a\x1b[0mb\x1b[0mc") {
		n++
		if n == 2 {
			break
		}
	}
	if n != 2 {
		t.Errorf("read %d tokens after breaking at two", n)
	}
}

func TestTokenKinds(t *testing.T) {
	tests := []struct {
		in        string
		sgr, link bool
	}{
		{"\x1b[0m", true, false},
		{"\x1b[m", true, false},
		{"\x1b[1;38;5;206m", true, false},
		{"\x1b[4:3m", true, false},
		{"\x1b[?25l", false, false},
		{"\x1b[>4;2m", false, false}, // a private marker: not SGR
		{"\x1b[?4m", false, false},
		{"\x1b[1 m", false, false}, // an intermediate byte: not SGR
		{"\x1b[31", false, false},  // cut short
		{"\x1b[2K", false, false},
		{"\x1b]8;;https://example.com\x1b\\", false, true},
		{"\x1b]8;id=a;https://example.com\x07", false, true},
		{"\x1b]8;;\x1b\\", false, true},
		{"\x1b]8;;\x07", false, true},
		{"\x1b]8;;https://example.com", false, false}, // cut short
		{"\x1b]8;nosemicolon\x07", false, false},
		{"\x1b]0;title\x07", false, false},
		{"\x1b]88;;x\x07", false, false},
		{"plain", false, false},
	}
	for _, tt := range tests {
		tk, rest := NextToken(tt.in)
		if rest != "" {
			t.Errorf("%q is more than one token", tt.in)
		}
		if tk.IsSGR() != tt.sgr || tk.IsHyperlink() != tt.link {
			t.Errorf("%q: IsSGR %t, IsHyperlink %t; want %t, %t", tt.in, tk.IsSGR(), tk.IsHyperlink(), tt.sgr, tt.link)
		}
	}
}

func TestKindString(t *testing.T) {
	names := map[Kind]string{Plain: "plain", CSI: "CSI", OSC: "OSC", DCS: "DCS", SOS: "SOS", PM: "PM", APC: "APC", ESC: "ESC", Kind(200): "unknown"}
	for kind, want := range names {
		if got := kind.String(); got != want {
			t.Errorf("Kind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}

func TestFinished(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"plain", "plain"},
		{"\x1b[1mwhole\x1b[0m \x1b]8;;https://example.com\x07x\x1b]8;;\x1b\\ \x1b7", "\x1b[1mwhole\x1b[0m \x1b]8;;https://example.com\x07x\x1b]8;;\x1b\\ \x1b7"},
		{"ab\x1b", "ab"},
		{"ab\x1b[31", "ab"},
		{"ab\x1b[31\ncd", "ab\ncd"},
		{"ab\x1b]0;title and the rest", "ab"},
		{"\x1b[1mab\x1b[3\x1b[0mcd\x1b(", "\x1b[1mab\x1b[0mcd"},
		{"a\x1b\x1b\x1b\nb", "a\nb"},
		{"a\x1b\x1b\x1bb", "a\x1bb"},
		{"\xcc\x1b\x80", "\xcc\x80"},
	}
	for _, tt := range tests {
		got := finished(tt.in)
		if got != tt.want {
			t.Errorf("finished(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if hasUnfinished(got) || hasUnfinished(tt.in) != (got != tt.in) {
			t.Errorf("finished(%q) = %q: hasUnfinished disagrees", tt.in, got)
		}
	}
}

func TestStrip(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"plain", "plain"},
		{"a\x1b[31mb\x1b[0mc", "abc"},
		{"\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"\x1b]0;title\x07text", "text"},
		{"\x1bPdata\x1b\\a\x1bXs\x1b\\b\x1b^p\x1b\\c\x1b_a\x1b\\d", "abcd"},
		{"a\x1b7b\x1b(Bc\x1b#8d", "abcd"},
		{"keeps\nnewlines\tand tabs\r\n", "keeps\nnewlines\tand tabs\r\n"},
		{"lone \x1b", "lone "},
		{"x\x1b\x1b\x1by", "x"},
		{"cut \x1b[31", "cut "},
		{"cut \x1b]0;title and the rest", "cut "},
		{"\x1b[31\nm", "\nm"},
		{"\x1b\xffx", "\xffx"},
		{"e\x1b[1m\u0301", "e\u0301"},
		{"\u009b31m stays: C1 controls are not sequences", "\u009b31m stays: C1 controls are not sequences"},
	}
	for _, tt := range tests {
		if got := Strip(tt.in); got != tt.want {
			t.Errorf("Strip(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"plain", "hello", 5},
		{"styled", "\x1b[1;31mhello\x1b[0m", 5},
		{"only sequences", "\x1b[1m\x1b[0m", 0},
		{"hyperlink", "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\", 4},
		{"title", "\x1b]0;a long title\x07x", 1},
		{"wide and styled", "\x1b[32m漢字\x1b[0m!", 5},
		{"emoji and styled", "\x1b[1m👍🏽\x1b[0m ok", 5},
		{"cut short csi", "ab\x1b[31", 2},
		{"cut short osc swallows the rest", "ab\x1b]0;cd ef", 2},
		{"lone escape", "ab\x1b", 2},
		{"interrupted csi shows its tail", "\x1b[31\nm", 1},
		{"newlines count nothing", "ab\ncd", 4},

		// A cluster that a sequence interrupts is still one cluster.
		{"accent after a sequence", "e\x1b[31m\u0301", 1},
		{"skin tone after a sequence", "👍\x1b[0m🏽", 2},
		{"selector after a sequence", "❤\x1b[1m\ufe0f", 2},
		{"flag across a sequence", "🇺\x1b[m🇸", 2},
		{"flag across two sequences", "🇺\x1b[m\x1b[1m🇸!", 3},
		{"zwj sequence across sequences", "👨\x1b[1m\u200d\x1b[0m👩", 2},
		{"jamo across a sequence", "\u1112\x1b[1m\u1161\u11ab", 2},
		{"crlf across a sequence", "a\r\x1b[0m\nb", 2},
		{"sequence before a mark at the start", "\x1b[1m\u0301a", 1},
		{"no cluster to continue", "a\x1b[1mb", 2},

		// A sequence between the bytes of one character: taking it out
		// leaves the character whole.
		{"sequence inside a character", "\xcc\x1b0\x80", 0},
		{"sequence inside a wide character", "\xe6\x1b[1m\xbc\x1b[0m\xa2", 2},
		{"lone escape inside a character", "0\xd8\x1b\x890", 3},
		{"sequence after a lone lead byte", "\xe6\x1b[1mab", 3},
		{"sequence before a stray continuation byte", "a\x1b[1m\xa2", 2},
	}
	for _, tt := range tests {
		if got := VisibleWidth(tt.in); got != tt.want {
			t.Errorf("%s: VisibleWidth(%q) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
		if got := Width(Strip(tt.in)); got != tt.want {
			t.Errorf("%s: Width(Strip(%q)) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
	}
	if got := (Metrics{AmbiguousWide: true}).VisibleWidth("\x1b[1mαβ\x1b[0m"); got != 4 {
		t.Errorf("with wide ambiguous characters VisibleWidth = %d, want 4", got)
	}
}

// everyForm holds every kind of escape sequence, whole, with text between.
const everyForm = "a\x1b[1;31mb\x1b[0m\x1b]8;;https://example.com\x1b\\c\x1b]8;;\x07" +
	"\x1bP1$r\x1b\\d\x1bXs\x1b\\\x1b^p\x1b\\\x1b_a\x1b\\e\x1b7\x1b(B\x1b $Af漢\x1b[38:2::1:2:3m字\x1b[m👍🏽"

// TestSplitAnywhere cuts a string that holds every kind of sequence at every
// byte, as a stream that arrives in pieces would: both halves must still
// scan, a half's tokens must join back to the half, and only the last token
// of the first half may differ from the tokens of the whole.
func TestSplitAnywhere(t *testing.T) {
	whole := slices.Collect(Tokens(everyForm))
	for _, tk := range whole {
		if !tk.Complete {
			t.Fatalf("the test string holds a sequence that is cut short: %q", tk.Text)
		}
	}
	for i := 0; i <= len(everyForm); i++ {
		head, tail := everyForm[:i], everyForm[i:]
		for _, part := range []string{head, tail} {
			var joined strings.Builder
			for tk := range Tokens(part) {
				if tk.Text == "" {
					t.Fatalf("cut at %d: an empty token in %q", i, part)
				}
				joined.WriteString(tk.Text)
			}
			if joined.String() != part {
				t.Fatalf("cut at %d: the tokens of %q join to %q", i, part, joined.String())
			}
		}
		// The tokens of the first half are the tokens of the whole, except
		// that the last one may be a shorter, unfinished version. A cut
		// right after an ESC leaves that ESC as one more token, alone.
		got := slices.Collect(Tokens(head))
		if strings.HasSuffix(head, "\x1b") {
			if got[len(got)-1] != tok(ESC, "\x1b", false) {
				t.Fatalf("cut at %d: the last token is %#v, want a lone ESC", i, got[len(got)-1])
			}
			got = got[:len(got)-1]
		}
		for k, tk := range got {
			switch {
			case k >= len(whole):
				t.Fatalf("cut at %d: %d tokens where the whole has %d", i, len(got), len(whole))
			case tk == whole[k]:
			case k == len(got)-1 && tk.Kind == whole[k].Kind && strings.HasPrefix(whole[k].Text, tk.Text):
				// Plain text cut in the middle is still whole; a sequence
				// cut in the middle is not.
				if tk.Complete != (tk.Kind == Plain) {
					t.Fatalf("cut at %d: the last token %q has Complete %t", i, tk.Text, tk.Complete)
				}
			default:
				t.Fatalf("cut at %d: token %d is %#v, but %#v in the whole", i, k, tk, whole[k])
			}
		}
		// What is visible in the first half is the start of what is
		// visible in the whole.
		if !strings.HasPrefix(Strip(everyForm), Strip(head)) {
			t.Fatalf("cut at %d: Strip gives %q, which does not start %q", i, Strip(head), Strip(everyForm))
		}
		// Truncate and Wrap never hand back an unfinished sequence.
		if out := Truncate(head, 3, "…"); hasUnfinished(out) {
			t.Fatalf("cut at %d: Truncate returns an unfinished sequence: %q", i, out)
		}
		for _, line := range Wrap(head, 4, WrapOptions{}) {
			if hasUnfinished(line) {
				t.Fatalf("cut at %d: Wrap returns an unfinished sequence: %q", i, line)
			}
		}
	}
}

// FuzzStrip checks the scanner on any input: it ends, its tokens are whole
// and join back to the input, no ESC survives Strip, stripping twice changes
// nothing more, and the visible width is the width of what Strip leaves.
func FuzzStrip(f *testing.F) {
	for _, seed := range fuzzSeeds() {
		f.Add(seed)
	}
	f.Add(everyForm)
	f.Fuzz(func(t *testing.T, s string) {
		var joined strings.Builder
		previous := Kind(255)
		for tk := range Tokens(s) {
			if tk.Text == "" {
				t.Fatalf("Tokens(%q) yields an empty token", s)
			}
			if (tk.Kind == Plain) == (tk.Text[0] == '\x1b') {
				t.Fatalf("Tokens(%q) yields %q with kind %d", s, tk.Text, tk.Kind)
			}
			if tk.Kind == Plain && (!tk.Complete || strings.Contains(tk.Text, "\x1b") || previous == Plain) {
				t.Fatalf("Tokens(%q) yields the plain token %q after kind %d", s, tk.Text, previous)
			}
			if tk.Kind != Plain && strings.Count(tk.Text, "\x1b") > 2 {
				t.Fatalf("Tokens(%q) yields a sequence with more than two ESC bytes: %q", s, tk.Text)
			}
			previous = tk.Kind
			joined.WriteString(tk.Text)
		}
		if joined.String() != s {
			t.Fatalf("the tokens of %q join to %q", s, joined.String())
		}

		stripped := Strip(s)
		if strings.Contains(stripped, "\x1b") {
			t.Fatalf("Strip(%q) = %q still holds an ESC", s, stripped)
		}
		if again := Strip(stripped); again != stripped {
			t.Fatalf("Strip(Strip(%q)) = %q, but Strip gave %q", s, again, stripped)
		}
		if !strings.Contains(s, "\x1b") && stripped != s {
			t.Fatalf("Strip changed %q, which has no ESC, to %q", s, stripped)
		}
		for _, m := range []Metrics{{}, {AmbiguousWide: true}} {
			if got, want := m.VisibleWidth(s), m.Width(stripped); got != want {
				t.Fatalf("%+v.VisibleWidth(%q) = %d, but Width(Strip) = %d", m, s, got, want)
			}
		}

		// The pieces are the clusters of the stripped text, with the
		// sequences where they were.
		var text strings.Builder
		var clusters []string
		width := 0
		for rest := s; rest != ""; {
			var p piece
			p, rest = Metrics{}.nextPiece(rest)
			if p.text == "" {
				t.Fatalf("an empty piece in %q", s)
			}
			text.WriteString(p.text)
			width += p.width
			if p.kind == Plain {
				clusters = append(clusters, Strip(p.text))
				if p.inner != strings.Contains(p.text, "\x1b") {
					t.Fatalf("the piece %q of %q has inner %t", p.text, s, p.inner)
				}
				if alone := Width(Strip(p.text)); alone != p.width {
					t.Fatalf("the piece %q of %q is %d wide, but %d on its own", p.text, s, p.width, alone)
				}
			} else if p.width != 0 {
				t.Fatalf("the sequence %q of %q has width %d", p.text, s, p.width)
			}
		}
		if text.String() != s {
			t.Fatalf("the pieces of %q join to %q", s, text.String())
		}
		if got := VisibleWidth(s); got != width {
			t.Fatalf("VisibleWidth(%q) = %d, but its pieces add up to %d", s, got, width)
		}
		if want := graphemes(stripped); !slices.Equal(clusters, want) {
			t.Fatalf("the pieces of %q hold the clusters %q, but the stripped text has %q", s, clusters, want)
		}

		// Dropping unfinished sequences leaves none, and no less text.
		whole := finished(s)
		if hasUnfinished(whole) || Strip(whole) != stripped {
			t.Fatalf("finished(%q) = %q", s, whole)
		}
	})
}

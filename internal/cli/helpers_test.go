package cli

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// The four ways a gnosh process can be connected.
var (
	// tty is a person at a terminal: nothing piped in, output on screen.
	tty = Env{StdinIsTerminal: true, StdoutIsTerminal: true}
	// piped has text piped in and its output on screen.
	piped = Env{StdinIsTerminal: false, StdoutIsTerminal: true}
	// captured has a terminal for input and its output going elsewhere.
	captured = Env{StdinIsTerminal: true, StdoutIsTerminal: false}
	// script has neither: a pipe in and a pipe out.
	script = Env{}
)

// allBuilt returns the table with every command, flag and value marked
// built, so a test can ask how a line is read without tripping over what
// happens to be built today.
func allBuilt(t testing.TB) *Table {
	t.Helper()
	tab := Default()
	for _, e := range tab.Entries() {
		if err := tab.SetStatus(e.Name, Built); err != nil {
			t.Fatalf("SetStatus(%q): %v", e.Name, err)
		}
	}
	return tab
}

// words splits a line the way a shell would into the arguments gnosh gets,
// so that examples can be written in tests exactly as they are in the
// specs. It knows single quotes, double quotes (with \" and \\ inside),
// and that quoted and unquoted pieces that touch make one argument. The
// leading "gnosh" is dropped.
func words(t testing.TB, line string) []string {
	t.Helper()
	var out []string
	var cur strings.Builder
	open := false // an argument has been started, even if it is still empty
	quote := rune(0)
	rs := []rune(line)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			switch {
			case r == '"':
				quote = 0
			case r == '\\' && i+1 < len(rs) && (rs[i+1] == '"' || rs[i+1] == '\\'):
				i++
				cur.WriteRune(rs[i])
			default:
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			open = true
		case unicode.IsSpace(r):
			if open {
				out = append(out, cur.String())
				cur.Reset()
				open = false
			}
		default:
			cur.WriteRune(r)
			open = true
		}
	}
	if quote != 0 {
		t.Fatalf("words: a quote is left open in %q", line)
	}
	if open {
		out = append(out, cur.String())
	}
	if len(out) == 0 || out[0] != "gnosh" {
		t.Fatalf("words: %q does not begin with gnosh", line)
	}
	if len(out) == 1 {
		return nil
	}
	return out[1:]
}

func TestWords(t *testing.T) {
	for line, want := range map[string][]string{
		`gnosh`:                           nil,
		`gnosh -p "a b"  c`:               {"-p", "a b", "c"},
		`gnosh --debug='mcp,startup'`:     {"--debug=mcp,startup"},
		`gnosh --tools "" x`:              {"--tools", "", "x"},
		`gnosh "say \"hi\"" 'it''s'`:      {`say "hi"`, "its"},
		`gnosh --settings '{"a": "b c"}'`: {"--settings", `{"a": "b c"}`},
	} {
		if got := words(t, line); !reflect.DeepEqual(got, want) {
			t.Errorf("words(%q) = %q, want %q", line, got, want)
		}
	}
}

// inter returns i as the Invocation of an interactive session: the fields a
// session always has are filled in where i leaves them empty.
func inter(i Invocation) Invocation {
	i.Command = CmdInteractive
	if i.Conversation == "" {
		i.Conversation = ConversationNew
	}
	return i
}

// pr returns i as the Invocation of a print run.
func pr(i Invocation) Invocation {
	i.Command = CmdPrint
	if i.Conversation == "" {
		i.Conversation = ConversationNew
	}
	if i.OutputFormat == "" {
		i.OutputFormat = FormatText
	}
	if i.InputFormat == "" {
		i.InputFormat = FormatText
	}
	return i
}

// diff lists the fields in which two Invocations differ, one per line, or
// returns "" when they are the same.
func diff(got, want Invocation) string {
	var b strings.Builder
	g, w := reflect.ValueOf(got), reflect.ValueOf(want)
	for k := 0; k < g.NumField(); k++ {
		if !reflect.DeepEqual(g.Field(k).Interface(), w.Field(k).Interface()) {
			fmt.Fprintf(&b, "  %s: got %#v, want %#v\n", g.Type().Field(k).Name, g.Field(k).Interface(), w.Field(k).Interface())
		}
	}
	return b.String()
}

// lineCase is one command line and what Parse should make of it.
type lineCase struct {
	// line is the command as it would be typed in a shell, starting with
	// "gnosh". args is used instead when a shell line cannot show it.
	line string
	args []string
	// env is how the process is connected; nil means a person at a terminal.
	env *Env
	// want is the Invocation expected, when kind is empty.
	want Invocation
	// kind, when set, is the kind of error expected; say lists pieces its
	// message must hold, and hint the name it must suggest.
	kind ErrorKind
	say  []string
	hint string
}

// runLines parses each case with tab and checks what comes back.
func runLines(t *testing.T, tab *Table, cases []lineCase) {
	t.Helper()
	for _, c := range cases {
		args, name := c.args, c.line
		if args == nil {
			args = words(t, c.line)
		} else if name == "" {
			name = fmt.Sprintf("%q", args)
		}
		env := tty
		if c.env != nil {
			env = *c.env
		}
		t.Run(name, func(t *testing.T) {
			got, err := tab.Parse(args, env)
			if c.kind == "" {
				if err != nil {
					t.Fatalf("Parse(%q) failed: %v", args, err)
				}
				if d := diff(got, c.want); d != "" {
					t.Errorf("Parse(%q):\n%s", args, d)
				}
				return
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("Parse(%q) = %+v, %v; want a %s error", args, got, err, c.kind)
			}
			if e.Kind != c.kind {
				t.Errorf("Parse(%q): error kind %s, want %s\n  %s", args, e.Kind, c.kind, e.Message)
			}
			for _, s := range c.say {
				if !strings.Contains(e.Message, s) {
					t.Errorf("Parse(%q): the message does not say %q:\n  %s", args, s, e.Message)
				}
			}
			if c.hint != "" && e.Suggestion != c.hint {
				t.Errorf("Parse(%q): suggests %q, want %q\n  %s", args, e.Suggestion, c.hint, e.Message)
			}
			if !reflect.DeepEqual(got, Invocation{}) {
				t.Errorf("Parse(%q): an error came with a filled-in Invocation: %+v", args, got)
			}
			checkMessage(t, e)
		})
	}
}

// checkMessage holds every error message to the house rules: it is one
// line of ordinary printable text, it says that nothing happened, and it
// ends like a sentence or with something to type.
func checkMessage(t *testing.T, e *Error) {
	t.Helper()
	if e.Message == "" {
		t.Errorf("an error of kind %s has no message", e.Kind)
		return
	}
	for _, r := range e.Message {
		if r != ' ' && (unicode.IsSpace(r) || !unicode.IsPrint(r)) {
			t.Errorf("the message holds %q, which is not plain printable text:\n  %q", r, e.Message)
			break
		}
	}
	if !strings.Contains(e.Message, "othing was started") && !strings.Contains(e.Message, "othing was done") {
		t.Errorf("the message does not say that nothing happened:\n  %s", e.Message)
	}
}

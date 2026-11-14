package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// sep stands between the arguments of a command line in a fuzz input. No
// argument of a real process can hold it.
const sep = "\x00"

// seedLines are command lines for the fuzzers to start from: one or more
// of every kind of thing the parser knows.
var seedLines = [][]string{
	{},
	{"hello there"},
	{"-y", "-C"},
	{"-p", "what is this", "--output-format", "json"},
	{"-p", "-y", "--cwd", "DIR", "-m", "p/m", "do it"},
	{"-r"},
	{"-r", "abc", "more"},
	{"-r", "abc", "--fork-session", "--session-id", sampleUUID},
	{"--allowedTools", "Bash(git log *)", "Bash(git diff *)", "Read"},
	{"--allowedTools", "Bash(go test *)", "run the tests and report"},
	{"--disallowedTools", "Bash Edit", "-p", "x"},
	{"--tools", "", "review this"},
	{"--add-dir", "../apps", "../lib"},
	{"--system-prompt", "- be terse", "--append-system-prompt-file", "x.md"},
	{"--settings", `{"env": {}}`, "--setting-sources", "user,local"},
	{"--debug=hooks,mcp", "--debug-file", "/tmp/d.log"},
	{"-p", "--input-format", "text", "--output-format", "stream-json", "--verbose"},
	{"-p", "x", "--max-turns", "3", "--goal", "done", "--small-model", "m"},
	{"--permission-mode", "plan", "--safe-mode", "--disable-slash-commands"},
	{"-pv", "x"},
	{"-v"},
	{"-yCn", "abc"},
	{"--", "-p", "--help"},
	{"-c", "/tmp/a/b"},
	{"-c", "--", "/tmp/a/b"},
	{"doctor"},
	{"trust", "--forget", "../x"},
	{"models"},
	{"session", "list", "--json"},
	{"s", "rename", "abc", "New", "title"},
	{"session", "show", "abc", "--json"},
	{"mem", "find", "flaky", "test"},
	{"mem", "--cwd", "/w", "log", "20"},
	{"logs", "-ft", "50"},
	{"dirs"},
	{"help", "session", "list"},
	{"version"},
	{"--alowedTools", "x"},
	{"docter"},
	{"session", "lst"},
	{"-m=x"},
	{"-ny"},
	{"--model"},
	{"--verbose=1"},
	{"-"},
	{""},
	{"--=", "---", "-=", "--\xff", "-\x00"},
	{"--allowedTools", "A(b (c),D(e)"},
}

func addSeeds(f *testing.F, add func(line string)) {
	f.Helper()
	for _, l := range seedLines {
		add(strings.Join(l, sep))
	}
}

// cut turns a fuzz input back into arguments.
func cut(line string) []string {
	if line == "" {
		return nil
	}
	return strings.Split(line, sep)
}

// FuzzParse throws arbitrary command lines at Parse, with everything built
// and with the table as it ships, and checks what must hold for all of
// them: no panic; the arguments are left alone; the same line gives the
// same answer; an error is a *Error with plain text and no Invocation; and
// an Invocation that comes back is one the rest of the program can rely
// on.
func FuzzParse(f *testing.F) {
	built, shipped := allBuilt(f), Default()
	known := map[string]bool{}
	for _, e := range shipped.Entries() {
		known[e.Name] = true
	}
	addSeeds(f, func(line string) {
		for bits := uint8(0); bits < 4; bits++ {
			f.Add(line, bits)
		}
	})
	f.Fuzz(func(t *testing.T, line string, envBits uint8) {
		args := cut(line)
		env := Env{StdinIsTerminal: envBits&1 != 0, StdoutIsTerminal: envBits&2 != 0}
		before := append([]string(nil), args...)
		for _, tab := range []*Table{built, shipped} {
			inv, err := tab.Parse(args, env)
			if !reflect.DeepEqual(args, before) {
				t.Fatalf("Parse changed its arguments: %q became %q", before, args)
			}
			again, err2 := tab.Parse(args, env)
			if describe(inv, err) != describe(again, err2) || !reflect.DeepEqual(inv, again) {
				t.Fatalf("Parse(%q) gave two answers:\n  %s\n  %s", args, describe(inv, err), describe(again, err2))
			}
			if err != nil {
				var e *Error
				if !errors.As(err, &e) {
					t.Fatalf("Parse(%q) returned an error that is not a *Error: %v", args, err)
				}
				if e.Kind == "" {
					t.Errorf("Parse(%q): the error has no kind: %s", args, e.Message)
				}
				if (e.Kind == ErrNotBuilt) != (len(e.NotBuilt) > 0) {
					t.Errorf("Parse(%q): kind %s with NotBuilt %q", args, e.Kind, e.NotBuilt)
				}
				if tab == built && e.Kind == ErrNotBuilt {
					t.Errorf("Parse(%q): not built, in a table where everything is: %s", args, e.Message)
				}
				for _, name := range e.NotBuilt {
					if !known[name] {
						t.Errorf("Parse(%q): NotBuilt names %q, which is no entry of the table", args, name)
					}
				}
				checkMessage(t, e)
				if len(e.Message) > 1200 {
					t.Errorf("Parse(%q): a message of %d bytes", args, len(e.Message))
				}
				if !reflect.DeepEqual(inv, Invocation{}) {
					t.Errorf("Parse(%q): an error came with an Invocation: %+v", args, inv)
				}
				continue
			}
			checkInvocation(t, tab, args, env, inv)
		}
		// A line that parses as shipped parses the same with everything
		// built: marking a thing built never changes what a line means.
		if inv, err := shipped.Parse(args, env); err == nil {
			if all, err := built.Parse(args, env); err != nil || !reflect.DeepEqual(inv, all) {
				t.Errorf("Parse(%q) means one thing as shipped and another with everything built:\n  %s\n  %s", args, describe(inv, nil), describe(all, err))
			}
		}
	})
}

// checkInvocation holds an Invocation that Parse returned to what its
// comments promise.
func checkInvocation(t *testing.T, tab *Table, args []string, env Env, inv Invocation) {
	t.Helper()
	bad := func(format string, a ...any) {
		t.Helper()
		t.Errorf("Parse(%q): "+format+"\n  %+v", append(append([]any{args}, a...), inv)...)
	}
	if inv.Command == "" {
		bad("no Command")
	}
	if (inv.HelpFor != nil) && inv.Command != CmdHelp {
		bad("HelpFor without CmdHelp")
	}
	if inv.Command == CmdHelp {
		if _, err := tab.Help(inv.HelpFor...); err != nil {
			bad("HelpFor names no command: %v", err)
		}
		return
	}
	session := inv.Command == CmdInteractive || inv.Command == CmdPrint
	printing := inv.Command == CmdPrint

	switch inv.Conversation {
	case ConversationNew, ConversationContinue, ConversationResume:
	case "":
		if session {
			bad("a session with no Conversation")
		}
	default:
		bad("Conversation %q", inv.Conversation)
	}
	if !session && inv.Conversation != "" {
		bad("a Conversation for a command that has none")
	}
	if (inv.Resume != "" || inv.ResumeIsFile) && inv.Conversation != ConversationResume {
		bad("Resume without ConversationResume")
	}
	if inv.ForkSession && inv.Conversation == ConversationNew {
		bad("a fork of nothing")
	}
	if inv.SessionID != "" {
		if isUUID(inv.SessionID) != "" {
			bad("SessionID is not a UUID")
		}
		if inv.Conversation != ConversationNew && !inv.ForkSession {
			bad("a new ID for an old conversation")
		}
	}
	if printing && inv.Conversation == ConversationResume && inv.Resume == "" {
		bad("print mode with nobody to pick a session")
	}

	switch inv.PermissionMode {
	case "", ModeDefault, ModePlan, ModeAcceptEdits, ModeDontAsk, ModeBypassPermissions:
	default:
		bad("PermissionMode %q", inv.PermissionMode)
	}
	for name, list := range map[string][]string{"AllowedTools": inv.AllowedTools, "DisallowedTools": inv.DisallowedTools, "Tools": inv.Tools} {
		for _, it := range list {
			if it == "" || spaceOutsideParens(it) {
				bad("%s holds %q, which is not one item", name, it)
			}
		}
	}
	for name, list := range map[string][]string{"AddDirs": inv.AddDirs, "SettingSources": inv.SettingSources} {
		for _, it := range list {
			if blank(it) {
				bad("%s holds an empty item", name)
			}
		}
	}
	if len(inv.Tools) > 0 && !inv.ToolsGiven {
		bad("Tools without ToolsGiven")
	}
	if (inv.DebugFilter != "" || inv.DebugFile != "") && !inv.Debug {
		bad("a debug filter or file without Debug")
	}
	if inv.SettingsInline != strings.HasPrefix(strings.TrimSpace(inv.Settings), "{") {
		bad("SettingsInline is wrong for %q", inv.Settings)
	}
	if inv.MaxTurns < 0 || inv.Tail < 0 {
		bad("a number out of range")
	}

	if session {
		if inv.ReadStdin == env.StdinIsTerminal {
			bad("ReadStdin is %v with StdinIsTerminal %v", inv.ReadStdin, env.StdinIsTerminal)
		}
		if inv.Command == CmdInteractive && !env.StdoutIsTerminal {
			bad("an interactive session with no terminal to draw on")
		}
		if inv.PrintImplied && (!printing || env.StdoutIsTerminal) {
			bad("PrintImplied is wrong")
		}
		if inv.Args != nil {
			bad("a session with Args")
		}
		if inv.Prompt != "" && blank(inv.Prompt) {
			bad("a prompt of nothing but white space")
		}
	} else if inv.Prompt != "" || inv.ReadStdin || inv.PrintImplied {
		bad("a prompt, or its source, for a command that takes none")
	}
	if printing {
		switch inv.OutputFormat {
		case FormatText, FormatJSON, FormatStreamJSON:
		default:
			bad("OutputFormat %q", inv.OutputFormat)
		}
		if inv.InputFormat != FormatText {
			bad("InputFormat %q", inv.InputFormat)
		}
		if inv.Prompt == "" && !inv.ReadStdin && inv.Goal == "" {
			bad("print mode with nothing to answer")
		}
	} else if inv.OutputFormat != "" || inv.InputFormat != "" || inv.Goal != "" || inv.MaxTurns != 0 || inv.SmallModel != "" {
		bad("a print-mode setting outside print mode")
	}
}

// FuzzAfterDashes checks that nothing after a lone "--" is ever read as a
// flag or as a command's name: whatever follows it, the flags are those
// typed before it, and what follows arrives untouched as the prompt or the
// command's arguments. The only errors allowed are about those arguments
// themselves (none, too many, empty).
func FuzzAfterDashes(f *testing.F) {
	tab := allBuilt(f)
	addSeeds(f, func(line string) { f.Add(line, uint8(3)) })
	for _, l := range [][]string{{"-p"}, {"--help"}, {"--version"}, {"--bogus"}, {"-x"}, {"mem"}, {"doctor"}, {"--model"}, {"--"}, {"-"}, {"--allowedTools", "x"}} {
		f.Add(strings.Join(l, sep), uint8(3))
	}
	// Each lead is a line up to its "--", with arguments that make it
	// complete, so that the flags it sets can be read off.
	leads := []struct {
		before []string
		filler []string
		// arguments gives what Invocation.Args must be for the words after
		// "--"; nil for a session, whose words are the prompt.
		arguments func(rest []string) []string
	}{
		{before: []string{}, filler: []string{"x"}},
		{before: []string{"-p"}, filler: []string{"x"}},
		{before: []string{"-p", "-v", "--max-turns", "3"}, filler: []string{"x"}},
		{before: []string{"-y", "-C", "--model", "m", "--allowedTools", "Read", "--add-dir", "../x", "--debug"}, filler: []string{"x"}},
		{before: []string{"trust"}, filler: []string{"x"}, arguments: func(rest []string) []string { return rest }},
		{before: []string{"mem", "find"}, filler: []string{"x"}, arguments: func(rest []string) []string { return append([]string{"find"}, rest...) }},
		{before: []string{"session", "rename", "--json"}, filler: []string{"i", "t"}, arguments: func(rest []string) []string {
			return []string{rest[0], strings.Join(rest[1:], " ")}
		}},
		{before: []string{"session", "show"}, filler: []string{"x"}, arguments: func(rest []string) []string { return rest }},
	}
	f.Fuzz(func(t *testing.T, line string, envBits uint8) {
		rest := cut(line)
		env := Env{StdinIsTerminal: envBits&1 != 0, StdoutIsTerminal: envBits&2 != 0}
		for _, lead := range leads {
			base, err := tab.Parse(append(append(append([]string{}, lead.before...), "--"), lead.filler...), env)
			if err != nil {
				t.Fatalf("the lead %q does not parse: %v", lead.before, err)
			}
			args := append(append(append([]string{}, lead.before...), "--"), rest...)
			inv, err := tab.Parse(args, env)
			if err != nil {
				var e *Error
				if !errors.As(err, &e) || (e.Kind != ErrUsage && e.Kind != ErrBadValue) {
					t.Errorf("Parse(%q): something after -- was read as more than an argument: %v", args, err)
				}
				continue
			}
			// The flags are the lead's, whatever followed.
			a, b := inv, base
			a.Prompt, a.Args, b.Prompt, b.Args = "", nil, "", nil
			if d := diff(a, b); d != "" {
				t.Errorf("Parse(%q): what follows -- changed more than the arguments:\n%s", args, d)
			}
			// And what followed arrives as it was typed.
			if lead.arguments == nil {
				if want := strings.Join(rest, " "); inv.Prompt != want {
					t.Errorf("Parse(%q): the prompt is %q, want %q", args, inv.Prompt, want)
				}
				continue
			}
			var want []string
			if len(rest) > 0 || len(lead.before) > 1 && lead.before[0] == "mem" {
				want = lead.arguments(rest)
			}
			if !reflect.DeepEqual(inv.Args, want) {
				t.Errorf("Parse(%q): Args is %q, want %q", args, inv.Args, want)
			}
		}
	})
}

// FuzzHelp asks for the help of arbitrary words and checks that it either
// comes, in shape, or is refused with a plain error.
func FuzzHelp(f *testing.F) {
	tab := Default()
	addSeeds(f, func(line string) { f.Add(line) })
	f.Fuzz(func(t *testing.T, line string) {
		path := cut(line)
		text, err := tab.Help(path...)
		if err != nil {
			var e *Error
			if !errors.As(err, &e) || e.Kind != ErrUnknownCommand || text != "" {
				t.Fatalf("Help(%q) = %q, %v", path, text, err)
			}
			checkMessage(t, e)
			return
		}
		for _, l := range strings.Split(text, "\n") {
			if len(l) > 80 {
				t.Errorf("Help(%q) has a line of %d columns: %s", path, len(l), l)
			}
			for _, r := range l {
				if r > unicode.MaxASCII || r < ' ' {
					t.Errorf("Help(%q) has a line with %q in it", path, r)
				}
			}
		}
		// The words that led here parse to a request for the same help.
		inv, err := tab.Parse(append(append([]string{}, path...), "--help"), tty)
		if err != nil || inv.Command != CmdHelp {
			t.Errorf("Help(%q) works and Parse(%q --help) = %+v, %v", path, path, inv, err)
			return
		}
		if same, _ := tab.Help(inv.HelpFor...); same != text {
			t.Errorf("Help(%q) and the help Parse(%q --help) asks for differ", path, path)
		}
	})
}

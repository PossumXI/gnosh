package shelltool

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHarmlessExit(t *testing.T) {
	t.Parallel()
	harmless := []string{
		"grep foo file",
		"  grep -rn 'a|b' src  ",
		"grep \"a;b\" file",
		"grep 'it''s' file",
		`grep a\;b file`,
		"/usr/bin/grep foo file",
		"egrep x y", "fgrep x y", "rg pattern", "rg -n 'x y' dir",
		"cat file | grep foo",
		"cat file | sort | grep -c foo",
		"LC_ALL=C grep foo file",
		"A=1 B='two words' grep foo file",
		"grep foo file 2>/dev/null",
		"grep foo file > out.txt",
		"grep \"$(cat pattern)\" file",
		"find . -name '*.go'",
		"diff a b", "diff -u a b | cat | diff - c",
		"test -f x", "[ -f x ]", "[ \"$a\" = b ]",
		"git diff --quiet", "git diff --exit-code HEAD~1 -- file",
		"git grep -n pattern", "git -C sub diff --quiet", "git --no-pager -c color.ui=never grep x",
	}
	for _, command := range harmless {
		if harmlessExit(command) == "" {
			t.Errorf("exit code 1 from %q should count as an answer", command)
		}
	}
	failures := []string{
		"", "   ", "ls nowhere", "false", "make test",
		"grep foo file; true",
		"cd dir && grep foo file",
		"grep foo file || echo none",
		"grep foo file &",
		"(grep foo file)",
		"{ grep foo file; }",
		"grep foo file\ngrep bar file",
		"grep `cat pattern` file",
		"grep $(cat pattern) file",
		"grep foo file | wc -l",
		"grep foo file | head",
		"cat <<EOF | grep foo\nEOF",
		"grep 'unterminated file",
		"grep \"unterminated file",
		"'grep' foo file",
		"\\grep foo file",
		"grepx foo", "mygrep foo", "xdiff a b",
		"git status", "git log --grep x", "git", "git -C", "git -C dir",
		"FOO=bar", "FOO=bar BAZ=qux",
		"env grep foo file", "time grep foo file", "! grep foo file", "sudo grep foo file",
		"1x=2 grep foo file",
		"\u00a0grep foo file", // a no-break space is part of the name, to a shell
	}
	for _, command := range failures {
		if meaning := harmlessExit(command); meaning != "" {
			t.Errorf("exit code 1 from %q should count as a failure, not as %q", command, meaning)
		}
	}
}

func TestShellWords(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a b  c", []string{"a", "b", "c"}},
		{"a 'b c' d", []string{"a", "b c", "d"}},
		{`a "b \"c\" d" e`, []string{"a", `b "c" d`, "e"}},
		{`a\ b c`, []string{"a b", "c"}},
		{"a''b \"\" c", []string{"ab", "", "c"}},
		{"tab\there", []string{"tab", "here"}},
		{`'it'\''s'`, []string{"it's"}},
		{`trailing\`, []string{`trailing\`}},
	}
	for _, tt := range tests {
		var got []string
		for _, w := range shellWords(tt.in) {
			got = append(got, w.text)
		}
		if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") || len(got) != len(tt.want) {
			t.Errorf("shellWords(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseStat(t *testing.T) {
	t.Parallel()
	// A real line, with a program name chosen to mislead: spaces and
	// parentheses of its own.
	line := "4242 (my (odd) prog) S 4000 4100 4200 34816 4242 4194560 1 2 3 4 5 6 7 8 20 0 1 0 987654321 1000 200 18446744073709551615 0 0\n"
	st, ok := parseStat([]byte(line))
	if !ok {
		t.Fatal("a good stat line was not read")
	}
	if st.name != "my (odd) prog" || st.state != 'S' || st.ppid != 4000 || st.pgid != 4100 || st.session != 4200 || st.start != 987654321 {
		t.Errorf("parseStat = %+v", st)
	}
	// Where the stack starts is the 28th field of the line.
	long := "4242 (sh) S 4000 4100 4200 0 -1 4194560 1 2 3 4 5 6 7 8 20 0 1 0 987654321 1000 200 18446744073709551615 94000000 94100000 140730000000000 0 0\n"
	if st, ok := parseStat([]byte(long)); !ok || st.stack != 140730000000000 {
		t.Errorf("parseStat = %+v, %v; want the start of the stack", st, ok)
	}
	if st.stack != 0 {
		t.Errorf("a short line gave a stack address: %+v", st)
	}
	for _, bad := range []string{
		"", "4242", "4242 (sh)", "4242 (sh) S 1 2", "4242 sh S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21",
		"4242 (sh) SS 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21",
		"4242 (sh) S x 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21",
		"4242 (sh) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 -5 20 21",
		") 4242 (sh S 1 2 3",
	} {
		if st, ok := parseStat([]byte(bad)); ok {
			t.Errorf("parseStat(%q) = %+v, want it refused", bad, st)
		}
	}
}

func TestCommandEnv(t *testing.T) {
	t.Parallel()
	got := commandEnv(
		[]string{"B=parent", "A=parent", "TERM=xterm", "OLDPWD=/old", "PWD=/wrong", "=odd", "NOEQUALS", "EMPTY=", "DUP=1", "DUP=2"},
		"/work",
		[]string{"C=extra", "A=extra", "B", "TERM=vt100", "", "=x", "GNOSH"},
	)
	want := []string{"A=extra", "AGENT=gnosh", "AI_AGENT=gnosh", "C=extra", "DUP=2", "EMPTY=", "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0", "PAGER=cat", "PWD=/work", "TERM=vt100"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("commandEnv:\n got %q\nwant %q", got, want)
	}
}

// FuzzHarmlessExit checks that no command line can upset the little shell
// reader, and that it never calls a command harmless that has more than
// one command in it.
func FuzzHarmlessExit(f *testing.F) {
	for _, seed := range []string{"grep a b", "a | grep 'b|c'", "git -C x diff", `grep "a\"b" c`, "x='", "\\", "a\\", "'", "\"", "[ a ]", "A=b", "grep a;b", "grep\na", "grep\n", "\u00a0grep a", "grep a\\\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, command string) {
		meaning := harmlessExit(command)
		for _, w := range shellWords(command) {
			if len(w.raw) == 0 {
				t.Fatalf("shellWords(%q) made a word from nothing", command)
			}
		}
		if meaning == "" {
			return
		}
		stage, ok := lastStage(command)
		if !ok || strings.TrimSpace(stage) == "" {
			t.Fatalf("%q is called harmless but has no last stage", command)
		}
		// Outside quotes, nothing that could join two commands. White
		// space around the command joins nothing.
		command = strings.Trim(command, " \t\r\n")
		quote := byte(0)
		for i := 0; i < len(command); i++ {
			c := command[i]
			switch {
			case quote == '\'':
				if c == '\'' {
					quote = 0
				}
			case c == '\\':
				i++
			case quote == '"':
				if c == '"' {
					quote = 0
				}
			case c == '\'' || c == '"':
				quote = c
			case strings.IndexByte(";&(){}`\n", c) >= 0:
				t.Fatalf("%q is called harmless (%s) but holds %q outside quotes", command, meaning, c)
			}
		}
	})
}

// FuzzParseStat checks that nothing found in /proc can upset the reader.
func FuzzParseStat(f *testing.F) {
	f.Add([]byte("1 (init) S 0 1 1 0 -1 4194560 1 2 3 4 5 6 7 8 20 0 1 0 12 1000 200 18446744073709551615"))
	f.Add([]byte("7 (a) b) c) R 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20"))
	f.Add([]byte("()"))
	f.Add([]byte(")("))
	f.Add([]byte("()\x00 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0"))
	f.Fuzz(func(t *testing.T, line []byte) {
		st, ok := parseStat(line)
		if !ok {
			return
		}
		if letter := st.state | 0x20; letter < 'a' || letter > 'z' {
			t.Fatalf("parseStat(%q) took %q for a state", line, st.state)
		}
		if !strings.Contains(string(line), "("+st.name+")") {
			t.Fatalf("parseStat(%q) made up the name %q", line, st.name)
		}
	})
}

// FuzzBashArgs checks that no arguments a model could send upset the
// reader: it either refuses them in words, or returns a command it can run
// with a time limit inside the bounds.
func FuzzBashArgs(f *testing.F) {
	for _, seed := range []string{
		`{"command": "ls"}`, `{"command": "ls", "timeout": 5000, "description": "list", "run_in_background": true}`,
		`{"command": 5}`, `{"command": "x", "timeout": "1e9"}`, `{"command": "x", "timeout": -1}`, `[]`, `null`, `"ls"`, `{`, ``,
		`{"command": "a\u0000b"}`, `{"command": "x", "timeout": 1e400}`, `{"command": "x", "run_in_background": "TRUE"}`,
		`{"command": "x", "timeout": {"a": 1}}`, `{"command": "\ud800"}`, `{"command":"x","command":"y"}`,
	} {
		f.Add([]byte(seed))
	}
	m := New(Options{})
	f.Fuzz(func(t *testing.T, input []byte) {
		a, err := m.bashArgs(json.RawMessage(input))
		if err != nil {
			if err.Error() == "" || !utf8.ValidString(err.Error()) {
				t.Fatalf("bashArgs(%q): a refusal with no usable words: %q", input, err)
			}
			return
		}
		if strings.TrimSpace(a.command) == "" || strings.IndexByte(a.command, 0) >= 0 {
			t.Fatalf("bashArgs(%q) accepted the command %q", input, a.command)
		}
		switch {
		case a.limit < 0:
			t.Fatalf("bashArgs(%q): limit %v", input, a.limit)
		case !a.background && (a.limit == 0 || a.limit > m.opt.MaxTimeout):
			t.Fatalf("bashArgs(%q): a foreground limit of %v", input, a.limit)
		case a.asked != 0 && a.asked <= a.limit:
			t.Fatalf("bashArgs(%q): asked %v but got %v", input, a.asked, a.limit)
		}
		if s := a.summary(); strings.ContainsAny(s, "\n\r") || utf8.RuneCountInString(s) > 200 {
			t.Fatalf("bashArgs(%q): summary %q is not one short line", input, s)
		}
		if _, err := taskArgsOf(json.RawMessage(input)); err != nil && err.Error() == "" {
			t.Fatalf("taskArgsOf(%q): a refusal with no words", input)
		}
	})
}

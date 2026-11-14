package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// goldenName returns the file that holds a command's help: gnosh.txt,
// gnosh-session-list.txt.
func goldenName(c *command) string {
	return strings.Join(append([]string{"gnosh"}, c.path()...), "-") + ".txt"
}

// TestHelpGolden compares the help of every command, from the table as it
// ships, with the copies kept in testdata/help.
//
// To write the copies afresh after a change to the table or the layout, run
// the test on the devbox with GNOSH_CLI_GOLDEN_DIR set to a folder under
// /tmp, read what changed, and copy the files into testdata/help:
//
//	GNOSH_CLI_GOLDEN_DIR=/tmp/cli-help go test -run TestHelpGolden ./internal/cli
func TestHelpGolden(t *testing.T) {
	tab := Default()
	out := os.Getenv("GNOSH_CLI_GOLDEN_DIR")
	if out != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	want := map[string]bool{}
	for _, c := range tab.commands {
		got, err := tab.Help(c.path()...)
		if err != nil {
			t.Fatalf("Help(%q): %v", c.path(), err)
		}
		name := goldenName(c)
		want[name] = true
		if out != "" {
			if err := os.WriteFile(filepath.Join(out, name), []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		kept, err := os.ReadFile(filepath.Join("testdata", "help", name))
		if err != nil {
			t.Errorf("no kept copy of the help for %q: %v", c.title(), err)
			continue
		}
		if got != string(kept) {
			t.Errorf("the help for %q differs from testdata/help/%s:\n%s", c.title(), name, firstDifference(got, string(kept)))
		}
	}
	if out != "" {
		t.Logf("wrote %d files to %s", len(want), out)
		return
	}
	// No kept copy is left over from a command that is gone.
	files, err := os.ReadDir(filepath.Join("testdata", "help"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !want[f.Name()] {
			t.Errorf("testdata/help/%s belongs to no command", f.Name())
		}
	}
}

// firstDifference shows the first line at which two texts part.
func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return "  line " + itoa(i+1) + "\n  got:  " + gl + "\n  kept: " + wl
		}
	}
	return "  (no difference found)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// TestHelpShape holds every command's help, with everything built and with
// the table as it ships, to the rules of the layout: 80 columns, plain
// ASCII, no tabs, no trailing spaces, one newline at the end.
func TestHelpShape(t *testing.T) {
	for label, tab := range map[string]*Table{"as shipped": Default(), "all built": allBuilt(t)} {
		for _, c := range tab.commands {
			text, err := tab.Help(c.path()...)
			if err != nil {
				t.Fatalf("%s: Help(%q): %v", label, c.path(), err)
			}
			if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
				t.Errorf("%s: the help for %q does not end with exactly one newline", label, c.title())
			}
			for n, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
				switch {
				case len(line) > 80:
					t.Errorf("%s: %q line %d is %d columns wide:\n  %s", label, c.title(), n+1, len(line), line)
				case strings.TrimRight(line, " ") != line:
					t.Errorf("%s: %q line %d ends with a space:\n  %q", label, c.title(), n+1, line)
				}
				for _, r := range line {
					if r < ' ' || r > '~' {
						t.Errorf("%s: %q line %d holds %q, which is not plain ASCII:\n  %s", label, c.title(), n+1, r, line)
						break
					}
				}
			}
			if strings.Contains(text, "\n\n\n") {
				t.Errorf("%s: the help for %q has two blank lines in a row", label, c.title())
			}
		}
	}
}

// TestHelpIsComplete checks that help says everything the parser takes:
// every spelling of every flag a command has, every subcommand, every value
// of a flag with a fixed set, and the command's own way of being typed.
func TestHelpIsComplete(t *testing.T) {
	tab := allBuilt(t)
	for _, c := range tab.commands {
		text, _ := tab.Help(c.path()...)
		lines := strings.Split(text, "\n")
		for _, f := range c.flags {
			line := flagLine(f)
			found := false
			for _, l := range lines {
				if l == "  "+line {
					found = true
				}
			}
			if !found {
				t.Errorf("the help for %q has no line %q", c.title(), line)
			}
			for _, name := range f.choiceNames() {
				if !strings.Contains(text, name) {
					t.Errorf("the help for %q does not name %s's value %s", c.title(), f.main(), name)
				}
			}
		}
		for _, s := range c.subs {
			found := false
			for _, l := range lines {
				if strings.HasPrefix(l, "  "+s.name+" ") || l == "  "+s.name {
					found = true
				}
			}
			if !found {
				t.Errorf("the help for %q does not list its command %s", c.title(), s.name)
			}
		}
		if c.parent != nil && !strings.Contains(text, "\n  "+usage(c)+"\n") {
			t.Errorf("the help for %q does not show how it is typed (%s)", c.title(), usage(c))
		}
		for _, a := range c.aliases {
			if !strings.Contains(text, c.parent.title()+" "+a) {
				t.Errorf("the help for %q does not give its other name %s", c.title(), a)
			}
		}
	}

	// The letter -v stands with the flag it means on a plain gnosh, and
	// the other flag's help says when the letter is its own.
	root, _ := tab.Help()
	for _, l := range []string{"  -v, --version", "  --verbose"} {
		if !strings.Contains(root, "\n"+l+"\n") {
			t.Errorf("help lacks the line %q:\n%.300s...", l, root)
		}
	}
	if !strings.Contains(root, "With -p, -v means this.") || !strings.Contains(root, "(With -p, -v means --verbose.)") {
		t.Errorf("help does not say what -v means with -p:\n%s", root)
	}
}

// TestHelpMarksWhatIsNotBuilt checks that help for the table as it ships
// says which commands, flags and values are not built, and stops saying so
// when they are.
func TestHelpMarksWhatIsNotBuilt(t *testing.T) {
	tab := Default()
	root, _ := tab.Help()
	for _, want := range []string{
		"  --fork-session\n      With -c or -r: go on in a copy of the session and leave the original as\n      it was. (Not built yet.)",
		"(Not built yet: json, stream-json.)",
		"  doctor                  Check that a turn could run here. (Not built yet.)",
		"  session <command>       Look at and manage sessions. (Not built yet.)",
	} {
		if !strings.Contains(root, want) {
			t.Errorf("the help as shipped does not say:\n%s", want)
		}
	}
	for _, built := range []string{"-c, -C, --continue", "-m, --model <model>", "-p, --print", "help [command]"} {
		at := strings.Index(root, "\n  "+built)
		if at < 0 {
			t.Errorf("the help has no %q", built)
			continue
		}
		// Its entry runs to the next line that is not indented further.
		rest := root[at+1:]
		end := len(rest)
		for _, stop := range []string{"\n  -", "\n\n"} {
			if i := strings.Index(rest[1:], stop); i >= 0 && i+1 < end {
				end = i + 1
			}
		}
		if !strings.HasPrefix(built, "-") {
			end = strings.Index(rest, "\n")
		}
		if entry := rest[:end]; strings.Contains(entry, "Not built") {
			t.Errorf("the help marks %q, which is built, as not built:\n%s", built, entry)
		}
	}
	doctor, _ := tab.Help("doctor")
	if !strings.Contains(doctor, "Not built yet in this build of gnosh") {
		t.Errorf("the help for gnosh doctor does not say it is not built:\n%s", doctor)
	}
	group, _ := tab.Help("session")
	if !strings.Contains(group, "None of these commands is built yet") {
		t.Errorf("the help for gnosh session does not say none of its commands is built:\n%s", group)
	}

	for _, name := range []string{"--fork-session", "--output-format=json", "--output-format=stream-json", "doctor", "doctor --hosts", "session list"} {
		if err := tab.SetStatus(name, Built); err != nil {
			t.Fatal(err)
		}
	}
	root, _ = tab.Help()
	for _, gone := range []string{"it was. (Not built yet.)", "(Not built yet: json", "could run here. (Not built yet.)", "manage sessions. (Not built yet.)"} {
		if strings.Contains(root, gone) {
			t.Errorf("the help still says %q after it was marked built", gone)
		}
	}
	doctor, _ = tab.Help("doctor")
	if strings.Contains(doctor, "Not built") {
		t.Errorf("the help for gnosh doctor still says it is not built:\n%s", doctor)
	}
	// One command of a group is enough for the group to count as there.
	group, _ = tab.Help("session")
	if strings.Contains(group, "None of these") || !strings.Contains(group, "show <id>               Print one session. (Not built yet.)") {
		t.Errorf("the help for gnosh session, with list built and the rest not:\n%s", group)
	}

	all := allBuilt(t)
	for _, c := range all.commands {
		if text, _ := all.Help(c.path()...); strings.Contains(text, "ot built") {
			t.Errorf("with everything built, the help for %q still says something is not", c.title())
		}
	}
}

func TestHelpForUnknownCommand(t *testing.T) {
	for _, c := range []struct {
		path []string
		hint string
	}{
		{[]string{"docter"}, "doctor"},
		{[]string{"session", "lst"}, "list"},
		{[]string{"nothing-like-it"}, ""},
		{[]string{"doctor", "more"}, ""},
	} {
		text, err := Help(c.path...)
		var e *Error
		if !errors.As(err, &e) || e.Kind != ErrUnknownCommand || text != "" {
			t.Errorf("Help(%q) = %q, %v; want an unknown-command error", c.path, text, err)
			continue
		}
		if e.Suggestion != c.hint {
			t.Errorf("Help(%q) suggests %q, want %q", c.path, e.Suggestion, c.hint)
		}
	}
	// Aliases lead to the same help.
	a, _ := Help("s", "ls")
	b, _ := Help("session", "list")
	if a == "" || a != b {
		t.Errorf("Help(s, ls) and Help(session, list) differ")
	}
}

func TestWrap(t *testing.T) {
	for _, c := range []struct {
		text  string
		width int
		want  []string
	}{
		{"", 10, []string{""}},
		{"one", 10, []string{"one"}},
		{"one two three", 7, []string{"one two", "three"}},
		{"one two three", 8, []string{"one two", "three"}},
		{"one two three", 13, []string{"one two three"}},
		{"a  b   c", 10, []string{"a b c"}},
		{"supercalifragilistic word", 5, []string{"supercalifragilistic", "word"}},
		{"first\nsecond line", 20, []string{"first", "second line"}},
		{"first\n\nthird", 20, []string{"first", "", "third"}},
	} {
		got := wrap(c.text, c.width)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrap(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

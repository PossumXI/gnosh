package cli

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

// TestTableIsSound checks the table against the rules the parser leans on.
// A row that breaks one would make some line read wrongly, so each is
// checked here once rather than hoped for.
func TestTableIsSound(t *testing.T) {
	tab := Default()

	ids := map[string]bool{}
	for _, f := range tab.flags {
		if ids[f.id] {
			t.Errorf("two flags have the id %q", f.id)
		}
		ids[f.id] = true
		if len(f.long) == 0 {
			t.Errorf("%s has no spelling as a word", f.id)
		}
		if f.help == "" || !strings.HasSuffix(strings.ReplaceAll(f.help, nbsp, " "), ".") && !strings.HasSuffix(f.help, ")") {
			t.Errorf("%s: the help is empty or does not end like a sentence: %q", f.id, f.help)
		}
		for _, l := range f.long {
			if l == "" || strings.ContainsAny(l, "= \t") || strings.HasPrefix(l, "-") {
				t.Errorf("%s: %q cannot be typed as --%s", f.id, l, l)
			}
		}
		for _, r := range f.short {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
				t.Errorf("%s: -%c is not a letter", f.id, r)
			}
		}
		switch f.kind {
		case kindBool:
			if f.arg != "" || len(f.choices) > 0 || f.number {
				t.Errorf("%s takes no value but describes one", f.id)
			}
		case kindList:
			if f.list == 0 || f.arg == "" || !strings.HasSuffix(f.arg, "...") {
				t.Errorf("%s is a list flag without a kind of list, or its value is not written with ...", f.id)
			}
		default:
			if f.arg == "" {
				t.Errorf("%s takes a value and help has no name for it", f.id)
			}
			if f.set == nil {
				t.Errorf("%s takes a value and records it nowhere", f.id)
			}
		}
		if f.kind != kindList && f.list != 0 {
			t.Errorf("%s has a kind of list but is not a list flag", f.id)
		}
		if f.printLetter != 0 && f.kind != kindBool {
			t.Errorf("%s has a print-mode letter and takes a value", f.id)
		}
		owned := false
		for _, c := range tab.commands {
			owned = owned || hasFlag(c.flags, f)
		}
		if !owned {
			t.Errorf("no command takes %s", f.id)
		}
	}

	names := map[string]bool{}
	for _, c := range tab.commands {
		title := c.title()
		if names[title] {
			t.Errorf("two commands are %q", title)
		}
		names[title] = true
		if c.summary == "" {
			t.Errorf("%s has no summary", title)
		}
		if c.needSub != (len(c.subs) > 0 && c.parent != nil) {
			t.Errorf("%s: needSub is %v with %d subcommands", title, c.needSub, len(c.subs))
		}
		if c.needSub && (c.cmd != "" || c.raw) {
			t.Errorf("%s needs a subcommand and yet is a command of its own", title)
		}
		if !c.needSub && c.cmd == "" {
			t.Errorf("%s leads to no Command", title)
		}
		if c.maxArgs >= 0 && c.minArgs > c.maxArgs {
			t.Errorf("%s needs %d arguments and takes %d at most", title, c.minArgs, c.maxArgs)
		}
		if (c.minArgs > 0 || c.maxArgs != 0) && !c.needSub && c.argsUsage == "" {
			t.Errorf("%s takes arguments and help does not show them", title)
		}
		if !hasFlag(c.flags, tab.flags[indexOf(tab, idHelp)]) {
			t.Errorf("%s does not take --help", title)
		}
		if c.raw && len(c.flags) != 1 {
			t.Errorf("%s passes everything on and still has flags of its own", title)
		}
		for id := range c.flagHelp {
			if !ids[id] {
				t.Errorf("%s has help for %q, which is no flag", title, id)
			}
		}

		// Within a command, and within everything that can be typed before
		// a group's subcommand, a spelling names one flag only.
		seen := map[string]*flag{}
		for _, f := range c.reach() {
			for _, s := range f.spellings() {
				if other, dup := seen[s]; dup && other != f {
					t.Errorf("%s: %s is both %s and %s", title, s, other.id, f.id)
				}
				seen[s] = f
			}
		}
		// A print-mode letter takes over a letter of a flag that takes no
		// value either, so the line is cut the same way whichever it means.
		for _, f := range c.flags {
			if f.printLetter == 0 {
				continue
			}
			other := c.letter(f.printLetter)
			if other == nil || other.kind != kindBool {
				t.Errorf("%s: -%c means %s in print mode, and outside it is not a flag without a value", title, f.printLetter, f.id)
			}
		}
		// No flag's word is made only of the command's one-letter flags:
		// "-print" can then be told from letters sharing a dash.
		for _, f := range c.reach() {
			for _, l := range f.long {
				all := len(l) > 1
				for _, r := range l {
					all = all && c.letter(r) != nil
				}
				if all {
					t.Errorf("%s: --%s is made only of one-letter flags, so -%s cannot be told from them", title, l, l)
				}
			}
		}
		// A subcommand's name or alias names one subcommand only.
		subs := map[string]bool{}
		for _, s := range c.subs {
			for _, w := range append([]string{s.name}, s.aliases...) {
				if subs[w] {
					t.Errorf("%s has two commands called %s", title, w)
				}
				subs[w] = true
				if w == "" || strings.HasPrefix(w, "-") || hasSpace(w) {
					t.Errorf("%s: %q cannot be typed as a command", title, w)
				}
			}
		}
	}

	// gnosh and gnosh run take the same flags: a leading -p is exactly run.
	var run *command
	for _, c := range tab.commands {
		if c.print {
			run = c
		}
	}
	if run == nil || !reflect.DeepEqual(run.flags, tab.root.flags) {
		t.Errorf("gnosh run does not take exactly the flags gnosh does")
	}
}

func indexOf(tab *Table, id string) int {
	for i, f := range tab.flags {
		if f.id == id {
			return i
		}
	}
	return 0
}

// TestTableCoversTheSpecs lists every flag and command of the two specs by
// name, so that dropping one from the table fails here and not in Jae's
// terminal.
func TestTableCoversTheSpecs(t *testing.T) {
	tab := Default()
	have := map[string]bool{}
	for _, e := range tab.Entries() {
		if e.Kind == EntryValue {
			continue
		}
		for _, s := range e.Spellings {
			if e.Kind == EntryFlag {
				for _, c := range e.Commands {
					have[c+" "+s] = true
				}
			} else {
				have["gnosh "+s] = true
			}
		}
	}
	for _, want := range []string{
		// harness-cli.md CLI-4 to CLI-9, on gnosh and on gnosh run.
		"-C", "--continue", "-c", "-s", "--session", "-r", "--resume", "--fork-session", "-n", "--name",
		"-m", "--model", "--small-model", "--reasoning-effort", "--effort",
		"-y", "--yolo", "--dangerously-skip-permissions", "--permission-mode",
		"--allowedTools", "--allowed-tools", "--disallowedTools", "--disallowed-tools", "--tools", "--add-dir", "--trust-folder",
		"--system-prompt", "--system-prompt-file", "--append-system-prompt", "--append-system-prompt-file",
		"--cwd", "-D", "--data-dir", "--safe-mode", "--mcp-config", "--strict-mcp-config", "-d", "--debug",
		"--output-format", "-q", "--quiet", "-v", "--verbose", "--goal",
		// spec/cc/cli.md, P0 and P1.
		"-p", "--print", "--input-format", "--include-partial-messages", "--replay-user-messages",
		"--max-turns", "--no-session-persistence", "--permission-prompts", "--session-id",
		"--allow-dangerously-skip-permissions", "--settings", "--setting-sources", "--bare",
		"--disable-slash-commands", "--agents", "--debug-file", "--version", "-h", "--help",
	} {
		for _, c := range []string{"gnosh", "gnosh run"} {
			if !have[c+" "+want] {
				t.Errorf("%s does not take %s", c, want)
			}
		}
	}
	for _, want := range []string{
		// harness-cli.md CLI-10 to CLI-12.
		"gnosh run", "gnosh r", "gnosh doctor", "gnosh trust", "gnosh models",
		"gnosh session list", "gnosh session show", "gnosh session last", "gnosh session delete", "gnosh session rename",
		"gnosh sessions list", "gnosh s list", "gnosh session ls", "gnosh session rm", "gnosh session web",
		"gnosh mem", "gnosh prompt show", "gnosh prompt path", "gnosh prompt export", "gnosh prompt tools", "gnosh prompt context",
		"gnosh logs", "gnosh dirs", "gnosh ssh", "gnosh projects", "gnosh stats",
		"gnosh trust --list", "gnosh trust --forget",
		"gnosh session list --json", "gnosh session show --json", "gnosh session last --json", "gnosh session delete --json", "gnosh session rename --json",
		"gnosh session web --port", "gnosh prompt export --force", "gnosh prompt tools --json", "gnosh prompt tools --export",
		"gnosh prompt context -s", "gnosh prompt context -C", "gnosh logs -f", "gnosh logs -t",
		"gnosh projects --json", "gnosh stats --all", "gnosh stats --crawl-dir",
		// spec/cc/cli.md section 5, P0 and P1.
		"gnosh mcp add", "gnosh mcp add-json", "gnosh mcp get", "gnosh mcp list", "gnosh mcp remove",
		"gnosh mcp add -s", "gnosh mcp add --scope", "gnosh mcp add -t", "gnosh mcp add --transport",
		"gnosh mcp add -e", "gnosh mcp add --env", "gnosh mcp add -H", "gnosh mcp add --header",
	} {
		if !have[want] {
			t.Errorf("the table has no %s", want)
		}
	}
	// What the specs list as not carried over, or not adopted, is absent.
	for _, gone := range []string{
		"gnosh server", "gnosh login", "gnosh logout", "gnosh update-providers", "gnosh schema",
		"gnosh --channels", "gnosh -H", "gnosh --host", "gnosh --from-pr", "gnosh --betas",
		"gnosh --plugin-dir", "gnosh --plugin-url", "gnosh --prompt-suggestions", "gnosh --file",
		"gnosh auth", "gnosh install", "gnosh plugin", "gnosh mcp serve", "gnosh mcp login",
	} {
		if have[gone] {
			t.Errorf("the table has %s, which the specs leave out", gone)
		}
	}
}

// TestSharedTable parses from many goroutines with one table. Parse and
// Help change nothing in it, which the race detector checks.
func TestSharedTable(t *testing.T) {
	tab := allBuilt(t)
	lines := [][]string{
		{"-y", "-C"},
		{"run", "-q", "--allowedTools", "Bash(git diff *),Edit", "do it"},
		{"session", "list", "--json"},
		{"--alowedTools", "x"},
		{"docter"},
		{"--help"},
		{"mcp", "add", "-e", "A=1", "x", "y"},
	}
	want := make([]string, len(lines))
	for i, l := range lines {
		inv, err := tab.Parse(l, tty)
		want[i] = describe(inv, err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 200; n++ {
				i := n % len(lines)
				inv, err := tab.Parse(lines[i], tty)
				if got := describe(inv, err); got != want[i] {
					t.Errorf("Parse(%q) gave a different answer on another goroutine:\n  %s\n  %s", lines[i], got, want[i])
					return
				}
				if _, err := tab.Help("session", "list"); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

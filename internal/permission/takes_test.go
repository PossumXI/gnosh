package permission

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// takesCall is one call a filter is held against.
type takesCall struct {
	tool   string
	call   core.ToolCall
	action core.Action
}

func shCall(cmd string) takesCall {
	return takesCall{"Bash", coreCall("Bash", "command", cmd), coreAction(cmd)}
}

func fileCall(tool, path string) takesCall {
	kind := core.ActionRead
	if tool == "Edit" || tool == "Write" {
		kind = core.ActionEdit
	}
	return takesCall{tool, coreCall(tool, "file_path", path), core.Action{Kind: kind, Paths: []string{path}}}
}

// A hook's "if" filter takes the calls a deny rule of the same text would
// refuse or could apply to, whatever the session's own rules and mode.
func TestTakes(t *testing.T) {
	// The session allows everything the filters are about, and skips
	// permissions: none of it may change what a filter takes.
	e := engine(t, ModeBypass, rules{allow: []string{"Bash(rm *)", "Edit(src/**)", "Read(./.env)"}})
	cases := []struct {
		name, rule, dir string
		c               takesCall
		want            bool
	}{
		{"the command", "Bash(rm *)", "", shCall("rm -rf build"), true},
		{"the command after another", "Bash(rm *)", "", shCall("ls && rm x"), true},
		{"the command in a wrapper", "Bash(rm *)", "", shCall("timeout 5 rm x"), true},
		{"another command", "Bash(rm *)", "", shCall("ls -la"), false},
		{"a program named by a variable", "Bash(rm *)", "", shCall(`"$tool" x`), true},
		{"a whole tool", "Bash", "", shCall("ls"), true},
		{"a whole tool, another tool's call", "Bash", "", fileCall("Read", "/proj/a.txt"), false},
		{"an edit inside", "Edit(src/**)", "", fileCall("Edit", "/proj/src/a.go"), true},
		{"an edit outside", "Edit(src/**)", "", fileCall("Edit", "/proj/docs/x.md"), false},
		{"a read of the file", "Read(./.env)", "", fileCall("Read", "/proj/.env"), true},
		{"a read of the file by a command", "Read(./.env)", "", shCall("cat .env"), true},
		{"a read of another file", "Read(./.env)", "", fileCall("Read", "/proj/a.txt"), false},
		{"a relative path read from the shell's directory", "Read(./sub/secret)", "/proj/sub", shCall("cat secret"), true},
		{"the same words in the project", "Read(./sub/secret)", "", shCall("cat secret"), false},
		{"every tool of an MCP server", "mcp__notes", "", takesCall{"mcp__notes__add", coreCall("mcp__notes__add", "text", "x"), core.Action{Kind: core.ActionOther}}, true},
	}
	for _, c := range cases {
		got, err := e.Takes(c.rule, c.dir, c.c.tool, c.c.call, c.c.action)
		if err != nil {
			t.Errorf("%s: Takes(%q): %v", c.name, c.rule, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: the filter %q takes %s %s: %v, want %v", c.name, c.rule, c.c.tool, c.c.call.Input, got, c.want)
		}
	}
	// The session's engine is as it was.
	if d := bash(e, "rm -rf build"); d.Behavior != Allow {
		t.Errorf("after the filters, rm -rf build is %v, want allow: %s", d.Behavior, d.Reason)
	}
}

// A rule that cannot be applied is an error, from Takes and ahead of any
// call from CheckRule.
func TestTakesBadRule(t *testing.T) {
	e := engine(t, ModeDefault, rules{})
	for _, rule := range []string{"Bash(", "Nonesuch(x)"} {
		if err := e.CheckRule(rule); err == nil {
			t.Errorf("CheckRule(%q) found nothing wrong", rule)
		}
		if _, err := e.Takes(rule, "", "Bash", coreCall("Bash", "command", "ls"), coreAction("ls")); err == nil {
			t.Errorf("Takes(%q) found nothing wrong", rule)
		}
	}
	if err := e.CheckRule("Bash(git push *)"); err != nil {
		t.Errorf("CheckRule of a good rule: %v", err)
	}
}

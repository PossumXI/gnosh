package permission

import (
	"strings"
	"testing"
)

// blockOptions are the usual test options with the blocks on, an allow
// rule for everything under /etc, one of the person's own additional
// directories (/shared) and one only the project's file adds (/theirs).
func blockOptions(mode Mode, reads, writes bool) Options {
	o := testOptions(mode, rules{allow: []string{"Read(//etc/**)", "Edit(//etc/**)", "Bash"}})
	o.Layers = append(o.Layers,
		Layer{Scope: ScopeCLI, Name: "additional directories", Dirs: []string{"/shared"}},
		Layer{Scope: ScopeProject, FromProject: true, Name: "the project's additional directories", Dirs: []string{"/theirs"}})
	o.BlockReads, o.BlockWrites = reads, writes
	o.OwnReads = []string{"/home/u/.local/share/gnosh/sessions/proj", "/home/u/.config/gnosh/skills"}
	o.SettingsFiles = []string{"/home/u/.config/gnosh/gnosh.json", "/proj/gnosh.json", "/proj/.gnosh/settings.local.json"}
	return o
}

// With blockReadsOutsideWorkingDirectories, Read, Grep and Glob refuse a
// path outside the working directories in every mode, bypassPermissions
// too, whatever an allow rule says; the working directories are the
// primary one and those the person added, not those only the project's
// file adds; gnosh's own files stay readable. Claude Code's
// permissions key of the same name.
func TestReadBlockRefusesFileToolsOutside(t *testing.T) {
	for _, mode := range []Mode{ModeDefault, ModeAcceptEdits, ModeBypass, ModeDontAsk, ModePlan} {
		e := mustEngine(t, blockOptions(mode, true, false))
		for _, c := range []struct{ tool, path string }{
			{"Read", "/etc/passwd"}, {"Grep", "/etc"}, {"Glob", "/home/u"}, {"Read", "/theirs/a.txt"},
			{"Read", "/home/u/.config/gnosh/gnosh.json"},
		} {
			d := file(e, c.tool, c.path)
			if d.Behavior != Deny || d.Stage != StageBlock || !strings.Contains(d.Reason, "blockReadsOutsideWorkingDirectories") || !strings.Contains(d.Reason, "/add-dir") {
				t.Errorf("%s: %s %s = %+v, want refused by the block", mode, c.tool, c.path, d)
			}
		}
		for _, c := range []struct{ tool, path string }{
			{"Read", "/proj/main.go"}, {"Grep", "/proj"}, {"Read", "/shared/notes.md"},
			{"Read", "/home/u/.local/share/gnosh/sessions/proj/abc/tool-results/out.txt"},
			{"Read", "/home/u/.config/gnosh/skills/deploy/SKILL.md"},
		} {
			// (Not blocked; the mode judges it as before, and dontAsk
			// refuses what would ask.)
			if d := file(e, c.tool, c.path); d.Stage == StageBlock || d.Behavior == Deny && mode != ModeDontAsk {
				t.Errorf("%s: %s %s = %+v, want it not blocked", mode, c.tool, c.path, d)
			}
		}
	}
	// What /add-dir adds counts.
	e := mustEngine(t, blockOptions(ModeBypass, true, false))
	if err := e.AddDir("/etc"); err != nil {
		t.Fatal(err)
	}
	if d := file(e, "Read", "/etc/passwd"); d.Behavior != Allow {
		t.Errorf("after /add-dir /etc: %+v", d)
	}
	// Search results and file discovery leave a blocked file out.
	if !e.ReadDenied("/var/log/syslog") || e.ReadDenied("/proj/a.go") || e.ReadDenied("/home/u/.config/gnosh/skills/x/y.md") {
		t.Error("ReadDenied does not follow the read block")
	}
	// Without the block, nothing of the kind.
	off := mustEngine(t, blockOptions(ModeBypass, false, false))
	if d := file(off, "Read", "/var/log/syslog"); d.Behavior != Allow {
		t.Errorf("no block, bypassPermissions: %+v", d)
	}
}

// A link inside the working directory that leads out of it is outside.
func TestReadBlockFollowsLinks(t *testing.T) {
	o := blockOptions(ModeBypass, true, true)
	fs := fakeFS{links: map[string]string{"/proj/out": "/var/secret"}}
	o.Resolve = fs.resolve
	e := mustEngine(t, o)
	if d := file(e, "Read", "/proj/out/key.pem"); d.Behavior != Deny || d.Stage != StageBlock {
		t.Errorf("a read through a link out: %+v", d)
	}
	if d := file(e, "Write", "/proj/out/key.pem"); d.Behavior != Deny || d.Stage != StageBlock {
		t.Errorf("a write through a link out: %+v", d)
	}
}

// With blockWritesOutsideWorkingDirectories, Write, Edit and NotebookEdit
// refuse a path outside the working directories in every mode, and a
// settings file wherever it is; EnterWorktree and ExitWorktree are off.
// Shell commands are not held by it, as in Claude Code.
func TestWriteBlockRefusesFileToolsOutside(t *testing.T) {
	for _, mode := range []Mode{ModeDefault, ModeAcceptEdits, ModeBypass} {
		e := mustEngine(t, blockOptions(mode, false, true))
		for _, c := range []struct{ tool, path, says string }{
			{"Write", "/etc/hosts", "outside the working directories"},
			{"Edit", "/theirs/x.go", "outside the working directories"},
			{"NotebookEdit", "/tmp/n.ipynb", "outside the working directories"},
			{"Write", "/proj/gnosh.json", "settings file"},
			{"Edit", "/proj/.gnosh/settings.local.json", "settings file"},
		} {
			d := file(e, c.tool, c.path)
			if d.Behavior != Deny || d.Stage != StageBlock || !strings.Contains(d.Reason, c.says) || !strings.Contains(d.Reason, "blockWritesOutsideWorkingDirectories") {
				t.Errorf("%s: %s %s = %+v", mode, c.tool, c.path, d)
			}
		}
		if d := file(e, "Write", "/proj/main.go"); d.Stage == StageBlock {
			t.Errorf("%s: a write inside: %+v", mode, d)
		}
		if d := file(e, "Edit", "/shared/notes.md"); d.Stage == StageBlock {
			t.Errorf("%s: a write in the person's added folder: %+v", mode, d)
		}
		if d := bash(e, "echo hi > /tmp/x"); d.Stage == StageBlock {
			t.Errorf("%s: a shell command was held by the write block: %+v", mode, d)
		}
		for _, tool := range []string{"EnterWorktree", "ExitWorktree"} {
			if e.ToolEnabled(tool) {
				t.Errorf("%s: %s is offered under the write block", mode, tool)
			}
		}
	}
	if e := mustEngine(t, blockOptions(ModeDefault, false, false)); !e.ToolEnabled("EnterWorktree") {
		t.Error("EnterWorktree is off without the block")
	}
}

// Under the read block a shell command that reads outside the working
// directories asks in every mode, bypassPermissions too (and dontAsk
// refuses it), whatever an allow rule says; so does one whose files gnosh
// cannot tell. A command that reads inside runs as before.
func TestReadBlockAsksAboutShellReadsOutside(t *testing.T) {
	for _, mode := range []Mode{ModeDefault, ModeBypass, ModeDontAsk} {
		e := mustEngine(t, blockOptions(mode, true, false))
		want := Ask
		if mode == ModeDontAsk {
			want = Deny
		}
		for _, cmd := range []string{"cat /etc/hosts", "head -5 /var/log/syslog", "grep -r key ~/.ssh", "wc -l < /etc/passwd"} {
			d := bash(e, cmd)
			if d.Behavior != want || d.Stage != StageBlock || !strings.Contains(d.Reason, "blockReadsOutsideWorkingDirectories") {
				t.Errorf("%s: %q = %+v, want %s by the block", mode, cmd, d, want)
			}
		}
		if d := bash(e, `cd "$(cat .where)" && cat notes`); d.Behavior != want || d.Stage != StageBlock || !strings.Contains(d.Reason, "cannot tell") {
			t.Errorf("%s: a command whose folder cannot be told = %+v", mode, d)
		}
		for _, cmd := range []string{"cat main.go", "ls /proj", "head /shared/notes.md"} {
			if d := bash(e, cmd); d.Stage == StageBlock {
				t.Errorf("%s: %q = %+v, want it not blocked", mode, cmd, d)
			}
		}
	}
}

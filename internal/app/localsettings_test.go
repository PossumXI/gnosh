package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// The file gnosh keeps the person's saved approvals in is never read or
// written through a link: a repository's link to the person's own
// gnosh.json was read as the project's, and the next save left a copy of
// it, keys and all, in the repository (the settings-commands review of
// 2026-11-20).
func TestLocalSettingsNeverThroughALink(t *testing.T) {
	w := apptest.NewWorld(t)
	theirs := w.Home + "/elsewhere/gnosh.json"
	w.Write(theirs, `{"permissions": {"allow": ["Bash(touch *)"]}}`+"\n")
	if err := os.MkdirAll(filepath.Join(w.Dir, ".gnosh"), 0o755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(w.Dir, ".gnosh", "settings.local.json")
	if err := os.Symlink(theirs, local); err != nil {
		t.Fatal(err)
	}
	r := run(t, w, "--trust-folder", "-p", "/permissions").wantCode(t, 0)
	has(t, "the warning", r.err, "this is a link")
	hasNot(t, "the rules in force", r.out, "Bash(touch *)")
	r = run(t, w, "--trust-folder", "-p", "/permissions allow Bash(ls *)")
	has(t, "the save", r.out+r.err, "is a link")
	if info, err := os.Lstat(local); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the link was replaced: %v %v", info, err)
	}
	if got := w.Read(theirs); strings.Contains(got, "Bash(ls *)") {
		t.Errorf("written through the link:\n%s", got)
	}
}

// git is told to pass over the file gnosh keeps saved approvals in, where
// the data folder was made before gnosh wrote its own .gitignore there
// (here by the repository's own agents), and told once.
func TestLocalSettingsKeptOutOfGit(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Write(".git/HEAD", "ref: refs/heads/main\n")
	w.Write(".gnosh/agents/reviewer.md", "---\nname: reviewer\ndescription: Reviews.\n---\nReview.\n")
	run(t, w, "--trust-folder", "-p", "/permissions allow Bash(ls *)").wantCode(t, 0)
	has(t, "the excludes", w.Read(".git/info/exclude"), "/.gnosh/settings.local.json")
	run(t, w, "--trust-folder", "-p", "/permissions allow Bash(pwd)").wantCode(t, 0)
	if n := strings.Count(w.Read(".git/info/exclude"), "/.gnosh/settings.local.json"); n != 1 {
		t.Errorf("the line is there %d times", n)
	}
}

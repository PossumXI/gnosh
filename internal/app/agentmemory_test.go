package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/config"
)

// A memory's index is given as far as its first 200 lines or 25 KB go,
// whichever ends first, and said to be cut when it is; a local memory's
// folder is made with a .gitignore that keeps it out of git.
func TestAgentMemoryIndex(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, ".gnosh", memoryLocalFolder, "rev")
	open := func(content string) (string, bool) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, memoryIndex), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		index, cut, err := openAgentMemory(dir, base, "local")
		if err != nil {
			t.Fatal(err)
		}
		return index, cut
	}
	lines := func(n int) string { return strings.Repeat("- a note\n", n) }
	if index, cut := open(lines(200)); cut || strings.Count(index, "\n") != 200 {
		t.Errorf("200 lines: %d lines, cut %v", strings.Count(index, "\n"), cut)
	}
	if index, cut := open(lines(250)); !cut || strings.Count(index, "\n") != 200 {
		t.Errorf("250 lines: %d lines, cut %v", strings.Count(index, "\n"), cut)
	}
	long := strings.Repeat("é", 20_000) // 40,000 bytes on one line
	if index, cut := open(long); !cut || len(index) > memoryIndexBytes || !utf8.ValidString(index) || index == "" {
		t.Errorf("one long line: %d bytes, cut %v, valid %v", len(index), cut, utf8.ValidString(index))
	}
	if index, cut := open(lines(10) + long); !cut || index != lines(10) {
		t.Errorf("a long line after ten: %q, cut %v", index, cut)
	}
	if got, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err != nil || !strings.Contains(string(got), "\n*\n") {
		t.Errorf("the local memory's .gitignore: %q %v", got, err)
	}
	if err := os.Remove(filepath.Join(dir, memoryIndex)); err != nil {
		t.Fatal(err)
	}
	if index, cut, err := openAgentMemory(dir, base, "local"); index != "" || cut || err != nil {
		t.Errorf("no index: %q %v %v", index, cut, err)
	}
}

// A kind's autoCompactWindow holds where it is the lower; the session's,
// set lower, wins (Claude Code takes the lowest).
func TestLowerWindow(t *testing.T) {
	for _, c := range []struct {
		kind    int
		session config.Value[int]
		want    int
	}{
		{200000, config.Value[int]{}, 200000},
		{200000, config.Value[int]{V: 150000, Set: true}, 150000},
		{200000, config.Value[int]{V: 500000, Set: true}, 200000},
	} {
		if got := lowerWindow(c.kind, c.session); got != c.want {
			t.Errorf("lowerWindow(%d, %+v) = %d, want %d", c.kind, c.session, got, c.want)
		}
	}
}

// Started in the home folder, its .claude folder's definitions are the
// person's own: not the folder's (held until it is trusted), nor read
// twice; its .gnosh folder is still the folder's.
func TestHomeFolderDefinitionsAreThePersons(t *testing.T) {
	cfg := &config.Config{Paths: config.Paths{Home: "/h", Cwd: "/h", ProjectRoot: "/h", ConfigDir: "/h/.config/gnosh"}}
	if got := strings.Join(projectDefDirs(cfg, "agents", nil), " "); got != "/h/.gnosh/agents" {
		t.Errorf("in the home folder: %s", got)
	}
	cfg.Paths.Cwd, cfg.Paths.ProjectRoot = "/h/proj", "/h/proj"
	if got := strings.Join(projectDefDirs(cfg, "skills", nil), " "); got != "/h/proj/.gnosh/skills /h/proj/.claude/skills" {
		t.Errorf("in a project: %s", got)
	}
}

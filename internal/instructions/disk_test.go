package instructions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The tests in this file use the real disk, through the default file
// system. Loading walks every directory above the temporary one, so the
// names looked for are ones no machine has lying about.
const (
	diskMain = "GNOSH-TEST-INSTRUCTIONS.md"
	diskDot  = ".gnosh-test-instructions"
)

// diskTree writes files under a new temporary directory and returns it.
func diskTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// under strips the temporary directory from the front of each path.
func under(dir string, ps []string) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = filepath.ToSlash(strings.TrimPrefix(p, dir+string(filepath.Separator)))
	}
	return out
}

func TestRealDisk(t *testing.T) {
	dir := diskTree(t, map[string]string{
		"home/user.md":                    "user; @notes.md\n",
		"home/notes.md":                   "notes\n",
		"outside/secret.md":               "secret\n",
		diskMain:                          "above the project, and mine\n",
		diskDot:                           "a file where a dot directory could be\n",
		"proj/" + diskMain:                "proj; @docs/a.md\n",
		"proj/docs/a.md":                  "a\n",
		"proj/" + diskDot + "/rules/r.md": "---\npaths: \"*.go\"\n---\nrule\n",
		"proj/sub/" + diskMain:            "sub\n",
		"proj/sub/x.go":                   "package x\n",
	})
	// A link that stays inside the project, and one that leaves it.
	if err := os.Symlink(filepath.Join("docs", "a.md"), filepath.Join(dir, "proj", "IN-"+diskMain)); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	if err := os.Symlink(filepath.Join(dir, "outside", "secret.md"), filepath.Join(dir, "proj", "OUT-"+diskMain)); err != nil {
		t.Fatal(err)
	}
	o := Options{
		Cwd:          filepath.Join(dir, "proj"),
		Home:         filepath.Join(dir, "home"),
		UserFiles:    []string{filepath.Join(dir, "home", "user.md")},
		ProjectNames: []string{diskMain, "IN-" + diskMain, "OUT-" + diskMain},
		DotDirs:      []string{diskDot},
		Trusted:      true,
	}
	s := load(t, o)
	same(t, "files", under(dir, paths(s.Files())), []string{
		"home/user.md",
		"home/notes.md",
		diskMain,
		"proj/" + diskMain,
		"proj/docs/a.md",
	})
	// The temporary directory may itself be reached through a link (it is
	// on macOS), so the refused file is named by where it really is.
	secret, err := filepath.EvalSymlinks(filepath.Join(dir, "outside", "secret.md"))
	if err != nil {
		t.Fatal(err)
	}
	same(t, "warnings", warned(s.Warnings()), []string{"import_refused " + secret})

	late, _ := touch(s, filepath.Join(dir, "proj", "sub", "x.go"))
	same(t, "late", under(dir, paths(late)), []string{"proj/sub/" + diskMain, "proj/" + diskDot + "/rules/r.md"})
	// The same file under its real path is the same file.
	real, err := filepath.EvalSymlinks(filepath.Join(dir, "proj", "sub", "x.go"))
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := touch(s, real); len(again) != 0 {
		t.Errorf("given twice: %v", paths(again))
	}

	ctx := context.Background()
	same(t, "nothing changed", s.Changed(ctx), nil)
	main := filepath.Join(dir, "proj", diskMain)
	if err := os.WriteFile(main, []byte("proj, edited; @docs/a.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	same(t, "an edit", s.Changed(ctx), []string{main})
	// The old content again, saved an hour later: the size is the same and
	// the time is not.
	if err := os.WriteFile(main, []byte("proj; @docs/a.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(main, later, later); err != nil {
		t.Fatal(err)
	}
	same(t, "a save", s.Changed(ctx), []string{main})
	if err := os.Remove(filepath.Join(dir, "home", "notes.md")); err != nil {
		t.Fatal(err)
	}
	same(t, "a removal", s.Changed(ctx), []string{filepath.Join(dir, "home", "notes.md"), main})
}

func TestRealDiskUnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read every file")
	}
	dir := diskTree(t, map[string]string{diskMain: "locked\n", "IN-" + diskMain: "open\n"})
	if err := os.Chmod(filepath.Join(dir, diskMain), 0); err != nil {
		t.Fatal(err)
	}
	s := load(t, Options{Cwd: dir, ProjectNames: []string{diskMain, "IN-" + diskMain}, Trusted: true})
	same(t, "files", under(dir, paths(s.Files())), []string{"IN-" + diskMain})
	ws := s.Warnings()
	if len(ws) != 1 || ws[0].Kind != WarnUnreadable || !strings.Contains(ws[0].Text, "permission was denied") {
		t.Errorf("warnings: %v", ws)
	}
}

func TestFileSystemWithoutLinks(t *testing.T) {
	// A file system that cannot report links is read as one with none.
	fsys := tree(map[string]string{"/p/CLAUDE.md": "plain; @a.md\n", "/p/a.md": "a\n"})
	s := load(t, Options{Cwd: "/p", ProjectNames: []string{"CLAUDE.md"}, Trusted: true, FS: onlyFS{fsys}})
	same(t, "files", paths(s.Files()), []string{"/p/CLAUDE.md", "/p/a.md"})
	same(t, "changed", s.Changed(context.Background()), nil)
}

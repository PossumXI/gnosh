package instructions

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLinksThatLeaveTheProject(t *testing.T) {
	o := Options{
		Cwd:          "/work/shop",
		ProjectNames: []string{"CLAUDE.md"},
		DotDirs:      []string{".gnosh"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/work/shop/CLAUDE.md":           "-> /etc/secret.md",
			"/work/shop/.gnosh/rules/own.md": "own rule\n",
			"/work/shop/.gnosh/rules/shared": "-> /shared/rules",
			"/shared/rules/x.md":             "shared rule\n",
			"/shared/rules/more":             "-> /other/place",
			"/other/place/y.md":              "a link out of the directory that was let in\n",
			"/etc/secret.md":                 "secret\n",
		}),
	}
	// A repository can hold a link under an instruction file's name that
	// points at a private file. It is treated as an import from outside.
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/work/shop/.gnosh/rules/own.md"})
	same(t, "warnings", warned(s.Warnings()), []string{
		"import_refused /etc/secret.md",
		"import_refused /shared/rules",
	})
	ws := s.Warnings()
	if ws[0].From != "/work/shop/CLAUDE.md" || !strings.Contains(ws[0].Text, "/work/shop/CLAUDE.md leads, through a link, to /etc/secret.md") {
		t.Errorf("warning: %+v", ws[0])
	}

	// A linked directory is asked about once, as a whole, and is not
	// searched unless the answer is yes.
	a := &asked{yes: map[string]bool{"/etc/secret.md": true, "/shared/rules": true}}
	o.ApproveImport = a.approve
	s = load(t, o)
	same(t, "asked", a.calls, []string{
		"/work/shop/CLAUDE.md -> /etc/secret.md",
		"/work/shop/.gnosh/rules/shared -> /shared/rules",
		"/work/shop/.gnosh/rules/shared/more -> /other/place",
	})
	same(t, "warnings", warned(s.Warnings()), []string{"import_refused /other/place"})
	same(t, "files", paths(s.Files()), []string{
		"/work/shop/CLAUDE.md",
		"/work/shop/.gnosh/rules/own.md",
		"/work/shop/.gnosh/rules/shared/x.md",
	})
	if f := s.Files()[2]; f.RealPath != "/shared/rules/x.md" || f.Kind != KindRule {
		t.Errorf("the shared rule: %+v", f)
	}
}

func TestLinkedFileImportsFromWhereItLives(t *testing.T) {
	o := Options{
		Cwd:          "/work/shop",
		ProjectNames: []string{"CLAUDE.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/work/shop/CLAUDE.md":         "-> docs/ai/main.md",
			"/work/shop/docs/ai/main.md":   "@style.md\n",
			"/work/shop/docs/ai/style.md":  "the right one\n",
			"/work/shop/style.md":          "the wrong one\n",
			"/work/shop/docs/ai/unused.md": "",
		}),
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/work/shop/CLAUDE.md", "/work/shop/docs/ai/style.md"})
	if f := s.Files()[0]; f.RealPath != "/work/shop/docs/ai/main.md" {
		t.Errorf("real path %q", f.RealPath)
	}
}

func TestLinkLoopsEnd(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md", "GNOSH.md", "LOST.md"},
		DotDirs:      []string{".gnosh"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/CLAUDE.md":                "-> b.md",
			"/p/b.md":                     "-> CLAUDE.md",
			"/p/GNOSH.md":                 "fine; @b.md\n",
			"/p/LOST.md":                  "-> nowhere.md",
			"/p/.gnosh/rules/dangling.md": "-> nowhere.md",
			"/p/.gnosh/rules/loop":        "-> .",
			"/p/.gnosh/rules/r.md":        "rule\n",
			"/p/.gnosh/rules/up":          "-> ..",
			"/p/sub/.gnosh/rules/back":    "-> ../../..",
			"/p/sub/x":                    "",
		}),
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/GNOSH.md", "/p/.gnosh/rules/r.md"})
	same(t, "warnings", warned(s.Warnings()), []string{
		"unreadable /p/CLAUDE.md",
		"unreadable /p/b.md",
		"unreadable /p/LOST.md",
		"unreadable /p/.gnosh/rules/dangling.md",
	})
	if w := s.Warnings()[2]; w.Text != "/p/LOST.md is a link that leads nowhere." {
		t.Errorf("warning: %s", w)
	}
	if w := s.Warnings()[0]; !strings.Contains(w.Text, "too many symbolic links") {
		t.Errorf("warning: %s", w)
	}

	// A rules directory below the working directory that links back up to
	// the project: every file under it is one already held, and the walk
	// ends.
	late, _ := touch(s, "/p/sub/x")
	same(t, "late", paths(late), nil)
}

func TestWorkingDirectoryReachedThroughALink(t *testing.T) {
	o := Options{
		Cwd:          "/link/proj",
		ProjectRoot:  "/real/base/proj", // as a tool that follows links would report it
		ProjectNames: []string{"CLAUDE.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/link":                         "-> /real/base",
			"/CLAUDE.md":                    "top\n",
			"/real/CLAUDE.md":               "not on the way the person came\n",
			"/real/base/CLAUDE.md":          "base; @notes.md\n",
			"/real/base/notes.md":           "notes\n",
			"/real/base/proj/CLAUDE.md":     "proj; @docs/a.md\n",
			"/real/base/proj/docs/a.md":     "a\n",
			"/real/base/proj/sub/CLAUDE.md": "sub\n",
			"/real/base/proj/sub/x.go":      "",
		}),
	}
	a := &asked{yes: map[string]bool{"/real/base/notes.md": true}}
	o.ApproveImport = a.approve
	s := load(t, o)
	// The directories above are the ones in the path the person gave.
	// Paths are shown that way too; the real ones are kept beside them.
	same(t, "files", paths(s.Files()), []string{
		"/CLAUDE.md",
		"/link/CLAUDE.md",
		"/link/notes.md",
		"/link/proj/CLAUDE.md",
		"/link/proj/docs/a.md",
	})
	if f := s.Files()[3]; f.RealPath != "/real/base/proj/CLAUDE.md" || f.Base != "/link/proj" {
		t.Errorf("the project file: %+v", f)
	}
	same(t, "asked", a.calls, []string{"/link/CLAUDE.md -> /real/base/notes.md"})

	// A touched file is recognised however its path is spelled.
	late, _ := touch(s, "/real/base/proj/sub/x.go")
	same(t, "late", paths(late), []string{"/link/proj/sub/CLAUDE.md"})
	if again, _ := touch(s, "/link/proj/sub/x.go"); len(again) != 0 {
		t.Errorf("given twice: %v", paths(again))
	}
}

func TestLinkInsideTheProjectIsFollowedToWhereTheFileIs(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/short":              "-> pkg/deep",
			"/p/pkg/CLAUDE.md":      "pkg\n",
			"/p/pkg/deep/CLAUDE.md": "deep\n",
			"/p/pkg/deep/x.go":      "",
			"/p/out":                "-> /elsewhere",
			"/elsewhere/CLAUDE.md":  "outside\n",
			"/elsewhere/y.go":       "",
		}),
	}
	s := load(t, o)
	late, _ := touch(s, "/p/short/x.go")
	same(t, "through a link that stays inside", paths(late), []string{"/p/pkg/CLAUDE.md", "/p/pkg/deep/CLAUDE.md"})
	late, warns := touch(s, "/p/out/y.go")
	same(t, "through a link that leads out", paths(late), nil)
	same(t, "warnings", warned(warns), nil)
}

// listedFS records which directories were listed.
type listedFS struct {
	fstest.MapFS
	listed map[string]int
}

func (l listedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	l.listed[name]++
	return l.MapFS.ReadDir(name)
}

func TestLinkedDirectoriesAreNotSearchedUnasked(t *testing.T) {
	mine := func(info fs.FileInfo) bool { return info.Sys() != "stranger" }
	fsys := listedFS{listed: map[string]int{}, MapFS: fstest.MapFS{
		// In the project: a rules directory that is a link to the top of
		// the disk, and a dot directory that is a link out.
		"tmp/work/.claude/rules/all":  {Data: []byte("/"), Mode: fs.ModeSymlink | 0o777},
		"tmp/work/.gnosh":             {Data: []byte("/elsewhere/dot"), Mode: fs.ModeSymlink | 0o777},
		"tmp/work/.claude/rules/r.md": {Data: []byte("the project's own rule\n"), Mode: 0o644},
		"elsewhere/dot/rules/x.md":    {Data: []byte("outside\n"), Mode: 0o644},
		"elsewhere/big/tree/y.md":     {Data: []byte("never looked at\n"), Mode: 0o644},
		// Above the project, in a shared directory: a stranger's dot
		// directory, and a stranger's link to the top of the disk.
		"tmp/.claude/rules/planted.md": {Data: []byte("planted\n"), Mode: 0o644, Sys: "stranger"},
		"tmp/.claude":                  {Mode: fs.ModeDir | 0o755, Sys: "stranger"},
		"tmp/.gnosh":                   {Data: []byte("/"), Mode: fs.ModeSymlink | 0o777, Sys: "stranger"},
	}}
	o := Options{
		Cwd:     "/tmp/work",
		DotDirs: []string{".claude", ".gnosh"},
		Owned:   mine,
		Trusted: true,
		FS:      fsys,
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/tmp/work/.claude/rules/r.md"})
	same(t, "warnings", warned(s.Warnings()), []string{
		"not_owned /tmp/.claude/rules",
		"import_refused /",
		"import_refused /elsewhere/dot/rules",
	})
	for name := range fsys.listed {
		if name != "tmp/work/.claude/rules" {
			t.Errorf("the directory %q was searched", name)
		}
	}
}

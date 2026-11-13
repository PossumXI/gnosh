package instructions

import (
	"context"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"
)

func TestChanged(t *testing.T) {
	fsys := tree(map[string]string{
		"/home/ana/GNOSH.md":     "user\n",
		"/p/CLAUDE.md":           "top; @docs/a.md @docs/later.md\n",
		"/p/docs/a.md":           "a\n",
		"/p/.claude/rules/r.md":  "rule\n",
		"/p/.claude/rules/go.md": "---\npaths: \"*.go\"\n---\ngo\n",
		"/p/sub/CLAUDE.md":       "sub\n",
		"/p/sub/x":               "",
	})
	o := Options{
		Cwd:          "/p",
		UserFiles:    []string{"/home/ana/GNOSH.md"},
		ProjectNames: []string{"CLAUDE.md"},
		LocalNames:   []string{"CLAUDE.local.md"},
		DotDirs:      []string{".claude"},
		Trusted:      true,
		FS:           fsys,
	}
	ctx := context.Background()
	file := func(content string) *fstest.MapFile {
		return &fstest.MapFile{Data: []byte(content), Mode: 0o644, ModTime: t0}
	}

	s := load(t, o)
	touch(s, "/p/sub/x")
	same(t, "untouched", s.Changed(ctx), nil)

	// Each change is made, seen, and undone.
	for _, c := range []struct {
		what   string
		change func() (undo func())
		want   []string
	}{
		{"a loaded file grows", func() func() {
			fsys["p/CLAUDE.md"] = file("top, and more; @docs/a.md @docs/later.md\n")
			return func() { fsys["p/CLAUDE.md"] = file("top; @docs/a.md @docs/later.md\n") }
		}, []string{"/p/CLAUDE.md"}},
		{"a loaded file is saved again with the same size", func() func() {
			fsys["p/docs/a.md"].ModTime = t0.Add(time.Second)
			return func() { fsys["p/docs/a.md"].ModTime = t0 }
		}, []string{"/p/docs/a.md"}},
		{"the person's file is removed", func() func() {
			old := fsys["home/ana/GNOSH.md"]
			delete(fsys, "home/ana/GNOSH.md")
			return func() { fsys["home/ana/GNOSH.md"] = old }
		}, []string{"/home/ana/GNOSH.md"}},
		{"a file appears under a name that was looked for", func() func() {
			fsys["p/CLAUDE.local.md"] = file("private\n")
			return func() { delete(fsys, "p/CLAUDE.local.md") }
		}, []string{"/p/CLAUDE.local.md"}},
		{"a missing import appears", func() func() {
			fsys["p/docs/later.md"] = file("later\n")
			return func() { delete(fsys, "p/docs/later.md") }
		}, []string{"/p/docs/later.md"}},
		{"a rule is added", func() func() {
			fsys["p/.claude/rules/new.md"] = file("new\n")
			return func() { delete(fsys, "p/.claude/rules/new.md") }
		}, []string{"/p/.claude/rules"}},
		{"a rule that waits for a match is edited", func() func() {
			fsys["p/.claude/rules/go.md"] = file("---\npaths: \"*.go\"\n---\ngo, edited\n")
			return func() { fsys["p/.claude/rules/go.md"] = file("---\npaths: \"*.go\"\n---\ngo\n") }
		}, []string{"/p/.claude/rules/go.md"}},
		{"a late file is edited", func() func() {
			fsys["p/sub/CLAUDE.md"] = file("sub, edited\n")
			return func() { fsys["p/sub/CLAUDE.md"] = file("sub\n") }
		}, []string{"/p/sub/CLAUDE.md"}},
		{"a file becomes a link to another", func() func() {
			old := fsys["p/docs/a.md"]
			fsys["p/docs/a.md"] = &fstest.MapFile{Data: []byte("../CLAUDE.md"), Mode: fs.ModeSymlink | 0o777}
			return func() { fsys["p/docs/a.md"] = old }
		}, []string{"/p/docs/a.md"}},
		{"a rules directory appears below", func() func() {
			fsys["p/sub/.claude/rules/s.md"] = file("s\n")
			return func() { delete(fsys, "p/sub/.claude/rules/s.md") }
		}, []string{"/p/sub/.claude/rules"}},
	} {
		undo := c.change()
		same(t, c.what, s.Changed(ctx), c.want)
		undo()
		same(t, c.what+", undone", s.Changed(ctx), nil)
	}

	// A file that is not an instruction file can change freely.
	fsys["p/sub/x"] = file("edited\n")
	fsys["p/README.md"] = file("new\n")
	same(t, "other files", s.Changed(ctx), nil)
}

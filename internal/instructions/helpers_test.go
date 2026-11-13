package instructions

import (
	"context"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// t0 is the modification time of every file in a test tree.
var t0 = time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)

// tree builds an in-memory file system from absolute paths and contents. A
// content of the form "-> target" makes a symbolic link.
func tree(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, content := range files {
		name := strings.TrimPrefix(p, "/")
		if target, ok := strings.CutPrefix(content, "-> "); ok {
			m[name] = &fstest.MapFile{Data: []byte(target), Mode: fs.ModeSymlink | 0o777, ModTime: t0}
			continue
		}
		m[name] = &fstest.MapFile{Data: []byte(content), Mode: 0o644, ModTime: t0}
	}
	return m
}

// load calls Load and fails the test on an error.
func load(t *testing.T, o Options) *Set {
	t.Helper()
	s, err := Load(context.Background(), o)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

// touch calls ForPath with a background context.
func touch(s *Set, p string) ([]File, []Warning) {
	return s.ForPath(context.Background(), p)
}

// brief writes each file as "path scope kind reason".
func brief(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path + " " + string(f.Scope) + " " + string(f.Kind) + " " + string(f.Reason)
	}
	return out
}

// paths lists the files' paths.
func paths(files []File) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

// warned writes each warning as "kind path".
func warned(ws []Warning) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = string(w.Kind) + " " + w.Path
	}
	return out
}

// same fails the test when got and want differ, showing both.
func same(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n got:\n\t%s\nwant:\n\t%s", what, strings.Join(got, "\n\t"), strings.Join(want, "\n\t"))
	}
}

// find returns the loaded file at path p, failing the test if there is none.
func find(t *testing.T, files []File, p string) File {
	t.Helper()
	for _, f := range files {
		if f.Path == p {
			return f
		}
	}
	t.Fatalf("no file %s among %v", p, paths(files))
	return File{}
}

// onlyFS hides every method of a file system but Open, so that it cannot
// report symbolic links.
type onlyFS struct{ fsys fs.FS }

func (o onlyFS) Open(name string) (fs.File, error) { return o.fsys.Open(name) }

package app

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// /memory's picker offers the files the model is given and the project's
// GNOSH.md, which is marked when it is not there yet.
func TestMemoryFiles(t *testing.T) {
	w := apptest.NewWorld(t)
	var got []MemoryFile
	_, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		got = c.MemoryFiles()
		return 0
	})
	if r.code != 0 {
		t.Fatalf("exit code %d", r.code)
	}
	i := slices.IndexFunc(got, func(m MemoryFile) bool {
		return filepath.Base(m.Path) == "GNOSH.md" && strings.Contains(m.About, "project")
	})
	if i < 0 {
		t.Fatalf("no project GNOSH.md among %+v", got)
	}
}

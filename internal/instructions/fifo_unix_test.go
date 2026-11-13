//go:build unix

package instructions

import (
	"context"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRealDiskPipeIsNotOpened(t *testing.T) {
	// A named pipe where an instruction file should be: opening it would
	// wait for a writer for ever.
	dir := diskTree(t, map[string]string{"IN-" + diskMain: "see @" + diskMain + ".pipe\n"})
	for _, name := range []string{diskMain, diskMain + ".pipe"} {
		if err := syscall.Mkfifo(filepath.Join(dir, name), 0o644); err != nil {
			t.Skipf("no named pipes here: %v", err)
		}
	}
	done := make(chan *Set, 1)
	go func() {
		s, _ := Load(context.Background(), Options{Cwd: dir, ProjectNames: []string{diskMain, "IN-" + diskMain}, Trusted: true})
		done <- s
	}()
	select {
	case s := <-done:
		if s == nil {
			t.Fatal("Load failed")
		}
		same(t, "files", under(dir, paths(s.Files())), []string{"IN-" + diskMain})
		same(t, "warnings", warned(s.Warnings()), []string{
			"not_file " + filepath.Join(dir, diskMain),
			"not_file " + filepath.Join(dir, diskMain+".pipe"),
		})
	case <-time.After(10 * time.Second):
		t.Fatal("loading is stuck on a named pipe")
	}
}

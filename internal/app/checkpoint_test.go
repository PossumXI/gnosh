package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// A session keeps the copies of its checkpointPrompts most recent prompts:
// older ones are let go, noted as such, and still noted when the session
// is opened again, so that a rewind of the files to before one of them is
// refused instead of putting back too little.
func TestCheckpointsLetGoOfOldPrompts(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "work", "f.txt")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	now := func() time.Time { clock = clock.Add(time.Second); return clock }
	store := filepath.Join(dir, "checkpoints")
	c := openCheckpoints(store, now)
	prompt := func(i int) string { return fmt.Sprintf("prompt-%03d", i) }
	for i := 1; i <= checkpointPrompts+2; i++ {
		if err := os.WriteFile(file, []byte(fmt.Sprintf("v%d\n", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := c.keep(prompt(i), file); err != nil {
			t.Fatalf("keep %d: %v", i, err)
		}
	}
	if c.gone([]string{prompt(1)}) == nil || c.gone([]string{prompt(2)}) == nil || c.gone([]string{prompt(3), prompt(checkpointPrompts + 2)}) != nil {
		t.Errorf("let go of: 1 %v, 2 %v, 3 %v", c.gone([]string{prompt(1)}), c.gone([]string{prompt(2)}), c.gone([]string{prompt(3)}))
	}
	blobs, _ := filepath.Glob(filepath.Join(store, "*-*"))
	if len(blobs) != checkpointPrompts {
		t.Errorf("%d copies are kept, want %d", len(blobs), checkpointPrompts)
	}
	// Opened again, the store still knows what it let go, and what it kept.
	again := openCheckpoints(store, now)
	if again.gone([]string{prompt(2)}) == nil || again.gone([]string{prompt(3)}) != nil {
		t.Error("a store opened again forgot which prompts' copies were let go")
	}
	if got := len(again.since(map[string]bool{prompt(3): true})); got != 1 {
		t.Errorf("a kept prompt's copy is not found after the store is opened again (%d)", got)
	}
}

// A file with another name (a hard link, as pnpm makes them) is left as
// it is: putting it back would cut it off from its other name.
func TestRestoreLeavesHardLinkedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hard links are told apart on unix")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := openCheckpoints(filepath.Join(dir, "checkpoints"), time.Now)
	if err := c.keep("p1", file); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(file, filepath.Join(dir, "other-name.txt")); err != nil {
		t.Skip("no hard links here: ", err)
	}
	done, failed := c.restore(c.since(map[string]bool{"p1": true}), restoreHow{})
	if len(done) != 0 || len(failed) != 1 || !strings.Contains(failed[0], "hard-linked") {
		t.Errorf("done %q, failed %q", done, failed)
	}
	if b, _ := os.ReadFile(file); string(b) != "after\n" {
		t.Errorf("the hard-linked file holds %q", b)
	}
}

// A copy that cannot be made is noted: a later change of the same file
// under the same prompt is not taken for what it held before the first,
// and a rewind says the file cannot be put back.
func TestFailedCopyIsNoted(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("v0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The store's folder cannot be made: a file stands where it would.
	store := filepath.Join(dir, "checkpoints")
	if err := os.WriteFile(store, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	c := openCheckpoints(store, time.Now)
	if err := c.keep("p1", file); err == nil {
		t.Fatal("a copy that could not be written was taken for made")
	}
	if err := os.WriteFile(file, []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = c.keep("p1", file)
	_, failed := c.restore(c.since(map[string]bool{"p1": true}), restoreHow{})
	if len(failed) != 1 || !strings.Contains(failed[0], "could not be kept") {
		t.Errorf("failed %q", failed)
	}
	if b, _ := os.ReadFile(file); string(b) != "v1\n" {
		t.Errorf("the file holds %q", b)
	}
}

// realTemp is a test's own folder, written with every link on the way
// followed, as the file tools give the paths they keep (macOS's /var is
// a link).
func realTemp(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeTo writes a test's file.
func writeTo(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// keepAll keeps the files under the prompt.
func keepAll(t *testing.T, c *checkpoints, prompt string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := c.keep(prompt, p); err != nil {
			t.Fatalf("keep %s: %v", p, err)
		}
	}
}

// A file is put back, or taken away, only where it was: the folders on
// the way to it must be what they were. One removed and made a link in
// its place (to ~/.ssh, say) would have a rewind write the person's
// authorized_keys there, or take away a file of theirs, and say it put
// back a file of the project (the rewind review of 2026-11-19).
func TestRestoreLeavesAFolderThatIsALinkNow(t *testing.T) {
	dir := realTemp(t)
	fx := filepath.Join(dir, "work", "fx")
	keys, made := filepath.Join(fx, "authorized_keys"), filepath.Join(fx, "made.txt")
	writeTo(t, keys, "K\n")
	c := openCheckpoints(filepath.Join(dir, "checkpoints"), time.Now)
	keepAll(t, c, "p2", keys, made)
	writeTo(t, keys, "K, edited\n")
	writeTo(t, made, "made by the model\n")

	elsewhere := filepath.Join(dir, "ssh")
	writeTo(t, filepath.Join(elsewhere, "authorized_keys"), "mine\n")
	writeTo(t, filepath.Join(elsewhere, "made.txt"), "mine too\n")
	if err := os.RemoveAll(fx); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, fx); err != nil {
		t.Skip("no symbolic links here: ", err)
	}
	done, failed := c.restore(c.since(map[string]bool{"p2": true}), restoreHow{})
	if len(done) != 0 || len(failed) != 2 || !strings.Contains(failed[0], "is a link now") || !strings.Contains(failed[1], "is a link now") {
		t.Errorf("done %q, failed %q", done, failed)
	}
	for name, want := range map[string]string{"authorized_keys": "mine\n", "made.txt": "mine too\n"} {
		if b, err := os.ReadFile(filepath.Join(elsewhere, name)); err != nil || string(b) != want {
			t.Errorf("%s, where the link leads, holds %q (%v), want %q", name, b, err, want)
		}
	}
	// A folder that is gone is made again: the file comes back in it.
	if err := os.Remove(fx); err != nil {
		t.Fatal(err)
	}
	done, failed = c.restore(c.since(map[string]bool{"p2": true}), restoreHow{})
	if len(done) != 2 || len(failed) != 0 {
		t.Errorf("with the folder gone: done %q, failed %q", done, failed)
	}
	if b, _ := os.ReadFile(keys); string(b) != "K\n" {
		t.Errorf("authorized_keys holds %q, want it back", b)
	}
}

// The session can refuse a file (an Edit deny rule added since, or a work
// tree that is gone), and a file that may not be written now is left as
// it is, as Write would leave it; one put back has its set-user-ID and the
// like again.
func TestRestoreRefusalsAndModes(t *testing.T) {
	dir := realTemp(t)
	denied, readonly, setuid := filepath.Join(dir, "denied.txt"), filepath.Join(dir, "readonly.txt"), filepath.Join(dir, "tool")
	writeTo(t, denied, "d0\n")
	writeTo(t, readonly, "r0\n")
	writeTo(t, setuid, "s0\n")
	if err := os.Chmod(setuid, 0o755|fs.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	c := openCheckpoints(filepath.Join(dir, "checkpoints"), time.Now)
	keepAll(t, c, "p1", denied, readonly, setuid)
	writeTo(t, denied, "d1\n")
	writeTo(t, readonly, "r1\n")
	writeTo(t, setuid, "s1\n")
	if err := os.Chmod(readonly, 0o444); err != nil {
		t.Fatal(err)
	}
	how := restoreHow{refuse: func(p string) string {
		if p == denied {
			return "an Edit deny rule covers it now"
		}
		return ""
	}}
	done, failed := c.restore(c.since(map[string]bool{"p1": true}), how)
	all := strings.Join(failed, "\n")
	if !strings.Contains(all, "denied.txt: an Edit deny rule covers it now, so it is left as it is") {
		t.Errorf("failed %q", failed)
	}
	if b, _ := os.ReadFile(denied); string(b) != "d1\n" {
		t.Errorf("the denied file holds %q", b)
	}
	if os.Geteuid() != 0 {
		// (Root may write anything.)
		if !strings.Contains(all, "readonly.txt: it may not be written now") {
			t.Errorf("failed %q", failed)
		}
		if b, _ := os.ReadFile(readonly); string(b) != "r1\n" {
			t.Errorf("the read-only file holds %q", b)
		}
	}
	if b, _ := os.ReadFile(setuid); string(b) != "s0\n" {
		t.Errorf("the set-user-ID file holds %q (done %q, failed %q)", b, done, failed)
	}
	if info, err := os.Stat(setuid); err != nil || info.Mode()&fs.ModeSetuid == 0 || info.Mode().Perm() != 0o755 {
		t.Errorf("the file put back has the mode %v (%v), want set-user-ID and 0755", info.Mode(), err)
	}
}

// Two spellings of one file (README.md and readme.md, on a disk that
// ignores case as macOS's do; here a second name of the file stands in
// for that) are kept as one file: two copies of it would be put back one
// over the other, and the later would win (the rewind review of
// 2026-11-19).
func TestKeepTakesTwoSpellingsAsOneFile(t *testing.T) {
	dir := realTemp(t)
	upper, lower := filepath.Join(dir, "README.md"), filepath.Join(dir, "readme.md")
	writeTo(t, upper, "v0\n")
	c := openCheckpoints(filepath.Join(dir, "checkpoints"), time.Now)
	keepAll(t, c, "p1", upper)
	writeTo(t, upper, "v1\n")
	if _, err := os.Lstat(lower); err != nil {
		if err := os.Link(upper, lower); err != nil {
			t.Skip("no second name for a file here: ", err)
		}
	}
	keepAll(t, c, "p1", lower)
	writeTo(t, lower, "v2\n")
	keepAll(t, c, "p2", lower)
	cps := c.since(map[string]bool{"p1": true, "p2": true})
	if len(cps) != 1 || cps[0].Path != upper {
		t.Fatalf("kept %+v, want README.md alone", cps)
	}
	if b, _ := os.ReadFile(filepath.Join(cps[0].dir, cps[0].Blob)); string(b) != "v0\n" {
		t.Errorf("the copy holds %q, want what the file held before the first change", b)
	}
}

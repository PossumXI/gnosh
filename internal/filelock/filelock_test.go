package filelock

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// patience is how long a test waits for something that should happen at
// once before it gives the failure a name instead of hanging.
const patience = 10 * time.Second

// mustTry takes the lock at path, which the test expects to be free.
func mustTry(t *testing.T, path string) func() {
	t.Helper()
	release, err := TryFile(path)
	if err != nil {
		t.Fatalf("TryFile(%s): %v", path, err)
	}
	return release
}

// mustBeHeld checks that the lock at path cannot be had right now.
func mustBeHeld(t *testing.T, path string) {
	t.Helper()
	release, err := TryFile(path)
	if err == nil {
		release()
		t.Fatalf("TryFile(%s) took a lock that should be held", path)
	}
	if !errors.Is(err, ErrContended) {
		t.Fatalf("TryFile(%s) = %v, want an error that is ErrContended", path, err)
	}
}

func TestTryFileExcludesAndLeavesTheFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")

	release := mustTry(t, path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the lock file was not made: %v", err)
	}
	mustBeHeld(t, path)
	_, err := TryFile(path)
	var perr *fs.PathError
	if !errors.As(err, &perr) || perr.Path != path {
		t.Fatalf("the error does not name the lock file: %v", err)
	}

	release()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the lock file should stay when the lock is let go: %v", err)
	}
	again := mustTry(t, path)
	again()
}

// A holder may write in its lock file and read it back while it holds the
// lock, and may remove the file before letting go.
func TestTheFileIsTheHoldersToUse(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")
	release := mustTry(t, path)
	if err := os.WriteFile(path, []byte("4242\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustBeHeld(t, path)
	if b, err := os.ReadFile(path); err != nil || string(b) != "4242\n" {
		t.Fatalf("read back %q, %v", b, err)
	}
	release()

	release = mustTry(t, path)
	if b, err := os.ReadFile(path); err != nil || string(b) != "4242\n" {
		t.Fatalf("taking the lock changed the file: %q, %v", b, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	release()
	// The path is free for a new file and a new lock.
	mustTry(t, path)()
}

// A release that is called again must not let go of a lock taken since,
// though the system may have given that lock the same number.
func TestReleaseWorksOnce(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")
	first := mustTry(t, path)
	first()
	first()
	second := mustTry(t, path)
	first()
	mustBeHeld(t, path)
	second()
	mustTry(t, path)()
}

func TestALockNeedsItsDirectory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing", "a.lock")
	for name, take := range map[string]func() (func(), error){
		"TryFile": func() (func(), error) { return TryFile(path) },
		"File":    func() (func(), error) { return File(context.Background(), path) },
	} {
		release, err := take()
		if err == nil {
			release()
			t.Fatalf("%s took a lock in a directory that is not there", name)
		}
		if errors.Is(err, ErrContended) || !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s = %v, want a not-exist error that is not ErrContended", name, err)
		}
	}
}

func TestFileTakesAFreeLock(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")
	release, err := File(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	mustBeHeld(t, path)
	release()
	mustTry(t, path)()
}

func TestFileWaitsForTheHolder(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")
	holder := mustTry(t, path)

	type result struct {
		release func()
		err     error
	}
	done := make(chan result, 1)
	go func() {
		release, err := File(context.Background(), path)
		done <- result{release, err}
	}()
	select {
	case r := <-done:
		t.Fatalf("File did not wait for the holder: %v", r.err)
	case <-time.After(30 * time.Millisecond):
	}

	holder()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		mustBeHeld(t, path)
		r.release()
	case <-time.After(patience):
		t.Fatal("File was not granted the lock its holder let go")
	}
	mustTry(t, path)()
}

func TestFileStopsWaitingWhenItsContextEnds(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")
	holder := mustTry(t, path)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	release, err := File(ctx, path)
	if err == nil {
		release()
		t.Fatal("File took a lock that was held")
	}
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrContended) {
		t.Fatalf("File = %v, want the context's error", err)
	}
	if waited := time.Since(start); waited > patience {
		t.Fatalf("File took %s to notice its context", waited)
	}

	// The wait that was given up must not keep the lock when it is
	// granted: once the holder lets go, the lock comes free.
	mustBeHeld(t, path)
	holder()
	within, stop := context.WithTimeout(context.Background(), patience)
	defer stop()
	release, err = File(within, path)
	if err != nil {
		t.Fatalf("the lock did not come free after a wait was given up: %v", err)
	}
	release()
}

func TestFileWithAContextThatIsOver(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")
	over, cancel := context.WithCancel(context.Background())
	cancel()

	// A free lock is taken all the same: nothing had to be waited for.
	release, err := File(over, path)
	if err != nil {
		t.Fatalf("File with a free lock: %v", err)
	}

	// A held one is not waited for at all.
	_, err = File(over, path)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("File with a held lock = %v, want the context's error", err)
	}
	release()

	late, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer stop()
	holder := mustTry(t, path)
	defer holder()
	if _, err = File(late, path); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("File past its deadline = %v, want the deadline's error", err)
	}
}

// Many holders in one process take the lock one at a time, each call being a
// holder of its own.
func TestHoldersTakeTurns(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.lock")
	const holders, turns = 8, 25
	var inside, taken atomic.Int32
	var overlapped atomic.Bool
	var wg sync.WaitGroup
	errs := make(chan error, holders)
	for range holders {
		wg.Go(func() {
			for range turns {
				release, err := File(context.Background(), path)
				if err != nil {
					errs <- err
					return
				}
				if inside.Add(1) != 1 {
					overlapped.Store(true)
				}
				taken.Add(1)
				inside.Add(-1)
				release()
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if overlapped.Load() {
		t.Fatal("two holders had the lock at once")
	}
	if got := taken.Load(); got != holders*turns {
		t.Fatalf("the lock was taken %d times, want %d", got, holders*turns)
	}
}

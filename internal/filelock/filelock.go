// Package filelock takes advisory locks on files, so that gnosh processes,
// and the parts of one gnosh process, can take turns at something on disk.
//
// A lock is a file that its holders agree on by path. Whoever holds the lock
// has the file open and locked through that open file; nothing is written to
// the file and nothing is read from it, so its contents are the caller's to
// use (a holder may keep a note there, such as its process id). The file is
// made when it is first locked and is left where it is when the lock is let
// go: other processes may be waiting on it, and may look at whether it is
// held.
//
// The lock is exclusive. Every call to TryFile or File is a holder of its
// own, so two calls exclude each other inside one process exactly as they do
// between two processes. The kernel lets go of a lock when its holder's
// process ends, however it ends, and a lock is never passed on to a program
// its holder starts.
//
// On Unix the lock is the whole-file lock of flock(2), taken exclusively.
// That choice is part of what callers rely on: a process that only wants to
// know whether a lock is held can open the file read-only and ask for a
// shared flock without waiting, which fails exactly while a holder has the
// lock, and takes nothing from the holder.
package filelock

import (
	"context"
	"errors"
	"os"
	"sync"
)

// ErrContended is the error TryFile wraps when the lock is held by someone
// else. Test for it with errors.Is.
var ErrContended = errors.New("the lock is held by another holder")

// TryFile takes the lock at path if it is free, and never waits. The lock
// file is made if it is not there; the directory it is in must be.
//
// On success the lock is held until the release it returns is called or the
// process ends. Calling release more than once does nothing more. When
// another holder has the lock, the error wraps ErrContended; any other error
// means the lock file could not be opened or locked at all.
func TryFile(path string) (release func(), err error) {
	h, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	got, err := tryLock(h)
	if !got {
		closeLockFile(h)
		if err == nil {
			err = ErrContended
		}
		return nil, &os.PathError{Op: "lock", Path: path, Err: err}
	}
	return releaser(h), nil
}

// File takes the lock at path, waiting for as long as another holder has it.
// The lock file is made if it is not there; the directory it is in must be.
//
// The wait is the kernel's: there is no polling and no time limit, and the
// lock is granted the moment it comes free. Only ctx ends the wait early;
// the error then wraps ctx.Err(). A lock that is free is taken without
// waiting, whatever the state of ctx.
//
// On success the lock is held until the release it returns is called or the
// process ends. Calling release more than once does nothing more.
func File(ctx context.Context, path string) (release func(), err error) {
	h, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	got, err := tryLock(h)
	if got {
		return releaser(h), nil
	}
	if err == nil {
		// Held by another. There is nothing to wait for if ctx is
		// over already.
		err = ctx.Err()
	}
	if err != nil {
		closeLockFile(h)
		return nil, &os.PathError{Op: "lock", Path: path, Err: err}
	}

	granted := make(chan error, 1)
	go func() { granted <- waitLock(h) }()
	select {
	case err := <-granted:
		if err != nil {
			closeLockFile(h)
			return nil, &os.PathError{Op: "lock", Path: path, Err: err}
		}
		return releaser(h), nil
	case <-ctx.Done():
		// A wait in the kernel cannot be called back. The caller is
		// answered now; the wait goes on behind it, and when the lock
		// is granted at last it is let go at once.
		go func() {
			if <-granted == nil {
				unlock(h)
			}
			closeLockFile(h)
		}()
		return nil, &os.PathError{Op: "lock", Path: path, Err: ctx.Err()}
	}
}

// releaser makes the function that lets go of the lock held through h. It
// works once: h is a number the system hands out again after it is closed,
// so a second call must not touch it.
func releaser(h handle) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			// Unlocking before closing frees the lock at this moment
			// even if a child process that is just being started
			// still has a copy of the open file for an instant.
			unlock(h)
			closeLockFile(h)
		})
	}
}

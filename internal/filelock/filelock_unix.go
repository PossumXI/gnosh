// The systems that have flock(2). Solaris and AIX are Unix without it, and
// have no lock of this kind to offer in its place.

//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package filelock

import (
	"os"
	"syscall"
)

// A handle is one holder's open lock file: a file descriptor of its own.
// flock(2) ties a lock to the open file, not to the process, which is what
// makes two holders in one process exclude each other.
type handle int

// openLockFile opens the lock file at path for a new holder, making it if it
// is not there, for its owner alone to read and write. It is opened for
// writing because some network file systems grant an exclusive lock only
// through a writable file, and close-on-exec so that a program this process
// starts does not inherit the lock.
func openLockFile(path string) (handle, error) {
	for {
		fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_CLOEXEC, 0o600)
		switch err {
		case nil:
			return handle(fd), nil
		case syscall.EINTR:
			// A signal arrived first; ask again.
		default:
			return -1, &os.PathError{Op: "open", Path: path, Err: err}
		}
	}
}

// tryLock takes h's lock if nobody holds it, shared or exclusive, and says
// whether it did. It does not wait. An error means the lock could not be
// asked for at all.
func tryLock(h handle) (bool, error) {
	err := flock(h, syscall.LOCK_EX|syscall.LOCK_NB)
	if err == syscall.EWOULDBLOCK {
		return false, nil
	}
	return err == nil, err
}

// waitLock takes h's lock, sleeping in the kernel until it is free.
func waitLock(h handle) error {
	return flock(h, syscall.LOCK_EX)
}

// unlock lets go of h's lock.
func unlock(h handle) {
	_ = flock(h, syscall.LOCK_UN)
}

// closeLockFile closes h. A lock still held through it goes with it.
func closeLockFile(h handle) {
	_ = syscall.Close(int(h))
}

// flock is flock(2), asked again whenever a signal cut it short.
func flock(h handle, how int) error {
	for {
		err := syscall.Flock(int(h), how)
		if err != syscall.EINTR {
			return err
		}
	}
}

//go:build windows

package filelock

import (
	"os"

	"golang.org/x/sys/windows"
)

// A handle is one holder's open lock file: a file handle of its own, which
// the lock belongs to.
type handle windows.Handle

// lockAt is the one byte of the lock file that is locked: the first byte
// past a gibibyte, far beyond anything a lock file holds. Windows refuses
// reads and writes of locked bytes through other handles, and a holder may
// keep a note at the start of its lock file; locking a byte that is never
// read or written keeps the lock advisory, as it is on Unix. A byte past the
// end of the file may be locked.
const lockAt = 1 << 30

// openLockFile opens the lock file at path for a new holder, making it if it
// is not there. It shares the file fully, so that other holders can open it
// to wait their turn and a holder can remove its own lock file. Handles made
// here are not inherited by programs this process starts.
func openLockFile(path string) (handle, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return 0, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return handle(h), nil
}

// tryLock takes h's lock if nobody holds it, and says whether it did. It
// does not wait. An error means the lock could not be asked for at all.
func tryLock(h handle) (bool, error) {
	err := windows.LockFileEx(windows.Handle(h),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, lockRange())
	if err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_IO_PENDING {
		return false, nil
	}
	return err == nil, err
}

// waitLock takes h's lock, sleeping in the kernel until it is free.
func waitLock(h handle) error {
	return windows.LockFileEx(windows.Handle(h), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, lockRange())
}

// unlock lets go of h's lock.
func unlock(h handle) {
	_ = windows.UnlockFileEx(windows.Handle(h), 0, 1, 0, lockRange())
}

// closeLockFile closes h. A lock still held through it goes with it.
func closeLockFile(h handle) {
	_ = windows.CloseHandle(windows.Handle(h))
}

// lockRange says where the locked byte is, in the form the lock calls take.
func lockRange() *windows.Overlapped {
	return &windows.Overlapped{Offset: lockAt}
}

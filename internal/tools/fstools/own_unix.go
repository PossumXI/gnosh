//go:build unix

package fstools

import (
	"io/fs"
	"os"
	"syscall"
)

// otherNames is how many other names (hard links) the file old describes
// has.
func otherNames(old fs.FileInfo) int {
	if old == nil {
		return 0
	}
	if st, ok := old.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
		return int(st.Nlink) - 1
	}
	return 0
}

// keepOwner gives the open file f the owner and group of the file old
// describes, as far as the system lets this process do so. A process that
// may not hand a file to another user can usually still give it the group;
// when it can do neither, the new file stays its own, and the write goes
// ahead.
func keepOwner(f *os.File, old fs.FileInfo) {
	want, ok := old.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}
	info, err := f.Stat()
	if err != nil {
		return
	}
	have, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (have.Uid == want.Uid && have.Gid == want.Gid) {
		return
	}
	if f.Chown(int(want.Uid), int(want.Gid)) != nil && have.Gid != want.Gid {
		f.Chown(-1, int(want.Gid))
	}
}

// mayWrite asks the system whether this process may write the file at
// path, without opening it: opening a file for writing, even to write
// nothing, wakes whatever is watching the file.
func mayWrite(path string) error {
	const writeAccess = 0x2 // W_OK
	return syscall.Access(path, writeAccess)
}

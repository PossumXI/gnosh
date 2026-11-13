//go:build darwin

package memjoin

import (
	"io/fs"
	"syscall"
	"time"
)

// changeTime is when a file's inode last changed. A command that keeps a
// file's modification time (git mv, cp -p) still moves this.
func changeTime(info fs.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Ctimespec.Sec), int64(st.Ctimespec.Nsec))
	}
	return time.Time{}
}

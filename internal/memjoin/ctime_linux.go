//go:build linux

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
		return time.Unix(int64(st.Ctim.Sec), int64(st.Ctim.Nsec))
	}
	return time.Time{}
}

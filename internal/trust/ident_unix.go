//go:build unix

package trust

import (
	"io/fs"
	"syscall"
)

// identity reads, out of what Stat returned, the numbers that tell one
// directory from another on this machine (device and inode) and the id of
// the user who owns it.
func identity(info fs.FileInfo) (dev, ino uint64, uid int, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0, false
	}
	return uint64(st.Dev), uint64(st.Ino), int(st.Uid), true
}

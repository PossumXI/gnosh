//go:build unix

package config

import (
	"io/fs"
	"syscall"
)

// ownerOf reads the owner's user id out of what Stat returned.
func ownerOf(info fs.FileInfo) (int, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

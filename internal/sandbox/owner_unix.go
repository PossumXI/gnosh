//go:build unix

package sandbox

import (
	"os"
	"strconv"
	"syscall"
)

// uidText is the person's user id, as text.
func uidText() string { return strconv.Itoa(os.Getuid()) }

// ownedByMe reports whether the file is the person's own.
func ownedByMe(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

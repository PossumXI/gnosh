//go:build unix

package instructions

import (
	"io/fs"
	"os"
	"syscall"
)

// hostOwner returns the host's ownership test: a file passes when it belongs
// to the account this program runs as, or to root. A file system that does
// not say who owns its files has no strangers on it, so everything there
// passes.
func hostOwner() func(fs.FileInfo) bool {
	uid := uint32(os.Getuid())
	return func(info fs.FileInfo) bool {
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return true
		}
		return st.Uid == 0 || st.Uid == uid
	}
}

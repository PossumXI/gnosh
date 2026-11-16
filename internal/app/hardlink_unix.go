//go:build unix

package app

import (
	"os"
	"syscall"
)

// hardLinked reports whether the file at p is a regular file with more
// than one name, a hard link: under another name it may be a file of the
// person's checkout, and a write to it here changes it there.
func hardLinked(p string) bool {
	info, err := os.Stat(p)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}

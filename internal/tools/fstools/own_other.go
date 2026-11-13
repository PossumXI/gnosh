//go:build !unix

package fstools

import (
	"io/fs"
	"os"
)

// keepOwner does nothing on systems without Unix file ownership.
func keepOwner(f *os.File, old fs.FileInfo) {}

// mayWrite finds out whether this process may write the file at path by
// opening it for writing and closing it again.
func mayWrite(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

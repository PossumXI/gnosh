//go:build !linux && !darwin

package memjoin

import (
	"io/fs"
	"time"
)

// changeTime is when a file's inode last changed, where the system tells:
// here it does not, and the modification time alone is gone by.
func changeTime(fs.FileInfo) time.Time { return time.Time{} }

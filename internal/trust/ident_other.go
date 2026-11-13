//go:build !unix

package trust

import "io/fs"

// identity cannot tell directories apart or name an owner on this system,
// so no folder can be recorded as trusted here.
func identity(fs.FileInfo) (dev, ino uint64, uid int, ok bool) { return 0, 0, 0, false }

//go:build !unix

package instructions

import "io/fs"

// hostOwner returns the host's ownership test. This host does not give
// files owners in a way the test can read, so everything passes.
func hostOwner() func(fs.FileInfo) bool {
	return func(fs.FileInfo) bool { return true }
}

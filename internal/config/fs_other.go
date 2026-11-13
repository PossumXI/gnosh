//go:build !unix

package config

import "io/fs"

// ownerOf cannot tell who owns a file on this system.
func ownerOf(fs.FileInfo) (int, bool) { return 0, false }

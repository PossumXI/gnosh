//go:build !unix

package sandbox

import "os"

// uidText is a stand-in where there are no user ids.
func uidText() string { return "user" }

// ownedByMe cannot be told here.
func ownedByMe(info os.FileInfo) bool { return true }

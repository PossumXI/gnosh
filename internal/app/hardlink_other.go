//go:build !unix

package app

// hardLinked reports whether the file at p has more than one name; where
// that cannot be told, it says no.
func hardLinked(p string) bool { return false }

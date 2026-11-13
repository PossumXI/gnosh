//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package app

// echoOff reports that this system's terminals cannot be told to hide what
// is typed.
func echoOff(fd int) (restore func(), ok bool) { return nil, false }

//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package app

import "golang.org/x/sys/unix"

// The requests that read and write a terminal's settings.
const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)

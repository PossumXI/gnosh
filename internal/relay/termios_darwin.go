//go:build darwin

package relay

import "golang.org/x/sys/unix"

// The requests that read and set a terminal's settings.
const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)

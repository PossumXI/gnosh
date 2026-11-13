//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package app

import "golang.org/x/sys/unix"

// echoOff stops the terminal behind fd from showing what is typed, for a
// secret, and returns what puts it back as it was. The Enter that ends the
// line is still shown, so that the line ends on the screen. It reports
// false when fd is not a terminal that can be told.
func echoOff(fd int) (restore func(), ok bool) {
	t, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return nil, false
	}
	before := *t
	t.Lflag &^= unix.ECHO
	t.Lflag |= unix.ECHONL
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, t); err != nil {
		return nil, false
	}
	return func() { _ = unix.IoctlSetTermios(fd, ioctlSetTermios, &before) }, true
}

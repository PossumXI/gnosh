//go:build linux

package app

import "golang.org/x/sys/unix"

// The requests that read and write a terminal's settings.
const (
	ioctlGetTermios = unix.TCGETS
	ioctlSetTermios = unix.TCSETS
)

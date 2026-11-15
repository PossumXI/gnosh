//go:build linux

package term

import (
	"os"

	"golang.org/x/sys/unix"
)

// ioctlGetTermios is the request that reads a terminal's settings.
const ioctlGetTermios = unix.TCGETS

// openPTY makes a pseudo-terminal for a test.
func openPTY() (master, slave *os.File, err error) { return OpenPTY() }

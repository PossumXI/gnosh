//go:build linux

package term

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// ioctlGetTermios is the request that reads a terminal's settings.
const ioctlGetTermios = unix.TCGETS

// openPTY makes a pseudo-terminal: master is the side a terminal emulator
// holds, slave the side a program sees as its terminal.
func openPTY() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	var name string
	err = onFd(master, func(fd int) error {
		if err := unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0); err != nil {
			return fmt.Errorf("unlock the pseudo-terminal: %w", err)
		}
		n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
		name = fmt.Sprintf("/dev/pts/%d", n)
		return err
	})
	if err == nil {
		slave, err = os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	}
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}

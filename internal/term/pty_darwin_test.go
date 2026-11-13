//go:build darwin

package term

import (
	"bytes"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ioctlGetTermios is the request that reads a terminal's settings.
const ioctlGetTermios = unix.TIOCGETA

// openPTY makes a pseudo-terminal: master is the side a terminal emulator
// holds, slave the side a program sees as its terminal.
func openPTY() (master, slave *os.File, err error) {
	master, err = os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	var name string
	err = onFd(master, func(fd int) error {
		if err := unix.IoctlSetInt(fd, unix.TIOCPTYGRANT, 0); err != nil {
			return fmt.Errorf("grant the pseudo-terminal: %w", err)
		}
		if err := unix.IoctlSetInt(fd, unix.TIOCPTYUNLK, 0); err != nil {
			return fmt.Errorf("unlock the pseudo-terminal: %w", err)
		}
		buf := make([]byte, 128)
		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(unix.TIOCPTYGNAME), uintptr(unsafe.Pointer(&buf[0]))); errno != 0 {
			return fmt.Errorf("name the pseudo-terminal: %w", errno)
		}
		if i := bytes.IndexByte(buf, 0); i >= 0 {
			buf = buf[:i]
		}
		name = string(buf)
		return nil
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

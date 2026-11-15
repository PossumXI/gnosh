//go:build linux || darwin

package term

import "os"

// withFd runs fn with f's descriptor, which stays open meanwhile.
func withFd(f *os.File, fn func(fd int) error) error {
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := conn.Control(func(fd uintptr) { ferr = fn(int(fd)) }); err != nil {
		return err
	}
	return ferr
}

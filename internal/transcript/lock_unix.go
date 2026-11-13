//go:build unix

package transcript

import (
	"errors"
	"os"
	"syscall"
)

// lockFile takes the advisory lock that marks f's one writer, without
// waiting. It reports false when another open of the file holds it. The
// lock goes when f is closed or the process ends.
func lockFile(f *os.File) (bool, error) {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return false, nil
		default:
			return false, err
		}
	}
}

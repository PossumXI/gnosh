//go:build !linux && !darwin

package term

import (
	"errors"
	"os"
)

// OpenPTY makes a pseudo-terminal, where gnosh knows how: on Linux and
// macOS.
func OpenPTY() (master, slave *os.File, err error) {
	return nil, nil, errors.New("gnosh makes pseudo-terminals only on Linux and macOS")
}

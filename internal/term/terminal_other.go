//go:build !unix

package term

import (
	"errors"
	"os"
)

// Terminal is not available on this system.
type Terminal struct{}

var errUnsupported = errors.New("gnosh's terminal handling is not written for this operating system")

// Open always fails on this system.
func Open(in, out *os.File, opts Options) (*Terminal, error) { return nil, errUnsupported }

// IsTerminal always reports false on this system.
func IsTerminal(f *os.File) bool { return false }

// GetSize always fails on this system.
func GetSize(f *os.File) (cols, rows int, err error) { return 0, 0, errUnsupported }

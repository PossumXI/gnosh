//go:build !unix

package ui

import (
	"errors"
	"os"
)

// openTerminal always fails on this system: gnosh's terminal handling is
// written for Linux and macOS.
func openTerminal(in, out *os.File, h TerminalHooks) (Terminal, error) {
	return nil, errors.New("gnosh's interface is not written for this operating system")
}

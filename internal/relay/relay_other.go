//go:build !linux && !darwin

package relay

import (
	"errors"
	"os"
)

// isTerminal reports whether f is a terminal; here gnosh does not relay,
// so it says no.
func isTerminal(*os.File) bool { return false }

// relay is not built here.
func relay(string, Options) (int, bool, error) {
	return 0, false, errors.New("the relay runs only on Linux and macOS")
}

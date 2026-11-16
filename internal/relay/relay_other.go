//go:build !linux && !darwin

package relay

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// relay is not built here: ssh runs without it.
func relay(path string, o Options) (int, syscall.Signal) { return plain(path, o) }

// plain runs ssh with gnosh's own input and output.
func plain(path string, o Options) (int, syscall.Signal) {
	err := command(path, o).Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return 0, 0
	case errors.As(err, &ee):
		return ee.ExitCode(), 0
	}
	fmt.Fprintf(o.Err, "gnosh ssh: %v\n", err)
	return 126, 0
}

// dieOf does nothing here: gnosh ends with ssh's status.
func dieOf(syscall.Signal) {}

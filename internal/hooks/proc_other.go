//go:build unix && !linux

package hooks

import (
	"errors"
	"syscall"
)

// groupGone reports whether no live process is left in the group that pid
// led. It is only meaningful once pid itself has been waited for. These
// systems do not count a process that has ended and waits to be collected,
// so the group answers only while a member is alive.
func groupGone(pid int) bool {
	return exists(pid) || errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH)
}

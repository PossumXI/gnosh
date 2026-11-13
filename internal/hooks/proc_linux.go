//go:build linux

package hooks

import (
	"bytes"
	"errors"
	"os"
	"strconv"
	"syscall"
)

// groupGone reports whether no live process is left in the group that pid
// led. It is only meaningful once pid itself has been waited for.
//
// Linux still counts, as a member of a group, a process that has ended and
// only waits to be collected by its parent. Where nobody collects such
// processes promptly (a container whose first process is not an init),
// the group would never look empty, and every hook that is stopped would
// cost the whole wait. So when the group still answers, its members are
// looked up, and it is gone if none of them is alive.
func groupGone(pid int) bool {
	if exists(pid) || errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
		return true
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue // not a process
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // it ended while we were looking
		}
		// After the program's name, which is in parentheses and may hold
		// anything: the state, the parent, the process group.
		fields := bytes.Fields(stat[bytes.LastIndexByte(stat, ')')+1:])
		if len(fields) < 3 {
			continue
		}
		group, err := strconv.Atoi(string(fields[2]))
		if err == nil && group == pid && fields[0][0] != 'Z' && fields[0][0] != 'X' {
			return false
		}
	}
	return true
}

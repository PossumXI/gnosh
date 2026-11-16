//go:build darwin

package relay

import "golang.org/x/sys/unix"

// sstop is the state of a stopped process (SSTOP in sys/proc.h).
const sstop = 4

// stopped reports whether the process pid is stopped.
func stopped(pid int) bool {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	return err == nil && kp.Proc.P_stat == sstop
}

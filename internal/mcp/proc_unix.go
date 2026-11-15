//go:build unix

package mcp

import (
	"os/exec"
	"syscall"
)

// ownGroup starts the server in a process group of its own.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// stop sends the server's group the signal of one step of stopping it:
// SIGINT, then SIGTERM, then SIGKILL.
func stop(cmd *exec.Cmd, step int) {
	sig := []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL}[min(step, 2)]
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}

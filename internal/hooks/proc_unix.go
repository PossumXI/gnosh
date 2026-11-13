//go:build unix

package hooks

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// ownGroup makes cmd start as the leader of a new session. That gives it a
// process group of its own, whose number is its process id, so that the
// hook and everything it starts can be stopped together; and it leaves the
// hook with no controlling terminal, so that it cannot read the keyboard or
// draw on the screen.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// terminate asks every process in the group that pid leads to stop.
// reaped says whether pid itself has already been waited for.
func terminate(pid int, reaped bool) {
	signalGroup(pid, syscall.SIGTERM, reaped)
}

// destroy kills every process in the group that pid leads.
func destroy(pid int, reaped bool) {
	signalGroup(pid, syscall.SIGKILL, reaped)
}

// signalGroup sends sig to the process group that pid leads.
//
// A process id is not handed out again while a process or a group still
// goes by it. So as long as pid has not been waited for, the group is
// ours. Once it has been, the number stays ours for as long as the group
// has a member left; if a process now answers to it, the group has ended
// and the number belongs to someone else, who must not be signalled.
func signalGroup(pid int, sig syscall.Signal, reaped bool) {
	if reaped && exists(pid) {
		return
	}
	syscall.Kill(-pid, sig)
}

// exists reports whether a process has this id.
func exists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// ending reads how a process ended: its exit status, or, when a signal
// killed it, -1 and the signal's number and name.
func ending(st *os.ProcessState) (exit int, signal string) {
	if st == nil {
		return -1, ""
	}
	if ws, ok := st.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return -1, fmt.Sprintf("signal %d (%s)", int(ws.Signal()), ws.Signal())
	}
	return st.ExitCode(), ""
}

// startError puts the reason a command could not be started into plain
// words.
func startError(err error) string {
	if errors.Is(err, syscall.E2BIG) {
		return "the event's details, such as the tool's command, are too long to hand to a program in its environment: " + err.Error()
	}
	return err.Error()
}

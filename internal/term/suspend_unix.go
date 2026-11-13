//go:build unix

package term

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// shellBehind reports whether something would continue the process once
// it is stopped: whether gnosh, or one of the programs that started it
// and share its process group, was started by a process outside the group
// and inside the session, as a shell that runs gnosh as a job is. It is
// the system's own test for whether a stop from the keyboard may stop a
// process: one in a process group with no such parent is never stopped by
// it, because nothing could wake it.
func shellBehind() bool {
	group := unix.Getpgrp()
	session, err := unix.Getsid(0)
	if err != nil {
		return false
	}
	pid := os.Getpid()
	for range 16 {
		parent := parentOf(pid)
		if parent <= 1 {
			return false
		}
		pg, err := unix.Getpgid(parent)
		if err != nil {
			return false
		}
		if pg != group {
			sid, err := unix.Getsid(parent)
			return err == nil && sid == session
		}
		pid = parent
	}
	return false
}

// parentOf is the process that started a process, or 0 when that cannot
// be told.
func parentOf(pid int) int {
	if pid == os.Getpid() {
		return os.Getppid()
	}
	// Where the system keeps a file for each process, its fourth field,
	// after the name in parentheses, is the parent.
	if data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if i := strings.LastIndexByte(string(data), ')'); i >= 0 {
			if fields := strings.Fields(string(data)[i+1:]); len(fields) >= 2 {
				if n, err := strconv.Atoi(fields[1]); err == nil {
					return n
				}
			}
		}
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

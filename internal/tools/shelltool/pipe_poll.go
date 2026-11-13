//go:build linux || darwin

package shelltool

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// readOutput reads the command's output from the pipe and hands each piece
// to take, until the pipe ends or is closed.
//
// It does the reading itself rather than through the file's Read, for one
// reason: so that reading a piece and taking it in happen under one lock,
// intake. Whoever holds that lock then knows that no piece is on its way
// from the pipe to the output, and can tell from the pipe alone whether
// the pump has caught up (see pipeState).
func (j *job) readOutput(buf []byte) {
	conn, err := j.pipe.SyscallConn()
	if err != nil {
		return
	}
	ask := make([]unix.PollFd, 1)
	for {
		var n int
		var readErr error
		// The function is called when the pipe may have something, and
		// again whenever it becomes readable after having had nothing.
		waitErr := conn.Read(func(fd uintptr) bool {
			j.intake.Lock()
			defer j.intake.Unlock()
			if n, readErr = readNow(int(fd), buf, ask); readErr == syscall.EAGAIN {
				return false
			}
			if n > 0 {
				j.take(buf[:n])
			}
			return true
		})
		// Our end was closed; or reading failed; or the pipe has ended.
		if waitErr != nil || readErr != nil || n <= 0 {
			return
		}
	}
}

// readNow reads what is in the pipe without ever waiting: it reports
// EAGAIN when there is nothing to read just now and the pipe has not
// ended. It asks first, so that it does not wait even if the pipe were,
// against all expectation, not in non-blocking mode. ask is room for the
// question, kept by the caller so that none is allocated for each read.
func readNow(fd int, buf []byte, ask []unix.PollFd) (int, error) {
	for {
		ask[0] = unix.PollFd{Fd: int32(fd), Events: unix.POLLIN}
		if _, err := unix.Poll(ask, 0); err == unix.EINTR {
			continue
		} else if err != nil {
			return 0, err
		}
		if ask[0].Revents == 0 {
			return 0, syscall.EAGAIN
		}
		n, err := syscall.Read(fd, buf)
		if err == syscall.EINTR {
			continue
		}
		return max(n, 0), err
	}
}

// pipeState says how the pipe stands at this moment: whether there is
// output in it waiting to be read, how much has been taken in so far, and
// whether any process still has the writing end open. No piece of output
// is in between the pipe and the output while it looks, so an empty pipe
// means that everything written so far has been taken in. known is false
// when the pipe could not be asked, as when it has just been closed.
func (j *job) pipeState() (waiting bool, taken int64, writers, known bool) {
	conn, err := j.pipe.SyscallConn()
	if err != nil {
		return false, 0, false, false
	}
	j.intake.Lock()
	defer j.intake.Unlock()
	fds := []unix.PollFd{{Events: unix.POLLIN}}
	var askErr error
	err = conn.Control(func(fd uintptr) {
		fds[0].Fd = int32(fd)
		for {
			if _, askErr = unix.Poll(fds, 0); askErr != unix.EINTR {
				return
			}
		}
	})
	if err != nil || askErr != nil {
		return false, 0, false, false
	}
	taken, _, _ = j.out.size()
	// The kernel reports a hang-up on a pipe once its last writer has gone.
	return fds[0].Revents&unix.POLLIN != 0, taken, fds[0].Revents&unix.POLLHUP == 0, true
}

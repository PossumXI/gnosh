//go:build linux || darwin

package relay

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
	xterm "golang.org/x/term"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// drainWait is how long the relay waits, once ssh has ended, for the rest
// of what it wrote: something it left running may hold its terminal open.
const drainWait = 2 * time.Second

// copyWait is how long the copies still queued at the end are given.
const copyWait = 3 * time.Second

// isTerminal reports whether f is a terminal.
func isTerminal(f *os.File) bool { return f != nil && xterm.IsTerminal(int(f.Fd())) }

// relay runs ssh on a pseudo-terminal of its own: what is typed goes to it
// as it is, what it writes goes to the terminal as it is, and the text of
// each clipboard write in what it writes goes to o.Copy. started says ssh
// was started; when it was not, err says why and nothing was changed.
func relay(path string, o Options) (status int, started bool, err error) {
	inFd, outFd := int(o.In.Fd()), int(o.Out.Fd())
	master, slave, err := term.OpenPTY()
	if err != nil {
		return 0, false, err
	}
	defer master.Close()
	// Plain blocking reads and writes on the master, as on the terminal:
	// not every system's poller takes a pseudo-terminal (Fd makes the
	// file blocking).
	master.Fd()
	// ssh's terminal starts as this one is: its settings, and its size.
	if t, err := unix.IoctlGetTermios(inFd, ioctlGetTermios); err == nil {
		_ = control(slave, func(fd int) error { return unix.IoctlSetTermios(fd, ioctlSetTermios, t) })
	}
	resize := func() {
		if ws, err := unix.IoctlGetWinsize(outFd, unix.TIOCGWINSZ); err == nil {
			_ = control(master, func(fd int) error { return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, ws) })
		}
	}
	resize()

	cmd := exec.Command(path, o.Args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGWINCH, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		slave.Close()
		return 0, false, err
	}
	// Only ssh holds its terminal now, so that its end ends the reading.
	slave.Close()

	saved, rawErr := xterm.MakeRaw(inFd)
	if rawErr == nil {
		defer xterm.Restore(inFd, saved)
	}

	copies := make(chan string, copiesQueued)
	copied := make(chan struct{})
	var failed int
	var lastErr error
	go func() {
		defer close(copied)
		for text := range copies {
			if err := o.Copy(text); err != nil {
				failed, lastErr = failed+1, err
			}
		}
	}()
	offer := func(text string) {
		for {
			select {
			case copies <- text:
				return
			default:
				select {
				case <-copies:
				default:
				}
			}
		}
	}

	// What is typed goes to ssh. (The reading ends with gnosh.)
	go func() { _, _ = io.Copy(master, o.In) }()
	// What ssh writes goes to the terminal, and through the scanner.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		var sc scanner
		buf := make([]byte, 32*1024)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				if _, werr := o.Out.Write(buf[:n]); werr != nil {
					return
				}
				sc.feed(buf[:n], offer)
			}
			if err != nil {
				return
			}
		}
	}()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	var werr error
loop:
	for {
		select {
		case sig := <-sigs:
			if sig == syscall.SIGWINCH {
				resize()
				continue
			}
			_ = cmd.Process.Signal(sig)
		case werr = <-waited:
			break loop
		}
	}
	select {
	case <-drained:
	case <-time.After(drainWait):
	}
	close(copies)
	finished := false
	select {
	case <-copied:
		finished = true
	case <-time.After(copyWait):
	}
	if rawErr == nil {
		_ = xterm.Restore(inFd, saved)
	}
	switch {
	case !finished:
		fmt.Fprintf(o.Err, "gnosh ssh: the clipboard took more than %s; the last of the far side's copies may not have reached it.\n", copyWait)
	case failed > 0:
		fmt.Fprintf(o.Err, "gnosh ssh: %d of the far side's copies did not reach the clipboard (%v).\n", failed, lastErr)
	}
	return exitStatus(werr), true, nil
}

// control runs fn with f's descriptor, leaving f as it was.
func control(f *os.File, fn func(fd int) error) error {
	conn, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var ferr error
	if err := conn.Control(func(fd uintptr) { ferr = fn(int(fd)) }); err != nil {
		return err
	}
	return ferr
}

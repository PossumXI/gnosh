//go:build linux || darwin

package term

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func init() { helpers["terminal"] = terminalHelper }

// terminalHelper is a small program with a Terminal on its standard input
// and output. It reports each event on standard error, suspends itself on
// Ctrl+Z and quits on q.
func terminalHelper() {
	t, err := Open(os.Stdin, os.Stdout, Options{OnStop: func() { fmt.Fprintln(os.Stderr, "told to stop") }})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	t.SetModes(Modes{BracketedPaste: true})
	fmt.Fprintln(os.Stderr, "ready")
	for ev := range t.Events() {
		fmt.Fprintln(os.Stderr, "event:", ev)
		if k, ok := ev.(Key); ok {
			switch {
			case k.Is('q', 0):
				t.Close()
				fmt.Fprintln(os.Stderr, "closed")
				return
			case k.Is('z', ModCtrl):
				t.Suspend()
			}
		}
	}
	fmt.Fprintln(os.Stderr, "ended:", t.Err())
}

// TestSignalsInAChild runs a program whose controlling terminal is a
// pseudo-terminal, as gnosh's is, and sends it what a real session sends:
// a change of size from the terminal itself, a request to stop, a request
// to end.
func TestSignalsInAChild(t *testing.T) {
	master, slave, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer master.Close()
	before := settings(t, master)

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self)
	cmd.Env = append(os.Environ(), "GNOSH_TERM_HELPER=terminal")
	cmd.Stdin, cmd.Stdout = slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	slave.Close()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	expect := func(want string) {
		t.Helper()
		select {
		case got, ok := <-lines:
			if !ok || got != want {
				cmd.Process.Kill()
				t.Fatalf("the child said %q (still running: %v), want %q", got, ok, want)
			}
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
			t.Fatalf("the child did not say %q", want)
		}
	}
	isRaw := func() bool { return settings(t, master).Lflag&unix.ICANON == 0 }

	expect("ready")
	if !isRaw() {
		t.Error("the terminal is not in raw mode while the child has it")
	}

	// The terminal changes size: the system tells the child (SIGWINCH).
	err = onFd(master, func(fd int) error {
		return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Row: 30, Col: 100})
	})
	if err != nil {
		t.Fatal(err)
	}
	expect("event: resize 100x30")

	master.WriteString("a")
	expect("event: a")

	// A request to stop (SIGTSTP): the child gives the terminal back and
	// asks to be stopped. Nothing here would ever continue it, and the
	// system does not stop such a process, so it takes the terminal back
	// and says the interface must redraw.
	cmd.Process.Signal(syscall.SIGTSTP)
	expect("told to stop")
	expect("event: resume")
	expect("event: resize 100x30")
	if !isRaw() {
		t.Error("the terminal is not in raw mode after the child resumed")
	}
	// Ctrl+Z is a key; the child chooses to suspend on it.
	master.WriteString("\x1a")
	expect("event: ctrl+z")
	expect("event: resume")
	expect("event: resize 100x30")

	// A request to end (SIGTERM): the child puts the terminal back as it
	// found it and then dies of the signal, as it would have without a
	// Terminal.
	cmd.Process.Signal(syscall.SIGTERM)
	err = cmd.Wait()
	exit, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("the child ended with %v, want death by SIGTERM", err)
	}
	if ws := exit.Sys().(syscall.WaitStatus); !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
		t.Errorf("the child ended with status %v, want death by SIGTERM", exit)
	}
	if after := settings(t, master); after != before {
		t.Errorf("settings after the child was told to end differ from those before it started:\n got %+v\nwant %+v", after, before)
	}
	// It turned bracketed paste off again on its way out.
	var output strings.Builder
	buf := make([]byte, 4096)
	for !strings.HasSuffix(output.String(), BracketedPasteOff) {
		master.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := master.Read(buf)
		output.Write(buf[:n])
		if err != nil {
			break
		}
	}
	if got := output.String(); !strings.HasSuffix(got, ShowCursor+BracketedPasteOff) || !strings.HasPrefix(got, BracketedPasteOn) {
		t.Errorf("the child wrote %q to the terminal, want bracketed paste turned on at the start and off at the end", got)
	}
}

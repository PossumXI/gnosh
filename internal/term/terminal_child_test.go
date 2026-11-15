//go:build linux || darwin

package term

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func init() {
	helpers["terminal"] = terminalHelper
	helpers["shell"] = shellHelper
	helpers["wrapper"] = wrapperHelper
}

// terminalHelper is a small program with a Terminal on its standard input
// and output. It reports each event on standard error, suspends itself on
// Ctrl+Z, saying so when it could not, and quits on q.
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
				if err := t.Suspend(); err != nil {
					fmt.Fprintln(os.Stderr, "not suspended:", err)
				}
			}
		}
	}
	fmt.Fprintln(os.Stderr, "ended:", t.Err())
}

// wrapperHelper is a program that runs terminalHelper as its child, in
// its own process group, as env, sudo or a script of the person's runs
// gnosh. It says which process that is, waits for it, and ends as it
// did.
func wrapperHelper() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "wrapper: error:", err)
		os.Exit(1)
	}
	program := exec.Command(self)
	program.Env = append(os.Environ(), "GNOSH_TERM_HELPER=terminal")
	program.Stdin, program.Stdout, program.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := program.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "wrapper: error:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "wrapper: the program is %d\n", program.Process.Pid)
	if err := program.Wait(); err != nil {
		fmt.Fprintln(os.Stderr, "wrapper: the program ended:", err)
		os.Exit(1)
	}
}

// shellHelper is as much of a shell with job control as the tests need.
// It leads a session on the terminal it was given, runs a job of its own
// in the foreground (terminalHelper, or the helper GNOSH_TERM_JOB names),
// takes the terminal back when the job stops, and puts the job in the
// foreground again and continues it when it is told to (SIGUSR1). It
// reports what it sees on standard error, where the job reports too.
func shellHelper() {
	say := func(format string, args ...any) { fmt.Fprintf(os.Stderr, "shell: "+format+"\n", args...) }
	// A shell that takes the terminal back is in the background when it
	// does, and must not be stopped for it.
	signal.Ignore(syscall.SIGTTOU, syscall.SIGTTIN, syscall.SIGTSTP)
	again := make(chan os.Signal, 1)
	signal.Notify(again, syscall.SIGUSR1)

	self, err := os.Executable()
	if err != nil {
		say("error: %v", err)
		os.Exit(1)
	}
	what := os.Getenv("GNOSH_TERM_JOB")
	if what == "" {
		what = "terminal"
	}
	job := exec.Command(self)
	job.Env = append(os.Environ(), "GNOSH_TERM_HELPER="+what)
	job.Stdin, job.Stdout, job.Stderr = os.Stdin, os.Stdout, os.Stderr
	// The job has a process group of its own, and the terminal is given
	// to it before it starts.
	job.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Foreground: true, Ctty: 0}
	if err := job.Start(); err != nil {
		say("error: %v", err)
		os.Exit(1)
	}
	pid := job.Process.Pid
	say("the job is %d", pid)
	for {
		var status syscall.WaitStatus
		if _, err := syscall.Wait4(pid, &status, syscall.WUNTRACED, nil); err != nil {
			if err == syscall.EINTR {
				continue
			}
			say("error: %v", err)
			os.Exit(1)
		}
		if status.Exited() {
			say("the job ended: exit status %d", status.ExitStatus())
			return
		}
		if status.Signaled() {
			say("the job ended: killed by %s", unix.SignalName(status.Signal()))
			return
		}
		if !status.Stopped() {
			continue
		}
		if err := unix.IoctlSetPointerInt(0, unix.TIOCSPGRP, unix.Getpgrp()); err != nil {
			say("error: take the terminal back: %v", err)
		}
		say("the job was stopped by %s", unix.SignalName(status.StopSignal()))
		<-again
		if err := unix.IoctlSetPointerInt(0, unix.TIOCSPGRP, pid); err != nil {
			say("error: give the terminal to the job: %v", err)
		}
		say("the job goes on")
		syscall.Kill(-pid, syscall.SIGCONT)
	}
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

	// A request to stop (SIGTSTP). Nothing here would ever continue the
	// child: it leads its session, with no shell behind it. The system
	// does not stop such a process, and neither does the Terminal: the
	// request is dropped, the interface is not told to make way, and the
	// terminal stays the child's.
	cmd.Process.Signal(syscall.SIGTSTP)
	time.Sleep(300 * time.Millisecond)
	master.WriteString("b")
	expect("event: b")
	if !isRaw() {
		t.Error("the terminal is not in raw mode after a request to stop that was dropped")
	}
	// Ctrl+Z is a key; the child chooses to suspend on it, and is told
	// why it was not stopped. The terminal was not touched.
	master.WriteString("\x1a")
	expect("event: ctrl+z")
	expect("not suspended: " + ErrNoShell.Error())
	if !isRaw() {
		t.Error("the terminal is not in raw mode after a Ctrl+Z that stopped nothing")
	}
	master.WriteString("c")
	expect("event: c")

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

// TestSuspendUnderAShell runs the program as a job of a shell with job
// control, as gnosh is run: Ctrl+Z, and a request to stop from outside,
// give the terminal back as it was found and stop the job, so that the
// shell sees it stopped; and when the shell continues it, it takes the
// terminal back and says the interface must redraw. The same holds when
// the shell's job is another program that runs it, as env or sudo does.
func TestSuspendUnderAShell(t *testing.T) {
	t.Run("the shell's own job", func(t *testing.T) { suspendUnderAShell(t, "terminal") })
	t.Run("run by the shell's job", func(t *testing.T) { suspendUnderAShell(t, "wrapper") })
}

func suspendUnderAShell(t *testing.T, job string) {
	master, slave, err := openPTY()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer master.Close()
	err = onFd(master, func(fd int) error {
		return unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, &unix.Winsize{Row: 30, Col: 100})
	})
	if err != nil {
		t.Fatal(err)
	}
	before := settings(t, master)

	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self)
	cmd.Env = append(os.Environ(), "GNOSH_TERM_HELPER=shell", "GNOSH_TERM_JOB="+job)
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
	// started are the shell's job and, when that is not it, the program
	// with the terminal.
	var started []int
	t.Cleanup(func() {
		// Whatever went wrong, nothing is left behind: not the shell, and
		// not a job that is stopped for good.
		for _, pid := range started {
			syscall.Kill(pid, syscall.SIGKILL)
		}
		cmd.Process.Kill()
		cmd.Wait()
	})
	// What the terminal is sent is read as it comes, so that nothing the
	// job writes ever waits on a full terminal.
	var written safeBuffer
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			written.Write(buf[:n])
			if err != nil {
				return
			}
		}
	}()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	next := func(what string) string {
		t.Helper()
		select {
		case got, ok := <-lines:
			if !ok {
				t.Fatalf("the shell and its job ended before %s", what)
			}
			return got
		case <-time.After(30 * time.Second):
			t.Fatalf("neither the shell nor its job said anything, waiting for %s", what)
		}
		return ""
	}
	expect := func(want string) {
		t.Helper()
		if got := next(strconv.Quote(want)); got != want {
			t.Fatalf("the shell or its job said %q, want %q", got, want)
		}
	}
	isRaw := func() bool { return settings(t, master).Lflag&unix.ICANON == 0 }
	// wrote waits for what the job has written to the terminal to end
	// with something.
	wrote := func(end, when string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !strings.HasSuffix(written.String(), end) {
			if time.Now().After(deadline) {
				t.Errorf("%s the job had written %q to the terminal, want %q at the end", when, written.String(), end)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	// The terminal's settings are what they were before the job had it,
	// and the job turned its modes off as it gave the terminal up.
	givenBack := func(when string) {
		t.Helper()
		if after := settings(t, master); after != before {
			t.Errorf("%s the terminal's settings differ from those the job found:\n got %+v\nwant %+v", when, after, before)
		}
		wrote(ShowCursor+BracketedPasteOff, when)
	}
	goOn := func() {
		t.Helper()
		if err := cmd.Process.Signal(syscall.SIGUSR1); err != nil {
			t.Fatal(err)
		}
		expect("shell: the job goes on")
		// The job has the terminal again, says the interface must draw
		// afresh, and says what size the terminal is now.
		expect("event: resume")
		expect("event: resize 100x30")
		if !isRaw() {
			t.Error("the terminal is not in raw mode after the job was continued")
		}
		wrote(BracketedPasteOn, "after the job was continued")
	}

	// program is the process with the terminal: the shell's job, or the
	// one that job runs.
	// The shell and, with a wrapper, the job each say a number, each from
	// a process of its own on the one terminal: on a busy machine the two
	// lines come in either order.
	// (The job's "ready" is a third process's line, and may come before
	// either number.)
	numbers := map[string]int{}
	want := 1
	if job == "wrapper" {
		want = 2
	}
	ready := false
	for len(numbers) < want || !ready {
		line := next("the job's number")
		var n int
		switch {
		case scanned(line, "shell: the job is %d", &n):
			numbers["shell"] = n
		case job == "wrapper" && scanned(line, "wrapper: the program is %d", &n):
			numbers["wrapper"] = n
		case line == "ready" && !ready:
			ready = true
		default:
			t.Fatalf("the shell or its job said %q, want the job's number, the program's, or ready", line)
		}
	}
	program := numbers["shell"]
	started = append(started, program)
	if job == "wrapper" {
		program = numbers["wrapper"]
		started = append(started, program)
	}
	if !isRaw() {
		t.Error("the terminal is not in raw mode while the job has it")
	}

	// Ctrl+Z is a key; the job chooses to suspend on it. The shell sees
	// the job stopped, with the terminal as the job found it.
	master.WriteString("\x1a")
	expect("event: ctrl+z")
	expect("shell: the job was stopped by SIGSTOP")
	givenBack("with the job stopped by Ctrl+Z")
	goOn()
	master.WriteString("a")
	expect("event: a")

	// A request to stop from outside: the interface is told to make way
	// first, and the same follows.
	if err := syscall.Kill(program, syscall.SIGTSTP); err != nil {
		t.Fatal(err)
	}
	expect("told to stop")
	expect("shell: the job was stopped by SIGSTOP")
	givenBack("with the job stopped from outside")
	goOn()

	// A second stop works as the first did.
	master.WriteString("\x1a")
	expect("event: ctrl+z")
	expect("shell: the job was stopped by SIGSTOP")
	givenBack("with the job stopped a second time")
	goOn()

	master.WriteString("q")
	expect("event: q")
	expect("closed")
	expect("shell: the job ended: exit status 0")
	if err := cmd.Wait(); err != nil {
		t.Errorf("the shell ended with %v", err)
	}
	started = nil
	if after := settings(t, master); after != before {
		t.Errorf("settings after the job ended differ from those before it started:\n got %+v\nwant %+v", after, before)
	}
}

// safeBuffer collects what one goroutine reads for another to look at.
type safeBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// scanned reports whether line reads as format, one number in it.
func scanned(line, format string, n *int) bool {
	_, err := fmt.Sscanf(line, format, n)
	return err == nil
}

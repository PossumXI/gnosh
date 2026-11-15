//go:build linux

package relay

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// screen is a terminal for a test: the master of a pseudo-terminal whose
// slave is gnosh ssh's input and output. What the test writes is typed;
// what it reads is what reached the screen.
type screen struct {
	t       *testing.T
	master  *os.File
	slave   *os.File
	mu      sync.Mutex
	shown   bytes.Buffer
	readEnd chan struct{}
}

func newScreen(t *testing.T, rows, cols uint16) *screen {
	t.Helper()
	master, slave, err := term.OpenPTY()
	if err != nil {
		t.Fatal(err)
	}
	s := &screen{t: t, master: master, slave: slave, readEnd: make(chan struct{})}
	s.size(rows, cols)
	go func() {
		defer close(s.readEnd)
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			s.mu.Lock()
			s.shown.Write(buf[:n])
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		slave.Close()
		master.Close()
	})
	return s
}

// size sets the terminal's size.
func (s *screen) size(rows, cols uint16) {
	s.t.Helper()
	if err := unix.IoctlSetWinsize(int(s.master.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols}); err != nil {
		s.t.Fatal(err)
	}
}

// see waits for the screen to show piece.
func (s *screen) see(piece string) {
	s.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		s.mu.Lock()
		shown := s.shown.String()
		s.mu.Unlock()
		if strings.Contains(shown, piece) {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("the screen never showed %q; it shows:\n%q", piece, shown)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fakeSSH writes a stand-in for ssh that prints its arguments and its
// terminal's size, copies "copied text" (OSC 52), reads a line, prints the
// size again (after a resize, if the test made one), and ends with status
// 7.
func fakeSSH(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	script := `#!/bin/sh
echo "args: $*"
echo "size: $(stty size)"
printf '\033]52;c;%s\007' "$(printf 'copied text' | base64)"
IFS= read -r line
echo "got: $line"
echo "size: $(stty size)"
exit 7
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// gnosh ssh at a terminal: ssh gets its arguments, a terminal of the same
// size and settings, what is typed, and a new size when the terminal
// changes; what it writes reaches the screen as it was written; the text
// it copies reaches the clipboard; and gnosh ssh ends with ssh's status,
// with the terminal's settings as they were.
func TestRelay(t *testing.T) {
	scr := newScreen(t, 30, 100)
	before, err := unix.IoctlGetTermios(int(scr.slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var copied []string
	done := make(chan int, 1)
	go func() {
		done <- Run(Options{SSH: fakeSSH(t), Args: []string{"-p", "2222", "box"}, In: scr.slave, Out: scr.slave, Err: scr.slave,
			Copy: func(text string) error {
				mu.Lock()
				copied = append(copied, text)
				mu.Unlock()
				return nil
			}})
	}()
	scr.see("args: -p 2222 box")
	scr.see("size: 30 100")
	scr.see("\x1b]52;c;")
	// The relay's terminal is raw: what is typed reaches ssh's terminal
	// as it is, and that one, set up as this one was, makes Enter a line.
	scr.size(40, 120)
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := io.WriteString(scr.master, "hello\r"); err != nil {
		t.Fatal(err)
	}
	scr.see("got: hello")
	scr.see("size: 40 120")
	select {
	case code := <-done:
		if code != 7 {
			t.Errorf("gnosh ssh ended with %d, want ssh's 7", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("gnosh ssh did not end when ssh did")
	}
	mu.Lock()
	if len(copied) != 1 || copied[0] != "copied text" {
		t.Errorf("copied %q, want the one write", copied)
	}
	mu.Unlock()
	after, err := unix.IoctlGetTermios(int(scr.slave.Fd()), ioctlGetTermios)
	if err != nil {
		t.Fatal(err)
	}
	if after.Lflag != before.Lflag || after.Iflag != before.Iflag || after.Oflag != before.Oflag {
		t.Errorf("the terminal's settings were not put back: %+v, were %+v", after, before)
	}
}

// Without a terminal, or with no clipboard to copy to, gnosh ssh is ssh:
// nothing in between, and ssh's status.
func TestPlain(t *testing.T) {
	ssh := fakeSSH(t)
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	called := false
	code := Run(Options{SSH: ssh, Args: []string{"box"}, In: in, Out: out, Err: out, Copy: func(string) error { called = true; return nil }})
	if code != 7 || called {
		t.Errorf("status %d, copy called %v; want 7 and no copy", code, called)
	}
	b, _ := os.ReadFile(out.Name())
	if !strings.Contains(string(b), "args: box") || !strings.Contains(string(b), "\x1b]52;c;") {
		t.Errorf("ssh's output was not passed on: %q", b)
	}

	if code := Run(Options{SSH: filepath.Join(t.TempDir(), "no-ssh"), In: in, Out: out, Err: out}); code != 255 {
		t.Errorf("a missing ssh ended with %d, want 255", code)
	}
}

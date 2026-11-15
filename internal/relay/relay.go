// Package relay is gnosh ssh: ssh run on a pseudo-terminal of gnosh's
// own, with every byte passed through as it is, and the text of each
// clipboard write the far side sends (OSC 52) put on this machine's
// clipboard too. Apple's Terminal drops OSC 52, so without the relay what
// is copied on the far side (gnosh's /copy, tmux's copy mode) never
// reaches the Mac's clipboard; a shell function in Apple's Terminal can
// send each interactive ssh through gnosh ssh.
//
// Where there is no terminal, or no clipboard of this machine's to put
// text on, gnosh ssh is ssh with nothing in between.
package relay

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/gnosh-ai/gnosh/internal/clipboard"
)

// Options is what Run needs.
type Options struct {
	// SSH is the ssh to run: a path, or a name on the PATH.
	SSH string
	// Args are ssh's arguments, as they were given.
	Args []string
	// In, Out and Err are gnosh's standard input, output and error.
	In, Out, Err *os.File
	// Copy puts text on this machine's clipboard. When it is nil, ssh
	// runs with nothing in between.
	Copy func(text string) error
}

// copiesQueued is how many copies may wait for the clipboard; when more
// come, the oldest gives way, so that a clipboard that hangs never holds
// the screen up.
const copiesQueued = 16

// Main is gnosh ssh with args: it runs ssh, through the relay where there
// is a clipboard to relay to, and returns ssh's exit status.
func Main(args []string) int {
	o := Options{SSH: "ssh", Args: args, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
	if tool := clipboard.Tool(runtime.GOOS, os.Getenv, exec.LookPath); tool != nil {
		o.Copy = func(text string) error { return clipboard.Copy(context.Background(), tool, nil, text) }
	}
	return Run(o)
}

// Run runs ssh as o says and returns its exit status: 255, as ssh's own
// for an error, when it could not be run.
func Run(o Options) int {
	path, err := exec.LookPath(o.SSH)
	if err != nil {
		fmt.Fprintf(o.Err, "gnosh ssh: %v\n", err)
		return 255
	}
	if o.Copy != nil && isTerminal(o.In) && isTerminal(o.Out) {
		code, started, err := relay(path, o)
		if started {
			return code
		}
		fmt.Fprintf(o.Err, "gnosh ssh: the clipboard relay could not start (%v), so ssh runs without it.\n", err)
	}
	return plain(path, o)
}

// plain runs ssh on gnosh's own input and output.
func plain(path string, o Options) int {
	cmd := exec.Command(path, o.Args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = o.In, o.Out, o.Err
	// Ctrl+C and Ctrl+\ at a terminal reach ssh as well as gnosh: ssh
	// decides, and gnosh waits for it. Other signals are passed on.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(o.Err, "gnosh ssh: %v\n", err)
		return 255
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case sig := <-sigs:
				if sig == syscall.SIGTERM || sig == syscall.SIGHUP {
					_ = cmd.Process.Signal(sig)
				}
			case <-done:
				return
			}
		}
	}()
	return exitStatus(cmd.Wait())
}

// exitStatus is the status gnosh ssh ends with, for what ssh's Wait
// returned: ssh's own, or 128 and the signal that ended it.
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ee.ExitCode()
	}
	return 255
}

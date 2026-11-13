// Command gnosh is an AI coding harness for the terminal: it takes a
// prompt, talks to a model, runs the tools the model asks for under the
// person's own rules, and keeps the session on disk.
//
// This file is the process's entry point and nothing more. The program
// itself is internal/app, which is handed everything it may touch (the
// arguments, the three streams, the environment, the signals) so that a
// test can hand it something else. How the program is put together is told
// in that package's doc.go.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"golang.org/x/term"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/buildinfo"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
)

func main() {
	// The GnoVM worker is this same program, started by gnosh itself with
	// one word. It is dispatched here, before the command line is read,
	// before any config or environment is looked at, and before a signal
	// is caught: the worker speaks a protocol on two pipes and must not be
	// touched by anything a user's settings could change.
	if len(os.Args) > 1 && os.Args[1] == "gnovm-worker" {
		os.Exit(gnovm.WorkerMain())
	}

	// SIGINT and SIGTERM stop a run in order (the turn, its commands, the
	// SessionEnd hooks, the session's file); a second one ends the process
	// at once. SIGHUP, which is what a dropped ssh connection sends, is
	// handled as SIGTERM is: without that, the commands a session started
	// in the background would be left running with nobody to stop them.
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	cwd, _ := os.Getwd() // empty when it is not known; the program says so
	info, _ := debug.ReadBuildInfo()
	os.Exit(app.Main(context.Background(), app.Options{
		Args:             os.Args[1:],
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		StdinIsTerminal:  term.IsTerminal(int(os.Stdin.Fd())),
		StdoutIsTerminal: term.IsTerminal(int(os.Stdout.Fd())),
		Environ:          os.Environ(),
		Cwd:              cwd,
		Features:         features(),
		OpenTerminal:     func() (io.ReadCloser, error) { return os.Open("/dev/tty") },
		Signals:          signals,
		Exit:             os.Exit,
		Build:            cli.BuildFromInfo(info),
		BuildID:          buildinfo.BuildID,
	}))
}

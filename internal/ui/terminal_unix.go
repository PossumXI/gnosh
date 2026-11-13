//go:build unix

package ui

import (
	"os"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// openTerminal takes over the real terminal that in and out are open on.
//
// The terminal package stops the process for Ctrl+Z and for a request to
// stop from outside, and takes the terminal back when the shell continues
// it. With no shell that would continue the process it stops nothing:
// Suspend returns errNoShell, and a request from outside is dropped
// without OnStop being called.
func openTerminal(in, out *os.File, h TerminalHooks) (Terminal, error) {
	t, err := term.Open(in, out, term.Options{OnTerminate: h.OnTerminate, OnStop: h.OnStop})
	if err != nil {
		return nil, err
	}
	return t, nil
}

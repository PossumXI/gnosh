package ui

import (
	"io"
	"os"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// Terminal is the terminal as the interface uses it: a place to write to,
// a stream of what the person typed, and the few things that give the
// terminal up and take it back. *term.Terminal is one. The tests have
// another, which writes to an emulated screen.
type Terminal interface {
	io.Writer
	// Events is where keys, pastes, answers and changes of size arrive.
	// It is closed when the terminal's input ends.
	Events() <-chan term.Event
	// Size is the terminal's size in character cells.
	Size() (cols, rows int, err error)
	// SetModes turns the terminal's modes on and off to match.
	SetModes(term.Modes) error
	// QueryCursorPosition asks the terminal where the cursor is; the
	// answer arrives among the events.
	QueryCursorPosition() error
	// Release hands the terminal to another program, and Resume takes it
	// back; Suspend stops the process as Ctrl+Z does and takes the
	// terminal back when the shell continues it. Suspend returns an
	// error, with the terminal untouched, when the process could not be
	// stopped: errNoShell when nothing would continue it.
	Release() error
	Resume() error
	Suspend() error
	// RestoreOnPanic is deferred in every goroutine that runs while the
	// terminal is raw: it puts the terminal back as it was found if the
	// goroutine is panicking, and lets the panic go on.
	RestoreOnPanic()
	// Close puts the terminal back as it was found.
	Close() error
}

// TerminalHooks are what the interface asks of a terminal it opens.
type TerminalHooks struct {
	// OnTerminate is called after the process was told to end and the
	// terminal has been put back as it was found. Ending the process is
	// then the session's business: the interface leaves in order.
	OnTerminate func(os.Signal)
	// OnStop is called when the process is told to stop from outside,
	// before the terminal is given back to the shell. A Resume event
	// follows when the shell has continued the process.
	OnStop func()
}

// errNoShell is why Ctrl+Z did not stop the process.
var errNoShell = term.ErrNoShell

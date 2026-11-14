package term

import (
	"errors"
	"os"
	"time"
)

// DefaultEscWait is how long a Terminal waits, after a byte that could
// be the Esc key or the start of a longer sequence, for the rest to arrive
// before it decides it was the key.
const DefaultEscWait = 50 * time.Millisecond

// ErrClipboardOff is returned by CopyToClipboard when Options.Clipboard
// was not set.
var ErrClipboardOff = errors.New("writing to the clipboard through the terminal is turned off")

// ErrClosed is returned by a Terminal's methods after Close.
var ErrClosed = errors.New("the terminal is closed")

// ErrNoShell is returned by Suspend when nothing would continue the
// process once it was stopped: it was not started as a job of a shell.
// Nothing was stopped and the terminal was not touched.
var ErrNoShell = errors.New("gnosh was not started from a shell that could bring it back, so it was not stopped")

// Options adjust a Terminal. The zero value is what gnosh uses.
type Options struct {
	// EscWait replaces DefaultEscWait when it is positive.
	EscWait time.Duration

	// Clipboard allows CopyToClipboard, which writes to the person's
	// clipboard through the terminal (OSC 52). It is off unless they
	// asked for it.
	Clipboard bool

	// OnTerminate, when set, is called after the process was told to
	// end (SIGTERM, SIGHUP, SIGINT or SIGQUIT) and the terminal has been
	// put back as it was found. It runs on a goroutine of its own, and
	// ending the process is then up to it. When it is nil the Terminal
	// passes the signal on to the process with its usual effect, so the
	// process ends, with the terminal in order.
	OnTerminate func(os.Signal)

	// OnStop, when set, is called when the process is told to stop from
	// outside (SIGTSTP sent by another program; Ctrl+Z is a key here and
	// sends nothing), before the terminal is given back. It is the
	// interface's chance to erase its live region, as it does itself
	// before calling Suspend. The Terminal then stops the process as
	// Suspend does, and a Resume event follows when the shell continues
	// it. When nothing would continue the process (see ErrNoShell) the
	// request is dropped and OnStop is not called.
	OnStop func()

	// NoSignals stops the Terminal from listening for signals at all:
	// no Resize events, and nothing is restored when the process is
	// told to end or stop. It is for tests and for programs that handle
	// every signal themselves.
	NoSignals bool
}

// Modes are the terminal modes gnosh turns on for as long as it has the
// terminal. Terminal.SetModes turns them on and remembers them, so that
// they are turned off again whenever the terminal is given up (Close,
// Release, Suspend, a signal that ends the process) and on again when it
// is taken back.
type Modes struct {
	// BracketedPaste: pasted text arrives as one Paste event.
	BracketedPaste bool
	// FocusReporting: Focus events arrive.
	FocusReporting bool
	// KittyKeyboard: the "disambiguate escape codes" level of the kitty
	// keyboard protocol. Set it only when the terminal said it knows the
	// protocol (see Probe).
	KittyKeyboard bool
}

// on is the control sequences that turn the modes on.
func (m Modes) on() string {
	s := ""
	if m.BracketedPaste {
		s += BracketedPasteOn
	}
	if m.FocusReporting {
		s += FocusReportingOn
	}
	if m.KittyKeyboard {
		s += KittyKeyboardPush
	}
	return s
}

// off is the control sequences that turn the modes off again.
func (m Modes) off() string {
	s := ""
	if m.KittyKeyboard {
		s += KittyKeyboardPop
	}
	if m.FocusReporting {
		s += FocusReportingOff
	}
	if m.BracketedPaste {
		s += BracketedPasteOff
	}
	return s
}

// leaveSequence is what is written when the terminal is given up: the
// modes off, and the few things an interrupted update could have left
// wrong put right (styles, the terminal's line wrapping, the cursor).
func leaveSequence(m Modes) string {
	return StyleReset + AutoWrapOn + ShowCursor + m.off()
}

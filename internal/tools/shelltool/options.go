package shelltool

import (
	"os"
	"time"
)

// Options sets up a Manager. The zero value is usable: every field has a
// default, given in its comment. For the time and size limits, zero means
// the default.
type Options struct {
	// Shell is the shell that runs commands: a POSIX shell that takes -c
	// (bash, sh, dash, zsh). Empty means /bin/bash when it exists and
	// /bin/sh when it does not.
	Shell string

	// Environ returns the environment commands start from. Nil means
	// os.Environ, read anew for each command.
	Environ func() []string

	// Env is laid over that environment last, after the manager's own
	// settings (see the package comment for the list). "NAME=value" sets a
	// variable; "NAME" alone removes it.
	Env []string

	// SpoolDir is the directory in which the manager makes a directory of
	// its own for output files. Close removes what the manager made, never
	// SpoolDir itself. Empty means os.TempDir().
	SpoolDir string

	// Now is the clock used for start times and durations. Nil means
	// time.Now. Timeouts and graces always run on real time.
	Now func() time.Time

	// DefaultTimeout is how long a foreground command may run when the
	// model names no timeout. Default: 2 minutes.
	DefaultTimeout time.Duration

	// MaxTimeout is the longest a foreground command may run. A call that
	// asks for more gets this much. It is never less than DefaultTimeout.
	// Default: 10 minutes.
	MaxTimeout time.Duration

	// TimeoutToBackground says what happens to a foreground command whose
	// time runs out. False, the default: it is stopped, and the result
	// carries the output so far. True: it keeps running as a background
	// command and the result says so; that is what Claude Code does, and it
	// suits a session that can pass OnEnd's news on to the model. A command
	// that starts with sleep is stopped either way.
	TimeoutToBackground bool

	// BackgroundTimeout is how long a background command may run when the
	// model names no timeout. Default: no limit.
	BackgroundTimeout time.Duration

	// MaxBackgroundTimeout is the longest a background command may be
	// given; a call that asks for more gets this much, and so does a call
	// that names no timeout when BackgroundTimeout is not set. Default: no
	// ceiling.
	MaxBackgroundTimeout time.Duration

	// MaxBackground is how many background commands may run at once; ones
	// that have ended do not count. Default: 50.
	MaxBackground int

	// TermGrace is how long a command being stopped has between SIGTERM and
	// SIGKILL. Default: 2 seconds.
	TermGrace time.Duration

	// KillWait is how long to wait, after SIGKILL, for the processes to be
	// gone before saying that they could not be stopped. Default: 5
	// seconds.
	KillWait time.Duration

	// PipeGrace is how long to wait for a command's output to end once its
	// shell has exited, when a process the command started still holds the
	// output open. It is what leaving such a process behind costs a call.
	// Default: 500 milliseconds.
	PipeGrace time.Duration

	// MaxOutputChars is how many characters of a command's output the model
	// is shown. Longer output is shown as its start and its end, this many
	// characters together, around a line that says what was left out.
	// Default: 30,000.
	MaxOutputChars int

	// MaxFailedOutputChars is the same limit for a command that failed. It
	// is never more than MaxOutputChars. Default: 10,000.
	MaxFailedOutputChars int

	// MaxSpoolBytes is how much of one command's output is kept in its
	// output file. Output past it still counts and still shows in the end
	// of what the model sees, but is not kept. Negative means no limit.
	// Default: 64 MiB.
	MaxSpoolBytes int64

	// MaxOutputBytes is how much one command may print in all before it is
	// stopped. Negative means no limit. Default: 5 GiB.
	MaxOutputBytes int64

	// SpoolBudget is how much disk the output files of finished foreground
	// commands may take together; the oldest are removed to stay under it.
	// The output files of background commands are kept until Close.
	// Negative means no limit. Default: 1 GiB.
	SpoolBudget int64

	// TailChars is how many characters of a background command's last
	// output OnEnd is given. Default: 2,000.
	TailChars int

	// NoOutputTool leaves the TaskOutput tool out of Tools. The model then
	// reads a background command's output file with the file-reading tool,
	// which is how Claude Code has worked since 2.1.277.
	NoOutputTool bool

	// OnEnd, when not nil, is told that a background command has ended, so
	// that the agent can tell the model. It is called from a goroutine of
	// the manager's, one call at a time; it must not block, and must not
	// call Close. An end the model has already been shown (by TaskOutput or
	// TaskStop), and an end caused by Close, is not announced.
	OnEnd func(Ended)
}

// The defaults, and the fixed sizes that are not worth an option.
const (
	defaultTimeout        = 2 * time.Minute
	defaultMaxTimeout     = 10 * time.Minute
	defaultMaxBackground  = 50
	defaultTermGrace      = 2 * time.Second
	defaultKillWait       = 5 * time.Second
	defaultPipeGrace      = 500 * time.Millisecond
	defaultOutputChars    = 30_000
	defaultFailedChars    = 10_000
	defaultSpoolBytes     = 64 << 20
	defaultOutputBytes    = 5 << 30
	defaultSpoolBudget    = 1 << 30
	defaultTailChars      = 2_000
	defaultMaxLingering   = 32                     // processes left by ended commands that are listened to at once
	drainWait             = 30 * time.Second       // how long output already in the pipe is waited for
	pipeHolds             = 1 << 20                // more than a pipe holds at once, on any system in use
	longCommand           = 64 << 10               // a longer command reaches the shell through a file
	maxLineBytes          = 1 << 20                // a longer line is cut for the filter
	pumpBufferBytes       = 32 << 10               // how much output is read at a time
	progressBacklogBytes  = 256 << 10              // live output held for a slow listener
	progressPartialLinger = 100 * time.Millisecond // how long a line waits for its newline
)

// withDefaults returns o with every unset field filled in.
func (o Options) withDefaults() Options {
	if o.Environ == nil {
		o.Environ = os.Environ
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.DefaultTimeout <= 0 {
		o.DefaultTimeout = defaultTimeout
	}
	if o.MaxTimeout <= 0 {
		o.MaxTimeout = defaultMaxTimeout
	}
	o.MaxTimeout = max(o.MaxTimeout, o.DefaultTimeout)
	o.BackgroundTimeout = max(o.BackgroundTimeout, 0)
	o.MaxBackgroundTimeout = max(o.MaxBackgroundTimeout, 0)
	if o.MaxBackgroundTimeout > 0 && (o.BackgroundTimeout == 0 || o.BackgroundTimeout > o.MaxBackgroundTimeout) {
		o.BackgroundTimeout = o.MaxBackgroundTimeout
	}
	if o.MaxBackground <= 0 {
		o.MaxBackground = defaultMaxBackground
	}
	if o.TermGrace <= 0 {
		o.TermGrace = defaultTermGrace
	}
	if o.KillWait <= 0 {
		o.KillWait = defaultKillWait
	}
	if o.PipeGrace <= 0 {
		o.PipeGrace = defaultPipeGrace
	}
	if o.MaxOutputChars <= 0 {
		o.MaxOutputChars = defaultOutputChars
	}
	if o.MaxFailedOutputChars <= 0 {
		o.MaxFailedOutputChars = defaultFailedChars
	}
	o.MaxFailedOutputChars = min(o.MaxFailedOutputChars, o.MaxOutputChars)
	if o.MaxSpoolBytes == 0 {
		o.MaxSpoolBytes = defaultSpoolBytes
	}
	if o.MaxOutputBytes == 0 {
		o.MaxOutputBytes = defaultOutputBytes
	}
	if o.SpoolBudget == 0 {
		o.SpoolBudget = defaultSpoolBudget
	}
	if o.TailChars <= 0 {
		o.TailChars = defaultTailChars
	}
	return o
}

// Package shelltool is the model's way to run shell commands: the Bash
// tool, background commands, their output, and stopping them.
//
// A Manager belongs to one session. It hands out the tools (Tools), keeps
// track of every process it starts, and stops them all when it is closed
// (Close). The tools are Bash, which runs a command; TaskOutput, which
// returns what a background command has printed since it was last asked;
// and TaskStop, which stops a background command. Their names and the
// names of their arguments are Claude Code's, and all of them are in
// names.go.
//
// The package does not decide which commands may run. A call says what it
// would do through core.Describer, with the command exactly as the model
// wrote it, and the permission rules that judge it live elsewhere.
//
// # How a command runs
//
// Each command gets a new shell: /bin/bash where there is one, /bin/sh
// otherwise, or Options.Shell. The shell is not interactive and reads no
// start-up files of its own accord. It runs in the session's working
// directory, with the null device as its standard input, with stdout and
// stderr going down one pipe so that the two stay in the order written,
// and as the leader of a new session. A new session means a new process
// group, which is what gets signalled when the command must stop, and it
// means no terminal: a program that would open the terminal to ask for a
// password fails at once instead of waiting where nobody will answer.
//
// The command is never pasted into a script. It reaches the shell as an
// argument of its own (through a file when it is longer than 64 KiB, which
// is more than an argument may be) and is handed to eval, so that quotes,
// dollars, backslashes, here-documents and several lines mean what they
// mean at a prompt, and a mistake in the command is reported by the shell
// in its own words. Two things differ from "sh -c": the shell's messages
// say "eval" where they would say "-c", and the shell holds two variables
// of the wrapper's, __gnosh_cwd and __gnosh_cmd.
//
// The environment is the parent's (Options.Environ) with these laid over
// it, and then Options.Env over those:
//
//	PAGER=cat               no pager waits for a key
//	GIT_PAGER=cat           nor git's
//	GIT_TERMINAL_PROMPT=0   git fails rather than ask for a password
//	TERM=dumb               programs keep to plain text
//	GNOSH=1                 a script can tell that gnosh is running it,
//	AGENT=gnosh             and that an AI agent is,
//	AI_AGENT=gnosh          under either of the names in use
//	PWD=<working directory> so the shell's idea of where it is, is right
//	OLDPWD                  removed: it would be the harness's, not the session's
//
// # What carries over
//
// Only the working directory. When the shell exits, by reaching the end,
// by exit, or by an error, a trap writes the directory it is in to a file.
// If that directory is inside one of the session's roots, the result's Cwd
// names it and the session moves there; it is spelled under the root the
// way the session spells the root, whatever links lie between. If it is
// outside every root, Cwd stays empty and the result tells the model that
// the working directory was reset to where it was. A command that replaces
// the shell with exec, or sets an EXIT trap of its own, leaves no word of
// where it ended; the session stays put and the result says so. A command
// that was stopped never moves the session. Variables, functions, aliases
// and shell options are gone with the shell.
//
// # Time
//
// A foreground command may run for Options.DefaultTimeout (two minutes),
// or for the timeout the call asks for, up to Options.MaxTimeout (ten
// minutes); a larger number is lowered to that, as Claude Code does. When
// the time is up the command is stopped and the result carries the output
// so far. With Options.TimeoutToBackground it is moved to the background
// instead, where it runs on; Manager.Background does the same when the
// person asks. A background command has no time limit unless the call or
// the options give it one.
//
// # Stopping
//
// A command is stopped when its time runs out, when the context of the
// call waiting for it is cancelled, when TaskStop or Manager.Stop asks,
// when it has printed more than Options.MaxOutputBytes, and when the
// manager is closed. Every process group in the command's session gets
// SIGTERM (and SIGCONT, so that a stopped process can act on it); after
// Options.TermGrace, whatever is left gets SIGKILL; then the shell is
// reaped and the session is looked at until it is empty. The process group
// alone would miss the few programs that move to a group of their own, as
// timeout(1) does, so the session is searched: through /proc on Linux, and
// through the kernel's process table on macOS. Elsewhere only the shell's
// group is signalled. A process that starts a session of its own (setsid,
// a proper daemon) is not reached, and is not meant to be.
//
// A signal to a group reaches what is in the group at that instant. A
// process forked a moment later has missed it, and so, in effect, has one
// that was just then turning from a copy of its parent shell into the
// program it was forked to run. Either would leave the shell waiting for
// a child that knows nothing of the signal. So while the manager waits it
// keeps looking at the session, and sends the signal to any process that
// was not there when it went out, or has become another program since.
//
// Nothing waits without a limit. If a process is still alive
// Options.KillWait after SIGKILL, the result names it and says that it
// could not be stopped.
//
// # A process left running
//
// A command that ends but leaves something running that still holds its
// output open (sleep 300 &, a server started with &) does not hold the
// call up. Once the shell has exited, the kernel is asked whether any
// process still has the other end of the output pipe open. If none has,
// what is in the pipe is all there will be, and it is read to the end,
// however slow a busy machine is about it. If one has, the output is given
// Options.PipeGrace to end. Then the call returns with what had been
// printed by then and with the command's own exit code, and says that a
// process was left running, which process group it is in, and how to stop
// it.
//
// That process is not killed. What it prints from then on is still read,
// so that it is never blocked on a full pipe or killed by a closed one,
// and goes to the command's output file. It is stopped when the manager is
// closed, but only if it can be shown to be the same process: its group's
// id may by then belong to something else. Listening costs a goroutine and
// two open files for each such command, so at most 32 are listened to at
// once; what a further command leaves behind is stopped at once, and that
// command's result says so.
//
// A process that let go of the command's output (cmd >log 2>&1 &) is
// nothing to the manager: the command ends cleanly and the process is left
// alone, at Close too.
//
// # Output
//
// A command's output is written to a file of its own as it arrives, at
// most Options.MaxSpoolBytes of it (64 MiB), and only its first and last
// few tens of kilobytes are held in memory, so a command may print any
// amount. The model is shown at most Options.MaxOutputChars characters
// (30,000), or Options.MaxFailedOutputChars (10,000) of a command that
// failed. Longer output is shown as its start and its end, cut between
// lines where that costs little, around one line of the form
//
//	[1234 lines omitted, full output in /path/to/cmd-7.out]
//
// NUL bytes are dropped and bytes that are not valid UTF-8 become U+FFFD.
// The file keeps the bytes as they were. It is removed at once when the
// model was shown everything, and otherwise kept until Close, or until
// the files of finished foreground commands add up to more than
// Options.SpoolBudget, when the oldest go.
//
// A result is the output, with a line above it when the command did not
// simply end ("[timed out after 120 s and was stopped; the output so far
// is below]") and bracketed notes under it: the exit code when it is not
// 0, a process left running, where the working directory is now. A
// non-zero exit code makes the result an error, except exit code 1 from
// the programs for which 1 is an answer and not a failure (grep and its
// kin, find, diff, test, git diff, git grep; see harmlessExit). The exit
// code is always in the result's Meta, with the other facts of the call;
// the keys are in names.go.
//
// While a foreground command runs, its output is passed to the session's
// Progress function a line at a time (a line without its newline after a
// tenth of a second). The command never waits for the listener: output the
// listener is too slow for is dropped for the listener, who is told how
// much.
//
// # Background commands
//
// With run_in_background, Bash returns as soon as the command has started,
// with an id ("bg1", "bg2", ...) and the path of the output file.
// TaskOutput returns the output the model has not been given yet, cut to
// the same length as a foreground result, and says whether the command is
// still running or how it ended; with a filter, a regular expression, it
// returns only the new lines that match. TaskStop stops the command as
// described above. At most Options.MaxBackground (50) run at once.
// Options.NoOutputTool leaves TaskOutput out, for a session whose model
// reads the output file with the file-reading tool instead.
//
// When a background command ends, Options.OnEnd is told: which command,
// how it ended, how long it ran, and the end of its output. Each end is
// announced once: not if TaskOutput or TaskStop has already shown it to
// the model, and not if Close caused it. Manager.Tasks lists the
// background commands and Manager.Stop stops one, for the person's own
// controls.
//
// # Closing
//
// Close stops everything that is running, waits until it is gone, and
// removes the manager's output files and the directory it made for them.
// It is bounded like everything else, and its error says what, if
// anything, could not be stopped.
package shelltool

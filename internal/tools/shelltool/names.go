package shelltool

// This file is the one table of names. Every name the model, a permission
// rule, a hook or the transcript can see is spelled here and nowhere else:
// change one here and the schemas, the descriptions, the argument decoding
// and the results all follow.
//
// The tool and argument names are Claude Code's, from its public tools
// reference, so that rules, hooks and habits written for it carry over.

// The tools.
const (
	// BashName is the tool that runs a shell command.
	BashName = "Bash"
	// OutputName is the tool that returns what a background command has
	// printed since it was last asked. Claude Code had it under this name
	// until 2.1.277 and now has the model read the output file instead; see
	// Options.NoOutputTool.
	OutputName = "TaskOutput"
	// StopName is the tool that stops a background command.
	StopName = "TaskStop"
)

// The arguments.
const (
	argCommand     = "command"           // Bash: the command, exactly as it is to run
	argDescription = "description"       // Bash: what the command does, in a few plain words
	argTimeout     = "timeout"           // Bash: milliseconds the command may run
	argBackground  = "run_in_background" // Bash: start it and return at once
	argTaskID      = "task_id"           // TaskOutput, TaskStop: the id Bash gave a background command
	argTaskIDOld   = "shell_id"          // the same, as Claude Code spelled it before; read but not advertised
	argFilter      = "filter"            // TaskOutput: a regular expression that picks lines
)

// The keys of ToolResult.Meta. Where Claude Code's structured Bash result
// has a field for the same fact, the key is that field's name.
const (
	metaExitCode       = "exitCode"                 // int: the shell's exit code; 128+n when signal n killed it; -1 when not known
	metaSignal         = "signal"                   // string: the signal that killed the shell, such as "SIGKILL"
	metaReason         = "reason"                   // string: one of the Reason constants
	metaInterrupted    = "interrupted"              // bool: stopped because the call was cancelled or the session closed
	metaDuration       = "durationMs"               // int64: how long the command ran
	metaDir            = "cwd"                      // string: the directory the command ran in
	metaOutputBytes    = "outputBytes"              // int64: how much the command printed in all
	metaTruncated      = "truncated"                // bool: the model was shown less than all of the output
	metaOutputFile     = "persistedOutputPath"      // string: the file that holds the output
	metaOutputFileSize = "persistedOutputSize"      // int64: how much of the output that file holds
	metaTaskID         = "backgroundTaskId"         // string: the id of a background command
	metaTimedOutAfter  = "timedOutAfterMs"          // int64: the timeout that ran out
	metaLeftRunning    = "leftRunning"              // []int: process groups the command left behind
	metaExitMeaning    = "returnCodeInterpretation" // string: what a non-zero exit code that is not a failure means
	metaRunning        = "running"                  // bool: TaskOutput, TaskStop: the command is still running
	metaTaskIDPlain    = "task_id"                  // string: TaskOutput, TaskStop: the command asked about
	metaTaskType       = "task_type"                // string: TaskStop: always taskTypeShell
	metaCommand        = "command"                  // string: TaskStop: the command that was stopped
)

// taskTypeShell is what Claude Code calls a background shell command in its
// task messages.
const taskTypeShell = "local_bash"

// The ways a command can end, as they appear in Task.Reason, Ended.Reason
// and ToolResult.Meta.
const (
	// ReasonExited: the shell ended by itself, with whatever exit code.
	ReasonExited = "exited"
	// ReasonTimeout: its time ran out and it was stopped.
	ReasonTimeout = "timeout"
	// ReasonCancelled: the tool call that was waiting for it was cancelled.
	ReasonCancelled = "cancelled"
	// ReasonStopped: the stop tool, or Manager.Stop, stopped it.
	ReasonStopped = "stopped"
	// ReasonClosed: the manager was closed while it ran.
	ReasonClosed = "closed"
	// ReasonOutputLimit: it printed more than Options.MaxOutputBytes.
	ReasonOutputLimit = "output_limit"
)

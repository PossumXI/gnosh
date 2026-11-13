package app

import (
	"context"
	"io"
	"os"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/hooks"
)

// Frontend is whatever stands between a session and whoever watches it: a
// person at a terminal, or a script reading standard output. The session
// code knows nothing else of either. Every front end gnosh has is one: the
// three writers of a run (text, json, stream-json), the plain interactive
// session that reads lines, and later the full interface, which is
// attached through Options.Interactive without a change to the session
// code.
//
// A front end is told everything that happens (Event), puts questions to
// the person when one arrives as an event, and drives the session through
// the handful of calls of Controls.
type Frontend interface {
	// Run drives the session from its first prompt to its end and returns
	// the program's exit code. It is called once, on the program's main
	// goroutine, when the session is ready. It returns promptly when ctx
	// ends. Stopping what runs, the SessionEnd hooks and closing the
	// session's file are done by the session code after Run returns.
	Run(ctx context.Context, c Controls) int

	// Event is told everything that happens in the session, in order and
	// one call at a time, from a goroutine of the session's. ev is a
	// pointer to one of the loop's events (*agent.TextEvent,
	// *agent.ToolStartEvent, *agent.ToolEndEvent, *agent.NoticeEvent,
	// *agent.TurnEndEvent and the rest of that package's list) or to one
	// of this package's (*HookEvent, *CapEvent). A front end passes over
	// the kinds it does not know.
	//
	// A question for the person arrives as an *agent.QuestionEvent and is
	// answered with Controls.Answer; an *agent.AnswerEvent says that it is
	// over, also when it was taken back unanswered.
	//
	// Event may take its time: events wait for it in order, and the
	// session never waits for a front end. It must not call
	// Controls.Command or Controls.Wait, which wait for the session.
	Event(ev any)
}

// Asker is a Frontend that can put a question to the person before a
// session exists: whether the folder is trusted, which model to use. A
// front end that is not one is treated as having nobody to ask.
type Asker interface {
	// AskBefore puts the question and returns the answer. An error means
	// the person left instead of answering.
	AskBefore(q agent.Question) (agent.Answer, error)
}

// FrontendInfo is what the maker of an interactive front end is given
// (Options.Interactive).
type FrontendInfo struct {
	// Stdin, Stdout and Stderr are the process's streams.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Getenv looks up an environment variable of the process.
	Getenv func(string) string
}

// Controls is the handful of calls a front end makes on a session. Every
// method may be called from any goroutine, and none waits for a turn
// unless it says so. In an interactive session /clear puts a new session
// behind the same Controls: a front end keeps the one it was given.
type Controls interface {
	// Submit sends a prompt and returns its id at once. A prompt sent
	// while a turn runs is queued. What becomes of it arrives as events: a
	// *agent.TurnEndEvent whose result names the prompt, or a
	// *agent.PromptReturnedEvent when it never went out.
	Submit(p agent.Prompt) (id string, err error)
	// Interrupt stops the turn that is running, all of it and at once. It
	// is what Esc and the first Ctrl+C do.
	Interrupt()
	// Answer gives the person's answer to the question with that id.
	Answer(question string, a agent.Answer) error
	// Queued lists the prompts that wait for their turn, and TakeQueued
	// takes the person's own off the queue for editing, as Up does in an
	// empty input box.
	Queued() []agent.QueuedPrompt
	TakeQueued() []agent.Prompt

	// Mode returns the permission mode in force, and SetMode changes it:
	// default, acceptEdits, plan, dontAsk or bypassPermissions. Skipping
	// permissions can only be switched to in a session that was started
	// with it within reach.
	Mode() string
	SetMode(mode string) error

	// Command runs a line that begins with a slash as a command of
	// gnosh's. Handled is false when the line names no command, and the
	// front end then sends it as a prompt or gives it back. It waits for
	// the command, and so must be called from Run and never from Event.
	Command(ctx context.Context, line string) (CommandResult, error)

	// State says what the session is doing right now and what it is
	// waiting for, and Context how full the model's window is. Neither
	// ever blocks, so a session that seems stuck can always be asked.
	State() agent.State
	Context() agent.ContextStatus
	// Info describes the session: its id, its model, its tools.
	Info() SessionInfo
	// Usage returns what the session has cost so far.
	Usage() agent.Totals

	// Signals delivers the SIGINT and SIGTERM the process receives. What
	// each means is the front end's to say: a run stops and exits, an
	// interactive session stops the turn at the first Ctrl+C.
	Signals() <-chan os.Signal
	// Notify fires the person's Notification hooks: kind is
	// "permission_prompt" (a question has waited a while) or "idle_prompt"
	// (a turn finished a while ago and nothing was typed since). It does
	// not wait for them.
	Notify(kind, title, message string)
	// RunEnd is called by the front end of a non-interactive run when its
	// last turn has ended, with the exit code the run would end with, and
	// returns the code it ends with: a feature may raise it.
	RunEnd(res agent.TurnResult, exit int) int
	// Wait waits until the session has nothing in hand and the front end
	// has been told everything that happened so far, or until ctx ends.
	// It must be called from Run and never from Event.
	Wait(ctx context.Context) error
	// Quit says why the session is ending, in the SessionEnd hooks' words:
	// "prompt_input_exit" when the person left, "other" otherwise. The
	// front end then returns from Run, and the session is closed.
	Quit(reason string)
}

// CommandResult is what running a slash command came to.
type CommandResult struct {
	// Handled says the line named a command and it was run.
	Handled bool
	// Text is what the command said, for the person.
	Text string
	// Quit says the command asked for the session to end.
	Quit bool
	// Name is the command that ran, without its slash.
	Name string
}

// SessionInfo describes a session to its front end.
type SessionInfo struct {
	// ID is the session's id and Path its file.
	ID   string
	Path string
	// Cwd is the working directory it started in.
	Cwd string
	// Provider and Model name the model main requests go to, Address is
	// where its server lives, and Thinking its thinking level.
	Provider string
	Model    string
	Address  string
	Thinking string
	// Tools are the tools the model may call, by name, in the order it is
	// told of them.
	Tools []string
	// Mode is the permission mode in force.
	Mode string
	// Commands are the slash commands, by name.
	Commands []string
	// Version names the build.
	Version string
	// Interactive says a person is there to be asked.
	Interactive bool
	// Resume is the command line that opens this session again.
	Resume string
	// NotifyHooks says a Notification hook is configured, so that the
	// timers behind Controls.Notify are worth keeping.
	NotifyHooks bool
}

// HookEvent says that hooks ran at one of the session's moments, and what
// each of them did. Every hook that ran is in it, also the ones that had
// nothing to say, so that no hook is ever invisible.
type HookEvent struct {
	// Event is the moment.
	Event hooks.Event
	// CallID and Tool name the tool call the hooks ran on, for the tool
	// events.
	CallID string
	Tool   string
	// Records has one entry for every hook that ran, in the order they
	// are configured.
	Records []hooks.Record
}

// CapEvent says a non-interactive run reached the number of replies it was
// given with --max-turns, and its turn is being stopped there.
type CapEvent struct {
	// Turns is the number that was given.
	Turns int
}

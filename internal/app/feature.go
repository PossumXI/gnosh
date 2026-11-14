package app

import (
	"context"
	"io"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/agentdef"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/hooks"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// This file is the seam between the wiring and the parts of gnosh that are
// built apart from it: the GnoVM machines and mem first of all. Such a part
// is a Feature. It is joined to the program without a change to this
// package: the program's main hands its features to Main (Options.Features),
// and everything a feature needs of the harness, and everything it may add
// to it, is in the types below.
//
// # The life of a feature
//
// In the order things happen in one run of the program:
//
//  1. The GnoVM worker dispatch. It is not a feature's: the program's main
//     looks at its first argument before it calls into this package at all,
//     and when that argument is "gnovm-worker" it runs the worker and exits.
//     Nothing below happens in such a process.
//  2. Feature.Boot, for each feature in the order of Options.Features,
//     before the command line is parsed and before any config or
//     environment is read. A Boot that reports done ends the program with
//     its exit code.
//  3. The command line is parsed against a table in which every name of
//     every feature's Built list is marked as built.
//  4. A subcommand a feature runs (Feature.Commands) is run now, with the
//     folder's config loaded and no model made. The program then exits.
//  5. For a session: the folder's trust is settled, the config is loaded
//     (with every feature's tool names and variables known to it), and
//     every check is run. Then Feature.Session is called, features in
//     order, once the session's facts are known and before its agent
//     exists. What each returns is joined into the session as the fields of
//     Contribution say.
//  6. Contribution.Ready, once the agent exists and before the first
//     prompt is sent.
//  7. While the session runs: the moments, in the places written beside
//     each field of Contribution.
//  8. When the session ends: the person's SessionEnd hooks; the shell
//     manager is closed, so that no command outlives the session; then
//     Contribution.Close, features in reverse order; then the session's
//     file is flushed to disk and its writer's lock is let go.
//  9. When the program ends, after the last session's file is flushed and
//     released: Feature.Shutdown, features in reverse order. This is where
//     a client that the whole process shares is closed.
//
// A sub-agent's session goes through the same steps in small. When the
// Agent tool starts a sub-agent (subagent.go), every feature's Session is
// called again for it, with Session.SubAgent set, Session.ID the
// sub-agent's own id, Session.Parent the session the person is in and
// Session.AgentType its kind, before the sub-agent exists. Of what the
// feature returns for it, these are joined to the sub-agent: Tools,
// System, PromptLines, BeforeRequest, Busy, Drivers and LateDrivers,
// AfterTool, BeforeSummary and AfterSummary, TurnEnded, Stored,
// AfterStored and Slid, ModeChanged, Ready and Close (called when the
// sub-agent has handed back its report). The rest belong to the session the person is
// in and are not used. A tool call a sub-agent makes is given the
// sub-agent's own id as its session (core.ToolEnv.SessionID) and the id
// of the session that started it beside it (ParentSessionID), and
// agent.FromContext inside the call returns the sub-agent.
//
// # Order, where several hands add to the same thing
//
// gnosh's own come first, then the features' in the order of
// Options.Features, each feature's own list in the order it gives it;
// except where a field below says otherwise. In particular:
//
//	tools           gnosh's own (Read, Write, Edit, Glob, Grep, Bash,
//	                TaskStop, Agent), then the features'. The order is the
//	                order the model is told of them, and it never changes
//	                between two sessions with the same config.
//	prompt lines    gnosh's own (the instruction files, the kinds of
//	                sub-agent a person or the project defined, and then
//	                the environment block, ahead of a session's first
//	                prompt), then the features'. Before all of them the
//	                loop has placed what the SessionStart and
//	                UserPromptSubmit hooks added. So a feature's last
//	                provider writes the last hidden line of a prompt.
//	reminders       gnosh's own, then the features'.
//	after a tool    gnosh's own effects (instruction files of a directory
//	                first worked in), then the features' AfterTool, then
//	                the person's PostToolUse hooks. What each adds for the
//	                model is stored in that order; any of them may end the
//	                turn. All of this is asked as soon as the call has
//	                returned, before its result is stored.
//	stored          every entry the loop stores is shown to the features'
//	                Stored first, and what they return is written with it
//	                in one write; once it is in the file the features'
//	                AfterStored are told, in order, before the loop goes
//	                on. A round's results are stored in the order the
//	                model made the calls, and a call that runs alone is
//	                not started before the result of the one before it
//	                has been through both.
//	turn-end        the features' Drivers, then the person's Stop hooks,
//	                then the features' LateDrivers; /loop comes after all
//	                of them, in the loop itself. The first driver that
//	                sends the model back is the only one asked that time.
//	busy            held in order, let go in reverse order.
//	after summary   gnosh's own (the instruction files in effect from the
//	                start are given back, and the ones given during the
//	                session are forgotten, so that they are given again),
//	                then the features', then the SessionStart hooks with
//	                the source "compact".

// Feature is a part of gnosh that is joined to the wiring at program
// start. It is plain data: every field may be left out, and a field added
// here later does not break a feature written today.
type Feature struct {
	// Name names the feature in messages: "gnovm".
	Name string

	// Tools lists every tool the feature may give a session, by name. The
	// list is needed before any config is read, because it is part of the
	// one list of tool names: a permission rule, --tools and a hook
	// matcher may name these tools, and a name that is in no list is a
	// startup error. A tool that Session returns without its name being
	// here is refused.
	Tools []ToolName

	// Env lists the GNOSH_ environment variables the feature reads itself.
	// A GNOSH_ variable that nobody claims stops startup as a likely
	// misspelling.
	Env []string

	// Built lists the names of the command-line table that the feature
	// makes work, as cli.Table.SetStatus takes them: "mem", "--goal". A
	// name the table does not have stops the program at once, so a
	// misspelt one cannot pass for done.
	Built []string

	// Boot, when not nil, is called before the command line is parsed and
	// before any config or environment is read, with the arguments after
	// the program's name. When it reports done, the program exits with
	// code. The GnoVM worker dispatch is not done here: it sits in the
	// program's main, ahead of everything in this package.
	Boot func(args []string) (code int, done bool)

	// Commands are the subcommands of the gnosh command line that the
	// feature runs, such as `gnosh mem`. Each must also be in Built.
	Commands []Command

	// Session, when not nil, is called once for every session, when its
	// facts are known and before its agent exists. An error stops the
	// session from starting and is shown to the person as it is.
	Session func(ctx context.Context, s *Session) (*Contribution, error)

	// Shutdown, when not nil, is called once as the program ends, after
	// the last session's file has been flushed and its lock released.
	Shutdown func(ctx context.Context)
}

// ToolName names one tool a feature may contribute, with what the rest of
// the program has to know about it before it sees the tool itself.
type ToolName struct {
	// Name is the tool's name, as the model, permission rules and hooks
	// see it.
	Name string
	// Quiet says a call of the tool never needs a person's approval: it
	// touches nothing outside what the feature itself keeps. A tool that
	// is not quiet asks before every call unless a rule or the mode allows
	// it. Deny rules, ask rules and hooks apply to a quiet tool as to any
	// other.
	Quiet bool
	// Guards lists the Claude Code tool names whose hooks also guard this
	// tool, for a tool Claude Code has nothing like (hooks.Options.Guards).
	// A hook that fits only this way can block the call and never approve
	// it.
	Guards []string
	// NoLogArgs says the tool's arguments and results are never written to
	// gnosh's log or shown in debugging output (gnosh.log, --debug-file,
	// --verbose): such a line names the tool and how the call went, and
	// nothing more. It is for a tool whose arguments may hold what the
	// person later asks to be forgotten. The session's own file still
	// holds the call, which is the feature's to erase.
	NoLogArgs bool
}

// Command is a subcommand of the gnosh command line that a feature runs.
type Command struct {
	// Command is the command of the command-line table it runs, such as
	// cli.CmdMem.
	Command cli.Command
	// Run runs it and returns the program's exit code. No model is made
	// and no provider is asked anything: Run must work with the model
	// server off and with no provider configured at all.
	Run func(ctx context.Context, c *CommandContext) int
}

// CommandContext is what a feature's subcommand is given.
type CommandContext struct {
	// Invocation is the command line as parsed. Invocation.Args holds the
	// words after the command's name.
	Invocation cli.Invocation
	// Config is the configuration in force for the folder, or nil when it
	// could not be loaded; Problems then says why. A command that only
	// reads gnosh's own data may go on without it.
	Config   *config.Config
	Problems []config.Problem
	// Dirs are the directories in use. Dirs.Session is empty: there is no
	// session. With a nil Config they are worked out from the environment
	// and the folder alone, so that a command which needs no config still
	// knows where things are; Dirs.Project, which a setting can move, is
	// then empty.
	Dirs Dirs
	// Sessions is the store the folder's sessions are kept in. It is
	// never nil.
	Sessions *transcript.Store
	// Stdin, Stdout and Stderr are the process's own.
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Getenv looks up an environment variable of the process.
	Getenv func(string) string
}

// Dirs are the directories one run of gnosh uses. Every path is absolute.
type Dirs struct {
	// Home is the person's home directory, Cwd the working directory the
	// session starts in, and ProjectRoot where its project begins.
	Home        string
	Cwd         string
	ProjectRoot string
	// Config is the person's config directory, State the directory gnosh
	// keeps its own data for the person in (the state file, the trust
	// record; a feature's process-wide data belongs in a folder of its own
	// under it, such as gnovm), and Cache the cache directory.
	Config string
	State  string
	Cache  string
	// Project is the project's data folder: .gnosh, or the directory given
	// with -D. It may not exist yet.
	Project string
	// Sessions is the root of the session store: State, or the directory
	// given with -D.
	Sessions string
	// Session is the session's own folder, beside its file: the place for
	// what belongs to one session and should go when the session is
	// deleted, such as the output files of its commands. It is made when
	// first needed.
	Session string
}

// Session is one session as a feature sees it: the facts it is given. The
// fields do not change after Feature.Session returns; the functions may be
// called for as long as the session lives, from any goroutine.
type Session struct {
	// ID is the id of this session's own log. For a main session that is
	// the session the person is in. For a sub-agent's session SubAgent is
	// set, Parent is the session the person is in, and AgentType says
	// what kind of sub-agent it is ("general-purpose", "Explore", or the
	// name of a definition).
	ID        string
	Parent    string
	SubAgent  bool
	AgentType string
	// AgentDef is the definition of a sub-agent's kind, as the Agent tool
	// found it: a feature that wants to know what the kind asks for reads
	// it here and need not look for the file. It is nil in the session
	// the person is in.
	AgentDef *agentdef.Def

	// Interactive says a person is there to be asked: the session was
	// opened in a terminal. It is false in a run (-p, gnosh run).
	Interactive bool
	// Source is how the session came to be, in the SessionStart hook's
	// words: "startup", "resume", "fork" or "clear"; and "subagent" for a
	// sub-agent's.
	Source string
	// Trusted says the folder's own settings are in force.
	Trusted bool

	// Config is the configuration in force, and Invocation the command
	// line as parsed.
	Config     *config.Config
	Invocation cli.Invocation
	// Dirs are the directories in use.
	Dirs Dirs
	// Build names this build of the program (buildinfo.BuildID).
	Build string

	// Sessions is the store the session is kept in, and Transcript the
	// session's own file. A feature may append entries of its own to it
	// (frame entries, meta entries) and read what it holds; the loop's
	// own entries reach it through this same value.
	Sessions   *transcript.Store
	Transcript *transcript.Session

	// Texts are the prompt texts in force: gnosh's own, with the person's
	// replacements.
	Texts *prompt.Set

	// Env returns what a tool call of this session is given to work with,
	// as it stands at the moment of the call: the working directory, the
	// roots, the session's id and the Store in which tools keep their own
	// state for the session. The Store is the same one in every Env and
	// in every tool call of the session.
	Env func() *core.ToolEnv

	// Offered reports whether a tool of that name would be offered to the
	// model in this session: --tools lists it (or was not given), and no
	// deny rule names the whole tool. A feature need not check: a tool it
	// returns that is not offered is left out.
	Offered func(tool string) bool

	// Agent returns the session's agent, or nil before it exists. It
	// exists by the time Contribution.Ready is called.
	Agent func() *agent.Agent

	// Notice puts a line in front of the person: in the conversation of an
	// interactive session, on stderr in a run. It works before the agent
	// exists; such a line is not kept in the session's file. It never
	// waits for the session, so it may be called from anywhere, the
	// callbacks that run while the loop is storing something (Stored,
	// AfterStored, Slid) included: the person is shown the line at once,
	// and it is kept in the session's file right after what was being
	// stored. The words of a feature's notice are not written to gnosh's
	// log, which only notes that there was one: what a notice quotes, the
	// feature can later erase from the session's file and from nowhere
	// else.
	Notice func(level agent.Level, source, text string)

	// Tasks sets the session's task list: the short list of what the
	// session is working through, which the front end shows under the
	// prompt. The list given replaces the one before; an empty one clears
	// it. It may be called from any goroutine, at any time after
	// Feature.Session has returned.
	Tasks func(tasks []Task)

	// Status sets a named fact for the front end's status line: "frame",
	// the frame the session is in; "goal", the goal it works towards. An
	// empty value takes the fact off. It may be called from any goroutine
	// and from any callback.
	Status func(name, value string)

	// Mode returns the permission mode in force right now: "default",
	// "acceptEdits", "plan", "dontAsk" or "bypassPermissions". The mode
	// can change while a session runs (the person switches it, or answers
	// a question with "switch to acceptEdits"), so a feature that acts on
	// it reads it when it acts. Contribution.ModeChanged is told of every
	// change.
	Mode func() string

	// Getenv looks up an environment variable of the process, and Now
	// reads the clock the program was given.
	Getenv func(string) string
	Now    func() time.Time
}

// Task is one line of a session's task list.
type Task struct {
	// Content says what is to be done, as an order: "Run the tests".
	// ActiveForm says the same as it reads while it is being done:
	// "Running the tests". An empty ActiveForm is shown as Content.
	Content    string
	ActiveForm string
	// Status is "pending", "in_progress" or "completed".
	Status string
}

// The values of Task.Status.
const (
	TaskPending    = "pending"
	TaskInProgress = "in_progress"
	TaskCompleted  = "completed"
)

// Contribution is what a feature adds to one session. Every field may be
// left out. The comment on each says when it is used; where several
// features and gnosh itself add to the same thing, the order is the one
// written at the top of this file.
type Contribution struct {
	// Tools are added to the tools the model may call, after gnosh's own.
	// Each name must be in Feature.Tools. A tool that is not offered in
	// this session (--tools, a deny rule) is left out, without a word. A
	// tool that implements agent.Concurrent may run beside other calls;
	// any other runs alone and in order, with everything before it stored.
	Tools []core.Tool

	// System, when not nil, may change what the system prompt is built
	// from, after gnosh has filled it in: the tools as they will be
	// offered, whether the folder holds Gno code, and what the command
	// line replaces or appends. (The instruction files are not part of
	// it: they go to the model with the first prompt, so that the system
	// prompt is the same for every project.) The part about mem is
	// sent whenever gnovm_run is among the tools; which of its two texts
	// (SystemInput.Memory) is what the config's option for it says
	// (config.Options.MemoryText), and may be changed here. Nothing that
	// changes from session to session may be put in: the system prompt
	// is the same bytes in every session with the same config.
	System func(in *prompt.SystemInput)

	// PromptLines are asked, in order, for the hidden lines of a prompt at
	// the moment it goes out (agent.Moments.PromptLines), after gnosh's
	// own. They are asked for hidden prompts too; LineInfo.Prompt.Hidden
	// tells them apart. A provider returns its text already marked the
	// way gnosh marks its own words (prompt.Wrap), or between its own
	// tags.
	PromptLines []func(ctx context.Context, info agent.LineInfo) ([]agent.Part, error)

	// BeforeRequest are asked, in order, before every request of a turn
	// for reminders to put in front of the model
	// (agent.Moments.BeforeRequest).
	BeforeRequest []func(ctx context.Context, info agent.RequestInfo) ([]agent.Part, error)

	// Busy, when not nil, is called when a prompt makes the idle session
	// busy, and what it returns is called when the session goes idle
	// again, after every prompt queued meanwhile has had its turn
	// (agent.Moments.Busy). Its context ends when the person stops the
	// turn, and it must return promptly then.
	Busy func(ctx context.Context) (release func())

	// Drivers are asked, in order, whether a turn may end, before the
	// person's Stop hooks. LateDrivers are asked after the Stop hooks.
	// /loop comes after both.
	Drivers     []agent.Driver
	LateDrivers []agent.Driver

	// AfterTool, when not nil, is called after every tool call that ran,
	// failed ones included, after gnosh's own effects and before the
	// person's PostToolUse hooks (agent.Moments.AfterTool). It is asked as
	// soon as the call has returned, which is before its result is stored,
	// and for tools that run side by side it may be asked for two calls at
	// once. What it returns is lines for the model, stored after the
	// round's results, and whether the turn is to end. What must follow
	// the stored result, in the order of the calls, belongs in
	// AfterStored.
	AfterTool func(ctx context.Context, done agent.ToolDone) (agent.ToolVerdict, error)

	// BeforeSummary, when not nil, may add to the instructions for a
	// summary, and AfterSummary says what the session is given back once
	// one is stored (agent.Moments.BeforeSummary and AfterSummary).
	BeforeSummary func(ctx context.Context, info agent.SummaryInfo) (string, error)
	AfterSummary  func(ctx context.Context, info agent.SummaryInfo) ([]agent.Part, error)

	// TurnEnded, when not nil, is told how every turn ended, after its
	// last entry is stored and after gnosh's own bookkeeping.
	TurnEnded func(ctx context.Context, res agent.TurnResult)

	// Stored, when not nil, is shown every entry the loop appends to the
	// session's log, just before it is written, and returns entries of the
	// feature's own to write right after it, in the same write: either
	// all of them reach the file or none. It is how a frame's push and pop
	// are recorded beside the result of the call that made them. It is
	// called while the log is being written, so it must not append to the
	// transcript itself, and must not wait on anything.
	//
	// Stored, AfterStored and Slid all run while the loop holds its log.
	// From inside them a feature may read the transcript, raise a notice
	// (Session.Notice, or the agent's Notice: neither waits for the log),
	// set the task list (Session.Tasks), hand the agent a note or a prompt
	// (Tell, Submit: both only queue), and ask for the session's state.
	// It must not call anything of the agent's that waits for the session
	// or takes a prompt back (Run, Wait, Drain, TakeQueued, Close): the
	// session is waiting for the callback.
	Stored func(e agent.Entry) []transcript.Entry

	// AfterStored, when not nil, is told of every entry the loop appended
	// once it is in the session's file: the entry, and what was written
	// for it, which is the entry's own lines followed by what the
	// features' Stored functions put beside it, each with the id it was
	// given. It is called for one entry at a time, in the order the loop
	// stored them, and the loop goes on only when it has returned: so the
	// results of a round arrive in the order the model made the calls,
	// each after it is stored and before the next call that runs alone is
	// started. It is where a feature does what must follow a stored
	// result, such as applying a pop it recorded and telling the person.
	// The session's file is written, so it may read the transcript and
	// append entries of its own to it. The loop still holds its log while
	// it runs: see Stored for what may be called from here. It must
	// return promptly.
	AfterStored func(ctx context.Context, e agent.Entry, written []transcript.Entry)

	// Slid, when not nil, is told what a slide took out of the model's
	// view (transcript.Slide), after the slide is recorded: the reads to
	// forget, the tokens that left. The loop holds its log while it runs:
	// see Stored for what may be called from here.
	Slid func(ctx context.Context, cuts []transcript.Cut)

	// StopGrace, when more than the loop's own ten seconds, is how long a
	// stopped turn waits for a call of one of the feature's tools to come
	// back. A tool that must finish its bookkeeping even when the turn is
	// stopped returns a result when it has, and that result is stored with
	// everything in it as long as it arrives within this time; past it the
	// loop goes on without the call and stores that it did not stop. The
	// session waits the longest time any of its features asks for.
	StopGrace time.Duration

	// RepeatKey, when not nil, may give the text by which one call and its
	// result are compared when the loop looks for a model repeating
	// itself (agent.RepeatPolicy.Key): for a tool whose every answer ends
	// in a duration or a counter, the answer with its numbers blanked. ok
	// false leaves the call to the next feature and then to the loop.
	RepeatKey func(call core.ToolCall, res core.ToolResult) (key string, ok bool)
	// RepeatNudge, when not nil, may return its own words for a repetition
	// (agent.RepeatPolicy.Nudge), already marked; "" leaves the words to
	// the next feature and then to the loop.
	RepeatNudge func(rep agent.Repetition) string

	// Protected lists more paths whose change always asks, whatever rule
	// or mode would let it run: the feature's own data. Each covers itself
	// and everything under it.
	Protected []string

	// Hooks, when not nil, may change the options the hook runner is made
	// with, before it is made: the path hooks are given as
	// GNOSH_MEM_STATUS, the guards of the feature's tools. It must leave
	// Fires alone.
	Hooks func(o *hooks.Options)

	// Slash are slash commands the feature adds: /mem, /goal. A name gnosh
	// itself has, or that an earlier feature added, is refused.
	Slash []SlashCommand

	// RunPrompt, when not nil, is asked for the first prompt of a
	// non-interactive run that was given none, on its command line or on
	// standard input. The features are asked in order, and the first that
	// reports true supplies it; the run then answers that prompt as it
	// would one that was typed. It is how a flag a feature claims can be a
	// run's whole command line: gnosh run --goal "the tests pass". With no
	// feature supplying one, a run without a prompt is refused as before.
	RunPrompt func(ctx context.Context) (agent.Prompt, bool)

	// ModeChanged, when not nil, is told the permission mode that has just
	// come into force, each time it changes while the session runs. It is
	// not called for the mode the session starts in: Session.Mode says
	// that.
	ModeChanged func(mode string)

	// Ready, when not nil, is called once the session's agent exists and
	// before its first prompt is sent. From here on the feature may use
	// the agent's side requests (Side), notes (Tell), notices (Notice),
	// questions (Ask) and its other methods.
	Ready func(a *agent.Agent)

	// RunEnd, when not nil, is called at the end of a non-interactive run,
	// after its last turn and before the result is printed, and may raise
	// the run's exit code: the largest code any feature returns wins over
	// the run's own when it is larger. A goal that is still unmet is the
	// case it is for. Lines it has for the person go through
	// Session.Notice. It is called after every feature's TurnEnded has
	// been told of that last turn.
	RunEnd func(ctx context.Context, end RunEnd) (exit int)

	// Close, when not nil, is called when the session ends, after the
	// person's SessionEnd hooks and after the shell manager is closed, and
	// before the session's file is flushed and released. Features are
	// closed in reverse order.
	Close func(ctx context.Context)
}

// RunEnd is what a feature is told at the end of a non-interactive run.
type RunEnd struct {
	// Result is how the run's last turn ended.
	Result agent.TurnResult
	// Exit is the exit code the run would end with.
	Exit int
}

// SlashCommand is a command a person types into a session, or gives as
// the prompt of a run: a line that begins with a slash and the command's
// name.
type SlashCommand struct {
	// Name is the command's name without its slash, and Aliases its other
	// spellings.
	Name    string
	Aliases []string
	// Args says what may follow the name, for help: "[instructions]".
	Args string
	// Summary says in a line what the command does, for /help.
	Summary string
	// InRun says the command may be the prompt of a non-interactive run.
	// What it says is then the run's result.
	InRun bool
	// Sensitive lists the first words of the command's arguments after
	// which what is typed is a text the person wants kept nowhere: for a
	// command whose "redact <text>" takes a secret out of everything, it
	// holds "redact". A front end shows such a line and remembers it only
	// as far as that word, and gnosh's log never holds a command's
	// arguments at all. An entry of "" means everything after the name.
	Sensitive []string
	// Run does it. An error is shown to the person as the command's
	// failure, in its own words.
	Run func(ctx context.Context, c *SlashContext) error
}

// SlashContext is what a slash command is given.
type SlashContext struct {
	// Args is what was typed after the command's name, without the blanks
	// around it.
	Args string
	// Session is the session's facts, and Agent its agent.
	Session *Session
	Agent   *agent.Agent
	// Say prints a line for the person, as fmt.Sprintf would format it. In
	// a run, what a command says is the run's result.
	Say func(format string, args ...any)
}

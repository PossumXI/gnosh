package cli

// Env is the little that reading a command line needs to know about the
// process it was typed into. The caller fills it in; Parse reads nothing by
// itself.
type Env struct {
	// StdinIsTerminal says that standard input is a terminal: a person can
	// type, and nothing is piped in.
	StdinIsTerminal bool
	// StdoutIsTerminal says that standard output is a terminal. When it is
	// not, `gnosh` with no command answers once and exits, as -p would,
	// because there is no screen to draw a session on.
	StdoutIsTerminal bool
}

// Command names what a command line asks for.
type Command string

// The commands. A session is CmdInteractive or CmdPrint; the others are the
// subcommands, named by the words that select them.
const (
	// CmdInteractive opens a session a person types into: `gnosh`.
	CmdInteractive Command = "interactive"
	// CmdPrint answers one prompt and exits: `gnosh -p`, `gnosh run`, or
	// `gnosh` with its output going somewhere other than a terminal.
	CmdPrint Command = "print"
	// CmdHelp prints help: -h, --help, or `gnosh help`. Invocation.HelpFor
	// says for which command.
	CmdHelp Command = "help"
	// CmdVersion prints which build this is: --version, -v outside print
	// mode, or `gnosh version`.
	CmdVersion Command = "version"

	// CmdDoctor checks that a turn could run here.
	CmdDoctor Command = "doctor"
	// CmdTrust trusts a folder's settings, lists the trusted folders
	// (Invocation.List) or forgets one (Invocation.Forget).
	CmdTrust Command = "trust"
	// CmdModels lists the models each provider offers.
	CmdModels Command = "models"
	// CmdSessionList and the five after it are `gnosh session ...`.
	CmdSessionList   Command = "session list"
	CmdSessionShow   Command = "session show"
	CmdSessionLast   Command = "session last"
	CmdSessionDelete Command = "session delete"
	CmdSessionRename Command = "session rename"
	CmdSessionWeb    Command = "session web"
	// CmdMem reads the project's mem; Invocation.Args holds the words after
	// `mem`, for mem itself to read.
	CmdMem Command = "mem"
	// CmdPromptShow and the four after it are `gnosh prompt ...`.
	CmdPromptShow    Command = "prompt show"
	CmdPromptPath    Command = "prompt path"
	CmdPromptExport  Command = "prompt export"
	CmdPromptTools   Command = "prompt tools"
	CmdPromptContext Command = "prompt context"
	// CmdLogs prints the project's log.
	CmdLogs Command = "logs"
	// CmdDirs says which config and data folders are in use.
	CmdDirs Command = "dirs"
	// CmdSSH runs ssh; Invocation.Args holds everything after `ssh`,
	// untouched.
	CmdSSH Command = "ssh"
	// CmdProjects lists the project folders gnosh has been used in.
	CmdProjects Command = "projects"
	// CmdStats prints usage totals.
	CmdStats Command = "stats"
	// CmdMCPAdd and the four after it are `gnosh mcp ...`.
	CmdMCPAdd     Command = "mcp add"
	CmdMCPAddJSON Command = "mcp add-json"
	CmdMCPGet     Command = "mcp get"
	CmdMCPList    Command = "mcp list"
	CmdMCPRemove  Command = "mcp remove"
)

// Conversation says which conversation a session command works in.
type Conversation string

// The ways to choose a conversation.
const (
	// ConversationNew starts a new one. It is what a session gets when no
	// flag says otherwise.
	ConversationNew Conversation = "new"
	// ConversationContinue goes on with the most recent one of the project:
	// -c, -C, --continue.
	ConversationContinue Conversation = "continue"
	// ConversationResume goes on with the one -r names (Invocation.Resume),
	// or lets the person pick when it names none.
	ConversationResume Conversation = "resume"
	// ConversationSession goes on with the one whose ID -s gives
	// (Invocation.Session).
	ConversationSession Conversation = "session"
)

// The values of --permission-mode, in Claude Code's spelling.
const (
	ModeDefault           = "default"
	ModePlan              = "plan"
	ModeAcceptEdits       = "acceptEdits"
	ModeDontAsk           = "dontAsk"
	ModeBypassPermissions = "bypassPermissions"
)

// The values of --output-format and --input-format.
const (
	FormatText       = "text"
	FormatJSON       = "json"
	FormatStreamJSON = "stream-json"
)

// Invocation is what a command line asks gnosh to do, as plain data. Parse
// fills it in and acts on none of it: no file is read, no directory checked,
// no tool name looked up. A field the command line did not set holds its
// zero value, unless its comment names a default.
type Invocation struct {
	// Command is what to do.
	Command Command

	// HelpFor, with CmdHelp, is the command whose help is wanted, as the
	// words that lead to it ("session", "list"). Empty means gnosh itself.
	HelpFor []string

	// Args is a subcommand's own arguments, in order and exactly as typed:
	// the folder of `trust`, the ID of `session show` and `session delete`,
	// the ID and then the title of `session rename`, the name of `prompt
	// show` and `prompt export`, the words after `mem` and after `ssh`, and
	// for `mcp` the server's name followed by its command or URL (or its
	// JSON).
	Args []string

	// Prompt is the prompt given on the command line: every argument that is
	// not a flag or a flag's value, joined by single spaces. Its text is not
	// changed in any other way.
	Prompt string
	// ReadStdin says standard input is not a terminal, so there may be piped
	// text to read. In print mode that text goes before Prompt, with a blank
	// line between; in an interactive session it is the first prompt.
	ReadStdin bool
	// PrintImplied says print mode was chosen because standard output is not
	// a terminal, not because -p or `run` was typed.
	PrintImplied bool

	// Conversation says which conversation to work in. Session commands
	// always set it; ConversationNew is the default.
	Conversation Conversation
	// Resume is what followed -r: a session ID, a session's name, words to
	// search the session list for, or the path of a transcript file. Empty
	// with ConversationResume means show the list to pick from.
	Resume string
	// ResumeIsFile says Resume is an absolute path ending in .jsonl, which
	// names a transcript file and not a session.
	ResumeIsFile bool
	// Session is the session ID that followed -s.
	Session string
	// SessionID is the ID the new conversation is to have (--session-id). It
	// is a UUID; Parse has checked its form.
	SessionID string
	// ForkSession says to go on in a copy of the conversation and leave the
	// original as it was.
	ForkSession bool
	// Name is the name to give the session (-n).
	Name string
	// NoSessionPersistence says not to save the session at all.
	NoSessionPersistence bool

	// Model is the model for this run, as typed: `model` or `provider/model`.
	Model string
	// SmallModel is the model for a print run's short side jobs.
	SmallModel string
	// Effort is the thinking level for the run (--reasoning-effort or
	// --effort), as typed: the levels depend on the model.
	Effort string

	// PermissionMode is the permission mode to start in, one of the Mode
	// constants; "" means the command line did not say. -y, --yolo and
	// --dangerously-skip-permissions set ModeBypassPermissions, and `manual`
	// is recorded as ModeDefault.
	PermissionMode string
	// AllowSkipPermissions says the person may switch to skipping
	// permissions later in the session (--allow-dangerously-skip-permissions).
	AllowSkipPermissions bool
	// AllowedTools holds the allow rules, one per item, as typed.
	AllowedTools []string
	// DisallowedTools holds the deny rules, one per item, as typed.
	DisallowedTools []string
	// Tools holds the names given to --tools, as typed (`default` included).
	// ToolsGiven tells `--tools ""`, which gives the model no tools, from no
	// --tools at all.
	Tools      []string
	ToolsGiven bool
	// AddDirs holds the extra folders the agent may read, as typed.
	AddDirs []string
	// TrustFolder says to use this folder's own settings for this one run.
	TrustFolder bool
	// PermissionPrompts is who answers permission questions in a print run:
	// "host" or "none"; "" means the command line did not say.
	PermissionPrompts string

	// SystemPrompt is text to use in place of gnosh's own instructions, and
	// SystemPromptFile the path of a file holding such text. Neither file is
	// read here. When both are given the file's text comes first.
	SystemPrompt     string
	SystemPromptFile string
	// AppendSystemPrompt is text to add to the end of the instructions, and
	// AppendSystemPromptFile the path of a file holding such text. When both
	// are given the file's text comes first.
	AppendSystemPrompt     string
	AppendSystemPromptFile string

	// Cwd is the folder to work in (--cwd).
	Cwd string
	// DataDir is the folder to keep sessions and data in (-D).
	DataDir string
	// Settings is what followed --settings: a file's path, or the settings
	// themselves as JSON. SettingsInline says it is JSON (it begins with a
	// brace).
	Settings       string
	SettingsInline bool
	// SettingSources lists which settings files to read: any of "user",
	// "project", "local". Nil means the command line did not say.
	SettingSources []string
	// Bare asks for the least gnosh can start with.
	Bare bool
	// SafeMode asks for a start with the user's customizations off.
	SafeMode bool
	// DisableSlashCommands turns skills and slash commands off.
	DisableSlashCommands bool
	// MCPConfig holds what was given to --mcp-config: each item a file's
	// path or JSON written out.
	MCPConfig []string
	// StrictMCPConfig says to use only the servers MCPConfig names.
	StrictMCPConfig bool
	// Agents is the JSON given to --agents (or, in print mode, the path of a
	// file holding it).
	Agents string
	// Debug asks for more in the log. --debug-file turns it on too.
	Debug bool
	// DebugFilter is the filter attached to --debug with an equals sign.
	DebugFilter string
	// DebugFile is the file to write the debug log to.
	DebugFile string
	// Verbose asks for each turn in full; in print mode, for the log and one
	// line per tool call on stderr.
	Verbose bool

	// OutputFormat is what a print run prints: one of the Format constants.
	// In print mode it is FormatText unless the command line says otherwise;
	// outside print mode it is "".
	OutputFormat string
	// InputFormat is what a print run reads from standard input: FormatText
	// or FormatStreamJSON. It defaults as OutputFormat does.
	InputFormat string
	// IncludePartialMessages asks for the answer piece by piece in a
	// stream-json run.
	IncludePartialMessages bool
	// ReplayUserMessages asks for each user message read from standard input
	// to be printed back.
	ReplayUserMessages bool
	// Quiet asks for no progress indicator on stderr.
	Quiet bool
	// Goal is the condition a print run works towards (--goal).
	Goal string
	// MaxTurns is the most turns the model may take in a print run. Zero
	// means no limit.
	MaxTurns int

	// JSON asks a subcommand to print JSON (`session ...`, `projects`,
	// `prompt tools`).
	JSON bool
	// List and Forget belong to `trust`: list the trusted folders, or forget
	// one instead of trusting it.
	List   bool
	Forget bool
	// Force belongs to `prompt export`: write the file even if one is there.
	Force bool
	// Export belongs to `prompt tools`: write each tool's description to a
	// file to edit.
	Export bool
	// Follow and Tail belong to `logs`: keep printing as the log grows, and
	// how many lines to print (zero means the command line did not say).
	Follow bool
	Tail   int
	// Port belongs to `session web`: the port to serve the page on (zero
	// means the command line did not say).
	Port int
	// All and CrawlDir belong to `stats`.
	All      bool
	CrawlDir string
	// MCPScope, MCPTransport, MCPEnv and MCPHeaders belong to `mcp`: where
	// the server is recorded ("local", "user" or "project"), how it is
	// reached ("stdio", "sse" or "http"), the KEY=value pairs for its
	// command, and the headers for a remote one. An empty scope or transport
	// means the command line did not say.
	MCPScope     string
	MCPTransport string
	MCPEnv       []string
	MCPHeaders   []string
}

// Exit codes, for the code that runs what Parse describes.
const (
	// ExitOK: the turn finished, or the command did what it was asked.
	ExitOK = 0
	// ExitFailure: the turn failed, a hook blocked the prompt, --goal is
	// still unmet, `gnosh doctor` found that a turn cannot run, or gnosh
	// refused to start (a command line it could not act on, flags that
	// cannot be combined, a session that cannot be resumed). Every error
	// Parse returns exits with this.
	ExitFailure = 1
	// ExitInterrupted: stopped by Ctrl-C (SIGINT). Nothing is printed.
	ExitInterrupted = 130
	// ExitTerminated: stopped by SIGTERM.
	ExitTerminated = 143
)

// Parse reads a command line (the arguments after the program's name) with
// gnosh's built-in table of commands and flags, and returns what it asks
// for. It reads no file, no environment variable and no input: env carries
// the two facts it needs. An error is a *Error whose text is ready to show.
//
// A flag whose feature is not built yet is an error, not a silent no-op.
// The code that wires features in makes its own table with Default and marks
// what it has built; Parse uses the table as it ships.
func Parse(args []string, env Env) (Invocation, error) {
	return Default().Parse(args, env)
}

// Help returns the help text for gnosh, or for the command the words name
// ("session", "list"), from the built-in table. Every line fits 80 columns.
func Help(command ...string) (string, error) {
	return Default().Help(command...)
}

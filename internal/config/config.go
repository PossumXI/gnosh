package config

import (
	"encoding/json"
	"time"
)

// Config is everything gnosh's configuration says, after every file, the
// state file, the environment and the command line have been read, merged
// and checked. It is plain data: other packages are handed the parts they
// need. Printing a Config, or any part of one, never shows a secret.
type Config struct {
	// Paths are the directories and files in use.
	Paths Paths
	// Files are the config files that were found, in the order they were
	// read, each with whether it is in force or held.
	Files []File

	// Trusted reports whether the project's own settings are in force.
	Trusted bool
	// Held is what the project's own files would do if the folder were
	// trusted, and do not do because it is not. It is empty in a trusted
	// folder, and in one that has no settings waiting for trust. The trust
	// question is asked only when Held is not empty, and lists it.
	Held []Held

	// Providers are the model providers, in the order they were first
	// added. Disabled ones and ones that cannot be used are included, each
	// saying why; Usable picks the rest.
	Providers []Provider
	// Models are the models registered by hand with model add.
	Models []Model
	// Large is the model that does the work, and Small the one that does
	// the short side jobs. A slot whose Model is not set has no choice
	// made for it: Small then follows Large, and Large is asked for.
	Large, Small Slot

	// Options are the option directive's settings.
	Options Options
	// Limits are the tool and request limits that Claude Code's
	// environment variables set.
	Limits Limits
	// Switches are the on/off settings of gnosh's own variables and flags.
	Switches Switches

	// Hosts are the other machines the model may run commands on.
	Hosts []Host
	// Hooks are the hooks in force, in the order they were configured.
	// The list is empty in safe mode and when disableAllHooks is set.
	Hooks []Hook
	// AllHooks are the hooks the configuration holds, in force or not.
	AllHooks []Hook
	// HooksOff says why Hooks is empty although AllHooks is not: "safe
	// mode", or the setting that switched them off. Empty otherwise.
	HooksOff string
	// Permissions are the permission rules, the starting mode and the
	// extra directories.
	Permissions Permissions
	// Env are the variables the env block sets for the commands and hooks
	// gnosh starts, sorted by name.
	Env []EnvVar
	// Context says which instruction files gnosh reads, and in what order.
	Context ContextFiles
	// MCP are the MCP servers.
	MCP []MCPServer
	// LSP are the language servers.
	LSP []LSPServer

	// BadEnv are the environment variables whose values could not be
	// used, for doctor.
	BadEnv []Problem

	secrets *secrets
}

// File is a config file Load found.
type File struct {
	// Path is where the file is. "--settings" stands for settings given on
	// the command line as JSON.
	Path string
	// Role is "gnoshrc", "settings" or "state".
	Role string
	// Scope is "user", "project", "local" or "command line".
	Scope string
	// Held reports that the file is the project's and the folder is not
	// trusted, so only what restricts the agent was taken from it.
	Held bool
	// OldName reports that the file was found under a name from before the
	// rewrite (see CFG-5).
	OldName bool
}

// Held is one thing a project's file says that waits for the folder to be
// trusted.
type Held struct {
	// Kind sorts what it would do; the Kind constants list the values.
	Kind string
	// Text is the setting in a few words taken from the file. It comes
	// from a stranger's file: show it escaped.
	Text string
	// Origin is the line that says it.
	Origin Origin
}

// The kinds of thing a project's settings can hold. The trust package has
// the same words: its Restricts says which kinds still apply in a folder
// that is not trusted, and its question says what each could do.
const (
	KindProvider      = "provider"
	KindModel         = "model"
	KindHost          = "host"
	KindHook          = "hook"
	KindHookOff       = "hook-off" // removing a hook, or switching all hooks off
	KindMCPLocal      = "mcp-local"
	KindMCPOnline     = "mcp-online"
	KindLSP           = "lsp"
	KindAllowRule     = "allow-rule"
	KindAskRule       = "ask-rule"
	KindDenyRule      = "deny-rule"
	KindMode          = "mode"
	KindAddDir        = "add-dir"
	KindEnv           = "env"
	KindOption        = "option"
	KindExtraPath     = "extra-path"
	KindAddress       = "address"
	KindDisabledSkill = "disabled-skill"
	KindRemoval       = "removal" // removing a provider, model, host or server
)

// Unexported spellings, so the tables read evenly.
const (
	kindOption        = KindOption
	kindExtraPath     = KindExtraPath
	kindAddress       = KindAddress
	kindDisabledSkill = KindDisabledSkill
)

// Provider is one model provider.
type Provider struct {
	// ID is the name it was added under.
	ID string
	// Name is the name shown to the user; ID when none was given.
	Name string
	// Type is the type as configured: llamacpp, openai-compat, and so on.
	Type string
	// Wire is the request format the type speaks: "llamacpp", "openai" or
	// "anthropic". It is empty for a type that is not built yet.
	Wire string
	// Endpoints are the addresses, in the order given. The first that
	// accepts a connection is the one to use (see SelectEndpoint).
	Endpoints []Endpoint
	// APIKey is the key. It may be empty for a local server.
	APIKey Secret
	// KeyFrom says where the key came from when it was not written out:
	// the variable's name with a dollar sign, such as "$OPENAI_API_KEY".
	KeyFrom string
	// Headers are the extra request headers, in the order given. One whose
	// value is empty is not sent.
	Headers []Header
	// ExtraBody is merged into every request body. Nil when none was given.
	ExtraBody map[string]any
	// DiscoverModels says whether to ask the server which models it has.
	// When nothing set it, it is true exactly when no model is registered
	// for the provider.
	DiscoverModels Value[bool]
	// Disabled keeps the entry without using it.
	Disabled bool
	// LeftOut says why the provider cannot be used, in a plain sentence.
	// Empty means nothing in the configuration stands in its way. A
	// provider that is left out stops nothing: gnosh starts, and every
	// place that lists providers shows the reason.
	LeftOut string
	// SystemPromptPrefix, ProviderOptions and FlatRate are read and not
	// used yet.
	SystemPromptPrefix string
	ProviderOptions    json.RawMessage
	FlatRate           bool
	// Origin is the last line that set the provider.
	Origin Origin
}

// Usable reports whether the provider can be used as far as the
// configuration can tell.
func (p Provider) Usable() bool { return !p.Disabled && p.LeftOut == "" }

// Endpoint is one address of a provider.
type Endpoint struct {
	// Given is the address as it was configured.
	Given string
	// URL is the root of the API: what "/chat/completions", "/messages" or
	// "/models" is added to. For a server on the user's own machine it
	// ends in /v1 whether or not the configured address did.
	//
	// Given and URL are for the client that connects. An address can
	// carry a password or a token in its query string, so neither is for
	// showing or logging: Display is.
	URL string
	// Display is URL without any user name, password or query string.
	Display string
	// Local reports that the address is on the user's own machine or
	// network: localhost, a loopback, private or link-local address, the
	// 100.64.0.0/10 range overlay networks use, a host name of one label,
	// or a name under .localhost, .local, .lan, .internal or .home.arpa.
	Local bool
	// Timeouts are the time limits of a request to this address.
	Timeouts Timeouts
}

// Timeouts are the time limits of one model request. A zero limit means
// there is none.
type Timeouts struct {
	// Connect is how long reaching the server may take.
	Connect time.Duration
	// FirstToken is how long the server may take, from the moment the
	// request is sent, to send the first piece of its answer.
	FirstToken time.Duration
	// Idle is how long the answer may go quiet between pieces.
	Idle time.Duration
	// Total is how long the whole request may take. gnosh sets none.
	Total time.Duration
	// FirstTokenFrom and IdleFrom say what set those two limits.
	FirstTokenFrom, IdleFrom Origin
}

// Header is one extra request header.
type Header struct {
	Name  string
	Value Secret
}

// Model is a model registered by hand.
type Model struct {
	Provider string
	ID       string
	// Name is the name shown to the user; ID when none was given.
	Name string
	// ContextWindow is the model's window in tokens. Not set means: ask
	// the server, and take 32,768 if it cannot say.
	ContextWindow Value[int]
	// DefaultMaxTokens is the reply limit sent with a request. Not set
	// means none is sent to a local server.
	DefaultMaxTokens Value[int]
	// ReasoningLevels are the model's thinking levels, weakest first. Nil
	// means the model's ID decides.
	ReasoningLevels []string
	// CanReason and SupportsImages say what the model can do.
	CanReason      Value[bool]
	SupportsImages Value[bool]
	// ReasoningEffort and the prices are read and not used yet.
	ReasoningEffort  Value[string]
	PriceInput       Value[float64]
	PriceOutput      Value[float64]
	PriceCacheCreate Value[float64]
	PriceCacheHit    Value[float64]
	// Origin is the last line that set the model.
	Origin Origin
}

// Ref is the model's name in the form the user types: provider/id.
func (m Model) Ref() string { return m.Provider + "/" + m.ID }

// ModelRef names a model: a provider's ID and the provider's name for the
// model.
type ModelRef struct {
	// Provider is the provider's ID. It is empty when the command line
	// named a model without a provider and more than one provider could
	// have it: whoever lists the providers' models settles it.
	Provider string
	ID       string
}

// String is "provider/id", or the bare ID when no provider is known.
func (r ModelRef) String() string {
	if r.Provider == "" {
		return r.ID
	}
	return r.Provider + "/" + r.ID
}

// Slot is one of the two model choices, large or small, with the settings
// that go with it.
type Slot struct {
	// Model is the model chosen. Not set means no choice has been made.
	Model Value[ModelRef]
	// Thinking is the thinking level: "on", "off", or one of the model's
	// own levels such as "low". Not set means on, for a model that can
	// think.
	Thinking Value[string]
	// Temperature is left to the provider when not set.
	Temperature Value[float64]
	// The rest are read and not used yet.
	MaxTokens        Value[int]
	TopP             Value[float64]
	TopK             Value[int]
	FrequencyPenalty Value[float64]
	PresencePenalty  Value[float64]
	ProviderOptions  json.RawMessage
}

// Host is another machine the model may run commands on.
type Host struct {
	Name string
	// SSH is the destination handed to ssh: user@address, or a Host alias
	// from the user's ssh configuration.
	SSH         string
	Description string
	Origin      Origin
}

// Hook is one hook: a command gnosh runs at a named moment.
type Hook struct {
	// Event is the moment, one of HookEventNames.
	Event string
	// Name is the name given with --name in gnoshrc, or "".
	Name string
	// Matcher says which tools or sources the hook runs for. Empty means
	// all of them.
	Matcher string
	// Command is the command line, handed to the shell.
	Command string
	// Timeout is how long the hook may run. Zero means the event's default.
	Timeout time.Duration
	// Scope is "user", "project", "local" or "command line".
	Scope string
	// BaseDir is the directory of the file that configured the hook.
	BaseDir string
	Origin  Origin
}

// Permissions are the permission settings.
type Permissions struct {
	// Mode is the mode the session starts in: default, plan, acceptEdits,
	// dontAsk or bypassPermissions.
	Mode Value[string]
	// BypassAvailable reports that the session may switch to
	// bypassPermissions later: it was started with -y, in that mode, or
	// with that mode as the user's own default.
	BypassAvailable bool
	// BypassDisabled reports that a settings file forbids
	// bypassPermissions.
	BypassDisabled Value[bool]
	// DenyInsteadOfPrompting refuses what would ask, and goes on.
	DenyInsteadOfPrompting Value[bool]
	// Layers are the rules, one layer per file or flag that gave any,
	// weakest first. Deny rules of every layer count, then ask rules, then
	// allow rules, so the order matters only for display.
	Layers []RuleLayer
	// Dirs are the extra directories the file tools may read.
	Dirs []ListItem
	// Tools is the --tools flag: the only built-in tools the model is
	// given. Not set means all of them; set and empty means none.
	Tools Value[[]string]
}

// RuleLayer is the permission rules of one source.
type RuleLayer struct {
	// Scope is "user", "project", "local" or "command line".
	Scope string
	// File is the file the rules are in, or "" for a flag.
	File string
	// BaseDir is what a rule path that starts with a single slash is
	// under: the config directory for the user's files, the working
	// directory for the project's files and for flags, and the file's own
	// directory for a file given with --settings.
	BaseDir          string
	Allow, Ask, Deny []Rule
}

// Rule is one permission rule, as written: a tool name, or a tool name
// with a pattern in parentheses. Load checks that the tool is one gnosh
// has. What the pattern means is the permission package's to say.
type Rule struct {
	Text   string
	Origin Origin
}

// EnvVar is one variable of the env block.
type EnvVar struct {
	Name string
	// Value is the value. An empty one takes the variable away from the
	// commands gnosh starts.
	Value  Secret
	Origin Origin
}

// MCPServer is one MCP server.
type MCPServer struct {
	Name string
	// Type is stdio, sse or http.
	Type    string
	Command string
	Args    []string
	Env     []EnvVar
	URL     string
	Headers []Header
	// Timeout is how long a tool call may take. Zero means the default.
	Timeout       time.Duration
	Disabled      bool
	EnabledTools  []string
	DisabledTools []string
	Origin        Origin
}

// LSPServer is one language server.
type LSPServer struct {
	Name        string
	Command     string
	Args        []string
	Env         []EnvVar
	FileTypes   []string
	RootMarkers []string
	// Timeout is how long a request may take. Zero means the default.
	Timeout     time.Duration
	Disabled    bool
	InitOptions json.RawMessage
	Options     json.RawMessage
	Origin      Origin
}

// Switches are the on/off settings that gnosh's own environment variables
// and flags control.
type Switches struct {
	// SafeMode starts gnosh with the config's customizations off: hooks,
	// MCP servers not given on the command line, context files, skills
	// found on disk and custom commands.
	SafeMode Value[bool]
	// Profile serves a debugging page on loopback.
	Profile Value[bool]
	// Metrics says whether usage events may be sent, after the option, the
	// variables and DO_NOT_TRACK have been weighed. Not set means nothing
	// decided it, which is off.
	Metrics Value[bool]
	// UpdateCheck says whether the update check runs. It is on only when a
	// repository is named and the check is not switched off.
	UpdateCheck Value[bool]
}

// ContextFiles says which instruction files the model is given, as data
// for the package that reads them. Load reads none of them.
type ContextFiles struct {
	// Off reports that no context file is read at all (safe mode).
	Off bool
	// ProjectNames are the file names looked for in the working directory
	// and in every directory above it, in the order they are read inside
	// one directory. Directories are read outermost first, the closest
	// last.
	ProjectNames []string
	// LocalNames are the names of the person's private file about a
	// project, read after ProjectNames in each directory.
	LocalNames []string
	// Cwd and ProjectRoot say where the walk ends and where the project
	// begins. A directory above ProjectRoot gives only files owned by the
	// user or by root.
	Cwd, ProjectRoot string
	// Global are the user's global files and folders, in the order they
	// are read. A folder gives every Markdown file in it. A file that does
	// not exist is passed over.
	Global []ListItem
	// Extra are more project context files, from option context-path.
	Extra []ListItem
	// ImportDepth is how many levels deep an @path line is followed.
	ImportDepth int
	// MaxFileBytes is the size past which a file is passed over, with a
	// word to the user.
	MaxFileBytes int64
	// NestedMaxChars is how much of a subdirectory's file, given when the
	// agent first touches that subdirectory, is shown before it is cut.
	NestedMaxChars int
}

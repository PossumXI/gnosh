package config

import (
	"maps"
	"slices"
	"strings"
)

// This file is the one table of what the configuration can say: every
// directive and flag of gnoshrc, every key of gnosh.json, every hook event,
// provider type and environment variable, each with what gnosh does about
// it today. The parsers read these tables and nothing else, so a word that
// is not here is a startup error, and a word that is here has exactly the
// status written beside it. (The option keys are the other half of the
// table, in options.go, beside the fields they set.)

// Feature is a part of gnosh that some settings are for. The program that
// calls Load says in Inputs.Built which parts it has. A setting for a part
// it does not have would look configured and do nothing, so it stops
// startup instead.
type Feature string

const (
	// FeatureHooks is the hook runner.
	FeatureHooks Feature = "hooks"
	// FeaturePermissions is permission rules and modes.
	FeaturePermissions Feature = "permissions"
	// FeatureHosts is running commands on declared machines.
	FeatureHosts Feature = "hosts"
	// FeatureMCP is MCP servers.
	FeatureMCP Feature = "mcp"
	// FeatureLSP is language servers.
	FeatureLSP Feature = "lsp"
	// FeatureEnv is the env block of gnosh.json reaching the commands and
	// hooks gnosh starts.
	FeatureEnv Feature = "env"
)

// featureWords says what each Feature is in a message.
var featureWords = map[Feature]string{
	FeatureHooks:       "hooks",
	FeaturePermissions: "permission rules",
	FeatureHosts:       "commands on other machines (hosts)",
	FeatureMCP:         "MCP servers",
	FeatureLSP:         "language servers",
	FeatureEnv:         "support for the env block",
}

// Status says what gnosh does today with something the configuration can
// say.
type Status int

const (
	// Works: the setting is read, checked and handed on in the Config, and
	// is in force as soon as the part of gnosh it is for takes it.
	Works Status = iota
	// Parsed: the setting is read and checked so that a file holding it
	// loads, and it is in the Config, but nothing uses it yet. Leaving it
	// unused does no harm.
	Parsed
	// Gated: as Works when the calling program has the Feature the setting
	// is for (Inputs.Built), and a startup error that says the part is not
	// built when it has not.
	Gated
	// NotBuilt: a startup error that says the setting is planned and not
	// built yet.
	NotBuilt
	// Gone: a setting of the earlier gnosh that is not carried over. A
	// startup error that says so and why.
	Gone
)

// String is the status in the words the report uses.
func (s Status) String() string {
	switch s {
	case Parsed:
		return "parsed for later"
	case Gated:
		return "works when built in"
	case NotBuilt:
		return "refused as not built"
	case Gone:
		return "refused as gone"
	}
	return "works now"
}

// flagKind is the shape of a flag's value.
type flagKind int

const (
	kText   flagKind = iota // one word
	kTruth                  // true or false; the value may be left out and means true
	kInt                    // a whole number
	kNumber                 // a number, possibly with a decimal point
	kJSON                   // one word holding a JSON object
	kPair                   // two words: a key and a value
	kWords                  // one word holding a list, split at commas and spaces
)

// flagSpec describes one flag of a directive.
type flagSpec struct {
	name   string
	kind   flagKind
	many   bool   // may be given more than once
	secret bool   // the value is never shown
	pri    string // the spec's priority
	status Status
	what   string // what it means, in a few words
}

// subSpec describes one form of a directive, such as "provider add".
type subSpec struct {
	name    string
	alias   string // another spelling of name ("rm" for "remove")
	arg     string // what follows the name: "<id>", "<rule>...", or ""
	flags   []flagSpec
	pri     string
	status  Status
	feature Feature // for Gated
}

// directiveSpec describes one directive: the first word of a line.
type directiveSpec struct {
	name string
	subs []subSpec
}

// The flags of each directive, with the priority the spec gives each.
var (
	providerFlags = []flagSpec{
		{"--type", kText, false, false, "P0", Works, "the wire format: " + list(providerTypeNames())},
		{"--base-url", kText, true, false, "P0", Works, "where the API is; given more than once, the first that answers is used"},
		{"--name", kText, false, false, "P0", Works, "the name shown to the user"},
		{"--api-key", kText, false, true, "P0", Works, "the key; empty for a local server"},
		{"--discover-models", kTruth, false, false, "P0", Works, "ask the server which models it has"},
		{"--extra-body", kJSON, false, false, "P1", Works, "a JSON object merged into every request body"},
		{"--extra-header", kPair, true, true, "P1", Works, "an HTTP header; an empty value is not sent"},
		{"--disable", kTruth, false, false, "P1", Works, "keep the entry and do not use it"},
		{"--system-prompt-prefix", kText, false, false, "P2", Parsed, "text put before the system prompt"},
		{"--provider-options", kJSON, false, false, "P2", Parsed, "provider-specific options"},
		{"--flat-rate", kTruth, false, false, "P2", Parsed, "do not count cost"},
		{"--api-key-command", kText, false, false, "P2", NotBuilt, "a command that prints the key"},
	}
	modelAddFlags = []flagSpec{
		{"--context-window", kInt, false, false, "P0", Works, "the model's window, in tokens"},
		{"--name", kText, false, false, "P0", Works, "the name shown to the user"},
		{"--default-max-tokens", kInt, false, false, "P1", Works, "the reply limit sent with a request"},
		{"--reasoning-levels", kWords, true, false, "P1", Works, "the model's thinking levels, weakest first"},
		{"--can-reason", kTruth, false, false, "P1", Works, "the model can think"},
		{"--supports-images", kTruth, false, false, "P1", Works, "the model can see images"},
		{"--reasoning-effort", kText, false, false, "P2", Parsed, "the level to start at"},
		{"--price-input", kNumber, false, false, "P2", Parsed, "price per million tokens read"},
		{"--price-output", kNumber, false, false, "P2", Parsed, "price per million tokens written"},
		{"--price-cache-create", kNumber, false, false, "P2", Parsed, "price per million tokens cached"},
		{"--price-cache-hit", kNumber, false, false, "P2", Parsed, "price per million tokens read from cache"},
	}
	modelSlotFlags = []flagSpec{
		{"--think", kTruth, false, false, "P1", Works, "thinking on or off"},
		{"--reasoning-effort", kText, false, false, "P1", Works, "the thinking level"},
		{"--temperature", kNumber, false, false, "P1", Works, "the sampling temperature"},
		{"--max-tokens", kInt, false, false, "P2", Parsed, "the reply limit"},
		{"--top-p", kNumber, false, false, "P2", Parsed, "nucleus sampling"},
		{"--top-k", kInt, false, false, "P2", Parsed, "top-k sampling"},
		{"--frequency-penalty", kNumber, false, false, "P2", Parsed, "frequency penalty"},
		{"--presence-penalty", kNumber, false, false, "P2", Parsed, "presence penalty"},
		{"--provider-options", kJSON, false, false, "P2", Parsed, "provider-specific options"},
	}
	hostAddFlags = []flagSpec{
		{"--ssh", kText, false, false, "P1", Gated, "user@address, or a Host alias of ssh's"},
		{"--description", kText, false, false, "P1", Gated, "what the machine is for"},
	}
	hookAddFlags = []flagSpec{
		{"--command", kText, false, false, "P1", Gated, "the command to run"},
		{"--name", kText, false, false, "P1", Gated, "a name, so a later line can update or remove it"},
		{"--matcher", kText, false, false, "P1", Gated, "which tools or sources it runs for"},
		{"--timeout", kNumber, false, false, "P1", Gated, "seconds before it is stopped"},
	}
	hookRemoveFlags = []flagSpec{
		{"--name", kText, false, false, "P1", Gated, "remove only the hook of this name"},
	}
	mcpAddFlags = []flagSpec{
		{"--type", kText, false, false, "P2", Gated, "stdio (the default), sse or http"},
		{"--command", kText, false, false, "P2", Gated, "the program to start (stdio)"},
		{"--args", kText, true, false, "P2", Gated, "one argument of the program"},
		{"--env", kPair, true, true, "P2", Gated, "a variable for the program"},
		{"--url", kText, false, false, "P2", Gated, "the server's address (sse, http)"},
		{"--header", kPair, true, true, "P2", Gated, "an HTTP header (sse, http)"},
		{"--timeout", kNumber, false, false, "P2", Gated, "seconds a tool call may take"},
		{"--disabled", kTruth, false, false, "P2", Gated, "keep the entry and do not start it"},
		{"--enabled-tools", kWords, true, false, "P2", Gated, "offer only these tools"},
		{"--disabled-tools", kWords, true, false, "P2", Gated, "do not offer these tools"},
	}
	lspAddFlags = []flagSpec{
		{"--command", kText, false, false, "P2", Gated, "the language server's program"},
		{"--args", kText, true, false, "P2", Gated, "one argument of the program"},
		{"--env", kPair, true, true, "P2", Gated, "a variable for the program"},
		{"--filetypes", kWords, true, false, "P2", Gated, "the file types it serves"},
		{"--root-markers", kWords, true, false, "P2", Gated, "files that mark a project's top folder"},
		{"--timeout", kNumber, false, false, "P2", Gated, "seconds a request may take"},
		{"--disabled", kTruth, false, false, "P2", Gated, "keep the entry and do not start it"},
		{"--init-options", kJSON, false, false, "P2", Gated, "initialization options"},
		{"--options", kJSON, false, false, "P2", Gated, "settings sent to the server"},
	}
)

// directives is every directive of gnoshrc, in the order the docs list
// them.
var directives = []directiveSpec{
	{"provider", []subSpec{
		{"add", "", "<id>", providerFlags, "P0", Works, ""},
		{"remove", "rm", "<id>", nil, "P0", Works, ""},
	}},
	{"model", []subSpec{
		{"add", "", "<provider>/<id>", modelAddFlags, "P0", Works, ""},
		{"remove", "rm", "<provider>/<id>", nil, "P0", Works, ""},
		{"large", "", "[<provider>/<id>]", modelSlotFlags, "P0", Works, ""},
		{"small", "", "[<provider>/<id>]", modelSlotFlags, "P0", Works, ""},
	}},
	{"option", []subSpec{
		{"<key>", "", "[value]", nil, "P0", Works, ""},
		{"reset", "", "<key>", nil, "P0", Works, ""},
	}},
	{"host", []subSpec{
		{"add", "", "<name>", hostAddFlags, "P1", Gated, FeatureHosts},
		{"remove", "rm", "<name>", nil, "P1", Gated, FeatureHosts},
	}},
	{"hook", []subSpec{
		{"add", "", "<event>", hookAddFlags, "P1", Gated, FeatureHooks},
		{"remove", "rm", "<event>", hookRemoveFlags, "P1", Gated, FeatureHooks},
	}},
	{"permissions", []subSpec{
		{"allow", "", "<rule>...", nil, "P1", Gated, FeaturePermissions},
		{"deny", "", "<rule>...", nil, "P1", Gated, FeaturePermissions},
	}},
	{"mcp", []subSpec{
		{"add", "", "<name>", mcpAddFlags, "P2", Gated, FeatureMCP},
		{"remove", "rm", "<name>", nil, "P2", Gated, FeatureMCP},
	}},
	{"lsp", []subSpec{
		{"add", "", "<name>", lspAddFlags, "P2", Gated, FeatureLSP},
		{"remove", "rm", "<name>", nil, "P2", Gated, FeatureLSP},
	}},
}

// plannedDirectives are words the spec names as replacements for shell
// constructs and that are not built yet. Each is a startup error that says
// so, rather than "unknown directive".
var plannedDirectives = map[string]string{
	"include": "include is planned and not built yet, so the other file was not read. Put its lines in this file",
}

// directiveNames lists the directives for a message.
func directiveNames() string {
	names := make([]string, len(directives))
	for i, d := range directives {
		names[i] = d.name
	}
	return list(names)
}

// findDirective returns the spec of a directive, or nil.
func findDirective(name string) *directiveSpec {
	for i := range directives {
		if directives[i].name == name {
			return &directives[i]
		}
	}
	return nil
}

// findSub returns the form of a directive that word names, or nil.
func (d *directiveSpec) findSub(word string) *subSpec {
	for i := range d.subs {
		if s := &d.subs[i]; s.name == word || s.alias != "" && s.alias == word {
			return s
		}
	}
	return nil
}

// subNames lists a directive's forms for a message.
func (d *directiveSpec) subNames() string {
	var names []string
	for _, s := range d.subs {
		if !strings.HasPrefix(s.name, "<") {
			names = append(names, s.name)
		}
	}
	return list(names)
}

// providerType describes one value of provider --type.
type providerType struct {
	name string
	// wire is the request format the type speaks: "llamacpp", "openai" or
	// "anthropic". Empty means the type is known and not built yet.
	wire string
	pri  string
	// v1 says a base URL of this type is completed with /v1: always, for a
	// server on the user's own machine, or only when the URL has no path.
	v1 string // "always", "bare" or ""
	// url is the service's public address, used when none is given.
	url string
	// keyVar is the environment variable read for the key when the
	// configuration gives none.
	keyVar string
}

// providerTypes is every value of provider --type.
var providerTypes = []providerType{
	{name: "llamacpp", wire: "llamacpp", pri: "P0", v1: "always"},
	{name: "openai-compat", wire: "openai", pri: "P0", v1: "bare"},
	{name: "anthropic", wire: "anthropic", pri: "P1", url: "https://api.anthropic.com", keyVar: "ANTHROPIC_API_KEY"},
	{name: "openai", wire: "openai", pri: "P1", v1: "bare", url: "https://api.openai.com/v1", keyVar: "OPENAI_API_KEY"},
	{name: "ollama", wire: "openai", pri: "P1", v1: "always"},
	{name: "lmstudio", wire: "openai", pri: "P1", v1: "always"},
	{name: "litellm", wire: "openai", pri: "P2", v1: "always"},
	{name: "omlx", wire: "openai", pri: "P2", v1: "always"},
	{name: "openrouter", pri: "P2"},
	{name: "vercel", pri: "P2"},
	{name: "google", pri: "P2"},
	{name: "google-vertex", pri: "P2"},
	{name: "azure", pri: "P2"},
	{name: "bedrock", pri: "P2"},
}

// providerTypeNames lists the provider types for a message.
func providerTypeNames() []string {
	names := make([]string, len(providerTypes))
	for i, t := range providerTypes {
		names[i] = t.name
	}
	return names
}

// findProviderType returns the description of a provider type, or nil.
func findProviderType(name string) *providerType {
	for i := range providerTypes {
		if providerTypes[i].name == name {
			return &providerTypes[i]
		}
	}
	return nil
}

// hookEvent describes one hook event gnosh knows.
type hookEvent struct {
	name string
	// matcher says what a matcher is tested against, or "" for an event
	// that has nothing to match, where a matcher is a mistake.
	matcher string
	// blocks reports whether a hook that cannot run holds work up: every
	// matching call, every prompt, or the end of every turn.
	blocks bool
	pri    string
}

// hookEvents is the fifteen events, in the spec's order. Names are exact:
// no other letter case and no underscore form is an event.
var hookEvents = []hookEvent{
	{"PreToolUse", "the tool's name", true, "P1"},
	{"PostToolUse", "the tool's name", false, "P1"},
	{"PostToolUseFailure", "the tool's name", false, "P1"},
	{"UserPromptSubmit", "", true, "P1"},
	{"SessionStart", "the source", false, "P1"},
	{"Stop", "", true, "P1"},
	{"SubagentStart", "the sub-agent's type", false, "P2"},
	{"SubagentStop", "", true, "P2"},
	{"StopFailure", "the error's kind", false, "P2"},
	{"PreCompact", "the trigger", false, "P2"},
	{"PostCompact", "the trigger", false, "P2"},
	{"SessionEnd", "the reason", false, "P2"},
	{"InstructionsLoaded", "the load reason", false, "P2"},
	{"PermissionRequest", "the tool's name", false, "P2"},
	{"Notification", "the type", false, "P2"},
}

// HookEventNames returns the names of the hook events gnosh knows, in the
// spec's order.
func HookEventNames() []string {
	names := make([]string, len(hookEvents))
	for i, e := range hookEvents {
		names[i] = e.name
	}
	return names
}

// findHookEvent returns the description of a hook event, or nil.
func findHookEvent(name string) *hookEvent {
	for i := range hookEvents {
		if hookEvents[i].name == name {
			return &hookEvents[i]
		}
	}
	return nil
}

// nearHookEvent finds the event a misspelt name most likely meant: the one
// with the same letters once case and underscores are set aside.
func nearHookEvent(name string) string {
	fold := func(s string) string {
		return strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(s))
	}
	for _, e := range hookEvents {
		if fold(e.name) == fold(name) {
			return e.name
		}
	}
	return ""
}

// hookFields is what gnosh does with each field a Claude Code hook handler
// can carry. "" means the field is read; "same" means it is accepted when
// it has the value gnosh would use anyway, given in hookFieldSame; any
// other text is the reason the field stops startup.
var hookFields = map[string]string{
	"type":          "",
	"command":       "",
	"timeout":       "",
	"statusMessage": "ignored",
	"once":          "ignored",
	"shell":         "same",
	"async":         "same",
	"asyncRewake":   "same",
	"onFailure":     "same",
	"if":            "gnosh cannot filter a hook the way Claude Code's \"if\" does, and a guard run some other way than written may let through what it was meant to stop. Move the test into the command",
	"args":          "gnosh runs a hook's command through the shell and has no \"args\" form. Write the arguments into the command",
}

// hookFieldSame is the one value each "same" field may have.
var hookFieldSame = map[string]string{
	"shell":       `"bash"`,
	"async":       "false",
	"asyncRewake": "false",
	"onFailure":   `"continue"`,
}

// settingsKeys is every key gnosh.json may hold at its top, with what it
// needs. Any other key is a startup error.
var settingsKeys = []struct {
	key     string
	pri     string
	status  Status
	feature Feature
	what    string
}{
	{"permissions", "P1", Gated, FeaturePermissions, "allow, ask and deny rules, the starting mode, more directories"},
	{"hooks", "P1", Gated, FeatureHooks, "hooks, in Claude Code's shape"},
	{"env", "P1", Gated, FeatureEnv, "variables for the commands and hooks gnosh starts"},
	{"disableAllHooks", "P1", Works, "", "switch every hook off"},
	{"$schema", "P1", Works, "", "accepted and not read"},
}

// permissionKeys is every key the permissions object may hold.
var permissionKeys = []struct {
	key    string
	pri    string
	status Status
	what   string
}{
	{"allow", "P1", Gated, "rules that run without asking"},
	{"ask", "P1", Gated, "rules that always ask"},
	{"deny", "P1", Gated, "rules that always refuse"},
	{"defaultMode", "P1", Gated, "the mode a session starts in"},
	{"additionalDirectories", "P1", Gated, "more directories the file tools may use"},
	{"disableBypassPermissionsMode", "P1", Gated, "\"disable\" forbids skipping permissions"},
	{"disableAutoMode", "P1", Works, "\"disable\"; gnosh has no auto mode, so it changes nothing"},
	{"denyInsteadOfPrompting", "P1", Gated, "refuse what would ask; not from a project's file"},
	{"blockReadsOutsideWorkingDirectories", "P2", NotBuilt, "refuse reads outside the working directories"},
	{"blockWritesOutsideWorkingDirectories", "P2", NotBuilt, "refuse writes outside the working directories"},
}

// permissionModes is every mode a session can start in. "manual" is
// another word for "default".
var permissionModes = []string{"default", "plan", "acceptEdits", "dontAsk", "bypassPermissions"}

// envVar describes an environment variable gnosh reads or sets.
type envVar struct {
	name string
	pri  string
	use  string // "reads", "sets", "accepts" (does nothing) or "tests"
	what string
}

// ownEnv is every GNOSH_ variable gnosh knows. Any other GNOSH_ name in the
// environment is a startup error: it is most likely a misspelling of one of
// these, and would otherwise do nothing without a word.
var ownEnv = []envVar{
	{"GNOSH_GLOBAL_CONFIG", "P0", "reads", "the directory to use in place of the user's config directory"},
	{"GNOSH_GLOBAL_DATA", "P0", "reads", "the directory to use in place of the user's state directory"},
	{"GNOSH_SAFE_MODE", "P2", "reads", "the same as --safe-mode"},
	{"GNOSH_PROFILE", "P2", "reads", "serve a debugging page on loopback"},
	{"GNOSH_DISABLE_METRICS", "P2", "reads", "1 forces analytics off, 0 turns them on"},
	{"GNOSH_DISABLE_UPDATE_CHECK", "P2", "reads", "0 turns the update check on"},
	{"GNOSH_METRICS_URL", "P2", "reads", "as option metrics-url"},
	{"GNOSH_UPDATE_REPO", "P2", "reads", "as option update-repo"},
	// The two exceptions to "unknown is an error": Jae's wrapper script
	// sets them, and there is no catalog for them to switch off. They go
	// when the wrapper is edited.
	{"GNOSH_DISABLE_PROVIDER_AUTO_UPDATE", "P0", "accepts", "does nothing: there is no provider catalog"},
	{"GNOSH_DISABLE_DEFAULT_PROVIDERS", "P0", "accepts", "does nothing: there is no provider catalog"},
	// What gnosh sets for the commands and hooks it starts. A gnosh started
	// from inside one of those inherits them.
	{"GNOSH", "P0", "sets", "1 in every command gnosh runs"},
	{"GNOSH_EVENT", "P1", "sets", "a hook's event"},
	{"GNOSH_SESSION_ID", "P1", "sets", "the session"},
	{"GNOSH_CWD", "P1", "sets", "the working directory"},
	{"GNOSH_PROJECT_DIR", "P1", "sets", "the project root"},
	{"GNOSH_TOOL_NAME", "P1", "sets", "a tool event's tool"},
	{"GNOSH_TOOL_USE_ID", "P1", "sets", "a tool event's call"},
	{"GNOSH_TOOL_INPUT_COMMAND", "P1", "sets", "a shell call's command"},
	{"GNOSH_TOOL_INPUT_FILE_PATH", "P1", "sets", "a file tool's path"},
	{"GNOSH_STOP_HOOK_ACTIVE", "P1", "sets", "1 when a Stop hook is keeping the turn going"},
	{"GNOSH_AGENT_ID", "P1", "sets", "a sub-agent's id"},
	{"GNOSH_AGENT_TYPE", "P1", "sets", "a sub-agent's type"},
	{"GNOSH_SESSION_START_SOURCE", "P1", "sets", "SessionStart's source"},
	{"GNOSH_SESSION_END_REASON", "P2", "sets", "SessionEnd's reason"},
	{"GNOSH_PRE_COMPACT_TRIGGER", "P2", "sets", "PreCompact's trigger"},
	{"GNOSH_POST_COMPACT_TRIGGER", "P2", "sets", "PostCompact's trigger"},
	{"GNOSH_STOP_FAILURE_ERROR", "P2", "sets", "StopFailure's error kind"},
	{"GNOSH_INSTRUCTIONS_LOAD_REASON", "P2", "sets", "InstructionsLoaded's reason"},
	{"GNOSH_NOTIFICATION_TYPE", "P2", "sets", "Notification's type"},
	{"GNOSH_ENV_FILE", "P2", "sets", "SessionStart's environment file"},
	{"GNOSH_MEM_STATUS", "P1", "sets", "the file that says what mem holds"},
	// Switches read by gnosh's own tests and development scripts.
	{"GNOSH_PEER_CHILD", "", "tests", "a test switch"},
	{"GNOSH_PEERS_SWARM", "", "tests", "a test switch"},
	{"GNOSH_PEER_WORK", "", "tests", "a test switch"},
	{"GNOSH_IGNORE_ROUNDS", "", "tests", "a test switch"},
	{"GNOSH_IGNORE_SEED", "", "tests", "a test switch"},
}

// ownEnvPrefixes are the families of names kept for tests and development
// scripts. A name under one is never a mistake.
var ownEnvPrefixes = []string{"GNOSH_TEST_", "GNOSH_DEV_"}

// knownOwnEnv reports whether name is a GNOSH_ variable gnosh knows.
func knownOwnEnv(name string, more []string) bool {
	for _, p := range ownEnvPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	if slices.Contains(more, name) {
		return true
	}
	for _, v := range ownEnv {
		if v.name == name {
			return true
		}
	}
	return false
}

// runTimeVariable reports whether gnosh itself sets the variable for the
// hooks and commands it runs, so that its value belongs to one run of one
// of those and never to a config file.
func runTimeVariable(name string) bool {
	if name == "CLAUDE_PROJECT_DIR" || name == "CLAUDE_ENV_FILE" {
		return true
	}
	for _, v := range ownEnv {
		if v.name == name {
			return v.use == "sets"
		}
	}
	return false
}

// readEnvNames lists the GNOSH_ variables gnosh reads, for a message.
func readEnvNames() []string {
	var names []string
	for _, v := range ownEnv {
		if v.use == "reads" {
			names = append(names, v.name)
		}
	}
	return names
}

// oldName is a name from before the rewrite that is still read.
type oldName struct {
	// what is the thing looked for.
	what string
	// now is its name today, and old the name still read when the one in
	// now does not exist.
	now, old string
	// only names the single key read from the old file, when the rest of
	// it is not read.
	only string
}

// oldNames is the complete list of names from the earlier gnosh that are
// still read. Each is here because one of Jae's own files still has that
// name, and for no other reason (CFG-5). No warning is printed about any of
// them. This table, and the three places that use it, are deleted once his
// files have moved, after asking him (OQ-1).
//
// The directory names are relative: "crush" beside the user's config
// directory, and the project's own folder.
var oldNames = []oldName{
	{what: "the user's gnoshrc", now: "gnosh/gnoshrc", old: "crush/crushrc"},
	{what: "the user's global context file", now: "gnosh/GNOSH.md", old: "crush/CRUSH.md"},
	{what: "the project's gnosh.json", now: "gnosh.json", old: "klash.json", only: "hooks"},
}

// Entry is one line of the table of what the configuration can say.
type Entry struct {
	// Name is the directive, flag, option, key or variable: "provider add
	// --base-url", "option notifications", "gnosh.json env".
	Name string
	// Priority is the spec's: P0, P1 or P2.
	Priority string
	Status   Status
	// Feature is the part of gnosh a Gated entry needs.
	Feature Feature
	// What says what it is for.
	What string
}

// Table lists everything the configuration can say and what gnosh does
// with each today. It is made from the tables the parsers read, so it
// cannot disagree with them.
func Table() []Entry {
	var out []Entry
	for _, d := range directives {
		if d.name == "option" {
			continue
		}
		for _, s := range d.subs {
			out = append(out, Entry{Name: d.name + " " + s.name + " " + s.arg, Priority: s.pri, Status: s.status, Feature: s.feature})
			for _, f := range s.flags {
				e := Entry{Name: d.name + " " + s.name + " " + f.name, Priority: f.pri, Status: f.status, What: f.what}
				if f.status == Gated {
					e.Feature = s.feature
				}
				out = append(out, e)
			}
		}
	}
	for _, word := range slices.Sorted(maps.Keys(plannedDirectives)) {
		out = append(out, Entry{Name: word, Priority: "P2", Status: NotBuilt, What: "read another file"})
	}
	for _, o := range optionSpecs {
		out = append(out, Entry{Name: "option " + o.key, Priority: o.pri, Status: o.status, What: o.what})
	}
	for _, key := range slices.Sorted(maps.Keys(goneOptions)) {
		out = append(out, Entry{Name: "option " + key, Status: Gone, What: goneOptions[key]})
	}
	for _, t := range providerTypes {
		e := Entry{Name: "provider type " + t.name, Priority: t.pri, Status: Works, What: "speaks the " + t.wire + " format"}
		if t.wire == "" {
			e.Status, e.What = NotBuilt, "the provider is left out, with that reason"
		}
		out = append(out, e)
	}
	for _, k := range settingsKeys {
		out = append(out, Entry{Name: "gnosh.json " + k.key, Priority: k.pri, Status: k.status, Feature: k.feature, What: k.what})
	}
	for _, k := range permissionKeys {
		e := Entry{Name: "gnosh.json permissions." + k.key, Priority: k.pri, Status: k.status, What: k.what}
		if k.status == Gated {
			e.Feature = FeaturePermissions
		}
		out = append(out, e)
	}
	for _, e := range hookEvents {
		out = append(out, Entry{Name: "hook event " + e.name, Priority: e.pri, Status: Gated, Feature: FeatureHooks})
	}
	for _, v := range ownEnv {
		if v.use == "reads" || v.use == "accepts" {
			out = append(out, Entry{Name: "$" + v.name, Priority: v.pri, Status: Works, What: v.what})
		}
	}
	for _, v := range limitVars {
		out = append(out, Entry{Name: "$" + v.name, Priority: "P1", Status: Works, What: v.what})
	}
	return out
}

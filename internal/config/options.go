package config

import (
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// ListItem is one value of a list option, with where it came from.
type ListItem struct {
	V      string
	Origin Origin
}

// Options are the settings of the option directive, each with its typed
// value and where that value came from. A field whose Set is false holds
// the default.
type Options struct {
	// RequestTimeout is how many seconds a model request may go without
	// sending anything before it is given up. Zero waits for ever. When
	// nothing set it, each provider address has the limits that suit it
	// (see Endpoint.Timeouts), and V is meaningless.
	RequestTimeout Value[int]
	// Notifications is how gnosh tells the user it needs him: auto, native,
	// osc, bell or disabled.
	Notifications Value[string]
	// MemEmbeddingsURL is the embedding server for mem. Unset, nothing is
	// sent anywhere.
	MemEmbeddingsURL Value[string]

	// AutoSummarize switches the compaction backstop.
	AutoSummarize Value[bool]
	// AutoCompactWindow is how full the context may get, in tokens, before
	// the backstop runs. Unset means automatic.
	AutoCompactWindow Value[int]
	// IdleCompaction switches the summary of a long conversation left idle.
	IdleCompaction Value[bool]
	// LoopMaxTurns is an optional cap on /loop. Unset, zero or negative
	// means none.
	LoopMaxTurns Value[int]
	// MaxBackgroundJobs is how many background commands may run at once.
	MaxBackgroundJobs Value[int]
	// MemReviewCloses switches the review of a closed todo.
	MemReviewCloses Value[bool]

	// GlobalContextPaths are more global context files or folders, as
	// absolute paths.
	GlobalContextPaths []ListItem
	// ContextPaths are more project context files, as absolute paths.
	ContextPaths []ListItem
	// SkillPaths are more places to find skills, as absolute paths.
	SkillPaths []ListItem
	// DisabledSkills are skills hidden from the model.
	DisabledSkills []ListItem
	// InitializeAs is the file /init writes.
	InitializeAs Value[string]

	// Metrics says whether anonymous usage events may be sent. Unset means
	// nothing decided it, which is off.
	Metrics Value[bool]
	// MetricsURL and MetricsKey say where usage events go.
	MetricsURL Value[string]
	MetricsKey Value[Secret]
	// UpdateRepo is the repository whose releases the update check reads.
	// Unset means no check.
	UpdateRepo Value[string]
	// ArtifactServer is the address recorded deliverables are served at,
	// or "off".
	ArtifactServer Value[string]

	// Debug writes more to the log.
	Debug Value[bool]
	// DataDirectory is the name of the project data folder.
	DataDirectory Value[string]
	// AttributionTrailerStyle is "off" or "co-authored-by": the trailer
	// line added to commits the agent makes.
	AttributionTrailerStyle Value[string]
	// AttributionGeneratedWith adds a "generated with" line to commits and
	// pull requests the agent makes.
	AttributionGeneratedWith Value[bool]
}

// defaultOptions returns the options as they are when nothing sets them.
func defaultOptions() Options {
	var o Options
	o.Notifications.V = "auto"
	o.AutoSummarize.V = true
	o.IdleCompaction.V = true
	o.MaxBackgroundJobs.V = 50
	o.MemReviewCloses.V = true
	o.InitializeAs.V = "AGENTS.md"
	o.ArtifactServer.V = "127.0.0.1:7777"
	o.DataDirectory.V = ".gnosh"
	o.AttributionTrailerStyle.V = "off"
	return o
}

// optKind is the shape of an option's value.
type optKind int

const (
	oTruth    optKind = iota // true or false; the value may be left out
	oInt                     // a whole number
	oEnum                    // one of a few words
	oText                    // any text
	oURL                     // an http or https address
	oPath                    // a file or directory; a leading ~ is the home directory
	oPathList                // a list of paths, one added per line
	oNameList                // a list of names, one added per line
)

// optionSpec describes one key of the option directive: what its value
// looks like, what gnosh does with it today, and which field it sets.
type optionSpec struct {
	key    string
	kind   optKind
	pri    string
	status Status
	def    string // the default, as the docs show it
	what   string

	enum     []string            // for oEnum: the words it takes
	min, max int                 // for oInt, when bounded is set: the least and the most
	bounded  bool                // the number must lie between min and max
	trust    string              // what an untrusted folder's line counts as; "" is kindOption
	fromCwd  bool                // a relative path is under the working directory, not beside the file
	check    func(string) string // for oText: says what is wrong with a value, or ""

	truth  func(*Options) *Value[bool]
	num    func(*Options) *Value[int]
	text   func(*Options) *Value[string]
	secret func(*Options) *Value[Secret]
	list   func(*Options) *[]ListItem
}

// optionSpecs is every key the option directive takes, grouped as the docs
// group them. A key that is not here is a startup error.
var optionSpecs = []optionSpec{
	// Options Jae uses.
	{key: "request-timeout", kind: oInt, pri: "P0", status: Works, def: "900 on your own machine, 60 elsewhere",
		what: "seconds a model request may go without sending anything; 0 waits for ever", bounded: true, min: 0, max: 1 << 30,
		num: func(o *Options) *Value[int] { return &o.RequestTimeout }},
	{key: "notifications", kind: oEnum, pri: "P0", status: Parsed, def: "auto",
		what: "how gnosh says it needs you", enum: []string{"auto", "native", "osc", "bell", "disabled"},
		text: func(o *Options) *Value[string] { return &o.Notifications }},
	{key: "mem-embeddings-url", kind: oURL, pri: "P1", status: Works, def: "unset",
		what: "the embedding server for mem; unset sends nothing anywhere", trust: kindAddress,
		text: func(o *Options) *Value[string] { return &o.MemEmbeddingsURL }},

	// Options about the turn.
	{key: "auto-summarize", kind: oTruth, pri: "P1", status: Works, def: "true",
		what: "the compaction backstop", truth: func(o *Options) *Value[bool] { return &o.AutoSummarize }},
	{key: "auto-compact-window", kind: oInt, pri: "P1", status: Works, def: "unset (automatic)",
		what: "tokens of context before the backstop runs", bounded: true, min: 100000, max: 1000000,
		num: func(o *Options) *Value[int] { return &o.AutoCompactWindow }},
	{key: "idle-compaction", kind: oTruth, pri: "P2", status: Parsed, def: "true",
		what: "summarize a long conversation left idle", truth: func(o *Options) *Value[bool] { return &o.IdleCompaction }},
	{key: "loop-max-turns", kind: oInt, pri: "P1", status: Works, def: "unset (no cap)",
		what: "an optional cap on /loop; zero or less means none", num: func(o *Options) *Value[int] { return &o.LoopMaxTurns }},
	{key: "max-background-jobs", kind: oInt, pri: "P1", status: Works, def: "50",
		what: "background commands that may run at once", bounded: true, min: 1, max: 100000,
		num: func(o *Options) *Value[int] { return &o.MaxBackgroundJobs }},
	{key: "mem-review-closes", kind: oTruth, pri: "P1", status: Works, def: "true",
		what: "review a todo when it is closed", truth: func(o *Options) *Value[bool] { return &o.MemReviewCloses }},

	// Options about what the model reads.
	{key: "global-context-path", kind: oPathList, pri: "P1", status: Works, def: "none beyond the two global files",
		what: "more global context: a file, or a folder of Markdown files", trust: kindExtraPath,
		list: func(o *Options) *[]ListItem { return &o.GlobalContextPaths }},
	{key: "context-path", kind: oPathList, pri: "P1", status: Works, def: "none beyond the usual names",
		what: "more project context files", trust: kindExtraPath, fromCwd: true,
		list: func(o *Options) *[]ListItem { return &o.ContextPaths }},
	{key: "skill-path", kind: oPathList, pri: "P2", status: Parsed, def: "none",
		what: "more places to find skills", trust: kindExtraPath,
		list: func(o *Options) *[]ListItem { return &o.SkillPaths }},
	{key: "disable-skill", kind: oNameList, pri: "P2", status: Parsed, def: "none",
		what: "hide a skill from the model", trust: kindDisabledSkill,
		list: func(o *Options) *[]ListItem { return &o.DisabledSkills }},
	{key: "initialize-as", kind: oText, pri: "P2", status: Parsed, def: "AGENTS.md",
		what: "the file /init writes", check: checkFileName,
		text: func(o *Options) *Value[string] { return &o.InitializeAs }},

	// Options about the network: all off unless set.
	{key: "metrics", kind: oTruth, pri: "P2", status: Parsed, def: "unset (off)",
		what: "send anonymous usage events", trust: kindAddress,
		truth: func(o *Options) *Value[bool] { return &o.Metrics }},
	{key: "metrics-url", kind: oURL, pri: "P2", status: Parsed, def: "unset",
		what: "where usage events go", trust: kindAddress,
		text: func(o *Options) *Value[string] { return &o.MetricsURL }},
	{key: "metrics-key", kind: oText, pri: "P2", status: Parsed, def: "unset",
		what: "the key sent with usage events", trust: kindAddress,
		secret: func(o *Options) *Value[Secret] { return &o.MetricsKey }},
	{key: "update-repo", kind: oText, pri: "P2", status: Parsed, def: "unset (no check)",
		what: "owner/repo whose releases the update check reads", trust: kindAddress, check: checkRepo,
		text: func(o *Options) *Value[string] { return &o.UpdateRepo }},
	{key: "artifact-server", kind: oText, pri: "P2", status: Parsed, def: "127.0.0.1:7777",
		what: "the address deliverables are served at, or off", trust: kindAddress, check: checkAddress,
		text: func(o *Options) *Value[string] { return &o.ArtifactServer }},

	// Options about gnosh itself.
	{key: "debug", kind: oTruth, pri: "P1", status: Works, def: "false",
		what: "write more to the log", truth: func(o *Options) *Value[bool] { return &o.Debug }},
	{key: "data-directory", kind: oPath, pri: "P2", status: Works, def: ".gnosh",
		what: "the project data folder's name", text: func(o *Options) *Value[string] { return &o.DataDirectory }},
	{key: "attribution-trailer-style", kind: oEnum, pri: "P2", status: Parsed, def: "off",
		what: "the trailer line added to the agent's commits", enum: []string{"off", "co-authored-by"},
		text: func(o *Options) *Value[string] { return &o.AttributionTrailerStyle }},
	{key: "attribution-generated-with", kind: oTruth, pri: "P2", status: Parsed, def: "false",
		what:  "add a \"generated with\" line to the agent's commits",
		truth: func(o *Options) *Value[bool] { return &o.AttributionGeneratedWith }},
}

// goneOptions are option keys of the earlier gnosh that are not carried
// over, each with the reason. Setting one is a startup error that gives
// the reason.
var goneOptions = map[string]string{
	"provider-auto-update": "there is no provider catalog to update: the providers are the ones provider add names",
	"default-providers":    "there is no built-in provider catalog: the providers are the ones provider add names",
	"auto-lsp":             "a language server is started only when lsp add names it",
	"debug-lsp":            "there is one log, and option debug writes more to it",
	"progress":             "the progress indicator of a run is switched off with -q",
	"ui":                   "the ui options set up the full-screen interface, which gnosh no longer has",
}

// goneOption returns why an option key is gone, or "".
func goneOption(key string) string {
	if key == "ui" || strings.HasPrefix(key, "ui.") || strings.HasPrefix(key, "ui-") || strings.HasPrefix(key, "ui_") {
		return goneOptions["ui"]
	}
	return goneOptions[key]
}

// findOption returns the description of an option key, or nil.
func findOption(key string) *optionSpec {
	for i := range optionSpecs {
		if optionSpecs[i].key == key {
			return &optionSpecs[i]
		}
	}
	return nil
}

// optionKeys lists the option keys for a message.
func optionKeys() []string {
	keys := make([]string, len(optionSpecs))
	for i, o := range optionSpecs {
		keys[i] = o.key
	}
	return keys
}

// trustKind is what a line setting this option counts as when the folder
// it is in is not trusted.
func (o *optionSpec) trustKind() string {
	if o.trust != "" {
		return o.trust
	}
	return kindOption
}

// parseTruth reads a truth value: true, false, 1, 0, yes or no, in any
// letter case.
func parseTruth(s string) (bool, bool) {
	switch strings.ToLower(s) {
	case "true", "1", "yes":
		return true, true
	case "false", "0", "no":
		return false, true
	}
	return false, false
}

// truthWords is how a message lists the truth values.
const truthWords = "true, false, 1, 0, yes or no"

// checkURL says what is wrong with an address, or "".
func checkURL(s string) string {
	if s == "" {
		return "it is empty"
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return "it must start with http:// or https://"
	}
	u, err := url.Parse(s)
	switch {
	case err != nil:
		return "it cannot be read as an address"
	case u.Hostname() == "":
		return "it names no host"
	}
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return "its port is not a number from 1 to 65535"
		}
	}
	return ""
}

// checkFileName says what is wrong with a plain file name, or "".
func checkFileName(s string) string {
	if s == "" || s == "." || s == ".." || strings.ContainsAny(s, `/\`) {
		return "it must be a file name with no folder in front"
	}
	return ""
}

// checkRepo says what is wrong with an owner/repo name, or "".
func checkRepo(s string) string {
	owner, repo, ok := strings.Cut(s, "/")
	if !ok || !isRepoWord(owner) || !isRepoWord(repo) {
		return "it must have the form owner/repo"
	}
	return ""
}

// isRepoWord reports whether s can be one half of owner/repo.
func isRepoWord(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '-' || c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}

// checkAddress says what is wrong with a host:port address or "off", or "".
func checkAddress(s string) string {
	if s == "off" {
		return ""
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" {
		return "it must be an address such as 127.0.0.1:7777, or off"
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "its port must be a number from 1 to 65535"
	}
	return ""
}

// expandHome replaces a leading ~ or ~/ with the home directory.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// absPath makes path absolute: a leading ~ is the home directory, and a
// relative path is under base.
func absPath(path, home, base string) string {
	path = expandHome(path, home)
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path)
}

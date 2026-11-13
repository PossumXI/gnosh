package cli

import (
	"fmt"
	"strings"
)

// The ids of the flags that the code outside the table asks about by name.
const (
	idHelp      = "--help"
	idVersion   = "--version"
	idPrint     = "--print"
	idContinue  = "--continue"
	idResume    = "--resume"
	idSession   = "--session"
	idSessionID = "--session-id"
	idFork      = "--fork-session"
	idYolo      = "--yolo"
	idMode      = "--permission-mode"
	idPartial   = "--include-partial-messages"
	idReplay    = "--replay-user-messages"
)

// The headings flags are listed under in help, in the order they appear.
const (
	groupConversation = "Which conversation"
	groupModel        = "Which model"
	groupPermissions  = "Permissions"
	groupTold         = "What the model is told"
	groupSettings     = "Settings for this run"
	groupPrint        = "Print mode"
	groupOther        = "Help and version"
	groupOwn          = "Flags"
)

// groupOrder is the order of the headings.
var groupOrder = []string{
	groupConversation, groupModel, groupPermissions, groupTold,
	groupSettings, groupPrint, groupOwn, groupOther,
}

// add puts a flag in the table: it takes the spellings apart, names the
// flag after its main spelling unless it has an id, and marks it built if
// the specs call it P0. A value of a flag is built only if both the flag
// and the value are P0.
func (t *Table) add(f *flag) *flag {
	for _, n := range strings.Fields(f.names) {
		if strings.HasPrefix(n, "--") {
			f.long = append(f.long, n[2:])
		} else {
			f.short = append(f.short, []rune(n[1:])...)
		}
	}
	if f.id == "" {
		f.id = f.main()
	}
	if f.group == "" {
		f.group = groupOwn
	}
	if f.pri == 0 {
		f.status = Built
	}
	for i := range f.choices {
		c := &f.choices[i]
		c.pri = max(c.pri, f.pri)
		if c.pri == 0 {
			c.status = Built
		}
	}
	t.flags = append(t.flags, f)
	return f
}

// command puts a command in the table under its parent, and marks it built
// if the specs call it P0.
func (t *Table) command(parent *command, c *command) *command {
	c.parent = parent
	if parent != nil {
		parent.subs = append(parent.subs, c)
	}
	if c.pri == 0 {
		c.status = Built
	}
	t.commands = append(t.commands, c)
	return c
}

// Default returns gnosh's table of commands and flags as it ships: every
// command and flag of spec/gnosh/harness-cli.md and the P0 and P1 ones of
// spec/cc/cli.md. What the specs call P0 is marked Built and the rest
// NotBuilt; the code that wires a feature in marks it with SetStatus when
// the feature lands. Each call returns a table of its own.
//
// To add a flag, add a row here and a field to Invocation; to add a
// command, add a row and a Command. Nothing else needs to know.
func Default() *Table {
	t := &Table{}

	// ------------------------------------------------------------------
	// The flags of a session: `gnosh` and `gnosh run` take all of them.
	// ------------------------------------------------------------------

	cont := t.add(&flag{names: "-c -C --continue", group: groupConversation, pri: 0,
		help:       "Continue the most recent session of this project. (-c used to take the working folder, which is --cwd now. So a single word right after -c is refused: write -C before a one-word prompt.)",
		refuseNext: wordAfterC})
	resume := t.add(&flag{names: "-r --resume", kind: kindOptional, arg: "session", group: groupConversation, pri: 0,
		help: "Resume a session by its ID or its name, or from a transcript file given as an absolute path ending in .jsonl. With nothing after it, show the sessions to pick from; in print mode a session must be named. The argument after -r is always taken as the session, never as the prompt.",
		set:  func(i *Invocation, v flagValue) { i.Resume = v.text }})
	session := t.add(&flag{names: "-s --session", kind: kindText, arg: "id", group: groupConversation, pri: 0,
		help: "Resume the session with this ID. An ID only, never a name.",
		set:  func(i *Invocation, v flagValue) { i.Session = v.text }})
	sessionID := t.add(&flag{names: "--session-id", kind: kindText, arg: "uuid", group: groupConversation, pri: 0,
		help:  "Give the new conversation this ID, which must be a UUID. With -c, -r or -s it needs --fork-session too, and names the copy.",
		check: isUUID,
		set:   func(i *Invocation, v flagValue) { i.SessionID = v.text }})
	fork := t.add(&flag{names: "--fork-session", group: groupConversation, pri: 1,
		help: "With -c, -r or -s: go on in a copy of the session and leave the original as it was.",
		set:  func(i *Invocation, _ flagValue) { i.ForkSession = true }})
	name := t.add(&flag{names: "-n --name", kind: kindText, arg: "name", group: groupConversation, pri: 1,
		help: "Name the session: the new one, the one resumed, or the --fork-session copy.",
		set:  func(i *Invocation, v flagValue) { i.Name = v.text }})

	model := t.add(&flag{names: "-m --model", kind: kindText, arg: "model", group: groupModel, pri: 0,
		help: "Use this model for this run only, written as model or provider/model. Nothing is saved.",
		set:  func(i *Invocation, v flagValue) { i.Model = v.text }})
	effort := t.add(&flag{names: "--reasoning-effort --effort", kind: kindText, arg: "level", group: groupModel, pri: 1,
		help: "How hard the model thinks in this run. The levels depend on the model.",
		set:  func(i *Invocation, v flagValue) { i.Effort = v.text }})

	yolo := t.add(&flag{names: "-y --yolo --dangerously-skip-permissions", group: groupPermissions, pri: 0,
		help: "Skip every permission question; the same as --permission-mode bypassPermissions. A print run refuses whatever would have asked, unless it is given this, a permission mode or allow rules."})
	allowSkip := t.add(&flag{names: "--allow-dangerously-skip-permissions", group: groupPermissions, pri: 1,
		help: "Let the session be switched to skipping permissions later, without starting that way.",
		set:  func(i *Invocation, _ flagValue) { i.AllowSkipPermissions = true }})
	mode := t.add(&flag{names: "--permission-mode", kind: kindText, arg: "mode", group: groupPermissions, pri: 1,
		help: "Start in this permission mode: default (also written manual), plan, acceptEdits, dontAsk or bypassPermissions.",
		choices: []choice{
			{name: ModeDefault}, {name: "manual", means: ModeDefault}, {name: ModePlan},
			{name: ModeAcceptEdits}, {name: ModeDontAsk}, {name: ModeBypassPermissions},
			{name: "auto", refuse: "gnosh has no auto mode (it is Claude Code's, where a second model reviews each action)"},
		},
		set: func(i *Invocation, v flagValue) { i.PermissionMode = v.text }})
	allowed := t.add(&flag{names: "--allowedTools --allowed-tools", kind: kindList, list: listRules, arg: "rules...", group: groupPermissions, pri: 1,
		help: `Let these run without asking. A rule is a tool's name, or a name with a pattern in parentheses: Read, "Bash(git` + nbsp + `diff` + nbsp + `*)".`,
		set:  func(i *Invocation, v flagValue) { i.AllowedTools = append(i.AllowedTools, v.items...) }})
	disallowed := t.add(&flag{names: "--disallowedTools --disallowed-tools", kind: kindList, list: listRules, arg: "rules...", group: groupPermissions, pri: 1,
		help: "Refuse these. A bare tool name takes the tool away from the model; a rule with a pattern refuses only the calls it matches.",
		set:  func(i *Invocation, v flagValue) { i.DisallowedTools = append(i.DisallowedTools, v.items...) }})
	tools := t.add(&flag{names: "--tools", kind: kindList, list: listRules, arg: "names...", group: groupPermissions, pri: 1, emptyOK: true,
		help: `Give the model only these built-in tools. "" gives it none, and default gives it all of them.`,
		set: func(i *Invocation, v flagValue) {
			i.ToolsGiven = true
			i.Tools = append(i.Tools, v.items...)
		}})
	addDir := t.add(&flag{names: "--add-dir", kind: kindList, list: listPaths, arg: "dirs...", group: groupPermissions, pri: 1,
		help: "Let the agent read files in these other folders without asking. Each must exist.",
		set:  func(i *Invocation, v flagValue) { i.AddDirs = append(i.AddDirs, v.items...) }})
	trustFolder := t.add(&flag{names: "--trust-folder", group: groupPermissions, pri: 1,
		help: "Use this folder's own settings for this one run, without recording the folder as trusted.",
		set:  func(i *Invocation, _ flagValue) { i.TrustFolder = true }})

	system := t.add(&flag{names: "--system-prompt", kind: kindText, arg: "text", group: groupTold, pri: 1, freeText: true,
		help: "Use this text in place of gnosh's own instructions.",
		set:  func(i *Invocation, v flagValue) { i.SystemPrompt = v.text }})
	systemFile := t.add(&flag{names: "--system-prompt-file", kind: kindText, arg: "path", group: groupTold, pri: 1,
		help: "The same, with the text read from a file. Given together with --system-prompt, the file's text comes first.",
		set:  func(i *Invocation, v flagValue) { i.SystemPromptFile = v.text }})
	appendSystem := t.add(&flag{names: "--append-system-prompt", kind: kindText, arg: "text", group: groupTold, pri: 1, freeText: true,
		help: "Add this text to the end of the instructions.",
		set:  func(i *Invocation, v flagValue) { i.AppendSystemPrompt = v.text }})
	appendSystemFile := t.add(&flag{names: "--append-system-prompt-file", kind: kindText, arg: "path", group: groupTold, pri: 1,
		help: "The same, with the text read from a file. Given together with --append-system-prompt, the file's text comes first.",
		set:  func(i *Invocation, v flagValue) { i.AppendSystemPromptFile = v.text }})

	cwd := t.add(&flag{names: "--cwd", kind: kindText, arg: "dir", group: groupSettings, pri: 0,
		help: "Work in this folder instead of the current one.",
		set:  func(i *Invocation, v flagValue) { i.Cwd = v.text }})
	dataDir := t.add(&flag{names: "-D --data-dir", kind: kindText, arg: "dir", group: groupSettings, pri: 0,
		help: "Keep sessions and data in this folder instead of the project's own. Commands that read them look there too.",
		set:  func(i *Invocation, v flagValue) { i.DataDir = v.text }})
	settings := t.add(&flag{names: "--settings", kind: kindText, arg: "file-or-json", group: groupSettings, pri: 0,
		help: "Extra settings for this run: a file, or the JSON itself. They win over the settings files for the keys they name.",
		set:  func(i *Invocation, v flagValue) { i.Settings = v.text }})
	settingSrc := t.add(&flag{names: "--setting-sources", kind: kindText, arg: "list", group: groupSettings, pri: 1,
		help: "Which settings files to read: any of user, project and local, with commas between.",
		check: func(v string) string {
			_, problem := settingSources(v)
			return problem
		},
		set: func(i *Invocation, v flagValue) { i.SettingSources, _ = settingSources(v.text) }})
	bare := t.add(&flag{names: "--bare", group: groupSettings, pri: 1,
		help: "Start with the least: no hooks, skills, custom commands, sub-agents, MCP servers or context files, except what other flags name.",
		set:  func(i *Invocation, _ flagValue) { i.Bare = true }})
	safeMode := t.add(&flag{names: "--safe-mode", group: groupSettings, pri: 1,
		help: "Start with your customizations off (hooks, MCP servers, context files, skills on disk, custom commands), to fix a broken one. Providers, the model, built-in tools and permissions stay as they are.",
		set:  func(i *Invocation, _ flagValue) { i.SafeMode = true }})
	noSlash := t.add(&flag{names: "--disable-slash-commands", group: groupSettings, pri: 1,
		help: "Turn off skills and slash commands for this session.",
		set:  func(i *Invocation, _ flagValue) { i.DisableSlashCommands = true }})
	mcpConfig := t.add(&flag{names: "--mcp-config", kind: kindList, list: listPaths, arg: "files-or-json...", group: groupSettings, pri: 2,
		help: "Load MCP servers from these files, or from the JSON itself.",
		set:  func(i *Invocation, v flagValue) { i.MCPConfig = append(i.MCPConfig, v.items...) }})
	strictMCP := t.add(&flag{names: "--strict-mcp-config", group: groupSettings, pri: 2,
		help: "Use only the MCP servers that --mcp-config names.",
		set:  func(i *Invocation, _ flagValue) { i.StrictMCPConfig = true }})
	agents := t.add(&flag{names: "--agents", kind: kindText, arg: "json", group: groupSettings, pri: 1,
		help: "Define sub-agents for this session, as a JSON object (in print mode, a file holding one will do).",
		set:  func(i *Invocation, v flagValue) { i.Agents = v.text }})
	debug := t.add(&flag{names: "-d --debug", kind: kindAttached, arg: "filter", group: groupSettings, pri: 1,
		help: "Write more to the log. A filter keeps only some kinds, and must be attached with an equals sign: --debug=hooks,mcp.",
		set: func(i *Invocation, v flagValue) {
			i.Debug = true
			i.DebugFilter = v.text
		}})
	debugFile := t.add(&flag{names: "--debug-file", kind: kindText, arg: "path", group: groupSettings, pri: 1,
		help: "Write the debug log to this file. Turns --debug on.",
		set: func(i *Invocation, v flagValue) {
			i.Debug = true
			i.DebugFile = v.text
		}})
	verbose := t.add(&flag{names: "--verbose", printLetter: 'v', group: groupSettings, pri: 1,
		help: "Show each turn in full. In print mode: the log, and one line per finished tool call, on stderr. After run or with -p, -v means this.",
		set:  func(i *Invocation, _ flagValue) { i.Verbose = true }})

	printMode := t.add(&flag{names: "-p --print", group: groupPrint, pri: 0, endsCommands: true,
		help: `Answer one prompt and exit, without the interface. The same as "gnosh run". Every word after it that is not a flag is the prompt.`})
	output := t.add(&flag{names: "--output-format", kind: kindText, arg: "format", group: groupPrint, pri: 0, printOnly: true,
		help:    "What to print: text (the answer, and the default), json (one result object at the end) or stream-json (a JSON line per message, then the result).",
		choices: []choice{{name: FormatText}, {name: FormatJSON, pri: 1}, {name: FormatStreamJSON, pri: 1}},
		set:     func(i *Invocation, v flagValue) { i.OutputFormat = v.text }})
	input := t.add(&flag{names: "--input-format", kind: kindText, arg: "format", group: groupPrint, pri: 0, printOnly: true,
		help:    "What standard input carries: text (the default) or stream-json (a JSON user message per line).",
		choices: []choice{{name: FormatText}, {name: FormatStreamJSON, pri: 1}},
		set:     func(i *Invocation, v flagValue) { i.InputFormat = v.text }})
	partial := t.add(&flag{names: "--include-partial-messages", group: groupPrint, pri: 1, printOnly: true,
		help: "Print the answer piece by piece as it is written. Needs --output-format stream-json.",
		set:  func(i *Invocation, _ flagValue) { i.IncludePartialMessages = true }})
	replay := t.add(&flag{names: "--replay-user-messages", group: groupPrint, pri: 1, printOnly: true,
		help: "Print back each user message read from standard input. Needs stream-json for both input and output.",
		set:  func(i *Invocation, _ flagValue) { i.ReplayUserMessages = true }})
	quiet := t.add(&flag{names: "-q --quiet", group: groupPrint, pri: 0, printOnly: true,
		help: "No progress indicator on stderr.",
		set:  func(i *Invocation, _ flagValue) { i.Quiet = true }})
	goal := t.add(&flag{names: "--goal", kind: kindText, arg: "condition", group: groupPrint, pri: 1, printOnly: true, freeText: true,
		help: "Keep working until this condition holds, as /goal does in a session. The run exits non-zero while the goal is still unmet.",
		set:  func(i *Invocation, v flagValue) { i.Goal = v.text }})
	maxTurns := t.add(&flag{names: "--max-turns", kind: kindText, arg: "n", group: groupPrint, pri: 0, printOnly: true, number: true, min: 1,
		help: "Stop after this many turns of the model. There is no limit unless this is given.",
		set:  func(i *Invocation, v flagValue) { i.MaxTurns = v.n }})
	smallModel := t.add(&flag{names: "--small-model", kind: kindText, arg: "model", group: groupPrint, pri: 1, printOnly: true,
		help: "The model for the run's short side jobs. Without it they use the main model.",
		set:  func(i *Invocation, v flagValue) { i.SmallModel = v.text }})
	noPersist := t.add(&flag{names: "--no-session-persistence", group: groupPrint, pri: 1, printOnly: true,
		help: "Do not save this session. It cannot be resumed.",
		set:  func(i *Invocation, _ flagValue) { i.NoSessionPersistence = true }})
	prompts := t.add(&flag{names: "--permission-prompts", kind: kindText, arg: "who", group: groupPrint, pri: 1, printOnly: true,
		help:    "Who answers permission questions: host (the program that started gnosh) or none (whatever would ask is refused).",
		choices: []choice{{name: "host"}, {name: "none"}},
		set:     func(i *Invocation, v flagValue) { i.PermissionPrompts = v.text }})

	help := t.add(&flag{names: "-h --help", group: groupOther, pri: 0,
		help: "Show help: gnosh's, or after a command's name that command's."})
	version := t.add(&flag{names: "-v --version", group: groupOther, pri: 0,
		help: "Print which build of gnosh this is: when it was built, and from which commit. (After run or with -p, -v means --verbose.)"})

	sessionFlags := []*flag{
		cont, resume, session, sessionID, fork, name,
		model, effort,
		yolo, allowSkip, mode, allowed, disallowed, tools, addDir, trustFolder,
		system, systemFile, appendSystem, appendSystemFile,
		cwd, dataDir, settings, settingSrc, bare, safeMode, noSlash, mcpConfig, strictMCP, agents, debug, debugFile, verbose,
		printMode, output, input, partial, replay, quiet, goal, maxTurns, smallModel, noPersist, prompts,
		help, version,
	}

	// ------------------------------------------------------------------
	// The flags of single commands.
	// ------------------------------------------------------------------

	trustList := t.add(&flag{names: "--list", id: "trust --list", pri: 1,
		help: "List the trusted folders.",
		set:  func(i *Invocation, _ flagValue) { i.List = true }})
	trustForget := t.add(&flag{names: "--forget", id: "trust --forget", pri: 1,
		help: "Forget the folder instead of trusting it.",
		set:  func(i *Invocation, _ flagValue) { i.Forget = true }})
	sessionJSON := t.add(&flag{names: "--json", id: "session --json", pri: 1,
		help: "Print JSON instead of text.",
		set:  func(i *Invocation, _ flagValue) { i.JSON = true }})
	webPort := t.add(&flag{names: "--port", id: "session web --port", kind: kindText, arg: "n", pri: 2, number: true, min: 1, max: 65535,
		help: "Serve the page on this port.",
		set:  func(i *Invocation, v flagValue) { i.Port = v.n }})
	exportForce := t.add(&flag{names: "--force", id: "prompt export --force", pri: 1,
		help: "Write the file even if one is already there.",
		set:  func(i *Invocation, _ flagValue) { i.Force = true }})
	toolsJSON := t.add(&flag{names: "--json", id: "prompt tools --json", pri: 1,
		help: "Print the exact objects the model is sent.",
		set:  func(i *Invocation, _ flagValue) { i.JSON = true }})
	toolsExport := t.add(&flag{names: "--export", id: "prompt tools --export", pri: 1,
		help: "Write each tool's description to a file you can edit, with a list for turning tools off.",
		set:  func(i *Invocation, _ flagValue) { i.Export = true }})
	logsFollow := t.add(&flag{names: "-f --follow", id: "logs --follow", pri: 1,
		help: "Keep printing as the log grows.",
		set:  func(i *Invocation, _ flagValue) { i.Follow = true }})
	logsTail := t.add(&flag{names: "-t --tail", id: "logs --tail", kind: kindText, arg: "n", pri: 1, number: true, min: 1,
		help: "Print the last n lines: 1000 unless this is given.",
		set:  func(i *Invocation, v flagValue) { i.Tail = v.n }})
	projectsJSON := t.add(&flag{names: "--json", id: "projects --json", pri: 2,
		help: "Print JSON instead of text.",
		set:  func(i *Invocation, _ flagValue) { i.JSON = true }})
	statsAll := t.add(&flag{names: "--all", id: "stats --all", pri: 2,
		help: "Count every project, not only this one.",
		set:  func(i *Invocation, _ flagValue) { i.All = true }})
	statsCrawl := t.add(&flag{names: "--crawl-dir", id: "stats --crawl-dir", kind: kindText, arg: "dir", pri: 2,
		help: "Look for sessions under this folder.",
		set:  func(i *Invocation, v flagValue) { i.CrawlDir = v.text }})
	mcpScope := t.add(&flag{names: "-s --scope", id: "mcp --scope", kind: kindText, arg: "scope", pri: 1,
		help:    "Where the server is recorded: local (this project, on this machine; the default), user or project.",
		choices: []choice{{name: "local"}, {name: "user"}, {name: "project"}},
		set:     func(i *Invocation, v flagValue) { i.MCPScope = v.text }})
	mcpTransport := t.add(&flag{names: "-t --transport", id: "mcp add --transport", kind: kindText, arg: "transport", pri: 1,
		help: "How the server is reached: stdio (a command gnosh starts; the default), sse or http.",
		choices: []choice{{name: "stdio"}, {name: "sse"}, {name: "http"},
			{name: "ws", refuse: "a ws server cannot be added this way; describe it in JSON and use gnosh mcp add-json"}},
		set: func(i *Invocation, v flagValue) { i.MCPTransport = v.text }})
	mcpEnv := t.add(&flag{names: "-e --env", id: "mcp add --env", kind: kindList, list: listPairs, arg: "KEY=value...", pri: 1,
		help:  "Environment variables for the server's command.",
		check: isEnvPair,
		set:   func(i *Invocation, v flagValue) { i.MCPEnv = append(i.MCPEnv, v.items...) }})
	mcpHeader := t.add(&flag{names: "-H --header", id: "mcp add --header", kind: kindList, list: listHeaders, arg: "headers...", pri: 1,
		help:  `Headers to send to a remote server, each written "Name: value".`,
		check: isHeader,
		set:   func(i *Invocation, v flagValue) { i.MCPHeaders = append(i.MCPHeaders, v.items...) }})

	// ------------------------------------------------------------------
	// The commands.
	// ------------------------------------------------------------------

	root := t.command(nil, &command{name: "gnosh", cmd: CmdInteractive, session: true, maxArgs: -1,
		argsUsage: "[prompt]", flags: sessionFlags, take: takePrompt, finish: finishSession,
		dashHint: promptAfterDashes,
		summary:  "gnosh: an AI harness that thinks in Gno."})
	t.root = root

	t.command(root, &command{name: "run", aliases: []string{"r"}, cmd: CmdPrint, session: true, print: true, maxArgs: -1,
		argsUsage: "[prompt...]", flags: sessionFlags, take: takePrompt, finish: finishSession, pri: 0,
		dashHint: promptAfterDashes,
		summary:  "Answer one prompt and exit; the same as -p.",
		about:    "Print mode: no interface, one answer, then exit. The words after run that are not flags are the prompt, and text piped in goes before it. Without -y, a permission mode or allow rules, whatever would have asked for permission is refused."})

	t.command(root, &command{name: "doctor", cmd: CmdDoctor, pri: 1,
		flags:   []*flag{cwd, dataDir, help},
		summary: "Check that a turn could run here.",
		about:   "Reads the config for this folder and prints one line per check. Exits 1 when a turn could not run. Starts no model."})

	t.command(root, &command{name: "trust", cmd: CmdTrust, pri: 1, maxArgs: 1, argsUsage: "[folder]",
		flags: []*flag{trustList, trustForget, cwd, help}, finish: finishTrust,
		summary: "Trust a folder's own settings, list the trusted folders, or forget one.",
		about:   "With no folder, the current one. --list takes no folder."})

	t.command(root, &command{name: "models", cmd: CmdModels, pri: 1,
		flags:   []*flag{cwd, help},
		summary: "List the models each provider offers.",
		about:   "Also names the providers that were left out, and why. It never loads a model."})

	sess := t.command(root, &command{name: "session", aliases: []string{"sessions", "s"}, needSub: true, pri: 1,
		argsUsage: "<command>", flags: []*flag{help},
		summary: "Look at and manage sessions.",
		about:   "These start no model."})
	sessionCommon := []*flag{sessionJSON, cwd, dataDir, help}
	t.command(sess, &command{name: "list", aliases: []string{"ls"}, cmd: CmdSessionList, pri: 1,
		flags: sessionCommon, summary: "List this project's sessions."})
	t.command(sess, &command{name: "show", cmd: CmdSessionShow, pri: 1, minArgs: 1, maxArgs: 1, argsUsage: "<id>",
		flags: sessionCommon, summary: "Print one session."})
	t.command(sess, &command{name: "last", cmd: CmdSessionLast, pri: 1,
		flags: sessionCommon, summary: "Print the most recent session."})
	t.command(sess, &command{name: "delete", aliases: []string{"rm"}, cmd: CmdSessionDelete, pri: 1, minArgs: 1, maxArgs: 1, argsUsage: "<id>",
		flags: sessionCommon, summary: "Delete a session."})
	t.command(sess, &command{name: "rename", cmd: CmdSessionRename, pri: 1, minArgs: 2, maxArgs: -1, argsUsage: "<id> <title...>",
		flags: sessionCommon, take: takeRename,
		summary: "Give a session a new title.",
		about:   "The words after the ID are the title."})
	t.command(sess, &command{name: "web", cmd: CmdSessionWeb, pri: 2,
		flags:   []*flag{webPort, cwd, dataDir, help},
		summary: "Serve a page on this machine for reading a session's messages."})

	t.command(root, &command{name: "mem", cmd: CmdMem, pri: 1, maxArgs: -1,
		argsUsage: "[show [name...] | find <words> | log [n]]", argsShort: "[show|find|log]",
		passes: true, dashHint: "a word for mem that begins with a dash goes after --",
		flags:   []*flag{cwd, dataDir, help},
		summary: "Read the project's mem, without a model.",
		about:   "The words after mem go to mem as they are: show, find and log read it. What changes mem is done with /mem inside a session."})

	prm := t.command(root, &command{name: "prompt", needSub: true, pri: 1,
		argsUsage: "<command>", flags: []*flag{help},
		summary: "See what the model is sent, and where to change it.",
		about:   "These start no session."})
	t.command(prm, &command{name: "show", cmd: CmdPromptShow, pri: 1, maxArgs: 1, argsUsage: "[name]",
		flags: []*flag{cwd, help}, summary: "Print the instructions in effect, and where they came from."})
	t.command(prm, &command{name: "path", cmd: CmdPromptPath, pri: 1,
		flags: []*flag{cwd, help}, summary: "Print the folder where your own versions of the instructions go."})
	t.command(prm, &command{name: "export", cmd: CmdPromptExport, pri: 1, maxArgs: 1, argsUsage: "[name]",
		flags: []*flag{exportForce, cwd, help}, summary: "Write the built-in instructions to that folder, as a file to edit."})
	t.command(prm, &command{name: "tools", cmd: CmdPromptTools, pri: 1,
		flags: []*flag{toolsJSON, toolsExport, cwd, help}, summary: "Print what the model is told about each tool, with sizes."})
	t.command(prm, &command{name: "context", cmd: CmdPromptContext, pri: 1,
		flags: []*flag{session, cont, cwd, dataDir, help}, finish: finishContext,
		flagHelp: map[string]string{
			idSession:  "Show what fills the context window of the session with this ID.",
			idContinue: "Show it for the most recent session of this project.",
		},
		summary: "Print what fills the context window, by category.",
		about:   "With neither -s nor -C, what a new session would start with."})

	t.command(root, &command{name: "logs", cmd: CmdLogs, pri: 1,
		flags:   []*flag{logsFollow, logsTail, cwd, dataDir, help},
		summary: "Print the end of the project's log, or follow it."})

	t.command(root, &command{name: "dirs", cmd: CmdDirs, pri: 1,
		flags:   []*flag{cwd, dataDir, help},
		summary: "Say which config and data folders are in use."})

	t.command(root, &command{name: "ssh", cmd: CmdSSH, pri: 2, raw: true, maxArgs: -1, argsUsage: "[ssh arguments]",
		flags:   []*flag{help},
		summary: "Run ssh, and put what is copied on the far side onto this computer's clipboard.",
		about:   "Everything after ssh goes to ssh as it is, flags included."})

	t.command(root, &command{name: "projects", cmd: CmdProjects, pri: 2,
		flags:   []*flag{projectsJSON, help},
		summary: "List the project folders gnosh has been used in."})

	t.command(root, &command{name: "stats", cmd: CmdStats, pri: 2,
		flags:   []*flag{statsAll, statsCrawl, cwd, dataDir, help},
		summary: "Print usage totals across sessions."})

	mcp := t.command(root, &command{name: "mcp", needSub: true, pri: 1,
		argsUsage: "<command>", flags: []*flag{help},
		summary: "Manage MCP servers."})
	t.command(mcp, &command{name: "add", cmd: CmdMCPAdd, pri: 1, minArgs: 2, maxArgs: -1,
		argsUsage: "<name> <command-or-url> [arguments...]", argsShort: "<name> <command>",
		passes: true, dashHint: "the server's own flags go after --, with its command",
		flags:   []*flag{mcpScope, mcpTransport, mcpEnv, mcpHeader, cwd, help},
		summary: "Add a server: a name, then the command that starts it, or its URL.",
		about:   "Put -- before the server's command when the command has flags of its own:\ngnosh mcp add files -- npx server --port 8080"})
	t.command(mcp, &command{name: "add-json", cmd: CmdMCPAddJSON, pri: 1, minArgs: 2, maxArgs: 2, argsUsage: "<name> <json>",
		flags: []*flag{mcpScope, cwd, help}, summary: "Add a server from its description in JSON."})
	t.command(mcp, &command{name: "get", cmd: CmdMCPGet, pri: 1, minArgs: 1, maxArgs: 1, argsUsage: "<name>",
		flags: []*flag{cwd, help}, summary: "Show one server."})
	t.command(mcp, &command{name: "list", cmd: CmdMCPList, pri: 1,
		flags: []*flag{cwd, help}, summary: "List the servers."})
	t.command(mcp, &command{name: "remove", cmd: CmdMCPRemove, pri: 1, minArgs: 1, maxArgs: 1, argsUsage: "<name>",
		flags: []*flag{mcpScope, cwd, help}, summary: "Remove a server."})

	t.command(root, &command{name: "help", cmd: CmdHelp, pri: 0, maxArgs: -1, argsUsage: "[command]",
		flags: []*flag{help}, finish: finishHelp, afterAnything: true,
		summary: "Show gnosh's help, or a command's."})
	t.command(root, &command{name: "version", cmd: CmdVersion, pri: 0,
		flags: []*flag{help}, afterAnything: true,
		summary: "Print which build of gnosh this is."})

	for _, c := range t.commands {
		c.index()
	}
	return t
}

// promptAfterDashes is how the session commands say to give a prompt that
// begins with a dash.
const promptAfterDashes = "a prompt that begins with a dash goes after --"

// wordAfterC refuses a single word right after -c. In the earlier gnosh -c
// took the working folder, and scripts still pass `-c DIR`; read the new
// way, that would continue some session, in whatever folder the script was
// started from, with DIR as its prompt. So the one shape that could be the
// old use is refused, and the message gives both ways to say what was
// meant. A prompt of several words after -c is not that shape and is left
// alone, as is anything after -C or --continue, which never took a folder.
func wordAfterC(typed, next string) string {
	if typed != "-c" || next == "" || hasSpace(next) {
		return ""
	}
	return fmt.Sprintf(
		"-c means --continue now. It used to take the working folder, so a single word right after it is refused rather than sent as the prompt, and nothing was started. For the folder, write --cwd %s. For a one-word prompt, write -C %s, or put -- before it.",
		shown(next), shown(next))
}

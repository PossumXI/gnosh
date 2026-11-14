// Package config reads gnosh's configuration and checks it.
//
// The rule it is built on: a mistake in the configuration stops gnosh at
// startup and says plainly what is wrong, where, and what to do about it.
// Nothing is ever accepted and then quietly left unused. A word gnosh does
// not know, a name it cannot match, a value it cannot use, a setting for a
// part of gnosh that is not built: each is a Problem with the file and the
// line, and Load returns all of them at once.
//
// # What is read, weakest first
//
//  1. gnosh's built-in defaults.
//  2. The user's own files in the config directory: gnosh.json, then
//     gnoshrc.
//  3. The state file: what was chosen inside gnosh with /model, /think and
//     /autocompact. It outranks the user's gnoshrc, so a choice sticks.
//  4. The project's files, in the working directory: gnosh.json, then
//     .gnoshrc (or gnoshrc; the dotted name wins), then the
//     settings.local.json in the project's data folder, where approvals a
//     person saves are written. They are in force only in a trusted folder.
//  5. Environment variables.
//  6. The command line (--settings, then the single flags), which holds for
//     the one run and is never saved.
//
// Within one file, later lines win. Lists (permission rules, hooks,
// directories, list options) add up across files instead of replacing each
// other. Every value in the Config carries its Origin: the layer, and the
// file and line or the flag or variable that set it.
//
// # Where things live
//
// The user's config directory is $GNOSH_GLOBAL_CONFIG, else
// $XDG_CONFIG_HOME/gnosh, else ~/.config/gnosh. The user's state directory
// (the state file, the trust record) is $GNOSH_GLOBAL_DATA, else
// $XDG_DATA_HOME/gnosh, else ~/.local/share/gnosh; no config file is ever
// read from it. A project's data folder is .gnosh: the closest one on the
// way up to the project root, or a new one at the root, or the directory
// given with -D. Paths has them all.
//
// Three names from the earlier gnosh are still read, because Jae's own
// files still have them, and for no other reason: the table oldNames in
// table.go lists them, and is deleted when his files have moved.
//
// # gnoshrc
//
// gnoshrc is a list of settings, one to a line, written as commands. It is
// not a script: there is no shell behind it.
//
//	file       = { line }
//	line       = [ directive ] [ "#" comment ] newline
//	directive  = word { word }
//	word       = part { part }
//	part       = bare | 'single-quoted' | "double-quoted"
//
// The rules that go with the grammar:
//
//   - One directive to a line. A backslash at the end of a line joins the
//     next line to it (not inside a comment, and not inside single quotes).
//   - "#" starts a comment where a word would begin. Blank lines are
//     skipped.
//   - Single quotes keep everything as written. Double quotes keep spaces,
//     and a backslash in them keeps a double quote, a backslash, a dollar
//     sign or a backquote. A quote closes on the line it opens on. Parts
//     written against each other make one word: --base-url "http://"$HOST.
//   - In a bare or double-quoted part, $NAME and ${NAME} are replaced by
//     the environment variable, or by nothing when it is not set (with a
//     warning). ${NAME:?message} stops startup with the message when the
//     variable is not set or empty. A value is never split into words, and
//     an empty one stays a word.
//   - The variables gnosh itself sets for the hooks and commands it runs
//     ($CLAUDE_PROJECT_DIR, $GNOSH_TOOL_NAME and the rest) are not filled
//     in here, since their value belongs to one run of a hook. Outside
//     single quotes they are a mistake; inside, they reach the command as
//     written.
//   - A leading ~ or ~/ in a value that is a path is the home directory.
//   - The first word is the directive: provider, model, option, host, hook,
//     permissions, mcp, lsp.
//   - Things named by an ID (a provider, a model, a host, an MCP or
//     language server, a named hook) take "add" to create or update and
//     "remove" (or "rm") to delete. On an update, what the line gives
//     replaces what was there and what it leaves out stays.
//   - A flag is written --name value, or --name=value. A value that begins
//     with two dashes goes in quotes. A flag that takes a truth value
//     accepts true, false, 1, 0, yes or no in any letter case, and the
//     value may be left out, which means true. Some flags may be given
//     more than once; some take two words, a key and a value.
//   - Anything of the shell's is a mistake, with a message that says so and
//     names what gnoshrc offers instead: a variable assignment, if, for,
//     case, source, a semicolon, &&, a pipe, a redirection, $(...),
//     backquotes, and parentheses outside quotes.
//
// The directives:
//
//	provider add <id> --type <type> [--base-url <url>]... [--name <text>]
//	             [--api-key <key>] [--discover-models <truth>]
//	             [--extra-body <json>] [--extra-header <name> <value>]...
//	             [--disable <truth>]
//	provider remove <id>
//	model add <provider>/<id> [--context-window <tokens>] [--name <text>]
//	             [--default-max-tokens <tokens>] [--reasoning-levels <list>]
//	             [--can-reason <truth>] [--supports-images <truth>]
//	model remove <provider>/<id>
//	model large [<provider>/<id>] [--think <truth>] [--reasoning-effort <level>]
//	             [--temperature <number>]
//	model small [<provider>/<id>] (the same flags)
//	option <key> [value]
//	option reset <key>
//	host add <name> --ssh <destination> [--description <text>]
//	host remove <name>
//	hook add <event> --command <command> [--name <name>] [--matcher <matcher>]
//	             [--timeout <seconds>]
//	hook remove <event> [--name <name>]
//	permissions allow <rule>...
//	permissions deny <rule>...
//	mcp add <name> [--type stdio|sse|http] [--command <program>] [--args <word>]...
//	             [--env <name> <value>]... [--url <url>] [--header <name> <value>]...
//	             [--timeout <seconds>] [--disabled <truth>]
//	             [--enabled-tools <list>] [--disabled-tools <list>]
//	mcp remove <name>
//	lsp add <name> --command <program> [--args <word>]... [--env <name> <value>]...
//	             [--filetypes <list>] [--root-markers <list>] [--timeout <seconds>]
//	             [--disabled <truth>] [--init-options <json>] [--options <json>]
//	lsp remove <name>
//
// Table lists every directive, flag, option, settings key and variable
// with what gnosh does about it today: it works, it is read and kept for
// later, or it stops startup because the part it is for is not built.
//
// An example file:
//
//	# the model server on this machine, and by its LAN address from elsewhere
//	provider add qwen --name "Qwen (ai box)" --type llamacpp \
//	    --base-url http://192.0.2.91:8080 --base-url http://127.0.0.1:18080
//	model add qwen/qwen3.8-27b --context-window 262144
//	model large qwen/qwen3.8-27b
//
//	# a hosted provider; the key comes from the environment
//	provider add claude --type anthropic --api-key "${ANTHROPIC_API_KEY:?export it first}"
//
//	option request-timeout 900      # seconds a reply may go quiet
//	option notifications disabled
//
//	host add render --ssh root@203.0.113.10 --description "the render box"
//	hook add Stop --name tests --command "./hooks/tests-must-pass.sh" --timeout 1800
//	permissions allow Read 'Bash(git diff *)'
//	permissions deny 'Bash(git push *)'
//
// # gnosh.json
//
// Beside a gnoshrc, gnosh.json holds settings in Claude Code's shape, so
// that a block copied from a Claude Code settings file works: permissions
// (allow, ask, deny, defaultMode, additionalDirectories and a few
// switches), hooks, env and disableAllHooks. It is strict JSON. Any other
// key is a mistake that names the key and the file, a key given twice is a
// mistake, and so is a value of the wrong shape. Everything that is
// gnosh's own is set in gnoshrc. --settings gives one more such file, or
// the JSON itself, for one run.
//
// # A folder that is not trusted
//
// A project's files can add a hook, start a server, let tools run without
// asking, or point a provider somewhere else. So until the folder is
// trusted (Inputs.Trusted) they are read by the same reader, checked in
// full, and then held: only settings of a kind that Inputs.KeepUntrusted
// accepts are put in force, which with the trust package's rule means deny
// rules, ask rules and switched-off skills. Config.Held lists everything
// that waits, for the trust question and for doctor. What is held is also
// taken out again after the last merge, so that no later step can put it
// back. A mistake in a project's file stops startup whether or not the
// folder is trusted: a deny rule in a file that could not be read would
// not be in force.
//
// # Hosts
//
// A host is another machine the model may run commands on: a name, the
// destination ssh is given, and what the person wrote about it. The
// declarations are checked as plain values, once every file has been read
// (checkHosts), so that the same mistakes are found whichever file, in
// whichever format, made the declaration: a name that is this machine's
// own ("local"), a name declared twice, a destination that is missing or
// is not user@address or one word ssh's own configuration can know. Each
// is a Problem that names the host and the place that declared it, and
// like every other mistake it stops startup.
//
// # Secrets
//
// A key, a header value, a variable's value and a password inside an
// address are kept as Secret, which prints as "[hidden]" with every verb,
// in logs and in JSON. No Problem and no line of Config.String or Doctor
// holds one.
//
// # What a caller does with the Config
//
// The Config is plain data. Each part of gnosh is handed what it needs:
// the provider client gets a Provider's Wire, an Endpoint's URL and
// Timeouts, the key, the headers and ExtraBody; the permission package gets
// Permissions; the hook runner gets Hooks; the shell tool gets CommandEnv,
// Limits and Hosts; the package that reads instruction files gets Context.
// Load reads none of those files and starts nothing.
package config

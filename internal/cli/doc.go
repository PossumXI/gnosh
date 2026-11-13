// Package cli is gnosh's command line: the table of its commands and flags,
// the parser that reads a line against that table, the checks that turn what
// was typed into a plain description of what to do, and the help text.
//
// It reads nothing but its arguments. No file is opened, no directory is
// looked at, no environment variable is read, no tool name is checked: the
// result, an Invocation, is plain data for the rest of the program to act
// on. The two facts about the process that the rules depend on (whether
// standard input and standard output are terminals) come in through Env.
//
//	table := cli.Default() // and SetStatus for what has been built since
//	inv, err := table.Parse(os.Args[1:], cli.Env{StdinIsTerminal: ..., StdoutIsTerminal: ...})
//	if err != nil {
//		fmt.Fprintln(os.Stderr, "gnosh:", err) // the message is ready to show
//		os.Exit(cli.ExitFailure)
//	}
//	switch inv.Command {
//	case cli.CmdHelp:
//		text, _ := table.Help(inv.HelpFor...)
//	case cli.CmdVersion:
//		fmt.Println(cli.VersionLine(build))
//	case cli.CmdInteractive, cli.CmdPrint:
//		...
//	}
//
// # What a line is made of
//
// The line is read left to right. Each argument is one of five things:
//
//   - Everything after a lone "--" is an argument of the command (for a
//     session, part of the prompt), whatever it looks like. Nothing after
//     "--" is ever read as a flag or as a command's name.
//   - "--name" or "--name=value" is a flag spelled as a word. Only whole
//     names count: gnosh does not accept the start of a name, and letter
//     case matters.
//   - "-x" is a one-letter flag, and "-xyz" is three of them sharing a dash.
//   - The first plain word names a command if gnosh has one by that name:
//     `gnosh doctor`, `gnosh session list`.
//   - Anything else is an argument. For `gnosh` and `gnosh run` the
//     arguments are the prompt: several are joined by single spaces, and
//     the text is otherwise passed on untouched, newlines and all.
//
// Flags and arguments mix freely, so these are the same:
//
//	gnosh -p "summarize this" --model qwen
//	gnosh --model qwen -p "summarize this"
//
// # Values
//
// A flag that takes a value takes it from the next argument, or from its own
// argument after an equals sign:
//
//	--model qwen        -m qwen        --model=qwen
//
// A value may not begin with a dash unless it is attached with the equals
// sign (`--name=-draft`), because a flag where a value should be is almost
// always a forgotten value. The flags whose value is free text
// (--system-prompt, --append-system-prompt, --goal) do take a next argument
// that begins with a dash, unless it is one of gnosh's own flags. An empty
// value is refused everywhere except `--tools ""`, which means "no tools":
// an empty value is nearly always a shell variable that was never set.
//
// Two flags take a value only sometimes:
//
//   - -r, --resume takes the next argument as the session unless that
//     argument begins with a dash. So `gnosh -r` shows the list, and in
//     `gnosh -r "fix the bug"` the words are a session's name, not a prompt.
//   - -d, --debug takes its filter only attached: `--debug=hooks,mcp`. In
//     `gnosh --debug hooks`, "hooks" is the prompt.
//
// One-letter flags that share a dash are read one by one, and one that takes
// a value must be the last: `-ys ID` is -y and then -s ID; `-sy ID` is
// refused. A one-letter flag never takes its value attached (`-mqwen`,
// `-m=qwen`); the message says how to write it.
//
// # List flags
//
// --allowedTools, --disallowedTools, --tools, --add-dir and --mcp-config
// take one or more values. The first argument after the flag is always
// taken. Each argument after that is taken too, up to the next flag, for as
// long as it has no space in it; for tools and rules, a space inside
// parentheses does not count. So:
//
//	--add-dir ../apps ../lib                      two folders
//	--allowedTools "Bash(git log *)" Read          two rules
//	--allowedTools "Bash(go test *)" "run the tests and report"
//	                                               one rule, then the prompt
//
// A prompt of two or more words therefore ends a list. A one-word prompt
// right after a list would be taken as one more item, so put the prompt
// first, or put "--" before it, or attach the list with an equals sign, which
// takes exactly one argument:
//
//	gnosh -p hello --tools Read
//	gnosh -p --tools Read -- hello
//	gnosh -p --tools=Read hello
//
// When that mistake leaves print mode without a prompt, the error names the
// flag that took the word. `--tools ""` takes nothing more after it.
//
// One case is refused instead of read either way: an argument with spaces
// that ends a list of tools or rules and itself reads like a list of them,
// as in `--disallowedTools Bash "Edit Write"`. Taken as the prompt, it would
// leave Edit and Write allowed without a word; the error says how to write
// it as rules and how to write it as a prompt.
//
// Within one argument, a list of tools or rules is cut at commas and
// spaces, but not inside parentheses:
//
//	--allowedTools "Bash(git diff *),Edit"         Bash(git diff *) and Edit
//	--disallowedTools "Bash Edit"                  Bash and Edit
//	--allowedTools "Bash(git log *, git show *)"   one rule
//
// If the parentheses of an argument do not pair up, it is one rule when
// that is the only way to read it, and an error when it could be two.
// Folders and MCP configs are never cut: each argument is one item.
//
// # A flag typed twice
//
// The last one counts, so a wrapper's defaults can be overridden by adding
// to the end of the line. The list flags are the exception: they add up.
//
// # Commands
//
// A first word that is a command's name (or alias) is that command, and its
// flags come after it. Flags typed before the name must be ones the command
// takes too. After -p, no word is a command: `gnosh -p run the tests` sends
// "run the tests". To send a command's name as a prompt in a session, put
// "--" before it: `gnosh -- doctor`.
//
// A first word that is one slip away from a command's name (a letter
// dropped, added, changed, or two swapped), on a line that would make sense
// for that command, is refused with the name it probably meant, instead of
// being sent to a model as a prompt: `gnosh docter`. For the three-letter
// commands only swapped letters count, because too many ordinary words are
// one letter away from "run".
//
// `gnosh mem` passes the words after it to mem unread, and `gnosh ssh`
// passes everything after it to ssh, flags included. `gnosh help [command]`
// and `gnosh version` do what --help and --version do.
//
// # Print mode, and -v
//
// `gnosh run` and `gnosh -p` are the same thing. `gnosh` with its output
// going anywhere but a terminal is print mode too (Invocation.PrintImplied
// says so). Print mode needs a prompt: on the command line, piped in, or
// both. Some flags only do something in print mode and are refused outside
// it, rather than accepted and ignored.
//
// -v is --version on a plain `gnosh`, as in Claude Code, and --verbose after
// `run` or with -p, as the scripts around gnosh use it.
//
// # -c
//
// -c and -C both mean --continue. The earlier gnosh used -c for the working
// folder, which is --cwd now and has no short form. A script that still
// says `-c DIR` would otherwise continue some session, in the folder the
// script happened to start in, with DIR as its prompt. So a single word
// right after -c is refused, with both fixes in the message: --cwd DIR for
// the folder, -C WORD for a one-word prompt. A prompt of several words
// after -c is fine, and so is anything after -C or --continue.
//
// # Built and not built
//
// Every command, flag and fixed value in the table has a Status. Using one
// that is NotBuilt is an error that says so: nothing is ever accepted and
// then ignored. The table ships with what the specs call P0 marked Built.
// The code that wires features together makes its own table and marks what
// it has built:
//
//	t := cli.Default()
//	if err := t.SetStatus("--fork-session", cli.Built); err != nil { ... }
//	inv, err := t.Parse(args, env)
//
// Table.Entries lists every name with its status, and help marks what is not
// built.
//
// # What is left to the caller
//
// Parse describes; it does not do. The code that runs an Invocation still
// has to: read standard input when ReadStdin says there may be some (in
// print mode it goes before the prompt, with a blank line between, and a
// pipe that stays silent must not be waited on for ever); read the files
// the prompt flags name; check that added folders exist and that tool names
// are ones gnosh has; apply the environment's equivalents of flags (safe
// mode); print the help and the version line, with the attribution notice
// where it is owed; and exit with the codes named here.
//
// # Errors
//
// Every error is a *Error. A message about something that was typed begins
// with it, exactly as typed (odd text is put in quotes, never re-spelled);
// every message says that nothing was started and what to type instead.
// Unknown flags and commands come with the nearest real name. The program
// exits with ExitFailure for all of them.
package cli

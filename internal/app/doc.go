// Package app is gnosh as one program: it joins the packages that each do
// one thing (the command line, the config, the trust record, the model
// providers, the turn loop, the tools, the permission rules, the hooks, the
// instruction files, the prompt texts and the session files) into the thing
// a person runs. None of those packages knows another; this one knows them
// all, and nothing else does.
//
// The program's main (main.go at the root of the repository) does two
// things. It hands the GnoVM worker its own entry, before anything else,
// when the process was started as "gnovm-worker". Otherwise it fills in
// Options from the operating system (the arguments, the three streams, the
// environment, the signals) and calls Main. A test fills in Options with a
// temporary home directory and buffers instead, and the program then
// touches nothing else: no global is read behind Options' back.
//
// # How it is put together
//
// A run of the program is a program value (app.go). It parses the command
// line and then does one of four things: prints help or the version;
// checks that a turn could run (doctor.go); trusts, lists or forgets a
// folder (trustcmd.go); or opens a session (session.go). A part of gnosh
// that is built elsewhere may add subcommands, tools and more through the
// seam in feature.go, without a change here.
//
// A session is a live value: the session's file, the rules it runs under,
// its tools, its model and its agent, joined together. Nothing in it is
// global, so the session that /clear starts is simply a second one.
//
// The turn loop (internal/agent) knows nothing of files, rules, hooks or
// screens. It asks for them through four small ports, and each has an
// adapter here, in a file of its own with tests of its own:
//
//   - The log (log.go). The loop's entries are written to the session's
//     file (internal/transcript) as that package's entries, with what the
//     loop needs to read them back. What the model is sent is the
//     transcript's own view of the file, so that a request always begins
//     with the bytes of the one before. When room is needed and the
//     session stands in a frame, the oldest part slides out of the view
//     without a summary.
//   - The gate (gate.go). May this tool call run? The person's PreToolUse
//     hooks first, then the permission rules and the mode, then, when it
//     comes to a question, the PermissionRequest hooks and the person.
//     Where nobody can be asked, the call does not run and the model reads
//     why.
//   - The moments (moments.go). The points of a session at which the
//     person's hooks run and at which gnosh adds its own lines: the
//     session's start, a prompt going out, before a request, after a tool,
//     around a summary, a turn's end, the session's end. The Stop hooks
//     are a turn-end driver: they may send the model back as often as they
//     object, with a checkpoint and no cap.
//   - The notices (notices.go). The end of a command that runs in the
//     background is told to the model without anybody asking.
//
// Beside them: model.go settles which model answers, at which address, and
// how large its window is, asking the server and never guessing between
// two models; tools.go makes the tools; instructions.go reads the
// instruction files and builds the system prompt, which holds nothing that
// changes from session to session; texts.go makes text harmless on its way
// to a model and back; slash.go is the registry of slash commands.
//
// Hosts are the other machines the config declares for commands. The
// config checks the declarations and holds a folder's own back until the
// folder is trusted, like everything else of a folder's that reaches out.
// What is in force goes to three places, and nowhere else:
//
//   - The shell tool (tools.go) is handed the hosts, and this machine's
//     name as results write it. A Bash call that names a host runs there
//     through ssh, and every shell result, a local one too, begins with a
//     line that says where it ran. The person's own command line, typed
//     behind "!", runs through the same tool without the rules that steer
//     a model (shellline.go).
//   - The first prompt's environment block (instructions.go) names the
//     hosts, with what the person wrote about each, and says that a path
//     belongs to the machine it was seen on. The system prompt and the
//     tools say nothing that depends on which hosts there are, so they are
//     the same bytes with and without them.
//   - The gate (gate.go) hands the permission engine the call with its
//     host argument, which is how the engine knows a command for another
//     machine: such a call always asks, whatever the allow rules and the
//     read-only list say, the deny rules are held against the command
//     before it leaves, and with permissions skipped the declaration is
//     the whole gate. A call that names this machine as its host is shown
//     to the engine as the local call it is.
//
// gnosh doctor and /status list the hosts. Doctor logs in to them only
// when it is asked to (gnosh doctor --hosts, /doctor hosts); by itself it
// reaches no other machine.
//
// subagent.go is the Agent tool. A sub-agent is a second session run by
// the same loop, with a context of its own: the tool gives it a session
// file of its own inside the folder of the session the person is in, the
// tools its kind allows, and the features' part in it, and waits for its
// report. Its tool calls go through the person's gate, rules and hooks
// like the session's own. The kinds of sub-agent are definitions, read by
// the package agentdef: gnosh's own two, the person's files in
// ~/.claude/agents, and, in a folder that is trusted, the project's.
//
// Whoever watches a session, a person or a script, is a Frontend
// (frontend.go). It is told everything that happens, the session's task
// list and the facts for a status line among it, and drives the session
// through a handful of calls (Controls, and the rest of them in
// SessionControls: a command of the person's own, moving a running
// command to the background, the prompts of earlier sessions, taking one
// queued prompt back). There are three: the writer of a run's output in
// its three formats (print.go), the plain session that reads lines and
// writes lines (plain.go), and whatever is attached in its place through
// Options.Interactive, which is where the full interface goes
// (internal/ui, attached in main.go). The maker of the full interface
// declines where there is no terminal it could draw on, and the session
// is then the plain one.
//
// # The order at startup
//
// Each step can stop the program, and when it does, nothing after it has
// happened. Nothing is sent to a model, and no session file is made,
// until every step before it has passed.
//
//  1. The worker dispatch, in main: a process started as "gnovm-worker"
//     runs the worker and exits. Nothing below happens in it.
//  2. Each feature's own dispatch (Feature.Boot).
//  3. The command line is parsed against the table of what this build
//     has. A flag or a command that is not built stops here, with a line
//     that says so.
//  4. Help and the version are printed and the program ends.
//  5. The folder is settled: where the project begins, and whether the
//     person has trusted it. Only files are read for this; nothing of the
//     folder's is run, and git is never asked.
//  6. The config is loaded as if the folder were not trusted. That says
//     what of the folder's own settings waits for trust. With something
//     waiting: a recorded trust or --trust-folder puts it in force; a
//     person at a terminal is asked, with what the settings would do in
//     front of them; and where nobody can be asked, the settings stay off
//     and one line on standard error says so. Trusted, the config is
//     loaded again with the folder's settings in force.
//  7. Every check of the config has run by now: its own, the permission
//     rules against the one list of tool names, the hooks against the
//     events this build fires. Every mistake is printed at once, each
//     with its file and line, and any mistake ends the program here.
//  8. The session's file is opened: a new one, or the one -c, -r or -s
//     names, or a copy of it. One session has one writer; a file left
//     unfinished by a kill is mended as it is read.
//  9. The session's log file and crash file; the prompt texts, gnosh's own
//     with the person's replacements; the permission engine; the
//     features' part in the session; the hook runner; the tools; the
//     instruction files; the system prompt.
//  10. The model: the provider's address that answers, the model the
//     config or the command line chose, its context window from the
//     server. A run without a model ends here and says why; a session at
//     a terminal opens all the same, so that /model and /doctor can put
//     it right.
//  11. The agent is made, and the front end is handed the session. The
//     SessionStart hooks run before the first prompt goes out, and the
//     instruction files and the facts about the folder go with it.
//
// # The order at shutdown
//
// A session ends because its work is done, because the person left, or
// because a signal arrived. The first SIGINT or SIGTERM is handed to the
// front end, which stops the turn and says which exit code that is (130,
// 143). A run that prints JSON (json, stream-json) still prints its
// result object then: the replies the model had written, what they cost,
// and that the run was stopped and by which signal. It gives the turn two
// seconds to say how far it had come, and from the moment the signal is
// acted on a second one ends the process at once, result or no result. A
// run that prints text prints nothing more. Whatever the reason, the order
// is the same:
//
//  1. From here on a second signal is not handled: it ends the process
//     at once.
//  2. The turn is stopped: the request to the model, the tools, the hooks
//     and the question that waits, if there is one. A tool that has
//     something to finish gets the stop grace to return its result, and
//     the result is stored.
//  3. Every command the session started is stopped, the ones in the
//     background too, each with everything it started.
//  4. The person's SessionEnd hooks run, with the reason.
//  5. The features are told, in reverse order.
//  6. The session's file is flushed to disk and its lock is let go. An
//     interactive session has printed how to resume it.
//  7. The features are told that the program ends, in reverse order, and
//     the program exits with its code.
//
// When the process is being torn down instead (Main's context ends, which
// is what a kill is to a program that embeds gnosh, and to a test), the
// session's file is closed first, exactly as a kill would leave it, and
// then the same steps run. A session that was killed loses nothing that
// was written: it is opened again with -c, and what was left unfinished is
// closed then.
//
// When gnosh itself hits a fault, what it can say is written to a crash
// file beside the project's sessions, the person is told where that is,
// and the session's file is left as it was. A tool or a hook that crashes
// does not end the session: the model is told, and the turn goes on.
package app

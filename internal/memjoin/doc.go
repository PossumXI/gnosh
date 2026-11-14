// Package memjoin joins mem and frames to the program. It is what makes
// the program gnosh: an agent whose plan lives in mem, in Gno, on disk,
// and whose context is a call stack, where the work of a finished todo
// leaves what the model is sent and only its result stays.
//
// Everything this needs exists elsewhere as a separate piece: the GnoVM
// machines and mem (internal/gnovm), the four tools a model calls
// (internal/gnovm/tools), the session log whose view applies the frame
// rule (internal/transcript), the loop (internal/agent), the words said to
// a model (internal/prompt), and the program around them (internal/app).
// This package is the join between them, and nothing else: it is one
// app.Feature, made by New, registered in the program's features.go. It
// follows spec/gnosh/integration-contract.md, whose section numbers are
// given below as "IC".
//
// # Once for the process
//
// New makes the feature. It names the four tools (gnovm_run, gnovm_new,
// gnovm_list, gnovm_drop: IC 5.1), says that none of them asks before it
// runs (they touch nothing but what the machines keep; deleting a
// machine asks through the tool's own Describe), claims the command
// `gnosh mem` and the flag --goal, and starts nothing. It also says that
// what gnovm_run is given and what it answers is kept out of gnosh's log:
// that tool is the one way mem's text travels, and an erasure, which
// takes a text out of mem and out of the sessions' files, could not take
// it out of a log.
//
// The GnoVM worker dispatch is not here: the program's main does it
// before anything else (IC 3.1).
//
// The process has one client of the GnoVM (IC 3.2). It is made the first
// time a session or `gnosh mem` needs it, on the directory DataDir gives:
// the folder gnovm under gnosh's own data directory, which honours
// XDG_DATA_HOME and lies outside every project (IC 3.3). Making it starts
// nothing: a worker is started by the first call of a tool. Feature
// Shutdown lets it go, which the wiring calls after the last session's
// file is flushed and released. Tests set Options.InProcess, and the
// carried client itself refuses to start a worker under go test (IC 3.6).
//
// # Once for each session
//
// Feature.Session is called when a session's facts are known, and again
// for every sub-agent the session starts. It returns what the join adds
// to that session, each piece through one field of app.Contribution. In
// the order things happen:
//
// Tools (IC 5.1, 5.2; plan.go). The session a person is in gets all four
// tools. A sub-agent gets the ones its definition names (the definition
// the Agent tool found, which the wiring hands over as Session.AgentDef,
// whichever folder it was read from), so the stock kinds, which name
// none, get none, and nothing else of the join either (IC 7.2,
// open-questions Q15). Plan mode does not change the list: a
// session's tools are fixed when it starts, and a model server keeps the
// list as part of the prompt it has read. In plan mode gnovm_list alone
// runs, and a call of one of the other three is answered with what plan
// mode means for the machines and for mem (prompt.PlanRefusal). The mode
// is asked at each call, each prompt and each turn's end, so entering or
// leaving plan mode while a session runs takes effect at once (IC 4.12).
//
// The tools are the carried ones, with every slot filled: the session's
// project and its id; the stack,
// read off the session's own log before every call on mem (memStack: IC
// 6.1, 7.3); the todo the session's /goal binds (boundGoal: IC 9.4);
// whether the todo the session stands in is fresh (memFresh: IC 6.6); the
// names for the texts a call keeps without naming them (namer: IC 5.5);
// the check of a close (checkCloses, closesKept: IC 8); and what a kept
// call hands back (onMemFrames, onMemGoals, onMemErased, onMemRules: IC
// 5.3.4), in that order, under a context that stopping the turn does not
// cancel. A sub-agent's requests are marked as a sub-agent's (IC 7.2).
//
// Protected and Hooks (IC 3.5, 12). The gnovm data directory is a path
// whose change always asks, and hooks are given the path of mem's
// status.json as GNOSH_MEM_STATUS, which is set before mem exists.
//
// Ready. The agent exists and no prompt has gone out. The person is told
// what in mem is due and what waits on them (IC 4.10), a run started with
// --goal binds its goal (IC 9.3), and the front end is handed where the
// session stands (see "What the front end is handed" below): a session
// that is opened again stands where its file says.
//
// RunPrompt (IC 9.3). A run that was given no prompt asks for one. A run
// started with --goal has one, the goal's directive, sent as /goal sends
// it: `gnosh -p --goal "the tests pass"` is a whole command line. A run
// that was given a prompt carries the directive with it, as a hidden
// part.
//
// Busy (IC 7.1). A prompt has made the idle session busy: the session's
// turn lock is held until it goes idle again, after every queued prompt
// has had its turn. That is how other sessions working the same plan see
// this one working. A sub-agent's session holds its own.
//
// PromptLines (IC 4.3 to 4.8). As a prompt goes out, whether it starts a
// turn or is folded into a running one, the join adds its hidden parts
// after the prompt's text and after what the hooks added, in this order:
// the digest of mem, with a session's first prompt, and again with a
// typed prompt when mem was changed elsewhere since the session last
// spoke; what mem recalls for the prompt, when an embedding server is
// set; the mem line, last on every prompt a person types in a session
// that has gnovm_run; and after it the frame line, while the session
// stands in a todo, written from the stack as it stands at that moment
// and carrying the line of every slide that concerns a todo still on the
// stack. In plan mode the mem line and the frame line are left out. The
// loop stores them in that order, so a conversation rebuilt
// from the file is the bytes that went out. Nothing is added to the
// system prompt: the part about mem is there whenever gnovm_run is among
// the tools (IC 4.1), and it is the same bytes in every session (IC 4.2).
// A sub-agent gets none of these lines.
//
// The tool call (IC 3.7, 5.3, 6.2). A gnovm_run call runs alone, with
// everything before it stored. When mem has kept a call that pushed or
// popped, the tool calls onMemFrames before it returns. There each event
// of this session becomes a frame entry, and what each pop removes is
// worked out once, by the code that cuts the view (transcript.Preview).
// From that one answer come the clauses the model reads in the pop's own
// result (prompt.PopReceipt: what was removed, what stays, the files
// edited, the other changes to mem, that the reads are forgotten, or that
// nothing came between); the reads to forget, which are forgotten there
// and then (fstools.Forget), so that a file read inside a popped todo
// must be read again before it is changed; and the number of tokens the
// person is told.
//
// Stored. The loop shows the join every entry it appends, just before it
// is written. For the result of a gnovm_run call the join returns the
// frame entries, which the wiring writes in the same write: the result
// and its pushes and pops are in the file together or not at all (IC
// 5.3.3, 6.4). The stack and every cut are derived from those entries
// alone, now and after a restart, so nothing else is kept anywhere (IC
// 6.1.1, 6.3). Stored also keeps the join's counts: the model's replies
// since mem was last used, and how the closes of the turn went.
//
// AfterStored. The entries are in the file. The person is told "popped
// `x`: about N tokens out of the model's context" (IC 6.2.8), and the
// front end is handed where the session now stands. The loop still holds
// its log at this moment; a notice does not wait for it, and neither does
// what the front end is handed.
//
// Every request after that is built from the view, which leaves the
// popped work out, says on the answer that entered the todo what was
// removed, keeps the prompts of the pop's own turn, and never parts a
// call from its result (IC 6.3: the transcript's and the wiring's doing;
// the join's part is the entries, and its tests hold the result end to
// end). The token count the loop works with follows by itself, since it
// is measured on the view (IC 6.5.1).
//
// BeforeRequest (IC 4.9, 6.5.3). Before each request of a turn the join
// may put a reminder in front of the model: the large-step line, once for
// a todo, when the conversation has grown by more than half the
// compaction point since the todo was entered; and the mem reminder,
// after ten replies with no call on mem.
//
// Slid (IC 6.5.2, 6.5.4). When the window fills while the session stands
// in a todo, the wiring's room-maker slides the oldest rounds out instead
// of writing a summary, records it, and tells the model and the person.
// The join is shown what went and forgets the reads made there. The next
// typed prompt's frame line says it again.
//
// Drivers (IC 8.5, 9.5). As a turn would end, before the person's Stop
// hooks: first the close check's send-back, once a turn, when the session
// stands in a todo whose last close was refused (a sub-agent that has
// gnovm_run is sent back the same way, and has no goal); then the goal.
// The goal driver judges every top-level todo the session closed as done
// in the turn, then asks the goal table that came with mem what to do
// about the bound goal: go on, ask whether there is a real reason to stop
// (every tenth time in a row, and when the goal waits for something
// said), end the turn, judge, bind again, or clear. No count ends the
// work. In plan mode neither sends anything.
//
// RepeatKey and RepeatNudge (IC 8.6). The loop's guard against a model
// repeating itself compares mem's answers with their numbers blanked, and
// a repeated refused close is told that the todo's work is still to do.
//
// AfterSummary (IC 4.7). mem is told that the session was summarized, and
// the digest is given back with what the session is handed afterwards.
//
// Slash (IC 9.2, 12). /mem passes what the person typed to mem and shows
// the answer; its arguments are never logged or put in an error, and the
// command says of itself that what follows "redact" and "forget" is what
// the person wants gone, so that an interface shows and remembers such a
// line only as far as the word. /goal binds, shows and clears the
// session's goal.
//
// RunEnd (IC 9.3). A run that ends with its goal still set says so and
// exits non-zero.
//
// Close. The session is taken off the process's list.
//
// # What the front end is handed
//
// Two things, through the session's facts, whenever one of them may have
// changed: when the result of a call on a machine is in the log, when a
// goal is bound or let go, when an erasure changed either, and when a
// session starts or is opened again. What has not changed is not sent
// twice: the wiring compares.
//
// The task list (IC 10, 6.7; mem.md section 15): mem's todos for the last
// task a call of the session changed, each with where it stands. While
// the session stands in a frame, the line in progress reads the frame: in
// `id` "purpose".
//
// The status line: "frame", the frame the session stands in, or nothing
// at the root; and "goal", the name of the todo the session's /goal
// binds, or nothing.
//
// A sub-agent has neither.
//
// # The checks on a close
//
// fresh.go is the fresh guard's input: the rows a pop made by this call
// would take, judged by the rule of what work is. closecheck.go is the
// close check: entered here, the claim check, and the review, which is a
// side request with prompt.CloseReview, fails open with a warning, and
// remembers its verdicts. rows.go lays a session's log out as what
// happened in it, for both.
//
// # What is kept where
//
// A session's goal binding and its task list are kept with the session,
// as meta entries of the session's own file (mem.goal, mem.tasks). The
// binding is changed only by compare-and-set, and only the one written
// under the session's own id counts, so a copy of a session is bound to
// nothing; the task list is copied with the session. Two more notes
// (mem.said) record what the model was told once.
//
// # Erasures
//
// erase.go follows an erasure in mem (IC 11.2). The erased text is taken
// out of gnosh's own copies of mem's text in every session of the
// project: hidden parts, summaries, bindings, task lists and the frames'
// own titles and results. Appending cannot take text away, so this is the
// one use of transcript.Redact, which writes a session's file again in
// one step. What the person saw is left as it was, and the sessions whose
// visible messages still hold a redacted secret are named. The erasure
// itself is never logged and never stored.
//
// # `gnosh mem`
//
// memcmd.go also runs `gnosh mem [show|find|log]` (IC 12): the project's
// mem read from a shell, with no session, no model and no provider, the
// machines run in the command's own process. It only reads, and in a
// project with no mem it says so and makes no file. A mistake in the
// config does not stop it: where mem is kept does not depend on the
// config.
package memjoin

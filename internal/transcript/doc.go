// Package transcript keeps gnosh's sessions and computes what the model is
// sent.
//
// A session is a file that only grows: one JSON object a line, never
// rewritten. Everything that happens in a conversation is appended to it,
// and nothing is ever taken out. What the model is sent is not the file
// but a view of it: a list of messages worked out from the file each time
// one is needed. A rewind, a summary, a finished frame and a slide all
// change the view and leave the file as it was. That is gnosh's fourth
// decision (ARCHITECTURE.md): the context is a call stack, and the
// transcript keeps it all.
//
// # Where sessions live
//
// A Store is given gnosh's data directory (DataDir: $XDG_DATA_HOME/gnosh,
// or ~/.local/share/gnosh). A session is one file,
//
//	<data>/projects/<slug>/<session-id>.jsonl
//
// where <slug> is the working directory the session was started in, with
// its separators turned into dashes (Slug has the exact rule), and the
// session id is a UUIDv7, so ids sort by the time they were made. There
// is no database and no index: the files are all there is.
//
// One writer has a session at a time. Opening it for writing takes an
// advisory lock on the file; a second writer gets a *LockedError and can
// open it read-only instead, or branch it. A branch is a new session whose
// file begins with a pointer back at an entry of the old one; when it is
// opened, the old session's entries up to that entry are read in front of
// its own, so its view begins with the old conversation.
//
// A file that gnosh was killed in the middle of writing ends in a torn
// line. Loading reports that as a Warning, leaves the line out, and the
// next entry starts on a line of its own. A corrupt line in the middle is
// left out the same way, with a warning that names the line.
//
// # The file
//
// Every line has seven fields: "type"; "uuid", the entry's own id;
// "parentUuid", the entry written before it in the same file (empty for
// the first); "sessionId"; "timestamp", in UTC with nine fractional
// digits; "cwd", the working directory at the time; and "v", the format
// version, 1. The other fields depend on the type. These are the types,
// with a line of each (the first with ids in full, the rest with short
// ones to save room):
//
// A user entry is a user-side message. "source" says where it came from:
// "prompt" (the person typed it), "tool_results", "hook" (context a hook
// added) or "notice" (the harness speaking for itself).
//
//	{"type":"user","uuid":"0193267a-7b01-7000-8000-5a1c3e9d0b01","parentUuid":"","sessionId":"0193267a-7a00-7000-8000-2f4e6a8c0d11","timestamp":"2026-11-13T08:00:00.000000000Z","cwd":"/work/app","v":1,"source":"prompt","message":{"role":"user","content":[{"type":"text","text":"fix the flaky test"}]}}
//	{"type":"user","uuid":"u3","parentUuid":"u2","sessionId":"s","timestamp":"2026-11-13T08:00:05.100000000Z","cwd":"/work/app","v":1,"source":"tool_results","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"in `fix-flaky`"}]}]}}
//
// An assistant entry is one reply of the model. "stop" says how it ended:
// one of core's stop reasons. "cancelled" and "error" mark a reply that
// was cut off: the turn it belongs to ended there. A reply the model is
// not to be sent again (one that held nothing, or only reasoning the
// provider does not want back) is written with no blocks in its message;
// it is in no view, and it still counts for where a turn ended.
//
//	{"type":"assistant","uuid":"u2","parentUuid":"u1","sessionId":"s","timestamp":"2026-11-13T08:00:04.210000000Z","cwd":"/work/app","v":1,"stop":"tool_use","message":{"role":"assistant","content":[{"type":"text","text":"I'll enter a todo for it."},{"type":"tool_use","id":"call_1","name":"gnovm_run","input":{"code":"mem.Todo(\"fix the flaky test\").Push()"}}]}}
//
// A frame entry is a frame boundary. A push and a pop are written right
// after the results of the tool call that made them, which "toolUseId"
// names. A slide records that part of the conversation left the view to
// make room: "anchor" is the reply whose round stays in front of the cut,
// "until" the reply the cut ends before, and "frame" the frame whose start
// was cut (absent for the rounds before the first frame).
//
//	{"type":"frame","uuid":"u4","parentUuid":"u3","sessionId":"s","timestamp":"2026-11-13T08:00:05.100200000Z","cwd":"/work/app","v":1,"op":"push","frame":"fix-flaky","label":"fix the flaky test","toolUseId":"call_1"}
//	{"type":"frame","uuid":"u17","parentUuid":"u16","sessionId":"s","timestamp":"2026-11-13T08:09:41.000000000Z","cwd":"/work/app","v":1,"op":"pop","frame":"fix-flaky","label":"fix the flaky test","result":"the test binds port 0 now; 50 runs pass","toolUseId":"call_5"}
//	{"type":"frame","uuid":"u30","parentUuid":"u29","sessionId":"s","timestamp":"2026-11-13T08:31:02.000000000Z","cwd":"/work/app","v":1,"op":"slide","frame":"update-readme","anchor":"u19","until":"u27"}
//
// A summary entry is text that stands in for the conversation before it.
// "covers" is the last entry it stands in for; without it, that is
// everything written before the summary.
//
//	{"type":"summary","uuid":"u40","parentUuid":"u39","sessionId":"s","timestamp":"2026-11-13T09:02:00.000000000Z","cwd":"/work/app","v":1,"summary":"The user asked for the flaky test to be fixed. It bound a fixed port; it binds port 0 now.","covers":"u39"}
//
// A rewind entry says the conversation continues from an earlier entry:
// what came after that entry, up to the rewind, is no longer part of the
// conversation. It stays in the file.
//
//	{"type":"rewind","uuid":"u11","parentUuid":"u10","sessionId":"s","timestamp":"2026-11-13T08:04:00.000000000Z","cwd":"/work/app","v":1,"to":"u8"}
//
// A title entry names the session. "source" is "user" for a name the
// person gave and "auto" for a generated one; a name the person gave is
// never replaced by a generated one, whichever was written last.
//
//	{"type":"title","uuid":"u20","parentUuid":"u19","sessionId":"s","timestamp":"2026-11-13T08:10:00.000000000Z","cwd":"/work/app","v":1,"source":"user","title":"Flaky listener test"}
//
// A meta entry is a small note: the model changed, the working directory
// changed, the permission mode changed.
//
//	{"type":"meta","uuid":"u21","parentUuid":"u20","sessionId":"s","timestamp":"2026-11-13T08:10:30.000000000Z","cwd":"/work/app","v":1,"key":"model","value":"qwen3.8-27b"}
//
// A usage entry is one request's token counts, its model and how long it
// took.
//
//	{"type":"usage","uuid":"u22","parentUuid":"u21","sessionId":"s","timestamp":"2026-11-13T08:10:34.000000000Z","cwd":"/work/app","v":1,"usage":{"input_tokens":1200,"output_tokens":38,"cache_read_tokens":900},"model":"qwen3.8-27b","durationMs":4210}
//
// A branch entry is the first line of a branched session. It points at
// the session and the entry the branch continues from.
//
//	{"type":"branch","uuid":"b1","parentUuid":"","sessionId":"s2","timestamp":"2026-11-13T10:00:00.000000000Z","cwd":"/work/app","v":1,"fromSession":"s","fromUuid":"u8"}
//
// A line of a type this version does not know is kept as it is and
// ignored by the view. So is a field it does not know on an entry it
// does. Both are written back unchanged when an entry is copied (Entry's
// Extra holds them), so a file from a newer gnosh survives this one.
//
// # The view
//
// View turns a session's entries into the messages the model is sent. It
// is a pure function: the same entries always give the same messages, and
// the entries are never changed. The rule is stated over rows. A reply of
// the model is an assistant row. A user entry is a result row (the
// results of a reply's tool calls) or a prompt row (anything else the
// user side says: a typed prompt, or a hidden one from a hook or the
// harness); one that holds both is two rows. A round is a reply with at
// least one tool call, together with the result rows that answer it.
//
// View applies five things, in this order.
//
// 1. Rewinds. Every entry is written after the tip of the conversation as
// it then stood, and a rewind entry moves the tip back to where it was
// right after the entry it names. What came between is off the
// conversation. Rewinds stack, a later rewind can cut off an earlier one,
// and a rewind to an entry that was cut off brings its line of the
// conversation back. A rewind that names an entry the session does not
// hold changes nothing.
//
// 2. The summary. The view starts at the latest summary that survived the
// rewinds. Its text, after one fixed sentence (SummaryPrefix), becomes
// the first user message, and the rows after what it covers follow.
//
// 3. Frames. In the words of gnosh's own description: each step of a task
// is a frame. A frame begins where the model steps into a todo (a push)
// and ends where it pops it with what came of it. From then on gnosh
// leaves every message between the two out of what the model is sent,
// with the pop and its result as the last line. Nothing is deleted: the
// transcript keeps it all. Only the model's context shrinks, the way a
// stack frame goes when a function returns and only its value comes back.
//
// The stack is read off the frame entries: a push puts a frame on it, a
// pop takes off the topmost frame with that id and no other, so the stack
// is the pushes without a matching pop, in order. A pop that names a frame
// not on the stack changes nothing. A frame still open cuts nothing. Only
// frames entered under the session's own id are on its stack, so a branch
// starts at the root, with the cuts it came with.
//
// A pop cuts every row after the push's round and before the pop's round.
// Precisely: the cut starts after the last result that answers a call of
// the reply that made the push, and ends before the reply that made the
// pop. Both rounds stay whole, so calls made beside the push or the pop
// keep their results, and no call is ever parted from its result. If the
// pop's reply cannot be found, or it is the reply that made the push,
// nothing is cut. If the push's reply cannot be found, the cut starts at
// the first reply after the push entry. Frames nest: an inner pop cuts its
// own range at once, and the outer pop later takes what is left between
// its push and its pop, the inner push and pop rounds included.
//
// Two things in a cut stay. The prompt rows of the turn the pop is made
// in are spared: the prompt the model is answering, prompts folded into
// that turn, and what the harness added to it. (A turn starts at a typed
// prompt, with the hidden rows that stand right before it, when what
// comes before them is nothing, the summary, a reply with no tool call, or
// anything at all after a reply that was cancelled or failed; a prompt
// that follows anything else was folded into a turn already running.)
// What the user said in earlier turns inside the frame goes with the
// rest. And when the push is behind the summary, the rows that continue
// the summary stay: the cut begins at the model's first reply after it.
//
// The answer to the push gets one line added, in the view only, saying
// what was cut: "[Then came the work in it, 7 tool calls (Bash 3, Edit 1,
// Read 3), removed from your context when you popped it.]"; or, when no
// tool was called in it, "[Then came 3 messages of work in it, removed
// from your context when you popped it.]". Without it the conversation
// would read as a todo entered and at once popped, and models copy that.
// The line tells of all the work between the two rounds, whether or not
// an earlier cut had taken some of it, and it is the same in every view
// from the pop on. An answer carries one such line: a pop's takes the
// place of a slide's, and of two of a kind the one whose cut reaches
// furthest stays.
//
// 4. The slide. When the view is over its budget (ViewOptions.Budget, in
// tokens by ViewOptions.Estimate) and the session stands in a frame, the
// oldest part of the conversation leaves the view without a pop. What
// goes, in order, until enough is freed: first the rounds before the
// first frame; then the whole body of each frame that has a frame above
// it, from the bottom; last the oldest rounds of the frame the session
// stands in. Every cut starts after a round that stays and ends before a
// reply with a tool call, so each open frame keeps the round that entered
// the next one, the frame the session stands in keeps its newest rounds,
// and the first message of the conversation is never cut. Neither is the
// prompt the model is answering (ViewOptions.Prompt). Where a cut may end
// anywhere, it frees what is over the budget and a tenth of the budget
// besides (ViewOptions.Room), and at least half of what it could: a slide
// moves the start of what the model reads, so it is made to be rare. The
// answer in front of each cut gets a line: "[Then came the work in it, 4
// tool calls (Read 4), removed from your context to make room.]", or
// "[Then came 2 messages, removed from your context to make room.]". For
// a frame's start the answer is the one to its own push call; for the
// rounds before the first frame, the one to the first call of the round
// that stays. Outside any frame nothing slides; ViewInfo.OverBudget says
// so, and the caller may summarize.
//
// A slide is planned by Slide, which is told how much to free (the loop
// knows that from the provider's own token counts: what is over its
// compaction point, and a tenth of the window), or by View when the view
// is over ViewOptions.Budget by its own estimate. Either way it comes
// back as frame entries. Appending them to the session records it: from
// then on every view cuts the same place, whatever its budget, which is
// what keeps the start of the conversation the same from one request to
// the next. A slide entry names the two replies that bound its cut by
// their entries; Cut.Mark gives the marker the frame rule's own text
// writes for it, which names a tool call of each.
//
// 5. Repair. Whatever the steps above left is made into a conversation
// any provider accepts: it starts with a user message; neighbours of the
// same role are joined in order; every tool call has exactly one result
// in the user message after it (a missing one is filled with a result
// that says the call was interrupted, a result that answers nothing is
// dropped); tool results come first in their message; and nothing is
// empty. Check verifies all of that, and the tests call it on every view.
//
// ViewInfo says what was left out and why: the stack, each cut with its
// frame, its count of messages and tokens and its note, the slides just
// planned, and the repairs. With ViewOptions.Trace it also says, for each
// message of the view, which entries it came from, and for each cut which
// entries it holds, which prompts it spared, and which replies and tool
// calls went with it.
//
// The rule is spec/gnosh/frames.md. CONFORMANCE.md, beside this file,
// lists every rule, edge case and worked example of that spec with the
// code that implements it and the test that proves it, the places where
// this package reads the spec in its own way and why, and the passages
// where the spec is unclear. The tests named TestSpec… are written from
// the spec's words, one section each; oracle_spec_test.go holds the rule a
// second time, as literally as the spec states it, and every view the
// tests compute is checked against it.
//
// # The prompt cache
//
// A model server keeps what it has read of a request and reads on from
// where the next one differs. So the view is made to change only at its
// end, and three things hold of it (the tests hold them over random
// sessions):
//
//   - A message written at the end (a prompt, a reply, results, a note)
//     changes no message of the view but the last, which it may add to. A
//     push changes nothing at all.
//   - A pop changes nothing before the cut: every block of the view up to
//     the push's own answer is as it was, and that answer is as it was but
//     for the line at its end. So the request after a pop shares with the
//     one before it everything through the push's call.
//   - A slide changes nothing before the answer of the round it is
//     anchored at. Everything after it is read again, which is why a slide
//     takes at least half of what it could.
//
// # What the loop does with it
//
// The loop is joined to this package by an adapter. The view is all the
// loop needs to build a request. Around a pop the frame rule asks more of
// the harness than a view: the pop's result tells the model what was
// removed, the reads made in the removed rows are forgotten, the person is
// told how much left the context, and an estimate of it is held against
// the token count until the provider reports a new one. Every fact those
// need is worked out here, once, by the same code that cuts the view. The
// adapter uses them in this order.
//
// Before every call on mem, it tells mem the stack:
//
//	stack := s.Stack() // bottom to top; Frame.ID is the todo's name
//
// When the stack is not empty and the call may close the frame the
// session stands in, it also tells mem whether any work was done in that
// frame. The rows to judge are the replies a pop made by this call would
// take, whether or not an earlier cut took them already. They come from a
// pop that is previewed and never appended:
//
//	top := stack[len(stack)-1]
//	cuts, _ := s.Preview([]transcript.Entry{transcript.PopEntry(top.ID, "", "", call.ID)}, opts)
//	rows := cuts[0].Replies
//
// The frame is not new to this call when cuts[0].PushReply is empty (the
// reply that entered it is not in the conversation) or equal to
// cuts[0].PopReply (this reply entered it). Work done since the plan was
// made counts too: the same preview, of a pop of stack[0]. A Reply that
// says something and calls nothing is an answer; a Call that IsError, or
// that has no result, did nothing.
//
// When mem has kept a call and hands back the frame events it made, the
// adapter makes a frame entry of each event that carries this session's
// id, in order (PushEntry, PopEntry), and previews them. The reply that
// made the call is stored by then; the call's own result is not, since
// the text below goes into it:
//
//	cuts, stack := s.Preview(frames, opts)
//
// There is one Cut for each pop among them. From each it writes the
// clauses of the result: Between == 0 means nothing came between the push
// and the pop; Spared > 0 means prompts of this turn stayed; Replies has
// the tool calls of the removed rows with their arguments (Call.Input,
// Call.Path) and whether each succeeded, for the files edited and the
// changes made to mem. It forgets the reads of every path those calls
// touched (Call.Path, and the files the Bash commands in Call.Input
// showed) and whatever else it keeps by entry (Removed). It adds Tokens
// to the estimate it holds against the next usage report, and tells the
// person: "popped `fix-flaky`: about 44K tokens out of the model's
// context".
//
// Then it stores the result and the frame entries together, so that
// neither is in the file without the other, with a context that stopping
// the turn does not cancel:
//
//	s.AppendAll(append([]transcript.Entry{result}, frames...)...)
//
// From then on every View leaves the frame's work out. What Preview said
// of a pop is, field for field, what View says of it once it is stored
// (ViewInfo.Cuts, with ViewOptions.Trace), so nothing is worked out
// twice.
//
// When the conversation is over the loop's compaction point and the
// session stands in a frame, the loop asks for room before it summarizes:
//
//	slides, cuts := transcript.Slide(s.Entries(), need, transcript.ViewOptions{
//		Session: s.ID(), Prompt: newestPromptOfTheTurn, Estimate: estimate, Trace: true})
//	s.AppendAll(slides...)
//
// Each Cut is used as a pop's is: its Replies for the reads to forget, its
// Tokens for the estimate and for the notice ("slid `x`: about N tokens
// of its start out of the model's context, instead of compacting"). Frame
// names the frame whose start left; when it is empty the rounds before
// the first frame left, and Before names that frame. No slides means
// there was nothing to slide, and the loop summarizes.
//
// When a prompt the person typed goes out, the line that tells the model
// where it stands is written from s.Stack() (Frame.ID, Frame.Label), and
// names the slides that are still Live among ViewInfo.Cuts. The line for a
// step that has grown large compares the loop's token count with
// Frame.TotalAtPush, which is read from the usage entries: so the adapter
// writes a usage entry for each of the session's own requests when its
// reply arrives, and for no other request.
//
// Nothing else is kept anywhere. A session that is opened again, rewound
// or branched has its stack, its cuts and its notes from the file alone.
// The package's Example (example_test.go) runs the steps around a pop from
// start to end.
//
// # A worked example
//
// A session, entry by entry:
//
//	 1  user       prompt: "fix the flaky test"
//	 2  assistant  calls gnovm_run (c1): mem.Todo("fix the flaky test").Push()
//	 3  user       result of c1
//	 4  frame      push fix-flaky, by c1
//	 5  assistant  calls Read (c2)
//	 6  user       result of c2
//	 7  assistant  "Which port does CI use?"
//	 8  user       prompt: "8080"
//	 9  assistant  calls Edit (c3)
//	10  user       result of c3
//	11  rewind     to entry 8
//	12  assistant  calls Edit (c4)
//	13  user       result of c4
//	14  user       prompt: "also say so in the README"
//	15  assistant  calls gnovm_run (c5): mem.Pop("the test binds port 0 now")
//	16  user       result of c5
//	17  frame      pop fix-flaky, by c5
//	18  assistant  calls Read (c6)
//
// and there gnosh was killed, before c6 came back.
//
// Rewinds: entry 11 takes entries 9 and 10 off the conversation. Summary:
// there is none. Frames: the pop cuts what lies after the push's round
// (entries 2 and 3) and before the pop's reply (entry 15), which is
// entries 5 to 14. The pop's turn began at entry 8, since that prompt
// follows a reply with no tool call; so the prompts 8 and 14 are spared,
// and entries 5, 6, 7, 12 and 13 go. Slide: the view is under its budget.
// Repair: entries 3, 8 and 14 are now neighbours on the user side and
// become one message, results first; and c6 gets a result saying it was
// interrupted. The model is sent:
//
//	user       "fix the flaky test"
//	assistant  calls gnovm_run (c1)
//	user       result of c1, with "[Then came the work in it, 2 tool calls
//	           (Edit 1, Read 1), removed from your context when you popped
//	           it.]" added; "8080"; "also say so in the README"
//	assistant  calls gnovm_run (c5)
//	user       result of c5
//	assistant  calls Read (c6)
//	user       result of c6: "This tool call was interrupted before it
//	           finished, so its result is not available."
//
// The file still holds all eighteen entries.
package transcript

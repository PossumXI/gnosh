// Package agent is the loop: the thing that takes a person's prompt, sends
// the conversation to a model, shows the answer as it arrives, runs the
// tools the model asks for under the person's rules, hands the results
// back, and keeps going until the work is done.
//
// It knows nothing of terminals, files, hooks or model servers. Everything
// outside it is reached through a small interface that this package
// defines (a port), and every port has a stand-in in the tests. The wiring
// joins the real packages to the ports.
//
// # A session
//
// An Agent is one session. It has one goroutine of its own, which owns the
// conversation: nothing else starts a turn, appends a reply or decides what
// happens next. That goroutine only ever waits on a channel, so with
// nothing in flight it uses no processor. Tools, the gate, hooks and the
// request to the model each run on a goroutine of their own and report
// back to it; whatever is shared between goroutines is behind one lock.
// Every method of an Agent may be called from any goroutine.
//
//	a, err := agent.New(agent.Options{Model: …, Log: …, Gate: …, Tools: …})
//	id, err := a.Submit(agent.Prompt{Text: "fix the flaky test"}) // returns at once
//	for ev := range a.Events() { … }                              // or Options.OnEvent
//	a.Interrupt()                                                 // Esc
//	a.Wait(ctx)                                                   // until idle
//	a.Close()
//
// Run submits a prompt and waits for the turn that answers it, which is
// all a run that nobody attends needs, and all a sub-agent needs.
//
// # A turn
//
// A turn is everything from a prompt until the session has nothing more to
// do about it:
//
//  1. The prompt is taken in. The session's start moment runs once, before
//     the first prompt, and then the prompt's own check, which may refuse
//     it (it then comes back to the person with the reason) or add hidden
//     parts to it. This happens when the prompt is sent, also while another
//     turn runs, one prompt at a time and in the order sent.
//  2. Before the prompt goes out, the conversation is brought under what
//     the model can hold (see "The context limit").
//  3. The prompt goes into the log with its hidden lines, which are worked
//     out at this moment: what the start moment and the check added, then
//     what each PromptLines provider returns, in order.
//  4. Rounds. Before each request, notes that wait and the BeforeRequest
//     reminders are stored, the view is asked of the log again, and the
//     request is made to fit. The reply is streamed and stored. If it asks
//     for tools they run, their results are stored in the order the model
//     wrote the calls, and the next round begins. Between two rounds the
//     model is told if it repeats itself, and what the person sent
//     meanwhile is handed to it.
//  5. A reply that asks for no tool means the turn is about to end, unless
//     something sends the model back: an empty reply is asked again; then
//     each Driver is asked, in order (a close that mem refused, a goal that
//     is not met, the person's Stop hooks). A send-back is a prompt of
//     gnosh's own inside the same turn, so whoever started the turn sees
//     exactly one end.
//  6. The turn's end is written to the log (EntryTurn) and told
//     (TurnEndEvent, then the TurnFailed and TurnEnded moments). If more
//     prompts wait, the next turn starts without the session going idle;
//     if /loop is on, the loop sends its next prompt; otherwise the session
//     goes idle and what the Busy moment held is let go.
//
// # Tools
//
// A reply may ask for many tools, and all of them run before the next
// round. A tool that says its call only looks (Concurrent) runs beside
// other such calls, up to Options.MaxParallel at once. Every other call
// runs alone and in order: everything before it has finished and is in the
// log before it starts, and nothing starts until it is done. That is what
// a call that changes the conversation itself (gnovm_run) relies on.
//
// On its way a call passes, in this order: the wiring's filter for
// arguments (Options.CleanInput); a refusal if the arguments name a key
// twice; the tool's own account of what it would do (core.Describer); the
// Gate, which answers yes, yes with other arguments, no with a reason the
// model reads, or no and end the turn; the tool; the wiring's filter for
// results (Options.CleanResult); and the AfterTool moment. The gate is
// asked about one call at a time, so a person is put one question at a
// time; calls that already run go on meanwhile. Every call gets a result,
// also the ones after a call that failed or was refused, and every call
// goes by an id that no other call in the session has. A tool that fails,
// panics, or does not stop when told cannot take the turn down or keep it
// from ending.
//
// What a tool returns is text for the model and nothing else. The loop
// never reads it for instructions, and the words that steer the loop (the
// stop marker, LOOP_DONE) are read only in the model's own replies.
//
// # Keeping the work going
//
// No count ever ends the work. A model that repeats the same call is told
// so, with the call quoted, and the turn goes on. A model that answers
// with nothing is asked again, and the person is told at every third time.
// A Driver may send the model back without limit; every tenth time in a
// row (Driver.Every) what the model is sent is a checkpoint: it is asked
// to judge, first and honestly, whether there is a real reason to stop,
// and to end its reply with the marker if there is. The marker counts only
// in the reply to a checkpoint. When the model stops that way the person
// is shown its reason and what the driver objected to. /loop (SetLoop)
// follows the same pattern, turn by turn.
//
// # The moments, in order
//
// For a prompt, as it is sent: SessionStart (once per process), then
// PromptSubmit; the two also run beside a turn that is under way. Before
// the first turn of a busy stretch: Busy. As the prompt goes out: room is
// made if it is needed, then PromptLines. For each request: the notes that wait (Tell), then
// BeforeRequest. For each tool call: Gate, the tool, AfterTool. As a turn
// would end: the Drivers. At its end: TurnFailed (a failed turn only),
// then TurnEnded. Around a summary: BeforeSummary, AfterSummary. When the
// session goes idle: what Busy returned. When it closes: SessionEnd. Fires
// lists them by the names the hook events go by.
//
// # When the provider fails
//
// The provider never retries; the loop decides. A request that the key or
// the request itself makes impossible ends the turn at once, with a
// message that names the provider and its address and says what it
// answered. Everything else is tried again, with a growing pause, and the
// person is told of every wait and its reason (RetryEvent). How long the
// loop keeps trying is a matter of the clock, never of a count of tries:
//
//   - a provider that says it is busy is there, and is waited for without
//     end, for as long as it asks;
//   - a server that had been answering and stopped (it is restarting, or
//     loading a model) is waited for five minutes;
//   - a server that has not answered once since its model was chosen is
//     called not running after a minute and a half.
//
// The clock starts at the first failure and starts over whenever the
// provider shows a sign of life, so failures an hour apart never use up
// each other's time. A reply that had begun to arrive when its request
// failed is thrown away; the next try writes it afresh. The person can
// stop a wait at any moment.
//
// # The context limit
//
// A request the model cannot hold is never sent. The size of the next
// request is the provider's own count of the last one plus the loop's
// estimate of what is new, scaled by how far the estimates have run off
// the counts. A model whose window nobody could learn is given a small
// one (UnknownWindow), and the person is told. Above the compaction point
// the loop first asks the log's owner for room (Roomer): the oldest part
// of the conversation leaves the model's view, and no model is asked for
// anything. Only when nothing can go is a summary written; it is the
// backstop. Its request begins exactly as the session's own requests do,
// a summary the model was cut off in is asked for again with less, and one
// that did not help is not followed by another. When the provider itself
// refuses a request as too long, that request is not sent again: room is
// made first. When nothing helps the turn ends, saying why and what to do.
//
// # Stopping
//
// Interrupt stops all of a turn at once, in whatever state it is: the
// request to the model, every tool, the gate's question to the person, a
// hook, a wait for a failed provider, a summary. What was running is told
// to stop and given a moment (Options.StopGrace); what has still not
// stopped then is left behind, and said so. The prompts gnosh had queued
// for itself are dropped and /loop goes off, or the turn would start
// itself again; what the person queued stays and goes out next; notes
// keep waiting.
//
// Whatever the moment, the log is left as a conversation a new process can
// pick up: a reply that was cut short is stored with what had arrived and
// the reason it was cut, every tool call has a result that says what
// became of it, and the turn has its end. A process that was killed
// cannot do that for itself, so the next one to open the session does it
// first (see Log.Recent). A prompt that was accepted is written down
// before it waits, so it either reaches the model or comes back. And when
// the log itself cannot be written, the turn ends saying so, and what it
// left open is settled as soon as the log can be written again.
//
// # Events
//
// Everything a front end shows comes from one stream of events: text and
// thinking as they arrive, each tool call's start, live output and end,
// questions and their answers, notices, token counts and timings, waits
// and their reasons, and each turn's end with the reason. Putting an
// event in the stream never blocks the loop. Events that wait for a slow
// consumer are kept in order, and pieces of live output that follow each
// other are joined, which loses nothing. Only past a large backlog are the
// oldest pieces of live output thrown away, and a LostEvent says so; what
// they carried still arrives whole with the reply or the tool's result.
//
// # Side requests, notes, sub-agents
//
// Side asks a model a short question outside the conversation, with no
// tools and as little thinking as the model has: the small model first,
// the large one if that fails or is cut off. Side requests are kept off
// the model while the session's own request is being answered, and every
// one is counted.
//
// Tell hands the model something the harness has to say that is not a
// prompt, such as the end of a background command. It goes out with the
// next request there is, and starts nothing by itself unless asked to.
//
// Spawn makes a sub-agent: a session run by this same loop, with a log,
// tools and instructions of its own. Its events go to its parent's
// consumer under the id of the call that started it, its questions are put
// to the person through its parent, and its cost is added to its parent's.
package agent

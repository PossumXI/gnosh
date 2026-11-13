# harness-agent-loop.md — the turn: what Jae changed about how the agent runs

What this file is: the behaviour of the loop that sends a prompt to the model,
runs its tool calls and decides when the turn is over, as Jae had it by the
end. It covers what happens while a turn runs (several tool calls, queued
input, stopping), the guards that keep work going without a counter, thinking
switches, provider errors, the context limit, sub-agents, and what the model
has to be told.

In one paragraph: a turn starts with a prompt and ends when the model answers
without asking for a tool, unless something sends it back to work: a Stop
hook, a goal, `/loop`, an empty answer. Nothing ever ends a turn on a count.
Input typed meanwhile is queued and never lost; Esc always stops everything.
The conversation is kept under the model's window by frames first, by sliding
second and by a summary last. And every request is built so that a local
model server can reuse what it has already read.

Vocabulary, shared with the other spec set (`00-overview.md`): a **round** is
one model call with the tool results that answer it; a **turn** is everything
from one prompt of the user's until the session goes idle.

Not in this file: frames, the slide, the close check, the goal judge, the
line put on every prompt for `mem`, and the session-start digest are in
`frames.md`, `close-check.md`, `mem.md` and `integration-contract.md`.
Provider wire details are in `harness-local-models.md`.

---

## 1. The turn

### LOOP-1 Shape of a turn (P0)

1. The user's prompt is taken (hooks first: HOOK-15, HOOK-16).
2. If the conversation is already past the limit for the model, it is brought
   under the limit **before** the prompt goes out (LOOP-50).
3. Rounds: the model replies; if the reply asks for tools, they run and their
   results go back; repeat.
4. When a reply asks for no tool, the turn is **about to end**. Each of
   these may send the model back to work instead:
   1. the reply was empty (LOOP-11);
   2. a close that `mem` refused, with no work since (`close-check.md`);
   3. a goal that is not met yet (`mem.md`);
   4. Stop hooks (HOOK-21).

   The goal check comes before the Stop hooks. The order of the first two
   is unconfirmed.
5. The turn ends and the session goes idle. Anything queued meanwhile now
   runs (LOOP-7). If `/loop` is on, gnosh sends the next "keep going"
   prompt, which starts a new turn (LOOP-22).

Each "send back" in step 4 is a prompt of gnosh's own (LOOP-8) inside the
**same** turn, so whoever started the turn (a script waiting on `gnosh
run`, the interface) sees exactly one end.

### LOOP-2 Several tool calls in one reply (P0)

A reply may ask for many tools, and all of them run before the next round.

- Tools that only look may run at the same time; tools that change
  something run one after another, in the order the model wrote them
  (`spec/cc/tools.md` has the rule). The earlier gnosh ran its web tools and
  its sub-agents this way, at most five at once.
- Every call gets a result, including the ones after a call that failed or
  was denied.
- **Do not make the model take one step per reply.** Measured on the AI box
  on 2026-11-07, same model, same task: the upstream harness fixed six bugs
  in 4 replies (12 file reads in one reply, 6 edits in another); the earlier
  gnosh took 16 to 40 replies, most with a single tool call, and was 1.3 to
  4 times slower on every task. Each reply is a round trip of seconds to
  minutes on a local model. Nothing in the instructions, the tool
  descriptions or the reminders may push toward one call per reply; reads
  and edits that do not depend on each other belong in one reply.

### LOOP-3 Results that arrive together (P0)

On 2026-11-10 Jae's session died with a fatal error when two tools started
in one round finished at the same moment: the record of which calls had
completed was not safe to write from two places at once. The crash left no
trace on screen (the restarted program drew over it) and none in the log.

Requirements:

- Everything the loop records about a round (which calls are done, what they
  returned, what is shown) is safe when tool calls finish at the same time.
- There is a test that releases five parallel calls together, run under the
  race detector, which is run on the build server before anything concurrent
  is merged. (The AI box cannot run it; that is how this one got through.)
- A fatal error writes what it can to a crash file in the project's data
  folder before the process dies, and the next start says a crash file is
  there (P1).

### LOOP-4 Streaming (P0)

- The reply's text, its thinking and **each tool call's input** are shown as
  they arrive. The earlier gnosh showed only a tool's name and a spinner
  until the whole input had been written; for a long input that was many
  seconds of nothing.
- A tool call cut off mid-stream keeps what arrived, in the session. It is
  sent back to the provider as a call with empty input, and a retried stream
  drops it.
- A reply left half-written by a crash or a kill is closed when the session
  is next opened (LOOP-6).

### LOOP-5 Stopping (P0)

Esc (interactive) and SIGINT or SIGTERM (a run) stop the turn. "Stop" means
all of it, at once:

- the model request in flight;
- every tool that is running, with the processes it started;
- any hook that is running, including the hooks of a prompt that has not
  gone out yet;
- every prompt gnosh had queued for itself (a Stop hook's reason, a nudge, a
  `/loop` continuation). **Esc must clear these**, or the turn restarts
  itself the moment it stops.

What Esc does **not** throw away: prompts the user queued (LOOP-7).

A stopped turn is recorded as stopped by the user. The next prompt starts a
new turn. The message for it says the user stopped the turn; never "context
canceled". (Jae, 2026-11-12: "what happened in the AI box? it says context
canceled". A key press had cancelled a turn that was waiting on a question,
and nothing on screen said so in plain words.)

Stopping must work in every state. States in which it once did not:

| State | What went wrong |
| :- | :- |
| A tool waiting on a background job | No limit on the wait, so the turn spun for 54 minutes with the model idle and Esc dead (TOOL-17). |
| A prompt's hooks still running | The prompt was registered nowhere: the session read as idle, Esc found nothing to stop, and the prompt went out when the hook ended. |
| A summary being written before a prompt | Must be cancellable; the prompt is then kept as a stopped turn. |
| An error that had become its own cause | The program looped for ever at full CPU checking it, with nothing in flight (LOOP-70). |

### LOOP-6 A session always reopens cleanly (P0)

When a session is opened, every assistant reply in it that has no end is
marked as interrupted ("gnosh was stopped before this reply finished")
before anything else happens. Without this a reply left by a kill showed as
a second "working" line beside the real turn, which then looked endless; Jae
reported it twice and kept cancelling turns that were in fact finishing.

Tool calls of that reply that never got a result are settled the same way,
and the log names them.

### LOOP-7 Input typed while a turn runs (P0)

Jae, 2026-10-21: "crush when a job is running and i queue a command, it shows
the queued command but when i hit 'up' it doesn't show what i just queued".
Jae, 2026-10-25: "the second command always queues and nothing happens".

- A prompt sent while a turn runs is **queued**, in the order sent, and the
  queue is shown.
- A queued prompt is given to the model **at the next round boundary of the
  running turn** (between tool results and the next model call). If the turn
  ends first, the queue starts the next turn.
- **Up, in an empty input box, takes the queued prompts back** into the box
  for editing, attachments included, and off the queue. Enter sends them
  again as one prompt. Claude Code lets its user edit queued messages the
  same way. With nothing queued, Up is ordinary history.
- History never offers a second copy of a prompt that is still queued, and
  never offers gnosh's own prompts (LOOP-8).
- **The queue outlives every failure.** A turn that failed, a summary that
  failed, a model server that gave up: what was queued behind it still runs.
  (After one failed summary nothing queued ever ran again.)
- A prompt typed while gnosh is still starting, or refused by a hook, comes
  back into the input box with its attachments and the reason.
- Text typed into the box while queued prompts are being brought back is
  kept, after them.

A queued prompt was lost in five different ways in the earlier gnosh (two
writers to the queue with no common lock; a pop discarded when the display
refreshed; a pop that took a script's prompt and left the script waiting for
ever; a session switch; a prompt blocked by a hook added mid-turn). The
transcript and the queue should be built so that "accepted" has one meaning:
it is on disk, and it will either reach the model or come back to the user.

### LOOP-8 Prompts gnosh sends for itself (P0)

A Stop hook's reason, a repetition nudge, a `/loop` continuation, a
checkpoint question, an empty-answer nudge, a background job's end notice, a
goal's continuation.

- The model reads them as part of the conversation.
- They are not shown as something the user typed, and are not in Up-arrow
  history.
- `UserPromptSubmit` hooks do not run on them.
- Where the user would otherwise wonder why a turn started (a background job
  ended), one quiet line in the conversation says so.
- They are **stored** like any prompt, so the conversation rebuilds
  byte-for-byte the same (LM-10).

Where they go matters to some model servers (LM-13).

### LOOP-9 Session names (P1)

- A new session is **named after its first prompt at once**, so it has a
  useful name from the start.
- A title from the model is asked for **when the first turn ends**, not as it
  begins, and never for a sub-agent's session (LM-14 has the reason: the
  title request used to push a conversation out of the model server's
  cache).
- A name the user gave (`/rename`, `-n`) is never overwritten: not by a late
  title, and not by a summary that read the session before the rename and
  saved it after.
- A non-interactive run keeps the name from its prompt.
- The title request asks for as little thinking as the model allows
  (LOOP-42). Otherwise a thinking model reasons through its small budget and
  every session is called "Untitled".
- A title that cannot be made is a warning in the log, not an error.

---

## 2. Keeping the work going, without counters

### LOOP-10 The repetition guard is a nudge (P0)

Rule in the earlier gnosh: when one round's signature (each call's tool,
input and result) occurs more than 5 times in the last 10 rounds, the model
is repeating itself.

What happens then, since 2026-10-22: **the model is told**, and the turn goes
on. It is told which call it keeps making, that making it again will tell it
nothing new, and to answer with what it has or take a different approach.
There is no limit on how often this may happen.

Before, the turn was ended silently. That cost a sub-agent 45 minutes of
work with nothing returned to its parent.

Details that had to be right:

- The guard looks only after a round **that the model will be asked again
  after**. Six identical calls followed by a final answer must not have the
  answer replaced by a nudge, and a turn ended by a denied permission must
  not be restarted by one.
- **Results that differ only in numbers are the same result.** Every `mem`
  receipt ends with a duration and an entry number, so a repeated `mem` call
  never looked repeated. Numbers in a result are blanked before comparing
  (for `mem` results at least).
- **Two calls taking turns are a loop.** In A-B-A-B each fills exactly 5 of
  10, under the bar. A window of 10 rounds holding only one or two distinct
  signatures counts.
- **A loop of refused closes gets its own words**: nothing was done in the
  todo, so its work is still to do; do it, or tell the user it is not done.
  A model once sent one refused call about 300 times in 40 minutes through
  30 nudges that said only "do not repeat".
- The nudge quotes the refusal or the repeated call, so the model can see
  what is meant.

Two cheaper guards that were discussed and not built (P2): hold an identical
command that has failed three times until the model states a different
approach; make "permission denied" say who the agent is running as and where
it can write.

### LOOP-11 A model that answers with nothing (P0; OQ-7)

Some models sometimes end a reply with no text and no tool call (on one of
Jae's models, about one reply in seven: reasoning only, or nothing at all).
The turn used to end in silence, and the session looked hung.

Now the model is asked again, with a short prompt of gnosh's own. Only the
model's own silence counts: a stopped, failed, filtered or length-limited
reply is left alone. The earlier gnosh gave up after three tries in a row
and logged what to change (lower the thinking level). The retry was seen
recovering real turns the day it shipped.

The three-try limit is the one counter left in the loop. Recommended
reading (OQ-7): keep asking, and at every third empty reply tell the user
what is happening and what usually fixes it, since the user is the one who
can change the thinking level.

### LOOP-20 Checkpoints, the common pattern (P0)

PRIN-4. Every guard that could go round for ever uses the same shape:

1. It goes on with no limit.
2. Every N rounds, the prompt sent to the model is a **checkpoint**: decide
   first, honestly, whether there is a genuine reason to stop (cannot be done
   as asked; blocked on the user; the check cannot pass here; repeating
   without progress). If yes: say why, and end the reply with the marker for
   that guard on a line of its own. If no: carry on.
3. The marker counts **only in the reply to the checkpoint**, and only in a
   finished reply.
4. When the model stops this way, the user is shown that it chose to stop,
   with its reason and with what the guard last objected to.

| Guard | Checkpoint every | Marker | Where |
| :- | :- | :- | :- |
| Stop hook, SubagentStop hook | 10 blocks in a row | `<!-- GNOSH_STOP -->` | HOOK-21 |
| Goal not met | 10 checks | the same (as first built on 2026-10-25; `mem.md` has the present rule) | `mem.md` |
| `/loop` | 25 turns | `LOOP_STOP` | LOOP-22 |

Any test that relies on the model choosing to stop has a bound of its own.

### LOOP-22 `/loop` (P1; OQ-17)

Jae, 2026-10-21: "crush has no /loop like command at all?" and then "a real
/loop. test it and make sure it works properly". Among his decisions on the
second version of `mem`, 2026-10-30: "loop stays".

What it is: `/loop` keeps the session working on its task. When a turn ends,
gnosh sends the next "keep going" prompt by itself, until the work is done.
`/noloop` turns it off. It replaces typing "continue" again and again.

Rules, each from a bug:

- **Done** is the word `LOOP_DONE` alone on a line of the reply. A sentence
  that merely mentions it ("I will reply LOOP_DONE when the files exist")
  is not done.
- **A first claim of done is questioned.** The model is asked to name what
  it produced and show the listing or output that proves it. Only a claim
  made in reply to that question ends the loop. (One model wrote "Given
  time, I'll output LOOP_DONE" to get out of a hard step, twice, and the
  loop believed it.)
- **Check-in every 25 turns** instead of a cap (LOOP-20). The answer that
  stops the loop is `LOOP_STOP`, read from the reply to the check-in itself,
  not from whatever message happens to be last in the turn.
- The loop **stops** when: the user presses Esc (also when armed with
  nothing running); the user denies a permission; a hook ends the turn; the
  turn fails; a prompt cannot be sent; the user switches to another session.
  A stop is final: a Stop hook is not consulted after a halt.
- Starting it while a turn runs only arms it: the running turn's end sends
  the first continuation. Starting it idle sends one at once. It must never
  send two for one turn, or the queue grows every round.
- "Busy" means **this** session. `/loop` in an idle session while another
  session works must still start.
- A summary written in the middle of a loop is not a finished turn.
- An optional cap exists for whoever wants one (`option loop-max-turns N`);
  it runs exactly N turns. Unset, there is none.
- While it is on, the interface says so, through the final turn.

Name clash: Claude Code has a `/loop` of its own that repeats a prompt on a
timer. Jae's `/loop` means "until done". OQ-17 recommends keeping his meaning
under his name.

### LOOP-23 `/goal` (P1)

As CC's `/goal <condition>`: the agent keeps working until a separate check
finds the condition met. In gnosh the goal lives in `mem` and the check runs
when the agent closes it; all of that is in `mem.md`. The harness's part:
the check is a short request to the small model with thinking off (LM-15);
"not met" sends the model back with the reason; there is no cap, with a
checkpoint every 10 checks; a goal goes on after a summary; a goal waiting
on the user ends the turn; the verdict is shown, and printed by a run
(CLI-9).

---

## 3. Thinking

### LOOP-40 `/think` and `/no_think`, and they stick (P0)

Jae, 2026-10-18: "make /think and /no-think … work on crush for all future
interactions". It was the second change ever made to the fork.

- `/think` and `/no_think` (also `/nothink`, `/no-think`) are **commands to
  gnosh**, typed in a session or given as the prompt of a run. They are not
  sent to the model.
- They take effect for **every later request**, in this session and every
  future one, until changed. The choice is saved in the user's state
  (CFG-2), not in the project.
- They say what they did ("thinking off for Qwen...") and point at `/model`
  for the levels in between.

His own lanes run with thinking **off**. A new user gets thinking **on** for
a model that can think (OQ-18).

### LOOP-41 A level per model (P1)

The on/off switch turned out to be the wrong shape (2026-10-25): it spoke
only one model family's dialect and did nothing at all on the other model
Jae ran, which has three levels and no "off".

- Each model has its own ordered list of thinking levels: `off, on` for a
  Qwen; `low, medium, high` for gpt-oss; whatever `--reasoning-levels`
  declares (CFG-13). With nothing declared the model's ID decides. A model
  with no known axis ignores the commands.
- The level is stored **per provider and model**.
- `/think` moves the current model to the top of its axis, `/no_think` to
  the bottom, and each says which level that was.
- `/model` shows each model's level and changes it with the left and right
  arrow keys, as CC's model picker does (UI-21).
- The status line shows the current model's level at all times, in the
  model's own words ("Thinking Off", "Reasoning Low"). Jae, 2026-10-25: "the
  right sidebar doesn't show 'low/med/high' effort level. likewise should
  show for think/nothink for qwen, a general solution please."

How a level is put on the wire for each kind of server: LM-5.

### LOOP-42 Side requests think as little as possible (P0)

A request for a session title, a todo's name, a goal check or a close
review asks for the **lowest** level the model has. For a model whose ID
names no axis, the plain "thinking off" switch is still sent. A thinking
model that reasons through a 40-token title budget produces no title.

Corollary for anyone replaying such a request in a test or a benchmark:
send the same switch, or the replay measures a call gnosh never makes. (A
replay without it had the model reason and be cut off 9 times in 10, which
was then misreported as a bug.)

### LOOP-43 What was learned about levels (for defaults and messages)

- On Jae's Qwen, thinking on is right more often and takes 2 to 4 times as
  long. With thinking off the model works sums in its head and writes
  closes before the work; the harness's checks matter most where the model
  thinks least.
- On gpt-oss, the highest level made it write prose and call **no tools**,
  and return nothing at all on a small budget. `/no_think` (lowest) brought
  the tool calls back. So an empty-answer warning should name the thinking
  level as the first thing to try.
- The thinking a model shows is worth reading. Jae asked on 2026-11-12 how
  to see "the internal dialogue". gnosh shows a model's thinking when the
  model sends it (UI-22).

---

## 4. Provider errors

### LOOP-45 What the user sees when the model server fails (P0)

Full detail in `harness-local-models.md` (LM-1 to LM-4). In short:

- A server that is **restarting or loading** is waited for; the turn is not
  lost. (A sub-agent once lost an hour of work to a restart its parent
  survived.)
- A server that is **not there** is reported in about a minute and a half,
  not twenty, and the message names the server and says it is not running.
- A request is retried with a short back-off a few times **per failure**,
  not a few times over the whole life of a long round.
- The error shown always says which provider, at which address, and what it
  answered or failed to answer. Never only "Provider Error".
- A turn that ends on a provider error fires `StopFailure`, not `Stop`, and
  what was queued behind it still runs (LOOP-7).
- When no provider can be used at all, gnosh still starts and says which
  were left out and why.

### LOOP-46 The clocks, in one place (P0)

| Clock | Default | Set by | What happens when it runs out |
| :- | :- | :- | :- |
| Until a response starts (remote API) | 10 minutes | `API_TIMEOUT_MS` | Provider error. LM-1. |
| Until a response starts (local server) | none | `API_TIMEOUT_MS` | n/a. LM-1. |
| Until the first piece of a reply | 5 times the idle limit (OQ-13: none for a local server) | `request-timeout` | "No reply started within N." LM-2. |
| Between pieces of a reply | 60 s (Jae: 900) | `request-timeout` | "Nothing received for N." LM-2. |
| A server that was answering and stopped | 5 minutes | fixed | The error names the server. LM-3. |
| A server that never answered | 15 s per retry | fixed | "Is not running." LM-3. |
| A shell command in the foreground | 2 minutes, then it moves to the background | `BASH_DEFAULT_TIMEOUT_MS`, the call's `timeout` | It keeps running; the model is told. TOOL-15. |
| A wait on a background job | 5 minutes | fixed | The output so far, and "still running". TOOL-17. |
| A hook | 600 s; 30 s for `UserPromptSubmit`; 1.5 s for `SessionEnd` | the hook's `timeout` | The hook could not evaluate. HOOK-7. |
| A search | its own timeout | `CLAUDE_CODE_GLOB_TIMEOUT_SECONDS` | The result says it was cut short. TOOL-14. |

None of these ends a turn by itself except a provider that stays gone. None
is a count of attempts (PRIN-4).

---

## 5. The context limit

### LOOP-50 Never send a request the model cannot hold (P0)

The check is made at **two** points: after each round, and **before a
prompt goes out**. The second matters when the conversation is already too
long for the model about to read it: after a switch to a model with a
smaller window, or a resumed session. (The earlier gnosh checked only after
a reply, so the first request after such a switch sent everything; the
model spent six minutes reading it, and the summary that followed had no
room to be written.)

Order of remedies:

1. **Frames**: finished todos are already out of the context (`frames.md`).
2. **The slide**: where the session stands in frames, the oldest rounds
   leave, with no summary (`frames.md`).
3. **A summary**, only when there is nothing to slide (LOOP-60).

### LOOP-51 An unknown window is a small one (P0)

A model whose context window gnosh could not learn is treated as having
32,768 tokens. The earlier gnosh treated "unknown" as "never compact"; the
session grew until the server refused every request, and since every later
request carried the same history it was wedged for good. Jae, 2026-10-21:
"it's still a bug if it crashes like that so fix the bug".

Compacting a larger model too early is wasteful and recoverable. Crashing is
neither. The window is learned from the server where it can be (LM-20),
because 32,768 is far too early for a 262K model.

### LOOP-52 Counting (P1)

The count used is the provider's own, from the last reply's usage. Where
gnosh has to estimate (text not yet sent), an estimate from characters is
scaled by how far the last estimate was off; number-heavy text ran 1.44
times the plain estimate on the AI box.

### LOOP-53 It must hold under small windows too (P1)

Jae, 2026-11-07, on a benchmark where the earlier gnosh ran a 32K-window
model to 40.5K after four slides: "make note to fix it later". On a server
with a real 32K slot that request fails. It was not diagnosed: either the
threshold was tuned for big windows, or the window discovered was not 32K.

Requirement: at 24K and 32K windows, with the fixed prompt as it really is,
a long task never produces a request over the window. This is tested
against a stand-in server that reports usage and refuses anything over its
window. The earlier gnosh's fixed prompt was about 14K tokens (LOOP-104),
which leaves little of a 32K window; a smaller one is part of the fix.

### LOOP-60 The summary is a backstop (P1)

When it runs: for a large window, when the conversation comes within 20,000
tokens of it (262,144 gives 242,144); for a smaller one, at four fifths of
it (131,072 gives 104,858; 32,768 gives 26,214). Where "large" begins is
inferred from those figures to be about 200,000 tokens (unconfirmed). CC's
variables and `/autocompact` can make it earlier, never later (CFG-51).

Jae's direction, 2026-10-31: "what if instead of compaction we do the
context sliding thing instead and 'pop' the first frame so to speak?" and
"phrase needed somewhere 'no more compactions'". So with `mem` in use a
summary should be rare, and every one that happens on his box is worth a
look.

What a summary costs on his machine, measured over 24 real summaries
(2026-11-01): a median of about 4,500 tokens written, at about 13 tokens a
second: **five to six minutes of silence each**, up to seventeen. Even a
6K-token context was summarized into 5.6K tokens. The likely cause is the
long nine-section form (LOOP-63); OQ-16.

### LOOP-61 How the summary request is made (P1)

- **It starts exactly as the session's own requests do**: same system
  prompt, same tools, same messages, with the request for a summary at the
  end. Then a local server reuses its cache. The earlier gnosh first sent a
  summary request with its own system prompt and no tools, which shared 19
  characters with the conversation; with a full context that meant twenty
  minutes of re-reading before a word of summary.
- **Room is left for the answer**: half of the gap between the compaction
  point and the window (13,107 tokens at 131,072; 10,000 at 262,144; 3,276
  at 32,768). If the request as it stands does not leave that room, it is
  sent bare (no system prompt or tools); if that still does not fit, with
  the earliest turns left out, cut at one of the agent's own messages, with
  a note saying so.
- **A summary the server cut short is not accepted.** A reply that ends on
  the length limit with the request and reply together filling the window
  is a cut-off summary: it is asked for again with less. If nothing fits,
  the conversation is kept as it was and the user reads that the summary did
  not fit in the model's context window. (A summary ending in the middle of
  a line of code was once stored as complete, and the session ran on it.)
- A summary that fails sends the waiting prompt as it is.
- It can be stopped with Esc.

### LOOP-62 A summary that does not help is not repeated (P1)

If the conversation is still over the point right after an automatic
summary, summarizing again will not fix it. With CC's percentage variable
set to 50, the earlier gnosh once summarized 1,562 times in two minutes.

Rule: after an automatic summary that left the context over the point, drop
the user's earlier-than-default setting; if it is over the default point
too, stop automatic summaries for the session, with a warning each time one
would have run. A summary the user asks for by hand resets this. The slide
is judged apart from this, so that giving up on summaries never turns the
slide off.

### LOOP-63 What survives a summary (P1)

As Claude Code does it (`spec/cc`), and each of these was added because its
absence cost a turn:

- The summary is asked for in CC's form: sections, every message of the
  user's, and the next step in the user's own words. `/compact <text>` adds
  the user's instructions for it.
- Afterwards the model is told it is going on from a conversation that ran
  out of room, and to carry on with the task without asking.
- The conversation the summary replaced is written to a file, and the model
  is told where it is.
- The files it read most recently are given again, read fresh from disk.
- Background jobs still running are named.
- Skills it had loaded are given again.
- Context files it had been given on demand are given again (CFG-23).
- The open work in `mem` is handed on (`mem.md`); a goal goes on.
- The session's name is kept.
- `SessionStart` hooks run with source `compact`.

### LOOP-64 Idle compaction (P2)

As CC's: a conversation of 200,000 tokens or more left idle for 54 minutes
is summarized then, so the next prompt sends a summary and a local model
reads it while the user is away rather than while he waits. Interactive
sessions only; `option idle-compaction false` turns it off. With frames in
use this should seldom apply.

### LOOP-65 Seeing and setting it (P1)

`/context` shows what fills the window by category (CLI-12).
`/autocompact 150k` sets how full the context may get, `/autocompact auto`
goes back to the default, `/autocompact` alone says what is set; the value
is saved for every model and never exceeds a model's own window.
`/compact [instructions]` summarizes now.

---

## 6. Sub-agents

### LOOP-80 The two kinds (P1)

| Kind | Started by | Has | For |
| :- | :- | :- | :- |
| Search sub-agent | The `Agent` tool (CC's name) | Reading and searching tools only. It changes nothing. | A question about the code that would otherwise fill the main context with file contents. It is told what CC tells its exploring agent, and the tool asks how broad the search should be. |
| Web-reading sub-agent | The tool that reads web pages on the model's behalf | Web fetch and web search, and file tools that can open **only the pages it fetched** (SAFE-21). | Reading the web without putting untrusted pages in the main context. |

A third, the **plan agent**, is plan mode: it plans with the project's
instructions and environment and changes nothing. Leaving plan mode to carry
the plan out tells the model to put the plan in `mem` first (`mem.md`).

### LOOP-81 Rules for every sub-agent (P1)

- It works in a session of its own, linked to its parent. It is never
  listed among the user's sessions and **never gets a generated title**
  (LM-14).
- It inherits its parent's permission standing. Under `gnosh run`, and under
  `-y`, it can act without asking, as its parent can. (It once waited for
  ever on a question nobody could answer.)
- The user's hooks run on its tool calls (HOOK-13), and `SubagentStart` and
  `SubagentStop` fire for it.
- A halt inside it ends the parent's turn.
- The repetition guard (LOOP-10) applies, and what it found so far is
  returned to the parent.
- A model server restart is ridden out for it as for the parent.
- The context limit (section 5) applies to it.
- Its prompt names the directories added with `--add-dir`.
- Its cost is added to the parent session's. Only the cost: its tokens are
  not counted as the parent's context.
- Its answer to the parent is its last reply, after any `SubagentStop` hook
  has let it finish.
- It has no tools that need a person (signing, paying, questions) and none
  that record deliverables.
- `--verbose` shows its tool calls under the call that started it.

### LOOP-82 What was learned about how models use them (P2)

- Told to ask "one narrow, concrete question", the local model still hands
  a sub-agent numbered lists of seven. A reworded tool description was
  measured and made no difference. A sub-agent takes minutes on a local
  model (2 to 18 in one traced run).
- The real cost was not the sub-agent's own work but what it did to the
  model server's cache (LM-14): 35 of 195 minutes in that run went to the
  parent re-reading its own conversation after each sub-agent.

---

## 7. A session that seems stuck

### LOOP-70 Nothing may spin with nothing in flight (P0)

Jae, 2026-10-30: "the ai box seems stuck?" The session had sat at over 100%
of a processor for two hours with the interface still drawing a spinner. It
had **no connection open and no child process**: a loop inside the program.
An error that had been recorded as its own cause was being walked for ever.
Cancelling did nothing, because the loop never looked.

Requirements:

- An error is never changed after it is made. A new error is built that
  wraps the old one.
- Every tool call has either a deadline or a way to be cancelled that it
  really honours (TOOL-17 lists the ones that had neither).
- **A debugging endpoint.** With `GNOSH_PROFILE` set, gnosh serves on
  loopback what is needed to see where it is stuck (every goroutine's stack,
  a short CPU profile), on a free port if the usual one is taken, with the
  port in the log. A stuck session of Jae's is diagnosed through this. It is
  **never** killed to get a dump.
- The signature to tell a stuck gnosh from a slow model, for the user or
  another session: full CPU, no network connection, no child process, and
  the session file's time frozen at the moment the turn began.

### LOOP-71 What a running session picks up (P1)

A running gnosh keeps the program and the config it started with. A new
build, a new hook and an edited config file all need a restart; a choice
made inside gnosh applies at once (CFG-42). This confused more than one
investigation, so `--version`, `/status` and the log's first lines say which
build is running and when it started.

### LOOP-75 Switching models mid-session (P1)

Jae, 2026-10-25: "when i tried to switch models on klash from qwen to gpt it
hung".

- Switching to a model whose server is not running must fail in seconds
  with a message saying so, not hang for twenty minutes (LM-3).
- Switching to a model with a smaller window brings the conversation under
  that window before the next prompt goes out (LOOP-50).
- On a server that holds one model at a time, the first request after a
  switch waits for the model to load (about 25 s to a minute on the box).
  The interface should say it is waiting for the model to load, and Esc
  cancels.

---

## 8. What the model has to be told

Requirements for the system prompt, the tool descriptions and the reminders.
**No wording here is the earlier gnosh's**; these say what must be conveyed
and why Jae wanted it. The build side writes its own text.

### LOOP-100 Security work: help, do not refuse (P0; OQ-11)

Jae's one deliberate change to the instructions, kept since 2026-10-22 and
made the built-in rule on 2026-10-30 at his word "sure do it".

Requirement, in three parts:

1. The model takes the initiative on defensive security work: it offers and
   does it, rather than waiting to be pushed.
2. "Someone could misuse this" is **not**, by itself, a reason to decline a
   coding request.
3. When the model is unsure what a request is for, it asks the user, and
   once answered it goes ahead.

Why: his work is security work (auditing realms, hardening servers,
reviewing exploits), and an instruction to decline whatever might be
misused stops that work.

His own sentence for this exists in his files. It was written by editing a
sentence of the old instructions, so it is not reproduced here; the build
side writes a fresh sentence from this requirement, and Jae may replace it
(OQ-11).

### LOOP-101 The instructions are his to replace, visibly (P1)

PRIN-13, CLI-12. And the lesson: for a week Jae's own sessions ran on a
three-week-old copy of the whole prompt, kept for the sake of one sentence,
so no later improvement reached him, including everything about `mem`.

So: prefer **small, named pieces** the user can override (one rule, one
tool's description) over one file that replaces everything; when a full
replacement is in use, say so at startup and in `gnosh prompt show`; and say
when the replacement is older than the build.

### LOOP-102 A project with Gno code audits its new Gno code (P1)

Jae, 2026-11-06/07, after fifteen review rounds elsewhere never read the two
guides his standing rule names: would the rule reach a new project? "think
hard, raise the limit or whatever, make sure empirically that it works, but
do the minimal thing that works. and push."

Requirement: when the working directory holds Gno code (a `gnomod.toml`,
`gno.mod` or `gnowork.toml`, or any `.gno` file), the model is told that a
plan which writes Gno code has an **audit todo right after the code todo**,
against the interrealm specification and the security guide, both of which
gnosh carries with its Gno skill so that every project has them.

Evidence that this is the minimal thing that works (Qwen, thinking off, a
small realm with three planted defects the guides name): with no rule, all
three shipped and the work was called ready in 9 minutes. With the rule only
in the skill, one run never opened the skill and two never reached the
audit. With the rule in the instructions: the audit was planned in 3 of 3
runs, both guides read in 3 of 3, 7 of 9 defects fixed. A rule in a
repository's own context file reaches only projects inside that repository
(CFG-20).

### LOOP-103 Things the model kept getting wrong until it was told (P1)

Each is one requirement; the reason is what happened without it.

| Tell the model | Because |
| :- | :- |
| Work on files with the file tools, not with shell commands, **and why**: they keep track of what it has read | It read files with `sed -n` and searched with shell `grep`, and its next edit was refused as an edit of a file it had never read. |
| Search where a thing belongs (the project; a tool's own lookup; a config or data folder), never the whole disk; when that finds nothing, ask | It ran a search over the entire filesystem three times in one day: for a library's source, for a secret the user had erased, for a tool that was not installed. |
| The project's `.gnosh/` folder is never deleted or moved | CFG-3. |
| Today's date in the form `2026-10-30` | It copies the form it is shown into calls that take only that form. |
| Which machine it is on and as whom, and which other machines are declared | PRIN-11. |
| A background command's end will be announced; do not poll or sleep for it | TOOL-16. |
| Plan work of several steps in `mem` | `mem.md`. And nothing else in the instructions may say the opposite: a leftover line telling the model to keep its plan to itself was why one model family never used `mem` at all. |
| When asked to hand work to a sub-agent, ask one narrow question | LOOP-82. |

A hint that the model needs at the moment of a failure goes **in the
failure's own result**, not in the instructions (PRIN-7). The line about not
searching the disk did not work in the system prompt and did work as a note
under "command not found".

### LOOP-104 The fixed part of every request is small and stable (P0)

"Fixed part" = the system prompt plus the tool definitions: what every
request of every session begins with.

- **Stable**: byte-for-byte the same from session to session with the same
  config, so a local server that has read it once reads it in half a second
  afterwards. Anything that changes per session (working directory, date,
  git status, the machine) is **not** in it; it is sent as the first message
  of the conversation instead (LM-11).
- **Small**: measured on 2026-11-07, the earlier gnosh's fixed part was
  31,087 characters of instructions and 33,187 of tool definitions, 29
  tools, about 14,000 tokens. Its upstream's was about 10,500 with 26
  tools; another harness's 5,500 with 7. On the box a cold session reads
  the fixed part at about 300 tokens a second, so 14,000 tokens is most of
  a minute before the first word. The section about `mem` alone was a
  quarter of the instructions.
- The size is printed by `gnosh prompt context` and `gnosh prompt tools`, so
  it can be watched.
- A leaner form for small windows is a named follow-up from the benchmarks
  (P2).

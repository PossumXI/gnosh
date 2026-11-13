# close-check.md — checking a todo's close before mem keeps it

What this file is: the checks the harness runs when the model closes a todo
as done, the flow between mem and the harness, the review's inputs and what
its judge must be told, the refusal texts, the turn-end send-back, the soft
hold on a plan (A11), the option, the evidence and the limits.

The gate inside mem (which closes are handed out, and re-running the input)
is carried over. **The three checks, the window, the review call, the
caches, the refusal texts and the send-backs are harness-side and must be
rebuilt.** Sources: CLOSE-CHECK.md (the design, two design reviews, two code
review rounds, the replay), IMPROVEMENTS.md Status 3, QA-FRAMES.md runs
claimA1–D1, BENCH.md and BENCH-SWARM.md (fixes A and C and the swarm fix),
GNOMEM.md, the memory notes, and a reading of the fork's own check code.

Landed 2026-11-05; changed 2026-11-06 (refusal texts), 2026-11-11 (fix A,
fix C, the review names its todo).

---

## 1. The problem

"With thinking off, Qwen sometimes writes a todo's result before doing the
work ('Median added in stats.go:25-36, tests pass'), and nothing was
written." [D] The fresh guard (`frames.md` §9) refuses a close when nothing
at all happened in the todo, "but a look counts as something: an `ls`, a
`view`, even a tool call that failed. One look, the same claim again, and
mem records work that was never done."

How common (QA, 2026-11-04): with thinking off, 17 of 90 accepted pops had no
work in their todo; one scenario made up a close in 5 of its 6 runs. With
thinking on: 1 false close in about 20 runs, at 3–5 times the time per step.
"Mem's guards matter most where the model thinks least."

Jae, 2026-11-05: "ok do both [the claim check and A11's soft hold]. but
also, maybe the thinking issue can be mitigated partially with better
prompts too? … maybe todo dones can have a separate review step upon frame
pop or something that double checks it was actually done? review until
convergence and implement solution. maybe the review thing since it's
expensive can be an option that can be turned off, on by default".

What prompts could do was measured: the reworded push receipt ("do it now,
with your tools; once what they return shows it done…") "did NOT stop Qwen's
result-first habit, the checks do".

---

## 2. Which closes are checked, and the flow

**Checked:** every close, as done, of **a todo the work is done in**: one
with no done todo under it (its todos, if any, were all dropped), that is
not an answer the user gave to a question. Closed any way: `mem.Pop`,
`mem.Done` or `Frame.Done` of the top frame, and `mem.Done(name, …)` of a
todo not on the stack ("`mem.Leave()` then `mem.Done(x, …)` would otherwise
get round them").

**Not checked:** a todo with done todos under it ("a parent is closed on its
todos' results, which were checked"; the goal judge still judges closed
tasks); `mem.Drop` and `mem.Leave` ("giving up and stepping out are honest
by nature"); the user's answer to a question; closes made by the user's
`/mem` or by inputs that did not ask for checking.

**The flow**, per `gnovm_run` call on mem:

1. The tool sends the input with `RunRequest.CheckCloses = true` (only the
   agent's own tool does; tests and the harness's own mem calls do not).
2. Inside mem, each such close is reported through a native (`closing`), and
   the realm runs on as if it were kept.
3. If any reported close is not yet approved, **the host keeps nothing of
   the input** and returns the closes (`RunResult.Closes`), with an error
   that the tool does not show.

   ```go
   type MemClose struct {
       Id       string // the todo
       Title    string // its task
       Result   string // what the input closes it with ("" when it gives none)
       Thoughts string // the thoughts kept on it, one a line: "name: text"
       Entered  bool   // the input itself entered the todo (it was not on the stack before)
       Worked   bool   // the input changed mem after entering it, beyond entering and closing it
   }
   ```

4. The tool hands the not-yet-approved closes to the harness's check (§3),
   with the session id and the call id.
5. **Refused:** the tool returns the refusal as the call's error. Nothing
   changed in mem, nothing is cut. What the run printed is dropped ("a
   'done' there would read as kept").
6. **Approved:** the tool adds each close to `RunRequest.Checked` and **runs
   the same request again**, names included (the small model would give new
   names on a new request). Approval is recorded two ways per close: by id
   and result, and by id with any result, so a todo approved once in a call
   is not asked about again though its result changed between runs ("a
   result made of the time could loop"). The rerun may surface closes not
   yet approved; the loop ends when a run brings none.
7. After a checked call the harness is told whether mem kept it (§5.4).

"Code is deterministic Gno with no I/O, so the second run reaches the same
close with the same result." A close costs two mem runs in agent sessions.

---

## 3. The checks, cheapest first

For each close, in order. The first refusal wins; an empty answer approves.

### 3.1 Entered here

A todo this same input entered and closed, with nothing done in mem after
entering it and no work in its window: refused [F].

```
not popped: this call entered `x` and closed it with nothing done in it, so nothing from the call was kept and you are not in `x`. If its work is still to do, enter it, do the work, then pop it with what came of it. If you already did its work where you are now, close it by its name without entering it: mem.Done("x", "what came of it").
```

This closes the gap where `mem.Next()` and `mem.Pop("…")` in one call were
never refused by the fresh guard.

### 3.2 The window: what counts as "in the todo"

Every check reads **the window**: the rows of the session between a start
point and the closing call, plus the calls made before the closing call in
its own round and their results.

The start point:

| Case | The window starts |
|---|---|
| the todo is on the stack, and the round that entered it also popped, left or dropped a frame (`mem.Pop("…"); mem.Next()` in one call) | after that round: "nothing comes between the todo before and this one" |
| the todo is on the stack otherwise | at the session's last frame event before its push in the same turn, or the turn's start, whichever is later, but no later than its push: "so work done on the way counts" |
| the input itself entered it | the window is the closing call's own round only, and the review is shown the call's code |
| closed by name, not on the stack | after the session's last frame event before the call (a sibling's pop, a Leave of this very todo); never before the present frame's push round, nor before the turn's start: "so a sibling's work is not this todo's" |
| **fix A** (Jae, 2026-11-11): the todo is under a plan, that is, the frame at the bottom of the stack is another frame | **reaches back to that bottom frame's push**, whatever the case above. "A todo's work done before it was entered (a whole plan worked from its root, then each todo entered and closed) is its evidence too." The plan's own close, and a lone todo's, keep their window. |

If the todo's push is on the stack but gone from the rows, the window cannot
be read and the close is approved unchecked.

The window also carries:

- a **start time**: one second after its first row's time (stored times are
  whole seconds, "so the todo before it … is never taken for its work");
- the **files the session's calls named**, by base name (from any call's
  `file_path` or `path` argument), to resolve a file a result names;
- **what the user asked in the turn**, when no visible prompt lies in the
  window.

### 3.3 The claim check (always on, free)

Refuse a result that says a file changed when nothing in the window changed
or showed that file.

**What is a claim.** Split the result into clauses (at `;`, a newline, a
full stop followed by space, or a spaced dash). A clause is skipped when it
says what is so already, what is to come, or what was done elsewhere: it
contains one of *already, still, unchanged, untouched, no change, not
changed, no edit(s), nothing to, need(s) no, needed no, no need, not needed,
did not, didn't, does not, doesn't, will, must, should, need(s) to, next,
later, before, earlier, per, todo, pending, left for*. In the clauses that
remain, only two shapes count:

1. **"`<file>` now `<verb>`"**: a file name (optionally with `:line` or
   `:from-to`) followed by "now" and one of *says, reads, returns,
   documents, has, calls, uses, prints, shows, takes, lists, includes,
   contains, handles, checks, gives, refuses, accepts, is, does*.
2. **A past-tense change word, then the file** (or the file alone before the
   word, as "stats.go updated"): *added, created, wrote, written, edited,
   changed, updated, fixed, renamed, removed, deleted, replaced, moved,
   implemented, inserted, appended, bumped, refactored, rewrote, rewritten,
   converted, documented*. With *added, created, wrote* or *written*, the
   claim is that the file **was made**.

A file name is a path-like word whose extension begins with a letter (so
`v1.4` is not one). A URL is not one.

**Where the file is.** Under the working directory as written; else an
absolute path that exists; else a file of that base name that the session's
calls named and that exists; else a search of the working directory for a
path ending with it (at most 20,000 entries; skipping version-control,
dependency and the harness's own data folders).

**Backed** when, in the window, any of:

- an edit, multi-edit, write, symbol-replace or rename tool call **that did
  not fail** changed a path ending in that name;
- the file's modification time **or** its change time is at or after the
  window's start (a shell command changed it; "git mv, cp -p keep mtime", so
  both are read);
- a view of it, by the view tool or by a shell command that showed it.

**Refusals** [F]:

```
not popped: nothing in `x` changed or showed `README.md`, which your result says changed. If the change is still to make, make it; if it was made before, view it here so it shows; then close `x` with what came of it. If you did it before you entered `x`, step out with mem.Leave("done before") and close it by its name from where you did it: mem.Done("x", "what came of it").
```

```
not popped: nothing in `x` made `stats_test.go`, which your result says was made, and there is no such file. Make it, then close `x` with what came of it; or close it with what you did instead.
```

- The second form is used when the result says the file was made, no such
  file is found, and its extension is a known source or text kind (go, gno,
  mod, sum, md, txt, json, yaml, yml, toml, py, js, ts, tsx, jsx, rs, c, h,
  cc, cpp, java, rb, sh, html, css, sql, proto, csv, xml, ini, cfg). A claim
  that names something else ("client.Get") is no claim of a file.
- For a close by name the verb is "closed", not "popped", and the last
  sentence (the Leave hint) is left out.

Why a list of tools was not used: "`go test` would have backed every claim,
and the read-only list lacked sed, find and go."

"A claim that names no file ('Median added, tests pass') is left to" the
review.

### 3.4 The review (option, on by default; one model call per todo closed)

Skipped when `mem_review_closes` is false. Otherwise the **small model** is
asked, with no tools, **thinking off** where the provider has a switch, at
**temperature 0**, with room for **1,500** answer tokens and a **3-minute**
timeout.

**What it is shown**, in this order:

1. The todo (its title, else its id).
2. The thoughts kept on it ("(none)" if none).
3. What the user asked in this turn, when the window holds none of the
   user's words (clipped to 2,000 characters).
4. **A header the harness computes itself**: the files the agent's edits and
   writes changed in the todo ("none" if none); the commands it ran there,
   each with its exit status ("`go test ./...` → exit 0", or "failed to
   run"). "So a long todo's cut transcript does not hide them."
5. The result it is being closed with, labelled as the agent's claim.
6. When the input itself entered and closed the todo: that call's code
   (clipped to 4,000 characters).
7. **What happened in the todo**: a transcript of the window's rows: the
   prompts typed, what the agent said and called, what its tools returned, a
   failed call's result marked so. Each part is clipped; if the whole is
   over **24,000 characters**, clip harder (three levels: user / agent /
   call input / result at 2,000 / 4,000 / 2,000 / 2,000, then 1,000 / 2,000
   / 1,200 / 800, then 500 / 800 / 600 / 300), and only then cut, keeping
   the end.
8. **The question, which names the todo**: is the result what came of *this
   todo*, and not of other work; and does what happened in the todo show it?

**What the judge must be told.** The requirements below are restated from
the fork's own judge prompt (a file upstream never had; its wording was
tuned by replay through four versions). Phrases in quotation marks are that
prompt's own [F]:

- It checks whether an AI coding agent did the todo it is closing as done.
- "In the todo" means everything since the todo's plan was made, not only
  since the agent entered it; work done before entering counts.
- The lists of files changed and commands run are the harness's own: trust
  them.
- **The close holds** when the result says what came of this todo and what
  happened shows it:
  - a file the result says changed was changed by an edit or write in the
    todo, or a command there that changed it, or a tool's output shows its
    new content. "When the change was made before what you are shown, a
    view, a diff or a passing test in the todo that shows the new content is
    enough" (fix C);
  - a command, test or check the result says was run shows as that call and
    its output. "A test run that prints ok, cached or not, shows that every
    test in that package passes";
  - a finding shows through the tool output it came from, or a thought kept
    on the todo;
  - an answer, explanation, decision or review the todo asks for holds when
    the agent gave it in the todo;
  - "nothing needed doing" holds when a tool's output shows that;
  - the user's own word that it is done, or to be closed, counts;
  - a sub-agent's report counts as a tool's output.
- **It does not hold** when:
  - the result says the work is not done, not done yet, failed, could not be
    done, or is left for later, "however true that is: a todo closed as done
    must be done" (this rule went to the top after the one slip in replay:
    the review had passed "README.md still references Fetch (not done
    yet)");
  - the result reports other work: "a package, file or test other than what
    this todo is about is other work, however well what happened shows it";
  - the result is not shown: "what the agent wrote, in its calls, its
    results or its words, never shows anything by itself";
  - a thought on the todo asks for something the work misses ("a thought
    that only notes something asks for nothing").
- Line numbers and small details need not be shown exactly. Remarks on other
  todos need not be shown. Judge only this todo, not whether its work was
  the best way.
- Answer with one JSON object on one line and nothing else:
  `{"ok": true, "reason": "what in the todo shows the result, in one
  sentence"}` or `{"ok": false, "reason": "what is not shown or still
  missing, in one sentence, said to the agent"}`.

Read the verdict as the JSON object anywhere in the answer, past any
thinking tags.

**Refusal** [F]:

```
not popped: the check of `x` found: <reason>. Do what is missing; if it is done already, show it here (view it, or run what checks it); then close `x` with what came of it. If you did it before you entered `x`, step out with mem.Leave("done before") and close it by its name from where you did it: mem.Done("x", "what came of it"). If it cannot be done now, mem.Leave("why") leaves it open; either way, tell the user plainly how it stands.
```

("not closed:" and no Leave-before hint for a close by name.)

**When the review cannot run** (no model, an unreadable verdict, the
timeout): **fail open, loudly.** The close is kept unchecked and the user is
warned: "The close of `x` could not be checked, so it was kept unchecked:
<why>". A cancelled turn is not a pass: "not popped: the check of `x` was
cancelled, so nothing was kept".

**The verdict cache.** A verdict is kept by (session, todo, result, the last
row of work in the window, the closing call's code when the input entered
it), a few hundred entries. So "a close sent again unchanged after a refusal
is not reviewed again", and a call that failed for another reason after an
approved close is not reviewed again when resent. New work makes a new key.

### 3.5 Otherwise

The close is approved.

---

## 4. The refusals, and what reads them

All refusals begin **`not popped:`** or **`not closed:`** followed by one of
"nothing came between your entering `x`" (mem's fresh guard), "this call
entered `x`", "nothing in `x`", "the check of `x`". Three things in the
harness recognise a refused close of a named todo by that shape:

1. the fresh guard's definition of work (`frames.md` §9: once a close of the
   frame was refused, an answer to the user is no work);
2. the loop nudge (§5.2);
3. the turn-end send-back (§5.1).

Keep the shape stable, or change all four together.

---

## 5. Around the check

### 5.1 The turn's end sends the model back, once

If a turn would end while the session stands in a frame whose **last attempt
to close it in this turn was refused** (by mem's fresh guard or by the
check) and no later attempt was kept, the agent is sent back **once per
turn** with a hidden prompt, and the user is told.

Why (A19): "the model did no work, tried to close the todo, was refused by
mem's fresh guard, and told the user 'Fixed', with made-up timings and an
invented endpoint. Mem stayed true; the user didn't." And it sends back
"even when it did work after the refusal: one look after a refused claim
counted as work, and the turn ended with the todo open and the user told it
was done".

Two forms [F]:

- Refused as never done, and nothing done since: user notice "mem refused to
  close `x`, since nothing was done in it; the agent goes back to it";
  prompt:

  ```
  mem refused to close `x`: nothing was done in it, and it is still open. Do its work, then pop it with what came of it; or, if it cannot be done now, tell the user plainly that it is not done. Do not tell the user it is done.
  ```

- Otherwise: user notice "The check refused to close `x`, and the turn ended
  with it open; the agent goes back to it"; prompt: "The close of `x` was
  refused, and it is still open: <the refusal line>" then the same two
  sentences.

Order at a turn's end: recover from an empty answer first; then this
send-back; then the goal driver; then the user's stop hooks.

### 5.2 The loop nudge

The harness's loop detection must be able to see a loop of mem calls. Two
fixes (A27):

- Every mem receipt ends "ran 31ms · kept as entry 23", so a repeated call
  never looked the same. **Blank the numbers in mem's answers** before
  comparing (only in mem's).
- Two calls going back and forth fill exactly half of a ten-step window.
  **Count a ten-step window with only one or two distinct steps as a loop.**

A loop whose repeated step is a refused close gets its own nudge: nothing
was done in the todo (or the check found something missing), so its work is
still to do: do it, or tell the user it is not done. **With no cap that ends
the turn** (Jae rejects limits that end work on a counter). The nudge quotes
the refusal. Seen without it: the same refused close sent about 300 times in
40 minutes, ignoring 30 generic "do not repeat" nudges.

### 5.3 Other fixes that landed with it

- A failed tool call is no work for the fresh guard.
- A thought on the todo itself is no work; a todo entered, thought on and
  closed in one call is reviewed, with that call's code shown.
- The closing call's own round (an edit sent beside the pop) is in the
  window.
- A todo whose todos were all dropped is checked; the user's answer to a
  question is not.

### 5.4 A reviewed task is not judged twice

When the review approved the close of a **top-level** todo with nothing
under it, and mem then kept the call, the turn-end goal judge (`mem.md` §13)
skips that todo, as long as its result is still the one approved. The
harness learns whether the call was kept through a callback after every
checked call; an approval in a call that was then refused or failed is
forgotten.

### 5.5 What the model is told about it

In the system prompt [R]: write a todo's result after its work, from what
the tools returned in it (the edit made, the command run and what it
printed), never what it means to do; each close is checked against what
happened in the todo, and refused when nothing there shows it. A todo that
needs nothing is closed by its name after the check that shows it:
`mem.Done("…", "nothing to do: …")`.

---

## 6. A11: the hold on a plan is soft

The other half of Jae's "do both".

**Before:** a session that pushed a plan's task and ended inside it kept it
held for good, so a later session could finish every todo but never close
the task ("3/3 done", open forever). Jae, 2026-10-31: "just leave it as is,
we will solve it differently later." Seen in every multi-session plan.

**Since 2026-11-05:** "A todo with todos under it is coordinated, not done."
Its hold stands **only while the holding session is working** (in a busy
stretch: its turn lock is held), or for that session itself. So:

- a plan pushed in one session and finished in another **closes**;
- a new session's `mem.Next()` takes the task's free todos instead of
  skipping the task as held;
- while the holding session is busy, nothing changes.

**A todo with nothing under it stays hard-held** "until its session lets it
go" (Jae's 2026-10-31 ruling, kept for leaves). So a leaf an ended session
stepped into still waits for that session: "in one run of the four-session
plan, the last session could not take the step the one before it had
entered". The way out for the user is to resume that session.

It is computed at read time from the turn locks (`peers.md` §6); nothing is
stored. **Where the harness cannot tell who works** (no data directory, or a
platform without the lock), **every hold stands.** A todo entered on the way
by Next from the root (`Via`) was already soft the same way.

The integration point: the harness must hold each session's turn lock while
it is busy, or plans never free up (`integration-contract.md` §7).

---

## 7. The option

`mem_review_closes` (boolean, default **true**; rc: `option mem-review-closes
false`). The design's words for it [D]: "Before a todo is popped as done,
the small model checks its result against what happened in the todo, and
refuses the close when nothing there shows it. One model call per todo
closed. false skips it; the free checks still run." The free checks are the
fresh guard, entered-here and the claim check.

(The config guide of the earlier tree listed `mem-embeddings-url` but not
this one; the option existed in the config and its schema. List both.)

---

## 8. Evidence

**Replay of every past close** (2026-11-05): all 244 accepted closes of
leaf todos in the QA runs of 3–4 November, mostly thinking off, sent through
the review with the box's Qwen.

- About **1 close in 9 was false or wrong**, and every known one was
  refused: no work in the todo, a failed edit, a view that showed the old
  text, another todo's result recorded, a result saying it was not done, a
  claim the todo's own output contradicts.
- About 12 claimed a check that never ran ("go build passes", "tests pass"
  with no run), "which a refusal makes the model run".
- The final prompt **wrongly refuses about 1 in 60**.
- A review takes **9–15 s** on the box, up to 38 s when busy.

**QA on the branch** (thinking off): four runs, every check passed. The two
scenarios the false closes came from "still wrote results before the work
(2-3 times a run); the guards refused them and the model then did the work:
no false close got through". In one run the review "refused three closes
that recorded another note's finding, and each was then recorded right; 19
reviews there, median 15 s, at most 38 s, 5.4 minutes of a 47-minute run".

**Bench** (2026-11-06/07): refused closes were the main cost of gnosh on
short tasks. A model "planned 9 todos with Push, worked in the plan root
without entering any, and then the close check refused its closes": the
tests were green at 684 s, the run ended at 1,667 s. Mem calls refused: 46%
on gpt-oss, 25% GLM, 23% Gemma. Hence fixes A and C.

**Fix A, before and after** (same GPU, six bug fixes): 223 s and 21 requests
with it; 791 s and 53 requests before; refused closes fell from 4 and 9 a
run to 1 and 0. With A, the reviewer still refused a todo popped before its
edit three times: "a real false claim, rightly refused".

**The swarm fix** (2026-11-11): a session called `mem.Next()` and, in the
same reply, opened the files of the todo it guessed came next; Next gave it
another; it worked the guessed one and closed the given one with that work,
and the review passed it. Replayed as the harness sends it (thinking off),
the old question passed the bad close 5 times in 10; the new question, which
names the todo and asks first whether the result is its own, refused it 10
of 10 and passed the 7 good closes beside it. On the last build 47 real
reviews refused nothing. Real reviews used a median of 55 answer tokens, 91
at most.

**A lesson recorded with it:** "replay a judge with the exact provider
options gnosh sends […], not just the prompt". A first replay left the
thinking-off switch out, the model thought, hit the old 400-token cap, and a
wrong story ("9 of 10 cut off") reached a commit message. The 1,500-token
room is headroom for a server that thinks anyway, where a review cut off at
its limit would leave the close unchecked.

---

## 9. Known limits

- "A session that cannot take its turn lock (a filesystem with no flock)
  has its task holds lapse mid-turn."
- With the review off, a view of the file backs a claim it does not show
  ("README.md:3 now says Get" after a view): "only the review reads what the
  view showed". Search output is not a view.
- "The review is a separate request: on the box each one is a new prompt for
  the server's RAM cache, so it may push out a saved session."
- A shell change in the window's first second can be missed.
- On a provider type whose thinking switch the harness does not know, the
  judge thinks; a review cut off at its limit keeps the close unchecked (the
  deliberate fail-open).
- One false refusal seen: an aside "owner.gno is unchanged" whose view had
  answered "unchanged since your last view", so the review saw no content.

## 10. Proposed, not built

- **H**: skip the review when every file the result names has an edit in the
  window and a test passed ("the review is about 5% of gnosh's GPU; the
  refusal loops and the step-per-reply are the rest").
- **D**: a push of a new plan also enters its first todo (prototyped; broke
  two peer tests; "do it in the host for the planning reply only, if at
  all").
- A pop's result checked against its todo's title for a mismatch (A13); the
  review's "other work" rule covers part of it.

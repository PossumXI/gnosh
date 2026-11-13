# peers.md — many sessions on one mem

What this file is: how several gnosh sessions, each in its own process,
work one plan in mem at once: the lock, order and `.Parallel()`, which todo
a session gets, when `mem.Next()` waits and how it is woken, who counts as
working, what a session is told, the guarantees, the failure cases, what was
measured, and how to run one.

Almost all of it is inside the carried code (the Gno library decides; the
mem host keeps peers records and rings pipes; the client waits). **What the
harness must do is small and exact**: hold each session's turn lock while it
is busy, mark sub-agents, never cancel what a kept call did, let a waiting
call wait, and tell the model the rules (§10). Sources: PARALLEL.md draft 4
with its status table and three review rounds, GNOMEM.md "Many sessions, one
plan", the peers memory note (the box and H100 runs), BENCH-SWARM.md,
https://gnosh.ai/peers.html.

---

## 1. What Jae asked (2026-11-01)

"make a plan for demonstrating parallelism in a tree of frames and todos and
sub todos, but taking into account concurrency and need for locking...
assume ... many peer sessions ... that they take turns locking a central
lock for transactionally manipulating the shared mem Gno tree store...
instead of timeouts and retries ... the 'right' solution of waiting and
asynchronous like epoll ... efficient ... and demonstrate that it works, is
faster than a single session going linearly, though in either synchronised
or parallel execution they both complete the same task because the mem
system works like that."

In plain words [D]: "Several gnosh sessions work one plan at once. The plan
is the tree of todos in mem. Each session asks mem for its next todo; mem
gives each a different one, because every call to mem takes its turn on one
lock. Todos stay in order, as they are today, except under a todo the plan
marks as parallel: its todos may be done at once, by different sessions, and
its own work (the check, the merge) comes after them. A session that has
nothing to do now, while another session is working on something it will
need, sleeps until mem changes, and then carries on. Nobody guesses how long
to wait and nobody asks again on a timer. One session alone does the tree as
it does today; three do the parallel parts at once, faster, and both end
with the same tree done."

---

## 2. One lock, waited for in the kernel

- There is one mem per project, shared by every gnosh process in it
  (`mem.md` §2), with one `lock` file in its directory.
- **Every mem call is a transaction under that lock**: take the lock, bring
  the store up to the journal, run, append to the journal, commit, let go.
  "So when two sessions ask `mem.Next()` at once, they get different todos."
- **The lock is waited for with a blocking file lock, "woken by its
  release"**, with no cap and no polling. The caller waits on the lock and
  on its own cancellation at once, so Esc stops the wait. A wait given up on
  must not leak: the helper closes the file once the kernel grants the lock.
- A worker request waiting for mem's lock says so in its heartbeats, and the
  client starts the request's deadline again when the wait ends.
- A catch-up that spends its time slice says "call again", and the client
  calls again at once; the model never retries.
- Esc during any mem call "sends the cancel and still awaits the reply, so a
  committed pop or push is never lost": a call still waiting for the lock is
  cancelled before it holds it; one past the lock finishes within
  milliseconds.

**What the file-lock helper must offer** (it is the harness's to supply; the
carried code calls it): a try-lock that returns at once with a "contended"
error and otherwise a release function; and a lock that waits in the kernel,
takes a context, and returns a release function. On a platform with no
blocking file lock, polling is acceptable ("Windows keeps its poll").

---

## 3. Order, and `.Parallel()`

- **Todos under a todo are in order**, as for one session. Every plan the
  prompt teaches puts its check last, so order is the default. (The first
  draft made siblings independent by default; the review found that "peers
  would run checks early".)
- **`f.Parallel()`** says the todos under `f` "may be done at once, in any
  order, by any session; what must follow them is that todo's own work, done
  before it is popped". Show marks it "its todos at once".
- **The rule, stated honestly:** "in an ordered todo, a todo does not start
  while one before it is being worked on, at or under it, by a session that
  is working now. Todos before it that wait on the user, on a day or words,
  or are held by a session not working now, are passed over as today."
- **The invariant:** "in an ordered todo, no todo starts while one before it
  is being worked on; and a todo is popped only after every todo under it."
- **Marking each leaf is refused.** An input that makes several sibling
  todos and marks each `.Parallel()`, with nothing under any, is refused and
  told which todo to mark: there "the mark does nothing and the plan would
  run in single file" (1 plan of 13 in the swarm bench did this; five
  helpers waited 12.5 minutes).

---

## 4. Which todo a session gets, and when Next waits

**One pick** decides behind Next, NextTodo, the next hint, Show's `← next`,
the receipts and the JSON summary. Going through the open todos under the
starting point, in outline order [D]:

- asked of the user, or waiting for a day or words: passed over;
- held by another session that is **not working**: passed over, with
  everything under it;
- held by another session that **is working**: "looked into for a free todo
  you can take (helping, by that todo's own order); if there is none, in an
  ordered todo the ones after it wait, in a parallel one the search goes
  on";
- free (or yours) with nothing open under it: this is it;
- free with todos under it: at the root, its first leaf you can take; in a
  frame, entered, "unless all that is left under it is working peers' work,
  in which case it counts as being worked on".

Todos beside the starting point come first; only when none is free does a
session take a free todo under one a working session holds.

**"With no other session working, every branch is today's."** One session,
and a later session taking up an ended one's plan, see exactly the
single-session Next, with the same texts. This was a hard requirement: every
recorded example and golden file had to stay unchanged.

**From the root**, Next enters the todo together with the todos above it,
entered *on the way* (`Via`), not held: "they are the session's while it
works, and anyone's once it stops", so others take the free todos under them
"but neither it nor its close". Under a `.Parallel()` todo nothing is
entered on the way, so "the parallel todo's own work goes to whichever
session comes when its todos are done".

**When Next waits** [D]: "Next waits only when it has nothing for you now
(no todo, and the frame you stand in still has open todos), and you would
have a todo, or could pop that frame, once the other working sessions finish
their work." "Finish" is computed bottom-up: a todo held by another working
session counts as finished once nothing open is left under it that does not
count. "A free todo, the waiter's own, and the todos of sessions not working
never count." So:

- a lone session told "pop it" never waits (nothing would wake it);
- a session at the root waits while a group's own check is still to come;
- it ends when only other sessions' own pops are left.

Conditions, all required:

1. **Only an input whose last statement is a bare `mem.Next()`** may wait,
   and only when that is the input's only `mem.Next()`. The host reads this
   from the parsed input. Any other Next answers at once.
2. **A sub-agent's never waits**: it would wait on its parent's hold while
   its parent waits on it through the agent tool.
3. The input's earlier statements commit as usual.
4. **A wait that would close a loop is refused**: the waiting transaction
   records whom it waits for in the session's peers record and checks the
   other records; a loop of sessions waiting for each other says why
   instead.
5. A todo held by a session that is not working (its turn ended, or its
   process died) is never waited for.

The waiting Next prints nothing and is a no-op push, so no hint follows.

---

## 5. Waking: one pipe per wait, no timers

"Whatever waits, waits in the kernel or on a wake-up, never on a timer."

- Before an input that may wait, the client makes a named pipe under
  `mem/waiters/new/`, opens both ends (the read end non-blocking so the
  runtime's poller parks the reader; a write end of its own, so an empty
  pipe reads as "nothing yet" rather than its end), then renames it into
  `mem/waiters/`. The input carries the pipe's name (`RunRequest.WaitPipe`).
- **The transaction that decides to wait activates the pipe** by renaming it
  into `mem/waiters/active/`, after it has rung the others, "so it never
  rings itself".
- **Every journal writer rings**: a call's commit, an erasure, an undo, and a
  call that changes a session's recorded stack. After its write, under the
  lock, it opens each active pipe write-only and non-blocking and writes one
  byte. "A full pipe means its waiter is rung already; no reader means its
  waiter is gone, and the pipe is removed. It never blocks."
- "So a waiter wakes once per change after it decided to wait, for nothing
  else, and misses none: a change either came before the decision (the
  decision saw it) or after the activation (it rings)."
- **A holder's turn ending, or its process dying**, also wakes: the waiter
  waits in the kernel, in shared mode, for each holder session's turn lock.
- **Esc** wakes the wait and returns at once: "stopped waiting for … after
  41s: the turn was stopped".
- If making or activating the pipe fails, Next answers at once as for one
  session.

A named pipe was chosen over file-system notification after review: watch
events "can repeat", watches die after an erasure's rename, and on one
platform a directory watch opens every file in it, the database files
included.

**After a wake** the client runs `mem.Next()` again as a new transaction,
with the stack the earlier ones left, a new request id (`<call id>/next-<n>`),
and a fresh pipe; it loops "until Next steps in or has nothing to wait for".

**One tool result.** The model gets the outputs in order, one line per wait
[K]:

```
waited 41s for `geom/square/area`, session 3c4d…
```

(waits with nothing between them share one line with their total), every
transaction's frame events in order, and the merged notes, rules and task
list. "Once anything committed, the result is never an error": a later
failure or Esc is said in its text ("(mem.Next() again failed: …;
mem.Show() says where you stand)"). A re-run kept though its worker died
before replying still delivers its frames.

"One wait costs no CPU and no model tokens: the call simply returns later."

---

## 6. Who is working: turn locks and peers records

**The turn lock.** `<gnovm data dir>/turns/<hash of the session id>.lock`.
The harness holds it, exclusively, **from when a prompt makes an idle
session busy until the session goes idle**:

- taken when a run starts; released only when the session's queue is empty
  and it stops;
- **a run for a queued prompt, a goal continuation or a retry, started from
  inside the run before it, shares the hold** (counted per process); taking
  it again there would wait for itself. Release last, after that hand-over,
  "so there is no gap between them";
- waited for with the run's own cancellation (a probe holds it for a
  moment; a session open in two processes waits behind the other and logs
  that it waited);
- never unlinked; the kernel drops it if the process dies;
- **idle compaction does not take it**; a sub-agent takes its own;
- if it cannot be had, carry on: "the session is then not seen working".

The carried `Lazy.HoldTurn(ctx, session)` does all of this and returns the
release function; the harness's job is to call it at the right two moments.

Why a turn lock and not process liveness: a session idle between turns would
make peers wait forever.

**The peers record.** `mem/peers/<hash>.json`, written under mem's lock by
each of a session's own `gnovm_run` calls that changes it: the stack the
call left, what its Next waits for, and on which pipe. Only the agent's own
tool calls keep it; the harness's own reads for a session (the goal table)
leave it alone. An erasure rewrites the names in it.

**Working** [D]: "A holder is working on a todo when its turn lock is held
and its last recorded stack … has the todo, or one above it, on it." For the
soft hold on a plan (`close-check.md` §6), working means "in a busy stretch
at all".

**Where it cannot be told** (no data directory; a platform without the
lock): nobody counts as working, every hold stands, and Next never waits.

---

## 7. What a session is told

By mem's receipts [K]:

- **Entering a todo whose todos other sessions did**: what each came to, one
  shortened line each, five at most: "its todo `x/y` was done by session
  ab12cd34 → result", then "and N more done by others: mem.Show("x")". The
  same before "pop it with its result". So "the session that pops it, and
  the judge that checks it, see the work".
- **Entering a todo while others work the plan**: "<sessions> also work on
  this plan now: keep to this todo's files, and take the next todo with
  mem.Next()." (Not said to a sub-agent.)
- **After a pop while others work**: the hint never names a todo ("another
  session may take it first"): "next: mem.Next(), by itself, which says
  which todo is yours while <sessions> work on this plan: it may not be the
  one after this, so read which before you open a file".
- **A pop refused because todos under it are open** names the sessions
  working on them and says "mem.Next() steps into a free one, or waits for
  them".
- **After a wait**: how long, for which todos, which session.
- Show marks a todo "held by session …, 5m ago", or "session … is in it" for
  one entered on the way.

By the system prompt (`mem.md` §14.1 item 8) [R]: other sessions may work
the same plan at once; plan independent todos under one todo marked
`.Parallel()`; step in with `mem.Next()`, by itself, never by a name; keep
to your todo's files; before ending a turn, finish the todo or drop it with
why; a bare `mem.Next()` ending the call waits for the others; a todo marked
done that is not is reopened and taken with `mem.Next()`, and "fix nothing
outside a todo, or the others fix it too".

---

## 8. Guarantees, and how each was checked

From the suite's demonstration, which runs **real processes** on one mem
(the test binary started N times; a child never starts another), with a
sleep for each todo's work, asserted from the journal:

- **Each todo is taken by one session at a time and done once.**
- **Parents close after their todos; ordered todos in order.**
- **The same final tree for every N** (the JSON summary without times and
  holders).
- **No peer waits while a todo it could take is free** (each wait decision
  is logged with what it saw).
- **Re-runs per wait are no more than the changes by others during it**; "a
  quiet holder (3 s, no change) causes none".
- **No peer ends while work that would later be its own remains.**
- **A holder killed mid-work, or whose turn ends holding: waiters wake**
  (within two seconds) "and are told it is not working".
- **A process killed between its journal append and its store commit: the
  next call catches up.**
- **A sub-agent never waits. A would-be loop is refused.**
- Deadlock is impossible for stacks made by Next; cross-branch pushes are
  covered by the loop check.

Times (twelve todos: three groups of two parallel leaves each, then the
build, then the notes, in order; 1 to 2.2 s a leaf) [G]:

```
1 peer              13.1 s
3 peers              7.0 s   1.87x   (their rule's own schedule: 6.9 s)
8 peers              5.7 s   2.30x   (5.5 s; the tree's longest chain is 4.5 s)
a planner + 3 peers  6.3 s   2.08x
```

The bar for each run comes from a simulation of the same pick over the
measured step times; each run must come within a small slack of it.

---

## 9. With a real model (what the runs showed)

**On the AI box (one GPU, three server slots, 2026-11-01): no reliable
speedup.** "Every run ended with the same tree and all checks passing; none
showed three sessions reliably faster than one." One session took 17m30s,
14m22s and 8m19s on the same tree; three took 18m28s, 13m59s, 21m56s,
10m34s and 14m21s. Why: three streams on one GPU decode at about 1.8× in
all; the server's prompt cache held about one prompt, so two of three peers
starting at once read their ~16K-token prompts in full; and "the model
varies more than the gain".

**On a rented H100 (four slots), 2026-11-01, nine real Go packages in three
groups, every run 12/12 checks:**

| Run | One session | Three sessions | |
|---|---|---|---|
| 1 | 20:51 (73 replies, 94 tool calls, 5 pops refused, 1 reopen) | 9:28 (81 replies, 96 calls, 2 refused) | **2.2×** |
| 2 | 35:38 (64 replies, 102 calls; an eval failure found at the final check cost 8 minutes) | 10:21 (86 replies, 104 calls, 6 refused) | **3.4×** |
| control: each group alone, nothing shared | wall 18:52 | | |

"The total work is about the same on both sides; the three split it, and two
slept on their pipes 39-57 s near the end, then took the group and task
checks." Cost: about $13.90. The page: https://gnosh.ai/peers.html.

**Six sessions on one 48 GB GPU (vLLM, 2026-11-11), same nine packages,
every run passed:**

| Harness | Sessions | Runs | Median | Fastest | Requests a run |
|---|---|---|---|---|---|
| gnosh | 6 | 9 | 500 s | 275 s | 95–129 |
| Crush | 1 | 4 | 448 s | 410 s | 11–13 |
| gnosh | 3 | 2 | 623 s | 554 s | 105–107 |
| gnosh | 1 | 4 | 794 s | 713 s | 23–74 |
| Codex | 1 | 2 | 934 s | 628 s | 7–21 |

"Six sessions finished 1.6 times faster than one at the median… Against
Crush they come out about even: Crush batches, writing several packages in
one reply, where a Gnosh session takes a step a reply." On a smaller job
(six one-line bug fixes) Crush won, 129 s against the six sessions' 196 s.
The server: one stream 30 tokens/s, six at once 167 in all. Jae's reason
for the run: "people who already have a lot of ram now theoretically can
benefit from the swarm aspect".

**Failure modes the runs found, and their fixes:**

| Seen | Fix |
|---|---|
| A model "popping" a todo right after entering it, copying the row of instant pops its cut context showed (10 reopens in one run) | the note on a popped push's answer (`frames.md` §4.4) and the fresh guard (§9) |
| An "autopilot cascade": six todos "finished" in 40 s with invented results | the fresh guard in mem |
| After a refusal, `mem.Show()` then the same pop, accepted | the guard counts only work |
| `.Parallel()` on each leaf | refused (§3) |
| A session opening the files of the todo it guessed was next, in the same reply as `mem.Next()` | the review names its todo (`close-check.md` §8); the hint and the prompt say to call Next by itself |
| Several peers closing the same group by name instead of taking it through Next | the prompt rule; the holds |

A stale-write guard on the edit tools "saved that session's file" when a
peer worked a todo another held. Keep that guard.

---

## 10. What the harness must do

1. **Hold the turn lock** for each main session from busy to idle (§6), and
   for each sub-agent session its own.
2. **Mark sub-agents**: every mem request from a sub-agent's session carries
   `SubAgent = true`.
3. **Pass the session id and the stack** with every mem request (it is how
   the peers record is kept).
4. **Let a waiting call wait.** A `gnovm_run` call may legitimately take
   minutes while its Next waits. Do not put a tool timeout on it; Esc ends
   it. Show the user that it is waiting (unconfirmed how the earlier UI
   showed this).
5. **Apply a kept call's frame events and side effects even when the turn
   was cancelled** (`frames.md` §11).
6. **Give each process its own session store, or tolerate lock retries.**
   Local runs sharing one session database "wait with SQLite's own busy
   retries"; the demonstrations gave each peer its own data directory.
7. **Tell the model the rules** (§7).
8. **The file-lock helper** of §2.

There is **no command-line flag for a swarm** (open for Jae). It is run by
hand or by the bench script:

- In one terminal give gnosh the task and say others will help [G]: "Two
  more gnosh sessions will work on this with you. First plan it in mem as
  one todo marked .Parallel(), with the parts that do not depend on each
  other as its todos."
- Once it has planned, open gnosh in the same project in more terminals and
  tell each: "Help with the plan in mem: take todos with mem.Next() until
  none are left."
- The bench waited for the planner's first successful `mem.Todo` call before
  starting the helpers, and told them "Do not plan the task again."

---

## 11. Known limits

- "A day-wait ending during a wait is not a change: the waiter wakes at the
  next one."
- "'Working' is a busy stretch: a session holding a todo while busy with
  something else is waited for."
- A parent whose sub-agent holds a todo sees the peers' branches, and may
  wait for it (sub-agents never wait, so it cannot loop).
- No way to un-mark `.Parallel()`.
- "The newest open task" was the most recently changed one; since layout 6
  the root goes by creation order.
- In one gnosh process, a lock wait holds up that process's other VM calls
  (the worker runs one request at a time).
- A model planning alone gets no feedback for `.Parallel()` until peers work
  its plan.
- Esc on a mem call waits for the worker's step; `Did` after a reopen; the
  `waiters/new` directory is never swept; a session started with its own
  data directory "usually misses the model server's prompt cache (cause not
  found)".
- **A swarm waits for its slowest session** (one reply of ~4,700 tokens held
  two rounds up by minutes).
- **Local mem's throughput**: about 2.7 calls a second at a 1,011-todo tree
  (`mem.md` §18). Ten peer processes are about 360 MB each.

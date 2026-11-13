# swarm.md — mem as a realm on gno.land

What this file is: the shared plan as a realm on a gno.land chain, so that
gnosh sessions on many machines can work one plan. What the realm does and
promises, its calls and limits, leases, events, the chain client, the demo
runner, every measured run, what is **not** built (syncing a local session's
mem with the realm), the design recorded for that, and the unlanded
frames-on-chain branch.

The realm (`gno/r/gnosh/mem/`), the chain client (`internal/gnochain/`) and
the demo runner (`internal/swarm/cmd/swarmdemo/`) are carried over as they
are. **Nothing in the earlier harness called them from a session**: the
swarm is, today, a realm plus a model-free demonstration. So this file has
almost no integration contract; it is here so the clean side knows what
exists, what it guarantees, and what the next step was meant to be.
Sources: SWARM.md draft 3 with its status table, SWARM-reviews.md, the swarm
memory note (2026-11-01 to 11-05), the gas-rewrite review note, the realm's
own source comments, https://gnosh.ai/swarm.html, and a memory note on the
frames realm (2026-11-06).

---

## 1. What Jae asked (2026-11-01)

"ok next step is to demonstrate massive parallelism coordinated by gno.land
as the base for mem realm; and a full featured demo so then local gnosh mem
interacting with synced gno.land mem realm (this part will be tricky), to
allow for massive swarm coordination."

In plain words [D]: "On one machine today, sessions take turns holding mem's
lock… To go past one machine, the shared plan moves to a realm on gno.land.
The chain is the lock: calls run one at a time in block order, so 'take the
next free todo' is decided inside the transaction. When nothing is free, a
session's Next puts it in a queue on chain, and the realm hands the next
todo that frees up to the oldest waiter in that same transaction, so a
finished todo wakes one session, not all of them."

"Thoughts, facts and the frame stack stay local; only the plan, who holds
what, and results go on chain, and all of that is public."

---

## 2. What is built, and what is not

| Piece | State |
|---|---|
| The realm: plans, todos, holds with leases, waiter queues, events | **built**, on main; 24 realm tests; module `gno.land/r/g1ecsuj0q572jr0dhu29q9njtnmw03hyu7tyyvv6/mem` (Jae's address) |
| The chain client: dial, query, eval, call, sign in process or through `gnokey`, a sender for many calls per transaction, a block follower | **built**, on main; shared with the Immune project |
| Demo A: the chain's own limits, no model, 1 to 1,000 workers, failures injected | **done**, three times over (2026-11-01, 11-04, 11-05); page https://gnosh.ai/swarm.html |
| **gnosh's local mem synced with the realm** (a session's `mem.Next()`/`mem.Pop()` on a shared plan becoming a transaction; one watcher and one renewer per machine; lost holds) | **not built. Stopped.** See §8. |
| Demo B (real sessions through the chain) and demo C (sync under failures) | not started; they need the sync |
| A realm for frame trees on chain, and `gnosh frames export` | built on a **local, unpushed branch** (2026-11-06); see §9 |

"Everything in a shared plan is public: titles, holders and results."

---

## 3. The realm

### 3.1 The promise

"A plan is a tree of todos. Next gives the caller the next free todo inside
the frame it stands in, held by a lease, in the same order gnosh's local mem
picks ('first free leaf, enter-mode in a frame'). PopNext pops the caller's
todo and gives it the next one in the same call. Every call is one
transaction and the chain runs them one at a time, so two callers never get
the same todo. When nothing is free but some todos are still held, the
caller waits in a queue on chain, and the realm hands the next todo that
frees up to the oldest waiter of that frame, in the call that frees it. A
hold whose lease runs out comes back on the next call." [K]

**Every todo is done exactly once.** In every measured run, failures
included, "every todo [was] done once and the same result digest as the
control".

### 3.2 The calls

Each call that changes a plan is one transaction. `cur realm` is gno's
crossing argument. `label` is the agent's label (the gnosh session id).

```go
func NewPlan(cur realm, id, title string, open bool, lease int) string
func AddMember(cur realm, plan, addr string)
func AddTodos(cur realm, plan, label, parent, names, titles string) string
func Parallel(cur realm, plan, label, id string)
func Next(cur realm, plan, label, frame string) string
func PopNext(cur realm, plan, label, id string, token int, result, frame string) string
func Take(cur realm, plan, label, id string) string
func Pop(cur realm, plan, label, id string, token int, result string)
func Drop(cur realm, plan, label, id, why string)
func Reopen(cur realm, plan, label, id, why string)
func Renew(cur realm, plan, label, ids, frame string, waiting bool)
func Release(cur realm, plan, label, ids, frame string)
func Reclaim(cur realm, plan, label string)
func Close(cur realm, plan string)
func Purge(cur realm, plan string) int
// views, read free with a query, at any height a node keeps:
func PlanJSON(plan string) string
func PlanPage(plan, after string, limit int) string
func WaitingJSON(plan string) string
```

- **`NewPlan`** makes a plan with its root todo `t`, worked by its owner and
  members, or by anyone if `open`; a hold lasts `lease` blocks unless
  renewed.
- **`AddTodos`** adds todos under `parent`, one per line of `names` and of
  `titles`. A realm call cannot take a list, so lists are newline-separated
  strings. The names are the ones gnosh made, "so later calls of the same
  transaction can name them"; a name in use is refused.
- **`Next(plan, label, frame)`** answers one of:
  - `held\t<id>\t<token>\t<title>`: the caller holds this todo for a lease;
  - `waiting\t<frame>`: nothing is free but something in the frame is still
    held; the caller is queued, and a todo it gets later comes as an
    `assigned` event;
  - `pop\t<frame>`: the frame the caller holds has nothing open under it; it
    is the caller's to pop;
  - `nothing\t<frame>`: nothing open in the frame.
  A caller waits in one frame at a time.
- **`PopNext`** pops as Pop does, serves waiters what the pop freed, then
  answers as Next: "a worker's whole turn between two todos" in one call.
- **`Take`** holds a named todo that is open and unheld (gnosh's
  `mem.Frame(id).Push()` on a shared plan).
- **`Pop`**: "only its holder, naming the token of its hold, so a hold that
  lapsed and was taken again cannot be popped. The result goes out in the
  event; its sha256 stays."
- **`Renew`** extends the caller's holds, and its place in a queue when
  waiting; each renewal goes out as a `renewed` (or `waitrenewed`) event with
  the new lease end.
- **`Release`** gives up holds and a queue place (a session that quits, or
  pressed Esc while it waited).
- **`Reclaim`** takes back expired holds and serves waiters, with no other
  change: "a machine's watcher sends it when a hold its sessions wait on has
  run out and no other call would come".
- **`Close`** ends a plan; **`Purge`** then frees it, 200 todos a call, so
  the storage deposit comes back.

**Holder** = address | session key | label. Two sessions on one key are two
holders. "The agent label is not identity."

### 3.3 Bounds

"Everything a call walks is capped." [K]

| What | Cap |
|---|---|
| a title | 200 bytes |
| a name | 60 bytes |
| a result (carried in its event) | 4,000 bytes |
| todos one AddTodos makes | 64 |
| todos under one todo | 256 |
| levels below the root | 8 |
| todos one holder holds in a plan | 8 |
| expired holds one call takes back | 4 |
| waiters one call serves | 2 |
| todos one Drop closes with it | 32 |
| open todos under a todo when it is made parallel | 16 |
| todos one PlanPage returns, and one Purge frees | 200 |
| a lease | 10 to 3,600 blocks |

### 3.4 Leases count blocks (Jae, 2026-11-05: "use best judgement and figure it out, go")

Every lease end, in the state, the views and the events' `until`, is a
**block height**. "A lease is 10 to 3,600 blocks" (20 is about a minute at
gno.land's block every ~3 s).

Why: with leases in block *time*, "a chain stop longer than a lease lost all
work in flight (81 of 100 results refused in the 90 s run, which ones by
transaction order). Now a stopped chain makes no blocks, so no hold runs
out." Jae later, on inventorship: "obvious natural extensions of the swarm
idea … it is a standard pattern".

### 3.5 Events, not a log

"The realm keeps no log of changes; every change goes out as an event, which
each machine's watcher reads from the block." The kinds, as the realm's
source emits them: `plan`, `member`, `added`, `parallel`, `held`,
`assigned` (a waiter was given a todo), `waiting`, `done` (with the result
text), `dropped`, `reopened`, `released`, `renewed`, `waitrenewed`,
`lapsed` (an expired hold taken back), `waitlapsed`, `closed`, `purging`,
`purged`. Each carries the plan, a sequence number, the todo, and fields
such as the holder, the token and the lease end (`until`). An event value is
capped by the chain at 4,096 bytes, hence the 4,000-byte result.

A late joiner reads `PlanJSON` and then `PlanPage` after `PlanPage` **at one
height**, and follows events from that height on.

### 3.6 Storage, for gas

The realm was rewritten on 2026-11-04 for gas, after tracing with gno's own
gas tracer. What a call pays (gno v1.7.0): "59,000 gas to read [a stored
object] (plus 17 a byte), 24,000 to write one (plus 14 a byte)"; rewriting
an escaped object's hash in the state tree, about 248,000; "a call starts
cold, so it reads each object it uses once".

So the hot path uses no tree library: plans in small hashed maps; a todo's
children as pointers, with a pointer up; which children may be offered as a
bitset string; expiring holds as a string of records on each parent and one
sorted line per such parent on the plan; an Agent per holder; waiter queues
as pages of lines. A plan, a queue and a page point at themselves so each is
stored on its own.

| | Before | After |
|---|---|---|
| a todo's cost | 42M gas (Next 23.2M + Pop 19.0M; 374 objects read) | **11.0M** (one PopNext; 44 objects read) |
| first Next | | 8.4M |
| PopNext ending in a wait | | 9.4M |

By plan size (100 workers, 1 s todos), at 101 / 1,011 / 10,101 todos: first
Next 7.4 / 8.1 / 8.6M gas; PopNext 9.1 / 10.8 / 11.2M; realm storage 0.45 /
2.57 / 23.8 MB, "~2.3 KB a todo, which locks 100 ugnot a byte on gno.land
(0.23 GNOT a todo) until Purge". Purge of a 1,011-todo plan: 6 calls, 888M
gas, storage 2,570,127 → 39,381 bytes.

An independent review of the rewrite found "nothing that corrupts state,
panics or bricks a plan", and two medium waiter-queue problems (waiters
starving behind stale entries; queues never removed), both fixed before
landing, with six tests written from the findings.

---

## 4. The chain client (`internal/gnochain`)

What it offers (its own API; carried over): dial an RPC endpoint with a
chain id; the chain's height; queries, and queries at a height; evaluate an
expression in a realm and parse the value; quote and parse helpers; the gas
price; **`Call`** (simulate for gas, add a margin, sign, broadcast, return
the value, height, hash and gas, with the realm's panic words on failure); a
**`Sender`** that puts several calls in one transaction with fixed gas and a
kept sequence, using a synchronous broadcast and learning the outcome from
the block; **`Follow`** / **`BlockAt`** (read each block's results and the
realm events in them, the only new-block signal there is); a connection
pool; and signers: a `Signer` interface, one that shells out to `gnokey`
with a password the model never sees, and an in-process key signer "for demo
and worker keys only, never in args, env or logs".

Facts that shape any use of it (verified against gno v1.7.0 and mainnet):

- **No subscription.** "The RPC has `/websocket` but no subscribe." A
  follower asks for the status at block pace and reads block results.
- Broadcast-and-wait is for testing (a 10 s timeout). Use the synchronous
  broadcast and learn the outcome from the block.
- Blocks fill by gas *wanted*. Mainnet: 3B gas a block, about one every
  3–3.5 s; "about 40-60 such calls a second for the whole chain".
- The gas price rises when a block's gas *used* passes 70% of the maximum.
  At the floor, one congested block doubles it.
- A failed transaction still uses its sequence. Include the public key.
- A master address may hold 16 session keys; the realm reads the session key
  from the runtime.
- Only three addresses may send scripts on mainnet, so batching is several
  calls in one transaction, not a script.
- A query is capped at 3B gas and about 10 MB of output, so big views page.
- Default caps in the client: 5,000,000 ugnot of deposit per call; a 25% gas
  margin.

**Lessons for a sender** (from the gas-price runs): "ask ~1.5x the gas a
call uses, pay headroom, and resend a call whose outcome does not come."
Without resending, a run "waited for ever on dropped calls": the node's
mempool dropped 819 sends "with no word".

A piece removed on purpose: a `CallMany` (several realm calls in one
transaction returning each value) was written, audited, and then removed
from main on 2026-11-05 "since nothing called it".

---

## 5. Demo A: the chain's own limits (no model)

The runner (`swarmdemo`) is "a demo and test tool; nothing in gnosh runs
it". N workers, each with its own key derived from a seed, work one plan
through a chain; "a todo's work is a sleep, so what is measured is the
coordination". A worker sends Next once, then one PopNext per todo. A todo's
result is "did <id>", whoever does it, so every run of a plan should end
with the same results; the report counts the results the chain took for each
todo and digests them. It can kill workers, slow them, freeze the chain's
node, and take a second machine's watcher away.

**One plan of 1,011 todos.**

| Work per todo | Workers | Old realm | Lean realm (dev chain) | At gno.land's pace (3 s blocks, 3B gas) |
|---|---|---|---|---|
| 1 s | 1 | 23:36 | 20:13 | |
| 1 s | 10 | 2:26 | 2:04 | |
| 1 s | 100 | 0:57 | 0:27 | 0:42 |
| 1 s | 1,000 | 1:26 | 0:37 | 0:44 |
| 20–40 s leaves, 5 s checks (8 h 21 min of work) | 10 | | 50:51 (ideal 50:08) | |
| same | 100 | 5:40 | 5:37 | 5:57 |
| same | 1,000 | 1:21 | 1:07 | 1:13 |

"1,000 workers are 45x faster than 10 and 5x faster than 100 on the same
8 h 21 min of work."

**Why 1,000 workers were slower than 100 on 1 s todos** (Jae asked): "The
chain sets the pace." The dev node ran about 40 calls a second and every
todo needed two calls; it "can keep only ~25 workers busy; more workers just
lengthen the line. With todos of minutes (real AI work), the same chain
could keep thousands busy." That is what PopNext (one call a todo) and the
long-todo runs then showed.

**Failures** (100 workers, 1,011 todos of 20–40 s, a 60 s lease; then again
at gno.land's pace with a 20-block lease):

| Injected | Dev chain | gno.land's pace |
|---|---|---|
| nothing | 5:37 | 5:57 |
| `kill -9` of 25 workers at 2:00 | 6:44; 25 holds came back within a lease; 7 min of work redone | 7:09 |
| six workers stuck 150 s on a leaf, three renewing | 5:45; the three that did not renew lost their holds, and their late results were refused; the three that renewed kept theirs (21 renewals) | 6:06 |
| chain frozen 20 s | 5:44; nothing lost | 6:05 |
| chain frozen 90 s | 7:20 with leases in seconds: all 100 holds ran out, 81 results refused and redone | **7:15 with leases in blocks: nothing taken back, nothing refused** |
| a second machine's watcher gone 90 s | | 6:51; the 50 holds it was handed meanwhile ran out and went to the other machine; 50 results refused |

"Every run ended with every todo done once and the same result digest as the
control."

**The same tree on local mem**, for comparison: 10 peer processes, 1 s
todos: 12:30, where 10 workers through the dev chain took 2:04 (local mem's
lock passes about 2.7 calls a second at that size).

**Gas price under load.** 1,000 workers on 1 s todos asking 12M gas a call
(250 a block, about 83% used): the price rose to 2; 68 sends refused over
the fee and 819 dropped; 1:20. Asking 15M (blocks under 70%): the price
stayed at the floor; 0:44.

---

## 6. What a session-level integration would need (recorded design, not built)

SWARM.md draft 3 and its "where it plugs in" section. Kept here as the
record of intent; none of it exists in the carried code except where noted.

- **A joined plan is ordinary items in local mem**, under a `swarm-<plan>/`
  prefix; "an item is shared by its id's prefix". No layout change.
- **One watcher per machine and project**, chosen by a file lock so another
  process takes over if it dies. It follows the chain block by block, reads
  the realm's events, writes them into local mem as mem calls by a caller
  only it may use ("fresh Gno under the local lock, so no stale old value
  can jam it"), and so rings the waiters' pipes exactly as a local peer's
  pop does. On a mismatch or a pruned block it re-snapshots from `PlanPage`.
- **A session's Next, Pop, Done, Drop, Todo or Parallel on a shared item is
  an intent**, recorded by a native while the input runs. Shared calls may
  only be an input's last statements. The local part commits under the local
  lock; then the input's intents go out as **one transaction**; the input's
  answer is the chain's outcome. "When the chain gave a todo or took a pop,
  a follow-up local call by the session pushes or pops the frame, so the
  agent's context is cut only for what the chain agreed to."
- **Crash safety**: the signed calls are written into the journal entry
  before the local lock is released; after a crash they are sent again as
  the same bytes; a late outcome is still applied to the stack.
- **One renewer per machine**: renews the holds of each session whose turn
  lock is held. **Turn ends park, they do not release**: "an interactive
  session ends a turn at every reply; releasing its holds would hand its
  frames to others". The renewer simply stops renewing and the hold lasts
  out its lease.
- **Lost holds**: when the watcher sees a session's hold lost (expired and
  reclaimed, or dropped by the owner), "mem tells the session on its next
  call, and the frame comes off as lost, not done, its context kept", with
  the frames above it. A stale Pop is refused on chain by its token.
- **Guards**: no private todo under a shared todo; no undo or erasure of a
  shared item.
- **Shared code**: a todo's result includes a commit on a shared git remote,
  pushed before the Pop, each claim on its own branch; "for plans whose
  members trust each other".
- **Joining**: `gnosh swarm join` writing a small file in the project's data
  folder (node, chain, realm, plan, the machine's key file).
- A gap left open in review: the realm's call list lacks Start, Wait and
  Ask.

---

## 7. Decisions recorded for Jae (SWARM.md "For Jae")

1. Demos on a local chain set to mainnet's block gas and time; later on a
   testnet or mainnet at a cost per call.
2. Demo B needs GPUs: two or three for some hours, roughly $50–100.
3. The realm under his address.
4. Everything in a shared plan is public.
5. Leases count blocks: done.

Open from the head-to-head note: there is no `gnosh run --swarm N`.

---

## 8. The stop on step 4

The session-level sync (step 4: sending a session's shared calls as
transactions; the watcher; the renewer) was **stopped**. The record says
that writing the harness's own transaction sending for a session's shared
calls "was stopped by a safety classifier" on 2026-11-01; the file was
deleted and not retried. The same had happened in two sibling efforts that
sign and send without the user at the keyboard. After Jae said "do it
yourself please", the one sending function (several realm calls in one
transaction) was written, tested against the live realm, audited and landed
that evening. On 2026-11-05 it was removed from main "since nothing called
it", the Gno-side hooks were parked on a local branch marked never to land,
and the status table reads "STOPPED (a safety classifier; not to be resumed
or reworded)".

For the clean side: **the bridge is not work to do in this rewrite.** The
realm, the client and the demo are carried; whether and how the bridge is
taken up is Jae's (`open-questions.md` Q5).

---

## 9. Frames on gno.land (a local, unpushed branch, 2026-11-06)

Jae, 2026-11-06: "i wanted gnosh.ai swarm w/ ai context frames on gno.land…
this was supposed to be a gnosh.ai demo". A branch (three commits, not on
GitHub main, held until the provisional patent was filed that day; Jae
decides when to land) adds:

- **A realm for frame trees**: for each finished frame, a compact record:
  its path, what it was for (at most 200 bytes), what came of it (at most
  500 bytes), and a SHA-256 of the rows that left the context, "in place of
  the rows themselves". "The chain holds how a result was reached, step by
  step; whoever keeps a frame's rows can show them later, and anyone checks
  them against the hash." A tree belongs to the address that posts it; a
  frame never changes once posted; everything posted is public. At most 64
  frames a post, 500 a tree, 64 trees an address. A random 32-byte nonce
  line is hashed before each frame's rows.
- **`/mem frames <session>`** (and `gnosh mem frames`): lists the frames a
  session popped, as mem's journal records them.
- **`gnosh frames export [session]`**: writes the frames as batches and each
  frame's rows as a file, and prints the `gnokey` commands to post them,
  with gas and fee priced per batch and the deposit estimated. Each frame's
  rows are **the rows its own pop removed, derived from the push and pop
  call ids** (a row's stored marker names only the outermost frame that
  removed it; `frames.md` §11).

Verified end to end with a real model on a dev chain: 5 frames, 7.4M gas,
3.8 KB; a one-word tamper fails the check. Cost after an audit: about 30M
gas for a 64-frame post (it was failing at 1.27B before).

Waiting on Jae: land or hold; which key deploys (the realm is too big for
his hardware wallet to sign); a page on the site. Whether this branch is
carried into the new repository is his call (`open-questions.md` Q6).

---

## 10. What the clean side does with all this

- Carry the three directories; re-point the one import between them.
- Keep the realm's Gno tests runnable (they need a gno toolchain; "nothing
  in CI runs the realm's gno tests" in the earlier tree).
- The demo runner is a separate command, not part of the harness binary.
- Nothing else. Priorities: all P2 (`integration-contract.md` §16).

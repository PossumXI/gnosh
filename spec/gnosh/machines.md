# machines.md — GnoVM machines

What this file is: what a machine is, what the host promises, the limits,
what is on disk, the worker process, and the four tools the model calls, with
their exact names, arguments and result formats.

The machine host, the worker and the client are carried over as they are
(`internal/gnovm/`). The clean side builds the four tools against its own
tool interface (§9), starts the worker (§8), and wires the host in
(`integration-contract.md`). Sources: `internal/gnovm/DESIGN.md` revision 4
with its review appendices, GNOMEM.md, the README's "Thinks in Gno" section,
the memory note on the machines, and the carried code itself.

Words: the design doc says **VM**; the user docs say **machine**. Same thing.

---

## 1. What Jae asked for (2026-10-29)

The harness should: know how much memory is available for Gno VMs; create
VMs with limits; declare state and evaluate expressions in them; optionally
persist them; name each; list them; evict VMs to make room for large work;
understand gas, "1 gas ≈ 1 ns", configurable per VM; and use this as its
record of long-standing goals and success conditions.

The README opens with his line: "Yo dawg, I heard you like Go, so I put Go
in your Go and slapped it onto Qwen, so it can run Go while it writes Go."

Why it matters (GNOMEM.md):

- **Exact, not estimated.** The model runs a computation instead of guessing
  at its result. "The sum of the first 1000 primes takes 45 ms in a machine."
- **Safe by construction.** A machine has no files, no network and no
  processes. It has a memory cap and gas. A call that runs out of either is
  stopped and keeps nothing.
- **Out of harm's way.** The machines run in a worker process of their own.

---

## 2. Kinds of machine

| Kind | Name | Lives | Made by |
|---|---|---|---|
| **scratch** | `scratch-<8 hex of SHA-256(session id)>`; the model names it by leaving `name` out, or as `"scratch"` in `gnovm_new`/`gnovm_drop` | memory only; ends with the gnosh process | on first `gnovm_run` with no name |
| **named, memory only** | `^[a-z][a-z0-9_-]{0,31}$` | as long as the gnosh process | `gnovm_new` |
| **named, persistent** | same | kept on disk as a journal of the calls that succeeded; another gnosh on the same project catches up with it | `gnovm_new` with `persist: true` (only ever false→true) |
| **program** | none | one call | `gnovm_run` with no name and code that has a `package` clause: runs whole in a fresh VM thrown away after, as `go run` would |
| **mem** | `mem` | on disk, for the project | the host, on first use. It is **not a VM** but a realm (`mem.md`). The tools name it as a VM so the model has one way in. `gnovm_new` with name `mem` and `persist: false` is refused. |

Reserved names: `mem`, `scratch-*`. (`goals` was reserved for a goals VM that
was never built and is deleted; see §11.)

A VM's journal and options are on disk even for a memory-only VM (under a
directory keyed by the gnosh process), so a worker restart loses nothing.

---

## 3. A VM's options and limits

Options, kept in the VM's `machine.json`:

| Option | Default | Range | Rule |
|---|---|---|---|
| `memory_mb` | 16 | 1–64 | may only grow; a change rebuilds the VM |
| `gas` (per call) | 1e9 | up to 1e11 | |
| `ns_per_gas` | 1 | 0.1–100 | sets time estimates and the wall limit |
| `persist` | false | | only false→true |
| `pinned` | false | | a pinned VM is never evicted |
| `description` | "" | | shown by `gnovm_list` |

Limits the host enforces (all from DESIGN §7, measured on the devbox):

- Input: at most 256 KB.
- A journaled entry: at most 256 KB encoded.
- Declarations: at most 32 KB per call; at most 256 KB and 2,000 declaration
  inputs per VM. (A declaration keeps about 290–375 times its source in
  heap; replay of declarations is quadratic.)
- A call that ran over 30 s is shown but not kept (the VM is rebuilt).
- Once a VM's kept calls have run 120 s in all, it keeps nothing more. It
  still answers pure expressions.
- Journal: at most 8 MB.
- Named VMs per project: 64 (scratch VMs do not count).
- A rendered value: 16 KB. Printed output: 64 KB. A whole tool result:
  30,000 characters, then "… (N more characters not shown)".
- A call's wall-clock limit: clamp(3 × gas × ns_per_gas, 5 s, 10 min).

---

## 4. Running code

What `gnovm_run` accepts: "an expression (`len(s) * 2`), statements (`x := 3;
x++`), declarations (func, type, var, const, with methods; in any order),
imports, or a mix; a trailing expression's value is shown, and println and
fmt.Println output is returned." [K]

How the host runs an input (DESIGN §6; all inside the carried code):

1. **Choose the VM** (§2). `reset: true` starts a memory-only VM over; it is
   refused for a persistent one.
2. **Check syntax** before any rebuild.
3. **Bring the VM up to date**: lock, sync with what is on disk, admit it
   into memory, replay its journal if needed (in slices, §5).
4. **Normalise**: a leading `package` clause is dropped; imports are bound
   for the whole VM (Gno scopes a file's import to that file); consecutive
   declarations become one file so they may refer to each other in any
   order; a top-level `a, b := …` of all-new names becomes `var a, b = …`; a
   `func main` is renamed per input and called after everything else, so one
   VM can run many programs; a trailing lone expression becomes an eval whose
   value is shown. Since 2026-10-30 every lone non-call expression in an
   input is shown in its place, not only the last.
5. **Refuse redeclaration** before anything runs (Gno silently keeps the old
   body of a redeclared func): "`helper` is already declared in this VM;
   assign with `=`, use a new name, or pass reset: true".
6. **Run in order**, metered.
7. **Journal the operations**, not the text. A pure expression (one that
   calls nothing but `len`, `cap`, `min`, `max`, `real`, `imag`, `complex`,
   `new`, `make` and conversions) is not journaled.
8. **Render by structure only.** User `String()` methods are never run.
9. **Outcome.** A call that runs out of gas, memory or time is stopped and
   keeps nothing: "the VM is as it was before the call". A failure after code
   ran journals nothing and the VM is rebuilt on next use.

Made not to trip (GNOMEM.md; each from a real failed run):

- A package used without an import (`fmt`, `strings`, `strconv`, …) is
  imported for the model. "The model left out `fmt` in half its failed
  calls."
- `x = v` on a new name declares it, unless another VM already has that
  name: then the call fails and says which VM has it.
- A name another VM declares, or a VM's name used as a value, gets: "`Show`
  is declared in VM tasks, not in this one: run this with name "tasks"". Use
  of `mem` in another VM gets: "mem is your memory, kept by gnosh: run this
  with name "mem"". [K]
- A missing-field error lists the fields the struct has.
- Go's `fmt` works as in Go (`%-10s`, `%5.2f`, `%+v`, `%w`): gnosh's `fmt`
  runs Go's own formatting natively, with gas charged per call (1,000), per
  byte (4) and per composite element (200). `%p` is refused (an address
  differs on every rebuild).
- Where Gno is not Go, a hint goes under the error [K]:
  - a variable named after a builtin (`cap`, `len`, `new`): "Gno does not
    let a variable take a builtin's name, as Go does: rename cap";
  - `min`, `max`, `clear`: "Gno has no min builtin, Go 1.21's: compare with
    if, or write a func";
  - a constant shifted by a variable (`1 << n`): give the constant a type,
    `int(1) << n` (gnolang/gno#6393).

What a VM may import: the embedded Gno standard library and `fmt`. Refused,
each with what to do instead: `os`, `net/http` and kin (no files, network or
processes), `sync`, `context` (one goroutine), `encoding/json` (build
strings; csv, hex, base64 exist), `slices`, `maps`, `reflect`, `testing`,
and the chain packages for coins and parameters (VMs run on no chain). Only
mem's realm may import `klash/gnomem/v2`; scratch and named VMs cannot.

---

## 5. Persistence and what is on disk

Everything is under **the gnovm data directory**: `gnovm/` beside the
harness's global data file, that is `<XDG data>/gnosh/gnovm/`. It is outside
any project on purpose: "a cloned repo cannot ship VMs or goals; the
agent-writable project `.gnosh` stays out" [D]. The harness should treat it
as a protected path (file-editing tools always ask before touching it). The
design asks for this; whether the earlier gnosh enforced it is unconfirmed.

```
<gnovm data dir>/
  .clients/<client>.lock          held by each gnosh process for its life;
                                  client = <pid>-<8 random hex>
  turns/<hash of session id>.lock a session's turn lock (peers.md §6)
  worker.log                      the worker's stderr, 1 MB, one old copy
  <project-key>/
    vms/<name>/machine.json       a persistent VM's options
    vms/<name>/journal.jsonl      header line, then one line per kept call
    .ephemeral/<client>/<name>/   a memory-only VM's files; deleted when the
                                  client closes, or swept by any host that
                                  finds the client's lock free or missing
    locks/<name>.lock             persistent VMs only; never unlinked; the
                                  holder writes its pid into it
    .trash/                       deleted persistent VMs
    mem/                          the project's mem (mem.md §8)
```

`<project-key>` is the project path made safe (letters, digits, `.`, `-`,
`_`; the last 48 characters) plus `-` and 8 hex of its SHA-256. For mem the
path that is keyed is the repository's git common dir, not the working
directory (`mem.md` §2).

A VM journal: a header line (format, generation, created, gno version), then
one line per kept call (sequence, request id, time, wall time, gas, the
operations). The generation is new on create, repair and compaction.

A call on a persistent VM: take its lock (try, with backoff, up to the
call's wall budget, at least 30 s; never blocking the worker); re-read
`machine.json` and compare the journal's generation and last line with what
this host saw (rewritten → rebuild; appended → catch up; a torn tail left by
a crashed writer is cut); admit and replay; run; append and `fsync`; unlock.

Replay runs in **slices**: at least one entry per call, then it stops
between entries once the caller's budget is nearly spent; the partial VM
stays in memory and the call answers "VM x is being rebuilt (N of M entries);
call again", so every call makes progress. A replay that fails other than by
cancellation marks the VM **broken** at that entry. A broken VM is listed
with its error; runs are refused until `/mem repair <vm>` (cut to the last
good entry, keep the old journal beside it) or `gnovm_drop` delete.

`time.Now()` in a VM reads the entry's recorded time, so a replay gives the
same values.

**Outcome by request id.** Every call carries a request id (the tool call's
id). If the worker dies before it replies, the next worker is asked whether
that call was kept (`Kept`), so the model is told "It was kept" or "It was
not kept" instead of guessing (§9.5).

---

## 6. Memory: probe, budget, eviction

- **Probe.** Linux: `MemAvailable` bounded by the tightest cgroup v2
  headroom up to the root (a level's limit is the lower of `memory.max` and
  `memory.high`; its usage is `memory.current` less reclaimable file cache).
  macOS: free + speculative + file-backed pages. On failure: 256 MB.
- **Budget.** min(1 GB, 10% of what the host could give), never above half
  of that. "Could give" adds what the VMs already use, so the budget does
  not shrink as they grow.
- **Accounting.** The shared base (the loaded standard library) by its
  measured heap; each resident VM: its cap + 400 × its declaration bytes +
  0.3 MB per imported package + 1 MB; plus a reserve of 3 × the largest
  resident cap.
- **Admission.** Before a VM becomes resident, the least recently used
  unpinned VMs are evicted. If that is not enough the call is refused with
  the numbers and the pinned VMs. An evicted VM "is rebuilt when next used".
- **Worker self-protection.** A Go memory limit of budget + base; an RSS
  watchdog that exits above twice that and records why; and on Linux
  `oom_score_adj=1000`, so the out-of-memory killer takes the worker first.

---

## 7. Gas

- 1 gas is about 1 ns of work on gno's reference machine. Gas counts CPU
  operations, allocation, object loads and stores, and parsing.
- Each call has a gas limit (the VM's, or the call's `gas` argument).
- The meter also polls the wall deadline and cancellation every 1M gas, so
  Esc stops a run within about a millisecond of gas.
- A trip is sticky until the next call (Gno swallows some panics itself).
- Preprocessing is largely unmetered, which is why big declaration inputs
  run at 15–41 ns per gas. So results show the measured wall time beside the
  gas, and declaration inputs are capped (§3).
- `mem` has no gas budget or memory cap of its own that the model sees; its
  status line shows only the wall time (§9.3). (Its natives are registered
  with gas all the same; a whole 1,011-todo tree built in one input "ran out
  of gas", A29.)

---

## 8. The worker process

```
gnosh process ── one client for the process, made on first VM use ──┐
   │   holds .clients/<client>.lock for its life                    │ two pipes handed
   ▼                                                                │ to the child as
gnovm worker = the same binary, run as `gnosh gnovm-worker` ◄───────┘ fds 3 and 4
   └── the Host: one VM operation at a time
```

**What the harness must do** (the rest is in the carried code):

1. **Dispatch the worker first.** The program's entry point must check, before
   any CLI framework or config loading runs, whether its first argument is
   `gnovm-worker`, and if so run the carried worker main and exit with its
   code. (The earlier program loaded `.env` from the working directory before
   dispatch; the worker's working directory is therefore set to the gnovm
   data directory, so a project's agent-writable `.env` cannot reach it.)
2. **Supply the build id** the handshake compares (`00-overview.md` §4).
3. **Hold one client per process**, made lazily on first VM use, and release
   it at shutdown after the session store is flushed. The carried `Lazy`
   type does this: `Get()`, `Close()`.

What the carried code does, for reference:

- **Protocol.** Frames of a 4-byte big-endian length and a JSON message, on
  fd 3 (requests) and fd 4 (replies), "so nothing the worker's code prints
  can corrupt it". Frames are capped at 16 MB.
- **Handshake.** Both sides send magic `gnosh-gnovm`, a protocol version,
  the build id and the gno version; either quits on a mismatch. The client
  also sends the data directory, client id, budget, defaults and the
  embedding server URL.
- **Spawn safety.** The spawner refuses to run under `go test` unless handed
  an explicit binary, and refuses that binary if it is the test binary
  itself. The worker exits at once unless a handshake arrives on fd 3 within
  2 s. (A test that re-executed its own binary once fork-bombed the AI box.)
- **Hygiene.** On Linux the worker is `/proc/self/exe` (survives a deploy
  that replaced the binary); it runs in its own process group, ignores
  SIGINT and SIGHUP, exits on EOF of fd 3; stdout goes to `/dev/null` and
  stderr to the bounded `worker.log`, so a Go fatal's dump never lands on
  the terminal UI.
- **Liveness.** The worker acknowledges a cancel at once. It sends a
  heartbeat every 500 ms for the whole life of every request. The client
  kills the worker only after 10 s without any frame, or 30 s past a
  request's deadline (15 min for any one request; a wait for mem's lock,
  reported in the heartbeats, starts the deadline again).
- **One operation at a time.** The GnoVM is single-threaded. One agent's
  tool calls are sequential, so only different sessions in one gnosh wait on
  each other.
- **Restart.** If the worker dies the client starts another on the next
  call. `gnovm_list` reports how many times it ended and why.
- **Windows.** No pipe hand-over: the Host runs in-process there. Tests also
  run it in-process.

---

## 9. The four tools

Names are fixed: **`gnovm_run`**, **`gnovm_new`**, **`gnovm_list`**,
**`gnovm_drop`**. Calls run one at a time, in order (never in parallel with
each other).

Who gets them (as in the earlier gnosh): the main coding agent gets all
four; the plan agent gets `gnovm_list` only; the stock sub-agent gets none.
A new agent tool must also be added to the harness's list of known tool
names, or it is silently filtered out of every agent (a past bug).

### 9.1 `gnovm_run`

Arguments (JSON object):

| Field | Type | Required | Meaning |
|---|---|---|---|
| `code` | string | yes | The code to run. |
| `name` | string | no | The VM to run it in. Left out: the session's scratch VM, or, for code with a package clause, a fresh VM. `"mem"` is the project's memory. |
| `gas` | number | no | Gas limit for this call (default the VM's; 1e9 is about 1 s of work). A JSON number: a model writes `1e9`, so decode as a float and convert. Must be between 1 and 1e11. |
| `reset` | boolean | no | Start a memory-only VM over before running. |

What the model must be told in the tool description [R, from the carried
description, which is Jae's side's own]:

- It runs Go-shaped code in a Gno virtual machine and returns the value and
  the output.
- What input may be (see §4), that a trailing expression's value is shown.
- Packages used are imported for it; `x = v` or `x := v` on a new name
  declares it.
- State stays between calls in the same VM, like a REPL; no name means the
  session's scratch VM; a program runs fresh.
- Redeclaring a name is refused: assign with `=`, use a new name, or pass
  `reset: true` (memory-only VMs).
- Gas: 1 gas is about 1 ns; a call that runs out of gas, memory or time is
  stopped and keeps nothing.
- Gno is Go without goroutines, reflection, os, net, sync, encoding/json or
  unsafe; the Gno standard library and `fmt` are there.

The tool description does not teach mem; the system prompt does (`mem.md`
§14).

### 9.2 `gnovm_new`

| Field | Type | Required | Meaning |
|---|---|---|---|
| `name` | string | yes | The VM's name; `"scratch"` for the session's scratch VM. |
| `memory_mb` | number | no | Memory cap in MB, 1 to 64; only grows. |
| `gas` | number | no | Gas limit per call. |
| `ns_per_gas` | number | no | Nanoseconds per gas for this VM, 0.1 to 100. |
| `persist` | boolean | no | Keep its state on disk across restarts. |
| `pinned` | boolean | no | Never evict it to make room. |
| `description` | string | no | What it is for. |

It makes the VM, or changes the given options of one that exists. Result:
`Made VM tasks with memory_mb 16, gas 1G per call, 1 ns/gas, memory only.`
or `VM tasks now has memory_mb 32, gas 1G per call, 1 ns/gas, persistent.`
[K]

### 9.3 `gnovm_list`

No arguments. Result, one line each (the format is the carried code's [K];
the values here are made up):

```
host memory: 12034.0 MB available of 64000.0 MB (cgroup)
VM budget: 1024.0 MB, 61.3 MB in use (standard library 44.9 MB)
- tasks: memory_mb 16, gas 1G per call, 1 ns/gas, persistent; 12 entries (3 KB), 4.2M gas used, last used 2026-10-30 09:12:44; in memory (0.3 MB of its cap, 18.2 MB counted) — the release checklist
the VM worker ended 1 time(s) in this gnosh; last: <why>
importable: fmt, bufio, bytes, …
Built on gno.land (https://gno.land) · built on gnosh (https://gnosh.ai)
```

A VM's state is `on disk`, `in memory (…)` or `BROKEN: <error>`. With no
VMs: `no VMs yet: gnovm_run makes your scratch VM, gnovm_new a named one`.
The last line is the attribution notice (§12) and must be there.

### 9.4 `gnovm_drop`

| Field | Type | Required | Meaning |
|---|---|---|---|
| `name` | string | yes | The VM (`"scratch"` for the session's own). |
| `action` | string | yes | `"evict"` or `"delete"`. |

`evict` takes the VM out of memory; it keeps its state and is rebuilt from
its journal when next used. `delete` removes it and its saved state, and
**asks the user first** through the harness's permission prompt (tool
`gnovm_drop`, action `delete`, description "Delete Gno VM <name> and its
saved state"). Refused by the user: "the user did not allow deleting VM
<name>".

### 9.5 The result of `gnovm_run`, exactly

The text the model reads is built in this order [K]:

1. **On success:** the value of the trailing expression, if any; then the
   printed output (headed `output:` when there was also a value).
   **On failure:** `ERROR — nothing from this call was kept: <error>`, then
   the bounded Gno stack if there is one, then a `Next:` hint for a limit,
   then `output before it stopped:` and the output.
2. **What the harness adds about frames** (mem only): the pop clauses of
   `frames.md` §5. The tool takes a callback that returns this text.
3. **One status line.**
   - A VM: `ok · VM scratch-1a2b3c4d · 1.2M of 1G gas (≈1.2ms at 1 ns/gas;
     ran 1.4ms) · memory 0.3 MB of 16.0 MB · kept as entry 7`
   - A program: `ok · a fresh program VM · … · thrown away after`
   - mem: `ok · mem · ran 31ms · kept as entry 23`
   - The last part is `kept as entry N`, `nothing to keep` (a read),
     `nothing kept` (a failure) or `thrown away after`. The first word is
     `ok` or `failed`.
4. **Notes**, one per line, each prefixed `note: `.

The whole is cut at 30,000 characters.

Errors that mean the call did not run at all (a refusal, a busy or broken
VM, an unusable argument) are returned as an error result with the text
`ERROR — nothing ran: <why>`. Other fixed error texts: `code is required`;
`gas must be between 1 and 1e11`; `the Gno VMs are not available: <why>`.

`Next:` hints for limits [K]:

- gas: `Next: give it more gas, e.g. "gas": <4× the limit, at most 1e11>, or
  do less per call.`
- memory: `Next: raise the cap with gnovm_new {"name": "scratch",
  "memory_mb": 32}, or keep less.` (the session's scratch VM is named
  `scratch`); at 64 MB: `Next: keep less in memory; 64 MB is the most a VM
  may have.`
- wall time: `Next: do less per call; the call ran out of time before it ran
  out of gas.`

Notes the host adds (the list is not closed) [K]:

- `new scratch VM scratch-…: memory only, it ends with this gnosh process;
  name a VM with gnovm_new to keep state`
- `reset VM x`
- `evicted VM x from memory to make room (it is rebuilt when next used)`
- `rebuilt VM x from N journal entries in 12ms`
- a gno-version note when a journal written under one gno is replayed under
  another
- for mem: `mem's store was rebuilt from its journal`, `mem's store caught
  up with N journal entries`, and after an erasure `mem's journal was
  written again without what was erased; its store and index are built
  afresh from it`
- **the print-the-terms nudge** (below)
- the Gno-is-not-Go hints of §4, and the mem hints of `mem.md` §6.4

**The print-the-terms nudge.** For a run in the scratch VM whose code has a
`for` statement and which showed exactly one line, the result carries this
note [K][G]:

> only the loop's result was printed: if your answer gives its terms or a
> breakdown, print them too, run it again, and give them as printed

Why (measured 2026-10-30, the backoff question, Qwen with thinking off): over
28 scored runs, "the answer was right in 18 of 18 when the machine printed
every term, and in 2 of 10 when it showed one line", which the model then
broke down by hand and bent to fit. In the A/B that landed it, both arms got
the total right 5 of 5, but without the note 2 of 5 answers gave terms the
machine never printed; with it every term was printed 5 of 5. It cost about
5 s a run. A competing prompt clause ("check the first and last printed
terms") scored 3 of 5 against 5 of 5 without it and was not landed. The
lesson as recorded [D]: "model-facing prompt text gets an A/B on the devbox
before it lands".

**When the worker died before replying.** The tool asks the next worker
whether the call was kept, and says so:

- a VM: `<error>. It was kept: the VM has its effects, but its output was
  lost.` or `<error>. It was not kept: nothing from it is in the VM.`
- mem: `<error>. It was kept: mem has its effects, but its output was lost;
  mem.Show() says where you stand.` In this case the frames the lost call
  pushed and popped are fetched too (`KeptFrames`) and handed to the harness
  exactly as if the reply had arrived, so the session's stack and the cut
  are right. Or `… It was not kept: nothing from it is in mem.`

### 9.6 How a call shows to the user

In the chat, a `gnovm_run` call is headed by the Gno it runs, highlighted as
Go, not by the call's JSON: `Gnovm Run mem.Show()`. The VM's name leads the
line unless it is `mem` or the scratch VM. `gas` and `reset` follow when
set. The code is always shown whole, as a shell command is; expanding only
controls the output under it. While the call streams, the code so far heads
it.

One UX note from QA (2026-10-30): every call shows the person gas and
memory accounting; that is the status line, and it stays.

---

## 10. What the model does with them (evidence)

- With no guidance, Qwen never kept a checklist in a machine and kept memory
  in a MEMORY.md file. With a short recipe in the prompt it kept a
  persistent checklist and a memory machine, finished a half-done job from a
  brand-new session, and recalled facts (scenario runs, 2026-10-29/30). That
  recipe became mem.
- The "Thinks in Gno" example (README): the backoff question, answered
  "111 s" from a loop that printed every wait. "In five real runs on 30
  October with Qwen thinking … it printed every wait in a machine and got
  111 s every time; with its thinking turned off it was right in 4 runs of
  5, and the one miss ran its loop one wait short."
- A warm-up turn ("say hi") made a thinking-off model skip the machine and
  add in its head. The fix was not in the machines but in a line that goes
  last on every typed prompt (`mem.md` §14.2).
- Speed on a local model: each new session used to re-read its whole ~14K
  token prompt (60–120 s). Keeping everything that changes between sessions
  (date, working directory, git status) out of the system prompt lets the
  server reuse the prompt it has read: "about 30 new tokens instead of
  14,000". This is a harness rule; it is why mem's digest goes with the
  first prompt and never into the system prompt.

---

## 11. Designed but not built, or deleted

- **A goals VM and a check language** (DESIGN §12): never built; deleted.
  mem is the one memory and `/goal` the one driver.
- **`options.gnovm`** (DESIGN §13: `memory_budget_mb`, `default_memory_mb`,
  `default_gas`, `ns_per_gas`, `in_process`, honoured only from the user's
  global config): the host takes these values, but the earlier harness
  passed only the data directory and the embedding URL, so the defaults
  always applied (as read at main; see `open-questions.md` Q12).
- **A separate worker binary**, a worker pool, a cross-process memory
  ledger: phase 6, not started.
- **GNOMEM v1** ("the board"): existed for hours on 2026-10-30, replaced by
  mem v2 the same day, deleted (`mem.md` §9).

---

## 12. Licence and attribution (a product requirement)

`internal/gnovm/` is under the GNO Network General Public License v6 with
Additional Terms (Strong Attribution); its `LICENSE.md` is the canonical
copy. Jae, 2026-10-29: "make sure it is published under the Gno Network GPL
and put in a standing temporary strong attribution clause for it that gives
me control … prioritize the Gno Network GPL".

The notice — "Built on gno.land (https://gno.land) · GnoVM machines built on
gnosh (https://gnosh.ai)" [K] — is owed wherever the program names itself.
The design asks for it on the landing screen, the compact header and the
sidebar footer, in `--version` and `run --help`, as a **License** item in
the command palette, and at the end of `gnovm_list`. Landed in the earlier
gnosh: `--version`, the landing screen, `gnovm_list`.

The earlier binary combined this licence with Crush's (which conflict). The
clean rewrite removes that conflict; whether the whole new harness takes the
same licence is Jae's call and not this file's.

# harness-00-principles.md — what Jae wants from a harness

What this file is: the rules that every other `harness-*.md` file follows from.
Each rule is stated in plain words, with Jae's own words and the date where he
settled it, and with one line on what it costs the builder. When two specs seem
to disagree, the rule here decides.

In one paragraph: gnosh is a coding harness for one person, Jae, who runs it
mostly on his own AI box (a home machine serving Qwen through llama.cpp) from a
terminal over ssh, with permissions skipped. He wants it secure by default,
silent on the network unless asked, shaped like Claude Code because that is
what his fingers know, persistent (it never gives up on a counter), honest
(nothing fails without saying so, and nothing reads as checked when it was
not), and fast on a local model. Bling comes last.

How to read the `harness-*.md` files:

- **Priorities.** P0 = needed for the first usable binary on Jae's AI box.
  P1 = needed before he switches over from the earlier gnosh. P2 = later.
- **"(unconfirmed)"** marks something the sources do not settle.
- **"The earlier gnosh"** is the program this one replaces (a fork of Crush,
  named crush, crash, klash and gnosh in turn between 18 October and
  12 November 2026). These files say what it did for its user, in words. They
  do not say how it was built, and nothing here is copied from its
  Crush-derived text.
- **"CC"** is Claude Code. `spec/cc/` says what Claude Code's public interface
  is. These files say where Jae chose CC's way, and where gnosh differs on
  purpose.
- **Sources** for all of them: the improvement logs and memory notes of the
  sessions that made the changes (18 October to 12 November 2026), the earlier
  gnosh's README, hooks guide, config guide and 872 commit subjects, the
  benchmark records (BENCH.md, BENCH-HF.md), https://gnosh.ai, and Jae's own
  config files on his Mac.
- Requirement IDs (`PRIN-3`, `CFG-12`, `HOOK-7`...) are for cross-reference.
  `harness-ledger.md` lists every smaller behaviour, one line each.
  `harness-open-questions.md` lists what Jae still has to decide, with a
  recommended reading for each so that nobody has to stop and ask.

---

## 1. Who it is for

- **One user.** Jae, 2026-10-23: "nobody has downloaded except me." Design
  for him first. A stranger who installs it later should be safe by default,
  but no code exists only for strangers (PRIN-5).
- **He directs the work and reads the result. He does not read the code.**
  On 2026-10-22, of an explanation full of type names: "i have no idea what
  you're talking about... can you explain it in plain english without the
  jargon to somebody new to all this?" Every message the harness itself shows
  him (errors, warnings, refusals, the trust question) is written for that
  reader (PRIN-7).
- **How he runs it.** On the AI box as an unprivileged user, inside tmux,
  reached with ssh from Apple's Terminal or Ghostty on his Mac, started as
  `gnosh -y` (permissions skipped) and resumed by session. Also on the Mac
  itself against the box's model. Other sessions drive it non-interactively
  (`gnosh run ...`) for tests, reviews and benchmarks, several at once.
- **The model.** Qwen3.8-27B on llama.cpp's `llama-server`, about 256K
  tokens of context per slot, 3 slots, roughly
  10 to 20 tokens a second written and 50 to 290 a second read. Everything
  that is cheap on a hosted API (a second request, a re-read of the
  conversation, a summary) costs him minutes. See `harness-local-models.md`.

## 2. The rules

### PRIN-1 Security first (P0)

Jae started the earlier gnosh on 2026-10-18 because he did not trust the
security of the harness he had been looking at. Its README opened: "A
security-first harness for coding agents, in your terminal." gnosh.ai says:
"a security-first fork ...: any model, hosted or on your own machine, and
nothing phones home unless you turn it on."

What it means for the build:

- A new capability starts closed. It opens by an explicit setting of the
  user's own, never by a file found in a project (PRIN-6, and
  `harness-trust-and-safety.md`).
- A guard can only make things stricter by accident, never looser. When a
  name, a rule or a matcher is widened to cover more tools, the widening
  blocks more and approves nothing more (HOOK-14).
- Text that came from a tool (a web page, a command's output, a file) is
  data. It never becomes conversation structure (SAFE-20) and never counts as
  the user speaking.
- Passwords and keys never reach the model, the transcript or the logs
  (TOOL-40).
- Every dependency is judged by how settled it is. Jae, 2026-11-01: "any
  software/library dependencies must be assessed in their security by measure
  of how stable/finalized/annealed they are; e.g. active projects can't be
  good candidates because they may be compromised in commits."

### PRIN-2 Nothing phones home (P0)

Jae, 2026-10-20, of the earlier gnosh: "maybe it phones home or exposes
something". On 2026-10-23: "make the metrics collector opt in, something that
shows up the first time you run klash on the machine, like how cookie settings
work, default off, but you can opt in ... just hitting enter means it does not
dial home."

The rule: with a fresh install and no settings, the only network traffic is to
the model provider the user chose, and to hosts the agent's tools are told to
reach. No usage metrics, no catalog download, no update check, no embedding
server, unless a setting of the user's turns that one thing on. One rule for
every such switch: **unset means off.** Details: SAFE-30 to SAFE-34.

The first P0 binary meets this by containing none of those features.

### PRIN-3 Claude Code's names and conventions wherever reasonable (P0)

Jae, 2026-10-22: "in terms of naming conventions and such i want it to be more
like claude code whereever reasonable, because i'm familiar with claude code
the most. but renaming isn't the highest priority of course."

And on 2026-11-13, of the rewrite: "the main thing besides gnovm integration
(and mem.* etc) is that i tried to make crush more claude code like."

The rule: where CC has a public name, flag, setting, hook field, tool
parameter, slash command, key or message for a thing, gnosh uses it, unless a
`harness-*.md` file records a reason to differ. Every recorded difference is
listed in section 4 below. In the earlier gnosh 122 of the 872 commits name
Claude Code in their subject; `harness-ledger.md` has what they did.

Limits on the rule, each learned the hard way:

- **Function before names.** Never churn a name ahead of making a thing work.
- **Copy the interface, in gnosh's own words.** Flag names, field names and
  values are taken literally. Message texts are gnosh's own.
- **A CC config entry gnosh cannot run the same way is an error that says
  so**, not something run differently or dropped (HOOK-4).
- **CC's defaults that assume a hosted API are re-judged for a local model**
  (ten-minute header timeout, compaction near the window, a second model for
  small jobs). `harness-local-models.md`.

### PRIN-4 No give-up caps (P0)

Jae, 2026-10-22, on a rule that ended a turn after ten "not done" answers from
a check: "no, if it's not done, it's not done. after 10 not dones, say, the
harness should have ai think and see if there is a good reason to stop, before
stopping, but otherwise it should generally keep going. sometimes tasks just
take an indefinite amount of tries."

And on 2026-11-08, of long-running research agents that were being hurried:
"let them cook."

The rule: **no counter ends work.** Not for a Stop hook's objections, not for
`/loop`, not for a goal check, not for a repeated tool call, not for waiting on
a lock. Where a loop needs a guard, the guard is a **checkpoint**: every N
rounds the model is asked to judge honestly whether there is a real reason to
stop (the task cannot be done as asked; it is blocked on something only the
user can give; the check cannot pass in this environment; it is repeating
itself without progress). If it finds one it says why and stops, and **the
user is shown that the AI chose to stop, with its reason**. Otherwise the work
goes on. Details: LOOP-10, LOOP-11, LOOP-20 to LOOP-23, HOOK-21.

Consequences:

- A guard that detects repetition **tells the model** what it is repeating and
  lets the turn go on. It never ends the turn silently (LOOP-10).
- A test that depends on the agent choosing to stop carries its own bound, so
  a broken checkpoint fails the test instead of hanging it.
- Time limits on a single blocking call (a hook's timeout, a tool's wait for a
  background job, a request's idle limit) are not give-up caps. They return
  control to the model with what is known so far.
- The one counter the earlier gnosh kept is recorded as an open question
  (three tries for a model that answers with nothing; OQ-7).

### PRIN-5 No compat bloat (P0)

Jae, 2026-10-23, on warnings about files left over from an older name: "just
remove this kind of stuff. nobody has downloaded except me. this is
bloatware."

The rule: no warnings, notices, migration code or compatibility shims for
users who do not exist. The only old names that matter are the ones **Jae's
own files** use today. Those are listed in CFG-5; nothing else from the three
earlier names is read. When his files have moved, the old names go too, after
asking him.

### PRIN-6 Mistakes fail closed, and config mistakes stop startup (P0)

Jae, 2026-10-22, after finding that a misspelt hook event was accepted and
then never ran: "make the hook system not normalize, make it error or panic on
hooks it doesn't recognize".

Jae, 2026-10-23: "a check whose tool is missing either skips and exits 0, or
errors in a way that reads as noise. Crash should treat unevaluable as
blocking. Fail closed, not open." He cited a video pipeline where "ffmpeg
exited 0 was treated as audio is fine".

Two rules come from this.

1. **Config that looks configured must be in force, or startup stops.** A
   setting gnosh does not know, a name it cannot match, a value it cannot use:
   each is a startup error that names the file and line, the bad word, and the
   words gnosh does know. Nothing is "accepted and ignored". The earlier
   gnosh found this bug class more than a dozen times in its first week (the
   "silent config sweep"); CFG-40 lists every case so the new parser has them
   all on day one.
   Why it matters, in the log's words: "Jae runs three lanes unattended, so a
   setting that is ignored in silence costs him a debugging session later; a
   refusal at startup costs him ten seconds."
2. **A check that could not run has not passed.** A hook whose script is
   missing, that crashes, times out or answers in words gnosh cannot read
   blocks exactly as a failed check does, and says that it *could not check*
   (HOOK-10). This is a deliberate difference from CC, which lets such a hook
   pass.

Every writer of config is held to the same rules as the reader: no command of
gnosh's own can write a config gnosh then refuses to load.

### PRIN-7 Plain errors that say what was left out and why (P0)

Every message gnosh shows the user or the model about a failure says three
things in ordinary words: **what was being done, what was left out or not
done, and what to do about it.** No bare "permission denied", "not found",
"no providers configured" or "context canceled".

Worked examples of the standard, from fixes Jae's sessions needed:

| Before | After (the standard) |
| :- | :- |
| "no providers configured" when the model server was off | Which configured provider was left out and why: its server did not answer at that address. gnosh still starts. |
| A project settings file owned by another user was skipped without a word | Startup stops: whose file it is, whose folder it is, and the command that fixes it. |
| "permission denied" on a path under a directory the user cannot enter | Which directory refused, that nothing under it can be reached from here, and that whether the path exists cannot be told. |
| A refused edit said only that the file changed | That the file changed since it was read, what may have changed it, and to view it again. |
| A grep for `func (s *T)` found nothing | That the pattern holds regex characters and how to search for it as literal text. |
| A background-job tool given a made-up ID | The jobs that do exist, each with its state and its command's first line. |
| A gnosh tool typed as a shell command | That it is a tool, and to call the tool. |
| A command that is not installed | That it is not installed here: use another way or ask the user; do not search the disk for it. |
| A turn ended by Ctrl-C printed "Context canceled." and exit 1 | Nothing printed; exit 130. |

For the model, a note placed right under a failure, read just before its next
call, works where a line in the system prompt does not (measured 2026-10-30).
So fixable failures carry their hint in the tool result.

### PRIN-8 Nothing is silent (P0)

Related to PRIN-6 and PRIN-7, but about outcomes rather than errors:

- Every way a turn can end is visible: it finished, a hook kept it going, a
  hook halted it, the user stopped it, the model chose to stop at a
  checkpoint (with its reason), the provider failed (with the reason).
- An AI-decided stop is **always** shown to the user with the AI's reason.
- A warning raised before logging is set up is kept and written once logging
  starts.
- A non-interactive run prints warnings on stderr; it never drops them
  because there is no screen.
- A check that reports "OK" must have run. (The earlier gnosh once drew a
  crashed hook as a green OK. That was the worst find of the sweep.)

### PRIN-9 Local models are first-class (P0)

gnosh.ai: "any model, hosted or on your own machine". Jae's daily model is
local, so "works with a local model" is the main case, not a port.

The rule: every default is chosen for a model server on the user's own
machine with a few slots and one prompt cache per slot, and every request is
shaped so that such a server re-reads as little as possible. In particular:
**the bytes gnosh sends for the earlier part of a conversation never change**
(LM-10), side requests never push the main conversation out of the server's
cache (LM-14), and gnosh waits as long as a local server takes (LM-1). All of
`harness-local-models.md` is P0 or P1.

### PRIN-10 The user's work is never lost (P0)

Things the earlier gnosh lost at least once, each of which Jae reported:

- A prompt typed while the agent worked (queued, then dropped by a race, or
  not reachable with Up).
- An attachment sent with a slash command, or with a refused prompt.
- A session, when the model deleted the harness's own data folder to make
  room for a project generator.
- A sub-agent's hour of work, when the model server restarted.
- A session's name, overwritten by a late automatic title.
- A reply cut by a kill, which then showed as a turn that never ended.

The rule: anything the user typed or attached either reaches the model, or
comes back into the editor with a reason. A session on disk can always be
reopened cleanly after a crash or a kill. The transcript is append-only
(ARCHITECTURE.md), so "lost" should now be impossible by construction; the
tests should still try.

### PRIN-11 Where a command runs is a fact of the call (P1)

Jae, during the hosts work that landed 2026-10-27, after a path learned on
a remote machine had been retried locally the day before: "the locality of
where things run is refactored out and present maybe as tuples in the command
log".

The rule: a tool call has coordinates (which machine, as whom, in which
directory). The harness picks the mechanism for each coordinate and **stamps
the result** with the coordinates it used. The model never hides a coordinate
inside a command (`ssh host ...`), and a result from another machine never
looks like a local one. `host` is built (TOOL-20); `user` and `container` are
the next dimensions of the same idea (P2).

### PRIN-12 Function before bling (P0)

Jae, 2026-10-22: "look at the gestalt from a functional point of view (e.g.
glitter and bling deprioritized over functionality)". Priorities he set for
the standing improvement loop, in order: things that break, hang, lose work,
mislead or silently do nothing; then simplification and hardening; then CC's
names and behaviour; visual polish last.

The look still matters to him (`harness-ui.md` has what he liked), but no P0
or P1 item is visual except "text can be selected and copied".

### PRIN-13 It is his to inspect and change (P1)

Jae, 2026-10-19: "how do i inspect or edit the context window for each prompt
using crush?", "how can i see the remaining 6000 or so tokens of json
descriptions of the tools", "how do i edit that?", "make it written in a place
where i can easily modify from the local machine."

The rule: everything the model is sent can be printed by a command (the
system prompt, each tool's description and schema with its size, the context
by category), and the user can replace the system prompt, replace a tool's
description, or hide a tool, with files of his own. A replacement is
**all-or-nothing and visible**: the command that prints the prompt says where
it came from. (The earlier gnosh's override silently hid three weeks of
prompt changes from his own sessions; UI-30 says how that is avoided.)

### PRIN-14 Money and keys need a person (P1)

Signing a transaction and paying for a fetch each wait for the user to see
exactly what will happen and to type the key's password into a masked prompt.
A run with nobody at the keyboard has no signing or paying tool at all.
Unattended signing was deliberately **not** built. TOOL-40 to TOOL-46.

### PRIN-15 Tested against the real flow (P1, process)

On 2026-10-22 every change the box's own agent made passed its own tests,
yet none was safe to merge as it was. Jae's answer: the requirement to test
against the real flow should live in the project's own instructions, not only
in a task's prompt. On
2026-10-30, of an advertised example that failed in his lane though its test
passed: "shouldn't this stuff be tested? i thought there were tests".

The rule for the build side: a behaviour the user sees is proved by running
the real binary against a stand-in model server (one that speaks the provider
protocol and replays a script), not only by unit tests. A test that exists to
catch a failure is first shown failing. Tests never touch the user's real
config or data directories, and never name a real host, address or login
(the repository is public).

## 3. What is not carried over

Dropped on purpose. None of these was asked for by Jae; each was the earlier
program's inheritance.

| Not carried over | Why |
| :- | :- |
| A full-screen interface with mouse capture, sidebar, dialogs and themes | ARCHITECTURE.md decision 3; Jae asked three times why he could not select and copy text. `harness-ui.md` keeps the needs. |
| The upstream vendor's services: its hosted model provider and login, its provider catalog service, its analytics, its release check | PRIN-2. A built-in provider list is P2 at most (OQ-3). |
| Client/server mode (one backend, several terminal clients) | Jae never used it (unconfirmed); several flags had to be refused under it. `harness-cli.md` section 6. |
| Windows support | Dropped from CI on 2026-11-05 under Jae's "do the reasonable things"; the loop's note: "Jae is the only user; macOS + Linux". |
| A separate todo-list tool beside `mem` | Removed 2026-10-30: two lists split the work and the session-only one was lost overnight. `mem` is the only task list. |
| A hard-coded list of banned shell commands | Not Jae's; it caused friction and stopped nothing. OQ-5 records the one part he chose to keep (ssh goes through declared hosts). |
| Messaging "channels" (prompts arriving from a chat service) | Inherited; no sign Jae used them (unconfirmed). P2 if ever. |
| Commit and pull-request attribution lines added by the harness | Jae's own commits carry no agent trailers. Off by default if built at all (OQ-12). |
| Old-name warnings, renamed-file notices, migration helpers | PRIN-5. |

## 4. Every deliberate difference from Claude Code

One table, so the list is in one place. Each row is specified in the file
named. Differences the earlier gnosh merely inherited from its upstream, and
that Jae never asked for, are not here: they are in `harness-open-questions.md`
with a recommendation to follow CC.

| # | Where | Claude Code | gnosh | Why | File |
| :- | :- | :- | :- | :- | :- |
| D1 | A hook that could not run, crashed, timed out or answered unreadably | Passes (non-blocking) | Blocks, saying it could not check | PRIN-6, Jae 2026-10-23 | hooks |
| D2 | A hook's non-zero exit other than 2 | Non-blocking error | With text on stderr: blocks. Silent: no opinion | Keeps one-line test hooks working while failing closed | hooks |
| D3 | A CC hook entry gnosh cannot run as written (`if` filter, non-command types, background hooks, a matcher on an event that takes none) | Runs it CC's way, or ignores the part | Startup error naming it | PRIN-6 | hooks |
| D4 | A guard matcher (`Edit\|Write`, `WebFetch`) | Covers CC's tools only | Also covers gnosh tools that do the same job, for blocking only | PRIN-1 | hooks |
| D5 | Stop-hook persistence | Loop guard is the hook's own business | No cap; a reflection checkpoint every 10 blocks | PRIN-4 | hooks, agent-loop |
| D6 | Response-header timeout | 10 minutes | None for a local model server; 10 minutes elsewhere | PRIN-9 | local-models |
| D7 | Non-interactive run and permissions | Since CC 2.1.302 a plain `-p` denies what would prompt | `gnosh run` approves its own session; `--permission-mode dontAsk` gives CC's behaviour | Jae's unattended runs depend on it; OQ-2 | cli |
| D8 | Bash allow rules | Looks through some substitutions, safe assignments, subshells | Asks at any substitution, assignment, background job, subshell, loop or redirect to a file | Stricter is safe | tools |
| D9 | Untrusted folder | Hooks and `env` from project settings still run | Nothing of the project's runs or grants; only what restricts applies | PRIN-1 | trust-and-safety |
| D10 | Declining the trust question | (unconfirmed) | Starts with the user's own settings only; asks again next time | Jae's spec, 2026-11-01 | trust-and-safety |
| D11 | Thinking | An effort level per model | The same, plus `/think` and `/no_think`, which move the model to one end and stick | "They are in the fingers" | agent-loop |
| D12 | Compaction | The way a long session survives | A backstop only; frames and the slide come first | The other spec set; LOOP-60 | agent-loop |
| D13 | Session title | A helper model names it as the session starts | Named after the prompt at once; the model's title is asked for after the first turn; never for a sub-agent | A title request at the start evicts a local server's cache | local-models |
| D14 | Tool calls on other machines | ssh inside the command | A `host` on the call, a stamp on the result | PRIN-11 | tools |
| D15 | Signing and paying | No such tools | `gnokey`, `x402_fetch`, behind a masked prompt | PRIN-14 | tools |
| D16 | Telemetry, update check | On by default, opt-out | Off by default, opt-in | PRIN-2 | trust-and-safety |
| D17 | Environment block in the system prompt | Part of the system prompt | Sent as the first user message, so the system prompt and tools are byte-identical across sessions | 70 to 90 s saved per new session on the box | local-models |

## 5. The P0 list

The first binary Jae can use on the AI box needs, at least:

1. His config read without change in meaning: a llama.cpp provider at a base
   URL, a model with its context window, `request-timeout`; and his choice of
   model and thinking level kept once he has made it (CFG-1 to CFG-5, CFG-10
   to CFG-13, CFG-18), with every unknown word a startup error (CFG-40).
2. The llama.cpp provider path done right: streaming, the thinking switch, the
   context window read from the server, waiting as long as it takes, riding
   out a restart, and a request layout that keeps the server's cache
   (LM-1 to LM-16).
3. The turn loop: tool calls (several per reply), queued input, Esc that
   always stops, the repetition nudge, the empty-answer retry, replies left
   unfinished by a kill closed on reopen (LOOP-1 to LOOP-11).
4. Never over the window: the context limit handled before a request is sent,
   with an unknown window assumed small (LOOP-50 to LOOP-53).
5. Tool results that cannot flood or forge: output capped by raw size, control
   tokens defused (TOOL-5, SAFE-20).
6. The core tools with the P0 behaviours in TOOL-1 to TOOL-19. (Commands
   that move to the background, and the notice when one ends, are P1.)
7. `gnosh`, `gnosh "prompt"`, `gnosh -p` / `gnosh run`, `-c` / `-C`, `-r`,
   `-s`, `-y` / `--dangerously-skip-permissions`, `-m`, `--cwd`, `-D`, plain
   text output, warnings on stderr, exit codes 130 and 143 (the P0 rows of
   CLI-1 to CLI-10, and CLI-42 to CLI-44).
8. Context files read from the folder, the folders above it and the user's
   global files (CFG-20).
9. A project's own settings **not applied at all** until the trust gate exists
   (SAFE-1). That is the safe P0; the gate itself is P1.
10. `/think` and `/no_think`, sticky (LOOP-40).
11. Text on screen that the terminal can select, copy and scroll (UI-1).
12. Nothing phones home (PRIN-2), true of P0 because nothing that phones is
    built.
13. Anything the config names that is not built yet (a hook, a host, an MCP
    server) **stops startup with a message saying so**. It is never accepted
    and ignored (PRIN-6, HOOK section 9).
14. Nothing spins with nothing in flight, and a stuck session can be examined
    without killing it (LOOP-70).

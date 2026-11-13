# harness-open-questions.md — what the sources leave unsettled, with a recommended reading for each

What this file is: the places where the sources contradict each other, where
a decision was put to Jae and never answered, or where the rewrite forces a
choice the earlier gnosh never had to make. Each has a **recommended
reading**, so the build side never has to stop and ask. Jae can overrule any
of them; the ones marked **Jae** are his to settle because they touch his
own files, his habits or his name on something.

Order: the ones that most need him first.

---

## Needs Jae

### OQ-1 Is `gnoshrc` a list of settings or a script? And when do his own files move?

**The problem.** In the earlier gnosh the config file was a shell script run
by a shell built into the program. The new gnosh has no shell of its own
(ARCHITECTURE.md: standard library first). The AI box's file is plain
directives and loads as a list. **His Mac's file is not**: it sets two
variables and uses an `if` with a network probe to pick between the home
address and a local forward (CFG-1).

His standing rule (2026-10-23) is that old names are kept only for his own
files and dropped once they move, "and ask first".

**Recommended reading.**

1. `gnoshrc` is a list of settings (CFG-10, CFG-11). A shell construct in it
   is a startup error that names the replacement.
2. `--base-url` may be given twice, first that answers wins (CFG-12). His Mac
   file becomes four plain lines.
3. Until he says otherwise, the new gnosh reads his three old-named files in
   place (CFG-5), so the AI box works on day one with nothing touched. The
   Mac's file needs the edit before the Mac can run the new gnosh.
4. When he agrees: copy the three files to their gnosh names, with him, and
   delete the fallback.

**The other reading**, if he wants his file to keep working unedited: run
the file with the system's own `bash`, with `provider`, `model`, `option`
and the rest defined as shell functions that print one directive each, and
read what they print. That keeps every shell feature, at the price of the
config being code again, a `bash` dependency, and a slower start (his probe
alone waits up to a second when he is away from home).

### OQ-2 Does a plain `gnosh run` approve everything, or deny what would ask?

**The contradiction.** Claude Code changed in 2.1.302: `claude -p` now
denies whatever would need approval. The earlier gnosh kept approving,
because every unattended run on the AI box (reviews, scenario tests,
benchmarks, the self-improvement runs) relies on a run being able to act.
PRIN-3 says follow CC; PRIN-1 says start closed.

**Recommended reading.** Keep approving for now (CLI-31), because the box's
scripts are his own and all of them would break; print one line on stderr
saying the run is approving its own tool calls and how to restrict it
(`--permission-mode dontAsk`). Revisit once those scripts pass an explicit
flag. **Jae**: say if he would rather have CC's default from the start and
have the scripts pass `-y`. (That is arguably the cleaner end state, and
several of those scripts pass `-y` already.)

### OQ-9 `-c`

**The contradiction.** Earlier gnosh: `-c` is the working directory and `-C`
is continue; "a decision for Jae" since 2026-10-22, never made. Claude Code,
and `spec/cc/cli.md`: `-c` is continue. He types `gnosh -y -C`.

**Recommended reading.** CLI-30: `-c` and `-C` both continue; `--cwd` has no
short form; the scripts that pass `-c DIR` are changed.

### OQ-11 The exact sentence for the security rule

**The problem.** LOOP-100. His own sentence was made by editing a sentence
of the old instructions, so this spec gives the requirement and not the
words.

**Recommended reading.** The build side writes its own sentence from the
requirement. **Jae** reads it and replaces it with words of his own if he
wants; a wholly new sentence of his is his to give.

### OQ-13 Time limits for a local model: should the defaults just be right?

**The contradiction.** `request-timeout` defaulted to 120 seconds on
2026-10-20 (his config comment says "the default 2-minute wait") and to 60
by the end (the config guide). The first-piece budget is five times it.
gnosh.ai tells every new user to add `option request-timeout 900`. LM-1
already says a local server gets no limit on when the response *starts*, but
llama.cpp starts the response early and then goes quiet while it reads, so
the first-piece budget is the limit that actually bites.

**Recommended reading.** For a **local** server: no limit before the first
piece of a reply, and 900 seconds between pieces, by default. For a remote
API: CC's ten minutes to start, and 60 seconds between pieces.
`request-timeout` still overrides both. Then the advice line on the website
goes. Test what counts as a "piece" against a real server (LM-2).

### OQ-18 Thinking: on or off by default?

**What the sources say.** The earlier gnosh's default for a model that can
think was **on**. Jae's own sessions run with it **off**. The test harness
defaulted to off. Thinking on was right more often (6 of 6 against 3 of 6 on
one sum) and took 2 to 4 times as long.

**Recommended reading.** On by default; `/no_think` once and it sticks
(LOOP-40). His own state will say off after he types it once (OQ-6).

### OQ-26 The purple and the pink

**The problem.** His direction (UI-50) was "in the likeness of" the upstream
program's look, "but better, in our own colors", and then "keep the original
colors too". The purple and the hot pink in question were the upstream's own
brand colours. ARCHITECTURE.md says gnosh has its own look and borrows
interfaces, never art.

**Recommended reading.** Keep everything he asked for about the *shape* of
the look (night sky, purple and hot pink carrying it, fades through green
and gold, crisp, nothing soft) and choose a purple and a hot pink that are
gnosh's own, visibly not the upstream's. **Jae** picks from a few rendered
options. Nothing visual blocks P0 or P1.

### OQ-5 Banned commands

**The contradiction.** The earlier gnosh refused a fixed list of commands in
the shell tool even with permissions skipped (remote shells, downloaders,
package managers, `sudo`, service and network tools). It was inherited. It
stopped nothing (a full path or a script got past it) and it got in his way
(2026-10-24: his session wrote a script around ssh to get through, which is
how a remote path later got mistaken for a local one). It was "a decision
for Jae" and was never made; three days later hosts were built and the
README said "`ssh` and `scp` stay banned in commands".

**Recommended reading.** No general list. Commands ask, or run under `-y`;
deny rules block. One rule of gnosh's own stays: with hosts declared, a
command starting `ssh` or `scp` is refused with a pointer to hosts
(TOOL-22), because that is his design for where commands run. **Jae**:
confirm, since on his box, under `-y`, this means the model can run `curl`,
`sudo` (it has none) and package tools again.

### OQ-17 `/loop`

**The problem.** His `/loop` (2026-10-21) means "keep working until done",
and on 2026-10-30 he said "loop stays". Claude Code has a `/loop` that
repeats a prompt on a timer. `spec/cc/tools.md` already notes "gnosh's own
`/loop`".

**Recommended reading.** `/loop` keeps his meaning (LOOP-22). If CC's
timer-style loop is ever wanted it gets another name. Also worth his
thought: `/goal` now does most of what `/loop` was for, with a check instead
of the model's word.

### OQ-3 Which providers does gnosh know out of the box?

**The problem.** The earlier gnosh knew dozens of hosted providers and their
models through its upstream vendor's catalog (built in, and refreshable from
the vendor's servers). None of that is carried over. His own use needs one
kind: llama.cpp.

**Recommended reading.** P0: `llamacpp` and `openai-compat`. P1:
`anthropic`, `openai`, `ollama`, `lmstudio`, each with the user's own
`provider add` line and no built-in model list (models are discovered or
listed by hand). P2: a small built-in list of gnosh's own, if the public
promise "any model, hosted or on your own machine" needs it. No catalog
download, ever, without a setting.

---

## Settled by recommendation unless he objects

### OQ-4 Does a choice made inside gnosh outrank the config file?

Unconfirmed in the earlier gnosh. **Recommended:** CFG-4: the state file
outranks the user's `gnoshrc`, so `/model` and `/think` stick; and `/status`
shows where each value came from, so a line in `gnoshrc` that is being
outranked is visible.

### OQ-6 His old state and old sessions

The old state file holds his model choice and thinking level; the old
sessions are in a database the new gnosh does not read. **Recommended:** no
import (PRIN-5). He picks the model and types `/no_think` once. Old sessions
stay readable with the old program. If he wants them, a one-off converter is
a separate small job.

### OQ-7 The three-try limit on empty replies

It is a counter, which PRIN-4 forbids, and it was added four days after that
rule without anyone raising it. **Recommended:** LOOP-11: keep asking, and
tell the user at every third empty reply what is happening and what usually
fixes it.

### OQ-8 JSON settings

The earlier gnosh's JSON settings had their own shape (inherited), into
which Claude Code's `hooks` and `permissions` blocks could be pasted.
**Recommended:** CFG-19: the JSON file is Claude Code's shape only (`hooks`,
`permissions`, `env`), so "paste your block" still works, and everything
gnosh-specific is in `gnoshrc`. An unknown key is an error.

### OQ-10 Tool names

The earlier tools had lower-case names of their own and accepted Claude
Code's. ARCHITECTURE.md gives the new tools Claude Code's names.
**Recommended:** CC's names everywhere, including a hook's `tool_name`; the
old lower-case names are not accepted (none of his own files uses one,
unconfirmed for the box's `permissions` lines). Two details:

- gnosh's deliverables tool (TOOL-50) must not be called `Artifact`: Claude
  Code has a tool of that name that publishes to a hosted service, which is
  the opposite of what gnosh's does, and Jae's standing rule is that his work
  never goes to such a service. Call it by a name of its own.
- `tool_response` in hook payloads: Claude Code's shape per tool where
  `spec/cc/tools.md` defines one; always including the text the model reads.

### OQ-12 Lines added to commits

The earlier gnosh added an attribution trailer and a "generated with" line
to commits and pull requests its agent made, by default. Jae's own rule for
this project's commits is no agent trailers. **Recommended:** off by
default; an option turns it on.

### OQ-14 The session page

Jae asked for it on 2026-10-19 to learn how the conversation is built.
Editing a message does not fit an append-only transcript. **Recommended:**
UI-40: a read-only page with the "what the model gets" panel, P2; editing
decided later.

### OQ-15 Holes left open in the trust work

Each was noted and left on 2026-11-01:

- language servers still start in an untrusted folder, and some build the
  project;
- `printenv` and `env` are read-only commands and print API keys;
- git hooks other than the read-triggered one run on commands that ask
  anyway, with no warning;
- a session file (and, in the new gnosh, a `mem`) that came with a
  repository is used.

**Recommended:** hold language servers until trust (they are P2 anyway);
take `printenv` and `env` off the read-only list, or mask values that look
like keys; say in the question for a commit or checkout when the repository
has hooks; show, when resuming a session in an untrusted folder, that it
came with the folder.

### OQ-16 Summaries

Two things. (a) The earlier gnosh used Claude Code's long summary form; on
the box a summary is a median of 4,500 tokens and five minutes, and can be
nearly half the size of what it replaced. (b) Idle compaction was on by
default, and with frames it should rarely have anything to do.
**Recommended:** a shorter form of gnosh's own, measured on the box before
it replaces the long one; idle compaction stays, P2.

### OQ-19 Where gnosh's own mid-turn notes go

LM-13. As a user message they are free on his box and cost the whole prompt
on some other models; inside a tool result they are cheap everywhere but mix
gnosh's words with untrusted output. **Recommended:** send them as a
user-role message (the earlier behaviour), wrapped so the model can tell
they are gnosh's; and make it a per-provider setting only if a model Jae
uses turns out to pay for it. Keep the trust line.

### OQ-20 Why the earlier gnosh was slower than its upstream, and what follows

Measured 2026-11-06/07 on four tasks, same model: both passed everything,
and the upstream was 1.3 to 4 times faster on every task at a 64K window. At
a 32K window the upstream failed twice and gnosh (with one fix) was the only
one to finish. The causes found, none of them bugs:

1. gnosh's checking discipline (work it out in Gno; nothing is done until
   `mem` shows it; a review of every close). The other spec set.
2. One tool call per reply (LOOP-2).
3. A fixed prompt about 3,300 tokens larger (LOOP-104).
4. Side requests (LM-14).

**Recommended for the harness:** treat LOOP-2, LOOP-104 and LM-14 as
performance requirements with numbers, and measure the new gnosh on the same
four tasks before any claim. Whether the discipline should be lighter on
short tasks is the other set's question and Jae's.

### OQ-21 Old variable names for hooks, and the agent marker

The earlier gnosh set every hook variable under three prefixes, and kept its
upstream's name as the value of `AGENT` and `AI_AGENT`. **Recommended:**
`GNOSH_*` only, plus Claude Code's two (HOOK-30); `AGENT=gnosh`,
`AI_AGENT=gnosh`. Before P1, read his two hook scripts on the box; if either
reads an old name, edit the script.

### OQ-22 Analytics

Built at his request (2026-10-23) with a collector of gnosh's own and a
public page of counts. **Recommended:** not in P0 or P1. If it returns,
exactly as SAFE-32.

### OQ-23 Tools that reach out

The earlier gnosh had a code-search tool that queried a public site without
asking, and a web search that used a free search page and fell back to
another. **Recommended:** no tool reaches a site without asking (TOOL-72).
Web search is P2 and needs a search source Jae chooses; the code-search tool
is not carried over.

### OQ-24 Inherited hook-protocol extras

HOOK section 8. **Recommended:** follow Claude Code for all four; keep exit
3.

### OQ-25 Client/server mode and channels

CLI-50, PRIN section 3. No sign he used either (unconfirmed).
**Recommended:** not carried over.

### OQ-27 Desktop notifications

He has them disabled. **Recommended:** accept the option at P0 so his config
loads; build `bell` and `osc` at P2; `native` later if asked.

---

## Smaller contradictions, resolved here

| # | The sources disagree about | Reading taken |
| :- | :- | :- |
| 1 | A hook's default timeout: the hooks guide's reference table says 60 s; its prose and the change log say 600 s, and 30 s for `UserPromptSubmit` | 600 and 30 (the table was stale). HOOK-7. |
| 2 | Whether a session can end twice: the guide says "a session ends once"; a change says "a session left twice ends twice" | Each time a session is left after being entered. Switching away and then quitting is one end. HOOK-18. |
| 3 | `SessionStart` sources: four in one table, five elsewhere | Five (`fork` was added last). HOOK-16. |
| 4 | When a command moves to the background: 60 s was "kept" on 2026-10-24 and changed to CC's 2 minutes the next day | 2 minutes. TOOL-15. |
| 5 | How many tools: 26, 27 and 29 at different dates | It varied with features; the final list is TOOL-1. |
| 6 | What the repetition guard does: an analysis of 2026-11-04 says it "stops a turn" | It has nudged since 2026-10-22, on Jae's ruling. LOOP-10. |
| 7 | Which old names are read: the README says every klash and crush name still works; Jae's rule says only his own files' | His rule. CFG-5. |
| 8 | Where analytics went: one address on 2026-10-23, another from 2026-10-24 | Moot (OQ-22). |
| 9 | The session page's port: the same default as the deliverables server at first | Different ports; a free one when taken. UI-40. |
| 10 | Whether read-only shell commands being held to the project differs from Claude Code | It does not; the README says "as in Claude Code". SAFE-11. |
| 11 | Whether the web-reading tool or the plain fetch is Claude Code's `WebFetch` | For guards, both (HOOK-14). For the tool list, `spec/cc/tools.md`. |

---

## Things I could not confirm

- The AI box's files as they are today (read from notes, not from the box):
  the exact `gnoshrc`, whether any `permissions` line uses an old tool name,
  whether a hook script reads an old variable name.
- Whether Jae ever used client/server mode, channels, MCP servers or
  language servers in his own sessions.
- Whether llama.cpp's keep-alive bytes during a long read count as stream
  activity (LM-2).
- What Claude Code does when the trust question is declined.
- Whether the state file outranked the config file in the earlier gnosh
  (OQ-4).
- The cause of the 32K overflow (LOOP-53).

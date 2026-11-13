# harness-tools.md — the tools, as Jae ended up with them

What this file is: the earlier gnosh's tools at the end, the behaviours each
one needed before a local model could work with it, the tools that are
gnosh's own (commands on other machines, signing with `gnokey`, paying with
x402, recording deliverables), and the permission rules around them.
`spec/cc/tools.md` and `spec/cc/permissions.md` give Claude Code's tool
names, arguments and rules, which the new gnosh uses. This file does not
repeat them: it says what the earlier gnosh had, how that maps onto Claude
Code's names, and what must be true beyond Claude Code's documentation.

In one paragraph: the model gets a small set of tools to read, search, edit
and run things. Almost every rule below exists because a model on Jae's box
lost minutes to its absence: a read that failed outright on a big file, an
edit refused with no reason, a wait that never ended, a "permission denied"
that sent it hunting for sudo. A tool's result is the cheapest place to
teach the model, so every refusal says what to do next. Tools that touch
keys or money stop for a person.

Read first: `harness-00-principles.md` (PRIN-7, PRIN-11, PRIN-14),
`spec/cc/tools.md`, `spec/cc/permissions.md`.

---

## 1. The tool set

### TOOL-1 What the earlier gnosh had, and the Claude Code name for each (P0)

The earlier gnosh's tools had lower-case names of their own. Claude Code's
names for them were accepted everywhere a tool is named (flags, permission
rules, hook matchers). The new gnosh's tools carry CC's names.

| Earlier gnosh | Claude Code | Arguments (earlier gnosh) | New gnosh |
| :- | :- | :- | :- |
| `view` | `Read` | `file_path`, `offset`, `limit`, `allow_large` | P0 |
| `write` | `Write` | `file_path`, `content` | P0 |
| `edit` | `Edit` | `file_path`, `old_string`, `new_string`, `replace_all` | P0 |
| `multiedit` | (`MultiEdit`, legacy) | `file_path`, `edits` (a list of `old_string`, `new_string`, `replace_all`) | Not a separate tool: several `Edit` calls in one reply (LOOP-2). The name still counts as `Edit` for guards. |
| `bash` | `Bash` | `command`, `description`, `working_dir`, `timeout` (ms), `run_in_background`, `auto_background_after` (s), `host` | P0; `host` P1 |
| `job_output` | (`BashOutput`, removed) | `shell_id`, `wait` | CC's way now: a background command's output is a file read with `Read`. TOOL-15. |
| `job_kill` | `TaskStop` | `shell_id` | P1 |
| `grep` | `Grep` | `pattern`, `path`, `include` / `glob`, `type`, `literal_text`, `ignore_case` / `-i`, `output_mode`, `-A`, `-B`, `-C`, `head_limit`, `offset`, `multiline` | P0 |
| `glob` | `Glob` | `pattern`, `path` | P0 |
| `ls` | (`LS`, removed) | `path`, `ignore`, `depth` | Not adopted; a directory is listed with a shell command. |
| `agent` | `Agent` (`Task`) | `prompt` (the tool asks the model to say in it how broad a search is wanted) | P1 (LOOP-80). `spec/cc/tools.md` lists it P0; Jae's first binary can do without it. |
| `question` | `AskUserQuestion` | `questions` (each: `label`, `type`, `question`, `description`, `choices`), `confirm_title`, `confirm_description` | P1 (TOOL-45) |
| `fetch` | `WebFetch` | `url`, `format` (`text`, `markdown`, `html`), `timeout` | P1 |
| `agentic_fetch` | (part of `WebFetch`, `WebSearch`) | `url`, `prompt` | P1 (LOOP-80, SAFE-21) |
| `download` | (none) | `url`, `file_path`, `timeout` | P2 |
| `sourcegraph` | (none) | `query`, `count`, `context_window`, `timeout` | P2; OQ-23 |
| `list_mcp_resources`, `read_mcp_resource` | `ListMcpResourcesTool`, `ReadMcpResourceTool` | | P2 |
| `lsp_diagnostics`, `lsp_references`, `lsp_definition`, `lsp_symbols`, `lsp_call_hierarchy`, `lsp_rename`, `lsp_replace_symbol`, `lsp_restart` | `LSP` (read-only) | | P2 |
| `gnokey` | (none) | `command`, `description`, `working_dir` | P1 (TOOL-40) |
| `x402_fetch` | (none) | `url`, `key`, `master`, `remote`, `home`, `format`, `description` | P2 (TOOL-46) |
| `artifact` | (CC's `Artifact` is a different thing) | `label`, `target`, `host` | P1 (TOOL-50) |
| `gnosh_info`, `gnosh_logs` | (none) | | P2 (TOOL-55) |
| `gnovm_run`, `gnovm_new`, `gnovm_list`, `gnovm_drop` | (none) | | The other spec set, `machines.md`. |
| `todos` | `TodoWrite` | | **Removed 2026-10-30.** `mem` is the only task list. A matcher on `TodoWrite` matches nothing. |

Tools only an interactive main agent has (never a sub-agent, never a run):
`question`, `gnokey`, `x402_fetch`, `artifact`.

### TOOL-2 One list of names (P0)

There is exactly one list of tool names in the program, and everything that
names tools reads it: the config check, the flags, the hook matchers, the
sub-agents' tool sets. A test proves that every tool the program can build
is really offered to a model. (A new tool once passed every test while no
model was ever given it, because its name was missing from a second,
hand-kept list.) The check on a real binary is `gnosh prompt tools`.

### TOOL-3 A tool's result teaches (P0)

PRIN-7. Every refusal and every failure the model can act on is an ordinary
tool result, in plain words, saying what happened and what to do next. It is
**not** an error that ends the turn. Three failures that used to end the
whole turn and now return an explanation: reading a path under a directory
the user cannot enter; writing there; editing there.

### TOOL-4 Tool input is shown as written (P0)

LOOP-4. And a tool call's one-line summary for logs and `--verbose` is
always one line.

### TOOL-5 Output is capped by its real size (P0)

A command once printed a compiled binary. It was nearly all characters with
no width on screen, so a megabyte measured as "short", passed the cap, was
stored as a 695,000-token result, and was resent on every later round until
the server refused the session for good.

- The cap counts **characters, not screen width**.
- Over the cap, the model gets the head and the tail, cut on character
  boundaries, with a line saying how much was left out and where the whole
  output is (a file it can read).
- Limits are CC's (`BASH_MAX_OUTPUT_LENGTH`, CFG-51), and the same cap
  applies to MCP tools' text.
- The whole output is not copied again and again while a command runs. (It
  was, ten times a second.)

### TOOL-6 Untrusted text is defused (P0)

SAFE-20: a model's control tokens inside any tool result are made harmless
before the model reads them.

### TOOL-7 A path that is not there, and a path that is closed (P1)

- **Not there:** as CC's tools answer: the file does not exist, what the
  working directory is, and a likely intended path when there is one (the
  same path inside the working directory; a file beside it with the same
  name and another extension).
- **Closed:** "permission denied" names the directory that actually
  refused, says nothing under it can be reached from here, and says that
  whether the path exists cannot be told. The model on the box read a bare
  "permission denied" on a path that belonged to another machine as a sudo
  problem, twice.

### TOOL-8 What a command's environment holds (P0)

Every command gnosh runs for the model sees `GNOSH=1`, `AGENT=gnosh` and
`AI_AGENT=gnosh`, so a script can tell it is run by an AI agent. (The
earlier gnosh kept its upstream's name in the last two "since other tools
may match on that value"; OQ-21.) The lines a SessionStart hook put in the
session's environment file run first (HOOK-23).

Tests of gnosh itself must not inherit the user's `GNOSH_*` settings through
this environment: the box's own agent once chased test failures that came
from the wrapper's variables.

---

## 2. Reading, searching, editing

Everything in this section is "as Claude Code does" unless marked. Each line
is here because its absence cost real turns; `spec/cc/tools.md` has the full
rules.

### TOOL-10 Read (P0)

- **2,000 lines by default.** At 200 lines the local model paged through
  every source file, one round trip each.
- **`offset` is the line number to start at**, the first line being 1. It
  used to be a count of lines to skip, so a view "from line 120" started at
  121.
- **A read over the size limit is cut, not refused**: at the last line that
  fits (25,000 tokens, `CLAUDE_CODE_FILE_READ_MAX_OUTPUT_TOKENS`), with the
  offset to read on from. A hard 200 KB failure once turned an 828-line log
  into nothing but an error, and at the old limit one read of a log took 40%
  of a local model's context.
- `allow_large` reads a section over the usual limit, up to what still fits
  in the context (P2).
- **An unchanged file's lines are given once.** A second read of lines the
  model already holds says the file is unchanged rather than sending them
  again. Known cost: something judging the conversation later (the close
  review) sees only "unchanged", not the content.
- A subdirectory's context files arrive with the first read there (CFG-20).

### TOOL-11 Reading counts, wherever it happens (P0)

A file is "read" for the purposes of editing when the model has seen it:

- through `Read`;
- through a shell command that shows **that one file**: `cat`, `head`,
  `tail`, `sed -n '10,20p'`, `grep` or `rg` on it. Only the one file shown
  counts. (One run lost four edits in its first seven minutes to "read the
  file first" after reading with `sed -n`.)

And the reverse, **what the agent read and no longer holds**:

- When a file the agent has read changes on disk by anything other than its
  own edit tools (a command of its own, the user's editor, a formatter, a
  background job), **the next tool result says so**, naming the files.
- A shell command's result names the read files that command changed.
- A file written back exactly as it was (`git stash` then `pop`) has not
  changed.
- A change of the same size within one tick of the clock is still a change.
- A resumed session is not told that every file it ever read has changed.
- Reads made inside a frame that was popped are forgotten (`frames.md`).

### TOOL-12 Images (P1)

- An image larger than the limits is **scaled down to fit** and sent (CC's
  limits: 2000 by 2000 pixels, about 5 MB encoded). The earlier gnosh
  refused every image over 200 KB, which is most screenshots.
- A model that cannot see images is told **what the image is** (type, size,
  dimensions) and that it cannot see it. A bare "this model does not support
  image data" sent the agent off to set up a second model on another
  machine, or to measure pixels with a script.
- A screenshot's file name is tried with the other kind of space before
  "AM" or "PM" (macOS uses a narrow no-break space there).

### TOOL-13 Edit and Write (P0)

- **The match is forgiving about typography**: an `old_string` written with
  straight quotes finds text with curly ones, and the common escaped forms,
  as CC's Edit does.
- **A file changed since it was read.** The edit is still made when its
  text matches the file as it is now **exactly and only once**; the model is
  told the file holds changes it has not seen, and the file is then treated
  as unread. Any looser match is refused, with word to read the file again.
- A refused edit says **why** and what to do: the file changed since it was
  read, what may have changed it, read it again.
- A write to a file that was never read says so. (It used to say the file
  had "changed" at the year 1.)
- An edit or write that went in whole says the file **need not be read
  back**, as CC's do.
- Line endings are kept as they were.
- What a file held before each change is kept, so `/rewind` can restore it
  (UI-23). Changes made through the shell are not tracked, and the rewind
  says so.
- Several edits to one file in one reply: each failed edit says why it
  failed. ("No changes made, all edits failed" sent the model back to
  re-read the file and guess.)

### TOOL-14 Grep and Glob (P0)

- Grep takes CC's parameters: lines of context before and after, a file
  filter, a file type, case-insensitive matching, the output mode (matching
  lines, only file names, or counts), a head limit and an offset to page
  with. The earlier gnosh dropped the ones it did not know **without a
  word**, so a model asking for context lines got none. An argument a tool
  does not take is an error that names it, never silence.
- Results come 250 lines or entries at a time, paged with `offset`.
- **A pattern that starts with a dash** is searched for, not read as an
  option of the search program.
- **An empty result for a pattern with regex characters says so**: a search
  for `func (s *service)` found nothing because the parentheses were groups,
  and the model moved on as if the method did not exist. The hint names
  literal matching.
- A search is **bounded by its timeout**, and a search cut short says so. A
  search that could not read some file still returns what it found.
- Glob: an absolute pattern searches where it points; newest files first; a
  search cut short by its timeout says so (`CLAUDE_CODE_GLOB_TIMEOUT_SECONDS`).
- Both ask before searching **outside** the working directory, and both pass
  by links that lead out of the folder searched (SAFE-10).

---

## 3. Running commands

### TOOL-15 Background commands (P1)

As CC (`spec/cc/tools.md` section 6.4), with these points from the earlier
gnosh:

- A command moves to the background by itself after **2 minutes**
  (`BASH_DEFAULT_TIMEOUT_MS`). It was 60 seconds; then every long build or
  render moved to the background and was polled, each poll a round over the
  whole conversation (at 180,000 tokens a round, for Jae's video jobs).
- The call's own `timeout`, in milliseconds as CC's, asks for longer, up to
  `BASH_MAX_TIMEOUT_MS`. The earlier gnosh ignored an argument it did not
  know, so a model asking for a ten-minute test run got 60 seconds.
- At most `max-background-jobs` (50) run at once; finished ones do not
  count.
- An unknown job ID is answered with the jobs there are: each ID, whether it
  is running or how it exited, and its command's first line; or that there
  are none. A stop of a job that had already exited says so, with its exit
  code.
- A job the shell refused to start is reported with the reason, not as a
  bare exit code 1.
- `/tasks` lists and stops them (UI-20).
- They are all stopped, with their process groups, when gnosh exits
  (CLI-43).

### TOOL-16 The agent is told when a background command ends (P1)

As CC. The lead came from Jae's own session, which polled for job output
162 times.

- When a background command ends, a notice goes to the session that started
  it: which command (its description, or its first line), how it ended
  (finished with exit code 0, failed with exit code N, was stopped, could
  not run), and where its output is.
- A running turn gets the notice between rounds; an idle session starts a
  turn for it (LOOP-8).
- **Each end is announced once.** Whoever sees the end first claims it: a
  read of the output that shows it ended, a stop, gnosh exiting, or the
  notice. No turn is started once gnosh has begun to shut down.
- The conversation shows one quiet line for it.
- When a command goes to the background the model is told it **will be told
  when it ends, and need not poll**.
- Only an interactive main agent gets notices. In a run and in sub-agents
  there are none, and the wording there must not promise one.
- Not built, from CC (P2): the notice that a command seems to be waiting
  for typed input (no new output for 45 seconds and the last line looks like
  a question).

### TOOL-17 Nothing a tool waits on is unbounded (P0)

Each of these once hung a turn:

| Wait | Rule |
| :- | :- |
| Waiting for a background job to finish | At most 5 minutes, then return the output so far and "still running". The model may wait again. (It waited 54 minutes once, with Esc dead.) |
| A file search | Bounded by its timeout; never falls back to a slower search that has no limit. |
| A code search on the web | Waits as long as the timeout the model gave it. (It was fixed at 30 seconds whatever the model asked, so 11 of 17 calls failed.) |
| A web search that is being rate-limited | Falls back to another search source rather than failing. |
| A piped stdin that never closes | CLI-3. |

### TOOL-18 A long foreground sleep is refused (P1)

As CC: a command that begins with `sleep N`, N of 25 seconds or more, is
refused before any permission question, with what to do instead: to wait
for a condition, run a loop that checks it in the background (the agent is
told when it ends); to wait for a command, run that command in the
background. Shorter sleeps chained together to get around this are named as
not the way. This applies where end notices exist (TOOL-16); elsewhere a
sleep is allowed. In Jae's own sessions 34 foreground commands started with
a sleep of 25 seconds or more.

### TOOL-19 Notes under a failed command (P1)

- **A gnosh tool typed as a command** (the model typing a tool's name into
  the shell): "that is a tool; call it as a tool".
- **A command that is not installed**: beside the shell's own message, a
  note that it is not installed here, to use another way or ask the user
  where it is, and not to search the disk for it. A lookup that finds
  nothing (`command -v x`, `which x`) prints nothing, so the same note is
  added to it.
- **A command's exit code is part of its result** and of the record kept.

---

## 4. Other machines: hosts

### TOOL-20 `host` on a command, and a stamp on every result (P1)

PRIN-11. Built 2026-10-27, after Jae's session retried a script at a path
that only existed on a remote machine. It had learned the path through a
wrapper script the model wrote around ssh, so a result from the remote
machine looked exactly like a local one.

- The shell tool takes `host`: the name of a declared machine (CFG-14).
  Left out, the command runs here.
- gnosh runs the command there through the system's `ssh`: never stopping to
  ask (batch mode), a 20-second connect limit, accepting a machine's key the
  first time and refusing a changed one, the command passed as one argument
  so nothing in it is read by the local shell. With a working directory, it
  changes into it first and stops if it cannot.
- **Every shell result, local ones included, begins with where it ran**:

  ```
  <host>render (root@203.0.113.10)</host><cwd>/root/renders</cwd>
  <host>local (dev@box)</host><cwd>/home/dev/project</cwd>
  ```

  A result with no output carries the stamp too.
- The record kept of each call holds the host name and the machine, so a
  logged shell call is the tuple (host, machine, directory, command, exit
  code).
- The model is told, with its environment, which machine it is on and as
  whom, and which hosts are declared with their descriptions; that **a path
  belongs to the machine it was seen on**; and that the file tools reach
  only this machine.
- An unknown host is answered with the declared names. With none declared,
  the model is told to ask the user to declare one, not to reach a machine
  some other way.

### TOOL-24 What a shell call's result holds (P0 without the host parts)

To the model, in this order: the stamp (TOOL-20), the command's output with
standard output and standard error together, or the words "no output", and
the exit code when it is not zero. For a command that moved to the
background: that it did, its ID, where its output is, and that the agent
will be told when it ends.

Kept with the call for the record, for `--verbose` and for hooks: the
description the model gave, start and end times, the working directory, the
host name and machine, the exit code, and whether it is a background job
and which.

### TOOL-21 What is and is not checked for a command on a host (P1)

- It **always asks**, when anything asks at all. The read-only shortcut and
  the allow rules are about this machine.
- "Allow for this session" there covers **that host**, not a local
  directory of the same name.
- The user's **deny** rules are applied to the command before it leaves.
- The question names the host.
- With permissions skipped (`-y`), as Jae runs the box, nothing asks: the
  declaration is the whole gate.

### TOOL-22 `ssh` and `scp` typed into a command (P1; OQ-5)

With hosts declared, where a command runs is a fact of the call. So a
command that starts `ssh` or `scp` is refused with a pointer to hosts. This
is the one piece of the earlier gnosh's inherited list of banned commands
that Jae's design kept. The rest of that list (downloaders, package
managers, `sudo`, service and network tools) was never his and is not
carried over: commands ask, or run under `-y`, and the user's deny rules
block. OQ-5.

### TOOL-23 The follow-ups Jae discussed (P2)

1. The file tools take `host` too, so the model never needs `cat` over ssh.
2. The next coordinates: `user` (as root or as the agent's own account) and
   `container`.

---

## 5. Permissions

`spec/cc/permissions.md` has modes and rule syntax. These are the points the
earlier gnosh had to get right.

### TOOL-30 When a command runs without asking (P1)

A shell command runs unasked only if **every part of it** either only reads
or matches an allow rule.

The earlier check looked at the command's first word, so each of these ran
unasked: a read-only word followed by a second line that wrote; `ls &
touch x`; `echo x > file`; `timeout 5 touch x`.

"Only reads" means, for each simple command in the line:

- the program is on a fixed list of read-only programs, with per-program
  checks of its arguments (a date command that does not set the date; `git`
  only with read-only subcommands);
- no variable assignment, no background job, no subshell;
- redirections only from files, to the null device, or between file
  descriptors;
- everything it reads lies **inside the project** (SAFE-11);
- `cd` into a directory of the project is read-only, so `cd sub && ls` does
  not ask.

### TOOL-31 Allow rules (P1)

`Bash(pattern)` rules match as CC's (`spec/cc/permissions.md`): `*` is any
text; a trailing ` *` or `:*` also matches the bare command; the wrappers CC
strips come off first; a compound command needs every part allowed; a rule
with a `*` never approves a command that runs whatever it is given (`watch`,
`setsid`, `flock`, a `find` that executes, deletes or writes).

**gnosh asks where CC would look further** (deliberate, stricter): at a
command substitution, any variable assignment (even a harmless one), a
background job, a subshell or loop, a redirect to a file, and a program name
that is not plain text.

### TOOL-32 Deny rules (P1)

A `Bash(pattern)` deny rule stops the commands it matches **wherever they
appear** in a command line: after `&&`, inside a substitution, in a loop
body. `Bash(git push *)` stops `cd x && git push` and not `git status`.

### TOOL-33 Protected files always ask (P1)

A change to a protected file asks even when the tool is allowed, the mode is
`acceptEdits`, or a hook approved it: the repository's `.git`, shell startup
files, the settings of gnosh and of other tools, key files. Names match in
any letter case, and a file is judged by where it really is (links
followed). Only skipping permissions altogether skips this.

### TOOL-34 "Allow for this session" (P1)

- It is not offered for a request that is asked every time (a protected
  file; a hook's `ask`).
- A grant covers the file where it really is, so a grant for a link's
  target matches the next request for the same file.
- A denial **by the user** ends the turn. A denial by a hook or by `dontAsk`
  mode lets the model read why and go on.

### TOOL-35 The question itself (P1)

A permission question shows exactly what will run: the command, the host
when there is one, where a link leads when the path is a link out of the
folder, and a note when the repository's own git config would run a program
(SAFE-15). Names and commands taken from a stranger's files are shown
escaped, never as terminal control sequences.

### TOOL-36 Known holes, recorded (P2; OQ-15)

- `printenv` and `env` count as read-only and print the environment's API
  keys to the model.
- Git hooks other than the one that fires on reads run on commands that ask
  anyway (commit, checkout), with no warning.

---

## 6. Keys and money

### TOOL-40 `gnokey`: signing with a password the model never sees (P1)

Jae, 2026-10-22: build "a real Gno integration (gno toolchain, realm deploy,
gnokey signing)".

The model passes a `gnokey` command line as it would be typed (`command`),
one line saying what it does (`description`), and optionally a directory.

1. gnosh asks permission as for a shell command, showing the exact command.
2. If the command **signs** (a transaction that is broadcast, or `sign`),
   gnosh asks the user for the key's password in a **masked prompt** that
   names the key and shows the exact command, and says when the chain is
   mainnet.
3. The password goes to `gnokey` on its standard input, through gnokey's
   own flag for that, which **gnosh inserts itself** in the right place.
4. The password is removed from the output before anything else sees it.
5. The model gets the output. It never sees the password, and neither do
   the transcript, the logs or any hook.

Rules:

- Queries, broadcasting an already-signed transaction, listing keys and
  verifying need no password.
- Building a transaction **without** broadcasting needs no password and
  prints the unsigned transaction.
- **Key management is refused outright** (add, delete, rotate, import,
  export, generate). It stays in the user's own terminal.
- A model that passes the password flag itself is refused.
- Esc at the password prompt ends the turn. A wrong password is reported as
  gnokey reports it; nothing moved.
- Creating and revoking **session keys** goes through the same prompt.

### TOOL-41 The three doors closed around it (P1)

The masked prompt is the only route to a signature.

1. **The shell tool refuses gnokey's password-on-stdin flag**, in either
   spelling and behind a pipe. That flag is the only way gnokey takes a
   password without a terminal, so the shell cannot sign; queries and
   unsigned transactions still work there.
2. **Key management is refused** (above).
3. **A run with nobody to type has no signing tool at all.** The model is
   told, by the Gno skill, to print the exact command for the user and
   stop.

Why door 3 needed door 1: in a headless run, told it could not sign, the
local model did not print the command. It spent its whole
262,000-token context in the shell looking for another way to supply the
password, and the run died having signed nothing. The boundary held; the
shell route was then closed and the skill reworded.

The model is also told never to ask for a password in the conversation or
through the question tool, and never to try to sign through the shell.
Asking in a plain question would put the password in the transcript; that
part is guidance only, and worth enforcing if it is ever seen (P2).

### TOOL-42 Session keys, as the safety story (P1)

gno.land lets a master key authorize a second key within limits the chain
enforces: an expiry, a spend limit per period (gas and storage deposits
included), the kinds of message it may send and the realms it may call, and
a floor no setting overrides (a session can never create or revoke sessions
or add a package). Signing with a session key bounds the damage when a
confused or hijacked agent gets the user to approve the wrong transaction.

What gnosh must **not** claim: that it signs without the user. Every
signature waits for the user at the prompt. Unattended signing with a
session key was designed and deliberately **not built**.

### TOOL-45 Questions, and the masked answer (P1)

The question tool is CC's `AskUserQuestion` (`spec/cc/tools.md`), with free
text as well as choices. Several questions in one call are shown as a set of
tabs with a confirmation at the end, which Jae used for answering batches.

A question can be marked **secret**: a single masked line; nothing trimmed;
an empty line not accepted; the answer returned to the tool that asked and
recorded nowhere. **Only gnosh's own tools can mark a question secret**; the
model's question tool cannot.

Stopping a turn while a question waits must say so in plain words (LOOP-5).

### TOOL-46 `x402_fetch`: paying for a URL, with a person at the prompt (P2)

Some web resources answer "402 Payment Required" with a price on a gno.land
chain (the x402 protocol). When the user tells the agent to pay:

1. A permission question names the tool, the URL, the key, and that a price
   and a password come next.
2. gnosh reads the seller's terms, picks an offer it may pay, builds the
   payment and checks it against the chain.
3. The masked prompt names **the price, the payee, the chain and the
   network fee**, and the payer.
4. `gnokey` signs one bank send of exactly that price; gnosh verifies the
   signature before the payment leaves.
5. The paid request goes out **once**, on a connection of its own, to the
   seller only, following no redirect.
6. The model gets the content and a receipt: the transaction's hash and the
   payer.

Arguments: `url`, `key` (a gnokey key name or address), `master` (only when
`key` is a session key: the account that registered it), `remote` (a node of
the seller's chain), `home` (gnokey's home directory holding the key),
`format`, `description`.

Limits, all enforced **before** the user is asked:

- only local and test chains; **mainnet is refused**;
- only the exact-price scheme, only the chain's base coin;
- at most 1 GNOT per payment, and a network fee of at most 1 GNOT on top.
  (The fee was once uncapped: a node quoting an absurd gas price would have
  made a 0.25 GNOT purchase cost 999.7, and the node can be one the model
  names.)
- a payee that is not the zero address; a key that is not a multisig; a key
  that is the payer's or the session's;
- the chain is read with a 15-second limit.

After a payment has left, gnosh says **only what it knows**: a failed
request says the payment may have settled and names its hash; any status
below 400 is a payment made. A refused price names the node that reported
it and tells the model not to go looking for another node.

Hints that were needed: a call without `home` says the default key store
was searched and to pass the home the user named; a session key without
`master` (which reads as an unfunded account) says to pass `master`.

Paying is attended only. The plain fetch tool, on meeting a 402 with x402
terms, says what they are.

---

## 7. Deliverables

### TOOL-50 `artifact`: naming what the user asked for (P1)

Jae, 2026-10-24: the interface should show "the main thing requested, not
necessarily all versions but the most recent versions", such as the path of
a video he had just been handed. He chose "the agent tags it" over gnosh
guessing paths out of replies.

- The model calls it with a short stable `label` and a `target`: a file
  path or a URL the user would open.
- Reusing a label replaces the earlier entry, so the list shows the latest
  version of each thing.
- The list is built from the calls recorded in the session, so it survives
  a restart with nothing extra stored.

What the model gets back: that the label is recorded (or claimed), its
target, and the URL when the file is being served (TOOL-52).

### TOOL-51 Named before it is made (P1)

In 4,583 messages of one long session the tool was never called once, though
its description asked for it. So the deliverable is named **before** the
work:

- A call with a label and no target **claims** it: it shows as pending, and
  the user sees what is being worked toward.
- A second call with the same label and the target makes it real.
- A claim never delivered stays visible.
- A target that is a file **must exist**, so recording a deliverable is
  proof that it exists, not one more claim.

### TOOL-52 Served at a URL (P2)

gnosh usually runs on a machine the user is not sitting at, where a path is
of no use to him.

- Each recorded file is served over HTTP at a URL holding a long random
  token; the register is kept in the project's data folder, so a URL
  outlives the session that made it.
- The server listens on **this machine only** (`127.0.0.1:7777`) by
  default; the user brings it to his own machine with an ssh port forward.
  `option artifact-server` moves it or turns it off. If the port is taken, a
  free one is used.
- Media types are set from gnosh's own table, so a video plays in the
  browser instead of downloading.
- With `host`, a file on a declared machine is copied back and served.
- A non-interactive run never opens the port.

### TOOL-55 The model can ask gnosh about itself (P2)

Two tools let the model read gnosh's own state (the settings in force, the
declared hosts) and its log, so that "why did that not work?" can be
answered from inside a session. A built-in skill explains how to configure
gnosh and how to write hooks, so the user can say what he wants configured.

---

## 8. Skills gnosh carries

### TOOL-60 The Gno skill (P1)

A skill built into gnosh that the model opens when a task touches `.gno`
files, a Gno module file, `gnokey`, or a local chain. What it must cover:

- how Gno differs from Go, how realms keep state and tell who is calling;
- testing, running a local chain as a background job, querying a chain for
  free, deploying and calling;
- the networks with their chain IDs and endpoints, mainnet called out
  because its coins are real;
- the signing rules of TOOL-40 and TOOL-41, in the model's own terms;
- **the two guides Jae's audits use** (the interrealm specification and the
  security guide), carried with the skill so every project has them, and the
  rule of LOOP-102;
- storage and cost advice **as measured**, not as folklore.

A skill is opened at the model's choice, and one run in three never opened
it. That is why the audit rule is also in the instructions (LOOP-102).

### TOOL-61 Skills in general (P2)

As CC (`spec/cc`): skills are found in `.claude/skills`, `.agents/skills`
and `.gnosh/skills` in the project and in the user's own folders;
`option skill-path` adds places and `option disable-skill` hides one.
gnosh's own built-in skills are addressed as `gnosh://skills/<name>/...`. A
project skills folder that links to somewhere outside the project is left
out in an untrusted folder (SAFE-3). Skills the agent loaded are given again
after a summary (LOOP-63).

---

## 9. Web tools

### TOOL-70 Fetching (P1)

- A fetch asks first (it reaches a site of the model's choosing).
- **Content that is not text says what it is** (its type and size). An
  audio file, a video and a model file each came back as "not valid UTF-8"
  after being read to the size limit.
- A page longer than the read limit is cut on a character boundary. (A cut
  through a multi-byte character made a long page fail as "not UTF-8".)
- A 402 with x402 terms names them (TOOL-46).

### TOOL-71 The web-reading sub-agent (P1)

LOOP-80, SAFE-21. Its own fetches go to public sites only, and a page the
user approved cannot redirect it into the user's network.

### TOOL-72 What reaches out without asking (P1; OQ-23)

The earlier gnosh's README had to admit one: its code-search tool sent the
agent's search to a public code-search site **without asking**. In the new
gnosh nothing reaches a site the user did not choose without asking first
(PRIN-2), outside `-y`.

---

## 10. MCP tools (P2)

As CC cuts and saves them: an MCP tool's text is cut at the output limit; a
binary resource is saved to a file, and a long one is shown by its start; an
embedded resource or a resource link is described in words; an image is
saved and named, and a model that cannot see it gets the text and where the
file is; audio is saved to a file.

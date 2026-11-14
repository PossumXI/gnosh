# harness-trust-and-safety.md — the trust gate, reading outside the folder, untrusted text, what leaves the machine

*Since 14 November 2026 gnosh is configured by one JSON file, `gnosh.json`, and no longer by `gnoshrc` (`spec/DECISIONS.md`, items 17 and 23 to 28). Where this file speaks of `gnoshrc`, of its directives or of its dashed option names, it records what the build was first given.*

What this file is: the security behaviour Jae asked for and the holes that
were found and closed while building it. Four parts: a project's own settings
wait until the folder is trusted; the agent reads outside the folder only
after asking; text that came from a tool can never pose as the conversation;
and nothing leaves the machine unless the user turned that thing on.

In one paragraph: starting the earlier program in a repository you had just
cloned ran the stranger's code with your permissions, because a project's
config file was a script and its settings could add hooks, servers and
pre-approved tools. gnosh now asks first, in plain words that say what could
go wrong. Until the answer is yes, a project can only take things away from
the agent. Every bypass a reviewer found is listed here as a test case.

Read first: `harness-00-principles.md` (PRIN-1, PRIN-2),
`spec/cc/permissions.md`.

---

## 1. A cloned repository runs nothing until you trust it

### SAFE-1 The rule (P0 in its simplest form; P1 in full)

Jae approved the gate on 2026-11-01. As Claude Code's workspace trust, and
stricter (SAFE-2).

**P0:** the first binary does not read a project's own settings at all. Only
the user's own config applies. That is safe, and enough for the AI box,
where Jae's session starts in his home directory with no project settings.

**P1:** the gate below, after which a trusted project's settings apply.

Why it matters even though `gnoshrc` is no longer a script (CFG-11): a
project's settings can still add a hook (a command that runs), start a
server (a command that runs), let tools run without asking, point a
provider's address, with the user's API key, at someone else's machine, set
environment variables, and declare hosts to log in to.

### SAFE-2 What an untrusted folder's settings can do: only restrict (P1)

Applied from an untrusted folder: its deny rules, its list of tools to turn
off, its list of skills to turn off. These only take something away.

**Everything else waits**: its `gnoshrc`; hooks; MCP and LSP servers (their
commands and their options); allow rules and allowed tools; providers with
their keys and addresses; `env`; hosts; models; every other option.

The gate **fails closed**: every config file a load reads, other than the
user's own global ones, is held. That includes a settings file in the
project's data folder, even when that folder is a link pointing out of the
project.

This is stricter than Claude Code, which still runs an untrusted folder's
hooks and applies its `env`.

### SAFE-3 What is still used from an untrusted folder (P1)

Text for the model, none of which runs anything:

- its context files (`AGENTS.md` and the rest), whose imports are already
  confined to the project (CFG-22);
- the skills **inside** the folder. A project skills folder that is a link
  to somewhere outside the project is left out, because the agent reads a
  skill's files without asking;
- its custom commands;
- its sessions.

Open, recorded (OQ-15): language servers still start as the agent opens
files, and a few of them build the project to read it, which runs its code.
A session file planted in a repository is prompt injection the user can see
when he resumes it. Whether a planted `mem` deserves the same care is for
the other spec set.

### SAFE-4 The question (P1)

Jae, 2026-11-01: "does it explain what can go wrong? i think that's
important, how it can go wrong".

- Asked in an interactive session, **before** the project's settings are
  loaded, and **only where something would be held**. A folder with no
  project settings, or none that wait for trust, is never asked about.
- It asks whether the user trusts the files in this folder, names the
  folder, and lists what its settings would do: the config file that would
  be read, each hook, each server, the tools let run without asking, the
  providers, and so on.
- Under that, a marked section says **what could go wrong** if whoever
  wrote them meant harm. One plain sentence per kind of harm, and **only for
  the kinds this folder's settings could do**:

  | The folder's settings hold | The sentence must say |
  | :- | :- |
  | Hooks, local MCP or LSP servers, anything that runs a command | It would run as you: a stranger's could steal your SSH and wallet keys, wipe your files, or plant a program that stays. |
  | Tools allowed without asking | A line hidden in any file here could get the agent to run what these settings allow, without asking you. |
  | A provider | It would read all the agent sees, could take your API key, and could tell the agent what to do. |
  | An online MCP server | It would see what the agent sends and could steer it. |
  | Environment variables | For example, a changed search path that swaps a program for another. |
  | Hosts | Logging in to machines with your SSH keys. |
  | Skills linked out of the folder, or extra context and skill paths | Reading files outside the folder. |
  | An address for analytics, embeddings or the deliverables server | Sending your work off the computer. |
  | Any value that gnosh would expand by running something | Any command hidden in a setting runs as you. |

- **No is selected first.**
- **Yes** records the folder. **No** starts the session with the user's own
  settings only, and gnosh asks again next time. Declining does not exit
  (Jae's spec). Ctrl-C quits (exit 130).
- Names and commands from the stranger's files are shown escaped, never as
  terminal control sequences.
- The list of what is held is computed **by the same reader that would
  apply the settings**. (A first version read the file a second way: it took
  the first of two duplicate keys and matched names exactly, while the
  loader took the last and ignored letter case, so the question could show
  less than what would run.)

### SAFE-5 Without a person (P1)

`gnosh run` (and `-p`) never asks. In an untrusted folder it leaves the
project's settings out and says so in **one line on stderr**: that the
folder is not trusted, that its project settings are off, and the two ways
to use them (`gnosh trust <folder>`, or `--trust-folder` for one run).

### SAFE-6 The record (P1)

- Kept in the user's state directory (CFG-2), **never in the project**.
- Keyed on a git repository's **main checkout**, so its worktrees share it.
- A worktree shares a repository's trust only when the repository lists
  that worktree as its own. A folder cannot borrow a trusted repository's
  trust by naming it in a `.git` of its own.
- A repository whose own git settings place its work tree somewhere else is
  keyed on the folder gnosh was started in.
- Outside a repository, a trusted folder covers the folders under it that
  are not repositories of their own.
- The **home directory** is trusted for one session at a time and never
  recorded.

### SAFE-7 From the command line (P1)

- `gnosh trust [folder]` trusts a folder and prints what its settings now
  do, with the same "what could go wrong" section, and how to undo it.
- `gnosh trust --list` lists the trusted folders.
- `gnosh trust --forget [folder]` forgets one.
- `--trust-folder`, on `gnosh` and `gnosh run`, trusts the folder for that
  one session without recording it. For scripts.
- `gnosh doctor` says whether the folder is trusted and what is held.

### SAFE-8 The bypasses that were found (P1: each is a test)

Independent reviews of the gate found these. Each must have a test that
fails without its fix.

| # | The trick | The fix |
| :- | :- | :- |
| 1 | A downloaded repository's own git settings place its work tree elsewhere. That moved gnosh's idea of where the project ends, so the project's own settings file counted as "outside the project" and applied in full. | A git root counts as the project's only if it holds the folder gnosh was started in. |
| 2 | Any settings file in the project's data folder caused the defaults to be filled in again, which put the held skills folders back. | What is held is taken away **after** every merge. |
| 3 | A `.git` that is a link to a trusted repository's worktree record passed the "does the repository list me?" check, because the check followed the link. | The `.git` must be an ordinary file, and the repository's record must name this folder. |
| 4 | The question and the loader read the file differently (SAFE-4). | One reader. |
| 5 | A committed file that is a link to a key outside the project (`notes.txt` pointing at `~/.ssh/id_ed25519`) was read without asking, because "inside the working directory" was judged on the path as written. | SAFE-10. |
| 6 | The shell tool's read-only set ran `cat ~/.ssh/id_ed25519` with no question at all. | SAFE-11. |
| 7 | A repository handed over with its `.git` can name, in its own git settings, a program that git runs on a mere read. | SAFE-15. |
| 8 | A session store inside the project turned on MCP servers that the user's own config had turned off. | In an untrusted folder nothing a project's stored state turns on is started; the question lists what it would have. |
| 9 | Git takes any unambiguous start of a long option, and the read-only check refused only full names. A shortened option made a read-only git command run a program. | Options that run a program are refused under any start of their names, and in any group of short options. |
| 10 | A date command asked to read dates from a file prints each line it cannot parse, so it printed a key file. | Programs that can be made to read a file are held to the project like any reader. |

---

## 2. Reading outside the folder

Trusted or not, the agent reads outside the working directory only after
asking.

### SAFE-10 Links that lead out (P1)

- A path is judged by **where it really leads** (links resolved), not as
  written.
- A link in the folder that leads out of it is read, listed or searched
  only after asking, and **the question says where it leads**.
- A listing or a search of the folder **passes by** such links. (A search
  program skips links it meets while walking but follows one named on its
  command line, so a named link is checked.)
- Permission is recorded against the real location, so `--add-dir` and an
  earlier grant match where the read really lands.

### SAFE-11 Read-only commands read the project only (P1)

As in Claude Code. A read-only command (TOOL-30) runs without asking only
when what it reads lies in the project:

- Every argument, and every option's value, of a command that reads files is
  taken as a path that must lie in the project, from wherever in the line
  the command may be running (a `cd` moves the rest of an `&&` chain; after
  `;` or `||` it may not have).
- Patterns are expanded to check what they match.
- Anything that cannot be known ahead asks: a variable, a pattern in braces
  (the shell turns it into words of its own, which once slipped an option
  past a check), a recursive read that follows links.
- Files fed in with `<` are checked too.
- A command that an **allow rule** names runs as the rule says. A command
  that runs only because it is read-only is held to the project.

### SAFE-12 `acceptEdits` covers the project, nothing else (P1)

As Claude Code's. Edits run without asking only for paths inside the working
directory and directories added with `--add-dir`, after resolving links, and
never for protected files (TOOL-33). The earlier gnosh first implemented
this mode by putting the edit tools on the allowed list, which approved them
**for every path**: a line hidden in a file could then have the agent
rewrite a shell startup file or gnosh's own config with no question.

### SAFE-13 Protected files always ask

TOOL-33.

### SAFE-14 `--add-dir` approves reading and listing files, only (P1)

CLI-34. A fresh reviewer found it approving reads of MCP resources and a
listing of keys, because the first version approved "any request that reads
or lists, with a path under an added directory".

### SAFE-15 A repository's own git settings run nothing until trust (P1)

A repository handed over with its `.git` (a download or a copy, not a
clone) can name programs in its own git settings that git runs when it
merely reads the repository: a file-system monitor on every status, content
filters, diff drivers, a signature checker, a custom ssh or proxy command, a
credential helper, custom upload and receive programs, and a hook that fires
when the index is written (which a status does).

In an untrusted folder where any of these is set:

- the model is given **no git status**, and is told it was not read and why;
- the agent's own git commands **ask first**, and the question says why;
- the trust question lists it.

gnosh's own git calls never take optional locks. The user's own git config
is not the repository's, and is not looked at.

---

## 3. Text from a tool stays text

### SAFE-20 Control tokens are defused (P0)

Jae, 2026-10-20: fetch results had crashed his session with a tokenizing
error from the model server.

Web pages, command output and server replies can carry a model's own control
tokens. Two families were seen: the angle-bracket-and-bar tokens of chat
templates (`<|im_start|>` and the like), and llama.cpp's **media marker**,
which a vision-enabled server reads out of plain text. That marker is
randomized each time the server starts, and the server's own status page
prints it. Any tool result that echoed it made the server count an image
with no data and reject the whole request, and since every later request
carried the same result, the session was dead.

Requirement: before any message that is not gnosh's own instructions is
sent to the model, every such token in it is made harmless **without
changing how it reads**: by a means invisible to a person and to the model,
that the server no longer recognizes as special, that covers the fixed and
the randomized marker whatever its random part, and that is safe to apply
twice. It applies to every message on every request, not only to new tool
results.

A text that can rewrite the conversation from inside a tool result is also
an injection route, which is the reason this is a security rule and not
only a crash fix.

### SAFE-21 The web reader cannot read the user's files or network (P1)

The sub-agent that reads web pages (LOOP-80) fetches whatever pages send it
to, without asking, in a session that is approved. So:

- its file tools can open **only the pages it fetched** (its own temporary
  folder). A page cannot talk it into reading a key or a `.env` and sending
  it off in a URL;
- its own fetches and searches go to **public sites only**: not loopback,
  not a private, link-local or tailnet address, not a one-word host name.
  A page cannot send it to the model server, the router or a machine on the
  user's private network;
- a URL **the user approved** cannot redirect it into the network either.
  Redirects are followed only to public addresses.

### SAFE-22 A file or a tool result is never the user (P0)

Nothing read from a file, a web page, a command or a tool is treated as the
user's instruction or the user's approval. Only what the user typed, and
what the user's own hooks added, counts as coming from the user. `mem` has
the same rule for standing rules (`mem.md`).

### SAFE-23 Local pages gnosh serves (P2)

Any page gnosh serves on loopback (the session page, UI-40; the
deliverables server, TOOL-52) refuses a request whose `Host` is not a
loopback name, a request from another site's origin, a cross-site frame, and
a write with no origin. A page on another site whose name is pointed at
127.0.0.1 could otherwise read every session and rewrite the history the
model is sent next.

---

## 4. Nothing phones home

### SAFE-30 The rule (P0)

PRIN-2. With a fresh install and no settings, the only traffic is to the
model provider the user chose. Each of these is **off unless the user turns
it on**, and for each: **unset means off**.

| Thing | The earlier gnosh's upstream did it from the first start | In gnosh |
| :- | :- | :- |
| Usage analytics | Yes, to the vendor | Off. On only by the first-run question, `option metrics true`, or `GNOSH_DISABLE_METRICS=0`; and only to gnosh's own collector or one the user names. P2 (OQ-22). |
| A provider and model catalog download | Yes, from the vendor | Not built (OQ-3). |
| A check for a newer release | Yes (and of the vendor's program, so the fork announced an "update" on every start) | Off. On only when the user names a repository. P2. |
| An embedding server for `mem` | n/a | Off until `option mem-embeddings-url` names one; then `mem`'s text and the prompts typed go to that server alone. |

How it was proved in the earlier gnosh, and should be again: a stand-in
server recorded every request a fresh config made; and a capture of all
outbound connections during a run on the box showed nothing but the model
server.

### SAFE-31 What does leave the machine, said honestly (P0)

The README had this list, and the new one should too:

- The prompts, and the files the agent reads, go to the model provider the
  user chose. Point gnosh at a model on the user's own machine and nothing
  leaves.
- The fetch tools reach the sites the agent asks for, after asking the
  user.
- MCP servers the user adds reach whatever they reach.
- Under `-y`, the agent's shell reaches whatever it runs. With a model that
  refuses nothing and a page that can steer it, the limit is what the
  account it runs as can do. (On the box that is an unprivileged account
  with a memory cap. That is the deployment's part of the safety story, not
  gnosh's.)

### SAFE-32 The analytics question (P2; OQ-22)

Jae, 2026-10-23: "like how cookie settings work, default off, but you can
opt in ... just hitting enter means it does not dial home."

- Asked **once per machine**, on the first interactive run, before anything
  else takes the terminal: allow anonymous analytics, yes or no, where
  **Enter declines**.
- The answer is stored in the user's state directory and not asked again.
- Never asked when there is no terminal (a pipe, a run): then nothing is
  sent.
- Skipped when the answer is already forced by a setting or a variable.
- `DO_NOT_TRACK=1` beats everything.
- If allowed, what is sent is keyed to a hash of the machine's ID: start and
  exit and how long it ran; sessions created, switched, deleted; for each
  prompt and reply the session ID, provider, model, thinking level, whether
  permissions were skipped, and token counts; errors; the operating system,
  processor type, terminal, shell and gnosh's version. **Never prompts,
  replies or code.**
- It goes to gnosh's own collector or to an address the user names. Sending
  needs both the yes and an address.

### SAFE-33 What gnosh listens on (P1)

Nothing listens unless asked, and what listens does so on loopback only:

| Listener | When |
| :- | :- |
| The debugging endpoint (LOOP-70) | Only with `GNOSH_PROFILE` set. |
| The deliverables server (TOOL-52) | Only in an interactive session, and (recommended) only once a deliverable has been recorded. |
| The session page (UI-40) | Only while `gnosh session web` runs. |

On a machine with other accounts, loopback is shared with them; the
deliverables server's URLs carry a long random token for that reason, and
the debugging endpoint has none, which is why it is opt-in.

### SAFE-34 Tests stay off the network and off the user's files (P0, process)

- No test reaches a real service. (Loading the config once fetched the
  vendor catalog during tests.)
- Every test and every real-flow check runs with its own config and state
  directories. A check that set only the config directory once wrote a
  setting into Jae's real state file, and his own sessions then compacted
  differently.
- Test data never holds a real host, address, login or path of Jae's. The
  repository is public; a fixture carrying a server's login and address was
  once stopped only by a secret guard on the push.
- A test that starts processes has a guard against starting itself and a
  deadline, and is first run under a cap on processes and memory. A test
  that re-ran itself without end once put ten thousand processes on the AI
  box, and the machine killed Jae's session and the model server sixty
  times.

---

## 5. Modes, and what still holds when permissions are skipped

### SAFE-40 Under `-y` (P0)

Jae runs with every permission question skipped. These still hold:

- The trust gate (it is about what the folder's settings do, not about tool
  calls).
- Deny rules, and tools turned off.
- A hook's block, and a hook's `ask`, which still puts the call to the user
  (HOOK-11).
- The password prompt for signing and paying (TOOL-40). There is no way to
  skip it.
- A command on a declared host needs the declaration (TOOL-21).
- The web reader's confinement (SAFE-21).
- Control tokens are defused (SAFE-20).

### SAFE-41 The modes (P1)

`default`, `plan`, `acceptEdits`, `dontAsk`, `bypassPermissions` as in
`spec/cc/permissions.md`, with gnosh's limits in SAFE-12 and TOOL-33, and
`dontAsk` as in CLI-31. `auto` is not supported and says so. Hooks are told
the mode (HOOK-31).

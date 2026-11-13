# sessions.md — transcripts, resume, rewind, compaction, export

What this file is: how a conversation is saved, found again, wound back,
shrunk when it gets too long, and written out for a person to read — as
Claude Code does it, so `-c`, `-r`, `/rewind`, `/compact` and `/export`
behave the way Jae expects.

In one paragraph: every session has an ID and is appended, line by line, to
one file as it happens. `--continue` reopens the newest session of the
current directory; `--resume` reopens a particular one. Before each prompt
the harness notes the state of the files the model has edited, so `/rewind`
(or Esc twice) can put the conversation, the files, or both back to how they
were at any earlier prompt. When the conversation nears the model's limit it
is *compacted*: replaced by a summary, with the standing instructions loaded
afresh. `/export` writes the conversation as plain text.

Source: code.claude.com/docs/en — sessions, checkpointing, context-window,
how-claude-code-works, model-config, claude-directory, headless,
agent-sdk/session-storage, agent-sdk/typescript, sub-agents, hooks; docs
version 2.1.323, read 2026-11-13. One section (2.2) records the *shape* of a
transcript as observed in a file written by the installed 2.1.323 — key
names only, no content — because the docs deliberately do not specify it.
"(unverified)" = neither documented nor observed.

---

## 1. Identity

- A session ID is a UUID. `--session-id <uuid>` sets it for a new session
  (must be a valid UUID).
- A session belongs to a **project directory**: the working directory it was
  started in.
- Names: `-n/--name` at start, `/rename` later, or from the picker
  (Ctrl+R). An unnamed session gets a generated title — a short summary of
  the first prompt written by a small model in the background (not for `-p`
  runs) — and accepting a plan replaces it with a title based on the plan.
  Both a set name and a generated title work as handles for `--resume`.
- Interactive sessions also get a default display name
  `<directory>-<two characters>` (for lists of running sessions); it is not
  a resume handle.

## 2. Transcripts

### 2.1 Documented

- Location: `~/.claude/projects/<project>/<session-id>.jsonl`.
  `<project>` = the working directory path with every non-alphanumeric
  character replaced by `-` (`/Users/you/my-app` → `-Users-you-my-app`).
  Over 200 characters, the name is cut to 200 and a hash of the full path
  is appended. `CLAUDE_CONFIG_DIR` moves the root.
- **JSONL**: one JSON object per line, appended continuously as the session
  runs.
- Each line is one of: a message; a tool use or result; a record of a
  request sent to the model; a metadata entry.
- **The entry format is declared internal and unstable**: it changes
  between versions and scripts must not parse it. The supported ways to get
  at a conversation are `/export`, `-p --output-format json|stream-json`,
  `-p --resume <id>` with a question, and the `transcript_path` given to
  hooks and the status line.
- Entries have a `uuid` (some metadata entries have none) and a `sessionId`;
  messages form a linked chain; forking a session rewrites every `sessionId`
  and remaps the message UUIDs rather than copying bytes.
- Reading a session back gives "the chain the agent would see on resume":
  after a compaction, earlier turns are represented by the summary, though
  the raw earlier entries are still in the file.
- Sub-agent transcripts: `<session-id>/subagents/agent-<agentId>.jsonl`
  beside the main file, same format, with `agent-<agentId>.meta.json`.
- Large tool outputs are spilled to `<session-id>/tool-results/`.
- A compaction is recorded as an entry
  `{"type":"system","subtype":"compact_boundary","compactMetadata":{"trigger":"auto","preTokens":167189}}`.
- The transcript is written asynchronously and can lag the conversation by a
  moment (which is why `Stop` hooks get `last_assistant_message`).
- Two processes resuming the same session without forking interleave their
  messages into the one file.
- Not written at all with `--no-session-persistence` (print mode) or
  `CLAUDE_CODE_SKIP_PROMPT_HISTORY=1`.
- Deleted after `cleanupPeriodDays` (30) by a background sweep; `claude
  purge [path]` deletes a project's data at once.
- Also on disk: `~/.claude/history.jsonl` — every prompt typed, with
  timestamp and project path, used for Up-arrow recall and Ctrl+R search;
  not subject to the sweep.

### 2.2 Observed shape (undocumented, 2.1.323, key names only)

Recorded so gnosh's own format can be sensible, **not** to be reproduced
field for field — the docs say this layout changes on any release.

- Conversation entries — `type` is `user`, `assistant`, `system` or
  `attachment` — share these top-level keys: `uuid`, `parentUuid` (`null` on
  the first entry, otherwise the UUID of the entry it follows), `sessionId`,
  `timestamp`, `cwd`, `gitBranch`, `version`, `isSidechain` (boolean;
  `false` throughout a main transcript), `userType`, `type`.
- `user` and `assistant` entries carry `message` in the model-API shape:
  `role`, `content` (a string, or an array of blocks: `text`, `thinking`,
  `tool_use`, `tool_result`), and for assistant also `id`, `model`,
  `stop_reason`, `usage`.
- A `user` entry holding a tool result also has `toolUseResult` (the tool's
  structured output) and a reference to the assistant entry that made the
  call; entries may carry `isMeta` (injected, not typed by the user),
  `permissionMode`, `promptId`.
- `system` entries have `subtype` (seen: `turn_duration`, `informational`,
  `away_summary`), `content`, `level`.
- Metadata entries have their own `type` and no `uuid`/`parentUuid` chain:
  the session's title, the last prompt (with a `leafUuid` pointing at the
  chain's tip), the permission mode, file-history snapshots and deltas (the
  checkpoints of section 5), and records of model requests.

So: a transcript is an append-only log whose conversation entries form a
**tree by `parentUuid`**; the "current conversation" is the path from the
latest leaf back to the root. That is what makes rewind, branch and
"restore conversation" cheap: they add a new branch, they do not delete.
`isSidechain`, going by its name, marks entries that belong to a sub-agent's
side conversation rather than the main chain (unverified: only `false` was
observed, in a main transcript).

### 2.3 gnosh's transcript — recommendation

Define gnosh's own format and version it; do not chase Claude Code's.
Keep the ideas that the public behaviour depends on:

```jsonc
// one object per line, append-only
{"type":"user","uuid":"…","parentUuid":null,"sessionId":"…","timestamp":"2026-11-13T08:00:00Z",
 "cwd":"/work/app","message":{"role":"user","content":"…"}}
{"type":"assistant","uuid":"…","parentUuid":"…","sessionId":"…","timestamp":"…",
 "message":{"id":"…","role":"assistant","model":"…","content":[…],"stop_reason":"tool_use","usage":{…}}}
{"type":"user","uuid":"…","parentUuid":"…","sessionId":"…","timestamp":"…",
 "message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"…","content":"…"}]},
 "toolUseResult":{…}}
{"type":"system","subtype":"compact_boundary","uuid":"…","parentUuid":"…","sessionId":"…",
 "compactMetadata":{"trigger":"auto","preTokens":61234}}
```

- `message` in the same block shape as stream-json output
  (`output-formats.md`), so one encoder serves both.
- `uuid` / `parentUuid` chaining; metadata lines (title, mode, checkpoint
  records) without a place in the chain.
- Sub-agents in `subagents/agent-<id>.jsonl`.
- A `format` version on the first line.
- Location: gnosh's own directory, in the same
  `projects/<project>/<session-id>.jsonl` layout so hooks' `transcript_path`
  has the familiar look. gnosh does **not** write into `~/.claude/projects`
  and does not promise to resume Claude Code transcripts.

## 3. Resume and continue

| Command | Meaning |
| :- | :- |
| `claude -c` / `--continue` | Reopen the most recent session of the current directory. Skips sessions made by `-p` and sessions whose first prompt was `/loop`; `-p -c` includes them. Includes sessions that added this directory with `/add-dir`. |
| `claude -r` / `--resume` | Open the session picker. |
| `claude -r <id>` | Resume that session from any directory: searched in the current project and its worktrees, then every other project (resolved only if exactly one other project has it). Not found: `No conversation found with session ID: <id>`. |
| `claude -r <name>` | Resume by name or generated title within the repository; an ambiguous name opens the picker pre-filled. |
| `claude -r /abs/path/x.jsonl` | Resume the session in that transcript file. |
| `--fork-session` | With `-c`/`-r`: continue in a copy with a new session ID; the original is untouched. |
| `/resume [name\|id]` (`/continue`) | Switch sessions from inside one. |
| `/branch [name]` | Copy the conversation so far into a new session and switch to it. Session-scoped permission grants and in-flight background work carry over (same process). |

What a resumed session restores:

- The conversation: messages, tool calls, results. After a compaction it
  opens at the compaction point: the summary, the messages the compaction
  kept, and everything since.
- A tool call that was running when the old process died is **not** rerun.
- The model it was using; the agent it ran as (`--agent`).
- An active `/goal` (counters reset); unexpired `/loop` tasks.
- Background shell commands and sub-agents that died with the old process
  appear as notes saying they did not finish; they are read with the next
  prompt and do not start a turn by themselves.
- Checkpoints: `/rewind` still works after a resume.
- `SessionStart` hooks run again with `source: "resume"` (or `"fork"`).
  Context that earlier hooks injected is replayed from the transcript, not
  regenerated.
- The system prompt recorded on the first request is reused
  (`--system-prompt-snapshot`, `cli.md`).

**Not** restored — pass again: `--mcp-config`, `--settings`,
`--fallback-model`, `--add-dir` directories (and `/add-dir` ones). Settings
files are simply re-read.

Permission mode on resume:

| How | Mode |
| :- | :- |
| Terminal `-c`, `-r <id>`, `-r <unique name>` | The mode the session was in — except `bypassPermissions`, which is never restored (it must be enabled at launch again). `--permission-mode` overrides. |
| `-p -c` / `-p -r` | What a new `-p` run would start in (plan mode is restored only if a prompt handler is present to approve the plan). |
| The picker, or `/resume` inside a session | The mode a new session would start in / the current session's mode; a session that ended in plan mode comes back in plan mode. |

Interrupted turns: a turn cut off by SIGTERM stays as it is on resume and the
next prompt drives on; `CLAUDE_CODE_RESUME_INTERRUPTED_TURN=1` continues it
automatically.

### The session picker

Shows sessions of the current worktree (plus those that added this directory),
each with its name or title, time since last activity, git branch and file
size. Keys: Up/Down or `k`/`j` to move; Enter to resume; `1`–`9` to resume
by position; Space to preview; Ctrl+R to rename; typing to search; Ctrl+A
all projects on the machine; Ctrl+W all worktrees of the repository; Ctrl+B
only the current git branch; Esc to leave. Sessions made by `/branch` or
`--fork-session` are grouped under their origin (Right/Left to expand).
Picking a session from an unrelated project copies a `cd` + resume command
to the clipboard instead of resuming in place.

## 4. `/clear`

Starts a new conversation with empty context **in a new session**; the old
one is saved and resumable. With no argument the new session keeps a
user-set name (not a generated title); with an argument, the *old* session
is given that name. Within the same process the rewind menu offers a
"previous session" entry to go back. Fires `SessionEnd` (`clear`) and
`SessionStart` (`clear`); in stream-json emits `conversation_reset`.

## 5. Checkpoints and `/rewind`

- **A checkpoint is taken at every user prompt that starts a turn**, before
  the turn runs. It records the state of the files the model's file tools
  have edited.
- Snapshots are kept for the **100 most recent checkpoints** per session,
  stored in `~/.claude/file-history/<session-id>/`, saved with the
  conversation (so they survive a resume) and deleted by the retention
  sweep.
- `fileCheckpointingEnabled: false` or
  `CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING=1` turns file snapshots off.

`/rewind` (aliases `/checkpoint`, `/undo`), or **Esc twice on an empty
input**, opens a menu listing the prompts sent in the session. (With text in
the input, double Esc clears the text instead, saving it to history.) Choose
a prompt, then:

| Choice | Effect |
| :- | :- |
| Restore code and conversation | Both go back to that point. |
| Restore conversation | The conversation goes back; files stay as they are. |
| Restore code | Files go back; the conversation stays. |
| Summarize from here | Compress the conversation from this point on into a summary. |
| Summarize up to here | Compress everything before this point; later messages stay. |
| Never mind | Back to the list. |

- The two code options appear only if that checkpoint has file changes to
  revert.
- After *restore conversation* or *summarize from here*, the selected
  prompt's text is put back into the input box, ready to edit and re-send.
- A summarize option takes optional guiding text typed on its row.
  Summarizing changes no files, and the original messages stay in the
  transcript.
- A rewound conversation is saved together with the next prompt; exiting
  before sending one can bring the rewound messages back on resume.

What is and is not tracked:

- Tracked: edits by the file tools (Write, Edit, NotebookEdit). Once a file
  has been edited that way, later changes to **that file** by shell
  commands are rewound too.
- Not tracked: files only ever changed by shell commands; edits by
  sub-agents (except a foreground forked skill); edits by other sessions or
  outside programs, unless to a file this session already tracks.
- A message typed while the model was working has no checkpoint of its own
  (shown as "no code restore"); rewind to the prompt that started the turn.
- Symlinked and hard-linked files are skipped, with a warning
  `Restored the code, but skipped N files`.
- Not a replacement for version control.

## 6. Compaction

Compaction replaces the conversation history with a summary so the session
can continue.

- **Manual**: `/compact [instructions]`. The instructions steer what the
  summary keeps (`/compact focus on the auth bug fix`). A section titled
  "Compact Instructions" in a memory file does the same standingly.
- **Automatic**: when the conversation reaches the auto-compact window.
  Default window = the model's context limit (models with a one-million
  window compact a little before it fills). For a model ID Claude Code does
  not recognise it assumes a 200K window (`CLAUDE_CODE_MAX_CONTEXT_TOKENS`
  corrects it). Controls: `autoCompactEnabled`, `autoCompactWindow`
  (100,000–1,000,000 tokens, capped at the model's window), `--autocompact`,
  `/autocompact`, `CLAUDE_CODE_AUTO_COMPACT_WINDOW`,
  `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` (1–100, can only make it earlier),
  `DISABLE_AUTO_COMPACT=1`, and `DISABLE_COMPACT=1` (no compaction at all,
  manual included).
- **Thrash guard**: if one file or tool output is so large that the context
  refills right after each summary, auto-compaction stops after a few
  attempts and shows an error instead of looping.
- **Partial**: the two "summarize" choices of `/rewind`.
- **Idle** (Claude Code only): a conversation left idle is compacted before
  its prompt cache expires. Not adopted.

Hooks: `PreCompact` (`trigger`: `manual` | `auto`; may block; its stdout is
added to the summary instructions), then `PostCompact` (gets
`compact_summary`), then `SessionStart` with `source: "compact"`.
stream-json: `system/status` `compacting`, then `system/compact_boundary`
with `compact_metadata: { trigger, pre_tokens }`.

What is there after a compaction:

| Thing | After |
| :- | :- |
| System prompt, output style | Still apply. |
| Project-root memory file and unscoped rules | Re-read from disk and re-injected. |
| Nested memory files, path-scoped rules | Gone until their trigger happens again. |
| The summary | Keeps the user's requests and key code; fine detail from early on may be lost. |
| Files the model read or edited | Up to five are re-read, most recently modified first; one over 5,000 tokens comes back as a path reference only. |
| Invoked skills | The latest invocation of each, first 5,000 tokens, 25,000 tokens in total, newest first. |
| Skill listing | Rebuilt. |
| Git status snapshot | Taken afresh, if the session started with one. |
| The plan written in plan mode | Re-injected. |
| Background commands and sub-agents | Keep running; the model is reminded which are running. |
| Context added earlier by hooks | Summarised with the rest. |
| `SessionStart` hooks matching `compact` | Run; their output joins the new context. |
| The recorded system prompt | Rebuilt (this is when changed `--system-prompt` flags take effect). |

The raw pre-compaction entries remain in the transcript file; on resume
only the summary and what followed are loaded.

gnosh's own context management (frames and `mem`) decides *how* it frees
context; what must match is the user-facing surface: `/compact
[instructions]`, automatic compaction with the settings above, the two
hooks, the `compact` session start, and the re-injection of standing
instructions.

## 7. `/export`

- `/export <filename>` writes the conversation to that file as **plain
  text**, "messages and tool outputs rendered as readable text".
- `/export` alone opens a small dialog: copy to the clipboard, or save to a
  file.
- It is the supported way to hand a conversation to a person; scripts use
  the JSON interfaces.
- Not documented (unverified): how a relative filename is resolved, whether
  an extension is added, the suggested default name, and exactly how tool
  calls and thinking are rendered. gnosh: resolve relative to the working
  directory, add `.txt` when there is no extension, suggest
  `<date>-<session-name>.txt`, render tool calls as one summary line each
  with their output indented, omit thinking.

---

## What gnosh adopts

**P0**

- [ ] Session ID = UUID; `--session-id`; one append-only JSONL transcript
      per session in a `projects/<project>/<session-id>.jsonl` layout with
      the documented `<project>` encoding.
- [ ] `-c` (with the `-p`/interactive difference) and `-r <id | path>`.
- [ ] Resume restores conversation and model; does not rerun interrupted
      tool calls.
- [ ] `transcript_path` for hooks.
- [ ] Automatic compaction with `DISABLE_AUTO_COMPACT`, re-injection of
      root memory files, and the thrash guard.

**P1**

- [ ] `/clear` as a new session; `/compact [instructions]`.
- [ ] The picker; resume by name; `--fork-session`; `/branch`; generated
      titles; `/rename`; `-n`.
- [ ] Checkpoints at each prompt, the 100-checkpoint store, `/rewind` with
      its five actions and Esc-Esc; prompt text restored to the input.
- [ ] `PreCompact`/`PostCompact`, `SessionStart(compact)`,
      `compact_boundary` in transcript and stream.
- [ ] `/export`; `history.jsonl`; retention sweep; `--no-session-persistence`.
- [ ] Permission-mode restoration rules, including never restoring
      `bypassPermissions`.
- [ ] Sub-agent transcripts under `subagents/`.

**P2**

- [ ] `autoCompactWindow` family of settings; partial summarize; resume
      across projects; `purge`; `CLAUDE_CODE_RESUME_INTERRUPTED_TURN`.

## Deliberately not adopted

- Byte-compatibility with Claude Code's transcript entries (declared
  internal by its own docs) and writing into `~/.claude/projects`.
- Background-session attach on resume (`claude attach`), `--from-pr`,
  PR-linked sessions.
- The "Resume this conversation?" usage-limit dialog.
- Idle compaction and prompt-cache-driven behaviour.
- Session storage adapters (the SDK `SessionStore`), cloud and desktop
  session lists, `CLAUDE_CODE_PROJECT_DIR_NAME`.

## Unverified (docs silent)

- The transcript entry format beyond what 2.1 lists (2.2 is observation,
  key names only).
- What exactly the compaction summary contains and how it is prompted
  (gnosh writes its own).
- How many attempts the thrash guard allows.
- `/export` details listed in section 7.
- Whether `/clear` fires `SessionEnd` before `SessionStart` (assumed).

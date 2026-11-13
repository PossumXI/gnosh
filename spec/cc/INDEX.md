# spec/cc — what gnosh takes from Claude Code

Jae's rule for gnosh: "in terms of naming conventions and such i want it to
be more like claude code whereever reasonable, because i'm familiar with
claude code the most." These files say exactly which public conventions
gnosh copies — flags, tool names and arguments, permission rules, hooks,
settings, memory files, commands — so that what he knows keeps working.

They were written from Claude Code's **public documentation**
(code.claude.com/docs/en, version 2.1.323, changelog entry 2026-11-12, read
2026-11-13) and from `claude --help` of the installed 2.1.323. Nothing here
comes from Claude Code's source code, system prompts or tool-description
prompts: every statement was taken from the public docs or checked against
them, and gnosh writes its own prompts, wording and look. Each file ends
with what gnosh adopts (P0 / P1 / P2), what it deliberately does not, and
what the docs left unsaid ("unverified").

Provenance notes, for the clean-room record:

- Some of Jae's earlier research notes on Claude Code turned out to have
  been read out of the program binary rather than the docs (hook events
  2.1.310, `/rename`, `/export`, `/rewind`, `/goal`, background
  notifications, and one paragraph of the tool-names note). They were set
  aside. Nothing in these files rests on them; where they overlap, the
  statement here comes from the public docs.
- `sessions.md` §2.2 lists the *key names* seen in one transcript file
  written by the installed program, because the docs declare that format
  internal. It is marked as observation, and gnosh is told not to copy it.
- No Crush or earlier-gnosh source was opened.

Message texts quoted in these files (error lines, notices) are there so the
behaviour can be recognised. gnosh words its own messages; what it copies
exactly is names: flags, tools, fields, values, file names.

- **P0** — first usable binary: print mode, core tools, permissions.
- **P1** — interactive daily use.
- **P2** — later.

## The files

| File | What it covers |
| :- | :- |
| `cli.md` | Flags and subcommands with exact spellings, argument-parsing rules, stdin, exit codes, signals. |
| `output-formats.md` | What `-p` prints: `text`, the `json` result object, every `stream-json` line type; the stdin stream format. |
| `tools.md` | Each built-in tool: name, input fields, result shape, limits, read-before-edit, background shells, old names. |
| `permissions.md` | Modes, rule syntax, Bash and path matching, precedence, protected paths, dangerous removals, workspace trust, the prompt. |
| `hooks.md` | All 33 hook events, matchers, stdin JSON, exit codes, JSON output per event, timeouts, the stop-hook guard. |
| `settings.md` | Settings files and precedence, the keys that matter, environment variables, disk layout — and how to combine them with `gnoshrc`. |
| `memory-files.md` | `CLAUDE.md` / `AGENTS.md` discovery and order, nested files, rules, `@` imports, delivery to the model. |
| `slash-commands-and-skills.md` | Built-in commands, `/loop`, `/goal`, skills and custom commands: layout, frontmatter, substitutions, shell injection, the listing. |
| `subagents.md` | Agent definition files, built-in types, what a sub-agent sees and returns, background agents, limits. |
| `sessions.md` | Transcripts, resume and continue, checkpoints and rewind, compaction, export. |
| `interactive.md` | The interactive session as behaviour: input box, keys, permission prompt, status-line JSON, indicators. |
| `mcp.md` | MCP configuration shape, scopes, tool naming, limits. |

## P0, in build order

1. **Command line** (`cli.md` §1–2, §4): own argument parser — short flags,
   `--flag=value`, variadic and optional-value flags, camelCase and
   kebab-case tool flags, paren-aware list splitting; `-p`; prompt from
   argument and/or stdin (10 MB cap); implicit print mode when stdout is not
   a terminal; `-v`, `-h`; "did you mean"; invalid flag → stderr, non-zero.
2. **Session and transcript** (`sessions.md` §1–2): UUID session ID,
   `--session-id`, append-only JSONL under
   `projects/<project>/<session-id>.jsonl`.
3. **Loop and model** (`cli.md` §3.3, §3.5): `--model`; `--system-prompt`,
   `--append-system-prompt` and their `-file` forms (file text first);
   `--max-turns`.
4. **Output** (`output-formats.md`): `text`; `json` = the result object;
   `stream-json` = `system/init`, one `assistant` line per content block,
   `user` tool-result lines with `tool_use_result`, final `result`;
   `--verbose`. Exit `0` / `1` / `143`.
5. **Core tools** (`tools.md` §1–8): `Read`, `Write`, `Edit`, `Bash`,
   `Grep`, `Glob`, with their limits; read-before-edit state; absolute
   paths; parallel read-only calls.
6. **Permissions** (`permissions.md`): the decision order; modes
   `default`, `acceptEdits`, `plan`, `dontAsk`, `bypassPermissions`; rule
   syntax and Bash / path matching; read-only command set; protected paths;
   dangerous-removal check; `--permission-mode`,
   `--dangerously-skip-permissions`, `--allowedTools`, `--disallowedTools`,
   `--tools`, `--add-dir`; denial in `-p` with `permission_denials`.
7. **Settings** (`settings.md`): user / project / local files and
   `--settings`, precedence, list merging; keys `permissions`, `hooks`,
   `disableAllHooks`, `env`; broken files skipped in `-p`; `gnosh doctor`.
8. **Memory files** (`memory-files.md` §1–2, §4–6): user file, project
   chain from root to working directory, `CLAUDE.local.md`, `AGENTS.md`
   fallback, `@` imports to depth four, delivery as context.
9. **Hooks** (`hooks.md` §2–5, §6.1–6.3, 6.5, 6.6, 6.8, 6.13): command
   hooks; matcher rules; stdin JSON; exit 0 / 2 / other; JSON output;
   events `SessionStart`, `UserPromptSubmit`, `PreToolUse`, `PostToolUse`,
   `PostToolUseFailure`, `Stop`, `SessionEnd`; fail closed on blocking
   events not yet built.
10. **Continue and resume** (`sessions.md` §3): `-c`, `-r <id | path>`.
11. **Compaction** (`sessions.md` §6): automatic, with re-injection of root
    memory files and a thrash guard.
12. **Signals** (`cli.md` §4.4): SIGINT ends the turn; SIGTERM kills running
    commands, runs `SessionEnd`, exits 143.

First things after P0: the `Agent` tool (foreground), then the interactive
session with `/clear`, `/compact`, `/model`, `/help`, `/exit`.

## Claude Code name → meaning, for everything in P0

### Flags

| Name | Meaning |
| :- | :- |
| `-p`, `--print` | Answer one prompt non-interactively and exit. |
| `--output-format text\|json\|stream-json` | Plain answer / one result object / one JSON event per line. |
| `--input-format text\|stream-json` | Prompt as text (P0) / JSON user messages per line (P1). |
| `--verbose` | Full turn-by-turn output; always passed with `stream-json`. |
| `-c`, `--continue` | Continue the newest conversation of this directory. |
| `-r`, `--resume [id]` | Resume a session by ID (or transcript path). |
| `--session-id <uuid>` | Use this ID for the new session. |
| `--model <name>` | Model for this session. |
| `--permission-mode <mode>` | Starting permission mode. |
| `--dangerously-skip-permissions` | Same as `--permission-mode bypassPermissions`. |
| `--allowedTools`, `--allowed-tools` | Allow rules: matching calls run without asking. |
| `--disallowedTools`, `--disallowed-tools` | Deny rules; a bare tool name removes the tool. |
| `--tools` | Which built-in tools exist at all (`""`, `default`, or names). |
| `--add-dir <dirs...>` | Extra directories the file tools may use. |
| `--system-prompt[-file]` | Replace the system prompt. |
| `--append-system-prompt[-file]` | Add to the system prompt. |
| `--max-turns <n>` | Cap on model turns in print mode. |
| `--settings <file-or-json>` | Extra settings for this session, above the settings files. |
| `-v`, `--version` / `-h`, `--help` | Version / help. |
| `doctor` | Read-only diagnostics of installation and settings. |

### Output

| Name | Meaning |
| :- | :- |
| `type: "result"` | The final object of a run (the whole of `json` output). |
| `subtype` | `success`, `error_max_turns`, `error_during_execution`. |
| `result` | The final answer text (success only). |
| `is_error`, `errors` | Whether the run ended in an error / the error texts. |
| `session_id` | ID to pass to `--resume`. |
| `num_turns`, `duration_ms`, `duration_api_ms` | Turns taken, wall time, time in model requests. |
| `total_cost_usd`, `usage`, `modelUsage` | Cost estimate, main-loop token usage, per-model totals. |
| `permission_denials` | Tool calls refused: `{ tool_name, tool_use_id, tool_input }`. |
| `type: "system"`, `subtype: "init"` | First stream line: `cwd`, `model`, `tools`, `permissionMode`, ... |
| `type: "assistant"` / `"user"` | One model content block / tool results fed back. |
| `parent_tool_use_id` | `null` in the main conversation; a sub-agent's owning call otherwise. |
| `tool_use_result` | A tool's structured result, beside the text the model saw. |

### Tools

| Name | Meaning | Input fields |
| :- | :- | :- |
| `Read` | Read a file, numbered lines. | `file_path`, `offset`, `limit` |
| `Write` | Create or overwrite a file. | `file_path`, `content` |
| `Edit` | Replace one exact string. | `file_path`, `old_string`, `new_string`, `replace_all` |
| `Bash` | Run a shell command. | `command`, `timeout`, `description` |
| `Grep` | Search file contents. | `pattern`, `path`, `glob`, `type`, `output_mode`, `-i`, `-n`, `-o`, `-A`, `-B`, `-C`, `context`, `head_limit`, `offset`, `multiline` |
| `Glob` | Find files by name. | `pattern`, `path` |
| `BASH_DEFAULT_TIMEOUT_MS` / `BASH_MAX_TIMEOUT_MS` | Bash timeout default (120,000) / ceiling (600,000). | |
| `BASH_MAX_OUTPUT_LENGTH` | Bash output read-back window (30,000). | |

### Permissions

| Name | Meaning |
| :- | :- |
| `default` (`manual`) | Ask before edits, commands, network. |
| `acceptEdits` | Also run file edits and a few filesystem commands inside the working directories. |
| `plan` | Look, do not change. |
| `dontAsk` | Deny whatever would have asked. |
| `bypassPermissions` | Ask about almost nothing. |
| `Tool` / `Tool(specifier)` | A rule: the whole tool / matching uses. |
| `Bash(git diff *)`, `Bash(ls:*)` | Command prefix rules. |
| `Read(path)`, `Edit(path)` | Path rules; anchors `//abs`, `~/home`, `/settings-relative`, `./cwd-relative`. |
| `permissions.allow` / `ask` / `deny` | Rule lists; checked deny, then ask, then allow. |
| `permissions.defaultMode` | Starting mode. |
| `permissions.additionalDirectories` | Extra working directories. |

### Settings and memory

| Name | Meaning |
| :- | :- |
| `~/.claude/settings.json` | User settings. |
| `.claude/settings.json` | Project settings, shared. |
| `.claude/settings.local.json` | Project settings, private; where saved approvals go. |
| `hooks`, `disableAllHooks`, `env` | Hook configuration, the off switch, environment variables. |
| `CLAUDE.md`, `.claude/CLAUDE.md` | Project instructions. |
| `CLAUDE.local.md` | Private project instructions. |
| `~/.claude/CLAUDE.md` | Instructions for every project. |
| `AGENTS.md` | Read when the project has no `CLAUDE.md`. |
| `@path` | Import another file into a memory file. |

### Hooks

| Name | Meaning |
| :- | :- |
| `SessionStart` / `SessionEnd` | A session begins or resumes / ends. |
| `UserPromptSubmit` | A prompt was submitted; may block it or add context. |
| `PreToolUse` | Before a tool call; may allow, deny, ask, or rewrite the input. |
| `PostToolUse` / `PostToolUseFailure` | After a tool call succeeded / failed. |
| `Stop` | The model finished; may make it continue. |
| `matcher` | Which tool (or reason) a hook group applies to. |
| `session_id`, `transcript_path`, `cwd`, `hook_event_name`, `permission_mode` | Fields every hook receives on stdin. |
| `tool_name`, `tool_input`, `tool_response`, `tool_use_id` | Tool-event fields. |
| exit `0` / `2` / other | Fine / block / warning that does not block. |
| `permissionDecision`, `updatedInput`, `additionalContext` | `PreToolUse` answers. |
| `decision: "block"`, `reason` | Block (prompt, stop) with an explanation. |
| `continue`, `stopReason`, `systemMessage` | Stop the model / why / a message for the user. |
| `stop_hook_active` | True when a stop hook already forced a continuation: the loop guard. |
| `CLAUDE_PROJECT_DIR` | The project root, for hook commands. |

## Open decisions for Jae

1. Read `~/.claude` and `.claude` settings as a fallback layer under gnosh's
   own, and write saved approvals to gnosh's own local file
   (`settings.md`).
2. Be stricter than Claude Code about untrusted folders in `-p`: no project
   hooks, `env` or skill grants unless the folder is trusted
   (`permissions.md` §11).
3. Keep `Glob` and `Grep` in the default tool set, unlike Claude Code on
   macOS and Linux (`tools.md` §2).
4. Keep the stop-hook continuation cap (8, resets on any tool call) as a
   loop guard, configurable, `0` = off (`hooks.md`).
5. `init` line: emit `gnosh_version` rather than `claude_code_version`, and
   list the sub-agent tool as `Agent` (`output-formats.md`).
6. Whether subprocesses get `CLAUDECODE=1` in addition to gnosh's own
   marker (`tools.md` §6.1).
7. Default commit attribution off (`settings.md`).

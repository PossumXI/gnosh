# subagents.md — agent definitions and how the main agent uses them

What this file is: how gnosh defines and runs sub-agents the way Claude Code
does, so the agent files Jae already has in `~/.claude/agents/` and
`.claude/agents/` load unchanged and behave the same.

In one paragraph: a sub-agent is a second model run with its own, empty
context window, its own instructions and its own (usually narrower) set of
tools. The main agent starts one by calling the `Agent` tool with a task;
the sub-agent works alone and hands back one final report. Nothing else it
did — the files it read, the commands it ran — enters the main
conversation, which is the point. A sub-agent *type* is a Markdown file: a
few settings at the top (name, description, tools, model) and the
sub-agent's system prompt below. Sub-agents can run in the background, in
which case their report arrives later as a notification.

Source: code.claude.com/docs/en — sub-agents, tools-reference,
agent-sdk/typescript (Agent input/output, task messages), hooks,
permissions, settings-reference; docs version 2.1.323, read 2026-11-13. Own
words; names and field names are literal. "(unverified)" = docs silent.

---

## 1. Where definitions come from

Highest priority first. When two definitions share a name the higher one
wins.

| Priority | Source | Scope |
| :- | :- | :- |
| 1 | Managed settings directory | Organization (not adopted) |
| 2 | `--agents '<json>'` | This session only |
| 3 | `.claude/agents/` | The project. Every `.claude/agents/` from the working directory up to the repository root is scanned; for the same name the one closest to the working directory wins. |
| 4 | `~/.claude/agents/` | All projects |
| 5 | A plugin's `agents/` | (not adopted) |

- Both agent directories are scanned **recursively**; a sub-folder is only
  for organising, the identity is the `name` field.
- A directory added with `--add-dir` or `/add-dir` contributes its
  `.claude/agents/` too.
- The two standard directories are watched: a new or edited file is picked
  up within seconds and used by the next delegation. Exceptions needing a
  restart: an agents directory that did not exist at session start, and
  agents inside added directories.
- Two files with the same name in one tree: only one loads, in filesystem
  order (not a defined precedence). `/doctor` reports the clash.

Files silently skipped (reason in the debug log): no `name`; the opening
`---` is not the first line; a `name` that starts with `-`, contains `:` or
exceeds 256 characters; a `name` without a `description`; YAML that does not
parse.

## 2. The definition file

```markdown
---
name: code-reviewer
description: Reviews code for quality and best practices
tools: Read, Glob, Grep
model: inherit
---

You are a code reviewer. When invoked, analyse the code and give specific,
actionable feedback on quality, security and best practices.
```

The frontmatter configures the agent; **the body is its entire system
prompt**. A sub-agent gets that prompt plus basic environment details (the
working directory and such) — not the main agent's system prompt.

Multi-word field names are camelCase and must match exactly; an unknown
field is ignored without an error (the debug log names up to five of them
and the likely intended spelling).

| Field | Req | Meaning |
| :- | :- | :- |
| `name` | yes | Unique identifier, at most 256 characters, no `:`. Hooks see it as `agent_type`. The file name need not match. |
| `description` | yes | When the main agent should delegate to this agent. This text is what the main agent matches a task against. |
| `tools` | no | Tools the agent may use: a comma-separated string (`Read, Grep, Bash`) or a YAML list. Omitted = every tool available to sub-agents. If nothing in the list resolves to a real tool, the launch fails with an error naming the entries. |
| `disallowedTools` | no | Tools to remove. Same format. An entry with a specifier (`Bash(git push *)`) still removes the **whole** tool. |
| `model` | no | A model name, or `inherit` (the main conversation's model). Claude Code also takes the aliases `sonnet`, `opus`, `haiku`, `fable`. |
| `permissionMode` | no | `default`, `acceptEdits`, `plan`, `dontAsk`, `bypassPermissions` (`auto`; `manual` = `default`). Section 6. |
| `maxTurns` | no | Maximum agentic turns. At the limit the agent's output is returned marked as partial, and it can be resumed. |
| `skills` | no | Skills whose **full text** is injected into the agent's context at start (up to 32 names). Skills with `disable-model-invocation` cannot be preloaded. The agent can still invoke other skills through the `Skill` tool. |
| `hooks` | no | Hooks active only while this agent runs. A `Stop` hook here is treated as `SubagentStop`. For a project agent they need the folder trusted. |
| `mcpServers` | no | MCP servers for this agent: names of configured servers, or inline definitions. |
| `background` | no | `true`: always run in the background even when the caller asked to wait. |
| `effort` | no | `low` … `max`, for this agent. |
| `isolation` | no | `worktree`: run in a temporary git worktree branched from the default branch; removed automatically if the agent changed nothing. |
| `omitClaudeMd` | no | `true`: start without the user, project and local memory files. Ignored when the agent runs as the main session agent. |
| `autoCompactWindow` | no | Compact this agent earlier: a token count, 100,000–1,000,000, written as a plain integer. |
| `memory` | no | Persistent memory directory for this agent: `user` (`~/.claude/agent-memory/<name>/`), `project` (`.claude/agent-memory/<name>/`), `local` (`.claude/agent-memory-local/<name>/`). |
| `color` | no | Display colour: `red`, `blue`, `green`, `yellow`, `purple`, `orange`, `pink`, `cyan`. |
| `initialPrompt` | no | When the agent runs as the *main session* agent: submitted as the first user turn, before any prompt the user gave. |
| `experimental` | no | Map; `cacheTtl: 5m \| 1h` (Anthropic prompt caching). |

### `--agents` JSON

```json
{
  "code-reviewer": {
    "description": "Expert code reviewer. Use proactively after code changes.",
    "prompt": "You are a senior code reviewer...",
    "tools": ["Read", "Grep", "Glob", "Bash"],
    "model": "inherit"
  }
}
```

Each top-level key is an agent name (must not start with `-`). `prompt` is
the system prompt (the file body's equivalent; may be empty). The other
keys are the frontmatter fields: `description`, `tools`, `disallowedTools`,
`model`, `permissionMode`, `mcpServers`, `hooks`, `maxTurns`,
`autoCompactWindow`, `skills`, `initialPrompt`, `memory`, `effort`,
`background`, `omitClaudeMd`, `isolation`. `color` and `experimental` are
ignored. With `-p` the flag's value may instead be a path to a JSON file.
The value is validated at startup; an invalid one exits with code 1.

## 3. Built-in agent types

| Type | Tools | Starts with memory files and git status | Purpose |
| :- | :- | :- | :- |
| `general-purpose` | Everything available to sub-agents | yes | Multi-step tasks that need both exploring and changing. The default when `subagent_type` is omitted. |
| `Explore` | Read-only (no Write, no Edit) | **no** | Fast file discovery and code search. The caller states a thoroughness: quick, medium, or very thorough. One-shot: cannot be resumed. |
| `Plan` | Read-only | **no** | Research during plan mode, so exploration output stays out of the (read-only) main conversation. One-shot. |

- A user or project agent with the same name **replaces** a built-in (for
  example an `Explore` with its own `model`).
- `CLAUDE_CODE_DISABLE_EXPLORE_PLAN_AGENTS=1` removes `Explore` and `Plan`.
- A deny rule `Agent(Explore)` switches one type off; a bare `Agent` deny
  removes delegation altogether.
- An `Agent` call without `subagent_type` fails with
  `subagent_type is required` when no `general-purpose` agent exists.
- Claude Code's other built-ins (`claude`, `statusline-setup`,
  `claude-code-guide`) are not adopted.

gnosh writes its own prompts for these three; only the names, tool sets and
start-up context are copied.

## 4. Calling a sub-agent

The main agent calls the `Agent` tool (old name `Task`): `description`,
`prompt`, optional `subagent_type`, `model`, `effort`, `run_in_background`,
`name`, `isolation` — shapes in `tools.md`.

Other ways a sub-agent starts:

- The user names one in plain language (the model decides), or **@-mentions**
  it in the prompt, which guarantees it runs for that task.
- `--agent <name>` / the `agent` setting: the **whole session** runs as that
  agent — its prompt replaces the default system prompt (as
  `--system-prompt` would), its tool restrictions and model apply, memory
  files still load, and the choice is restored on resume.
- A skill with `context: fork`.

How the main agent chooses: from the task, the agents' `description` fields
and the context. Descriptions cost context — Claude Code warns at startup
when custom agents' descriptions together pass 15,000 tokens.

Launching never asks for permission; the sub-agent's own tool calls do.

## 5. What a sub-agent starts with

A fresh context window. It does **not** see the conversation, the skills
already invoked, or files already read. It gets:

1. **System prompt**: the agent's own prompt plus environment details.
2. **Task message**: the `prompt` the caller wrote. This is all it knows
   about the task.
3. **Memory files**: every level the main conversation loads (user,
   project, local, rules, `AGENTS.md`) — except for `Explore`, `Plan`, and
   agents with `omitClaudeMd`. Nested and path-scoped files still load on
   demand as it touches files.
4. **Git status snapshot** taken when it starts — except `Explore` and
   `Plan`.
5. **Preloaded skills** named in `skills`.

Never passed down: the output style, the main conversation's auto memory,
the parent's context-window size (a sub-agent's window is its own model's).

Working directory: the main conversation's current directory. A `cd` inside
a sub-agent does not persist between its own shell calls and never affects
the main conversation.

### Model

In order: the per-call `model` parameter; the definition's `model`
(`inherit` = the main model); the `CLAUDE_CODE_SUBAGENT_MODEL` environment
variable; the main conversation's model.
`CLAUDE_CODE_SUBAGENT_MODEL_FORCE=1` makes the variable override the first
two. Sub-agents inherit the main conversation's thinking on/off setting.

### Tools

Start from the main conversation's built-in and MCP tools, then:

1. `disallowedTools` is applied, then `tools` is resolved against what is
   left. A tool in both is removed.
2. Removed from **every** sub-agent, even if listed: `Agent` at the depth
   limit, `AskUserQuestion`, `EnterPlanMode`, `ExitPlanMode` (unless its
   `permissionMode` is `plan`), `ScheduleWakeup`.
3. A **background** sub-agent keeps every MCP tool but only these built-ins:
   `Read`, `Grep`, `Glob`, `Bash`, `Edit`, `Write`, `NotebookEdit`,
   `WebFetch`, `WebSearch`, `Skill`, `ToolSearch`, `Monitor`, `TaskStop`,
   `EnterWorktree`, `ExitWorktree` (plus Anthropic-specific ones). So one
   definition can resolve to different tools in foreground and background.
4. `tools: Agent(worker, researcher)` restricts which types may be spawned —
   but only for an agent running as the main session (`--agent`); inside an
   ordinary sub-agent definition the list in parentheses is ignored.
   Omitting `Agent` from `tools` means it cannot spawn at all.

Hooks from settings files run inside sub-agents too, with `agent_id` and
`agent_type` in the input.

## 6. Permissions of a sub-agent

Unset `permissionMode` = inherit the main conversation's mode. When set:

| Main conversation is in | Sub-agent runs in |
| :- | :- |
| `bypassPermissions` or `acceptEdits` | The same mode; the definition's value is ignored. |
| `plan` | `plan`, whatever was set. With `dontAsk` set, it also never prompts: what plan mode would ask about is denied. |
| `default` or `dontAsk` | The definition's value — except `bypassPermissions`, which a sub-agent gets only when the main conversation has it. |

The main conversation's permission rules apply to the sub-agent's calls.
`permissions.disableBypassPermissionsMode` makes a definition's
`bypassPermissions` ignored.

Prompts:

- **Foreground** sub-agent: its permission prompts appear exactly as the
  main conversation's would, at the moment of each call.
- **Background** sub-agent: the prompt appears in the main session and names
  the sub-agent; Esc denies that one call without stopping it. An answer
  that lasts beyond one call (a session grant) applies to the whole session.
- Where nothing can prompt (a background sub-agent in a `-p` run),
  `PermissionRequest` hooks still run and, absent a decision, the call is
  denied.

No message from one agent to another ever counts as the user's approval, and
no agent message can change a sub-agent's permission settings, memory files
or configuration.

## 7. Foreground and background

- **Foreground**: the caller's `Agent` tool call blocks until the sub-agent
  finishes; the tool result is the report.
- **Background**: the tool call returns at once with
  `status: "async_launched"`, an `agentId` and an `outputFile`; the
  sub-agent keeps working; its report reaches the main agent later as a
  **completion notification**, marked as an automated event rather than a
  message from the user. The main agent is expected to wait for that
  notification before reporting results, and to say "still running" if asked
  earlier.

Which one happens when the caller leaves `run_in_background` out:

| Caller | Default |
| :- | :- |
| The main agent | Background (interactive terminal since v2.1.232, everywhere since v2.1.302). |
| A sub-agent | It **waits** for the agent it spawned. |
| `-p` runs | The model may wait, on its own judgment or when asked. |

`background: true` in a definition forces background.
`CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1` makes every sub-agent run in the
foreground. A waiting foreground sub-agent can be pushed to the background
with Ctrl+B. In Claude Code's interactive mode the main agent cannot choose
to wait unless "fork mode" is turned off (`CLAUDE_CODE_FORK_SUBAGENT=0`) —
gnosh does not copy that coupling: its main agent may pass
`run_in_background: false` to wait.

Background specifics:

- A background sub-agent may leave its own background shell command running
  past its turn; when that ends the sub-agent is notified.
- In a `-p` run the process stays open until background sub-agents finish.
- In stream-json: `system/task_started` (with `task_type: "local_agent"`),
  `task_progress`, `task_notification`; the sub-agent's own messages carry
  `parent_tool_use_id` and `agent_id` (`output-formats.md`).
- The user lists and stops them with `/tasks`; a finished one stays listed
  as done for 30 seconds.

Limits:

- **Depth**: sub-agents may spawn their own, up to **three layers** below
  the main conversation (`CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH`; `1` turns
  nesting off). At the limit the `Agent` tool is withheld.
- **Concurrency**: at **20** running sub-agents the next spawn fails with
  `Concurrent subagent limit reached` and the model is told not to retry
  (`CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS`). No limit on the total over a
  session.
- `--max-budget-usd`: once the cap is reached spawning fails with
  `Budget limit reached` and running background sub-agents are stopped.

## 8. What comes back

- The final report: the sub-agent's last text. Structured form
  (`tool_use_result` / `PostToolUse` `tool_response`): `status`, `agentId`,
  `content`, `totalToolUseCount`, `totalDurationMs`, `totalTokens`, `usage`
  — the last two describe only the sub-agent's **final** model request.
- A sub-agent stopped by `maxTurns` returns its output marked partial.
- **Errors**: a foreground sub-agent cut off by a model error after it
  already produced text returns that partial text with a note that it did
  not finish; one that produced nothing fails with
  `Agent terminated early due to an API error` plus the detail. A background
  one is marked failed and the notification names the error and includes its
  last output.
- **Report scanning**: before the main agent reads a report, text that
  imitates harness output (a `<system-reminder>` tag, a line beginning
  `Human:` or `Assistant:`) gets a backslash inserted so it reads as plain
  text, and a marker line is put in front when the report imitates such tags
  or mentions permission-bypass settings. Nothing is removed. This is a
  speed bump against instructions smuggled in from files or web pages the
  sub-agent read, not a filter.

## 9. Resuming a sub-agent

- Each `Agent` call creates a **new** instance. A finished sub-agent's ID is
  returned so it can be continued; `Explore` and `Plan` return none.
- In Claude Code the main agent resumes one by sending it a message with the
  `SendMessage` tool, addressed by agent ID or name; the sub-agent then runs
  again **in the background** under the same ID with its full history (tool
  calls, results, reasoning) and the tool set of its first run.
- A sub-agent the *user* stopped does not auto-resume; a message to it is
  refused.
- Transcripts: `~/.claude/projects/<project>/<session-id>/subagents/agent-<agentId>.jsonl`,
  with an `agent-<agentId>.meta.json` beside each. They are untouched by a
  compaction of the main conversation, survive a restart of the same
  session, and are deleted by the retention sweep.
- Sub-agents compact their own context automatically; the transcript records
  it as a `system` entry with `subtype: "compact_boundary"` and
  `compactMetadata: { trigger, preTokens }`.

## 10. Forks

A *fork* is a sub-agent that inherits the whole conversation — same system
prompt, tools, model and history — instead of starting fresh. Its tool calls
still stay out of the main conversation and only its result returns. The
user starts one with `/subtask <task>`; the model may start one itself while
fork mode is on. A fork cannot spawn further forks. `Agent(fork)` in deny
stops them.

gnosh has its own design for parallel sessions sharing one `mem`; forks are
recorded here for vocabulary, not as a requirement.

---

## What gnosh adopts

**P0** — none. Sub-agents come right after the first usable binary.

**P1**

- [ ] First: the `Agent` tool (alias `Task`), foreground, returning the
      final report and the `completed` result shape.
- [ ] Built-in types `general-purpose` and `Explore` with the tool sets and
      start-up context of section 3 (gnosh's own prompts).
- [ ] Fresh context: own system prompt, task message, memory files, git
      status; nothing from the conversation.
- [ ] Tool filtering of section 5; no `AskUserQuestion` in sub-agents.
- [ ] The parent's permission rules and mode apply to the sub-agent.

- [ ] Definition files in `.claude/agents/` and `~/.claude/agents/`
      (recursive, watched), `--agents` JSON, the skip rules.
- [ ] Fields `name`, `description`, `tools`, `disallowedTools`, `model`
      (`inherit`), `permissionMode`, `maxTurns`, `skills`, `hooks`,
      `background`, `omitClaudeMd`, `color`.
- [ ] Background sub-agents, completion notification, `/tasks`, `TaskStop`,
      permission prompts surfaced in the main session.
- [ ] Depth limit 3 and concurrency limit 20, with their variables.
- [ ] `SubagentStart` / `SubagentStop` hooks; `agent_id` / `agent_type` in
      hook input; sub-agent transcripts under `subagents/`.
- [ ] `Plan` type; `--agent` / `agent` setting; @-mention of an agent.
- [ ] Report scanning.

**P2**

- [ ] Resume by ID, `isolation: worktree`, `mcpServers`, `effort`,
      `autoCompactWindow`, `initialPrompt`, `memory` scopes,
      `CLAUDE_CODE_SUBAGENT_MODEL[_FORCE]`, `Agent(type, ...)` allowlists,
      forks and `/subtask`.

## Deliberately not adopted

- Managed and plugin agent sources; plugin-scoped names (`plugin:agent`).
- Agent teams (teammates, `team_name`, team messages), cross-session
  messaging, `ListAgents`.
- `SubagentHandback` and classifier review of sub-agent reports.
- `isolation: "remote"`, `experimental.cacheTtl`, model aliases
  `sonnet`/`opus`/`haiku`/`fable`.
- The built-in agents `claude`, `statusline-setup`, `claude-code-guide`.
- The rule that an interactive main agent cannot wait for a sub-agent
  unless fork mode is off.
- (`SendMessage` as the resume mechanism was on this list; gnosh has
  adopted it since: spec/DECISIONS.md 48.)

## Unverified (docs silent)

- The text of the completion notification and of the delegation framing.
- Exactly which environment details are appended to a sub-agent's system
  prompt.
- How an @-mention of an agent is spelled when a file of the same name
  exists.
- Whether a sub-agent's `maxTurns` counts turns the same way `--max-turns`
  does (assumed yes).

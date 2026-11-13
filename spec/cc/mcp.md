# mcp.md — MCP client: configuration, scopes, tool names

What this file is: how gnosh connects to MCP servers (outside programs that
offer extra tools) using Claude Code's configuration shape and naming, so a
`.mcp.json` that works there works here. All of it is P1 or P2 — nothing
here is needed for the first usable binary.

In one paragraph: an MCP server is listed in a small JSON file under a name.
It is either a local program gnosh starts and talks to over stdin/stdout, or
a URL gnosh connects to. Each tool the server offers appears to the model
under the name `mcp__<server>__<tool>`, and that same name is what
permission rules and hook matchers use. Servers can be listed for one
project (shared in `.mcp.json`), privately for one project, or for the user
everywhere.

Source: code.claude.com/docs/en — mcp, cli-reference, headless, permissions,
settings-reference, env-vars, agent-sdk/typescript; `claude mcp --help`;
docs version 2.1.323, read 2026-11-13. Own words; names and field names are
literal. "(unverified)" = docs silent.

---

## 1. Configuration shape

The same object is used in `.mcp.json`, in `--mcp-config` files or inline
strings, and by `claude mcp add-json`:

```json
{
  "mcpServers": {
    "local-tool": {
      "command": "npx",
      "args": ["-y", "some-mcp-server"],
      "env": { "API_KEY": "${API_KEY}" }
    },
    "remote-tool": {
      "type": "http",
      "url": "${API_BASE_URL:-https://api.example.com}/mcp",
      "headers": { "Authorization": "Bearer ${API_KEY}" }
    }
  }
}
```

| Field | Applies to | Meaning |
| :- | :- | :- |
| `type` | all | `stdio` (the default when absent), `http` (alias `streamable-http`), `sse` (deprecated), `ws`. **An entry with a `url` but no `type` is an error** — it is skipped and reported as `MCP server "<name>" has a "url" but no "type"; add "type": "http" (or "sse" / "ws") to this entry`. `"type": "sdk"` entries are skipped (only an embedding program can register those). |
| `command` | stdio | Executable to start. |
| `args` | stdio | Argument list. |
| `env` | stdio | Extra environment variables for the server process. |
| `url` | http, sse, ws | Endpoint. |
| `headers` | http, sse, ws | Static request headers. |
| `headersHelper` | http, sse, ws | A command that prints headers at connect time. Not run from an untrusted project. (P2) |
| `timeout` | all | Hard wall-clock limit per tool call, milliseconds; values below 1000 are ignored. |
| `alwaysLoad` | all | Load this server's tools up front rather than deferring them to tool search. |

### Environment expansion

In `command`, `args`, `env`, `url` and `headers`:

- `${VAR}` → the variable's value; `${VAR:-default}` → the default when
  `VAR` is **unset** (a variable set to the empty string counts as set).
- An unset variable with no default leaves the literal `${VAR}` in place and
  produces a warning in `claude mcp list`.
- In a **remote** server's `url` and `headers`, certain credential variables
  always read as empty — the harness's own model credentials, cloud-provider
  credentials, and things like `HTTPS_PROXY` or `NPM_TOKEN` — so a
  repository's `.mcp.json` cannot send them to a server it names. gnosh
  applies the same idea to its own provider keys.
- A stdio server's process gets `CLAUDE_PROJECT_DIR` (the project root) in
  its environment. Because that variable is set for the server and not for
  the harness, a reference to it in `command`/`args` needs a default:
  `${CLAUDE_PROJECT_DIR:-.}`.

## 2. Scopes

| Scope | Loaded in | Shared | Stored in (Claude Code) |
| :- | :- | :- | :- |
| `local` (default for `mcp add`) | The current project only | No | `~/.claude.json`, under `projects["<path>"].mcpServers` |
| `project` | The current project | Yes, committed | `.mcp.json` at the project root |
| `user` | Every project | No | `~/.claude.json`, top-level `mcpServers` |
| Command line | This session | — | `--mcp-config <file-or-json>...`; `--strict-mcp-config` uses only these |

Note the name clash: MCP "local scope" lives in the home directory, while
"local settings" means `.claude/settings.local.json`.

Precedence when the same **name** is defined twice: local, then project,
then user. The whole entry from the winning scope is used; fields are not
merged.

Project servers need approval:

- Interactive: the first time, each `.mcp.json` server is asked about before
  it is connected; the answers are remembered
  (`claude mcp reset-project-choices` forgets them). Settings
  `enableAllProjectMcpServers`, `enabledMcpjsonServers`,
  `disabledMcpjsonServers` pre-answer; a repository's own copy of those
  approvals does not count until the folder is trusted.
- `-p`: no prompt can be shown, so project servers are connected **without
  asking**. `disabledMcpjsonServers`, `--setting-sources user`, or
  `--strict-mcp-config` keep them out.

**Recommendation for gnosh**: `.mcp.json` in the project root, read as-is.
For user and local scope gnosh keeps its own file (the same `mcpServers`
shape) instead of writing `~/.claude.json`; optionally, and only on Jae's
say-so, it *reads* `mcpServers` from `~/.claude.json` so servers already
added there appear in gnosh too. In `-p`, gnosh follows its stricter trust
rule (`permissions.md`): project servers connect only in a trusted folder.

## 3. Managing servers from the shell

```text
claude mcp add [options] <name> <commandOrUrl> [args...]
    -s, --scope <local|user|project>      default "local"
    -t, --transport <stdio|sse|http>      default stdio
    -e, --env <KEY=value...>
    -H, --header <header...>
claude mcp add-json [options] <name> '<json>'
claude mcp get <name>
claude mcp list
claude mcp remove [options] <name>
claude mcp reset-project-choices
claude mcp serve                           (run the harness itself as an MCP server)
```

- For a stdio server, `--` separates the harness's options from the server
  command: `claude mcp add --env KEY=value --transport stdio myserver -- python server.py --port 8080`.
- `--env` takes several pairs, so another option must come between `--env`
  and the server name.
- `ws` cannot be given to `--transport`; use `add-json`.
- `list` and `get` health-check approved servers and show unapproved
  project servers as pending.
- Inside a session: `/mcp` lists servers with status and offers reconnect,
  enable and disable.

## 4. Tools

- **Name**: `mcp__<server>__<tool>`. Characters in the server name outside
  letters, digits, `_` and `-` become `_` (a server configured as
  `my.server` yields `mcp__my_server__...`).
- The name is used verbatim in permission rules (`mcp__github`,
  `mcp__github__*`, `mcp__github__get_issue`; allow rules take no
  parenthesised specifier), in hook matchers (regular expressions such as
  `mcp__github__.*`), and in `--allowedTools` / `--disallowedTools`
  (`"mcp__*"` denies every MCP tool). `--tools` does not affect MCP tools.
- Hook input for an MCP tool call carries
  `mcp_server: { "name": ..., "source": ... }`.
- An MCP tool call needs approval in `default` mode unless an allow rule
  covers it (unverified: whether a read-only annotation alone skips the
  prompt; gnosh: it does not). A tool whose annotations mark it read-only
  may run in parallel with other read-only calls.
- Tool names longer than 128 characters and tools with invalid input
  schemas get special handling described on the mcp page (not transcribed
  here; P2).

### Limits

| Limit | Value |
| :- | :- |
| Server startup | `MCP_TIMEOUT`, 30,000 ms. With `--mcp-config` in `-p`, the first turn waits up to this long for pending servers. |
| One tool call | Per-server `timeout`, else `MCP_TOOL_TIMEOUT` (default effectively unlimited, about 28 hours). |
| Idle (no response and no progress) | 5 minutes for remote servers, 30 minutes for stdio; `CLAUDE_CODE_MCP_TOOL_IDLE_TIMEOUT` (ms; `0` disables). |
| Result size | `MAX_MCP_OUTPUT_TOKENS`, 25,000 tokens; a warning from 10,000. A successful text result over the limit, or over 50,000 characters, is saved to a file in the session's `tool-results` directory and replaced by a message naming the path. |
| Error text | Over about 11,000 characters: first 5,000 and last 5,000 kept, with a marker between. |
| One response from a remote server | 16 MiB. |
| Tool description shown to the model | 4,096 characters when loaded up front; 16,384 when loaded through tool search. |

A main-conversation MCP call that runs longer than two minutes is moved to
a background task and its result arrives as a task notification (P2).

### Stopping a stdio server

On disconnect: SIGINT, then SIGTERM 100 ms later if still running, then
SIGKILL after a further 400 ms.

### Start-up and failures

- By default servers connect in the background and their tools appear as
  they finish (`MCP_CONNECTION_NONBLOCKING=0` waits instead).
- stream-json `system/init` reports `mcp_servers: [{ name, status }]` with
  status `connected`, `failed`, `needs-auth`, `pending` or `disabled`, and
  `mcp_server_errors` for skipped `--mcp-config` entries
  (`unknown_type`, `url_missing_type`, `invalid_config`, `reserved_name`).
- Remote servers reconnect automatically after a drop.
- A server may change its tool list during a session
  (`tools/list_changed`); the tool set is refreshed.
- `roots/list` is answered with the launch directory plus the additional
  working directories, and `roots/list_changed` is sent when they change.

## 5. Beyond tools (P2)

- **Tool search**: instead of putting every MCP tool definition in the
  context, only names and server instructions load at start and the model
  fetches definitions on demand through the `ToolSearch` tool.
  `ENABLE_TOOL_SEARCH`: unset/`true` = defer; `auto` / `auto:N` = defer only
  when the definitions exceed 10% (or N%) of the context window; `false` =
  load everything up front. For a small-context local model, deferring
  matters more, not less.
- **Resources**: `@server:protocol://path` in a prompt attaches a resource;
  `@` completion lists resources beside files; tools
  `ListMcpResourcesTool` (`server?`) and `ReadMcpResourceTool`
  (`server`, `uri`).
- **Prompts as commands**: a server's prompts appear as
  `/mcp__<server>__<prompt>` (listed as `/server:prompt (MCP)`); arguments
  are split on whitespace; the result is injected into the conversation.
- **Elicitation**: a server may ask the user for input mid-call; hooks
  `Elicitation` / `ElicitationResult` can answer. With no one to answer the
  request is cancelled.
- **OAuth** for remote servers (`claude mcp login <name>`, `--client-id`,
  `--callback-port`), and `headersHelper`.
- **Sub-agent-scoped servers**: `mcpServers` in an agent definition.
- **Serving**: `claude mcp serve` exposes the harness's own tools to other
  MCP clients.

---

## What gnosh adopts

**P0** — nothing.

**P1**

- [ ] `.mcp.json` and `--mcp-config` in the `mcpServers` shape; stdio and
      `http` transports; `${VAR}` / `${VAR:-default}` expansion;
      `CLAUDE_PROJECT_DIR` for stdio servers.
- [ ] Tool naming `mcp__<server>__<tool>` with the character rule, used
      identically in rules, matchers and flags.
- [ ] Approval of project servers; `--strict-mcp-config`;
      `disabledMcpjsonServers`.
- [ ] `MCP_TIMEOUT`, per-server `timeout`, result-size limit with spill to
      a file.
- [ ] `mcp add | add-json | get | list | remove`, `/mcp`.
- [ ] `mcp_servers` in `system/init`; `mcp_server` in hook input.
- [ ] stdio shutdown sequence.

**P2**

- [ ] `sse` and `ws` transports; OAuth; `headersHelper`; reconnection.
- [ ] Tool search and `ToolSearch`; resources and `@server:` mentions;
      prompts as commands; elicitation; `alwaysLoad`.
- [ ] Automatic backgrounding of long calls; idle timeout.
- [ ] `mcp serve`; sub-agent `mcpServers`; `roots/list`.

## Deliberately not adopted

- claude.ai connectors (`mcp__claude_ai_*`), their organisation controls,
  `allowAllClaudeAiMcps`, `disableClaudeAiConnectors`.
- Managed MCP (`managed-mcp.json`, `managedMcpServers`,
  `allowManagedMcpServersOnly`).
- Plugin-provided servers and the `mcp__plugin_<plugin>_<server>__` form.
- `claude mcp add-from-claude-desktop`, channels, MCP Apps UI resources,
  `"type": "sdk"` in-process servers, `anthropic/*` tool metadata keys
  (gnosh may define its own equivalents).
- Writing `~/.claude.json`.

## Unverified (docs silent)

- The MCP protocol revision and client capabilities Claude Code declares.
- The exact approval dialog flow for `.mcp.json` servers.
- How two servers whose names normalise to the same string are told apart.
- Whether `env` values are merged over or replace the inherited environment
  for a stdio server (gnosh: merged over).

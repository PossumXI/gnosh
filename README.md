# gnosh

An AI harness that thinks in Gno.

gnosh sits between a person, a language model and the machine. The model
reads, edits and runs things under the person's rules, and it keeps its plan
in `mem`, in Gno, on disk. Each step of a task is a frame: when the step is
done and popped, its work leaves what the model is sent and its result stays,
the way a function's locals go when it returns. A new session starts from
what is still open.

This is gnosh written from scratch in Go. It is not a fork of anything.

## Where it stands

It runs. Built from this tree, it has answered prompts, used tools, asked
permission, planned and finished a task in `mem`, and popped frames against
a real model (Qwen 3.8 27B on llama.cpp). `tools/dev/remote-ci.sh` builds,
vets and tests every package from a clean copy.

Built:

- **Print mode**, `gnosh -p`: text, JSON and streamed JSON output, streamed
  JSON input for a conversation over standard input, `--json-schema` for a
  result as JSON that conforms to a schema, `--bare` for minimal mode,
  `--no-session-persistence` to keep nothing, `--permission-prompts none`
  for a run nobody is there to answer, `--agents` for kinds of sub-agent of
  the run's own, and `--agent` to run the session as one. It waits for the
  background commands its turns start and lets the model answer how they
  ended.
- **A session** in the terminal's own scrollback.
- **Tools:** the file and shell tools, WebFetch (a page read by the small
  model with the model's request, from the open web only), commands on
  declared hosts over ssh, and MCP servers, those gnosh starts itself and
  those at a URL (from `~/.config/gnosh/mcp.json`, a trusted project's
  `.mcp.json`, and `--mcp-config` for one run, in Claude Code's shape; their
  tools are `mcp__<server>__<tool>`).
- **Permissions:** modes, plan mode included (the model presents its plan
  with ExitPlanMode and you approve it), and rules.
- **Hooks:** command hooks in the shell and exec forms, in the background
  too, http hooks and mcp_tool hooks, with Claude Code's `if` filters (one
  permission rule each) and their status messages.
- **What the model is given:** instruction files, skills (with
  allowed-tools and disallowed-tools, effort, `context: fork` in a
  sub-agent, paths, hooks of their own, and commands run before the model
  sees them), and a status line command (Claude Code's `statusLine`).
- **Sub-agents** in the foreground and, in a session, in the background:
  general-purpose, Explore, Plan and your own, given more to do once they
  end with SendMessage.
- **`mem`** and frames, with the close check, `/goal`, the slide, peers'
  turn locks, and `gnosh mem`.
- **Commands:** `gnosh doctor`, `gnosh trust`, `gnosh mcp` (add, add-json,
  get, list and remove, with Claude Code's local, project and user scopes),
  `gnosh ssh` (ssh that puts what the far side copies on this machine's
  clipboard: see below), and `--worktree` (a session in a git work tree of
  its own, as Claude Code's).
- **Deliverables:** the `Deliver` tool, by which the model names what was
  asked for, `/deliverables`, and each delivered file served at an address
  on this machine.

In a session, Claude Code's commands work as there: `/help`, `/clear`,
`/compact`, `/resume`, `/branch`, `/rewind` (the conversation, the files Write
and Edit changed, or both), `/model`, `/effort`, `/config`, `/plan`, `/permissions`,
`/add-dir`, `/status`, `/context`, `/btw` (a side question, also while the
model works; nothing of it is kept), `/recap`, `/cost`, `/tasks`, `/diff`,
`/export`,
`/copy`, `/init`, `/memory`, `/agents`, `/mcp` (with reconnect, enable and
disable), `/skills`, `/reload-skills`, `/hooks`, `/statusline` and `/doctor`, with gnosh's
own `/mem`, `/goal`, `/loop` and `/deliverables` beside them. Shift+Tab goes through the
permission modes, Esc stops a turn and Esc twice goes back to an earlier
prompt, Ctrl+O shows the model's reasoning, Ctrl+B moves a running command
to the background, `@path` in a prompt brings that file (or a directory's
listing) along with it, and a line that begins with `!` is a command of your own.
`/panel` (or the `sidePanel` option) draws the session on the whole screen
beside a side panel of the files it changed and its deliverables, with their
addresses; `/panel` again gives the screen back, with all that was said in
the scrollback.

Not built yet: signing in to an MCP server by OAuth (a key goes in its
`headers`), and a WebSearch tool of gnosh's own (a search engine's MCP
server gives the model one). The interface is in use over
ssh, in tmux. Its wording to the model has not yet been compared run
against run. Nothing here is released.

## Licence

The GNO Network General Public License, version 6, with Additional Terms
(Strong Attribution): see [LICENSE.md](LICENSE.md). The GnoVM subsystem has
its own copy in `internal/gnovm/LICENSE.md`. A user interface built on gnosh
shows "built on gnosh" with a link to <https://gnosh.ai>.

## Build

    go build -o gnosh .

Go 1.27 or newer. The harness uses the standard library and the Go team's
`x/term` and `x/sys`; the GnoVM part uses gno and a pure-Go SQLite.

## A model

In `~/.config/gnosh/gnosh.json`, for a llama.cpp server on this machine:

    {
      // a model server on this machine
      "providers": {
        "local": { "type": "llamacpp", "baseUrl": "http://127.0.0.1:8080" }
      }
    }

The file is JSON in which comments and a comma after the last item are
allowed. It holds `permissions`, `hooks`, `env` and `model` with Claude
Code's names and meaning, and gnosh's own `providers`, `models`,
`smallModel`, `hosts` and `options` beside them; a project may keep a
`gnosh.json` of its own, which counts once the folder is trusted. The
package documentation of `internal/config` lists every key.

Then `gnosh doctor` says whether a turn can run, and `gnosh` starts a
session. Nothing leaves the machine except what is sent to the model server
you name.

## Copying over ssh

A program on the far side of ssh copies to your clipboard through the
terminal (OSC 52): gnosh's `/copy` does, and so does tmux's copy mode.
Apple's Terminal drops such copies. `gnosh ssh` runs ssh and puts them on
this machine's clipboard itself. In zsh, this sends every interactive ssh
typed in Apple's Terminal through it, and leaves scripts' ssh alone:

```sh
if [[ $TERM_PROGRAM == Apple_Terminal ]]; then
  ssh() {
    if [[ -t 0 && -t 1 ]] && (( $+commands[gnosh] )); then
      gnosh ssh "$@"
    else
      command ssh "$@"
    fi
  }
fi
```

ssh keeps your terminal: it reads what you type, asks for passwords, and
stops with Ctrl+Z or `~^Z` as it does alone, and its exit status is
gnosh's. Only what it writes passes through gnosh. The clipboard is only
ever written, never read, but anything shown on the far side can write it,
as in terminals that take OSC 52 themselves.

## Reading on

- [ARCHITECTURE.md](ARCHITECTURE.md): how it is put together.
- [PROVENANCE.md](PROVENANCE.md): how it was written and what it is built from.
- [CARRIED.md](CARRIED.md): the files brought over from the earlier gnosh.
- [AUTHORSHIP.md](AUTHORSHIP.md): who asked for what.
- [GNOMEM.md](GNOMEM.md): `mem`, from the model's side.
- `spec/`: what it does, in words.

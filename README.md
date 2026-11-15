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

Built: print mode (`gnosh -p`, with text, JSON and streamed JSON output), an
interactive session in the terminal's own scrollback, the file and shell
tools, WebFetch (a page read by the small model with the model's request,
from the open web only), MCP servers, those gnosh starts itself and those
at a URL (from `~/.config/gnosh/mcp.json`, and a trusted project's
`.mcp.json`, in Claude Code's shape; their tools are
`mcp__<server>__<tool>`), commands on
declared hosts over ssh, permission modes and rules,
hooks, instruction files, sub-agents in the foreground, `mem` and frames
with the close check, `/goal`, the slide and peers' turn locks, `gnosh mem`,
`gnosh doctor`, `gnosh trust`, `gnosh mcp` (add, add-json, get, list and
remove, with Claude Code's local, project and user scopes), and
deliverables: the `Deliver` tool by which
the model names what was asked for, `/deliverables`, and each delivered file
served at an address on this machine.

In a session, Claude Code's commands work as there: `/help`, `/clear`,
`/compact`, `/resume`, `/rewind` (the conversation, the files Write and Edit
changed, or both), `/model`, `/plan`, `/permissions`, `/add-dir`, `/status`,
`/context`, `/cost`, `/tasks`, `/diff`, `/export`, `/init`, `/memory`,
`/agents`, `/mcp` (with reconnect, enable and disable), `/skills`, `/hooks`
and `/doctor`, with gnosh's own `/mem`,
`/goal`, `/loop` and `/deliverables` beside them. Shift+Tab goes through the
permission modes, Esc stops a turn and Esc twice goes back to an earlier
prompt, Ctrl+O shows the model's reasoning, Ctrl+B moves a running command
to the background, and a line that begins with `!` is a command of your own.

Not built yet: signing in to an MCP server by OAuth (a key goes in its
`headers`), background sub-agents, web search. The interface is in use over
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

## Reading on

- [ARCHITECTURE.md](ARCHITECTURE.md): how it is put together.
- [PROVENANCE.md](PROVENANCE.md): how it was written and what it is built from.
- [CARRIED.md](CARRIED.md): the files brought over from the earlier gnosh.
- [AUTHORSHIP.md](AUTHORSHIP.md): who asked for what.
- [GNOMEM.md](GNOMEM.md): `mem`, from the model's side.
- `spec/`: what it does, in words.

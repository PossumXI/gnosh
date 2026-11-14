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
tools, commands on declared hosts over ssh, permission modes and rules,
hooks, instruction files, sub-agents in the foreground, `mem` and frames
with the close check, `/goal`, the slide and peers' turn locks, `gnosh mem`,
`gnosh doctor`, `gnosh trust`.

Not built yet: MCP servers, rewind, background sub-agents, web search. The
interface has been tried in tmux only. Its wording to the model has not yet
been compared run against run. Hosts have been tested against a stand-in
for ssh only, not yet against a real machine. Nothing here is released.

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

In `~/.config/gnosh/gnoshrc`, for a llama.cpp server on this machine:

    provider add local --type llamacpp --base-url "http://127.0.0.1:8080"

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

# gnosh architecture

gnosh is an AI coding harness for the terminal: it sits between a person, a
language model and the machine, lets the model read, edit and run things under
the person's rules, and keeps the model's plan and findings in `mem`, in Gno,
on disk. It is written in Go, from scratch (see PROVENANCE.md).

## The four decisions everything else follows from

1. **Standard library first.** One Go module (`github.com/gnosh-ai/gnosh`,
   Go 1.27). Besides the standard library it may use `golang.org/x/term` and
   `golang.org/x/sys` (pinned to releases at least two months old) and, for
   the GnoVM, gno's own modules. Nothing else is added without Jae's say:
   every dependency is something that can be compromised in a commit.
   No Charm code of any kind, application or library.
2. **Claude Code's public conventions.** Flag names, tool names and argument
   shapes, permission modes and rule syntax, hook events and their JSON
   protocol, memory files, slash commands, skills: where Claude Code has a
   documented way and gnosh has no reason to differ, gnosh does it that way,
   because that is what Jae knows. `spec/cc/` holds the details. gnosh has
   its own look and its own words; it borrows interfaces, never text or art.
3. **An inline interface.** gnosh draws in the terminal's own scrollback: no
   alternate screen, no mouse capture. Finished output is written once and
   left alone, so selecting, copying and scrolling are the terminal's own.
   Only a small live region at the bottom (the answer being written, the
   input box, a status line) is redrawn.
4. **The context is a call stack.** What the model is sent is a *view*
   computed from the full transcript, not the transcript itself. A finished
   frame (a todo in `mem`) leaves the view and its result stays. Nothing is
   ever deleted from the transcript. `spec/gnosh/` holds the rules.

## Packages

| package | what it is |
|---|---|
| `internal/core` | The shared vocabulary: messages and blocks, provider and tool interfaces, stream events, errors. Standard library only. |
| `internal/llm` | Model services: the OpenAI-compatible chat-completions wire format, llama.cpp's extras, Anthropic's messages format. HTTP and server-sent events on `net/http`. Time limits. No retries (the agent loop decides those). |
| `internal/tools/fstools` | Read, Write, Edit, Glob, Grep, and the gitignore matching they share. |
| `internal/tools/shelltool` | Bash, background shells, their output and stopping them. |
| `internal/text` | Text for terminals: grapheme clusters, display width, ANSI-aware wrapping and truncation, styles, colour profiles, gradients. Pure functions. |
| `internal/term` | The terminal itself: raw mode, key decoding, resize, capabilities. `internal/term/vtest` is a small screen emulator for tests. |
| `internal/transcript` | Sessions as append-only JSONL files, and the view the model is sent. |
| *next* `internal/agent` | The loop: send, stream, ask permission, run hooks, run tools, repeat. |
| *next* `internal/permission`, `internal/hooks`, `internal/config` | Rules and modes; hook events; `gnoshrc` and settings. |
| *next* `internal/markdown`, `internal/ui` | Streaming Markdown to styled lines; the live region, the editor, prompts. |
| *carried* `internal/gnovm`, `internal/gnochain`, `internal/swarm`, `gno/` | The GnoVM machines, `mem`, the chain client and the swarm, brought over from the earlier gnosh (see PROVENANCE.md). |

`main.go` and the command line come with the agent loop.

## Rules for every package

- **Imports.** A package imports the standard library, `internal/core`, and
  only those other `internal/` packages its brief names. `internal/core`
  changes only through the orchestrating session; if it lacks something,
  define what you need inside your own package and say so in your report.
  Never edit `go.mod` or `go.sum`; if you need a module, say so in your report.
- **No globals, no `init` side effects.** State lives in values that are
  passed in. Anything that reads the clock, the environment, the home
  directory or the network takes it as a parameter or an option with a
  default, so tests can replace it.
- **Errors say what happened in plain words**: what was being done, to what,
  and what was left out. Wrap with `%w`. Failures the model can act on are
  tool results, not Go errors (see `core.Tool`).
- **No give-up counters.** Nothing ends work because a count ran out. Where
  a loop needs a guard, it reports and lets the caller (or the model) judge.
- **Contexts.** Everything that can block takes a `context.Context` and stops
  promptly when it is cancelled, without leaking goroutines or processes.
- **Tests.** Every behaviour in the brief has a test. Table-driven where it
  fits, `t.TempDir()` for files, `net/http/httptest` for servers, no real
  network, no sleeps longer than 50 ms, no dependence on the machine's own
  config (set `HOME` and the `XDG_*` variables to temp dirs when code reads
  them). Tests must pass with `-race` on Linux (the devbox) and be written to
  pass on macOS too. Fuzz the parsers.
- **Hosts and addresses in tests** are made-up: `203.0.113.x`, `example.com`.
- **Comments** are full plain-English sentences that say what a thing is for.
  Every exported name has one.
- **Style.** `gofmt`. Go 1.27 features are welcome (`iter`, `slices`,
  `maps`, `log/slog`, `min`/`max`, range-over-func).

## Building and testing

Go builds and tests never run on the Mac. From the repo root:

    tools/dev/remote-test.sh [-race] [-v] [-run REGEX] internal/yourpkg [more dirs]

copies `go.mod`, `go.sum`, `internal/core` and the named directories to the
devbox and runs `gofmt -l`, `go vet` and `go test` there. Inside the named
directories the devbox copy mirrors the Mac's (files deleted here are deleted
there), so anything generated on the devbox must be written to stdout or to
`/tmp` and copied back, never left in the tree. Running `gofmt -w` on the Mac
is fine. Do not run `go build`, `go vet` or `go test` on the Mac.

To run something else on the devbox:

    ssh -i ~/.ssh/id_ed25519_kourt -o IdentitiesOnly=yes root@157.245.185.86 'cd /root/src/gnosh && export PATH=/usr/local/go/bin:$PATH && <command>'

On the devbox, work only inside `/root/src/gnosh` and `/tmp`. Never use
`pgrep -f` or `pkill -f` there.

# Provenance

gnosh was started from an empty repository on 13 November 2026 and written
from scratch. It is not a fork of anything. It contains no code from
Charmbracelet's Crush, and none from the earlier program of the same name that
was a fork of Crush, except the files listed in `CARRIED.md`: those are the
earlier gnosh's own work (the GnoVM machines, `mem`, the chain client, the
swarm), or gno's under gno's own licence, and each was checked against Crush's
code before it was brought over.

## How it is written

The work is split in two, and the split is the point.

**The spec side** may read the earlier tree. It writes down, in words, what
gnosh does: behaviour, formats, option names, worked examples. Its output is
`spec/`. It writes no code in this repository, and its specs hold no code or
prompt text taken from Crush-derived files and no account of how Crush is put
together inside.

**The build side** writes all the code. It works from:

- `spec/`, `ARCHITECTURE.md`, and this repository;
- the files listed in `CARRIED.md`, once they are listed;
- public documentation of the protocols and formats it speaks (model
  providers' HTTP APIs, terminal control sequences, Unicode's data files and
  test vectors, gitignore's rules, Claude Code's user documentation);
- the Go standard library and the pinned dependencies in `go.mod`.

The build side never opens, fetches, searches for or reconstructs from memory:

- Crush's source, in the original, a fork or a mirror
  (`github.com/charmbracelet/crush`);
- the earlier gnosh / klash / crash tree (`github.com/gnosh-ai/gnosh` before
  this repository replaced it, `~/src/jaekwon/crush*`, `/root/src/gnosh-*` and
  `/root/src/klash*` on the devbox, `~/src/jaekwon/gnosh-carry` until its
  files are listed in `CARRIED.md`);
- the libraries Charm wrote for Crush's own use (`fantasy`, `catwalk`), or
  any other Charm library. Their licences would allow it; the rule is here
  so that this program's structure is its own;
- Claude Code's source code or prompt text. gnosh follows Claude Code's
  documented interface, in its own words.

A builder who recognises that it has seen another program's implementation of
the thing it is writing designs from the spec and from first principles, and
does not rebuild that implementation.

## How files are carried over

The spec side compared every file the earlier gnosh had added against Charm's
code: the tree at the point the fork began, upstream's tree today, and every
line upstream has added since. It measured the share of a file's significant
lines that also stand in Charm's code and the longest run of consecutive
lines the two share, and it took git's own copy and rename detection as a
second opinion. A file with no meaningful overlap was staged, byte for byte.
A file with overlap was not staged, and what it did is written again from
the spec.

A porter on the build side reads the staged files and nothing else of the
earlier tree. It brings a file in with its import paths changed and nothing
more, except where the file leaned on a helper of the earlier harness: such
a helper is written again from what its callers need.

Twenty files were their author's own work except for one block of four to
seven lines that also stands in Crush. They come over, if at all, only as
copies with that block cut out.

Gno's standard library and gno's B+tree package come under gno's own licence,
with its notices beside them.

`CARRIED.md` lists every file brought over with its old path, its numbers
from the scan, the checksum of the staged original, and what was changed.

The session that directs the work wrote `internal/core` and the build
briefs. It read the earlier tree's file names and sizes and its licence, and
did not open its Crush-derived source.

## What is kept as evidence

- `spec/`: what the build side was given.
- `CARRIED.md`: every file brought over, where it came from, and the numbers
  from the scan that compared it with Crush's code.
- `AUTHORSHIP.md`: who asked for what, in their own words.
- The git history, which starts empty.
- A comparison of this tree against Crush's, to be run before anything is
  published, with its method and output under `tools/audit/`.

## Dependencies

| module | version | released | why it is settled enough |
|---|---|---|---|
| `golang.org/x/term` | v0.46.0 | 2026-09-08 | The Go team's own; raw mode and terminal size only. Pinned two months back rather than at the newest release. |
| `golang.org/x/sys` | v0.48.0 | 2026-08-31 | The Go team's own; needed by `x/term`. |

## Data that is not gnosh's

| what | where | whose | terms |
|---|---|---|---|
| Unicode Character Database 17.0.0: the tables generated from it, and two of its test files | `internal/text/tables.go`, `internal/text/testdata/` | Unicode, Inc. | Unicode License V3 (`internal/text/LICENSE-UNICODE.txt`) |

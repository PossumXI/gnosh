# Similarity audit: the new gnosh against Charm's code

Made 2026-11-13 by the similarity auditor, on the spec side of the clean-room
rewrite (PROVENANCE.md, "What is kept as evidence", last item). It measures
whether anything in this repository matches Charmbracelet's code, and says
what it found, including what looks bad. Nothing in this directory is Crush
source: paths, line numbers, names, counts and plain words only. It is an
engineering record, not legal advice.

**In one sentence.** Measured four ways against every version of every file
Charm has published in Crush, and against the Charm libraries the earlier
gnosh used, the code written for this repository shares nothing with Charm's
code beyond what unrelated Go programs share with it, except eleven lines
that declare a to-do type in one carried file, and none of its prompt text
shares a sentence with Crush's; the measurements cannot show whether a
builder drew on memory of Charm's code without reproducing it, and they
turned up six things, most of them small, for a person to look at, listed
under "Findings".

## What was compared

| what | value |
|---|---|
| the new gnosh | this repository at commit `e9f3fcc1f18deef32635ce893a73dc30a8420d92` (2026-11-13), 1,173 files, as `git archive` gives them. Files changed since and not committed were not looked at. |
| Crush at the fork point | `52c75da8acbcd11dc269ae582f3ed7947ba3cff6` (2026-10-18), the earlier gnosh's merge base with charmbracelet/crush `main` |
| Crush upstream | charmbracelet/crush `main` at `7ef47d5c85f3b2b55eee234b51ba5275d61bab4a` (2026-11-13), 206 commits after the fork point. The earlier inventory compared against `dd40fd9c`, three commits before. |
| Crush's whole history | every commit reachable from the 182 branches and 211 tags of charmbracelet/crush as fetched on 2026-11-13 |
| the earlier gnosh | github.com/gnosh-ai/gnosh `main` at `1c77d2bd58482c37dd858b24326a6e1829117904` |
| where it ran | the devbox, in `/root/src/gnosh-new-audit/` (scripts, the copy of the tree, and all raw working files, which hold Crush's code and stay there). `run.sh` was run from the start into an empty folder (`work2/`) after the first pass (`work/`), and gave the same `results.tsv` byte for byte. |

Two commits were added to this repository while the audit ran (`94e98be` and
`13cea505c89f43283cbd14ab8cfa28f818dd7a8b`: 31 files changed or added, most of
them in `internal/memjoin`, and one removed). Everything below, and
`results.tsv`, is of commit `e9f3fcc1`, and line numbers are that commit's.
The same measures were then run on those 31 files as they stand at
`13cea505`: against `crush`, `hist` and `libs` they have no counted line run,
a line share of 3.1% at most, one varied window of exact tokens (the
slice-equality loop of `internal/memjoin/tasks.go`, which moved to lines 79
to 89), and no sentence in common with any Crush text. They change nothing
said here. The audit should be run again on the commit that is published.

### The corpora: what the new tree was compared with

Each has a short name, which is the prefix of its columns in `results.tsv`.

| name | what it is | size |
|---|---|---|
| `crush` | (a) Crush's tree at the fork point, (b) upstream's tree today, and (c) every file version that an upstream commit since the fork point brought in, which holds every line upstream has added. This is the earlier inventory's "core" tier, rebuilt against today's upstream. | 2,498 text files, 1,831 of them Go (4.6 million tokens); 118,286 distinct lines |
| `hist` | `crush` plus every file version reachable from any upstream branch or tag: Crush's whole history, its unmerged branches, what Charm deleted before the fork. The inventory's "ext" tier. Crush's old history vendored other people's code (Go's `x` packages, the Anthropic and OpenAI SDKs, `mvdan.cc/sh`, Charm's own libraries), so this corpus is wider than "Charm's code". | 24,352 text files, 17,120 of them Go (46.2 million tokens); 969,045 distinct lines |
| `libs` | (d) the Charm libraries the earlier gnosh's `go.mod` pinned, at those versions, from the Go module cache: bubbletea 2.1.0, bubbles 2.2.1, lipgloss 2.0.6, glamour 2.0.1, fantasy 0.45.1, catwalk 0.52.49, fang 2.0.1, log 2.0.1, x/vcr 0.1.1, colorprofile 0.4.3, ultraviolet (2026-10-08), the openai-go fork (2026-09-21), and twelve modules of charmbracelet/x (ansi 0.11.9, term, termios, windows, editor, etag, powernap, exp/charmtone, golden, ordered, slice, strings) | 24 modules, 2,228 text files, 1,066 of them Go (1.7 million tokens) |
| `old` | not asked for: the earlier gnosh's own tree, less the files CARRIED.md lists. Overlap here is not overlap with Charm; it shows what came from the earlier gnosh by a road other than CARRIED.md. | 1,765 text files, 1,199 of them Go |
| `deps` | not asked for: the other 166 modules the earlier gnosh required and the new `go.mod` does not (goldmark, chroma, uniseg, go-udiff, the model providers' SDKs and so on). The new gnosh does itself what several of them did, so the same question applies to them as to (d). | 20,733 text files, 10,947 of them Go (35.9 million tokens) |

### The files compared

Every file of the tree was scanned; the similarity question is asked of the
first group.

| group | files | what |
|---|---:|---|
| **new code** | 788 | everything CARRIED.md does not list: 583 Go files, 3 shell and 3 Python scripts; 50 prompt texts (the 48 of `internal/prompt/texts` and the 2 built-in agent definitions); 104 files of test data; 41 documents (`spec/` and the root's); `go.mod`, `go.sum`, `.gitignore`, one JSON list. The files CARRIED.md calls "written fresh" are here. |
| **carried** | 205 | CARRIED.md's rows of kind *carried* (195) and *carried with changes* (10): 91 Go and 19 Gno files, 91 files of test data, 3 documents, 1 other |
| **carried with larger changes** | 7 | the files of `internal/gnovm/tools` that CARRIED.md's second table lists |
| **third-party** | 173 | gno's standard library (160 `.gno` files, 3 notices) and B+tree (3 files, 2 notices), Unicode's two test files and licence, the two Markdown specification example files |

The Go files that hold tables generated from other people's data
(`internal/text/tables.go`, `internal/markdown/entity.go`) are counted as new
code.

## The measures

All of them are made by the scripts beside this file; `run.sh` runs them in
order.

**1. Line share** and **2. line runs** are the earlier inventory's, computed
by its own script (`taintscan.py`, copied here byte for byte), so its
thresholds carry over. Lines are trimmed, inner whitespace is made one space,
and the names crush, crash, klash and gnosh in any case become one token. A
line is *significant* when it has 25 characters or more and 12 letters or
digits, is not an import path, and does not also stand in a reference of
generic Go (Go 1.27.1's own source and thirteen common modules: nine of
`golang.org/x`, gno, testify, cobra, pflag; 1,556,900 distinct lines).

- *share*: the share of a file's distinct significant lines that stand
  anywhere in the corpus.
- *run*: 4 or more consecutive lines of 12 characters or more that stand
  consecutively, in the same order, in one corpus file; counted when the run
  holds 2 significant lines. Reported: the longest, and the lines in such
  runs in all.
- Added here, because the generic filter leaves nothing of a standard-library
  file to measure: the same two without that filter (*rawshare*, and
  *rawrun*: a run counted when 4 of its lines are not imports).

**3. Exact tokens** and **4. identifier-insensitive tokens** are new
(`tokscan.go`, on Go's own scanner). Comments go, and so do the semicolons the
scanner adds at line ends, so line breaks do not matter.

- *exact*: string contents are stripped (every string literal is one
  placeholder); identifiers, numbers, keywords and operators stay as written.
  Window: 40 consecutive tokens.
- *abstract*: every identifier becomes one placeholder as well, predeclared
  names included, and every literal becomes its kind. A copy with every name
  changed has the same abstract stream. Window: 60 consecutive tokens.

No sampling and no winnowing: every window of every target file is looked up
against every window of every corpus file (a 64-bit rolling hash, and the
tokens are compared again before a match is believed). Per file: the tokens
covered by shared windows, and the longest run of tokens that stands, token
for token, in one corpus file.

One refinement was needed, and it is the main thing to understand about these
two measures. With string contents stripped, any list of twenty strings is
the same forty tokens as any other, and so is any table of like rows. Such
lists dominate the plain counts everywhere: a file of Go's `net/http` shares
a run of 357 exact tokens with Crush, and one of ollama's 501. So each
measure is given twice: *with lists and tables*, as defined above, and
*varied*, counting only windows that hold at least 12 different tokens of
their 40 (exact) or 9 of their 60 (abstract). The setting was chosen on the
calibration sets:

| a window is varied at (exact / abstract) | Crush files the earlier gnosh modified, with a varied window (of 291) | "short block" files with one (of 20) | control files with one (of 2,242); longest run | new code with one (of 527); longest run |
|---|---|---|---|---|
| 10 / 8 different tokens | 288 / 286 | 15 / 10 | 76 / 227; 117 / 223 | 29 / 66; 43 / 87 |
| **12 / 9** | 288 / 286 | 15 / 7 | 62 / 195; 117 / 223 | 2 / 43; 41 / 83 |
| 15 / 10 | 288 / 286 | 12 / 7 | 57 / 179; 117 / 223 | 2 / 29; 41 / 81 |
| 18 / 12 | 288 / 282 | 10 / 4 | 46 / 152; 88 / 117 | 2 / 10; 41 / 77 |

At every setting the new code stays inside the control set's range. 12 / 9 is
the highest at which the exact measure still sees every short-block file it
saw at 10. (`tokscan`'s `-div` and `-diva` set it; `run.sh` runs 12 / 9, and
the other three rows were run by hand against `crush`.)

**Words** (`textscan.py`), for text a model or a person reads. Text is
case-folded and split into words; a sentence ends at a full stop, question or
exclamation mark, a blank line, or the start of a list item. Per text: how
many of its sentences of 5 words or more stand word for word in the corpus,
and the longest run of consecutive words that stands in one corpus text. The
corpus is every non-Go text file of Crush (its prompt templates and tool
descriptions among them) and every string literal of its Go code. A Go file
of the new tree is measured by its string literals.

**Names and whole files** (`names.py`, `idents.go`): git's own copy
detection, file names, declared names, the exported names of each package,
the words of the config file, brand words, environment variables, struct-tag
keys, `go.mod`.

### Calibration: what the measures read on files whose answer is known

Against `crush`. "Crush files" are from the earlier gnosh's tree: the 443 Go
files it never changed, the 299 it modified, the 28 added Go files the
inventory called REWRITE and the 20 it held back for one block of 4 to 7
shared lines. The control set is unrelated Go of a similar kind: three
packages of Go's standard library and four permissively licensed command-line
programs (fzf 0.75.0 and ollama 0.40.16, MIT; GitHub's `gh` 2.104.0, MIT;
`mvdan.cc/sh` 3.14.1, BSD), 2,432 files.

| set | files | line share: median / 95th / highest | files with a counted run; longest | exact, varied: files with a window; longest run | abstract, varied: files; longest | exact with lists and tables: longest run; highest cover |
|---|---:|---|---|---|---|---|
| Crush files, unchanged | 443 | 100% / 100% / 100% | 434; 695 lines | 391 of 391; 5,457 | 391; 3,700 | 5,457; 100% |
| Crush files, modified | 299 | 85.9% / 100% / 100% | 292; 1,016 | 288 of 291; 6,677 | 286; 3,884 | 8,663; 100% |
| REWRITE files | 28 | 20.9% / 100% / 100% | 16; 377 | 19 of 24; 2,923 | 15; 2,923 | 2,923; 93.6% |
| short-block files | 20 | 5.6% / 11.9% / 13.9% | 18; 7 | 15 of 20; 85 | 7; 86 | 89; 29.5% |
| control: Go `net/http` | 157 | none significant (raw: 0% / 4.5% / 11.1%) | 0 (raw: 1; 7) | 5 of 144; 117 | 13; 117 | 357; 21.5% |
| control: Go `os/exec` | 29 | (raw: 0% / 9.3% / 11.3%) | 0 | 0 of 17 | 0 | 0 |
| control: Go `text/template` | 17 | (raw: 0% / 1.3% / 1.5%) | 0 | 1 of 16; 40 | 1; 62 | 189; 9.5% |
| control: fzf | 91 | 0% / 1.0% / 4.7% | 0 (raw: 2; 5) | 1 of 81; 41 | 1; 61 | 100; 54.0% |
| control: gh | 985 | 0% / 4.2% / 12.5% | 0 (raw: 5; 14) | 17 of 916; 56 | 91; 106 | 135; 28.0% |
| control: mvdan.cc/sh | 77 | 0% / 2.7% / 9.7% | 0 (raw: 1; 4) | 0 of 67 | 7; 85 | 203; 20.5% |
| control: ollama | 1,076 | 0% / 2.6% / 17.6% | 0 (raw: 16; 6) | 38 of 1,001; 75 | 82; 223 | 501; 92.0% |

Reading it. A Crush file, changed or not, lights up on every measure: runs of
hundreds of lines and thousands of tokens. A file with one copied block of 4
to 7 lines is seen by the line run in 18 of 20 cases, by varied exact tokens
in 15, by varied abstract tokens in 7: small copies can slip under the token
windows, and are caught by lines only if the lines were left as they were.
Unrelated Go never has a counted line run against Crush; its line share tops
out at 17.6%; 62 of its 2,242 larger files (2.8%) share some varied window of
40 exact tokens with Crush, the longest 117 tokens; 195 (8.7%) share a varied
window of 60 abstract tokens, the longest 223. That is what "ordinary Go
resembles ordinary Go" looks like, and it is the yardstick below.

gh and ollama use some of Charm's libraries (they import them; they do not
contain them), so their numbers against `libs` are, if anything, high.
Against `hist` the control set is no control: Crush's old history holds
`mvdan.cc/sh` and parts of Go itself as vendored code, and those controls
read 80 to 100% there. For `hist` the yardstick is the control set's range
against `crush`.

### Thresholds

- For lines, the inventory's rules: a file is suspect when its longest
  counted run is 8 lines or more, or its run lines are 10% of the file, or
  20% of its significant lines (and at least 5) stand in the corpus, or 5 of
  them, being 12%, stand in one Charm file; and a file with any counted run
  is at least a "short block" case.
- For every measure, the highest value in the control set: a file of new code
  above it is listed and judged.
- Whatever the numbers, the twenty highest files of each measure were read on
  both sides.

## Results

`results.tsv` has one row per file with every measure against every corpus;
`tables.md` has every distribution; `review.md` has one line of judgement for
each of the 135 files that appear in a top-twenty list.

### New code against Charm's code

| measure | against `crush` (a to c) | against `hist` | against `libs` (d) | control set's highest (vs `crush`; vs `libs`) |
|---|---|---|---|---|
| line share, 556 files with 10+ significant lines: median / 95th / highest | 0.0% / 1.2% / 10.0% | 0.0% / 2.2% / 10.0% | 0.0% / 1.0% / 7.4% | 17.6%; 12.0% |
| the same without the generic-Go filter: 95th / highest | 4.2% / 10.8% | 5.5% / 13.8% | 3.0% / 11.8% | 22.5%; 41.7% |
| files with a counted line run | 0 | 0 | 0 | 0; 1 file, 4 lines |
| files with any run of 4+ lines; longest | 4; 5 lines | 4; 5 | 0 | 25 files, 14 lines; 8 files, 14 lines |
| exact tokens, varied: files with a shared window (of 527); longest run | 2; 41 tokens | 5; 53 | 7; 55 | 62 files, 117; 58 files, 62 |
| exact tokens, varied: highest cover of one file | 5.5% | 5.5% | 7.2% | 28.0%; 59.4% |
| abstract tokens, varied: files with a shared window; longest run | 43; 83 tokens | 65; 117 | 51; 121 | 195 files, 223; 222 files, 125 |
| abstract tokens, varied: highest cover of one file | 47.2% | 48.2% | 16.2% | 69.4%; 97.5% |
| exact tokens with lists and tables: longest run; highest cover | 268; 99.5% | 1,770; 99.6% | 434; 99.6% | 501, 92.0%; 343, 91.8% |
| abstract tokens with lists and tables: longest run; highest cover | 268; 99.5% | 1,772; 99.8% | 434; 44.7% | 502, 92.9%; 465, 98.8% |

No file of new code meets any of the inventory's line rules against any of
the three corpora, and none has a counted run at all. On every measure that
tells code from code (line share, line runs, varied token runs and cover) the
new code lies inside the control set's range.

**Files above the control set's highest.** Four, all on the measures that
include lists and tables, and none of them with a single varied window:

| file | measure | what it is |
|---|---|---|
| `internal/markdown/entity.go` | exact and abstract cover with lists and tables, 99.5% to 99.8% against all three (control 91.8% to 92.9%) | A map of 253 names to strings, generated from Python's `html.entities` (regenerated byte for byte here). It matches any Go map of string pairs. Data, not Charm's. |
| `internal/text/tables.go` | exact and abstract run with lists and tables against `hist`, 1,770 tokens (control 501), and cover 93.4% to 94.3% | Packed strings joined with `+`, generated from the Unicode Character Database (regenerated byte for byte here). The match is `golang.org/x/text`'s tables, which Crush's history vendors: the same form, not the same contents. Data, not Charm's. |
| `internal/diff/lines_test.go` | exact run with lists and tables against `libs`, 434 tokens (control 343) | Rows of strings in a table test, lines 32 to 94, against rows of strings in a lipgloss test. A table of data. |
| `internal/config/table.go` | exact run with lists and tables against `libs`, 361 tokens (control 343) | Rows of three strings (environment variables and what they mean), lines 490 to 532, against the same lipgloss test. A table of data. Finding 3 is about the names in this file. |

**The top twenty of each measure** were read on both sides: 382 matches in
135 files (`review.tsv`, `review.md`). What they are:

- Lists and tables of literals: 40 files (lists of command names, sets
  written as `map[string]bool`, rows of table tests, const blocks).
- Single lines scattered through a file: 47 files. The lines are JSON field
  tags of public wire formats (Anthropic's and OpenAI's APIs, Claude Code's
  hook input and stream output), HTTP header lines, and standard-library
  idiom (`filepath.EvalSymlinks(t.TempDir())`, `strings.Join(parts, "\n")`).
- Test scaffolding: the head and loop of a table-driven or fuzz test, an
  `httptest` handler's first lines.
- A buffer behind a mutex with `Write` and `String` (five test files), and
  methods that take a lock, read or set one field, and return.
- Getters and setters in a row; a slice-equality loop; a slice reversal; a
  UUID written out with `hex.Encode`.

The longest varied run of exact tokens that new code shares with `crush` is
41 tokens, twice: the first four lines of a lock test
(`internal/filelock/filelock_test.go` 125 to 128, against Crush's
`internal/lock/lock_test.go` 49 to 53), forced by the one function name the
carried code calls; and an eleven-line slice-equality loop
(`internal/memjoin/tasks.go` 66 to 76). The longest against `libs` is 55
tokens of table-test rows. The longest abstract run against `libs`, 121
tokens, is six rows of a table test for the `Retry-After` headers
(`internal/llm/errors_test.go` 272 to 277, against fantasy's `retry_test.go`
424 to 429): the same shape of row for the same HTTP rule, cases worded
differently, nothing shared in the code under test.

Nothing read was expression that could only have come from Crush. Two things
that these lists brought up are taken further under Findings: the names of
finding 4 and the tag files of finding 5.

**Against the dropped dependencies (`deps`).** The same background: no
counted line run, line share at most 12.9% (4 lines of 31), 15 files with a
varied exact window, the longest 55 tokens of table-test rows. The Unicode
tables were regenerated from Unicode's files, not taken from a width or
grapheme library.

**Against the earlier gnosh (`old`).** This is not a comparison with Charm.
Of new Go code, 8 files have a counted line run, all of them the files
written fresh under `internal/gnovm`, and all of them the 7 lines of that
subsystem's licence header; 10 files have a varied exact window, the longest
55 tokens; 71 have a varied abstract window, the longest 234 tokens (finding
6) and the next 94. The prompt texts are another matter: 20 of the 44 share
a run of 12 words or more with the earlier tree (finding 2).

### Prompt texts and other words, against Crush's

| what | units | sentences | sentences that also stand in `crush` / `hist` / `libs` | longest shared run of words |
|---|---:|---:|---|---|
| prompt texts (`internal/prompt/texts`, the two agent definitions) | 44 of 20+ words | 331 | 0 / 0 / 0 | 6 / 6 / 5 words |
| string literals of new Go code (tool descriptions, messages, tests) | 397 files | 15,106 | 26 / 46 / 26 | 13 / 21 / 21 |
| documents (`spec/`, the root's) | 41 | 9,719 | 1 / 1 / 0 | 8 / 8 / 8 |
| test data (help texts, goldens) | 102 | 1,368 | 0 / 0 / 0 | 10 / 10 / 9 |
| for comparison: the earlier gnosh's prompt templates and tool descriptions (Crush's files as it changed them) | 47 | 2,117 | 787 | median 32, 95th 725 |
| for comparison: string literals of the control programs | 1,823 files | 45,702 | 767 against `crush` | 95th percentile 7 words or fewer in each program |

No prompt text shares a sentence with any Crush text or any string of Crush's
code, in any version. The 26 shared "sentences" of the string literals were
all read: import paths, `127.0.0.1` addresses, a date layout, character
classes, the address of OpenAI's API. The longest shared runs of words are a
line of OpenAI's wire format in a test (13 words), a provider's error message
quoted in a test, and the numbers 1 to 21. The one shared sentence in the
documents is five words of a flag's description in `spec/gnosh/harness-cli.md`.

### The carried files

`carried_check.py`, for all 380 rows of CARRIED.md's two tables:

- **Bytes.** The 195 *carried* files and the 168 *third-party* files are byte
  for byte the earlier gnosh's files at their old paths. The 10 *carried with
  changes* differ from them in exactly one line each. The SHA-256 the table
  records is that of the earlier gnosh's file in all 380 rows.
- **Numbers.** The inventory's measures were taken again with its own script.
  Of the earlier gnosh's file against the inventory's own corpus they give
  the table's line share and longest run in all 373 rows that have numbers (7
  are binary test data). Of the file as it stands in this tree, against that
  corpus and against today's, they agree in 369; the four that differ are
  among the seven tools files, which were changed by design:
  `docs_test.go` 1.2% to 1.1%, `tools.go` 1.7% to 1.0%, `tools_test.go` 3.2%
  to 3.1%, and **`memtodos.go` 0.0% to 11.1%** (finding 1). No carried file
  has a counted run.
- **Tokens.** Carried Go files: none has a varied exact window against
  `crush`; 7 of 102 have a varied abstract window, the longest 74 tokens,
  inside the control range.
- The tree's own `tools/audit/carried.py check` passes.

### The third-party files

`thirdparty.py`:

- gno's standard library: all 160 `.gno` files under `internal/gnovm/stdlibs`
  are byte for byte the files of `gnovm/stdlibs` in
  `github.com/gnolang/gno v1.5.1-0.20260928125126-5cdbc25fcde0`, the version
  `VERSION` names (the tree leaves out 94 of gno's 254). `LICENSE.md` is gno's
  `LICENSE.md` after a 320-byte preface; `LICENSE-GO` is Go's `LICENSE`.
- gno's B+tree: the three `.gno` files are byte for byte
  `examples/gno.land/p/nt/bptree/v0` of the same version.
- Unicode: the tree's generator, run again on files fetched from unicode.org
  and checked against the SHA-256 it records for each, writes
  `internal/text/tables.go` byte for byte; the two test files are Unicode's.
- Markdown: `commonmark-0.31.2.json` is byte for byte the file at
  spec.commonmark.org; the GFM page fetched today has the SHA-256 the notice
  records, and the tree's extractor run on it writes `gfm-0.29.json` byte for
  byte.
- `internal/markdown/entity.go`: the tree's generator writes it byte for
  byte.
- Against `crush` and `libs` no third-party file has a counted line run or a
  varied window of exact tokens. Against `hist`, `LICENSE-GO` shares its 23
  lines (Go's licence text, which Crush's history also holds, as CARRIED.md
  says), and 9 of gno's standard-library files share long runs with Go
  packages that Crush's old history vendored (up to 1,311 varied exact
  tokens): Go's code in both, not Charm's.

### Names and whole files

- **git.** With Crush's two trees in one commit and the new tree added in the
  next, `git diff -C20% --find-copies-harder` pairs 7 of the 1,173 files with
  a Crush file: two empty lock files with an empty Crush file (100%), and
  five race-detector tag files with Crush's two (21% to 36%; finding 5). No
  other file reaches 20%.
- **Every commit.** Of the 1,473 blobs and trees in every commit of both
  branches of this repository, two are also objects of Crush's repository:
  the empty file and Go's BSD licence text.
- **File names.** 161 of the 681 Go files outside the third-party trees have
  a file name some Crush file also has (`bash_test.go`, `edit.go`,
  `config.go`). Compared pair by pair, the race files aside, the most any
  pair shares is 1.9% of its long lines. Of the 34 directories of new code
  under `internal/`, 7 have the path of a Crush package: `agent`, `app`,
  `config`, `diff`, `hooks`, `permission`, `ui`.
- **go.mod.** No line of `go.mod` or `go.sum` names a Charm module; none of
  the 148 modules of the build list does; none of the 546 packages of the
  import graph, tests included, does; no Go file imports one.
- **Brand words.** "crush" stands in new code only in the table of old file
  names that are still read (`internal/config/table.go`, 4 times) and its
  test, and in `tools/audit/carried.py`; otherwise in documents that tell the
  history, and in carried code where CARRIED.md says so
  (`internal/gnovm/scenario_test.go`, `client.go`). "charm" and
  "charmbracelet" stand only in documents, in `carried.py` and in the carried
  licence and design note of `internal/gnovm`. No `CRUSH_`, `CHARM_` or `CATWALK_` environment variable is read
  by new code; the one `CRUSH_PROFILE` is in carried `internal/gnovm/client.go`,
  as CARRIED.md says.
- **Declared names.** 569 names that Crush, its history or a Charm library
  declares, and that no reference Go code declares, stand in the new tree
  (`idents.tsv` on the devbox). In new code, from `crush` itself: 4 package
  names (`hyper`, `lsp`, `mcp`, `styles`, each used as an ordinary variable or
  field name), 14 types, 31 functions, 13 methods, 8 constants, 31 variables
  and 48 fields. Read through, they fall into: names of Claude Code's documented interface
  (`PermissionMode`, `PermissionRequest`, `ToolInput`, `ToolResponse`,
  `UpdatedInput`, `AdditionalContext`, `LastAssistantMessage`); names of the
  model APIs (`ContextWindow`, `ExtraBody`, `MaxInputTokens`); the to-do
  names of finding 1; the config names of finding 3; `raceEnabled` (finding
  5); and ordinary compounds (`firstLines`, `tailLines`, `mustJSON`,
  `openPTY`, `NewRunner`).
- **Exported names, package by package.** For each package, the Charm package
  that declares the most of the same exported names. Control packages (169):
  median 9%, 95th percentile 23%. New code (26 packages): median 14%, and two
  stand out: `internal/term`, 88 of 170 names also in ultraviolet, and
  `internal/text`, 36 of 71 also in x/ansi (`api.tsv` on the devbox).
  Finding 4.

## Findings, most serious first

None of these is a passage of Charm's code in new code. They are the places
where this tree and Charm's code, or this tree and its own record, meet.

**1. Eleven lines of a carried file declare a to-do type as Crush does.**
`internal/gnovm/tools/memtodos.go`, lines 20 to 36: a status type, its three
constants and a three-field struct. The same declarations stand in Crush's
`internal/session/session.go`, lines 20 to 26 and 35 to 39 at the fork point,
token for token but for one struct tag. The staged original had none of it
(line share 0.0%); the porter, replacing "the earlier harness's todo type and
its three statuses" (CARRIED.md), declared them again in the file, and five
significant lines of the 45 now stand in that one Crush file: 11.1%, where
the inventory's own rule calls a file suspect at 5 lines and 12%. The three
status words and the field names are Claude Code's published to-do schema,
and the Go names reached the porter as the names of the symbols the staged
file used, so there was little room to write it otherwise. It is still the
one place where lines written for this repository reproduce a declaration of
Crush's. *Needs a human look.* New names for the type and its constants would
end it; three files of `internal/memjoin` and the tests use them.

**2. Prompt text was brought from the earlier gnosh and is not in
CARRIED.md.** None of it is Charm's (no sentence of any prompt text stands in
any Crush corpus), but two statements of the record do not hold for it.

| new file | from | how much |
|---|---|---|
| `internal/prompt/texts/close-review.md` | the earlier gnosh's `internal/agent/templates/close_check.md` (an added file; staged by the inventory as its own work) | 22 of 23 sentences; a run of 371 words; git calls it a 94% copy |
| `internal/prompt/texts/goal-judge.md` | `internal/agent/templates/goal_check.md` (added; staged) | 8 of 11 sentences; a run of 167 words; git: 90% |
| `internal/prompt/texts/memory-full.md`, `memory.md` | the memory block the earlier gnosh wrote into `internal/agent/templates/coder.md.tpl`, **a Crush file it modified**; the inventory cut the block out as `own-text/coder-prompt-memory-block.md` | 1,120 of 1,295 words and 935 of 1,103 words lie in runs of 8+ words shared with that block |
| `internal/prompt/texts/gno.md` | the gno block of the same template (`own-text/coder-prompt-gno-block.md`) | 87 of 88 words |
| `internal/prompt/texts/deliver.md` | the artifacts block of the same template | 140 of 191 words |
| `mem-line.md`, `close-entered-here.md`, `close-sendback.md`, `frame-line.md` and ten more short texts | messages in the earlier gnosh's own added Go files (`internal/agent/mem_line.go`, `close_check.go`, `refused_close.go`, `frames.go`, `mem_reminder.go`) and, for two of them, its hooks documentation; most are quoted in `spec/gnosh/` | runs of 12 to 50 words |

The commit that added them says so ("the memory, Gno and close-review texts
reuse the earlier gnosh's own wording where the spec still asks for it"). But
CARRIED.md says it lists every file that was not written for this repository
and that "nothing else that was staged was brought", and PROVENANCE.md says
the specs hold no prompt text taken from Crush-derived files. Two of these
texts are staged files under new names, and four are made of blocks that
stood in a Crush-derived file. Less than half of the memory text is in `spec/`, so it
reached the builder by another road (the inventory's `own-text` folder, or
the brief). The inventory checked each line of those blocks absent from the
fork-point template and from Charm's corpus; this audit's word measure
agrees. *Needs a human look*, to bring the two documents in line with what
was done.

**3. Two interfaces of Crush's design are kept on purpose.** They came through
`spec/`, which says why (Jae's own files use them). They are names, not code,
and the code behind them was written anew: no file of `internal/config` has a
line run with Crush's code or a varied window of exact tokens, and its three
varied windows of abstract tokens are a list, a set, and eight lines that
marshal, make a directory and write a file.

- *The config file's language.* Crush reads a file of shell-like lines
  (`crushrc`, since July 2026); the new gnosh reads the same language as
  `gnoshrc`, and Jae's `crushrc` itself as a fallback
  (`spec/gnosh/harness-config.md`, CFG-1 and CFG-5). Of the 48 flags and 22
  dashed option names that stand in Crush's package for it, 41 and 16 stand
  in the new `internal/config` (`rc.tsv` on the devbox lists them), beside 12
  the earlier gnosh added and 25 of its own. Sixteen unexported names of
  Crush's package are names in the new one as well, each the obvious name
  for its job: `flagKind`, `flagSpec`, `optionSpec`, `optionSpecs`,
  `providerAdd`, `modelAdd`, `modelRemove`, `mcpAdd`, `lspAdd`, `hookAdd`,
  `hookRemove` and five `...AddFlags` and `...RemoveFlags` tables. Both sides
  were read: the tables hold different fields and the parsing is built
  differently (new `internal/config/table.go` 88 to 126 and `options.go` 100
  to 140, against Crush's `internal/shellconfig/flags.go` 12 to 68 and
  `options.go` 169 to 198).
- *The question tool's arguments.* `spec/gnosh/harness-tools.md` (TOOL-1)
  gives `AskUserQuestion` the argument names of Crush's question tool
  (`confirm_title`, `confirm_description`, and `label`, `type`, `choices` for
  each question), and `internal/ui/question.go` (lines 287 and 324 to 325)
  reads them. The same table lists Crush's lower-case tool names beside
  Claude Code's; none of those names is in new code.

*Needs a human look* only as a decision already taken: a reader should know
that the config language is Crush's.

**4. Names shared with Charm's terminal libraries.** `internal/term` and
`internal/text` are written from the standards they cite (ECMA-48, the DEC
manuals, xterm's control sequences, the kitty keyboard protocol), and no
code of theirs matches a Charm library. Their names often do: 88 of
`internal/term`'s 170 exported names are also ultraviolet's, most of them
key, modifier and mouse names; 36 of `internal/text`'s 71 are also x/ansi's
(the sixteen colour names, the ECMA-48 control names, `Bold`, `Italic`,
`Wrap`, `Truncate`, `Strip`). Most of that is the common vocabulary of the
trade: the same `internal/term` shares 76 names with gdamore/tcell, an
unrelated terminal library, and `internal/text` 19 with termenv. The key
list is not ultraviolet's list: the order differs, and it stops at F35, the
kitty protocol's last function key, where ultraviolet's goes to F63. Three
names are less common and are spelled as Charm spells them: `DefaultEscTimeout`, with ultraviolet's value of fifty
milliseconds (`internal/term/terminal.go` 9 to 12 and 28, against
ultraviolet's `terminal_reader.go` 25 to 27 and 48); and `EraseLineRight` and
`ResetStyle` (`internal/term/seq.go` 29 and 47, against x/ansi's `screen.go`
71 and `style.go` 12), whose values the standards fix. `internal/text`'s
`Profile`, with `Convert` and `TrueColor` and `ANSI256`, has the shape of
colorprofile's and of termenv's before it; read side by side with
colorprofile's `profile.go`, the constants, the conversion and the detection
are its own. *Needs a human look*: whether those three names should change.

**5. Race-detector tag files with Crush's file names.** Four packages have a
pair of files named `race_on_test.go` and `race_off_test.go` (the repository
root, `internal/diff`, `internal/markdown`, `internal/transcript`), each
holding a build constraint, a comment and one constant, which three of the
four call `raceEnabled` (`internal/diff` calls it `underRace`). Crush's
`internal/backend/race_on_test.go` and `race_off_test.go` hold the same two
functional lines, with the constant `raceEnabled`, under the same file names,
and git pairs five of the eight with them at 21% to 36%. The constraint line
is the only way to write it and the constant is one line; no other pair of
files of that name was found in the module cache outside Charm's code, though
`race_on.go` and `raceEnabled` are both in use elsewhere. The first of the four was committed
before the carried tests that read `raceEnabled` arrived. The fifth pair,
`internal/gnovm/racedetector_{on,off}_test.go`, was written fresh with other
file names because the earlier gnosh's pair had been a copy of Crush's. Idiom,
by this auditor's reading; a person may prefer the other four renamed the
same way.

**6. One function has the shape of one in the earlier gnosh (not Charm's).**
`internal/cli/suggest.go`, lines 22 to 43: an edit-distance routine. With
identifiers set aside it is the same 234 tokens as the routine at lines 162
to 183 of the earlier gnosh's `internal/ui/dialog/slash.go`, the next highest
such match against the earlier tree being 94. That file is the earlier
gnosh's own (added 2026-10-24); the inventory held it back for four lines
elsewhere in it and offered a copy with those lines cut. The routine is the
textbook optimal-string-alignment distance on three rolling rows, and the
names inside differ; it stands in no version of Crush and in no Charm
library. Whether a builder had the excised copy open or wrote the textbook
routine again, the measures cannot say. If it was taken from the copy,
CARRIED.md should say so. *Needs a human look.*

Also seen, and not findings:

- `go.mod` and `go.sum` share lines with Crush's (41% and 35% of their
  lines; a run of 30 lines in `go.sum`): the same modules at the same
  versions have the same checksums.
- The files written fresh under `internal/gnovm` share a run of 7 lines with
  the earlier gnosh: the subsystem's own licence header.
- `internal/gnovm/LICENSE.md` (carried) still says most of gnosh derives from
  Crush; CARRIED.md already notes it.

## What needs a human look, in one place

| where in this tree | where in the other | what |
|---|---|---|
| `internal/gnovm/tools/memtodos.go` 20 to 36 | Crush `internal/session/session.go` 20 to 26 and 35 to 39 (fork point) | the same eleven lines of declarations, one struct tag apart (finding 1) |
| `internal/prompt/texts/` (`close-review.md`, `goal-judge.md`, `memory.md`, `memory-full.md`, `gno.md`, `deliver.md` and the short texts of finding 2); CARRIED.md "What was brought"; PROVENANCE.md "How it is written" | the earlier gnosh's `internal/agent/templates/close_check.md`, `goal_check.md`, `coder.md.tpl`; the carry folder's `own-text/` | text carried without a record (finding 2) |
| `internal/term/terminal.go` 9 to 12, 28; `internal/term/seq.go` 29, 47 | ultraviolet `terminal_reader.go` 25 to 27, 48; x/ansi `screen.go` 71, `style.go` 12 | three names as Charm's libraries spell them (finding 4) |
| `race_on_test.go`, `race_off_test.go` at the root and in `internal/diff`, `internal/markdown`, `internal/transcript` | Crush `internal/backend/race_on_test.go`, `race_off_test.go` | the same file names and two lines (finding 5) |
| `internal/cli/suggest.go` 22 to 43 | the earlier gnosh's `internal/ui/dialog/slash.go` 162 to 183 | the same routine with other names; not Charm's (finding 6) |
| `internal/config/table.go`, `options.go`, `rc.go`; `internal/ui/question.go` 287, 324 to 325 | Crush `internal/shellconfig/`; Crush `internal/agent/tools/question.go` | interfaces kept on purpose (finding 3): for knowledge, not for change |

## What was not checked, and what the measures cannot show

- **Ideas and structure.** Every measure here finds shared text or shared
  token sequences. A file written while looking at a Crush file, with every
  line recast, shares neither. So does a design that follows Crush's without
  a line in common. The identifier-insensitive measure sees renamed copies of
  60 tokens and more; it does not see a reordered or restructured one.
- **Small blocks.** A copied block under 40 tokens (exact) or 60 (abstract)
  is below the token windows, and the line measures see it only if its lines
  were left unchanged and are long enough to count. The calibration says how
  often: of twenty files known to hold one block of 4 to 7 Crush lines, lines
  see 18, exact tokens 15, abstract tokens 7. Finding 1 was found by the line
  share of one file changing, not by a window.
- **Memory.** Whether a builder (a language model) reproduced Charm's code
  from what it had seen in training cannot be tested except through its
  output, which is what was measured.
- **Claude Code.** There is no corpus of Claude Code's source or prompt text
  to compare with. The tree follows Claude Code's documented interface; that
  it does so "in its own words" was not checked.
- **Other programs.** Only Crush, Charm's libraries and the earlier gnosh's
  other dependencies were compared. Other coding harnesses were not.
- **The working tree.** The commit was audited. Files changed and not
  committed (at the time, `internal/permission`) were not.
- **Shell and Python.** The six scripts of new code have line and word
  measures only.
- **The generic-Go filter** drops lines that Go's own source and a dozen
  common modules also have, as the inventory's did; the unfiltered columns are
  beside the filtered ones.
- **The hand review** was one reader's. Its sorting of matches is by simple
  rules on the new tree's own lines (`review.py`); every match so sorted was
  then read, but 382 matches were read once, not twice.

## What is in this directory

| file | what |
|---|---|
| `README.md` | this |
| `results.tsv` | one row per file of the tree (1,173), 96 columns. `sig_lines`, `long_lines`, `tokens`, `words`, `sentences` are the file's own sizes. Then for each corpus prefix (`crush`, `hist`, `libs`, `old`, `deps`): `_share`, `_run`, `_run_lines`, `_lrun` (longest run of lines each somewhere in the corpus), `_rawshare`, `_rawrun`, `_rawrun_lines`; `_tok_run`, `_tok_cov` (exact tokens with lists and tables: longest run in one corpus file, tokens covered), `_tok_vrun`, `_tok_vcov` (the same, varied windows only); `_abs_run`, `_abs_cov`, `_abs_vrun`, `_abs_vcov` (identifier-insensitive); `_word_run`, `_sent_shared`. The last four columns name the Crush or library file behind the best line share and the longest varied runs, with line ranges. A Crush file is named `tier:path@blob`: `base` the fork-point tree, `up` brought in by a later upstream commit, `hist` the rest of the history. |
| `tables.md` | every distribution (each set of files against `crush`, `hist`, `libs`, for lines, tokens and words), the control range, and the twenty highest files of each measure |
| `review.md`, `review.tsv` | the hand review: one line per file, and one row per match with both line ranges |
| `run.sh` | runs the whole audit; its head says what it needs |
| `common.py` | settings; reads CARRIED.md to group the files |
| `taintscan.py` | the earlier inventory's line scan, unchanged (SHA-256 `37a5364f764c3d068b5b28687916a36f8b5d2e8db2d98bef7848695b1590a3cd`) |
| `build_corpus.py` | gathers the corpora and the lists of files |
| `linescan.py` | measures 1 and 2 |
| `tokscan.go` | measures 3 and 4; also lists string literals |
| `textscan.py` | the word measures |
| `idents.go`, `names.py` | names and whole files |
| `thirdparty.py`, `carried_check.py` | the third-party and carried files |
| `report.py`, `review.py`, `peek.py` | the tables; the review list and its verdicts; a viewer that prints both sides of a match on the terminal |

To run it again, on a machine that may hold Crush's source and never inside
this working tree:

    CRUSH_REPO=/path/to/clone NEW_TREE=/path/to/copy WORK=/path/to/work tools/audit/similarity/run.sh

The two Go files carry `//go:build ignore`, so they are not part of the
module's build.

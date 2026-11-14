# Similarity audit: the new gnosh against Charm's code

Made 2026-11-14 by the similarity auditor, on the spec side of the clean-room
rewrite (PROVENANCE.md, "What is kept as evidence", last item). This is the
second run. The first was made on 13 November 2026, of commit `e9f3fcc`, and
its record is this directory as commit `de16b1f` has it. This run measures
the tree again from the start, at commit `1a1e947`, and says what it found,
including what looks bad. Nothing in this directory is Crush source: paths,
line numbers, names, counts and plain words only. It is an engineering
record, not legal advice.

**In one sentence.** Measured the same four ways against every version of
every file Charm has published in Crush, including what it published in the
day since the first audit, and against the Charm libraries the earlier gnosh
used, the code written for this repository shares nothing with Charm's code
beyond what unrelated Go programs share with it, the eleven lines the first
audit found are gone, and none of its prompt text shares a sentence with
Crush's; but names of Crush's design are still part of gnosh's interface
(some keys of `gnosh.json` and its layout, the environment variables a hook
is given, much of the command line), which is a matter of names and not of
code and is the first of four things listed under "Findings" for a person to
look at; and the measurements still cannot show whether a builder drew on
memory of Charm's code without reproducing it.

## What changed since the first audit

The first audit's six findings, one by one:

| the first audit's finding | now |
|---|---|
| 1. Eleven lines of `internal/gnovm/tools/memtodos.go` declared a to-do type as Crush does | **Gone.** Commit `290dd70` wrote the type anew (`TaskLine`, `LineState`). No line of the file stands in any Crush corpus: 0 of its 47 significant lines, where it was 5 of 45. |
| 2. Prompt text was brought from the earlier gnosh without a record | **Gone.** CARRIED.md has a section for it ("Words carried into `internal/prompt`") and PROVENANCE.md says so. The 20 texts that share a run of 12 words or more with the earlier gnosh are the six long ones that section names and fourteen short ones of the kind it describes. None of it is Charm's, as before. |
| 3. Two interfaces of Crush's design were kept on purpose | **Changed.** Both are gone: gnosh reads one JSON file and no longer the line language (`fbf5bf0`), and the question tool takes Claude Code's arguments. But this run compared every name of the configuration, the environment and the command line with Crush's, which the first did not, and found that other names of Crush's design are still in use. That is finding 1 below. |
| 4. Three names were spelled as Charm's terminal libraries spell them | **Changed.** The three were renamed (`DefaultEscWait`, `EraseToLineEnd`, `StyleReset`). One of the same kind is left, the field `Options.EscTimeout`: finding 4 below. |
| 5. Race-detector tag files had Crush's file names | **Gone**, but for one constant. The four pairs are now `racedetector_on_test.go` and `racedetector_off_test.go`, their constant is `raceDetector` or `underRace`, and git pairs none of them with a Crush file. `raceEnabled` stands only in `internal/gnovm`, where two carried tests read it. |
| 6. An edit-distance routine has the shape of one in the earlier gnosh (not Charm's) | **Stands**, unchanged: finding 2 below. |

What else is different in this run:

- **The tree.** 15 commits later it has 1,256 files where it had 1,173: 92
  added, 9 removed, 5 renamed, 144 changed. The larger arrivals are `bench/`
  (32 files carried from the earlier gnosh, with a table of their own in
  CARRIED.md, and 4 written fresh), commands on declared hosts
  (`internal/tools/shelltool/host.go`, `rules.go`, `internal/config/hosts.go`),
  the JSON config reader (`internal/config/jsonc.go`, `file.go`, `source.go`,
  `expand.go` in place of `rc.go` and `lex.go`), the question tool, skills
  and rewind in `internal/app`, the root `LICENSE.md`, and this directory.
- **Upstream moved.** Crush's `main` went from `7ef47d5c` to `ba19000e`
  (three commits), two branches and three tags are new, and nine other
  branches and the `nightly` tag moved. The first audit's corpora were
  rebuilt as they were, so that the numbers compare, and what Charm has
  published since is one more corpus, `since`.
- **The scripts.** `common.py` reads CARRIED.md's third table (the bench);
  `build_corpus.py` builds `since`; `configsurface.py` and `tabledump.go` are
  new and compare names (the section "The names of the configuration, the
  environment and the command line"); `thirdparty.py` also holds the root
  licence to gno's text; `report.py` can print the calibration table;
  `review.py` holds this run's verdicts. `taintscan.py` is unchanged.
- **The numbers.** New code went from 589 files to 631 (from 527 Go files
  of 100 tokens or more to 553), and the measures moved little. The line
  shares are where they were or a little lower. The files with a varied
  window of exact tokens are 3, 6 and 8 against `crush`, `hist` and `libs`,
  where they were 2, 5 and 7; those with a varied window of abstract tokens
  47, 69 and 54, where they were 43, 65 and 51. Two longest runs against
  `crush` grew, both in new test files, and both were read: the varied run
  of exact tokens from 41 tokens to 53 (the head of a table test), and the
  run with lists and tables from 268 to 321 (the rows of one).

## What was compared

| what | value |
|---|---|
| the new gnosh | this repository at commit `1a1e947a924f6c0f1c711f579dd8c330927bc434` (2026-11-14), 1,256 files, as `git archive` gives them. Files changed since and not committed were not looked at. |
| Crush at the fork point | `52c75da8acbcd11dc269ae582f3ed7947ba3cff6` (2026-10-18), the earlier gnosh's merge base with charmbracelet/crush `main` |
| Crush upstream, as the first audit had it | charmbracelet/crush `main` at `7ef47d5c85f3b2b55eee234b51ba5275d61bab4a` (2026-11-13), 206 commits after the fork point |
| Crush's whole history, as the first audit had it | every commit reachable from the 182 branches and 211 tags of charmbracelet/crush as fetched on 2026-11-13 |
| Crush since | charmbracelet/crush fetched again on 2026-11-14 at 08:42 UTC: `main` at `ba19000e13a0a81cdc7f3e09154fe602e1a73a76` (209 commits after the fork point), 184 branches and 214 tags. Everything those reach that the first fetch did not. |
| the earlier gnosh | github.com/gnosh-ai/gnosh `main` at `1c77d2bd58482c37dd858b24326a6e1829117904` |
| where it ran | the devbox, in `/root/src/gnosh-new-audit/`: the scripts (`scripts3/`), the copy of the tree (`tree-1a1e947/`), the later fetch (`crush-now.git`, which borrows the first clone's objects and leaves that clone as it was) and all raw working files, which hold Crush's code and stay there. `run.sh` was run from the start into an empty folder twice: a first pass (`work3-pass1/`), and, once the scripts and the verdicts of the hand review were final, again (`work3/`). The two gave the same `results.tsv` byte for byte (SHA-256 `7b04a644e297183c46996ac93601634a61e3c971a93385a419a1371fac0b44e5`). The first audit's folders (`work/`, `work2/`, and its follow-up scan, now `delta-13cea50/`) are beside them. |

One commit was added to this repository while the audit ran:
`1a77e144f5e099d8f64b358bcab42b3ab027c4cc`, which adds twelve lines of Jae's
own prompts to `AUTHORSHIP.md` and touches nothing else. Everything below,
and `results.tsv`, is of commit `1a1e947`, and line numbers are that
commit's. The line and word measures were then run on `AUTHORSHIP.md` as it
stands at `1a77e14`: none of its 59 significant lines stands in any corpus,
and it shares no sentence and no run of five words with any Crush text. It
changes nothing said here. The audit should be run again on the commit that
is published.

### The corpora: what the new tree was compared with

Each has a short name, which is the prefix of its columns in `results.tsv`.
The first three are the first audit's and came out of the rebuild with the
same sizes to the line.

| name | what it is | size |
|---|---|---|
| `crush` | (a) Crush's tree at the fork point, (b) upstream's tree at `7ef47d5c`, and (c) every file version that an upstream commit between the two brought in, which holds every line upstream added. This is the earlier inventory's "core" tier. | 2,498 text files, 1,831 of them Go (4.6 million tokens); 118,286 distinct lines |
| `hist` | `crush` plus every file version reachable from any upstream branch or tag at the first fetch: Crush's whole history, its unmerged branches, what Charm deleted before the fork. The inventory's "ext" tier. Crush's old history vendored other people's code (Go's `x` packages, the Anthropic and OpenAI SDKs, `mvdan.cc/sh`, Charm's own libraries), so this corpus is wider than "Charm's code". | 24,352 text files, 17,120 of them Go (46.2 million tokens); 969,045 distinct lines |
| `libs` | (d) the Charm libraries the earlier gnosh's `go.mod` pinned, at those versions, from the Go module cache: bubbletea 2.1.0, bubbles 2.2.1, lipgloss 2.0.6, glamour 2.0.1, fantasy 0.45.1, catwalk 0.52.49, fang 2.0.1, log 2.0.1, x/vcr 0.1.1, colorprofile 0.4.3, ultraviolet (2026-10-08), the openai-go fork (2026-09-21), and twelve modules of charmbracelet/x (ansi 0.11.9, term, termios, windows, editor, etag, powernap, exp/charmtone, golden, ordered, slice, strings) | 24 modules, 2,228 text files, 1,066 of them Go (1.7 million tokens) |
| `since` | new in this run: every text file version that upstream's branches and tags reach at the second fetch and that `hist` does not hold. A file is shown as `now:path@blob` when one of the three commits on `main` brought it in (it would be in `crush` had that corpus been built today), else as `nowhist:path@blob` (a branch or a tag). | 61 text files, 57 of them Go (0.5 million tokens); 8 from `main`, 53 from branches and tags; 16,754 distinct lines |
| `old` | not asked for: the earlier gnosh's own tree, less the files CARRIED.md lists as brought. Overlap here is not overlap with Charm; it shows what came from the earlier gnosh by a road other than CARRIED.md. | 1,733 text files, 1,199 of them Go (32 fewer files than at the first audit: the bench is carried now) |
| `deps` | not asked for: the other 166 modules the earlier gnosh required and the new `go.mod` does not (goldmark, chroma, uniseg, go-udiff, the model providers' SDKs and so on). The new gnosh does itself what several of them did, so the same question applies to them as to (d). | 20,733 text files, 10,947 of them Go (35.9 million tokens) |

### The files compared

Every file of the tree was scanned; the similarity question is asked of the
first group.

| group | files | what |
|---|---:|---|
| **new code** | 839 | everything CARRIED.md's tables do not list: 609 Go files, 17 Python and 5 shell scripts; 51 prompt texts (the 49 of `internal/prompt/texts` and the 2 built-in agent definitions); 105 files of test data; 45 documents (`spec/` and the root's); `go.mod`, `go.sum`, `.gitignore`, two JSON lists and two tables. The files CARRIED.md calls "written fresh" are here. So are the 19 files of this directory as the first audit left them (its 14 scripts are among the code, its README and two Markdown tables among the documents). |
| **carried** | 225 | CARRIED.md's rows of kind *carried* (193) and *carried with changes* (12) in its table of files, and the 20 bench files it calls *carried*: 91 Go and 19 Gno files, 13 scripts, 91 files of test data, 4 documents, 7 others |
| **carried with larger changes** | 19 | the 7 files of `internal/gnovm/tools` and the 12 bench files CARRIED.md calls *carried with changes* (9 Python scripts, `run.sh`, 2 documents) |
| **third-party** | 173 | gno's standard library (160 `.gno` files, 3 notices) and B+tree (3 files, 2 notices), Unicode's two test files and licence, the two Markdown specification example files |

The Go files that hold tables generated from other people's data
(`internal/text/tables.go`, `internal/markdown/entity.go`) are counted as new
code.

## The measures

All of them are made by the scripts beside this file; `run.sh` runs them in
order. The four measures of code and the measure of words are the first
audit's, unchanged.

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
- Because the generic filter leaves nothing of a standard-library file to
  measure: the same two without that filter (*rawshare*, and *rawrun*: a run
  counted when 4 of its lines are not imports).

**3. Exact tokens** and **4. identifier-insensitive tokens** (`tokscan.go`,
on Go's own scanner). Comments go, and so do the semicolons the scanner adds
at line ends, so line breaks do not matter.

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

One refinement is the main thing to understand about these two measures.
With string contents stripped, any list of twenty strings is the same forty
tokens as any other, and so is any table of like rows. Such lists dominate
the plain counts everywhere: a file of Go's `net/http` shares a run of 357
exact tokens with Crush, and one of ollama's 501. So each measure is given
twice: *with lists and tables*, as defined above, and *varied*, counting
only windows that hold at least 12 different tokens of their 40 (exact) or 9
of their 60 (abstract). The setting was chosen on the calibration sets, and
the table was made again in this run (`CALIBRATE=1`; the first three columns
came out as they were):

| a window is varied at (exact / abstract) | Crush files the earlier gnosh modified, with a varied window (of 291) | "short block" files with one (of 20) | control files with one (of 2,242); longest run | new code with one (of 553); longest run |
|---|---|---|---|---|
| 10 / 8 different tokens | 288 / 286 | 15 / 10 | 76 / 227; 117 / 223 | 31 / 69; 55 / 87 |
| **12 / 9** | 288 / 286 | 15 / 7 | 62 / 195; 117 / 223 | 3 / 47; 53 / 83 |
| 15 / 10 | 288 / 286 | 12 / 7 | 57 / 179; 117 / 223 | 3 / 32; 50 / 81 |
| 18 / 12 | 288 / 282 | 10 / 4 | 46 / 152; 88 / 117 | 3 / 13; 45 / 77 |

At every setting the new code stays inside the control set's range. 12 / 9 is
the highest at which the exact measure still sees every short-block file it
saw at 10.

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
brand words, environment variables, struct-tag keys, `go.mod`.

**The names of an interface** (`configsurface.py`, new in this run). A
config file, an environment and a command line are names, not code, and the
measures above say little about them. This one lists every name of gnosh's
configuration, of the environment it reads and sets, and of its command
line, and looks each up among the names of Crush's: the struct tags of its
config package, the properties of its `schema.json`, the words of its line
language, the fields of catwalk's provider and model types, the servers of
its `internal/discover`, the environment names in its Go code, and the
commands and flags of its `internal/cmd`, in every version of every file.
Two names are the same when they are equal once letter case, dashes,
underscores and a leading `--` or `$` are set aside (`base_url`,
`--base-url` and `baseUrl` are one name) and once the brand is folded as
the line scan folds it (`GNOSH_X` and `CRUSH_X` are one name). For a name
both have, it says who else has it: Claude Code's documented interface (the
name stands in this tree's `spec/cc`), the model providers' Go SDKs
(Anthropic's, Google's, and OpenAI's in Charm's fork), and unrelated Go code
(the generic-Go reference and the control programs).

### Calibration: what the measures read on files whose answer is known

Against `crush`. "Crush files" are from the earlier gnosh's tree: the 443 Go
files it never changed, the 299 it modified, the 28 added Go files the
inventory called REWRITE and the 20 it held back for one block of 4 to 7
shared lines. The control set is unrelated Go of a similar kind: three
packages of Go's standard library and four permissively licensed command-line
programs (fzf 0.75.0 and ollama 0.40.16, MIT; GitHub's `gh` 2.104.0, MIT;
`mvdan.cc/sh` 3.14.1, BSD), 2,432 files. Every number of this table came out
of this run as it came out of the first.

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
read 80 to 100% there. Against `since` it is none either: 57 Go files are
too few for the controls to have a range. For `hist` and for `since` the
yardstick is the control set's range against `crush`.

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
each of the 147 files that appear in a top-twenty list.

### New code against Charm's code

| measure | against `crush` (a to c) | against `hist` | against `libs` (d) | against `since` | control set's highest (vs `crush`; vs `libs`) |
|---|---|---|---|---|---|
| line share, 598 files with 10+ significant lines: median / 95th / highest | 0.0% / 1.2% / 10.0% | 0.0% / 2.2% / 10.0% | 0.0% / 0.9% / 7.4% | 0.0% / 0.0% / 0.8% | 17.6%; 12.0% |
| the same without the generic-Go filter: 95th / highest | 3.9% / 10.0% | 5.2% / 12.0% | 2.8% / 11.8% | 1.4% / 6.7% | 22.5%; 41.7% |
| files with a counted line run | 0 | 0 | 0 | 0 | 0; 1 file, 4 lines |
| files with any run of 4+ lines; longest | 4; 5 lines | 4; 5 | 0 | 1; 4 | 25 files, 14 lines; 8 files, 14 lines |
| exact tokens, varied: files with a shared window (of 553); longest run | 3; 53 tokens | 6; 53 | 8; 55 | 0 | 62 files, 117; 58 files, 62 |
| exact tokens, varied: highest cover of one file | 3.7% | 3.7% | 7.2% | 0 | 28.0%; 59.4% |
| abstract tokens, varied: files with a shared window; longest run | 47; 83 tokens | 69; 117 | 54; 121 | 1; 61 | 195 files, 223; 222 files, 125 |
| abstract tokens, varied: highest cover of one file | 47.2% | 48.2% | 16.2% | 12.2% | 69.4%; 97.5% |
| exact tokens with lists and tables: longest run; highest cover | 321; 99.5% | 1,770; 99.6% | 434; 99.6% | 0 | 501, 92.0%; 343, 91.8% |
| abstract tokens with lists and tables: longest run; highest cover | 339; 99.5% | 1,772; 99.8% | 434; 44.8% | 126; 25.2% | 502, 92.9%; 465, 98.8% |

No file of new code meets any of the inventory's line rules against any of
the four corpora, and none has a counted run at all. On every measure that
tells code from code (line share, line runs, varied token runs and cover) the
new code lies inside the control set's range.

**Files above the control set's highest.** Three, all on the measures that
include lists and tables, and all three as at the first audit. None has a
varied window of exact tokens against any Charm corpus; the first and the
third have one varied window of abstract tokens each (62 tokens against
`hist` and `libs`, 60 against `hist`), which is a window and a token or two
more:

| file | measure | what it is |
|---|---|---|
| `internal/markdown/entity.go` | exact and abstract cover with lists and tables, 99.5% to 99.8% against `crush`, `hist` and `libs` (control 91.8% to 92.9%) | A map of 253 names to strings, generated from Python's `html.entities` (regenerated byte for byte here). It matches any Go map of string pairs. Data, not Charm's. |
| `internal/text/tables.go` | exact and abstract run with lists and tables against `hist`, 1,770 tokens (control 501), and cover 93.4% to 94.3% | Packed strings joined with `+`, generated from the Unicode Character Database (regenerated byte for byte here). The match is `golang.org/x/text`'s tables, which Crush's history vendors: the same form, not the same contents. Data, not Charm's. |
| `internal/diff/lines_test.go` | exact run with lists and tables against `libs`, 434 tokens (control 343) | Rows of strings in a table test, lines 32 to 94, against rows of strings in a lipgloss test. A table of data. |

`internal/config/table.go`, the fourth such file at the first audit, is no
longer above: its table of environment variables is shorter.

**The top twenty of each measure** come to 413 matches in 147 files
(`review.tsv`, `review.md`). 309 of them stand exactly as they stood at the
first audit: the same measure, value and lines on both sides. They keep the
reading they were given then; for the 17 of them whose file changed
elsewhere, the lines were looked at again. The other 104 are new or have
moved, and all were read in this run: 47 pairs of line ranges side by side,
and the rest (single lines, and runs of lines each of which stands somewhere
in a corpus) on the new tree's side. What the 413 are:

- Lists and tables of literals: 46 files (lists of command names and of
  settings keys, sets written as `map[string]bool`, rows of table tests,
  seeds of fuzz tests, const blocks, struct literals filled field by field).
- Single lines scattered through a file: 53 files. The lines are JSON field
  tags of public wire formats (Anthropic's and OpenAI's APIs, Claude Code's
  hook input, stream output and question tool), HTTP header lines, and
  standard-library idiom (`filepath.EvalSymlinks(t.TempDir())`,
  `strings.Join(parts, "\n")`).
- Test scaffolding: the head and loop of a table-driven or fuzz test, an
  `httptest` handler's first lines, a helper that writes a file into a
  directory.
- A buffer behind a mutex with `Write` and `String` (five test files), and
  methods that take a lock, read or set one field, and return.
- Getters and setters in a row; a slice-equality loop; a slice reversal; a
  UUID written out with `hex.Encode`; the loop that reads a JSON decoder
  token by token; a switch that gives each signal its name.

The longest varied run of exact tokens that new code shares with `crush` is
53 tokens: the head of a table test and its first five rows
(`internal/tools/shelltool/host_test.go` 915 to 925, against Crush's
`internal/discover/discover_test.go` 204 to 214). Both declare a struct of
`input` and `want` and fill it with pairs of strings; one tests what a call
that names the local machine becomes, the other how a URL is trimmed. The
next two are the 41 tokens the first audit named: the first four lines of a
lock test (`internal/filelock/filelock_test.go` 125 to 128), forced by the
one function name the carried code calls, and the slice-equality loop of
`internal/memjoin/tasks.go`, now at lines 142 to 152. The longest against
`libs` is 55 tokens of table-test rows. The longest abstract run against
`libs`, 121 tokens, is still the six rows of a table test for the
`Retry-After` headers (`internal/llm/errors_test.go` 272 to 277, against
fantasy's `retry_test.go` 424 to 429). The longest run with lists and tables
against `crush`, 321 exact tokens, is new: the rows of a table test in
`internal/tools/shelltool/rules_test.go` (22 to 79), each a command and the
program it reaches, against rows of a name and a command in a test Crush
added after the fork point (`internal/agent/tools/safe_test.go` 141 to 209).
The rows share their shape and none of their contents.

Nothing read was expression that could only have come from Crush.

**Against what Charm published since (`since`).** No counted line run, a
line share of 0.8% at most (one line of a file), and no window of exact
tokens at all. One file has a varied window of abstract tokens,
61 tokens: the switch that names each signal (`internal/tools/shelltool/text.go`
60 to 76), against a switch that gives each tool a title, which the first
audit had already read against the fork-point version of the same Crush
file. The other matches there are struct literals filled field by field
against a new table of theme colours, single lines of idiom, and the
four-line loop that reads a JSON decoder token by token
(`internal/app/texts.go` 237 to 242), which the first audit listed against
`crush`.

**Against the dropped dependencies (`deps`).** The same background: no
counted line run, line share at most 12.9% (4 lines of 31), 16 files with a
varied exact window, the longest 55 tokens of table-test rows. The Unicode
tables were regenerated from Unicode's files, not taken from a width or
grapheme library.

**Against the earlier gnosh (`old`).** This is not a comparison with Charm.
Of new Go code, 8 files have a counted line run, all of them the files
written fresh under `internal/gnovm`, and all of them the 7 lines of that
subsystem's licence header; 11 files have a varied exact window, the longest
55 tokens; 75 have a varied abstract window, the longest 234 tokens (finding
2) and the next 94. Of the prompt texts, 20 of the 45 share a run of 12
words or more with the earlier tree, and CARRIED.md now says which and from
where. The root `LICENSE.md` shares 628 lines running with the earlier
gnosh's: the licence itself (see "The third-party files").

### The files that are new or were rewritten since the first audit

The same four measures, for the files the changes were in. "None" under
tokens means no varied window of 40 exact or 60 abstract tokens.

| files | lines: highest share against `crush` / `hist` / `libs`; counted runs | tokens against `crush`, `hist`, `libs`, `since` | what the matches are |
|---|---|---|---|
| hosts: `internal/tools/shelltool/host.go`, `rules.go`, `internal/config/hosts.go` | 1.0% / 1.0% / 0%; none | none | three single lines of `host.go` |
| their tests: `host_test.go`, `rules_test.go`, `standin_test.go`, `internal/config/hosts_test.go`, `internal/app/hosts_test.go` | 0.7% / 3.0% / 1.5%; none | exact: 53 tokens (`host_test.go`, against `crush` and `hist`); abstract: 79 (`host_test.go`, `rules_test.go`) | heads and rows of table tests, read above; JSON tags and test set-up lines |
| the config reader: `internal/config/jsonc.go`, `file.go`, `source.go`, `expand.go`, `build.go`, `load.go`, `options.go`, `table.go`, `doc.go` | 0% / 1.6% / 0.8%; none | exact: none; abstract: 63 (`table.go`: the list of Claude Code's keys that gnosh refuses, against lists of command names) | a list; three single lines of idiom |
| its tests: `jsonc_test.go`, `file_test.go`, `load_test.go` and the rest | 0.5% / 1.1% / 0.3%; none | exact: 55 against `libs` (`jsonc_test.go`: rows of three strings); abstract: 76 | rows of table tests, a list of config texts, a map of host names |
| all of `internal/config` that is not a test (25 files) | 1.9% / 2.8% / 0.9%; none | exact: none; abstract: 3 files, the longest 63 | a list, a const block of kinds, and eight lines that marshal, make a directory and write a file |
| `internal/ui/tool.go`, `where_test.go`, `question.go` | 0% / 1.3% / 0.3%; none | exact: 53 against `hist` (`tool.go` 120 to 124: reversing a slice in place, as at the first audit); abstract: none | idiom |
| `internal/app`: `askuser.go`, `more.go`, `rewind.go`, `skills.go`, `skill/skill.go` and their tests | 1.5% / 2.1% / 0.5%; none | exact: none; abstract: 61 and 60 against `libs` (two tests) | JSON tags of Claude Code's question tool; rows of a table test; a helper that writes a file |
| `internal/gnovm/tools/memtodos.go`, `memtodos_test.go` (carried, rewritten) | 0% / 0% / 0%; none | none | nothing |
| `bench/`, the 4 files written fresh | 0% / 0% / 0%; none | not Go | nothing; no sentence in common |
| `bench/`, the 32 carried files | 0% / 2.6% / 0%; none | not Go | one line each of `compare.py` and `table.py` and two of `tasks/rename/make.sh` stand in `hist`, as CARRIED.md says of them; one "sentence", a loopback address |
| `spec/DECISIONS.md` | 0% / 0% / 0%; none | not Go | no sentence and no run of 5 words in common |

### Prompt texts and other words, against Crush's

| what | units | sentences | sentences that also stand in `crush` / `hist` / `libs` / `since` | longest shared run of words |
|---|---:|---:|---|---|
| prompt texts (`internal/prompt/texts`, the two agent definitions) | 45 of 20+ words | 334 | 0 / 0 / 0 / 0 | 6 / 6 / 5 / 5 words |
| string literals of new Go code (tool descriptions, messages, tests) | 423 files | 16,534 | 27 / 47 / 26 / 2 | 13 / 21 / 21 / 6 |
| documents (`spec/`, the root's, this directory's) | 45 | 11,145 | 1 / 2 / 1 / 1 | 11 / 16 / 16 / 8 |
| test data (help texts, goldens) | 103 | 1,374 | 0 / 0 / 0 / 0 | 10 / 10 / 9 / 5 |
| for comparison: the earlier gnosh's prompt templates and tool descriptions (Crush's files as it changed them) | 47 | 2,117 | 787 | median 32, 95th 725 |
| for comparison: string literals of the control programs | 1,823 files | 45,702 | 767 against `crush` | 95th percentile 7 words or fewer in each program |

No prompt text shares a sentence with any Crush text or any string of Crush's
code, in any version. The shared "sentences" of the string literals were all
read: import paths, `127.0.0.1` addresses, a date layout, character classes,
the address of OpenAI's API, a UUID of zeros, "timed out waiting for %s". The
longest shared runs of words are a line of OpenAI's wire format in a test (13
words), a provider's error message quoted in a test, and the numbers 1 to
21. Against `hist` and `libs` there are also error texts of the operating
system and of Go's own packages, events of Anthropic's stream, and a few
phrases of five words that anyone writes: "the directory to search in", the
description of a search tool's argument, is the one that is not a test's. In
the documents: five words of a flag's description in
`spec/gnosh/harness-cli.md`, as before; and the root `LICENSE.md`, whose
legal phrases ("including but not limited to the implied warranties of
merchantability and fitness for a particular purpose", 16 words) also stand
in licences that Crush's history and Charm's libraries carry.

### The carried files

`carried_check.py`, for all 413 rows of CARRIED.md's three tables:

- **Bytes.** The 193 *carried* files, the 168 *third-party* files and the 20
  bench files called *carried* are byte for byte the earlier gnosh's files at
  their old paths. Of the 12 *carried with changes*, 10 differ from them in
  exactly one line each, and two in more, as CARRIED.md says: `GNOMEM.md` (5
  lines) and `internal/gnovm/LICENSE.md` (its closing notes). Of the 12 bench
  files *carried with changes*, two differ in one line and ten have lines
  added or left out. The one bench file *not brought* is not in the tree. The
  SHA-256 the tables record is that of the earlier gnosh's file in all 413
  rows.
- **Numbers.** The inventory's measures were taken again with its own script.
  Of the earlier gnosh's file against the inventory's own corpus they give
  the table's line share and longest run in all 406 rows that have numbers (7
  are binary test data). Of the file as it stands in this tree, against that
  corpus and against today's, they agree in 400 of 405; the five that differ
  are files changed by design: of the tools, `docs_test.go` 1.2% to 1.1%,
  `tools.go` 1.7% to 1.0%, `tools_test.go` 3.2% to 3.1%; of the bench,
  `compare.py` 2.7% to 2.6% and `table.py` 2.9% to 2.3%. `memtodos.go`, which
  read 11.1% at the first audit, reads the table's 0.0% again. No carried
  file has a counted run.
- **Tokens.** Carried Go files: none has a varied exact window against
  `crush`; 7 of 102 have a varied abstract window, the longest 74 tokens,
  inside the control range. Of the tools files, one has one varied window of
  exact tokens against `crush`: `tools_test.go` 37 to 41, four assertions on
  a tool's response (43 tokens), against a test that upstream added after
  the fork point. The file is as it was at the first audit, which did not
  name it; it is the earlier gnosh's own test, and the assertions are the
  usual ones on the two fields a tool result has.
- The tree's own `tools/audit/carried.py check` passes, for the table of
  files, the tools and the bench.

### The third-party files

`thirdparty.py`:

- gno's standard library: all 160 `.gno` files under `internal/gnovm/stdlibs`
  are byte for byte the files of `gnovm/stdlibs` in
  `github.com/gnolang/gno v1.5.1-0.20260928125126-5cdbc25fcde0`, the version
  `VERSION` names (the tree leaves out 94 of gno's 254). `LICENSE.md` is gno's
  `LICENSE.md` after a 320-byte preface; `LICENSE-GO` is Go's `LICENSE`.
- gno's B+tree: the three `.gno` files are byte for byte
  `examples/gno.land/p/nt/bptree/v0` of the same version.
- The root `LICENSE.md`, new since the first audit: from its heading "GNO
  NETWORK GENERAL PUBLIC LICENSE" it holds the whole of that text of gno's
  `LICENSE.md`, byte for byte, between what is gnosh's own (the notice and
  the Additional Terms before it, the closing notes after it). The same
  text stood in the earlier gnosh's `LICENSE.md`. It is NewTendermint's
  licence, not Charm's and not gnosh's writing; neither CARRIED.md nor
  PROVENANCE.md's table of data that is not gnosh's names it, and the file
  itself says whose it is.
- Unicode: the tree's generator, run again on files fetched from unicode.org
  and checked against the SHA-256 it records for each, writes
  `internal/text/tables.go` byte for byte; the two test files are Unicode's.
- Markdown: `commonmark-0.31.2.json` is byte for byte the file at
  spec.commonmark.org; the GFM page fetched today has the SHA-256 the notice
  records, and the tree's extractor run on it writes `gfm-0.29.json` byte for
  byte.
- `internal/markdown/entity.go`: the tree's generator writes it byte for
  byte.
- Against `crush`, `libs` and `since` no third-party file has a counted line
  run or a varied window of exact tokens. Against `hist`, `LICENSE-GO`
  shares its 23 lines (Go's licence text, which Crush's history also holds,
  as CARRIED.md says), and 9 of gno's standard-library files share long runs
  with Go packages that Crush's old history vendored (up to 1,311 varied
  exact tokens): Go's code in both, not Charm's.

### Names and whole files

- **git.** With Crush's three trees in one commit (the fork point, upstream
  at the first audit, upstream now) and the new tree added in the next,
  `git diff -C20% --find-copies-harder` pairs 2 of the 1,256 files with a
  Crush file: two empty lock files with an empty Crush file. It was 7 at
  the first audit; the five race-detector files are no longer paired. No
  other file reaches 20%.
- **Every commit.** Of the 1,915 blobs and trees in every commit of both
  branches of this repository, two are also objects of Crush's repository as
  fetched then or now (54,155 objects): the empty file and Go's BSD licence
  text.
- **File names.** 156 of the 707 Go files outside the third-party trees have
  a file name some Crush file also has (`bash_test.go`, `edit.go`,
  `config.go`). Compared pair by pair, the most any pair shares is 1.9% of
  its long lines. Of the 34 directories of new Go code under `internal/`, 6
  have the path of a Crush package (`agent`, `app`, `config`, `diff`,
  `hooks`, `permission`), and `ui` is the parent of Crush's interface
  packages.
- **go.mod.** No line of `go.mod` or `go.sum` names a Charm module; none of
  the 148 modules of the build list does; none of the 549 packages of the
  import graph, tests included, does; no Go file imports one.
- **Brand words.** Outside this directory and the documents, "crush" now
  stands in no new code at all (at the first audit it stood in a table of
  old file names that were still read). It stands in carried files:
  `bench/run.sh`, `analyze.py` and `matrix.sh`, which can run Charm's crush
  beside gnosh for a comparison, and `internal/gnovm/scenario_test.go`,
  `client.go` and `mempeers.go`, as CARRIED.md says. "charm" and
  "charmbracelet" stand only in documents, in this directory and
  `tools/audit/carried.py`, and in carried documents. No `CRUSH_`, `CHARM_`
  or `CATWALK_` environment variable is read by new code; carried code names
  `CRUSH_PROFILE` (`internal/gnovm/client.go`) and, for the runs of Charm's
  crush, `CRUSH_BIN` and two `CRUSH_DISABLE_` variables (`bench/run.sh`).
  Names that are Crush's with gnosh's name in place of crush's are in the
  next section.
- **Declared names.** 537 names that Crush, its history or a Charm library
  declares, and that no reference Go code declares, stand in the new tree
  (`idents.tsv` on the devbox); it was 569. In new code, from `crush`
  itself: 3 package names (`hyper`, `mcp`, `styles`, each used as an
  ordinary variable or field name), 9 types, 26 functions, 12 methods, 5
  constants, 24 variables and 43 fields: 122 names where there were 149.
  Gone are the to-do names of the first audit's finding 1, the two argument
  names of Crush's question tool, and 28 names of the config language
  (`flagKind`, `flagSpec`, `optionSpec`, `providerAdd`, `modelAdd` and the
  rest). Seven are new: `QuestionForm`, and six ordinary compounds
  (`TestModels`, `fillIn`, `inflight`, `lastCall`, `wantMeta`,
  `withConfig`). `QuestionForm` (`internal/app/askuser.go` 44) is the type
  that carries a model's questions to the front end; Crush's question dialog
  is a type of the same name (`internal/ui/dialog/question_form.go`). Its
  fields are Claude Code's argument names and nothing else of the two is
  alike. The rest fall as before into names of Claude Code's documented
  interface (`PermissionMode`, `PermissionRequest`, `ToolInput`,
  `ToolResponse`, `UpdatedInput`, `AdditionalContext`,
  `LastAssistantMessage`, `ActiveForm`), names of the model APIs
  (`ContextWindow`, `ExtraBody`, `MaxInputTokens`), the config names of
  finding 1 (`DiscoverModels`, `DisabledSkills`, `SmallModel`, `yolo`),
  `raceEnabled`, and ordinary compounds (`firstLines`, `tailLines`,
  `mustJSON`, `openPTY`, `NewRunner`).
- **Exported names, package by package.** For each package, the Charm package
  that declares the most of the same exported names. Control packages (169):
  median 9%, 95th percentile 23%. New code (27 packages): median 14%, and the
  same two stand out: `internal/term`, 87 of 170 names also in ultraviolet,
  and `internal/text`, 36 of 71 also in x/ansi (`api.tsv` on the devbox).
  Finding 4.

### The names of the configuration, the environment and the command line

This is the part of the tree that was rewritten to take another program's
interface out, so it was compared name by name (`configsurface.py`;
`config-surface.tsv`, `config-flags.tsv`, `config-idents.tsv`,
`config-brand.tsv`). The code is not in question: the 25 files of
`internal/config` that are not tests have no line run and no varied window
of exact tokens against any Crush corpus, a line share of 2.8% at most, and
of the 16 unexported names the first audit found there none is left.

**What gnosh's names are.** `config.Table()` lists 91 entries (88 names:
`model` and `smallModel` take the same three fields). They were read from
the source text and checked against the table the program itself prints
(`tabledump.go`): the two agree. Beside them the comparison took the
environment variables gnosh sets, the hook handler's fields, the permission
modes and the Claude Code keys gnosh names only to refuse them, which makes
192 entries from the tables, and every other word-like string of
`internal/config` that is not a test's, the state file's keys among them,
127 more. `config-surface.tsv` has a row for each.

**How many are also Crush's.** 51 of the 88 are a name of Crush's
configuration too, the same or respelled. Most are not Crush's to begin
with: 20 are Claude Code's documented keys, which Crush's hooks and
permissions follow as gnosh's do (`model`, `permissions` with `allow` and
`deny`, `hooks` and 12 of its 15 event names, `env`, `$schema`, and
`headers`, Claude Code's key for a server's request headers); two are the
providers' own variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`); nine are
words too plain to be anyone's (`type`, `name` twice, `id`, `description`,
`disabled`, `debug`, `models`, `providers`). On the command line, 22 of
gnosh's 67 flags are also flags of Crush's (`spec/cc` has 8 of the 22), and
16 of its 32 commands and subcommands have the name of one of Crush's. The
table below has the other 20 of the 51, those of the plain ones that need a
word, and every name outside `config.Table()` that the comparison turned
up.

| gnosh's name | Crush's name, and where it stands | who else has it | reading |
|---|---|---|---|
| `options`, the object that holds gnosh's own switches | `options`, the object that holds Crush's own switches in `crush.json` | opencode has `options` inside a provider; nothing found with it at the top for this | **Crush's layout.** A plain word, used for the same thing in the same place. |
| `providers`, `models` at the top of the file | the same two keys of `crush.json` | opencode's config has `providers`; `models` is everywhere | Common words. In gnosh `models` is a table by `<provider>/<id>`, which Crush's is not. |
| `providers.<id>.baseUrl`, `apiKey` | `base_url`, `api_key`; `--base-url`, `--api-key` | options of Anthropic's and OpenAI's SDKs (`BaseURL`, `APIKey`); opencode's `baseURL`, `apiKey` | Terms of the provider APIs. |
| `providers.<id>.extraBody` | `extra_body`; `--extra-body` | Google's SDK (`extraBody`, on the wire); OpenAI's Python SDK (`extra_body`) | A term of the provider SDKs, as the builder's table says. |
| `providers.<id>.discoverModels` | `discover_models`; `--discover-models` (a provider's field, at the fork point) | nothing found: not in `spec/cc`, the SDKs, the control programs, opencode's schema | **Particular to Crush.** |
| `providers.<id>.headers`, `disabled` | Crush has both words, for an MCP server; a provider's are `extra_headers` and `disable` | Claude Code's `.mcp.json` (`headers`); opencode (`headers`, `disabled`) | Common words, and not Crush's names for these two fields. |
| `models.<…>.contextWindow` | `context_window` (catwalk's model field); `--context-window` | the term of every provider's documentation | A term of the trade. As a key it is catwalk's two words in camelCase. |
| `models.<…>.maxOutputTokens` | `max_output_tokens` in catwalk and in Crush's reader of LiteLLM; Crush's own key for the limit is `default_max_tokens` | Google's API (`maxOutputTokens`), OpenAI's (`max_output_tokens`), Claude Code's `CLAUDE_CODE_MAX_OUTPUT_TOKENS` | A term of the provider APIs. |
| `model.thinking`, `temperature` | `thinking`, `--think`; `temperature` | Anthropic's API; every provider's API | Terms of the provider APIs. |
| provider types `llamacpp`, `ollama`, `lmstudio`, `litellm`, `omlx` | the five servers of Crush's `internal/discover`, and values of `type` in its `schema.json` | each is the server's own name | Each name is public. **The set of five is Crush's set.** |
| provider types `anthropic`, `openai` | two of catwalk's types | the companies' names | Plain. `openai-compat`, Crush's third, was respelled `openai-compatible` on purpose (decision 24). |
| `options.requestTimeout` | `request_timeout`; `request-timeout` (an option of Crush's, at the fork point) | an option of Anthropic's and OpenAI's Go SDKs (`WithRequestTimeout`); LiteLLM's `request_timeout` | A common term, and also Crush's option of that name for that thing. |
| `options.disabledSkills` | `disabled_skills`; `--disabled-skills` (an option of Crush's, at the fork point) | nothing found | **Particular to Crush.** Plain words for it, and the builder's table says so; they are also Crush's words. |
| `options.debug` | `debug` | Claude Code's `--debug` | Plain. |
| state file: `models.large`, `models.small` (`internal/config/state.go` 43 to 44, `load.go` 522) | `large` and `small`, the two model slots of `crush.json` and of the line language (`model large …`) | nothing found; catwalk has `default_large_model_id` and `default_small_model_id` | **Particular to Crush.** In `gnosh.json` the slots were renamed `model` and `smallModel`; the file gnosh writes for itself kept the old pair. |
| `$GNOSH_PROFILE` (read: serve a debugging page) | `CRUSH_PROFILE` (`main.go`, at the fork point, for the same thing) | nothing found | **Particular to Crush**, with the brand changed. |
| `$GNOSH_DATA_DIR` (read) | `CRUSH_DATA_DIR` (upstream added it after the fork point; at the fork point Crush had `CRUSH_GLOBAL_DATA`) | the pattern of Claude Code's `CLAUDE_CONFIG_DIR`, which `GNOSH_CONFIG_DIR` follows | A common pattern; the builder could not have taken it from Crush's later commit. |
| `GNOSH=1` in every command gnosh runs | `CRUSH=1` | Claude Code's `CLAUDECODE=1` | A pattern both follow (decision 6). |
| `AGENT=gnosh`, `AI_AGENT=gnosh` | `AGENT=crush`, `AI_AGENT=crush` | detector packages read `AI_AGENT`; `AGENT` is a proposal for all agents | A convention across tools, not Crush's own (looked up on the web, not measured). |
| `$GNOSH_EVENT`, `GNOSH_SESSION_ID`, `GNOSH_CWD`, `GNOSH_PROJECT_DIR`, `GNOSH_TOOL_NAME`, `GNOSH_TOOL_INPUT_COMMAND`, `GNOSH_TOOL_INPUT_FILE_PATH` (set for a hook) | `CRUSH_EVENT`, `CRUSH_SESSION_ID`, `CRUSH_CWD`, `CRUSH_PROJECT_DIR`, `CRUSH_TOOL_NAME`, `CRUSH_TOOL_INPUT_COMMAND`, `CRUSH_TOOL_INPUT_FILE_PATH`: what Crush's hook runner sets, at the fork point | Claude Code gives a hook `CLAUDE_PROJECT_DIR` and the rest as JSON on its input, not as variables | **Particular to Crush**: the scheme (a hook's input repeated as variables under the program's prefix) and these seven names. The other 13 `GNOSH_` variables a hook is given have no counterpart in Crush; the earlier gnosh added them in the same scheme. |
| `gnosh.json`, `.gnosh/`, `GNOSH.md`, `GNOSH.local.md`, `gnosh.log` | `crush.json`, `.crush/`, `CRUSH.md`, `CRUSH.local.md`, `crush.log` | `opencode.json`; Claude Code's `.claude/`, `CLAUDE.md`, `CLAUDE.local.md` | The program's own name in patterns many tools follow. |
| command line: `gnosh run`, with `-q` / `--quiet`, `-y` / `--yolo`, `--cwd`, `-D` / `--data-dir`, `-s` / `--session`, `--small-model`, `--reasoning-effort` | `crush run` and the same flags with the same letters (Crush's `--cwd` also has `-c`) | `run` is also opencode's command for the same thing, and `-y` / `--yolo` Gemini CLI's flag; `--quiet` and `--data-dir` are common flags; `reasoning_effort` is OpenAI's parameter. `spec/cc` has none of them. | **Crush's command line**, kept: `spec/gnosh/harness-cli.md` says Jae's scripts call it this way. |
| commands: `session list`, `show`, `last`, `delete`, `rename`; `logs` with `-f` / `--follow` and `-t` / `--tail`; `dirs`; `models`; `projects --json`; `stats --all --crawl-dir` | the same commands and flags of Crush's `internal/cmd`, at the fork point | `--follow` and `--tail` are every log reader's | **Crush's commands**, kept. `projects`, `stats` and `session delete` are in gnosh's table and not built yet. |
| command line: `-m` / `--model`, `-d` / `--debug`, `--continue` (`-C` is Crush's letter, `-c` Claude Code's; gnosh takes both), `--verbose`, `--output-format`, `-h`, `-v` | the same | Claude Code's documented flags | Claude Code's. |

Names that are near and not the same, each the result of a renaming:
`smallModel` (Crush: the slot `small`, the flag `--small-model`),
`models.<…>.thinkingLevels` (`reasoning_levels`), `options.projectDataDir`
(`data_directory`), `options.maxBackgroundTasks` (`max_background_jobs`),
`openai-compatible` (`openai-compat`).

**The old language.** Of the 48 flags and 22 dashed option names that stand
in Crush's package for its line language, the new `internal/config` holds one
flag (`--reasoning-effort`, as the name of a command-line flag in a message)
and no option name; it held 41 and 16. A file named `gnoshrc` is not read.
`internal/config/load_test.go` (128 to 130) writes three lines of the old
language into such files to show that they are ignored, which is the one
place new code holds a line of it.

**Go names in `internal/config`.** Of the 335 unexported names the package's
code declares and the 105 its tests declare, 36 and 14 are also declared
somewhere in Crush's config packages (`internal/config`,
`internal/shellconfig`, `internal/env`, `internal/discover`, in any
version). All but two are names unrelated Go code declares as well (`cwd`,
`env`, `files`, `headers`, `key`, `kind`, `model`, `models`, `name`, `opts`,
`path`, `provider`, `providers`, `readFile`, `source`, `url`, `value` and
the like). The two: `hookEvents`, the table of hook events
(`table.go` 230), which is also the name of a variable in a file upstream
added after the fork point; and `providerType`, a type here
(`table.go` 168) and a struct field there. Both are the obvious name. Of
the exported names, eight are not declared by unrelated Go: `AllowedTools`
and `PermissionMode` (Claude Code's), `ContextWindow`, `ExtraBody`,
`DiscoverModels`, `DisabledSkills`, `SmallModel` (the names of the table
above, as Go spells them) and `Limits`.

## Findings, most serious first

None of these is a passage of Charm's code in new code. They are the places
where this tree and Charm's work, or this tree and its own record, meet.

**1. Names of Crush's design are still part of gnosh's interface.** The
table above has them; the ones that are particular to Crush, by this
auditor's reading, are:

- in `gnosh.json`: `discoverModels` and `disabledSkills`; the `options`
  object as the home of the program's own switches, with `debug`,
  `requestTimeout` and `disabledSkills` in it as they are in Crush's; and
  the five local server types as a set;
- in the state file gnosh writes: the model slots `large` and `small`;
- in the environment: `GNOSH_PROFILE`, and the variables a hook is given
  (seven names that are Crush's with the prefix changed, and the scheme of
  the other thirteen);
- on the command line: `gnosh run` and its `-q`, `-y` / `--yolo`, `--cwd`,
  `-D` / `--data-dir`, `-s` / `--session` and `--small-model`; and the
  commands `session`, `logs`, `dirs`, `models`, `projects` and `stats` with
  their flags.

How they came here is on the record and involves no reading of Crush's code
by a builder: `spec/gnosh/harness-config.md` (CFG-12, CFG-13),
`harness-hooks.md` (HOOK-30) and `harness-cli.md` wrote down what the
earlier gnosh's interface was, the earlier gnosh's interface was in these
parts Crush's, and the builders worked from the specs. When the line
language was replaced, most of
its names were replaced with it (`extra_headers`, `disable`,
`default_max_tokens`, `reasoning_levels`, `data_directory`,
`max_background_jobs`, the slots, `openai-compat`); the ones above were
carried into camelCase. Decision 17 of `spec/DECISIONS.md` says that "every
name taken from it" goes, which is therefore not quite so; and the builder's
own table gives "plain words" as the source of `discoverModels`,
`disabledSkills`, `requestTimeout` and `GNOSH_PROFILE`, which is true of
what the builder knew and not of where the names come from. The hook
variables and the command line were never part of that decision: decision
10 keeps them ("Hook variables are `GNOSH_*`"), and the command line is kept
because Jae's scripts call it, without either saying that the design is
Crush's.

These are names, a file's layout, and letters of flags. No code behind them
matches Crush's on any measure. Whether an interface that follows another
program's matters is not something this audit can say. *Needs a human look*:
for each group, either a new name, or a line in `spec/DECISIONS.md` that
says it is kept, that it was Crush's, and why (as decision 16 did for the
line language while it was kept). The cheapest to change are the ones
nothing of Jae's depends on yet: `discoverModels` and `disabledSkills`
(the bench's runner writes the first into the config it makes), and the
state file's two slots, which gnosh alone writes. `GNOSH_PROFILE`, the hook
variables and the command line are what his wrapper, his hooks and his
scripts use (`harness-config.md` section 1, `harness-cli.md`; for the hooks
the spec marks it as unconfirmed, OQ-21).

**2. One function has the shape of one in the earlier gnosh (not Charm's).**
Unchanged since the first audit, where it was finding 6, and nothing in the
record speaks of it yet. `internal/cli/suggest.go`, lines 22 to 43: an
edit-distance routine. With identifiers set aside it is the same 234 tokens
as the routine at lines 162 to 183 of the earlier gnosh's
`internal/ui/dialog/slash.go`, the next highest such match against the
earlier tree being 94. That file is the earlier gnosh's own (added
2026-10-24); the inventory held it back for four lines elsewhere in it and
offered a copy with those lines cut. The routine is the textbook
optimal-string-alignment distance on three rolling rows, and the names
inside differ; it stands in no version of Crush and in no Charm library.
Whether a builder had the excised copy open or wrote the textbook routine
again, the measures cannot say. If it was taken from the copy, CARRIED.md
should say so. *Needs a human look.*

**3. Three records are behind the code.** None is Charm's text; each says
something that is no longer so.

- `spec/gnosh/harness-config.md` still specifies the `gnoshrc` line language
  in full, with the table of flags it took from the earlier program, and
  `spec/cc/settings.md`, `permissions.md`, `hooks.md` and `memory-files.md`
  still speak of `gnoshrc`. Decision 17 replaced all of it, and the specs do
  not say so where they stand. A reader of `spec/` alone would take gnosh to
  read that language. The specs are kept as evidence of what the build side
  was given, so the remedy is a line at the head of each, not a rewrite.
- `spec/DECISIONS.md` 17 and the "from" column of `internal/config/table.go`
  and `options.go`, as said under finding 1.
- `internal/permission/protect.go` (line 38) and `doc.go` (line 171) still
  count `gnoshrc` and `.gnoshrc` among gnosh's own config files to protect,
  and their test does. Harmless, and stale. (This is the committed tree; the
  working tree had uncommitted changes in that package, which were not
  looked at.)

*Needs a human look*, of the smaller kind.

**4. Names shared with Charm's terminal libraries: one is left.** The three
names the first audit pointed at were renamed. `Options.EscTimeout`
(`internal/term/terminal.go` 29, used in `terminal_unix.go` 123) is still
the name of the field ultraviolet has for the same setting
(`terminal_reader.go`), and no reference Go code declares it; the default it
replaces, now `DefaultEscWait`, still has ultraviolet's value of fifty
milliseconds. The rest is as the first audit said and is the common
vocabulary of the trade: 87 of `internal/term`'s 170 exported names are also
ultraviolet's, where 76 are also tcell's, an unrelated terminal library; 36
of `internal/text`'s 71 are also x/ansi's, 19 also termenv's; 7 of the 28
names of `internal/term/seq.go` are also x/ansi's (`CursorUp`, `CursorDown`,
`EraseLine`, `HideCursor`, `ShowCursor`, `SaveCursor`, `RestoreCursor`). No
code of either package matches a Charm library. *For a person to decide*
whether the one field should change with its three fellows.

Also seen, and not findings:

- The front end's task type (`internal/app/feature.go` 361 to 376: `Task`
  with `Content`, `ActiveForm` and `Status`, and the three status words) has
  the field names and the words of Claude Code's `TodoWrite`, which
  `spec/cc/tools.md` gives, and which Crush's to-do type has too. It was
  there at the first audit. Read beside Crush's declaration it is not the
  same lines: another type name, other constant names, plain string
  constants, another order of fields, no tags.
- `go.mod` and `go.sum` share lines with Crush's (41% and 35% of their
  lines; a run of 30 lines in `go.sum`): the same modules at the same
  versions have the same checksums.
- The files written fresh under `internal/gnovm` share a run of 7 lines with
  the earlier gnosh: the subsystem's own licence header.
- The bench can run Charm's crush and OpenAI's Codex beside gnosh, so three
  of its carried files hold what that takes: the one line of config and the
  command line `bench/run.sh` gives crush, and the names of two tables of
  its session database in `analyze.py`. CARRIED.md says so ("What in the
  bench is not the earlier gnosh's own"). No line run and no sentence of
  the bench stands in any Crush corpus.
- One string of a test, the three words "You are gnosh." repeated to make
  a long system prompt (`internal/agent/stress_test.go` 106), stands in a
  test of Crush's with Crush's name in place of gnosh's. It is how every
  such text begins.

## What needs a human look, in one place

| where in this tree | where in the other | what |
|---|---|---|
| `internal/config/table.go` 104 and 188 to 195; `options.go` 110, 140; `state.go` 43 to 44 and `load.go` 522; `table.go` 327 | Crush `internal/config/config.go` 147, 480, 481, 56 to 57 (fork point); `internal/discover/`; `main.go` 16 | `discoverModels`, `disabledSkills`, `requestTimeout`, the five local server types, the slots `large` and `small`, `GNOSH_PROFILE`: Crush's names, respelled (finding 1) |
| `internal/config/table.go` 330 to 338; `internal/hooks/input.go` 325 to 381; `spec/gnosh/harness-hooks.md` HOOK-30; `spec/DECISIONS.md` 10 | Crush `internal/hooks/` (fork point) | seven hook variables that are Crush's with the prefix changed, and the scheme (finding 1) |
| `internal/cli/table.go` 103 to 316 (`run`, `--yolo`, `--cwd`, `--data-dir`, `--quiet`, `--session`, `--small-model`; `session`, `logs`, `dirs`, `models`, `projects`, `stats`); `spec/gnosh/harness-cli.md` | Crush `internal/cmd/root.go`, `run.go`, `session.go`, `logs.go`, `dirs.go`, `models.go`, `projects.go`, `stats.go` (fork point) | Crush's command line, kept (finding 1) |
| `spec/DECISIONS.md` 17; the "from" of the entries above in `internal/config/table.go` and `options.go` | | they say the names were all replaced, or come from plain words (findings 1 and 3) |
| `internal/cli/suggest.go` 22 to 43; CARRIED.md | the earlier gnosh's `internal/ui/dialog/slash.go` 162 to 183 | the same routine with other names; not Charm's; not on record (finding 2) |
| `spec/gnosh/harness-config.md`; `spec/cc/settings.md`, `permissions.md`, `hooks.md`, `memory-files.md` | | they specify or mention a config language that decision 17 replaced (finding 3) |
| `internal/permission/protect.go` 38, `doc.go` 171, `path_test.go` 257 to 258 | | `gnoshrc` still protected as a config file (finding 3) |
| `internal/term/terminal.go` 29; `terminal_unix.go` 123 | ultraviolet `terminal_reader.go` | the field `EscTimeout` (finding 4) |

## What was not checked, and what the measures cannot show

- **Ideas and structure.** Every measure of code here finds shared text or
  shared token sequences. A file written while looking at a Crush file, with
  every line recast, shares neither. So does a design that follows Crush's
  without a line in common. The identifier-insensitive measure sees renamed
  copies of 60 tokens and more; it does not see a reordered or restructured
  one. The comparison of names sees an interface that follows Crush's in its
  names; it does not see one that follows it in behaviour under new names.
- **Small blocks.** A copied block under 40 tokens (exact) or 60 (abstract)
  is below the token windows, and the line measures see it only if its lines
  were left unchanged and are long enough to count. The calibration says how
  often: of twenty files known to hold one block of 4 to 7 Crush lines, lines
  see 18, exact tokens 15, abstract tokens 7.
- **Memory.** Whether a builder (a language model) reproduced Charm's code
  from what it had seen in training cannot be tested except through its
  output, which is what was measured.
- **Claude Code.** There is no corpus of Claude Code's source or prompt text
  to compare with. The tree follows Claude Code's documented interface; that
  it does so "in its own words" was not checked. "Claude Code's documented
  key" above means that the name stands in this tree's `spec/cc`, which the
  spec side wrote from Claude Code's documentation; the documentation itself
  was not read again.
- **Who else has a name.** For the table of names, three things were
  measured: `spec/cc`, three providers' Go SDKs, and the control programs
  with the generic-Go reference. Beyond those, opencode's published config
  schema and page of commands and Gemini CLI's published list of flags were
  read on the web on 14 November, and what is said of OpenAI's Python SDK,
  of LiteLLM and of the `AGENT` and `AI_AGENT` convention rests on a web
  search and on general knowledge. No other harness was compared, so
  "nothing found" means nothing in those, not nothing anywhere.
- **Other interfaces.** Slash commands, key bindings, tool names, the words
  of the screen and the shape of the session files were not compared by
  name with Crush's. The first audit found none of Crush's tool names in new
  code, and the declared names say the same now; the others were not looked
  at.
- **Other programs.** Only Crush, Charm's libraries and the earlier gnosh's
  other dependencies were compared as code.
- **Upstream after the second fetch.** `since` is what Charm had published
  by 08:42 UTC on 14 November 2026.
- **The working tree.** The commit was audited. Files changed and not
  committed (at the time, `internal/permission`) were not.
- **Shell and Python.** The 22 scripts of new code and the 23 carried ones
  have line and word measures only.
- **The generic-Go filter** drops lines that Go's own source and a dozen
  common modules also have, as the inventory's did; the unfiltered columns are
  beside the filtered ones.
- **The hand review** was one reader's. Its sorting of matches is by simple
  rules on the new tree's own lines (`review.py`). Of the 413 matches, 104
  were read in this run and 309 keep the first audit's reading because
  nothing about them changed; none was read twice by two readers.

## What is in this directory

| file | what |
|---|---|
| `README.md` | this |
| `results.tsv` | one row per file of the tree (1,256), 113 columns. `sig_lines`, `long_lines`, `tokens`, `words`, `sentences` are the file's own sizes. Then for each corpus prefix (`crush`, `hist`, `libs`, `since`, `old`, `deps`): `_share`, `_run`, `_run_lines`, `_lrun` (longest run of lines each somewhere in the corpus), `_rawshare`, `_rawrun`, `_rawrun_lines`; `_tok_run`, `_tok_cov` (exact tokens with lists and tables: longest run in one corpus file, tokens covered), `_tok_vrun`, `_tok_vcov` (the same, varied windows only); `_abs_run`, `_abs_cov`, `_abs_vrun`, `_abs_vcov` (identifier-insensitive); `_word_run`, `_sent_shared`. The last four columns name the Crush or library file behind the best line share and the longest varied runs, with line ranges. A Crush file is named `tier:path@blob`: `base` the fork-point tree, `up` brought in by an upstream commit before the first audit, `hist` the rest of the history, `now` and `nowhist` the same two of what was published since. The `group` column is `new`, `carried`, `carried-tools`, `carried-bench` or `third-party`. |
| `tables.md` | every distribution (each set of files against `crush`, `hist`, `libs` and `since`, for lines, tokens and words), the control range, and the twenty highest files of each measure |
| `review.md`, `review.tsv` | the hand review: one line per file, and one row per match with both line ranges |
| `config-surface.tsv` | one row per name of gnosh's configuration and environment (319): what it is, where the builder's table says it comes from, whether Crush's configuration has it (the same, respelled, or the same but for the brand), under which names, of which kind and in which versions, and who else has it |
| `config-flags.tsv` | the same for the commands and flags of the command line |
| `config-idents.tsv` | the names `internal/config` declares that Crush's config packages also declare, with whether unrelated Go declares them |
| `config-brand.tsv` | names in the tree's Go strings, and file names, that stand in Crush with its own name in place of gnosh's |
| `run.sh` | runs the whole audit; its head says what it needs |
| `common.py` | settings; reads CARRIED.md's three tables to group the files |
| `taintscan.py` | the earlier inventory's line scan, unchanged (SHA-256 `37a5364f764c3d068b5b28687916a36f8b5d2e8db2d98bef7848695b1590a3cd`) |
| `build_corpus.py` | gathers the corpora and the lists of files |
| `linescan.py` | measures 1 and 2 |
| `tokscan.go` | measures 3 and 4; also lists string literals |
| `textscan.py` | the word measures |
| `idents.go`, `names.py` | names and whole files |
| `configsurface.py`, `tabledump.go` | the names of the configuration, the environment and the command line; `tabledump.go` prints `config.Table()` from a scratch copy of the package, so that the reading of the source text can be checked against it |
| `thirdparty.py`, `carried_check.py` | the third-party and carried files |
| `report.py`, `review.py`, `peek.py` | the tables (and, with `calibrate`, the table of settings above); the review list and its verdicts; a viewer that prints both sides of a match on the terminal |

To run it again, on a machine that may hold Crush's source and never inside
this working tree:

    CRUSH_REPO=/path/to/clone NEW_TREE=/path/to/copy WORK=/path/to/work tools/audit/similarity/run.sh

with `SINCE_REPO=/path/to/later/fetch UPSTREAM_NOW=commit` for the corpus
`since` (the head of `run.sh` says how to make that fetch without touching
the first clone), and `CALIBRATE=1` for the table of settings. The three Go
files carry `//go:build ignore`, so they are not part of the module's build.

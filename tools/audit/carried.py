#!/usr/bin/env python3
"""Writes and checks the tables of carried files in CARRIED.md.

Run from the repository root, with Python 3 and nothing else.

    tools/audit/carried.py check
        Needs only this repository. For every row of the table of files it
        hashes the file in the tree, first undoing the edits this script
        lists for that file, and compares the result with the SHA-256 of the
        staged original that the row records. It also checks that the
        carried directories hold nothing but the files in the table and the
        files listed as written fresh, and that the summary table adds up.
        For the tools in internal/gnovm/tools, which were made from their
        staged files with changes too large to undo, it checks what can be
        checked without the originals; "The tools" below says what. The
        files of bench have a rule of their own; "The bench" below says it.
        It exits non-zero and says why if anything differs.

    tools/audit/carried.py write --staged DIR --scan FILE
        Needs the inventory's staging folder: DIR is its files/ directory and
        FILE its scan.tsv. It compares every file in the carried directories
        with its staged original, requires each difference to be exactly the
        edits listed below, and rewrites the tables of CARRIED.md, each
        between its pair of marker lines. The rest of CARRIED.md is written
        by hand. For the tools it checks the record below against the staged
        originals and the scan, and writes the list of their strings. It
        then runs the check.

    tools/audit/carried.py tools --staged DIR --scan FILE
        The part of "write" that is about the tools, alone: of the staging
        folder it reads only DIR/internal/gnovm/tools and the scan's rows
        for those files, and of CARRIED.md it rewrites only the tools'
        table. It then runs the check.

    tools/audit/carried.py bench --staged DIR --scan FILE
        The part of "write" that is about the bench, alone: of the staging
        folder it reads only DIR/bench and the scan's rows for those files,
        and of CARRIED.md it rewrites only the bench's table. It then runs
        the check.

The table of files has one row per carried file: its path here, its path in
the earlier gnosh, its kind, the size of the staged original, the scan's two
measures for it (the share of its distinct significant lines that also stand
in Charm's code, and the longest run of consecutive lines the two share),
the SHA-256 of the staged original, and what was changed.

The tools. The seven files of internal/gnovm/tools (the gnovm_run, gnovm_new,
gnovm_list and gnovm_drop tools and their tests) were written against the
earlier harness's tool interface. Each file here was made from the staged
file of the same name by replacing every call into that harness, so no list
of one-line edits gives the original back, and the files have a table of
their own in CARRIED.md. What "check" holds them to:

  - the directory holds exactly the files this script lists for it, each
    made from a staged file or written fresh, and every staged file of the
    directory is the source of one;
  - the table in CARRIED.md is the one this script writes from its own
    record of the staged originals (size, SHA-256, the scan's numbers), so
    neither can be changed without the other;
  - every string the staged tools.go and memtodos.go held (what a model
    is told about the tools, their argument names, every result and error
    text) is still, byte for byte, a string of the files here that are not
    tests, but for the few this script lists as dropped, with the reason.
    The strings are kept in tools/audit/gnovm-tools-strings.json, which
    "write" and "tools" make from the staged files.

"write" and "tools" check more, since they have the originals: that each
staged file has the size and SHA-256 recorded here and the scan's numbers,
and that every string of the staged tools.go and memtodos.go is either kept
or listed as dropped. What none of the three can check is that the code
around the strings does what the staged code did; the package's tests are
for that.

The bench. The staged files of bench (the harness bench: its metering proxy,
its runners, four tasks, scoring scripts and the earlier results) came in
three ways, and each has a row in a table of its own in CARRIED.md:

  - carried: byte for byte. "check" hashes the file and compares it with
    the SHA-256 of the staged original, as for the table of files.
  - carried with changes: the staged file with lines replaced, added or left
    out, so that it drives this repository's gnosh and not the earlier
    one. The lines that were replaced are not kept anywhere, this script
    included: some of them named the earlier program's files, and one block
    quoted words of other programs' prompts. So "check" cannot undo the
    changes. It holds each such file to the lines that were kept instead:
    tools/audit/bench-kept.json lists, for each file, the runs of
    consecutive lines of the staged original that stand in the file here
    (where each begins in the original, how many lines it is, and the
    SHA-256 of its bytes), and "check" finds every run in the file, byte
    for byte and in order. A changed line of the original is thus seen, and
    a line added between two runs is not: the lines that are new are this
    repository's and are read in the file itself.
  - not brought: staged, and left behind. "check" fails if it appears.

"check" also holds bench to exactly those files and the ones this script
lists as written fresh, and the table in CARRIED.md to this script's
record. "write" and "bench", which have the originals, check that record
against them (size, SHA-256, the scan's verdict and numbers), that a
carried file is its original, and make the list of kept runs by comparing
each changed file with its original, line by line.
"""

import argparse
import csv
import difflib
import hashlib
import json
import os
import sys

CARRIED_MD = "CARRIED.md"
FILES_BEGIN = "<!-- carried-table:begin (written by tools/audit/carried.py; do not edit by hand) -->"
FILES_END = "<!-- carried-table:end -->"
SUMMARY_BEGIN = "<!-- carried-summary:begin (written by tools/audit/carried.py; do not edit by hand) -->"
SUMMARY_END = "<!-- carried-summary:end -->"
TOOLS_BEGIN = "<!-- carried-tools:begin (written by tools/audit/carried.py; do not edit by hand) -->"
TOOLS_END = "<!-- carried-tools:end -->"
BENCH_BEGIN = "<!-- carried-bench:begin (written by tools/audit/carried.py; do not edit by hand) -->"
BENCH_END = "<!-- carried-bench:end -->"

# The directories that hold carried files. Each carried file keeps the path
# it had in the earlier gnosh.
ROOTS = [
    "gno/r/gnosh/mem",
    "internal/gnochain",
    "internal/gnovm",
    "internal/swarm/cmd/swarmdemo",
]

# Carried files outside those directories, each at the path it had.
SINGLE_FILES = [
    "GNOMEM.md",
]

# Staged files under those directories that are not rows of the table of
# files: the tools, which were brought with larger changes and are checked
# on their own (TOOLS below).
LEFT_BEHIND = ["internal/gnovm/tools/"]

# Files in those directories that were not carried but written for this
# repository. CARRIED.md says why each exists.
FRESH = [
    "internal/gnovm/racedetector_off_test.go",
    "internal/gnovm/racedetector_on_test.go",
    "internal/gnovm/sqlite.go",
    "internal/gnovm/sqlite_test.go",
]

# The summary table's rows: a path prefix and what is under it. A file
# counts under the first prefix it starts with.
GROUPS = [
    ("GNOMEM.md", "mem's own documentation, at the repository root: a carried test reads it"),
    ("gno/r/gnosh/mem/", "the shared mem realm for gno.land"),
    ("internal/gnochain/", "the gno.land client"),
    ("internal/gnovm/internal/syncstdlibs/", "the tool that copies gno's standard library in"),
    ("internal/gnovm/klashlibs/fmt/", "the VMs' fmt library"),
    ("internal/gnovm/klashlibs/klash/gnomem/v2/", "gnomem v2, mem's realm in the local VM"),
    ("internal/gnovm/klashlibs/klash/bptree/v0/", "gno's B+tree package, with its LICENSE.md and SOURCE"),
    ("internal/gnovm/stdlibs/", "gno's standard library, with its LICENSE.md, LICENSE-GO and VERSION"),
    ("internal/gnovm/testdata/", "mem's scenarios, goldens and upgrade fixtures"),
    ("internal/gnovm/", "the GnoVM machines and mem: the package's own files, DESIGN.md and LICENSE.md"),
    ("internal/swarm/cmd/swarmdemo/", "the swarm demo"),
]

KINDS = ["carried", "carried with changes", "third-party"]

# The edits made to carried files: for each file, the lines replaced. Each
# is (the original's text, the text here, what the table says of it). The
# original's text stands exactly once in the original and the new text
# exactly once in the file here, so an edit can be undone to get the
# original back, byte for byte.
LOCK = (
    '\t"github.com/charmbracelet/crush/internal/lock"\n',
    '\tlock "github.com/gnosh-ai/gnosh/internal/filelock"\n',
    "import path, 1 line: the earlier harness's `internal/lock` is now `internal/filelock`, imported under the name `lock` so no call changed",
)
EDITS = {
    "internal/gnovm/client.go": [LOCK],
    "internal/gnovm/host.go": [LOCK],
    "internal/gnovm/memhost.go": [LOCK],
    "internal/gnovm/memhost_test.go": [LOCK],
    "internal/gnovm/mempeers_host_test.go": [LOCK],
    "internal/gnovm/turns.go": [LOCK],
    "internal/gnovm/worker.go": [(
        '\t"github.com/charmbracelet/crush/internal/version"\n',
        '\tversion "github.com/gnosh-ai/gnosh/internal/buildinfo"\n',
        "import path, 1 line: the earlier harness's `internal/version` is now `internal/buildinfo`, imported under the name `version` so no call changed",
    )],
    "internal/gnovm/realworker_test.go": [(
        "//\tID=-X=github.com/charmbracelet/crush/internal/version.BuildID=gnovm-it\n",
        "//\tID=-X=github.com/gnosh-ai/gnosh/internal/buildinfo.BuildID=gnovm-it\n",
        "build instruction in a comment, 1 line: the linker flag names `internal/buildinfo.BuildID` under the new module path",
    )],
    "internal/gnochain/msg.go": [(
        '\t"github.com/charmbracelet/crush/internal/gnochain",\n',
        '\t"github.com/gnosh-ai/gnosh/internal/gnochain",\n',
        "import path, 1 line: the package's own path as it is registered with amino, which refuses a type whose package path differs",
    )],
    "internal/swarm/cmd/swarmdemo/main.go": [(
        '\t"github.com/charmbracelet/crush/internal/gnochain"\n',
        '\t"github.com/gnosh-ai/gnosh/internal/gnochain"\n',
        "import path, 1 line",
    )],
}


# The tools: internal/gnovm/tools. For each file made from a staged file,
# the staged file it was made from (the same path), that original's size and
# SHA-256, the scan's verdict for it, how many symbols of the earlier harness
# it used (the scan's n_crush_syms), the scan's two measures (ext_sigd_pct
# and ext_srun_longest), and what the file is now.
TOOLS_DIR = "internal/gnovm/tools"
TOOLS = {
    "internal/gnovm/tools/closecheck_test.go": (
        5714, "3ac02e4070c3bcb544e598f79d87d37f37487b3f7be7f41d7a41e0a271d7ed2c", "CARRY", 0, "0.0", "0",
        "the same test, on this harness's tool interface"),
    "internal/gnovm/tools/docs_test.go": (
        32330, "81eef8d1b8885aff0281ff77897b102ae2a0f9da21fca1d7a946bc7e1d0c4635", "ADAPT", 1, "1.2", "0",
        "the same tests for GNOMEM.md; those of the earlier README and of the earlier harness's goal file are left out"),
    "internal/gnovm/tools/memtodos.go": (
        2618, "fb57a815625b4c138ba695831f1bb82afe5e1dd58d1f0d5916a8ba7b5d2742ed", "ADAPT", 8, "0.0", "0",
        "the same mapping of a goal to a task list, onto a task-list type of its own"),
    "internal/gnovm/tools/memtodos_test.go": (
        2557, "6005bda8ef20fb1ebdec2f94545bcbb6777a8bb2c7c58a1327df0b20fdada54a", "ADAPT", 11, "0.0", "0",
        "the same test, on that type"),
    "internal/gnovm/tools/namer_test.go": (
        1724, "1615ddf3667a042b56b172730584f3f5f1611acc3b0b645aa6b437e462581359", "CARRY", 0, "0.0", "0",
        "the same test"),
    "internal/gnovm/tools/tools.go": (
        28001, "d4383a347d7da6d8db3a30b5ec0ba7b271f2fc141a8a43d3ae80f991bbef73ed", "ADAPT", 11, "1.7", "0",
        "the four tools as core.Tool values; the permission prompt is gone from it"),
    "internal/gnovm/tools/tools_test.go": (
        4986, "ec451fe095b039bb559fd7fe2e36585f784ad8d85819fd6706754324741749d0", "ADAPT", 2, "3.2", "0",
        "the same tests, on this harness's tool interface"),
}

# Files of that directory written for this repository. CARRIED.md says why
# each exists.
TOOLS_FRESH = [
    "internal/gnovm/tools/check_test.go",
    "internal/gnovm/tools/options.go",
    "internal/gnovm/tools/params.go",
    "internal/gnovm/tools/run_test.go",
]

# The staged program files of the tools, whose strings are what a model and
# a person read of them, and where the kept ones are listed.
TOOLS_PROGRAM = ["internal/gnovm/tools/tools.go", "internal/gnovm/tools/memtodos.go"]
TOOLS_STRINGS = "tools/audit/gnovm-tools-strings.json"

# Strings of the staged program files that are not in the files here, and
# why. Everything else those files held as a string is still here.
TOOLS_DROPPED = {
    "mem's goal could not be shown as the session's todos":
        "a log line of memtodos.go, for a task list that could not be written: the list's setter now tells the person itself, and this package logs nowhere",
    "error":
        "the key of that log line's one attribute",
    'json:"frames,omitempty"':
        "the tag of the one field of tools.go's metadata struct: the frames are kept under the same key, \"frames\", in the result's Meta (MetaFrames), which needs no struct",
}


# The bench: bench. For each staged file, at the path it had: the staged
# original's size and SHA-256, the scan's verdict and its two measures
# (ext_sigd_pct and ext_srun_longest), how the file came (CARRIED, CHANGED or
# LEFT), and what was changed, or why it was left.
BENCH_DIR = "bench"
BENCH_KEPT = "tools/audit/bench-kept.json"
CARRIED, CHANGED, LEFT = "carried", "carried with changes", "not brought"
SESSION_FILES = "gnosh's session files are read where the earlier gnosh's session database was"
BENCH = {
    "bench/.gitignore": (13, "32dae3052f331ee34d628ef535709b301259a45df7c7522c4d35dcf49873f00b", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/README.md": (4309, "8b17df23c8a106cf14c0cd83c96134b9e8c6b8c5ec2421eb341d658404f4c5f6", "CARRY", "0.0", "0", CHANGED,
                        "written again for this repository; the staged file's account of the pieces, of the swarm and of the other harnesses stands where it still holds"),
    "bench/RESULTS.md": (7672, "c8f967bc21632d5511dd0ee749b1c9b4d93f2bcae97a193bddf76b5271f78413", "CARRY", "0.0", "0", CHANGED,
                         "a label added under the title: whose numbers these are, and when and how each part was measured. Every line of the original stands"),
    "bench/analyze.py": (4909, "f82af2ca999faa93a245ec1bd8e5690b911ce7721e4e2f4e65ebc88c5b8e35eb", "CARRY", "0.0", "0", CHANGED,
                         "gnosh's session files are read where the earlier gnosh's session database and its log were; the readers of crush's database and of Codex's files stand as they were"),
    "bench/cachetest.py": (2962, "8533768aee01a354c691b2c13b67a2a697ef13dad751f235ee0197a12a396bdd", "CARRY", "0.0", "0", CHANGED,
                           "1 line: the reminder it appends is this gnosh's, in this gnosh's tag, where it was the earlier one's"),
    "bench/compare.py": (3282, "cdb47a1d5a5fecc768a7be140bafdfb62f2927a162f92204ccc0f93327bacf22", "CARRY", "2.7", "0", CHANGED,
                         "runs that a cap stopped are counted apart from runs that failed"),
    "bench/matrix.sh": (1129, "5f9bca7dce23aef69ee1b616c7a1a65a4111dff80502bcf4a9488d3f757b47a9", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/memerrors.py": (2680, "271d7b56b88838df33ed24b1f02e6a48e9475f14e62710e926c8e299befa645c", "CARRY", "0.0", "0", CHANGED, SESSION_FILES),
    "bench/prefixdiff.py": (1526, "1e02bd9f1877b865965e9e3440ec7da3920cfb7e7bb5b62f9c7b63835da01a97", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/proxy.py": (21803, "f37d66139c6198dd6f5953dde2c5e0e7e92e813000da320f49dc991f054deb37", "CARRY", "0.0", "0", CHANGED,
                       "how a summary request is told: by the first words of this gnosh's own, or by a pattern given from outside, where the staged file held words of the earlier gnosh's, crush's and Codex's prompts; and the comment that went with it. Nothing else"),
    "bench/reads.py": (1449, "f31fc0ce573be9c96427c10863a173f333d5a531d4164d25ea87dddc0e22dbad", "CARRY", "0.0", "0", CHANGED,
                       "1 line: a summary request is told by the proxy's own mark alone, and no longer by two words of the earlier prompts as well"),
    "bench/rescore.py": (1485, "65732919f8813d229869972bf5ff01e2e0929d826faba76fda3a08ebd93d596c", "CARRY", "0.0", "0", CHANGED,
                         "the fields it copies are the ones analyze.py now gives, and what gnosh printed of each session is added"),
    "bench/reviewab.py": (6526, "0c69311446d3ed6b281e3ad39de884e0ea3f5f2a04508e8f822934ba70d27ec8", "CARRY", "0.0", "0", LEFT,
                          "it replayed the earlier gnosh's close reviews by rebuilding their request by hand from its session database; this gnosh words that request differently, and nothing in the scoring of the four tasks uses it"),
    "bench/rounds.py": (1006, "1c202ccd1bea0ef3afa1c6ed007929ede224e6f72cee285d3b29ff3fb9b33d46", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/run.sh": (11927, "766ff2f8eb534b70a838d2e650e706436828db536fdd903c8089848e00c32467", "CARRY", "0.0", "0", CHANGED,
                     "the gnosh branch drives this gnosh: its command line, its config, its home; the switches a run can be given; what the server says of its window; the row's new fields. crush's and Codex's branches stand but for the cap"),
    "bench/streams.py": (1365, "4e2d00502cdacd62981c43d4a84dc80856d8c7c2b1c79002545e678c28587692", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/table.py": (3060, "2e47280b1681b57d5957f70330234d0745d06933b774974f11bee56b4088aaf2", "CARRY", "2.9", "0", CHANGED,
                       "every harness in the rows gets a line, gnosh's ways of running first, where three names were listed; runs that a cap stopped are counted and listed apart"),
    "bench/tasks/README.md": (1955, "0a45b21385f2b487c501e64e6686b16ac4d15462aec61bd4d462f8527f6b1e8d", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/algos/check.sh": (1137, "ba46db79b6d30503838ac622a86dd432081c9445b099f6b4cf0e73346162fb15", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/algos/make.sh": (26646, "b2ae63e5a12b7114b17999f65d0e2d22e9b907577afffb935dc02d36c1702cc0", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/algos/prompt.txt": (349, "9e94adaaf17442b92c90e93de56c6a2a8a96dafa25507c801c2b22cf62238cee", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/bugs/check.sh": (1350, "c7427b27e3af5195b781ecfa54f0221f212790c453e757b6808502c1d7d7fc18", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/bugs/make.sh": (12580, "b9a2d1445517ae695afdbb9566aad32c6cbc127efde623720b4aa3033132e8aa", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/bugs/prompt.txt": (158, "feb98a09f7a2e2d13cc9e390a6f736214cc6e327bbca0c8f3c395ecedd972ae7", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/continue/check.sh": (3962, "b0de861a8a913518d60a9706c9759cc592d6a1ef317e56db38943261c3bf560d", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/continue/make.sh": (5516, "47d65f47c8029b5817085e186584d5854d6d01338cda1dd1359c28a1cfd4f093", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/continue/prompt.txt": (1298, "2159d6e1ec5fcc5341cb1689e9e6db33053f34958e4f524b76c5db7faa8993d0", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/continue/prompt2.txt": (42, "a033a2186168c7059bb9f2e3ed7f5f650a1e4f38396434429427a1e2dce37942", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/rename/check.sh": (1968, "505f6710e9f4b3df27ee231c73dd0cd4fb8bdfa2bc17c3a5061da461f3276bc8", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/rename/make.sh": (9877, "9c7b95202853a4b881af7ecf6f4315fe8d7decf1e5b26ac949ea3f99eb31d354", "CARRY", "1.4", "0", CARRIED, "nothing"),
    "bench/tasks/rename/prompt.txt": (532, "30dc4b6d4e149b6250931905827ceeb2ef3951d4d4677f95638a5e5be60de646", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/tasks/selftest.sh": (802, "8bb0a17377940391a3cf2dfcb905199f99b3c876d3e193dfe201a3401308dcc5", "CARRY", "0.0", "0", CARRIED, "nothing"),
    "bench/waitplan.py": (2100, "18c1b727544b009785ebae7e3a6f72332c3bab207b0e232cfd21a3a8710fdcf7", "CARRY", "0.0", "0", CHANGED, SESSION_FILES),
}

# Files of bench written for this repository. CARRIED.md says why each
# exists.
BENCH_FRESH = [
    "bench/dryrun.py",
    "bench/dryrun.sh",
    "bench/serverfacts.py",
    "bench/sessions.py",
]


def fail(msg):
    print("carried.py: " + msg, file=sys.stderr)
    sys.exit(1)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def read(path):
    with open(path, "rb") as f:
        return f.read()


def tree_files():
    """Every file under the carried directories, and the single files, by
    path; but for the tools, which tools_files lists."""
    out = []
    for path in SINGLE_FILES:
        if not os.path.isfile(path):
            fail("no file %s: run from the repository root" % path)
        out.append(path)
    for root in ROOTS:
        if not os.path.isdir(root):
            fail("no directory %s: run from the repository root" % root)
        for dirpath, _, names in os.walk(root):
            for name in names:
                path = os.path.join(dirpath, name).replace(os.sep, "/")
                if name != ".DS_Store" and not path.startswith(TOOLS_DIR + "/"):  # .DS_Store is the Finder's, never committed
                    out.append(path)
    return sorted(out)


def tools_files():
    """Every file of the tools' directory, by path."""
    out = []
    for dirpath, _, names in os.walk(TOOLS_DIR):
        for name in names:
            if name != ".DS_Store":
                out.append(os.path.join(dirpath, name).replace(os.sep, "/"))
    return sorted(out)


def go_strings(src, path):
    """The values of the string literals of Go source, in order: those in
    double quotes with their escapes read, and those in backquotes (a struct
    tag is one). Comments, rune literals and import declarations are passed
    over."""
    out = []
    i, n = 0, len(src)

    def quoted(i):
        """Reads the string in double quotes that starts at i."""
        simple = {"a": "\a", "b": "\b", "f": "\f", "n": "\n", "r": "\r", "t": "\t", "v": "\v", "\\": "\\", '"': '"', "'": "'"}
        val = bytearray()
        i += 1
        while i < n and src[i] != '"':
            c = src[i]
            if c == "\n":
                fail("%s: a string in double quotes runs past the end of its line" % path)
            if c != "\\":
                val += c.encode()
                i += 1
                continue
            e = src[i + 1]
            if e in simple:
                val += simple[e].encode()
                i += 2
            elif e == "x":
                val.append(int(src[i + 2 : i + 4], 16))
                i += 4
            elif e == "u":
                val += chr(int(src[i + 2 : i + 6], 16)).encode()
                i += 6
            elif e == "U":
                val += chr(int(src[i + 2 : i + 10], 16)).encode()
                i += 10
            elif e in "01234567":
                val.append(int(src[i + 1 : i + 4], 8))
                i += 4
            else:
                fail("%s: an escape this script does not read: \\%s" % (path, e))
        return val.decode("utf-8", "surrogateescape"), i + 1

    def raw(i):
        """Reads the string in backquotes that starts at i."""
        j = src.index("`", i + 1)
        return src[i + 1 : j].replace("\r", ""), j + 1

    def skip(i):
        """Passes over a comment, a rune literal or a string that starts at
        i, and returns where it ends; i itself if none starts there."""
        if src.startswith("//", i):
            j = src.find("\n", i)
            return n if j < 0 else j
        if src.startswith("/*", i):
            return src.index("*/", i) + 2
        if src[i] == "'":
            j = i + 1
            while src[j] != "'":
                j += 2 if src[j] == "\\" else 1
            return j + 1
        if src[i] == '"':
            return quoted(i)[1]
        if src[i] == "`":
            return raw(i)[1]
        return i

    while i < n:
        c = src[i]
        if c == '"':
            val, i = quoted(i)
            out.append(val)
        elif c == "`":
            val, i = raw(i)
            out.append(val)
        elif c.isalpha() or c == "_":
            j = i
            while j < n and (src[j].isalnum() or src[j] == "_"):
                j += 1
            word, i = src[i:j], j
            if word != "import":
                continue
            # An import declaration: one path, or a list in parentheses.
            while i < n and src[i] in " \t":
                i += 1
            if i < n and src[i] == "(":
                while src[i] != ")":
                    j = skip(i)
                    i = j if j > i else i + 1
                i += 1
            else:
                while i < n and src[i] != "\n":
                    j = skip(i)
                    i = j if j > i else i + 1
        else:
            j = skip(i)
            i = j if j > i else i + 1
    return out


def tools_strings_here():
    """The strings of the tools' files here that are not tests."""
    here = set()
    for path in tools_files():
        if path.endswith(".go") and not path.endswith("_test.go"):
            here.update(go_strings(read(path).decode(), path))
    return here


def tools_table():
    """The table of the tools in CARRIED.md, from this script's record."""
    out = [
        "| file | made from the staged | the scan's verdict | symbols of the earlier harness it used | bytes | line share | longest run | SHA-256 of the staged original | what it is now |",
        "|---|---|---|---:|---:|---:|---:|---|---|",
    ]
    for path in sorted(TOOLS):
        size, digest, verdict, syms, share, run, now = TOOLS[path]
        out.append("| `%s` | `%s` | %s | %d | %s | %s%% | %s | `%s` | %s |" % (
            path, path, verdict, syms, format(size, ","), share, run, digest, now))
    return out


def check_tools():
    """Holds the tools' directory to its record; returns how many of its
    files were made from staged files, how many are fresh, and how many
    strings of the staged ones stand in them."""
    in_dir = set(tools_files())
    for path in TOOLS:
        if path not in in_dir:
            fail("%s is recorded as made from a staged file but is not in the tree" % path)
    extra = sorted(in_dir - set(TOOLS) - set(TOOLS_FRESH))
    if extra:
        fail("in %s but neither made from a staged file nor listed as fresh: %s" % (TOOLS_DIR, ", ".join(extra)))
    text = read(CARRIED_MD).decode()
    for path in TOOLS_FRESH:
        if path not in in_dir:
            fail("%s is listed as fresh but is not in the tree" % path)
        if "`" + path + "`" not in text:
            fail("%s is fresh but %s does not say why it exists" % (path, CARRIED_MD))
    lines = text.split("\n")
    a, b = block(lines, TOOLS_BEGIN, TOOLS_END)
    if [l for l in lines[a + 1 : b] if l] != tools_table():
        fail("the table of the tools in %s is not the one this script's record gives" % CARRIED_MD)
    if not os.path.isfile(TOOLS_STRINGS):
        fail("no file %s: it lists the strings of the staged tools" % TOOLS_STRINGS)
    kept = json.loads(read(TOOLS_STRINGS).decode())
    if not kept or kept != sorted(set(kept)) or any(s in TOOLS_DROPPED for s in kept):
        fail("%s is not a sorted list of strings, each once, none of them listed as dropped" % TOOLS_STRINGS)
    here = tools_strings_here()
    gone = [s for s in kept if s not in here]
    if gone:
        fail("strings of the staged tools that no longer stand in %s, byte for byte: %s" % (TOOLS_DIR, "; ".join(repr(s) for s in gone)))
    return len(TOOLS), len(TOOLS_FRESH), len(kept)


def bench_files():
    """Every file of the bench's directory, by path. What python leaves
    beside a script it ran is passed over: it is never committed."""
    out = []
    for dirpath, dirs, names in os.walk(BENCH_DIR):
        dirs[:] = [d for d in dirs if d != "__pycache__"]
        for name in names:
            if name != ".DS_Store":
                out.append(os.path.join(dirpath, name).replace(os.sep, "/"))
    return sorted(out)


def bench_kept(orig, here):
    """What of a staged original stands in the file made from it: the runs
    of consecutive lines the two share, in order, as [the run's first line
    in the original, its length in lines, the SHA-256 of its bytes], and how
    many of the original's lines that are not blank there are, and how many
    of those stand."""
    a, b = orig.split(b"\n"), here.split(b"\n")
    blocks, kept = [], 0
    for m in difflib.SequenceMatcher(None, a, b, autojunk=False).get_matching_blocks():
        if m.size:
            run = a[m.a : m.a + m.size]
            blocks.append([m.a + 1, m.size, sha256(b"\n".join(run))])
            kept += sum(1 for line in run if line.strip())
    return {"lines": sum(1 for line in a if line.strip()), "kept": kept, "blocks": blocks}


def bench_stands(path, record):
    """Finds every kept run of a staged original in the file here, in order,
    and returns how many lines that are not blank they hold."""
    lines = read(path).split(b"\n")
    pos, kept = 0, 0
    for first, size, digest in record["blocks"]:
        at = next((i for i in range(pos, len(lines) - size + 1) if sha256(b"\n".join(lines[i : i + size])) == digest), None)
        if at is None:
            which = "line %d" % first if size == 1 else "lines %d to %d" % (first, first + size - 1)
            fail("%s: %s of its staged original no longer stand in it, byte for byte and after the lines before them" % (path, which))
        kept += sum(1 for line in lines[at : at + size] if line.strip())
        pos = at + size
    return kept


def bench_table(kept):
    """The table of the bench in CARRIED.md, from this script's record and
    the list of kept runs."""
    out = [
        "| path, as it was and as it is | how it came | bytes | line share | longest run | SHA-256 of the staged original | lines kept | what was changed, or why it was left |",
        "|---|---|---:|---:|---:|---|---:|---|",
    ]
    count, size_of, lines, stand = {}, {}, 0, 0
    for path in sorted(BENCH):
        size, digest, _, share, run, how, what = BENCH[path]
        count[how] = count.get(how, 0) + 1
        size_of[how] = size_of.get(how, 0) + size
        cell = {CARRIED: "all", LEFT: "none"}.get(how)
        if how == CHANGED:
            cell = "%d of %d" % (kept[path]["kept"], kept[path]["lines"])
            lines += kept[path]["lines"]
            stand += kept[path]["kept"]
        out.append("| `%s` | %s | %s | %s%% | %s | `%s` | %s | %s |" % (path, how, format(size, ","), share, run, digest, cell, what))
    out.append("| **all** | %s | **%s** | | | | %d of %d in the changed files | |" % (
        ", ".join("%d %s (%s bytes)" % (count[h], h, format(size_of[h], ",")) for h in (CARRIED, CHANGED, LEFT) if h in count),
        format(sum(v[0] for v in BENCH.values()), ","), stand, lines))
    return out


def check_bench():
    """Holds the bench's directory to its record; returns how many of its
    staged files are here byte for byte, how many with changes, how many of
    the changed files' lines stand and of how many, and how many files are
    fresh."""
    in_dir = set(bench_files())
    here = set(p for p, v in BENCH.items() if v[5] != LEFT)
    for path in sorted(BENCH):
        if (path in here) != (path in in_dir):
            fail("%s is recorded as %s, and the tree says otherwise" % (path, BENCH[path][5]))
    extra = sorted(in_dir - here - set(BENCH_FRESH))
    if extra:
        fail("in %s but neither a staged file nor listed as fresh: %s" % (BENCH_DIR, ", ".join(extra)))
    text = read(CARRIED_MD).decode()
    for path in BENCH_FRESH:
        if path not in in_dir:
            fail("%s is listed as fresh but is not in the tree" % path)
        if "`" + path + "`" not in text:
            fail("%s is fresh but %s does not say why it exists" % (path, CARRIED_MD))
    if not os.path.isfile(BENCH_KEPT):
        fail("no file %s: it lists the lines kept of the bench's changed files" % BENCH_KEPT)
    kept = json.loads(read(BENCH_KEPT).decode())
    changed = sorted(p for p, v in BENCH.items() if v[5] == CHANGED)
    if sorted(kept) != changed:
        fail("%s does not list exactly the bench's changed files" % BENCH_KEPT)
    carried, lines, stand = 0, 0, 0
    for path in sorted(here):
        size, digest, _, _, _, how, _ = BENCH[path]
        data = read(path)
        if how == CARRIED:
            if sha256(data) != digest or len(data) != size:
                fail("%s is not its staged original, byte for byte" % path)
            carried += 1
            continue
        if sha256(data) == digest:
            fail("%s is its staged original, byte for byte: record it as carried" % path)
        if bench_stands(path, kept[path]) != kept[path]["kept"]:
            fail("%s: the count of its kept lines is not the one %s gives" % (path, BENCH_KEPT))
        lines += kept[path]["lines"]
        stand += kept[path]["kept"]
    md = text.split("\n")
    a, b = block(md, BENCH_BEGIN, BENCH_END)
    if [l for l in md[a + 1 : b] if l] != bench_table(kept):
        fail("the table of the bench in %s is not the one this script's record gives" % CARRIED_MD)
    return carried, len(changed), stand, lines, len(BENCH) - len(here), len(BENCH_FRESH)


def original_of(path, here):
    """The staged original of a file, from its bytes here: its edits undone."""
    data = here
    for old, new, _ in EDITS.get(path, []):
        old_b, new_b = old.encode(), new.encode()
        if data.count(new_b) != 1:
            fail("%s: the edited line is not there exactly once: %r" % (path, new))
        data = data.replace(new_b, old_b)
    return data


def changed_cell(path):
    edits = EDITS.get(path, [])
    return "; ".join(note for _, _, note in edits) if edits else "nothing"


def block(lines, begin, end):
    """Where the block between two marker lines starts and ends in lines."""
    try:
        return lines.index(begin), lines.index(end)
    except ValueError:
        fail("%s lacks the marker lines %s and %s" % (CARRIED_MD, begin, end))


def file_rows():
    """The rows of the table of files in CARRIED.md: (path, old path, kind,
    size, digest, what changed) each."""
    lines = read(CARRIED_MD).decode().split("\n")
    a, b = block(lines, FILES_BEGIN, FILES_END)
    rows = []
    for line in lines[a + 1 : b]:
        if not line.startswith("| `"):
            continue
        cells = [c.strip() for c in line.strip().strip("|").split(" | ")]
        if len(cells) != 8:
            fail("a row does not have 8 cells: %r" % cells)
        path, old_path, kind, size, _, _, digest, changed = cells
        rows.append((path.strip("`"), old_path.strip("`"), kind, int(size.replace(",", "")), digest.strip("`"), changed))
    if not rows:
        fail("the table of files in %s is empty" % CARRIED_MD)
    return rows


def summary(rows):
    """The summary table for rows of (path, kind, size)."""
    counts = [[0, 0, {}] for _ in GROUPS]
    for path, kind, size in rows:
        for i, (prefix, _) in enumerate(GROUPS):
            if path.startswith(prefix):
                counts[i][0] += 1
                counts[i][1] += size
                counts[i][2][kind] = counts[i][2].get(kind, 0) + 1
                break
        else:
            fail("%s belongs to no row of the summary" % path)
    out = ["| what | files | bytes | of which |", "|---|---:|---:|---|"]
    for (prefix, what), (n, size, kinds) in zip(GROUPS, counts):
        of_which = ", ".join("%d %s" % (kinds[k], k) for k in KINDS if k in kinds)
        out.append("| `%s`: %s | %d | %s | %s |" % (prefix, what, n, format(size, ","), of_which))
    total = {}
    for _, kind, size in rows:
        n, s = total.get(kind, (0, 0))
        total[kind] = (n + 1, s + size)
    of_which = ", ".join("%d %s (%s bytes)" % (total[k][0], k, format(total[k][1], ",")) for k in KINDS if k in total)
    out.append("| **all** | **%d** | **%s** | %s |" % (len(rows), format(sum(s for _, _, s in rows), ","), of_which))
    return out


def check():
    rows = file_rows()
    listed = set()
    for path, old_path, kind, size, digest, changed in rows:
        if path != old_path:
            fail("%s: carried files keep their paths, but the row says %s" % (path, old_path))
        if path in listed:
            fail("%s is listed twice" % path)
        listed.add(path)
        if not os.path.isfile(path):
            fail("%s is in the table but not in the tree" % path)
        here = read(path)
        orig = original_of(path, here)
        if sha256(orig) != digest:
            fail("%s is not its staged original with the listed edits" % path)
        if len(orig) != size:
            fail("%s: the size in the table is not the original's" % path)
        if kind not in KINDS:
            fail("%s: unknown kind %s" % (path, kind))
        if (kind == "carried with changes") != (path in EDITS):
            fail("%s: its kind, %s, does not fit the edits listed for it" % (path, kind))
        if kind == "third-party" and here != orig:
            fail("%s is third-party and may not be changed" % path)
        if changed != changed_cell(path):
            fail("%s: the table's account of what changed is not this script's" % path)
    for path in EDITS:
        if path not in listed:
            fail("%s has edits listed here but no row in the table" % path)
    in_tree = set(tree_files())
    extra = sorted(in_tree - listed - set(FRESH))
    if extra:
        fail("in the carried directories but neither carried nor listed as fresh: " + ", ".join(extra))
    text = read(CARRIED_MD).decode()
    for path in FRESH:
        if path not in in_tree:
            fail("%s is listed as fresh but is not in the tree" % path)
        if "`" + path + "`" not in text:
            fail("%s is fresh but %s does not say why it exists" % (path, CARRIED_MD))
    lines = text.split("\n")
    a, b = block(lines, SUMMARY_BEGIN, SUMMARY_END)
    want = summary([(path, kind, size) for path, _, kind, size, _, _ in rows])
    if [l for l in lines[a + 1 : b] if l] != want:
        fail("the summary table in %s does not add up to the table of files" % CARRIED_MD)
    made, fresh, strings = check_tools()
    bench = check_bench()
    print("carried.py: ok: %d carried files are their staged originals, but for the listed edits to %d of them; "
          "%d fresh files beside them" % (len(rows), len(EDITS), len(FRESH)))
    print("carried.py: ok: %s holds the %d files made from its staged files and %d fresh ones, as recorded; "
          "all %d strings kept from the staged tools stand in them" % (TOOLS_DIR, made, fresh, strings))
    print("carried.py: ok: %s holds %d of its staged files byte for byte and %d with changes, in which %d of the "
          "originals' %d lines stand in order; %d staged file was not brought; %d fresh files beside them"
          % ((BENCH_DIR,) + bench))


def write(staged, scan_path):
    with open(scan_path, newline="") as f:
        scan = {r["path"]: r for r in csv.DictReader(f, delimiter="\t")}
    files = [p for p in tree_files() if p not in FRESH]
    staged_here = set(SINGLE_FILES)
    for root in ROOTS:
        for dirpath, _, names in os.walk(os.path.join(staged, root)):
            for name in names:
                rel = os.path.relpath(os.path.join(dirpath, name), staged).replace(os.sep, "/")
                if name != ".DS_Store" and not any(rel.startswith(p) for p in LEFT_BEHIND):
                    staged_here.add(rel)
    if staged_here != set(files):
        fail("the tree and the staging folder differ: only staged %s; only here %s"
             % (sorted(staged_here - set(files)), sorted(set(files) - staged_here)))

    table = [
        "| new path | old path | kind | bytes | line share | longest run | SHA-256 of the staged original | what was changed |",
        "|---|---|---|---:|---:|---:|---|---|",
    ]
    rows = []
    for path in files:
        orig = read(os.path.join(staged, path))
        here = read(path)
        if original_of(path, here) != orig:
            fail("%s differs from its staged original by more than the listed edits" % path)
        for old, _, _ in EDITS.get(path, []):
            if orig.count(old.encode()) != 1:
                fail("%s: the original line is not in the original exactly once: %r" % (path, old))
        row = scan.get(path)
        if row is None:
            fail("%s is not in the scan" % path)
        if int(row["bytes"]) != len(orig):
            fail("%s: the scan's size is not the staged file's" % path)
        if row["verdict"] not in ("CARRY", "ADAPT", "THIRD-PARTY"):
            fail("%s: the scan's verdict is %s" % (path, row["verdict"]))
        if row["verdict"] == "THIRD-PARTY":
            kind = "third-party"
            if here != orig:
                fail("%s is third-party and may not be changed" % path)
        elif path in EDITS:
            kind = "carried with changes"
        else:
            kind = "carried"
        if row["kind"] == "binary":
            share = run = "n/a (binary)"
        else:
            share, run = row["ext_sigd_pct"] + "%", row["ext_srun_longest"]
        table.append("| `%s` | `%s` | %s | %s | %s | %s | `%s` | %s |" % (
            path, path, kind, format(len(orig), ","), share, run, sha256(orig), changed_cell(path)))
        rows.append((path, kind, len(orig)))

    lines = read(CARRIED_MD).decode().split("\n")
    a, b = block(lines, FILES_BEGIN, FILES_END)
    lines[a + 1 : b] = [""] + table + [""]
    a, b = block(lines, SUMMARY_BEGIN, SUMMARY_END)
    lines[a + 1 : b] = [""] + summary(rows) + [""]
    with open(CARRIED_MD, "w", newline="\n") as f:
        f.write("\n".join(lines))
    print("carried.py: wrote %d rows to %s" % (len(files), CARRIED_MD))
    write_tools(staged, scan)
    write_bench(staged, scan)


def write_tools(staged, scan):
    """Holds this script's record of the tools to the staged originals and
    the scan, writes the list of the staged tools' strings that the files
    here keep, and rewrites the tools' table in CARRIED.md."""
    staged_tools = set()
    for dirpath, _, names in os.walk(os.path.join(staged, TOOLS_DIR)):
        for name in names:
            if name != ".DS_Store":
                staged_tools.add(os.path.relpath(os.path.join(dirpath, name), staged).replace(os.sep, "/"))
    if staged_tools != set(TOOLS):
        fail("the staged files of %s are not the ones recorded here: only staged %s; only recorded %s"
             % (TOOLS_DIR, sorted(staged_tools - set(TOOLS)), sorted(set(TOOLS) - staged_tools)))
    for path, (size, digest, verdict, syms, share, run, _) in TOOLS.items():
        orig = read(os.path.join(staged, path))
        if len(orig) != size or sha256(orig) != digest:
            fail("%s: the staged original is not the one recorded here (%d bytes, %s)" % (path, len(orig), sha256(orig)))
        row = scan.get(path)
        if row is None:
            fail("%s is not in the scan" % path)
        got = (int(row["bytes"]), row["verdict"], int(row["n_crush_syms"]), row["ext_sigd_pct"], row["ext_srun_longest"])
        if got != (size, verdict, syms, share, run):
            fail("%s: the scan's numbers are not the ones recorded here: %r" % (path, got))
    was = []
    for path in TOOLS_PROGRAM:
        was += go_strings(read(os.path.join(staged, path)).decode(), path)
    was = set(s for s in was if s)
    for s in TOOLS_DROPPED:
        if s not in was:
            fail("%r is listed as dropped, but the staged tools never held it" % s)
    here = tools_strings_here()
    gone = sorted(s for s in was if s not in here and s not in TOOLS_DROPPED)
    if gone:
        fail("strings of the staged tools that are neither in %s nor listed as dropped: %s" % (TOOLS_DIR, "; ".join(repr(s) for s in gone)))
    kept = sorted(s for s in was if s not in TOOLS_DROPPED)
    with open(TOOLS_STRINGS, "w", newline="\n") as f:
        f.write(json.dumps(kept, ensure_ascii=False, indent=0) + "\n")
    lines = read(CARRIED_MD).decode().split("\n")
    a, b = block(lines, TOOLS_BEGIN, TOOLS_END)
    lines[a + 1 : b] = [""] + tools_table() + [""]
    with open(CARRIED_MD, "w", newline="\n") as f:
        f.write("\n".join(lines))
    print("carried.py: wrote the %d rows of the tools to %s, and the %d strings kept from the staged tools to %s"
          % (len(TOOLS), CARRIED_MD, len(kept), TOOLS_STRINGS))


def write_bench(staged, scan):
    """Holds this script's record of the bench to the staged originals and
    the scan, writes the list of the lines kept in the changed files, and
    rewrites the bench's table in CARRIED.md."""
    staged_bench = set()
    for dirpath, _, names in os.walk(os.path.join(staged, BENCH_DIR)):
        for name in names:
            if name != ".DS_Store":
                staged_bench.add(os.path.relpath(os.path.join(dirpath, name), staged).replace(os.sep, "/"))
    if staged_bench != set(BENCH):
        fail("the staged files of %s are not the ones recorded here: only staged %s; only recorded %s"
             % (BENCH_DIR, sorted(staged_bench - set(BENCH)), sorted(set(BENCH) - staged_bench)))
    kept = {}
    for path, (size, digest, verdict, share, run, how, _) in sorted(BENCH.items()):
        orig = read(os.path.join(staged, path))
        if len(orig) != size or sha256(orig) != digest:
            fail("%s: the staged original is not the one recorded here (%d bytes, %s)" % (path, len(orig), sha256(orig)))
        row = scan.get(path)
        if row is None:
            fail("%s is not in the scan" % path)
        got = (int(row["bytes"]), row["verdict"], row["ext_sigd_pct"], row["ext_srun_longest"])
        if got != (size, verdict, share, run):
            fail("%s: the scan's numbers are not the ones recorded here: %r" % (path, got))
        if how == LEFT:
            if os.path.exists(path):
                fail("%s is recorded as not brought, and it is in the tree" % path)
            continue
        if not os.path.isfile(path):
            fail("%s is recorded as %s but is not in the tree" % (path, how))
        here = read(path)
        if how == CARRIED and here != orig:
            fail("%s is recorded as carried and differs from its staged original" % path)
        if how == CHANGED:
            if here == orig:
                fail("%s is recorded as changed and is its staged original, byte for byte" % path)
            kept[path] = bench_kept(orig, here)
    with open(BENCH_KEPT, "w", newline="\n") as f:
        f.write(json.dumps(kept, indent=0, sort_keys=True) + "\n")
    lines = read(CARRIED_MD).decode().split("\n")
    a, b = block(lines, BENCH_BEGIN, BENCH_END)
    lines[a + 1 : b] = [""] + bench_table(kept) + [""]
    with open(CARRIED_MD, "w", newline="\n") as f:
        f.write("\n".join(lines))
    print("carried.py: wrote the %d rows of the bench to %s, and the lines kept of its %d changed files to %s"
          % (len(BENCH), CARRIED_MD, len(kept), BENCH_KEPT))


def main():
    ap = argparse.ArgumentParser(description="Write or check the tables of carried files in CARRIED.md.")
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check", help="check the tree against the tables")
    w = sub.add_parser("write", help="rewrite the tables from the staged originals")
    w.add_argument("--staged", required=True, help="the staging folder's files/ directory")
    w.add_argument("--scan", required=True, help="the staging folder's scan.tsv")
    t = sub.add_parser("tools", help="hold the tools' record to their staged originals, and rewrite their table and their strings")
    t.add_argument("--staged", required=True, help="the staging folder's files/ directory")
    t.add_argument("--scan", required=True, help="the staging folder's scan.tsv")
    b = sub.add_parser("bench", help="hold the bench's record to its staged originals, and rewrite its table and its list of kept lines")
    b.add_argument("--staged", required=True, help="the staging folder's files/ directory")
    b.add_argument("--scan", required=True, help="the staging folder's scan.tsv")
    args = ap.parse_args()
    if args.cmd == "write":
        write(args.staged, args.scan)
    if args.cmd == "tools":
        with open(args.scan, newline="") as f:
            write_tools(args.staged, {r["path"]: r for r in csv.DictReader(f, delimiter="\t")})
    if args.cmd == "bench":
        with open(args.scan, newline="") as f:
            write_bench(args.staged, {r["path"]: r for r in csv.DictReader(f, delimiter="\t")})
    check()


if __name__ == "__main__":
    main()

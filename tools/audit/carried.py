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
        checked without the originals; "The tools" below says what.
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
"""

import argparse
import csv
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
    print("carried.py: ok: %d carried files are their staged originals, but for the listed edits to %d of them; "
          "%d fresh files beside them" % (len(rows), len(EDITS), len(FRESH)))
    print("carried.py: ok: %s holds the %d files made from its staged files and %d fresh ones, as recorded; "
          "all %d strings kept from the staged tools stand in them" % (TOOLS_DIR, made, fresh, strings))


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
    args = ap.parse_args()
    if args.cmd == "write":
        write(args.staged, args.scan)
    if args.cmd == "tools":
        with open(args.scan, newline="") as f:
            write_tools(args.staged, {r["path"]: r for r in csv.DictReader(f, delimiter="\t")})
    check()


if __name__ == "__main__":
    main()

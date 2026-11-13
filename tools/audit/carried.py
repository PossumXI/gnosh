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
        It exits non-zero and says why if anything differs.

    tools/audit/carried.py write --staged DIR --scan FILE
        Needs the inventory's staging folder: DIR is its files/ directory and
        FILE its scan.tsv. It compares every file in the carried directories
        with its staged original, requires each difference to be exactly the
        edits listed below, and rewrites the two tables of CARRIED.md, each
        between its pair of marker lines. The rest of CARRIED.md is written
        by hand. It then runs the check.

The table of files has one row per carried file: its path here, its path in
the earlier gnosh, its kind, the size of the staged original, the scan's two
measures for it (the share of its distinct significant lines that also stand
in Charm's code, and the longest run of consecutive lines the two share),
the SHA-256 of the staged original, and what was changed.
"""

import argparse
import csv
import hashlib
import os
import sys

CARRIED_MD = "CARRIED.md"
FILES_BEGIN = "<!-- carried-table:begin (written by tools/audit/carried.py; do not edit by hand) -->"
FILES_END = "<!-- carried-table:end -->"
SUMMARY_BEGIN = "<!-- carried-summary:begin (written by tools/audit/carried.py; do not edit by hand) -->"
SUMMARY_END = "<!-- carried-summary:end -->"

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

# Staged files under those directories that were not brought.
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


def fail(msg):
    print("carried.py: " + msg, file=sys.stderr)
    sys.exit(1)


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def read(path):
    with open(path, "rb") as f:
        return f.read()


def tree_files():
    """Every file under the carried directories, and the single files, by path."""
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
                if name != ".DS_Store":  # the Finder's, never committed
                    out.append(os.path.join(dirpath, name).replace(os.sep, "/"))
    return sorted(out)


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
    print("carried.py: ok: %d carried files are their staged originals, but for the listed edits to %d of them; "
          "%d fresh files beside them" % (len(rows), len(EDITS), len(FRESH)))


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


def main():
    ap = argparse.ArgumentParser(description="Write or check the tables of carried files in CARRIED.md.")
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check", help="check the tree against the tables")
    w = sub.add_parser("write", help="rewrite the tables from the staged originals")
    w.add_argument("--staged", required=True, help="the staging folder's files/ directory")
    w.add_argument("--scan", required=True, help="the staging folder's scan.tsv")
    args = ap.parse_args()
    if args.cmd == "write":
        write(args.staged, args.scan)
    check()


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""List invisible, control and bidirectional characters written literally in
gnosh's own Go sources.

A source file should show everything it holds: such a character belongs in a
string as an escape (\\u200b), where a reader can see it. Carried and
third-party code and test data are left alone. Run from anywhere:

    python3 tools/audit/unseen.py          # lists them; exit 1 if any
    python3 tools/audit/unseen.py --allow internal/markdown   # skip a directory

Emoji joined by U+200D in a test's sample text are a judgement call: write
them as escapes too, so a diff shows what changed.
"""
import os, subprocess, sys, unicodedata

SKIP = ("internal/gnovm/", "internal/gnochain/", "internal/swarm/", "gno/")
SPACES = "\u00a0\u2007\u202f\u3000"


def main():
    root = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    allow = tuple(a.rstrip("/") + "/" for a in sys.argv[2:]) if sys.argv[1:2] == ["--allow"] else ()
    files = subprocess.run(["git", "-C", root, "ls-files", "-co", "--exclude-standard", "*.go"],
                           capture_output=True, text=True, check=True).stdout.split()
    total = 0
    for f in files:
        if f.startswith(SKIP) or f.startswith(allow) or "/testdata/" in f:
            continue
        with open(os.path.join(root, f), encoding="utf-8") as fh:
            for n, line in enumerate(fh, 1):
                bad = [c for c in line if c not in "\t\n" and
                       (unicodedata.category(c) in ("Cf", "Cc", "Co", "Cs", "Zl", "Zp") or c in SPACES)]
                if bad:
                    total += len(bad)
                    print("%s:%d: %s" % (f, n, " ".join("U+%04X" % ord(c) for c in bad)))
    if total:
        print("%d character(s) a reader cannot see" % total)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

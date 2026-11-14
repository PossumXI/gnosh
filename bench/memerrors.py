#!/usr/bin/env python3
"""How gnosh's mem calls went: every gnovm_run call in the given runs' gnosh
session files, and the errors they returned, by kind.

Usage: memerrors.py <run dir>...
"""
import collections
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import sessions  # noqa: E402

RULES = [
    ("nothing came between your entering", "pop with no work since entering the todo"),
    ("you are in `", "pop or close aimed at another todo than the one it stands in"),
    ("not popped: the check of", "close refused by the close review"),
    ("not closed: the check of", "close refused by the close review"),
    ("is a name, not a task", "a todo name given where a task text is wanted"),
    ("has open todos", "pop of a todo whose own todos are still open"),
    ("name mem not declared", "mem code run without name \"mem\""),
    ("not declared", "undeclared name in the Gno code"),
    ("expected", "Gno syntax error"),
    ("cannot use", "Gno type error"),
    ("no such todo", "unknown todo name"),
    ("not found", "unknown todo name"),
]


def classify(text):
    for key, label in RULES:
        if key in text:
            return label
    return "other: " + text[30:120].replace("\n", " ")


def main():
    calls = 0
    kinds = collections.Counter()
    example = {}
    for d in sys.argv[1:]:
        root = sessions.store(d.rstrip("/"))
        for path in sessions.session_files(root) + sessions.subagent_files(root):
            ents = sessions.entries(path)
            res = sessions.results(ents)
            for cid, name, _ in sessions.calls(ents):
                if name != "gnovm_run":
                    continue
                calls += 1
                text = res.get(cid, ("", False))[0]
                if text.startswith("ERROR"):
                    k = classify(text)
                    kinds[k] += 1
                    example.setdefault(k, text[:240].replace("\n", " | "))
    errors = sum(kinds.values())
    print("gnovm_run calls: %d, errors: %d (%.0f%%)" % (calls, errors, 100.0 * errors / max(calls, 1)))
    for k, n in kinds.most_common():
        print("%4d  %s" % (n, k))
    others = [v for k, v in example.items() if k.startswith("other")]
    for v in others[:5]:
        print("  e.g. " + v)


if __name__ == "__main__":
    main()

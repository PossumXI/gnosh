#!/usr/bin/env python3
"""What gnosh leaves behind in a bench run directory, read for scoring.

A run directory (bench/run.sh) holds the run's own home, and gnosh keeps
everything under it:

  home/.local/share/gnosh/projects/<folder>/<session id>.jsonl   a session
  home/.local/share/gnosh/projects/<folder>/<session id>/...     its sub-agents
  home/.local/share/gnosh/gnovm/                                 mem
  session-N.out                                                  what session N printed

A session file is one JSON object a line (internal/transcript/doc.go says
what each is). This module reads the few kinds the scoring needs: a reply's
tool calls, the results that answer them, and the frame and summary entries.
session-N.out is the run's --output-format stream-json: the init line, the
messages, and the result object last.

Usage: sessions.py <run dir>...   -> one JSON line per run (what facts() finds)

Python 3.8+, standard library only.
"""
import glob
import json
import os
import sys


def store(run_dir):
    """Where a run keeps gnosh's sessions."""
    return os.path.join(run_dir, "home", ".local", "share", "gnosh", "projects")


def session_files(root):
    """The session files under root, a run's store or any folder: the
    sessions a person or a script started, oldest first."""
    files = glob.glob(os.path.join(root, "*", "*.jsonl"))
    if not files:  # root may be one project's folder, or hold a single file
        files = glob.glob(os.path.join(root, "*.jsonl"))
    return sorted(files, key=lambda f: (os.path.getmtime(f), f))


def subagent_files(root):
    """The session files of sub-agents under root: each lies in the folder
    named after the session that started it."""
    return sorted(glob.glob(os.path.join(root, "*", "*", "**", "*.jsonl"), recursive=True))


def entries(path):
    """The entries of a session file, in order. A line that is not JSON (a
    line torn by a kill, or one being written right now) is passed over."""
    out = []
    try:
        f = open(path, errors="replace")
    except OSError:
        return out
    with f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                e = json.loads(line)
            except ValueError:
                continue
            if isinstance(e, dict):
                out.append(e)
    return out


def text_of(content):
    """The text of a block's content: a string, or text parts joined."""
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "\n".join(p.get("text") or "" for p in content if isinstance(p, dict))
    return ""


def blocks(entry):
    """The content blocks of a user or assistant entry."""
    content = (entry.get("message") or {}).get("content")
    return [b for b in content if isinstance(b, dict)] if isinstance(content, list) else []


def calls(ents):
    """Every tool call of a session, in order: (id, name, input)."""
    out = []
    for e in ents:
        if e.get("type") != "assistant":
            continue
        for b in blocks(e):
            if b.get("type") == "tool_use":
                inp = b.get("input")
                out.append((b.get("id"), b.get("name") or "?", inp if isinstance(inp, dict) else {}))
    return out


def results(ents):
    """Every tool result of a session, by the id of its call:
    (text, is_error)."""
    out = {}
    for e in ents:
        if e.get("type") != "user":
            continue
        for b in blocks(e):
            if b.get("type") == "tool_result":
                out[b.get("tool_use_id")] = (text_of(b.get("content")), bool(b.get("is_error")))
    return out


def frames(ents, op):
    """The frame entries of one kind: "push", "pop" or "slide"."""
    return [e for e in ents if e.get("type") == "frame" and e.get("op") == op]


def stream(path):
    """What a session printed (--output-format stream-json): its init line
    and its last result object, either None when it is not there. A session
    stopped by a signal prints a result too ("error_during_execution"); one
    that was killed outright prints none."""
    init = result = None
    try:
        f = open(path, errors="replace")
    except OSError:
        return init, result
    with f:
        for line in f:
            try:
                o = json.loads(line)
            except ValueError:
                continue
            if not isinstance(o, dict):
                continue
            if o.get("type") == "system" and o.get("subtype") == "init":
                init = o
            elif o.get("type") == "result":
                result = o
    return init, result


def facts(run_dir):
    """What gnosh itself says of a run, from what each session printed: the
    session ids, how each session ended, the turns gnosh counted, the window
    it believed the model has, and the tokens it counted over every request
    (the main loop's and the side requests')."""
    out = {"session_ids": [], "results": [], "num_turns": [], "tools_offered": None,
           "window_believed": None, "gnosh_prompt_tokens": 0, "gnosh_generated": 0}
    outs = glob.glob(os.path.join(run_dir, "session-*.out"))
    for path in sorted(outs, key=lambda p: int(os.path.basename(p)[len("session-"):-len(".out")])):
        init, result = stream(path)
        if init is not None:
            if init.get("session_id") not in out["session_ids"]:
                out["session_ids"].append(init.get("session_id"))
            out["tools_offered"] = init.get("tools")
            out["permission_mode"] = init.get("permissionMode")
            out["gnosh_build"] = init.get("gnosh_version")
        if result is None:
            out["results"].append(None)
            out["num_turns"].append(None)
            continue
        out["results"].append(result.get("subtype"))
        out["num_turns"].append(result.get("num_turns"))
        for usage in (result.get("modelUsage") or {}).values():
            out["gnosh_prompt_tokens"] += usage.get("inputTokens") or 0
            out["gnosh_generated"] += usage.get("outputTokens") or 0
            if usage.get("contextWindow"):
                out["window_believed"] = usage["contextWindow"]
    return out


if __name__ == "__main__":
    for d in sys.argv[1:]:
        print(json.dumps(dict(facts(d.rstrip("/")), dir=d)))

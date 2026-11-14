#!/usr/bin/env python3
"""Waits until a gnosh session has made a plan in mem: a gnovm_run call on
mem whose code plans todos (mem.Todo) and whose result is not an error.

Usage: waitplan.py <sessions dir> [timeout seconds] [pid]
  exit 0 = planned; 1 = timed out, or process pid (the planning session) ended first

Reads the session files under <sessions dir> (a run's
home/.local/share/gnosh/projects, see sessions.py) while gnosh writes them,
every second.
"""
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import sessions  # noqa: E402


def planned(root):
    for path in sessions.session_files(root):
        ents = sessions.entries(path)
        res = sessions.results(ents)
        for cid, name, inp in sessions.calls(ents):
            if name == "gnovm_run" and inp.get("name") == "mem" and "mem.Todo(" in (inp.get("code") or ""):
                text, is_error = res.get(cid, ("ERROR", True))  # no result yet
                if not is_error and not text.startswith("ERROR"):
                    return True
    return False


def alive(pid):
    try:
        os.kill(pid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True


def main():
    root = sys.argv[1]
    deadline = time.time() + (float(sys.argv[2]) if len(sys.argv) > 2 else 900)
    pid = int(sys.argv[3]) if len(sys.argv) > 3 else 0
    while time.time() < deadline:
        if planned(root):
            return 0
        if pid and not alive(pid):
            return 1
        time.sleep(1)
    return 1


if __name__ == "__main__":
    sys.exit(main())

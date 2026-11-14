#!/usr/bin/env python3
"""The dry run's two halves (bench/dryrun.sh drives them).

  dryrun.py script <name> <solved dir> <out.json> <expect.json>
      Writes the script of replies the scripted model server
      (tools/dev/scriptmodel) gives in one run, and what a run that was
      served exactly those replies must show.
  dryrun.py check <run dir> <expect.json>
      Holds a finished run to that: its row, the proxy's log, the request
      bodies the proxy saved, what gnosh printed, and gnosh's session files.
      Prints one line per check and exits 1 if any failed.
  dryrun.py standin <port file>
      A stand-in for the three kinds of model server serverfacts.py reads
      (/plain, /router and /vllm in front of the paths), which records
      what it was asked (/asked).

The scripted server answers the chat requests it is sent with its replies in
order, whoever asks: gnosh's own requests and its short side requests (the
names of new todos, the review of a close) alike. So a script lists both, in
the order gnosh makes them, and a run that made one request more or one
fewer ends on the wrong reply, which the check sees.

What the scripted server reports for each request is fixed by its code: a
quarter of the request body's bytes as prompt tokens, and a quarter of the
reply's bytes plus one as generated tokens. The check works both out again,
the first from the bodies the proxy saved and the second from the script,
and holds the proxy's log and gnosh's own count to them.

Python 3.8+, standard library only.
"""
import glob
import json
import os
import sys
import threading
import urllib.parse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import analyze  # noqa: E402
import sessions  # noqa: E402
import waitplan  # noqa: E402

MEM_TOOLS = ["gnovm_run", "gnovm_new", "gnovm_list", "gnovm_drop"]


def call(tool, **args):
    return {"name": tool, "args": args}


def mem(code):
    return call("gnovm_run", name="mem", code=code)


def reply(text="", thinking="", calls=(), side=False):
    """One reply of a script. side marks the answer to a request gnosh
    makes on its own account, which is not a turn of the session."""
    return {"text": text, "thinking": thinking, "calls": list(calls), "side": side}


def read(solved, name):
    with open(os.path.join(solved, name)) as f:
        return f.read()


def scripts(solved):
    """The scripts by name. solved is a folder holding each task's solved
    copy (tasks/<task>/make.sh --solved), from which the scripted "model" is
    handed a file where a run is to show part of a task done."""
    short = [reply(thinking="A look, and that is all.", calls=[call("Bash", command="ls"), call("Read", file_path="go.mod")]),
             reply(text="Looked at the project. The dry run stops here.")]
    planned = [reply(calls=[call("Read", file_path="go.mod")]),
               reply(calls=[mem('mem.Todo("rename tallyho to quokka", "rename the module", "rename the files").Push()')]),
               reply(text="tallyho-quokka-rename\nmodule-rename\nfiles-rename", side=True),
               reply(text="Planned in mem. The dry run stops here.")]
    return {
        # The four tasks. rename: a plan in mem, a todo entered, one edit, and
        # a close that the review lets through. The task is not done.
        "rename": {"task": "rename", "passed": False, "check": "FAIL: the old name is still in",
                   "replies": [[
                       reply(thinking="Look first.", calls=[call("Bash", command="ls"), call("Read", file_path="go.mod")]),
                       reply(calls=[mem('mem.Todo("rename tallyho to quokka", "rename the module", "rename the files").Push()')]),
                       reply(text="tallyho-quokka-rename\nmodule-rename\nfiles-rename", side=True),
                       reply(calls=[mem("mem.Next()")]),
                       reply(calls=[call("Edit", file_path="go.mod", old_string="module example.com/tallyho", new_string="module example.com/quokka")]),
                       reply(calls=[mem('mem.Pop("go.mod now says module example.com/quokka")')]),
                       reply(text='{"ok": true, "reason": "the edit to go.mod in the todo shows the new module path"}', side=True),
                       reply(text="Renamed the module path and nothing else. The dry run stops here."),
                   ]],
                   "mem": {"Todo": 1, "Next": 1, "Pop": 1}, "pops": 1, "mem_errors": 0,
                   "mem_show": ["rename tallyho to quokka", "rename the files"]},
        # algos: no plan; one of the nine packages written from the solved copy.
        "algos": {"task": "algos", "passed": False, "check": "FAIL: 1/9 packages pass",
                  "replies": [[
                      reply(calls=[call("Read", file_path="stack/stack.go"), call("Read", file_path="stack/stack_test.go")]),
                      reply(calls=[call("Write", file_path="stack/stack.go", content=read(solved, "algos/stack/stack.go"))]),
                      reply(calls=[call("Bash", command="go test ./stack/")]),
                      reply(text="stack is written and its tests pass. Eight packages are left. The dry run stops here."),
                  ]],
                  "mem": {}, "pops": 0, "mem_errors": 0, "mem_show": []},
        # bugs: one of the six bugs fixed, a close that the review refuses, and
        # the turn that gnosh then holds open.
        "bugs": {"task": "bugs", "passed": False, "check": "FAIL: 1/6 bugs fixed",
                 "replies": [[
                     reply(calls=[call("Bash", command="go test ./... 2>&1 | tail -20")]),
                     reply(calls=[mem('mem.Todo("fix the failing tests", "fix money", "fix cart").Push()')]),
                     reply(text="failing-tests-fix\nmoney-fix\ncart-fix", side=True),
                     reply(calls=[mem("mem.Next()")]),
                     reply(calls=[call("Read", file_path="money/money.go")]),
                     reply(calls=[call("Write", file_path="money/money.go", content=read(solved, "bugs/money/money.go"))]),
                     reply(calls=[mem('mem.Pop("money.String prints the cents with two digits, and go test ./money passes")')]),
                     reply(text='{"ok": false, "reason": "no run of go test ./money is shown in the todo"}', side=True),
                     reply(text="The close was refused and the test run is still to do."),
                     # gnosh sends the model back to a todo whose close was refused, once.
                     reply(text="The todo stays open: its test run is still to do. The dry run stops here."),
                 ]],
                 "mem": {"Todo": 1, "Next": 1, "Pop": 1}, "pops": 0, "mem_errors": 1,
                 "mem_show": ["fix the failing tests", "fix money"]},
        # continue: two sessions. The first plans and stops; the second, the
        # same session continued, is handed the solved files, so that the
        # check is seen to pass as well as to fail.
        "continue": {"task": "continue", "passed": True, "check": "PASS: 6/6 changes done", "check1": "FAIL: 0/6 changes done",
                     "replies": [[
                         reply(calls=[call("Read", file_path="notes.go"), call("Read", file_path="notes_test.go")]),
                         reply(calls=[mem('mem.Todo("six changes to notes", "tags", "search", "delete many", "with tag", "stats", "export").Push()')]),
                         reply(text="notes-six-changes\ntags-field\nsearch-method\ndelete-many\nwith-tag\nstats-method\nexport-method", side=True),
                         reply(text="Planned the six changes in mem and made none. The dry run stops here."),
                     ], [
                         reply(calls=[call("Read", file_path="notes.go"), call("Read", file_path="notes_test.go")]),
                         reply(calls=[call("Write", file_path="notes.go", content=read(solved, "continue/notes.go")),
                                      call("Write", file_path="notes_test.go", content=read(solved, "continue/notes_test.go"))]),
                         reply(calls=[call("Bash", command="go test ./...")]),
                         reply(text="All six changes are made and the tests pass."),
                     ]],
                     "mem": {"Todo": 1}, "pops": 0, "mem_errors": 0, "mem_show": ["six changes to notes", "export"]},
        # The same short run, the ways a run can be made.
        "short": {"task": "rename", "passed": False, "check": "FAIL: the old name is still in", "replies": [short],
                  "mem": {}, "pops": 0, "mem_errors": 0, "mem_show": []},
        # A plan and its names: two requests, one of them a side request.
        "planned": {"task": "rename", "passed": False, "check": "FAIL: the old name is still in", "replies": [planned],
                    "mem": {"Todo": 1}, "pops": 0, "mem_errors": 0, "mem_show": ["rename the module"]},
        # A command that outlasts the wall cap.
        "sleeper": {"task": "rename", "passed": False, "check": "FAIL: the old name is still in",
                    "replies": [[reply(calls=[call("Bash", command="sleep 300")]),
                                 reply(text="This reply is never asked for.")]],
                    "stopped": True, "mem": {}, "pops": 0, "mem_errors": 0, "mem_show": []},
        # The two-session task with a command in its first session that
        # outlasts the cap on the whole run: the second is never started.
        "sleeper2": {"task": "continue", "passed": False, "check": "FAIL: 0/6 changes done", "check1": "FAIL: 0/6 changes done",
                     "replies": [[reply(calls=[call("Bash", command="sleep 300")]),
                                  reply(text="This reply is never asked for.")],
                                 [reply(text="Nor is this one.")]],
                     "stopped": True, "mem": {}, "pops": 0, "mem_errors": 0, "mem_show": []},
        # The two-session task with its second prompt given to a new session.
        "fresh": {"task": "continue", "passed": False, "check": "FAIL: 0/6 changes done", "check1": "FAIL: 0/6 changes done",
                  "replies": [[reply(text="Nothing done in this dry run.")],
                              [reply(text="Nothing is left from before that this session knows of.")]],
                  "mem": {}, "pops": 0, "mem_errors": 0, "mem_show": []},
        # More turns than the turn cap allows.
        "long": {"task": "rename", "passed": False, "check": "FAIL: the old name is still in",
                 "replies": [[reply(calls=[call("Bash", command="ls")]),
                              reply(calls=[call("Read", file_path="go.mod")]),
                              reply(calls=[call("Read", file_path="README.md")]),
                              reply(text="This reply is never asked for.")]],
                 "served": 2, "mem": {}, "pops": 0, "mem_errors": 0, "mem_show": []},
        # A swarm of two: the planner plans and stops, the helper finds
        # nothing it need do. The last two replies are alike, so that it
        # does not matter which session is given which.
        "swarm": {"task": "rename", "passed": False, "check": "FAIL: the old name is still in",
                  "replies": [[
                      reply(calls=[mem('mem.Todo("rename tallyho to quokka", "rename the module", "rename the files").Push()')]),
                      reply(text="tallyho-quokka-rename\nmodule-rename\nfiles-rename", side=True),
                      reply(text="Nothing more in this dry run."),
                      reply(text="Nothing more in this dry run."),
                  ]],
                  "mem": {"Todo": 1}, "pops": 0, "mem_errors": 0, "mem_show": ["rename the module"]},
    }


def generated(r):
    """The tokens the scripted server reports for a reply: a quarter of the
    bytes of its text, its thinking and its calls' names and arguments, plus
    one (internal/app/apptest/model.go)."""
    size = len(r["text"].encode()) + len(r["thinking"].encode())
    for c in r["calls"]:
        size += len(c["name"]) + len(args_text(c))
    return size // 4 + 1


def args_text(c):
    """A call's arguments as the script file holds them, which is the text
    the scripted server sends and counts."""
    return json.dumps(c["args"], separators=(",", ":"))


def write_script(name, solved, out, expect):
    spec = scripts(solved)[name]
    flat = [r for part in spec["replies"] for r in part]
    # The script file: the replies as scriptmodel reads them. Each call's
    # arguments are written exactly as args_text gives them.
    items = []
    for r in flat:
        fields = []
        if r["text"]:
            fields.append('"text": ' + json.dumps(r["text"]))
        if r["thinking"]:
            fields.append('"thinking": ' + json.dumps(r["thinking"]))
        if r["calls"]:
            fields.append('"calls": [' + ", ".join('{"name": %s, "args": %s}' % (json.dumps(c["name"]), args_text(c)) for c in r["calls"]) + "]")
        items.append("{" + ", ".join(fields) + "}")
    with open(out, "w") as f:
        f.write("[\n " + ",\n ".join(items) + "\n]\n")
    served = flat[:spec["served"]] if "served" in spec else flat
    if spec.get("stopped"):
        served = flat[:1]
    tools = {}
    for r in served:
        for c in r["calls"]:
            tools[c["name"]] = tools.get(c["name"], 0) + 1
    e = {
        "name": name, "task": spec["task"], "passed": spec["passed"], "check": spec["check"], "check1": spec.get("check1"),
        "stopped": bool(spec.get("stopped")),
        "requests": len(served),
        "turns": [sum(1 for r in part if not r["side"]) for part in spec["replies"]],
        "generated": sum(generated(r) for r in served),
        "tool_calls": sum(len(r["calls"]) for r in served),
        "tools": tools,
        "last_text": served[-1]["text"],
        "sessions": len(spec["replies"]),
        "mem": spec["mem"], "pops": spec["pops"], "mem_errors": spec["mem_errors"], "mem_show": spec["mem_show"],
    }
    with open(expect, "w") as f:
        json.dump(e, f, indent=1)


class Checks:
    def __init__(self, title):
        self.title, self.bad = title, 0

    def eq(self, what, got, want):
        ok = got == want
        self.bad += 0 if ok else 1
        print("  %s %s: %s" % ("ok  " if ok else "FAIL", what, json.dumps(got) if ok else "%s, want %s" % (json.dumps(got), json.dumps(want))))
        return ok

    def true(self, what, ok, detail=""):
        self.bad += 0 if ok else 1
        print("  %s %s%s" % ("ok  " if ok else "FAIL", what, (": " + detail) if detail and not ok else ""))
        return ok


def bodies(d):
    """The request bodies the proxy saved for run d, in the order sent."""
    return [json.load(open(f)) for f in sorted(glob.glob(os.path.join(d, "requests", "req-*.json")))]


def system_of(body):
    return "\n".join(m.get("content") or "" for m in body.get("messages") or [] if m.get("role") == "system" and isinstance(m.get("content"), str))


def check(d, expect_path, opts):
    e = json.load(open(expect_path))
    c = Checks(e["name"])
    print("%s (%s)" % (e["name"], os.path.basename(d)))
    row = json.load(open(os.path.join(d, "row.json")))
    log = [json.loads(line) for line in open(os.path.join(d, "proxy.jsonl")) if line.strip()]
    posts = [r for r in log if r.get("method") != "GET"]
    sent = bodies(d)
    swarm = row["harness"] == "gnosh-swarm"

    # The scorer.
    c.eq("the row's task", row["task"], e["task"])
    c.eq("the check passed", row["passed"], e["passed"])
    c.true("the check's line begins %r" % e["check"], row["check_msg"].startswith(e["check"]), row["check_msg"])
    if e.get("check1"):
        c.true("the check between the sessions begins %r" % e["check1"], row.get("check1", "").startswith(e["check1"]), row.get("check1", ""))
    want_outcome = opts.get("outcome") or ("passed" if e["passed"] else "failed")
    c.eq("the row's outcome", row["outcome"], want_outcome)
    if want_outcome == "stopped":
        c.eq("what stopped it", row.get("stopped_by"), opts.get("stopped_by"))

    # The proxy against the scripted server.
    c.eq("requests the proxy counted, against replies the script served", row["requests"], e["requests"])
    c.eq("requests the proxy logged", len(posts), e["requests"])
    c.eq("request bodies the proxy saved", len(sent), e["requests"])
    c.eq("errors the proxy saw", row["errors"], 0)
    if e["stopped"]:
        # The one request was cut off when the session was stopped: the
        # scripted server had answered it, and what the proxy made of the
        # cut is not held to a number. A session the cap stops still prints
        # its result object, which says that it was stopped mid-run.
        c.eq("how the session ended (stopped, and its result says so)", row["results"], ["error_during_execution"])
    else:
        served_prompt = sum(len(json.dumps(b).encode()) // 4 for b in sent)
        c.eq("prompt tokens: the proxy's log, against a quarter of each body it sent", row["prompt_tokens"], served_prompt)
        c.eq("generated tokens: the proxy's log, against the script", row["generated"], e["generated"])
        c.eq("tool calls the proxy saw streamed", row["tool_calls"], e["tool_calls"])
        c.eq("prompt tokens gnosh counted", row["gnosh_prompt_tokens"], row["prompt_tokens"])
        c.eq("generated tokens gnosh counted", row["gnosh_generated"], row["generated"])
        c.eq("every request was a chat completion answered 200", sorted({"%s %s" % (r.get("path"), r.get("status")) for r in posts}), ["/v1/chat/completions 200"])
        c.eq("every request named the pinned model", sorted({b.get("model") for b in sent}), [row["model"]])
        if opts.get("turn_cap"):
            c.eq("how the session ended", row["results"], ["error_max_turns"])
            c.eq("turns gnosh counted", row["num_turns"], [int(opts["turn_cap"])])
        elif not swarm:
            # The last reply of the script is what the last session answered
            # with: every reply was asked for, in order, and none after it.
            _, result = sessions.stream(os.path.join(d, "session-%d.out" % e["sessions"]))
            c.eq("the answer gnosh printed is the script's last reply", (result or {}).get("result"), e["last_text"])
            c.eq("turns gnosh counted, a session", row["num_turns"], e["turns"])
            c.eq("how each session ended", row["results"], ["success"] * e["sessions"])

    # What gnosh was told, and what it believed.
    c.eq("the window gnosh believed", row["window_believed"], row["window"])
    c.eq("the window the scripted server says it has", [row["window_server"], row["window_server_from"]], [100000, "props.n_ctx"])
    c.eq("so the window told is not the server's", row["window_true"], False)
    c.eq("requests past the window told", row["over_window"], 0)
    c.eq("gnosh asked the proxy for no list of models", row["gets"], [])

    # The session files.
    a = analyze.analyze(d)
    c.eq("tool calls by name, from the session files", a.get("tools"), e["tools"])
    c.eq("mem's calls, from the session files", a.get("mem", {}), e["mem"])
    c.eq("todos popped", a.get("pops"), e["pops"])
    want_sessions = row.get("swarm") or int(opts.get("session_files", 1))
    c.eq("session files", a.get("sessions"), want_sessions)
    c.eq("session ids gnosh printed", len(row["session_ids"]), want_sessions)
    planned = any("Todo" in k for k in e["mem"])
    c.eq("waitplan.py finds a plan", waitplan.planned(sessions.store(d)), planned)
    errs = 0
    for path in sessions.session_files(sessions.store(d)):
        ents = sessions.entries(path)
        res = sessions.results(ents)
        errs += sum(1 for cid, name, _ in sessions.calls(ents) if name == "gnovm_run" and res.get(cid, ("", False))[0].startswith("ERROR"))
    c.eq("mem calls refused", errs, e["mem_errors"])

    # How the run was made.
    mem_on = opts.get("mem", "on") == "on"
    c.eq("the row says mem is on", row["mem_on"], mem_on)
    offered = row.get("tools_offered") or []
    c.eq("the machine tools are offered", [t for t in MEM_TOOLS if t in offered], MEM_TOOLS if mem_on else [])
    if mem_on:
        show = open(os.path.join(d, "mem.txt")).read() if os.path.exists(os.path.join(d, "mem.txt")) else ""
        for words in e["mem_show"]:
            c.true("gnosh mem show names %r" % words, words in show, show[:300])
        if not e["mem_show"]:
            c.true("gnosh mem show ran", os.path.exists(os.path.join(d, "mem.txt")))
    else:
        c.true("no mem.txt without mem", not os.path.exists(os.path.join(d, "mem.txt")))
        c.true("no machine of mem's was made", not glob.glob(os.path.join(d, "home", ".local", "share", "gnosh", "gnovm", "*", "mem")))
    main = [b for b in sent if b.get("tools")]
    side = [b for b in sent if not b.get("tools")]
    c.true("gnosh's own requests carry tools", bool(main))
    if "memory" in opts:
        text = open(opts["memory"]).read().strip()
        c.true("the system prompt holds the text about mem that was asked for", all(text in system_of(b) for b in main))
    if "not_memory" in opts:
        for path in opts["not_memory"].split(","):
            text = open(path).read().strip()
            c.true("the system prompt does not hold %s" % os.path.basename(path), not any(text in system_of(b) for b in main))
    if "thinking" in opts:
        want = {"on": True, "off": False}[opts["thinking"]]
        c.eq("enable_thinking on gnosh's own requests", sorted({(b.get("chat_template_kwargs") or {}).get("enable_thinking") for b in main}), [want])
    if "slot" in opts:
        c.eq("id_slot on gnosh's own requests", sorted({b.get("id_slot") for b in main}), [int(opts["slot"])])
        c.eq("the row's slot", row["slot"], int(opts["slot"]))
    else:
        c.eq("no request is pinned to a slot", sorted({b.get("id_slot") for b in sent}, key=str), [None])
    if "side_slot" in opts:
        c.true("there was a side request", bool(side))
        c.eq("id_slot on the side requests", sorted({b.get("id_slot") for b in side}), [int(opts["side_slot"])])
    ptype = opts.get("provider", "llamacpp")
    c.eq("how gnosh spoke to the server", row.get("provider_type"), ptype)
    c.eq("llama.cpp's cache_prompt on the requests", sorted({b.get("cache_prompt") for b in sent}, key=str), [True] if ptype == "llamacpp" else [None])
    if "harness" in opts:
        c.eq("the row's harness", row["harness"], opts["harness"])
    if "started" in opts:
        c.eq("sessions started", len(glob.glob(os.path.join(d, "session-*.out"))), int(opts["started"]))
    if "appended" in opts:
        text = open(opts["appended"]).read().strip()
        c.true("the system prompt ends with the text appended to it", all(system_of(b).rstrip().endswith(text) for b in main))
    if "suffix" in opts:
        text = open(opts["suffix"]).read()
        users = [m.get("content") for m in main[0].get("messages") or [] if m.get("role") == "user"]
        c.true("the prompt carries the text added to it", any(isinstance(u, str) and text.strip() in u for u in users))
    if "variant" in opts:
        c.eq("the row's variant", row.get("variant"), opts["variant"])
        c.true("the row's version names the variant", ("+variant " + opts["variant"]) in row["version"], row["version"])
    if "prompts" in opts:
        c.true("the row's version names the folder of prompt texts", ("+prompts " + opts["prompts"]) in row["version"], row["version"])
    if "exit_codes" in opts:
        c.eq("the sessions' exit codes", row["exit_codes"], [int(x) for x in opts["exit_codes"].split(",")])
    else:
        c.eq("the sessions' exit codes", row["exit_codes"], [0] * (row.get("swarm") or e["sessions"]))
    c.eq("timed out", row["timed_out"], want_outcome == "stopped" and opts.get("stopped_by") == "wall cap")

    # The run kept to itself.
    home = os.path.join(d, "home")
    c.true("the run's sessions are under its own home", bool(sessions.session_files(sessions.store(d))))
    c.true("its config is under its own home", os.path.isfile(os.path.join(home, ".config", "gnosh", "gnosh.json")))
    config = json.load(open(os.path.join(home, ".config", "gnosh", "gnosh.json")))
    c.true("its config turns discovery off and names one model",
           all(p.get("discoverModels") is False for p in config["providers"].values()) and config["model"]["id"] == "box/" + row["model"])
    return c.bad


class StandIn(BaseHTTPRequestHandler):
    """Three model servers under one port, by the first part of the path:
    /plain (a llama.cpp server with one model), /router (one that holds
    several: qwen loaded, other not) and /vllm. /asked lists what was asked."""
    asked = []
    lock = threading.Lock()

    def log_message(self, fmt, *args):
        pass

    def do_GET(self):
        u = urllib.parse.urlsplit(self.path)
        if u.path == "/asked":
            return self.send(200, StandIn.asked)
        with StandIn.lock:
            StandIn.asked.append(self.path)
        kind, _, rest = u.path.lstrip("/").partition("/")
        rest = "/" + rest
        q = urllib.parse.parse_qs(u.query)
        if kind == "plain":
            if rest == "/v1/models":
                return self.send(200, {"object": "list", "data": [{"id": "qwen", "object": "model", "meta": {"n_ctx_train": 262144}}]})
            if rest == "/props":
                return self.send(200, {"default_generation_settings": {"n_ctx": 32768}, "total_slots": 3})
            if rest == "/slots":
                return self.send(200, [{"id": 0, "is_processing": True}, {"id": 1, "is_processing": False}, {"id": 2, "is_processing": False}])
        if kind == "router":
            if rest == "/v1/models":
                return self.send(200, {"object": "list", "data": [
                    {"id": "qwen", "object": "model", "status": {"value": "loaded"}},
                    {"id": "other", "object": "model", "status": {"value": "unloaded"}}]})
            if rest == "/slots" and q.get("model") == ["qwen"] and q.get("autoload") == ["false"]:
                return self.send(200, [{"id": 0, "is_processing": False}, {"id": 1, "is_processing": True}])
        if kind == "vllm" and rest == "/v1/models":
            return self.send(200, {"object": "list", "data": [{"id": "qwen", "object": "model", "max_model_len": 131072}]})
        self.send(404, {"error": {"message": "not found"}})

    def send(self, code, obj):
        data = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def standin(port_file):
    srv = ThreadingHTTPServer(("127.0.0.1", 0), StandIn)
    srv.daemon_threads = True
    with open(port_file + ".tmp", "w") as f:
        f.write(str(srv.server_address[1]))
    os.replace(port_file + ".tmp", port_file)
    srv.serve_forever()


def main():
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "script" and len(sys.argv) == 6:
        write_script(*sys.argv[2:6])
        return 0
    if cmd == "check" and len(sys.argv) >= 4:
        opts = dict(a.split("=", 1) for a in sys.argv[4:])
        return 1 if check(sys.argv[2].rstrip("/"), sys.argv[3], opts) else 0
    if cmd == "standin" and len(sys.argv) == 3:
        standin(sys.argv[2])
        return 0
    sys.exit(__doc__)


if __name__ == "__main__":
    sys.exit(main())

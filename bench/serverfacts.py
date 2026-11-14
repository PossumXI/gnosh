#!/usr/bin/env python3
"""What a model server says of itself, asked without disturbing it.

Usage:
  serverfacts.py <base url> <model>
      -> one JSON object, exit 0 (also when the server cannot be reached:
         the object then says why)
  serverfacts.py --gate [--idle-slots N] <base url> <model>
      -> the same object, and exit 0 only when a run may start: the server
         answers, it lists the model, and, where the server says so, that
         model is the one loaded. With --idle-slots, at least N of its
         slots must be idle too.

A bench run wants to know two things before it starts: that the server
holds the model the run names, and how large a context the server really
gives that model, which need not be the window the harness is told.

The server is asked for its list of models (GET /v1/models) and, with one
exception, nothing else. A llama.cpp server that holds several models
loads a model as soon as a request names it and puts out the one in use,
so such a server (a router: its list says which models are loaded) is
never asked about a model. The exception is a llama.cpp server that runs
one model: its /props names no model and gives the context of a slot.

The window is taken from the first of these that is there:
  the list's entry for the model: meta.n_ctx (llama.cpp), max_model_len
      (vLLM), context_length (others)
  /props: default_generation_settings.n_ctx (llama.cpp, one model)
A router's window is in neither: it is what the server was started with,
and run.sh takes it from SERVER_WINDOW.

--idle-slots asks /slots, which a llama.cpp server answers with one entry
per slot ("is_processing"). A router is asked only for a model its own
list calls loaded, and with autoload=false, so the question cannot load
one. This part has been tried against a stand-in only, never against a
real router.

Python 3.8+, standard library only.
"""
import argparse
import json
import sys
import urllib.error
import urllib.parse
import urllib.request


def get(base, path, timeout=10):
    """The JSON a GET of base + path answers with."""
    base = base.rstrip("/")
    if base.endswith("/v1"):
        base = base[:-3]
    req = urllib.request.Request(base + path, headers={"Accept": "application/json"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.loads(r.read().decode("utf-8", "replace"))


def status_of(entry):
    """A listed model's state, as a router gives it ("loaded", "unloaded",
    "loading"), or None when the list says nothing of it."""
    st = entry.get("status")
    if isinstance(st, dict):
        st = st.get("value")
    return st if isinstance(st, str) else None


def whole(v):
    """v as a positive whole number, or None."""
    return v if isinstance(v, int) and not isinstance(v, bool) and v > 0 else None


def facts(base, model):
    out = {"upstream": base, "model": model, "listed": [], "model_listed": False, "router": False,
           "status": None, "loaded": [], "window_server": None, "window_source": None}
    try:
        listing = get(base, "/v1/models")
    except (OSError, ValueError, urllib.error.URLError) as e:
        out["error"] = "the list of models could not be read: %s" % repr(e)[:200]
        return out
    data = listing.get("data") if isinstance(listing, dict) else None
    data = [m for m in (data or []) if isinstance(m, dict)]
    out["listed"] = [m.get("id") for m in data]
    out["router"] = any("status" in m for m in data)
    out["loaded"] = [m.get("id") for m in data if status_of(m) == "loaded"]
    for m in data:
        if m.get("id") != model:
            continue
        out["model_listed"] = True
        out["status"] = status_of(m)
        meta = m.get("meta") if isinstance(m.get("meta"), dict) else {}
        for source, v in (("models.meta.n_ctx", meta.get("n_ctx")),
                          ("models.max_model_len", m.get("max_model_len")),
                          ("models.context_length", m.get("context_length"))):
            if whole(v):
                out["window_server"], out["window_source"] = v, source
                break
    # /props speaks for the one model of a server that runs one model. A
    # router is never asked: the question could only be about one of its
    # models, and asking about a model loads it.
    if out["window_server"] is None and out["model_listed"] and not out["router"] and len(data) == 1:
        try:
            props = get(base, "/props")
        except (OSError, ValueError, urllib.error.URLError):
            props = None
        if isinstance(props, dict):
            settings = props.get("default_generation_settings")
            n = whole(settings.get("n_ctx")) if isinstance(settings, dict) else None
            if n:
                out["window_server"], out["window_source"] = n, "props.n_ctx"
            if whole(props.get("total_slots")):
                out["slots"] = props["total_slots"]
    return out


def idle_slots(base, model, f):
    """How many of the model's slots are idle, or None when the server
    cannot be asked without risk, or does not say."""
    if f["router"]:
        if f["status"] != "loaded":
            return None
        path = "/slots?" + urllib.parse.urlencode({"model": model, "autoload": "false"})
    elif len(f["listed"]) == 1:
        path = "/slots"
    else:
        return None
    try:
        slots = get(base, path)
    except (OSError, ValueError, urllib.error.URLError):
        return None
    if not isinstance(slots, list):
        return None
    return sum(1 for s in slots if isinstance(s, dict) and s.get("is_processing") is False)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--gate", action="store_true", help="exit 0 only when a run may start")
    ap.add_argument("--idle-slots", type=int, default=0, help="with --gate: this many slots must be idle")
    ap.add_argument("base")
    ap.add_argument("model")
    a = ap.parse_args()
    f = facts(a.base, a.model)
    ready, why = True, []
    if "error" in f:
        ready, why = False, [f["error"]]
    elif not f["model_listed"]:
        ready, why = False, ["the server does not list %s; it lists: %s" % (a.model, ", ".join(map(str, f["listed"])) or "nothing")]
    elif f["router"] and f["status"] != "loaded":
        ready, why = False, ["%s is %s on the server, not loaded; loaded: %s" % (a.model, f["status"] or "in no known state", ", ".join(map(str, f["loaded"])) or "nothing")]
    if a.idle_slots:
        f["idle_slots"] = idle_slots(a.base, a.model, f) if ready else None
        if ready and (f["idle_slots"] is None or f["idle_slots"] < a.idle_slots):
            ready = False
            why.append("%s slots are idle, and %d are wanted" % ("it is not known how many" if f["idle_slots"] is None else f["idle_slots"], a.idle_slots))
    if a.gate:
        f["ready"] = ready
        if why:
            f["not_ready"] = "; ".join(why)
    print(json.dumps(f))
    return 0 if ready or not a.gate else 1


if __name__ == "__main__":
    sys.exit(main())

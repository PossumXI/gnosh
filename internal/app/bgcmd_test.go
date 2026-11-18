package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// gnosh --bg starts a new session in the background on the prompt, as a
// run of its own (the flags written out, a new session's id, -p, and the
// prompt after --), keeps its process id beside its log, and says how to
// follow it. gnosh logs <session> prints what it printed, and gnosh stop
// <session> says when it is not running. -p, -c and a missing prompt are
// refused before anything starts.
func TestBackgroundFromTheShell(t *testing.T) {
	w := apptest.NewWorld(t)
	var got []string
	var log string
	o := testOptions(w, "--bg", "-y", "--name", "nightly", "fix the flaky test")
	o.Detach = func(args, env []string, l string) (int, error) {
		got, log = args, l
		return 4242, nil
	}
	r := runWith(t, context.Background(), o).wantCode(t, 0)
	if log == "" {
		t.Fatalf("nothing was started: %s", r.err)
	}
	id := strings.TrimSuffix(filepath.Base(log), ".background.log")
	want := []string{"--name=nightly", "--dangerously-skip-permissions", "--session-id=" + id, "--print", "--", "fix the flaky test"}
	slices.Sort(want[:3])
	head := slices.Clone(got[:3])
	slices.Sort(head)
	if len(got) != 6 || !slices.Equal(head, want[:3]) || !slices.Equal(got[3:], want[3:]) {
		t.Fatalf("the run was started with %q", got)
	}
	has(t, "what is said", r.out, "gnosh: backgrounded · "+id+" · process 4242", "gnosh agents ", "gnosh logs "+id[:13], "gnosh stop "+id[:13], "gnosh -r "+id)
	if pid, _ := os.ReadFile(strings.TrimSuffix(log, ".log") + ".pid"); strings.TrimSpace(string(pid)) != "4242" {
		t.Errorf("the process id kept: %q", pid)
	}

	// What the run printed, by the start of its id.
	f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("The flaky test is fixed.\n")
	f.Close()
	r = run(t, w, "logs", id[:13]).wantCode(t, 0)
	has(t, "gnosh logs", r.out, "goes on in the background with: fix the flaky test", "The flaky test is fixed.")
	r = run(t, w, "stop", id[:13]).wantCode(t, 0)
	has(t, "gnosh stop", r.out, "the session "+id+" is not running: gnosh -r "+id+" opens it")
	r = run(t, w, "stop", "nosuch").wantCode(t, 1)
	has(t, "gnosh stop", r.err, "no session ran in the background under the ID nosuch", "gnosh agents lists them")

	for _, bad := range [][]string{{"--bg", "-p", "x"}, {"--bg", "-c", "x"}, {"--bg"}} {
		o := testOptions(w, bad...)
		started := false
		o.Detach = func([]string, []string, string) (int, error) { started = true; return 1, nil }
		r := runWith(t, context.Background(), o).wantCode(t, 1)
		if started {
			t.Errorf("%q started a run", bad)
		}
		has(t, strings.Join(bad, " "), r.err, "nothing was started")
	}
}

// gnosh agents lists the sessions in the background of every folder, the
// working ones first, each by the start of its ID with when it started,
// how it stands, its folder and its prompt; --cwd keeps that folder's
// own. --json prints the working ones, and with --all the rest too.
// gnosh logs finds a run of another folder, and a run whose log says gnosh
// stop stopped it is listed as stopped.
func TestAgentsListsTheRunsOfEveryFolder(t *testing.T) {
	w := apptest.NewWorld(t)
	other := filepath.Join(w.Root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	start := func(cwd, prompt string) (id, log string) {
		t.Helper()
		o := testOptions(w, "--bg", prompt)
		o.Cwd = cwd
		o.Detach = func(_, _ []string, l string) (int, error) {
			log = l
			return 4242, nil
		}
		runWith(t, context.Background(), o).wantCode(t, 0)
		return strings.TrimSuffix(filepath.Base(log), ".background.log"), log
	}
	addTo := func(log, text string) {
		t.Helper()
		f, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		f.WriteString(text)
		f.Close()
	}
	done, doneLog := start(w.Dir, "fix the flaky test")
	addTo(doneLog, "All 42 tests pass.\n")
	working, workingLog := start(other, "write the docs")
	addTo(workingLog, "Reading the README.\n")
	// The working run holds its session, as a run does while it works.
	file := filepath.Join(filepath.Dir(workingLog), working+".jsonl")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(run(t, w, "agents").wantCode(t, 0).out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], working[:13]+"  ") || !strings.HasPrefix(lines[1], done[:13]+"  ") {
		t.Fatalf("gnosh agents printed:\n%s", strings.Join(lines, "\n"))
	}
	has(t, "the working run", lines[0], "  working ", "  "+other+"  ", "write the docs")
	has(t, "the run that is done", lines[1], "  done  ", "  "+w.Dir+"  ", "fix the flaky test")
	has(t, "how to go on", lines[2], "gnosh logs <name>", "gnosh stop <name>", "gnosh -r <name>")
	out := run(t, w, "agents", "--cwd", other).wantCode(t, 0).out
	has(t, "gnosh agents --cwd", out, working[:13])
	hasNot(t, "gnosh agents --cwd", out, done[:13])

	var rows []map[string]any
	if err := json.Unmarshal([]byte(run(t, w, "agents", "--json").wantCode(t, 0).out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["id"] != working || rows[0]["kind"] != "background" || rows[0]["state"] != "working" ||
		rows[0]["cwd"] != other || rows[0]["prompt"] != "write the docs" || rows[0]["status"] != "Reading the README." || rows[0]["pid"] != nil {
		t.Errorf("gnosh agents --json: %v", rows)
	}
	rows = nil
	if err := json.Unmarshal([]byte(run(t, w, "agents", "--json", "--all").wantCode(t, 0).out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1]["id"] != done || rows[1]["state"] != "done" || rows[1]["status"] != "All 42 tests pass." {
		t.Errorf("gnosh agents --json --all: %v", rows)
	}

	// From this folder, the run of the other.
	has(t, "gnosh logs", run(t, w, "logs", working[:13]).wantCode(t, 0).out, "goes on in the background with: write the docs", "Reading the README.")
	// A run gnosh stop stopped; its own last line is told, not the stop's.
	addTo(doneLog, "gnosh: 2026-11-18T12:00:00Z"+runStopped+"\n")
	lines = strings.Split(run(t, w, "agents", "--cwd", w.Dir).wantCode(t, 0).out, "\n")
	has(t, "the stopped run", lines[0], done[:13], "  stopped  ")
	if err := json.Unmarshal([]byte(run(t, w, "agents", "--json", "--all", "--cwd", w.Dir).wantCode(t, 0).out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "stopped" || rows[0]["status"] != "All 42 tests pass." {
		t.Errorf("the stopped run as JSON: %v", rows)
	}
	f.Close()
	has(t, "gnosh agents once nothing runs", run(t, w, "agents", "--json").wantCode(t, 0).out, "[]")
	empty := filepath.Join(w.Root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	has(t, "gnosh agents --cwd of a folder with none", run(t, w, "agents", "--cwd", empty).wantCode(t, 0).out,
		"No session runs in the background from "+empty+" or a folder in it, and none ran in the last week: gnosh --bg <prompt> starts one.")
}

package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

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
// own. --json prints the working ones, and with --all the rest too. A run
// works while its process does: a session held open by some other gnosh
// is not working. gnosh logs finds a run of another folder, and gnosh stop
// stops one and marks it stopped.
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
	// The run quotes gnosh's own lines: they are its output, not gnosh's.
	addTo(doneLog, "  gnosh: its folder is /elsewhere\ngnosh: its session file is /elsewhere/x.jsonl\nAll 42 tests pass.\n")
	working, workingLog := start(other, "write the docs")
	addTo(workingLog, "Reading the README.\n")
	// The working run's process: one whose command line names its
	// session, as a run's does, and which ends when asked to.
	proc := exec.Command("sh", "-c", `trap "exit 0" TERM; while :; do sleep 0.1; done`, working)
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() { proc.Wait(); close(ended) }()
	t.Cleanup(func() { proc.Process.Kill(); <-ended })
	if err := os.WriteFile(strings.TrimSuffix(workingLog, ".log")+".pid", []byte(strconv.Itoa(proc.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The finished run's session is open in a gnosh of its own.
	file := filepath.Join(filepath.Dir(doneLog), done+".jsonl")
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
		rows[0]["cwd"] != other || rows[0]["prompt"] != "write the docs" || rows[0]["status"] != "Reading the README." ||
		rows[0]["pid"] != float64(proc.Process.Pid) {
		t.Errorf("gnosh agents --json: %v", rows)
	}
	rows = nil
	if err := json.Unmarshal([]byte(run(t, w, "agents", "--json", "--all").wantCode(t, 0).out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1]["id"] != done || rows[1]["state"] != "done" || rows[1]["status"] != "All 42 tests pass." || rows[1]["cwd"] != w.Dir {
		t.Errorf("gnosh agents --json --all: %v", rows)
	}

	// From this folder, the run of the other: its log, then a stop.
	has(t, "gnosh logs", run(t, w, "logs", working[:13]).wantCode(t, 0).out, "goes on in the background with: write the docs", "Reading the README.")
	r := run(t, w, "stop", working[:13]).wantCode(t, 0)
	has(t, "gnosh stop", r.out, "gnosh: the session "+working+" was stopped. What it did so far is kept")
	has(t, "gnosh stop", r.err, "was asked to stop; it ends what it was doing first")
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("the run's process did not end")
	}
	lines = strings.Split(run(t, w, "agents", "--cwd", other).wantCode(t, 0).out, "\n")
	has(t, "the stopped run", lines[0], working[:13], "  stopped  ")
	if err := json.Unmarshal([]byte(run(t, w, "agents", "--json", "--all", "--cwd", other).wantCode(t, 0).out), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "stopped" || rows[0]["status"] != "Reading the README." {
		t.Errorf("the stopped run as JSON: %v", rows)
	}
	has(t, "gnosh agents once nothing runs", run(t, w, "agents", "--json").wantCode(t, 0).out, "[]")
	empty := filepath.Join(w.Root, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	has(t, "gnosh agents --cwd of a folder with none", run(t, w, "agents", "--cwd", empty).wantCode(t, 0).out,
		"No session runs in the background from "+empty+" or a folder in it, and none ran in the last week: gnosh --bg <prompt> starts one.")
}

// gnosh --bg takes what is piped in with the prompt, as gnosh -p does,
// and starts the run in this folder when --worktree is given, leaving the
// work tree to the run.
func TestBackgroundTakesPipedInput(t *testing.T) {
	w := apptest.NewWorld(t)
	var got []string
	o := testOptions(w, "--bg", "review this diff")
	o.Stdin = strings.NewReader("--- a/x.go\n+++ b/x.go\n")
	o.StdinIsTerminal = false
	o.Detach = func(args, _ []string, _ string) (int, error) {
		got = args
		return 4242, nil
	}
	runWith(t, context.Background(), o).wantCode(t, 0)
	if len(got) == 0 || got[len(got)-1] != "--- a/x.go\n+++ b/x.go\n\nreview this diff" {
		t.Errorf("the run was given %q", got)
	}
}

// gnosh --bg --worktree leaves the work tree to the run: the process that
// starts it makes none, and the run is given --worktree.
func TestBackgroundLeavesTheWorkTreeToTheRun(t *testing.T) {
	w := apptest.NewWorld(t)
	var got []string
	o := testOptions(w, "--bg", "fix it", "--worktree")
	o.Detach = func(args, _ []string, _ string) (int, error) {
		got = args
		return 4242, nil
	}
	r := runWith(t, context.Background(), o).wantCode(t, 0)
	if !slices.ContainsFunc(got, func(a string) bool { return strings.HasPrefix(a, "--worktree") }) {
		t.Errorf("the run was given %q", got)
	}
	hasNot(t, "what is said", r.out+r.err, "work tree")
}

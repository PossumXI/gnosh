package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
	has(t, "what is said", r.out, "gnosh: backgrounded · "+id+" · process 4242", "gnosh logs "+id[:13], "gnosh stop "+id[:13], "gnosh -r "+id)
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
	has(t, "gnosh stop", r.err, "no session of this folder ran in the background under the ID nosuch")

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

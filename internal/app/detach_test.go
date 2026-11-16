package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// /background ends the session here and hands it to a run of its own, on
// the prompt given: the same flags, the session resumed, -p, and the prompt
// after --, with its output in a file beside the session's.
func TestBackgroundCommand(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Hi."})
	var got [][]string
	var logs []string
	s := sitWith(t, w, func(o *Options) {
		o.Detach = func(args []string, log string) (int, error) {
			got, logs = append(got, args), append(logs, log)
			return 4242, nil
		}
	}, "-y", "say hi")
	s.see("Hi.")
	s.see("-- done in ")
	s.typeLine("/bg")
	s.see("say what the session is to do on its own: /background <prompt>")
	s.typeLine("/background fix the flaky test, then run them all")
	s.see("The session goes on in the background, on its own, with: fix the flaky test, then run them all")
	if code := s.exit(); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	id := strings.TrimSuffix(filepath.Base(w.Sessions()[0]), ".jsonl")
	want := []string{"--dangerously-skip-permissions", "--resume=" + id, "--print", "--", "fix the flaky test, then run them all"}
	if len(got) != 1 || !slices.Equal(got[0], want) {
		t.Fatalf("the run was started with %q, want %q", got, want)
	}
	if logs[0] != filepath.Join(filepath.Dir(w.Sessions()[0]), id+".background.log") {
		t.Errorf("its output goes to %s", logs[0])
	}
	has(t, "what is said on the way out", s.errs.String(), "gnosh: the session goes on in the background (process 4242)", id+".background.log", "gnosh -r "+id+" opens it again once it is done")
	if _, err := os.Stat(w.Sessions()[0]); err != nil {
		t.Errorf("the session's file: %v", err)
	}

	// A gnosh that cannot do it says so, and the session stays.
	w = apptest.NewWorld(t)
	s = sit(t, w)
	s.see("Type a prompt and Enter.")
	s.typeLine("/background go on")
	s.see("this gnosh cannot go on without its terminal")
	s.typeLine("/exit")
	s.exit()
}

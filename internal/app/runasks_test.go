package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A run in the background puts its permission question in its .ask file,
// says on stderr that it waits for an answer in gnosh agents, and waits:
// an answer that does not give the question's token back is not heard,
// and the right one lets the call run. Where its questions go is not in
// the environment of what it runs. gnosh agents --json says blocked while
// it waits.
func TestRunAsksTheAgentView(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo \"[$GNOSH_RUN_ASKS]\" > env.txt; touch made.txt"}`}}},
		apptest.Reply{Text: "Made it."})
	base := filepath.Join(w.Root, "run")
	o := testOptions(w, "-p", "make the file")
	o.Environ = append(o.Environ, AsksEnv+"="+base)
	done := make(chan result, 1)
	go func() { done <- runWith(t, context.Background(), o) }()

	var ask runAsk
	deadline := time.Now().Add(30 * time.Second)
	for {
		data, err := os.ReadFile(base + ".ask")
		if err == nil && json.Unmarshal(data, &ask) == nil && ask.ID != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the run put no question in its .ask file")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ask.Title != "Allow Bash?" || !strings.Contains(ask.Text, "touch made.txt") || len(ask.Token) != 32 || len(ask.Choices) < 2 || ask.Choices[0].Key != "yes" {
		t.Fatalf("the question: %+v", ask)
	}
	// A forged answer, without the token: not heard.
	forged, _ := json.Marshal(runAnswer{ID: ask.ID, Choice: "yes"})
	if err := os.WriteFile(base+".answer", forged, 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * askLook)
	if w.Exists("made.txt") {
		t.Fatal("an answer without the question's token let the call run")
	}
	if _, err := os.Stat(base + ".ask"); err != nil {
		t.Fatalf("the question went with the forged answer: %v", err)
	}
	if err := answerAsk(base+".log", ask.ID, "yes", ""); err != nil {
		t.Fatal(err)
	}
	var r result
	select {
	case r = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the run did not end after its question was answered")
	}
	r.wantCode(t, 0)
	if !w.Exists("made.txt") {
		t.Errorf("the answered call did not run")
	}
	if got, _ := os.ReadFile(filepath.Join(w.Dir, "env.txt")); strings.TrimSpace(string(got)) != "[]" {
		t.Errorf("the command saw where the run's questions go: %q", got)
	}
	has(t, "stderr", r.err, "waiting for an answer in gnosh agents: Allow Bash?")
	if _, err := os.Stat(base + ".ask"); !os.IsNotExist(err) {
		t.Errorf("the .ask file is still there after the answer: %v", err)
	}
	if err := answerAsk(base+".log", ask.ID, "yes", ""); err == nil {
		t.Errorf("a question answered already took another answer")
	}
}

// A run's state is blocked while its question waits, and the question's
// title is its status, in gnosh agents --json; gnosh --bg tells the run
// where its questions go.
func TestBlockedRunInTheList(t *testing.T) {
	w := apptest.NewWorld(t)
	var log string
	o := testOptions(w, "--bg", "clean the build")
	o.Detach = func(_, env []string, l string) (int, error) {
		log = l
		want := AsksEnv + "=" + strings.TrimSuffix(l, ".log")
		found := false
		for _, kv := range env {
			found = found || kv == want
		}
		if !found {
			t.Errorf("the run's environment does not say where its questions go")
		}
		return 4242, nil
	}
	runWith(t, context.Background(), o).wantCode(t, 0)
	id := strings.TrimSuffix(filepath.Base(log), ".background.log")
	// The run's process: one whose command line names its session.
	proc := exec.Command("sh", "-c", `trap "exit 0" TERM; while :; do sleep 0.1; done`, id)
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proc.Process.Kill(); proc.Wait() })
	if err := os.WriteFile(pidFile(log), []byte(strconv.Itoa(proc.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	has(t, "gnosh agents --json before", run(t, w, "agents", "--json").wantCode(t, 0).out, `"state": "working"`)
	ask, _ := json.Marshal(runAsk{ID: "q1", Token: strings.Repeat("ab", 16), Title: "Allow Bash?", Text: "  $ rm -rf build",
		Choices: []askChoice{{Key: "yes", Label: "Yes, this once"}, {Key: "no", Label: "No"}}})
	if err := os.WriteFile(askFile(log), ask, 0o600); err != nil {
		t.Fatal(err)
	}
	has(t, "gnosh agents --json", run(t, w, "agents", "--json").wantCode(t, 0).out, `"state": "blocked"`, `"status": "Allow Bash?"`)
	if err := answerAsk(log, "q1", "maybe", ""); err == nil || !strings.Contains(err.Error(), "is not one of the question's answers") {
		t.Errorf("an answer the question does not offer: %v", err)
	}
	if err := answerAsk(log, "q1", "no", "use git clean"); err != nil {
		t.Fatal(err)
	}
	var a runAnswer
	data, _ := os.ReadFile(answerFile(log))
	if json.Unmarshal(data, &a) != nil || a.ID != "q1" || a.Token != strings.Repeat("ab", 16) || a.Choice != "no" || a.Text != "use git clean" {
		t.Errorf("the answer written: %s", data)
	}
}

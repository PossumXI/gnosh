package app

import (
	"context"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// waitFor waits until ok says yes, and fails the test when it does not
// within ten seconds.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// userTexts returns the text of the user messages of a request, in order.
func userTexts(req apptest.Request) []string {
	var out []string
	for _, m := range req.Messages() {
		if m["role"] == "user" {
			out = append(out, apptest.Text(m))
		}
	}
	return out
}

// -c goes on with the folder's latest session, and -r with the one that is
// named: the model is sent the earlier messages again, byte for byte, with
// the new prompt after them, and everything lands in the one file.
func TestContinueAndResume(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Text: "I will remember 42."},
		apptest.Reply{Text: "It was 42."},
		apptest.Reply{Text: "Still 42."},
		apptest.Reply{Text: "42, in a copy."})
	first := run(t, w, "-p", "--output-format", "json", "remember the number 42").wantCode(t, 0)
	id := jsonLines(t, first.out)[0]["session_id"].(string)

	second := run(t, w, "-c", "-p", "--output-format", "json", "what was the number?").wantCode(t, 0)
	if got := jsonLines(t, second.out)[0]["session_id"]; got != id {
		t.Errorf("-c went on in session %v, want %s", got, id)
	}
	reqs := w.Model.Requests()
	end := strings.Index(string(reqs[0].Body), `],"model":`)
	if !strings.HasPrefix(string(reqs[1].Body), string(reqs[0].Body)[:end]) {
		t.Errorf("the request of the continued session does not begin with the bytes of the first run's request")
	}
	users := userTexts(reqs[1])
	if len(users) != 2 || !strings.Contains(users[0], "remember the number 42") || users[1] != "what was the number?" {
		t.Errorf("user messages of the continued session: %q", users)
	}
	has(t, "the continued request", string(reqs[1].Body), "I will remember 42.")

	// By the start of its id.
	third := run(t, w, "-r", id[:8], "-p", "--output-format", "json", "and now?").wantCode(t, 0)
	if got := jsonLines(t, third.out)[0]["session_id"]; got != id {
		t.Errorf("-r went on in session %v, want %s", got, id)
	}
	if users := userTexts(w.Model.Requests()[2]); len(users) != 3 || users[2] != "and now?" {
		t.Errorf("user messages of the resumed session: %q", users)
	}
	if s := w.Sessions(); len(s) != 1 {
		t.Fatalf("session files: %v", s)
	}
	checkSession(t, w, w.Sessions()[0])

	// --fork-session goes on in a copy and leaves the original as it was.
	before, _ := os.ReadFile(w.Sessions()[0])
	fork := run(t, w, "-r", id, "--fork-session", "-p", "--output-format", "json", "once more").wantCode(t, 0)
	forkID := jsonLines(t, fork.out)[0]["session_id"].(string)
	if forkID == id {
		t.Errorf("--fork-session went on in the original session")
	}
	if users := userTexts(w.Model.Requests()[3]); len(users) != 4 || users[3] != "once more" {
		t.Errorf("user messages of the copy: %q", users)
	}
	after, _ := os.ReadFile(strings.Replace(w.Sessions()[0], forkID, id, 1))
	if string(before) != string(after) {
		t.Errorf("the original session's file changed when a copy of it was continued")
	}
	w.Model.Done()

	// A session that is not there is said so, and nothing is sent.
	r := run(t, w, "-r", "no-such-session", "-p", "hello").wantCode(t, 1)
	has(t, "stderr", r.err, "no-such-session", "nothing was started")
}

// -c in a folder with no session says so plainly.
func TestContinueWithNothingToContinue(t *testing.T) {
	w := apptest.NewWorld(t)
	r := run(t, w, "-c", "-p", "hello").wantCode(t, 1)
	has(t, "stderr", r.err, "there is no session to continue")
	if n := len(w.Model.Requests()); n != 0 {
		t.Errorf("the model was sent %d requests", n)
	}
}

// A session that is killed while a tool runs loses nothing that was
// written: the command it started is stopped with it, and when the
// session is opened again the unfinished call is closed, so that the
// model is sent a whole conversation and carries on.
func TestKilledMidToolAndResumed(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Text: "Starting the long job.", Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo $$ > pid.txt; sleep 300"}`}}},
		apptest.Reply{Text: "Picking up where it stopped."})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan result, 1)
	go func() { done <- runWith(t, ctx, testOptions(w, "-y", "-p", "run the long job")) }()
	waitFor(t, "the command to start", func() bool { return w.Exists("pid.txt") && strings.TrimSpace(w.Read("pid.txt")) != "" })
	pid, err := strconv.Atoi(strings.TrimSpace(w.Read("pid.txt")))
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	cancel()
	var r result
	select {
	case r = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the program did not end when it was killed")
	}
	if r.code == 0 {
		t.Errorf("a killed run exited 0")
	}
	if took := time.Since(started); took > 15*time.Second {
		t.Errorf("the program took %s to end", took)
	}
	// Nothing is left running.
	waitFor(t, "the command to be stopped", func() bool { return syscall.Kill(pid, 0) == syscall.ESRCH })

	// What was written before the kill is all there: the prompt, and the
	// reply with its call.
	sessions := w.Sessions()
	if len(sessions) != 1 {
		t.Fatalf("session files: %v", sessions)
	}
	raw, _ := os.ReadFile(sessions[0])
	has(t, "the session's file", string(raw), "run the long job", "Starting the long job.", `"tool_use"`, "sleep 300")

	// Opened again, the conversation is whole: the call has a result that
	// says it was cut off, and the new prompt follows.
	run(t, w, "-c", "-y", "-p", "go on").wantCode(t, 0)
	reqs := w.Model.Requests()
	if len(reqs) != 2 {
		t.Fatalf("the model was sent %d requests, want 2", len(reqs))
	}
	if names, texts := toolResults(reqs[1]); len(names) != 1 || names[0] != "c1" {
		t.Errorf("the call that was cut off has no result in the resumed conversation: %q %q", names, texts)
	}
	users := userTexts(reqs[1])
	if len(users) == 0 || !strings.Contains(users[len(users)-1], "go on") {
		t.Errorf("the new prompt is not the last user message: %q", users)
	}
	checkSession(t, w, sessions[0])
	w.Model.Done()
}

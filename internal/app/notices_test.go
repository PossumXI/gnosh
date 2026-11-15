package app

import (
	"context"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// In a session a person attends, the end of a command that runs in the
// background is told to the model without anybody asking: the session
// wakes, the model reads how the command ended, and answers.
func TestBackgroundCommandEndWakesTheSession(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"sleep 0.4; echo finished > bg.txt; echo the job is done","run_in_background":true}`}}},
		apptest.Reply{Text: "It runs in the background now."},
		apptest.Reply{Text: "The background job has finished."})
	s := sit(t, w, "-y")
	s.see("Type a prompt and Enter.")
	s.typeLine("start the job")
	s.see("It runs in the background now.")
	s.see("-- done in ")
	// Nobody types anything: the end of the command starts the next turn.
	s.see("A background command ended")
	s.see("The background job has finished.")
	if !w.Exists("bg.txt") {
		t.Errorf("the background command did not run to its end")
	}
	third := requestText(t, w, 2)
	has(t, "what the model read about the command's end", third, "bg1", "exit code 0")
	s.typeLine("/exit")
	s.exit()
	w.Model.Done()
	checkSession(t, w, w.Sessions()[0])
}

// /tasks lists the session's background commands, running and ended, and
// stops one by its id.
func TestTasksCommand(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"sleep 60","run_in_background":true}`}}},
		apptest.Reply{Text: "It sleeps in the background."},
		apptest.Reply{Text: "Noted that it was stopped."})
	s := sit(t, w, "-y")
	s.see("Type a prompt and Enter.")
	s.typeLine("start a long job")
	s.see("It sleeps in the background.")
	s.see("-- done in ")
	s.typeLine("/tasks")
	s.see("bg1  running for ")
	s.see(": sleep 60")
	s.see("its output: ")
	s.see("/tasks stop <id> stops one.")
	s.typeLine("/tasks stop bg1")
	s.see("bg1 is stopped: sleep 60")
	s.typeLine("/tasks")
	s.see("bg1  killed by ")
	s.typeLine("/exit")
	s.exit()
}

// A run waits for the background work its turn started (spec/cc/cli.md
// 4.5): the command's end wakes the model, the turn that starts is the
// run's too, and the run ends when nothing is left running. Text output
// prints each turn's reply; JSON output reports the last turn.
func TestRunWaitsForItsBackgroundWork(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"sleep 1; touch late.txt; echo the job is done","run_in_background":true}`}}},
		apptest.Reply{Text: "Started."},
		apptest.Reply{Text: "The job is done."})
	r := run(t, w, "-y", "-p", "start it").wantCode(t, 0)
	if !w.Exists("late.txt") {
		t.Errorf("the run did not wait for its background command")
	}
	if r.out != "Started.\n\nThe job is done.\n" {
		t.Errorf("the run printed %q", r.out)
	}
	has(t, "what the model read", requestText(t, w, 2), "A background command has ended", "bg1", "exit code 0")
	w.Model.Done()

	w.Model.Add(
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Bash", Args: `{"command":"sleep 1; echo again","run_in_background":true}`}}},
		apptest.Reply{Text: "Started again."},
		apptest.Reply{Text: "Done again."})
	r = run(t, w, "-y", "-p", "--output-format", "json", "start it again").wantCode(t, 0)
	has(t, "the JSON result", r.out, `"result":"Done again."`)
	hasNot(t, "the JSON result", r.out, "Started again.")
}

// A ceiling set with CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS stops what is
// left once the run has waited that long, and the run ends.
func TestRunWaitCeiling(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"sleep 60; touch late.txt","run_in_background":true}`}}},
		apptest.Reply{Text: "Started."})
	o := testOptions(w, "-y", "-p", "start it")
	o.Environ = append(o.Environ, "CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=500")
	began := time.Now()
	r := runWith(t, context.Background(), o).wantCode(t, 0)
	if took := time.Since(began); took > 30*time.Second {
		t.Errorf("the run took %v", took)
	}
	has(t, "stderr", r.err, "waited", "for background work and stops it now (CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS): bg1")
	w.Model.Done()
	time.Sleep(time.Second)
	if w.Exists("late.txt") {
		t.Errorf("the background command outlived the run")
	}
}

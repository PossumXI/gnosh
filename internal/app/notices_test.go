package app

import (
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

// A run that nobody attends is not woken by a background command, and
// leaves none behind: when the run ends, what it started is stopped.
func TestRunLeavesNoBackgroundCommandBehind(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"sleep 1.5; touch late.txt","run_in_background":true}`}}},
		apptest.Reply{Text: "Started."})
	run(t, w, "-y", "-p", "start it").wantCode(t, 0)
	w.Model.Done()
	time.Sleep(2 * time.Second)
	if w.Exists("late.txt") {
		t.Errorf("a command the run started in the background was still running after the run ended")
	}
}

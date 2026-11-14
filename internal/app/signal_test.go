package app

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// started is a run of the program that is under way, for a test that sends
// it a signal.
type started struct {
	t        *testing.T
	out, err *safeBuffer
	signals  chan os.Signal
	done     chan int
}

// startRun starts the program on a goroutine, with signals it can be sent.
func startRun(t *testing.T, o Options) *started {
	t.Helper()
	s := &started{t: t, out: &safeBuffer{}, err: &safeBuffer{}, signals: make(chan os.Signal, 4), done: make(chan int, 1)}
	o.Stdout, o.Stderr, o.Signals = s.out, s.err, s.signals
	go func() { s.done <- Main(context.Background(), o) }()
	return s
}

// end waits for the program to end and returns how it ended.
func (s *started) end() result {
	s.t.Helper()
	select {
	case code := <-s.done:
		return result{code: code, out: s.out.String(), err: s.err.String()}
	case <-time.After(patience):
		s.t.Fatalf("the program did not end.\nstdout:\n%s\nstderr:\n%s", s.out.String(), s.err.String())
	}
	return result{}
}

// commandPid waits for the command of a turn to have written its process
// id to a file, and returns it.
func commandPid(t *testing.T, w *apptest.World, name string) int {
	t.Helper()
	deadline := time.Now().Add(patience)
	for {
		if w.Exists(name) {
			if pid, err := strconv.Atoi(strings.TrimSpace(w.Read(name))); err == nil {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("the command never wrote %s", name)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// processGone waits until no process has the id any more.
func processGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(patience)
	for syscall.Kill(pid, 0) != syscall.ESRCH {
		if time.Now().After(deadline) {
			t.Fatalf("the process %d is still there", pid)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A run that a signal stops still says, in the two JSON formats, what it
// was: the result object, with the replies the model had written, what
// they cost, and that the run was stopped and by which signal. Then it
// exits with the signal's code. The text format prints nothing more.
func TestRunStoppedBySignalPrintsItsResult(t *testing.T) {
	job := func() apptest.Reply {
		return apptest.Reply{Text: "Starting.", Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo $$ > pid.txt; sleep 300"}`}}, In: 100, Out: 7}
	}
	tests := []struct {
		format string
		sig    os.Signal
		code   int
		word   string
	}{
		{"json", syscall.SIGTERM, 143, "SIGTERM"},
		{"stream-json", syscall.SIGTERM, 143, "SIGTERM"},
		{"json", os.Interrupt, 130, "an interrupt (SIGINT)"},
		{"stream-json", os.Interrupt, 130, "an interrupt (SIGINT)"},
	}
	for _, tt := range tests {
		t.Run(tt.format+" "+tt.word, func(t *testing.T) {
			w := apptest.NewWorld(t, job())
			s := startRun(t, testOptions(w, "-y", "-p", "--output-format", tt.format, "run the long job"))
			pid := commandPid(t, w, "pid.txt")
			s.signals <- tt.sig
			r := s.end().wantCode(t, tt.code)
			processGone(t, pid)

			lines := jsonLines(t, r.out)
			if len(lines) == 0 {
				t.Fatalf("nothing was printed:\n%s", r.err)
			}
			obj := lines[len(lines)-1]
			if tt.format == "json" && len(lines) != 1 {
				t.Errorf("the json format printed %d objects, want the result alone:\n%s", len(lines), r.out)
			}
			if tt.format == "stream-json" && (lines[0]["subtype"] != "init" || len(lines) < 3) {
				t.Errorf("the stream does not begin with its first line and hold the reply:\n%s", r.out)
			}
			errs, _ := obj["errors"].([]any)
			if obj["type"] != "result" || obj["subtype"] != "error_during_execution" || obj["is_error"] != true ||
				obj["terminal_reason"] != "aborted_tools" || obj["num_turns"] != float64(1) || len(errs) != 1 {
				t.Fatalf("the result object: %v", obj)
			}
			if want := "the run was stopped by " + tt.word + " after 1 turns of the model"; errs[0] != want {
				t.Errorf("errors = %q, want %q", errs[0], want)
			}
			if _, has := obj["result"]; has {
				t.Errorf("a run that was stopped has an answer: %v", obj["result"])
			}
			if id, _ := obj["session_id"].(string); id == "" || !strings.HasSuffix(w.Sessions()[0], id+".jsonl") {
				t.Errorf("session_id = %v, and the session's file is %s", obj["session_id"], w.Sessions()[0])
			}
			usage, _ := obj["usage"].(map[string]any)
			if usage["input_tokens"] != float64(100) || usage["output_tokens"] != float64(7) {
				t.Errorf("usage = %v, want what the one reply cost", usage)
			}
			checkSession(t, w, w.Sessions()[0])
		})
	}

	t.Run("text prints nothing more", func(t *testing.T) {
		w := apptest.NewWorld(t, job())
		s := startRun(t, testOptions(w, "-y", "-p", "run the long job"))
		pid := commandPid(t, w, "pid.txt")
		s.signals <- syscall.SIGTERM
		r := s.end().wantCode(t, 143)
		processGone(t, pid)
		if r.out != "Starting.\n" && r.out != "" {
			t.Errorf("stdout after SIGTERM: %q", r.out)
		}
		hasNot(t, "the output", r.out+r.err, `"type":"result"`, "context canceled")
	})

	t.Run("stopped while the model was writing", func(t *testing.T) {
		hold := make(chan struct{})
		defer close(hold)
		arrived := make(chan struct{})
		var once sync.Once
		w := apptest.NewWorld(t, apptest.Reply{Text: "This answer never ends, because the model is held.", Hold: hold,
			Arrived: func() { once.Do(func() { close(arrived) }) }})
		s := startRun(t, testOptions(w, "-p", "--output-format", "json", "tell me a long story"))
		select {
		case <-arrived:
		case <-time.After(patience):
			t.Fatal("the request never reached the model")
		}
		s.signals <- syscall.SIGTERM
		r := s.end().wantCode(t, 143)
		lines := jsonLines(t, r.out)
		if len(lines) != 1 || lines[0]["subtype"] != "error_during_execution" || lines[0]["terminal_reason"] != "aborted_streaming" || lines[0]["is_error"] != true {
			t.Errorf("the result of a run stopped while the model wrote:\n%s", r.out)
		}
		errs, _ := lines[0]["errors"].([]any)
		if len(errs) != 1 || !strings.Contains(errs[0].(string), "the run was stopped by SIGTERM after ") {
			t.Errorf("errors = %v", errs)
		}
	})
}

// A run that reaches its own cap of turns prints its result in the stream
// format too, as its last line.
func TestRunAtItsCapPrintsItsResultInAStream(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		return &apptest.Reply{Calls: []apptest.Call{{ID: "c" + strconv.Itoa(n), Name: "Bash", Args: `{"command":"true"}`}}}
	})
	r := run(t, w, "-y", "-p", "--max-turns", "2", "--output-format", "stream-json", "loop").wantCode(t, 1)
	lines := jsonLines(t, r.out)
	obj := lines[len(lines)-1]
	if obj["type"] != "result" || obj["subtype"] != "error_max_turns" || obj["is_error"] != true || obj["num_turns"] != float64(2) || obj["terminal_reason"] != "max_turns" {
		t.Errorf("the last line of the stream: %v", obj)
	}
}

// Only the first signal is waited on. A second one, while the run is still
// winding up and before its result is written, ends the program at once.
func TestSecondSignalDoesNotWaitForTheResult(t *testing.T) {
	// A command that takes no notice of being asked to stop: the turn
	// then takes its whole grace to end, and the result waits on it.
	w := apptest.NewWorld(t, apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"trap '' TERM INT HUP; echo $$ > pid.txt; sleep 300"}`}}})
	o := testOptions(w, "-y", "-p", "--output-format", "json", "run the stubborn job")
	type exit struct {
		code int
		at   time.Time
		out  string
	}
	exits := make(chan exit, 1)
	var s *started
	o.Exit = func(code int) {
		select {
		case exits <- exit{code, time.Now(), s.out.String()}:
		default:
		}
	}
	s = startRun(t, o)
	pid := commandPid(t, w, "pid.txt")
	// Whatever the program leaves behind, the test does not.
	defer syscall.Kill(-pid, syscall.SIGKILL)
	s.signals <- syscall.SIGTERM
	time.Sleep(50 * time.Millisecond)
	second := time.Now()
	s.signals <- syscall.SIGTERM
	select {
	case e := <-exits:
		if e.code != 143 {
			t.Errorf("the second signal ended the program with %d, want 143", e.code)
		}
		if took := e.at.Sub(second); took > 1500*time.Millisecond {
			t.Errorf("the second signal took %s to end the program", took)
		}
		if strings.Contains(e.out, `"type":"result"`) {
			t.Logf("the result had been written when the second signal came: %s", e.out)
		}
	case <-time.After(patience):
		t.Fatal("the second signal did not end the program")
	}
	// In a test the program cannot really be ended: it goes on to its own
	// end, which is the first signal's.
	s.end().wantCode(t, 143)
}

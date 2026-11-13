package memjoin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The turn lock, with two sessions on one project. A session holds its
// turn lock from when a prompt makes it busy until it goes idle, and that
// is how other sessions tell that it is working: a plan it stands in is
// held against them for as long as it works, and no longer.
func TestTurnLockAndThePlanAnotherSessionHolds(t *testing.T) {
	const task = "read-notes"
	w := newWorld(t)
	w.Write("notes.txt", "hello\n")
	arrived, release := make(chan struct{}), make(chan struct{})

	// The first session plans a task, does its one todo, and is then kept
	// waiting by its model: it is busy, and stands in the task.
	slow := says("The notes say hello.")
	slow.Arrived = func() { close(arrived) }
	slow.Hold = release
	w.script.add(
		reply(memCall("a1", `mem.Todo("read the notes", "read notes.txt").Push()`)),
		reply(memCall("a2", `mem.Next()`)),
		reply(call("a3", "Read", map[string]any{"file_path": "notes.txt"})),
		reply(memCall("a4", `mem.Pop("notes.txt says hello")`)),
		slow,
		// The second session, meanwhile.
		reply(memCall("b1", `mem.Done("`+task+`", "its todo is done: notes.txt says hello")`)),
		says("The task is another session's."),
	)
	first := make(chan result, 1)
	go func() { first <- w.runWith(w.options("-y", "-p", "read the notes")) }()
	<-arrived

	// While it works, its turn lock is held, and its plan cannot be
	// closed from another session.
	locks, _ := filepath.Glob(filepath.Join(w.gnoDir(), "turns", "*.lock"))
	if len(locks) != 1 {
		t.Errorf("turn locks while one session works: %v, want one", locks)
	}
	second := w.run("-y", "-p", "close the task if it is done")
	second.wantCode(t, 0)
	reqs := w.script.requests()
	held := lastResult(t, reqs[len(reqs)-1])
	has(t, "closing a plan that a working session stands in", held, "ERROR — nothing from this call was kept: `"+task+"`", " is held by ")
	hasNot(t, "closing a plan that a working session stands in", held, "done `"+task+"`")

	// The first session's turn ends; it goes idle and lets its lock go.
	close(release)
	(<-first).wantCode(t, 0)

	// Now the plan's hold has lapsed: a todo with todos under it is held
	// only while its session works. Another session closes it.
	w.script.add(
		reply(memCall("c1", `mem.Done("`+task+`", "its todo is done: notes.txt says hello")`)),
		says("Closed."),
	)
	third := w.run("-y", "-p", "close the task if it is done")
	third.wantCode(t, 0)
	reqs = w.script.requests()
	has(t, "closing the plan once its session is idle", lastResult(t, reqs[len(reqs)-1]), "done `"+task+"` \"read the notes\"")
	w.script.done()

	// The task was closed by the session's own call, so it was judged as
	// the turn ended, and the person was told.
	has(t, "stderr of the third run", third.err, "Task done: \"read the notes\" (`"+task+"`): the conversation shows it")
	if n := len(w.script.sides(sideJudge)); n != 1 {
		t.Errorf("the judge was asked %d times, want once: for the task the third session closed", n)
	}

	// The three sessions each have a file, and nothing was started
	// outside the world.
	if n := len(w.Sessions()); n != 3 {
		t.Errorf("%d session files, want 3", n)
	}
	if _, err := os.Stat(filepath.Join(w.gnoDir(), "worker.log")); err == nil {
		t.Errorf("a worker was started")
	}
}

// Where a sub-agent's mem.Next() answers at once (TestSubAgentsAndMem),
// another session's waits. The same plan and the same moment: one session
// holds a todo of a group and is at work, and a second session has just
// done the other todo of the group and asks for the next. A session a
// person is in waits for the one at work, since the group's own work
// could be its own once that one is done; when the session at work goes
// idle, the wait ends and the waiter is told that nothing is free. It is
// the sub-agent flag, which only a sub-agent's requests carry, that makes
// the same call answer at once there.
func TestNextOfAnotherSessionWaitsForTheOneAtWork(t *testing.T) {
	const (
		group  = "build-shapes"
		circle = "build-shapes/circle"
		square = "build-shapes/square"
	)
	w := newWorld(t)
	w.Write("square.txt", "+--+\n|  |\n|  |\n+--+\n")
	arrived, release := make(chan struct{}), make(chan struct{})
	slow := says("The circle is drawn.")
	slow.Arrived = func() { close(arrived) }
	slow.Hold = release
	w.script.add(
		// The first session: the plan, a step into the circle, and work
		// that takes a while.
		reply(memCall("a1", `mem.Todo("build the shapes", "circle", "square").Parallel()`)),
		reply(memCall("a2", `mem.Next()`)),
		slow,
		// The second session, meanwhile: the square, and then the next.
		reply(memCall("b1", `mem.Next()`)),
		reply(call("b2", "Read", map[string]any{"file_path": "square.txt"})),
		reply(memCall("b3", "mem.Pop(\"square.txt holds a square of four lines\")\nmem.Next()")),
		says("The square is done."),
	)
	first := make(chan result, 1)
	go func() { first <- w.runWith(w.options("-y", "-p", "build the shapes")) }()
	<-arrived

	second := make(chan result, 1)
	go func() { second <- w.runWith(w.options("-y", "-p", "take the next todo")) }()
	// The second session's third call reaches mem, pops the square, and
	// then waits: its result does not come while the first session works.
	deadline := time.Now().Add(60 * time.Second)
	for len(w.script.requests()) < 6 {
		if time.Now().After(deadline) {
			t.Fatalf("the second session made %d requests in a minute, want 3 (after the first session's 3)", len(w.script.requests())-3)
		}
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case r := <-second:
		t.Fatalf("the second session ended while the first was at work: its mem.Next() did not wait.\nstdout:\n%s\nstderr:\n%s", r.out, r.err)
	case <-time.After(1500 * time.Millisecond):
	}
	if n := len(w.script.requests()); n != 6 {
		t.Fatalf("%d requests were made while the second session's call should have been waiting, want 6", n)
	}
	for i, req := range w.memRequests() {
		if req.SubAgent {
			t.Errorf("mem call %d of a session a person is in carries the sub-agent flag", i)
		}
	}

	// The first session goes idle: the wait ends, and the second session
	// reads that nothing is free.
	close(release)
	(<-first).wantCode(t, 0)
	(<-second).wantCode(t, 0)
	w.script.done()
	reqs := w.script.requests()
	has(t, "what the waiting session read when the wait ended", lastResult(t, reqs[len(reqs)-1]),
		"done `"+square+"` \"square\" → square.txt holds a square of four lines",
		"`"+circle+"` is held by session ")
}

// mem's digest reaches a resumed session again when mem was changed
// elsewhere since the session last spoke, and not otherwise.
func TestDigestAgainWhenMemChangedElsewhere(t *testing.T) {
	w := newWorld(t)

	// The first session speaks an hour ago, by the clock it is given.
	w.script.add(says("Hello."))
	o := w.options("-p", "hello")
	o.Now = clockAt(-1)
	w.runWith(o).wantCode(t, 0)
	sessionOne := strings.TrimSuffix(filepath.Base(w.Sessions()[0]), ".jsonl")
	first := w.script.requests()[0]
	hasNot(t, "the first prompt of a session in a project with no mem", lastUser(t, first), "<mem>")

	// Another session keeps a fact, now.
	w.script.add(
		reply(memCall("f1", `mem.Fact("cmd/test", "go test -race ./...")`)),
		says("Kept."),
	)
	w.run("-y", "-p", "keep the test command").wantCode(t, 0)

	// The first session is taken up again: its next typed prompt carries
	// the digest, after its words and before the mem line.
	w.script.add(says("I see the fact."))
	o = w.options("-r", sessionOne, "-p", "anything new?")
	o.Now = clockAt(1)
	w.runWith(o).wantCode(t, 0)
	reqs := w.script.requests()
	again := lastUser(t, reqs[len(reqs)-1])
	has(t, "the prompt typed after mem changed elsewhere", again, "<mem>", "`cmd/test` = go test -race ./...", "</mem>")
	if words, digest := strings.Index(again, "anything new?"), strings.Index(again, "<mem>"); words < 0 || digest < words {
		t.Errorf("the digest does not follow the prompt's words:\n%s", again)
	}

	// Taken up once more, with mem as it was: no digest.
	w.script.add(says("Nothing new."))
	o = w.options("-r", sessionOne, "-p", "and now?")
	o.Now = clockAt(2)
	w.runWith(o).wantCode(t, 0)
	reqs = w.script.requests()
	msgs := messagesOf(reqs[len(reqs)-1])
	last := msgs[len(msgs)-1].text
	if !strings.HasPrefix(last, "and now?") || strings.Contains(last, "<mem>") {
		t.Errorf("a prompt typed with mem unchanged carries a digest:\n%s", last)
	}
	w.script.done()
}

// clockAt is a clock that tells the present moment moved by a number of
// hours, so that a test can make a session speak earlier or later than
// mem was changed without waiting.
func clockAt(hours int) func() time.Time {
	return func() time.Time { return time.Now().Add(time.Duration(hours) * time.Hour) }
}

package shelltool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

func TestCloseStopsEverything(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)
	live := newCollector()
	env.Progress = live.take

	// Two foreground commands in flight, two background ones, and one that
	// has ended but left a process holding its output.
	left := bash(t, m, env, `sleep 300 & echo "left=$!"`)
	background := make(map[string]int)
	var ids []string
	for i := range 2 {
		id, _ := startBackground(t, m, env, fmt.Sprintf(`echo "bg%d=$$"; echo ready; sleep 300 & sleep 300 & wait`, i))
		ids = append(ids, id)
		for name, pid := range pidsIn(readUntil(t, m, env, id, "ready\n")) {
			background[name] = pid
		}
	}
	first := begin(t, t.Context(), m, BashName, env, args{argCommand: tree, argTimeout: 600_000})
	second := begin(t, t.Context(), m, BashName, env, args{argCommand: `echo "other=$$"; echo other-is-up; sleep 300`, argTimeout: 600_000})
	live.waitFor(t, "ready\n")
	live.waitFor(t, "other-is-up\n")

	start := time.Now()
	if err := m.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	took := time.Since(start)

	// The calls in flight come back and say what happened.
	all := pidsIn(printed(left))
	for _, done := range []<-chan core.ToolResult{first, second} {
		res := <-done
		headline, body, _ := parts(res)
		if !res.IsError || !strings.Contains(headline, "because the session is ending") || res.Meta[metaReason] != ReasonClosed {
			t.Errorf("a call in flight at Close came back with %q, Meta %v", headline, res.Meta)
		}
		for name, pid := range pidsIn(body) {
			all[name] = pid
		}
	}
	for name, pid := range background {
		all[name] = pid
	}
	if len(all) != 7 {
		t.Fatalf("pids = %v, want seven", all)
	}
	// Close waits: by the time it returns, nothing is left. (Only Linux
	// lets the test tell a process that is dead and not yet reaped from a
	// live one at once; elsewhere it gives the reaper a moment.)
	if runtime.GOOS != "linux" {
		allGone(t, all)
	}
	for name, pid := range all {
		if alive(pid) {
			t.Errorf("%s (pid %d) is still alive after Close returned", name, pid)
		}
	}
	if took > m.opt.TermGrace+5*time.Second {
		t.Errorf("Close took %v although everything dies on SIGTERM", took)
	}
	if _, err := os.Stat(m.dir); !os.IsNotExist(err) {
		t.Errorf("the output directory %s is still there after Close: %v", m.dir, err)
	}
	for _, task := range m.Tasks() {
		if task.Running || task.Reason != ReasonClosed {
			t.Errorf("after Close: %+v", task)
		}
	}
	if n := news.count(); n != 0 {
		t.Errorf("OnEnd was called %d times for commands that Close stopped", n)
	}

	// A closed manager runs nothing, and says so as the harness's error.
	_, err := toolNamed(t, m, BashName).Run(t.Context(), core.ToolCall{Input: json.RawMessage(`{"command": "echo late"}`)}, env)
	if !errors.Is(err, errClosed) {
		t.Errorf("Run after Close: %v, want the closed error", err)
	}
	// What is left of the tasks can still be asked about.
	res := run(t, t.Context(), m, StopName, env, args{argTaskID: ids[0]})
	if text := core.TextOf(res.Content); !strings.Contains(text, "had already ended: it was stopped after") {
		t.Errorf("TaskStop after Close: %q", text)
	}
	if err := m.Close(); err != nil {
		t.Errorf("a second Close: %v", err)
	}
}

// TestNothingIsLeftOpen runs some dozens of commands of every kind through
// one manager and checks that, once it is closed, the test binary holds no
// more open files than before. It is not parallel: other tests' files would
// be counted.
func TestNothingIsLeftOpen(t *testing.T) {
	// list names the files this process has open; the one that reading the
	// list itself opens is among them, each time.
	list := func() []string {
		entries, err := os.ReadDir("/dev/fd")
		if err != nil {
			t.Skipf("this system does not list open files: %v", err)
		}
		var names []string
		for _, entry := range entries {
			target, err := os.Readlink("/dev/fd/" + entry.Name())
			if err != nil {
				target = "(closed again)"
			}
			names = append(names, entry.Name()+" -> "+target)
		}
		return names
	}
	openFiles := func() int { return len(list()) }
	env := newEnv(t)
	// The Go runtime opens a few files of its own the first time a pipe or
	// a process is used. One command through a manager that is then closed
	// gets that out of the way before the count is taken.
	warm := newManager(t, Options{})
	bash(t, warm, env, "true")
	if err := warm.Close(); err != nil {
		t.Fatal(err)
	}
	before := openFiles()
	m := newManager(t, Options{MaxOutputChars: 200, PipeGrace: 20 * time.Millisecond, TermGrace: 200 * time.Millisecond})
	live := newCollector()
	env.Progress = live.take
	for i := range 15 {
		// One that simply runs, one whose output is cut and kept in a file,
		// one that fails to start a program, one that is stopped at its
		// timeout, one that leaves a process behind, and a background one.
		bash(t, m, env, "echo plain")
		bash(t, m, env, "seq 1 200")
		bash(t, m, env, "no-such-program-here")
		run(t, t.Context(), m, BashName, env, args{argCommand: "sleep 300", argTimeout: 20})
		if i%5 == 0 {
			bash(t, m, env, "sleep 300 & echo left")
			startBackground(t, m, env, "echo background; sleep 300")
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for openFiles() > before && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if after := list(); len(after) != before {
		t.Errorf("%d files are open after the manager was closed, %d before it was made:\n%s", len(after), before, strings.Join(after, "\n"))
	}
}

func TestCloseWithNothingStarted(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	m := New(Options{SpoolDir: parent})
	if err := m.Close(); err != nil {
		t.Error(err)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("a manager that ran nothing left files: %v", entries)
	}
	// The parent directory is the caller's and stays.
	if _, err := os.Stat(parent); err != nil {
		t.Error(err)
	}
}

func TestCloseRacesWithStart(t *testing.T) {
	t.Parallel()
	// Commands being started while the manager closes must either be
	// refused or be stopped: none may slip through and run on. The check
	// at the end of the test run would find one that did.
	for round := range 5 {
		m := newManager(t, Options{})
		env := newEnv(t)
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				input := mustJSON(t, args{argCommand: "sleep 300", argBackground: i%2 == 0, argTimeout: 600_000})
				res, err := toolNamed(t, m, BashName).Run(t.Context(), core.ToolCall{ID: fmt.Sprint(round, i), Input: input}, env)
				if err != nil && !errors.Is(err, errClosed) {
					t.Errorf("Run: %v", err)
				}
				if err == nil && i%2 != 0 && res.Meta[metaReason] != ReasonClosed {
					t.Errorf("a foreground sleep ended with %v", res.Meta)
				}
			})
		}
		time.Sleep(time.Duration(round) * time.Millisecond)
		if err := m.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		wg.Wait()
	}
}

func TestManyCommandsAtOnce(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)
	const n = 48
	// Meanwhile the person's task list is being refreshed and a call names a
	// command that is not there: both look at every background command, the
	// ones that are only just starting included.
	watching, stop := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(watching)
		for {
			select {
			case <-stop:
				return
			default:
			}
			for _, task := range m.Tasks() {
				if task.ID == "" || task.OutputFile == "" || task.PID <= 1 || task.Started.IsZero() {
					t.Errorf("a half-made task was listed: %+v", task)
				}
			}
			res := run(t, t.Context(), m, OutputName, env, args{argTaskID: "bg999"})
			if !res.IsError {
				t.Errorf("TaskOutput for a command that is not there: %s", core.TextOf(res.Content))
			}
		}
	}()
	defer func() {
		close(stop)
		<-watching
	}()
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			switch i % 4 {
			case 0, 1: // runs to its end
				res := bash(t, m, env, fmt.Sprintf("echo start %d; sleep 0.0%d; echo end %d >&2; exit %d", i, i%7, i, i%5))
				if got, want := printed(res), fmt.Sprintf("start %d\nend %d", i, i); got != want {
					t.Errorf("command %d: output %q, want %q", i, got, want)
				}
				if code := exitCode(t, res); code != i%5 {
					t.Errorf("command %d: exit code %d, want %d", i, code, i%5)
				}
			case 2: // is stopped at its timeout
				res := run(t, t.Context(), m, BashName, env, args{argCommand: fmt.Sprintf("echo waiting %d; sleep 300", i), argTimeout: 1000 + i})
				if headline, body, _ := parts(res); !strings.HasPrefix(headline, "timed out") || body != fmt.Sprintf("waiting %d", i) {
					t.Errorf("command %d: %q, %q", i, headline, body)
				}
			case 3: // runs in the background and is read
				id, _ := startBackground(t, m, env, fmt.Sprintf("echo background %d; exit %d", i, i%3))
				want := fmt.Sprintf("background %d\n", i)
				if got := readUntil(t, m, env, id, want); got != want {
					t.Errorf("command %d: output %q, want %q", i, got, want)
				}
			}
		})
	}
	wg.Wait()

	tasks := m.Tasks()
	if len(tasks) != n/4 {
		t.Fatalf("%d background commands, want %d", len(tasks), n/4)
	}
	seen := make(map[string]bool)
	for _, task := range tasks {
		if seen[task.ID] {
			t.Errorf("the id %s was given out twice", task.ID)
		}
		seen[task.ID] = true
	}
	eventually(t, "every background command to end", func() bool {
		for _, task := range m.Tasks() {
			if task.Running {
				return false
			}
		}
		return true
	})
	eventually(t, "the manager's books to be empty", func() bool {
		m.mu.Lock()
		defer m.mu.Unlock()
		return len(m.running) == 0 && len(m.lingering) == 0 && m.bgRunning == 0 && len(m.calls) == 0
	})
}

func TestTimeoutMovesToBackground(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{TimeoutToBackground: true, OnEnd: news.take})
	env := newEnv(t)
	live := newCollector()
	env.Progress = live.take
	command := `echo "parent=$$"; cd sub 2>/dev/null; while [ ! -e finish ]; do sleep 0.02; done; echo "the rest"; exit 5`
	res := run(t, t.Context(), m, BashName, env, args{argCommand: command, argTimeout: 400})

	headline, body, notes := parts(res)
	id, _ := res.Meta[metaTaskID].(string)
	if res.IsError || id != "bg1" || !strings.HasPrefix(headline, "still running after 0.4 s, so it was moved to the background as bg1") {
		t.Fatalf("headline = %q, id %q, IsError %v", headline, id, res.IsError)
	}
	pids := pidsIn(body)
	if len(pids) != 1 || !alive(pids["parent"]) {
		t.Fatalf("output so far = %q; the command should still be running", body)
	}
	if res.Cwd != "" || !strings.Contains(strings.Join(notes, "\n"), "a change of directory it makes will not carry over") {
		t.Errorf("Cwd = %q, notes %q", res.Cwd, notes)
	}
	if res.Meta[metaTimedOutAfter] != int64(400) {
		t.Errorf("Meta = %v", res.Meta)
	}
	if tasks := m.Tasks(); len(tasks) != 1 || !tasks[0].Running || tasks[0].ID != id {
		t.Errorf("Tasks() = %+v", tasks)
	}

	// It is an ordinary background command now. A read gives only what the
	// result of the call did not show.
	if body := taskBody(readTask(t, m, env, id, "")); body != "(no new output)" {
		t.Errorf("first read after the move = %q", body)
	}
	seenLive := live.text()
	touch(t, env.Cwd, "finish")
	ended := news.next(t)
	if ended.ID != id || ended.ExitCode != 5 || ended.Reason != ReasonExited {
		t.Errorf("news = %+v", ended.Task)
	}
	if body := taskBody(readTask(t, m, env, id, "")); body != "the rest" {
		t.Errorf("read after it ended = %q", body)
	}
	// The call was over, so nothing more was sent to its listener.
	if got := live.text(); got != seenLive {
		t.Errorf("live output went on after the call returned: %q", got[len(seenLive):])
	}
	allGone(t, pids)
}

func TestSleepIsNotMovedToBackground(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{TimeoutToBackground: true})
	env := newEnv(t)
	// A command that only waits has nothing to go on with.
	res := run(t, t.Context(), m, BashName, env, args{argCommand: "sleep 300", argTimeout: 300})
	if headline, _, _ := parts(res); !res.IsError || !strings.HasPrefix(headline, "timed out after 0.3 s and was stopped") {
		t.Errorf("headline = %q", headline)
	}
	if len(m.Tasks()) != 0 {
		t.Errorf("Tasks() = %+v", m.Tasks())
	}
	for command, want := range map[string]bool{
		"sleep 5": true, "  sleep 5; echo x": true, "sleep": true, "sleep\t1": true,
		"sleeper 5": false, "echo sleep": false, "": false, "nosleep": false,
	} {
		if got := startsWithSleep(command); got != want {
			t.Errorf("startsWithSleep(%q) = %v", command, got)
		}
	}
}

func TestMoveToBackgroundOnRequest(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	live := newCollector()
	env.Progress = live.take
	input := mustJSON(t, args{argCommand: `echo "parent=$$"; echo ready; while [ ! -e finish ]; do sleep 0.02; done; echo "the rest"`, argTimeout: 600_000})
	done := make(chan core.ToolResult, 1)
	go func() {
		res, err := toolNamed(t, m, BashName).Run(t.Context(), core.ToolCall{ID: "the-call", Name: BashName, Input: input}, env)
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	live.waitFor(t, "ready\n")

	if m.Background("some-other-call") {
		t.Error("Background found a command for a call that has none")
	}
	if !m.Background("the-call") {
		t.Fatal("Background did not find the command of the call in flight")
	}
	res := <-done
	headline, body, _ := parts(res)
	id, _ := res.Meta[metaTaskID].(string)
	if res.IsError || id == "" || !strings.HasPrefix(headline, "moved to the background as "+id+" at the person's request") {
		t.Fatalf("headline = %q, Meta %v", headline, res.Meta)
	}
	pids := pidsIn(body)
	if !strings.HasSuffix(body, "\nready") || !alive(pids["parent"]) {
		t.Errorf("output so far = %q", body)
	}
	if m.Background("the-call") {
		t.Error("Background found the call again after it returned")
	}
	touch(t, env.Cwd, "finish")
	if got := readUntil(t, m, env, id, "the rest\n"); got != "the rest\n" {
		t.Errorf("read after the move = %q", got)
	}
	allGone(t, pids)
}

func TestMoveToBackgroundWhenThereIsNoRoom(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{TimeoutToBackground: true, MaxBackground: 1})
	env := newEnv(t)
	startBackground(t, m, env, "sleep 300")
	// With the background full, a command whose time is up is stopped, as
	// it would be without the option.
	res := run(t, t.Context(), m, BashName, env, args{argCommand: `echo "parent=$$"; while :; do sleep 300; done`, argTimeout: 300})
	headline, body, _ := parts(res)
	if !res.IsError || !strings.HasPrefix(headline, "timed out after 0.3 s and was stopped") {
		t.Errorf("headline = %q", headline)
	}
	allGone(t, pidsIn(body))
}

func TestCancelWhileStopToolWaits(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{TermGrace: 600 * time.Millisecond})
	env := newEnv(t)
	id, _ := startBackground(t, m, env, `trap '' TERM; echo "parent=$$"; echo ready; while :; do sleep 300; done`)
	pids := pidsIn(readUntil(t, m, env, id, "ready\n"))
	// The call to TaskStop is itself cancelled while the command takes its
	// time to die. The stop goes on without it.
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	res := run(t, ctx, m, StopName, env, args{argTaskID: id})
	if text := core.TextOf(res.Content); !res.IsError || !strings.Contains(text, "interrupted while "+id+" was being stopped") {
		t.Errorf("result = %q", text)
	}
	allGone(t, pids)
}

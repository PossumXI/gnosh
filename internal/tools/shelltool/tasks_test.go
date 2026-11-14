package shelltool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// stepper is a background command that prints a line each time the test
// lets it, so that the test decides when output arrives.
const stepper = `echo "parent=$$"
for step in one two three; do
  while [ ! -e "$step" ]; do sleep 0.02; done
  echo "step $step"
  echo "noise for $step" >&2
done
printf 'last words, no newline'
exit 7`

// ends collects what Options.OnEnd is told.
type ends struct {
	mu   sync.Mutex
	got  []Ended
	news chan struct{}
}

func newEnds() *ends { return &ends{news: make(chan struct{}, 100)} }

func (e *ends) take(ev Ended) {
	e.mu.Lock()
	e.got = append(e.got, ev)
	e.mu.Unlock()
	e.news <- struct{}{}
}

// next waits for the next piece of news.
func (e *ends) next(t *testing.T) Ended {
	t.Helper()
	select {
	case <-e.news:
	case <-time.After(10 * time.Second):
		t.Fatal("gave up waiting to be told that a background command ended")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.got[len(e.got)-1]
}

func (e *ends) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.got)
}

// startBackground starts a command in the background and returns its id.
func startBackground(t *testing.T, m *Manager, env *core.ToolEnv, command string) (id string, res core.ToolResult) {
	t.Helper()
	res = run(t, t.Context(), m, BashName, env, args{argCommand: command, argBackground: true})
	if res.IsError {
		t.Fatalf("starting %q in the background: %s", command, core.TextOf(res.Content))
	}
	id, _ = res.Meta[metaTaskID].(string)
	if id == "" {
		t.Fatalf("the result gives no task id: %v", res.Meta)
	}
	return id, res
}

// readTask calls TaskOutput.
func readTask(t *testing.T, m *Manager, env *core.ToolEnv, id, filter string) core.ToolResult {
	t.Helper()
	a := args{argTaskID: id}
	if filter != "" {
		a[argFilter] = filter
	}
	return run(t, t.Context(), m, OutputName, env, a)
}

// readUntil calls TaskOutput until what it has returned in all contains
// want, and returns all of it.
func readUntil(t *testing.T, m *Manager, env *core.ToolEnv, id, want string) string {
	t.Helper()
	var all strings.Builder
	eventually(t, fmt.Sprintf("%q in the output of %s", want, id), func() bool {
		res := readTask(t, m, env, id, "")
		if body := taskBody(res); !strings.HasPrefix(body, "(no ") {
			all.WriteString(body + "\n")
		}
		return strings.Contains(all.String(), want)
	})
	return all.String()
}

// taskBody returns the output part of a TaskOutput result: everything but
// the line about the command's state. The newlines at its end are gone,
// as they are from every result.
func taskBody(res core.ToolResult) string {
	_, text := stampOf(res)
	if i := strings.LastIndex(text, "\n\n["); i >= 0 {
		return text[:i]
	}
	return text
}

// taskState returns the line about the command's state.
func taskState(res core.ToolResult) string {
	_, text := stampOf(res)
	return text[strings.LastIndex(text, "\n[")+1:]
}

func TestBackgroundCommand(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)

	start := time.Now()
	id, res := startBackground(t, m, env, stepper)
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("starting a background command took %v; it should return at once", took)
	}
	text := core.TextOf(res.Content)
	file, _ := res.Meta[metaOutputFile].(string)
	for _, want := range []string{"Started in the background as " + id, file, OutputName, StopName, "You will be told when it ends"} {
		if !strings.Contains(text, want) {
			t.Errorf("the result of starting it does not mention %q:\n%s", want, text)
		}
	}
	if id != "bg1" {
		t.Errorf("the first background command is %q, want bg1", id)
	}

	// Each read returns only what is new.
	got := readUntil(t, m, env, id, "parent=")
	pids := pidsIn(got)
	if got != fmt.Sprintf("parent=%d\n", pids["parent"]) {
		t.Errorf("first read = %q", got)
	}
	res = readTask(t, m, env, id, "")
	if body := taskBody(res); body != "(no new output)" {
		t.Errorf("a read with nothing new = %q", body)
	}
	if state := taskState(res); !strings.HasPrefix(state, "["+id+" is still running, ") || res.Meta[metaRunning] != true {
		t.Errorf("state = %q, running = %v", state, res.Meta[metaRunning])
	}

	touch(t, env.Cwd, "one")
	if got := readUntil(t, m, env, id, "noise for one\n"); got != "step one\nnoise for one\n" {
		t.Errorf("second read = %q", got)
	}

	// With a filter, only matching lines come back, and the rest of what
	// was new is passed over for good.
	touch(t, env.Cwd, "two")
	eventually(t, "the second step's output to be written", func() bool {
		data, _ := os.ReadFile(file)
		return strings.Contains(string(data), "noise for two\n")
	})
	res = readTask(t, m, env, id, `^step \w+$`)
	if body := taskBody(res); body != "step two" {
		t.Errorf("filtered read = %q, want only the matching line", body)
	}
	res = readTask(t, m, env, id, "")
	if body := taskBody(res); body != "(no new output)" {
		t.Errorf("after a filtered read, the lines it passed over came back: %q", body)
	}
	res = readTask(t, m, env, id, "no such thing")
	if body := taskBody(res); body != "(no new output)" {
		t.Errorf("a filtered read with nothing new = %q", body)
	}

	// It runs to its end by itself. The last line has no newline, and
	// comes with the end.
	touch(t, env.Cwd, "three")
	ended := news.next(t)
	if ended.ID != id || ended.Running || ended.Reason != ReasonExited || ended.ExitCode != 7 || ended.Signal != "" {
		t.Errorf("the news of its end: %+v", ended.Task)
	}
	if ended.Command != stepper || ended.OutputFile != file || ended.PID != pids["parent"] || ended.Duration <= 0 {
		t.Errorf("the news of its end: %+v", ended.Task)
	}
	if want := "step three\nnoise for three\nlast words, no newline"; !strings.HasSuffix(ended.Tail, want) {
		t.Errorf("tail = %q, want it to end %q", ended.Tail, want)
	}
	res = readTask(t, m, env, id, "")
	if body := taskBody(res); body != "step three\nnoise for three\nlast words, no newline" {
		t.Errorf("last read = %q", body)
	}
	if state := taskState(res); !strings.HasPrefix(state, "["+id+" failed with exit code 7 after ") {
		t.Errorf("state = %q", state)
	}
	if res.Meta[metaRunning] != false || res.Meta[metaExitCode] != 7 || res.Meta[metaReason] != ReasonExited {
		t.Errorf("Meta = %v", res.Meta)
	}
	res = readTask(t, m, env, id, "")
	if body := taskBody(res); body != "(no more output)" {
		t.Errorf("a read after everything was read = %q", body)
	}

	tasks := m.Tasks()
	if len(tasks) != 1 || tasks[0].ID != id || tasks[0].Running || tasks[0].ExitCode != 7 || tasks[0].OutputBytes == 0 {
		t.Errorf("Tasks() = %+v", tasks)
	}
	allGone(t, pids)
}

func TestBackgroundFilterWaitsForWholeLines(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	id, res := startBackground(t, m, env, `printf 'ERROR: half a'; while [ ! -e go ]; do sleep 0.02; done; echo ' line'; echo 'ok: fine'; echo 'ERROR: second'`)
	file := res.Meta[metaOutputFile].(string)
	eventually(t, "the half line to be written", func() bool {
		data, _ := os.ReadFile(file)
		return string(data) == "ERROR: half a"
	})
	// The line is not whole yet: a filter must not see half of it now and
	// the other half later.
	if body := taskBody(readTask(t, m, env, id, "^ERROR")); body != "(no new output)" {
		t.Errorf("read of half a line through a filter = %q", body)
	}
	touch(t, env.Cwd, "go")
	var got strings.Builder
	eventually(t, "both matching lines", func() bool {
		if body := taskBody(readTask(t, m, env, id, "^ERROR")); !strings.HasPrefix(body, "(no ") {
			got.WriteString(body + "\n")
		}
		return strings.Count(got.String(), "\n") >= 2
	})
	if want := "ERROR: half a line\nERROR: second\n"; got.String() != want {
		t.Errorf("filtered output = %q, want %q", got.String(), want)
	}
}

func TestBackgroundOutputWithoutFilterIsNotHeldBack(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	// A prompt, a progress bar: output with no newline is still output,
	// and without a filter there is no reason to wait for the line's end.
	// What must wait is half a character.
	id, res := startBackground(t, m, env, `printf 'progress: 50%% \342\202'; while [ ! -e go ]; do sleep 0.02; done; printf '\254 done\n'`)
	file := res.Meta[metaOutputFile].(string)
	eventually(t, "the first piece to be written", func() bool {
		data, _ := os.ReadFile(file)
		return len(data) == len("progress: 50% ")+2
	})
	if body := taskBody(readTask(t, m, env, id, "")); body != "progress: 50% " {
		t.Errorf("first read = %q, want the text so far without the half character", body)
	}
	touch(t, env.Cwd, "go")
	if got := readUntil(t, m, env, id, "done"); got != "€ done\n" {
		t.Errorf("second read = %q", got)
	}
}

func TestBackgroundLongOutputIsCut(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxOutputChars: 1000})
	env := newEnv(t)
	id, res := startBackground(t, m, env, "seq 1 5000; touch finished")
	file := res.Meta[metaOutputFile].(string)
	eventually(t, "the command to finish", func() bool {
		_, err := os.Stat(filepath.Join(env.Cwd, "finished"))
		return err == nil && m.Tasks()[0].Running == false
	})
	res = readTask(t, m, env, id, "")
	body := taskBody(res)
	if !strings.HasPrefix(body, "1\n2\n") || !strings.HasSuffix(body, "\n4999\n5000") || !strings.Contains(body, "lines omitted, full output in "+file+"]") {
		t.Errorf("a long read is not cut as a foreground result is:\n%s", body)
	}
	if res.Meta[metaTruncated] != true {
		t.Errorf("Meta = %v", res.Meta)
	}
	// With a filter the cut applies to what matched.
	id, _ = startBackground(t, m, env, "seq 1 5000")
	eventually(t, "the second command to finish", func() bool { return m.Tasks()[1].Running == false })
	if body := taskBody(readTask(t, m, env, id, "^12.$")); body != "120\n121\n122\n123\n124\n125\n126\n127\n128\n129" {
		t.Errorf("filtered read = %q", body)
	}
}

func TestBackgroundOutputPastTheFileLimit(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxSpoolBytes: 4096, MaxOutputChars: 1000})
	env := newEnv(t)
	// A background command that prints more than its output file may hold
	// can still be followed: what is new comes from memory, where the latest
	// output is always kept.
	id, res := startBackground(t, m, env, `seq 1 3000; while [ ! -e go ]; do sleep 0.02; done; echo "after the wait"; echo done`)
	file := res.Meta[metaOutputFile].(string)
	eventually(t, "the numbers to be printed", func() bool { return m.Tasks()[0].OutputBytes == 13893 })

	res = readTask(t, m, env, id, "")
	body := taskBody(res)
	if !strings.HasPrefix(body, "1\n2\n3\n") || !strings.HasSuffix(body, "\n2999\n3000") {
		t.Errorf("a read past the file's limit does not show both ends of what is new:\n%s", body)
	}
	if !strings.Contains(body, "lines omitted, the first 4 KiB of the output are in "+file+"]") {
		t.Errorf("the read does not say that the file holds only the start:\n%s", body)
	}
	if st, err := os.Stat(file); err != nil || st.Size() != 4096 {
		t.Errorf("the output file: %v, %v; want exactly 4096 bytes", st, err)
	}

	touch(t, env.Cwd, "go")
	if got := readUntil(t, m, env, id, "done\n"); got != "after the wait\ndone\n" {
		t.Errorf("output after the file was full = %q", got)
	}
	// A filter works on what is kept in memory too, and the read says how
	// much lay between the file and memory and so was never looked at.
	id, _ = startBackground(t, m, env, `seq 1 3000; echo "needle 1"; echo hay; echo "needle 2"`)
	eventually(t, "the second command to end", func() bool { return !m.Tasks()[1].Running })
	want := "[7.6 KiB of output here were not kept: the output file is full]\nneedle 1\nneedle 2"
	if body := taskBody(readTask(t, m, env, id, "^needle")); body != want {
		t.Errorf("filtered read past the file's limit = %q, want %q", body, want)
	}
}

func TestStopBackgroundCommand(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)
	id, _ := startBackground(t, m, env, tree)
	pids := pidsIn(readUntil(t, m, env, id, "ready\n"))
	if len(pids) != 3 {
		t.Fatalf("pids = %v", pids)
	}

	res := run(t, t.Context(), m, StopName, env, args{argTaskID: id})
	text := core.TextOf(res.Content)
	if res.IsError || !strings.HasPrefix(text, "Stopped "+id+" (echo \"parent=$$\"): it was stopped after ") || !strings.Contains(text, "its shell ended on SIGTERM") {
		t.Errorf("stop result = %q (IsError %v)", text, res.IsError)
	}
	if res.Meta[metaTaskIDPlain] != id || res.Meta[metaTaskType] != "local_bash" || res.Meta[metaCommand] != tree || res.Meta[metaReason] != ReasonStopped {
		t.Errorf("Meta = %v", res.Meta)
	}
	allGone(t, pids)

	// Stopping it again changes nothing and says so.
	res = run(t, t.Context(), m, StopName, env, args{argTaskID: id})
	if text := core.TextOf(res.Content); res.IsError || !strings.HasPrefix(text, id+" had already ended: it was stopped after ") {
		t.Errorf("second stop = %q (IsError %v)", text, res.IsError)
	}
	// The older spelling of the argument is understood.
	res = run(t, t.Context(), m, StopName, env, args{argTaskIDOld: id})
	if text := core.TextOf(res.Content); !strings.HasPrefix(text, id+" had already ended") {
		t.Errorf("stop by shell_id = %q", text)
	}

	if state := taskState(readTask(t, m, env, id, "")); !strings.HasPrefix(state, "["+id+" was stopped after ") {
		t.Errorf("state after the stop = %q", state)
	}
	if tasks := m.Tasks(); len(tasks) != 1 || tasks[0].Running || tasks[0].Reason != ReasonStopped || tasks[0].Signal != "SIGTERM" || tasks[0].ExitCode != 143 {
		t.Errorf("Tasks() = %+v", tasks)
	}

	// The model stopped it itself, so it is not told again that it ended.
	if err := m.Close(); err != nil {
		t.Error(err)
	}
	if n := news.count(); n != 0 {
		t.Errorf("OnEnd was called %d times for a command the model stopped itself", n)
	}
}

func TestStopBackgroundCommandThatIgnoresSIGTERM(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{TermGrace: 200 * time.Millisecond})
	env := newEnv(t)
	id, _ := startBackground(t, m, env, `trap '' TERM; echo "parent=$$"; echo ready; while :; do sleep 300; done`)
	pids := pidsIn(readUntil(t, m, env, id, "ready\n"))
	res := run(t, t.Context(), m, StopName, env, args{argTaskID: id})
	if text := core.TextOf(res.Content); !strings.Contains(text, "it did not exit when asked and was killed") {
		t.Errorf("stop result = %q", text)
	}
	allGone(t, pids)
}

func TestEndIsAnnouncedOnce(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)

	// One that ends by itself, unseen: announced.
	first, _ := startBackground(t, m, env, "echo quick; exit 3")
	ended := news.next(t)
	if ended.ID != first || ended.ExitCode != 3 || ended.Tail != "quick\n" {
		t.Errorf("news = %+v, tail %q", ended.Task, ended.Tail)
	}
	if !strings.HasPrefix(ended.Summary, first+" (echo quick; exit 3) failed with exit code 3 after ") {
		t.Errorf("summary = %q", ended.Summary)
	}
	// A command the model described is named by its description.
	res := run(t, t.Context(), m, BashName, env, args{argCommand: "true", argDescription: "Do  nothing,\nquickly", argBackground: true})
	if ended := news.next(t); !strings.HasPrefix(ended.Summary, res.Meta[metaTaskID].(string)+" (Do nothing, quickly) finished with exit code 0 after ") || ended.Description != "Do  nothing,\nquickly" {
		t.Errorf("summary = %q, description %q", ended.Summary, ended.Description)
	}

	// One the person stops through the manager: the model did not do it,
	// so the model is told.
	second, _ := startBackground(t, m, env, "sleep 300")
	task, err := m.Stop(t.Context(), second)
	if err != nil || task.Running || task.Reason != ReasonStopped {
		t.Errorf("Stop = %+v, %v", task, err)
	}
	if ended := news.next(t); ended.ID != second || ended.Reason != ReasonStopped {
		t.Errorf("news = %+v", ended.Task)
	}
	if _, err := m.Stop(t.Context(), "bg99"); err == nil {
		t.Error("Stop of an unknown command returned no error")
	}

	// One still running when the manager closes: nobody is left to tell.
	startBackground(t, m, env, "sleep 300")
	if err := m.Close(); err != nil {
		t.Error(err)
	}
	if n := news.count(); n != 3 {
		t.Errorf("OnEnd was called %d times, want 3", n)
	}
}

func TestBackgroundTimeLimit(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)
	res := run(t, t.Context(), m, BashName, env, args{argCommand: `echo "parent=$$"; sleep 300`, argBackground: true, argTimeout: 400})
	if text := core.TextOf(res.Content); !strings.Contains(text, "It is stopped if it is still running after 0.4 s.") {
		t.Errorf("the start result does not give the time limit: %s", text)
	}
	ended := news.next(t)
	if ended.Reason != ReasonTimeout || ended.Running {
		t.Errorf("news = %+v", ended.Task)
	}
	allGone(t, pidsIn(ended.Tail))
	id := ended.ID
	if state := taskState(readTask(t, m, env, id, "")); !strings.HasPrefix(state, "["+id+" was stopped when its time, 0.4 s, ran out") {
		t.Errorf("state = %q", state)
	}
}

func TestUnknownBackgroundCommand(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	for _, name := range []string{OutputName, StopName} {
		res := run(t, t.Context(), m, name, env, args{argTaskID: "bg7"})
		if text := core.TextOf(res.Content); !res.IsError || !strings.Contains(text, `There is no background command "bg7": this session has started none.`) {
			t.Errorf("%s with no commands at all: %q (IsError %v)", name, text, res.IsError)
		}
	}
	running, _ := startBackground(t, m, env, "sleep 300 # a long nap")
	finished, _ := startBackground(t, m, env, "exit 4")
	eventually(t, "the second command to end", func() bool { return !m.Tasks()[1].Running })
	for _, name := range []string{OutputName, StopName} {
		res := run(t, t.Context(), m, name, env, args{argTaskID: "bg7"})
		text := core.TextOf(res.Content)
		want := fmt.Sprintf("There is no background command \"bg7\". This session's background commands are:\n- %s: running: sleep 300 # a long nap\n- %s: ended (exited, exit code 4): exit 4", running, finished)
		if !res.IsError || text != want {
			t.Errorf("%s with an unknown id:\n got: %s\nwant: %s", name, text, want)
		}
	}

	// Arguments that cannot be used.
	bad := []struct{ input, want string }{
		{`{}`, "task_id is missing"},
		{`{"task_id": ""}`, "task_id is missing"},
		{`{"task_id": 3}`, "task_id must be a string"},
		{`{"task_id": "bg1", "filter": "("}`, "filter is not a regular expression"},
		{`{"task_id": "bg1", "filter": 5}`, "filter must be a string"},
	}
	for _, tt := range bad {
		for _, name := range []string{OutputName, StopName} {
			call := core.ToolCall{ID: "c", Name: name, Input: []byte(tt.input)}
			tool := toolNamed(t, m, name)
			_, err := tool.(core.Describer).Describe(call, env)
			res, runErr := tool.Run(t.Context(), call, env)
			if err == nil || !strings.Contains(err.Error(), tt.want) || runErr != nil || !res.IsError || core.TextOf(res.Content) != err.Error() {
				t.Errorf("%s %s: Describe said %v; Run said %q, %v", name, tt.input, err, core.TextOf(res.Content), runErr)
			}
		}
	}

	// What the two tools say a call would do.
	action, err := toolNamed(t, m, OutputName).(core.Describer).Describe(core.ToolCall{Input: []byte(`{"task_id": "bg1"}`)}, env)
	if err != nil || action.Kind != core.ActionRead || len(action.Paths) != 0 || !strings.Contains(action.Summary, "bg1") {
		t.Errorf("TaskOutput describes itself as %+v, %v", action, err)
	}
	action, err = toolNamed(t, m, StopName).(core.Describer).Describe(core.ToolCall{Input: []byte(`{"task_id": "bg1"}`)}, env)
	if err != nil || action.Kind != core.ActionOther || action.Summary != "Stop background command bg1: sleep 300 # a long nap" {
		t.Errorf("TaskStop describes itself as %+v, %v", action, err)
	}
}

func TestTooManyBackgroundCommands(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxBackground: 2})
	env := newEnv(t)
	first, _ := startBackground(t, m, env, "sleep 300")
	startBackground(t, m, env, "sleep 300")
	res := run(t, t.Context(), m, BashName, env, args{argCommand: "sleep 300", argBackground: true})
	if text := core.TextOf(res.Content); !res.IsError || !strings.Contains(text, "2 background commands are already running") {
		t.Errorf("a third background command: %q (IsError %v)", text, res.IsError)
	}
	// One that has ended makes room.
	run(t, t.Context(), m, StopName, env, args{argTaskID: first})
	if id, _ := startBackground(t, m, env, "sleep 300"); id != "bg3" {
		t.Errorf("the next background command is %q, want bg3", id)
	}
	// Foreground commands are not counted.
	if res := bash(t, m, env, "echo fine"); res.IsError {
		t.Errorf("a foreground command was refused: %s", core.TextOf(res.Content))
	}
}

func TestBackgroundCommandOutlivesItsCall(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	ctx, cancel := context.WithCancel(t.Context())
	res := run(t, ctx, m, BashName, env, args{argCommand: `echo "parent=$$"; sleep 300`, argBackground: true})
	id := res.Meta[metaTaskID].(string)
	// The call that started it is over and its context is cancelled, as at
	// the end of a turn. The command is the session's, not the call's.
	cancel()
	pids := pidsIn(readUntil(t, m, env, id, "parent="))
	if !alive(pids["parent"]) || !m.Tasks()[0].Running {
		t.Error("a background command was stopped when the call that started it ended")
	}
	// A background command's change of directory is its own affair.
	if res.Cwd != "" {
		t.Errorf("Cwd = %q from a background start", res.Cwd)
	}
}

func TestBackgroundCommandThatCannotStart(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)
	// A command the shell cannot even read is still a command that ran and
	// failed, and is reported like one, with the shell's own reason.
	id, _ := startBackground(t, m, env, "if then fi")
	ended := news.next(t)
	if ended.ID != id || ended.ExitCode != 2 || !strings.Contains(ended.Tail, "syntax error") {
		t.Errorf("news = %+v, tail %q", ended.Task, ended.Tail)
	}
}

func TestBackgroundCommandLeavesAProcess(t *testing.T) {
	t.Parallel()
	news := newEnds()
	m := newManager(t, Options{OnEnd: news.take})
	env := newEnv(t)
	id, _ := startBackground(t, m, env, `(while [ ! -e stop ]; do sleep 0.02; done; echo late) & echo "left=$!"; echo "parent=$$"`)
	ended := news.next(t)
	pids := pidsIn(ended.Tail)
	if ended.ID != id || ended.ExitCode != 0 || len(ended.LeftRunning) != 1 || ended.LeftRunning[0] != pids["parent"] {
		t.Errorf("news = %+v", ended.Task)
	}
	res := readTask(t, m, env, id, "")
	if state := taskState(res); !strings.Contains(state, "finished with exit code 0") || !strings.Contains(state, "left a process running that still holds its output open: process group "+itoa(pids["parent"])) {
		t.Errorf("state = %q", state)
	}
	// What the process left behind prints still reaches a later read.
	touch(t, env.Cwd, "stop")
	if got := readUntil(t, m, env, id, "late\n"); got != "late\n" {
		t.Errorf("late output = %q", got)
	}
	allGone(t, map[string]int{"left": pids["left"]})
	// Once it has gone, the state stops saying that it is there.
	eventually(t, "the state to say that the process left running has ended", func() bool {
		res = readTask(t, m, env, id, "")
		return strings.Contains(taskState(res), "the process it left running has ended since")
	})
	if body := taskBody(res); body != "(no more output)" {
		t.Errorf("a read after everything ended = %q", body)
	}
}

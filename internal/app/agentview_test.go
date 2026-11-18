package app

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// gnosh agents opens the agent view where there is a terminal (with
// --json it still prints), and the view's source lists the runs of every
// folder, the working first: a working run with its conversation so far,
// read again as it grows, and one that is over with what it printed and
// when it ended. Forget takes a run that is over off the list and keeps
// its session, and refuses a working one; Stop stops one; Dispatch and
// Open run gnosh again, as gnosh --bg and gnosh -r, and say what a child
// that failed said.
func TestAgentViewSource(t *testing.T) {
	w := apptest.NewWorld(t)
	other := filepath.Join(w.Root, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	start := func(cwd, prompt string) (id, log string) {
		t.Helper()
		o := testOptions(w, "--bg", prompt)
		o.Cwd = cwd
		o.Detach = func(_, _ []string, l string) (int, error) {
			log = l
			return 4242, nil
		}
		runWith(t, context.Background(), o).wantCode(t, 0)
		return strings.TrimSuffix(filepath.Base(log), ".background.log"), log
	}
	done, doneLog := start(w.Dir, "fix the flaky test")
	f, err := os.OpenFile(doneLog, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("The test raced on its clock.\n\nAll 42 tests pass.\n")
	f.Close()
	working, workingLog := start(other, "write the docs")
	proc := exec.Command("sh", "-c", `trap "exit 0" TERM; while :; do sleep 0.1; done`, working)
	if err := proc.Start(); err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() { proc.Wait(); close(ended) }()
	t.Cleanup(func() { proc.Process.Kill(); <-ended })
	if err := os.WriteFile(strings.TrimSuffix(workingLog, ".log")+".pid", []byte(strconv.Itoa(proc.Process.Pid)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The working run's conversation so far.
	root := filepath.Dir(filepath.Dir(filepath.Dir(workingLog)))
	ts, err := transcript.New(root).CreateWithID(other, working)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()
	if _, err := ts.AppendAll(
		transcript.PromptEntry("write the docs"),
		transcript.AssistantEntry(core.Message{Role: core.RoleAssistant, Blocks: []core.Block{
			core.ToolUse("c1", "Bash", json.RawMessage(`{"command":"ls docs"}`)),
		}}, core.StopToolUse),
	); err != nil {
		t.Fatal(err)
	}

	// Executable stands in for gnosh: it says what it was asked to do,
	// where, and starts a run or fails as the test says.
	bin := filepath.Join(w.Root, "fake-gnosh")
	said := filepath.Join(w.Root, "said")
	script := `#!/bin/sh
printf '%s|' "$(pwd -P)" "$@" > ` + said + `
if [ "$1" = --bg ]; then
	if [ "$3" = fail ]; then echo "gnosh: the folder is not trusted." >&2; exit 1; fi
	echo "gnosh: backgrounded · 01a1eea5-0000-7000-8000-000000000001 · process 7"
	exit 0
fi
echo "gnosh: no session $2." >&2
exit 1
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var src AgentSource
	var info AgentViewInfo
	o := testOptions(w, "agents")
	o.Executable = bin
	o.AgentView = func(i AgentViewInfo) int {
		info, src = i, i.Source
		return 0
	}
	runWith(t, context.Background(), o).wantCode(t, 0)
	if src == nil || info.Cwd != w.Dir || info.Scoped || info.All {
		t.Fatalf("the view was opened with %+v", info)
	}
	// --json prints, view or no view.
	jo := testOptions(w, "agents", "--json")
	jo.AgentView = func(AgentViewInfo) int { t.Error("gnosh agents --json opened the view"); return 0 }
	has(t, "gnosh agents --json", runWith(t, context.Background(), jo).wantCode(t, 0).out, `"id": "`+working+`"`)

	runs := src.Runs()
	if len(runs) != 2 || runs[0].ID != working || runs[1].ID != done {
		t.Fatalf("the runs: %+v", runs)
	}
	wr, dr := runs[0], runs[1]
	if wr.State != "working" || wr.Short != working[:13] || wr.Cwd != other || wr.Prompt != "write the docs" || !wr.Ended.IsZero() {
		t.Errorf("the working run: %+v", wr)
	}
	if dr.State != "done" || dr.Cwd != w.Dir || dr.Ended.IsZero() || strings.Join(dr.Output, "|") != "The test raced on its clock.|All 42 tests pass." {
		t.Errorf("the run that is over: %+v", dr)
	}
	conv := src.Conversation(working)
	if len(conv) != 2 || conv[0].Kind != "prompt" || conv[1].Kind != "reply" || conv[1].Message.Blocks[0].Name != "Bash" {
		t.Fatalf("the working run's conversation: %+v", conv)
	}
	// What is written since is read too.
	if _, err := ts.Append(transcript.UserEntry(core.Message{Role: core.RoleUser, Blocks: []core.Block{
		core.ToolResultBlock("c1", core.TextResult("intro.md")),
	}}, transcript.SourceToolResults)); err != nil {
		t.Fatal(err)
	}
	if conv = src.Conversation(working); len(conv) != 3 || conv[2].Kind != "results" {
		t.Fatalf("the conversation after it grew: %+v", conv)
	}
	if len(src.Conversation(done)) != 0 {
		t.Errorf("a run with no session file has a conversation")
	}

	// Forget: not a working run; one that is over leaves the list, and its
	// session stays where it was.
	if err := src.Forget(working); err == nil || !strings.Contains(err.Error(), "is working: stop it first") {
		t.Errorf("forgetting a working run: %v", err)
	}
	sessionFile := filepath.Join(filepath.Dir(doneLog), done+".jsonl")
	if err := os.WriteFile(sessionFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := src.Forget(done); err != nil {
		t.Fatal(err)
	}
	if fileExists(doneLog) || !fileExists(sessionFile) {
		t.Errorf("after Forget, the log is there: %v; the session is there: %v", fileExists(doneLog), fileExists(sessionFile))
	}
	if runs = src.Runs(); len(runs) != 1 || runs[0].ID != working {
		t.Errorf("the runs after Forget: %+v", runs)
	}

	// Dispatch runs gnosh --bg in the view's folder.
	id, err := src.Dispatch(context.Background(), "", "tidy the README", nil)
	if err != nil || id != "01a1eea5-0000-7000-8000-000000000001" {
		t.Errorf("Dispatch: %q, %v", id, err)
	}
	real := func(dir string) string {
		r, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if got, _ := os.ReadFile(said); string(got) != real(w.Dir)+"|--bg|--|tidy the README|" {
		t.Errorf("gnosh --bg was run as %q", got)
	}
	if _, err := src.Dispatch(context.Background(), "", "fail", nil); err == nil || err.Error() != "the folder is not trusted" {
		t.Errorf("a Dispatch that fails: %v", err)
	}
	// What gnosh agents is given for them goes to the runs it starts.
	fo := testOptions(w, "agents", "-y", "--model", "qwen/qwen3.8-27b", "--effort", "high")
	fo.Executable = bin
	fo.AgentView = func(i AgentViewInfo) int {
		if want := []string{"--permission-mode=bypassPermissions", "--model=qwen/qwen3.8-27b", "--effort=high"}; !slices.Equal(i.Flags, want) {
			t.Errorf("the view was given the flags %q", i.Flags)
		}
		if _, err := i.Source.Dispatch(context.Background(), "", "tidy it", i.Flags); err != nil {
			t.Errorf("Dispatch with flags: %v", err)
		}
		return 0
	}
	runWith(t, context.Background(), fo).wantCode(t, 0)
	if got, _ := os.ReadFile(said); string(got) != real(w.Dir)+"|--bg|--permission-mode=bypassPermissions|--model=qwen/qwen3.8-27b|--effort=high|--|tidy it|" {
		t.Errorf("gnosh --bg with the view's flags was run as %q", got)
	}
	// Resume runs gnosh --bg on the run's session, in its folder.
	if err := src.Resume(context.Background(), working, "go on", nil); err != nil {
		t.Errorf("Resume: %v", err)
	}
	if got, _ := os.ReadFile(said); string(got) != real(other)+"|--bg|--resume="+working+"|--|go on|" {
		t.Errorf("gnosh --bg -r was run as %q", got)
	}
	// Open runs gnosh -r in the run's folder; a failure says why.
	src.Runs()
	if err := src.Open(working); err == nil || err.Error() != "no session "+working {
		t.Errorf("an Open that fails: %v", err)
	}
	if got, _ := os.ReadFile(said); string(got) != real(other)+"|-r|"+working+"|" {
		t.Errorf("gnosh -r was run as %q", got)
	}

	// Stop stops the working run.
	said2, err := src.Stop(context.Background(), working)
	if err != nil || !strings.Contains(said2, "was stopped") {
		t.Errorf("Stop: %q, %v", said2, err)
	}
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("the run's process did not end")
	}
	if runs = src.Runs(); len(runs) != 1 || runs[0].State != "stopped" {
		t.Errorf("the runs after Stop: %+v", runs)
	}
}

// gnosh attach opens the agent view on one run, named by its id, the start
// of one, or its name, with every folder's runs however old; with no
// terminal it says how to follow the run instead; a run it cannot find is
// said so.
func TestAttach(t *testing.T) {
	w := apptest.NewWorld(t)
	var log string
	o := testOptions(w, "--bg", "--name", "nightly", "clean the build")
	o.Detach = func(_, _ []string, l string) (int, error) {
		log = l
		return 4242, nil
	}
	runWith(t, context.Background(), o).wantCode(t, 0)
	id := strings.TrimSuffix(filepath.Base(log), ".background.log")
	for _, word := range []string{id[:13], "nightly"} {
		var info AgentViewInfo
		ao := testOptions(w, "attach", word)
		ao.AgentView = func(i AgentViewInfo) int { info = i; return 0 }
		runWith(t, context.Background(), ao).wantCode(t, 0)
		if info.Focus != id || !info.All || info.Scoped {
			t.Errorf("gnosh attach %s opened the view with %+v", word, info)
		}
	}
	r := run(t, w, "attach", id[:13]).wantCode(t, 1)
	has(t, "gnosh attach with no terminal", r.err, "gnosh attach opens the agent view, which needs a terminal: gnosh logs -f "+id[:13]+" follows the run here.")
	r = run(t, w, "attach", "nosuch").wantCode(t, 1)
	has(t, "gnosh attach of no run", r.err, "no session ran in the background under the ID nosuch")
}

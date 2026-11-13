package memjoin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// Rules reach the person, the start notice says what waits on them, and
// /mem shows what mem holds.
func TestRulesNoticeAndMemCommand(t *testing.T) {
	w := newWorld(t,
		reply(memCall("c1", "mem.Rule(\"gofumpt\", \"always run gofumpt before committing\")\nmem.Todo(\"release v2\").Ask(\"which day suits you for the release?\")")),
		reply(memCall("c2", `mem.Rule("gofumpt", "run gofumpt -l . before committing")`)),
		says("Noted."),
	)
	r := w.run("-y", "-p", "always run gofumpt before committing; and ask me about the release day")
	r.wantCode(t, 0)
	has(t, "stderr of the session that set the rule", r.err,
		"mem: this session set a rule every session will follow as yours: `gofumpt` = \"always run gofumpt before committing\". If you did not ask for it, /mem forget gofumpt removes it",
		"mem: this session changed the rule `gofumpt`, which every session follows as yours, to \"run gofumpt -l . before committing\" (it was \"always run gofumpt before committing\"). If you did not ask for it, /mem log shows the change and /mem undo reverses it")

	// The next session: gnosh itself says what waits on the person,
	// before any work, and /mem prints mem's outline with no model asked.
	asked := len(w.Model.Requests())
	r = w.run("-p", "/mem")
	r.wantCode(t, 0)
	has(t, "stderr as a session starts", r.err, "mem waiting on you: \"which day suits you for the release?\" (release-v2/")
	has(t, "/mem", r.out, "`gofumpt`", "run gofumpt -l . before committing", "`release-v2` \"release v2\"", "which day suits you for the release?")
	hasNot(t, "/mem", r.out, "mem.Show(")

	r = w.run("-p", "/mem log")
	r.wantCode(t, 0)
	has(t, "/mem log", r.out, "#1 ", "#2 ", "the agent in \"always run gofumpt before committing; and ask me about the", "mem.Rule(\"gofumpt\", \"run gofumpt -l . before committing\")")

	r = w.run("-p", "/mem show gofumpt")
	r.wantCode(t, 0)
	has(t, "/mem show", r.out, "run gofumpt -l . before committing")

	r = w.run("-p", "/mem help")
	r.wantCode(t, 0)
	has(t, "/mem help", r.out, "/mem redact <text>   take a secret's exact text out of mem and its history")

	// The person removes the rule. It is their own doing, so nothing
	// warns them of it.
	r = w.run("-p", "/mem forget gofumpt")
	r.wantCode(t, 0)
	hasNot(t, "/mem forget", r.err, "this session removed the rule")
	r = w.run("-p", "/mem show gofumpt")
	hasNot(t, "/mem show after the rule was forgotten", r.out, "run gofumpt -l . before committing")

	if n := len(w.Model.Requests()); n != asked {
		t.Errorf("/mem made %d requests to the model", n-asked)
	}
	w.script.done()
}

// gnosh mem reads the project's mem from a shell, with no model and no
// provider set up, and changes nothing.
func TestGnoshMem(t *testing.T) {
	w := newWorld(t)

	// No mem yet: it says so, and makes no file.
	r := w.run("mem")
	r.wantCode(t, 0)
	if got := strings.TrimSpace(r.out); got != "no mem here yet: a session makes one when it first keeps something" {
		t.Errorf("gnosh mem in a project with no mem: %q", got)
	}
	if _, err := os.Stat(w.gnoDir()); err == nil {
		t.Errorf("gnosh mem made the gnovm data directory in a project with no mem")
	}

	w.script.add(
		reply(memCall("c1", "mem.Todo(\"ship the release\", \"build\", \"test\")\nmem.Fact(\"cmd/test\", \"go test -race ./...\")")),
		says("Planned."),
	)
	w.run("-y", "-p", "plan the release").wantCode(t, 0)

	// From here on no provider is configured at all.
	if err := os.WriteFile(filepath.Join(w.ConfigDir(), "gnoshrc"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	asked := len(w.Model.Paths())

	r = w.run("mem")
	r.wantCode(t, 0)
	has(t, "gnosh mem", r.out, "`ship-release` \"ship the release\"", "`cmd/test`")
	r = w.run("mem", "show", "ship-release")
	r.wantCode(t, 0)
	has(t, "gnosh mem show", r.out, "\"build\"", "\"test\"")
	r = w.run("mem", "find", "race")
	r.wantCode(t, 0)
	has(t, "gnosh mem find", r.out, "`cmd/test`")
	r = w.run("mem", "log")
	r.wantCode(t, 0)
	has(t, "gnosh mem log", r.out, "#1 ", "mem.Fact(\"cmd/test\", \"go test -race ./...\")")

	// What it refuses exits 1: a name mem does not hold, a find with no
	// words, and everything that would change mem.
	r = w.run("mem", "show", "nope")
	r.wantCode(t, 1)
	has(t, "gnosh mem show of a name mem does not hold", r.out, "no item `nope`; gnosh mem lists them")
	r = w.run("mem", "find")
	r.wantCode(t, 1)
	has(t, "gnosh mem find with no words", r.out, "gnosh mem find takes words: gnosh mem find release key")
	r = w.run("mem", "forget", "cmd/test")
	r.wantCode(t, 1)
	has(t, "gnosh mem forget", r.out, "gnosh mem only reads mem: show, find and log", "To change mem, type /mem forget … in a session.")
	hasNot(t, "gnosh mem forget", r.out, "cmd/test")
	r = w.run("mem", "show", "cmd/test")
	r.wantCode(t, 0)
	has(t, "the fact after a refused forget", r.out, "go test -race ./...")

	if n := len(w.Model.Paths()); n != asked {
		t.Errorf("gnosh mem asked the model server %d times", n-asked)
	}
	if _, err := os.Stat(filepath.Join(w.gnoDir(), "worker.log")); err == nil {
		t.Errorf("gnosh mem started a worker")
	}
}

// The mem reminder, end to end: ten replies with no call on mem, and the
// next request carries the reminder as a hidden part.
func TestReminderEndToEnd(t *testing.T) {
	var replies []apptest.Reply
	for i := 0; i < 10; i++ {
		replies = append(replies, reply(call(fmt.Sprintf("c%d", i), "Read", map[string]any{"file_path": "notes.txt", "offset": i + 1, "limit": 1})))
	}
	replies = append(replies, says("I read ten lines."))
	w := newWorld(t, replies...)
	w.Write("notes.txt", filler("note", 2000))
	w.run("-y", "-p", "read the notes a line at a time").wantCode(t, 0)
	w.script.done()
	reqs := w.script.requests()
	reminder := prompt.Builtin().MemReminder(nil, nil)
	for i, req := range reqs[:10] {
		if strings.Contains(string(req.Body), "You have not used mem for a while") {
			t.Errorf("request %d already carries the reminder", i)
		}
	}
	if got := lastUser(t, reqs[10]); got != reminder {
		t.Errorf("the hidden part before the eleventh reply:\n%s\nwant:\n%s", got, reminder)
	}

	// The session never used a machine or mem, so nothing was started for
	// it: no client of the GnoVM was made, at the turn's end either.
	if _, err := os.Stat(filepath.Join(w.gnoDir(), ".clients")); err == nil {
		t.Errorf("a client of the GnoVM was started for a session that never used it")
	}
	if n := len(w.memRequests()); n != 0 {
		t.Errorf("%d requests went to mem in a session whose model never called it", n)
	}
}

// After a summary the session is given the digest of mem again, with what
// it is handed back.
func TestDigestAfterASummary(t *testing.T) {
	w := newWorld(t,
		reply(memCall("c1", `mem.Fact("cmd/test", "go test -race ./...")`)),
		says("Kept."),
	)
	w.run("-y", "-p", "keep the test command").wantCode(t, 0)
	before := lastUser(t, w.script.requests()[0])
	hasNot(t, "the first prompt, with mem still empty", before, "<mem>")

	// /compact asks the model for a summary; the request begins as the
	// session's own do.
	w.script.add(says("The user asked for the test command to be kept, and it was."))
	r := w.run("-c", "-p", "/compact")
	r.wantCode(t, 0)

	w.script.add(says("Nothing is left."))
	w.run("-c", "-p", "what is left?").wantCode(t, 0)
	reqs := w.script.requests()
	last := reqs[len(reqs)-1]
	body := conversation(last)
	text := ""
	for _, m := range messagesOf(last)[1:] {
		if m.role == "user" {
			text += m.text + "\n"
		}
	}
	summary, digest, words := strings.Index(text, "The user asked for the test command to be kept"), strings.Index(text, "<mem>"), strings.Index(text, "what is left?")
	if !(summary >= 0 && summary < digest && digest < words) {
		t.Errorf("after a summary the model is not sent the summary, then the digest, then the next prompt:\n%s", body)
	}
	has(t, "the digest given back after the summary", text, "`cmd/test` = go test -race ./...")
	w.script.done()
}

// The task list is mem's: after a kept call it is the todos of the last
// task the call changed, and the session's file holds it.
func TestTaskListFromMem(t *testing.T) {
	b := newBench(t, Options{}, nil)
	j := b.join()
	b.mem(`mem.Todo("rename the project", "rename it in the code", "rename it in the tests").Push()`)
	want := []gnovmtools.Todo{
		{Content: "rename it in the code", Status: gnovmtools.TodoStatusInProgress, ActiveForm: "rename it in the code"},
		{Content: "rename it in the tests", Status: gnovmtools.TodoStatusPending, ActiveForm: "rename it in the tests"},
	}
	check := func(what string, want []gnovmtools.Todo) {
		t.Helper()
		j.mu.Lock()
		got := append([]gnovmtools.Todo(nil), j.tasks.list...)
		j.mu.Unlock()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("the task list %s:\n%v\nwant:\n%v", what, got, want)
		}
	}
	check("after the plan", want)
	b.mem(`mem.Next()`)
	if got := strings.Join(j.taskLines(), "\n"); got != "[>] in `rename-project/rename-code` \"rename it in the code\"\n[ ] rename it in the tests" {
		t.Errorf("the task list as lines, standing in the first todo:\n%s", got)
	}
	b.call("Read", map[string]any{"file_path": filepath.Join(b.project, "nothing.txt")})
	os.WriteFile(filepath.Join(b.project, "main.go"), []byte("package gnosh\n"), 0o644)
	b.call("Read", map[string]any{"file_path": filepath.Join(b.project, "main.go")})
	b.mem("mem.Pop(\"looked at main.go: it says gnosh\")\nmem.Next()")
	want[0].Status, want[1].Status = gnovmtools.TodoStatusCompleted, gnovmtools.TodoStatusInProgress
	check("after the first todo was popped", want)

	// The session's file holds the list, so that a session opened again,
	// or a copy of it, has it.
	var stored []string
	for _, e := range b.ts.Entries() {
		if e.Key == metaTasks {
			stored = append(stored, e.Value)
		}
	}
	if len(stored) == 0 || stored[len(stored)-1] != mustJSON(t, want) {
		t.Errorf("the task lists written to the session's file: %q", stored)
	}
	if got := b.feature.Name; got != "mem" {
		t.Errorf("the feature is named %q", got)
	}
}

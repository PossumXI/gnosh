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
	inOrder(t, "stderr of the session that set the rule", r.err,
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
	if err := os.WriteFile(w.SettingsFile(), []byte("{}\n"), 0o600); err != nil {
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

	// A mistake in the config stops a session from starting, and not
	// this: where mem is kept does not depend on the config, and reading
	// it needs nothing the config holds.
	if err := os.WriteFile(w.SettingsFile(), []byte(`{"frobnicate": true}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r = w.run("mem")
	r.wantCode(t, 0)
	has(t, "gnosh mem with a mistake in the config", r.out, "`ship-release` \"ship the release\"", "`cmd/test`")
	if r = w.run("-p", "hello"); r.code == 0 {
		t.Errorf("a session started though the config has a mistake")
	}

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
// task the call changed, the front end is handed it with the frame the
// session stands in on the line in progress, and the session's file holds
// it.
func TestTaskListFromMem(t *testing.T) {
	b := newBench(t, Options{}, nil)
	j := b.join()
	if b.list() != "" || len(b.facts) != 0 {
		t.Errorf("a task list or a status fact was set before the session was ready: %q %q", b.lists, b.facts)
	}
	b.mem(`mem.Todo("rename the project", "rename it in the code", "rename it in the tests").Push()`)
	want := []gnovmtools.TaskLine{
		{Name: "rename-project/rename-code", Title: "rename it in the code", AtWork: "rename it in the code", State: gnovmtools.LineWorking},
		{Name: "rename-project/rename-tests", Title: "rename it in the tests", AtWork: "rename it in the tests", State: gnovmtools.LineOpen},
	}
	check := func(what string, want []gnovmtools.TaskLine) {
		t.Helper()
		j.mu.Lock()
		got := append([]gnovmtools.TaskLine(nil), j.tasks.list...)
		j.mu.Unlock()
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("the task list %s:\n%v\nwant:\n%v", what, got, want)
		}
	}
	check("after the plan", want)
	// The session stands in the task: the line in progress reads the
	// frame, and the status line names it.
	if got, want := b.list(), "[>] in `rename-project` \"rename the project\" | [ ] rename it in the tests"; got != want {
		t.Errorf("the task list the front end was handed after the plan:\n%s\nwant:\n%s", got, want)
	}
	if got, _ := b.fact("frame"); got != "rename-project" {
		t.Errorf("the status line's frame after the plan: %q", got)
	}
	b.mem(`mem.Next()`)
	if got, want := b.list(), "[>] in `rename-project/rename-code` \"rename it in the code\" | [ ] rename it in the tests"; got != want {
		t.Errorf("the task list, standing in the first todo:\n%s\nwant:\n%s", got, want)
	}
	if got, _ := b.fact("frame"); got != "rename-project/rename-code" {
		t.Errorf("the status line's frame, standing in the first todo: %q", got)
	}
	b.call("Read", map[string]any{"file_path": filepath.Join(b.project, "nothing.txt")})
	os.WriteFile(filepath.Join(b.project, "main.go"), []byte("package gnosh\n"), 0o644)
	b.call("Read", map[string]any{"file_path": filepath.Join(b.project, "main.go")})
	b.mem("mem.Pop(\"looked at main.go: it says gnosh\")\nmem.Next()")
	want[0].State, want[1].State = gnovmtools.LineDone, gnovmtools.LineWorking
	check("after the first todo was popped", want)
	if got, want := b.list(), "[x] rename it in the code | [>] in `rename-project/rename-tests` \"rename it in the tests\""; got != want {
		t.Errorf("the task list, standing in the second todo:\n%s\nwant:\n%s", got, want)
	}
	// The second todo and then the task are popped: at the root no line
	// reads a frame, and the status line names none.
	os.WriteFile(filepath.Join(b.project, "main_test.go"), []byte("package gnosh\n"), 0o644)
	b.call("Read", map[string]any{"file_path": filepath.Join(b.project, "main_test.go")})
	b.mem(`mem.Pop("looked at main_test.go: it says gnosh")`)
	if got, _ := b.fact("frame"); got != "rename-project" {
		t.Errorf("the status line's frame, back in the task: %q", got)
	}
	b.mem(`mem.Pop("both files say gnosh")`)
	if got, ok := b.fact("frame"); !ok || got != "" {
		t.Errorf("the status line's frame at the root: %q", got)
	}
	if got, want := b.list(), "[x] rename it in the code | [x] rename it in the tests"; got != want {
		t.Errorf("the task list once the task is done:\n%s\nwant:\n%s", got, want)
	}
	want[1].State = gnovmtools.LineDone

	// The session's file holds the list, so that a session opened again,
	// or a copy of it, has it.
	var stored []string
	for _, e := range b.ts.Entries() {
		if e.Key == metaTasks {
			stored = append(stored, e.Value)
		}
	}
	if len(stored) == 0 || stored[len(stored)-1] != encodeTasks(want) {
		t.Errorf("the task lists written to the session's file: %q", stored)
	}
	// It is kept in words of the join's own, and reads back as it was. A
	// value that is no list of this build's is not read.
	if got, want := stored[len(stored)-1], `[{"name":"rename-project/rename-code","title":"rename it in the code","state":"done"},{"name":"rename-project/rename-tests","title":"rename it in the tests","state":"done"}]`; got != want {
		t.Errorf("the task list as the session's file keeps it:\n%s\nwant:\n%s", got, want)
	}
	if back, ok := decodeTasks(stored[len(stored)-1]); !ok || fmt.Sprint(back) != fmt.Sprint(want) {
		t.Errorf("the kept task list read back: %v, %v", back, ok)
	}
	working := []gnovmtools.TaskLine{{Title: "tag it", AtWork: "in `release-v2/tag` \"tag it\"", State: gnovmtools.LineWorking}}
	if kept := encodeTasks(working); kept != `[{"title":"tag it","at_work":"in `+"`release-v2/tag`"+` \"tag it\"","state":"working"}]` {
		t.Errorf("a line that reads otherwise while it is worked, as it is kept: %s", kept)
	} else if back, ok := decodeTasks(kept); !ok || fmt.Sprint(back) != fmt.Sprint(working) {
		t.Errorf("that line read back: %v, %v", back, ok)
	}
	for _, value := range []string{``, `{}`, `[{"title":"tag it","state":"paused"}]`, `[{"state":"open"}]`, `[{"words":"tag it","where":"waiting"}]`} {
		if list, ok := decodeTasks(value); ok {
			t.Errorf("%q was read as a task list: %v", value, list)
		}
	}
	if list, ok := decodeTasks(`[]`); !ok || len(list) != 0 {
		t.Errorf("an empty list was not read as one: %v, %v", list, ok)
	}
	if got := b.feature.Name; got != "mem" {
		t.Errorf("the feature is named %q", got)
	}
}

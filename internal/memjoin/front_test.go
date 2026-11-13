package memjoin

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// toolNamesOf lists the tools a request offers the model, in order.
func toolNamesOf(r apptest.Request) []string {
	var out []string
	tools, _ := r.JSON["tools"].([]any)
	for _, t := range tools {
		tm, _ := t.(map[string]any)
		fn, _ := tm["function"].(map[string]any)
		name, _ := fn["name"].(string)
		out = append(out, name)
	}
	return out
}

// eventAt returns the place among the front end's events of the first one
// that ok accepts, or -1.
func eventAt[E any](f *front, ok func(E) bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, ev := range f.events {
		if e, is := ev.(E); is && ok(e) {
			return i
		}
	}
	return -1
}

// The task list and the status line, as a front end is handed them. After
// each kept call on mem the front end has mem's todos for the task the
// call changed, with the frame the session stands in on the line in
// progress, and the status line's frame; a session that is opened again
// is handed both as it starts; and the goal the session is bound to is
// named for the status line while it is bound.
func TestTaskListAndStatusLineReachTheFrontEnd(t *testing.T) {
	const (
		task  = "rename-greeting"
		todo1 = "rename-greeting/change-hello-hi-greet"
		todo2 = "rename-greeting/note-change-notes-md"
	)
	w := newWorld(t,
		reply(memCall("c1", `mem.Todo("rename the greeting", "change hello to hi in greet.txt", "note the change in NOTES.md").Push()`)),
		reply(memCall("c2", `mem.Next()`)),
		reply(call("c3", "Read", map[string]any{"file_path": "greet.txt"})),
		reply(call("c4", "Edit", map[string]any{"file_path": "greet.txt", "old_string": "hello", "new_string": "hi"})),
		reply(memCall("c5", "mem.Pop(\"greet.txt now says hi\")\nmem.Next()")),
		says("The first todo is done, and I stand in the second."),
	)
	w.Write("greet.txt", "hello, world\n")
	w.Write("NOTES.md", "Notes on the greeting.\n")

	var info app.SessionInfo
	var atStart []string
	f, r := w.attach(func(ctx context.Context, c app.SessionControls, f *front) int {
		info = c.Info()
		atStart = append(f.facts(0), f.taskLists(0)...)
		f.say(ctx, c, "rename the greeting to hi")
		c.Quit("prompt_input_exit")
		return 0
	}, "-y")
	r.wantCode(t, 0)
	w.script.done()

	// A new session stands nowhere and has no list: nothing is sent.
	if len(atStart) != 0 {
		t.Errorf("a new session's front end was handed %q before anything happened", atStart)
	}
	// The status line's frame follows each push and pop, once each: the
	// call that popped the first todo and entered the second is one
	// change.
	if got, want := f.facts(0), []string{"frame=" + task, "frame=" + todo1, "frame=" + todo2}; !reflect.DeepEqual(got, want) {
		t.Errorf("the status facts the front end was handed:\n%q\nwant:\n%q", got, want)
	}
	wantLists := []string{
		"[>] in `" + task + "` \"rename the greeting\" | [ ] note the change in NOTES.md",
		"[>] in `" + todo1 + "` \"change hello to hi in greet.txt\" | [ ] note the change in NOTES.md",
		"[x] change hello to hi in greet.txt | [>] in `" + todo2 + "` \"note the change in NOTES.md\"",
	}
	if got := f.taskLists(0); !reflect.DeepEqual(got, wantLists) {
		t.Errorf("the task lists the front end was handed:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(wantLists, "\n"))
	}
	// The person is told what the pop took out of the model's context
	// when the call that popped has ended, and before the model is asked
	// again.
	ended := eventAt(f, func(e *agent.ToolEndEvent) bool { return strings.Contains(string(e.Input), "mem.Pop(") })
	popped := eventAt(f, func(e *agent.NoticeEvent) bool { return strings.HasPrefix(e.Text, "popped `"+todo1+"`: about ") })
	answered := eventAt(f, func(e *agent.TextEvent) bool { return strings.HasPrefix(e.Text, "The first") })
	if !(ended >= 0 && ended < popped && popped < answered) {
		t.Errorf("the call that popped ended at event %d, the person was told at event %d, and the model's next reply began at event %d: want them in that order", ended, popped, answered)
	}

	var pops []string
	for _, line := range f.noticed(0) {
		if name, _, ok := strings.Cut(line, ": about "); ok && strings.HasPrefix(name, "popped `") {
			pops = append(pops, name)
		}
	}
	if !reflect.DeepEqual(pops, []string{"popped `" + todo1 + "`"}) {
		t.Errorf("what the person was told of pops: %q, want the first todo's, once", pops)
	}

	// What /mem takes after "redact" and after "forget" is what the
	// person wants gone, and the command says so itself, for whatever
	// interface shows the line and keeps a history.
	var mem, goal *app.CommandInfo
	for i := range info.CommandList {
		switch info.CommandList[i].Name {
		case "mem":
			mem = &info.CommandList[i]
		case "goal":
			goal = &info.CommandList[i]
		}
	}
	if mem == nil || !reflect.DeepEqual(mem.Sensitive, []string{"redact", "forget"}) || !mem.InRun {
		t.Errorf("what is said of /mem: %+v", mem)
	}
	if goal == nil || len(goal.Sensitive) != 0 {
		t.Errorf("what is said of /goal: %+v", goal)
	}

	// The session is opened again in another process: its front end is
	// handed where it stands as it starts, from the session's file. Then
	// a goal is bound and let go, and the status line follows.
	hold := make(chan struct{})
	held := says("I will see to the notes.")
	held.Hold = hold
	w.script.add(held)
	var facts map[string]string
	var tasks []app.Task
	var bound, cleared []string
	f, r = w.attach(func(ctx context.Context, c app.SessionControls, f *front) int {
		facts, tasks = c.Status(), c.Tasks()
		from := f.mark()
		if _, err := c.Command(ctx, "/goal the notes say hi"); err != nil {
			t.Errorf("/goal: %v", err)
		}
		bound = f.facts(from)
		from = f.mark()
		if _, err := c.Command(ctx, "/goal clear"); err != nil {
			t.Errorf("/goal clear: %v", err)
		}
		cleared = f.facts(from)
		close(hold)
		if err := c.Wait(ctx); err != nil {
			t.Errorf("waiting for the directive's turn: %v", err)
		}
		c.Quit("prompt_input_exit")
		return 0
	}, "-y", "-c")
	r.wantCode(t, 0)
	w.script.done()
	if facts["frame"] != todo2 || facts["goal"] != "" {
		t.Errorf("the status facts of a session opened again: %v, want its frame %s and no goal", facts, todo2)
	}
	if got := taskLine(tasks); got != wantLists[2] {
		t.Errorf("the task list of a session opened again:\n%s\nwant:\n%s", got, wantLists[2])
	}
	if got := f.facts(0); len(got) == 0 || got[0] != "frame="+todo2 {
		t.Errorf("the status facts a session opened again sent: %q, want its frame %s first", got, todo2)
	}
	if !reflect.DeepEqual(bound, []string{"goal=notes-say-hi"}) {
		t.Errorf("the status facts sent when a goal was bound: %q", bound)
	}
	if !reflect.DeepEqual(cleared, []string{"goal="}) {
		t.Errorf("the status facts sent when the goal was let go: %q", cleared)
	}
}

// Plan mode entered and left while a session runs. The gnovm tools the
// model is offered are the same list in every mode, so that the fixed
// prompt a model server keeps is the same bytes; what the mode changes is
// which of them run. In plan mode gnovm_list alone runs, the others are
// refused with what plan mode means, and the prompts carry no mem line.
// The moment the person leaves plan mode, all of it is back.
func TestPlanModeEnteredAndLeftMidSession(t *testing.T) {
	w := newWorld(t,
		// In the mode the session began in.
		reply(memCall("c1", `mem.Fact("cmd/test", "go test ./...")`)),
		says("Kept."),
		// In plan mode.
		reply(memCall("c2", `mem.Todo("release v2", "tag it").Push()`)),
		reply(call("c3", "gnovm_list", map[string]any{})),
		says("The plan: tag it."),
		// Out of it again.
		reply(memCall("c4", `mem.Todo("release v2", "tag it").Push()`)),
		says("The plan is on mem."),
	)
	var modes []string
	f, r := w.attach(func(ctx context.Context, c app.SessionControls, f *front) int {
		modes = append(modes, c.Mode())
		f.say(ctx, c, "keep the test command")
		if err := c.SetMode("plan"); err != nil {
			t.Errorf("switching to plan mode: %v", err)
		}
		modes = append(modes, c.Mode())
		f.say(ctx, c, "plan the release")
		if err := c.SetMode("default"); err != nil {
			t.Errorf("leaving plan mode: %v", err)
		}
		modes = append(modes, c.Mode())
		f.say(ctx, c, "go ahead")
		c.Quit("prompt_input_exit")
		return 0
	})
	r.wantCode(t, 0)
	w.script.done()
	if got := strings.Join(modes, " "); got != "default plan default" {
		t.Fatalf("the modes the session went through: %q", got)
	}
	reqs := w.script.requests()
	if len(reqs) != 7 {
		t.Fatalf("the session made %d requests, want 7", len(reqs))
	}

	// The same tools and the same system prompt in every request, plan
	// mode or not: the memory section is there throughout.
	sys, tools := system(t, reqs[0]), strings.Join(toolNamesOf(reqs[0]), " ")
	has(t, "the tools", tools, "gnovm_run gnovm_new gnovm_list gnovm_drop")
	memLine := prompt.Builtin().MemLine()
	for i, req := range reqs {
		wellFormed(t, fmt.Sprintf("request %d", i), req)
		if got := system(t, req); got != sys {
			t.Errorf("request %d has another system prompt than the first", i)
		}
		if got := strings.Join(toolNamesOf(req), " "); got != tools {
			t.Errorf("request %d offers the tools %q, and the first offered %q", i, got, tools)
		}
	}

	// Before plan mode: the mem line, and the call runs.
	if got := lastUser(t, reqs[0]); !strings.HasSuffix(got, "keep the test command\n\n"+memLine) {
		t.Errorf("the prompt typed before plan mode does not end with the mem line:\n%s", got)
	}
	has(t, "the call before plan mode", lastResult(t, reqs[1]), "fact `cmd/test` is new", "ok · mem · ")

	// In plan mode: the prompt is the person's words alone, the call on
	// mem is refused in the words of the text, and gnovm_list runs.
	if got := lastUser(t, reqs[2]); !strings.HasSuffix(got, "plan the release") || strings.Contains(got, memLine) {
		t.Errorf("the prompt typed in plan mode:\n%s", got)
	}
	refused := lastResult(t, reqs[3])
	if want := "This call did not run: " + prompt.Builtin().PlanRefusal("gnovm_run") + "."; refused != want {
		t.Errorf("gnovm_run in plan mode was answered:\n%s\nwant:\n%s", refused, want)
	}
	has(t, "gnovm_list in plan mode", lastResult(t, reqs[4]), "VM budget", "- mem: ")

	// Out of plan mode: the mem line is back, and the same call runs.
	if got := lastUser(t, reqs[5]); !strings.HasSuffix(got, "go ahead\n\n"+memLine) {
		t.Errorf("the prompt typed after plan mode does not end with the mem line:\n%s", got)
	}
	has(t, "the call after plan mode", lastResult(t, reqs[6]), "added `release-v2` \"release v2\"", "in `release-v2`")
	if got := f.facts(0); !reflect.DeepEqual(got, []string{"frame=release-v2"}) {
		t.Errorf("the status facts: %q", got)
	}

	// Nothing reached the GnoVM's mem in plan mode.
	var codes []string
	for _, req := range w.memRequests() {
		codes = append(codes, req.Code)
	}
	if len(codes) != 2 || !strings.Contains(codes[0], "mem.Fact(") || !strings.Contains(codes[1], "mem.Todo(") {
		t.Errorf("the calls that reached mem: %q, want the one before plan mode and the one after", codes)
	}

	// A session that begins in plan mode has the same tools and the same
	// system prompt, and its first call on mem is refused the same way;
	// when the person leaves plan mode, the call runs.
	w.script.add(
		reply(memCall("d1", `mem.Show()`)),
		says("I may only look for now."),
		reply(memCall("d2", `mem.Show()`)),
		says("mem shows the release."),
	)
	before := len(w.script.requests())
	_, r = w.attach(func(ctx context.Context, c app.SessionControls, f *front) int {
		if got := c.Mode(); got != "plan" {
			t.Errorf("the session began in the mode %q", got)
		}
		f.say(ctx, c, "what is planned?")
		if err := c.SetMode("acceptEdits"); err != nil {
			t.Errorf("leaving plan mode: %v", err)
		}
		f.say(ctx, c, "look again")
		c.Quit("prompt_input_exit")
		return 0
	}, "--permission-mode", "plan")
	r.wantCode(t, 0)
	w.script.done()
	later := w.script.requests()[before:]
	if len(later) != 4 {
		t.Fatalf("the session that began in plan mode made %d requests, want 4", len(later))
	}
	if got := system(t, later[0]); got != sys {
		t.Errorf("a session that begins in plan mode has another system prompt")
	}
	if got := strings.Join(toolNamesOf(later[0]), " "); got != tools {
		t.Errorf("a session that begins in plan mode is offered %q, want %q", got, tools)
	}
	first := lastUser(t, later[0])
	has(t, "the first prompt of a session that begins in plan mode", first, "<mem>", "`release-v2` \"release v2\"", "what is planned?")
	hasNot(t, "the first prompt of a session that begins in plan mode", first, memLine)
	if got, want := lastResult(t, later[1]), "This call did not run: "+prompt.Builtin().PlanRefusal("gnovm_run")+"."; got != want {
		t.Errorf("gnovm_run in a session that began in plan mode was answered:\n%s\nwant:\n%s", got, want)
	}
	has(t, "the call once plan mode was left", lastResult(t, later[3]), "`release-v2` \"release v2\"", "ok · mem · ")
}

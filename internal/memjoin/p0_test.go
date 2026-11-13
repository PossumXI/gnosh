package memjoin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/core"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// bigFile is a file large enough that reading it shows in a request's
// size: 300 numbered lines.
func bigFile() string {
	var b strings.Builder
	b.WriteString("hello, world\n")
	for i := 2; i <= 300; i++ {
		fmt.Fprintf(&b, "line %03d of the greeting's long and winding documentation\n", i)
	}
	return b.String()
}

// system returns a request's system prompt.
func system(t *testing.T, r apptest.Request) string {
	t.Helper()
	msgs := messagesOf(r)
	if len(msgs) == 0 || msgs[0].role != "system" {
		t.Fatalf("the request does not begin with a system prompt:\n%s", conversation(r))
	}
	return msgs[0].text
}

// wellFormed fails the test unless every tool call of a request is
// answered in the messages right after it, in order, and no result answers
// a call that is not there: no call is split from its result.
func wellFormed(t *testing.T, what string, r apptest.Request) {
	t.Helper()
	msgs := messagesOf(r)
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		if m.role == "tool" {
			t.Errorf("%s: message %d answers %s, which the message before did not call:\n%s", what, i, m.id, conversation(r))
			continue
		}
		for k, id := range m.calls {
			at := i + 1 + k
			if at >= len(msgs) || msgs[at].role != "tool" || msgs[at].id != id {
				t.Errorf("%s: the call %s of message %d is not answered right after it:\n%s", what, id, i, conversation(r))
			}
		}
		i += len(m.calls)
	}
}

// samePrefix fails the test unless the first n messages of two requests
// are the same bytes.
func samePrefix(t *testing.T, what string, before, after apptest.Request, n int) {
	t.Helper()
	a, b := messagesOf(before), messagesOf(after)
	if len(a) < n || len(b) < n {
		t.Fatalf("%s: the requests hold %d and %d messages, and %d were to be the same", what, len(a), len(b), n)
	}
	for i := 0; i < n; i++ {
		if a[i].raw != b[i].raw {
			t.Errorf("%s: message %d differs between the two requests, so a model server would read the conversation again from there.\nbefore: %s\nafter:  %s", what, i, clip(a[i].raw), clip(b[i].raw))
			return
		}
	}
}

// find returns the index of the first message of a request that makes a
// call whose arguments hold piece, or -1.
func find(r apptest.Request, piece string) int {
	for i, m := range messagesOf(r) {
		for _, a := range m.args {
			if strings.Contains(a, piece) {
				return i
			}
		}
	}
	return -1
}

// The first working run of mem with frames, through the real program: a
// model plans a task in mem with two todos, steps into the first, works on
// a file, and the process ends. A second process takes the session up from
// disk, pops the first todo with a result, does and pops the second, and a
// second session then starts from what the first left open.
//
// What the model server was sent is checked at every request.
func TestP0PlanPushPopAndTheContextShrinks(t *testing.T) {
	const (
		task  = "rename-greeting"
		todo1 = "rename-greeting/change-hello-hi-greet"
		todo2 = "rename-greeting/note-change-notes-md"
	)
	w := newWorld(t)
	w.Write("greet.txt", bigFile())
	notes := w.Write("NOTES.md", "Notes on the greeting.\n")

	// The first process: plan, step in, read and edit, and stop with a
	// question, standing in the first todo.
	w.script.add(
		reply(memCall("c1", `mem.Todo("rename the greeting", "change hello to hi in greet.txt", "note the change in NOTES.md").Push()`)),
		reply(memCall("c2", `mem.Next()`)),
		reply(call("c3", "Read", map[string]any{"file_path": "greet.txt"})),
		reply(call("c4", "Edit", map[string]any{"file_path": "greet.txt", "old_string": "hello", "new_string": "hi"})),
		says("greet.txt says hi now. Shall I go on with the notes?"),
	)
	r := w.run("-y", "-p", "rename the greeting to hi")
	r.wantCode(t, 0)
	if !strings.HasPrefix(w.Read("greet.txt"), "hi, world\n") {
		t.Fatalf("the edit did not go in: %q", w.Read("greet.txt")[:40])
	}
	first := w.script.requests()
	if len(first) != 5 {
		t.Fatalf("the first run made %d requests, want 5", len(first))
	}

	// The memory section is in the system prompt, and the system prompt
	// is the same bytes in every request: nothing volatile is in it.
	sys := system(t, first[0])
	memory := prompt.Builtin().System(prompt.SystemInput{Tools: []core.ToolSpec{{Name: "gnovm_run"}}})[1]
	has(t, "the system prompt", sys, memory)
	hasNot(t, "the system prompt", sys, w.Dir, w.Home, "2026-11-13", "<mem>", "rename-greeting")

	// The first prompt: the environment, the prompt's own words, and the
	// mem line last. mem is empty, so there is no digest; the stack is
	// empty, so there is no frame line.
	memLine := prompt.Builtin().MemLine()
	prompt0 := messagesOf(first[0])[1]
	if prompt0.role != "user" || !strings.HasSuffix(prompt0.text, "rename the greeting to hi\n\n"+memLine) {
		t.Errorf("the first prompt does not end with its words and then the mem line:\n%s", prompt0.text)
	}
	hasNot(t, "the first prompt", prompt0.text, "<mem>", "You are in `")

	// A push changes nothing that was sent before: each request begins
	// with the bytes of the one before it.
	for i := 1; i < len(first); i++ {
		wellFormed(t, fmt.Sprintf("request %d", i), first[i])
		samePrefix(t, fmt.Sprintf("requests %d and %d", i-1, i), first[i-1], first[i], len(messagesOf(first[i-1])))
	}
	has(t, "the answer to the plan", messagesOf(first[1])[3].text,
		"added `"+task+"` \"rename the greeting\"", "added `"+todo1+"`", "added `"+todo2+"`", "in `"+task+"`")
	has(t, "the answer to mem.Next()", messagesOf(first[2])[5].text, "in `"+todo1+"` \"change hello to hi in greet.txt\"")

	// The session's file holds the two pushes, and nothing was started
	// outside the test's own directories.
	sessions := w.Sessions()
	if len(sessions) != 1 {
		t.Fatalf("session files: %v", sessions)
	}
	if got := stackIDs(w.session(sessions[0]).Stack()); got != task+" "+todo1 {
		t.Errorf("the stack read from disk is %q, want the task and its first todo", got)
	}
	if _, err := os.Stat(filepath.Join(w.gnoDir(), "worker.log")); err == nil {
		t.Errorf("a worker was started: the tests run the machines in their own process")
	}

	// The second process takes the session up from disk.
	w.script.add(
		reply(memCall("c5", "mem.Pop(\"greet.txt now says hi\")\nmem.Next()")),
		reply(call("c6", "Read", map[string]any{"file_path": "NOTES.md"})),
		reply(call("c7", "Edit", map[string]any{"file_path": "NOTES.md", "old_string": "Notes on the greeting.", "new_string": "The greeting is hi now."})),
		reply(memCall("c8", `mem.Pop("NOTES.md says the greeting changed")`)),
		reply(call("c9", "Edit", map[string]any{"file_path": "NOTES.md", "old_string": "hi now", "new_string": "hi, now"})),
		reply(call("c10", "Read", map[string]any{"file_path": "NOTES.md"})),
		reply(call("c11", "Edit", map[string]any{"file_path": "NOTES.md", "old_string": "hi now", "new_string": "hi, now"})),
		says("Both todos are done."),
	)
	r = w.run("-y", "-c", "-p", "yes, go on")
	r.wantCode(t, 0)
	all := w.script.requests()
	if len(all) != 13 {
		t.Fatalf("the two runs made %d requests, want 13", len(all))
	}
	second := all[5:]
	for i, req := range second {
		wellFormed(t, fmt.Sprintf("request %d of the second run", i), req)
		if got := system(t, req); got != sys {
			t.Errorf("request %d of the second run has another system prompt than the first run's", i)
		}
	}

	// What was rebuilt from the file is what went out: the second
	// process's first request begins with the bytes of the first
	// process's last.
	samePrefix(t, "the last request before the restart and the first after it", first[4], second[0], len(messagesOf(first[4])))

	// The prompt typed while the session stands in a todo carries the mem
	// line and then the frame line, with the stack as it was read from
	// disk.
	msgs := messagesOf(second[0])
	typed := msgs[len(msgs)-1]
	frameLine := prompt.Builtin().FrameLine([]prompt.Frame{
		{ID: task, Purpose: "rename the greeting"},
		{ID: todo1, Purpose: "change hello to hi in greet.txt"},
	}, nil)
	if typed.role != "user" || !strings.HasSuffix(typed.text, "yes, go on\n\n"+memLine+"\n\n"+frameLine) {
		t.Errorf("the prompt typed inside a todo does not end with its words, the mem line and the frame line:\n%s", typed.text)
	}
	has(t, "the frame line", frameLine, "You are in `"+todo1+"` \"change hello to hi in greet.txt\"", "`"+task+"`, `"+todo1+"`")

	// The pop. Before it the model was sent the file it read; after it
	// the todo's work is gone, and the pop's own result says so.
	before, after := second[0], second[1]
	const line = "line 150 of the greeting's long and winding documentation"
	has(t, "the request that popped", string(before.Body), line, "Shall I go on with the notes?")
	hasNot(t, "the request after the pop", string(after.Body), line, "Shall I go on with the notes?", `\"old_string\":\"hello\"`)
	pop := find(after, "mem.Pop")
	if pop < 0 {
		t.Fatalf("the request after the pop does not hold the pop's call:\n%s", conversation(after))
	}
	receipt := messagesOf(after)[pop+1].text
	has(t, "the pop's result", receipt,
		"done `"+todo1+"` \"change hello to hi in greet.txt\" → greet.txt now says hi\n",
		"in `"+todo2+"` \"note the change in NOTES.md\"",
		// The file was read in the process before this one, which kept no
		// record of it here, so no read was there to forget: the sentence
		// about reads is said only when it is true.
		"Everything since you entered `"+todo1+"` was removed from your context, but for what the user said in this turn: this result stays; the user still sees all of it. In it you edited greet.txt.\nok · mem · ran ",
		" · kept as entry 3")

	// The prompt cache: everything through the call that entered the todo
	// is the same bytes as before the pop, and its answer is as it was but
	// for one line at its end.
	push := find(after, "mem.Next()")
	samePrefix(t, "the requests before and after the pop", before, after, push+1)
	pushed, was := messagesOf(after)[push+1], messagesOf(before)[push+1]
	const note = "[Then came the work in it, 2 tool calls (Edit 1, Read 1), removed from your context when you popped it.]"
	if pushed.text != strings.TrimRight(was.text, "\n")+"\n"+note {
		t.Errorf("the answer to the push after the pop is not the answer before it with the note at its end:\nbefore: %q\nafter:  %q", was.text, pushed.text)
	}
	// What the user said in the pop's own turn stays, right after it.
	if next := messagesOf(after)[push+2]; next.role != "user" || !strings.Contains(next.text, "yes, go on") {
		t.Errorf("the prompt of the pop's own turn does not follow the push's answer:\n%s", conversation(after))
	}
	if !strings.Contains(messagesOf(after)[1].text, "rename the greeting to hi") {
		t.Errorf("the first prompt, typed before the todo was entered, is gone:\n%s", conversation(after))
	}
	t.Logf("the request that made the pop: about %d tokens; the request after it: about %d tokens",
		len(before.Body)/4, len(after.Body)/4)
	if len(after.Body) >= len(before.Body) {
		t.Errorf("the request after the pop (%d bytes) is no smaller than the one before it (%d bytes)", len(after.Body), len(before.Body))
	}

	// The second pop takes the second todo's work out the same way. This
	// todo's file was read in this process, and the read is forgotten.
	has(t, "the request that made the second pop", string(second[3].Body), "Notes on the greeting.")
	hasNot(t, "the request after the second pop", string(second[4].Body), "Notes on the greeting.")
	pop2 := find(second[4], "NOTES.md says the greeting changed")
	has(t, "the second pop's result", messagesOf(second[4])[pop2+1].text,
		"done `"+todo2+"` \"note the change in NOTES.md\" → NOTES.md says the greeting changed\n",
		"Everything since you entered `"+todo2+"` was removed from your context: this result stays; the user still sees all of it. In it you edited NOTES.md. Your reads inside it are forgotten: read a file again before you edit it.\n")
	samePrefix(t, "the requests before and after the second pop", second[3], second[4], pop+1)
	if got := messagesOf(second[4])[pop+1].text; !strings.HasSuffix(got, "\n[Then came the work in it, 2 tool calls (Edit 1, Read 1), removed from your context when you popped it.]") {
		t.Errorf("the answer that entered the second todo does not end with the note of what was removed:\n%s", got)
	}

	// A file read inside a popped todo must be read again before it is
	// changed: the model no longer has what the read vouched for.
	refused := messagesOf(second[5])
	has(t, "the edit of a file read in a popped todo", refused[len(refused)-1].text, "This session has not read "+notes)
	edited := messagesOf(second[7])
	has(t, "the edit after the file was read again", edited[len(edited)-1].text, "Edited "+notes)
	if got := w.Read("NOTES.md"); got != "The greeting is hi, now.\n" {
		t.Errorf("NOTES.md = %q: the edit after a new read did not go in, or the refused one did", got)
	}

	// The person was told what each pop took out of the model's context,
	// in the order the pops were made.
	inOrder(t, "stderr", r.err,
		"popped `"+todo1+"`: about ", " tokens out of the model's context",
		"popped `"+todo2+"`: about ", " tokens out of the model's context")
	w.script.done()

	// Read from disk again, the stack and the cuts are what they were:
	// the session stands in the task, and what a model would be sent is
	// what the last request held.
	s := w.session(w.Sessions()[0])
	if got := stackIDs(s.Stack()); got != task {
		t.Errorf("the stack read from disk is %q, want the task alone", got)
	}
	view, info := s.View(transcript.ViewOptions{Trace: true})
	if err := transcript.Check(view); err != nil {
		t.Errorf("the session's file does not hold a conversation a provider accepts: %v", err)
	}
	var pops []string
	for _, c := range info.Cuts {
		pops = append(pops, fmt.Sprintf("%s %s: %d messages, spared %d", c.Kind, c.Frame, c.Messages, c.Spared))
	}
	if got, want := strings.Join(pops, "; "), "pop "+todo1+": 5 messages, spared 3; pop "+todo2+": 4 messages, spared 0"; got != want {
		t.Errorf("the cuts read from disk are %q, want %q", got, want)
	}
	var sentCalls, viewCalls []string
	for _, m := range messagesOf(second[len(second)-1]) {
		sentCalls = append(sentCalls, m.calls...)
	}
	for _, m := range view {
		for _, u := range m.ToolUses() {
			viewCalls = append(viewCalls, u.ID)
		}
	}
	if strings.Join(viewCalls, " ") != strings.Join(sentCalls, " ") || len(viewCalls) != 7 {
		t.Errorf("the tool calls a model would be sent from disk are %v, and the last request held %v: want the same seven", viewCalls, sentCalls)
	}
	if last := view[len(view)-1]; last.Role != core.RoleAssistant || last.Text() != "Both todos are done." {
		t.Errorf("the view read from disk does not end with the last reply: %+v", last)
	}

	// A second session of the same project starts from what the first
	// left open: its first prompt carries the digest of mem.
	w.script.add(says("The task is open; both its todos are done."))
	r = w.run("-p", "what is left to do?")
	r.wantCode(t, 0)
	reqs := w.script.requests()
	fresh := reqs[len(reqs)-1]
	if got := system(t, fresh); got != sys {
		t.Errorf("the second session's system prompt differs from the first session's: the system prompt must be the same bytes in every session")
	}
	startMsgs := messagesOf(fresh)
	if len(startMsgs) != 2 {
		t.Fatalf("the second session's first request holds %d messages, want the system prompt and the prompt:\n%s", len(startMsgs), conversation(fresh))
	}
	start := startMsgs[1].text
	digestAt, lineAt, wordsAt := strings.Index(start, "<mem>"), strings.Index(start, memLine), strings.Index(start, "what is left to do?")
	if !(wordsAt >= 0 && wordsAt < digestAt && digestAt < lineAt) || !strings.HasSuffix(start, memLine) {
		t.Errorf("the second session's first prompt is not its words, then the digest, then the mem line:\n%s", start)
	}
	has(t, "the digest", start,
		"From your memory (mem), kept by gnosh for this project",
		"open todos, in the order made:\n- `"+task+"` \"rename the greeting\" (2/2 done",
		"</mem>")
	hasNot(t, "the second session's first prompt", start, "You are in `")
	if len(w.Sessions()) != 2 {
		t.Errorf("session files: %v, want two", w.Sessions())
	}
	w.script.done()
}

// stackIDs writes a stack as its ids, bottom to top.
func stackIDs(stack []transcript.Frame) string {
	ids := make([]string, len(stack))
	for i, f := range stack {
		ids[i] = f.ID
	}
	return strings.Join(ids, " ")
}

// The frame entries are written from the frames the tool keeps with its
// result, and only from the events of the session itself.
func TestFrameEntriesAreTheSessionsOwn(t *testing.T) {
	j := &join{id: "s1"}
	raw := `[{"kind":"push","id":"a","purpose":"do a","session":"s1"},
		{"kind":"pop","id":"a","purpose":"do a","result":"done","thoughts":2,"session":"s1"},
		{"kind":"push","id":"b","purpose":"do b","session":"another"},
		{"kind":"wave","id":"c","session":"s1"},
		{"kind":"push","id":"","session":"s1"}]`
	var meta any
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatal(err)
	}
	entries, thoughts := j.frameEntries("call_1", gnovmtools.FramesOf(map[string]any{"frames": meta}))
	if len(entries) != 2 || len(thoughts) != 2 {
		t.Fatalf("entries: %+v", entries)
	}
	if e := entries[0]; e.Op != transcript.OpPush || e.Frame != "a" || e.Label != "do a" || e.ToolUseID != "call_1" {
		t.Errorf("the push: %+v", e)
	}
	if e := entries[1]; e.Op != transcript.OpPop || e.Frame != "a" || e.Result != "done" || e.ToolUseID != "call_1" || thoughts[1] != 2 {
		t.Errorf("the pop: %+v, thoughts %v", e, thoughts)
	}
}

// What a pop's receipt lists as other changes to mem: the calls that
// changed it, by their word and the name they were given.
func TestMemChanges(t *testing.T) {
	for _, c := range []struct{ code, want string }{
		{`mem.Fact("port", "the shared port is 8080")`, "Fact `port`"},
		{`mem.Fact("port")`, ""},
		{`mem.Fact("the shared port is 8080")`, "Fact `the shared port is 8080`"},
		{`mem.Done("x/y", "it holds")`, "Done `x/y`"},
		{"mem.Think(\"a finding\")\nmem.Show()\nmem.Next()\nmem.Pop(\"r\")", ""},
		{`mem.Frame("x").Done("r"); mem.Frame("x/y").Think("t")`, "Done `x`"},
		{`mem.Frame("x").Push()`, ""},
		{"mem.Later(\"fix the flaky test\")\n  mem.Rule(`gofumpt`, \"always\")\nx := 3", "Later `fix the flaky test`, Rule `gofumpt`"},
		{`mem.Todo(title)`, "Todo"},
		{`r := mem.Todo("a"); r.Todo("b")`, ""},
		{`mem.Wait("x", "2026-12-01")`, "Wait `x`"},
	} {
		if got := strings.Join(memChanges(c.code), ", "); got != c.want {
			t.Errorf("memChanges(%q) = %q, want %q", c.code, got, c.want)
		}
	}
}

func TestTokenCount(t *testing.T) {
	for n, want := range map[int]string{-5: "0", 0: "0", 850: "850", 999: "999", 1000: "1K", 1499: "1K", 44400: "44K", 999499: "999K", 999500: "1.0M", 1234567: "1.2M"} {
		if got := tokenCount(n); got != want {
			t.Errorf("tokenCount(%d) = %q, want %q", n, got, want)
		}
	}
}

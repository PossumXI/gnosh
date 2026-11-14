package memjoin

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// filler is a file of about size bytes whose every line holds word, so
// that a test can tell whether a request holds the file.
func filler(word string, size int) string {
	var b strings.Builder
	for i := 1; b.Len() < size; i++ {
		fmt.Fprintf(&b, "%s line %04d: the quick brown fox jumps over the lazy dog\n", word, i)
	}
	return b.String()
}

// The slide, end to end, at a small window. A session that stands in a
// todo and fills its window gets no summary: the oldest rounds of the
// todo leave the model's view instead. The model is told so before its
// next reply, the person is told, the reads of what slid out are
// forgotten, and the next typed prompt's frame line says it again.
func TestSlideInsteadOfASummary(t *testing.T) {
	const (
		task = "review-files"
		todo = "review-files/read-all-four"
	)
	read := func(id, name string) map[string]any { return map[string]any{"file_path": name} }
	w := newWorld(t,
		reply(memCall("c1", `mem.Todo("review the files", "read all four").Push()`)),
		reply(memCall("c2", `mem.Next()`)),
		reply(call("c3", "Read", read("c3", "alpha.txt"))),
		reply(call("c4", "Read", read("c4", "bravo.txt"))),
		reply(call("c5", "Read", read("c5", "charlie.txt"))),
		reply(call("c6", "Read", read("c6", "delta.txt"))),
		// By now the window is full, and the first two reads have slid
		// out: a file read there must be read again, one read after it
		// need not.
		reply(
			call("c7", "Edit", map[string]any{"file_path": "alpha.txt", "old_string": "alpha line 0001", "new_string": "alpha line one"}),
			call("c8", "Edit", map[string]any{"file_path": "delta.txt", "old_string": "delta line 0001", "new_string": "delta line one"}),
		),
		says("I read the four files and changed delta.txt."),
	)
	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		w.Write(name+".txt", filler(name, 6000))
	}
	// The room is made at four fifths of the window. The number was 16000
	// before the question tool joined every session's list of tools; the
	// 833 more keep the point as far above what the session starts with
	// as it was (the tool adds about 666 tokens to every request).
	w.Model.SetWindow(16833)
	r := w.run("-y", "-p", "review the files")
	r.wantCode(t, 0)
	w.script.done()
	reqs := w.script.requests()
	if len(reqs) != 8 {
		t.Fatalf("the run made %d requests, want 8: a slide asks no model for anything", len(reqs))
	}
	for i, req := range reqs {
		wellFormed(t, fmt.Sprintf("request %d", i), req)
		if tools, _ := req.JSON["tools"].([]any); len(tools) == 0 {
			t.Errorf("request %d carries no tools: it is not the session's own", i)
		}
	}

	// Before the slide the model was sent all four files; after it the
	// two oldest reads are gone, and the newest are there.
	before, after := string(reqs[5].Body), string(reqs[6].Body)
	has(t, "the request before the slide", before, "alpha line 0005", "bravo line 0005", "charlie line 0005")
	hasNot(t, "the request after the slide", after, "alpha line 0005", "bravo line 0005")
	has(t, "the request after the slide", after, "charlie line 0005", "delta line 0005")
	t.Logf("the request before the slide: about %d tokens; the request after it: about %d tokens", len(before)/4, len(after)/4)

	// The answer that entered the todo says what went, and why.
	push := find(reqs[6], "mem.Next()")
	if got := messagesOf(reqs[6])[push+1].text; !strings.HasSuffix(got, "\n[Then came the work in it, 2 tool calls (Read 2), removed from your context to make room.]") {
		t.Errorf("the answer that entered the todo does not end with the slide's note:\n%s", got)
	}
	samePrefix(t, "the requests before and after the slide", reqs[5], reqs[6], push+1)

	// The model is told before its next reply, in gnosh's own words.
	told := prompt.Builtin().SlideLine([]prompt.Slide{{Frame: todo}})
	if got := lastUser(t, reqs[6]); got != told {
		t.Errorf("what the model was told of the slide:\n%s\nwant:\n%s", got, told)
	}
	has(t, "the slide's line", told, "The start of `"+todo+"` left your context to make room; mem.Show(\""+todo+"\") has the plan, the results so far and the thoughts. Your reads there are forgotten: read a file again before you edit it.")

	// The person is told too.
	has(t, "stderr", r.err, "slid `"+todo+"`: about ", " tokens of its start out of the model's context, instead of compacting")
	hasNot(t, "stderr", r.err, "summar")

	// The reads of what slid out are forgotten; a read that is still in
	// front of the model stands.
	edits := results(reqs[7])
	has(t, "the edit of a file whose read slid out", edits[len(edits)-2], "This session has not read "+w.Path("alpha.txt"))
	has(t, "the edit of a file whose read is still there", edits[len(edits)-1], "Edited "+w.Path("delta.txt"))

	// The next prompt the person types carries the frame line, and the
	// frame line says once more that the todo's start is gone.
	w.script.add(says("Yes."))
	r = w.run("-c", "-p", "are you still in the todo?")
	r.wantCode(t, 0)
	reqs = w.script.requests()
	typed := lastUser(t, reqs[len(reqs)-1])
	frameLine := prompt.Builtin().FrameLine([]prompt.Frame{
		{ID: task, Purpose: "review the files"},
		{ID: todo, Purpose: "read all four"},
	}, []prompt.Slide{{Frame: todo}})
	if !strings.HasSuffix(typed, "are you still in the todo?\n\n"+prompt.Builtin().MemLine()+"\n\n"+frameLine) {
		t.Errorf("the prompt typed after the slide does not end with the mem line and a frame line that tells of the slide:\n%s", typed)
	}
	has(t, "the frame line", frameLine, "The start of `"+todo+"` left your context to make room")
}

// The large-step line: well before the window fills, a todo that has
// taken in more than half of what the window allows since it was entered
// is said to be large, once.
func TestLargeStepLine(t *testing.T) {
	const todo = "review-files/read-all-four"
	w := newWorld(t,
		reply(memCall("c1", `mem.Todo("review the files", "read all four").Push()`)),
		reply(memCall("c2", `mem.Next()`)),
		reply(call("c3", "Read", map[string]any{"file_path": "alpha.txt"})),
		reply(call("c4", "Read", map[string]any{"file_path": "bravo.txt"})),
		reply(call("c5", "Read", map[string]any{"file_path": "charlie.txt"})),
		reply(call("c6", "Read", map[string]any{"file_path": "delta.txt"})),
		reply(call("c6b", "Read", map[string]any{"file_path": "alpha.txt", "limit": 3})),
		says("That is a lot of reading."),
	)
	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		w.Write(name+".txt", filler(name, 17500))
	}
	w.Model.SetWindow(40000)
	r := w.run("-y", "-p", "review the files")
	r.wantCode(t, 0)
	w.script.done()
	reqs := w.script.requests()

	said := -1
	for i, req := range reqs {
		if strings.Contains(string(req.Body), "This todo is large") {
			said = i
			break
		}
	}
	if said < 0 {
		t.Fatalf("the model was never told that the todo is large:\n%s", conversation(reqs[len(reqs)-1]))
	}
	line := lastUser(t, reqs[said])
	if !strings.HasPrefix(line, "<gnosh>This todo is large: pop it with what you have, or split it into todos under it (you are in `"+todo+"`, and about ") ||
		!strings.HasSuffix(line, " tokens came in since you entered it).</gnosh>") {
		t.Errorf("the large-step line:\n%s", line)
	}
	if n := strings.Count(string(reqs[len(reqs)-1].Body), "This todo is large"); n != 1 {
		t.Errorf("the large-step line was said %d times in one todo, want once", n)
	}
	hasNot(t, "stderr", r.err, "slid `", "summar")

	// It is said once for a todo, also by a process that takes the
	// session up later: the session's file remembers.
	var marks []string
	for _, e := range w.session(w.Sessions()[0]).Entries() {
		if e.Key == metaSaid {
			marks = append(marks, e.Value)
		}
	}
	if len(marks) != 1 || !strings.HasPrefix(marks[0], "large-step:") {
		t.Errorf("what the session's file remembers of what was said: %q", marks)
	}
	w.script.add(reply(call("c7", "Read", map[string]any{"file_path": "alpha.txt", "limit": 5})), says("Still here."))
	r = w.run("-y", "-c", "-p", "carry on")
	r.wantCode(t, 0)
	reqs = w.script.requests()
	if n := strings.Count(string(reqs[len(reqs)-1].Body), "This todo is large"); n != 1 {
		t.Errorf("after a restart the large-step line stands %d times in the conversation, want once", n)
	}
}

// A pop on the round that would have crossed the compaction point: the
// popped work is out of the view before the next request is measured, so
// no room has to be made, by a slide or by a summary, and the session
// goes on well under the point.
func TestPopKeepsTheSessionUnderThePoint(t *testing.T) {
	w := newWorld(t,
		reply(memCall("c1", `mem.Todo("review the files", "read the first three", "read the fourth").Push()`)),
		reply(memCall("c2", `mem.Next()`)),
		reply(call("c3", "Read", map[string]any{"file_path": "alpha.txt"})),
		reply(call("c4", "Read", map[string]any{"file_path": "bravo.txt"})),
		reply(call("c5", "Read", map[string]any{"file_path": "charlie.txt"})),
		reply(memCall("c6", "mem.Pop(\"read alpha.txt, bravo.txt and charlie.txt: each holds numbered lines\")\nmem.Next()")),
		reply(call("c7", "Read", map[string]any{"file_path": "delta.txt"})),
		reply(memCall("c8", `mem.Pop("read delta.txt: numbered lines too")`)),
		says("All four hold numbered lines."),
	)
	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		w.Write(name+".txt", filler(name, 6000))
	}
	// The room is made at four fifths of the window. The number was 16000
	// before the question tool joined every session's list of tools; the
	// 833 more keep the point as far above what the session starts with
	// as it was (the tool adds about 666 tokens to every request).
	w.Model.SetWindow(16833)
	r := w.run("-y", "-p", "review the files")
	r.wantCode(t, 0)
	w.script.done()
	reqs := w.script.requests()
	if len(reqs) != 9 {
		t.Fatalf("the run made %d requests, want 9", len(reqs))
	}
	// With a fourth file read in the same todo the window would have been
	// full (TestSlideInsteadOfASummary); popped in time, nothing slides.
	hasNot(t, "stderr", r.err, "slid `", "summar", "left your context to make room")
	has(t, "stderr", r.err, "popped `review-files/read-first-three`: about ")
	sizes := make([]int, len(reqs))
	for i, req := range reqs {
		sizes[i] = len(req.Body) / 4
		if strings.Contains(string(req.Body), "left your context to make room") {
			t.Errorf("request %d tells the model of a slide", i)
		}
	}
	t.Logf("the size of each request, in tokens as the server counts them: %v", sizes)
	if sizes[6] >= sizes[5] || sizes[6] > 9000 {
		t.Errorf("the request after the pop is about %d tokens, and the one that made it was about %d: the popped reads are still being sent", sizes[6], sizes[5])
	}
	for i, n := range sizes {
		if n > 13466 {
			t.Errorf("request %d is about %d tokens, over the point at which room is made for a window of 16833", i, n)
		}
	}
}

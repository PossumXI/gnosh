package transcript

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// The example lines in doc.go are real: each reads as an entry of a type
// this version defines, with nothing left over, and is written back as it
// stands there.
func TestDocExamples(t *testing.T) {
	doc, err := os.ReadFile("doc.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(doc), "\n") {
		example, ok := strings.CutPrefix(line, "//\t{")
		if !ok {
			continue
		}
		example = "{" + example
		var e Entry
		if err := e.UnmarshalJSON([]byte(example)); err != nil {
			t.Errorf("an example in doc.go is not an entry: %v\n%s", err, example)
			continue
		}
		if !knownType(e.Type) || len(e.Extra) != 0 || len(e.kept) != 0 || e.V != Version || e.Timestamp.IsZero() {
			t.Errorf("an example in doc.go has something this version does not define: %+v\n%s", e, example)
		}
		again, err := e.line()
		if err != nil || !sameJSON(t, string(again), example) {
			t.Errorf("an example in doc.go is written back differently (%v):\n%s\n%s", err, example, again)
		}
		seen[string(e.Type)+" "+string(e.Op)+" "+string(e.Source)] = true
	}
	for _, want := range []string{
		"user  prompt", "user  tool_results", "assistant  ", "frame push ", "frame pop ", "frame slide ",
		"summary  ", "rewind  ", "title  user", "meta  ", "usage  ", "branch  ",
	} {
		if !seen[want] {
			t.Errorf("doc.go has no example line for %q", want)
		}
	}
}

// The worked example in doc.go, word for word.
func TestDocWorkedExample(t *testing.T) {
	info := expectView(t, `
		prompt fix the flaky test
		call c1:gnovm_run
		results c1
		push fix-flaky c1
		call c2:Read
		results c2
		say Which port does CI use?
		@p prompt 8080
		call c3:Edit                    # a wrong turn, rewound below
		results c3
		rewind @p
		call c4:Edit
		results c4
		prompt also say so in the README
		call c5:gnovm_run
		results c5
		pop fix-flaky c5 the test binds port 0 now
		call c6:Read                    # gnosh was killed before this came back
	`, ViewOptions{Budget: 100_000, Trace: true}, `
		U: fix the flaky test
		A: call(c1)
		U: result(c1)* + 8080 + also say so in the README
		A: call(c5)
		U: result(c5)
		A: call(c6)
		U: interrupted(c6)
	`)
	if info.Rewound != 2 || len(info.Cuts) != 1 || len(info.Repairs) != 1 || info.Merged != 2 || len(info.NewSlides) != 0 {
		t.Errorf("info = %+v", info)
	}
	c := info.Cuts[0]
	if c.Note != "[Then came the work in it, 2 tool calls (Edit 1, Read 1), removed from your context when you popped it.]" ||
		c.Spared != 2 || strings.Join(c.Removed, " ") != "e5 e6 e7 e12 e13" {
		t.Errorf("cut = %+v", c)
	}
	if got := strings.Join(info.Sources[2], " "); got != "e3 p e14" {
		t.Errorf("the third message comes from %q", got)
	}
}

// TestViewScales guards against a view whose cost grows with the square of
// the session. Each of these would take minutes that way; the bounds are
// far above what they take now.
func TestViewScales(t *testing.T) {
	if testing.Short() {
		t.Skip("builds transcripts of 200,000 entries")
	}
	todos, notices := 25_000, 100_000
	if raceDetector {
		todos, notices = 5_000, 20_000
	}
	var entries []Entry
	add := func(e Entry) {
		e.UUID = fmt.Sprintf("e%d", len(entries)+1)
		e.SessionID = "s"
		entries = append(entries, e)
	}
	calls := 0
	round := func(tool string) string {
		calls++
		id := fmt.Sprintf("t%d", calls)
		add(AssistantEntry(core.Message{Blocks: []core.Block{core.ToolUse(id, tool, nil)}}, core.StopToolUse))
		add(UserEntry(core.Message{Blocks: []core.Block{core.ToolResultBlock(id, core.TextResult("ok"))}}, SourceToolResults))
		return id
	}
	timed := func(what string, opts ViewOptions) ([]core.Message, ViewInfo) {
		t.Helper()
		start := time.Now()
		msgs, info := View(entries, opts)
		took := time.Since(start)
		t.Logf("%s: %d entries, %d messages in the view, in %v", what, len(entries), len(msgs), took.Round(time.Millisecond))
		if took > 10*time.Second {
			t.Errorf("%s: the view took %v", what, took)
		}
		if err := Check(msgs); err != nil {
			t.Errorf("%s: %v", what, err)
		}
		return msgs, info
	}

	// One prompt, then a long run of its own: an outer frame, and inside
	// it 25,000 todos, each entered, worked on for a round and popped.
	// Every fourth pop names a call that is nowhere in the conversation,
	// so it cuts nothing.
	add(PromptEntry("go"))
	add(PushEntry("outer", "", round("mem")))
	for i := range todos {
		frame := fmt.Sprintf("f%d", i)
		add(PushEntry(frame, "", round("mem")))
		round("Read")
		id := round("mem")
		if i%4 == 0 {
			id = "no-such-call"
		}
		add(PopEntry(frame, "", "done", id))
	}
	msgs, info := timed("a long run of frames", ViewOptions{})
	if len(info.Cuts) != todos || len(info.Stack) != 1 {
		t.Errorf("%d cuts and a stack of %d, want %d and 1", len(info.Cuts), len(info.Stack), todos)
	}
	// The prompt, the outer push round, and of each todo its push round
	// and its pop round; the round of work too where the pop cut nothing.
	if want := 1 + 2 + 4*todos + 2*(todos/4); len(msgs) != want {
		t.Errorf("the view has %d messages, want %d", len(msgs), want)
	}
	// Over its budget, it slides the outer frame's whole run.
	_, info = timed("the same, sliding", ViewOptions{Budget: 1000})
	if len(info.NewSlides) != 1 {
		t.Errorf("planned %d slides, want 1", len(info.NewSlides))
	}

	// 100,000 notices in a row are one message.
	entries = nil
	add(PromptEntry("go"))
	for i := range notices {
		add(UserEntry(core.UserText(fmt.Sprintf("notice %d", i)), SourceNotice))
	}
	msgs, info = timed("a long run of notices", ViewOptions{})
	if len(msgs) != 1 || len(msgs[0].Blocks) != notices+1 || info.Merged != notices {
		t.Errorf("the view has %d messages; merged %d", len(msgs), info.Merged)
	}
}

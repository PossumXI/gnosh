package transcript_test

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// What the loop's join does around a pop, in order, with nothing but the
// names this package exports (doc.go, "What the loop does with it").
func Example() {
	dir, _ := os.MkdirTemp("", "gnosh-transcript-example")
	defer os.RemoveAll(dir)
	s, _ := transcript.New(dir).Create("/work/app")
	defer s.Close()

	call := func(id, tool, input string) transcript.Entry {
		m := core.Message{Blocks: []core.Block{core.ToolUse(id, tool, json.RawMessage(input))}}
		return transcript.AssistantEntry(m, core.StopToolUse)
	}
	result := func(id, text string) transcript.Entry {
		m := core.Message{Blocks: []core.Block{core.ToolResultBlock(id, core.TextResult(text))}}
		return transcript.UserEntry(m, transcript.SourceToolResults)
	}

	// The model entered a todo and worked in it. Now it calls mem.Pop: its
	// reply is stored, and the call is about to run.
	s.AppendAll(
		transcript.PromptEntry("fix the flaky test"),
		call("c1", "gnovm_run", `{"name":"mem","code":"mem.Todo(\"fix the flaky test\").Push()"}`),
		result("c1", "in `fix-flaky`"),
		transcript.PushEntry("fix-flaky", "fix the flaky test", "c1"),
		call("c2", "Read", `{"file_path":"/work/app/listen_test.go"}`),
		result("c2", "package net …"),
		call("c3", "Edit", `{"file_path":"/work/app/listen_test.go"}`),
		result("c3", "edited"),
		call("c4", "gnovm_run", `{"name":"mem","code":"mem.Pop(\"it binds port 0 now\")"}`),
	)

	// Before the call runs: the stack mem is told, and the rows that say
	// whether any work was done in the frame the session stands in.
	stack := s.Stack()
	top := stack[len(stack)-1]
	fmt.Println("the session stands in", top.ID)
	would, _ := s.Preview([]transcript.Entry{transcript.PopEntry(top.ID, "", "", "c4")}, transcript.ViewOptions{})
	for _, reply := range would[0].Replies {
		for _, c := range reply.Calls {
			fmt.Printf("in it: %s, failed: %v\n", c.Name, c.IsError)
		}
	}

	// mem kept the call and handed back one event, a pop. What it removes
	// is asked before anything is stored, since the result says it.
	frames := []transcript.Entry{transcript.PopEntry("fix-flaky", "fix the flaky test", "it binds port 0 now", "c4")}
	cuts, after := s.Preview(frames, transcript.ViewOptions{})
	cut := cuts[0]
	var edited []string
	for _, reply := range cut.Replies {
		for _, c := range reply.Calls {
			if c.Name == "Edit" && c.Answered && !c.IsError && !slices.Contains(edited, c.Path) {
				edited = append(edited, c.Path)
			}
		}
	}
	text := "done `fix-flaky` → it binds port 0 now\n"
	if cut.Between == 0 {
		text += "nothing came between, so nothing was removed from your context."
	} else {
		text += "Everything since you entered `" + cut.Frame + "` was removed from your context"
		if cut.Spared > 0 {
			text += ", but for what the user said in this turn"
		}
		text += ": this result stays."
		for _, path := range edited {
			text += " In it you edited " + path + "."
		}
	}
	fmt.Println(text)
	fmt.Println("frames left on the stack:", len(after))
	fmt.Println("the estimate grows by the cut's tokens:", cut.Tokens > 0)

	// The result and the pop are stored together.
	s.AppendAll(append([]transcript.Entry{result("c4", text)}, frames...)...)

	// The next request.
	msgs, info := s.View(transcript.ViewOptions{Trace: true})
	for _, m := range msgs {
		for _, b := range m.Blocks {
			switch b.Type {
			case core.BlockText:
				fmt.Printf("%s: %s\n", m.Role, b.Text)
			case core.BlockToolUse:
				fmt.Printf("%s: calls %s (%s)\n", m.Role, b.Name, b.ID)
			case core.BlockToolResult:
				fmt.Printf("%s: result of %s: %s\n", m.Role, b.ToolUseID, core.TextOf(b.Content))
			}
		}
	}
	// What the view says of the stored pop is what the preview said.
	stored := info.Cuts[0]
	fmt.Println("the same cut:", stored.Note == cut.Note && stored.Tokens == cut.Tokens && slices.Equal(stored.Removed, cut.Removed))

	// Output:
	// the session stands in fix-flaky
	// in it: Read, failed: false
	// in it: Edit, failed: false
	// done `fix-flaky` → it binds port 0 now
	// Everything since you entered `fix-flaky` was removed from your context: this result stays. In it you edited /work/app/listen_test.go.
	// frames left on the stack: 0
	// the estimate grows by the cut's tokens: true
	// user: fix the flaky test
	// assistant: calls gnovm_run (c1)
	// user: result of c1: in `fix-flaky`
	// [Then came the work in it, 2 tool calls (Edit 1, Read 1), removed from your context when you popped it.]
	// assistant: calls gnovm_run (c4)
	// user: result of c4: done `fix-flaky` → it binds port 0 now
	// Everything since you entered `fix-flaky` was removed from your context: this result stays. In it you edited /work/app/listen_test.go.
	// the same cut: true
}

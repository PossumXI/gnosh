package app

import (
	"encoding/json"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// The transcript view's entries: the conversation the rewinds left, each
// prompt, reply and result whole, the work of a popped step too, a
// reply's model from the usage entry written after it, and what each
// entry is.
func TestTranscriptEntries(t *testing.T) {
	ts, err := transcript.New(t.TempDir()).Create("/work/app")
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()
	reply := core.Message{Role: core.RoleAssistant, Blocks: []core.Block{
		{Type: core.BlockThinking, Text: "read it first"},
		core.ToolUse("c1", "Read", json.RawMessage(`{"file_path":"/work/app/a.go"}`)),
	}}
	if _, err := ts.Append(transcript.PromptEntry("an earlier prompt")); err != nil {
		t.Fatal(err)
	}
	first, err := ts.Append(transcript.PromptEntry("fix the test"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ts.AppendAll(
		transcript.PromptEntry("a turn the rewind cuts off"),
		transcript.RewindEntry(first.UUID),
		transcript.PushEntry("fix", "make the test pass", "c0"),
		transcript.AssistantEntry(reply, core.StopToolUse),
		transcript.UsageEntry(core.Usage{InputTokens: 10}, "qwen/qwen3.8-27b", 0),
		transcript.UserEntry(core.Message{Role: core.RoleUser, Blocks: []core.Block{core.ToolResultBlock("c1", core.TextResult("package app"))}}, transcript.SourceToolResults),
		transcript.PopEntry("fix", "make the test pass", "it passes", "c2"),
		transcript.UserEntry(core.UserText("a hook's word"), transcript.SourceHook),
	)
	if err != nil {
		t.Fatal(err)
	}
	got := (&live{ts: ts}).transcriptEntries()
	var kinds []string
	for _, e := range got {
		kinds = append(kinds, e.Kind)
	}
	want := []string{"prompt", "prompt", "frame", "reply", "results", "frame", "hook"}
	if len(kinds) != len(want) {
		t.Fatalf("kinds %q, want %q", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("kinds %q, want %q", kinds, want)
		}
	}
	if got[1].Message.Blocks[0].Text != "fix the test" {
		t.Errorf("the second prompt is %q", got[1].Message.Blocks[0].Text)
	}
	if r := got[3]; r.Model != "qwen/qwen3.8-27b" || len(r.Message.Blocks) != 2 || r.Message.Blocks[0].Text != "read it first" {
		t.Errorf("the reply: model %q, blocks %+v", r.Model, r.Message.Blocks)
	}
	if f := got[5]; f.Op != "pop" || f.Frame != "fix" || f.Text != "it passes" {
		t.Errorf("the pop: %+v", f)
	}
	if (&live{}).transcriptEntries() != nil {
		t.Errorf("a session with no file has entries")
	}
}

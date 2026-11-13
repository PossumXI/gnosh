package core

import (
	"encoding/json"
	"errors"
	"iter"
	"testing"
)

func seqOf(evs []Event, err error) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {
		for _, ev := range evs {
			if !yield(ev, nil) {
				return
			}
		}
		if err != nil {
			yield(Event{}, err)
		}
	}
}

func TestAccumulatorBuildsMessage(t *testing.T) {
	evs := []Event{
		{Type: EventThinkingDelta, Text: "let me "},
		{Type: EventThinkingDelta, Text: "look"},
		{Type: EventThinkingSignature, Signature: "sig1"},
		{Type: EventThinkingDelta, Text: "second thought"},
		{Type: EventTextDelta, Text: "I'll read "},
		{Type: EventTextDelta, Text: ""},
		{Type: EventTextDelta, Text: "two files."},
		{Type: EventToolUseStart, ID: "a", Name: "Read"},
		{Type: EventToolUseStart, ID: "b", Name: "Read"},
		{Type: EventToolUseDelta, ID: "b", Text: `{"file_path":`},
		{Type: EventToolUseDelta, ID: "a", Text: `{ "file_path": "/x" }`},
		{Type: EventToolUseDelta, ID: "b", Text: `"/y"}`},
		{Type: EventToolUseStop, ID: "a"},
		{Type: EventToolUseDelta, ID: "nope", Text: `ignored`},
		{Type: EventUsage, Usage: Usage{InputTokens: 1}},
		{Type: EventUsage, Usage: Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 7}},
		{Type: EventStop, StopReason: StopToolUse},
	}
	msg, usage, stop, err := Collect(seqOf(evs, nil))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Role != RoleAssistant {
		t.Fatalf("role = %q", msg.Role)
	}
	want := []Block{
		{Type: BlockThinking, Text: "let me look", Signature: "sig1"},
		{Type: BlockThinking, Text: "second thought"},
		{Type: BlockText, Text: "I'll read two files."},
		{Type: BlockToolUse, ID: "a", Name: "Read", Input: json.RawMessage(`{"file_path":"/x"}`)},
		{Type: BlockToolUse, ID: "b", Name: "Read", Input: json.RawMessage(`{"file_path":"/y"}`)},
	}
	if len(msg.Blocks) != len(want) {
		t.Fatalf("got %d blocks, want %d: %+v", len(msg.Blocks), len(want), msg.Blocks)
	}
	for i := range want {
		g, w := msg.Blocks[i], want[i]
		if g.Type != w.Type || g.Text != w.Text || g.Signature != w.Signature ||
			g.ID != w.ID || g.Name != w.Name || string(g.Input) != string(w.Input) {
			t.Errorf("block %d = %+v, want %+v", i, g, w)
		}
	}
	if usage.PromptTokens() != 17 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v", usage)
	}
	if stop != StopToolUse {
		t.Errorf("stop = %q", stop)
	}
	if got := msg.Text(); got != "I'll read two files." {
		t.Errorf("Text() = %q", got)
	}
	if n := len(msg.ToolUses()); n != 2 {
		t.Errorf("ToolUses() = %d", n)
	}
	if _, err := json.Marshal(msg); err != nil {
		t.Errorf("message does not encode: %v", err)
	}
}

func TestAccumulatorBadAndEmptyInputs(t *testing.T) {
	var a Accumulator
	a.Add(Event{Type: EventToolUseStart, ID: "empty", Name: "Glob"})
	a.Add(Event{Type: EventToolUseStart, ID: "cut", Name: "Edit"})
	a.Add(Event{Type: EventToolUseDelta, ID: "cut", Text: `{"file_path": "/x", "old_str`})
	a.Add(Event{Type: EventToolUseStart, ID: "list", Name: "Edit"})
	a.Add(Event{Type: EventToolUseDelta, ID: "list", Text: `[1,2]`})

	msg := a.Message()
	for _, b := range msg.Blocks {
		if string(b.Input) != "{}" {
			t.Errorf("%s input = %s, want {}", b.ID, b.Input)
		}
	}
	bad := a.BadInputs()
	if len(bad) != 2 || bad["cut"] == "" || bad["list"] != "[1,2]" {
		t.Errorf("BadInputs = %v", bad)
	}
	if _, ok := bad["empty"]; ok {
		t.Errorf("empty arguments should count as {}")
	}
	if _, err := json.Marshal(msg); err != nil {
		t.Errorf("message does not encode: %v", err)
	}
}

func TestAccumulatorSignatureWithoutThinking(t *testing.T) {
	var a Accumulator
	a.Add(Event{Type: EventTextDelta, Text: "hi"})
	a.Add(Event{Type: EventThinkingSignature, Opaque: "redacted"})
	a.Add(Event{Type: EventTextDelta, Text: "there"})
	msg := a.Message()
	if len(msg.Blocks) != 3 || msg.Blocks[1].Type != BlockThinking || msg.Blocks[1].Opaque != "redacted" {
		t.Fatalf("blocks = %+v", msg.Blocks)
	}
	if msg.Text() != "hithere" {
		t.Errorf("Text() = %q", msg.Text())
	}
}

func TestCollectKeepsPartialOnError(t *testing.T) {
	boom := &ProviderError{Provider: "local", Kind: ErrTimeout, Timeout: "idle"}
	msg, _, stop, err := Collect(seqOf([]Event{{Type: EventTextDelta, Text: "half an ans"}}, boom))
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if msg.Text() != "half an ans" || stop != "" {
		t.Errorf("partial = %q, stop = %q", msg.Text(), stop)
	}
	var pe *ProviderError
	if !errors.As(err, &pe) || !pe.Retryable() {
		t.Errorf("want a retryable ProviderError, got %v", err)
	}
	if got := pe.Error(); got != "local: timeout (idle)" {
		t.Errorf("Error() = %q", got)
	}
}

package llm

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// TestRequestsOnlyGrow checks what a local model server's cache lives on: a
// session's next request begins with exactly what the request before it
// said. The server reuses what it has read only up to the first token that
// differs, so a turn that is written differently the second time it is
// sent costs a long conversation minutes of reading. Hosted caches work on
// the same principle.
func TestRequestsOnlyGrow(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake")
	turns := []core.Message{
		{Role: core.RoleUser, Blocks: []core.Block{core.Text("<reminder>be brief</reminder>"), core.Text("Read main.go")}},
		{Role: core.RoleAssistant, Blocks: []core.Block{
			{Type: core.BlockThinking, Text: "I should read it.", Signature: "sig-1"},
			core.Text("I'll read it."),
			core.ToolUse("call_1", "Read", json.RawMessage(`{ "file_path" : "main.go" }`)),
			core.ToolUse("call_2", "Shot", nil),
		}},
		{Role: core.RoleUser, Blocks: []core.Block{
			core.ToolResultBlock("call_1", core.TextResult("package main")),
			{Type: core.BlockToolResult, ToolUseID: "call_2", Content: []core.Block{core.Text("A picture."), {Type: core.BlockImage, Data: png}}},
			core.Text("Go on."),
		}},
		{Role: core.RoleAssistant, Blocks: []core.Block{{Type: core.BlockThinking, Text: "", Signature: "sig-2"}, core.Text("Done.")}},
		{Role: core.RoleUser, Blocks: []core.Block{core.Text("Look:"), {Type: core.BlockImage, MediaType: "image/png", Data: png}}},
		{Role: core.RoleAssistant, Blocks: []core.Block{core.ToolUse("call_3", "Glob", json.RawMessage(`{"pattern":"*.go"}`))}},
		{Role: core.RoleUser, Blocks: []core.Block{core.ToolResultBlock("call_3", core.ErrorResult("no matches"))}},
	}
	base := core.Request{
		Model:    "m",
		System:   []string{"You are gnosh.", "The project is a compiler."},
		Tools:    []core.ToolSpec{{Name: "Read", Description: "Reads a file."}, {Name: "Shot"}, {Name: "Glob"}},
		Thinking: core.Thinking{Mode: "on", Effort: "high"},
	}

	type sentBody struct {
		System   json.RawMessage   `json:"system"`
		Tools    json.RawMessage   `json:"tools"`
		Messages []json.RawMessage `json:"messages"`
	}
	for _, typ := range []string{TypeOpenAI, TypeLlamaCpp, TypeAnthropic} {
		for _, history := range []string{"", "keep", "drop"} {
			p, err := New(Config{Type: typ, BaseURL: "http://203.0.113.7", HistoryThinking: history})
			if err != nil {
				t.Fatal(err)
			}
			var before sentBody
			for n := 1; n <= len(turns); n += 2 {
				req := base
				req.Messages = turns[:n]
				raw, err := p.(wire).encode(&req)
				if err != nil {
					t.Fatal(err)
				}
				// The same request is written the same way every time.
				if again, _ := p.(wire).encode(&req); !bytes.Equal(raw, again) {
					t.Errorf("%s/%s: one request, written twice, came out differently", typ, history)
				}
				// Anthropic's cache marks move with the end of the
				// conversation and are not part of what the model
				// reads, so they are set aside.
				raw = bytes.ReplaceAll(raw, []byte(`"cache_control":{"type":"ephemeral"},`), nil)
				if bytes.Contains(raw, []byte("cache_control")) {
					t.Fatalf("%s/%s: a cache mark in an unexpected place: %s", typ, history, raw)
				}
				var now sentBody
				if err := json.Unmarshal(raw, &now); err != nil {
					t.Fatal(err)
				}
				if n > 1 {
					if !bytes.Equal(now.System, before.System) || !bytes.Equal(now.Tools, before.Tools) {
						t.Errorf("%s/%s: the system prompt or the tools changed between requests", typ, history)
					}
					if len(now.Messages) <= len(before.Messages) {
						t.Fatalf("%s/%s: request %d holds %d messages, the one before held %d", typ, history, n, len(now.Messages), len(before.Messages))
					}
					for i, old := range before.Messages {
						if !bytes.Equal(now.Messages[i], old) {
							t.Errorf("%s/%s: message %d was sent as\n  %s\nand is now sent as\n  %s", typ, history, i, old, now.Messages[i])
						}
					}
				}
				before = now
			}
		}
	}
}

package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/core"
)

// This file is --include-partial-messages (spec/cc/output-formats.md 4.4):
// in stream-json, the main agent's replies also come as the Messages API's
// streaming events, synthesized from what gnosh's provider streams, since
// gnosh's models need not speak that protocol. A reply is message_start,
// then for each of its blocks content_block_start, its pieces as
// content_block_delta (text_delta, thinking_delta, input_json_delta), and
// content_block_stop; then message_delta and message_stop. A block ends
// where the next begins, except the last, which ends after the reply's
// assistant lines, as Claude Code writes the assistant line of a block
// just before its stop. A reply whose provider sent it whole is told as
// one piece a block. Sub-agents' replies are never streamed this way.

// streamed is the main agent's reply being streamed.
type streamed struct {
	open bool
	id   string
	// model is the model the request went to, and sent when it went.
	model string
	sent  time.Time
	// next is the index the next block gets, and block the kind of the
	// one open ("" for none): "text", "thinking", or "tool_use:" and the
	// call's id.
	next  int
	block string
	index int
}

// streamEventLine is a stream_event line.
type streamEventLine struct {
	Type            string  `json:"type"`
	Event           any     `json:"event"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
	UUID            string  `json:"uuid"`
	SessionID       string  `json:"session_id"`
	TTFTMS          *int64  `json:"ttft_ms,omitempty"`
}

type evMessageStart struct {
	Type    string     `json:"type"`
	Message apiMessage `json:"message"`
}

type evBlockStart struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock any    `json:"content_block"`
}

type evBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta any    `json:"delta"`
}

type evBlockStop struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

type evMessageDelta struct {
	Type  string         `json:"type"`
	Delta evStop         `json:"delta"`
	Usage evOutputTokens `json:"usage"`
}

type evStop struct {
	StopReason   *string `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

type evOutputTokens struct {
	OutputTokens int `json:"output_tokens"`
}

type evType struct {
	Type string `json:"type"`
}

type textDelta struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type thinkingDelta struct {
	Type     string `json:"type"`
	Thinking string `json:"thinking"`
}

type inputDelta struct {
	Type        string `json:"type"`
	PartialJSON string `json:"partial_json"`
}

// partialOn says the run writes stream events.
func (f *printer) partialOn() bool {
	return f.inv.IncludePartialMessages && f.format == cli.FormatStreamJSON
}

// messageID is the id of the n-th message of the session.
func (f *printer) messageID(n int) string {
	return fmt.Sprintf("msg_%s_%d", shortID(strings.ReplaceAll(f.sessionID(), "-", "")), n)
}

func (f *printer) streamEvent(ev any, ttft *int64) {
	f.init()
	f.encode(streamEventLine{Type: "stream_event", Event: ev, UUID: newUUID(), SessionID: f.sessionID(), TTFTMS: ttft})
}

// partialPiece takes in the next piece of the main agent's reply.
func (f *printer) partialPiece(ev any) {
	if !f.partialOn() {
		return
	}
	switch ev := ev.(type) {
	case *agent.TextEvent:
		if f.sm.block != "text" {
			f.partialBlock("text", textBlock{Type: "text"})
		}
		f.partialDelta(textDelta{Type: "text_delta", Text: ev.Text})
	case *agent.ThinkingEvent:
		if f.sm.block != "thinking" {
			f.partialBlock("thinking", thinkingBlock{Type: "thinking"})
		}
		f.partialDelta(thinkingDelta{Type: "thinking_delta", Thinking: ev.Text})
	case *agent.ToolCallEvent:
		f.partialBlock("tool_use:"+ev.CallID, toolUseBlock{Type: "tool_use", ID: ev.CallID, Name: ev.Name, Input: json.RawMessage("{}")})
	case *agent.ToolInputEvent:
		if f.sm.block == "tool_use:"+ev.CallID {
			f.partialDelta(inputDelta{Type: "input_json_delta", PartialJSON: ev.Text})
		}
	}
}

// partialOpen starts the message being streamed, if it has not started.
func (f *printer) partialOpen() {
	if f.sm.open {
		return
	}
	f.msgSeq++
	f.sm.open, f.sm.id, f.sm.next, f.sm.block = true, f.messageID(f.msgSeq), 0, ""
	var ttft *int64
	if !f.sm.sent.IsZero() {
		ms := f.p.now().Sub(f.sm.sent).Milliseconds()
		ttft = &ms
	}
	f.streamEvent(evMessageStart{Type: "message_start", Message: apiMessage{
		ID: f.sm.id, Type: "message", Role: "assistant", Model: f.sm.model, Content: []any{},
	}}, ttft)
}

// partialBlock starts a block, ending the one before it.
func (f *printer) partialBlock(kind string, start any) {
	f.partialOpen()
	f.partialStop()
	f.sm.block, f.sm.index = kind, f.sm.next
	f.sm.next++
	f.streamEvent(evBlockStart{Type: "content_block_start", Index: f.sm.index, ContentBlock: start}, nil)
}

func (f *printer) partialDelta(delta any) {
	f.streamEvent(evBlockDelta{Type: "content_block_delta", Index: f.sm.index, Delta: delta}, nil)
}

// partialStop ends the open block, if there is one.
func (f *printer) partialStop() {
	if f.sm.block == "" {
		return
	}
	f.streamEvent(evBlockStop{Type: "content_block_stop", Index: f.sm.index}, nil)
	f.sm.block = ""
}

// partialWhole tells a reply that was not streamed as one piece a block,
// leaving the last block open for its assistant line.
func (f *printer) partialWhole(msg core.Message) {
	if !f.partialOn() || f.sm.open {
		return
	}
	f.partialOpen()
	for _, b := range msg.Blocks {
		switch b.Type {
		case core.BlockText:
			f.partialPiece(&agent.TextEvent{Text: b.Text})
		case core.BlockThinking:
			if b.Text != "" {
				f.partialPiece(&agent.ThinkingEvent{Text: b.Text})
			}
		case core.BlockToolUse:
			f.partialPiece(&agent.ToolCallEvent{CallID: b.ID, Name: b.Name})
			if len(b.Input) > 0 {
				f.partialPiece(&agent.ToolInputEvent{CallID: b.ID, Text: string(b.Input)})
			}
		}
	}
}

// partialEnd ends the message being streamed: its last block, then
// message_delta and message_stop. A message cut off ends the same way.
func (f *printer) partialEnd(stop core.StopReason, usage core.Usage) {
	if !f.partialOn() || !f.sm.open {
		return
	}
	f.partialStop()
	f.streamEvent(evMessageDelta{Type: "message_delta", Delta: evStop{StopReason: stopWord(stop)}, Usage: evOutputTokens{OutputTokens: usage.OutputTokens}}, nil)
	f.streamEvent(evType{Type: "message_stop"}, nil)
	f.sm = streamed{model: f.sm.model, sent: f.sm.sent}
}

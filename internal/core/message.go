// Package core is gnosh's shared vocabulary: the handful of types every other
// package agrees on. It holds no behaviour beyond putting those types
// together, and it depends on nothing but the standard library.
//
// A conversation is a list of Messages that alternate between the user and
// the assistant. A Message is a list of Blocks: text, the model's reasoning,
// a request to run a tool, the result of running one, or an image. Tool
// results travel in user messages, because they are what the harness says
// back to the model.
package core

import (
	"encoding/json"
	"strings"
)

// Role says who a message is from.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// BlockType says what a Block holds.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockThinking   BlockType = "thinking"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockImage      BlockType = "image"
)

// Block is one piece of a message. Which fields are set depends on Type.
type Block struct {
	Type BlockType `json:"type"`

	// Text is the words of a text block or the reasoning of a thinking block.
	Text string `json:"text,omitempty"`

	// Signature is a provider's proof that a thinking block is its own. A
	// provider that issues one wants it sent back unchanged.
	Signature string `json:"signature,omitempty"`

	// Opaque is a provider payload that stands in for reasoning the provider
	// will not show. It is sent back unchanged and never displayed.
	Opaque string `json:"opaque,omitempty"`

	// ID names a tool_use block, so that its result can point back at it.
	ID string `json:"id,omitempty"`

	// Name is the tool a tool_use block asks for.
	Name string `json:"name,omitempty"`

	// Input is the tool_use block's arguments: one JSON object.
	Input json.RawMessage `json:"input,omitempty"`

	// ToolUseID is the ID of the tool_use block a tool_result answers.
	ToolUseID string `json:"tool_use_id,omitempty"`

	// Content is what a tool_result says: text and image blocks.
	Content []Block `json:"content,omitempty"`

	// IsError marks a tool_result that reports a failure.
	IsError bool `json:"is_error,omitempty"`

	// MediaType and Data are an image block's type ("image/png") and bytes.
	MediaType string `json:"media_type,omitempty"`
	Data      []byte `json:"data,omitempty"`
}

// Message is one turn of the conversation.
type Message struct {
	Role   Role    `json:"role"`
	Blocks []Block `json:"content"`
}

// Text makes a text block.
func Text(s string) Block { return Block{Type: BlockText, Text: s} }

// ToolUse makes a tool_use block. A nil or empty input becomes "{}".
func ToolUse(id, name string, input json.RawMessage) Block {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	return Block{Type: BlockToolUse, ID: id, Name: name, Input: input}
}

// ToolResultBlock makes the tool_result block that answers the tool_use
// block named id.
func ToolResultBlock(id string, r ToolResult) Block {
	return Block{Type: BlockToolResult, ToolUseID: id, Content: r.Content, IsError: r.IsError}
}

// UserText makes a user message holding one text block.
func UserText(s string) Message {
	return Message{Role: RoleUser, Blocks: []Block{Text(s)}}
}

// AssistantText makes an assistant message holding one text block.
func AssistantText(s string) Message {
	return Message{Role: RoleAssistant, Blocks: []Block{Text(s)}}
}

// Text returns the message's text blocks joined together. Reasoning, tool
// calls, tool results and images are left out.
func (m Message) Text() string {
	var b strings.Builder
	for _, blk := range m.Blocks {
		if blk.Type == BlockText {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

// ToolUses returns the message's tool_use blocks, in order.
func (m Message) ToolUses() []Block {
	var out []Block
	for _, blk := range m.Blocks {
		if blk.Type == BlockToolUse {
			out = append(out, blk)
		}
	}
	return out
}

// TextOf returns the text blocks of a block list joined together. It is
// the plain reading of a tool result's content.
func TextOf(blocks []Block) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type == BlockText {
			b.WriteString(blk.Text)
		}
	}
	return b.String()
}

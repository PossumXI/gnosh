package core

import (
	"bytes"
	"encoding/json"
	"iter"
	"strings"
)

// Accumulator folds a stream's events into the assistant message they
// spell out. Every consumer of a stream uses it, so that a message is put
// together the same way everywhere. The zero value is ready to use.
type Accumulator struct {
	blocks []Block
	args   []*strings.Builder // parallel to blocks; non-nil for tool_use
	byID   map[string]int     // tool_use id -> index into blocks
	sealed bool               // the last thinking block is closed
	usage  Usage
	stop   StopReason
}

// Add takes the next event.
func (a *Accumulator) Add(ev Event) {
	switch ev.Type {
	case EventTextDelta:
		a.appendText(BlockText, ev.Text)
	case EventThinkingDelta:
		a.appendText(BlockThinking, ev.Text)
	case EventThinkingSignature:
		i := len(a.blocks) - 1
		if i < 0 || a.blocks[i].Type != BlockThinking || a.sealed {
			a.push(Block{Type: BlockThinking})
			i++
		}
		a.blocks[i].Signature = ev.Signature
		a.blocks[i].Opaque = ev.Opaque
		a.sealed = true
	case EventToolUseStart:
		a.push(Block{Type: BlockToolUse, ID: ev.ID, Name: ev.Name})
		a.args[len(a.args)-1] = &strings.Builder{}
		if a.byID == nil {
			a.byID = make(map[string]int)
		}
		a.byID[ev.ID] = len(a.blocks) - 1
	case EventToolUseDelta:
		if i, ok := a.byID[ev.ID]; ok {
			a.args[i].WriteString(ev.Text)
		}
	case EventUsage:
		a.usage = ev.Usage
	case EventStop:
		a.stop = ev.StopReason
	}
}

func (a *Accumulator) push(b Block) {
	a.blocks = append(a.blocks, b)
	a.args = append(a.args, nil)
	a.sealed = false
}

func (a *Accumulator) appendText(t BlockType, s string) {
	if s == "" {
		return
	}
	i := len(a.blocks) - 1
	if i >= 0 && a.blocks[i].Type == t && !(t == BlockThinking && a.sealed) {
		a.blocks[i].Text += s
		return
	}
	a.push(Block{Type: t, Text: s})
}

// Message returns the assistant message put together so far. Each tool
// call's Input is its arguments if they are a JSON object, and "{}" if they
// are empty or cannot be read; BadInputs lists the ones that could not.
func (a *Accumulator) Message() Message {
	out := make([]Block, len(a.blocks))
	copy(out, a.blocks)
	for i := range out {
		if out[i].Type != BlockToolUse {
			continue
		}
		raw, ok := objectJSON(a.args[i].String())
		if !ok {
			raw = json.RawMessage("{}")
		}
		out[i].Input = raw
	}
	return Message{Role: RoleAssistant, Blocks: out}
}

// BadInputs returns, by tool call id, the argument text of every call whose
// arguments are not a JSON object. The caller tells the model about them
// instead of running those calls.
func (a *Accumulator) BadInputs() map[string]string {
	var bad map[string]string
	for i, b := range a.blocks {
		if b.Type != BlockToolUse {
			continue
		}
		s := a.args[i].String()
		if _, ok := objectJSON(s); !ok {
			if bad == nil {
				bad = make(map[string]string)
			}
			bad[b.ID] = s
		}
	}
	return bad
}

// Usage returns the last token counts seen.
func (a *Accumulator) Usage() Usage { return a.usage }

// StopReason returns why the model stopped, or "" if the stream has not
// said.
func (a *Accumulator) StopReason() StopReason { return a.stop }

// objectJSON reports whether s is one JSON object, and returns it compact.
// Empty text counts as the empty object.
func objectJSON(s string) (json.RawMessage, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return json.RawMessage("{}"), true
	}
	if s[0] != '{' || !json.Valid([]byte(s)) {
		return nil, false
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// Collect reads a whole stream and returns the message it spelled out. On
// an error it returns what had arrived before the error along with it.
func Collect(seq iter.Seq2[Event, error]) (Message, Usage, StopReason, error) {
	var a Accumulator
	for ev, err := range seq {
		if err != nil {
			return a.Message(), a.Usage(), a.StopReason(), err
		}
		a.Add(ev)
	}
	return a.Message(), a.Usage(), a.StopReason(), nil
}

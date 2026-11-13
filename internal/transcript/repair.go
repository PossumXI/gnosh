package transcript

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// InterruptedResult is what the view tells the model in place of a tool
// result that is not there: the process died in the middle of the call, or
// the result was cut away.
const InterruptedResult = "This tool call was interrupted before it finished, so its result is not available."

// OpeningNotice is the user message the view puts first when what it has
// would otherwise begin with a reply of the model.
const OpeningNotice = "(The start of this conversation is not shown.)"

// RepairKind sorts the changes that make a view a valid conversation.
type RepairKind string

const (
	// RepairFilledResult: a tool call had no result, so an error result
	// saying it was interrupted was added.
	RepairFilledResult RepairKind = "filled_result"
	// RepairDroppedResult: a tool result answered no call of the reply
	// before it, answered one a second time, or sat in a reply.
	RepairDroppedResult RepairKind = "dropped_result"
	// RepairDroppedToolUse: a tool call had no id, repeated an id of the
	// same reply, or sat in a user message.
	RepairDroppedToolUse RepairKind = "dropped_tool_use"
	// RepairDroppedBlock: a block held nothing.
	RepairDroppedBlock RepairKind = "dropped_block"
	// RepairAddedOpening: the view began with a reply, so OpeningNotice was
	// put in front of it.
	RepairAddedOpening RepairKind = "added_opening"
)

// Repair is one change that was made to keep the view valid.
type Repair struct {
	Kind RepairKind
	// ToolUseID is the tool call concerned, when there is one.
	ToolUseID string
	// Message is the index, in the view, of the message that was changed
	// or that the dropped piece would have been part of.
	Message int
}

// empty reports whether a block holds nothing a provider would accept.
func empty(b *core.Block) bool {
	switch b.Type {
	case core.BlockText:
		return strings.TrimSpace(b.Text) == ""
	case core.BlockThinking:
		return b.Text == "" && b.Signature == "" && b.Opaque == ""
	case core.BlockImage:
		return len(b.Data) == 0
	}
	return false
}

// clean returns blocks without the ones that hold nothing, including those
// inside a tool result. It copies only when it has to drop something.
func clean(blocks []core.Block, dropped func()) []core.Block {
	bad := func(b *core.Block) bool {
		return empty(b) || b.Type == core.BlockToolResult && slices.ContainsFunc(b.Content, func(c core.Block) bool { return empty(&c) })
	}
	first := slices.IndexFunc(blocks, func(b core.Block) bool { return bad(&b) })
	if first < 0 {
		return blocks
	}
	out := slices.Clone(blocks[:first])
	for i := first; i < len(blocks); i++ {
		b := blocks[i]
		if empty(&b) {
			dropped()
			continue
		}
		if b.Type == core.BlockToolResult {
			b.Content = clean(b.Content, dropped)
		}
		out = append(out, b)
	}
	return out
}

// repair turns the pieces of a view into a conversation any provider
// accepts, and returns with each message the entries it came from:
//
//   - blocks that hold nothing are dropped, and so are messages left empty;
//   - it starts with a user message (OpeningNotice is added if it must be);
//   - neighbours of the same role are joined, in order;
//   - every tool call has exactly one result in the user message after it:
//     a missing one is filled with InterruptedResult, a result that answers
//     no call of the reply before it is dropped, and so is a second answer
//     to the same call;
//   - in a user message the tool results come first.
func repair(in []piece, info *ViewInfo) ([]core.Message, [][]int) {
	out := make([]core.Message, 0, len(in))
	src := make([][]int, 0, len(in))
	// own says, for each message of out, whether its block list was made
	// here, so that more can be appended to it in place. A list that came
	// from an entry is never written to.
	own := make([]bool, 0, len(in))
	// want is the tool calls of the last reply in out, and got says which
	// of them have their result.
	var want []string
	var got []bool

	note := func(kind RepairKind, id string) {
		info.Repairs = append(info.Repairs, Repair{Kind: kind, ToolUseID: id, Message: max(len(out)-1, 0)})
	}
	add := func(role core.Role, blocks []core.Block, from []int) {
		if n := len(out); n > 0 && out[n-1].Role == role {
			if !own[n-1] {
				grown := make([]core.Block, 0, 2*(len(out[n-1].Blocks)+len(blocks)))
				out[n-1].Blocks = append(grown, out[n-1].Blocks...)
				own[n-1] = true
			}
			out[n-1].Blocks = append(out[n-1].Blocks, blocks...)
			// A piece's source list has no room to spare, so appending to
			// it makes a new list and leaves the piece's alone.
			src[n-1] = append(src[n-1], from...)
			info.Merged++
			return
		}
		out = append(out, core.Message{Role: role, Blocks: blocks})
		src = append(src, from)
		own = append(own, false)
	}
	// settle gives every call of the last reply that has no result one
	// that says it was interrupted.
	settle := func() {
		var fill []core.Block
		for i, id := range want {
			if !got[i] {
				fill = append(fill, core.ToolResultBlock(id, core.ErrorResult(InterruptedResult)))
			}
		}
		if len(fill) > 0 {
			if n := len(out); out[n-1].Role == core.RoleUser {
				out[n-1].Blocks = slices.Concat(out[n-1].Blocks, fill)
				own[n-1] = true
			} else {
				out = append(out, core.Message{Role: core.RoleUser, Blocks: fill})
				src = append(src, nil)
				own = append(own, true)
			}
			for i, id := range want {
				if !got[i] {
					note(RepairFilledResult, id)
				}
			}
		}
		want, got = nil, nil
	}

	for _, p := range in {
		blocks := clean(p.blocks, func() { note(RepairDroppedBlock, "") })
		if p.role == core.RoleUser {
			blocks = filter(blocks, func(b *core.Block) bool {
				switch b.Type {
				case core.BlockToolUse:
					note(RepairDroppedToolUse, b.ID)
					return false
				case core.BlockToolResult:
					i := slices.Index(want, b.ToolUseID)
					if i < 0 || got[i] {
						note(RepairDroppedResult, b.ToolUseID)
						return false
					}
					got[i] = true
				}
				return true
			})
			if len(blocks) > 0 {
				add(core.RoleUser, blocks, p.src)
			}
			continue
		}
		var ids []string
		blocks = filter(blocks, func(b *core.Block) bool {
			switch b.Type {
			case core.BlockToolResult:
				note(RepairDroppedResult, b.ToolUseID)
				return false
			case core.BlockToolUse:
				if b.ID == "" || slices.Contains(ids, b.ID) {
					note(RepairDroppedToolUse, b.ID)
					return false
				}
				ids = append(ids, b.ID)
			}
			return true
		})
		if len(blocks) == 0 {
			continue
		}
		if len(out) == 0 {
			out = append(out, core.UserText(OpeningNotice))
			src = append(src, nil)
			own = append(own, true)
			note(RepairAddedOpening, "")
		}
		settle()
		add(core.RoleAssistant, blocks, p.src)
		want, got = ids, make([]bool, len(ids))
	}
	if len(out) > 0 {
		settle()
	}
	for i := range out {
		if out[i].Role == core.RoleUser {
			out[i].Blocks = resultsFirst(out[i].Blocks)
		} else {
			out[i].Blocks = slices.Clone(out[i].Blocks)
		}
	}
	return out, src
}

// filter returns the blocks that keep reports true for, in order. It
// copies only when one is left out.
func filter(blocks []core.Block, keep func(*core.Block) bool) []core.Block {
	for i := range blocks {
		if keep(&blocks[i]) {
			continue
		}
		out := slices.Clone(blocks[:i])
		for j := i + 1; j < len(blocks); j++ {
			if keep(&blocks[j]) {
				out = append(out, blocks[j])
			}
		}
		return out
	}
	return blocks
}

// resultsFirst returns a copy of a user message's blocks with the tool
// results moved in front of the others, each group in its own order.
func resultsFirst(blocks []core.Block) []core.Block {
	out := make([]core.Block, 0, len(blocks))
	for i := range blocks {
		if blocks[i].Type == core.BlockToolResult {
			out = append(out, blocks[i])
		}
	}
	for i := range blocks {
		if blocks[i].Type != core.BlockToolResult {
			out = append(out, blocks[i])
		}
	}
	return out
}

// Check verifies that messages is a conversation any provider accepts, the
// invariants of core.Request.Messages and a little more:
//
//   - it starts with a user message, and the roles alternate;
//   - no message is empty and no block holds nothing;
//   - tool calls are in assistant messages, each with an id of its own;
//   - the user message after a reply holds exactly one result for each of
//     the reply's tool calls and no other result, and the results come
//     before its other blocks;
//   - it does not end with a reply whose tool calls are unanswered.
//
// It returns nil for a valid conversation (an empty one is valid) and
// otherwise an error that names the first thing wrong.
func Check(messages []core.Message) error {
	var want []string
	for i, m := range messages {
		switch {
		case m.Role != core.RoleUser && m.Role != core.RoleAssistant:
			return fmt.Errorf("message %d has the role %q, which is neither user nor assistant", i, m.Role)
		case i == 0 && m.Role != core.RoleUser:
			return fmt.Errorf("the conversation starts with an assistant message")
		case i > 0 && m.Role == messages[i-1].Role:
			return fmt.Errorf("messages %d and %d are both %s messages", i-1, i, m.Role)
		case len(m.Blocks) == 0:
			return fmt.Errorf("message %d is empty", i)
		}
		for j := range m.Blocks {
			b := &m.Blocks[j]
			if empty(b) {
				return fmt.Errorf("message %d, block %d: a %s block that holds nothing", i, j, b.Type)
			}
			if b.Type == core.BlockToolResult {
				for k := range b.Content {
					if empty(&b.Content[k]) {
						return fmt.Errorf("message %d, block %d: the tool result holds a %s block that holds nothing", i, j, b.Content[k].Type)
					}
				}
			}
		}
		if m.Role == core.RoleAssistant {
			want = want[:0]
			for j := range m.Blocks {
				b := &m.Blocks[j]
				switch b.Type {
				case core.BlockToolResult:
					return fmt.Errorf("message %d, block %d: a tool result in an assistant message", i, j)
				case core.BlockToolUse:
					if b.ID == "" {
						return fmt.Errorf("message %d, block %d: a tool call without an id", i, j)
					}
					if slices.Contains(want, b.ID) {
						return fmt.Errorf("message %d: two tool calls with the id %q", i, b.ID)
					}
					want = append(want, b.ID)
				}
			}
			continue
		}
		got := make([]bool, len(want))
		others := false
		for j := range m.Blocks {
			b := &m.Blocks[j]
			switch b.Type {
			case core.BlockToolUse:
				return fmt.Errorf("message %d, block %d: a tool call in a user message", i, j)
			case core.BlockToolResult:
				k := slices.Index(want, b.ToolUseID)
				switch {
				case k < 0:
					return fmt.Errorf("message %d, block %d: a result for %q, which the message before did not call", i, j, b.ToolUseID)
				case got[k]:
					return fmt.Errorf("message %d: two results for the tool call %q", i, b.ToolUseID)
				case others:
					return fmt.Errorf("message %d, block %d: a tool result after other blocks", i, j)
				}
				got[k] = true
			default:
				others = true
			}
		}
		for k, id := range want {
			if !got[k] {
				return fmt.Errorf("message %d: the tool call %q of the message before has no result", i, id)
			}
		}
		want = want[:0]
	}
	if len(want) > 0 {
		return fmt.Errorf("the conversation ends with a reply whose tool call %q has no result", want[0])
	}
	return nil
}

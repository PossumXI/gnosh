package transcript

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// kinds lists the kinds of the repairs made, in order.
func kinds(info ViewInfo) []RepairKind {
	var out []RepairKind
	for _, r := range info.Repairs {
		out = append(out, r.Kind)
	}
	return out
}

func TestRepair(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    string
		repairs []RepairKind
		merged  int
	}{
		{
			name: "a reply left with unanswered tool calls is completed",
			src: `
				prompt go
				call t1 t2
			`,
			want: `
				U: go
				A: call(t1) + call(t2)
				U: interrupted(t1) + interrupted(t2)
			`,
			repairs: []RepairKind{RepairFilledResult, RepairFilledResult},
		},
		{
			name: "a missing result is filled in the user message that follows",
			src: `
				prompt go
				call t1
				prompt never mind, do this instead
				say ok
			`,
			want: `
				U: go
				A: call(t1)
				U: interrupted(t1) + never mind, do this instead
				A: ok
			`,
			repairs: []RepairKind{RepairFilledResult},
		},
		{
			name: "only the missing one of two results is filled",
			src: `
				prompt go
				call t1 t2
				results t2
				say ok
			`,
			want: `
				U: go
				A: call(t1) + call(t2)
				U: result(t2) + interrupted(t1)
				A: ok
			`,
			repairs: []RepairKind{RepairFilledResult},
		},
		{
			name: "a result that answers no call is dropped",
			src: `
				prompt go
				results t9
				say ok
			`,
			want: `
				U: go
				A: ok
			`,
			repairs: []RepairKind{RepairDroppedResult},
		},
		{
			name: "a second result for the same call is dropped",
			src: `
				prompt go
				call t1
				results t1
				results t1
				say ok
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: ok
			`,
			repairs: []RepairKind{RepairDroppedResult},
		},
		{
			name: "a result that comes after another reply answers nothing",
			src: `
				prompt go
				call t1
				say never mind
				results t1
			`,
			want: `
				U: go
				A: call(t1)
				U: interrupted(t1)
				A: never mind
			`,
			repairs: []RepairKind{RepairFilledResult, RepairDroppedResult},
		},
		{
			name: "neighbours of the same role are joined, in order",
			src: `
				prompt one
				prompt two
				hook three
				say four
				say five
			`,
			want: `
				U: one + two + three
				A: four + five
			`,
			merged: 3,
		},
		{
			name: "a view that would begin with a reply gets an opening",
			src: `
				say hello
				prompt hi
			`,
			want: `
				U: (The start of this conversation is not shown.)
				A: hello
				U: hi
			`,
			repairs: []RepairKind{RepairAddedOpening},
		},
		{
			name: "in a user message the results come first",
			src: `
				prompt go
				call t1
				hook some context
				results t1
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1) + some context
			`,
			merged: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := expectView(t, tt.src, ViewOptions{}, tt.want)
			if got := kinds(info); !reflect.DeepEqual(got, tt.repairs) {
				t.Errorf("repairs = %v, want %v", got, tt.repairs)
			}
			if info.Merged != tt.merged {
				t.Errorf("Merged = %d, want %d", info.Merged, tt.merged)
			}
		})
	}
}

func TestRepairBlocks(t *testing.T) {
	blocks := func(b ...core.Block) core.Message { return core.Message{Blocks: b} }
	t.Run("blocks that hold nothing are dropped, and messages left empty with them", func(t *testing.T) {
		entries := []Entry{
			PromptEntry("go"),
			AssistantEntry(blocks(core.Text(""), core.Text(" \n")), core.StopEndTurn),
			UserEntry(blocks(core.Text(""), core.Text("still here")), SourcePrompt),
			AssistantEntry(blocks(core.Block{Type: core.BlockThinking}, core.Text("answer"), core.ToolUse("t1", "Read", nil)), core.StopToolUse),
			UserEntry(blocks(
				core.ToolResultBlock("t1", core.ToolResult{Content: []core.Block{core.Text(""), core.Text("data")}}),
				core.Block{Type: core.BlockImage, MediaType: "image/png"},
			), SourceToolResults),
		}
		msgs, info := mustView(t, entries, ViewOptions{})
		want := `
			U: go + still here
			A: answer + call(t1)
			U: result(t1: data)
		`
		if got := render(msgs); got != tidy(want) {
			t.Errorf("view:\n%s\n--- want ---\n%s", got, tidy(want))
		}
		if got := len(info.Repairs); got != 6 {
			t.Errorf("%d repairs, want 6 dropped blocks: %+v", got, info.Repairs)
		}
		for _, r := range info.Repairs {
			if r.Kind != RepairDroppedBlock {
				t.Errorf("repair %+v, want a dropped block", r)
			}
		}
		// The entries still hold what they held.
		if len(entries[1].Message.Blocks) != 2 || len(entries[4].Message.Blocks[0].Content) != 2 {
			t.Errorf("the entries were changed")
		}
	})
	t.Run("a thinking block with only a signature is kept", func(t *testing.T) {
		entries := []Entry{
			PromptEntry("go"),
			AssistantEntry(blocks(core.Block{Type: core.BlockThinking, Signature: "sig"}, core.Text("answer")), core.StopEndTurn),
		}
		msgs, info := mustView(t, entries, ViewOptions{})
		if len(msgs) != 2 || len(msgs[1].Blocks) != 2 || len(info.Repairs) != 0 {
			t.Errorf("view = %+v, repairs = %+v", msgs, info.Repairs)
		}
	})
	t.Run("tool calls in a user message and results in a reply are dropped", func(t *testing.T) {
		entries := []Entry{
			UserEntry(blocks(core.Text("go"), core.ToolUse("x1", "Bash", nil)), SourcePrompt),
			AssistantEntry(blocks(core.ToolResultBlock("x2", core.TextResult("nope")), core.Text("ok")), core.StopEndTurn),
		}
		msgs, info := mustView(t, entries, ViewOptions{})
		if got, want := render(msgs), "U: go\nA: ok"; got != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
		}
		if got, want := kinds(info), []RepairKind{RepairDroppedToolUse, RepairDroppedResult}; !reflect.DeepEqual(got, want) {
			t.Errorf("repairs = %v, want %v", got, want)
		}
	})
	t.Run("a tool call with no id, or with an id the reply used already, is dropped", func(t *testing.T) {
		entries := []Entry{
			PromptEntry("go"),
			AssistantEntry(blocks(core.ToolUse("t1", "Read", nil), core.ToolUse("t1", "Read", nil), core.ToolUse("", "Read", nil)), core.StopToolUse),
			UserEntry(blocks(core.ToolResultBlock("t1", core.TextResult("out t1"))), SourceToolResults),
		}
		msgs, info := mustView(t, entries, ViewOptions{})
		if got, want := render(msgs), "U: go\nA: call(t1)\nU: result(t1)"; got != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
		}
		if got, want := kinds(info), []RepairKind{RepairDroppedToolUse, RepairDroppedToolUse}; !reflect.DeepEqual(got, want) {
			t.Errorf("repairs = %v, want %v", got, want)
		}
	})
	t.Run("the view's messages do not share their block lists with the entries", func(t *testing.T) {
		entries := script(t, `
			prompt go
			call t1
			results t1
		`)
		msgs, _ := mustView(t, entries, ViewOptions{})
		for i := range msgs {
			msgs[i].Blocks[0] = core.Text("scribbled on")
		}
		if entries[0].Message.Text() != "go" || entries[1].Message.Blocks[0].Type != core.BlockToolUse || entries[2].Message.Blocks[0].Type != core.BlockToolResult {
			t.Errorf("writing to the view's messages changed the entries")
		}
	})
}

func TestCheck(t *testing.T) {
	user := func(b ...core.Block) core.Message { return core.Message{Role: core.RoleUser, Blocks: b} }
	asst := func(b ...core.Block) core.Message { return core.Message{Role: core.RoleAssistant, Blocks: b} }
	use := func(id string) core.Block { return core.ToolUse(id, "Read", nil) }
	res := func(id string) core.Block { return core.ToolResultBlock(id, core.TextResult("out")) }
	hi := core.Text("hi")

	good := [][]core.Message{
		nil,
		{user(hi)},
		{user(hi), asst(hi)},
		{user(hi), asst(hi, use("t1"), use("t2")), user(res("t2"), res("t1"), hi), asst(hi)},
		{user(hi), asst(core.Block{Type: core.BlockThinking, Opaque: "x"}, use("t1")), user(res("t1"))},
		{user(hi), asst(use("t1")), user(core.ToolResultBlock("t1", core.ToolResult{})), asst(use("t1")), user(res("t1"))},
	}
	for i, msgs := range good {
		if err := Check(msgs); err != nil {
			t.Errorf("good conversation %d: %v", i, err)
		}
	}

	bad := []struct {
		name string
		msgs []core.Message
		want string
	}{
		{"starts with a reply", []core.Message{asst(hi)}, "starts with an assistant message"},
		{"two user messages in a row", []core.Message{user(hi), user(hi)}, "both user messages"},
		{"two replies in a row", []core.Message{user(hi), asst(hi), asst(hi)}, "both assistant messages"},
		{"an unknown role", []core.Message{{Role: "system", Blocks: []core.Block{hi}}}, "neither user nor assistant"},
		{"an empty message", []core.Message{user()}, "is empty"},
		{"an empty text block", []core.Message{user(core.Text(" "))}, "holds nothing"},
		{"an empty block inside a result", []core.Message{user(hi), asst(use("t1")), user(core.ToolResultBlock("t1", core.TextResult("")))}, "holds a text block that holds nothing"},
		{"an unanswered call at the end", []core.Message{user(hi), asst(use("t1"))}, "ends with a reply"},
		{"an unanswered call in the middle", []core.Message{user(hi), asst(use("t1"), use("t2")), user(res("t1")), asst(hi)}, `"t2" of the message before has no result`},
		{"a result nobody asked for", []core.Message{user(hi), asst(hi), user(res("t1"))}, "did not call"},
		{"a result one message late", []core.Message{user(hi), asst(use("t1")), user(res("t1")), asst(hi), user(res("t1"))}, "did not call"},
		{"two results for one call", []core.Message{user(hi), asst(use("t1")), user(res("t1"), res("t1"))}, "two results"},
		{"a result after other blocks", []core.Message{user(hi), asst(use("t1")), user(hi, res("t1"))}, "after other blocks"},
		{"a tool call in a user message", []core.Message{user(use("t1"))}, "tool call in a user message"},
		{"a result in a reply", []core.Message{user(hi), asst(res("t1"))}, "tool result in an assistant message"},
		{"a tool call without an id", []core.Message{user(hi), asst(use(""))}, "without an id"},
		{"two calls with one id", []core.Message{user(hi), asst(use("t1"), use("t1"))}, "two tool calls"},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			err := Check(tt.msgs)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Check = %v, want an error holding %q", err, tt.want)
			}
		})
	}
}

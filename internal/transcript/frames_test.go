package transcript

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// The frame rule, as examples. Each case is a transcript in the words of
// script (view_test.go) and the view it must give. In a view, result(id)*
// is a tool result that got a note saying what was cut after it.
func TestFrames(t *testing.T) {
	const popNote = ", removed from your context when you popped it.]"
	tests := []struct {
		name  string
		src   string
		want  string
		check func(t *testing.T, info ViewInfo)
	}{
		{
			name: "a popped frame leaves its push round and its pop round",
			src: `
				prompt fix the bug
				call t1:mem
				results t1
				push A t1
				call t2:Read
				results t2
				call t3:Edit
				results t3
				call t4:mem
				results t4
				pop A t4 the bug is fixed
				say all done
			`,
			want: `
				U: fix the bug
				A: call(t1)
				U: result(t1)*
				A: call(t4)
				U: result(t4)
				A: all done
			`,
			check: func(t *testing.T, info ViewInfo) {
				want := Cut{
					Kind: OpPop, Frame: "A", Label: "label A", Result: "the bug is fixed",
					UUID: "e11", PushUUID: "e4", Messages: 4,
					Note: "[Then came the work in it, 2 tool calls (Edit 1, Read 1)" + popNote,
				}
				if len(info.Cuts) != 1 {
					t.Fatalf("Cuts = %+v", info.Cuts)
				}
				got := info.Cuts[0]
				if got.Tokens <= 0 {
					t.Errorf("the cut freed %d tokens", got.Tokens)
				}
				got.Tokens = 0
				if !reflect.DeepEqual(got, want) {
					t.Errorf("cut = %+v\nwant  %+v", got, want)
				}
				if len(info.Stack) != 0 {
					t.Errorf("Stack = %+v, want none", info.Stack)
				}
			},
		},
		{
			name: "a frame still open cuts nothing",
			src: `
				prompt fix the bug
				call t1:mem
				results t1
				push A t1
				call t2:Read
				results t2
			`,
			want: `
				U: fix the bug
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)
			`,
			check: func(t *testing.T, info ViewInfo) {
				want := []Frame{{ID: "A", Label: "label A", UUID: "e4", ToolUseID: "t1"}}
				if !reflect.DeepEqual(info.Stack, want) || len(info.Cuts) != 0 {
					t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
				}
			},
		},
		{
			name: "an inner pop cuts its own range at once",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2
				results t2
				push B t2
				call t3:Read
				results t3
				call t4
				results t4
				pop B t4 b is done
				call t5:Edit
				results t5
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)*
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Stack) != 1 || info.Stack[0].ID != "A" {
					t.Errorf("Stack = %+v, want A", info.Stack)
				}
				if len(info.Cuts) != 1 || info.Cuts[0].Messages != 2 ||
					info.Cuts[0].Note != "[Then came the work in it, 1 tool call (Read 1)"+popNote {
					t.Errorf("Cuts = %+v", info.Cuts)
				}
			},
		},
		{
			name: "an outer pop takes what is left, the inner push and pop rounds included",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2
				results t2
				push B t2
				call t3:Read
				results t3
				call t4
				results t4
				pop B t4 b is done
				call t5:Edit
				results t5
				call t6
				results t6
				pop A t6 a is done
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t6)
				U: result(t6)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Cuts) != 2 {
					t.Fatalf("Cuts = %+v", info.Cuts)
				}
				// B's pop had taken two messages already; A's pop takes the
				// other six. A's note tells of all the work between.
				if b, a := info.Cuts[0], info.Cuts[1]; b.Frame != "B" || b.Messages != 2 || a.Frame != "A" || a.Messages != 6 ||
					a.Note != "[Then came the work in it, 4 tool calls (Bash 2, Edit 1, Read 1)"+popNote {
					t.Errorf("Cuts = %+v", info.Cuts)
				}
			},
		},
		{
			name: "calls made beside the push or the pop stay with their results",
			src: `
				prompt go
				call t1:mem t2:Read      # the push, and a read beside it
				results t1 t2
				push A t1
				call t3:Edit
				results t3
				call t4:mem t5:Edit      # the pop, and an edit beside it
				results t4 t5
				pop A t4 done
			`,
			want: `
				U: go
				A: call(t1) + call(t2)
				U: result(t1)* + result(t2)
				A: call(t4) + call(t5)
				U: result(t4) + result(t5)
			`,
		},
		{
			name: "a pop that names a frame not on the stack changes nothing",
			src: `
				prompt go
				call t1
				results t1
				@p pop Z t1 nothing to pop
				say ok
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: ok
			`,
			check: func(t *testing.T, info ViewInfo) {
				if !reflect.DeepEqual(info.UnmatchedPops, []string{"p"}) || len(info.Cuts) != 0 {
					t.Errorf("UnmatchedPops = %v, Cuts = %+v", info.UnmatchedPops, info.Cuts)
				}
			},
		},
		{
			name: "a pop whose push was cut off by a rewind changes nothing",
			src: `
				@p prompt go
				call t1
				results t1
				push A t1
				call t2
				results t2
				rewind @p
				call t3
				results t3
				call t4
				results t4
				@pop pop A t4 done
			`,
			want: `
				U: go
				A: call(t3)
				U: result(t3)
				A: call(t4)
				U: result(t4)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if !reflect.DeepEqual(info.UnmatchedPops, []string{"pop"}) || len(info.Stack) != 0 {
					t.Errorf("UnmatchedPops = %v, Stack = %+v", info.UnmatchedPops, info.Stack)
				}
			},
		},
		{
			name: "a rewind into a popped frame opens it again",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2:Read
				@y results t2
				call t3
				results t3
				pop A t3 done
				rewind @y
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Stack) != 1 || info.Stack[0].ID != "A" || len(info.Cuts) != 0 {
					t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
				}
			},
		},
		{
			name: "a pop whose reply is not in the conversation cuts nothing, and leaves the frame",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				call t3
				results t3
				pop A - done
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)
				A: call(t3)
				U: result(t3)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Stack) != 0 || len(info.Cuts) != 1 || info.Cuts[0].Messages != 0 || info.Cuts[0].Reason == "" || info.Cuts[0].Note != "" {
					t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
				}
			},
		},
		{
			name: "a frame entered and left in one reply cuts nothing",
			src: `
				prompt go
				call t1 t2
				results t1 t2
				push A t1
				pop A t2 trivial
				say ok
			`,
			want: `
				U: go
				A: call(t1) + call(t2)
				U: result(t1) + result(t2)
				A: ok
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Cuts) != 1 || !strings.Contains(info.Cuts[0].Reason, "one reply") {
					t.Errorf("Cuts = %+v", info.Cuts)
				}
			},
		},
		{
			name: "a pop straight after the push has nothing to cut and leaves no note",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2
				results t2
				pop A t2 nothing to do
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Cuts) != 1 || !strings.Contains(info.Cuts[0].Reason, "nothing came between") || info.Cuts[0].Note != "" {
					t.Errorf("Cuts = %+v", info.Cuts)
				}
			},
		},
		{
			name: "a pop spares the prompts of its own turn, and cuts what was said in earlier ones",
			src: `
				prompt first task
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				say which port?
				prompt 8080              # an earlier turn inside the frame: cut
				call t3
				results t3
				say that part is done
				prompt now finish it     # the turn the pop is made in: spared
				hook a reminder          # and what the harness added to it
				call t4
				results t4
				pop A t4 finished
			`,
			want: `
				U: first task
				A: call(t1)
				U: result(t1)* + now finish it + a reminder
				A: call(t4)
				U: result(t4)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if c := info.Cuts[0]; c.Spared != 2 || c.Messages != 7 ||
					c.Note != "[Then came the work in it, 2 tool calls (Bash 1, Read 1)"+popNote {
					t.Errorf("cut = %+v", c)
				}
			},
		},
		{
			name: "a prompt folded into a running turn belongs to that turn",
			src: `
				prompt task
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				prompt also do this      # arrived while the turn ran
				call t3
				results t3
				pop A t3 ok
			`,
			want: `
				U: task
				A: call(t1)
				U: result(t1)* + also do this
				A: call(t3)
				U: result(t3)
			`,
		},
		{
			name: "a turn goes back past a prompt that followed tool results",
			src: `
				prompt task
				call t1
				results t1
				push A t1
				say what next?
				prompt do part one       # starts the turn: it follows an answer
				call t2
				results t2
				prompt do part two       # folded into it
				call t3
				results t3
				pop A t3 ok
			`,
			want: `
				U: task
				A: call(t1)
				U: result(t1)* + do part one + do part two
				A: call(t3)
				U: result(t3)
			`,
		},
		{
			name: "a cancelled turn ends where it stopped, so its prompt is not spared",
			src: `
				prompt task
				call t1
				results t1
				push A t1
				say what next?
				prompt do part one
				call! t2                 # this reply was cancelled
				results t2
				prompt do part two       # so this starts a new turn
				call t3
				results t3
				pop A t3 ok
			`,
			want: `
				U: task
				A: call(t1)
				U: result(t1)* + do part two
				A: call(t3)
				U: result(t3)
			`,
		},
		{
			name: "a pop whose push is behind the summary cuts everything after the summary's continuation",
			src: `
				prompt task
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				summary the story so far
				notice carry on from the summary
				call t3:Edit
				results t3
				call t4
				results t4
				pop A t4 done
			`,
			want: `
				U: summary(the story so far) + carry on from the summary
				A: call(t4)
				U: result(t4)
			`,
			check: func(t *testing.T, info ViewInfo) {
				// Only what the model still had is counted.
				if len(info.Stack) != 0 || len(info.Cuts) != 1 || info.Cuts[0].Messages != 2 {
					t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
				}
			},
		},
		{
			name: "the stack survives a summary",
			src: `
				prompt task
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				summary the story so far
				notice carry on from the summary
				call t3:Edit
				results t3
			`,
			want: `
				U: summary(the story so far) + carry on from the summary
				A: call(t3)
				U: result(t3)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Stack) != 1 || info.Stack[0].ID != "A" {
					t.Errorf("Stack = %+v, want A", info.Stack)
				}
			},
		},
		{
			name: "a frame entered after a summary is cut as any other",
			src: `
				prompt task
				say first answer
				summary earlier work
				prompt next task
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				call t3
				results t3
				pop A t3 done
			`,
			want: `
				U: summary(earlier work) + next task
				A: call(t1)
				U: result(t1)*
				A: call(t3)
				U: result(t3)
			`,
		},
		{
			name: "a branch keeps the cuts it came with and starts at the root",
			src: `
				session s0
				prompt task
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				call t3
				results t3
				pop A t3 done
				call t4
				results t4
				push B t4                # open when the branch was made
				call t5:Read
				results t5
				session s1               # the branch
				other
				call t6
				results t6
				@pop pop B t6 done in the branch
			`,
			want: `
				U: task
				A: call(t1)
				U: result(t1)*
				A: call(t3)
				U: result(t3)
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
				A: call(t6)
				U: result(t6)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Stack) != 0 || !reflect.DeepEqual(info.UnmatchedPops, []string{"pop"}) {
					t.Errorf("Stack = %+v, UnmatchedPops = %v", info.Stack, info.UnmatchedPops)
				}
			},
		},
		{
			name: "a frame pushed again under the same id is a new frame",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				call t3
				results t3
				pop A t3 first pass done
				call t4
				results t4
				push A t4
				call t5:Edit
				results t5
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t3)
				U: result(t3)
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
			`,
			check: func(t *testing.T, info ViewInfo) {
				want := []Frame{{ID: "A", Label: "label A", UUID: "e12", ToolUseID: "t4"}}
				if !reflect.DeepEqual(info.Stack, want) {
					t.Errorf("Stack = %+v, want %+v", info.Stack, want)
				}
			},
		},
		{
			name: "a push whose reply is not known is cut from the push entry on, with no note",
			src: `
				prompt task
				push A -
				call t2:Read
				results t2
				call t3
				results t3
				pop A t3 done
			`,
			want: `
				U: task
				A: call(t3)
				U: result(t3)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if c := info.Cuts[0]; c.Messages != 2 || c.Note != "" {
					t.Errorf("cut = %+v", c)
				}
			},
		},
		{
			name: "popping a frame takes the frames above it off the stack too",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2
				results t2
				push B t2                # never popped
				call t3:Read
				results t3
				call t4
				results t4
				pop A t4 all of it
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t4)
				U: result(t4)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if len(info.Stack) != 0 || !reflect.DeepEqual(info.Cuts[0].Inner, []string{"B"}) {
					t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
				}
			},
		},
		{
			name: "the trampoline: pop one todo and enter the next in one reply",
			src: `
				prompt go
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				call t3                  # pops A and pushes B
				results t3
				pop A t3 a is done
				push B t3
				call t4:Edit
				results t4
				call t5
				results t5
				pop B t5 b is done
			`,
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t3)
				U: result(t3)*
				A: call(t5)
				U: result(t5)
			`,
		},
		{
			name: "tool results and a prompt in one user message are two rows",
			src: `
				prompt task
				call t1
				results t1
				push A t1
				call t2:Read
				mixed t2 | and check the port
				call t3
				results t3
				pop A t3 done
			`,
			want: `
				U: task
				A: call(t1)
				U: result(t1)* + and check the port
				A: call(t3)
				U: result(t3)
			`,
		},
		{
			name: "with no tool call in what was cut, the note counts messages",
			src: `
				prompt task
				call t1
				results t1
				push A t1
				say here is the explanation
				prompt thanks, close it
				call t2
				results t2
				pop A t2 explained
			`,
			want: `
				U: task
				A: call(t1)
				U: result(t1)* + thanks, close it
				A: call(t2)
				U: result(t2)
			`,
			check: func(t *testing.T, info ViewInfo) {
				if c := info.Cuts[0]; c.Note != "[Then came 1 message of work in it"+popNote || c.Spared != 1 {
					t.Errorf("cut = %+v", c)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := expectView(t, tt.src, ViewOptions{}, tt.want)
			if tt.check != nil {
				tt.check(t, info)
			}
		})
	}
}

func TestFrameNoteIsAddedToThePushAnswerOnly(t *testing.T) {
	entries := script(t, `
		prompt go
		call t1:mem t2:Read
		results t1 t2
		push A t1
		call t3:Edit
		results t3
		call t4:mem
		results t4
		pop A t4 done
	`)
	msgs, info := mustView(t, entries, ViewOptions{Trace: true})
	note := "[Then came the work in it, 1 tool call (Edit 1), removed from your context when you popped it.]"
	if got := core.TextOf(msgs[2].Blocks[0].Content); got != "out t1\n"+note {
		t.Errorf("the push's answer reads %q", got)
	}
	if got := core.TextOf(msgs[2].Blocks[1].Content); got != "out t2" {
		t.Errorf("the answer beside the push's reads %q", got)
	}
	// The stored entry is not edited.
	if got := core.TextOf(entries[2].Message.Blocks[0].Content); got != "out t1" {
		t.Errorf("the stored result now reads %q", got)
	}
	if want := []string{"e5", "e6"}; !reflect.DeepEqual(info.Cuts[0].Removed, want) {
		t.Errorf("Removed = %v, want %v", info.Cuts[0].Removed, want)
	}
	if want := [][]string{{"e1"}, {"e2"}, {"e3"}, {"e7"}, {"e8"}}; !reflect.DeepEqual(info.Sources, want) {
		t.Errorf("Sources = %v, want %v", info.Sources, want)
	}
}

func TestStack(t *testing.T) {
	entries := script(t, `
		session s0
		prompt task
		call t1
		results t1
		push A t1
		call t2
		results t2
		push B t2
		session s1
		call t3
		results t3
		push C t3
	`)
	ids := func(frames []Frame) string {
		var s []string
		for _, f := range frames {
			s = append(s, f.ID)
		}
		return strings.Join(s, " ")
	}
	if got := ids(Stack(entries, "")); got != "C" {
		t.Errorf("the stack of the last entry's session is %q, want C", got)
	}
	if got := ids(Stack(entries, "s0")); got != "A B" {
		t.Errorf("the stack of s0 is %q, want A B", got)
	}
	if got := ids(Stack(nil, "")); got != "" {
		t.Errorf("the stack of nothing is %q", got)
	}
}

package transcript

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// ten counts every message as ten tokens, so that the sums in these tests
// can be done in the head.
func ten(core.Message) int { return 10 }

// record appends planned slides to a transcript the way a session would:
// with a UUID and the session of the entry before.
func record(entries []Entry, slides []Entry) []Entry {
	out := append([]Entry(nil), entries...)
	for i, s := range slides {
		s.UUID = fmt.Sprintf("slide%d", i+1)
		s.SessionID = entries[len(entries)-1].SessionID
		s.V = Version
		out = append(out, s)
	}
	return out
}

// slideRefs writes slide entries as "frame anchor until", for comparing.
func slideRefs(slides []Entry) []string {
	var out []string
	for _, s := range slides {
		frame := s.Frame
		if frame == "" {
			frame = "-"
		}
		if s.Type != TypeFrame || s.Op != OpSlide {
			frame = "not a slide: " + string(s.Type)
		}
		out = append(out, frame+" "+s.Anchor+" "+s.Until)
	}
	return out
}

// The slide, as examples. Every message counts ten tokens.
func TestSlide(t *testing.T) {
	// A session that explored before it entered a frame.
	const explored = `
		prompt explore and fix
		@a1 call t1:Read
		results t1
		@a2 call t2:Read
		results t2
		@a3 call t3:Read
		results t3
		@a4 call t4:mem
		results t4
		push A t4
		@a5 call t5:Edit
		results t5
		@a6 call t6:Edit
		results t6
	`
	// A session two frames deep.
	const nested = `
		prompt go
		@a1 call t1:mem
		results t1
		push A t1
		@a2 call t2:Read
		results t2
		@a3 call t3:Read
		results t3
		@a4 call t4:mem
		results t4
		push B t4
		@a5 call t5:Edit
		results t5
		@a6 call t6:Edit
		results t6
		@a7 call t7:Edit
		results t7
	`
	// A session whose frame was entered in an earlier turn.
	const spoken = `
		prompt go
		@a1 call t1:mem
		results t1
		push A t1
		@a2 call t2:Read
		results t2
		@a3 call t3:Read
		results t3
		@q prompt and now the other thing
		@a4 call t4:Edit
		results t4
		@a5 call t5:Edit
		results t5
	`
	// A frame with four rounds in it.
	const four = `
		prompt go
		@a1 call t1:mem
		results t1
		push A t1
		@a2 call t2
		results t2
		@a3 call t3
		results t3
		@a4 call t4
		results t4
		@a5 call t5
		results t5
	`
	tests := []struct {
		name   string
		src    string
		opts   ViewOptions
		want   string
		slides []string // each "frame anchor until"
		tokens int
		over   bool
	}{
		{
			name: "under the budget nothing slides",
			src:  explored,
			opts: ViewOptions{Budget: 130},
			want: `
				U: explore and fix
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)
				A: call(t3)
				U: result(t3)
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
				A: call(t6)
				U: result(t6)
			`,
			tokens: 130,
		},
		{
			// 130 tokens against 100: 30 over, and a tenth of the budget
			// for room, so 40 must go. The first round stays as the anchor.
			name: "the rounds before the first frame go first",
			src:  explored,
			opts: ViewOptions{Budget: 100},
			want: `
				U: explore and fix
				A: call(t1)
				U: result(t1)*
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
				A: call(t6)
				U: result(t6)
			`,
			slides: []string{"- a1 a4"},
			tokens: 90,
		},
		{
			// 5 over and 12 for room is 17, but a cut takes at least half
			// of the 40 tokens it could: one round.
			name: "a cut takes at least half of what it could",
			src:  explored,
			opts: ViewOptions{Budget: 125},
			want: `
				U: explore and fix
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
			slides: []string{"- a1 a3"},
			tokens: 110,
		},
		{
			// 150 against 130 with 13 for room: 33 to free. The body of A,
			// the lower frame, holds 40.
			name: "then the whole body of the lower frame",
			src:  nested,
			opts: ViewOptions{Budget: 130},
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
				A: call(t6)
				U: result(t6)
				A: call(t7)
				U: result(t7)
			`,
			slides: []string{"A a1 a4"},
			tokens: 110,
		},
		{
			// 50 to free: A's body gives 40, and the frame the session
			// stands in gives its oldest rounds, at least half of them.
			name: "and last the oldest rounds of the frame the session stands in",
			src:  nested,
			opts: ViewOptions{Budget: 110, Room: 10},
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t4)
				U: result(t4)*
				A: call(t7)
				U: result(t7)
			`,
			slides: []string{"A a1 a4", "B a4 a7"},
			tokens: 70,
		},
		{
			name: "the newest round of the frame always stays, however small the budget",
			src:  nested,
			opts: ViewOptions{Budget: 1},
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t4)
				U: result(t4)*
				A: call(t7)
				U: result(t7)
			`,
			slides: []string{"A a1 a4", "B a4 a7"},
			tokens: 70,
			over:   true,
		},
		{
			name: "the prompt the model is answering is never cut",
			src:  spoken,
			opts: ViewOptions{Budget: 60, Prompt: "q"},
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t3)
				U: result(t3) + and now the other thing
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
			`,
			slides: []string{"A a1 a3"},
			tokens: 90,
			over:   true,
		},
		{
			name: "but a prompt of an earlier turn goes with the rounds around it",
			src:  spoken,
			opts: ViewOptions{Budget: 60},
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t5)
				U: result(t5)
			`,
			slides: []string{"A a1 a5"},
			tokens: 50,
		},
		{
			// The prompt lies before the first frame, so that stretch is
			// two parts: the turns before the prompt, then the turn's own
			// rounds; each keeps its first round.
			name: "before the first frame, the turns before the prompt go first, then the turn's own rounds",
			src: `
				prompt first look around
				@a1 call t1:Read
				results t1
				@a2 call t2:Read
				results t2
				@a3 call t3:Read
				results t3
				say here is what I found
				@q prompt now fix it
				@a4 call t4:Read
				results t4
				@a5 call t5:Read
				results t5
				@a6 call t6:mem
				results t6
				push A t6
				@a7 call t7:Edit
				results t7
			`,
			opts: ViewOptions{Budget: 150, Room: 10, Prompt: "q"},
			want: `
				U: first look around
				A: call(t1)
				U: result(t1)*
				A: call(t3)
				U: result(t3)
				A: here is what I found
				U: now fix it
				A: call(t4)
				U: result(t4)*
				A: call(t6)
				U: result(t6)
				A: call(t7)
				U: result(t7)
			`,
			slides: []string{"- a1 a3", "- a4 a6"},
			tokens: 130,
		},
		{
			name: "outside any frame nothing slides: that is left to a summary",
			src: `
				prompt go
				call t1
				results t1
				call t2
				results t2
				call t3
				results t3
			`,
			opts: ViewOptions{Budget: 20},
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)
				A: call(t3)
				U: result(t3)
			`,
			tokens: 70,
			over:   true,
		},
		{
			name: "a frame whose push is behind the summary cannot slide",
			src: `
				prompt task
				call t1:mem
				results t1
				push A t1
				call t2:Read
				results t2
				summary so far
				notice carry on
				call t3:Read
				results t3
				call t4:Read
				results t4
			`,
			opts: ViewOptions{Budget: 20},
			want: `
				U: summary(so far) + carry on
				A: call(t3)
				U: result(t3)
				A: call(t4)
				U: result(t4)
			`,
			tokens: 50,
			over:   true,
		},
		{
			name: "one round of the frame is kept by default",
			src:  four,
			opts: ViewOptions{Budget: 10},
			want: `
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t5)
				U: result(t5)
			`,
			slides: []string{"A a1 a5"},
			tokens: 50,
			over:   true,
		},
		{
			name: "KeepRounds keeps more of them",
			src:  four,
			opts: ViewOptions{Budget: 10, KeepRounds: 3},
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
			slides: []string{"A a1 a3"},
			tokens: 90,
			over:   true,
		},
		{
			name: "with as many rounds to keep as there are, nothing slides",
			src:  four,
			opts: ViewOptions{Budget: 10, KeepRounds: 4},
			want: `
				U: go
				A: call(t1)
				U: result(t1)
				A: call(t2)
				U: result(t2)
				A: call(t3)
				U: result(t3)
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
			`,
			tokens: 110,
			over:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.opts.Estimate = ten
			entries := script(t, tt.src)
			msgs, info := mustView(t, entries, tt.opts)
			if got := render(msgs); got != tidy(tt.want) {
				t.Errorf("view:\n%s\n--- want ---\n%s", got, tidy(tt.want))
			}
			if got := slideRefs(info.NewSlides); !reflect.DeepEqual(got, tt.slides) {
				t.Errorf("NewSlides = %q, want %q", got, tt.slides)
			}
			if info.Tokens != tt.tokens || info.OverBudget != tt.over {
				t.Errorf("Tokens = %d, OverBudget = %v; want %d, %v", info.Tokens, info.OverBudget, tt.tokens, tt.over)
			}

			// Slide plans the same cuts from the need alone.
			if need := 10*len(msgs0(t, entries)) - tt.opts.Budget; need > 0 {
				room := tt.opts.Room
				if room == 0 {
					room = tt.opts.Budget / 10
				}
				planned, cuts := Slide(entries, need+room, tt.opts)
				if got := slideRefs(planned); !reflect.DeepEqual(got, tt.slides) {
					t.Errorf("Slide = %q, want %q", got, tt.slides)
				}
				freed := 0
				for _, c := range cuts {
					freed += c.Tokens
				}
				if want := 10 * (len(msgs0(t, entries)) - len(msgs0(t, record(entries, planned)))); len(cuts) != len(planned) || freed != want {
					t.Errorf("Slide says its %d cuts free %d tokens; recording %d slides frees %d", len(cuts), freed, len(planned), want)
				}
			}

			// Once the slides are recorded, a view with no budget cuts the
			// same, and a view with the budget plans nothing more.
			recorded := record(entries, info.NewSlides)
			again, info2 := mustView(t, recorded, ViewOptions{Estimate: ten, Prompt: tt.opts.Prompt})
			if !reflect.DeepEqual(msgs, again) {
				t.Errorf("with the slides recorded the view is:\n%s\n--- it was ---\n%s", render(again), render(msgs))
			}
			for _, c := range info2.Cuts {
				if c.Planned || c.UUID == "" {
					t.Errorf("a recorded cut reads as planned: %+v", c)
				}
			}
			if len(info.NewSlides) > 0 {
				_, info3 := mustView(t, recorded, tt.opts)
				if len(info3.NewSlides) != 0 && !tt.over {
					t.Errorf("with the slides recorded, the view planned more: %q", slideRefs(info3.NewSlides))
				}
			}
		})
	}
}

// msgs0 returns the rows of a transcript's view with nothing planned, one
// message a row, for sizing a need.
func msgs0(t *testing.T, entries []Entry) []piece {
	t.Helper()
	var info ViewInfo
	c := newConv(entries, ViewOptions{}, &info)
	st := c.derive()
	for i := range st.cuts {
		c.apply(&st.cuts[i])
	}
	return c.pieces()
}

func TestSlideInfo(t *testing.T) {
	entries := script(t, `
		prompt go
		@a1 call t1:mem
		results t1
		push A t1
		@a2 call t2:Read
		results t2
		@a3 call t3:Read
		results t3
		@a4 call t4:mem
		results t4
		push B t4
		@a5 call t5:Edit
		results t5
		@a6 call t6:Edit
		results t6
		@a7 call t7:Edit
		results t7
	`)
	_, info := mustView(t, entries, ViewOptions{Budget: 110, Room: 10, Estimate: ten, Trace: true})
	want := []Cut{
		{
			Kind: OpSlide, Frame: "A", Planned: true, Live: true, Between: 4, Messages: 4, Tokens: 40,
			PushCall: "t1", PopCall: "t4", PushReply: "a1", PopReply: "a4",
			Note:    "[Then came the work in it, 2 tool calls (Read 2), removed from your context to make room.]",
			Removed: []string{"a2", "e6", "a3", "e8"}, SparedEntries: []string{},
		},
		{
			Kind: OpSlide, Frame: "B", Planned: true, Live: true, Between: 4, Messages: 4, Tokens: 40,
			PushCall: "t4", PopCall: "t7", PushReply: "a4", PopReply: "a7",
			Note:    "[Then came the work in it, 2 tool calls (Edit 2), removed from your context to make room.]",
			Removed: []string{"a5", "e13", "a6", "e15"}, SparedEntries: []string{},
		},
	}
	got := slices.Clone(info.Cuts)
	for i := range got {
		if n := len(got[i].Replies); n != 2 {
			t.Errorf("cut %d lists %d replies, want 2", i, n)
		}
		got[i].Replies = nil
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Cuts = %+v\nwant   %+v", got, want)
	}
	stack := []Frame{
		{ID: "A", Label: "label A", UUID: "e4", ToolUseID: "t1", Slid: true},
		{ID: "B", Label: "label B", UUID: "e11", ToolUseID: "t4", Slid: true},
	}
	if !reflect.DeepEqual(info.Stack, stack) {
		t.Errorf("Stack = %+v\nwant    %+v", info.Stack, stack)
	}
}

func TestRecordedSlide(t *testing.T) {
	const slid = `
		prompt go
		@a1 call t1:mem
		results t1
		push A t1
		@a2 call t2:Read
		results t2
		@a3 call t3:Read
		results t3
		@s slide A @a1 @a3       # the first round of A left to make room
		@a4 call t4:Edit
		results t4
	`
	t.Run("a recorded slide is cut in every later view", func(t *testing.T) {
		msgs, info := mustView(t, script(t, slid), ViewOptions{})
		want := `
			U: go
			A: call(t1)
			U: result(t1)*
			A: call(t3)
			U: result(t3)
			A: call(t4)
			U: result(t4)
		`
		if got := render(msgs); got != tidy(want) {
			t.Errorf("view:\n%s\n--- want ---\n%s", got, tidy(want))
		}
		note := "[Then came the work in it, 1 tool call (Read 1), removed from your context to make room.]"
		if got := core.TextOf(msgs[2].Blocks[0].Content); got != "out t1\n"+note {
			t.Errorf("the anchor's answer reads %q", got)
		}
		if len(info.Cuts) != 1 || info.Cuts[0].UUID != "s" || !info.Cuts[0].Live || info.Cuts[0].Planned || info.Cuts[0].Messages != 2 {
			t.Errorf("Cuts = %+v", info.Cuts)
		}
		if len(info.Stack) != 1 || !info.Stack[0].Slid {
			t.Errorf("Stack = %+v, want A, slid", info.Stack)
		}
	})
	t.Run("when the frame is popped, the pop's note takes the slide's place", func(t *testing.T) {
		msgs, info := mustView(t, script(t, slid+`
			call t5:mem
			results t5
			pop A t5 done
		`), ViewOptions{})
		want := `
			U: go
			A: call(t1)
			U: result(t1)*
			A: call(t5)
			U: result(t5)
		`
		if got := render(msgs); got != tidy(want) {
			t.Errorf("view:\n%s\n--- want ---\n%s", got, tidy(want))
		}
		// The pop's note tells of all the work, the slid round included.
		note := "[Then came the work in it, 3 tool calls (Edit 1, Read 2), removed from your context when you popped it.]"
		if got := core.TextOf(msgs[2].Blocks[0].Content); got != "out t1\n"+note {
			t.Errorf("the push's answer reads %q", got)
		}
		if len(info.Cuts) != 2 || info.Cuts[0].Live || info.Cuts[1].Kind != OpPop || info.Cuts[1].Messages != 4 {
			t.Errorf("Cuts = %+v", info.Cuts)
		}
	})
	t.Run("a slide whose bounds are not in the conversation cuts nothing", func(t *testing.T) {
		msgs, info := mustView(t, script(t, `
			prompt go
			@a1 call t1:mem
			results t1
			push A t1
			@a2 call t2:Read
			results t2
			slide A @nowhere @a2
			slide A @a2 @a1          # the wrong way round
			slide A @a1 @a2          # nothing lies between
		`), ViewOptions{})
		if len(msgs) != 5 || len(info.Cuts) != 3 {
			t.Fatalf("view has %d messages, Cuts = %+v", len(msgs), info.Cuts)
		}
		for _, c := range info.Cuts {
			if c.Messages != 0 || c.Reason == "" {
				t.Errorf("cut = %+v, want nothing cut and a reason", c)
			}
		}
	})
	t.Run("a slide of the rounds before the first frame is live until the stack empties", func(t *testing.T) {
		const src = `
			prompt go
			@a1 call t1:Read
			results t1
			@a2 call t2:Read
			results t2
			@a3 call t3:mem
			results t3
			push A t3
			slide - @a1 @a3
			call t4
			results t4
		`
		_, info := mustView(t, script(t, src), ViewOptions{})
		if len(info.Cuts) != 1 || !info.Cuts[0].Live || info.Cuts[0].Frame != "" || info.Stack[0].Slid {
			t.Errorf("Cuts = %+v, Stack = %+v", info.Cuts, info.Stack)
		}
		_, info = mustView(t, script(t, src+`
			pop A t4 done
		`), ViewOptions{})
		if len(info.Cuts) != 2 || info.Cuts[0].Live {
			t.Errorf("after the pop, Cuts = %+v", info.Cuts)
		}
	})
}

// Every step of the view at once: a rewind, a summary, a popped frame, a
// slide and a repair, in one session.
func TestAllStagesTogether(t *testing.T) {
	entries := script(t, `
		prompt start
		say first answer
		@s summary the early work
		prompt do the big thing
		@a1 call t1:mem
		results t1
		push A t1
		@a2 call t2:Read
		@r results t2
		call t3:Edit             # a wrong turn, rewound
		results t3
		rewind @r
		@a3 call t4:Read
		results t4
		@a4 call t5:mem
		results t5
		push B t5
		@a5 call t6:Edit
		results t6
		@a6 call t7:mem
		results t7
		@pop pop B t7 b is done
		@a7 call t8:Bash         # gnosh was killed before this came back
	`)
	// Without a budget: the rewind, the summary, the pop and the repair.
	msgs, info := mustView(t, entries, ViewOptions{Estimate: ten})
	want := `
		U: summary(the early work) + do the big thing
		A: call(t1)
		U: result(t1)
		A: call(t2)
		U: result(t2)
		A: call(t4)
		U: result(t4)
		A: call(t5)
		U: result(t5)*
		A: call(t7)
		U: result(t7)
		A: call(t8)
		U: interrupted(t8)
	`
	if got := render(msgs); got != tidy(want) {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, tidy(want))
	}
	if info.Rewound != 2 || info.Summary != "s" || info.Summarized != 2 || info.Merged != 1 || info.Tokens != 130 ||
		len(info.Cuts) != 1 || info.Cuts[0].Messages != 2 || len(info.Stack) != 1 || !reflect.DeepEqual(kinds(info), []RepairKind{RepairFilledResult}) {
		t.Errorf("info = %+v", info)
	}

	// Over its budget, the frame the session stands in slides as well:
	// 130 tokens against 100, so 40 must go; the cut takes at least half
	// of the 90 it could.
	msgs, info = mustView(t, entries, ViewOptions{Estimate: ten, Budget: 100, Trace: true})
	want = `
		U: summary(the early work) + do the big thing
		A: call(t1)
		U: result(t1)*
		A: call(t7)
		U: result(t7)
		A: call(t8)
		U: interrupted(t8)
	`
	if got := render(msgs); got != tidy(want) {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, tidy(want))
	}
	if got := slideRefs(info.NewSlides); !reflect.DeepEqual(got, []string{"A a1 a6"}) {
		t.Errorf("NewSlides = %q", got)
	}
	if info.Tokens != 70 || info.OverBudget || len(info.Cuts) != 2 || !info.Stack[0].Slid {
		t.Errorf("info = %+v", info)
	}
	// The pop had taken B's round of work; the slide takes the other six
	// messages, and its note tells of all four calls between.
	slide := info.Cuts[1]
	if slide.Messages != 6 || slide.Tokens != 60 || !slide.Planned ||
		slide.Note != "[Then came the work in it, 4 tool calls (Edit 1, Read 2, mem 1), removed from your context to make room.]" {
		t.Errorf("slide = %+v", slide)
	}
	if want := [][]string{{"s", "e4"}, {"a1"}, {"e6"}, {"a6"}, {"e21"}, {"a7"}, {}}; !reflect.DeepEqual(info.Sources, want) {
		t.Errorf("Sources = %v, want %v", info.Sources, want)
	}
}

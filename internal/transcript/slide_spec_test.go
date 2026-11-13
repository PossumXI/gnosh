package transcript

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// The slide as spec/gnosh/frames.md section 8 states it. The loop decides
// when a slide is tried and how much it must free (section 8.2: what is
// over the compaction point, and a tenth of the window); these tests give
// that need to Slide, as the loop's join does, and check what goes. Every
// row counts ten tokens, so that the sums can be done in the head.

// slid plans a slide of need tokens, records it as a session would, and
// returns what was planned ("frame anchor until" each), the cuts, and the
// view with it recorded.
func slid(t *testing.T, src string, need int, prompt string) ([]string, []Cut, string, []Entry) {
	t.Helper()
	entries := script(t, src)
	opts := ViewOptions{Estimate: ten, Prompt: prompt, Trace: true}
	slides, cuts := Slide(entries, need, opts)
	if len(slides) != len(cuts) {
		t.Fatalf("%d slides and %d cuts", len(slides), len(cuts))
	}
	recorded := entries
	if len(slides) > 0 {
		recorded = record(entries, slides)
	}
	msgs, _ := mustView(t, recorded, ViewOptions{Estimate: ten})
	return slideRefs(slides), cuts, render(msgs), recorded
}

// freed sums what cuts took.
func freed(cuts []Cut) int {
	n := 0
	for _, c := range cuts {
		n += c.Tokens
	}
	return n
}

// A session that explored before it entered a frame.
const specExplored = `
	prompt explore and fix
	@a1 call t1:Read
	results t1
	@a2 call t2:Read
	results t2
	@a3 call t3:Read
	results t3
	@a4 call t4:gnovm_run
	results t4
	push A t4
	@a5 call t5:Edit
	results t5
	@a6 call t6:Edit
	results t6
`

// A session two frames deep.
const specNested = `
	prompt go
	@a1 call t1:gnovm_run
	results t1
	push A t1
	@a2 call t2:Read
	results t2
	@a3 call t3:Read
	results t3
	@a4 call t4:gnovm_run
	results t4
	push B t4
	@a5 call t5:Edit
	results t5
	@a6 call t6:Edit
	results t6
	@a7 call t7:Edit
	results t7
`

// Section 8.1: "At the point where the session would be compacted, standing
// in frames, gnosh cuts instead of summarizing ... Compaction remains only
// with no frame to cut." And 8.2: "If nothing can slide, the ordinary
// compaction decision runs."
func TestSpec8_1_OnlyASessionInFramesSlides(t *testing.T) {
	const rounds = `
		prompt go
		call t1:Read
		results t1
		call t2:Read
		results t2
		call t3:Read
		results t3
	`
	if refs, _, _, _ := slid(t, rounds, 1000, ""); len(refs) != 0 {
		t.Errorf("a session in no frame slid %q", refs)
	}
	if refs, _, _, _ := slid(t, rounds+`
		call t4:gnovm_run
		results t4
		push A t4
		call t5:Edit
		results t5
	`, 1000, ""); len(refs) == 0 {
		t.Errorf("a session in a frame did not slide")
	}
	// A frame entered and left again is no frame to cut.
	if refs, _, _, _ := slid(t, rounds+`
		call t4:gnovm_run
		results t4
		push A t4
		call t5:Edit
		results t5
		call t6:gnovm_run
		results t6
		pop A t6 done
		call t7:Read
		results t7
	`, 1000, ""); len(refs) != 0 {
		t.Errorf("a session that left its frame slid %q", refs)
	}
}

// Section 8.3 (a): "First: the rounds before the first frame ... The
// region's first round stays ... The cut runs from after that anchor round
// to just before a later assistant row that has a tool call, the bottom
// frame's push row at the latest." And 8.4 for where it ends.
func TestSpec8_3a_TheRoundsBeforeTheFirstFrameGoFirst(t *testing.T) {
	tests := []struct {
		name   string
		need   int
		slides []string
		freed  int
		view   string
	}{
		{
			// The part holds 40 tokens: the shortest cut that frees the need
			// and at least half of them ends before the third read.
			name: "a small need takes half of the part", need: 10,
			slides: []string{"- a1 a3"}, freed: 20,
			view: `
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
		},
		{
			name: "a larger need takes it up to the bottom frame's push", need: 30,
			slides: []string{"- a1 a4"}, freed: 40,
			view: `
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
		},
		{
			// The push round of the frame is never cut, and neither is the
			// frame's newest round: only then comes (c), the oldest rounds
			// of the frame itself.
			name: "when that is not enough the frame's own oldest rounds follow", need: 1000,
			slides: []string{"- a1 a4", "A a4 a6"}, freed: 60,
			view: `
				U: explore and fix
				A: call(t1)
				U: result(t1)*
				A: call(t4)
				U: result(t4)*
				A: call(t6)
				U: result(t6)
			`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, cuts, view, _ := slid(t, specExplored, tt.need, "")
			if !reflect.DeepEqual(refs, tt.slides) || freed(cuts) != tt.freed {
				t.Errorf("slides = %q freeing %d, want %q freeing %d", refs, freed(cuts), tt.slides, tt.freed)
			}
			if view != tidy(tt.view) {
				t.Errorf("view:\n%s\n--- want ---\n%s", view, tidy(tt.view))
			}
			if cuts[0].Frame != "" || cuts[0].Before != "A" {
				t.Errorf("the first cut is %+v, want the rounds before A", cuts[0])
			}
		})
	}
}

// Section 8.3 (a): "If the prompt the turn is answering lies in this
// region, the region is two parts, tried in order: the turns before the
// prompt first (the cut ends at a row before the prompt), then the turn's
// own rounds after it; each part anchored by its own first round." And
// 8.5: "Before a new prompt goes out nothing is protected ... A slide does
// not spare other prompt rows inside its range".
func TestSpec8_3a_ThePromptLiesBeforeTheFirstFrame(t *testing.T) {
	const src = `
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
		@a6 call t6:gnovm_run
		results t6
		push A t6
		@a7 call t7:Edit
		results t7
	`
	t.Run("the turns before the prompt first", func(t *testing.T) {
		refs, cuts, view, _ := slid(t, src, 10, "q")
		if !reflect.DeepEqual(refs, []string{"- a1 a3"}) || freed(cuts) != 20 {
			t.Errorf("slides = %q freeing %d", refs, freed(cuts))
		}
		if !strings.Contains(view, "U: now fix it") || !strings.Contains(view, "A: here is what I found") {
			t.Errorf("view:\n%s", view)
		}
	})
	t.Run("then the turn's own rounds after it", func(t *testing.T) {
		refs, cuts, view, _ := slid(t, src, 30, "q")
		if !reflect.DeepEqual(refs, []string{"- a1 a3", "- a4 a6"}) || freed(cuts) != 40 {
			t.Errorf("slides = %q freeing %d", refs, freed(cuts))
		}
		if want := tidy(`
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
		`); view != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
		}
	})
	t.Run("before a new prompt goes out nothing is protected", func(t *testing.T) {
		refs, cuts, view, _ := slid(t, src, 30, "")
		if !reflect.DeepEqual(refs, []string{"- a1 a4"}) || freed(cuts) != 60 {
			t.Errorf("slides = %q freeing %d", refs, freed(cuts))
		}
		if strings.Contains(view, "now fix it") {
			t.Errorf("an earlier prompt was spared by a slide:\n%s", view)
		}
	})
}

// Section 8.3 (b): "Then each frame from the bottom of the stack. A frame
// with a frame above it: its whole body goes, 'from its push's round to the
// next frame's push'. A body with nothing left to cut is passed over."
// Then (c): "Last: the oldest rounds of the top frame". And 14.15: "lower
// frame first".
func TestSpec8_3b_TheLowerFramesWholeBodyThenTheTopFramesOldestRounds(t *testing.T) {
	t.Run("the lower frame's whole body, whatever the need", func(t *testing.T) {
		refs, cuts, view, _ := slid(t, specNested, 10, "")
		if !reflect.DeepEqual(refs, []string{"A a1 a4"}) || freed(cuts) != 40 {
			t.Errorf("slides = %q freeing %d", refs, freed(cuts))
		}
		if want := tidy(`
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
		`); view != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
		}
	})
	t.Run("then the oldest rounds of the top frame", func(t *testing.T) {
		// 40 from A's body leaves 10 to free; the top frame holds 60, and a
		// cut takes at least half of that.
		refs, cuts, view, _ := slid(t, specNested, 50, "")
		if !reflect.DeepEqual(refs, []string{"A a1 a4", "B a4 a7"}) || freed(cuts) != 80 {
			t.Errorf("slides = %q freeing %d", refs, freed(cuts))
		}
		if want := tidy(`
			U: go
			A: call(t1)
			U: result(t1)*
			A: call(t4)
			U: result(t4)*
			A: call(t7)
			U: result(t7)
		`); view != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
		}
	})
	t.Run("a body with nothing left to cut is passed over", func(t *testing.T) {
		_, _, _, recorded := slid(t, specNested, 10, "")
		slides, cuts := Slide(recorded, 10, ViewOptions{Estimate: ten})
		if got := slideRefs(slides); !reflect.DeepEqual(got, []string{"B a4 a7"}) || freed(cuts) != 40 {
			t.Errorf("the next slide is %q freeing %d", got, freed(cuts))
		}
	})
}

// Section 8.3 (b): "If the prompt the turn is answering lies inside that
// body, only the oldest rounds from the frame's start up to a row before
// the prompt go (as (c))."
func TestSpec8_3b_ThePromptLiesInsideTheLowerFramesBody(t *testing.T) {
	refs, cuts, view, _ := slid(t, `
		prompt go
		@a1 call t1:gnovm_run
		results t1
		push A t1
		@a2 call t2:Read
		results t2
		@a3 call t3:Read
		results t3
		say what next?
		@q prompt enter the next one
		@a4 call t4:Read
		results t4
		@a5 call t5:gnovm_run
		results t5
		push B t5
		@a6 call t6:Edit
		results t6
		@a7 call t7:Edit
		results t7
	`, 1000, "q")
	if !reflect.DeepEqual(refs, []string{"A a1 a3", "B a5 a7"}) || freed(cuts) != 40 {
		t.Errorf("slides = %q freeing %d", refs, freed(cuts))
	}
	if want := tidy(`
		U: go
		A: call(t1)
		U: result(t1)*
		A: call(t3)
		U: result(t3)
		A: what next?
		U: enter the next one
		A: call(t4)
		U: result(t4)
		A: call(t5)
		U: result(t5)*
		A: call(t7)
		U: result(t7)
	`); view != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
	}
}

// Section 8.3 (c): "from after its push round to just before a later
// assistant row that has a tool call (and before the prompt, when the
// prompt lies in its body)". Section 8.5: the prompt is "the row of the
// newest prompt of the turn, typed, folded in or hidden, held by id". And
// 14.15: "before the prompt; before a hidden prompt".
func TestSpec8_3c_TheTopFrameAndThePrompt(t *testing.T) {
	src := func(prompt string) string {
		return `
			prompt go
			@a1 call t1:gnovm_run
			results t1
			push A t1
			@a2 call t2:Read
			results t2
			@a3 call t3:Read
			results t3
			@q ` + prompt + `
			@a4 call t4:Edit
			results t4
			@a5 call t5:Edit
			results t5
		`
	}
	for name, prompt := range map[string]string{
		"a prompt the person typed":      "prompt and now the other thing",
		"a hidden prompt of the harness": "notice and now the other thing",
		"words a hook put in":            "hook and now the other thing",
	} {
		t.Run(name, func(t *testing.T) {
			refs, cuts, view, _ := slid(t, src(prompt), 1000, "q")
			if !reflect.DeepEqual(refs, []string{"A a1 a3"}) || freed(cuts) != 20 {
				t.Errorf("slides = %q freeing %d", refs, freed(cuts))
			}
			if want := tidy(`
				U: go
				A: call(t1)
				U: result(t1)*
				A: call(t3)
				U: result(t3) + and now the other thing
				A: call(t4)
				U: result(t4)
				A: call(t5)
				U: result(t5)
			`); view != want {
				t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
			}
		})
	}
	t.Run("a prompt that is not the one being answered goes with the rounds around it", func(t *testing.T) {
		refs, cuts, view, _ := slid(t, src("prompt and now the other thing"), 1000, "")
		if !reflect.DeepEqual(refs, []string{"A a1 a5"}) || freed(cuts) != 70 {
			t.Errorf("slides = %q freeing %d", refs, freed(cuts))
		}
		if want := tidy(`
			U: go
			A: call(t1)
			U: result(t1)*
			A: call(t5)
			U: result(t5)
		`); view != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
		}
	})
}

// Section 8.3: "The walk stops, with no slide, at the first frame whose
// push row is not among the rows. When that is the bottom frame, (a) is
// skipped as well."
func TestSpec8_3_TheWalkStopsAtAFrameWhosePushIsNotAmongTheRows(t *testing.T) {
	t.Run("the bottom frame: nothing slides", func(t *testing.T) {
		refs, _, _, _ := slid(t, `
			prompt go
			@a1 call t1:Read
			results t1
			@a2 call t2:Read
			results t2
			push A -                     # its push call is not known
			@a3 call t3:Edit
			results t3
			@a4 call t4:Edit
			results t4
		`, 1000, "")
		if len(refs) != 0 {
			t.Errorf("slides = %q, want none", refs)
		}
	})
	t.Run("a frame above the bottom: only what lies below it", func(t *testing.T) {
		refs, _, _, _ := slid(t, `
			prompt go
			@a1 call t1:Read
			results t1
			@a2 call t2:Read
			results t2
			@a3 call t3:gnovm_run
			results t3
			push A t3
			@a4 call t4:Read
			results t4
			push B -                     # its push call is not known
			@a5 call t5:Read
			results t5
			@a6 call t6:gnovm_run
			results t6
			push C t6
			@a7 call t7:Edit
			results t7
			@a8 call t8:Edit
			results t8
		`, 1000, "")
		if !reflect.DeepEqual(refs, []string{"- a1 a3"}) {
			t.Errorf("slides = %q, want the rounds before the first frame alone", refs)
		}
	})
	t.Run("a frame whose push round a pop has removed", func(t *testing.T) {
		// A pop of a frame below the top (which mem refuses) takes the
		// upper frame's push round with it.
		refs, _, _, _ := slid(t, `
			prompt go
			@a1 call t1:gnovm_run
			results t1
			push A t1
			@a2 call t2:gnovm_run
			results t2
			push B t2
			@a3 call t3:Read
			results t3
			@a4 call t4:gnovm_run
			results t4
			pop A t4 done
			@a5 call t5:Edit
			results t5
			@a6 call t6:Edit
			results t6
		`, 1000, "")
		if len(refs) != 0 {
			t.Errorf("slides = %q, want none", refs)
		}
	})
}

// Section 8.3: "Only rows the model still has are counted or cut: rows not
// already removed by a pop or slide, and not at or before the last summary
// row." Section 8.5: "Summary rows and the rows behind them" are never cut.
// And 14.15: "rows behind a summary neither counted nor cut".
func TestSpec8_3_OnlyRowsTheModelStillHasAreCountedOrCut(t *testing.T) {
	t.Run("rows a pop removed", func(t *testing.T) {
		// The part before the first frame holds X's push round, its two
		// rounds of work (popped) and its pop round, and one more read:
		// 60 tokens the model still has, not 100.
		refs, cuts, view, _ := slid(t, `
			prompt go
			@a1 call t1:Read
			results t1
			@a2 call t2:gnovm_run
			results t2
			push X t2
			@a3 call t3:Read
			results t3
			@a4 call t4:Read
			results t4
			@a5 call t5:gnovm_run
			results t5
			pop X t5 done
			@a6 call t6:Read
			results t6
			@a7 call t7:gnovm_run
			results t7
			push A t7
			@a8 call t8:Edit
			results t8
		`, 10, "")
		if !reflect.DeepEqual(refs, []string{"- a1 a6"}) {
			t.Fatalf("slides = %q, want the cut that frees half of the 60", refs)
		}
		if c := cuts[0]; c.Tokens != 40 || c.Messages != 4 || c.Between != 8 {
			t.Errorf("cut = %+v, want 40 tokens in 4 messages of the 8 between", c)
		}
		if want := tidy(`
			U: go
			A: call(t1)
			U: result(t1)*
			A: call(t6)
			U: result(t6)
			A: call(t7)
			U: result(t7)
			A: call(t8)
			U: result(t8)
		`); view != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
		}
	})
	t.Run("rows behind the summary", func(t *testing.T) {
		refs, cuts, view, _ := slid(t, `
			prompt old work
			call t0:Read
			results t0
			summary earlier work
			notice carry on
			@a1 call t1:Read
			results t1
			@a2 call t2:Read
			results t2
			@a3 call t3:gnovm_run
			results t3
			push A t3
			@a4 call t4:Edit
			results t4
		`, 1000, "")
		if !reflect.DeepEqual(refs, []string{"- a1 a3"}) || freed(cuts) != 20 {
			t.Errorf("slides = %q freeing %d", refs, freed(cuts))
		}
		if want := tidy(`
			U: summary(earlier work) + carry on
			A: call(t1)
			U: result(t1)*
			A: call(t3)
			U: result(t3)
			A: call(t4)
			U: result(t4)
		`); view != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", view, want)
		}
	})
}

// Section 8.4: "let body be the tokens of the still-present rows from after
// the anchor round to the end of the part. Walk the candidate end rows
// (assistant rows with a tool call) oldest first. Take the shortest cut
// that frees at least max(need, body / 2); if none does, the longest. A cut
// that frees nothing is no cut."
func TestSpec8_4_HowMuchAPartialCutTakes(t *testing.T) {
	// A frame with five rounds in it: a body of 100 tokens.
	const five = `
		prompt go
		@a1 call t1:gnovm_run
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
		@a6 call t6
		results t6
	`
	for _, tt := range []struct {
		name  string
		need  int
		slide string
		freed int
	}{
		{"a small need: half of the body", 10, "A a1 a5", 60},
		{"the need, when it is more than half", 70, "A a1 a6", 80},
		{"more than there is: the longest cut", 500, "A a1 a6", 80},
	} {
		t.Run(tt.name, func(t *testing.T) {
			refs, cuts, _, _ := slid(t, five, tt.need, "")
			if !reflect.DeepEqual(refs, []string{tt.slide}) || freed(cuts) != tt.freed {
				t.Errorf("slides = %q freeing %d, want %q freeing %d", refs, freed(cuts), tt.slide, tt.freed)
			}
		})
	}
	t.Run("a cut that frees nothing is no cut", func(t *testing.T) {
		refs, _, _, _ := slid(t, `
			prompt go
			@a1 call t1:gnovm_run
			results t1
			push A t1
			@a2 call t2
			results t2
		`, 500, "")
		if len(refs) != 0 {
			t.Errorf("slides = %q, want none: the frame holds one round, and a cut ends before a round", refs)
		}
	})
	t.Run("nothing is planned for no need", func(t *testing.T) {
		if refs, _, _, _ := slid(t, five, 0, ""); len(refs) != 0 {
			t.Errorf("slides = %q for a need of nothing", refs)
		}
	})
}

// Section 8.6: "A slide's cut is recorded and applied exactly as a pop's
// range: ... the range joins the session's ranges so every later request
// leaves them out ... So 'a record derived again from the rows cuts the
// same'."
//
// The record is a frame entry that names the two replies that bound the
// cut by their entries, where the spec's marker names them by a call each
// holds (CONFORMANCE.md has why). This test is the requirement behind the
// marker: derived again from the file, the record cuts the same.
func TestSpec8_6_ARecordedSlideCutsTheSameInEveryLaterRequest(t *testing.T) {
	refs, _, view, recorded := slid(t, specNested, 50, "")
	if len(refs) != 2 {
		t.Fatalf("slides = %q", refs)
	}
	// Later requests, with more of the conversation after it.
	more := append(recorded[:len(recorded):len(recorded)], named("m-", script(t, `
		call t8:Bash
		results t8
		say done for now
		prompt next
	`))...)
	msgs, info := mustView(t, more, ViewOptions{Estimate: ten})
	if got := render(msgs); !strings.HasPrefix(got, view) || len(info.NewSlides) != 0 {
		t.Errorf("a later request no longer begins with the slid view:\n%s\n--- the slid view ---\n%s", got, view)
	}
	// A restart, which has only the file.
	ld, err := load(ctx, bytes.NewReader(jsonl(t, recorded)), "")
	if err != nil {
		t.Fatal(err)
	}
	again, info := mustView(t, ld.entries, ViewOptions{Estimate: ten})
	if got := render(again); got != view {
		t.Errorf("after a restart the view is\n%s\n--- it was ---\n%s", got, view)
	}
	for _, c := range info.Cuts {
		if c.Planned || c.UUID == "" || !c.Live {
			t.Errorf("a recorded slide reads as %+v", c)
		}
	}
	// And a copy of the session: the rows carry their marks into it.
	branch := append(ld.entries[:len(ld.entries):len(ld.entries)], Entry{Type: TypeBranch, UUID: "b", SessionID: "copy", V: Version})
	if got, _ := mustView(t, branch, ViewOptions{Estimate: ten}); render(got) != view {
		t.Errorf("in a copy of the session the view is\n%s\n--- it was ---\n%s", render(got), view)
	}
}

// Section 8.6: "Its note on the anchor's answer says '…, removed from your
// context to make room.]' (with no calls: '[Then came N messages, removed
// from your context to make room.]')."
//
// The first form is read as the pop's line with the other ending:
// CONFORMANCE.md records the passage and the reading.
func TestSpec8_6_TheSlidesNote(t *testing.T) {
	t.Run("with tool calls", func(t *testing.T) {
		_, cuts, _, recorded := slid(t, specNested, 10, "")
		msgs, _ := mustView(t, recorded, ViewOptions{})
		want := "[Then came the work in it, 2 tool calls (Read 2), removed from your context to make room.]"
		if got := answerTo(t, msgs, "t1"); got != "out t1\n"+want || cuts[0].Note != want {
			t.Errorf("the anchor's answer reads %q; the cut's note is %q", got, cuts[0].Note)
		}
	})
	t.Run("with no tool call", func(t *testing.T) {
		refs, cuts, _, recorded := slid(t, `
			prompt go
			@a1 call t1:gnovm_run
			results t1
			push A t1
			say let me think about this
			notice keep going
			@a2 call t2:Read
			results t2
		`, 10, "")
		if !reflect.DeepEqual(refs, []string{"A a1 a2"}) {
			t.Fatalf("slides = %q", refs)
		}
		msgs, _ := mustView(t, recorded, ViewOptions{})
		want := "[Then came 2 messages, removed from your context to make room.]"
		if got := answerTo(t, msgs, "t1"); got != "out t1\n"+want || cuts[0].Note != want {
			t.Errorf("the anchor's answer reads %q; the cut's note is %q", got, cuts[0].Note)
		}
	})
	t.Run("on the frame's own push answer, not on the first call of that reply", func(t *testing.T) {
		_, _, _, recorded := slid(t, `
			prompt go
			@a1 call t0:Read t1:gnovm_run    # the push is the second call of its reply
			results t0 t1
			push A t1
			@a2 call t2:Read
			results t2
			@a3 call t3:Read
			results t3
			@a4 call t4:Read
			results t4
		`, 10, "")
		msgs, _ := mustView(t, recorded, ViewOptions{})
		if got := answerTo(t, msgs, "t0"); got != "out t0" {
			t.Errorf("the answer beside the push's reads %q", got)
		}
		if got := answerTo(t, msgs, "t1"); !strings.HasSuffix(got, toMakeRoom) {
			t.Errorf("the push's answer reads %q", got)
		}
	})
}

// Section 8.6: "The line is repeated in frame lines only while the frame it
// names is still on the stack. A pop takes its frame out of the slid list
// ('pushed again under the same id, it is a new frame, with nothing cut
// from it'); a pop that empties the stack clears the list." And 14.15: "a
// frame popped and pushed again is not 'slid'". For the rounds before the
// first frame the line names the bottom frame.
func TestSpec8_6_TheSlidListFollowsTheStack(t *testing.T) {
	live := func(info ViewInfo) (out []string) {
		for _, c := range info.Cuts {
			if c.Kind == OpSlide && c.Live {
				out = append(out, c.Frame+"/"+c.Before)
			}
		}
		return out
	}
	slidFrames := func(info ViewInfo) (out []string) {
		for _, f := range info.Stack {
			if f.Slid {
				out = append(out, f.ID)
			}
		}
		return out
	}
	steps := []struct {
		lines string
		live  []string // each "frame/before"
		slid  []string
	}{
		{`
			prompt go
			@a1 call t1:Read
			results t1
			@a2 call t2:Read
			results t2
			@a3 call t3:gnovm_run
			results t3
			push A t3
			slide - @a1 @a3              # the rounds before A
			@a4 call t4:Read
			results t4
			@a5 call t5:gnovm_run
			results t5
			push B t5
			@a6 call t6:Read
			results t6
			@a7 call t7:Read
			results t7
			slide A @a3 @a5              # the whole body of A
			slide B @a5 @a7              # the start of B
		`, []string{"/A", "A/", "B/"}, []string{"A", "B"}},
		{`
			@a8 call t8:gnovm_run
			results t8
			pop B t8 done                # takes B out of the list
		`, []string{"/A", "A/"}, []string{"A"}},
		{`
			@a9 call t9:gnovm_run
			results t9
			push B t9                    # pushed again: a new frame, with nothing cut from it
		`, []string{"/A", "A/"}, []string{"A"}},
		{`
			@a10 call t10:gnovm_run
			results t10
			pop B t10 done again
			@a11 call t11:gnovm_run
			results t11
			pop A t11 all done           # empties the stack: the list is cleared
		`, nil, nil},
		{`
			@a12 call t12:gnovm_run
			results t12
			push A t12                   # and A again is a new frame too
		`, nil, nil},
	}
	src := ""
	for i, s := range steps {
		src += s.lines
		_, info := mustView(t, script(t, src), ViewOptions{})
		if got := live(info); !reflect.DeepEqual(got, s.live) {
			t.Errorf("step %d: the live slides are %q, want %q", i, got, s.live)
		}
		if got := slidFrames(info); !reflect.DeepEqual(got, s.slid) {
			t.Errorf("step %d: the frames marked slid are %q, want %q", i, got, s.slid)
		}
	}
}

// Section 8.8, the run that landed at a 24,576 window: "1 slide (the rounds
// before the first frame, 692 tokens, told once), 4 pops, 0 summaries", and
// the build before it: "the model read six files at the root before
// pushing anything, so there was nothing to slide in a frame (fixed by
// 8.3 (a) ...)". The run is rebuilt to scale: six reads at the root of 100
// tokens each, then a task of four todos; the compaction point is 700
// tokens and the window 1,000.
func TestSpec8_8_ExplorationSlidesPopsDoTheRest(t *testing.T) {
	size := func(m core.Message) int {
		n := 10
		for _, b := range m.Blocks {
			if b.Type == core.BlockToolResult && strings.HasPrefix(b.ToolUseID, "read") {
				n += 90
			}
		}
		return n
	}
	const point, window = 700, 1000
	src := `
		prompt rename klash to gnosh everywhere
		@r1 call read1:Read
		results read1
		@r2 call read2:Read
		results read2
		@r3 call read3:Read
		results read3
		@r4 call read4:Read
		results read4
		@r5 call read5:Read
		results read5
		@r6 call read6:Read
		results read6
		@plan call t1:gnovm_run
		results t1
		push rename t1
		call n1:gnovm_run
		results n1
		push rename/todo1 n1
		call e1:Edit
		results e1
		call b1:Bash
		results b1
	`
	entries := script(t, src)
	_, info := mustView(t, entries, ViewOptions{Estimate: size})
	if info.Tokens != 750 {
		t.Fatalf("the conversation holds %d tokens, want 750", info.Tokens)
	}
	// Over the point: the slide frees what is over it and a tenth of the
	// window besides.
	slides, cuts := Slide(entries, info.Tokens-point+window/10, ViewOptions{Estimate: size})
	if got := slideRefs(slides); !reflect.DeepEqual(got, []string{"- r1 r5"}) || cuts[0].Tokens != 330 || cuts[0].Before != "rename" {
		t.Fatalf("slides = %q, cuts = %+v; want the reads before the first frame", got, cuts)
	}
	entries = record(entries, slides)

	// The four todos, popped in turn. The conversation never comes back to
	// the point: no second slide, and no summary.
	pops := 0
	for i := 1; i <= 4; i++ {
		n := string(rune('0' + i))
		if i > 1 {
			entries = append(entries, named("w"+n+"-", script(t, `
				call e`+n+`:Edit
				results e`+n+`
				call b`+n+`:Bash
				results b`+n+`
			`))...)
		}
		next := ""
		if i < 4 {
			next = "push rename/todo" + string(rune('1'+i)) + " p" + n
		}
		entries = append(entries, named("p"+n+"-", script(t, `
			call p`+n+`:gnovm_run
			results p`+n+`
			pop rename/todo`+n+` p`+n+` done
			`+next))...)
		_, info := mustView(t, entries, ViewOptions{Estimate: size, Budget: point})
		if info.OverBudget || len(info.NewSlides) != 0 {
			t.Errorf("after todo %d the conversation holds %d tokens and wants to slide %q", i, info.Tokens, slideRefs(info.NewSlides))
		}
		for _, c := range info.Cuts {
			if c.Kind == OpPop && c.Frame == "rename/todo"+n && c.Tokens == 40 {
				pops++
			}
		}
	}
	_, info = mustView(t, entries, ViewOptions{Estimate: size})
	if pops != 4 || len(info.Cuts) != 5 || info.Tokens > point {
		t.Errorf("%d pops, %d cuts, %d tokens at the end; want 4 pops, 1 slide and the conversation under %d", pops, len(info.Cuts), info.Tokens, point)
	}
}

package transcript

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// What the package hands the loop. spec/gnosh/frames.md gives the harness
// duties around a pop that are not the view's: the clauses of the pop's
// result (section 5), forgetting what the removed rows vouched for (6), the
// frame line (7), the large-step line (8.7), the fresh guard (9), the
// pending estimate and the notice to the person (10). They belong to the
// loop's join; these tests hold that every fact they need is here, worked
// out once, and the same before the pop is stored as after.

// code returns the Gno a gnovm_run call ran.
func code(c Call) string {
	var args struct{ Code string }
	json.Unmarshal(c.Input, &args)
	return args.Code
}

// command returns the command line a Bash call ran.
func command(c Call) string {
	var args struct{ Command string }
	json.Unmarshal(c.Input, &args)
	return args.Command
}

// calls flattens a cut's replies into its tool calls, in order.
func calls(c Cut) (out []Call) {
	for _, r := range c.Replies {
		out = append(out, r.Calls...)
	}
	return out
}

// The frame of section 5's example: inside fix-flaky/find-cause the model
// read and edited a test file, failed to edit another, kept a fact and a
// thought, had a close refused, and showed a file with a shell command.
// Then it pops; the transcript ends at the reply that makes the call.
const findCause = `
	prompt the listener test is flaky
	call c1:gnovm_run
	results c1
	push fix-flaky c1
	call c2:gnovm_run
	results c2
	push fix-flaky/find-cause c2
	call c3:Read=/work/app/internal/net/listen_test.go
	results c3
	call c4:Edit=/work/app/internal/net/listen_test.go
	results c4
	call c5:Edit=/work/app/other.go
	fail c5
	call c6:gnovm_run=mem.Fact("port",_"8080")
	results c6
	call c7:gnovm_run=mem.Think("why",_"it_races")
	results c7
	call c8:gnovm_run=mem.Done("fix-flaky")
	fail c8
	call c9:Bash=cat_foo.go
	results c9
	call c10:gnovm_run=mem.Pop("it_races_on_the_shared_port")
`

// Section 5: the clauses "Everything since you entered `x` was removed from
// your context", "but for what the user said in this turn", "In it you
// edited …", "In it you also changed mem: …", "Your reads inside it are
// forgotten", and "nothing came between". Integration-contract.md 5.3.4
// has the order: the frames layer is handed the events, and "the text it
// returns" goes into the result, so all of it is needed before the pop's
// own result is stored. Section 6: "The harness forgets the reads of every
// path the removed rows touched: the file_path arguments of the view, edit,
// multi-edit and write tools, and the file a shell command showed".
func TestSpec5_ThePopsResultIsWrittenFromThePreview(t *testing.T) {
	entries := script(t, findCause)
	before := jsonl(t, entries)
	pop := PopEntry("fix-flaky/find-cause", "find why TestFlaky fails", "it races on the shared port", "c10")
	cuts, stack := Preview(entries, []Entry{pop}, ViewOptions{})
	if len(cuts) != 1 || ids(stack) != "fix-flaky" {
		t.Fatalf("cuts = %+v, stack = %q", cuts, ids(stack))
	}
	if !bytes.Equal(before, jsonl(t, entries)) {
		t.Fatalf("Preview changed the entries it was shown")
	}
	c := cuts[0]

	// "Everything since you entered … was removed": something came between.
	// Nothing was spared: the pop is made in the turn the frame was
	// entered in, and the prompt lies before the push.
	if c.Between != 14 || c.Messages != 14 || c.Spared != 0 || c.Tokens <= 0 {
		t.Errorf("cut = %+v", c)
	}
	// The files edited: successful calls of the file-changing tools, each
	// once. The failed edit of other.go is not one.
	var edited []string
	for _, call := range calls(c) {
		if call.Name == "Edit" && call.Answered && !call.IsError && !slices.Contains(edited, call.Path) {
			edited = append(edited, call.Path)
		}
	}
	if want := []string{"/work/app/internal/net/listen_test.go"}; !reflect.DeepEqual(edited, want) {
		t.Errorf("edited = %q, want %q", edited, want)
	}
	// The other changes to mem: the calls on mem whose result is not an
	// error, by their code.
	var kept, refused []string
	for _, call := range calls(c) {
		if call.Name != "gnovm_run" {
			continue
		}
		if call.IsError {
			refused = append(refused, code(call))
		} else {
			kept = append(kept, code(call))
		}
	}
	if want := []string{`mem.Fact("port", "8080")`, `mem.Think("why", "it races")`}; !reflect.DeepEqual(kept, want) {
		t.Errorf("kept mem calls = %q, want %q", kept, want)
	}
	if want := []string{`mem.Done("fix-flaky")`}; !reflect.DeepEqual(refused, want) {
		t.Errorf("refused mem calls = %q, want %q", refused, want)
	}
	// The reads to forget: every path a call named, and the commands a
	// shell ran, for the harness's own parser.
	var paths, commands []string
	for _, call := range calls(c) {
		if call.Path != "" && !slices.Contains(paths, call.Path) {
			paths = append(paths, call.Path)
		}
		if call.Name == "Bash" {
			commands = append(commands, command(call))
		}
	}
	if want := []string{"/work/app/internal/net/listen_test.go", "/work/app/other.go"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %q, want %q", paths, want)
	}
	if want := []string{"cat foo.go"}; !reflect.DeepEqual(commands, want) {
		t.Errorf("commands = %q, want %q", commands, want)
	}
	// What a hook or the harness gave the model in the removed rows (a
	// subdirectory's context file) is found by the entries removed.
	if want := []string{"e8", "e9", "e10", "e11", "e12", "e13", "e14", "e15", "e16", "e17", "e18", "e19", "e20", "e21"}; !reflect.DeepEqual(c.Removed, want) {
		t.Errorf("Removed = %q", c.Removed)
	}
	// Each call knows where its result is, for what the harness kept
	// beside it.
	if first := calls(c)[0]; first.ID != "c3" || first.ResultUUID != "e9" || core.TextOf(first.Content) != "out c3" {
		t.Errorf("the first call is %+v", first)
	}

	// The pop's result is stored, then the pop: the cut is the previewed
	// one, to the last field.
	stored := append(entries, script(t, `
		@r results c10
		@p pop fix-flaky/find-cause c10 it races on the shared port
	`)...)
	stored[len(stored)-1].Label = pop.Label
	msgs, info := mustView(t, stored, ViewOptions{Trace: true})
	got := cutBy(t, info, "p")
	got.UUID = ""
	if !reflect.DeepEqual(got, c) {
		t.Errorf("the stored pop's cut is\n%+v\nthe preview said\n%+v", got, c)
	}
	if want := "out c2\n[Then came the work in it, 7 tool calls (Bash 1, Edit 2, Read 1, gnovm_run 3)" + whenPopped; answerTo(t, msgs, "c2") != want || c.Note != want[len("out c2\n"):] {
		t.Errorf("the push's answer reads %q; the preview's note was %q", answerTo(t, msgs, "c2"), c.Note)
	}
}

// Section 5: "After 'removed from your context', when prompt rows were
// spared: ', but for what the user said in this turn'" and "When nothing
// came between the push and the pop: 'nothing came between, so nothing was
// removed from your context.'" And: "One line per pop when an input pops
// more than once."
func TestSpec5_TheOtherClausesHaveTheirFacts(t *testing.T) {
	t.Run("prompt rows were spared", func(t *testing.T) {
		cuts, _ := Preview(script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:Read
			results t2
			say which port?
			prompt 8080
			call t3:gnovm_run
		`), []Entry{PopEntry("A", "", "done", "t3")}, ViewOptions{})
		if c := cuts[0]; c.Spared != 1 || !reflect.DeepEqual(c.SparedEntries, []string{"e8"}) || c.Between != 3 {
			t.Errorf("cut = %+v", c)
		}
	})
	t.Run("nothing came between", func(t *testing.T) {
		cuts, _ := Preview(script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:gnovm_run
		`), []Entry{PopEntry("A", "", "done", "t2")}, ViewOptions{})
		if c := cuts[0]; c.Between != 0 || c.Messages != 0 || c.Tokens != 0 || len(c.Replies) != 0 || c.Reason == "" {
			t.Errorf("cut = %+v", c)
		}
	})
	t.Run("an input that pops more than once", func(t *testing.T) {
		cuts, stack := Preview(script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:gnovm_run
			results t2
			push A/x t2
			call t3:Read
			results t3
			call t4:gnovm_run
		`), []Entry{
			PopEntry("A/x", "", "x done", "t4"),
			PopEntry("A", "", "all done", "t4"),
			PushEntry("B", "", "t4"),
		}, ViewOptions{Estimate: ten})
		if len(cuts) != 2 || ids(stack) != "B" {
			t.Fatalf("cuts = %+v, stack = %q", cuts, ids(stack))
		}
		// The inner pop takes its round; the outer one what is left.
		if in, out := cuts[0], cuts[1]; in.Frame != "A/x" || in.Tokens != 20 || out.Frame != "A" || out.Tokens != 20 || out.Between != 4 {
			t.Errorf("cuts = %+v", cuts)
		}
	})
}

// Section 13.3, a real pop: the frame was entered by name, "the README
// read, edited and searched", and the pop's result said "Everything since
// you entered … was removed from your context: this result stays; the user
// still sees all of it. In it you edited README.md. Your reads inside it
// are forgotten: view a file before you edit it." No thought was kept, no
// prompt spared, nothing else changed in mem: each clause that is there,
// and each that is not, has its fact.
func TestSpec13_3_ARealPop(t *testing.T) {
	cuts, _ := Preview(script(t, `
		prompt rename klash to gnosh everywhere
		call c1:gnovm_run=mem.Frame("klash-to-gnosh-rename/gnosh-readme-rename").Push()
		results c1
		push klash-to-gnosh-rename/gnosh-readme-rename c1
		call c2:Read=/work/app/README.md
		results c2
		call c3:Edit=/work/app/README.md
		results c3
		call c4:Edit=/work/app/README.md
		results c4
		call c5:Grep
		results c5
		call c6:gnovm_run=mem.Pop("README.md_fully_renamed")
	`), []Entry{PopEntry("klash-to-gnosh-rename/gnosh-readme-rename", "rename klash to gnosh in README.md", "README.md fully renamed", "c6")}, ViewOptions{})
	c := cuts[0]
	var edited, read, mem []string
	for _, call := range calls(c) {
		switch {
		case call.Name == "Edit" && !call.IsError && !slices.Contains(edited, call.Path):
			edited = append(edited, call.Path)
		case call.Name == "Read":
			read = append(read, call.Path)
		case call.Name == "gnovm_run":
			mem = append(mem, code(call))
		}
	}
	if c.Between != 8 || c.Spared != 0 || len(mem) != 0 ||
		!reflect.DeepEqual(edited, []string{"/work/app/README.md"}) || !reflect.DeepEqual(read, []string{"/work/app/README.md"}) {
		t.Errorf("cut = %+v\nedited %q, read %q, mem calls %q", c, edited, read, mem)
	}
	if want := "[Then came the work in it, 4 tool calls (Edit 2, Grep 1, Read 1)" + whenPopped; c.Note != want {
		t.Errorf("Note = %q, want %q", c.Note, want)
	}
}

// The first builder's claim: "a pop can be previewed by viewing the entries
// with a pop appended". It holds only for an entry that carries the
// session's id, which an entry not yet appended does not: without it the
// pop is nobody's and changes nothing. Preview stamps it.
func TestPreview(t *testing.T) {
	entries := script(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		call t3:gnovm_run
	`)
	pop := PopEntry("A", "", "done", "t3")
	t.Run("an entry with no session id is not the session's", func(t *testing.T) {
		_, info := View(append(entries[:len(entries):len(entries)], pop), ViewOptions{Session: "s1"})
		if len(info.Cuts) != 0 || len(info.UnmatchedPops) != 1 || ids(info.Stack) != "A" {
			t.Errorf("Cuts = %+v, UnmatchedPops = %v, Stack = %q", info.Cuts, info.UnmatchedPops, ids(info.Stack))
		}
	})
	t.Run("Preview takes it for the session's own", func(t *testing.T) {
		cuts, stack := Preview(entries, []Entry{pop}, ViewOptions{})
		if len(cuts) != 1 || cuts[0].Messages != 2 || cuts[0].UUID != "" || cuts[0].PopReply != "e7" || len(stack) != 0 {
			t.Errorf("cuts = %+v, stack = %+v", cuts, stack)
		}
	})
	t.Run("a pop of a frame the session does not stand in", func(t *testing.T) {
		cuts, stack := Preview(entries, []Entry{PopEntry("B", "", "done", "t3")}, ViewOptions{})
		if len(cuts) != 1 || cuts[0].Reason == "" || cuts[0].Messages != 0 || cuts[0].Frame != "B" || ids(stack) != "A" {
			t.Errorf("cuts = %+v, stack = %q", cuts, ids(stack))
		}
	})
	t.Run("another session's pop does not touch this one's stack", func(t *testing.T) {
		theirs := pop
		theirs.SessionID = "someone-else"
		cuts, stack := Preview(entries, []Entry{theirs}, ViewOptions{})
		if len(cuts) != 1 || cuts[0].Reason == "" || ids(stack) != "A" {
			t.Errorf("cuts = %+v, stack = %q", cuts, ids(stack))
		}
	})
	t.Run("a push alone makes no cut", func(t *testing.T) {
		cuts, stack := Preview(entries, []Entry{PushEntry("A/x", "the next one", "t3")}, ViewOptions{})
		if len(cuts) != 0 || ids(stack) != "A A/x" || stack[1].Label != "the next one" || stack[1].ToolUseID != "t3" {
			t.Errorf("cuts = %+v, stack = %+v", cuts, stack)
		}
	})
	t.Run("a session previews for its own stack", func(t *testing.T) {
		st, _ := newStore(t)
		s, _ := st.Create(cwd)
		defer s.Close()
		mustAppend(t, s, entries...)
		cuts, stack := s.Preview([]Entry{pop}, ViewOptions{})
		if len(cuts) != 1 || cuts[0].Messages != 2 || len(stack) != 0 {
			t.Errorf("cuts = %+v, stack = %+v", cuts, stack)
		}
		if got := ids(s.Stack()); got != "A" || len(s.Entries()) != len(entries) {
			t.Errorf("previewing changed the session: stack %q, %d entries", got, len(s.Entries()))
		}
	})
}

// Section 9: "The top frame is fresh unless ... Work came between the push
// round and this call's round: in the rows 4.1 would take for a pop made by
// this call, the rows inner pops removed already included". Those rows are
// a previewed pop's Replies. "Work came since the plan was made: when the
// frame at the bottom of the stack is another frame, work between its push
// and now counts too": a previewed pop of the bottom frame. And 14.16: "a
// refused pop, a mem read and a failed call are no work".
func TestSpec9_TheFreshGuardsRowsAreAPreviewedPops(t *testing.T) {
	entries := script(t, `
		prompt do the two todos
		call c1:gnovm_run
		results c1
		push plan c1
		call c2:Edit=/w/a.go                     # work from the plan's root, before any todo was entered
		results c2
		call c3:gnovm_run=mem.Next()
		results c3
		push plan/one c3
		call c4:gnovm_run=mem.Show()             # a read of mem
		results c4
		call c5:Bash=mv_a_b                      # a call that failed
		fail c5
		call c6:gnovm_run=mem.Pop("done")        # a pop mem refused
		fail c6
		say it is done, as I said                # an answer, after a refused close
		notice the todo is still open
		call c7:gnovm_run=mem.Pop("done")        # the call at hand
	`)
	stack := Stack(entries, "")
	top, bottom := stack[len(stack)-1], stack[0]

	cuts, _ := Preview(entries, []Entry{PopEntry(top.ID, "", "", "c7")}, ViewOptions{})
	in := cuts[0]
	type fact struct {
		name, code string
		failed     bool
	}
	var got []fact
	answers := 0
	for _, r := range in.Replies {
		if r.Said && len(r.Calls) == 0 {
			answers++
		}
		for _, c := range r.Calls {
			got = append(got, fact{c.Name, code(c) + command(c), c.IsError})
		}
	}
	want := []fact{
		{"gnovm_run", "mem.Show()", false},
		{"Bash", "mv a b", true},
		{"gnovm_run", `mem.Pop("done")`, true},
	}
	if !reflect.DeepEqual(got, want) || answers != 1 {
		t.Errorf("the rows of the top frame hold %+v and %d answers, want %+v and 1", got, answers, want)
	}
	// The push is among the rows before this call's row, and was not made
	// by it.
	if in.PushCall != "c3" || in.PushReply == "" || in.PushReply == in.PopReply {
		t.Errorf("cut = %+v", in)
	}

	// Since the plan was made: the edit counts.
	cuts, after := Preview(entries, []Entry{PopEntry(bottom.ID, "", "", "c7")}, ViewOptions{})
	since := calls(cuts[0])
	if len(since) != 5 || since[0].Name != "Edit" || since[0].IsError || since[0].Path != "/w/a.go" {
		t.Errorf("the rows since the plan was made hold %+v", since)
	}
	// Asking takes nothing off the stack that is asked about.
	if ids(after) != "plan/one" || ids(Stack(entries, "")) != "plan plan/one" {
		t.Errorf("the stack after the previewed pop is %q", ids(after))
	}
}

// Section 9, rule 1: the top frame is not fresh when "its push call is not
// known; or its push row is not among the rows before this call's row".
func TestSpec9_AFrameWhosePushIsNotBeforeTheCall(t *testing.T) {
	for name, tt := range map[string]struct {
		src  string
		test func(Cut) bool
	}{
		"its push call is not known": {
			`prompt go
			 push A -
			 call t2:gnovm_run`,
			func(c Cut) bool { return c.PushCall == "" && c.PushReply == "" },
		},
		"its push row is not among the rows": {
			`prompt go
			 push A lost
			 call t2:gnovm_run`,
			func(c Cut) bool { return c.PushCall == "lost" && c.PushReply == "" },
		},
		"it was entered by the reply that now closes it": {
			`prompt go
			 call t1:gnovm_run t2:gnovm_run
			 results t1
			 push A t1`,
			func(c Cut) bool { return c.PushReply != "" && c.PushReply == c.PopReply },
		},
	} {
		t.Run(name, func(t *testing.T) {
			cuts, _ := Preview(script(t, tt.src), []Entry{PopEntry("A", "", "", "t2")}, ViewOptions{})
			if len(cuts) != 1 || !tt.test(cuts[0]) {
				t.Errorf("cuts = %+v", cuts)
			}
		})
	}
}

// Section 10: "When rows are removed (pop or slide), estimate their tokens
// with the harness's usual estimator, counting only rows the model still
// had (not already removed by an inner pop, not behind the summary), and
// add it to the session's pending estimate". The person is told "popped
// `x`: about 44K tokens out of the model's context".
func TestSpec10_TheEstimateCountsOnlyWhatTheModelStillHad(t *testing.T) {
	// One token a block, so that the size of a list does not depend on how
	// its rows are joined into messages.
	blocks := func(m core.Message) int { return len(m.Blocks) }
	entries := script(t, `
		prompt old work
		call t0:Read
		results t0
		summary @e1 the prompt, and no more      # the first round is still in the view
		@p1 call t1:gnovm_run
		results t1
		push A t1
		call t2:gnovm_run
		results t2
		push A/x t2
		call t3:Read t4:Read
		results t3 t4
		call t5:gnovm_run
		results t5
		@inner pop A/x t5 read both
		call t6:Edit
		results t6
		@a1 call t7:Bash
		results t7
		call t8:Bash
		results t8
		@slide slide A @p1 @a1               # the start of A left to make room
		call t9:gnovm_run
		results t9
		@outer pop A t9 done
	`)
	_, info := mustView(t, entries, ViewOptions{Estimate: blocks})
	inner, slide, outer := cutBy(t, info, "inner"), cutBy(t, info, "slide"), cutBy(t, info, "outer")
	// The inner pop takes one reply of two calls and their two results.
	// The slide takes the three rounds around it, and not what the pop
	// took already. The outer pop takes the two rounds that were left.
	if inner.Tokens != 4 || slide.Tokens != 6 || outer.Tokens != 4 || outer.Between != 12 {
		t.Errorf("the inner pop freed %d, the slide %d, the outer pop %d of %d messages; want 4, 6 and 4 of 12", inner.Tokens, slide.Tokens, outer.Tokens, outer.Between)
	}
	// Together they are what left the list.
	full := slices.Clone(entries)
	for i := range full {
		if full[i].Type == TypeFrame {
			full[i].Type = "not-a-frame"
		}
	}
	_, whole := View(full, ViewOptions{Estimate: blocks})
	if got, want := inner.Tokens+slide.Tokens+outer.Tokens, whole.Tokens-info.Tokens; got != want {
		t.Errorf("the cuts say they freed %d tokens; the list is %d shorter", got, want)
	}
}

// Section 14, settled case 12: "A pop on the round that crosses the
// compaction point: the subtraction keeps it under the point." The round's
// usage report says the conversation is over the point; the estimate of
// what the pop took is there before the next request is built.
func TestSpec14_12_APopOnTheRoundThatCrossesTheCompactionPoint(t *testing.T) {
	const point = 19661 // four fifths of a 24,576 window
	big := strings.Repeat("word ", 2400)
	entries := script(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
	`)
	for _, id := range []string{"r1", "r2", "r3"} {
		entries = append(entries,
			Entry{Type: TypeAssistant, UUID: id + "a", SessionID: "s1", Message: core.Message{Role: core.RoleAssistant, Blocks: []core.Block{core.ToolUse(id, "Read", nil)}}},
			Entry{Type: TypeUser, UUID: id + "r", SessionID: "s1", Source: SourceToolResults, Message: core.Message{Role: core.RoleUser, Blocks: []core.Block{core.ToolResultBlock(id, core.TextResult(big))}}},
		)
	}
	entries = append(entries, named("p-", script(t, `
		call t2:gnovm_run
		usage 19900 80               # the report for the request that made the pop: over the point
	`))...)
	cuts, _ := Preview(entries, []Entry{PopEntry("A", "", "done", "t2")}, ViewOptions{})
	total, pending := 19900+80, cuts[0].Tokens
	if pending < 7000 || total-pending > point {
		t.Errorf("the pop's estimate is %d tokens: %d − %d is not under the point of %d", pending, total, pending, point)
	}
}

// Section 8.7: the large-step line is given "when the session's total has
// grown by more than half the compaction point since the frame was
// pushed". Integration-contract.md 6.2: "A push only appends to the stack
// (with the call's id, and the session's token total at that moment".
// Section 8.2: the total is the "prompt + completion tokens of the last
// usage report".
func TestSpec8_7_TheSessionsTotalWhenAFrameWasEntered(t *testing.T) {
	const src = `
		prompt go
		call t0:gnovm_run
		results t0
		push early t0                # before any usage report
		usage 12000 300
		call t1:gnovm_run
		results t1
		push A t1
		usage 14000 250
		call t2:gnovm_run
		results t2
		push B t2
		usage 30000 900              # a report after the push does not change it
	`
	totals := func(frames []Frame) (out []int) {
		for _, f := range frames {
			out = append(out, f.TotalAtPush)
		}
		return out
	}
	if got, want := totals(Stack(script(t, src), "")), []int{0, 12300, 14250}; !reflect.DeepEqual(got, want) {
		t.Errorf("the totals at the pushes are %v, want %v", got, want)
	}
	// A cached read counts as prompt.
	entries := script(t, `
		prompt go
		call t1:gnovm_run
		results t1
	`)
	entries = append(entries, UsageEntry(core.Usage{InputTokens: 500, CacheReadTokens: 11000, CacheWriteTokens: 300, OutputTokens: 40}, "qwen", 0), PushEntry("A", "", "t1"))
	for i := range entries {
		entries[i].SessionID = "s1"
	}
	if got := Stack(entries, "")[0].TotalAtPush; got != 11840 {
		t.Errorf("the total at the push is %d, want 11840", got)
	}
}

// Section 7: the frame line names the frame the session stands in, its
// title, and "(your stack: `fix-flaky`, then `fix-flaky/find-cause`)" when
// the stack holds more than one frame. "It is computed as the prompt goes
// out ... 'so a prompt queued behind a turn that pushed or popped tells of
// the stack as it then stands'." "After a slide that concerns a frame
// still on the stack, the slide's line (8.6) is appended": "The start of
// `x` left your context", or "Your rounds before `x`" for the bottom frame.
func TestSpec7_TheFrameLineHasItsFacts(t *testing.T) {
	src := `
		prompt fix the flaky test
		@a1 call t1:Read
		results t1
		@a2 call t2:Read
		results t2
		@a3 call t3:gnovm_run
		results t3
		push fix-flaky t3
		@a4 call t4:Read
		results t4
		@a5 call t5:gnovm_run
		results t5
		push fix-flaky/find-cause t5
		@a6 call t6:Read
		results t6
		@a7 call t7:Read
		results t7
	`
	stack := Stack(script(t, src), "")
	type line struct{ id, label string }
	var got []line
	for _, f := range stack {
		got = append(got, line{f.ID, f.Label})
	}
	if want := []line{{"fix-flaky", "label fix-flaky"}, {"fix-flaky/find-cause", "label fix-flaky/find-cause"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("the stack is %+v, want %+v", got, want)
	}
	// Slides, and what each line would name.
	names := func(info ViewInfo) (out []string) {
		for _, c := range info.Cuts {
			switch {
			case c.Kind != OpSlide || !c.Live:
			case c.Frame == "":
				out = append(out, "the rounds before "+c.Before)
			default:
				out = append(out, "the start of "+c.Frame)
			}
		}
		return out
	}
	src += `
		slide - @a1 @a3
		slide fix-flaky @a3 @a5
		slide fix-flaky/find-cause @a5 @a7
	`
	_, info := mustView(t, script(t, src), ViewOptions{})
	if want := []string{"the rounds before fix-flaky", "the start of fix-flaky", "the start of fix-flaky/find-cause"}; !reflect.DeepEqual(names(info), want) {
		t.Errorf("the slides to tell of are %q, want %q", names(info), want)
	}
	// A prompt queued behind a turn that popped tells of the stack as it
	// then stands.
	src += `
		call t8:gnovm_run
		results t8
		pop fix-flaky/find-cause t8 it races
	`
	_, info = mustView(t, script(t, src), ViewOptions{})
	if got := ids(info.Stack); got != "fix-flaky" {
		t.Errorf("the stack is %q", got)
	}
	if want := []string{"the rounds before fix-flaky", "the start of fix-flaky"}; !reflect.DeepEqual(names(info), want) {
		t.Errorf("the slides to tell of are %q, want %q", names(info), want)
	}
}

// Section 8.6: a slide's rows are forgotten as a pop's are ("the reads and
// views in them forgotten"), and the person is told "slid `x`: about N
// tokens of its start out of the model's context". Slide hands back, for
// each cut, the same facts a pop's cut has.
func TestSpec8_6_ASlidesCutHasTheFactsOfAPops(t *testing.T) {
	entries := script(t, `
		prompt go
		@a1 call t1:gnovm_run
		results t1
		push A t1
		@a2 call t2:Read=/w/a.go
		results t2
		@a3 call t3:Bash=cat_b.go
		results t3
		@a4 call t4:Edit=/w/a.go
		results t4
	`)
	slides, cuts := Slide(entries, 1000, ViewOptions{Estimate: ten, Trace: true})
	if len(slides) != 1 || len(cuts) != 1 {
		t.Fatalf("slides = %q", slideRefs(slides))
	}
	c := cuts[0]
	got := calls(c)
	if c.Frame != "A" || c.Tokens != 40 || !c.Live || !c.Planned || len(got) != 2 ||
		got[0].Path != "/w/a.go" || command(got[1]) != "cat b.go" || !reflect.DeepEqual(c.Removed, []string{"a2", "e6", "a3", "e8"}) {
		t.Errorf("cut = %+v", c)
	}
}

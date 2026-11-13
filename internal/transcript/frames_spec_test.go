package transcript

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// The frame rule as spec/gnosh/frames.md states it, section by section.
// Each test builds the transcript a passage describes, in the words of
// script (view_test.go), and asserts the view the passage says results.
// The section is in the test's name; CONFORMANCE.md lists every passage
// beside the test that holds it. Sections 8 (the slide), 12.2 (the prompt
// cache) and what the package hands the loop have files of their own.

const (
	whenPopped = ", removed from your context when you popped it.]"
	toMakeRoom = ", removed from your context to make room.]"
)

// named puts prefix in front of the entries' UUIDs, for a script whose
// entries follow another's in one session.
func named(prefix string, entries []Entry) []Entry {
	for i := range entries {
		entries[i].UUID = prefix + entries[i].UUID
	}
	return entries
}

// ids writes a stack as its frames' ids, bottom to top.
func ids(frames []Frame) string {
	var s []string
	for _, f := range frames {
		s = append(s, f.ID)
	}
	return strings.Join(s, " ")
}

// answerTo returns what the view says the tool call id returned.
func answerTo(t testing.TB, msgs []core.Message, id string) string {
	t.Helper()
	for _, m := range msgs {
		for _, b := range m.Blocks {
			if b.Type == core.BlockToolResult && b.ToolUseID == id {
				return core.TextOf(b.Content)
			}
		}
	}
	t.Fatalf("the view holds no result for %s:\n%s", id, render(msgs))
	return ""
}

// cutBy returns the cut that the frame entry uuid made.
func cutBy(t testing.TB, info ViewInfo, uuid string) Cut {
	t.Helper()
	for _, c := range info.Cuts {
		if c.UUID == uuid {
			return c
		}
	}
	t.Fatalf("no cut by entry %q among %+v", uuid, info.Cuts)
	return Cut{}
}

// says reports whether any message of the view holds text.
func says(msgs []core.Message, text string) bool {
	for _, m := range msgs {
		for _, b := range m.Blocks {
			if strings.Contains(b.Text, text) || strings.Contains(core.TextOf(b.Content), text) {
				return true
			}
		}
	}
	return false
}

// Section 2: "If the new store carries tool results and prompt text in one
// user-role message, treat the results and the text as separate rows for
// this rule." The results go with their round; the words are a prompt of
// their turn.
func TestSpec2_ResultsAndWordsInOneMessageAreSeparateRows(t *testing.T) {
	info := expectView(t, `
		prompt task
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		say done with part one
		prompt part two                    # the turn the pop is made in
		call t3:Read
		mixed t3 | and mind the port       # t3's result, and words the person typed, in one message
		call t4:gnovm_run
		results t4
		pop A t4 done
	`, ViewOptions{Trace: true}, `
		U: task
		A: call(t1)
		U: result(t1)* + part two + and mind the port
		A: call(t4)
		U: result(t4)
	`)
	c := info.Cuts[0]
	if c.Spared != 2 || c.Note != "[Then came the work in it, 2 tool calls (Read 2)"+whenPopped {
		t.Errorf("cut = %+v", c)
	}
	// The message that held both is among the removed and among the spared.
	if !slices.Contains(c.Removed, "e10") || !slices.Contains(c.SparedEntries, "e10") {
		t.Errorf("Removed = %v, SparedEntries = %v", c.Removed, c.SparedEntries)
	}
}

// Section 3.3: "a push appends a frame (with the id of the call that pushed
// it); a pop removes the topmost frame with that id ... The stack is 'the
// pushes without a matching pop, in order'." And 14.8: "Pop then Next in
// one input: both applied in order, two events".
func TestSpec3_3_TheStackIsThePushesWithoutAMatchingPop(t *testing.T) {
	steps := []struct{ lines, stack string }{
		{"prompt go", ""},
		{"call t1:gnovm_run \n results t1 \n push a t1", "a"},
		{"call t2:gnovm_run \n results t2 \n push a/x t2", "a a/x"},
		{"call t3:gnovm_run \n results t3 \n pop a/x t3 done \n push a/y t3", "a a/y"},
		{"call t4:gnovm_run \n results t4 \n pop nowhere t4 what?", "a a/y"},
		{"call t5:gnovm_run \n results t5 \n pop a/y t5 done", "a"},
		{"call t6:Read \n results t6", "a"},
		{"call t7:gnovm_run \n results t7 \n pop a t7 all done", ""},
	}
	src := ""
	for _, s := range steps {
		src += s.lines + "\n"
		entries := script(t, src)
		stack := Stack(entries, "")
		if got := ids(stack); got != s.stack {
			t.Errorf("after %q the stack is %q, want %q", s.lines, got, s.stack)
		}
		// The view agrees, and each frame carries the call that pushed it.
		_, info := mustView(t, entries, ViewOptions{})
		if !reflect.DeepEqual(info.Stack, stack) {
			t.Errorf("after %q the view's stack is %+v, Stack says %+v", s.lines, info.Stack, stack)
		}
		for _, f := range stack {
			if want := map[string]string{"a": "t1", "a/x": "t2", "a/y": "t3"}[f.ID]; f.ToolUseID != want {
				t.Errorf("frame %s was pushed by %q, want %q", f.ID, f.ToolUseID, want)
			}
		}
	}
}

// Section 3.3: "a pop removes the topmost frame with that id". mem never
// lets a pop name a frame below the top, or one frame stand twice on a
// stack; if a transcript says so all the same, the words still hold.
func TestSpec3_3_APopRemovesTheTopmostFrameWithThatID(t *testing.T) {
	t.Run("of two frames with one id, the upper one goes", func(t *testing.T) {
		_, info := mustView(t, script(t, `
			prompt go
			call t1
			results t1
			push a t1
			call t2
			results t2
			push b t2
			call t3
			results t3
			push a t3
			call t4:Read
			results t4
			call t5
			results t5
			@p pop a t5 done
		`), ViewOptions{Trace: true})
		if got := ids(info.Stack); got != "a b" {
			t.Errorf("stack = %q, want a b", got)
		}
		if c := cutBy(t, info, "p"); c.PushCall != "t3" || !reflect.DeepEqual(c.Removed, []string{"e11", "e12"}) {
			t.Errorf("cut = %+v, want the upper frame's range", c)
		}
	})
	t.Run("a pop of a frame below the top leaves the frames above it on the stack", func(t *testing.T) {
		msgs, info := mustView(t, script(t, `
			prompt go
			call t1
			results t1
			push a t1
			call t2
			results t2
			push b t2
			call t3:Read
			results t3
			call t4
			results t4
			@p pop a t4 done
		`), ViewOptions{})
		if got := ids(info.Stack); got != "b" {
			t.Errorf("stack = %q, want b: the pushes without a matching pop", got)
		}
		if c := cutBy(t, info, "p"); !reflect.DeepEqual(c.Above, []string{"b"}) {
			t.Errorf("Above = %v, want b", c.Above)
		}
		if got, want := render(msgs), tidy(`
			U: go
			A: call(t1)
			U: result(t1)*
			A: call(t4)
			U: result(t4)
		`); got != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
		}
	})
	t.Run("a pop whose push round another cut has removed still cuts by the rows", func(t *testing.T) {
		// B's push round went with A's range. B's own pop is read off the
		// rows all the same: everything after B's push round and before
		// B's pop round goes, A's pop round with it (the list is cut by
		// the union of the ranges), and the note has no answer left to
		// stand on.
		msgs, info := mustView(t, script(t, `
			prompt go
			call t1
			results t1
			push a t1
			call t2
			results t2
			push b t2
			call t3:Read
			results t3
			call t4
			results t4
			pop a t4 done
			call t5:Edit
			results t5
			call t6
			results t6
			@p pop b t6 done
		`), ViewOptions{Trace: true})
		if got, want := render(msgs), tidy(`
			U: go
			A: call(t1)
			U: result(t1)*
			A: call(t6)
			U: result(t6)
		`); got != want {
			t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
		}
		c := cutBy(t, info, "p")
		if len(info.Stack) != 0 || c.Messages != 4 || c.Between != 6 || c.PushCall != "t2" {
			t.Errorf("Stack = %+v, cut = %+v", info.Stack, c)
		}
	})
}

// Section 3.3: "A pop naming a frame that is not on the stack records
// nothing." (open-questions.md, section D, has why this reading and not
// the earlier live path's.)
func TestSpec3_3_APopNamingAFrameNotOnTheStackRecordsNothing(t *testing.T) {
	info := expectView(t, `
		prompt go
		call t1:Read
		results t1
		call t2:gnovm_run
		results t2
		@p pop ghost t2 nothing to pop
		say ok
	`, ViewOptions{}, `
		U: go
		A: call(t1)
		U: result(t1)
		A: call(t2)
		U: result(t2)
		A: ok
	`)
	if len(info.Cuts) != 0 || len(info.Stack) != 0 || !reflect.DeepEqual(info.UnmatchedPops, []string{"p"}) {
		t.Errorf("Cuts = %+v, Stack = %+v, UnmatchedPops = %v", info.Cuts, info.Stack, info.UnmatchedPops)
	}
}

// Section 3.3: a pop "records a range (frame id, push call id, pop call
// id)".
func TestSpec3_3_ARangeIsAFrameIDAPushCallIDAndAPopCallID(t *testing.T) {
	_, info := mustView(t, script(t, `
		prompt go
		@push call t1:Read t2:gnovm_run
		results t1 t2
		push fix-flaky t2
		call t3:Edit
		results t3
		@pop call t4:gnovm_run
		results t4
		@p pop fix-flaky t4 fixed
	`), ViewOptions{})
	c := cutBy(t, info, "p")
	if c.Kind != OpPop || c.Frame != "fix-flaky" || c.PushCall != "t2" || c.PopCall != "t4" || c.PushReply != "push" || c.PopReply != "pop" {
		t.Errorf("cut = %+v", c)
	}
	if c.Mark() != "fix-flaky" {
		t.Errorf("Mark = %q", c.Mark())
	}
}

// Section 3.4: "A branched or rewound copy carries the rows, but their
// events are stamped with the old session. So a /branch and the rewind copy
// start at the root: only pushes made under their own session id count."
// Section 12.7: "Each session has its own stack and its own cut."
func TestSpec3_4_OnlyPushesUnderTheSessionsOwnIDCount(t *testing.T) {
	const src = `
		session old
		prompt task
		call t1
		results t1
		push A t1
		call t2:Read
		results t2
		call t3
		results t3
		pop A t3 done                # the cut the copy came with
		call t4
		results t4
		push B t4                    # open when the copy was made
		call t5:Read
		results t5
		session new                  # the copy: every entry from here on is its own
		other
	`
	entries := script(t, src)
	if got := ids(Stack(entries, "")); got != "" {
		t.Errorf("the copy's stack is %q, want the root", got)
	}
	if got := ids(Stack(entries, "new")); got != "" {
		t.Errorf("the copy's stack, asked for by id, is %q", got)
	}
	if got := ids(Stack(entries, "old")); got != "B" {
		t.Errorf("the old session's stack is %q, want B", got)
	}
	// It carries the cut it came with.
	msgs, _ := mustView(t, entries, ViewOptions{})
	if got, want := render(msgs), tidy(`
		U: task
		A: call(t1)
		U: result(t1)*
		A: call(t3)
		U: result(t3)
		A: call(t4)
		U: result(t4)
		A: call(t5)
		U: result(t5)
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}

	// A pop the copy makes of the old session's frame is a pop of a frame
	// that is not on its stack: it records nothing. Its own pushes count.
	entries = script(t, src+`
		call t6
		results t6
		@p pop B t6 done in the copy
		call t7
		results t7
		push C t7
	`)
	_, info := mustView(t, entries, ViewOptions{})
	if got := ids(info.Stack); got != "C" || !reflect.DeepEqual(info.UnmatchedPops, []string{"p"}) || len(info.Cuts) != 1 {
		t.Errorf("stack = %q, UnmatchedPops = %v, Cuts = %+v", got, info.UnmatchedPops, info.Cuts)
	}
	// And the old session, looked at through the copy's entries, still
	// stands in B: nobody else's pop changed its stack.
	if got := ids(Stack(entries, "old")); got != "B" {
		t.Errorf("the old session's stack is now %q", got)
	}
}

// Section 3: "The stack persists across turns and restarts. 'A question to
// the user, Esc, a crash or a new day leave the model inside the frame it
// was in' ... A frame is never auto-popped at a turn's end." Section 12.4:
// "The stack is derived again from the rows". And 14.5.
func TestSpec3_TheStackPersistsAcrossTurnsAndRestarts(t *testing.T) {
	steps := []string{
		"prompt fix the flaky test \n call t1:gnovm_run \n results t1 \n push fix-flaky t1",
		"call t2:Read \n results t2 \n say which port does CI use?", // a question: the turn ends
		"prompt 8080 \n call! t3:Bash",                              // a new turn, stopped with Esc
		"results t3 \n prompt try again",                            // and another
		"call t4:Bash",                                              // gnosh is killed in the middle of the call
	}
	src := ""
	for _, s := range steps {
		src += s + "\n"
		entries := script(t, src)
		if got := ids(Stack(entries, "")); got != "fix-flaky" {
			t.Errorf("after %q the stack is %q", s, got)
		}
		// A restart has only the file.
		ld, err := load(ctx, bytes.NewReader(jsonl(t, entries)), "")
		if err != nil {
			t.Fatal(err)
		}
		if got := ids(Stack(ld.entries, "")); got != "fix-flaky" {
			t.Errorf("after %q and a restart the stack is %q", s, got)
		}
	}
}

// Section 4.1: "The push round stays whole: its assistant row, every other
// call it made, and their results ... The pop round stays whole: the
// assistant row with the pop call, any text beside it, every other call
// that reply made (an edit sent in the same reply as the pop), and their
// results ... everything in replies strictly between goes." Here every
// result is an entry of its own, as a loop that stores each result when its
// call ends writes them. 14.7: "A parallel sibling of the push: its result
// stays in the push's round."
func TestSpec4_1_ThePushRoundAndThePopRoundStayWhole(t *testing.T) {
	msgs, info := mustView(t, script(t, `
		prompt fix it
		tell I'll plan first. | t1:Read=/w/a.go t2:gnovm_run     # a read, and the push beside it
		results t1
		results t2
		push A t2
		call t3:Edit=/w/a.go
		results t3
		say halfway there
		notice keep going                                         # a hidden continuation of this turn
		tell Done, closing. | t4:gnovm_run t5:Edit=/w/b.go        # the pop, words beside it, and an edit after it
		results t4
		@p pop A t4 fixed
		results t5
		say all done
	`), ViewOptions{Trace: true})
	if got, want := render(msgs), tidy(`
		U: fix it
		A: I'll plan first. + call(t1) + call(t2)
		U: result(t1) + result(t2)* + keep going
		A: Done, closing. + call(t4) + call(t5)
		U: result(t4) + result(t5)
		A: all done
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}
	c := cutBy(t, info, "p")
	if !reflect.DeepEqual(c.Removed, []string{"e6", "e7", "e8"}) || c.Spared != 1 || c.Messages != 3 {
		t.Errorf("cut = %+v", c)
	}
}

// Section 4.1: between the two rounds go "the model's rounds, its replies
// and questions, reasoning". A reply that was cancelled or failed is one of
// them, and the calls it never got an answer to go with it: nothing is
// left behind that a provider would refuse.
func TestSpec4_1_ACancelledOrFailedReplyInsideTheFrameGoesWithTheRest(t *testing.T) {
	info := expectView(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call! t2:Bash t3:Read        # stopped: t3 never ran
		results t2
		prompt carry on
		call? t4:Edit                # the provider failed in the middle of it
		say! and then
		nothing?
		call t5:gnovm_run
		results t5
		pop A t5 done
	`, ViewOptions{}, `
		U: go
		A: call(t1)
		U: result(t1)* + carry on
		A: call(t5)
		U: result(t5)
	`)
	if len(info.Repairs) != 0 {
		t.Errorf("the view needed repairs: %+v", info.Repairs)
	}
	if c := info.Cuts[0]; c.Note != "[Then came the work in it, 3 tool calls (Bash 1, Edit 1, Read 1)"+whenPopped {
		t.Errorf("cut = %+v", c)
	}
}

// Section 4.1, step 1: "Find the assistant row holding the pop call. If it
// is not among the rows, nothing is removed." The pop happened all the
// same, so the frame is off the stack.
func TestSpec4_1_Step1_ThePopCallIsNotAmongTheRows(t *testing.T) {
	info := expectView(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		call t3:gnovm_run
		results t3
		pop A lost done              # no reply holds a call named "lost"
	`, ViewOptions{}, `
		U: go
		A: call(t1)
		U: result(t1)
		A: call(t2)
		U: result(t2)
		A: call(t3)
		U: result(t3)
	`)
	if c := info.Cuts[0]; len(info.Stack) != 0 || c.PopReply != "" || c.Messages != 0 || c.Between != 0 || c.Note != "" || c.Reason == "" {
		t.Errorf("Stack = %+v, cut = %+v", info.Stack, c)
	}
}

// Section 4.1, step 2: "Find the assistant row holding the push call. If it
// is the same row (pushed and popped in one input), nothing is removed."
func TestSpec4_1_Step2_PushedAndPoppedInOneInput(t *testing.T) {
	for name, frames := range map[string]string{
		"by one call":               "push A t2 \n pop A t2 trivial",
		"by two calls of one reply": "push A t2 \n pop A t3 trivial",
	} {
		t.Run(name, func(t *testing.T) {
			info := expectView(t, `
				prompt go
				call t1:Read
				results t1
				call t2:gnovm_run t3:gnovm_run
				results t2 t3
				`+frames+`
				say ok
			`, ViewOptions{}, `
				U: go
				A: call(t1)
				U: result(t1)
				A: call(t2) + call(t3)
				U: result(t2) + result(t3)
				A: ok
			`)
			if c := info.Cuts[0]; len(info.Stack) != 0 || c.PushReply != c.PopReply || c.PushReply == "" || c.Messages != 0 || c.Note != "" {
				t.Errorf("Stack = %+v, cut = %+v", info.Stack, c)
			}
		})
	}
}

// Section 4.1, step 3: "The range starts after the last result row that
// answers any call of the push's assistant row, and ends before the pop's
// assistant row." Section 11: "a spared prompt then sits directly after the
// push's results ... send them as one user-side message, results first."
func TestSpec4_1_Step3_TheRangeStartsAfterTheLastResultOfThePushRound(t *testing.T) {
	msgs, info := mustView(t, script(t, `
		prompt go
		call t1:gnovm_run t2:Read
		results t1
		hook a hook's words between the two results     # before the round's last result: not in the range
		results t2
		push A t1
		hook a hook's words after the round             # in the range, and of an earlier turn
		call t3:Bash
		results t3
		say what now?
		prompt carry on                                 # the turn the pop is made in
		call t4:gnovm_run
		results t4
		@p pop A t4 done
	`), ViewOptions{Trace: true})
	if got, want := render(msgs), tidy(`
		U: go
		A: call(t1) + call(t2)
		U: result(t1)* + result(t2) + a hook's words between the two results + carry on
		A: call(t4)
		U: result(t4)
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}
	if c := cutBy(t, info, "p"); !reflect.DeepEqual(c.Removed, []string{"e7", "e8", "e9", "e10"}) || !reflect.DeepEqual(c.SparedEntries, []string{"e11"}) {
		t.Errorf("cut = %+v", c)
	}
}

// Section 4.1, step 4: "A summary row inside the range is never removed.
// After a summary row, the rows that continue the summary stay: removal
// resumes at the model's first own (assistant) row after it ... 'A pop
// whose push is older than the session's summary ... removes everything
// after the summary message; the summary and its continuation prompt
// stay.' (The rows between that push and the summary are flagged with the
// rest. The model was not being sent them, so they free nothing and are
// not counted.)" And 14.6.
func TestSpec4_1_Step4_ASummaryInsideTheRange(t *testing.T) {
	entries := script(t, `
		prompt task
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		@s summary the story so far
		notice carry on from the summary     # the summary's continuation
		hook what mem holds                  # and what the session is given back after it
		call t3:Edit
		results t3
		say which file next?
		prompt the other one
		call t4:gnovm_run
		results t4
		@p pop A t4 done
	`)
	msgs, info := mustView(t, entries, ViewOptions{Trace: true})
	if got, want := render(msgs), tidy(`
		U: summary(the story so far) + carry on from the summary + what mem holds + the other one
		A: call(t4)
		U: result(t4)
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}
	c := cutBy(t, info, "p")
	// Flagged with the rest: the two rows between the push and the summary.
	if want := []string{"e5", "e6", "e10", "e11", "e12"}; !reflect.DeepEqual(c.Removed, want) {
		t.Errorf("Removed = %v, want %v", c.Removed, want)
	}
	// Counted: only the three the model still had.
	tokens := 0
	for _, e := range entries[9:12] {
		tokens += EstimateTokens(e.Message)
	}
	if c.Messages != 3 || c.Tokens != tokens || c.Spared != 1 {
		t.Errorf("cut = %+v, want 3 messages and %d tokens", c, tokens)
	}
	if slices.Contains(c.Removed, "s") {
		t.Errorf("the summary is among the removed: %v", c.Removed)
	}
}

// Section 4.1, step 5: "If the push call is not known, or its row is not
// among the rows, the range starts after the last summary row before the
// pop, or at the first row when there is none; either way removal begins at
// the first assistant row from there, as in step 4. Such a range carries no
// note."
//
// The reading taken here is narrower, and CONFORMANCE.md says why. The
// spec's store knew a push only by its call, so a push whose row was gone
// had no place at all. This transcript writes a push as an entry of its
// own, so the frame's start is known even then, and the rule itself ("every
// row after the frame's push round") is kept: what came before the frame
// was entered stays. Read to the letter, the fallback would also take the
// first task's rounds here, and with them the results of every step
// finished before this one.
func TestSpec4_1_Step5_ThePushIsNotAmongTheRows(t *testing.T) {
	t.Run("the cut starts at the first reply after the push entry, and leaves no note", func(t *testing.T) {
		info := expectView(t, `
			prompt first task
			call t1:Read
			results t1
			say the file is fine
			prompt now the second task
			push A lost                  # the reply that made the push call is not in the conversation
			call t2:Edit
			results t2
			call t3:gnovm_run
			results t3
			pop A t3 done
		`, ViewOptions{Trace: true}, `
			U: first task
			A: call(t1)
			U: result(t1)
			A: the file is fine
			U: now the second task
			A: call(t3)
			U: result(t3)
		`)
		c := info.Cuts[0]
		if c.PushReply != "" || c.PushCall != "lost" || c.Note != "" || !reflect.DeepEqual(c.Removed, []string{"e7", "e8"}) {
			t.Errorf("cut = %+v", c)
		}
	})
	t.Run("a push that names no call is the same", func(t *testing.T) {
		expectView(t, `
			prompt task
			call t1:Read
			results t1
			push A -
			call t2:Edit
			results t2
			call t3:gnovm_run
			results t3
			pop A t3 done
		`, ViewOptions{}, `
			U: task
			A: call(t1)
			U: result(t1)
			A: call(t3)
			U: result(t3)
		`)
	})
	t.Run("with a summary between, removal begins at the first reply after it", func(t *testing.T) {
		expectView(t, `
			prompt task
			push A lost
			call t1:Read
			results t1
			summary so far
			notice carry on
			call t2:Edit
			results t2
			call t3:gnovm_run
			results t3
			pop A t3 done
		`, ViewOptions{}, `
			U: summary(so far) + carry on
			A: call(t3)
			U: result(t3)
		`)
	})
}

// Section 4.1: "Nothing is rewritten and nothing synthetic is inserted in
// place of the removed stretch, with one exception: a short note appended
// to the push's answer."
func TestSpec4_1_NothingIsRewrittenAndNothingSyntheticIsInserted(t *testing.T) {
	entries := script(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		call t3:gnovm_run
		results t3
		pop A t3 done
		say all done
	`)
	msgs, info := mustView(t, entries, ViewOptions{Trace: true})
	for i, src := range info.Sources {
		if len(src) == 0 {
			t.Errorf("message %d of the view comes from no entry: %+v", i, msgs[i])
		}
	}
	// Every block of the view is a stored block, but for the one line.
	stored := map[string]bool{}
	for _, e := range entries {
		for _, b := range e.Message.Blocks {
			stored[asJSON([]core.Message{{Blocks: []core.Block{b}}})] = true
		}
	}
	changed := 0
	for _, m := range msgs {
		for _, b := range m.Blocks {
			if !stored[asJSON([]core.Message{{Blocks: []core.Block{b}}})] {
				changed++
				if b.ToolUseID != "t1" || core.TextOf(b.Content) != "out t1\n[Then came the work in it, 1 tool call (Read 1)"+whenPopped {
					t.Errorf("a block that is not a stored one: %+v", b)
				}
			}
		}
	}
	if changed != 1 {
		t.Errorf("%d blocks of the view differ from the stored ones, want the push's answer alone", changed)
	}
}

// Section 4.2: "A pop spares the prompt rows of the turn it is made in: the
// prompt the model is answering, prompts folded into that turn, and its
// hidden continuations and reminders. What the user said in earlier turns
// while the model stood in the frame is cut". The scenario is QA finding
// A18's: a frame entered in an earlier turn spans the prompt of this one,
// and a model left with the earlier turn's "do only (1) now" planned the
// finished work again.
func TestSpec4_2_APopSparesThePromptsOfItsOwnTurn(t *testing.T) {
	const src = `
		prompt do only (1) now
		call t1:gnovm_run                       # the plan: (1) and (2)
		results t1
		push renames t1
		call t2:gnovm_run
		results t2
		push renames/one t2
		call t3:Edit
		results t3
		call t4:gnovm_run                       # pop (1), step into (2)
		results t4
		pop renames/one t4 the flag is --shout now
		push renames/two t4
		say (1) is done; say when to go on.     # the turn ends with the model inside (2)
		prompt now do (2)                       # the question this turn answers
		call t5:Edit
		results t5
		call t6:gnovm_run
		results t6
		pop renames/two t6 the docs say --shout
	`
	expectView(t, src, ViewOptions{}, `
		U: do only (1) now
		A: call(t1)
		U: result(t1)
		A: call(t2)
		U: result(t2)*
		A: call(t4)
		U: result(t4)* + now do (2)
		A: call(t6)
		U: result(t6)
	`)
	// The task's own pop, later in the same turn, spares it too.
	msgs, _ := mustView(t, script(t, src+`
		call t7:gnovm_run
		results t7
		pop renames t7 both renamed
	`), ViewOptions{})
	if got, want := render(msgs), tidy(`
		U: do only (1) now
		A: call(t1)
		U: result(t1)* + now do (2)
		A: call(t7)
		U: result(t7)
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}
	if says(msgs, "(1) is done") {
		t.Errorf("the model's words of the earlier turn are still in the view")
	}
}

// Section 4.3, each clause in turn. In every case the frame A is entered
// in the first turn and popped by the last reply; what the view keeps
// after the push's answer is what the rule spares.
func TestSpec4_3_WhereATurnStarts(t *testing.T) {
	const head = `
		prompt task
		call t1
		results t1
		push A t1
	`
	const pop = `
		call t9
		results t9
		pop A t9 ok
	`
	view := func(spared string) string {
		return "U: task\nA: call(t1)\nU: result(t1)*" + spared + "\nA: call(t9)\nU: result(t9)"
	}
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// "That run starts a turn if what precedes it is nothing (the
			// start of the conversation)".
			name: "nothing precedes the run: the first turn",
			src: head + `
				call t2:Read
				results t2
				notice a reminder
			` + pop,
			want: view(" + a reminder"),
		},
		{
			// "or a summary row".
			name: "a summary row precedes the run",
			src: `
				prompt first
				say answer
				summary earlier work
				prompt second task               # follows the summary: starts the turn
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				prompt and also this             # follows results: folded into that turn
			` + pop,
			want: "U: summary(earlier work) + second task\nA: call(t1)\nU: result(t1)* + and also this\nA: call(t9)\nU: result(t9)",
		},
		{
			// "or an assistant row with no tool call (the model's answer to
			// the turn before)".
			name: "an answer precedes the run",
			src: head + `
				call t2:Read
				results t2
				say which port?
				prompt 8080                      # an earlier turn: cut
				call t3
				results t3
				say that part is done
				prompt now the rest              # the pop's turn
			` + pop,
			want: view(" + now the rest"),
		},
		{
			// "or anything else, when the nearest assistant row before the
			// run ended cancelled or failed (result rows may lie between it
			// and the run): 'A cancelled turn ends where it stopped. The
			// next prompt starts a new turn, so the cancelled turn's prompt
			// is not spared.'"
			name: "the reply before the run was cancelled, with results between",
			src: head + `
				say what next?
				prompt do part one
				call! t2
				results t2
				prompt do part two
			` + pop,
			want: view(" + do part two"),
		},
		{
			name: "the reply before the run failed",
			src: head + `
				say what next?
				prompt do part one
				call? t2
				results t2
				prompt do part two
			` + pop,
			want: view(" + do part two"),
		},
		{
			// A reply stopped before anything of it arrived holds nothing,
			// so the model is never sent it; it is still the reply at which
			// the turn stopped.
			name: "the reply before the run was cancelled before it said anything",
			src: head + `
				say what next?
				prompt do part one
				nothing!
				prompt do part two
			` + pop,
			want: view(" + do part two"),
		},
		{
			name: "the reply before the run failed before it said anything",
			src: head + `
				say what next?
				prompt do part one
				nothing?
				prompt do part two
			` + pop,
			want: view(" + do part two"),
		},
		{
			// A reply that holds nothing and was not cut off is no answer
			// to the turn before: the model is asked again and the turn
			// goes on.
			name: "an empty reply that was not cut off does not end the turn",
			src: head + `
				say what next?
				prompt do part one
				nothing
				notice you sent nothing: go on
				prompt do part two
			` + pop,
			want: view(" + do part one + you sent nothing: go on + do part two"),
		},
		{
			// "Otherwise that prompt was folded into a running turn: keep
			// scanning back."
			name: "a prompt that follows tool results was folded in",
			src: head + `
				call t2:Read
				results t2
				prompt also do this
			` + pop,
			want: view(" + also do this"),
		},
		{
			name: "the scan goes back past a folded prompt to the turn's start",
			src: head + `
				say what next?
				prompt do part one               # starts the turn: it follows an answer
				call t2
				results t2
				prompt do part two               # folded into it
			` + pop,
			want: view(" + do part one + do part two"),
		},
		{
			// "If no such run is found, the turn starts at the first row."
			name: "no prompt the person typed at all",
			src: `
				notice a job's output
				call t1
				results t1
				push A t1
				call t2:Read
				results t2
				notice another notice
			` + pop,
			want: "U: a job's output\nA: call(t1)\nU: result(t1)* + another notice\nA: call(t9)\nU: result(t9)",
		},
		{
			// "Scan back from the pop's row for a visible (not hidden)
			// prompt row": a hidden continuation after an answer starts no
			// turn, and is spared as part of the one that is running.
			name: "a hidden continuation after an answer starts no turn",
			src: head + `
				call t2:Read
				results t2
				say is it done?
				notice the goal is not met: keep going
			` + pop,
			want: view(" + the goal is not met: keep going"),
		},
		{
			// "Take the whole run of consecutive prompt rows that ends at
			// it."
			name: "the run takes in the hidden rows around the typed prompt",
			src: head + `
				call t2:Read
				results t2
				say ok so far
				notice the background job ended
				prompt next part
				hook you are in A
			` + pop,
			want: view(" + the background job ended + next part + you are in A"),
		},
		{
			name: "an earlier turn's hidden rows go with it",
			src: head + `
				say what next?
				prompt part one
				hook you are in A (part one)
				call t2:Read
				results t2
				say part one is done
				prompt part two
				hook you are in A (part two)
			` + pop,
			want: view(" + part two + you are in A (part two)"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expectView(t, tt.src, ViewOptions{}, tt.want)
		})
	}
}

// Section 4.4: the line, its counts and when there is none.
func TestSpec4_4_TheNote(t *testing.T) {
	t.Run("the tool calls of the removed replies, by name, sorted, each with its count", func(t *testing.T) {
		msgs, _ := mustView(t, script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:Read t3:Read
			results t2 t3
			call t4:Bash
			results t4
			call t5:Edit t6:Bash
			results t5 t6
			call t7:Read t8:Bash
			results t7 t8
			call t9:gnovm_run
			results t9
			pop A t9 done
		`), ViewOptions{})
		// The spec's own example, in this harness's tool names.
		if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came the work in it, 7 tool calls (Bash 3, Edit 1, Read 3), removed from your context when you popped it.]"; got != want {
			t.Errorf("the push's answer reads %q, want %q", got, want)
		}
	})
	t.Run("one call reads 1 tool call", func(t *testing.T) {
		msgs, _ := mustView(t, script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:Read
			results t2
			call t3:gnovm_run
			results t3
			pop A t3 done
		`), ViewOptions{})
		if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came the work in it, 1 tool call (Read 1), removed from your context when you popped it.]"; got != want {
			t.Errorf("the push's answer reads %q, want %q", got, want)
		}
	})
	t.Run("with no tool call among the removed rows it counts messages, the spared ones left out", func(t *testing.T) {
		msgs, info := mustView(t, script(t, `
			prompt explain the cache
			call t1:gnovm_run
			results t1
			push A t1
			say here is how it works
			prompt and the eviction?
			say it evicts the oldest
			prompt thanks, close it
			call t2:gnovm_run
			results t2
			pop A t2 explained
		`), ViewOptions{})
		if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came 3 messages of work in it, removed from your context when you popped it.]"; got != want {
			t.Errorf("the push's answer reads %q, want %q", got, want)
		}
		if c := info.Cuts[0]; c.Spared != 1 || c.Between != 3 {
			t.Errorf("cut = %+v", c)
		}
	})
	t.Run("when every row of the range was spared there is no note", func(t *testing.T) {
		info := expectView(t, `
			prompt task
			call t1:gnovm_run
			results t1
			push A t1
			prompt and close it right away
			call t2:gnovm_run
			results t2
			pop A t2 nothing to do
		`, ViewOptions{}, `
			U: task
			A: call(t1)
			U: result(t1) + and close it right away
			A: call(t2)
			U: result(t2)
		`)
		if c := info.Cuts[0]; c.Note != "" || c.Between != 0 || c.Spared != 1 {
			t.Errorf("cut = %+v", c)
		}
	})
	t.Run("rows an inner pop had removed already are counted with the rest", func(t *testing.T) {
		msgs, info := mustView(t, script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:gnovm_run
			results t2
			push A/x t2
			call t3:Read t4:Read
			results t3 t4
			call t5:gnovm_run
			results t5
			pop A/x t5 read both
			call t6:Edit
			results t6
			call t7:gnovm_run
			results t7
			pop A t7 done
		`), ViewOptions{})
		if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came the work in it, 5 tool calls (Edit 1, Read 2, gnovm_run 2), removed from your context when you popped it.]"; got != want {
			t.Errorf("the push's answer reads %q, want %q", got, want)
		}
		// The inner pop took two messages; the outer one takes the other
		// six, and tells of all eight.
		if in, out := info.Cuts[0], info.Cuts[1]; in.Messages != 2 || out.Messages != 6 || out.Between != 8 {
			t.Errorf("Cuts = %+v", info.Cuts)
		}
	})
	t.Run("rows a slide had removed already are counted with the rest", func(t *testing.T) {
		msgs, _ := mustView(t, script(t, `
			prompt go
			@a1 call t1:gnovm_run
			results t1
			push A t1
			@a2 call t2:Read
			results t2
			@a3 call t3:Read
			results t3
			slide A @a1 @a3              # the first round of A left to make room
			call t4:Edit
			results t4
			call t5:gnovm_run
			results t5
			pop A t5 done
		`), ViewOptions{})
		if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came the work in it, 3 tool calls (Edit 1, Read 2), removed from your context when you popped it.]"; got != want {
			t.Errorf("the push's answer reads %q, want %q", got, want)
		}
	})
	t.Run("it changes only the model-facing copy: the stored row is not edited", func(t *testing.T) {
		entries := script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:Read
			results t2
			call t3:gnovm_run
			results t3
			pop A t3 done
		`)
		mustView(t, entries, ViewOptions{})
		if got := core.TextOf(entries[2].Message.Blocks[0].Content); got != "out t1" {
			t.Errorf("the stored result now reads %q", got)
		}
	})
}

// Section 4.4: "It is said in every request after the pop: the pop's own
// turn, every later turn, and a summary's request alike. So what one
// request sent, the next sends again". And 14.18. A summary's request is
// the same list with the request for a summary after it, so its part of
// the test is that the list does not depend on who asks.
func TestSpec4_4_TheNoteIsSaidInEveryRequestAfterThePop(t *testing.T) {
	src := `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		call t3:gnovm_run
		results t3
		pop A t3 done
	`
	const want = "out t1\n[Then came the work in it, 1 tool call (Read 1), removed from your context when you popped it.]"
	var first []core.Message
	for i, more := range []string{
		"",                                    // the request right after the pop
		"call t4:Bash \n results t4",          // later in the pop's own turn
		"say all done \n prompt next thing",   // a later turn
		"call t5:Read \n results t5",          // and on
		"notice write a summary of the above", // a summary's request
	} {
		src += more + "\n"
		msgs, _ := mustView(t, script(t, src), ViewOptions{})
		if got := answerTo(t, msgs, "t1"); got != want {
			t.Errorf("request %d: the push's answer reads %q", i, got)
		}
		if i == 0 {
			first = msgs
			continue
		}
		// What the first request after the pop sent, each later one sends
		// again, byte for byte.
		if len(msgs) < len(first) || asJSON(msgs[:len(first)-1]) != asJSON(first[:len(first)-1]) {
			t.Errorf("request %d does not begin with the request before it:\n%s", i, render(msgs))
		}
	}
}

// Section 4.4: "One answer carries one note. When several ranges begin at
// the same push (a slide, then the pop), the pop's note wins over a
// slide's; between two of the same kind, the one whose range reaches
// furthest."
func TestSpec4_4_OneAnswerCarriesOneNote(t *testing.T) {
	notes := func(s string) int { return strings.Count(s, "[Then came") }
	t.Run("a slide, then the pop: the pop's", func(t *testing.T) {
		src := `
			prompt go
			@a1 call t1:gnovm_run
			results t1
			push A t1
			@a2 call t2:Read
			results t2
			@a3 call t3:Read
			results t3
			slide A @a1 @a3
		`
		msgs, _ := mustView(t, script(t, src), ViewOptions{})
		if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came the work in it, 1 tool call (Read 1)"+toMakeRoom; got != want {
			t.Errorf("after the slide the answer reads %q, want %q", got, want)
		}
		msgs, _ = mustView(t, script(t, src+`
			call t4:gnovm_run
			results t4
			pop A t4 done
		`), ViewOptions{})
		got := answerTo(t, msgs, "t1")
		if want := "out t1\n[Then came the work in it, 2 tool calls (Read 2)" + whenPopped; got != want || notes(got) != 1 {
			t.Errorf("after the pop the answer reads %q, want %q", got, want)
		}
	})
	t.Run("two pops that begin at one push: the one that reaches furthest", func(t *testing.T) {
		src := `
			prompt go
			call t1:gnovm_run
			results t1
			push plan t1                 # entered on the way
			push plan/first t1           # to the todo itself: two frames, one call
			call t2:Read
			results t2
			call t3:gnovm_run
			results t3
			pop plan/first t3 read it
		`
		msgs, _ := mustView(t, script(t, src), ViewOptions{})
		if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came the work in it, 1 tool call (Read 1)"+whenPopped; got != want {
			t.Errorf("after the inner pop the answer reads %q, want %q", got, want)
		}
		msgs, _ = mustView(t, script(t, src+`
			call t4:Edit
			results t4
			call t5:gnovm_run
			results t5
			pop plan t5 all done
		`), ViewOptions{})
		got := answerTo(t, msgs, "t1")
		if want := "out t1\n[Then came the work in it, 3 tool calls (Edit 1, Read 1, gnovm_run 1)" + whenPopped; got != want || notes(got) != 1 {
			t.Errorf("after the outer pop the answer reads %q, want %q", got, want)
		}
	})
	t.Run("two slides that begin at one push: the one that reaches furthest", func(t *testing.T) {
		msgs, _ := mustView(t, script(t, `
			prompt go
			@a1 call t1:gnovm_run
			results t1
			push A t1
			@a2 call t2:Read
			results t2
			@a3 call t3:Read
			results t3
			slide A @a1 @a3
			@a4 call t4:Edit
			results t4
			@a5 call t5:Bash
			results t5
			slide A @a1 @a5
		`), ViewOptions{})
		got := answerTo(t, msgs, "t1")
		if want := "out t1\n[Then came the work in it, 3 tool calls (Edit 1, Read 2)" + toMakeRoom; got != want || notes(got) != 1 {
			t.Errorf("the answer reads %q, want %q", got, want)
		}
	})
}

// Section 4.5: "Stacks nest. An inner pop removes its own range at once; an
// outer pop later removes what is left between its push and its pop, the
// inner push and pop rounds included, so a finished goal leaves its push
// and its pop. The model-facing list is cut by the union of all ranges."
func TestSpec4_5_NestedFramesAreCutByTheUnionOfTheirRanges(t *testing.T) {
	steps := []struct{ lines, want string }{
		{`
			prompt go
			call t1 \n results t1 \n push a t1
			call t2 \n results t2 \n push a/b t2
			call t3 \n results t3 \n push a/b/c t3
			call t4:Read \n results t4
		`, `
			U: go
			A: call(t1) \n U: result(t1)
			A: call(t2) \n U: result(t2)
			A: call(t3) \n U: result(t3)
			A: call(t4) \n U: result(t4)
		`},
		{`
			call t5 \n results t5 \n pop a/b/c t5 c done
			call t6:Edit \n results t6
		`, `
			U: go
			A: call(t1) \n U: result(t1)
			A: call(t2) \n U: result(t2)
			A: call(t3) \n U: result(t3)*
			A: call(t5) \n U: result(t5)
			A: call(t6) \n U: result(t6)
		`},
		{`
			call t7 \n results t7 \n pop a/b t7 b done
			call t8:Bash \n results t8
		`, `
			U: go
			A: call(t1) \n U: result(t1)
			A: call(t2) \n U: result(t2)*
			A: call(t7) \n U: result(t7)
			A: call(t8) \n U: result(t8)
		`},
		{`
			call t9 \n results t9 \n pop a t9 a done
		`, `
			U: go
			A: call(t1) \n U: result(t1)*
			A: call(t9) \n U: result(t9)
		`},
	}
	lines := func(s string) string { return strings.ReplaceAll(s, `\n`, "\n") }
	src := ""
	for i, s := range steps {
		src += lines(s.lines)
		msgs, _ := mustView(t, script(t, src), ViewOptions{})
		if got, want := render(msgs), tidy(lines(s.want)); got != want {
			t.Errorf("step %d, view:\n%s\n--- want ---\n%s", i, got, want)
		}
	}
}

// Section 4.6: Done and Drop of the present frame, and Leave, are pops: "a
// drop's result reads 'dropped: why'", a leave's "left open: why". The
// harness is handed a pop event for each and cuts the same way. And 14.9.
func TestSpec4_6_OtherSpellingsThatPop(t *testing.T) {
	var views []string
	for _, result := range []string{"the port is 8080", "dropped: not needed after all", "left open: for tomorrow"} {
		msgs, info := mustView(t, script(t, `
			prompt go
			call t1:gnovm_run
			results t1
			push A t1
			call t2:Read
			results t2
			call t3:gnovm_run
			results t3
			pop A t3 `+result), ViewOptions{})
		views = append(views, render(msgs))
		if len(info.Stack) != 0 || info.Cuts[0].Result != result || info.Cuts[0].Messages != 2 {
			t.Errorf("%q: Stack = %+v, Cuts = %+v", result, info.Stack, info.Cuts)
		}
	}
	if views[0] != views[1] || views[0] != views[2] || views[0] != tidy(`
		U: go
		A: call(t1)
		U: result(t1)*
		A: call(t3)
		U: result(t3)
	`) {
		t.Errorf("the three views differ, or are not the cut one:\n%s", strings.Join(views, "\n---\n"))
	}
}

// Section 4.7: "Reopen of a popped frame does not bring its rows back, and
// undo of the entry that popped it leaves them removed." Neither writes a
// frame event, so nothing un-cuts; a frame entered again under the same id
// is a new frame with a range of its own.
func TestSpec4_7_ReopenAndUndoDoNotBringRowsBack(t *testing.T) {
	info := expectView(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		call t3:gnovm_run
		results t3
		pop A t3 done
		call t4:gnovm_run            # mem.Reopen("A", "one more thing"): no frame event
		results t4
		call t5:gnovm_run            # mem.Frame("A").Push()
		results t5
		push A t5
		call t6:Edit
		results t6
	`, ViewOptions{}, `
		U: go
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
	`)
	if got := ids(info.Stack); got != "A" || info.Stack[0].ToolUseID != "t5" {
		t.Errorf("Stack = %+v, want the new A, pushed by t5", info.Stack)
	}
}

// Section 4.8: "A refused or failed input returns no events, so nothing is
// cut." With no frame entry, a call that says Pop changes nothing.
func TestSpec4_8_ACallThatWasNotKeptCutsNothing(t *testing.T) {
	info := expectView(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		call t3:gnovm_run=mem.Pop("done")
		fail t3                      # mem refused it: no event
	`, ViewOptions{}, `
		U: go
		A: call(t1)
		U: result(t1)
		A: call(t2)
		U: result(t2)
		A: call(t3)
		U: result(t3)
	`)
	if got := ids(info.Stack); got != "A" || len(info.Cuts) != 0 {
		t.Errorf("Stack = %q, Cuts = %+v", got, info.Cuts)
	}
}

// Section 11, "The flag", and integration-contract.md 6.4: "One marker per
// stored row: empty, a frame id, or a slide mark slide:<frame>:<push call
// id>:<pop call id>. Set by row id. Copied with the row. Never set on a
// summary row." Section 8.6 says which calls a slide mark names: "the
// first call of the anchor round's assistant row (the frame's push call
// for a frame)" and "the first call of the row the cut ends before (the
// next frame's push call for a whole body)".
//
// Nothing in a transcript is rewritten, so the marker is not stored on the
// rows: a pop or slide entry is the record, and the marker of each row is
// read off it (Cut.Mark, Cut.Removed).
func TestSpec11_TheStoredMarker(t *testing.T) {
	entries := script(t, `
		prompt go
		@a1 call t1:Read
		results t1
		@a2 call t2:Read
		results t2
		@a3 call t9:Read t3:gnovm_run    # the push of A is the second call of its reply
		results t9 t3
		push A t3
		@a4 call t4:Edit
		results t4
		@a5 call t5:Edit
		results t5
		@a6 call t8:Read t6:gnovm_run    # and so is the push of B
		results t8 t6
		push B t6
		@a7 call t7:Bash
		results t7
		@a8 call t10:Bash
		results t10
		@a9 call t11:Bash
		results t11
	`)
	slides, cuts := Slide(entries, 1000, ViewOptions{Estimate: ten, Trace: true})
	if got, want := slideRefs(slides), []string{"- a1 a3", "A a3 a6", "B a6 a9"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("slides = %q, want %q", got, want)
	}
	var marks []string
	for _, c := range cuts {
		marks = append(marks, c.Mark())
	}
	// The rounds before the first frame: no frame, the first call of the
	// anchor's reply, the first call of the reply the cut ends before. A
	// frame's body: the frame, its own push call, the next frame's push
	// call. The top frame's start: its push call, the first call of the
	// reply the cut ends before.
	if want := []string{"slide::t1:t9", "slide:A:t3:t6", "slide:B:t6:t11"}; !reflect.DeepEqual(marks, want) {
		t.Errorf("marks = %q, want %q", marks, want)
	}

	// Recorded, and with B popped, the record derived again from the file
	// gives every row the same marker.
	recorded := record(entries, slides)
	recorded = append(recorded, script(t, `
		@a10 call t12:gnovm_run
		results t12
		@p pop B t12 done
		@s summary @a1 the first round
	`)...)
	ld, err := load(ctx, bytes.NewReader(jsonl(t, recorded)), "")
	if err != nil {
		t.Fatal(err)
	}
	_, info := mustView(t, ld.entries, ViewOptions{Trace: true})
	got := map[string]string{}
	for _, c := range info.Cuts {
		for _, uuid := range c.Removed {
			got[uuid] = c.Mark() // the cut made last gives the row its marker
		}
	}
	want := map[string]string{
		"a2": "slide::t1:t9", "e5": "slide::t1:t9",
		"a4": "slide:A:t3:t6", "e10": "slide:A:t3:t6", "a5": "slide:A:t3:t6", "e12": "slide:A:t3:t6",
		"a7": "B", "e17": "B", "a8": "B", "e19": "B", "a9": "B", "e21": "B",
	}
	if _, marked := got["s"]; marked {
		t.Errorf("the summary carries a marker")
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("markers = %v\nwant      %v", got, want)
	}
}

// Section 11: "Model-facing readers skip flagged rows ... Human-facing
// readers keep them: the chat, export, the rewind scan of changed files,
// erasure scans. The goal judge, though a model, reads the full list". And
// section 12.10: "Nothing is deleted: you still see all of it, the
// transcript keeps it".
func TestSpec11_ModelFacingReadersSkipHumanFacingReadersKeep(t *testing.T) {
	entries := script(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Edit=/w/a.go
		results t2
		call t3:gnovm_run
		results t3
		pop A t3 done
	`)
	before := jsonl(t, entries)
	_, info := mustView(t, entries, ViewOptions{Trace: true})
	model := set(info.Sources)
	if model["e5"] || model["e6"] {
		t.Errorf("the model's list holds the popped rows: %v", info.Sources)
	}
	var human []string
	for _, e := range Conversation(entries) {
		human = append(human, e.UUID)
	}
	if want := []string{"e1", "e2", "e3", "e4", "e5", "e6", "e7", "e8", "e9"}; !reflect.DeepEqual(human, want) {
		t.Errorf("the full list is %v, want %v", human, want)
	}
	if !bytes.Equal(before, jsonl(t, entries)) {
		t.Errorf("computing the view changed the transcript")
	}
}

// Section 11: "Applied even on Esc ... 'Esc after a call kept its pop or
// push must not lose them.'" The reply that made the call is recorded as
// cancelled; its frame events count all the same. And 14.19.
func TestSpec11_AppliedEvenOnEsc(t *testing.T) {
	info := expectView(t, `
		prompt go
		call! t1:gnovm_run           # Esc, after mem had kept the push
		results t1
		push A t1
		prompt carry on
		call t2:Read
		results t2
		call! t3:gnovm_run           # and again after it had kept the pop
		results t3
		pop A t3 done
	`, ViewOptions{}, `
		U: go
		A: call(t1)
		U: result(t1)* + carry on
		A: call(t3)
		U: result(t3)
	`)
	if len(info.Stack) != 0 || info.Cuts[0].Messages != 2 {
		t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
	}
}

// Section 12.1: "The stack survives a summary" and "Rows at and before the
// last summary are neither counted nor cut by a later pop or slide (they
// were not in the context)."
func TestSpec12_1_SummariesAndTheStack(t *testing.T) {
	const src = `
		prompt task
		@a1 call t1:gnovm_run
		results t1
		push A t1
		@a2 call t2:Read
		results t2
		@a3 call t3:Read
		results t3
		summary the story so far
		notice carry on
		@a4 call t4:Edit
		results t4
		@a5 call t5:Edit
		results t5
		@a6 call t6:Edit
		results t6
	`
	entries := script(t, src)
	msgs, info := mustView(t, entries, ViewOptions{Estimate: ten})
	if got := ids(info.Stack); got != "A" {
		t.Errorf("after the summary the stack is %q, want A", got)
	}
	if got, want := render(msgs), tidy(`
		U: summary(the story so far) + carry on
		A: call(t4)
		U: result(t4)
		A: call(t5)
		U: result(t5)
		A: call(t6)
		U: result(t6)
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}
	// The frame's push is behind the summary, so nothing of it can slide.
	if slides, _ := Slide(entries, 1000, ViewOptions{Estimate: ten}); len(slides) != 0 {
		t.Errorf("slid %q with the frame's push behind the summary", slideRefs(slides))
	}
	// A pop counts what it takes of the six rows after the summary's
	// continuation, and nothing of the four behind it.
	_, info = mustView(t, script(t, src+`
		call t7:gnovm_run
		results t7
		@p pop A t7 done
	`), ViewOptions{Estimate: ten})
	if c := cutBy(t, info, "p"); c.Messages != 6 || c.Tokens != 60 || c.Between != 10 {
		t.Errorf("cut = %+v, want 6 messages and 60 tokens of the 10 between", c)
	}
}

// Section 12.3: "A sub-agent is a session of its own with its own (empty)
// stack ... A sub-agent given the tool pushes and pops in its own
// conversation".
func TestSpec12_3_ASubAgentHasAStackOfItsOwn(t *testing.T) {
	st, _ := newStore(t)
	parent, _ := st.Create(cwd)
	mustAppend(t, parent, script(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push plan t1
		call t2:Agent
	`)...)
	sub, _ := st.Create(cwd)
	if got := ids(sub.Stack()); got != "" {
		t.Errorf("a new sub-agent's stack is %q", got)
	}
	mustAppend(t, sub, script(t, `
		prompt look into the parser
		call s1:gnovm_run
		results s1
		push plan/parser s1
		call s2:Read
		results s2
		call s3:gnovm_run
		results s3
		pop plan/parser s3 it is fine
		call s4:gnovm_run
		results s4
		push plan/lexer s4
	`)...)
	if got := ids(sub.Stack()); got != "plan/lexer" {
		t.Errorf("the sub-agent's stack is %q, want plan/lexer", got)
	}
	if got := ids(parent.Stack()); got != "plan" {
		t.Errorf("the parent's stack is %q, want plan", got)
	}
	msgs, _ := sub.View(ViewOptions{})
	if got, want := render(msgs), tidy(`
		U: look into the parser
		A: call(s1)
		U: result(s1)*
		A: call(s3)
		U: result(s3)
		A: call(s4)
		U: result(s4)
	`); got != want {
		t.Errorf("the sub-agent's view:\n%s\n--- want ---\n%s", got, want)
	}
}

// Section 12.4: "The stack is derived again from the rows; the model stands
// where it stood ... Flagged rows stay out of the model-facing list." A
// session closed and opened again has only its file.
func TestSpec12_4_AResumedSessionStandsWhereItStood(t *testing.T) {
	st, _ := newStore(t)
	s, _ := st.Create(cwd)
	mustAppend(t, s, script(t, `
		prompt go
		usage 9000 100
		call t1:gnovm_run
		results t1
		push plan t1
		call t2:gnovm_run
		results t2
		push plan/one t2
		call t3:Read
		results t3
		call t4:gnovm_run
		results t4
		pop plan/one t4 done
		push plan/two t4
		call t5:Edit
		results t5
		call t6:Edit
		results t6
		call t7:Edit
		results t7
	`)...)
	// A slide too, recorded as a session records it.
	slides, _ := Slide(s.Entries(), 15, ViewOptions{Estimate: ten, Session: s.ID()})
	if len(slides) == 0 {
		t.Fatal("nothing slid")
	}
	if _, err := s.AppendAll(slides...); err != nil {
		t.Fatal(err)
	}
	msgs, info := s.View(ViewOptions{Trace: true})
	stack := s.Stack()
	s.Close()

	again, err := st.Open(ctx, s.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	msgs2, info2 := again.View(ViewOptions{Trace: true})
	if asJSON(msgs) != asJSON(msgs2) || !reflect.DeepEqual(info, info2) {
		t.Errorf("the reopened session's view:\n%s\n--- it was ---\n%s", render(msgs2), render(msgs))
	}
	if got := again.Stack(); !reflect.DeepEqual(got, stack) || ids(got) != "plan plan/two" {
		t.Errorf("the reopened session's stack is %+v, was %+v", got, stack)
	}
	if !info2.Stack[0].Slid || info2.Stack[1].Slid || info2.Stack[0].TotalAtPush != 9100 {
		t.Errorf("Stack = %+v, want plan slid, and entered at 9100 tokens", info2.Stack)
	}
	if set(info2.Sources)["e9"] {
		t.Errorf("a popped row is back in the view after the restart")
	}
}

// Section 12.5: "A branch, and a rewind copy, start at the root; they carry
// the flags." And 14.10. Here the branch is the store's own.
func TestSpec12_5_ABranchCarriesTheCutsAndStartsAtTheRoot(t *testing.T) {
	st, _ := newStore(t)
	s, _ := st.Create(cwd)
	mustAppend(t, s, script(t, `
		prompt task
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		call t3:gnovm_run
		results t3
		pop A t3 done
		call t4:gnovm_run
		results t4
		push B t4
		call t5:Read
		results t5
	`)...)
	b, err := st.Branch(s, "")
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	const carried = `
		U: task
		A: call(t1)
		U: result(t1)*
		A: call(t3)
		U: result(t3)
		A: call(t4)
		U: result(t4)
		A: call(t5)
		U: result(t5)
	`
	msgs, info := b.View(ViewOptions{})
	if got := render(msgs); got != tidy(carried) || len(info.Stack) != 0 || len(b.Stack()) != 0 {
		t.Errorf("the branch's view:\n%s\nits stack: %+v", got, info.Stack)
	}
	if got := ids(s.Stack()); got != "B" {
		t.Errorf("the session branched from now stands in %q", got)
	}
	// The branch's own frames are its own; the old session's B is not its
	// to pop.
	mustAppend(t, b, named("b-", script(t, `
		call t6:gnovm_run
		results t6
		pop B t6 not mine
		call t7:gnovm_run
		results t7
		push C t7
		call t8:Edit
		results t8
		call t9:gnovm_run
		results t9
		pop C t9 done in the branch
	`))...)
	msgs, info = b.View(ViewOptions{})
	if got, want := render(msgs), tidy(carried+`
		A: call(t6)
		U: result(t6)
		A: call(t7)
		U: result(t7)*
		A: call(t9)
		U: result(t9)
	`); got != want || len(info.Stack) != 0 || len(info.UnmatchedPops) != 1 {
		t.Errorf("the branch's view:\n%s\n--- want ---\n%s\ninfo: %+v", got, want, info)
	}
	if got := ids(s.Stack()); got != "B" {
		t.Errorf("the session branched from now stands in %q", got)
	}
}

// Section 12.5: "Rewind itself is unchanged: its points are the user's
// prompts; one inside a popped range is still a point; after a rewind the
// stack is derived again from what remains." And 14.10: "a rewind into a
// popped range derives the stack from what remains."
//
// A rewind here is an entry that takes what came after its target off the
// conversation, the pop's entry with it. So the frame is open again and
// the rows it had removed are the frame's work again, as they were at
// that point (CONFORMANCE.md has the passage and this reading).
func TestSpec12_5_ARewind(t *testing.T) {
	const src = `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		@ask say which port?
		@q prompt 8080
		call t3:gnovm_run
		results t3
		pop A t3 done
		call t4:Bash
		@after results t4
		prompt and the next thing
	`
	t.Run("to a point after the pop: the cut stays", func(t *testing.T) {
		info := expectView(t, src+"rewind @after", ViewOptions{}, `
			U: go
			A: call(t1)
			U: result(t1)* + 8080
			A: call(t3)
			U: result(t3)
			A: call(t4)
			U: result(t4)
		`)
		if len(info.Stack) != 0 {
			t.Errorf("Stack = %+v", info.Stack)
		}
	})
	t.Run("into the popped range: the frame is on the stack again", func(t *testing.T) {
		info := expectView(t, src+"rewind @ask", ViewOptions{}, `
			U: go
			A: call(t1)
			U: result(t1)
			A: call(t2)
			U: result(t2)
			A: which port?
		`)
		if got := ids(info.Stack); got != "A" || len(info.Cuts) != 0 {
			t.Errorf("Stack = %q, Cuts = %+v", got, info.Cuts)
		}
	})
	t.Run("to before the push: neither the frame nor its cut is left", func(t *testing.T) {
		info := expectView(t, strings.Replace(src, "prompt go", "@start prompt go", 1)+"rewind @start", ViewOptions{}, `
			U: go
		`)
		if len(info.Stack) != 0 || len(info.Cuts) != 0 {
			t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
		}
	})
}

// Section 12.6: "After a rewind into a popped range, or an undo, the
// session can stand in a frame mem holds closed. Then Pop, Done or Leave on
// it takes it off the stack with no change in mem ... The harness applies
// that pop event like any other."
func TestSpec12_6_AFrameMemAlreadyHoldsClosed(t *testing.T) {
	info := expectView(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:Read
		results t2
		@ask say which port?
		prompt 8080
		call t3:gnovm_run
		results t3
		pop A t3 done
		rewind @ask                  # back into the popped range: mem still holds A done
		prompt 9090, as it turns out
		call t4:gnovm_run            # mem.Pop: "A was already done: it is off your stack now"
		results t4
		pop A t4 done
	`, ViewOptions{}, `
		U: go
		A: call(t1)
		U: result(t1)* + 9090, as it turns out
		A: call(t4)
		U: result(t4)
	`)
	if len(info.Stack) != 0 || len(info.Cuts) != 1 || info.Cuts[0].Messages != 3 {
		t.Errorf("Stack = %+v, Cuts = %+v", info.Stack, info.Cuts)
	}
}

// Section 13.1, the walkthrough the docs show: the stack after each call,
// and "rename-code is done; its work leaves the context".
func TestSpec13_1_TheWalkthrough(t *testing.T) {
	steps := []struct{ lines, stack string }{
		{`prompt rename the project to gnosh
		  call c1:gnovm_run=mem.Todo("rename_the_project").Push()
		  results c1
		  push rename-project c1`, "rename-project"},
		{`call c2:gnovm_run=mem.Todo("rename_it_in_the_code")
		  results c2`, "rename-project"},
		{`call c3:gnovm_run=mem.Todo("rename_it_in_the_tests")
		  results c3`, "rename-project"},
		{`call c4:gnovm_run=mem.Next()
		  results c4
		  push rename-project/rename-code c4`, "rename-project rename-project/rename-code"},
		{`call c5:Edit=/w/go.mod
		  results c5
		  call c6:Edit=/w/main.go
		  results c6`, "rename-project rename-project/rename-code"},
		{`call c7:gnovm_run=mem.Pop("go.mod_and_main.go_say_gnosh")
		  results c7
		  pop rename-project/rename-code c7 go.mod and main.go say gnosh`, "rename-project"},
		{`call c8:gnovm_run=mem.Next()
		  results c8
		  push rename-project/rename-tests c8`, "rename-project rename-project/rename-tests"},
	}
	src := ""
	for _, s := range steps {
		src += s.lines + "\n"
		if got := ids(Stack(script(t, src), "")); got != s.stack {
			t.Errorf("after\n%s\nthe stack is %q, want %q", s.lines, got, s.stack)
		}
	}
	expectView(t, src, ViewOptions{}, `
		U: rename the project to gnosh
		A: call(c1)
		U: result(c1)
		A: call(c2)
		U: result(c2)
		A: call(c3)
		U: result(c3)
		A: call(c4)
		U: result(c4)*
		A: call(c7)
		U: result(c7)
		A: call(c8)
		U: result(c8)
	`)
}

// Section 13.2, the list before and after a pop.
//
// The example is the design's, from before two later changes, and differs
// from the rule as it stands in two places (CONFORMANCE.md records both):
// its "before" list shows no note on the answer that entered
// fix-flaky/reproduce, though that frame is popped and the note "is said
// in every request after the pop" (4.4); and its "after" list shows no
// prompt of the later turn, though a pop "spares the prompt rows of the
// turn it is made in" (4.2). The rule is taken over the picture in both.
func TestSpec13_2_TheListBeforeAndAfterAPop(t *testing.T) {
	const before = `
		prompt the listener test is flaky: fix it
		call c1:gnovm_run                # Todo(...).Push()
		results c1
		push fix-flaky c1
		call c2:gnovm_run                # Next()
		results c2
		push fix-flaky/reproduce c2
		call c3:Bash                     # the work of reproduce
		results c3
		call c4:gnovm_run                # Pop + Next
		results c4
		pop fix-flaky/reproduce c4 it fails one run in twelve
		push fix-flaky/find-cause c4
		call c5:Read                     # view: 900 lines
		results c5
		say which port?
		prompt 8080
		call c6:Bash                     # bash: 12 runs
		results c6
	`
	// Inside fix-flaky/find-cause, fix-flaky/reproduce already popped.
	msgs, info := mustView(t, script(t, before), ViewOptions{})
	if got, want := render(msgs), tidy(`
		U: the listener test is flaky: fix it
		A: call(c1)
		U: result(c1)
		A: call(c2)
		U: result(c2)*
		A: call(c4)
		U: result(c4)
		A: call(c5)
		U: result(c5)
		A: which port?
		U: 8080
		A: call(c6)
		U: result(c6)
	`); got != want {
		t.Errorf("before, view:\n%s\n--- want ---\n%s", got, want)
	}
	if got := ids(info.Stack); got != "fix-flaky fix-flaky/find-cause" {
		t.Errorf("before, the stack is %q", got)
	}

	// After mem.Pop("…") and mem.Next() in a later turn.
	msgs, info = mustView(t, script(t, before+`
		say it is the shared port
		prompt go on
		call c7:gnovm_run                # Pop + Next
		results c7
		pop fix-flaky/find-cause c7 it races on the shared port
		push fix-flaky/rerun c7
		say carrying on
	`), ViewOptions{})
	if got, want := render(msgs), tidy(`
		U: the listener test is flaky: fix it
		A: call(c1)
		U: result(c1)
		A: call(c2)
		U: result(c2)*
		A: call(c4)
		U: result(c4)* + go on
		A: call(c7)
		U: result(c7)
		A: carrying on
	`); got != want {
		t.Errorf("after, view:\n%s\n--- want ---\n%s", got, want)
	}
	if got, want := answerTo(t, msgs, "c2"), "out c2\n[Then came the work in it, 1 tool call (Bash 1)"+whenPopped; got != want {
		t.Errorf("the answer that entered reproduce reads %q", got)
	}
	if got, want := answerTo(t, msgs, "c4"), "out c4\n[Then came the work in it, 2 tool calls (Bash 1, Read 1)"+whenPopped; got != want {
		t.Errorf("the answer that entered find-cause reads %q", got)
	}
	// "The '8080' is gone from the model's context with the question".
	if says(msgs, "8080") || says(msgs, "which port?") {
		t.Errorf("the question or its answer is still in the view")
	}
	if got := ids(info.Stack); got != "fix-flaky fix-flaky/rerun" {
		t.Errorf("after, the stack is %q", got)
	}

	// "Had '8080' been said in the same turn as the pop, it would stay."
	msgs, _ = mustView(t, script(t, before+`
		call c7:gnovm_run
		results c7
		pop fix-flaky/find-cause c7 it races on the shared port
		push fix-flaky/rerun c7
	`), ViewOptions{})
	if got, want := render(msgs), tidy(`
		U: the listener test is flaky: fix it
		A: call(c1)
		U: result(c1)
		A: call(c2)
		U: result(c2)*
		A: call(c4)
		U: result(c4)* + 8080
		A: call(c7)
		U: result(c7)
	`); got != want {
		t.Errorf("popped in the turn of 8080, view:\n%s\n--- want ---\n%s", got, want)
	}
}

// Section 13.4: "Eight functions, one todo each, thinking off: 'the eight
// pops freed about 11.6K tokens, and the session's last request was ~17K
// tokens where ~29K would have piled up.'" The run is rebuilt to scale: a
// fixed part of about 14K tokens stands in for the system prompt and the
// tools, and each todo's work is about 1,450 tokens.
func TestSpec13_4_EightPops(t *testing.T) {
	text := func(tokens int) string { return strings.Repeat("word ", tokens*4/5) }
	var entries []Entry
	add := func(e Entry) {
		e.UUID = fmt.Sprintf("n%d", len(entries))
		e.SessionID = "s"
		entries = append(entries, e)
	}
	round := func(id, tool string, tokens int) {
		add(AssistantEntry(core.Message{Blocks: []core.Block{core.ToolUse(id, tool, nil)}}, core.StopToolUse))
		add(UserEntry(core.Message{Blocks: []core.Block{core.ToolResultBlock(id, core.TextResult(text(tokens)))}}, SourceToolResults))
	}
	add(PromptEntry(text(14200)))
	round("plan", "gnovm_run", 200)
	add(PushEntry("eight-functions", "", "plan"))
	for i := range 8 {
		n := string(rune('1' + i))
		round("next"+n, "gnovm_run", 240) // mem's reply to stepping in: 210 to 270 tokens
		add(PushEntry("eight-functions/f"+n, "", "next"+n))
		round("read"+n, "Read", 680)
		round("edit"+n, "Edit", 340)
		round("test"+n, "Bash", 360)
		round("pop"+n, "gnovm_run", 80) // the pop's line: about 80 tokens
		add(PopEntry("eight-functions/f"+n, "", "done", "pop"+n))
	}
	msgs, info := mustView(t, entries, ViewOptions{})
	freed := 0
	for _, c := range info.Cuts {
		if c.Messages != 6 {
			t.Errorf("cut = %+v, want the three rounds of work", c)
		}
		freed += c.Tokens
	}
	piled := 0
	for _, e := range entries {
		if e.IsMessage() {
			piled += EstimateTokens(e.Message)
		}
	}
	t.Logf("the eight pops freed %d tokens; the last request is %d tokens where %d would have piled up", freed, info.Tokens, piled)
	near := func(got, want int) bool { return got > want*95/100 && got < want*105/100 }
	if len(info.Cuts) != 8 || !near(freed, 11600) || !near(info.Tokens, 17400) || !near(piled, 29000) {
		t.Errorf("freed %d, last request %d, piled up %d; want about 11,600, 17,400 and 29,000", freed, info.Tokens, piled)
	}
	// What is left of each todo is its push round and its pop round.
	if want := 1 + 2 + 8*4; len(msgs) != want {
		t.Errorf("the view has %d messages, want %d", len(msgs), want)
	}
}

// Section 14, settled cases 1 and 2. "Trampoline: a task with three todos,
// entered and popped in turn; after each pop the list is the pushes, the
// pops and the present todo's work; the last Next is the kept no-op whose
// text says to pop the task." "Nested: pop the task after its todos; its
// push and its pop remain."
func TestSpec14_1_TheTrampolineAndTheTask(t *testing.T) {
	const head = "U: rename the project\nA: call(t1)\nU: result(t1)\nA: call(t2)\n"
	steps := []struct{ lines, want, stack string }{
		{`
			prompt rename the project
			call t1:gnovm_run            # the task and its three todos, entered
			results t1
			push rename t1
			call t2:gnovm_run            # mem.Next()
			results t2
			push rename/code t2
			call t3:Edit                 # the present todo's work
			results t3
		`, head + "U: result(t2)\nA: call(t3)\nU: result(t3)", "rename rename/code"},
		{`
			call t4:gnovm_run            # mem.Pop("…"); mem.Next()
			results t4
			pop rename/code t4 code renamed
			push rename/tests t4
			call t5:Edit
			results t5
		`, head + "U: result(t2)*\nA: call(t4)\nU: result(t4)\nA: call(t5)\nU: result(t5)", "rename rename/tests"},
		{`
			call t6:gnovm_run            # mem.Pop("…"); mem.Next()
			results t6
			pop rename/tests t6 tests renamed
			push rename/docs t6
			call t7:Edit
			results t7
		`, head + "U: result(t2)*\nA: call(t4)\nU: result(t4)*\nA: call(t6)\nU: result(t6)\nA: call(t7)\nU: result(t7)", "rename rename/docs"},
		{`
			call t8:gnovm_run            # mem.Pop("…"); mem.Next(): nothing to step into, a kept no-op
			results t8
			pop rename/docs t8 docs renamed
		`, head + "U: result(t2)*\nA: call(t4)\nU: result(t4)*\nA: call(t6)\nU: result(t6)*\nA: call(t8)\nU: result(t8)", "rename"},
		{`
			call t9:gnovm_run            # mem.Pop("the project is renamed")
			results t9
			pop rename t9 the project is renamed
		`, "U: rename the project\nA: call(t1)\nU: result(t1)*\nA: call(t9)\nU: result(t9)", ""},
	}
	src := ""
	var msgs []core.Message
	for i, s := range steps {
		src += s.lines
		var info ViewInfo
		msgs, info = mustView(t, script(t, src), ViewOptions{})
		if got := render(msgs); got != s.want {
			t.Errorf("step %d, view:\n%s\n--- want ---\n%s", i, got, s.want)
		}
		if got := ids(info.Stack); got != s.stack {
			t.Errorf("step %d, the stack is %q, want %q", i, got, s.stack)
		}
	}
	if got, want := answerTo(t, msgs, "t1"), "out t1\n[Then came the work in it, 7 tool calls (Edit 3, gnovm_run 4)"+whenPopped; got != want {
		t.Errorf("the task's push answer reads %q, want %q", got, want)
	}
}

// Section 14, settled case 3: "A side quest: a new todo pushed inside a
// todo, popped, carry on." Section 7's line asks for it: "Other work the
// user asks for meanwhile goes in a frame of its own".
func TestSpec14_3_ASideQuest(t *testing.T) {
	info := expectView(t, `
		prompt fix the flaky test
		call t1:gnovm_run
		results t1
		push fix-flaky t1
		call t2:Read
		results t2
		prompt while you are there, rename the port flag     # folded into the turn
		call t3:gnovm_run                                    # mem.Todo("rename the port flag").Push()
		results t3
		push fix-flaky/rename-port-flag t3
		call t4:Edit
		results t4
		call t5:gnovm_run
		results t5
		pop fix-flaky/rename-port-flag t5 it is --port now
		call t6:Bash                                         # carry on with the flaky test
		results t6
	`, ViewOptions{}, `
		U: fix the flaky test
		A: call(t1)
		U: result(t1)
		A: call(t2)
		U: result(t2) + while you are there, rename the port flag
		A: call(t3)
		U: result(t3)*
		A: call(t5)
		U: result(t5)
		A: call(t6)
		U: result(t6)
	`)
	if got := ids(info.Stack); got != "fix-flaky" {
		t.Errorf("the stack is %q, want fix-flaky", got)
	}
}

// Section 14, settled case 4: "Words inside a frame across turns: the model
// asks, the turn ends, the user answers, a prompt is folded in, a notice
// arrives; the stack is the same across the turns; the pop removes earlier
// turns' words and spares this turn's".
func TestSpec14_4_WordsInsideAFrameAcrossTurns(t *testing.T) {
	steps := []string{
		`prompt fix the flaky test
		 call t1:gnovm_run
		 results t1
		 push fix-flaky t1
		 call t2:Read
		 results t2
		 say which port does CI use?`, // the model asks; the turn ends
		`prompt 8080
		 call t3:Bash
		 results t3
		 prompt and keep the old name
		 call t4:gnovm_run=mem.Think("port",_"the_user_says_the_shared_port_is_8080")
		 results t4
		 say done with the port; anything else?`, // the user answers, a prompt is folded in; the turn ends
		`notice the background test run ended
		 prompt no, close it`, // a notice arrives, and the last turn begins
	}
	src := ""
	for _, s := range steps {
		src += s + "\n"
		if got := ids(Stack(script(t, src), "")); got != "fix-flaky" {
			t.Errorf("the stack is %q after\n%s", got, s)
		}
	}
	msgs, info := mustView(t, script(t, src+`
		call t5:gnovm_run
		results t5
		pop fix-flaky t5 it binds port 0 now
	`), ViewOptions{})
	if got, want := render(msgs), tidy(`
		U: fix the flaky test
		A: call(t1)
		U: result(t1)* + the background test run ended + no, close it
		A: call(t5)
		U: result(t5)
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}
	if c := info.Cuts[0]; c.Spared != 2 || c.Note != "[Then came the work in it, 3 tool calls (Bash 1, Read 1, gnovm_run 1)"+whenPopped {
		t.Errorf("cut = %+v", c)
	}
}

// Section 14, known and left: "'A spared count can disagree with the list
// until the next run', when the turn has ... a row a slide cut." Here the
// count is of what the pop left the model with, so it agrees with the
// list: a prompt of the turn that a slide took earlier stays taken, and is
// not counted as spared.
func TestSpec14_Known_ASparedCountAgreesWithTheList(t *testing.T) {
	msgs, info := mustView(t, script(t, `
		prompt go
		@a1 call t1:gnovm_run
		results t1
		push A t1
		@a2 call t2:Read
		results t2
		notice an early reminder             # of this turn, and inside the slide's range
		@a3 call t3:Read
		results t3
		slide A @a1 @a3
		notice a later reminder
		@a4 call t4:gnovm_run
		results t4
		@p pop A t4 done
	`), ViewOptions{Trace: true})
	if got, want := render(msgs), tidy(`
		U: go
		A: call(t1)
		U: result(t1)* + a later reminder
		A: call(t4)
		U: result(t4)
	`); got != want {
		t.Errorf("view:\n%s\n--- want ---\n%s", got, want)
	}
	if c := cutBy(t, info, "p"); c.Spared != 1 || !reflect.DeepEqual(c.SparedEntries, []string{"e11"}) {
		t.Errorf("cut = %+v, want one prompt spared", c)
	}
}

// Section 14, settled case 13: "No round between push and pop: the 'nothing
// came between' text, nothing flagged, nothing forgotten."
func TestSpec14_13_NoRoundBetweenPushAndPop(t *testing.T) {
	info := expectView(t, `
		prompt go
		call t1:gnovm_run
		results t1
		push A t1
		call t2:gnovm_run
		results t2
		pop A t2 it was done already
	`, ViewOptions{Trace: true}, `
		U: go
		A: call(t1)
		U: result(t1)
		A: call(t2)
		U: result(t2)
	`)
	c := info.Cuts[0]
	if c.Between != 0 || c.Messages != 0 || c.Tokens != 0 || c.Spared != 0 || c.Note != "" || len(c.Removed) != 0 || len(c.Replies) != 0 {
		t.Errorf("cut = %+v, want nothing between, nothing removed", c)
	}
}

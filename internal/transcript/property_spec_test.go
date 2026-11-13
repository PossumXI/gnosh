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

// What spec/gnosh/frames.md requires of every session, checked over random
// transcripts with pushes, pops, slides, summaries, rewinds, branches,
// cancelled and failed replies and several tool calls a reply
// (randomTranscript, property_test.go), one entry at a time. Each property
// is stated in the spec's words where it is checked.

// traced computes a view with its sources and what each cut took.
func traced(entries []Entry) ([]core.Message, ViewInfo) {
	return View(entries, ViewOptions{Trace: true})
}

// uncut returns the entries with their frame entries turned into entries
// of a type the view does not know: the same conversation, rewinds and
// summary and all, with nothing popped and nothing slid.
func uncut(entries []Entry) []Entry {
	out := slices.Clone(entries)
	for i := range out {
		if out[i].Type == TypeFrame {
			out[i].Type = "not-a-frame"
		}
	}
	return out
}

// promptOf reports whether the user entry holds anything but tool
// results: a prompt row.
func promptOf(e *Entry) bool {
	return e.Type == TypeUser && slices.ContainsFunc(e.Message.Blocks, func(b core.Block) bool { return b.Type != core.BlockToolResult })
}

// storedAnswer returns what the conversation holds as the result of the
// tool call id made by the reply with the UUID reply.
func storedAnswer(entries []Entry, reply, id string) (string, bool) {
	entries = Conversation(entries)
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].UUID != reply {
			continue
		}
		for _, e := range entries[i+1:] {
			if e.Type == TypeAssistant && len(e.Message.Blocks) > 0 {
				break
			}
			for _, b := range e.Message.Blocks {
				if e.Type == TypeUser && b.Type == core.BlockToolResult && b.ToolUseID == id {
					return core.TextOf(b.Content), true
				}
			}
		}
		break
	}
	return "", false
}

// resultFor returns the text of the result for the tool call id in m.
func resultFor(m core.Message, id string) (text string, at int) {
	for i, b := range m.Blocks {
		if b.Type == core.BlockToolResult && b.ToolUseID == id {
			return core.TextOf(b.Content), i
		}
	}
	return "", -1
}

// asBlock writes one block of a message of the given role as the bytes a
// request would carry, with the role in front.
func asBlock(role core.Role, b core.Block) string {
	return string(role) + " " + asJSON([]core.Message{{Blocks: []core.Block{b}}})
}

// flat lays a view out as its blocks, in order, and says at which block
// each message starts.
func flat(msgs []core.Message) (blocks []string, starts []int) {
	for _, m := range msgs {
		starts = append(starts, len(blocks))
		for _, b := range m.Blocks {
			blocks = append(blocks, asBlock(m.Role, b))
		}
	}
	return blocks, starts
}

// specCounts says how often each property found something to check.
type specCounts struct {
	appended, pushes, pops, slides, notes, spared, previews, early, rounds int
}

func TestSpecProperties(t *testing.T) {
	seeds := uint64(250)
	if testing.Short() || raceEnabled {
		seeds = 50
	}
	var n specCounts
	for seed := range seeds {
		checkSession(t, seed, &n)
	}
	t.Logf("checked: %+v", n)
	if s := int(seeds); n.appended < 50*s || n.pushes < 2*s || n.pops < 2*s || n.slides < s/2 || n.notes < 2*s || n.spared < s || n.previews < 3*s || n.early < 2*s || n.rounds < s*4/5 {
		t.Errorf("the random transcripts did not reach what the properties are about: %+v", n)
	}
}

// FuzzSpecProperties holds the same over whatever sessions the fuzzer's
// seeds make.
func FuzzSpecProperties(f *testing.F) {
	for seed := range uint64(4) {
		f.Add(1000 + seed)
	}
	f.Fuzz(func(t *testing.T, seed uint64) {
		var n specCounts
		checkSession(t, seed, &n)
	})
}

// checkSession makes the random session of a seed and checks everything
// the spec requires of it.
func checkSession(t *testing.T, seed uint64, n *specCounts) {
	t.Helper()
	entries, r := randomTranscript(seed)
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("seed %d: %s", seed, fmt.Sprintf(format, args...))
	}

	// "Check(View(...)) holds; the view is deterministic" (mustView),
	// and it is the view the spec's words give (the oracle, in
	// mustView too).
	msgs, info := mustView(t, entries, ViewOptions{Trace: true})

	// Section 12.4: "The stack is derived again from the rows ...
	// Flagged rows stay out of the model-facing list." No entry is
	// ever needed that is not in the file: a session loaded from its
	// file has the view and the stack it had.
	ld, err := load(ctx, bytes.NewReader(jsonl(t, entries)), "")
	if err != nil {
		fail("loading: %v", err)
	}
	again, info2 := traced(ld.entries)
	if asJSON(again) != asJSON(msgs) || !reflect.DeepEqual(info, info2) {
		fail("after a reload the view is\n%s\n--- it was ---\n%s", render(again), render(msgs))
	}

	// Section 4.1: "a call and its result are never on opposite sides
	// of a cut", and section 11: "no tool call is split from its
	// result". Whatever had to be mended in the view (a result made
	// up for a call that has none, a result dropped that answers no
	// call) had to be mended with nothing cut as well.
	_, whole := traced(uncut(entries))
	need := func(info ViewInfo) map[string]int {
		m := map[string]int{}
		for _, r := range info.Repairs {
			m[string(r.Kind)+" "+r.ToolUseID]++
		}
		return m
	}
	was := need(whole)
	budget := ViewOptions{Budget: 1 + r.IntN(max(info.Tokens, 1)), Trace: true}
	_, tight := mustView(t, entries, budget)
	for name, v := range map[string]ViewInfo{"view": info, "slid view": tight} {
		for k, c := range need(v) {
			if c > was[k] {
				fail("the %s needed the repair %q %d times, and %d with nothing cut: a cut parted a call from its result", name, k, c, was[k])
			}
		}
	}

	// "Nothing in the open top frame's latest round is ever removed":
	// the newest round of the conversation, which is the newest round
	// of the frame the session stands in, is in every view that has
	// not been summarized past it, popped, slid or not.
	alive := Conversation(entries)
	in := set(whole.Sources)
	for p := len(alive) - 1; p >= 0; p-- {
		e := &alive[p]
		if e.Type != TypeAssistant || len(e.Message.ToolUses()) == 0 {
			continue
		}
		var calls []string
		for _, b := range e.Message.ToolUses() {
			calls = append(calls, b.ID)
		}
		for name, v := range map[string]ViewInfo{"view": info, "slid view": tight} {
			have := set(v.Sources)
			if in[e.UUID] && !have[e.UUID] {
				fail("the %s lost %q, the reply of the newest round", name, e.UUID)
			}
			for q := p + 1; q < len(alive) && alive[q].Type != TypeAssistant; q++ {
				if u := &alive[q]; u.Type == TypeUser && answersAny(u, calls) && in[u.UUID] && !have[u.UUID] {
					fail("the %s lost %q, a result of the newest round", name, u.UUID)
				}
			}
		}
		n.rounds++
		break
	}

	// The rest looks at the session one entry at a time: the view
	// before entry i was written, and the view after.
	before, info0 := traced(nil)
	for i := range entries {
		after, info1 := traced(entries[:i+1])
		checkStep(fail, n, entries, i, before, info0, after, info1)
		before, info0 = after, info1
	}
}

// checkStep checks what writing entry i did to the view.
func checkStep(fail func(string, ...any), n *specCounts, entries []Entry, i int, before []core.Message, info0 ViewInfo, after []core.Message, info1 ViewInfo) {
	e := &entries[i]
	switch {
	case e.Type == TypeUser, e.Type == TypeAssistant, e.Type == TypeTitle, e.Type == TypeMeta, e.Type == TypeUsage, e.Type == "mystery":
		// Section 4.4: "what one request sent, the next sends again, and
		// 'the prefix a provider cached holds from one request to the
		// next'." A message or a note written at the end leaves every
		// message of the view as it was, but for the last, which it may
		// add to.
		if k := len(before) - 1; k > 0 {
			if len(after) < k || asJSON(after[:k]) != asJSON(before[:k]) {
				fail("writing entry %d (%s) changed the view before its last message:\n%s\n--- it was ---\n%s", i, e.UUID, render(after), render(before))
			}
		}
		n.appended++
		return
	case e.Type == TypeFrame && e.Op == OpPush:
		// Section 12.2: "A push costs nothing on the server. 'It is a
		// marker gnosh keeps.'"
		if asJSON(before) != asJSON(after) {
			fail("entry %d (%s), a push, changed the view:\n%s\n--- it was ---\n%s", i, e.UUID, render(after), render(before))
		}
		n.pushes++
		return
	case e.Type != TypeFrame || e.Op != OpPop && e.Op != OpSlide:
		return
	}

	var cut *Cut
	for k := range info1.Cuts {
		if info1.Cuts[k].UUID == e.UUID {
			cut = &info1.Cuts[k]
		}
	}
	if cut == nil {
		// A pop of a frame the session does not stand in "records
		// nothing"; a cut that lies behind the summary shows nothing.
		if asJSON(before) != asJSON(after) {
			fail("entry %d (%s) made no cut and still changed the view:\n%s\n--- it was ---\n%s", i, e.UUID, render(after), render(before))
		}
		return
	}
	if cut.Kind == OpPop {
		n.pops++
	} else {
		n.slides++
	}

	// Section 12.2: "A pop removes everything after one point and keeps the
	// pop there, so the prefix is intact through the push and the next
	// request reads the pop and what follows." "A pop keeps the prefix
	// through the push's call. The note is appended at the end of the
	// push's answer". Section 14, case 14: "the request after a pop shares
	// the prefix through the push's call." For a slide the anchor stands
	// where the push does.
	//
	// So: nothing of the view changes that lies before both the first
	// message with a removed row in it and the push's own answer; and the
	// answer is as it was but for the line at its end. "Before" is counted
	// in blocks, since a prompt the pop spares may come to stand at the end
	// of the message before the cut.
	removed := map[string]bool{}
	for _, uuid := range cut.Removed {
		removed[uuid] = true
	}
	was, starts := flat(before)
	is, _ := flat(after)
	point := len(was)
	for k, src := range info0.Sources {
		if slices.ContainsFunc(src, func(uuid string) bool { return removed[uuid] }) {
			point = starts[k]
			break
		}
	}
	answer, at := -1, -1
	if cut.PushReply != "" && cut.PushCall != "" {
		for k, src := range info0.Sources {
			if slices.Contains(src, cut.PushReply) && k+1 < len(before) {
				if _, at = resultFor(before[k+1], cut.PushCall); at >= 0 {
					answer = k + 1
					point = min(point, starts[answer]+at)
				}
				break
			}
		}
	}
	for k := 0; k < point; k++ {
		if k >= len(is) || is[k] != was[k] {
			fail("entry %d (%s): block %d of the view changed, before the cut point at block %d:\n%s\n--- it was ---\n%s", i, e.UUID, k, point, render(after), render(before))
		}
	}
	if stored, ok := storedAnswer(entries[:i], cut.PushReply, cut.PushCall); ok && answer >= 0 && point == starts[answer]+at {
		if point >= len(is) || !strings.HasPrefix(is[point], "user ") {
			fail("entry %d (%s): the push's answer is gone from the view:\n%s", i, e.UUID, render(after))
		}
		old, _ := resultFor(before[answer], cut.PushCall)
		now := ""
		for _, m := range after {
			if text, k := resultFor(m, cut.PushCall); k >= 0 && asBlock(m.Role, m.Blocks[k]) == is[point] {
				now = text
			}
		}
		for _, text := range []string{old, now} {
			rest, ok := strings.CutPrefix(text, stored)
			if !ok || rest != "" && (strings.Count(rest, "[Then came") != 1 || !strings.HasSuffix(rest, ".]")) {
				fail("entry %d (%s): the push's answer is not what was stored with one line at its end: %q, stored %q", i, e.UUID, text, stored)
			}
		}
		// "One answer carries one note": the line is this cut's, or that
		// of another cut that begins at the same answer.
		if now != stored && !slices.ContainsFunc(info1.Cuts, func(c Cut) bool {
			return c.Note != "" && c.PushReply == cut.PushReply && c.PushCall == cut.PushCall && strings.HasSuffix(now, c.Note)
		}) {
			fail("entry %d (%s): the line on the push's answer is no cut's note: %q", i, e.UUID, now)
		}
		if now != stored {
			n.notes++
		}
	}

	// Section 4.2: "A pop spares the prompt rows of the turn it is made in:
	// the prompt the model is answering, prompts folded into that turn, and
	// its hidden continuations and reminders." The nearest prompt the
	// person typed before the pop's reply is of that turn by any reading of
	// where it starts, and so is every prompt row after it: none of them
	// is ever removed by the pop.
	if cut.Kind == OpPop && cut.PopReply != "" {
		alive := Conversation(entries[:i+1])
		p := len(alive) - 1
		for p >= 0 && alive[p].UUID != cut.PopReply {
			p--
		}
		q := p - 1
		for q >= 0 && !(promptOf(&alive[q]) && alive[q].Source == SourcePrompt) {
			q--
		}
		// A prompt is in the view when its words are. (An entry that holds
		// results too may be in the view for those alone, its words having
		// gone in an earlier slide.)
		for k := max(q, 0); q >= 0 && k < p; k++ {
			for _, b := range alive[k].Message.Blocks {
				if alive[k].Type != TypeUser || b.Type == core.BlockToolResult {
					continue
				}
				if words := asBlock(core.RoleUser, b); slices.Contains(was, words) && !slices.Contains(is, words) {
					fail("entry %d (%s): the pop removed the words of %q, a prompt of the turn it was made in:\n%s\n--- it was ---\n%s", i, e.UUID, alive[k].UUID, render(after), render(before))
				}
			}
		}
		if cut.Spared > 0 {
			n.spared++
		}
	}

	// What Preview says of an entry before it is written is what the view
	// says of it afterwards: with the round's results stored, and, for a
	// pop, with only the reply that made the call.
	k := i
	for k > 0 && entries[k-1].Type == TypeFrame {
		k--
	}
	session := ViewOptions{Session: e.SessionID}
	cuts, stack := Preview(entries[:k], entries[k:i+1], session)
	if len(cuts) == 0 || !reflect.DeepEqual(cuts[len(cuts)-1], *cut) || !reflect.DeepEqual(stack, info1.Stack) {
		fail("entry %d (%s): previewed, the cut was\n%+v\nstored, it is\n%+v\nstacks %+v and %+v", i, e.UUID, cuts, *cut, stack, info1.Stack)
	}
	n.previews++
	if cut.Kind == OpPop && cut.PopReply != "" {
		p := k - 1
		for p >= 0 && entries[p].Type == TypeUser {
			p--
		}
		if p >= 0 && entries[p].UUID == cut.PopReply {
			cuts, stack := Preview(entries[:p+1], entries[k:i+1], session)
			if len(cuts) == 0 || !reflect.DeepEqual(cuts[len(cuts)-1], *cut) || !reflect.DeepEqual(stack, info1.Stack) {
				fail("entry %d (%s): previewed before the call's result was stored, the cut was\n%+v\nstored, it is\n%+v", i, e.UUID, cuts, *cut)
			}
			n.early++
		}
	}
}

// Section 12.2, the measured run: "the request after a 3K-token pop re-read
// 2,572 tokens instead of the whole 20K-token conversation, and the
// requests after it were back to a few hundred each". Which tokens a
// server reads again is the server's; what the harness owes it is that the
// request after the pop begins with the bytes of the request before it,
// through the push's call. The run is rebuilt to scale.
func TestSpec12_2_TheRequestAfterAPopSharesThePrefixThroughThePushsCall(t *testing.T) {
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
	request := func() []core.Message {
		msgs, _ := mustView(t, entries, ViewOptions{})
		return msgs
	}
	size := func(msgs []core.Message) (n int) {
		for _, m := range msgs {
			n += EstimateTokens(m)
		}
		return n
	}
	// shared counts the messages two requests begin with alike, and, in
	// the first that differs, the bytes they share.
	shared := func(a, b []core.Message) (msgs, bytes int) {
		for msgs < len(a) && msgs < len(b) && asJSON(a[msgs:msgs+1]) == asJSON(b[msgs:msgs+1]) {
			msgs++
		}
		if msgs < len(a) && msgs < len(b) {
			x, y := asJSON(a[msgs:msgs+1]), asJSON(b[msgs:msgs+1])
			for bytes < len(x) && bytes < len(y) && x[bytes] == y[bytes] {
				bytes++
			}
		}
		return msgs, bytes
	}

	// A conversation of about 17K tokens, then a todo with 3K of work.
	add(PromptEntry(text(14000)))
	round("plan", "gnovm_run", 300)
	add(PushEntry("task", "", "plan"))
	round("earlier", "Read", 2500)
	round("next", "gnovm_run", 240)
	add(PushEntry("task/todo", "", "next"))
	round("read", "Read", 1800)
	round("edit", "Edit", 600)
	round("test", "Bash", 600)
	before := request() // the request the pop was written in answer to
	if n := size(before); n < 19500 || n > 20500 {
		t.Fatalf("the conversation holds %d tokens, want about 20K", n)
	}
	round("pop", "gnovm_run", 250)
	add(PopEntry("task/todo", "", "done", "pop"))
	after := request()

	// Through the push's call: the prompt, the plan's round, the earlier
	// round, and the reply that stepped into the todo.
	msgs, bytes := shared(before, after)
	if msgs != 6 {
		t.Fatalf("the two requests share %d messages, want the 6 through the push's call:\n%s", msgs, render(after))
	}
	// And then the push's answer, up to the line added at its end.
	stored := asJSON([]core.Message{{Role: core.RoleUser, Blocks: []core.Block{core.ToolResultBlock("next", core.TextResult(text(240)))}}})
	if want := strings.Index(stored, text(240)) + len(text(240)); bytes < want {
		t.Errorf("the push's answer shares %d bytes with what was sent before, want its whole text, %d", bytes, want)
	}
	// What is new to the server is the line, and the pop's round.
	fresh := size(after[msgs+1:]) + len("[Then came the work in it, 3 tool calls (Bash 1, Edit 1, Read 1)"+whenPopped)/4
	if fresh > 400 || size(before)-size(after) < 2700 {
		t.Errorf("after the pop %d tokens are new and the request is %d tokens shorter; want a few hundred, and about 3K", fresh, size(before)-size(after))
	}

	// "The requests after it were back to a few hundred each": each begins
	// with the whole of the one before.
	last := after
	for _, id := range []string{"a", "b", "c"} {
		round(id, "Read", 200)
		next := request()
		msgs, _ := shared(last, next)
		if msgs != len(last) {
			t.Errorf("a request after the pop shares %d of the %d messages of the one before", msgs, len(last))
		}
		if fresh := size(next[msgs:]); fresh > 400 {
			t.Errorf("a request after the pop brings %d new tokens", fresh)
		}
		last = next
	}
}

// Section 12.2: "A slide cuts early rounds, so 'everything after the cut is
// re-read'. That is why it cuts enough (at least half the body) to be
// rare." What comes before the cut, through the anchor's call, is as it
// was.
func TestSpec12_2_ASlideKeepsThePrefixThroughItsAnchor(t *testing.T) {
	entries := script(t, specNested)
	before, _ := mustView(t, entries, ViewOptions{})
	slides, _ := Slide(entries, 50, ViewOptions{Estimate: ten})
	after, _ := mustView(t, record(entries, slides), ViewOptions{})
	// The anchor of the first cut is the reply that entered A: the prompt
	// and that reply are as they were, and its answer but for the line.
	if asJSON(before[:2]) != asJSON(after[:2]) {
		t.Errorf("the slide changed the view before its anchor's answer:\n%s", render(after))
	}
	if got := answerTo(t, after, "t1"); !strings.HasPrefix(got, answerTo(t, before, "t1")+"\n[Then came") {
		t.Errorf("the anchor's answer reads %q", got)
	}
	// With nothing more to cut, later requests begin with this one.
	more := append(record(entries, slides), named("m-", script(t, `
		call t8:Bash
		results t8
	`))...)
	later, _ := mustView(t, more, ViewOptions{})
	if asJSON(later[:len(after)]) != asJSON(after) {
		t.Errorf("a request after the slide does not begin with the one before:\n%s", render(later))
	}
}

package transcript

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// maker writes a random transcript the way a session might: prompts,
// rounds of tool calls, frames entered and left, rewinds, summaries,
// slides, a branch now and then, and the odd thing gone wrong.
type maker struct {
	r        *rand.Rand
	entries  []Entry
	stack    []string // the frames the session stands in
	old      []string // tool calls of earlier rounds
	n, calls int
	session  string
	sessions int
}

func (m *maker) add(e Entry) string {
	m.n++
	e.UUID = fmt.Sprintf("e%d", m.n)
	e.SessionID = m.session
	e.V = Version
	m.entries = append(m.entries, e)
	return e.UUID
}

func (m *maker) text() string {
	words := []string{"read", "the", "file", "then", "fix", "it", "please", "port", "8080", "tests"}
	n := 1 + m.r.IntN(30)
	out := make([]string, n)
	for i := range out {
		out[i] = words[m.r.IntN(len(words))]
	}
	return strings.Join(out, " ")
}

// sync reads the stack back from the entries, after something that may
// have changed it.
func (m *maker) sync() {
	m.stack = m.stack[:0]
	for _, f := range Stack(m.entries, "") {
		m.stack = append(m.stack, f.ID)
	}
}

func (m *maker) earlier() string {
	if len(m.entries) == 0 || m.r.IntN(6) == 0 {
		return "nowhere"
	}
	return m.entries[m.r.IntN(len(m.entries))].UUID
}

func (m *maker) round() {
	tools := []string{"Read", "Edit", "Bash", "Grep", "gnovm_run"}
	var msg core.Message
	if m.r.IntN(4) == 0 {
		msg.Blocks = append(msg.Blocks, core.Text(m.text()))
	}
	var ids []string
	for range 1 + m.r.IntN(3) {
		m.calls++
		id := fmt.Sprintf("t%d", m.calls)
		ids = append(ids, id)
		tool := tools[m.r.IntN(len(tools))]
		var input json.RawMessage
		if tool == "Read" || tool == "Edit" {
			input = json.RawMessage(fmt.Sprintf(`{"file_path":"/work/app/f%d.go"}`, m.r.IntN(5)))
		}
		msg.Blocks = append(msg.Blocks, core.ToolUse(id, tool, input))
	}
	stop := core.StopToolUse
	switch m.r.IntN(30) {
	case 0:
		stop = core.StopCancelled
	case 1:
		stop = core.StopError
	}
	m.add(AssistantEntry(msg, stop))

	answered := ids
	switch m.r.IntN(20) {
	case 0:
		m.old = append(m.old, ids...)
		return // gnosh died before any result came back
	case 1:
		answered = ids[:m.r.IntN(len(ids))]
	}
	result := func(id string) core.Block {
		res := core.TextResult(strings.Repeat("x", m.r.IntN(300)))
		res.IsError = m.r.IntN(8) == 0
		return core.ToolResultBlock(id, res)
	}
	switch {
	case len(answered) == 0:
	case m.r.IntN(2) == 0:
		// One entry for each result, as a loop that stores a result when
		// its call ends writes them, with a note of the harness between
		// them now and then.
		for _, id := range answered {
			m.add(UserEntry(core.Message{Blocks: []core.Block{result(id)}}, SourceToolResults))
			if m.r.IntN(12) == 0 {
				m.add(UserEntry(core.UserText(m.text()), SourceHook))
			}
		}
	default:
		var res core.Message
		for _, id := range answered {
			res.Blocks = append(res.Blocks, result(id))
		}
		src := SourceToolResults
		if m.r.IntN(10) == 0 {
			// Results and words in one message: hidden words, or a prompt
			// the person typed.
			res.Blocks = append(res.Blocks, core.Text(m.text()))
			if m.r.IntN(2) == 0 {
				src = SourcePrompt
			}
		}
		m.add(UserEntry(res, src))
	}

	// The frame entries the round's calls made: a pop, a push, or the
	// trampoline's pop and push by one call. Now and then an entry names
	// no call, and a pop one of an earlier round.
	pick := func(stale bool) string {
		switch m.r.IntN(25) {
		case 0:
			return ""
		case 1:
			if stale && len(m.old) > 0 {
				return m.old[m.r.IntN(len(m.old))]
			}
		}
		return ids[m.r.IntN(len(ids))]
	}
	popped := false
	if len(m.stack) > 0 && m.r.IntN(4) == 0 {
		id := m.stack[len(m.stack)-1]
		switch m.r.IntN(10) {
		case 0:
			id = "nobody"
		case 1:
			id = m.stack[m.r.IntN(len(m.stack))]
		}
		call := pick(true)
		m.add(PopEntry(id, "", "result of "+id, call))
		popped = true
		if m.r.IntN(2) == 0 {
			if !slices.Contains(ids, call) {
				call = ids[0]
			}
			m.add(PushEntry(fmt.Sprintf("f%d", m.r.IntN(6)), "a label", call))
		}
	}
	if !popped && m.r.IntN(5) == 0 {
		call := pick(false)
		m.add(PushEntry(fmt.Sprintf("f%d", m.r.IntN(6)), "a label", call))
		if m.r.IntN(6) == 0 {
			// Entered on the way: two frames by one call.
			m.add(PushEntry(fmt.Sprintf("f%d", m.r.IntN(6)), "a label", call))
		}
	}
	m.old = append(m.old, ids...)
	if m.entries[len(m.entries)-1].Type == TypeFrame {
		m.sync()
	}
}

func (m *maker) step() {
	switch p := m.r.IntN(100); {
	case p < 16:
		m.add(PromptEntry(m.text()))
	case p < 60:
		m.round()
	case p < 66:
		stop := core.StopEndTurn
		switch m.r.IntN(12) {
		case 0:
			stop = core.StopError
		case 1:
			stop = core.StopCancelled
		}
		m.add(AssistantEntry(core.AssistantText(m.text()), stop))
	case p < 68:
		// A reply that holds nothing: the model said nothing, or it was
		// stopped or failed before anything arrived.
		stop := []core.StopReason{core.StopEndTurn, core.StopCancelled, core.StopError}[m.r.IntN(3)]
		m.add(AssistantEntry(core.Message{}, stop))
	case p < 73:
		src := SourceHook
		if m.r.IntN(2) == 0 {
			src = SourceNotice
		}
		m.add(UserEntry(core.UserText(m.text()), src))
	case p < 77:
		m.add(RewindEntry(m.earlier()))
		m.sync()
	case p < 79:
		covers := ""
		if m.r.IntN(3) == 0 {
			covers = m.earlier()
		}
		m.add(SummaryEntry(m.text(), covers))
		if m.r.IntN(2) == 0 {
			m.add(UserEntry(core.UserText("carry on"), SourceNotice))
		}
	case p < 82:
		switch m.r.IntN(4) {
		case 0:
			m.add(TitleEntry(m.text(), SourceAuto))
		case 1:
			m.add(MetaEntry("model", "qwen"))
		case 2:
			m.add(UsageEntry(core.Usage{InputTokens: 1000 + 37*m.n, OutputTokens: m.r.IntN(400)}, "qwen", 0))
		default:
			m.add(Entry{Type: "mystery"})
		}
	case p < 86:
		// The view is over a budget, and its slides are recorded.
		_, info := View(m.entries, ViewOptions{Budget: 50 + m.r.IntN(600)})
		for _, s := range info.NewSlides {
			m.add(s)
		}
	case p < 87 && len(m.entries) > 0:
		// A branch: the entries so far, then a new session.
		from, at := m.session, m.entries[len(m.entries)-1].UUID
		m.sessions++
		m.session = fmt.Sprintf("s%d", m.sessions)
		m.add(Entry{Type: TypeBranch, FromSession: from, FromUUID: at})
		m.sync()
	default:
		m.round()
	}
}

// randomTranscript makes a transcript from a seed, sometimes cut short or
// with entries missing, as after a crash or a corrupt line.
func randomTranscript(seed uint64) ([]Entry, *rand.Rand) {
	r := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	m := &maker{r: r, session: "s0"}
	m.add(PromptEntry(m.text()))
	for range 5 + r.IntN(120) {
		m.step()
	}
	entries := m.entries
	if r.IntN(3) == 0 {
		entries = entries[:r.IntN(len(entries)+1)]
	}
	if r.IntN(3) == 0 {
		kept := make([]Entry, 0, len(entries))
		for _, e := range entries {
			if r.IntN(30) != 0 {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	return entries, r
}

// set gathers the UUIDs of a view's sources.
func set(sources [][]string) map[string]bool {
	out := map[string]bool{}
	for _, src := range sources {
		for _, uuid := range src {
			out[uuid] = true
		}
	}
	return out
}

// answersAny reports whether the user entry e holds a result for one of ids.
func answersAny(e *Entry, ids []string) bool {
	for _, b := range e.Message.Blocks {
		if b.Type == core.BlockToolResult && slices.Contains(ids, b.ToolUseID) {
			return true
		}
	}
	return false
}

func TestViewProperties(t *testing.T) {
	seeds := uint64(200)
	if testing.Short() || raceEnabled {
		seeds = 60
	}
	slid, popped := 0, 0
	for seed := range seeds {
		entries, r := randomTranscript(seed)
		fail := func(format string, args ...any) {
			t.Helper()
			t.Fatalf("seed %d: %s", seed, fmt.Sprintf(format, args...))
		}

		// The view with no budget, and with one it cannot meet without
		// sliding. mustView checks that each is a valid conversation, is
		// the same when computed again, and leaves the entries alone.
		msgs0, info0 := mustView(t, entries, ViewOptions{Trace: true})
		opts := ViewOptions{Budget: 1 + r.IntN(max(info0.Tokens, 1)), KeepRounds: r.IntN(3), Trace: true}
		alive := Conversation(entries)
		if r.IntN(2) == 0 {
			for i := len(alive) - 1; i >= 0; i-- {
				if alive[i].Type == TypeUser && alive[i].Source == SourcePrompt {
					opts.Prompt = alive[i].UUID
					break
				}
			}
		}
		msgs1, info1 := mustView(t, entries, opts)
		for _, c := range info0.Cuts {
			if c.Kind == OpPop && c.Messages > 0 {
				popped++
			}
		}
		slid += len(info1.NewSlides)

		// Every message comes from entries that are part of the
		// conversation, in the conversation's order.
		pos := map[string]int{}
		for i, e := range alive {
			pos[e.UUID] = i
		}
		for name, v := range map[string]struct {
			msgs []core.Message
			info ViewInfo
		}{"plain": {msgs0, info0}, "slid": {msgs1, info1}} {
			if len(v.info.Sources) != len(v.msgs) {
				fail("%s view: %d messages but %d source lists", name, len(v.msgs), len(v.info.Sources))
			}
			last := -1
			for i, src := range v.info.Sources {
				for _, uuid := range src {
					p, ok := pos[uuid]
					if !ok {
						fail("%s view: message %d comes from %q, which is not in the conversation", name, i, uuid)
					}
					if uuid == v.info.Summary {
						// The summary comes first in the view wherever its
						// entry is in the file: it may cover less than all
						// that was written before it.
						continue
					}
					if p < last {
						fail("%s view: message %d comes from %q, out of order", name, i, uuid)
					}
					last = p
				}
			}
		}

		// A slide only ever takes messages out.
		in0, in1 := set(info0.Sources), set(info1.Sources)
		for uuid := range in1 {
			if !in0[uuid] {
				fail("the slid view shows %q, which the plain view does not", uuid)
			}
		}
		if info1.OverBudget != (info1.Tokens > opts.Budget) {
			fail("OverBudget = %v with %d tokens against %d", info1.OverBudget, info1.Tokens, opts.Budget)
		}
		for _, c := range info1.Cuts {
			if c.Planned && (c.Tokens <= 0 || c.Messages <= 0 || c.Kind != OpSlide) {
				fail("a planned slide that frees nothing: %+v", c)
			}
		}

		// The stack is the same either way, and with no frame nothing slides.
		if len(info0.Stack) != len(info1.Stack) {
			fail("the stack is %+v plain and %+v slid", info0.Stack, info1.Stack)
		}
		if len(info0.Stack) == 0 && len(info1.NewSlides) > 0 {
			fail("slides were planned with no frame on the stack: %q", slideRefs(info1.NewSlides))
		}

		// The first message of the conversation stays.
		if len(info0.Sources) > 0 && len(info0.Sources[0]) > 0 && !in1[info0.Sources[0][0]] {
			fail("the slide cut the first message, %q", info0.Sources[0][0])
		}

		// Nothing in an open frame's most recent round is ever dropped:
		// the last reply with a tool call between the frame's push and the
		// next frame's push (or the end), and the results that answer it.
		for i, f := range info0.Stack {
			lo, hi := pos[f.UUID], len(alive)
			if i+1 < len(info0.Stack) {
				hi = pos[info0.Stack[i+1].UUID]
			}
			for p := hi - 1; p > lo; p-- {
				e := &alive[p]
				if e.Type != TypeAssistant || len(e.Message.ToolUses()) == 0 || !in0[e.UUID] {
					continue
				}
				if !in1[e.UUID] {
					fail("the slide cut %q, the newest round of the open frame %q", e.UUID, f.ID)
				}
				var ids []string
				for _, b := range e.Message.ToolUses() {
					ids = append(ids, b.ID)
				}
				for q := p + 1; q < hi && alive[q].Type != TypeAssistant; q++ {
					if u := &alive[q]; u.Type == TypeUser && in0[u.UUID] && answersAny(u, ids) && !in1[u.UUID] {
						fail("the slide cut %q, a result of the newest round of the open frame %q", u.UUID, f.ID)
					}
				}
				break
			}
		}

		// With the planned slides recorded, a view with no budget is the
		// same view: a recorded slide cuts where the planned one did.
		if len(entries) > 0 {
			recorded := record(entries, info1.NewSlides)
			again, _ := mustView(t, recorded, ViewOptions{Prompt: opts.Prompt})
			if !reflect.DeepEqual(msgs1, again) {
				fail("with its slides recorded the view is\n%s\n--- it was ---\n%s", render(again), render(msgs1))
			}
		}
	}
	// The generator must reach what the properties are about.
	if slid < 10 || popped < 20 {
		t.Errorf("the random transcripts gave only %d slides and %d pops that cut something", slid, popped)
	}
}

// jsonl writes entries as the lines of a session file.
func jsonl(t testing.TB, entries []Entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	for i := range entries {
		line, err := entries[i].line()
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// FuzzView loads whatever bytes it is given as a session file and checks
// that the view of it is a valid conversation, with and without a budget.
func FuzzView(f *testing.F) {
	for seed := range uint64(8) {
		entries, _ := randomTranscript(seed)
		f.Add(jsonl(f, entries), 200)
	}
	f.Add([]byte(`{"type":"assistant","message":{"role":"user","content":[{"type":"tool_use","id":"a"},{"type":"tool_use","id":"a"}]}}`+"\n"+
		`{"type":"frame","op":"pop","frame":"x","toolUseId":"a"}`+"\n"+
		`{"type":"frame","op":"slide","anchor":"","until":""}`+"\n"+
		`{"type":"rewind","to":""}`+"\n"), 1)
	f.Fuzz(func(t *testing.T, data []byte, budget int) {
		ld, err := load(context.Background(), bytes.NewReader(data), "")
		if err != nil {
			t.Fatalf("loading from memory failed: %v", err)
		}
		for _, opts := range []ViewOptions{{}, {Budget: budget, Trace: true}, {Budget: 1, KeepRounds: 2}} {
			msgs, info := View(ld.entries, opts)
			if err := Check(msgs); err != nil {
				t.Fatalf("the view is not a valid conversation: %v\n%s", err, render(msgs))
			}
			again, info2 := View(ld.entries, opts)
			if !reflect.DeepEqual(msgs, again) || !reflect.DeepEqual(info, info2) {
				t.Fatalf("the view is not the same when computed again")
			}
			for _, s := range info.NewSlides {
				if s.Anchor == "" || s.Until == "" {
					t.Fatalf("a planned slide that cannot be recorded: %+v", s)
				}
			}
			Stack(ld.entries, "")
			Conversation(ld.entries)
		}
		// Whatever the file holds, the view of it is the one the spec's
		// words give (oracle_spec_test.go).
		if diff := againstSpec(ld.entries, ""); diff != "" {
			t.Fatal(diff)
		}
		// And a pop of each frame the session stands in can be previewed.
		for _, f := range Stack(ld.entries, "") {
			cuts, _ := Preview(ld.entries, []Entry{PopEntry(f.ID, "", "", f.ToolUseID)}, ViewOptions{})
			if len(cuts) != 1 || cuts[0].Kind != OpPop || cuts[0].Frame != f.ID {
				t.Fatalf("previewing a pop of %q gave %+v", f.ID, cuts)
			}
		}
	})
}

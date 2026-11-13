package transcript

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// The frame rule a second time.
//
// specView follows the words of spec/gnosh/frames.md as literally as it
// can and takes no care to be fast: it scans back and forth over the rows
// the way the spec's paragraphs do. It shares with the package only what
// the rule does not speak of (which entries a rewind leaves, the first
// sentence of a summary's message, how a line is joined to a result's
// text, how a list is made valid for a provider), so that a slip in the
// package's reading of the rule shows as a difference between the two.
// Every view the tests compute is checked against it (mustView), and
// TestSpecOracle runs both over transcripts nobody thought of.
//
// It does not plan slides. A recorded slide is a range like any other; how
// one is chosen is held by the examples in slide_spec_test.go.

// specRow is a row in the sense of frames.md section 2.
type specRow struct {
	// kind is 'P' for a prompt row, 'A' for an assistant row, 'R' for a
	// result row, and 'H' for a reply that holds nothing and was cancelled
	// or failed: the model is never sent it, so no range counts it, but it
	// is the reply at which its turn stopped (section 4.3).
	kind   byte
	ent    int // the entry it came from: an index into the conversation
	blocks []core.Block
	hidden bool // a prompt row the person did not type
	ended  bool // a reply that was cancelled or failed
}

// specFrame is a frame on a session's stack (section 3.3).
type specFrame struct {
	id, call string
	pushRow  int // the assistant row that holds the push call, or -1
	pos      int // how many rows come before the push entry
}

// specCut is what one pop or recorded slide comes to.
type specCut struct {
	kind     FrameOp
	frame    string
	removed  []string // the entries in the range that do not stay
	between  int      // how many of them there are
	spared   int      // the prompts of the pop's turn the model is left with
	messages int      // the entries it took that the model still had
	tokens   int      // and their size
	note     string
}

// specResult is the view as the spec has it.
type specResult struct {
	msgs      []core.Message
	stack     []string
	cuts      map[string]specCut // by the UUID of the frame entry
	unmatched []string
}

func specView(entries []Entry, session string) specResult {
	if session == "" && len(entries) > 0 {
		session = entries[len(entries)-1].SessionID
	}
	alive := Conversation(entries)

	// The view starts at the latest summary. What it covers is behind it.
	sum, covered := -1, -1
	for p := len(alive) - 1; p >= 0 && sum < 0; p-- {
		if alive[p].Type != TypeSummary {
			continue
		}
		sum, covered = p, p-1
		for q := p - 1; q >= 0 && alive[p].Covers != ""; q-- {
			if alive[q].UUID == alive[p].Covers {
				covered = q
				break
			}
		}
	}

	// Section 2: the rows. Results and other text in one user message are
	// separate rows.
	var rows []specRow
	type event struct{ pos, ent int }
	var events []event
	first := 0 // the first row that is not behind the summary
	for p := range alive {
		e := &alive[p]
		switch e.Type {
		case TypeAssistant:
			ended := e.Stop == core.StopCancelled || e.Stop == core.StopError
			if len(e.Message.Blocks) > 0 {
				rows = append(rows, specRow{kind: 'A', ent: p, blocks: e.Message.Blocks, ended: ended})
			} else if ended {
				rows = append(rows, specRow{kind: 'H', ent: p, ended: true})
			}
		case TypeUser:
			var results, rest []core.Block
			for _, b := range e.Message.Blocks {
				if b.Type == core.BlockToolResult {
					results = append(results, b)
				} else {
					rest = append(rest, b)
				}
			}
			if len(results) > 0 {
				rows = append(rows, specRow{kind: 'R', ent: p, blocks: results})
			}
			if len(rest) > 0 {
				rows = append(rows, specRow{kind: 'P', ent: p, blocks: rest, hidden: e.Source != SourcePrompt})
			}
		case TypeFrame:
			events = append(events, event{len(rows), p})
		}
		if p <= covered {
			first = len(rows)
		}
	}
	reply := func(r int) bool { return rows[r].kind == 'A' }
	toolCalls := func(r int) (ids []string) {
		for _, b := range rows[r].blocks {
			if b.Type == core.BlockToolUse {
				ids = append(ids, b.ID)
			}
		}
		return ids
	}
	// find is "the assistant row holding the call", looking back from the
	// entry that names it.
	find := func(pos int, id string) int {
		for r := pos - 1; r >= 0 && id != ""; r-- {
			if reply(r) && slices.Contains(toolCalls(r), id) {
				return r
			}
		}
		return -1
	}
	// answer is the result row that answers the call id of the assistant
	// row a, or -1. The results of a round lie before the next reply.
	answer := func(a int, id string) int {
		for r := a + 1; r < len(rows) && !reply(r); r++ {
			if rows[r].kind != 'R' {
				continue
			}
			for _, b := range rows[r].blocks {
				if b.ToolUseID == id && slices.Contains(toolCalls(a), id) {
					return r
				}
			}
		}
		return -1
	}
	// roundEnd is "the last result row that answers any call of" the
	// assistant row a, or a itself.
	roundEnd := func(a int) int {
		last := a
		for _, id := range toolCalls(a) {
			for r := a + 1; r < len(rows) && !reply(r); r++ {
				if rows[r].kind == 'R' && slices.ContainsFunc(rows[r].blocks, func(b core.Block) bool { return b.ToolUseID == id }) {
					last = max(last, r)
				}
			}
		}
		return last
	}
	// cont is "the model's first own (assistant) row after" the summary:
	// the rows before it continue the summary.
	cont := first
	for cont < len(rows) && !reply(cont) {
		cont++
	}

	// Section 4.3: where the turn of the pop at row pop starts.
	turnStart := func(pop int) int {
		r := pop - 1
		for {
			// "Scan back from the pop's row for a visible (not hidden)
			// prompt row."
			for r >= first && !(rows[r].kind == 'P' && !rows[r].hidden) {
				r--
			}
			if r < first {
				// "If no such run is found, the turn starts at the first
				// row."
				return first
			}
			// "Take the whole run of consecutive prompt rows that ends at
			// it."
			run := r
			for run-1 >= first && rows[run-1].kind == 'P' {
				run--
			}
			// "That run starts a turn if what precedes it is nothing, or a
			// summary row,"
			if run == first {
				return run
			}
			// "or an assistant row with no tool call,"
			if before := rows[run-1]; before.kind == 'H' || before.kind == 'A' && len(toolCalls(run-1)) == 0 {
				return run
			}
			// "or anything else, when the nearest assistant row before the
			// run ended cancelled or failed".
			for q := run - 1; q >= 0; q-- {
				if rows[q].kind == 'A' || rows[q].kind == 'H' {
					if rows[q].ended {
						return run
					}
					break
				}
			}
			// "Otherwise that prompt was folded into a running turn: keep
			// scanning back."
			r = run - 1
		}
	}

	out := specResult{cuts: map[string]specCut{}}
	removed := make([]bool, len(rows))
	type place struct {
		row  int
		call string
	}
	type line struct {
		kind FrameOp
		to   int
		text string
	}
	notes := map[place]line{}

	// take applies one range, rows [from, to). lead is the reply the range
	// follows: when it is behind the summary, the summary is inside the
	// range. Prompt rows at or after turn are spared.
	take := func(uuid string, kind FrameOp, frame string, from, to, lead, turn int, at place) {
		cut := specCut{kind: kind, frame: frame}
		names := map[string]int{}
		calls, last, lastNew := 0, -1, -1
		for r := from; r < to; r++ {
			row := &rows[r]
			if row.kind == 'H' {
				continue
			}
			had := r >= first && !removed[r]
			if row.kind == 'P' && r >= turn {
				// Section 4.2: spared.
				if had {
					cut.spared++
				}
				continue
			}
			if sum >= 0 && lead < first && r >= first && r < cont {
				// Section 4.1, step 4: the rows that continue the summary.
				continue
			}
			if row.kind == 'A' {
				for _, b := range row.blocks {
					if b.Type == core.BlockToolUse {
						names[b.Name]++
						calls++
					}
				}
			}
			if row.ent != last {
				cut.between++
				cut.removed = append(cut.removed, alive[row.ent].UUID)
				last = row.ent
			}
			if had {
				// Section 10: only what the model still had is counted.
				removed[r] = true
				role := core.RoleUser
				if row.kind == 'A' {
					role = core.RoleAssistant
				}
				cut.tokens += EstimateTokens(core.Message{Role: role, Blocks: row.blocks})
				if row.ent != lastNew {
					cut.messages++
					lastNew = row.ent
				}
			}
		}
		// Section 4.4: the note, unless every row was spared or the push
		// is not among the rows.
		if at.row >= 0 && cut.between > 0 {
			cut.note = specNote(kind, names, calls, cut.between)
			// "One answer carries one note": the pop's wins over a
			// slide's, and of two of a kind the one that reaches furthest.
			if old, ok := notes[at]; !ok || kind == OpPop && old.kind == OpSlide || kind == old.kind && to > old.to {
				notes[at] = line{kind, to, cut.note}
			}
		}
		out.cuts[uuid] = cut
	}

	// Section 3.3: walk the frame events in order, each on the stack of the
	// session that made it.
	stacks := map[string][]specFrame{}
	for _, ev := range events {
		e := &alive[ev.ent]
		stack := stacks[e.SessionID]
		switch e.Op {
		case OpPush:
			stacks[e.SessionID] = append(stack, specFrame{id: e.Frame, call: e.ToolUseID, pushRow: find(ev.pos, e.ToolUseID), pos: ev.pos})
		case OpPop:
			// "a pop removes the topmost frame with that id"
			k := len(stack) - 1
			for k >= 0 && stack[k].id != e.Frame {
				k--
			}
			if k < 0 {
				// "A pop naming a frame that is not on the stack records
				// nothing."
				out.unmatched = append(out.unmatched, e.UUID)
				continue
			}
			f := stack[k]
			stacks[e.SessionID] = append(stack[:k:k], stack[k+1:]...)

			// Section 4.1.
			from, to, lead, turn, at := 0, 0, 0, len(rows), place{-1, f.call}
			pop := find(ev.pos, e.ToolUseID)
			switch {
			case pop < 0:
				// Step 1: the pop call is not among the rows.
			case pop == f.pushRow:
				// Step 2: pushed and popped in one input.
			default:
				to = pop
				if f.pushRow >= 0 {
					// Step 3.
					from, lead = roundEnd(f.pushRow)+1, f.pushRow
					at.row = answer(f.pushRow, f.call)
				} else {
					// Step 5, as this package reads it: the push entry has
					// a place of its own, so the range starts there, and
					// "removal begins at the first assistant row from
					// there". No note.
					from = f.pos
					for from < pop && !reply(from) {
						from++
					}
					lead = from
				}
				turn = turnStart(pop)
			}
			take(e.UUID, OpPop, f.id, from, to, lead, turn, at)
		case OpSlide:
			// Section 8.6: a slide's range is recorded and applied as a
			// pop's, with nothing spared. Its bounds are two replies.
			anchor, until := -1, -1
			for r := 0; r < ev.pos; r++ {
				if !reply(r) {
					continue
				}
				if uuid := alive[rows[r].ent].UUID; uuid != "" {
					if uuid == e.Anchor {
						anchor = r
					}
					if uuid == e.Until {
						until = r
					}
				}
			}
			from, to, at := 0, 0, place{-1, ""}
			if anchor >= 0 && until >= 0 {
				from, to = roundEnd(anchor)+1, until
				// The note goes on "the first call of the anchor round's
				// assistant row (the frame's push call for a frame)".
				if ids := toolCalls(anchor); len(ids) > 0 {
					at.call = ids[0]
				}
				for k := len(stack) - 1; k >= 0 && e.Frame != ""; k-- {
					if stack[k].id == e.Frame && stack[k].pushRow == anchor && stack[k].call != "" {
						at.call = stack[k].call
						break
					}
				}
				if at.call != "" {
					at.row = answer(anchor, at.call)
				}
			}
			take(e.UUID, OpSlide, e.Frame, from, to, anchor, len(rows), at)
		}
	}
	for _, f := range stacks[session] {
		out.stack = append(out.stack, f.id)
	}

	// Section 11: the list is the rows left, with the notes on the answers
	// they belong to; then it is made valid.
	var pieces []piece
	if sum >= 0 {
		m := core.UserText(SummaryPrefix + alive[sum].Summary)
		pieces = append(pieces, piece{role: m.Role, blocks: m.Blocks})
	}
	for r := first; r < len(rows); r++ {
		if removed[r] || rows[r].kind == 'H' {
			continue
		}
		blocks := rows[r].blocks
		for at, n := range notes {
			if at.row != r {
				continue
			}
			blocks = slices.Clone(blocks)
			for i := range blocks {
				if blocks[i].Type == core.BlockToolResult && blocks[i].ToolUseID == at.call {
					blocks[i].Content = appendLine(blocks[i].Content, n.text)
					break
				}
			}
		}
		role := core.RoleUser
		if rows[r].kind == 'A' {
			role = core.RoleAssistant
		}
		pieces = append(pieces, piece{role: role, blocks: blocks})
	}
	var info ViewInfo
	out.msgs, _ = repair(pieces, &info)
	return out
}

// specNote is the line of sections 4.4 and 8.6.
func specNote(kind FrameOp, names map[string]int, calls, messages int) string {
	tail := ", removed from your context when you popped it.]"
	if kind == OpSlide {
		tail = ", removed from your context to make room.]"
	}
	if calls == 0 {
		// "With no tool call among the removed rows".
		n := fmt.Sprintf("%d messages", messages)
		if messages == 1 {
			n = "1 message"
		}
		if kind == OpSlide {
			return "[Then came " + n + tail
		}
		return "[Then came " + n + " of work in it" + tail
	}
	// "the tools are listed by name, sorted, each with its count. One call
	// reads '1 tool call'."
	var list []string
	for name, n := range names {
		list = append(list, fmt.Sprintf("%s %d", name, n))
	}
	slices.Sort(list)
	n := fmt.Sprintf("%d tool calls", calls)
	if calls == 1 {
		n = "1 tool call"
	}
	return "[Then came the work in it, " + n + " (" + strings.Join(list, ", ") + ")" + tail
}

// asJSON writes messages as the bytes a request would carry.
func asJSON(msgs []core.Message) string {
	b, err := json.Marshal(msgs)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// againstSpec compares the package's view of entries, with nothing planned,
// against specView's. It returns what differs, or nothing.
func againstSpec(entries []Entry, session string) string {
	want := specView(entries, session)
	got, info := View(entries, ViewOptions{Session: session, Trace: true})
	if asJSON(got) != asJSON(want.msgs) {
		return fmt.Sprintf("the view is not the one frames.md describes:\n%s\n--- by the spec ---\n%s", render(got), render(want.msgs))
	}
	var stack []string
	for _, f := range info.Stack {
		stack = append(stack, f.ID)
	}
	if !reflect.DeepEqual(stack, want.stack) {
		return fmt.Sprintf("the stack is %q; by the spec it is %q", stack, want.stack)
	}
	if !reflect.DeepEqual(info.UnmatchedPops, want.unmatched) {
		return fmt.Sprintf("the pops that changed nothing are %q; by the spec %q", info.UnmatchedPops, want.unmatched)
	}
	// The cuts are compared by the entry that made each, which takes
	// entries that can be told apart.
	seen := map[string]bool{}
	for _, e := range entries {
		if e.Type == TypeFrame && (e.UUID == "" || seen[e.UUID]) {
			return ""
		}
		seen[e.UUID] = true
	}
	for _, c := range info.Cuts {
		w, ok := want.cuts[c.UUID]
		if !ok {
			return fmt.Sprintf("the cut of entry %q is not one the spec makes: %+v", c.UUID, c)
		}
		removed := c.Removed
		if len(removed) == 0 {
			removed = nil
		}
		if c.Kind != w.kind || c.Frame != w.frame || !reflect.DeepEqual(removed, w.removed) || c.Between != w.between ||
			c.Spared != w.spared || c.Messages != w.messages || c.Tokens != w.tokens || c.Note != w.note {
			return fmt.Sprintf("the cut of entry %q is\n%+v\nby the spec it is\n%+v", c.UUID, c, w)
		}
	}
	return ""
}

// TestSpecOracle runs the package and the spec's own words over random
// transcripts: pushes, pops, recorded slides, summaries, rewinds, branches,
// cancelled and failed replies, replies with several tool calls, and the
// odd thing gone wrong. It looks at every stretch of each session, not only
// at its end.
func TestSpecOracle(t *testing.T) {
	seeds := uint64(1500)
	if testing.Short() || raceDetector {
		seeds = 300
	}
	for seed := range seeds {
		entries, _ := randomTranscript(seed)
		for n := len(entries); n > 0; n -= 1 + len(entries)/12 {
			if diff := againstSpec(entries[:n], ""); diff != "" {
				t.Fatalf("seed %d, the first %d entries: %s", seed, n, diff)
			}
		}
	}
}

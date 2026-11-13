package transcript

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// This file is the frame rule: how the stack is read off the frame
// entries, and what a pop or a recorded slide takes out of the view. It
// follows spec/gnosh/frames.md (sections 3, 4 and 11); doc.go restates the
// rule in this package's own terms. frames_test.go holds it as examples.

// frame is a frame on a session's stack.
type frame struct {
	id, label string
	call      string // the tool call that entered it
	row       int    // the reply that made that call: an assistant row, or -1
	at        int    // how many rows come before its push entry
	ent       int    // its push entry
}

// cut is one pop or slide: the rows it covers and what it left behind.
type cut struct {
	kind                 FrameOp
	frame, label, result string
	ent                  int // the frame entry that made it; -1 when planned by this call
	pushEnt              int // the entry that entered the frame; -1 for a slide
	at                   int // how many rows come before the entry that made it
	inner                []string
	live                 bool

	// The cut covers rows [from, to). Nothing is covered when from >= to;
	// reason then says why.
	from, to int
	reason   string
	// turn is the row the pop's own turn starts at: prompt rows of the
	// range at or after it are spared. A slide spares none.
	turn int
	// anchor and until are a slide's bounds: the assistant row whose round
	// stays in front of the cut, and the one the cut ends before.
	anchor, until int
	// noteRow and noteCall say where the cut's note goes: the result row
	// and the tool call whose answer gets it. noteRow is -1 for none.
	noteRow  int
	noteCall string

	// What apply found.
	messages, tokens, spared int
	note                     string
}

// derived is what the frame entries of a conversation come to.
type derived struct {
	cuts      []cut
	stack     []frame         // the stack of the conversation's own session
	slid      map[string]bool // frames on that stack whose start has slid
	unmatched []int           // pop entries that named a frame not on the stack
}

// derive walks the frame entries in order and reads off each session's
// stack and every cut.
//
// A push puts a frame on the stack of the session that wrote the entry. A
// pop takes the topmost frame with its id off that session's stack, along
// with any frames above it, and makes a cut. A pop that names a frame not
// on the stack changes nothing. Entries written under another session id
// (the session a branch came from) keep their cuts but put nothing on this
// session's stack: a branch starts at the root.
func (c *conv) derive() *derived {
	type session struct {
		stack  []frame
		slides []int // live slides: indices into st.cuts
	}
	st := &derived{slid: map[string]bool{}}
	sessions := map[string]*session{}
	for _, ev := range c.events {
		e := &c.entries[ev.ent]
		s := sessions[e.SessionID]
		if s == nil {
			s = &session{}
			sessions[e.SessionID] = s
		}
		c.walk(ev.at)
		switch e.Op {
		case OpPush:
			s.stack = append(s.stack, frame{id: e.Frame, label: e.Label, call: e.ToolUseID, row: c.callRow(e.ToolUseID), at: ev.at, ent: ev.ent})
		case OpPop:
			k := len(s.stack) - 1
			for k >= 0 && s.stack[k].id != e.Frame {
				k--
			}
			if k < 0 {
				st.unmatched = append(st.unmatched, ev.ent)
				continue
			}
			f := s.stack[k]
			gone := map[string]bool{f.id: true}
			var inner []string
			for _, g := range s.stack[k+1:] {
				inner = append(inner, g.id)
				gone[g.id] = true
			}
			s.stack = s.stack[:k]
			// A slide stays worth telling of only while its frame is on
			// the stack; an empty stack clears them all.
			s.slides = slices.DeleteFunc(s.slides, func(i int) bool {
				if len(s.stack) == 0 || gone[st.cuts[i].frame] {
					st.cuts[i].live = false
					return true
				}
				return false
			})
			cut := c.popCut(f, ev, e)
			cut.inner = inner
			st.cuts = append(st.cuts, cut)
		case OpSlide:
			cut := c.slideCut(ev, e)
			cut.live = len(s.stack) > 0 && (e.Frame == "" || slices.ContainsFunc(s.stack, func(f frame) bool { return f.id == e.Frame }))
			st.cuts = append(st.cuts, cut)
			if cut.live {
				s.slides = append(s.slides, len(st.cuts)-1)
			}
		}
	}
	if s := sessions[c.session]; s != nil {
		st.stack = s.stack
		for _, i := range s.slides {
			st.slid[st.cuts[i].frame] = true
		}
	}
	return st
}

// popCut works out what the pop written as entry e takes out, for the
// frame f it closes.
//
//  1. The pop's reply is the assistant row that holds the pop's tool call.
//     If it is not in the conversation, nothing is removed.
//  2. If the same reply made the push, nothing is removed.
//  3. The cut starts after the last result row that answers a call of the
//     push's reply, and ends before the pop's reply. So the push round and
//     the pop round both stay whole.
//  4. When the push's reply is not in the conversation, the cut starts at
//     the first assistant row after the push entry, and leaves no note.
//  5. The prompt rows of the turn the pop is made in are spared.
func (c *conv) popCut(f frame, ev event, e *Entry) cut {
	cut := cut{
		kind: OpPop, frame: f.id, label: cmp.Or(e.Label, f.label), result: e.Result,
		ent: ev.ent, pushEnt: f.ent, at: ev.at, noteRow: -1, turn: len(c.rows),
	}
	popRow := c.callRow(e.ToolUseID)
	if popRow < 0 {
		cut.reason = "the reply that made the pop call is not in the conversation"
		return cut
	}
	pushRow := f.row
	if pushRow == popRow {
		cut.reason = "the frame was entered and left in one reply"
		return cut
	}
	from := f.at
	noteRow := -1
	if pushRow >= 0 {
		from = c.lastAnswer(pushRow) + 1
		noteRow = c.answerRow(pushRow, f.call)
	} else {
		for from < popRow && c.rows[from].kind != rowAssistant {
			from++
		}
	}
	if from >= popRow {
		cut.reason = "nothing came between entering the frame and leaving it"
		return cut
	}
	cut.from, cut.to = from, popRow
	cut.noteRow, cut.noteCall = noteRow, f.call
	cut.turn = c.turnStart(popRow)
	return cut
}

// slideCut works out what the recorded slide e takes out: the rows after
// its anchor's round and before the reply it ends at.
func (c *conv) slideCut(ev event, e *Entry) cut {
	cut := cut{kind: OpSlide, frame: e.Frame, ent: ev.ent, pushEnt: -1, at: ev.at, noteRow: -1, turn: len(c.rows)}
	anchor := c.entryRow(e.Anchor)
	until := c.entryRow(e.Until)
	if anchor < 0 || until < 0 {
		cut.reason = "the replies that bound the slide are not in the conversation"
		return cut
	}
	c.bound(&cut, anchor, until)
	return cut
}

// bound sets a slide's rows from its anchor and the reply it ends before.
func (c *conv) bound(cut *cut, anchor, until int) {
	from := c.lastAnswer(anchor) + 1
	if from >= until {
		cut.reason = "nothing lies between the slide's bounds"
		return
	}
	cut.from, cut.to = from, until
	cut.anchor, cut.until = anchor, until
	if call := firstToolUse(c.rows[anchor].blocks); call != "" {
		cut.noteRow, cut.noteCall = c.answerRow(anchor, call), call
	}
}

// walk notes the assistant rows before row at in the lookup tables. The
// frame entries are walked in order, so a table read while handling one of
// them knows exactly the rows that come before it.
func (c *conv) walk(at int) {
	for ; c.walked < at && c.walked < len(c.rows); c.walked++ {
		row := &c.rows[c.walked]
		if row.kind != rowAssistant {
			continue
		}
		if c.byUUID != nil {
			c.byUUID[c.entries[row.ent].UUID] = c.walked
		}
		for i := range row.blocks {
			if b := &row.blocks[i]; b.Type == core.BlockToolUse && b.ID != "" {
				c.calls[b.ID] = c.walked
			}
		}
	}
}

// callRow returns the nearest assistant row, among those walked, that
// holds the tool call id, or -1.
func (c *conv) callRow(id string) int {
	if r, ok := c.calls[id]; ok && id != "" {
		return r
	}
	return -1
}

// entryRow returns the assistant row, among those walked, that came from
// the entry uuid, or -1.
func (c *conv) entryRow(uuid string) int {
	if r, ok := c.byUUID[uuid]; ok && uuid != "" {
		return r
	}
	return -1
}

// answers reports whether result row r holds the answer to a call of the
// assistant row a, or to the one call id when id is not empty.
func (c *conv) answers(r, a int, id string) bool {
	for i := range c.rows[r].blocks {
		res := &c.rows[r].blocks[i]
		if res.Type != core.BlockToolResult || id != "" && res.ToolUseID != id {
			continue
		}
		for j := range c.rows[a].blocks {
			if use := &c.rows[a].blocks[j]; use.Type == core.BlockToolUse && use.ID == res.ToolUseID {
				return true
			}
		}
	}
	return false
}

// lastAnswer returns the last result row that answers a call of the
// assistant row a, or a itself when none does. The rows up to it are a's
// round.
func (c *conv) lastAnswer(a int) int {
	last := a
	for r := a + 1; r < len(c.rows) && c.rows[r].kind != rowAssistant; r++ {
		if c.rows[r].kind == rowResult && c.answers(r, a, "") {
			last = r
		}
	}
	return last
}

// answerRow returns the result row that answers the call id of the
// assistant row a, or -1.
func (c *conv) answerRow(a int, id string) int {
	for r := a + 1; r < len(c.rows) && c.rows[r].kind != rowAssistant; r++ {
		if c.rows[r].kind == rowResult && c.answers(r, a, id) {
			return r
		}
	}
	return -1
}

// firstToolUse returns the id of the first tool call among blocks.
func firstToolUse(blocks []core.Block) string {
	for i := range blocks {
		if blocks[i].Type == core.BlockToolUse {
			return blocks[i].ID
		}
	}
	return ""
}

// turn is a run of prompt rows that starts a turn: its first row, and the
// last prompt in it that the person typed.
type turn struct {
	start, prompt int
}

// turnStart returns the row at which the turn that holds the assistant
// row at begins.
//
// Going back from at, the nearest prompt the person typed is found, and
// with it the run of prompt rows it ends. That run starts the turn when
// what comes before it is nothing, the summary, a reply with no tool call
// (the answer to the turn before), or anything else when the nearest reply
// before it was cancelled or failed. Otherwise the prompt was folded into
// a turn already running, and the search goes on further back. When no
// such run is found the turn starts at the first row the model has.
func (c *conv) turnStart(at int) int {
	if !c.turnsBuilt {
		c.turnsBuilt = true
		reply := -1 // the nearest assistant row so far
		for r := c.start; r < len(c.rows); {
			switch c.rows[r].kind {
			case rowAssistant:
				reply = r
				r++
			case rowResult:
				r++
			case rowPrompt:
				first, typed := r, -1
				for ; r < len(c.rows) && c.rows[r].kind == rowPrompt; r++ {
					if !c.rows[r].hidden {
						typed = r
					}
				}
				if typed < 0 {
					continue
				}
				starts := first == c.start
				if !starts {
					before := &c.rows[first-1]
					starts = before.kind == rowAssistant && before.calls == 0 || reply >= 0 && c.rows[reply].ended
				}
				if starts {
					c.turns = append(c.turns, turn{start: first, prompt: typed})
				}
			}
		}
	}
	// The turns are in order: take the last whose prompt comes before at.
	i, _ := slices.BinarySearchFunc(c.turns, at, func(t turn, at int) int { return cmp.Compare(t.prompt, at) })
	if i == 0 {
		return c.start
	}
	return c.turns[i-1].start
}

// stays reports whether row r of the cut is left in the view: a prompt of
// the pop's own turn, or a row that continues the summary when the cut
// began behind it.
func (c *conv) stays(cut *cut, r int) bool {
	if c.rows[r].kind == rowPrompt && r >= cut.turn {
		return true
	}
	return c.summary >= 0 && cut.from <= c.start && r >= c.start && r < c.cont
}

// apply takes the cut's rows out of the view, counts what that freed, and
// puts the cut's note on the answer in front of it.
func (c *conv) apply(cut *cut) {
	if cut.from >= cut.to {
		return
	}
	calls := map[string]int{}
	total, messages, lastEnt, lastTaken := 0, 0, -1, -1
	for r := cut.from; r < cut.to; r++ {
		row := &c.rows[r]
		if c.stays(cut, r) {
			if row.kind == rowPrompt && r >= cut.turn && r >= c.start {
				cut.spared++
			}
			continue
		}
		if row.kind == rowAssistant {
			for i := range row.blocks {
				if row.blocks[i].Type == core.BlockToolUse {
					calls[row.blocks[i].Name]++
					total++
				}
			}
		}
		if row.ent != lastEnt {
			messages++
			lastEnt = row.ent
		}
		if r >= c.start && !c.removed[r] {
			c.removed[r] = true
			cut.tokens += c.rowTokens(r)
			if row.ent != lastTaken {
				// A user entry that is two rows is still one message.
				cut.messages++
				lastTaken = row.ent
			}
		}
	}
	if cut.noteRow < 0 || messages == 0 {
		return
	}
	cut.note = noteText(cut.kind, calls, total, messages)
	c.addNote(cut.noteRow, note{call: cut.noteCall, text: cut.note, kind: cut.kind, to: cut.to})
}

// note is a line added to one answer of a result row.
type note struct {
	call string  // the tool call whose answer gets the line
	text string  // the line
	kind FrameOp // the kind of cut that made it
	to   int     // where that cut ends
}

// addNote records a note for result row r. An answer carries one note:
// a pop's wins over a slide's, and between two of a kind the one whose
// cut reaches furthest.
func (c *conv) addNote(r int, n note) {
	if c.notes == nil {
		c.notes = map[int][]note{}
	}
	for i, old := range c.notes[r] {
		if old.call != n.call {
			continue
		}
		if n.kind != old.kind && n.kind == OpPop || n.kind == old.kind && n.to > old.to {
			c.notes[r][i] = n
		}
		return
	}
	c.notes[r] = append(c.notes[r], n)
}

// noteText writes the line that tells the model what a cut removed: the
// tool calls of the removed replies by name, or, when there were none, how
// many messages went.
func noteText(kind FrameOp, calls map[string]int, total, messages int) string {
	end := "removed from your context when you popped it.]"
	if kind == OpSlide {
		end = "removed from your context to make room.]"
	}
	if total > 0 {
		names := make([]string, 0, len(calls))
		for name := range calls {
			names = append(names, name)
		}
		slices.Sort(names)
		for i, name := range names {
			names[i] = fmt.Sprintf("%s %d", name, calls[name])
		}
		word := "tool calls"
		if total == 1 {
			word = "tool call"
		}
		if kind == OpSlide {
			return fmt.Sprintf("[Then came %d %s (%s), %s", total, word, strings.Join(names, ", "), end)
		}
		return fmt.Sprintf("[Then came the work in it, %d %s (%s), %s", total, word, strings.Join(names, ", "), end)
	}
	word := "messages"
	if messages == 1 {
		word = "message"
	}
	if kind == OpSlide {
		return fmt.Sprintf("[Then came %d %s, %s", messages, word, end)
	}
	return fmt.Sprintf("[Then came %d %s of work in it, %s", messages, word, end)
}

// withNotes returns row r's blocks with its notes added to the answers
// they belong to. The row's own blocks are not changed.
func (c *conv) withNotes(r int) []core.Block {
	notes := c.notes[r]
	if len(notes) == 0 {
		return c.rows[r].blocks
	}
	out := slices.Clone(c.rows[r].blocks)
	for _, n := range notes {
		for i := range out {
			if out[i].Type == core.BlockToolResult && out[i].ToolUseID == n.call {
				out[i].Content = appendLine(out[i].Content, n.text)
				break
			}
		}
	}
	return out
}

// appendLine returns content with line added to the end of its last text
// block, or as a text block of its own when it has none.
func appendLine(content []core.Block, line string) []core.Block {
	out := slices.Clone(content)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Type != core.BlockText {
			continue
		}
		switch {
		case out[i].Text == "":
			out[i].Text = line
		case strings.HasSuffix(out[i].Text, "\n"):
			out[i].Text += line
		default:
			out[i].Text += "\n" + line
		}
		return out
	}
	return append(out, core.Text(line))
}

// cutInfo describes a cut for the caller.
func (c *conv) cutInfo(cut *cut, trace bool) Cut {
	out := Cut{
		Kind: cut.kind, Frame: cut.frame, Label: cut.label, Result: cut.result,
		Planned: cut.ent < 0, Inner: cut.inner,
		Messages: cut.messages, Tokens: cut.tokens, Spared: cut.spared,
		Note: cut.note, Live: cut.live, Reason: cut.reason,
	}
	if cut.ent >= 0 {
		out.UUID = c.entries[cut.ent].UUID
	}
	if cut.pushEnt >= 0 {
		out.PushUUID = c.entries[cut.pushEnt].UUID
	}
	if trace {
		var src []int
		for r := cut.from; r < cut.to; r++ {
			if !c.stays(cut, r) {
				src = append(src, c.rows[r].ent)
			}
		}
		out.Removed = c.uuids(src)
	}
	return out
}

// report writes the stack and the cuts into info.
func (c *conv) report(st *derived, opts ViewOptions, info *ViewInfo) {
	for _, f := range st.stack {
		info.Stack = append(info.Stack, Frame{
			ID: f.id, Label: f.label, UUID: c.entries[f.ent].UUID, ToolUseID: f.call, Slid: st.slid[f.id],
		})
	}
	for i := range st.cuts {
		cut := &st.cuts[i]
		if c.summary >= 0 && cut.at <= c.start {
			// The cut was made behind the summary: the view shows nothing
			// of it.
			continue
		}
		info.Cuts = append(info.Cuts, c.cutInfo(cut, opts.Trace))
	}
	for _, ent := range st.unmatched {
		info.UnmatchedPops = append(info.UnmatchedPops, c.entries[ent].UUID)
	}
}

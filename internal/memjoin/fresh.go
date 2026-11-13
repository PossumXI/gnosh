package memjoin

import (
	"context"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// turnState is what the join remembers of the turn that is running. It
// is fed by what the loop stores (count), and guarded by the join's lock.
type turnState struct {
	// began is when a prompt last made the idle session busy.
	began time.Time
	// id is the loop's id of the turn, and start when its first prompt
	// was stored.
	id    string
	start time.Time
	// asked is what the person typed in the turn, prompt by prompt.
	asked []string
	// refused holds, by todo, the line that refused the last attempt to
	// close it in this turn, for as long as no later attempt was kept.
	refused map[string]string
	// sentBack says the model was already sent back to a refused close in
	// this turn: that happens once a turn.
	sentBack bool
	// memCalls counts the calls on mem that were kept or answered in this
	// turn. A turn with none has closed nothing in mem.
	memCalls int
}

// prompt notes a prompt the loop stores. A prompt of another turn starts
// the turn's record over.
func (t *turnState) prompt(e agent.Entry) {
	if e.Turn != t.id {
		*t = turnState{began: t.began, id: e.Turn, start: e.Time}
	}
	if e.Hidden {
		return
	}
	if text := strings.TrimSpace(core.TextOf(e.Blocks)); text != "" {
		t.asked = append(t.asked, text)
	}
}

// result notes how a call on mem went: a refused close is remembered by
// its todo, and a kept call that left a todo clears what was remembered of
// it.
func (t *turnState) result(e agent.Entry) {
	text := core.TextOf(e.Result.Content)
	if !e.Interrupted && !e.Denied {
		t.memCalls++
	}
	if e.Result.IsError {
		if line, todo, ok := refusalOf(text); ok {
			if t.refused == nil {
				t.refused = map[string]string{}
			}
			t.refused[todo] = line
		}
		return
	}
	for _, f := range gnovmtools.FramesOf(e.Result.Meta) {
		if f.Kind == "pop" {
			delete(t.refused, f.Id)
		}
	}
}

// since is when the running turn began, for asking mem what the session
// closed in it.
func (t *turnState) since() time.Time {
	if !t.start.IsZero() {
		return t.start
	}
	return t.began
}

// memFresh is the tools' MemFresh slot: the fresh guard's input. It
// returns the todo the session stands in when that todo is fresh, which
// mem then refuses to pop: no work came between the round that entered it
// and the round of this call.
//
// The rows that are judged are the ones a pop made by this call would
// take, whether or not an earlier pop took them already: a previewed pop,
// never stored. Work done since the plan was made counts too: when the
// session stands in a todo of a plan, the same is asked of the todo at
// the bottom of the stack (fix A).
//
// A todo whose entering reply is not in the conversation, or that this
// very reply entered, is not refused by this guard.
func (j *join) memFresh(_ context.Context, _ string, callID string) string {
	stack := j.ts.Stack()
	if len(stack) == 0 {
		return ""
	}
	top := stack[len(stack)-1].ID
	events, known := j.workSince(top, callID)
	if !known || worked(events, top) {
		return ""
	}
	if bottom := stack[0].ID; bottom != top {
		if events, known := j.workSince(bottom, callID); known && worked(events, top) {
			return ""
		}
	}
	return top
}

// workSince returns what happened between the round that entered frame
// and the round of the call callID, as events. known is false when the
// frame's entering reply is not in the conversation, when this reply
// entered it, or when the session does not stand in it.
func (j *join) workSince(frame, callID string) (events []event, known bool) {
	cuts, _ := j.ts.Preview([]transcript.Entry{transcript.PopEntry(frame, "", "", callID)}, transcript.ViewOptions{})
	if len(cuts) != 1 {
		return nil, false
	}
	c := cuts[0]
	if c.PushReply == "" || c.PopReply == "" || c.PushReply == c.PopReply || c.PushCall == "" {
		return nil, false
	}
	return repliesAsEvents(c.Replies), true
}

package agent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// tellingLog is a Log whose owner tells other code of what it has just
// stored, and of the room it has just made, from inside its own methods:
// the way the wiring tells a feature.
type tellingLog struct {
	*slideLog
	stored func(e Entry)
	slid   func()
}

func (l *tellingLog) Append(ctx context.Context, entries ...Entry) error {
	if err := l.slideLog.Append(ctx, entries...); err != nil {
		return err
	}
	for _, e := range entries {
		l.stored(e)
	}
	return nil
}

func (l *tellingLog) MakeRoom(ctx context.Context, req RoomRequest) (Room, error) {
	room, err := l.slideLog.MakeRoom(ctx, req)
	l.slid()
	return room, err
}

// A notice raised by code the log itself is calling is shown at once and
// stored as soon as the log is free: it does not wait for the log it is
// inside of, which would be waiting for ever. The notices are stored in
// the order they were raised, each right after what was being stored.
func TestNoticeFromInsideTheLog(t *testing.T) {
	var a atomic.Pointer[Agent]
	inner := &slideLog{memLog: newMemLog()}
	log := &tellingLog{slideLog: inner}
	log.stored = func(e Entry) {
		if e.Kind == EntryResult {
			a.Load().Notice(LevelInfo, "feature", "stored the result of "+e.ToolUseID)
			a.Load().Notice(LevelInfo, "feature", "and told of "+e.ToolUseID+" again")
		}
	}
	log.slid = func() { a.Load().Notice(LevelInfo, "feature", "room was made") }
	j := &job{want: 40}
	r := newRig(t, func(o *Options) {
		o.Model.Info.ContextWindow = 0
		o.Log = log
		o.Tools = []core.Tool{fileTool(1500)}
	})
	a.Store(r.a)
	r.log = inner.memLog
	r.model.honest = true
	r.model.window = UnknownWindow
	r.model.answer(j.respond)

	// The rig's own bound makes a deadlock a failure and not a hang.
	if res := r.run("read forty files"); res.Reason != EndCompleted {
		t.Fatalf("result = %+v", res)
	}
	r.idle()

	results, rooms := 0, 0
	entries := inner.memLog.all()
	for i, e := range entries {
		switch {
		case e.Kind == EntryNotice && e.Source == "feature" && e.Text == "room was made":
			rooms++
		case e.Kind == EntryResult:
			results++
			if i+2 >= len(entries) ||
				entries[i+1].Kind != EntryNotice || entries[i+1].Text != "stored the result of "+e.ToolUseID ||
				entries[i+2].Kind != EntryNotice || entries[i+2].Text != "and told of "+e.ToolUseID+" again" {
				t.Errorf("the result of %s is not followed by its two notices, in order", e.ToolUseID)
			}
		}
	}
	if results != 40 {
		t.Errorf("%d results were stored, want 40", results)
	}
	if asked := len(inner.asked); asked == 0 || rooms != asked {
		t.Errorf("room was asked for %d times, and %d notices raised from inside it were stored", asked, rooms)
	}
	// Every one was shown, too.
	shown := 0
	for _, ev := range r.rec.all() {
		if n, ok := ev.(*NoticeEvent); ok && n.Source == "feature" {
			shown++
		}
	}
	if shown != 2*results+rooms {
		t.Errorf("%d notices were shown, want %d", shown, 2*results+rooms)
	}
	if strings.TrimSpace(r.a.State().Waiting) != "" && r.a.State().Busy {
		t.Errorf("the session is still busy: %+v", r.a.State())
	}
}

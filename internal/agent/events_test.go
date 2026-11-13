package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// checkEvents fails the test unless a stream of events is one a front end
// can follow: numbered without gaps, busy stretches and turns properly
// opened and closed, every tool call started before it ends and ended
// exactly once, and live output only while its request or its call is
// open.
func checkEvents(t *testing.T, evs []Event) {
	t.Helper()
	fail := func(i int, format string, args ...any) {
		t.Helper()
		from := max(0, i-6)
		t.Errorf("event %d: %s\n… %s", i, fmt.Sprintf(format, args...), typesOf(evs[from:min(len(evs), i+3)]))
	}
	type place struct{ agent, turn string }
	busy := false
	turns := map[string]string{}   // agent -> the open turn
	asking := map[place]bool{}     // a request is out
	started := map[string]bool{}   // tool calls started
	ended := map[string]bool{}     // tool calls ended
	questions := map[string]bool{} // questions that wait

	for i, ev := range evs {
		m := ev.EventMeta()
		if m.Seq != uint64(i+1) {
			t.Fatalf("event %d (%T) is numbered %d", i, ev, m.Seq)
		}
		if m.Time.IsZero() {
			fail(i, "%T has no time", ev)
		}
		at := place{m.Agent, m.Turn}
		switch e := ev.(type) {
		case *BusyEvent:
			if m.Agent == "" {
				if busy {
					fail(i, "busy twice")
				}
				busy = true
			}
		case *IdleEvent:
			if m.Agent == "" {
				if !busy {
					fail(i, "idle without busy")
				}
				if turns[""] != "" {
					fail(i, "idle inside a turn")
				}
				busy = false
			}
		case *TurnStartEvent:
			if m.Agent == "" && !busy {
				fail(i, "a turn started in an idle session")
			}
			if turns[m.Agent] != "" {
				fail(i, "a turn started inside a turn")
			}
			if m.Turn == "" {
				fail(i, "a turn without an id")
			}
			turns[m.Agent] = m.Turn
		case *TurnEndEvent:
			if turns[m.Agent] != m.Turn || m.Turn == "" {
				fail(i, "the end of turn %q, but %q is open", m.Turn, turns[m.Agent])
			}
			if e.Result.Turn != m.Turn {
				fail(i, "the result names another turn")
			}
			if asking[at] {
				fail(i, "a turn ended with a request still out")
			}
			turns[m.Agent] = ""
		case *RequestEvent:
			if e.Purpose == "main" || e.Purpose == "summary" {
				if asking[at] {
					fail(i, "a request went out while another was out")
				}
				asking[at] = true
			}
		case *ReplyEvent:
			if !asking[at] {
				fail(i, "a reply without a request")
			}
			asking[at] = false
		case *RetryEvent:
			asking[at] = false
		case *TextEvent, *ThinkingEvent, *ToolCallEvent, *ToolInputEvent:
			if !asking[at] {
				fail(i, "%T outside a request", ev)
			}
		case *ToolStartEvent:
			if started[m.Agent+e.CallID] {
				fail(i, "call %s started twice", e.CallID)
			}
			started[m.Agent+e.CallID] = true
			if turns[m.Agent] == "" {
				fail(i, "a tool call outside a turn")
			}
		case *ToolProgressEvent:
			if !started[m.Agent+e.CallID] || ended[m.Agent+e.CallID] {
				fail(i, "output of call %s outside its run", e.CallID)
			}
		case *ToolEndEvent:
			if !started[m.Agent+e.CallID] {
				fail(i, "call %s ended before it started", e.CallID)
			}
			if ended[m.Agent+e.CallID] {
				fail(i, "call %s ended twice", e.CallID)
			}
			ended[m.Agent+e.CallID] = true
		case *QuestionEvent:
			questions[e.Question.ID] = true
		case *AnswerEvent:
			if !questions[e.ID] {
				fail(i, "an answer to a question that was never put")
			}
			delete(questions, e.ID)
		}
		if m.Turn != "" && turns[m.Agent] != m.Turn {
			switch ev.(type) {
			case *TurnEndEvent, *NoticeEvent, *ToolProgressEvent, *QueueEvent, *PromptReturnedEvent, *SideEvent, *RequestEvent, *LoopEvent, *LostEvent:
				// These may trail a turn or come from beside it.
			default:
				fail(i, "%T names turn %q, but %q is open", ev, m.Turn, turns[m.Agent])
			}
		}
	}
	if busy {
		t.Errorf("the stream ends in a busy stretch")
	}
	for id := range started {
		if !ended[id] {
			t.Errorf("call %s never ended", id)
		}
	}
	for agent, turn := range turns {
		if turn != "" {
			t.Errorf("turn %s of agent %q never ended", turn, agent)
		}
	}
	if len(questions) != 0 {
		t.Errorf("%d questions were never closed", len(questions))
	}
}

// longReply is a reply that arrives in many small pieces.
func longReply(pieces int) (step, string) {
	var evs []core.Event
	var text strings.Builder
	for i := 0; i < pieces; i++ {
		piece := fmt.Sprintf("w%d ", i)
		text.WriteString(piece)
		evs = append(evs, core.Event{Type: core.EventTextDelta, Text: piece})
	}
	return step{evs: append(evs, usageEv(100, pieces), stopEv(core.StopEndTurn))}, text.String()
}

// A consumer that is slow does not stall the loop, and loses nothing:
// pieces of live output that wait are joined, in order.
func TestSlowConsumerStallsNothingAndLosesNothing(t *testing.T) {
	chatty := look("Chatty", func(_ context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
		for i := 0; i < 300; i++ {
			env.Progress(core.Progress{Stream: "stdout", Text: fmt.Sprintf("%s:%d\n", call.ID, i)})
		}
		return core.TextResult("chatted"), nil
	})
	r := newRig(t, func(o *Options) { o.Tools = []core.Tool{chatty} })
	r.rec.delay = time.Millisecond
	thinking := step{evs: []core.Event{{Type: core.EventThinkingDelta, Text: "let me "}, {Type: core.EventThinkingDelta, Text: "think"}}}
	calls := use(tc("a", "Chatty", `{}`), tc("b", "Chatty", `{}`))
	calls.evs = append(thinking.evs, calls.evs...)
	reply, want := longReply(3000)
	r.model.push(calls, reply)

	began := time.Now()
	res, err := r.a.Run(context.Background(), Prompt{Text: "talk a lot"})
	took := time.Since(began)
	if err != nil || res.Reason != EndCompleted || res.Text != want {
		t.Fatalf("result = %+v, %v", res.Reason, err)
	}
	// Handing 3,600 pieces to this consumer one at a time would take
	// seconds. The turn did not wait for that.
	if took > 1500*time.Millisecond {
		t.Errorf("the turn took %s: the loop waited for the consumer", took)
	}
	r.idle()

	evs := r.rec.all()
	checkEvents(t, evs)
	var text, thought strings.Builder
	progress := map[string]*strings.Builder{"a": {}, "b": {}}
	pieces := 0
	for _, ev := range evs {
		switch e := ev.(type) {
		case *TextEvent:
			text.WriteString(e.Text)
			pieces++
		case *ThinkingEvent:
			thought.WriteString(e.Text)
		case *ToolProgressEvent:
			progress[e.CallID].WriteString(e.Text)
		case *LostEvent:
			t.Errorf("events were thrown away: %+v", e)
		}
	}
	if text.String() != want {
		t.Errorf("the streamed text lost something: %d bytes, want %d", text.Len(), len(want))
	}
	if thought.String() != "let me think" {
		t.Errorf("thinking = %q", thought.String())
	}
	for id, b := range progress {
		var wantOut strings.Builder
		for i := 0; i < 300; i++ {
			fmt.Fprintf(&wantOut, "%s:%d\n", id, i)
		}
		if b.String() != wantOut.String() {
			t.Errorf("the live output of call %s lost something or is out of order", id)
		}
	}
	if pieces >= 3000 {
		t.Errorf("%d text events reached a consumer that fell behind; they should have been joined", pieces)
	}
}

// When a consumer falls so far behind that the waiting output passes the
// backlog, the oldest pieces are thrown away and a LostEvent says so.
// Nothing else is ever thrown away, and the whole reply still arrives.
func TestBacklogShedsOnlyLiveOutputAndSaysSo(t *testing.T) {
	gate := make(chan struct{})
	var rec recorder
	r := newRig(t, func(o *Options) {
		o.Tools = []core.Tool{tool("Bash", nil)}
		o.EventBacklog = 2000
		o.OnEvent = func(ev Event) {
			<-gate // the consumer is stuck
			rec.on(ev)
		}
	})
	r.rec = &rec
	reply, want := longReply(4000)
	r.model.push(use(tc("c", "Bash", `{}`)), reply)

	res, err := r.a.Run(context.Background(), Prompt{Text: "talk a lot"})
	if err != nil || res.Reason != EndCompleted || res.Text != want {
		t.Fatalf("the turn did not finish beside a stuck consumer: %+v, %v", res.Reason, err)
	}
	close(gate)
	r.idle()

	evs := rec.all()
	checkEvents(t, evs)
	shown, lost, lostEvents := 0, 0, 0
	for _, ev := range evs {
		switch e := ev.(type) {
		case *TextEvent:
			shown += len(e.Text)
		case *ToolInputEvent:
			shown += len(e.Text)
		case *LostEvent:
			lost += e.Bytes
			lostEvents += e.Events
		}
	}
	if lost == 0 || lostEvents == 0 {
		t.Fatal("nothing was thrown away though the backlog was passed")
	}
	// The live output was the reply's text and the arguments of one call.
	if total := len(want) + len(`{}`); shown+lost != total {
		t.Errorf("%d bytes shown and %d reported lost, of %d: some went missing without a word", shown, lost, total)
	}
	// Everything but live output arrived, and the reply arrived whole.
	replies := eventsOf[*ReplyEvent](&rec)
	if len(replies) != 2 || replies[1].Message.Text() != want {
		t.Errorf("the whole reply did not arrive")
	}
	if n := len(eventsOf[*ToolEndEvent](&rec)); n != 1 {
		t.Errorf("%d tool ends", n)
	}
	if n := len(eventsOf[*TurnEndEvent](&rec)); n != 1 {
		t.Errorf("%d turn ends", n)
	}
}

// Without a consumer function, the events arrive on a channel, which is
// closed when the session is closed and everything has been read.
func TestEventsChannel(t *testing.T) {
	r := newRig(t, func(o *Options) { o.OnEvent = nil })
	ch := r.a.Events()
	if ch == nil {
		t.Fatal("Events() is nil")
	}
	var mu sync.Mutex
	var evs []Event
	done := make(chan struct{})
	go func() {
		defer close(done)
		for ev := range ch {
			mu.Lock()
			evs = append(evs, ev)
			mu.Unlock()
		}
	}()
	r.model.push(say("hello"))
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	if res, err := r.a.Run(ctx, Prompt{Text: "hi"}); err != nil || res.Reason != EndCompleted {
		t.Fatalf("result = %+v, %v", res, err)
	}
	if err := r.a.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	r.a.Close()
	select {
	case <-done:
	case <-time.After(bound):
		t.Fatal("the channel was not closed")
	}
	mu.Lock()
	defer mu.Unlock()
	checkEvents(t, evs)
	inOrder(t, evs, "BusyEvent", "TurnStartEvent", "TextEvent", "ReplyEvent", "TurnEndEvent", "IdleEvent")
}

// The events of a full turn, with everything in it, are in an order a
// front end can follow.
func TestEventsOfAFullTurnAreInOrder(t *testing.T) {
	bash, started, letGo := heldTool("Bash")
	r := newRig(t, func(o *Options) {
		o.Tools = []core.Tool{bash, look("Read", nil)}
		o.Gate = GateFunc(func(ctx context.Context, req GateRequest) (Decision, error) {
			if req.Call.Name == "Bash" {
				ans, err := req.Ask(ctx, Question{Kind: "permission", Text: "run it?"})
				return Decision{Allow: ans.Choice == "yes"}, err
			}
			return Decision{Allow: true}, nil
		})
		o.Drivers = []Driver{{Name: "stop-hook", Decide: func(_ context.Context, end TurnEnd) (Verdict, error) {
			if end.Streak == 0 {
				return Verdict{Action: Continue, Prompt: "check once more"}, nil
			}
			return Verdict{}, nil
		}}}
	})
	r.model.push(
		use(tc("r1", "Read", `{}`), tc("r2", "Read", `{}`), tc("b1", "Bash", `{}`)),
		broken(core.ErrServer, 500),
		silence(),
		say("done"),
		say("checked"),
	)
	done := make(chan TurnResult, 1)
	go func() { done <- r.run("do it all") }()
	q := waitEvent[*QuestionEvent](t, r.rec, nil)
	r.a.Answer(q.Question.ID, Answer{Choice: "yes"})
	<-started
	r.a.Submit(Prompt{Text: "and one more thing"})
	waitQueue(t, r, "and one more thing")
	letGo()
	if res := <-done; res.Reason != EndCompleted || res.Text != "checked" {
		t.Fatalf("result = %+v", res)
	}
	r.idle()
	evs := r.rec.all()
	checkEvents(t, evs)
	inOrder(t, evs,
		"BusyEvent", "TurnStartEvent", "PromptEvent", "RequestEvent", "ToolCallEvent", "ReplyEvent",
		"ToolStartEvent", "ToolEndEvent", "QuestionEvent", "AnswerEvent", "QueueEvent", "ToolEndEvent",
		"PromptEvent", "RequestEvent", "RetryEvent", "RequestEvent", "ReplyEvent", "PromptEvent",
		"RequestEvent", "TextEvent", "ReplyEvent", "DriverEvent", "PromptEvent", "RequestEvent", "ReplyEvent",
		"TurnEndEvent", "IdleEvent")
	r.checkLog()
}

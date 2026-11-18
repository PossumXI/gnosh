package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// stopPoint is one moment of a turn at which the person stops it.
type stopPoint struct {
	name string
	// setup prepares the options and the model's script, and returns a
	// channel that is closed when the turn has reached the point.
	setup func(o *Options, m *fakeModel) <-chan struct{}
	// prime, when set, runs a first turn before the one that is stopped.
	prime func(r *rig)
	// want is how the stopped prompt ends.
	want EndReason
	// check looks at what the stop left behind.
	check func(t *testing.T, r *rig)
}

// once returns a function that closes ch the first time it is called.
func once(ch chan struct{}) func() {
	var o sync.Once
	return func() { o.Do(func() { close(ch) }) }
}

func TestInterruptAtEveryPoint(t *testing.T) {
	block := func(ctx context.Context, reached func()) {
		reached()
		<-ctx.Done()
	}
	results := func(r *rig) []string {
		var out []string
		for _, e := range r.log.all() {
			if e.Kind == EntryResult {
				out = append(out, core.TextOf(e.Result.Content))
			}
		}
		return out
	}
	points := []stopPoint{
		{
			name: "while the prompt's hook runs",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Moments.PromptSubmit = func(ctx context.Context, p Prompt) (Admission, error) {
					if p.Text == "stop me" {
						block(ctx, once(ch))
					}
					return Admission{}, ctx.Err()
				}
				return ch
			},
			want: EndWithdrawn,
			check: func(t *testing.T, r *rig) {
				ret := eventsOf[*PromptReturnedEvent](r.rec)
				if len(ret) != 1 || ret[0].Why != ReturnStopped || ret[0].Prompt.Text != "stop me" {
					t.Errorf("the prompt did not come back: %+v", ret)
				}
				if len(r.log.all()) != 0 || r.model.count() != 0 {
					t.Errorf("a prompt stopped in its hook left something behind: %v", r.log.kinds())
				}
			},
		},
		{
			name: "while the session becomes busy",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Moments.Busy = func(ctx context.Context) func() {
					if FromContext(ctx) == nil {
						panic("the busy moment was not given its session")
					}
					block(ctx, once(ch))
					return func() {}
				}
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				if r.model.count() != 0 {
					t.Error("the model was called")
				}
				if k := r.log.kinds(); len(k) != 2 || k[0] != EntryPrompt || k[1] != EntryTurn {
					t.Errorf("the prompt should be kept as a stopped turn: %v", k)
				}
			},
		},
		{
			name: "while the prompt's lines are made",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Moments.PromptLines = []func(context.Context, LineInfo) ([]Part, error){
					func(ctx context.Context, info LineInfo) ([]Part, error) {
						if info.Prompt.Text == "stop me" {
							block(ctx, once(ch))
						}
						return nil, ctx.Err()
					},
				}
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				if k := r.log.kinds(); len(k) != 2 || k[0] != EntryPrompt {
					t.Errorf("log = %v", k)
				}
			},
		},
		{
			name: "before the reply starts",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				m.push(step{at: once(ch), stall: true})
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				// Nothing had arrived, and the reply is still stored, as
				// cancelled: that is what ends the turn in the log.
				k := r.log.kinds()
				as := r.log.of(EntryAssistant)
				if len(k) != 3 || k[2] != EntryTurn || len(as) != 1 || !as[0].Silent || as[0].StopReason != core.StopCancelled || len(as[0].Message.Blocks) != 0 {
					t.Errorf("log = %v, reply %+v", k, as)
				}
			},
		},
		{
			name: "in the middle of the answer",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				m.push(step{
					evs: []core.Event{{Type: core.EventThinkingDelta, Text: "hmm"}, {Type: core.EventTextDelta, Text: "The answer is"}},
					mid: once(ch), stall: true,
				})
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				as := r.log.all()[1]
				if as.Kind != EntryAssistant || as.StopReason != core.StopCancelled || as.Message.Text() != "The answer is" || as.Silent {
					t.Errorf("what arrived was not kept: %+v", as)
				}
				rep := eventsOf[*ReplyEvent](r.rec)
				if len(rep) != 1 || rep[0].StopReason != core.StopCancelled {
					t.Errorf("reply events = %+v", rep)
				}
			},
		},
		{
			name: "while only reasoning has arrived",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				m.push(step{evs: []core.Event{{Type: core.EventThinkingDelta, Text: "let me think"}}, mid: once(ch), stall: true})
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				as := r.log.all()[1]
				if as.Kind != EntryAssistant || !as.Silent || as.StopReason != core.StopCancelled {
					t.Errorf("entry = %+v", as)
				}
			},
		},
		{
			name: "in the middle of a tool call's arguments",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Tools = []core.Tool{tool("Edit", func(context.Context, core.ToolCall, *core.ToolEnv) (core.ToolResult, error) {
					panic("a call whose reply was cut must not run")
				})}
				m.push(step{
					evs: append(callEvents(tc("whole", "Edit", `{"a":1}`)),
						core.Event{Type: core.EventToolUseStart, ID: "half", Name: "Edit"},
						core.Event{Type: core.EventToolUseDelta, ID: "half", Text: `{"file_path": "/x", "new_str`}),
					mid: once(ch), stall: true,
				})
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				as := r.log.all()[1]
				uses := as.Message.ToolUses()
				if len(uses) != 2 || string(uses[1].Input) != "{}" || !strings.Contains(as.BadInputs["half"], "new_str") {
					t.Errorf("the cut call was not kept as it arrived: %+v %v", uses, as.BadInputs)
				}
				res := results(r)
				if len(res) != 2 || !strings.Contains(res[0], "cut off") || !strings.Contains(res[1], "cut off") {
					t.Errorf("results = %q", res)
				}
			},
		},
		{
			name: "while a tool runs",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Tools = []core.Tool{
					tool("Bash", func(ctx context.Context, _ core.ToolCall, _ *core.ToolEnv) (core.ToolResult, error) {
						block(ctx, once(ch))
						// A tool may hand back what it had when it was cut.
						return core.ErrorResult("the command was stopped after printing: line 1"), nil
					}),
					tool("Edit", nil),
				}
				m.push(use(tc("b", "Bash", `{}`), tc("e", "Edit", `{}`)))
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				res := results(r)
				if len(res) != 2 || !strings.Contains(res[0], "line 1") || !strings.Contains(res[1], "the user stopped the turn first") {
					t.Errorf("results = %q", res)
				}
			},
		},
		{
			name: "while several tools run side by side",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				var started sync.WaitGroup
				started.Add(3)
				go func() { started.Wait(); close(ch) }()
				o.Tools = []core.Tool{look("Look", func(ctx context.Context, _ core.ToolCall, _ *core.ToolEnv) (core.ToolResult, error) {
					started.Done()
					<-ctx.Done()
					return core.ToolResult{}, ctx.Err()
				})}
				m.push(use(tc("a", "Look", `{}`), tc("b", "Look", `{}`), tc("c", "Look", `{}`)))
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				res := results(r)
				if len(res) != 3 {
					t.Fatalf("results = %q", res)
				}
				for _, s := range res {
					if !strings.Contains(s, "while this call was running") {
						t.Errorf("result = %q", s)
					}
				}
			},
		},
		{
			name: "while a question waits for the person",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Tools = []core.Tool{tool("Bash", func(context.Context, core.ToolCall, *core.ToolEnv) (core.ToolResult, error) {
					panic("a call whose question was never answered must not run")
				})}
				o.Gate = GateFunc(func(ctx context.Context, req GateRequest) (Decision, error) {
					go func() {
						// The question is up once its event is out.
						time.Sleep(5 * time.Millisecond)
						once(ch)()
					}()
					if _, err := req.Ask(ctx, Question{Kind: "permission", Text: "run it?"}); err != nil {
						return Decision{}, err
					}
					return Decision{Allow: true}, nil
				})
				m.push(use(tc("b", "Bash", `{}`)))
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				ans := eventsOf[*AnswerEvent](r.rec)
				if len(ans) != 1 || !ans[0].Withdrawn {
					t.Errorf("the question was not taken back: %+v", ans)
				}
				if res := results(r); len(res) != 1 || !strings.Contains(res[0], "the user stopped the turn first") {
					t.Errorf("results = %q", res)
				}
			},
		},
		{
			name: "between two rounds",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Tools = []core.Tool{tool("Bash", nil)}
				o.Moments.BeforeRequest = []func(context.Context, RequestInfo) ([]Part, error){
					func(ctx context.Context, info RequestInfo) ([]Part, error) {
						if info.Round == 2 {
							block(ctx, once(ch))
						}
						return nil, nil
					},
				}
				m.push(use(tc("b", "Bash", `{}`)))
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				if res := results(r); len(res) != 1 || !strings.Contains(res[0], "Bash ran") {
					t.Errorf("the finished round was not kept: %q", res)
				}
				if r.model.count() != 1 {
					t.Errorf("%d requests", r.model.count())
				}
			},
		},
		{
			name: "while a turn-end check runs",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Drivers = []Driver{{Name: "stop-hook", Decide: func(ctx context.Context, end TurnEnd) (Verdict, error) {
					if strings.Contains(end.Text, "nearly") {
						block(ctx, once(ch))
					}
					return Verdict{}, nil
				}}}
				m.push(say("nearly done"))
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				as := r.log.all()[1]
				if as.Kind != EntryAssistant || as.StopReason != core.StopEndTurn {
					t.Errorf("the finished reply was not kept: %+v", as)
				}
			},
		},
		{
			name: "while waiting to try a failed request again",
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				o.Retry = RetryPolicy{Pauses: []time.Duration{time.Hour}, GoneAfter: 2 * time.Hour}
				m.push(step{
					evs: []core.Event{{Type: core.EventTextDelta, Text: "half an"}},
					mid: func() {
						go func() {
							time.Sleep(5 * time.Millisecond)
							once(ch)()
						}()
					},
					err: &core.ProviderError{Provider: "fake", Kind: core.ErrServer, Status: 500},
				})
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				retry := eventsOf[*RetryEvent](r.rec)
				if len(retry) != 1 || retry[0].Wait != time.Hour || !retry[0].Discarded {
					t.Errorf("retry events = %+v", retry)
				}
				if r.model.count() != 1 {
					t.Errorf("%d requests", r.model.count())
				}
			},
		},
		{
			name: "while a summary is written before the prompt goes out",
			prime: func(r *rig) {
				r.model.push(say(strings.Repeat("a long answer ", 3600)))
				if res := r.run("first"); res.Reason != EndCompleted {
					r.t.Fatalf("priming turn: %+v", res)
				}
			},
			setup: func(o *Options, m *fakeModel) <-chan struct{} {
				ch := make(chan struct{})
				// The first turn leaves the conversation past the point of
				// this window, so the second prompt needs a summary first.
				o.Model.Info.ContextWindow = 20000
				m.honest = true
				m.respond = func(req *core.Request, n int) step {
					if strings.Contains(lastUserText(req), "Write a summary") {
						return step{at: once(ch), stall: true}
					}
					return say("unexpected")
				}
				return ch
			},
			want: EndStopped,
			check: func(t *testing.T, r *rig) {
				var prompts []string
				for _, e := range r.log.all() {
					if e.Kind == EntryPrompt {
						prompts = append(prompts, core.TextOf(e.Blocks))
					}
					if e.Kind == EntrySummary {
						t.Error("a summary that was stopped was stored")
					}
				}
				if len(prompts) != 2 || prompts[1] != "stop me" {
					t.Errorf("the waiting prompt was not kept as a stopped turn: %q", prompts)
				}
			},
		},
	}

	for _, p := range points {
		t.Run(p.name, func(t *testing.T) {
			var reached <-chan struct{}
			var model *fakeModel
			r := newRig(t, func(o *Options) {
				model = o.Model.Provider.(*fakeModel)
				reached = p.setup(o, model)
			})
			if p.prime != nil {
				p.prime(r)
			}

			done := make(chan TurnResult, 1)
			go func() { done <- r.run("stop me") }()
			select {
			case <-reached:
			case <-time.After(bound):
				t.Fatalf("the turn never reached the point; state %+v; events %s", r.a.State(), typesOf(r.rec.all()))
			}
			began := time.Now()
			r.a.Interrupt()
			res := <-done
			if took := time.Since(began); took > 2*time.Second {
				t.Errorf("stopping took %s", took)
			}
			r.idle()

			if res.Reason != p.want {
				t.Fatalf("the prompt ended as %q, want %q: %+v", res.Reason, p.want, res)
			}
			if res.Reason == EndStopped {
				ends := eventsOf[*TurnEndEvent](r.rec)
				last := ends[len(ends)-1].Result
				if last.Reason != EndStopped || last.Err != nil {
					t.Errorf("turn end = %+v", last)
				}
			}
			for _, n := range eventsOf[*NoticeEvent](r.rec) {
				if strings.Contains(strings.ToLower(n.Text), "context canceled") {
					t.Errorf("the person was shown %q", n.Text)
				}
			}
			r.checkLog()
			if p.check != nil {
				p.check(t, r)
			}

			// A new process picks the log up and carries on.
			next := &fakeModel{name: "fake"}
			next.push(say("resumed"))
			rec := &recorder{}
			b, err := New(Options{
				SessionID: "session-1", Model: Model{Provider: next, Info: core.ModelInfo{ID: "big", ContextWindow: 100000}},
				Log: r.log, Gate: AllowAll, OnEvent: rec.on, NewID: testID,
			})
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			ctx, cancel := context.WithTimeout(context.Background(), bound)
			defer cancel()
			got, err := b.Run(ctx, Prompt{Text: "go on"})
			if err != nil || got.Reason != EndCompleted || got.Text != "resumed" {
				t.Fatalf("the next process could not carry on: %+v %v\n%s", got, err, dumpLog(r.log))
			}
			reqs := next.requests()
			if len(reqs) != 1 {
				t.Fatalf("%d requests in the next process", len(reqs))
			}
			if err := checkView(reqs[0].Messages); err != nil {
				t.Errorf("the next process sent a conversation that is not valid: %v", err)
			}
			if !strings.Contains(lastUserText(reqs[0]), "go on") {
				t.Errorf("the new prompt is not at the end: %q", lastUserText(reqs[0]))
			}
			checkLogValid(t, r.log)
		})
	}
}

// Stopping twice, stopping an idle session and stopping after the end are
// all harmless.
func TestInterruptWhenThereIsNothingToStop(t *testing.T) {
	r := newRig(t, nil)
	r.a.Interrupt()
	r.a.Interrupt()
	r.model.push(say("one"))
	if res := r.run("first"); res.Reason != EndCompleted {
		t.Fatalf("a stop before the turn reached into it: %+v", res)
	}
	r.a.Interrupt()
	r.model.push(say("two"))
	if res := r.run("second"); res.Reason != EndCompleted {
		t.Fatalf("a stop between turns reached into the next: %+v", res)
	}
	r.checkLog()
}

// A provider that does not end its answer when the request is cancelled
// cannot keep the turn from stopping.
func TestProviderThatDoesNotStop(t *testing.T) {
	letGo := make(chan struct{})
	reached := make(chan struct{})
	r := newRig(t, func(o *Options) { o.StopGrace = 20 * time.Millisecond })
	r.model.push(step{evs: []core.Event{{Type: core.EventTextDelta, Text: "so far"}}, mid: once(reached), hang: letGo})
	defer close(letGo)

	done := make(chan TurnResult, 1)
	go func() { done <- r.run("ask the deaf server") }()
	<-reached
	r.a.Interrupt()
	res := <-done
	if res.Reason != EndStopped {
		t.Fatalf("result = %+v", res)
	}
	if as := r.log.of(EntryAssistant); len(as) != 1 || as[0].Message.Text() != "so far" || as[0].StopReason != core.StopCancelled {
		t.Errorf("entries = %+v", as)
	}
	waitEvent(t, r.rec, func(n *NoticeEvent) bool { return strings.Contains(n.Text, "had not ended its reply") })
	r.checkLog()
}

// Closing a session in the middle of a turn leaves a log that the next
// process can pick up.
func TestCloseMidTurn(t *testing.T) {
	started := make(chan struct{})
	var ended []EndInfo
	r := newRig(t, func(o *Options) {
		o.Tools = []core.Tool{look("Wait", func(ctx context.Context, _ core.ToolCall, _ *core.ToolEnv) (core.ToolResult, error) {
			close(started)
			<-ctx.Done()
			return core.ToolResult{}, ctx.Err()
		})}
		o.Moments.SessionEnd = func(_ context.Context, info EndInfo) { ended = append(ended, info) }
	})
	r.model.push(use(tc("w", "Wait", `{}`)))
	if _, err := r.a.Submit(Prompt{Text: "work"}); err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := r.a.Submit(Prompt{Text: "queued behind"}); err != nil {
		t.Fatal(err)
	}
	waitQueue(t, r, "queued behind")
	if err := r.a.End("prompt_input_exit"); err != nil {
		t.Fatal(err)
	}
	r.a.Close() // a second close does nothing

	r.checkLog()
	ends := eventsOf[*TurnEndEvent](r.rec)
	if len(ends) != 1 || ends[0].Result.Reason != EndStopped {
		t.Errorf("turn ends = %+v", ends)
	}
	ret := eventsOf[*PromptReturnedEvent](r.rec)
	if len(ret) != 1 || ret[0].Why != ReturnClosed || ret[0].Prompt.Text != "queued behind" {
		t.Errorf("the queued prompt did not come back: %+v", ret)
	}
	if len(ended) != 1 || ended[0].Reason != "prompt_input_exit" {
		t.Errorf("SessionEnd was told %+v", ended)
	}
	if n := len(eventsOf[*IdleEvent](r.rec)); n != 1 {
		t.Errorf("the busy stretch was closed %d times: %s", n, typesOf(r.rec.all()))
	}
}

// A session that never had a message does not end.
func TestSessionEndNeedsAMessage(t *testing.T) {
	called := 0
	r := newRig(t, func(o *Options) {
		o.Moments.SessionEnd = func(context.Context, EndInfo) { called++ }
	})
	r.a.Close()
	if called != 0 {
		t.Errorf("SessionEnd ran for a session that never had a message")
	}
}

// TurnContext is the running turn's context, which stopping the turn
// ends, and nil when no turn runs.
func TestTurnContext(t *testing.T) {
	reached := make(chan context.Context, 1)
	r := newRig(t, func(o *Options) {
		o.Tools = []core.Tool{tool("Bash", func(ctx context.Context, _ core.ToolCall, _ *core.ToolEnv) (core.ToolResult, error) {
			reached <- FromContext(ctx).TurnContext()
			<-ctx.Done()
			return core.ErrorResult("stopped"), nil
		})}
	})
	if r.a.TurnContext() != nil {
		t.Fatal("a turn context with no turn running")
	}
	r.model.push(use(tc("b", "Bash", `{}`)))
	if _, err := r.a.Submit(Prompt{Text: "go"}); err != nil {
		t.Fatal(err)
	}
	var turn context.Context
	select {
	case turn = <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("the tool did not run")
	}
	if turn == nil || turn.Err() != nil {
		t.Fatalf("the turn's context while it runs: %v", turn)
	}
	r.a.Interrupt()
	select {
	case <-turn.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("stopping the turn did not end its context")
	}
	r.idle()
	if r.a.TurnContext() != nil {
		t.Error("a turn context after the turn")
	}
}

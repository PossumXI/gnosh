package agent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// agentTool is a tool that hands a task to a sub-agent, the way the wiring
// builds one: it takes its own session from the context, spawns, runs the
// task and closes the sub-agent.
func agentTool(logs *[]*memLog, mod func(*SubOptions)) *fakeTool {
	var mu sync.Mutex
	return tool("Agent", func(ctx context.Context, call core.ToolCall, _ *core.ToolEnv) (core.ToolResult, error) {
		var args struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(call.Input, &args); err != nil {
			return core.ErrorResult("bad arguments"), nil
		}
		log := newMemLog()
		mu.Lock()
		*logs = append(*logs, log)
		mu.Unlock()
		sub := SubOptions{
			CallID: call.ID, Type: "Explore", Log: log,
			Tools:  []core.Tool{look("Grep", nil)},
			System: []string{"You explore the code and change nothing."},
		}
		if mod != nil {
			mod(&sub)
		}
		child, err := FromContext(ctx).Spawn(sub)
		if err != nil {
			return core.ToolResult{}, err
		}
		defer child.Close()
		res, err := child.Run(ctx, Prompt{Text: args.Prompt})
		if err != nil {
			return core.ToolResult{}, err
		}
		switch res.Reason {
		case EndCompleted:
			return core.TextResult(res.Text), nil
		case EndHalted:
			out := core.ErrorResult("The sub-agent was ended: " + res.Detail)
			out.Meta = map[string]any{MetaHalt: "the sub-agent was ended: " + res.Detail}
			return out, nil
		}
		return core.ErrorResult("The sub-agent did not finish (" + string(res.Reason) + "). What it had: " + res.Text), nil
	})
}

// A sub-agent is a session of its own, run by this same loop: a fresh
// conversation in its own log, with its events sent to the parent's
// consumer under the id of the call that started it.
func TestSubAgent(t *testing.T) {
	var logs []*memLog
	var mu sync.Mutex
	var hooked []ToolDone
	var gated []GateRequest
	var started []StartInfo
	r := newRig(t, func(o *Options) {
		o.System = []string{"the parent's instructions"}
		o.Tools = []core.Tool{agentTool(&logs, func(sub *SubOptions) {
			sub.Moments.SessionStart = func(_ context.Context, info StartInfo) ([]Part, error) {
				mu.Lock()
				started = append(started, info)
				mu.Unlock()
				return []Part{{Source: "SubagentStart", Text: "<start>be thorough</start>"}}, nil
			}
		})}
		o.Moments.AfterTool = func(_ context.Context, d ToolDone) (ToolVerdict, error) {
			mu.Lock()
			hooked = append(hooked, d)
			mu.Unlock()
			return ToolVerdict{}, nil
		}
		o.Gate = GateFunc(func(_ context.Context, req GateRequest) (Decision, error) {
			mu.Lock()
			gated = append(gated, req)
			mu.Unlock()
			return Decision{Allow: true}, nil
		})
	})
	// The parent and the sub-agent share the model: the script is the
	// order in which they speak.
	r.model.push(
		use(tc("a1", "Agent", `{"prompt":"find the port the server listens on"}`)),
		use(tc("g1", "Grep", `{"pattern":"Listen"}`)),
		say("The server listens on port 8080 (main.go:12)."),
		say("It listens on 8080."),
	)
	res := r.run("which port?")
	r.idle()
	if res.Reason != EndCompleted || res.Text != "It listens on 8080." || res.Rounds != 2 {
		t.Fatalf("result = %+v", res)
	}

	reqs := r.model.requests()
	if len(reqs) != 4 {
		t.Fatalf("%d requests", len(reqs))
	}
	// The sub-agent starts fresh: its own instructions and tools, the task
	// as its first message, and nothing of the parent's conversation.
	sub := reqs[1]
	if strings.Join(sub.System, "|") != "You explore the code and change nothing." || len(sub.Tools) != 1 || sub.Tools[0].Name != "Grep" {
		t.Errorf("the sub-agent's request = system %q, %d tools", sub.System, len(sub.Tools))
	}
	if got, want := userBlocks(sub), []string{"find the port the server listens on", "<start>be thorough</start>"}; len(sub.Messages) != 1 || !slices.Equal(got, want) {
		t.Errorf("the sub-agent was sent %q", got)
	}
	// Its answer is the parent's tool result, and none of its work is in
	// the parent's conversation.
	got, _ := toolResults(reqs[3])
	if got["a1"] != "The server listens on port 8080 (main.go:12)." {
		t.Errorf("the parent was handed %q", got["a1"])
	}
	for _, e := range r.log.all() {
		if e.ToolName == "Grep" {
			t.Error("the sub-agent's work is in the parent's log")
		}
	}
	if len(logs) != 1 {
		t.Fatalf("%d sub-agent logs", len(logs))
	}
	checkLogValid(t, logs[0])
	if k := logs[0].talk(); !slices.Equal(k, []EntryKind{EntryPrompt, EntryNote, EntryAssistant, EntryResult, EntryAssistant, EntryTurn}) {
		t.Errorf("the sub-agent's log = %v", k)
	}
	r.checkLog()

	// Its events reach the parent's consumer, marked.
	evs := r.rec.all()
	checkEvents(t, evs)
	var subEvents, subTools int
	agentID := ""
	for _, ev := range evs {
		m := ev.EventMeta()
		if m.Agent == "" {
			if m.Parent != "" {
				t.Errorf("%T has a parent call and no agent", ev)
			}
			continue
		}
		subEvents++
		agentID = m.Agent
		if m.Parent != "a1" || m.Session != "session-1" {
			t.Errorf("a sub-agent's %T is marked %+v", ev, m)
		}
		if e, ok := ev.(*ToolEndEvent); ok && e.Name == "Grep" {
			subTools++
		}
	}
	if subEvents == 0 || subTools != 1 {
		t.Errorf("%d events of the sub-agent, %d of them its tool call", subEvents, subTools)
	}
	for _, e := range eventsOf[*TurnEndEvent](r.rec) {
		if (e.Agent != "") != (e.Result.Text == "The server listens on port 8080 (main.go:12).") {
			t.Errorf("turn end = %+v", e)
		}
	}

	// The person's hooks and gate see the sub-agent's calls, and who
	// makes them.
	mu.Lock()
	defer mu.Unlock()
	var hookedGrep, gatedGrep bool
	for _, d := range hooked {
		if d.Call.Name == "Grep" {
			hookedGrep = d.AgentID == agentID && d.AgentType == "Explore" && d.ParentCall == "a1" && d.Session == "session-1"
		}
	}
	for _, g := range gated {
		if g.Call.Name == "Grep" {
			gatedGrep = g.AgentID == agentID && g.AgentType == "Explore" && g.ParentCall == "a1" && g.Session == "session-1"
		}
	}
	if !hookedGrep || !gatedGrep {
		t.Errorf("the sub-agent's call was hooked %v and gated %v with the right marks", hookedGrep, gatedGrep)
	}
	if len(started) != 1 || started[0].Source != "Explore" || !started[0].Empty {
		t.Errorf("the sub-agent's start = %+v", started)
	}

	// Its cost is added to the parent's, by model; its requests are not
	// the parent's own.
	u := r.a.Usage()
	if u.Requests != 2 || u.ByModel["big"].Requests != 4 {
		t.Errorf("totals = %+v", u)
	}
}

// Stopping the parent's turn stops the sub-agent, and both logs are left
// as conversations a new process can pick up.
func TestSubAgentIsStoppedWithItsParent(t *testing.T) {
	var logs []*memLog
	reached := make(chan struct{})
	r := newRig(t, func(o *Options) { o.Tools = []core.Tool{agentTool(&logs, nil)} })
	r.model.push(
		use(tc("a1", "Agent", `{"prompt":"look into it"}`)),
		use(tc("g1", "Grep", `{}`)),
		step{evs: []core.Event{{Type: core.EventTextDelta, Text: "So far I found"}}, mid: once(reached), stall: true},
	)
	done := make(chan TurnResult, 1)
	go func() { done <- r.run("delegate") }()
	<-reached
	r.a.Interrupt()
	res := <-done
	r.idle()
	if res.Reason != EndStopped {
		t.Fatalf("result = %+v", res)
	}
	var reasons []EndReason
	for _, e := range eventsOf[*TurnEndEvent](r.rec) {
		reasons = append(reasons, e.Result.Reason)
	}
	if !slices.Equal(reasons, []EndReason{EndStopped, EndStopped}) {
		t.Errorf("turns ended as %v", reasons)
	}
	// What the sub-agent had found is not lost to the parent.
	var result string
	for _, e := range r.log.of(EntryResult) {
		result = core.TextOf(e.Result.Content)
	}
	if !strings.Contains(result, "So far I found") {
		t.Errorf("the parent's tool result = %q", result)
	}
	r.checkLog()
	checkLogValid(t, logs[0])
	checkEvents(t, r.rec.all())
}

// Something that ends a sub-agent for good ends the parent's turn too.
func TestHaltInsideASubAgentEndsTheParentsTurn(t *testing.T) {
	var logs []*memLog
	r := newRig(t, func(o *Options) {
		o.Tools = []core.Tool{agentTool(&logs, func(sub *SubOptions) {
			sub.Gate = GateFunc(func(context.Context, GateRequest) (Decision, error) {
				return Decision{Halt: true, Reason: "a hook ended the turn: no searching today"}, nil
			})
		})}
	})
	r.model.push(
		use(tc("a1", "Agent", `{"prompt":"look into it"}`)),
		use(tc("g1", "Grep", `{}`)),
	)
	res := r.run("delegate")
	if res.Reason != EndHalted || !strings.Contains(res.Detail, "the sub-agent was ended") || res.Driver != "Agent" {
		t.Fatalf("result = %+v", res)
	}
	if r.model.count() != 2 {
		t.Errorf("%d requests after the halt", r.model.count())
	}
	r.checkLog()
	checkLogValid(t, logs[0])
}

// A sub-agent's question is put to the person through its parent, and a
// driver of its own may send it back before it hands in its answer.
func TestSubAgentQuestionAndStopDriver(t *testing.T) {
	var logs []*memLog
	r := newRig(t, func(o *Options) {
		o.Tools = []core.Tool{agentTool(&logs, func(sub *SubOptions) {
			sub.Gate = GateFunc(func(ctx context.Context, req GateRequest) (Decision, error) {
				ans, err := req.Ask(ctx, Question{Kind: "permission", Text: "may the sub-agent search?"})
				return Decision{Allow: ans.Choice == "yes"}, err
			})
			sub.Drivers = []Driver{{Name: "subagent-stop", Decide: func(_ context.Context, end TurnEnd) (Verdict, error) {
				if end.Streak == 0 {
					return Verdict{Action: Continue, Prompt: "<hook>cite the file</hook>"}, nil
				}
				return Verdict{}, nil
			}}}
		})}
	})
	r.model.push(
		use(tc("a1", "Agent", `{"prompt":"look into it"}`)),
		use(tc("g1", "Grep", `{}`)),
		say("It is in there somewhere."),
		say("It is in main.go, line 12."),
		say("main.go:12."),
	)
	done := make(chan TurnResult, 1)
	go func() { done <- r.run("delegate") }()
	q := waitEvent[*QuestionEvent](t, r.rec, nil)
	if q.Agent == "" || q.Parent != "a1" || q.Question.Tool != "Grep" {
		t.Errorf("the question = %+v", q)
	}
	if err := r.a.Answer(q.Question.ID, Answer{Choice: "yes"}); err != nil {
		t.Fatal(err)
	}
	res := <-done
	r.idle()
	if res.Reason != EndCompleted || res.Text != "main.go:12." {
		t.Fatalf("result = %+v", res)
	}
	// The answer handed to the parent is the sub-agent's last reply, after
	// its own driver let it finish.
	got, _ := toolResults(r.model.requests()[4])
	if got["a1"] != "It is in main.go, line 12." {
		t.Errorf("the parent was handed %q", got["a1"])
	}
	checkEvents(t, r.rec.all())
	checkLogValid(t, logs[0])
}

func TestSpawnNeedsALog(t *testing.T) {
	r := newRig(t, nil)
	if _, err := r.a.Spawn(SubOptions{}); err == nil {
		t.Error("a sub-agent without a log of its own was made")
	}
	child, err := r.a.Spawn(SubOptions{Log: newMemLog(), CallID: "c9"})
	if err != nil {
		t.Fatal(err)
	}
	if child.Depth() != 1 || child.SessionID() != "session-1" || child.AgentID() == "" || child.Events() != nil {
		t.Errorf("child = depth %d, session %q, id %q", child.Depth(), child.SessionID(), child.AgentID())
	}
	if FromContext(context.Background()) != nil {
		t.Error("FromContext found a session in an empty context")
	}
	child.Close()
}

package memjoin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/hooks"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/tools/fstools"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// bench is the join without the program around it: one session's facts,
// made by hand, and what the join contributes to it. A test plays the
// loop's part: it stores a reply, runs the tool, and stores the result
// with what the join puts beside it.
type bench struct {
	t       *testing.T
	root    string
	state   string
	project string
	feature app.Feature
	s       *app.Session
	c       *app.Contribution
	j       *join
	ts      *transcript.Session
	store   *sync.Map

	mu      sync.Mutex
	notices []string
	sent    []gnovm.RunRequest
	calls   int
}

// newBench makes a session's facts in a temporary directory and joins it.
// edit may change the facts before the join sees them.
func newBench(t *testing.T, o Options, edit func(*app.Session)) *bench {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b := &bench{t: t, root: root, state: filepath.Join(root, "data", "gnosh"), project: filepath.Join(root, "proj"), store: new(sync.Map)}
	if err := os.MkdirAll(b.project, 0o755); err != nil {
		t.Fatal(err)
	}
	st := transcript.New(b.state)
	ts, err := st.Create(b.project)
	if err != nil {
		t.Fatal(err)
	}
	b.ts = ts
	o.InProcess = true
	o.watch = func(req gnovm.RunRequest) {
		b.mu.Lock()
		b.sent = append(b.sent, req)
		b.mu.Unlock()
	}
	var joined *join
	o.joined = func(_ *app.Contribution, j *join) { joined = j }
	b.feature = New(o)
	b.s = &app.Session{
		ID:         ts.ID(),
		Source:     "startup",
		Dirs:       app.Dirs{Home: filepath.Join(root, "home"), Cwd: b.project, ProjectRoot: b.project, State: b.state, Sessions: b.state},
		Sessions:   st,
		Transcript: ts,
		Texts:      prompt.Builtin(),
		Env:        b.env,
		Notice: func(level agent.Level, source, text string) {
			b.mu.Lock()
			b.notices = append(b.notices, text)
			b.mu.Unlock()
		},
		Now: time.Now,
	}
	if edit != nil {
		edit(b.s)
	}
	c, err := b.feature.Session(context.Background(), b.s)
	if err != nil {
		t.Fatal(err)
	}
	b.c, b.j = c, joined
	t.Cleanup(func() {
		if c.Close != nil {
			c.Close(context.Background())
		}
		b.feature.Shutdown(context.Background())
		ts.Close()
	})
	return b
}

// env is what a tool call of the bench's session is given.
func (b *bench) env() *core.ToolEnv {
	return &core.ToolEnv{Cwd: b.project, Roots: []string{b.project}, SessionID: b.ts.ID(), Store: b.store}
}

// tool returns the contributed tool called name, or nil.
func (b *bench) tool(name string) core.Tool {
	for _, t := range b.c.Tools {
		if t.Spec().Name == name {
			return t
		}
	}
	return nil
}

// told returns what the person was told so far, once what waits to be
// told has been.
func (b *bench) told() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Join(b.notices, "\n")
}

// say stores a reply of the model that calls nothing.
func (b *bench) say(text string) {
	b.t.Helper()
	b.append(agent.Entry{Kind: agent.EntryAssistant, Message: core.AssistantText(text), StopReason: core.StopEndTurn})
}

// prompt stores a prompt the person typed.
func (b *bench) prompt(text string) {
	b.t.Helper()
	b.append(agent.Entry{Kind: agent.EntryPrompt, Turn: "turn-1", Blocks: []core.Block{core.Text(text)}})
}

// append stores an entry of the loop's as the wiring's log does: the
// entry, then what the join puts beside it, in one write; and then the
// join is told.
func (b *bench) append(e agent.Entry) {
	b.t.Helper()
	var te transcript.Entry
	switch e.Kind {
	case agent.EntryPrompt:
		te = transcript.UserEntry(core.Message{Blocks: e.Blocks}, transcript.SourcePrompt)
	case agent.EntryNote:
		te = transcript.UserEntry(core.Message{Blocks: e.Blocks}, transcript.SourceNotice)
	case agent.EntryAssistant:
		te = transcript.AssistantEntry(e.Message, e.StopReason)
	case agent.EntryResult:
		te = transcript.UserEntry(core.Message{Blocks: []core.Block{core.ToolResultBlock(e.ToolUseID, e.Result)}}, transcript.SourceToolResults)
	case agent.EntrySummary:
		te = transcript.SummaryEntry(core.TextOf(e.Blocks), "")
	default:
		b.t.Fatalf("the bench cannot store a %s entry", e.Kind)
	}
	out := []transcript.Entry{te}
	if b.c.Stored != nil {
		out = append(out, b.c.Stored(e)...)
	}
	written, err := b.ts.AppendAll(out...)
	if err != nil {
		b.t.Fatal(err)
	}
	if b.c.AfterStored != nil {
		b.c.AfterStored(context.Background(), e, written)
	}
}

// call plays one round of the loop: the model calls a tool, the tool
// runs, and its result is stored.
func (b *bench) call(name string, args map[string]any) core.ToolResult {
	b.t.Helper()
	b.calls++
	id := "call_" + b.ts.ID()[len(b.ts.ID())-8:] + "_" + jsonID(b.calls)
	raw, _ := json.Marshal(args)
	b.append(agent.Entry{Kind: agent.EntryAssistant, Message: core.Message{Role: core.RoleAssistant, Blocks: []core.Block{core.ToolUse(id, name, raw)}}, StopReason: core.StopToolUse})
	var tool core.Tool
	if tool = b.tool(name); tool == nil {
		for _, t := range fstools.New(fstools.Options{}) {
			if t.Spec().Name == name {
				tool = t
			}
		}
	}
	if tool == nil {
		b.t.Fatalf("the bench has no tool called %s", name)
	}
	res, err := tool.Run(context.Background(), core.ToolCall{ID: id, Name: name, Input: raw}, b.env())
	if err != nil {
		b.t.Fatalf("%s could not run: %v", name, err)
	}
	b.append(agent.Entry{Kind: agent.EntryResult, ToolUseID: id, ToolName: name, Result: res})
	return res
}

// mem plays a call on mem and returns what the model reads of it.
func (b *bench) mem(code string) string {
	b.t.Helper()
	return core.TextOf(b.call(gnovmtools.RunToolName, map[string]any{"name": "mem", "code": code}).Content)
}

func jsonID(n int) string {
	raw, _ := json.Marshal(n)
	return string(raw)
}

// lastSent returns the last request the tools sent to the GnoVM.
func (b *bench) lastSent() gnovm.RunRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.sent) == 0 {
		b.t.Fatal("no request was sent to the GnoVM")
	}
	return b.sent[len(b.sent)-1]
}

// wait lets what the join has to tell the person be told.
func (b *bench) wait() {
	if j := b.join(); j != nil {
		j.notify.wait()
	}
}

// join returns the bench's session as the join holds it.
func (b *bench) join() *join { return b.j }

// The client's lifecycle: nothing is started until a machine or mem is
// first used; the data is in the folder gnovm under gnosh's own data
// directory; and the client is let go as the program ends.
func TestLifecycle(t *testing.T) {
	b := newBench(t, Options{}, nil)
	dir := DataDir(b.state)
	if dir != filepath.Join(b.state, "gnovm") {
		t.Errorf("the gnovm data directory is %s", dir)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Errorf("the gnovm data directory was made before anything used it")
	}
	if got := len(b.c.Tools); got != 4 {
		t.Fatalf("the main agent was given %d gnovm tools, want four", got)
	}
	var names []string
	for _, tool := range b.c.Tools {
		names = append(names, tool.Spec().Name)
	}
	if strings.Join(names, " ") != "gnovm_run gnovm_new gnovm_list gnovm_drop" {
		t.Errorf("the tools, in order: %v", names)
	}

	// Hooks are told where mem's status file is, before mem exists, and
	// the folder is one whose change always asks.
	var ho hooks.Options
	b.c.Hooks(&ho)
	if !strings.HasPrefix(ho.MemStatus, dir+string(filepath.Separator)) || !strings.HasSuffix(ho.MemStatus, filepath.Join("mem", "status.json")) {
		t.Errorf("GNOSH_MEM_STATUS would be %q", ho.MemStatus)
	}
	if len(b.c.Protected) != 1 || b.c.Protected[0] != dir {
		t.Errorf("the protected paths: %v", b.c.Protected)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Errorf("naming mem's status file made the gnovm data directory")
	}

	// The first call starts the client, in this process.
	out := b.mem(`mem.Fact("cmd/test", "go test ./...")`)
	if !strings.Contains(out, "ok · mem · ") {
		t.Fatalf("the first call on mem: %s", out)
	}
	if _, err := os.Stat(ho.MemStatus); err != nil {
		t.Errorf("mem's status file is not where the hooks were told: %v", err)
	}
	locks, _ := filepath.Glob(filepath.Join(dir, ".clients", "*.lock"))
	if len(locks) != 1 {
		t.Errorf("client locks while the client is in use: %v", locks)
	}
	if _, err := os.Stat(filepath.Join(dir, "worker.log")); err == nil {
		t.Errorf("a worker was started under test")
	}
	req := b.lastSent()
	if req.Project != b.project || req.Session != b.ts.ID() || req.Name != "mem" || req.Req == "" || req.SubAgent {
		t.Errorf("the request on mem: %+v", req)
	}

	// The program's end lets the client go.
	b.feature.Shutdown(context.Background())
	locks, _ = filepath.Glob(filepath.Join(dir, ".clients", "*.lock"))
	if len(locks) != 0 {
		t.Errorf("client locks after shutdown: %v", locks)
	}
}

// A sub-agent's session: the stock one has no gnovm tools. One that is
// given them is a session of its own to mem, its requests are marked as a
// sub-agent's, it holds a turn lock of its own, and it gets none of the
// lines, the reminder, the goal or the commands.
func TestSubAgent(t *testing.T) {
	sub := func(s *app.Session) { s.SubAgent, s.Parent, s.AgentType = true, "the-parent-session", "helper" }

	stock := newBench(t, Options{}, sub)
	if len(stock.c.Tools) != 0 {
		t.Errorf("the stock sub-agent was given %d gnovm tools", len(stock.c.Tools))
	}
	stock.feature.Shutdown(context.Background())

	b := newBench(t, Options{SubAgentTools: func(kind string) bool { return kind == "helper" }}, sub)
	if len(b.c.Tools) != 4 {
		t.Fatalf("a sub-agent that is given the tools has %d of them", len(b.c.Tools))
	}
	if b.c.PromptLines != nil || b.c.BeforeRequest != nil || b.c.Drivers != nil || b.c.Slash != nil || b.c.AfterSummary != nil {
		t.Errorf("a sub-agent was given lines, reminders, turn-end checks or commands")
	}

	out := b.mem(`mem.Todo("check the links").Push()`)
	if !strings.Contains(out, "in `check-links`") {
		t.Fatalf("the sub-agent's call on mem: %s", out)
	}
	req := b.lastSent()
	if !req.SubAgent || req.Session != b.ts.ID() {
		t.Errorf("the sub-agent's request: SubAgent %v, session %q, want its own session %q", req.SubAgent, req.Session, b.ts.ID())
	}
	if got := stackIDs(b.ts.Stack()); got != "check-links" {
		t.Errorf("the sub-agent's own stack: %q", got)
	}

	// Its own turn lock, held while it is busy.
	turns := filepath.Join(DataDir(b.state), "turns")
	release := b.c.Busy(context.Background())
	locks, _ := filepath.Glob(filepath.Join(turns, "*.lock"))
	if len(locks) != 1 {
		t.Errorf("turn locks while the sub-agent is busy: %v", locks)
	}
	release()
}

// A session that begins in plan mode may look and not change: it is given
// gnovm_list alone, and its prompts carry the digest but not the mem
// line.
func TestPlanMode(t *testing.T) {
	// Another session keeps a fact first, so that there is a digest.
	maker := newBench(t, Options{}, nil)
	maker.mem(`mem.Fact("cmd/test", "go test ./...")`)
	maker.feature.Shutdown(context.Background())

	b := newBench(t, Options{}, func(s *app.Session) {
		s.Dirs = maker.s.Dirs
		cfg := &config.Config{}
		cfg.Permissions.Mode.V = "plan"
		cfg.Options.MemReviewCloses.V = true
		cfg.Options.AutoSummarize.V = true
		s.Config = cfg
	})
	if len(b.c.Tools) != 1 || b.c.Tools[0].Spec().Name != gnovmtools.ListToolName {
		t.Fatalf("the plan agent's gnovm tools: %d", len(b.c.Tools))
	}
	if b.c.Drivers != nil {
		t.Errorf("plan mode was given turn-end checks")
	}
	parts, err := b.c.PromptLines[0](context.Background(), agent.LineInfo{Prompt: agent.Prompt{Text: "plan the release"}, First: true})
	if err != nil || len(parts) != 1 || parts[0].Source != srcDigest || !strings.Contains(parts[0].Text, "`cmd/test` = go test ./...") {
		t.Errorf("the hidden parts of a prompt in plan mode: %+v, %v", parts, err)
	}
}

// The mem reminder: after ten replies with no call on mem and no reminder
// among them, counted since the last summary. A session that is opened
// again counts from its file.
func TestReminderCount(t *testing.T) {
	b := newBench(t, Options{}, nil)
	j := b.join()
	ask := func() string {
		parts, err := b.c.BeforeRequest[0](context.Background(), agent.RequestInfo{})
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range parts {
			if p.Source == srcReminder {
				return p.Text
			}
		}
		return ""
	}
	b.prompt("do a lot of small things")
	for i := 0; i < 9; i++ {
		b.say("one more thing")
	}
	if got := ask(); got != "" {
		t.Errorf("reminded after nine replies: %s", got)
	}
	b.say("and another")
	want := prompt.Builtin().MemReminder(nil, nil)
	if got := ask(); got != want {
		t.Errorf("the reminder after ten replies:\n%s\nwant:\n%s", got, want)
	}

	// The reminder itself starts the count over, and so does a call on
	// mem, and a summary.
	b.append(agent.Entry{Kind: agent.EntryNote, Source: srcReminder, Blocks: []core.Block{core.Text(want)}})
	if got := ask(); got != "" {
		t.Errorf("reminded again right after a reminder")
	}
	for i := 0; i < 10; i++ {
		b.say("more")
	}
	if ask() == "" {
		t.Errorf("not reminded ten replies after the last reminder")
	}
	b.mem(`mem.Todo("fix the flaky test", "find the cause").Push()`)
	if got := ask(); got != "" {
		t.Errorf("reminded right after a call on mem")
	}

	// Standing in a todo, with a task list, the reminder says where the
	// session stands and how its todos stand.
	b.mem(`mem.Next()`)
	for i := 0; i < 10; i++ {
		b.say("more")
	}
	got := ask()
	has(t, "the reminder inside a todo", got,
		"You are in `fix-flaky-test/find-cause` \"find the cause\"",
		"Where your todos in mem stand:\n1. find the cause (in progress)")
	hasNot(t, "the reminder inside a todo", got, "Other work the user asks for meanwhile")

	// Opened again, the session counts from what its file holds.
	fresh := &join{id: j.id, ts: j.ts}
	fresh.load()
	if fresh.remind.replies != 10 {
		t.Errorf("a session opened again counts %d replies since mem was last used, want 10", fresh.remind.replies)
	}
	if len(fresh.tasks.list) != 1 || fresh.tasks.list[0].Content != "find the cause" {
		t.Errorf("a session opened again has the task list %+v", fresh.tasks.list)
	}
}

// Frames survive a stopped turn. When the loop gave up waiting for a call
// and stored that it did not stop, and mem then turns out to have kept
// the call, the frames are written all the same: the stack must have
// them.
func TestFramesOfACallTheLoopGaveUpOn(t *testing.T) {
	b := newBench(t, Options{}, nil)
	j := b.join()
	b.mem(`mem.Todo("fix the flaky test", "find the cause").Push()`)
	if got := stackIDs(b.ts.Stack()); got != "fix-flaky-test" {
		t.Fatalf("the stack: %q", got)
	}

	// A call whose result the loop stored before the tool came back.
	raw, _ := json.Marshal(map[string]string{"name": "mem", "code": "mem.Next()"})
	b.append(agent.Entry{Kind: agent.EntryAssistant, Message: core.Message{Role: core.RoleAssistant, Blocks: []core.Block{core.ToolUse("late-call", gnovmtools.RunToolName, raw)}}, StopReason: core.StopToolUse})
	b.append(agent.Entry{Kind: agent.EntryResult, ToolUseID: "late-call", ToolName: gnovmtools.RunToolName, Result: core.ErrorResult("the call had not stopped"), Interrupted: true})
	if got := stackIDs(b.ts.Stack()); got != "fix-flaky-test" {
		t.Fatalf("the stack before the late frames: %q", got)
	}
	text := j.onMemFrames(context.Background(), j.id, "late-call", []gnovm.MemFrame{
		{Kind: "push", Id: "fix-flaky-test/find-cause", Purpose: "find the cause", Session: j.id},
	})
	if text != "" {
		t.Errorf("a push has nothing to tell the model: %q", text)
	}
	if got := stackIDs(b.ts.Stack()); got != "fix-flaky-test fix-flaky-test/find-cause" {
		t.Errorf("the stack after the late frames: %q", got)
	}

	// The frames of a call whose result is stored afterwards are written
	// beside that result, whatever the result says.
	b.append(agent.Entry{Kind: agent.EntryAssistant, Message: core.Message{Role: core.RoleAssistant, Blocks: []core.Block{core.ToolUse("stopped-call", gnovmtools.RunToolName, raw)}}, StopReason: core.StopToolUse})
	text = j.onMemFrames(context.Background(), j.id, "stopped-call", []gnovm.MemFrame{
		{Kind: "pop", Id: "fix-flaky-test/find-cause", Purpose: "find the cause", Result: "it races", Session: j.id},
	})
	if text != "nothing came between, so nothing was removed from your context." {
		t.Errorf("the pop's receipt: %q", text)
	}
	b.append(agent.Entry{Kind: agent.EntryResult, ToolUseID: "stopped-call", ToolName: gnovmtools.RunToolName, Result: core.ErrorResult("the turn was stopped"), Interrupted: true})
	if got := stackIDs(b.ts.Stack()); got != "fix-flaky-test" {
		t.Errorf("the stack after a pop whose result was replaced: %q", got)
	}
}

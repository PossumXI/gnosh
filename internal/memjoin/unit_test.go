package memjoin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/app/agentdef"
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
	// mode is the permission mode the session is in, which a test may
	// change; lists holds every task list the join set, and facts every
	// status fact, as "name=value", in order.
	mode  string
	lists []string
	facts []string
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
		Tasks: func(tasks []app.Task) {
			b.mu.Lock()
			b.lists = append(b.lists, taskLine(tasks))
			b.mu.Unlock()
		},
		Status: func(name, value string) {
			b.mu.Lock()
			b.facts = append(b.facts, name+"="+value)
			b.mu.Unlock()
		},
		Mode: func() string {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.mode == "" {
				return "default"
			}
			return b.mode
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

// setMode switches the session's permission mode, as the person would.
func (b *bench) setMode(mode string) {
	b.mu.Lock()
	b.mode = mode
	b.mu.Unlock()
}

// fact returns the status fact called name as the join last set it, and
// whether it ever set it.
func (b *bench) fact(name string) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i := len(b.facts) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(b.facts[i], name+"="); ok {
			return v, true
		}
	}
	return "", false
}

// list returns the task list as the join last set it.
func (b *bench) list() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.lists) == 0 {
		return ""
	}
	return b.lists[len(b.lists)-1]
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

// A sub-agent's session. What it is given follows its definition, which
// the wiring hands over with its facts, whichever folder it was read
// from: the join looks for no file itself. The stock kinds are given no
// gnovm tools, and nothing else of the join either; so is a kind a person
// defined that names none, and a sub-agent with no definition. A kind
// whose definition names gnovm tools is given exactly those: it is a
// session of its own to mem, its requests are marked as a sub-agent's, it
// holds a turn lock of its own, and it gets none of the lines, the
// reminder, the goal or the commands.
func TestSubAgent(t *testing.T) {
	// define reads a definition from the text of its file, as the Agent
	// tool does.
	define := func(name, front string) *agentdef.Def {
		t.Helper()
		d, reason := agentdef.Parse("---\nname: " + name + "\ndescription: A helper for the test.\n" + front + "---\n\nYou are a helper.\n")
		if reason != "" {
			t.Fatalf("the definition of %s cannot be read: %s", name, reason)
		}
		return &d
	}
	defs := map[string]*agentdef.Def{
		"plain-helper": define("plain-helper", ""),
		"searcher":     define("searcher", "tools: Read, Grep\n"),
		"reader":       define("reader", "tools: Read, gnovm_list\n"),
		"planner":      define("planner", "tools: Read, gnovm_run\n"),
		"keeper":       define("keeper", "tools: gnovm_run, gnovm_new, gnovm_list, gnovm_drop\ndisallowedTools: gnovm_drop\n"),
	}
	for _, d := range agentdef.Builtin() {
		defs[d.Name] = &d
	}
	sub := func(kind string) func(s *app.Session) {
		return func(s *app.Session) {
			s.SubAgent, s.Parent, s.AgentType, s.AgentDef, s.Source = true, "the-parent-session", kind, defs[kind], "subagent"
			// A file that says otherwise, where the join once looked for
			// definitions itself: nothing reads it now.
			dir := filepath.Join(s.Dirs.Home, ".claude", "agents")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			text := "---\nname: " + kind + "\ndescription: Not the definition in force.\ntools: gnovm_run, gnovm_new, gnovm_list, gnovm_drop\n---\n\nYou are a helper.\n"
			if err := os.WriteFile(filepath.Join(dir, kind+".md"), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	names := func(b *bench) string {
		var out []string
		for _, tool := range b.c.Tools {
			out = append(out, tool.Spec().Name)
		}
		return strings.Join(out, " ")
	}
	if defs["general-purpose"] == nil || defs["Explore"] == nil || defs["no-definition"] != nil {
		t.Fatalf("the definitions at hand: %v", defs)
	}

	// The stock kinds, a kind that names no tools, a kind that names
	// tools and no gnovm tool, and a sub-agent that came with no
	// definition: nothing of the join.
	for _, kind := range []string{"general-purpose", "Explore", "plain-helper", "searcher", "no-definition"} {
		b := newBench(t, Options{}, sub(kind))
		if len(b.c.Tools) != 0 || b.c.Stored != nil || b.c.Busy != nil || b.c.PromptLines != nil || b.c.Ready != nil {
			t.Errorf("a %s sub-agent was given something of the join: tools %q", kind, names(b))
		}
		b.feature.Shutdown(context.Background())
	}
	// A kind is given the gnovm tools its definition names, and no other.
	reader := newBench(t, Options{}, sub("reader"))
	if got := names(reader); got != "gnovm_list" {
		t.Errorf("the tools of a kind that names gnovm_list: %q", got)
	}
	reader.feature.Shutdown(context.Background())
	keeper := newBench(t, Options{}, sub("keeper"))
	if got := names(keeper); got != "gnovm_run gnovm_new gnovm_list" {
		t.Errorf("the tools of a kind that names all four and disallows one: %q", got)
	}
	keeper.feature.Shutdown(context.Background())

	b := newBench(t, Options{}, sub("planner"))
	if got := names(b); got != "gnovm_run" {
		t.Fatalf("the tools of a kind that names gnovm_run: %q", got)
	}
	if b.c.PromptLines != nil || b.c.BeforeRequest != nil || b.c.Slash != nil || b.c.AfterSummary != nil || b.c.RunPrompt != nil {
		t.Errorf("a sub-agent was given lines, reminders or commands")
	}
	// It can close a todo, so a turn of its that would end on a refused
	// close is sent back, as the session's own is; it has no goal.
	if len(b.c.Drivers) != 1 || b.c.Drivers[0].Name != "mem-close" {
		t.Errorf("the turn-end checks of a sub-agent that has gnovm_run: %+v", b.c.Drivers)
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
	// A sub-agent has no task list and no status line of its own.
	if len(b.lists) != 0 || len(b.facts) != 0 {
		t.Errorf("a sub-agent set a task list or a status fact: %q %q", b.lists, b.facts)
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

// Plan mode, in which the model may look and not change. The session
// keeps its four gnovm tools, so that its fixed prompt is the same in and
// out of plan mode, and while the mode lasts gnovm_list alone runs: the
// others are refused with what plan mode means. Its prompts carry the
// digest and no mem line, it is not reminded of mem, and nothing sends it
// back as a turn ends. The person may switch the mode at any moment, and
// each of these follows at once.
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
	b.setMode("plan")
	var names []string
	for _, tool := range b.c.Tools {
		names = append(names, tool.Spec().Name)
	}
	if got := strings.Join(names, " "); got != "gnovm_run gnovm_new gnovm_list gnovm_drop" {
		t.Fatalf("the gnovm tools of a session that begins in plan mode: %q", got)
	}
	if len(b.c.Drivers) != 2 {
		t.Fatalf("the session has %d turn-end checks, want the two it has in every mode", len(b.c.Drivers))
	}
	lines := func(first bool) []agent.Part {
		t.Helper()
		parts, err := b.c.PromptLines[0](context.Background(), agent.LineInfo{Prompt: agent.Prompt{Text: "plan the release"}, First: first})
		if err != nil {
			t.Fatal(err)
		}
		return parts
	}
	if parts := lines(true); len(parts) != 1 || parts[0].Source != srcDigest || !strings.Contains(parts[0].Text, "`cmd/test` = go test ./...") {
		t.Errorf("the hidden parts of a prompt in plan mode: %+v", parts)
	}

	// The three tools that run or change something are refused, at the
	// gate's question and at the call; gnovm_list runs.
	refusal := func(tool string) string { return prompt.Builtin().PlanRefusal(tool) }
	has(t, "the refusal", refusal("gnovm_run"), "plan mode is on, and gnovm_run does not run in it", "gnovm_list still lists the machines", "put it on mem first")
	for _, name := range []string{"gnovm_run", "gnovm_new", "gnovm_drop"} {
		call := core.ToolCall{ID: "p-" + name, Name: name, Input: json.RawMessage(`{"name":"mem","code":"mem.Show()","action":"delete"}`)}
		act, err := b.tool(name).(core.Describer).Describe(call, b.env())
		if err == nil || err.Error() != refusal(name) {
			t.Errorf("%s in plan mode, asked what it would do: %v, want the refusal", name, err)
		}
		if act.Kind == "" {
			t.Errorf("%s in plan mode does not say what the call was aimed at", name)
		}
		res, err := b.tool(name).Run(context.Background(), call, b.env())
		if err != nil || !res.IsError || core.TextOf(res.Content) != refusal(name) {
			t.Errorf("%s in plan mode, run: %q, %v", name, core.TextOf(res.Content), err)
		}
	}
	if n := len(b.sent); n != 0 {
		t.Errorf("%d requests reached the GnoVM in plan mode", n)
	}
	if _, isGated := b.tool("gnovm_list").(gated); isGated {
		t.Errorf("gnovm_list is gated, and it only looks")
	}
	if out := core.TextOf(b.call("gnovm_list", map[string]any{}).Content); !strings.Contains(out, "VM budget") {
		t.Errorf("gnovm_list in plan mode: %s", out)
	}

	// No reminder of mem while the model cannot use it, and no send-back.
	for i := 0; i < 12; i++ {
		b.say("planning")
	}
	if parts, _ := b.c.BeforeRequest[0](context.Background(), agent.RequestInfo{}); len(parts) != 0 {
		t.Errorf("reminded of mem in plan mode: %+v", parts)
	}
	for _, d := range b.c.Drivers {
		if v, err := d.Decide(context.Background(), agent.TurnEnd{}); err != nil || v.Action != agent.End {
			t.Errorf("the turn-end check %s in plan mode: %+v, %v", d.Name, v, err)
		}
	}

	// The person leaves plan mode: the same session's next prompt carries
	// the mem line, the reminder is due, and the tools run.
	b.setMode("acceptEdits")
	parts := lines(false)
	if len(parts) != 1 || parts[0].Source != srcMemLine {
		t.Errorf("the hidden parts of a prompt after plan mode: %+v", parts)
	}
	if parts, _ := b.c.BeforeRequest[0](context.Background(), agent.RequestInfo{}); len(parts) != 1 || parts[0].Source != srcReminder {
		t.Errorf("not reminded of mem after twelve replies, once plan mode was left: %+v", parts)
	}
	if out := b.mem(`mem.Todo("release v2", "tag it").Push()`); !strings.Contains(out, "in `release-v2`") {
		t.Errorf("gnovm_run after plan mode: %s", out)
	}
	if got, _ := b.fact("frame"); got != "release-v2" {
		t.Errorf("the status line's frame after the push: %q", got)
	}

	// And enters it again, standing in a todo: no frame line and no mem
	// line, and the call is refused again.
	b.setMode("plan")
	if parts := lines(false); len(parts) != 0 {
		t.Errorf("the hidden parts of a prompt back in plan mode: %+v", parts)
	}
	res, _ := b.tool("gnovm_run").Run(context.Background(), core.ToolCall{ID: "again", Name: "gnovm_run", Input: json.RawMessage(`{"name":"mem","code":"mem.Next()"}`)}, b.env())
	if !res.IsError || core.TextOf(res.Content) != refusal("gnovm_run") {
		t.Errorf("gnovm_run back in plan mode: %q", core.TextOf(res.Content))
	}
}

// A goal in plan mode. The goal is the person's to set in any mode: it is
// bound, and the status line names it. What plan mode holds back is the
// work: as a turn ends the model is not sent back to the goal, and once
// the person leaves plan mode it is.
func TestGoalInPlanMode(t *testing.T) {
	b := newBench(t, Options{}, nil)
	b.setMode("plan")
	var goal *app.SlashCommand
	for i := range b.c.Slash {
		if b.c.Slash[i].Name == "goal" {
			goal = &b.c.Slash[i]
		}
	}
	if goal == nil {
		t.Fatal("the session has no /goal")
	}
	var said []string
	err := goal.Run(context.Background(), &app.SlashContext{Args: "the notes say hi", Session: b.s, Say: func(format string, args ...any) {
		said = append(said, fmt.Sprintf(format, args...))
	}})
	// The bench has no agent to send the directive to; the goal is set
	// all the same, and the command says both.
	if err == nil || !strings.Contains(err.Error(), "the goal is set, and the agent could not be told of it") {
		t.Errorf("/goal in a session with no agent: %v", err)
	}
	if got, want := strings.Join(said, "\n"), "Goal set: the notes say hi (`notes-say-hi` in mem)\nPlan mode is on: the agent may plan for the goal, and is not sent back to it until you leave plan mode."; got != want {
		t.Errorf("/goal in plan mode said:\n%s\nwant:\n%s", got, want)
	}
	if got, _ := b.fact("goal"); got != "notes-say-hi" {
		t.Errorf("the status line's goal: %q", got)
	}

	// As a turn ends in plan mode, nothing is sent.
	var driver agent.Driver
	for _, d := range b.c.Drivers {
		if d.Name == "mem-goal" {
			driver = d
		}
	}
	b.prompt("plan for the goal")
	b.say("Here is the plan.")
	if v, err := driver.Decide(context.Background(), agent.TurnEnd{Text: "Here is the plan."}); err != nil || v.Action != agent.End || v.Prompt != "" || v.Notice != "" {
		t.Errorf("the goal's turn-end check in plan mode: %+v, %v", v, err)
	}
	// Out of plan mode, the model is sent back to the goal.
	b.setMode("default")
	v, err := driver.Decide(context.Background(), agent.TurnEnd{Text: "Here is the plan."})
	if err != nil || v.Action != agent.Continue || !strings.Contains(v.Prompt, "`notes-say-hi`") {
		t.Errorf("the goal's turn-end check after plan mode: %+v, %v", v, err)
	}

	// The goal is let go: the status line names none.
	said = nil
	if err := goal.Run(context.Background(), &app.SlashContext{Args: "clear", Session: b.s, Say: func(format string, args ...any) {
		said = append(said, fmt.Sprintf(format, args...))
	}}); err != nil {
		t.Errorf("/goal clear: %v", err)
	}
	if got, ok := b.fact("goal"); !ok || got != "" {
		t.Errorf("the status line's goal after /goal clear: %q", got)
	}
}

// A session that is opened again is handed where it stands as it starts:
// its frame, its goal and its task list, read from its file.
func TestStatusOfASessionOpenedAgain(t *testing.T) {
	b := newBench(t, Options{}, nil)
	b.mem(`mem.Todo("rename the project", "rename it in the code", "rename it in the tests").Push()`)
	b.mem(`mem.Next()`)
	bound := gnovm.GoalBinding{Condition: "the project is renamed", Name: "project-renamed", Title: "the project is renamed"}
	if !b.join().swapGoal("", encodeGoal(bound)) {
		t.Fatal("the binding could not be stored")
	}

	// The same session's facts, joined again, as a new process would.
	var facts, lists []string
	again := *b.s
	again.Status = func(name, value string) { facts = append(facts, name+"="+value) }
	again.Tasks = func(tasks []app.Task) { lists = append(lists, taskLine(tasks)) }
	c, err := b.feature.Session(context.Background(), &again)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 || len(lists) != 0 {
		t.Errorf("the front end was handed %q and %q before the session was ready", facts, lists)
	}
	c.Ready(nil)
	if got, want := strings.Join(facts, " "), "frame=rename-project/rename-code goal=project-renamed"; got != want {
		t.Errorf("the status facts of a session opened again: %q, want %q", got, want)
	}
	if got, want := strings.Join(lists, "\n"), "[>] in `rename-project/rename-code` \"rename it in the code\" | [ ] rename it in the tests"; got != want {
		t.Errorf("the task list of a session opened again:\n%s\nwant:\n%s", got, want)
	}
	c.Close(context.Background())
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
	if len(fresh.tasks.list) != 1 || fresh.tasks.list[0].Title != "find the cause" || fresh.tasks.list[0].State != gnovmtools.LineWorking {
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

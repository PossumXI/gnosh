package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// echoTool is a tool a feature brings: it says its input back.
type echoTool struct{ ran *[]string }

func (echoTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "Echo", Description: "Says its text back.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`)}
}

func (e echoTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	var in struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(call.Input, &in)
	*e.ran = append(*e.ran, in.Text)
	return core.TextResult("echo: " + in.Text), nil
}

// testFeature is a feature as another builder would write one: everything
// it does goes through the seam, and it keeps a diary of when it was
// called.
type testFeature struct {
	mu    sync.Mutex
	diary []string
	ran   []string
}

func (tf *testFeature) note(format string, args ...any) {
	tf.mu.Lock()
	defer tf.mu.Unlock()
	tf.diary = append(tf.diary, fmt.Sprintf(format, args...))
}

func (tf *testFeature) feature() Feature {
	objected := false
	return Feature{
		Name:  "test-feature",
		Tools: []ToolName{{Name: "Echo", Quiet: true}},
		Env:   []string{"GNOSH_ECHO_LOUD"},
		Built: []string{"mem"},
		Boot: func(args []string) (int, bool) {
			tf.note("boot")
			if len(args) > 0 && args[0] == "feature-worker" {
				return 7, true
			}
			return 0, false
		},
		Commands: []Command{{Command: cli.CmdMem, Run: func(ctx context.Context, c *CommandContext) int {
			tf.note("command mem %s", strings.Join(c.Invocation.Args, " "))
			fmt.Fprintf(c.Stdout, "mem of %s, sessions under %s\n", c.Dirs.Cwd, c.Dirs.Sessions)
			return 0
		}}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			tf.note("session %s interactive=%v loud=%q", s.Source, s.Interactive, s.Getenv("GNOSH_ECHO_LOUD"))
			return &Contribution{
				Tools: []core.Tool{echoTool{ran: &tf.ran}},
				PromptLines: []func(context.Context, agent.LineInfo) ([]agent.Part, error){
					func(ctx context.Context, info agent.LineInfo) ([]agent.Part, error) {
						return []agent.Part{{Source: "test-feature", Text: prompt.Wrap("The feature's line for this prompt.")}}, nil
					},
				},
				Drivers: []agent.Driver{{Name: "test-check", Decide: func(ctx context.Context, end agent.TurnEnd) (agent.Verdict, error) {
					if objected {
						return agent.Verdict{Action: agent.End}, nil
					}
					objected = true
					return agent.Verdict{Action: agent.Continue, Prompt: prompt.Wrap("The feature's check is not satisfied yet."), Objection: "not satisfied yet"}, nil
				}}},
				AfterTool: func(ctx context.Context, done agent.ToolDone) (agent.ToolVerdict, error) {
					tf.note("after tool %s", done.Call.Name)
					return agent.ToolVerdict{}, nil
				},
				TurnEnded: func(ctx context.Context, res agent.TurnResult) { tf.note("turn ended %s", res.Reason) },
				Slash: []SlashCommand{{Name: "ping", Summary: "Answers pong.", InRun: true, Run: func(ctx context.Context, c *SlashContext) error {
					c.Say("pong %s", c.Args)
					return nil
				}}},
				Ready:  func(a *agent.Agent) { tf.note("ready") },
				RunEnd: func(ctx context.Context, end RunEnd) int { tf.note("run end %d", end.Exit); return 3 },
				Close:  func(ctx context.Context) { tf.note("close") },
			}, nil
		},
		Shutdown: func(ctx context.Context) { tf.note("shutdown") },
	}
}

// A feature is joined to the program through the seam alone: its tool is
// offered and runs under the person's rules, its line rides on the prompt,
// its check holds the turn open, its slash command and its subcommand run,
// and it is told of each stage of the program's life in order.
func TestFeatureSeam(t *testing.T) {
	tf := &testFeature{}
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Echo", Args: `{"text":"hello"}`}}},
		apptest.Reply{Text: "The tool said hello."},
		apptest.Reply{Text: "Now the check is satisfied."})
	o := testOptions(w, "-p", "use the feature")
	o.Features = []Feature{tf.feature()}
	o.Environ = append(o.Environ, "GNOSH_ECHO_LOUD=yes")
	r := runWith(t, t.Context(), o)
	// The feature raised the exit code when the run ended.
	r.wantCode(t, 3)
	if r.out != "The tool said hello.\n\nNow the check is satisfied.\n" {
		t.Errorf("stdout = %q", r.out)
	}
	// Its tool is offered after gnosh's own, and ran without a question.
	first := requestText(t, w, 0)
	if i, j := strings.Index(first, `"name":"TaskStop"`), strings.Index(first, `"name":"Echo"`); i < 0 || j < i {
		t.Errorf("the feature's tool is not offered after gnosh's own")
	}
	if len(tf.ran) != 1 || tf.ran[0] != "hello" {
		t.Errorf("the feature's tool ran with %q", tf.ran)
	}
	has(t, "what the model read of the tool", toolResult(t, w, 1, "c1"), "echo: hello")
	// Its line rides on the prompt, and its check sent the model back.
	has(t, "the prompt's message", userTexts(w.Model.Requests()[0])[0], "The feature's line for this prompt.", "use the feature")
	has(t, "the third request", requestText(t, w, 2), "The feature's check is not satisfied yet.")
	has(t, "stderr", r.err, "test-check", "not satisfied yet")
	// Its life, in order.
	want := "boot | session startup interactive=false loud=\"yes\" | ready | after tool Echo | turn ended completed | run end 0 | close | shutdown"
	if got := strings.Join(tf.diary, " | "); got != want {
		t.Errorf("the feature's diary:\n%s\nwant:\n%s", got, want)
	}
	w.Model.Done()

	// Its slash command, as the prompt of a run.
	tf2 := &testFeature{}
	o = testOptions(w, "-p", "/ping twice")
	o.Features = []Feature{tf2.feature()}
	r = runWith(t, t.Context(), o).wantCode(t, 0)
	if r.out != "pong twice\n" {
		t.Errorf("/ping printed %q", r.out)
	}

	// Its subcommand, which the command line now takes as built.
	tf3 := &testFeature{}
	o = testOptions(w, "mem", "show", "open")
	o.Features = []Feature{tf3.feature()}
	r = runWith(t, t.Context(), o).wantCode(t, 0)
	has(t, "what the subcommand printed", r.out, "mem of "+w.Dir, "sessions under "+w.DataDir())
	if got := strings.Join(tf3.diary, " | "); got != "boot | command mem show open | shutdown" {
		t.Errorf("the feature's diary for its subcommand: %s", got)
	}

	// Its own worker dispatch, before anything else of the program.
	tf4 := &testFeature{}
	o = testOptions(w, "feature-worker")
	o.Features = []Feature{tf4.feature()}
	runWith(t, t.Context(), o).wantCode(t, 7)

	// Without the feature, its variable is a mistake and its subcommand is
	// not built: nothing of it leaks into the program.
	o = testOptions(w, "-p", "hello")
	o.Environ = append(o.Environ, "GNOSH_ECHO_LOUD=yes")
	has(t, "stderr", runWith(t, t.Context(), o).wantCode(t, 1).err, "$GNOSH_ECHO_LOUD")
}

// A rule may name a feature's tool, and --tools may leave it out; a tool
// a feature brings without declaring its name is refused at startup.
func TestFeatureToolsFollowTheRules(t *testing.T) {
	t.Run("a deny rule names the tool", func(t *testing.T) {
		tf := &testFeature{}
		w := apptest.NewWorld(t, apptest.Reply{Text: "No Echo here."})
		o := testOptions(w, "-p", "--disallowedTools", "Echo", "--", "hello")
		o.Features = []Feature{tf.feature()}
		runWith(t, t.Context(), o)
		hasNot(t, "the request", requestText(t, w, 0), `"name":"Echo"`)
	})
	t.Run("--tools leaves it out", func(t *testing.T) {
		tf := &testFeature{}
		w := apptest.NewWorld(t, apptest.Reply{Text: "Only Read."})
		o := testOptions(w, "-p", "--tools", "Read", "--", "hello")
		o.Features = []Feature{tf.feature()}
		runWith(t, t.Context(), o)
		first := requestText(t, w, 0)
		has(t, "the request", first, `"name":"Read"`)
		hasNot(t, "the request", first, `"name":"Echo"`, `"name":"Bash"`)
	})
	t.Run("an undeclared tool", func(t *testing.T) {
		tf := &testFeature{}
		w := apptest.NewWorld(t)
		f := tf.feature()
		f.Tools = nil
		o := testOptions(w, "-p", "hello")
		o.Features = []Feature{f}
		r := runWith(t, t.Context(), o).wantCode(t, 1)
		has(t, "stderr", r.err, "Echo", "test-feature")
		if n := len(w.Model.Requests()); n != 0 {
			t.Errorf("the model was sent %d requests", n)
		}
	})
}

// stepTool is a tool that runs alone and in order, as gnovm_run does. A
// slow step does not give up when the turn is stopped: it finishes its
// bookkeeping first and then returns a result, never an error. Every
// result carries what the tool wants kept beside it, as Meta["frames"].
type stepTool struct {
	note    func(format string, args ...any)
	started chan struct{}
}

func (stepTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "Step", Description: "Does one step.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"},"slow":{"type":"boolean"}},"required":["name"]}`)}
}

func (s stepTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	var in struct {
		Name string `json:"name"`
		Slow bool   `json:"slow"`
	}
	_ = json.Unmarshal(call.Input, &in)
	s.note("run %s", in.Name)
	text := "step " + in.Name + " done"
	if in.Slow {
		close(s.started)
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond) // the bookkeeping a stop does not reach
		text = "step " + in.Name + " finished its bookkeeping after the stop"
	}
	res := core.TextResult(text)
	res.Meta = map[string]any{"frames": []any{map[string]any{"op": "push", "id": "job-" + in.Name, "call": call.ID}}}
	return res, nil
}

// What a feature such as mem needs of the loop and its log, all at once:
// a call that runs alone is not started before the result of the one
// before it is stored and the feature has been told; the feature is told
// only once the result is in the file, with what it wrote beside it; a
// result that a tool returns after the turn was stopped is stored like any
// other, with everything in it; and it all reads back from the file.
func TestFeatureIsToldOfStoredResultsInOrder(t *testing.T) {
	var mu sync.Mutex
	var diary []string
	note := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		diary = append(diary, fmt.Sprintf(format, args...))
	}
	started := make(chan struct{})
	names := map[string]string{} // a call's id -> the step's name
	feature := Feature{
		Name:  "steps",
		Tools: []ToolName{{Name: "Step", Quiet: true}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{
				Tools: []core.Tool{stepTool{note: note, started: started}},
				Stored: func(e agent.Entry) []transcript.Entry {
					if e.Kind != agent.EntryResult || e.Result.Meta["frames"] == nil {
						return nil
					}
					frame := e.Result.Meta["frames"].([]any)[0].(map[string]any)
					mu.Lock()
					names[e.ToolUseID] = strings.TrimPrefix(frame["id"].(string), "job-")
					mu.Unlock()
					return []transcript.Entry{transcript.PushEntry(frame["id"].(string), "a job", e.ToolUseID)}
				},
				AfterStored: func(ctx context.Context, e agent.Entry, written []transcript.Entry) {
					if e.Kind != agent.EntryResult {
						return
					}
					// The result is in the file by now, and so is the push.
					inFile, pushed := false, false
					for _, te := range s.Transcript.Entries() {
						for _, b := range te.Message.Blocks {
							if b.Type == core.BlockToolResult && b.ToolUseID == e.ToolUseID {
								inFile = true
							}
						}
						if te.Type == transcript.TypeFrame && te.ToolUseID == e.ToolUseID {
							pushed = true
						}
					}
					mu.Lock()
					name := names[e.ToolUseID]
					mu.Unlock()
					ids := 0
					for _, w := range written {
						if w.UUID != "" {
							ids++
						}
					}
					note("stored %s (in the file: %v, its push too: %v, written: %d with ids: %d)", name, inFile, pushed, len(written), ids)
				},
			}, nil
		},
	}
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "Step", Args: `{"name":"a"}`},
			{ID: "c2", Name: "Step", Args: `{"name":"b"}`},
			{ID: "c3", Name: "Step", Args: `{"name":"c","slow":true}`},
		}})
	pr, pw := io.Pipe()
	s := &seat{t: t, in: pw, out: &safeBuffer{}, errs: &safeBuffer{}, signals: make(chan os.Signal, 4), done: make(chan int, 1)}
	o := testOptions(w)
	o.Features = []Feature{feature}
	o.Stdin, o.Stdout, o.Stderr = pr, s.out, s.errs
	o.StdinIsTerminal, o.StdoutIsTerminal = true, true
	o.Signals = s.signals
	go func() { s.done <- Main(context.Background(), o) }()
	s.see("Type a prompt and Enter.")
	s.typeLine("do the three steps")
	select {
	case <-started:
	case <-time.After(patience):
		t.Fatalf("the slow step never started:\n%s", clip(s.out.String()))
	}
	s.signals <- os.Interrupt
	s.see("-- stopped in ")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}

	mu.Lock()
	got := strings.Join(diary, "\n")
	mu.Unlock()
	const told = " (in the file: true, its push too: true, written: 2 with ids: 2)"
	want := "run a\nstored a" + told + "\nrun b\nstored b" + told + "\nrun c\nstored c" + told
	if got != want {
		t.Errorf("the feature's diary:\n%s\nwant:\n%s", got, want)
	}

	// The file, read as a later process reads it: the slow step's own
	// result, with its Meta, and not a line that says it did not stop.
	sessions := w.Sessions()
	if len(sessions) != 1 {
		t.Fatalf("session files: %v", sessions)
	}
	ts, err := transcript.New(w.DataDir()).OpenPath(context.Background(), sessions[0], true)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Close()
	var results []agent.Entry
	for e := range newSessionLog(ts, prompt.Builtin()).Recent(context.Background()) {
		if e.Kind == agent.EntryResult {
			results = append([]agent.Entry{e}, results...)
		}
	}
	if len(results) != 3 {
		t.Fatalf("the file holds %d results, want 3", len(results))
	}
	last := results[2]
	if text := core.TextOf(last.Result.Content); text != "step c finished its bookkeeping after the stop" {
		t.Errorf("the result of the step that was stopped: %q", text)
	}
	frames, _ := last.Result.Meta["frames"].([]any)
	if len(frames) != 1 {
		t.Fatalf("the result's Meta did not come back from the file: %#v", last.Result.Meta)
	}
	if frame, _ := frames[0].(map[string]any); frame["op"] != "push" || frame["id"] != "job-c" || frame["call"] != last.ToolUseID {
		t.Errorf("the result's Meta came back as %#v", frames[0])
	}
	if stack := ts.Stack(); len(stack) != 3 || stack[2].ID != "job-c" {
		t.Errorf("the frames the feature wrote beside the results: %+v", stack)
	}
}

// Every tool call goes by an id that no other call has, in this session
// or in any other: what a project keeps beside its sessions names a call
// by its id alone.
func TestCallIDsAreUniqueAcrossSessions(t *testing.T) {
	script := func() []apptest.Reply {
		return []apptest.Reply{
			{Calls: []apptest.Call{{ID: "call_0", Name: "Read", Args: `{"file_path":"a.txt"}`}, {ID: "call_0", Name: "Read", Args: `{"file_path":"a.txt"}`}}},
			{Calls: []apptest.Call{{ID: "call_0", Name: "Read", Args: `{"file_path":"a.txt"}`}}},
			{Text: "Read."},
		}
	}
	w := apptest.NewWorld(t, append(script(), script()...)...)
	w.Write("a.txt", "hello\n")
	run(t, w, "-p", "read it").wantCode(t, 0)
	run(t, w, "-p", "read it again").wantCode(t, 0)
	reqs := w.Model.Requests()
	ids := append(callIDs(reqs[2]), callIDs(reqs[5])...)
	seen := map[string]bool{}
	form := regexp.MustCompile(`^call_[0-9a-f]{24}$`)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("two calls go by the id %s", id)
		}
		if !form.MatchString(id) {
			t.Errorf("the id %q is not of gnosh's own make", id)
		}
		seen[id] = true
	}
	if len(ids) != 6 {
		t.Errorf("the two sessions made %d calls, want 6: %v", len(ids), ids)
	}
	// Each result answers the call it belongs to.
	for _, n := range []int{2, 5} {
		if names, _ := toolResults(reqs[n]); strings.Join(names, " ") != "c1 c2 c3" {
			t.Errorf("request %d: the results answer %v", n, names)
		}
	}
}

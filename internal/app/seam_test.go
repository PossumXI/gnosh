package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// sitWith starts an interactive session with features joined to it.
func sitWith(t *testing.T, w *apptest.World, change func(*Options), args ...string) *seat {
	t.Helper()
	pr, pw := io.Pipe()
	s := &seat{t: t, in: pw, out: &safeBuffer{}, errs: &safeBuffer{}, signals: make(chan os.Signal, 4), done: make(chan int, 1)}
	o := testOptions(w, args...)
	o.Stdin, o.Stdout, o.Stderr = pr, s.out, s.errs
	o.StdinIsTerminal, o.StdoutIsTerminal = true, true
	o.Signals = s.signals
	if change != nil {
		change(&o)
	}
	go func() { s.done <- Main(context.Background(), o) }()
	t.Cleanup(func() {
		pw.Close()
		<-s.done
	})
	return s
}

// A feature may raise a notice from the three callbacks that run while the
// loop is storing something. It used to wait for ever there: the notice is
// itself kept in the log, and the log was busy with the callback. Now the
// person is shown the line at once and it is stored right after.
func TestNoticeFromTheLogCallbacksDoesNotHang(t *testing.T) {
	const window = 16000
	var mu sync.Mutex
	stored, after, slid := 0, 0, 0
	feature := Feature{
		Name: "talkative",
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			pushed := false
			return &Contribution{
				Stored: func(e agent.Entry) []transcript.Entry {
					if e.Kind != agent.EntryResult {
						return nil
					}
					mu.Lock()
					stored++
					n := stored
					mu.Unlock()
					s.Notice(agent.LevelInfo, "talkative", fmt.Sprintf("storing result %d", n))
					if !pushed {
						// The session stands in a frame from its first
						// result on, so that it can slide.
						pushed = true
						return []transcript.Entry{transcript.PushEntry("big-job", "the big job", e.ToolUseID)}
					}
					return nil
				},
				AfterStored: func(ctx context.Context, e agent.Entry, written []transcript.Entry) {
					if e.Kind != agent.EntryResult {
						return
					}
					mu.Lock()
					after++
					n := after
					mu.Unlock()
					s.Notice(agent.LevelInfo, "talkative", fmt.Sprintf("stored result %d", n))
					// The agent's own Notice is as safe.
					s.Agent().Notice(agent.LevelInfo, "talkative", fmt.Sprintf("and once more for result %d", n))
				},
				Slid: func(ctx context.Context, cuts []transcript.Cut) {
					mu.Lock()
					slid++
					mu.Unlock()
					s.Notice(agent.LevelWarning, "talkative", fmt.Sprintf("a slide took %d pieces out of the view", len(cuts)))
				},
			}, nil
		},
	}
	w := apptest.NewWorld(t)
	w.Model.SetWindow(window)
	bigFiles(w, 8)
	reads := 0
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		if isSummaryRequest(r) {
			return &apptest.Reply{Text: "SUMMARY: the user asked to read all the big files; some are read."}
		}
		reads++
		if reads <= 8 {
			return &apptest.Reply{Calls: []apptest.Call{{ID: fmt.Sprintf("c%d", reads), Name: "Read", Args: fmt.Sprintf(`{"file_path":"big%d.txt"}`, reads-1)}}}
		}
		return &apptest.Reply{Text: "All read."}
	})
	o := testOptions(w, "-p", "read all the big files")
	o.Features = []Feature{feature}
	// runWith fails the test, with every goroutine's stack, if the program
	// does not end: that is what a deadlock here looks like.
	r := runWith(t, t.Context(), o).wantCode(t, 0)
	if r.out != "All read.\n" {
		t.Errorf("stdout = %q", r.out)
	}
	mu.Lock()
	defer mu.Unlock()
	if stored != 8 || after != 8 {
		t.Errorf("the feature was shown %d results before they were stored and told of %d after", stored, after)
	}
	if slid == 0 {
		t.Errorf("the conversation outgrew its window and nothing slid, so the third callback was never tried")
	}
	has(t, "stderr", r.err, "storing result 1", "stored result 8", "and once more for result 8", "a slide took")
	// Each line is kept in the session's file, right after its result.
	raw, _ := os.ReadFile(w.Sessions()[0])
	has(t, "the session's file", string(raw), "storing result 3", "stored result 3", "and once more for result 3", "a slide took")
	for _, req := range w.Model.Requests() {
		if tokens := len(req.Body) / 4; tokens > window {
			t.Errorf("a request of about %d tokens went out, more than the window of %d", tokens, window)
		}
	}
	checkSession(t, w, w.Sessions()[0])
	// The log notes that the feature spoke, and not what it said: a
	// feature can erase its words from the session's file, not from there.
	logs, _ := filepath.Glob(filepath.Join(w.DataDir(), "projects", "*", "gnosh.log"))
	data, _ := os.ReadFile(logs[0])
	has(t, "the log", string(data), "(talkative): a line of ")
	hasNot(t, "the log", string(data), "storing result", "a slide took")
}

// recordingFrontend is a front end of the simplest kind, as another
// interface would be attached: it keeps the events it is told and sends
// one prompt.
type recordingFrontend struct {
	mu     sync.Mutex
	events []any
	tasks  []Task
	prompt string
}

func (f *recordingFrontend) Event(ev any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, ev)
}

func (f *recordingFrontend) Run(ctx context.Context, c Controls) int {
	if _, err := c.Submit(agent.Prompt{Text: f.prompt}); err != nil {
		return 1
	}
	if err := c.Wait(ctx); err != nil {
		return 1
	}
	f.mu.Lock()
	f.tasks = c.Tasks()
	f.mu.Unlock()
	c.Quit("prompt_input_exit")
	return 0
}

// planTool is a feature's tool that sets the session's task list.
type planTool struct{ s *Session }

func (planTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "Plan", Description: "Writes the task list.", InputSchema: json.RawMessage(`{"type":"object","properties":{"done":{"type":"integer"}}}`)}
}

func (p planTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	var in struct {
		Done int `json:"done"`
	}
	_ = json.Unmarshal(call.Input, &in)
	tasks := []Task{
		{Content: "Read the code", ActiveForm: "Reading the code", Status: TaskPending},
		{Content: "Fix the flaky test", ActiveForm: "Fixing the flaky test", Status: TaskPending},
		{Content: "Run the tests", ActiveForm: "Running the tests", Status: TaskPending},
	}
	for i := range tasks {
		switch {
		case i < in.Done:
			tasks[i].Status = TaskCompleted
		case i == in.Done:
			tasks[i].Status = TaskInProgress
		}
	}
	p.s.Tasks(tasks)
	return core.TextResult("the list is set"), nil
}

func planFeature() Feature {
	return Feature{
		Name:  "plan",
		Tools: []ToolName{{Name: "Plan", Quiet: true}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{Tools: []core.Tool{planTool{s: s}}}, nil
		},
	}
}

// A feature sets the session's task list. The plain session prints it when
// it changes, /status shows it, and another front end is handed all of it
// as an event and may ask for it at any time.
func TestTaskList(t *testing.T) {
	script := func() []apptest.Reply {
		return []apptest.Reply{
			{Calls: []apptest.Call{{ID: "c1", Name: "Plan", Args: `{"done":1}`}}},
			{Calls: []apptest.Call{{ID: "c2", Name: "Plan", Args: `{"done":1}`}}},
			{Text: "The list is set."},
		}
	}
	w := apptest.NewWorld(t, script()...)
	s := sitWith(t, w, func(o *Options) { o.Features = []Feature{planFeature()} })
	s.see("Type a prompt and Enter.")
	s.typeLine("plan the work")
	s.see("tasks (1 of 3 done):")
	s.see("  [x] Read the code")
	s.see("  [>] Fixing the flaky test")
	s.see("  [ ] Run the tests")
	s.see("The list is set.")
	// The same list set again is not printed again.
	if n := strings.Count(s.out.String(), "tasks (1 of 3 done):"); n != 1 {
		t.Errorf("the task list was printed %d times, want once", n)
	}
	s.typeLine("/status")
	s.see("tasks        [x] Read the code")
	s.see("             [>] Fixing the flaky test")
	s.typeLine("/exit")
	s.exit()
	// The list's words are not in the log: they may be erased later.
	logs, _ := filepath.Glob(filepath.Join(w.DataDir(), "projects", "*", "gnosh.log"))
	data, _ := os.ReadFile(logs[0])
	has(t, "the log", string(data), "the task list has 3 lines, 1 of them done")
	hasNot(t, "the log", string(data), "Fixing the flaky test")

	// What another front end receives.
	w = apptest.NewWorld(t, script()...)
	front := &recordingFrontend{prompt: "plan the work"}
	o := testOptions(w)
	o.Features = []Feature{planFeature()}
	o.StdinIsTerminal, o.StdoutIsTerminal = true, true
	o.Interactive = func(FrontendInfo) Frontend { return front }
	runWith(t, t.Context(), o).wantCode(t, 0)
	var got []*TasksEvent
	for _, ev := range front.events {
		if te, ok := ev.(*TasksEvent); ok {
			got = append(got, te)
		}
	}
	if len(got) != 1 || len(got[0].Tasks) != 3 || got[0].Tasks[1] != (Task{Content: "Fix the flaky test", ActiveForm: "Fixing the flaky test", Status: TaskInProgress}) {
		t.Errorf("the front end was sent %d task events: %+v", len(got), got)
	}
	if len(front.tasks) != 3 || front.tasks[0].Status != TaskCompleted {
		t.Errorf("Controls.Tasks: %+v", front.tasks)
	}
}

// A run that is given a flag a feature claims and no prompt at all: the
// feature supplies the run's first prompt. Without one, a run with no
// prompt is refused as before.
func TestRunPromptFromAFeature(t *testing.T) {
	goal := Feature{
		Name:  "goal",
		Built: []string{"--goal"},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{RunPrompt: func(ctx context.Context) (agent.Prompt, bool) {
				if s.Invocation.Goal == "" {
					return agent.Prompt{}, false
				}
				return agent.Prompt{Text: "Work until this holds: " + s.Invocation.Goal}, true
			}}, nil
		},
	}
	w := apptest.NewWorld(t, apptest.Reply{Text: "The tests pass now."})
	o := testOptions(w, "run", "--goal", "the tests pass")
	o.Features = []Feature{goal}
	r := runWith(t, t.Context(), o).wantCode(t, 0)
	if r.out != "The tests pass now.\n" {
		t.Errorf("stdout = %q\nstderr: %s", r.out, r.err)
	}
	if users := userTexts(w.Model.Requests()[0]); len(users) != 1 || !strings.HasSuffix(users[0], "Work until this holds: the tests pass") {
		t.Errorf("the run's prompt: %q", users)
	}
	// A prompt that was typed is the run's prompt; the feature is not asked.
	w = apptest.NewWorld(t, apptest.Reply{Text: "Typed."})
	o = testOptions(w, "run", "--goal", "the tests pass", "fix the build")
	o.Features = []Feature{goal}
	runWith(t, t.Context(), o).wantCode(t, 0)
	if users := userTexts(w.Model.Requests()[0]); len(users) != 1 || !strings.HasSuffix(users[0], "fix the build") {
		t.Errorf("the run's prompt: %q", users)
	}
	// No prompt and no feature with one.
	w = apptest.NewWorld(t)
	o = testOptions(w, "run")
	o.Features = []Feature{goal}
	r = runWith(t, t.Context(), o).wantCode(t, 1)
	has(t, "stderr", r.err, "the prompt is empty")
}

// modeTool is a feature's tool that says which permission mode is in
// force at the moment it runs.
type modeTool struct{ s *Session }

func (modeTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "Mode", Description: "Says the permission mode.", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}
}

func (m modeTool) Run(context.Context, core.ToolCall, *core.ToolEnv) (core.ToolResult, error) {
	return core.TextResult("the mode is " + m.s.Mode()), nil
}

// The permission mode can change while a session runs. A feature reads
// the mode in force whenever it needs it, and is told of each change.
func TestFeatureSeesThePermissionModeChange(t *testing.T) {
	var mu sync.Mutex
	var changes []string
	feature := Feature{
		Name:  "mode",
		Tools: []ToolName{{Name: "Mode", Quiet: true}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{
				Tools: []core.Tool{modeTool{s: s}},
				ModeChanged: func(mode string) {
					mu.Lock()
					changes = append(changes, mode)
					mu.Unlock()
				},
			}, nil
		},
	}
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Mode", Args: `{}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Read", Args: `{"file_path":"a.txt"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "Edit", Args: `{"file_path":"a.txt","old_string":"two plus","new_string":"three plus"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c4", Name: "Mode", Args: `{}`}}},
		apptest.Reply{Text: "Done."})
	w.Write("a.txt", "two plus two\n")
	s := sitWith(t, w, func(o *Options) { o.Features = []Feature{feature} })
	s.see("Type a prompt and Enter.")
	s.typeLine("change the file")
	s.see("? Allow Edit?")
	s.see("  3. Yes, and switch to acceptEdits for the rest of the session")
	s.typeLine("3")
	s.see("Done.")
	s.see("permission mode acceptEdits")
	s.typeLine("/exit")
	s.exit()
	has(t, "what the tool saw before the change", toolResult(t, w, 1, "c1"), "the mode is default")
	has(t, "what the tool saw after the change", toolResult(t, w, 4, "c4"), "the mode is acceptEdits")
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(changes, " ") != "acceptEdits" {
		t.Errorf("the feature was told of the changes %q, want acceptEdits once", changes)
	}
}

// vaultTool is a feature's tool whose arguments may hold what the person
// later asks to be forgotten, and which asks the person for a secret.
type vaultTool struct{ got *[]string }

func (vaultTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "Vault", Description: "Keeps a thing.", InputSchema: json.RawMessage(`{"type":"object","properties":{"code":{"type":"string"},"ask":{"type":"boolean"}}}`)}
}

func (v vaultTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	var in struct {
		Code string `json:"code"`
		Ask  bool   `json:"ask"`
	}
	_ = json.Unmarshal(call.Input, &in)
	if !in.Ask {
		return core.TextResult("kept: " + in.Code), nil
	}
	ans, err := agent.FromContext(ctx).Ask(ctx, agent.Question{Kind: "secret", Title: "The passphrase of the signing key"})
	if err != nil {
		return core.ErrorResult("nobody could be asked for the passphrase"), nil
	}
	*v.got = append(*v.got, ans.Text)
	return core.TextResult(fmt.Sprintf("the passphrase has %d characters", len(ans.Text))), nil
}

func vaultFeature(got *[]string) Feature {
	return Feature{
		Name:  "vault",
		Tools: []ToolName{{Name: "Vault", Quiet: true, NoLogArgs: true}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{Tools: []core.Tool{vaultTool{got: got}}}, nil
		},
	}
}

// What a tool marked NoLogArgs is given and what it answers reach neither
// the log nor the debugging output, with debugging on; and the answer to a
// question that asks for a secret is written nowhere at all.
func TestLogKeepsNoArgumentsAndNoSecrets(t *testing.T) {
	const marked, ordinary, secret = "swordfish-123-in-a-mem-call", "echo an-ordinary-command", "hunter2-is-the-passphrase"
	var got []string
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "Vault", Args: `{"code":"` + marked + `"}`},
			{ID: "c2", Name: "Bash", Args: `{"command":"` + ordinary + `"}`},
			{ID: "c3", Name: "Vault", Args: `{"ask":true}`},
		}},
		apptest.Reply{Text: "Kept."})
	logFile := filepath.Join(w.Root, "debug.log")
	s := sitWith(t, w, func(o *Options) { o.Features = []Feature{vaultFeature(&got)} }, "-y", "--debug", "--debug-file", logFile)
	s.see("Type a prompt and Enter.")
	s.typeLine("keep it")
	s.see("? The passphrase of the signing key")
	s.see("gnosh keeps it nowhere.")
	s.typeLine(secret)
	s.see("Kept.")
	s.typeLine("/exit")
	s.exit()
	if len(got) != 1 || got[0] != secret {
		t.Fatalf("the tool was given %q", got)
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	has(t, "the log", log, "Vault", "an-ordinary-command", "asked (secret): The passphrase of the signing key")
	hasNot(t, "the log", log, marked, "kept: ", secret)
	hasNot(t, "the screen", s.out.String()+s.errs.String(), secret)
	raw, _ := os.ReadFile(w.Sessions()[0])
	hasNot(t, "the session's file", string(raw), secret)

	// --verbose in a run shows the call was made and not what it held.
	w = apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "Vault", Args: `{"code":"` + marked + `"}`},
			{ID: "c2", Name: "Bash", Args: `{"command":"` + ordinary + `"}`},
		}},
		apptest.Reply{Text: "Kept."})
	o := testOptions(w, "-y", "--verbose", "-p", "keep it")
	o.Features = []Feature{vaultFeature(&got)}
	r := runWith(t, t.Context(), o).wantCode(t, 0)
	has(t, "stderr", r.err, "gnosh: Vault\n", "Bash: "+ordinary)
	hasNot(t, "stderr", r.err, marked, "kept: ")
}

// A feature's subcommand is told where things are even when the config
// has a mistake in it: a command that needs no model still works.
func TestFeatureCommandHasItsDirectoriesWithoutAConfig(t *testing.T) {
	var seen *CommandContext
	feature := Feature{
		Name:  "mem",
		Built: []string{"mem"},
		Commands: []Command{{Command: cli.CmdMem, Run: func(ctx context.Context, c *CommandContext) int {
			seen = c
			return 0
		}}},
	}
	w := apptest.NewWorld(t)
	w.Settings(`{"frobnicate": true}`)
	w.Write("sub/a.txt", "x\n")
	o := testOptions(w, "mem", "show")
	o.Cwd = w.Path("sub")
	o.Features = []Feature{feature}
	runWith(t, t.Context(), o).wantCode(t, 0)
	if seen == nil {
		t.Fatal("the command was not run")
	}
	if seen.Config != nil || len(seen.Problems) == 0 {
		t.Errorf("a config with a mistake was handed over as good: %v, %d problems", seen.Config != nil, len(seen.Problems))
	}
	want := Dirs{
		Home: w.Home, Cwd: w.Path("sub"), ProjectRoot: w.Path("sub"),
		Config: w.ConfigDir(), State: w.DataDir(), Cache: filepath.Join(w.Home, ".cache", "gnosh"), Sessions: w.DataDir(),
	}
	if seen.Dirs != want {
		t.Errorf("the directories:\n%+v\nwant:\n%+v", seen.Dirs, want)
	}
	if seen.Sessions == nil || seen.Sessions.Dir != w.DataDir() {
		t.Errorf("the session store: %+v", seen.Sessions)
	}
	// With a good config they are the same directories.
	good := apptest.NewWorld(t)
	o = testOptions(good, "mem", "show")
	o.Features = []Feature{feature}
	runWith(t, t.Context(), o).wantCode(t, 0)
	if seen.Config == nil || seen.Dirs.State != good.DataDir() || seen.Dirs.Config != good.ConfigDir() || seen.Dirs.Cwd != good.Dir {
		t.Errorf("with a good config: %+v", seen.Dirs)
	}
}

// --version names the build by what it was given at link time when the
// program holds no commit, as a build from a source archive does.
func TestVersionOfABuildWithoutACommit(t *testing.T) {
	w := apptest.NewWorld(t)
	for _, tc := range []struct{ id, want string }{
		{"release-2026-11-13", "unknown-time release-2026-11-13 (gnosh)\n"},
		{"devel", "unknown-time unknown-commit (gnosh)\n"},
		{"", "unknown-time unknown-commit (gnosh)\n"},
	} {
		o := testOptions(w, "--version")
		o.Build, o.BuildID = cli.Build{}, tc.id
		if got := runWith(t, t.Context(), o).wantCode(t, 0).out; got != tc.want {
			t.Errorf("with the build id %q, --version prints %q, want %q", tc.id, got, tc.want)
		}
	}
	// A build that knows its commit prints that.
	if got := run(t, w, "--version").out; got != "2026-11-13T00:00:00Z 0123456789ab (gnosh)\n" {
		t.Errorf("--version prints %q", got)
	}
}

// At the end of a run the features are told how the last turn ended
// before they are asked about the run's exit code, and what they say at
// either moment has reached the front end before the result is printed:
// the result is the last line of the stream.
func TestRunEndComesAfterTurnEndedAndBeforeTheResult(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Done."})
	var mu sync.Mutex
	var order []string
	note := func(what string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, what)
	}
	feature := Feature{
		Name: "goal",
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{
				TurnEnded: func(ctx context.Context, res agent.TurnResult) {
					// Slow enough that a front end which did not wait
					// would be ahead of it.
					time.Sleep(50 * time.Millisecond)
					note("turn ended")
					s.Notice(agent.LevelInfo, "goal", "The turn is over, says the feature.")
				},
				RunEnd: func(ctx context.Context, end RunEnd) int {
					note("run end")
					s.Notice(agent.LevelWarning, "goal", "The goal is not met, says the feature.")
					return 2
				},
			}, nil
		},
	}
	o := testOptions(w, "-p", "--output-format", "stream-json", "do it")
	o.Features = []Feature{feature}
	r := runWith(t, t.Context(), o).wantCode(t, 2)
	if got := strings.Join(order, ", "); got != "turn ended, run end" {
		t.Errorf("the feature was told: %s", got)
	}
	lines := jsonLines(t, r.out)
	var kinds []string
	for _, l := range lines {
		kind, _ := l["type"].(string)
		if content, _ := l["content"].(string); strings.Contains(content, "says the feature") {
			kind = content
		}
		kinds = append(kinds, kind)
	}
	n := len(kinds)
	if n < 3 || kinds[n-3] != "The turn is over, says the feature." || kinds[n-2] != "The goal is not met, says the feature." || kinds[n-1] != "result" {
		t.Errorf("the stream ends with %q, want the feature's two lines and then the result", kinds[max(0, n-4):])
	}
	has(t, "stderr", r.err, "The turn is over, says the feature.", "The goal is not met, says the feature.")
}

// The end of a run does not wait for a turn whose end the features will
// never be told of, as when the writing of a turn's end crashed: a session
// that has gone idle is past it.
func TestRunEndDoesNotWaitForATurnNobodyWillBeToldOf(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Done."})
	var took time.Duration
	_, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		if _, err := c.Submit(agent.Prompt{Text: "do it"}); err != nil {
			return 1
		}
		_ = c.Wait(ctx)
		began := time.Now()
		c.RunEnd(agent.TurnResult{Turn: "a-turn-that-never-ended", Reason: agent.EndFailed}, 1)
		took = time.Since(began)
		c.Quit("other")
		return 0
	})
	r.wantCode(t, 0)
	if took > patience {
		t.Errorf("RunEnd waited %s for a turn that is over", took)
	}
}

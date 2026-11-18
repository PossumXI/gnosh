package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/term/vtest"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

// fakeRuns is an app.AgentSource whose runs a test sets, and which keeps
// what was asked of it.
type fakeRuns struct {
	mu          sync.Mutex
	runs        []app.AgentRun
	convs       map[string][]app.TranscriptEntry
	dispatchErr error
	dispatched  []string
	stopped     []string
	forgotten   []string
	opened      []string
	now         func() time.Time
}

func (f *fakeRuns) Runs() []app.AgentRun {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.runs)
}

func (f *fakeRuns) Conversation(id string) []app.TranscriptEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.convs[id])
}

func (f *fakeRuns) set(id string, change func(*app.AgentRun)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.runs {
		if f.runs[i].ID == id {
			change(&f.runs[i])
		}
	}
}

func (f *fakeRuns) Stop(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	f.stopped = append(f.stopped, id)
	f.mu.Unlock()
	f.set(id, func(r *app.AgentRun) { r.State, r.Ended = "stopped", f.now() })
	return "the session " + id + " was stopped. What it did so far is kept: gnosh -r " + id + " opens it.", nil
}

func (f *fakeRuns) Forget(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forgotten = append(f.forgotten, id)
	f.runs = slices.DeleteFunc(f.runs, func(r app.AgentRun) bool { return r.ID == id })
	return nil
}

func (f *fakeRuns) Dispatch(_ context.Context, prompt string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dispatchErr != nil {
		return "", f.dispatchErr
	}
	f.dispatched = append(f.dispatched, prompt)
	id := fmt.Sprintf("01a1eea5-9999-7000-8000-%012d", len(f.dispatched))
	f.runs = append([]app.AgentRun{{ID: id, Short: id[:13], State: "working", Prompt: prompt, Started: f.now()}}, f.runs...)
	return id, nil
}

func (f *fakeRuns) Open(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, id)
	return nil
}

// The ids of the runs of agentRig.
const (
	runTests = "01a1eea5-1111-7000-8000-000000000001"
	runDocs  = "01a1eea5-2222-7000-8000-000000000002"
	runFlaky = "01a1eea5-3333-7000-8000-000000000003"
	runShip  = "01a1eea5-4444-7000-8000-000000000004"
)

// agentRig is an agent view of four runs: two working (one running a
// command, one waiting on the model), one done and one stopped.
type agentRig struct {
	t   *testing.T
	v   *agentView
	src *fakeRuns
	clk *clock
}

func newAgentRig(t *testing.T) *agentRig {
	clk := &clock{at: time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)}
	now := clk.Now()
	src := &fakeRuns{now: clk.Now, runs: []app.AgentRun{
		{ID: runTests, Short: runTests[:13], State: "working", Cwd: "/home/jae/src/app", Folder: "~/src/app",
			Prompt: "run the tests and fix what fails", Started: now.Add(-2 * time.Minute)},
		{ID: runDocs, Short: runDocs[:13], Name: "docs", State: "working", Cwd: "/home/jae/src/app", Folder: "~/src/app",
			Prompt: "write the docs", Started: now.Add(-30 * time.Second)},
		{ID: runFlaky, Short: runFlaky[:13], State: "done", Cwd: "/home/jae/src/lib", Folder: "~/src/lib", Prompt: "fix the flaky test",
			Output: []string{"The test raced on its clock.", "All 42 tests pass."}, Started: now.Add(-time.Hour), Ended: now.Add(-time.Hour + 45*time.Second)},
		{ID: runShip, Short: runShip[:13], State: "stopped", Cwd: "/home/jae/src/app", Folder: "~/src/app", Prompt: "deploy it",
			Output: []string{"Building."}, Started: now.Add(-3 * time.Hour), Ended: now.Add(-3*time.Hour + 5*time.Minute)},
	}, convs: map[string][]app.TranscriptEntry{
		runTests: {
			{Kind: "prompt", Message: core.UserText("run the tests and fix what fails")},
			{Kind: "reply", Message: core.Message{Role: core.RoleAssistant, Blocks: []core.Block{
				{Type: core.BlockText, Text: "Running the tests."},
				core.ToolUse("c1", "Bash", json.RawMessage(`{"command":"go test ./..."}`)),
			}}},
		},
		runDocs: {{Kind: "prompt", Message: core.UserText("write the docs")}},
	}}
	v := newAgentView(DefaultTheme(text.NoColor), text.Metrics{}, clk.Now, app.AgentViewInfo{Source: src, Cwd: "/home/jae/src/app", Home: "/home/jae"})
	v.refresh()
	return &agentRig{t: t, v: v, src: src, clk: clk}
}

// screen is the view at 100 by 20, as plain lines: the rows from the top,
// then the rows at the bottom.
func (g *agentRig) screen() (body, live []string) {
	b, l, _, _ := g.v.render(100, 20)
	for _, s := range b {
		body = append(body, strings.TrimRight(text.Strip(s), " "))
	}
	for _, s := range l {
		live = append(live, strings.TrimRight(text.Strip(s), " "))
	}
	return body, live
}

// keys types a key script into the view.
func (g *agentRig) keys(script string) *agentRig {
	g.t.Helper()
	evs, err := keyscript.Parse(script)
	if err != nil {
		g.t.Fatal(err)
	}
	for _, ev := range evs {
		g.v.input(ev)
	}
	return g
}

// row is the row of the list that names a run, or "".
func row(body []string, name string) string {
	for _, l := range body {
		if strings.Contains(l, " "+name) {
			return l
		}
	}
	return ""
}

// The list: a title that counts the runs, the working ones under Working
// and the rest under Completed, no line left empty between; each run with
// its mark, its name, what it is doing (the command it runs, "thinking"
// while the model has the turn) or how it ended, and its age, frozen at
// the end for one that is over. The first run is chosen, and the last row
// names the keys.
func TestAgentViewLists(t *testing.T) {
	g := newAgentRig(t)
	body, live := g.screen()
	if body[0] != "gnosh agents · 2 working · 2 completed" || body[1] != "Working" || body[4] != "Completed" {
		t.Fatalf("the list:\n%s", strings.Join(body, "\n"))
	}
	for _, c := range []struct{ name, row string }{
		{"01a1eea5-1111", "> ✽ 01a1eea5-1111  $ go test ./..."},
		{"docs", "  ✽ docs           thinking"},
		{"01a1eea5-3333", "  ∙ 01a1eea5-3333  All 42 tests pass."},
		{"01a1eea5-4444", "  ∙ 01a1eea5-4444  stopped · Building."},
	} {
		if r := row(body, c.name); !strings.HasPrefix(r, c.row) {
			t.Errorf("the row of %s is %q, not %q...", c.name, r, c.row)
		}
	}
	for name, age := range map[string]string{"01a1eea5-1111": " 2m", "docs": " 30s", "01a1eea5-3333": " 45s", "01a1eea5-4444": " 5m"} {
		if r := row(body, name); !strings.HasSuffix(r, age) || len([]rune(r)) != 100 {
			t.Errorf("the row of %s does not end at the right edge with its age %q: %q", name, age, r)
		}
	}
	if live[len(live)-1] != "↑↓ choose · Space peeks · Enter watches · Ctrl+X stops · ? keys · Esc leaves" {
		t.Errorf("the keys line: %q", live[len(live)-1])
	}
	if len(body)+len(live) != 20 {
		t.Errorf("the view is %d rows, not 20", len(body)+len(live))
	}
	// A minute on, the working runs are older and the others are not.
	g.clk.add(time.Minute)
	body, _ = g.screen()
	if !strings.HasSuffix(row(body, "docs"), " 1m") || !strings.HasSuffix(row(body, "01a1eea5-3333"), " 45s") {
		t.Errorf("a minute on:\n%s", strings.Join(body, "\n"))
	}
}

// Down chooses the next run; Space peeks at it (its folder, its prompt,
// what it did last, and for one that is over what it printed) and closes
// again; ? shows the keys; Esc closes them, and with nothing open leaves.
func TestAgentViewPeekAndKeys(t *testing.T) {
	g := newAgentRig(t)
	g.keys("<Space>")
	body, _ := g.screen()
	peek := strings.Join(body, "\n")
	for _, want := range []string{"─ 01a1eea5-1111 · working 2m · ~/src/app ─", "> run the tests and fix what fails", "  Running the tests.", "  ▸ $ go test ./..."} {
		if !strings.Contains(peek, want) {
			t.Errorf("peeking does not show %q:\n%s", want, peek)
		}
	}
	if strings.Count(peek, "run the tests and fix what fails") != 1 {
		t.Errorf("peeking shows the prompt twice:\n%s", peek)
	}
	g.keys("<Down> <Down>")
	body, _ = g.screen()
	peek = strings.Join(body, "\n")
	if !strings.HasPrefix(row(body, "01a1eea5-3333"), "> ") || !strings.Contains(peek, "─ 01a1eea5-3333 · done 45s · ~/src/lib ─") ||
		!strings.Contains(peek, "  The test raced on its clock.\n  All 42 tests pass.") {
		t.Errorf("peeking at the run that is done:\n%s", peek)
	}
	g.keys("<Space>")
	if body, _ = g.screen(); strings.Contains(strings.Join(body, "\n"), "─ 01a1eea5-3333") {
		t.Errorf("Space did not close the peek")
	}
	g.keys(`"?"`)
	if body, _ = g.screen(); !strings.Contains(strings.Join(body, "\n"), "─ keys ─") {
		t.Errorf("? did not show the keys")
	}
	g.keys("<Esc>")
	if body, _ = g.screen(); strings.Contains(strings.Join(body, "\n"), "─ keys ─") || g.v.quit {
		t.Errorf("Esc did not just close the keys")
	}
	g.keys("<End>")
	if body, _ = g.screen(); !strings.HasPrefix(row(body, "01a1eea5-4444"), "> ") {
		t.Errorf("End did not choose the last run")
	}
	g.keys("<Esc>")
	if !g.v.quit {
		t.Errorf("Esc with nothing open did not leave")
	}
}

// A prompt and Enter starts a run in the view's folder and chooses it; a
// prompt too short is taken for a slip. Ctrl+C empties the box, and twice
// on an empty box leaves. A run that could not be started says why.
func TestAgentViewDispatch(t *testing.T) {
	g := newAgentRig(t)
	g.keys(`"hi" <Enter>`)
	if _, live := g.screen(); live[len(live)-1] != "Too short: a prompt of 4 characters or more starts a run." || len(g.src.dispatched) != 0 {
		t.Errorf("a short prompt: %q, %q", live, g.src.dispatched)
	}
	g.keys("<Ctrl+C>")
	if g.v.box.Text() != "" || g.v.quit {
		t.Errorf("Ctrl+C did not empty the box: %q", g.v.box.Text())
	}
	g.keys(`"tidy the README"`)
	if _, live := g.screen(); live[0] != "> tidy the README" || live[1] != "Enter starts a run in ~/src/app · Esc empties the box" {
		t.Errorf("the box with a prompt: %q", live)
	}
	g.keys("<Enter>")
	body, live := g.screen()
	if !slices.Equal(g.src.dispatched, []string{"tidy the README"}) || g.v.box.Text() != "" {
		t.Fatalf("the run was not started: %q", g.src.dispatched)
	}
	if body[0] != "gnosh agents · 3 working · 2 completed" || !strings.HasPrefix(row(body, "01a1eea5-9999"), "> ✽ 01a1eea5-9999") ||
		live[len(live)-1] != "Started 01a1eea5-9999: Enter watches it." {
		t.Errorf("after the start:\n%s\n%s", strings.Join(body, "\n"), strings.Join(live, "\n"))
	}
	g.v.info.Flags = []string{"--permission-mode=acceptEdits"}
	g.keys(`"tidy"`)
	if _, live := g.screen(); live[1] != "Enter starts a run in ~/src/app with --permission-mode=acceptEdits · Esc empties the box" {
		t.Errorf("the box with a prompt and the view's flags: %q", live)
	}
	g.v.box.Reset()
	g.src.dispatchErr = errors.New("the folder is not trusted")
	g.keys(`"tidy it again" <Enter>`)
	if _, live := g.screen(); live[len(live)-1] != "The run was not started: the folder is not trusted." {
		t.Errorf("a start that failed: %q", live)
	}
	g.keys("<Ctrl+C>")
	if _, live := g.screen(); live[len(live)-1] != "Ctrl+C again leaves the view; the runs go on." || g.v.quit {
		t.Errorf("the first Ctrl+C on an empty box: %q", live)
	}
	g.keys("<Ctrl+C>")
	if !g.v.quit {
		t.Errorf("the second Ctrl+C did not leave")
	}
}

// What the box holds is a filter when its words begin with n:, s: or o:
// (the name or prompt, the state, what it printed): the list shows only
// what passes it, and Enter opens the run chosen among them.
func TestAgentViewFilters(t *testing.T) {
	g := newAgentRig(t)
	for _, c := range []struct {
		filter string
		names  []string
	}{
		{"s:work", []string{"01a1eea5-1111", "docs"}},
		{"s:stop", []string{"01a1eea5-4444"}},
		{"s:completed", []string{"01a1eea5-3333", "01a1eea5-4444"}},
		{"n:DOCS", []string{"docs"}},
		{"n:deploy s:stopped", []string{"01a1eea5-4444"}},
		{"o:42", []string{"01a1eea5-3333"}},
		{"n:nothing", nil},
	} {
		g.v.box.Reset()
		g.keys(fmt.Sprintf("%q", c.filter))
		body, live := g.screen()
		var names []string
		for _, n := range []string{"01a1eea5-1111", "docs", "01a1eea5-3333", "01a1eea5-4444"} {
			if row(body, n) != "" {
				names = append(names, n)
			}
		}
		if !slices.Equal(names, c.names) {
			t.Errorf("%s lists %q, not %q", c.filter, names, c.names)
		}
		if c.names == nil && !strings.Contains(strings.Join(body, "\n"), "No run is like that.") {
			t.Errorf("%s with no match:\n%s", c.filter, strings.Join(body, "\n"))
		}
		if live[len(live)-1] != "Enter opens the run chosen · Esc empties the box" {
			t.Errorf("%s: the keys line is %q", c.filter, live[len(live)-1])
		}
	}
	g.v.box.Reset()
	g.keys(`"s:stopped" <Enter>`)
	if g.v.open != runShip || len(g.src.dispatched) != 0 {
		t.Errorf("Enter on a filter opened %q and started %q", g.v.open, g.src.dispatched)
	}
}

// Ctrl+X stops a working run; on one that is over it asks first, and the
// second press within two seconds takes it off the list. Later than that
// it asks again.
func TestAgentViewStopAndForget(t *testing.T) {
	g := newAgentRig(t)
	g.keys("<Ctrl+X>")
	body, live := g.screen()
	if !slices.Equal(g.src.stopped, []string{runTests}) || !strings.HasPrefix(row(body, "01a1eea5-1111"), "> ∙ 01a1eea5-1111  stopped") {
		t.Fatalf("Ctrl+X on a working run: stopped %q\n%s", g.src.stopped, strings.Join(body, "\n"))
	}
	if live[len(live)-1] != "01a1eea5-1111 was stopped. What it did so far is kept: gnosh -r 01a1eea5-1111 opens it" {
		t.Errorf("what was said of the stop: %q", live[len(live)-1])
	}
	g.v.sel = runFlaky
	g.keys("<Ctrl+X>")
	if _, live = g.screen(); live[len(live)-1] != "Ctrl+X again takes 01a1eea5-3333 off the list; its session is kept." || len(g.src.forgotten) != 0 {
		t.Errorf("the first Ctrl+X on a run that is over: %q, forgot %q", live, g.src.forgotten)
	}
	g.keys("<Ctrl+X>")
	body, live = g.screen()
	if !slices.Equal(g.src.forgotten, []string{runFlaky}) || row(body, "01a1eea5-3333") != "" ||
		live[len(live)-1] != "01a1eea5-3333 is off the list. Its session is kept: gnosh -r 01a1eea5-3333 opens it." {
		t.Errorf("the second Ctrl+X: forgot %q\n%s\n%s", g.src.forgotten, strings.Join(body, "\n"), live[len(live)-1])
	}
	g.v.sel = runShip
	g.keys("<Ctrl+X>")
	g.clk.add(3 * time.Second)
	g.v.tick()
	g.keys("<Ctrl+X>")
	if len(g.src.forgotten) != 1 {
		t.Errorf("a second Ctrl+X 3 s later took the run off the list")
	}
}

// Enter on a working run watches its conversation as the transcript view
// lays it out, and what is written to it since shows; the last row says
// whose it is and how it stands. Esc goes back. Enter on a run that is
// over opens it; so does Enter on one watched once it is over.
func TestAgentViewWatch(t *testing.T) {
	g := newAgentRig(t)
	g.keys("<Enter>")
	if g.v.watch == nil {
		t.Fatal("Enter did not watch the working run")
	}
	body, live := g.screen()
	all := strings.Join(body, "\n")
	for _, want := range []string{"> run the tests and fix what fails", "Running the tests.", "▸ Bash", `"command": "go test ./..."`} {
		if !strings.Contains(all, want) {
			t.Errorf("the run watched does not show %q:\n%s", want, all)
		}
	}
	if len(live) != 1 || live[0] != "watching 01a1eea5-1111 · working 2m · Ctrl+X stops it · Esc goes back" {
		t.Errorf("the last row: %q", live)
	}
	g.src.mu.Lock()
	g.src.convs[runTests] = append(g.src.convs[runTests], app.TranscriptEntry{Kind: "results", Message: core.Message{Role: core.RoleUser,
		Blocks: []core.Block{core.ToolResultBlock("c1", core.TextResult("ok  example.com/app  0.4s"))}}})
	g.src.mu.Unlock()
	g.clk.add(watchLook)
	g.v.tick()
	if body, _ = g.screen(); !strings.Contains(strings.Join(body, "\n"), "ok  example.com/app  0.4s") {
		t.Errorf("what was written since does not show:\n%s", strings.Join(body, "\n"))
	}
	g.src.set(runTests, func(r *app.AgentRun) { r.State, r.Ended = "done", g.clk.Now() })
	g.clk.add(agentLook)
	g.v.tick()
	if _, live = g.screen(); live[0] != "watching 01a1eea5-1111 · done 2m · Enter opens it · Esc goes back" {
		t.Errorf("the last row once it is over: %q", live)
	}
	g.keys("<Esc>")
	if g.v.watch != nil || g.v.quit {
		t.Fatalf("Esc did not go back to the list")
	}
	g.keys("<Enter>")
	if g.v.watch != nil || g.v.open != runTests {
		t.Errorf("Enter on a run that is over: watch %v, open %q", g.v.watch, g.v.open)
	}
}

// The view on a terminal: it takes the whole screen; Enter on a run that
// is over gives the terminal to gnosh -r and takes it back after, the list
// drawn again; Esc leaves, and the terminal is put back.
func TestAgentViewOnATerminal(t *testing.T) {
	g := newAgentRig(t)
	ft := newFakeTerm(100, 20, vtest.AllAnswers)
	open := NewAgentView(Options{Open: func(TerminalHooks) (Terminal, error) { return ft, nil }, Now: g.clk.Now})
	done := make(chan int, 1)
	go func() { done <- open(app.AgentViewInfo{Source: g.src, Cwd: "/home/jae/src/app", Home: "/home/jae"}) }()
	l := &loop{t: t, ft: ft}
	l.has("gnosh agents · 2 working · 2 completed", "$ go test ./...")
	l.keys("<Down> <Down> <Enter>")
	l.eventually("gnosh -r ran", func(string) bool {
		g.src.mu.Lock()
		defer g.src.mu.Unlock()
		return slices.Equal(g.src.opened, []string{runFlaky})
	})
	l.has("All 42 tests pass.")
	ft.mu.Lock()
	released, resumed := ft.released, ft.resumed
	ft.mu.Unlock()
	if released != 1 || resumed != 1 {
		t.Errorf("the terminal was released %d times and taken back %d", released, resumed)
	}
	l.keys("<Esc>")
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("the view ended with %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Esc did not leave the view")
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	if ft.closed != 1 {
		t.Errorf("the terminal was closed %d times", ft.closed)
	}
}

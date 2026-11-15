package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// The plain words for a deliverable: one line when it is claimed, one
// when it is delivered, with the address and the file whole.
func TestDeliverableWords(t *testing.T) {
	const address = "http://127.0.0.1:7777/k3JQm3fK0aP9xR2tV5wY8zB1dE4gH7jL0nQ3sU6vX9y/ad.mp4"
	pending := Deliverable{Label: "demo reel", Pending: true}
	file := Deliverable{Label: "hebrew ad video", Path: "/root/renders/ad.mp4", Host: "render", URL: address}
	local := Deliverable{Label: "notes", Path: "/home/dev/notes.md"}
	link := Deliverable{Label: "landing\n page", URL: "https://example.com/landing"}
	redo := file
	redo.Pending = true

	for _, tc := range []struct {
		d        Deliverable
		replaced bool
		state    string
		line     string
	}{
		{pending, false, "pending", "deliverable pending: demo reel"},
		{file, false, "delivered", "deliverable: hebrew ad video | " + address + " | /root/renders/ad.mp4 on render"},
		{file, true, "new version", "deliverable, new version: hebrew ad video | " + address + " | /root/renders/ad.mp4 on render"},
		{local, false, "delivered", "deliverable: notes | /home/dev/notes.md"},
		{link, false, "delivered", "deliverable: landing page | https://example.com/landing"},
		{redo, false, "a new version is pending", "deliverable, a new version is pending: hebrew ad video | " + address + " | /root/renders/ad.mp4 on render"},
	} {
		if got := tc.d.State(tc.replaced); got != tc.state {
			t.Errorf("State of %+v = %q, want %q", tc.d, got, tc.state)
		}
		if got := tc.d.Line(tc.replaced); got != tc.line {
			t.Errorf("Line of %+v =\n%s\nwant\n%s", tc.d, got, tc.line)
		}
	}
	if !file.Delivered() || file.Link() || !link.Delivered() || !link.Link() || pending.Delivered() || pending.Link() || !redo.Delivered() {
		t.Errorf("what is delivered and what is a link is told wrongly")
	}
	if file.Where() != "/root/renders/ad.mp4 on render" || local.Where() != "/home/dev/notes.md" || link.Where() != "" {
		t.Errorf("where the files are: %q, %q, %q", file.Where(), local.Where(), link.Where())
	}

	// The counts: an entry whose newer version is on its way is a
	// promise that is open.
	list := []Deliverable{pending, file, local, link}
	if d, p := CountDeliverables(list); d != 3 || p != 1 {
		t.Errorf("the list counts %d delivered and %d pending", d, p)
	}
	if d, p := CountDeliverables([]Deliverable{redo, {Label: "nothing"}}); d != 0 || p != 1 {
		t.Errorf("a list with a newer version on its way counts %d delivered and %d pending", d, p)
	}
	for sep, want := range map[string]string{" · ": "3 delivered · 1 pending", ", ": "3 delivered, 1 pending"} {
		if got := DeliverableCount(list, sep); got != want {
			t.Errorf("DeliverableCount = %q, want %q", got, want)
		}
	}
	if got := DeliverableCount([]Deliverable{pending}, ", "); got != "1 pending" {
		t.Errorf("a list of one claim counts as %q", got)
	}
	if got := DeliverableCount(nil, ", "); got != "" {
		t.Errorf("an empty list counts as %q", got)
	}

	// The list in plain lines, and how an address that answers here only
	// is reached from elsewhere.
	got := strings.Join(DeliverablesText([]Deliverable{pending, redo, link}), "\n")
	want := strings.Join([]string{
		"Deliverables, the latest version of each (1 delivered, 2 pending):",
		"  demo reel (pending)",
		"  hebrew ad video (a new version is pending) | " + address + " | /root/renders/ad.mp4 on render",
		"  landing page | https://example.com/landing",
		"An address at 127.0.0.1:7777 answers on this machine only. From another machine, forward the port first: ssh -L 7777:127.0.0.1:7777 <this machine>",
	}, "\n")
	if got != want {
		t.Errorf("the list in plain lines:\n%s\nwant\n%s", got, want)
	}
	if lines := DeliverablesText(nil); len(lines) != 1 || !strings.HasPrefix(lines[0], "Nothing has been claimed or delivered in this session yet.") {
		t.Errorf("an empty list in plain lines: %q", lines)
	}
	for _, list := range [][]Deliverable{{pending}, {link}, {local}, {{Label: "x", Path: "/x", URL: "https://example.com/x"}}} {
		if hint := DeliverablesHint(list); hint != "" {
			t.Errorf("the list %+v, which holds no address of this machine alone, is given the hint %q", list, hint)
		}
	}
}

// What a result of the Deliver tool holds is read back the same way from
// the tool's own hands and from a session's file.
func TestDeliverableOfAResult(t *testing.T) {
	d := Deliverable{Label: "hebrew ad video", Path: "/root/renders/ad.mp4", Host: "render", URL: "http://127.0.0.1:7777/t/ad.mp4"}
	meta := map[string]any{MetaDeliverable: d, MetaReplaced: true}
	if got, replaced, ok := DeliverableOf(meta); !ok || !replaced || got != d {
		t.Errorf("from the tool: %+v, %v, %v", got, replaced, ok)
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if got, replaced, ok := DeliverableOf(back); !ok || !replaced || got != d {
		t.Errorf("from JSON: %+v, %v, %v", got, replaced, ok)
	}
	if got, _, ok := DeliverableOf(map[string]any{MetaDeliverable: &d}); !ok || got != d {
		t.Errorf("from a pointer: %+v, %v", got, ok)
	}
	for name, meta := range map[string]map[string]any{
		"no facts":          nil,
		"another tool's":    {"exitCode": 0},
		"a label of blanks": {MetaDeliverable: Deliverable{Label: "  "}},
		"something else":    {MetaDeliverable: "hebrew ad video"},
		"a nil pointer":     {MetaDeliverable: (*Deliverable)(nil)},
		"an empty object":   {MetaDeliverable: map[string]any{}},
	} {
		if got, _, ok := DeliverableOf(meta); ok {
			t.Errorf("%s: read as the deliverable %+v", name, got)
		}
	}
}

// handTool is a tool as the Deliver tool is, for the tests of what the
// program does around it: it records the deliverable its arguments name,
// says what a call would do the way its arguments ask, and with a host
// runs nothing but says which command it would.
type handTool struct {
	ran *[]string
}

func (handTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: DeliverToolName, Description: "Records a deliverable.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"label":{"type":"string"},"target":{"type":"string"},"host":{"type":"string"}},"required":["label"]}`)}
}

type handArgs struct {
	Label  string `json:"label"`
	Target string `json:"target"`
	Host   string `json:"host"`
}

func (handTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	var in handArgs
	_ = json.Unmarshal(call.Input, &in)
	switch {
	case in.Target == "":
		return core.Action{Kind: core.ActionOther, Summary: "Claim " + in.Label}, nil
	case in.Host != "" && in.Host != "local":
		return core.Action{Kind: core.ActionExecute, Command: "cat '" + in.Target + "'", Summary: "Copy " + in.Target + " back from " + in.Host}, nil
	}
	return core.Action{Kind: core.ActionRead, Paths: []string{handPath(env, in.Target)}, Summary: "Hand over " + in.Target}, nil
}

// handPath is the file a target names: a relative one is under the
// working directory.
func handPath(env *core.ToolEnv, target string) string {
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(env.Cwd, target)
}

func (h handTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	var in handArgs
	_ = json.Unmarshal(call.Input, &in)
	*h.ran = append(*h.ran, in.Label)
	d := Deliverable{Label: in.Label, Pending: in.Target == "", Host: in.Host}
	if in.Target != "" {
		d.Path = in.Target
		if in.Host == "" {
			d.Path, d.URL = handPath(env, in.Target), "http://127.0.0.1:7777/token/"+filepath.Base(in.Target)
		}
	}
	res := core.TextResult("Recorded.")
	res.Meta = map[string]any{MetaDeliverable: d, MetaReplaced: false}
	return res, nil
}

// handFeature is a feature that brings handTool and keeps the session's
// list of deliverables the way the real one does: an entry joins it when
// its result is in the session's file.
func handFeature(ran *[]string) Feature {
	return Feature{
		Name:  "hand",
		Tools: []ToolName{{Name: DeliverToolName, Quiet: true}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			if s.SubAgent {
				return &Contribution{}, nil
			}
			var list []Deliverable
			return &Contribution{
				Tools: []core.Tool{handTool{ran: ran}},
				AfterStored: func(ctx context.Context, e agent.Entry, written []transcript.Entry) {
					if d, _, ok := DeliverableOf(e.Result.Meta); ok && e.Kind == agent.EntryResult {
						list = append(slices.DeleteFunc(list, func(old Deliverable) bool { return old.Label == d.Label }), d)
						s.Deliverables(list)
					}
				},
				Ready: func(*agent.Agent) {
					if s.Shell() == nil {
						panic("a feature is not given the session's shell manager by the time it is ready")
					}
				},
			}, nil
		},
	}
}

func deliverCall(id string, args map[string]string) apptest.Reply {
	raw, _ := json.Marshal(args)
	return apptest.Reply{Calls: []apptest.Call{{ID: id, Name: DeliverToolName, Args: string(raw)}}}
}

// The plain session prints a deliverable as one line when it is claimed
// and one when it is delivered, in place of the call's own two lines, and
// counts them in the line that ends a turn. A call that did not go well
// is shown as any other call is.
func TestPlainSessionShowsDeliverables(t *testing.T) {
	var ran []string
	w := apptest.NewWorld(t,
		deliverCall("c1", map[string]string{"label": "hebrew ad video"}),
		deliverCall("c2", map[string]string{"label": "hebrew ad video", "target": "out/ad.mp4"}),
		apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: DeliverToolName, Args: `{"label":"notes","label":"twice"}`}}},
		apptest.Reply{Text: "The video is there."})
	s := sitWith(t, w, func(o *Options) { o.Features = []Feature{handFeature(&ran)} })
	s.see("Type a prompt and Enter.")
	s.typeLine("make the video")
	s.see("deliverable pending: hebrew ad video\n")
	s.see("deliverable: hebrew ad video | http://127.0.0.1:7777/token/ad.mp4 | " + filepath.Join(w.Dir, "out/ad.mp4") + "\n")
	s.see("* Deliver: ")
	s.see("  = failed: This call did not run")
	s.see("The video is there.")
	s.see(" | deliverables: 1 delivered\n")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}
	screen := s.out.String()
	hasNot(t, "the screen", screen, "* Deliver: hebrew ad video", "  = ok")
	if !slices.Equal(ran, []string{"hebrew ad video", "hebrew ad video"}) {
		t.Errorf("the tool ran for %q", ran)
	}
}

// The session's list of deliverables is what a feature last set: the
// front end is told when it changes, and can ask for it.
func TestDeliverablesReachTheFrontEnd(t *testing.T) {
	var ran []string
	w := apptest.NewWorld(t,
		deliverCall("c1", map[string]string{"label": "demo reel"}),
		deliverCall("c2", map[string]string{"label": "notes", "target": "notes.md"}),
		apptest.Reply{Text: "Done."})
	var atStart, atEnd []Deliverable
	front := &testFront{run: func(ctx context.Context, c SessionControls, f *testFront) int {
		atStart = c.Deliverables()
		if _, err := c.Submit(agent.Prompt{Text: "make them"}); err != nil {
			t.Error(err)
		}
		if err := c.Wait(ctx); err != nil {
			t.Error(err)
		}
		atEnd = c.Deliverables()
		// What comes back is a copy: changing it changes nothing.
		atEnd[0].Label = "changed"
		if again := c.Deliverables(); again[0].Label != "demo reel" {
			t.Errorf("the list a front end is given is the session's own: %+v", again)
		}
		atEnd[0].Label = "demo reel"
		c.Quit("prompt_input_exit")
		return 0
	}}
	o := testOptions(w)
	o.Features = []Feature{handFeature(&ran)}
	o.StdinIsTerminal, o.StdoutIsTerminal = true, true
	o.Signals = make(chan os.Signal, 4)
	o.Interactive = func(FrontendInfo) Frontend { return front }
	runWith(t, t.Context(), o).wantCode(t, 0)
	_, lists, _ := front.told()

	reel := Deliverable{Label: "demo reel", Pending: true}
	notes := Deliverable{Label: "notes", Path: filepath.Join(w.Dir, "notes.md"), URL: "http://127.0.0.1:7777/token/notes.md"}
	if len(atStart) != 0 {
		t.Errorf("a new session has the deliverables %+v", atStart)
	}
	if !slices.Equal(atEnd, []Deliverable{reel, notes}) {
		t.Errorf("the session's deliverables: %+v", atEnd)
	}
	if len(lists) != 2 || !slices.Equal(lists[0], []Deliverable{reel}) || !slices.Equal(lists[1], []Deliverable{reel, notes}) {
		t.Errorf("the lists the front end was told of: %+v", lists)
	}

	// The facts of a call's result read back from the session's file.
	entries := sessionEntriesOf(t, w)
	var kept []Deliverable
	var others int
	for i := range entries {
		tool, meta, ok := ResultMeta(&entries[i])
		if !ok {
			others++
			continue
		}
		if tool != DeliverToolName {
			t.Errorf("a result of the tool %q", tool)
		}
		if d, _, ok := DeliverableOf(meta); ok {
			kept = append(kept, d)
		}
	}
	if !slices.Equal(kept, []Deliverable{reel, notes}) || others == 0 {
		t.Errorf("the session's file gives back %+v out of %d entries", kept, len(entries))
	}
}

// sessionEntriesOf reads the entries of a world's one session file.
func sessionEntriesOf(t *testing.T, w *apptest.World) []transcript.Entry {
	t.Helper()
	files := w.Sessions()
	if len(files) != 1 {
		t.Fatalf("the world has %d session files", len(files))
	}
	s, err := transcript.New(w.DataDir()).OpenPath(context.Background(), files[0], true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	return slices.Clone(s.Entries())
}

// testFront is a front end for a test: what it does with the session's
// controls is the test's to say, and it keeps what it is told for the test
// to read afterwards.
type testFront struct {
	run func(ctx context.Context, c SessionControls, f *testFront) int
	// answer says what the person answers to a question; nil answers no.
	answer func(q agent.Question) agent.Answer

	mu        sync.Mutex
	c         Controls
	questions []agent.Question
	lists     [][]Deliverable
	ends      []*agent.ToolEndEvent
}

// Event keeps what a test reads, and answers a question.
func (f *testFront) Event(ev any) {
	f.mu.Lock()
	c := f.c
	q, asked := ev.(*agent.QuestionEvent)
	switch e := ev.(type) {
	case *agent.QuestionEvent:
		f.questions = append(f.questions, e.Question)
	case *DeliverablesEvent:
		f.lists = append(f.lists, e.List)
	case *agent.ToolEndEvent:
		f.ends = append(f.ends, e)
	}
	f.mu.Unlock()
	if !asked || c == nil {
		return
	}
	ans := agent.Answer{Choice: "no", Text: "not now"}
	if f.answer != nil {
		ans = f.answer(q.Question)
	}
	go func() { _ = c.Answer(q.Question.ID, ans) }()
}

// Run hands the test the session's controls.
func (f *testFront) Run(ctx context.Context, c Controls) int {
	f.mu.Lock()
	f.c = c
	f.mu.Unlock()
	return f.run(ctx, c.(SessionControls), f)
}

// told returns what the front end was told: the questions, the lists of
// deliverables and the ends of tool calls, each in order.
func (f *testFront) told() ([]agent.Question, [][]Deliverable, []*agent.ToolEndEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.questions), slices.Clone(f.lists), slices.Clone(f.ends)
}

// What a call of a feature's tool reaches beyond the feature's own
// keeping is judged as that reach is judged everywhere. A call that runs
// a command on a declared host is judged as that command on that host is:
// it asks, the person's deny rules are held against the command, and with
// permissions skipped it runs. A call that hands over a file is judged as
// reading that file is: inside the working directories it asks nothing,
// outside them it asks, unless a rule allows the reading. A call that
// reaches nothing is the quiet tool's alone.
func TestWhatAFeaturesToolReachesIsJudged(t *testing.T) {
	onRender := map[string]string{"label": "ad", "target": "renders/ad.mp4", "host": "render"}
	session := func(t *testing.T, w *apptest.World, answer func(agent.Question) agent.Answer, args ...string) (ran []string, questions []agent.Question, ends []*agent.ToolEndEvent) {
		t.Helper()
		front := &testFront{answer: answer, run: func(ctx context.Context, c SessionControls, f *testFront) int {
			if _, err := c.Submit(agent.Prompt{Text: "deliver it"}); err != nil {
				t.Error(err)
			}
			if err := c.Wait(ctx); err != nil {
				t.Error(err)
			}
			c.Quit("prompt_input_exit")
			return 0
		}}
		o := testOptions(w, args...)
		o.Features = []Feature{handFeature(&ran)}
		o.StdinIsTerminal, o.StdoutIsTerminal = true, true
		o.Signals = make(chan os.Signal, 4)
		o.Interactive = func(FrontendInfo) Frontend { return front }
		runWith(t, t.Context(), o).wantCode(t, 0)
		questions, _, ends = front.told()
		return ran, questions, ends
	}
	yes := func(agent.Question) agent.Answer { return agent.Answer{Choice: "yes"} }

	t.Run("it asks, naming the host and the command", func(t *testing.T) {
		w, _ := hostWorld(t, deliverCall("c1", onRender), apptest.Reply{Text: "Done."})
		ran, questions, _ := session(t, w, yes)
		if len(questions) != 1 || len(ran) != 1 {
			t.Fatalf("%d questions, and the tool ran for %q", len(questions), ran)
		}
		q := questions[0]
		if q.Title != "Allow Deliver?" {
			t.Errorf("the question's title: %q", q.Title)
		}
		has(t, "the question", q.Text, "Copy renders/ad.mp4 back from render", "  $ cat 'renders/ad.mp4'",
			"Why gnosh asks: Deliver runs a command on another machine for this call, which is judged as that command is. The command runs on the host render.")
	})
	t.Run("a no does not run it", func(t *testing.T) {
		w, _ := hostWorld(t, deliverCall("c1", onRender), apptest.Reply{Text: "Done."})
		ran, questions, ends := session(t, w, nil)
		if len(questions) != 1 || len(ran) != 0 || len(ends) != 1 || !ends[0].Denied {
			t.Errorf("%d questions, the tool ran for %q, and the call's end is %+v", len(questions), ran, ends)
		}
	})
	t.Run("with permissions skipped nothing asks", func(t *testing.T) {
		w, _ := hostWorld(t, deliverCall("c1", onRender), apptest.Reply{Text: "Done."})
		ran, questions, _ := session(t, w, nil, "-y")
		if len(questions) != 0 || len(ran) != 1 {
			t.Errorf("%d questions, and the tool ran for %q", len(questions), ran)
		}
	})
	t.Run("an allow rule for the tool is about this machine", func(t *testing.T) {
		w, _ := hostWorld(t, deliverCall("c1", onRender), apptest.Reply{Text: "Done."})
		w.Allow("Deliver", "Bash(cat *)")
		ran, questions, _ := session(t, w, nil)
		if len(questions) != 1 || len(ran) != 0 {
			t.Errorf("%d questions, and the tool ran for %q", len(questions), ran)
		}
	})
	t.Run("a deny rule for the command refuses it, also with permissions skipped", func(t *testing.T) {
		for _, args := range [][]string{nil, {"-y"}} {
			w, _ := hostWorld(t, deliverCall("c1", onRender), apptest.Reply{Text: "Done."})
			w.Deny("Bash(cat *)")
			ran, questions, ends := session(t, w, yes, args...)
			if len(questions) != 0 || len(ran) != 0 || len(ends) != 1 || !ends[0].Denied {
				t.Fatalf("%v: %d questions, the tool ran for %q, and the call's end is %+v", args, len(questions), ran, ends)
			}
			has(t, "what the model reads", core.TextOf(ends[0].Result.Content), "This call did not run.",
				"Deliver runs a command on another machine for this call, which is judged as that command is.", "The deny rule Bash(cat *)")
		}
	})
	t.Run("a deny rule for the tool refuses it as it does any call", func(t *testing.T) {
		w, _ := hostWorld(t, deliverCall("c1", map[string]string{"label": "ad", "target": "renders/ad.mp4", "host": "web"}), apptest.Reply{Text: "Done."})
		w.Deny("Deliver(host:web)")
		ran, questions, ends := session(t, w, yes)
		if len(questions) != 0 || len(ran) != 0 || len(ends) != 1 || !ends[0].Denied {
			t.Fatalf("%d questions, the tool ran for %q, and the call's end is %+v", len(questions), ran, ends)
		}
		has(t, "what the model reads", core.TextOf(ends[0].Result.Content), "The deny rule Deliver(host:web)")
	})
	t.Run("without a host the tool asks nothing", func(t *testing.T) {
		w, _ := hostWorld(t,
			deliverCall("c1", map[string]string{"label": "ad"}),
			deliverCall("c2", map[string]string{"label": "ad", "target": "ad.mp4"}),
			deliverCall("c3", map[string]string{"label": "ad", "target": "ad.mp4", "host": "local"}),
			deliverCall("c4", map[string]string{"label": "ad", "host": "render"}),
			apptest.Reply{Text: "Done."})
		ran, questions, _ := session(t, w, nil)
		if len(questions) != 0 || len(ran) != 4 {
			t.Errorf("%d questions, and the tool ran for %q", len(questions), ran)
		}
	})
	t.Run("a file is held to the rules about files", func(t *testing.T) {
		w, _ := hostWorld(t, deliverCall("c1", map[string]string{"label": "key", "target": "secret/key.txt"}), apptest.Reply{Text: "Done."})
		w.Deny("Read(./secret/**)")
		ran, questions, ends := session(t, w, yes)
		if len(questions) != 0 || len(ran) != 0 || len(ends) != 1 || !ends[0].Denied {
			t.Fatalf("%d questions, the tool ran for %q, and the call's end is %+v", len(questions), ran, ends)
		}
		has(t, "what the model reads", core.TextOf(ends[0].Result.Content), "The deny rule Read(./secret/**)")
	})
	t.Run("a file outside the working directories asks, as reading it does", func(t *testing.T) {
		outside := func(w *apptest.World) string { return filepath.Join(w.Home, "private", "key.txt") }
		script := func(w *apptest.World) {
			w.Model.Add(deliverCall("c1", map[string]string{"label": "key", "target": outside(w)}), apptest.Reply{Text: "Done."})
		}
		w := apptest.NewWorld(t)
		script(w)
		ran, questions, _ := session(t, w, yes)
		if len(questions) != 1 || len(ran) != 1 {
			t.Fatalf("%d questions, and the tool ran for %q", len(questions), ran)
		}
		has(t, "the question", questions[0].Text, "Hand over "+outside(w),
			"Why gnosh asks: Deliver hands over a file for this call, which is judged as reading that file is. "+outside(w)+" is outside the working directories, and reading there asks first.")
		if questions[0].Title != "Allow Deliver?" {
			t.Errorf("the question's title: %q", questions[0].Title)
		}

		// A no does not run it; a rule that allows the reading, and
		// skipped permissions, each let it run without a question.
		w = apptest.NewWorld(t)
		script(w)
		if ran, questions, _ := session(t, w, nil); len(questions) != 1 || len(ran) != 0 {
			t.Errorf("answered no: %d questions, and the tool ran for %q", len(questions), ran)
		}
		w = apptest.NewWorld(t)
		script(w)
		w.Allow("Read(/" + filepath.Join(w.Home, "private") + "/**)")
		if ran, questions, _ := session(t, w, nil); len(questions) != 0 || len(ran) != 1 {
			t.Errorf("with a rule that allows the reading: %d questions, and the tool ran for %q", len(questions), ran)
		}
		w = apptest.NewWorld(t)
		script(w)
		if ran, questions, _ := session(t, w, nil, "-y"); len(questions) != 0 || len(ran) != 1 {
			t.Errorf("with permissions skipped: %d questions, and the tool ran for %q", len(questions), ran)
		}
	})
	t.Run("where Read is switched off, the tool's own answer stands", func(t *testing.T) {
		w := apptest.NewWorld(t,
			deliverCall("c1", map[string]string{"label": "ad", "target": "ad.mp4"}),
			deliverCall("c2", map[string]string{"label": "key", "target": "secret/key.txt"}),
			apptest.Reply{Text: "Done."})
		w.Deny("Read", "Read(./secret/**)")
		ran, questions, ends := session(t, w, nil)
		if len(questions) != 0 || !slices.Equal(ran, []string{"ad"}) || len(ends) != 2 || ends[0].Denied || !ends[1].Denied {
			t.Fatalf("%d questions, the tool ran for %q, and the calls' ends are %+v", len(questions), ran, ends)
		}
		has(t, "what the model reads", core.TextOf(ends[1].Result.Content), "The deny rule Read(./secret/**)")
	})
}

package deliver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/core"
)

// safeBuffer is a buffer several goroutines may write to and read.
type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// result is how one run of the program ended.
type result struct {
	code     int
	out, err string
}

// wantCode fails the test unless the run ended with the code.
func (r result) wantCode(t *testing.T, code int) result {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit code %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, code, r.out, r.err)
	}
	return r
}

// world is a place to run gnosh in with the feature: a home, a project
// folder and a model server that answers from a script. Nothing in it is
// the real machine's. Files are served on a port the system picks, so
// that two tests never meet on one.
type world struct {
	*apptest.World
	t *testing.T
	// most is the largest file copied back from a host, when a test sets
	// one.
	most int64

	mu sync.Mutex
	// served is the address of every server the feature started, in
	// order.
	served []string
}

// newWorld makes a world whose model answers with replies, in order.
func newWorld(t *testing.T, replies ...apptest.Reply) *world {
	t.Helper()
	w := &world{World: apptest.NewWorld(t, replies...), t: t}
	w.Settings(`{"options": {"deliverablesServer": "127.0.0.1:0"}}`)
	return w
}

// feature is the feature as the tests run it: every server it starts is
// noted.
func (w *world) feature() app.Feature {
	return New(Options{MaxCopyBytes: w.most, served: func(base string) {
		w.mu.Lock()
		w.served = append(w.served, base)
		w.mu.Unlock()
	}})
}

// servers returns the addresses of the servers the feature started.
func (w *world) servers() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.served...)
}

// options are the Options a test runs the program with.
func (w *world) options(args ...string) app.Options {
	return app.Options{
		Args:      args,
		Stdin:     strings.NewReader(""),
		Environ:   w.Env(),
		Cwd:       w.Dir,
		Features:  []app.Feature{w.feature()},
		Build:     cli.Build{Commit: "0123456789ab", Time: time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)},
		BuildID:   "test-build",
		StdinWait: 50 * time.Millisecond,
		StopGrace: 20 * time.Second,
		ModelWait: 5 * time.Second,
		Retry: agent.RetryPolicy{
			Pauses:          []time.Duration{10 * time.Millisecond},
			GoneAfter:       400 * time.Millisecond,
			NotRunningPause: 10 * time.Millisecond,
			NotRunningAfter: 400 * time.Millisecond,
			BusyMax:         20 * time.Millisecond,
		},
	}
}

// run runs the program with args in the world's folder and waits for it.
func (w *world) run(args ...string) result {
	w.t.Helper()
	return w.runWith(w.options(args...))
}

// runWith runs the program with o and waits for it, failing the test when
// it does not end.
func (w *world) runWith(o app.Options) result {
	w.t.Helper()
	var out, errb safeBuffer
	if o.Stdout == nil {
		o.Stdout = &out
	}
	if o.Stderr == nil {
		o.Stderr = &errb
	}
	done := make(chan int, 1)
	go func() { done <- app.Main(context.Background(), o) }()
	select {
	case code := <-done:
		return result{code: code, out: out.String(), err: errb.String()}
	case <-time.After(120 * time.Second):
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		w.t.Fatalf("the program did not end within two minutes.\nstdout:\n%s\nstderr:\n%s\ngoroutines:\n%s", out.String(), errb.String(), buf[:n])
	}
	return result{}
}

// front is a front end as an interface is attached to a session
// (app.Options.Interactive): it keeps the events it is told, answers the
// questions it is put the way the test says, and what it does with the
// session's controls is the test's to say.
type front struct {
	t   *testing.T
	run func(ctx context.Context, c app.SessionControls, f *front) int
	// answer says what the person answers to a question. Nil means no.
	answer func(q agent.Question) agent.Answer

	mu        sync.Mutex
	c         app.Controls
	events    []any
	questions []agent.Question
}

// Event keeps an event, and answers a question.
func (f *front) Event(ev any) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	c := f.c
	q, asked := ev.(*agent.QuestionEvent)
	if asked {
		f.questions = append(f.questions, q.Question)
	}
	f.mu.Unlock()
	if !asked || c == nil {
		return
	}
	ans := agent.Answer{Choice: "no", Text: "not now"}
	if f.answer != nil {
		ans = f.answer(q.Question)
	}
	// The answer is given from a goroutine of its own: Event must not
	// hold the session up.
	go func() { _ = c.Answer(q.Question.ID, ans) }()
}

// Run hands the test the session's controls.
func (f *front) Run(ctx context.Context, c app.Controls) int {
	more, ok := c.(app.SessionControls)
	if !ok {
		f.t.Errorf("the front end was handed controls of type %T, which are not SessionControls", c)
		return 1
	}
	f.mu.Lock()
	f.c = c
	f.mu.Unlock()
	return f.run(ctx, more, f)
}

// attach runs an interactive session of the world with a scripted front
// end attached, and waits for it to end. answer says what the person
// answers to a question; nil answers no.
func (w *world) attach(answer func(agent.Question) agent.Answer, run func(ctx context.Context, c app.SessionControls, f *front) int, args ...string) (*front, result) {
	w.t.Helper()
	f := &front{t: w.t, run: run, answer: answer}
	o := w.options(args...)
	o.StdinIsTerminal, o.StdoutIsTerminal = true, true
	o.Signals = make(chan os.Signal, 4)
	o.Interactive = func(app.FrontendInfo) app.Frontend { return f }
	return f, w.runWith(o)
}

// say sends a prompt as the person would type it and waits until the
// session has nothing in hand and the front end has been told everything.
func (f *front) say(ctx context.Context, c app.SessionControls, text string) {
	f.t.Helper()
	if _, err := c.Submit(agent.Prompt{Text: text}); err != nil {
		f.t.Errorf("the prompt %q could not be sent: %v", text, err)
		return
	}
	if err := c.Wait(ctx); err != nil {
		f.t.Errorf("waiting for the turn of %q: %v", text, err)
	}
}

// yes is the person who answers yes to every question.
func yes(agent.Question) agent.Answer { return agent.Answer{Choice: "yes"} }

// ends returns the ends of the Deliver calls the front end was told of, in
// order.
func (f *front) ends() []*agent.ToolEndEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*agent.ToolEndEvent
	for _, ev := range f.events {
		if e, ok := ev.(*agent.ToolEndEvent); ok && e.Name == app.DeliverToolName {
			out = append(out, e)
		}
	}
	return out
}

// lists returns the lists of deliverables the front end was handed, in
// order, each as one line.
func (f *front) lists() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, ev := range f.events {
		if e, ok := ev.(*app.DeliverablesEvent); ok {
			out = append(out, listLine(e.List))
		}
	}
	return out
}

// asked returns the questions the person was put.
func (f *front) asked() []agent.Question {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agent.Question(nil), f.questions...)
}

// noticed returns the lines the person was told, in order.
func (f *front) noticed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, ev := range f.events {
		if e, ok := ev.(*agent.NoticeEvent); ok {
			out = append(out, e.Text)
		}
	}
	return out
}

// listLine writes a list of deliverables on one line, each entry as the
// plain front end writes it, with the token of an address left out: it is
// different every time.
func listLine(list []app.Deliverable) string {
	parts := make([]string, len(list))
	for i, d := range list {
		d.URL = noToken(d.URL)
		parts[i] = d.Line(false)
	}
	return strings.Join(parts, " ; ")
}

// noToken writes the address gnosh serves a file at without what changes
// from run to run: the port and the token.
func noToken(address string) string {
	rest, ok := strings.CutPrefix(address, "http://127.0.0.1:")
	if !ok {
		return address
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || len(parts[1]) != 43 {
		return address
	}
	return "http://127.0.0.1:PORT/TOKEN/" + parts[2]
}

// text is what the model reads of a call's result.
func text(e *agent.ToolEndEvent) string { return core.TextOf(e.Result.Content) }

// call is a scripted call of a tool, with its arguments as a map.
func call(id, name string, args map[string]any) apptest.Call {
	raw, _ := json.Marshal(args)
	return apptest.Call{ID: id, Name: name, Args: string(raw)}
}

// deliver is a scripted reply that calls Deliver.
func deliver(id string, args map[string]any) apptest.Reply {
	return apptest.Reply{Calls: []apptest.Call{call(id, app.DeliverToolName, args)}}
}

// says is a scripted reply of words alone.
func says(text string) apptest.Reply { return apptest.Reply{Text: text} }

// toolNamesOf lists the tools a request offers the model, in order.
func toolNamesOf(r apptest.Request) []string {
	var out []string
	tools, _ := r.JSON["tools"].([]any)
	for _, t := range tools {
		tm, _ := t.(map[string]any)
		fn, _ := tm["function"].(map[string]any)
		name, _ := fn["name"].(string)
		out = append(out, name)
	}
	return out
}

// systemOf returns the system prompt of a request.
func systemOf(r apptest.Request) string {
	var parts []string
	for _, m := range r.Messages() {
		if m["role"] == "system" {
			parts = append(parts, apptest.Text(m))
		}
	}
	return strings.Join(parts, "\n")
}

// has fails the test unless text holds every piece.
func has(t *testing.T, what, text string, pieces ...string) {
	t.Helper()
	for _, p := range pieces {
		if !strings.Contains(text, p) {
			t.Errorf("%s does not say %q:\n%s", what, p, text)
		}
	}
}

// hasNot fails the test if text holds any piece.
func hasNot(t *testing.T, what, text string, pieces ...string) {
	t.Helper()
	for _, p := range pieces {
		if strings.Contains(text, p) {
			t.Errorf("%s says %q, and should not:\n%s", what, p, text)
		}
	}
}

// fetch asks an address for what it serves, and returns the answer's
// status, its headers and its body.
func fetch(t *testing.T, method, address string, header ...string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, address, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s %s: reading the answer: %v", method, address, err)
	}
	return resp.StatusCode, resp.Header, string(body)
}

// registerOf reads the register a world's project keeps, as the file
// holds it.
func (w *world) registerOf() registerFile {
	w.t.Helper()
	var file registerFile
	data, err := os.ReadFile(filepath.Join(w.Dir, ".gnosh", folderName, registerName))
	if err != nil {
		w.t.Fatalf("the project has no register: %v", err)
	}
	if err := json.Unmarshal(data, &file); err != nil {
		w.t.Fatalf("the register is not JSON: %v\n%s", err, data)
	}
	return file
}

// seat is a person at a terminal the full interface cannot draw on: what
// they type goes down a pipe that the program is told is a terminal, and
// what the program prints is read as it comes. The session is the plain
// one.
type seat struct {
	t    *testing.T
	in   *io.PipeWriter
	out  *safeBuffer
	errs *safeBuffer
	done chan int
	pos  int
}

// sit starts an interactive session of the world with a person at its
// terminal.
func (w *world) sit(args ...string) *seat {
	w.t.Helper()
	pr, pw := io.Pipe()
	s := &seat{t: w.t, in: pw, out: &safeBuffer{}, errs: &safeBuffer{}, done: make(chan int, 1)}
	o := w.options(args...)
	o.Stdin, o.Stdout, o.Stderr = pr, s.out, s.errs
	o.StdinIsTerminal, o.StdoutIsTerminal = true, true
	o.Signals = make(chan os.Signal, 4)
	go func() { s.done <- app.Main(context.Background(), o) }()
	w.t.Cleanup(func() {
		pw.Close()
		select {
		case <-s.done:
		case <-time.After(60 * time.Second):
			w.t.Errorf("the session did not end when its input was closed.\nscreen:\n%s", s.out.String())
		}
	})
	return s
}

// see waits until the screen shows piece after everything seen so far.
func (s *seat) see(piece string) {
	s.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		screen := s.out.String()
		if i := strings.Index(screen[s.pos:], piece); i >= 0 {
			s.pos += i + len(piece)
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("the screen never showed %q after what was seen before.\nseen before:\n%s\nsince then:\n%s\nstderr:\n%s", piece, screen[:s.pos], screen[s.pos:], s.errs.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// typeLine types a line and Enter.
func (s *seat) typeLine(line string) {
	s.t.Helper()
	if _, err := io.WriteString(s.in, line+"\n"); err != nil {
		s.t.Fatalf("typing %q: %v", line, err)
	}
}

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// checkLeaks fails the test if a goroutine that did not exist when it was
// called is still there after the test's other cleanups have run. Call it
// first, so that its cleanup runs last.
func checkLeaks(t *testing.T) {
	t.Helper()
	before := goroutineIDs()
	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			var leaked []string
			for id, stack := range goroutineStacks() {
				if !before[id] {
					leaked = append(leaked, stack)
				}
			}
			if len(leaked) == 0 {
				return
			}
			if time.Now().After(deadline) {
				t.Errorf("%d goroutine(s) outlived the test:\n\n%s", len(leaked), strings.Join(leaked, "\n\n"))
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
}

func goroutineIDs() map[int]bool {
	ids := make(map[int]bool)
	for id := range goroutineStacks() {
		ids[id] = true
	}
	return ids
}

// goroutineStacks returns the stack of every goroutine but the caller's,
// by goroutine id.
func goroutineStacks() map[int]string {
	buf := make([]byte, 1<<16)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	out := make(map[int]string)
	for i, stanza := range strings.Split(string(buf), "\n\n") {
		if i == 0 {
			continue // The first stanza is the goroutine that asked.
		}
		rest, ok := strings.CutPrefix(stanza, "goroutine ")
		if !ok {
			continue
		}
		num, _, _ := strings.Cut(rest, " ")
		id, err := strconv.Atoi(num)
		if err != nil {
			continue
		}
		out[id] = stanza
	}
	return out
}

// fake is a model service for tests: an httptest server that records what
// it is sent and knows how many connections it has open.
type fake struct {
	t    *testing.T
	srv  *httptest.Server
	stop chan struct{} // closed when the test ends, to free handlers that hang

	mu    sync.Mutex
	conns map[net.Conn]bool
	reqs  []sent
}

// sent is one request the fake received.
type sent struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
	Proto  string // the HTTP version of the request
	Close  bool   // the request asked for the connection to be closed after it
}

// newFake starts a fake service. It checks for leaked goroutines when the
// test ends.
func newFake(t *testing.T, handler func(f *fake, w http.ResponseWriter, r *http.Request)) *fake {
	t.Helper()
	return startFake(t, handler, false)
}

// startFake starts a fake service, over TLS if secure is set. A TLS fake
// offers HTTP/2 as well as HTTP/1.1.
func startFake(t *testing.T, handler func(f *fake, w http.ResponseWriter, r *http.Request), secure bool) *fake {
	t.Helper()
	checkLeaks(t)
	f := &fake{t: t, stop: make(chan struct{}), conns: make(map[net.Conn]bool)}
	f.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, sent{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body, Proto: r.Proto, Close: r.Close})
		f.mu.Unlock()
		handler(f, w, r)
	}))
	f.srv.Config.ConnState = func(c net.Conn, state http.ConnState) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch state {
		case http.StateNew:
			f.conns[c] = true
		case http.StateClosed, http.StateHijacked:
			delete(f.conns, c)
		}
	}
	// What the server has to say about clients that hang up on it is
	// expected here, and only noise in the test log.
	f.srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	if secure {
		f.srv.EnableHTTP2 = true
		f.srv.StartTLS()
	} else {
		f.srv.Start()
	}
	t.Cleanup(func() {
		close(f.stop)
		f.srv.Close()
	})
	return f
}

// requests returns what the fake has received so far.
func (f *fake) requests() []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sent(nil), f.reqs...)
}

// last returns the last request the fake received.
func (f *fake) last() sent {
	f.t.Helper()
	reqs := f.requests()
	if len(reqs) == 0 {
		f.t.Fatal("the server received no request")
	}
	return reqs[len(reqs)-1]
}

// waitClosed fails the test unless every connection to the fake is closed
// soon. It is how a test shows that a stream left no connection behind.
func (f *fake) waitClosed() {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		f.mu.Lock()
		n := len(f.conns)
		f.mu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			f.t.Errorf("%d connection(s) to the server are still open", n)
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// Steps of a stream the fake plays, besides strings (written and flushed)
// and time.Durations (waited out).
type (
	// hang waits until the client goes away.
	hang struct{}
	// cut breaks the connection without ending the response.
	cut struct{}
)

// play answers a request with status 200 and a stream of events.
func (f *fake) play(w http.ResponseWriter, r *http.Request, steps ...any) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher := w.(http.Flusher)
	flusher.Flush()
	for _, step := range steps {
		switch step := step.(type) {
		case string:
			io.WriteString(w, step)
			flusher.Flush()
		case time.Duration:
			timer := time.NewTimer(step)
			select {
			case <-timer.C:
			case <-r.Context().Done():
				timer.Stop()
				return
			case <-f.stop:
				timer.Stop()
				return
			}
		case hang:
			select {
			case <-r.Context().Done():
			case <-f.stop:
			}
			return
		case cut:
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				f.t.Errorf("hijack: %v", err)
				return
			}
			conn.Close()
			return
		default:
			f.t.Errorf("play: unknown step %T", step)
		}
	}
}

// playing makes a handler that plays the same stream for every request.
func playing(steps ...any) func(*fake, http.ResponseWriter, *http.Request) {
	return func(f *fake, w http.ResponseWriter, r *http.Request) { f.play(w, r, steps...) }
}

// answering makes a handler that answers every request with a status, some
// headers and a body.
func answering(status int, header map[string]string, body string) func(*fake, http.ResponseWriter, *http.Request) {
	return func(f *fake, w http.ResponseWriter, r *http.Request) {
		for k, v := range header {
			w.Header().Set(k, v)
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	}
}

// provider makes a provider of the given type that talks to the fake.
// Made-up tool call ids are "made_1", "made_2", and so on.
func (f *fake) provider(typ string, change func(*Config)) core.Provider {
	f.t.Helper()
	n := 0
	cfg := Config{
		Name:    "test",
		Type:    typ,
		BaseURL: f.srv.URL + "/v1",
		NewID: func() string {
			n++
			return fmt.Sprintf("made_%d", n)
		},
	}
	if typ == TypeAnthropic {
		cfg.BaseURL = f.srv.URL
	}
	if change != nil {
		change(&cfg)
	}
	p, err := New(cfg)
	if err != nil {
		f.t.Fatal(err)
	}
	return p
}

// show writes an event as one line, for comparing streams.
func show(e core.Event) string {
	switch e.Type {
	case core.EventTextDelta:
		return fmt.Sprintf("text %q", e.Text)
	case core.EventThinkingDelta:
		return fmt.Sprintf("thinking %q", e.Text)
	case core.EventThinkingSignature:
		s := "signature"
		if e.Signature != "" {
			s += fmt.Sprintf(" %q", e.Signature)
		}
		if e.Opaque != "" {
			s += fmt.Sprintf(" opaque %q", e.Opaque)
		}
		return s
	case core.EventToolUseStart:
		return fmt.Sprintf("tool %s %s", e.ID, e.Name)
	case core.EventToolUseDelta:
		return fmt.Sprintf("args %s %q", e.ID, e.Text)
	case core.EventToolUseStop:
		return "done " + e.ID
	case core.EventUsage:
		u := e.Usage
		return fmt.Sprintf("usage in=%d out=%d read=%d write=%d", u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens)
	case core.EventStop:
		return "stop " + string(e.StopReason)
	}
	return fmt.Sprintf("?%+v", e)
}

// showMessage writes a message's blocks one per line.
func showMessage(m core.Message) []string {
	var out []string
	for _, b := range m.Blocks {
		switch b.Type {
		case core.BlockText:
			out = append(out, fmt.Sprintf("text %q", b.Text))
		case core.BlockThinking:
			s := fmt.Sprintf("thinking %q", b.Text)
			if b.Signature != "" {
				s += fmt.Sprintf(" signature %q", b.Signature)
			}
			if b.Opaque != "" {
				s += fmt.Sprintf(" opaque %q", b.Opaque)
			}
			out = append(out, s)
		case core.BlockToolUse:
			out = append(out, fmt.Sprintf("tool_use %s %s %s", b.ID, b.Name, b.Input))
		default:
			out = append(out, fmt.Sprintf("?%+v", b))
		}
	}
	return out
}

// result is everything a stream gave.
type result struct {
	events  []string   // each event, as show writes it
	err     error      // the error the stream ended with, if any
	message []string   // the message core.Collect makes of the stream
	usage   core.Usage // the usage core.Collect reports
	stop    core.StopReason
	stops   int // how many EventStop events there were
}

// drain reads a stream to its end.
func drain(seq iter.Seq2[core.Event, error]) result {
	var res result
	var events []core.Event
	for ev, err := range seq {
		if err != nil {
			res.err = err
			break
		}
		events = append(events, ev)
		res.events = append(res.events, show(ev))
		if ev.Type == core.EventStop {
			res.stops++
		}
	}
	msg, usage, stop, _ := core.Collect(func(yield func(core.Event, error) bool) {
		for _, ev := range events {
			if !yield(ev, nil) {
				return
			}
		}
		if res.err != nil {
			yield(core.Event{}, res.err)
		}
	})
	res.message = showMessage(msg)
	res.usage = usage
	res.stop = stop
	return res
}

// ask sends a one-line request and reads the whole answer.
func ask(p core.Provider) result {
	return drain(p.Stream(context.Background(), &core.Request{Model: "m", Messages: []core.Message{core.UserText("hi")}}))
}

// wantLines fails the test unless got and want hold the same lines.
func wantLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n  got:\n    %s\n  want:\n    %s", what, strings.Join(got, "\n    "), strings.Join(want, "\n    "))
	}
}

// wantOK fails the test unless the stream ended well, with exactly one
// stop event, as its last.
func wantOK(t *testing.T, res result) {
	t.Helper()
	if res.err != nil {
		t.Fatalf("the stream ended with an error: %v", res.err)
	}
	if res.stops != 1 || len(res.events) == 0 || !strings.HasPrefix(res.events[len(res.events)-1], "stop ") {
		t.Errorf("want exactly one stop event, as the last; got %d in\n    %s", res.stops, strings.Join(res.events, "\n    "))
	}
}

// wantFailure fails the test unless err is a *core.ProviderError of the
// given kind, and returns it.
func wantFailure(t *testing.T, err error, kind core.ErrorKind) *core.ProviderError {
	t.Helper()
	var pe *core.ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("want a *core.ProviderError of kind %s, got %T: %v", kind, err, err)
	}
	if pe.Kind != kind {
		t.Fatalf("kind = %s, want %s (error: %v)", pe.Kind, kind, pe)
	}
	if pe.Provider != "test" {
		t.Errorf("provider = %q, want %q", pe.Provider, "test")
	}
	return pe
}

// wantJSON fails the test unless got and want are the same JSON value.
func wantJSON(t *testing.T, what string, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Fatalf("%s: what was sent is not JSON: %v\n%s", what, err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("%s: the expected JSON in the test is broken: %v\n%s", what, err, want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("%s:\n  got:\n%s\n  want:\n%s", what, indentJSON(g), indentJSON(w))
	}
}

func indentJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("    ", "  ")
	enc.Encode(v)
	return "    " + strings.TrimRight(buf.String(), "\n")
}

// events joins server-sent events the way a server writes them: each one
// followed by a blank line.
func events(evs ...string) string {
	var b strings.Builder
	for _, ev := range evs {
		b.WriteString(ev)
		b.WriteString("\n\n")
	}
	return b.String()
}

package llm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// The time limits in these tests are real ones, kept short. A test that
// waits for a limit to run out makes the server hang until the client is
// gone, so it cannot fail by being slow; a test that must not hit a limit
// keeps well under it.

// wantTimeout fails the test unless err is the named time limit running
// out.
func wantTimeout(t *testing.T, err error, name string) {
	t.Helper()
	e := wantFailure(t, err, core.ErrTimeout)
	if e.Timeout != name {
		t.Fatalf("the %q limit ran out, want %q (%v)", e.Timeout, name, e)
	}
	if !e.Retryable() || e.Message == "" {
		t.Errorf("retryable %v, message %q", e.Retryable(), e.Message)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a time limit of the provider's looks like the caller's context ending: %v", err)
	}
}

// pause waits d in steps short enough for the house rule on sleeps.
func pause(d time.Duration) []any {
	var steps []any
	for ; d > 40*time.Millisecond; d -= 40 * time.Millisecond {
		steps = append(steps, 40*time.Millisecond)
	}
	return append(steps, d)
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestConnectLimit(t *testing.T) {
	t.Run("the server cannot be reached", func(t *testing.T) {
		checkLeaks(t)
		var dials atomic.Int32
		client := newHTTPClientDialing(func(ctx context.Context, network, addr string) (net.Conn, error) {
			dials.Add(1)
			<-ctx.Done() // a host that swallows packets
			return nil, ctx.Err()
		})
		p, err := New(Config{Name: "test", Type: TypeOpenAI, BaseURL: "http://203.0.113.9/v1", HTTPClient: client,
			Timeouts: Timeouts{Connect: 30 * time.Millisecond, FirstToken: 30 * time.Second, Idle: 30 * time.Second, Total: 60 * time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		start := time.Now()
		res := ask(p)
		wantTimeout(t, res.err, "connect")
		if time.Since(start) > 10*time.Second || dials.Load() != 1 {
			t.Errorf("took %s, %d dials", time.Since(start), dials.Load())
		}
	})

	t.Run("the server accepts and never finishes TLS", func(t *testing.T) {
		checkLeaks(t)
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		// The server reads what the client says and never answers. It
		// reports when the client hangs up.
		var wg sync.WaitGroup
		hungUp := make(chan struct{}, 8)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					io.Copy(io.Discard, c)
					c.Close()
					hungUp <- struct{}{}
				}()
			}
		}()
		t.Cleanup(func() {
			ln.Close()
			wg.Wait()
		})
		p, err := New(Config{Name: "test", Type: TypeAnthropic, BaseURL: "https://" + ln.Addr().String(),
			Timeouts: Timeouts{Connect: 40 * time.Millisecond, FirstToken: 30 * time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		wantTimeout(t, ask(p).err, "connect")
		// The client must give the connection up by itself, though the
		// handshake it was in the middle of would wait for ever.
		select {
		case <-hungUp:
		case <-time.After(5 * time.Second):
			t.Error("the connection is still open after the limit ran out")
		}
	})

	t.Run("slow headers are not a slow connection", func(t *testing.T) {
		// Servers that hold their headers back until the first token
		// must not trip the connect limit: the connection is long made.
		f := newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
			for _, step := range pause(300 * time.Millisecond) {
				time.Sleep(step.(time.Duration))
			}
			f.play(w, r, okStream)
		})
		res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Connect: 150 * time.Millisecond} }))
		wantOK(t, res)
		f.waitClosed()
	})

	t.Run("a client that does not report its connections is held to the headers", func(t *testing.T) {
		checkLeaks(t)
		client := &http.Client{Transport: roundTripper(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()
			return nil, r.Context().Err()
		})}
		p, err := New(Config{Name: "test", Type: TypeOpenAI, BaseURL: "http://203.0.113.9/v1", HTTPClient: client,
			Timeouts: Timeouts{Connect: 30 * time.Millisecond, FirstToken: 30 * time.Second}})
		if err != nil {
			t.Fatal(err)
		}
		wantTimeout(t, ask(p).err, "connect")
	})
}

func TestFirstTokenLimit(t *testing.T) {
	// The connect limit is the shorter one, and still it is the first-token
	// limit that runs out: the connection is made at once.
	limits := Timeouts{Connect: 150 * time.Millisecond, FirstToken: 250 * time.Millisecond, Idle: 20 * time.Millisecond}

	t.Run("the server sends no headers", func(t *testing.T) {
		f := newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-f.stop:
			}
		})
		res := ask(f.provider(TypeLlamaCpp, func(c *Config) { c.Timeouts = limits }))
		wantTimeout(t, res.err, "first_token")
		f.waitClosed()
	})

	t.Run("headers and events that carry nothing of the answer do not count", func(t *testing.T) {
		f := newFake(t, playing(": ping\n\n", events(delta(roleChunk)), 30*time.Millisecond, ": ping\n\n", 30*time.Millisecond, events(delta(`{"content":""}`)), hang{}))
		res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = limits }))
		wantTimeout(t, res.err, "first_token")
		if len(res.events) != 0 {
			t.Errorf("events: %q", res.events)
		}
		f.waitClosed()
	})

	t.Run("anthropic: the message's start and pings do not count", func(t *testing.T) {
		f := newFake(t, playing(events(messageStart(`{"input_tokens":5,"output_tokens":1}`), ping), hang{}))
		res := ask(f.provider(TypeAnthropic, func(c *Config) { c.Timeouts = limits }))
		wantTimeout(t, res.err, "first_token")
		wantLines(t, "events", res.events, []string{`usage in=5 out=1 read=0 write=0`})
		f.waitClosed()
	})

	t.Run("anthropic: a block that opens does count", func(t *testing.T) {
		// Thinking that is not shown opens a block and then says nothing
		// for as long as the model thinks.
		steps := []any{events(messageStart(`{"input_tokens":5,"output_tokens":1}`), blockStart(0, `{"type":"thinking","thinking":"","signature":""}`))}
		steps = append(steps, pause(350*time.Millisecond)...)
		steps = append(steps, events(
			blockDelta(0, `{"type":"signature_delta","signature":"sig"}`), blockStop(0),
			blockStart(1, `{"type":"text","text":""}`), blockDelta(1, `{"type":"text_delta","text":"ok"}`), blockStop(1),
			messageDelta("end_turn", ""), messageStop))
		f := newFake(t, playing(steps...))
		res := ask(f.provider(TypeAnthropic, func(c *Config) { c.Timeouts = Timeouts{FirstToken: 200 * time.Millisecond} }))
		wantOK(t, res)
		f.waitClosed()
	})

	t.Run("the first piece of the answer ends the wait", func(t *testing.T) {
		steps := []any{events(delta(`{"content":"a"}`))}
		steps = append(steps, pause(350*time.Millisecond)...)
		steps = append(steps, events(delta(`{"content":"b"}`), finish("stop"), doneLine))
		f := newFake(t, playing(steps...))
		res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{FirstToken: 200 * time.Millisecond} }))
		wantOK(t, res)
		wantLines(t, "message", res.message, []string{`text "ab"`})
		f.waitClosed()
	})
}

func TestIdleLimit(t *testing.T) {
	t.Run("the server goes silent in the middle of the answer", func(t *testing.T) {
		f := newFake(t, playing(events(delta(`{"content":"half an ans"}`)), hang{}))
		res := ask(f.provider(TypeOpenAI, func(c *Config) {
			c.Timeouts = Timeouts{Connect: 10 * time.Second, FirstToken: 10 * time.Second, Idle: 40 * time.Millisecond, Total: 30 * time.Second}
		}))
		wantTimeout(t, res.err, "idle")
		wantLines(t, "events", res.events, []string{`text "half an ans"`})
		wantLines(t, "message", res.message, []string{`text "half an ans"`})
		f.waitClosed()
	})

	t.Run("silence before the answer begins is not idleness", func(t *testing.T) {
		steps := []any{events(delta(roleChunk))}
		steps = append(steps, pause(150*time.Millisecond)...)
		steps = append(steps, events(delta(`{"content":"ok"}`), finish("stop"), doneLine))
		f := newFake(t, playing(steps...))
		res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Idle: 30 * time.Millisecond} }))
		wantOK(t, res)
		f.waitClosed()
	})

	t.Run("every piece that arrives starts the wait again", func(t *testing.T) {
		var steps []any
		for range 12 {
			steps = append(steps, events(delta(`{"content":"x"}`)), 30*time.Millisecond)
		}
		steps = append(steps, events(finish("stop"), doneLine))
		f := newFake(t, playing(steps...))
		res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Idle: 300 * time.Millisecond} }))
		wantOK(t, res)
		wantLines(t, "message", res.message, []string{`text "xxxxxxxxxxxx"`})
		f.waitClosed()
	})

	t.Run("a server's keep-alive lines count as signs of life", func(t *testing.T) {
		steps := []any{events(delta(`{"content":"x"}`))}
		for range 10 {
			steps = append(steps, 30*time.Millisecond, ": still here\n\n")
		}
		steps = append(steps, events(finish("stop"), doneLine))
		f := newFake(t, playing(steps...))
		res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Idle: 250 * time.Millisecond} }))
		wantOK(t, res)
		f.waitClosed()
	})

	t.Run("the consumer's own time is not held against the server", func(t *testing.T) {
		f := newFake(t, playing(events(delta(`{"content":"a"}`), delta(`{"content":"b"}`), delta(`{"content":"c"}`), finish("stop"), doneLine)))
		p := f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Idle: 30 * time.Millisecond} })
		var got []string
		for ev, err := range p.Stream(context.Background(), &core.Request{Model: "m", Messages: []core.Message{core.UserText("hi")}}) {
			if err != nil {
				t.Fatalf("after %q: %v", got, err)
			}
			got = append(got, show(ev))
			time.Sleep(45 * time.Millisecond) // a slow terminal, a busy interface
		}
		wantLines(t, "events", got, []string{`text "a"`, `text "b"`, `text "c"`, `stop end_turn`})
		f.waitClosed()
	})

	t.Run("silence after the model said why it stopped loses nothing", func(t *testing.T) {
		// The answer is whole; only the usage report and the last line
		// are missing.
		f := newFake(t, playing(events(delta(`{"content":"all of it"}`), finish("stop")), hang{}))
		res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Idle: 40 * time.Millisecond} }))
		wantOK(t, res)
		wantLines(t, "events", res.events, []string{`text "all of it"`, `stop end_turn`})
		f.waitClosed()

		f = newFake(t, playing(events(blockStart(0, `{"type":"text","text":"all of it"}`), messageDelta("max_tokens", "")), hang{}))
		res = ask(f.provider(TypeAnthropic, func(c *Config) { c.Timeouts = Timeouts{Idle: 40 * time.Millisecond} }))
		wantOK(t, res)
		wantLines(t, "events", res.events, []string{`text "all of it"`, `stop max_tokens`})
		f.waitClosed()

		// The same when the connection breaks at that point.
		f = newFake(t, playing(events(delta(`{"content":"all of it"}`), finish("length")), cut{}))
		res = ask(f.provider(TypeOpenAI, nil))
		wantOK(t, res)
		wantLines(t, "events", res.events, []string{`text "all of it"`, `stop max_tokens`})
	})
}

func TestTotalLimit(t *testing.T) {
	f := newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		// An answer that never ends, with no pause long enough to be
		// idleness.
		steps := []any{}
		for range 2000 {
			steps = append(steps, events(delta(`{"content":"x"}`)), 10*time.Millisecond)
		}
		f.play(w, r, steps...)
	})
	res := ask(f.provider(TypeOpenAI, func(c *Config) {
		c.Timeouts = Timeouts{Connect: 10 * time.Second, FirstToken: 10 * time.Second, Idle: 10 * time.Second, Total: 300 * time.Millisecond}
	}))
	wantTimeout(t, res.err, "total")
	if len(res.events) == 0 || len(res.events) > 200 {
		t.Errorf("%d events before the limit", len(res.events))
	}
	f.waitClosed()

	// The limit covers the wait for the headers as well.
	f = newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-f.stop:
		}
	})
	res = ask(f.provider(TypeAnthropic, func(c *Config) { c.Timeouts = Timeouts{Total: 40 * time.Millisecond} }))
	wantTimeout(t, res.err, "total")
	f.waitClosed()
}

func TestLimitsOnWholeResponses(t *testing.T) {
	// A model list is not a stream, but the limits watch it too: the
	// first-token limit until the headers, the idle limit over the body.
	f := newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-f.stop:
		}
	})
	p := f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{FirstToken: 40 * time.Millisecond} })
	_, err := p.(core.ModelLister).Models(context.Background())
	wantTimeout(t, err, "first_token")
	f.waitClosed()

	f = newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		io.WriteString(w, `{"data":[`)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-f.stop:
		}
	})
	p = f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{FirstToken: 10 * time.Second, Idle: 40 * time.Millisecond} })
	_, err = p.(core.ModelLister).Models(context.Background())
	wantTimeout(t, err, "idle")
	f.waitClosed()

	// The body of an error is read under the same watch.
	f = newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		io.WriteString(w, `{"error":{"mess`)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-f.stop:
		}
	})
	res := ask(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Idle: 40 * time.Millisecond} }))
	wantTimeout(t, res.err, "idle")
	f.waitClosed()
}

func TestCancel(t *testing.T) {
	request := &core.Request{Model: "m", Messages: []core.Message{core.UserText("hi")}}
	hangs := func(f *fake, w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-f.stop:
		}
	}
	// wantCtxErr checks that a stream ended with the context's own error
	// and nothing dressed up as a provider's failure.
	wantCtxErr := func(t *testing.T, err, want error) {
		t.Helper()
		if !errors.Is(err, want) {
			t.Fatalf("err = %v, want %v", err, want)
		}
		var pe *core.ProviderError
		if errors.As(err, &pe) {
			t.Fatalf("the caller's cancellation came back as a provider error: %v", pe)
		}
	}

	t.Run("before the request is sent", func(t *testing.T) {
		f := newFake(t, hangs)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := drain(f.provider(TypeOpenAI, nil).Stream(ctx, request))
		wantCtxErr(t, res.err, context.Canceled)
		if n := len(f.requests()); n != 0 {
			t.Errorf("%d requests were sent", n)
		}
	})

	t.Run("while connecting", func(t *testing.T) {
		checkLeaks(t)
		client := newHTTPClientDialing(func(ctx context.Context, network, addr string) (net.Conn, error) {
			<-ctx.Done() // The dial ends only because the request did.
			return nil, ctx.Err()
		})
		p, err := New(Config{Name: "test", Type: TypeOpenAI, BaseURL: "http://203.0.113.9/v1", HTTPClient: client})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(30*time.Millisecond, cancel)
		res := drain(p.Stream(ctx, request))
		timer.Stop()
		cancel()
		wantCtxErr(t, res.err, context.Canceled)
	})

	t.Run("while waiting for the headers", func(t *testing.T) {
		for _, typ := range []string{TypeOpenAI, TypeAnthropic} {
			f := newFake(t, hangs)
			ctx, cancel := context.WithCancel(context.Background())
			timer := time.AfterFunc(30*time.Millisecond, cancel)
			res := drain(f.provider(typ, func(c *Config) { c.Timeouts = Timeouts{FirstToken: 30 * time.Second} }).Stream(ctx, request))
			timer.Stop()
			cancel()
			wantCtxErr(t, res.err, context.Canceled)
			f.waitClosed()
		}
	})

	t.Run("in the middle of the answer", func(t *testing.T) {
		f := newFake(t, playing(events(delta(`{"content":"a"}`), delta(`{"content":"b"}`)), hang{}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var got []string
		var last error
		for ev, err := range f.provider(TypeOpenAI, nil).Stream(ctx, request) {
			if err != nil {
				last = err
				continue // The sequence must end by itself after its error.
			}
			got = append(got, show(ev))
			if len(got) == 2 {
				cancel()
			}
		}
		wantCtxErr(t, last, context.Canceled)
		wantLines(t, "events", got, []string{`text "a"`, `text "b"`})
		f.waitClosed()
	})

	t.Run("with a cause", func(t *testing.T) {
		f := newFake(t, playing(events(delta(`{"content":"a"}`)), hang{}))
		ctx, cancel := context.WithCancelCause(context.Background())
		timer := time.AfterFunc(30*time.Millisecond, func() { cancel(errors.New("the user pressed escape")) })
		res := drain(f.provider(TypeAnthropic, nil).Stream(ctx, request))
		timer.Stop()
		wantCtxErr(t, res.err, context.Canceled)
		f.waitClosed()
	})

	t.Run("the caller's deadline", func(t *testing.T) {
		f := newFake(t, playing(events(delta(`{"content":"a"}`)), hang{}))
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		res := drain(f.provider(TypeOpenAI, func(c *Config) { c.Timeouts = Timeouts{Idle: 30 * time.Second} }).Stream(ctx, request))
		wantCtxErr(t, res.err, context.DeadlineExceeded)
		wantLines(t, "events", res.events, []string{`text "a"`})
		f.waitClosed()
	})

	t.Run("a model list", func(t *testing.T) {
		f := newFake(t, hangs)
		ctx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(30*time.Millisecond, cancel)
		_, err := f.provider(TypeAnthropic, nil).(core.ModelLister).Models(ctx)
		timer.Stop()
		cancel()
		wantCtxErr(t, err, context.Canceled)
		f.waitClosed()
	})
}

func TestStopReadingEarly(t *testing.T) {
	request := &core.Request{Model: "m", Messages: []core.Message{core.UserText("hi")}}
	for _, typ := range []string{TypeOpenAI, TypeLlamaCpp, TypeAnthropic} {
		t.Run(typ, func(t *testing.T) {
			stream := events(delta(`{"content":"a"}`), delta(`{"content":"b"}`), delta(`{"content":"c"}`))
			if typ == TypeAnthropic {
				stream = events(blockStart(0, `{"type":"text","text":"a"}`), blockDelta(0, `{"type":"text_delta","text":"b"}`), blockDelta(0, `{"type":"text_delta","text":"c"}`))
			}
			// The server would go on for ever; the consumer leaves after
			// the first event.
			f := newFake(t, playing(stream, hang{}))
			p := f.provider(typ, func(c *Config) {
				c.Timeouts = Timeouts{Connect: 10 * time.Second, FirstToken: 10 * time.Second, Idle: 10 * time.Second, Total: 10 * time.Second}
			})
			n := 0
			for _, err := range p.Stream(context.Background(), request) {
				if err != nil {
					t.Fatal(err)
				}
				n++
				break
			}
			if n != 1 {
				t.Fatalf("read %d events", n)
			}
			// Nothing is left behind: the connection is gone at once,
			// and newFake's cleanup finds no goroutine.
			f.waitClosed()
		})
	}

	t.Run("after an error nothing more is yielded", func(t *testing.T) {
		f := newFake(t, answering(500, nil, `{"error":{"message":"boom"}}`))
		yields := 0
		for range f.provider(TypeOpenAI, nil).Stream(context.Background(), request) {
			yields++
		}
		if yields != 1 {
			t.Errorf("%d pairs were yielded, want the error alone", yields)
		}
		f.waitClosed()
	})

	t.Run("each range sends the request again", func(t *testing.T) {
		f := newFake(t, playing(okStream))
		seq := f.provider(TypeOpenAI, nil).Stream(context.Background(), request)
		wantOK(t, drain(seq))
		wantOK(t, drain(seq))
		if n := len(f.requests()); n != 2 {
			t.Errorf("%d requests, want 2", n)
		}
		f.waitClosed()
	})

	t.Run("no request", func(t *testing.T) {
		f := newFake(t, playing(okStream))
		for _, typ := range []string{TypeOpenAI, TypeAnthropic} {
			res := drain(f.provider(typ, nil).Stream(context.Background(), nil))
			wantFailure(t, res.err, core.ErrInvalidRequest)
		}
		if n := len(f.requests()); n != 0 {
			t.Errorf("%d requests were sent", n)
		}
	})
}

func TestTheProvidersOwnClient(t *testing.T) {
	tr := newHTTPClient().Transport.(*http.Transport)
	if tr.Proxy == nil || !tr.DisableKeepAlives || !tr.DisableCompression || tr.ResponseHeaderTimeout != 0 {
		t.Errorf("the transport: proxy from the environment %v, one connection per request %v, no compression %v, header timeout %s",
			tr.Proxy != nil, tr.DisableKeepAlives, tr.DisableCompression, tr.ResponseHeaderTimeout)
	}

	f := newFake(t, playing(okStream))
	wantOK(t, ask(f.provider(TypeOpenAI, nil)))
	if got := f.last(); got.Proto != "HTTP/1.1" || !got.Close || got.Header.Get("Accept-Encoding") != "" {
		t.Errorf("proto %s, Connection: close %v, Accept-Encoding %q", got.Proto, got.Close, got.Header.Get("Accept-Encoding"))
	}
	f.waitClosed()

	// A very large event goes through: a tool call whose arguments hold
	// a whole file, sent in one piece.
	big := strings.Repeat("0123456789", 300_000)
	f = newFake(t, playing(events(
		delta(`{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"Write","arguments":"{\"content\":\"`+big+`\"}"}}]}`),
		finish("tool_calls"), doneLine)))
	msg, _, stop, err := core.Collect(f.provider(TypeOpenAI, nil).Stream(context.Background(), &core.Request{Model: "m", Messages: []core.Message{core.UserText("hi")}}))
	if err != nil || stop != core.StopToolUse || len(msg.Blocks) != 1 || len(msg.Blocks[0].Input) != len(big)+len(`{"content":""}`) {
		t.Errorf("err %v, stop %s, %d blocks", err, stop, len(msg.Blocks))
	}
	f.waitClosed()
}

func TestTLS(t *testing.T) {
	// The provider's own client against an HTTPS server that would rather
	// speak HTTP/2: the stream arrives, over HTTP/1.1, and the connection
	// is closed afterwards like any other.
	f := startFake(t, playing(events(delta(`{"content":"over TLS"}`), finish("stop"), doneLine)), true)
	roots := x509.NewCertPool()
	roots.AddCert(f.srv.Certificate())
	client := newHTTPClient()
	client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: roots}

	p := f.provider(TypeOpenAI, func(c *Config) {
		c.HTTPClient = client
		c.Timeouts = Timeouts{Connect: 10 * time.Second, FirstToken: 10 * time.Second, Idle: 10 * time.Second, Total: 10 * time.Second}
	})
	if !strings.HasPrefix(f.srv.URL, "https://") {
		t.Fatalf("the fake is not an HTTPS server: %s", f.srv.URL)
	}
	res := ask(p)
	wantOK(t, res)
	wantLines(t, "message", res.message, []string{`text "over TLS"`})
	if got := f.last(); got.Proto != "HTTP/1.1" || !got.Close {
		t.Errorf("proto %s, Connection: close %v", got.Proto, got.Close)
	}
	f.waitClosed()

	// The consumer leaves early: the TLS connection goes too.
	f = startFake(t, playing(events(delta(`{"content":"a"}`)), hang{}), true)
	roots = x509.NewCertPool()
	roots.AddCert(f.srv.Certificate())
	client = newHTTPClient()
	client.Transport.(*http.Transport).TLSClientConfig = &tls.Config{RootCAs: roots}
	p = f.provider(TypeOpenAI, func(c *Config) { c.HTTPClient = client })
	for _, err := range p.Stream(context.Background(), &core.Request{Model: "m", Messages: []core.Message{core.UserText("hi")}}) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	f.waitClosed()

	// A certificate nobody vouches for is a network failure, and says why.
	f = startFake(t, playing(okStream), true)
	res = ask(f.provider(TypeOpenAI, nil))
	e := wantFailure(t, res.err, core.ErrNetwork)
	if !strings.Contains(e.Message, "certificate") {
		t.Errorf("message = %q", e.Message)
	}
	f.waitClosed()
}

func TestNetworkFailure(t *testing.T) {
	checkLeaks(t)
	// A server that refuses the connection. The refusal is made up here
	// rather than got from a closed port, which another test on the same
	// machine could open again in the meantime.
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	client := newHTTPClientDialing(func(ctx context.Context, network, addr string) (net.Conn, error) {
		return nil, refused
	})
	p, err := New(Config{Name: "test", Type: TypeOpenAI, BaseURL: "http://user:PASSWORD@203.0.113.9:8080/v1?sig=SECRETSIG", APIKey: "sk-SECRETKEY", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	res := ask(p)
	e := wantFailure(t, res.err, core.ErrNetwork)
	if !e.Retryable() || !errors.Is(e, syscall.ECONNREFUSED) || e.Status != 0 || !strings.Contains(e.Message, "refused") {
		t.Errorf("retryable %v, err %v, status %d, message %q", e.Retryable(), e.Err, e.Status, e.Message)
	}
	for _, secret := range []string{"SECRETSIG", "SECRETKEY", "PASSWORD"} {
		if strings.Contains(fmt.Sprintf("%v %+v %v", e, *e, e.Err), secret) {
			t.Errorf("the error gives away %s: %v", secret, e)
		}
	}
	_, err = p.(core.ModelLister).Models(context.Background())
	wantFailure(t, err, core.ErrNetwork)
}

func TestTrace(t *testing.T) {
	var got []TraceRecord
	f := newFake(t, func(f *fake, w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("X-Request-Id", "req_models")
			w.WriteHeader(403)
			io.WriteString(w, `{"error":{"message":"no"}}`)
			return
		}
		w.Header().Set("X-Request-Id", "req_1")
		f.play(w, r, ": ping\n\n", events(delta(`{"content":"hi"}`), "garbage line\nevent: odd\ndata: {}", finish("stop"), doneLine))
	})
	p := f.provider(TypeOpenAI, func(c *Config) {
		c.BaseURL = strings.Replace(f.srv.URL, "http://", "http://user:PASSWORD@", 1) + "/v1?sig=SECRETSIG"
		c.APIKey = "sk-SECRETKEY"
		c.Headers = map[string]string{"X-Proxy-Token": "SECRETTOKEN"}
		c.Trace = func(r TraceRecord) { got = append(got, r) }
	})
	wantOK(t, ask(p))
	if q := f.last().Query; q != "sig=SECRETSIG" {
		t.Errorf("the base URL's query was not sent: %q", q)
	}

	var lines []string
	for _, r := range got {
		switch r.Kind {
		case TraceRequest:
			lines = append(lines, fmt.Sprintf("request %s %s %s", r.Method, strings.Replace(r.URL, f.srv.URL, "", 1), r.Body))
		case TraceResponse:
			lines = append(lines, fmt.Sprintf("response %d %s %s", r.Status, r.Header.Get("X-Request-Id"), r.Body))
		case TraceEvent:
			lines = append(lines, fmt.Sprintf("event %q %s", r.Event, r.Data))
		case TraceIgnored:
			lines = append(lines, fmt.Sprintf("ignored %s", r.Data))
		default:
			lines = append(lines, fmt.Sprintf("? %+v", r))
		}
	}
	wantLines(t, "trace", lines, []string{
		`request POST /v1/chat/completions {"messages":[{"content":"hi","role":"user"}],"model":"m","stream":true,"stream_options":{"include_usage":true}}`,
		`response 200 req_1 `,
		`ignored : ping`,
		`event "" {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		`ignored garbage line`,
		`event "odd" {}`,
		`event "" {"id":"chatcmpl-1","object":"chat.completion.chunk","created":1,"model":"m","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`event "" [DONE]`,
	})

	// A call that is not a stream is traced too, with the body it got.
	all := got
	got = nil
	_, err := p.(core.ModelLister).Models(context.Background())
	wantFailure(t, err, core.ErrAuth)
	if len(got) != 2 || got[0].Kind != TraceRequest || got[0].Method != "GET" || got[0].Body != "" ||
		got[1].Kind != TraceResponse || got[1].Status != 403 || got[1].Body != `{"error":{"message":"no"}}` || got[1].Header.Get("X-Request-Id") != "req_models" {
		t.Errorf("trace of a model list: %+v", got)
	}

	// No trace record holds a secret.
	for _, r := range append(all, got...) {
		for _, secret := range []string{"SECRETSIG", "SECRETKEY", "PASSWORD", "SECRETTOKEN"} {
			if strings.Contains(fmt.Sprintf("%+v", r), secret) {
				t.Errorf("a trace record gives away %s: %+v", secret, r)
			}
		}
	}
}

func TestNew(t *testing.T) {
	bad := []struct {
		name string
		cfg  Config
		want string
	}{
		{"no type", Config{BaseURL: "http://example.com"}, "no type"},
		{"unknown type", Config{Type: "gemini", BaseURL: "http://example.com"}, `unknown type "gemini"`},
		{"no base URL", Config{Type: TypeOpenAI}, "no base URL"},
		{"blank base URL", Config{Type: TypeOpenAI, BaseURL: "  "}, "no base URL"},
		{"base URL that cannot be read", Config{Type: TypeOpenAI, BaseURL: "http://exa mple.com"}, "cannot be read"},
		{"base URL with another scheme", Config{Type: TypeOpenAI, BaseURL: "ftp://example.com"}, "http:// or https://"},
		{"base URL with no scheme", Config{Type: TypeOpenAI, BaseURL: "example.com/v1"}, "http:// or https://"},
		{"base URL with no host", Config{Type: TypeOpenAI, BaseURL: "http:///v1"}, "no host"},
		{"negative limit", Config{Type: TypeOpenAI, BaseURL: "http://example.com", Timeouts: Timeouts{Idle: -1}}, "idle time limit is negative"},
		{"bad history thinking", Config{Type: TypeOpenAI, BaseURL: "http://example.com", HistoryThinking: "yes"}, "history thinking"},
		{"bad think tags", Config{Type: TypeOpenAI, BaseURL: "http://example.com", ThinkTags: "true"}, "think tags"},
		{"bad prompt cache", Config{Type: TypeAnthropic, BaseURL: "http://example.com", PromptCache: "1"}, "prompt cache"},
		{"bad cache TTL", Config{Type: TypeAnthropic, BaseURL: "http://example.com", CacheTTL: "2h"}, "cache TTL"},
		{"bad max tokens field", Config{Type: TypeOpenAI, BaseURL: "http://example.com", MaxTokensField: "n_predict"}, "max tokens field"},
		{"negative default max tokens", Config{Type: TypeAnthropic, BaseURL: "http://example.com", DefaultMaxTokens: -1}, "negative"},
	}
	for _, tt := range bad {
		tt.cfg.Name = "mine"
		p, err := New(tt.cfg)
		if err == nil || p != nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), `"mine"`) {
			t.Errorf("%s: provider %v, err %v; want an error holding %q and the provider's name", tt.name, p, err, tt.want)
		}
	}

	for _, typ := range []string{TypeOpenAI, TypeLlamaCpp, TypeAnthropic} {
		p, err := New(Config{Type: typ, BaseURL: "https://example.com/v1/", Extra: map[string]any{"a": 1}, Headers: map[string]string{"X": "y"}})
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		if p.Name() != typ {
			t.Errorf("%s: with no name given, Name() = %q", typ, p.Name())
		}
		_, lists := p.(core.ModelLister)
		_, counts := p.(core.TokenCounter)
		if !lists || counts != (typ != TypeOpenAI) {
			t.Errorf("%s: lists models %v, counts tokens %v", typ, lists, counts)
		}
	}
	p, err := New(Config{Name: "local", Type: TypeLlamaCpp, BaseURL: "http://127.0.0.1:8080/v1"})
	if err != nil || p.Name() != "local" {
		t.Errorf("Name() = %q, err %v", p.Name(), err)
	}

	// The provider keeps its own copy of the configuration's maps.
	extra := map[string]any{"o": map[string]any{"a": 1}}
	headers := map[string]string{"X-A": "1"}
	pp, err := newProvider(Config{Type: TypeOpenAI, BaseURL: "http://example.com", Extra: extra, Headers: headers})
	if err != nil {
		t.Fatal(err)
	}
	extra["o"].(map[string]any)["a"] = 2
	extra["new"] = true
	headers["X-A"] = "2"
	if pp.extra["o"].(obj)["a"] != 1 || pp.extra["new"] != nil || pp.headers["X-A"] != "1" {
		t.Errorf("the provider shares the caller's maps: %v %v", pp.extra, pp.headers)
	}

	// Endpoints.
	for _, tt := range []struct{ typ, base, want string }{
		{TypeOpenAI, "https://api.example.com/v1", "https://api.example.com/v1/chat/completions"},
		{TypeOpenAI, "https://api.example.com/v1/", "https://api.example.com/v1/chat/completions"},
		{TypeOpenAI, "https://x.example.com/openai/deployments/d?api-version=2024-10-21", "https://x.example.com/openai/deployments/d/chat/completions?api-version=2024-10-21"},
		{TypeOpenAI, "http://127.0.0.1:11434", "http://127.0.0.1:11434/v1/chat/completions"},
		{TypeOpenAI, "http://127.0.0.1:11434/", "http://127.0.0.1:11434/v1/chat/completions"},
		{TypeOpenAI, "https://api.example.com/v1beta/openai", "https://api.example.com/v1beta/openai/chat/completions"},
		{TypeLlamaCpp, "http://127.0.0.1:8080/v1", "http://127.0.0.1:8080/v1/chat/completions"},
		{TypeLlamaCpp, "http://127.0.0.1:8080", "http://127.0.0.1:8080/v1/chat/completions"},
		{TypeLlamaCpp, "http://127.0.0.1:8080/", "http://127.0.0.1:8080/v1/chat/completions"},
		{TypeLlamaCpp, "http://127.0.0.1:8080/prefix", "http://127.0.0.1:8080/prefix/v1/chat/completions"},
		{TypeLlamaCpp, "http://127.0.0.1:8080/prefix/v1/", "http://127.0.0.1:8080/prefix/v1/chat/completions"},
		{TypeAnthropic, "https://api.example.com", "https://api.example.com/v1/messages"},
		{TypeAnthropic, "https://api.example.com/", "https://api.example.com/v1/messages"},
		{TypeAnthropic, "https://api.example.com/v1", "https://api.example.com/v1/messages"},
		{TypeAnthropic, "https://proxy.example.com/anthropic", "https://proxy.example.com/anthropic/v1/messages"},
	} {
		p, err := New(Config{Type: tt.typ, BaseURL: tt.base})
		if err != nil {
			t.Fatal(err)
		}
		if got := p.(wire).streamURL(); got != tt.want {
			t.Errorf("%s at %s: stream URL %s, want %s", tt.typ, tt.base, got, tt.want)
		}
	}
	pp, _ = newProvider(Config{Type: TypeLlamaCpp, BaseURL: "http://127.0.0.1:8080/prefix/v1?k=v"})
	if got := pp.rootEndpoint("/props", map[string][]string{"model": {"a/b:c"}}); got != "http://127.0.0.1:8080/prefix/props?k=v&model=a%2Fb%3Ac" {
		t.Errorf("root endpoint = %s", got)
	}

	// A Config.NewID that repeats itself still gives each call its own id.
	stuck, err := newProvider(Config{Type: TypeOpenAI, BaseURL: "http://example.com", NewID: func() string { return "same" }})
	if err != nil {
		t.Fatal(err)
	}
	used := map[string]bool{"taken": true}
	var ids []string
	for _, wire := range []string{"", "", "taken", "fresh", "fresh", ""} {
		ids = append(ids, stuck.callID(used, wire))
	}
	wantLines(t, "ids", ids, []string{"same", "same_1", "same_2", "fresh", "same_3", "same_4"})

	// A made-up id is one of a kind.
	seen := make(map[string]bool)
	for range 1000 {
		id := randomID()
		if seen[id] || !strings.HasPrefix(id, "call_") || len(id) != len("call_")+24 {
			t.Fatalf("id %q", id)
		}
		seen[id] = true
	}
}

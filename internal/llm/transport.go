package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// userAgent is all a request says about who sent it.
const userAgent = "gnosh"

const (
	// maxWholeBody is the most that is read of a response that is taken
	// whole: a model list, a token count, a rendered prompt.
	maxWholeBody = 64 << 20
	// maxErrorBody is the most that is read of the body of a response
	// whose status is an error. What an API has to say about a failure is
	// short; more than this is a web page nobody needs all of.
	maxErrorBody = 1 << 20
)

// newHTTPClient makes the client a provider uses when it is given none.
//
// It keeps things plain on purpose. Every request gets a connection of its
// own, which is closed when the request is over: nothing is left running
// behind a finished stream, and no request is ever sent down a connection
// that went stale while the laptop slept or the VPN changed. It speaks
// HTTP/1.1, which is all a single stream needs. It asks for no
// compression, so that no proxy holds events back to fill a block. It has
// no time limits of its own; Timeouts are the provider's. And it does not
// follow redirects: an API that redirects is a wrong base URL, and
// following one would hand the key to wherever it points.
func newHTTPClient() *http.Client {
	// The dialer's keep-alive probes are TCP's, not HTTP's: they notice a
	// peer that vanished in the middle of a long, quiet wait for a first
	// token.
	return newHTTPClientDialing((&net.Dialer{KeepAlive: 15 * time.Second}).DialContext)
}

// dialFunc makes a network connection, as net.Dialer.DialContext does.
type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// newHTTPClientDialing is newHTTPClient with the function that makes its
// connections given.
func newHTTPClientDialing(dial dialFunc) *http.Client {
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	return &http.Client{
		Transport: &http.Transport{
			Proxy:              http.ProxyFromEnvironment,
			DialContext:        dialForRequest(dial),
			DisableKeepAlives:  true,
			DisableCompression: true,
			Protocols:          &protocols,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// requestKey is the key under which a request's context carries the
// context that ends when the request does.
type requestKey struct{}

// dialForRequest ties each connection to the request it is made for.
//
// net/http dials with a context of its own, which keeps the request's
// values and drops its cancellation: a dial goes on after the request
// that wanted it has been given up, in case another request can use the
// connection. Here no other request ever will. So the dial is stopped
// when the request ends, and a connection that was made is closed when
// the request ends, whatever state it is in: half way through a TLS
// handshake, waiting on a proxy, or in use. That is what keeps anything
// from outliving a stream.
func dialForRequest(dial dialFunc) dialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		request, _ := ctx.Value(requestKey{}).(context.Context)
		if request == nil {
			return dial(ctx, network, addr)
		}
		dialCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(request, cancel)
		conn, err := dial(dialCtx, network, addr)
		stop()
		cancel()
		if err != nil {
			return nil, err
		}
		context.AfterFunc(request, func() { conn.Close() })
		return conn, nil
	}
}

// limitError is why a request's context was cancelled when one of its time
// limits ran out.
type limitError struct {
	name  string
	after time.Duration
}

func (e *limitError) Error() string {
	return fmt.Sprintf("the %s time limit of %s ran out", e.name, e.after)
}

// deadline is one time limit of a request.
type deadline struct {
	name  string
	d     time.Duration
	timer *time.Timer
	// gen counts how often the limit was set or lifted, so that a timer
	// that fires just as its limit is lifted can tell it is too late.
	gen uint64
}

// limits watches a request's four time limits. A limit that runs out
// cancels the request's context with a *limitError as the cause; whatever
// was blocked on the network then fails, and the cause says why.
//
// No goroutine waits on anything here. A limit is a timer, and a timer
// only costs a goroutine for the instant it fires.
type limits struct {
	mu      sync.Mutex
	cancel  context.CancelCauseFunc
	closed  bool
	started bool // the answer has begun: reads are held to the idle limit

	connect, first, idle, total deadline
}

// init gives the limits their lengths and the function that ends the
// request.
func (l *limits) init(t Timeouts, cancel context.CancelCauseFunc) {
	l.cancel = cancel
	l.connect = deadline{name: "connect", d: t.Connect}
	l.first = deadline{name: "first_token", d: t.FirstToken}
	l.idle = deadline{name: "idle", d: t.Idle}
	l.total = deadline{name: "total", d: t.Total}
}

// set starts d's clock, from zero if it was already running.
func (l *limits) set(d *deadline) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.setLocked(d)
}

func (l *limits) setLocked(d *deadline) {
	l.liftLocked(d)
	if l.closed || d.d <= 0 {
		return
	}
	gen := d.gen
	d.timer = time.AfterFunc(d.d, func() { l.fire(d, gen) })
}

// lift stops d's clock.
func (l *limits) lift(d *deadline) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.liftLocked(d)
}

func (l *limits) liftLocked(d *deadline) {
	d.gen++
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

// fire is what d's timer runs: the limit has run out, unless it was
// lifted or set again in the meantime.
func (l *limits) fire(d *deadline, gen uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || d.gen != gen {
		return
	}
	d.timer = nil
	l.cancel(&limitError{name: d.name, after: d.d})
}

// begun says the answer has begun: the first-token limit is over and the
// idle limit takes its place.
func (l *limits) begun() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.liftLocked(&l.first)
	l.started = true
}

// reading and readDone go around every read of the response body. The
// idle limit only runs while a read is waiting, so the time the stream's
// consumer takes over an event is never held against the server.
func (l *limits) reading() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.started {
		l.setLocked(&l.idle)
	}
}

func (l *limits) readDone() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.liftLocked(&l.idle)
}

// close stops every clock for good.
func (l *limits) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	for _, d := range []*deadline{&l.connect, &l.first, &l.idle, &l.total} {
		l.liftLocked(d)
	}
}

// exchange is one request to a service and its response.
type exchange struct {
	p      *provider
	parent context.Context
	ctx    context.Context
	cancel context.CancelCauseFunc
	lim    limits
	resp   *http.Response
}

// send sends a request and returns once the response's headers are in. The
// caller reads the body through Read or readAll, and calls close when it
// is done, whatever happened.
//
// On failure the error is the one the caller of the package is to get:
// ctx's own error if ctx was cancelled, a *core.ProviderError otherwise.
func (p *provider) send(ctx context.Context, method, target string, body []byte, accept string) (*exchange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	x := &exchange{p: p, parent: ctx}
	x.ctx, x.cancel = context.WithCancelCause(ctx)
	x.lim.init(p.limits, x.cancel)

	// The connect limit runs while the transport is getting a connection,
	// and is lifted as soon as it has one. A transport that does not
	// report its connections leaves it running until the headers arrive.
	hooks := &httptrace.ClientTrace{
		GetConn: func(string) { x.lim.set(&x.lim.connect) },
		GotConn: func(httptrace.GotConnInfo) { x.lim.lift(&x.lim.connect) },
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	// The request's context carries x.ctx as a value as well, for
	// dialForRequest to find.
	reqCtx := httptrace.WithClientTrace(context.WithValue(x.ctx, requestKey{}, x.ctx), hooks)
	req, err := http.NewRequestWithContext(reqCtx, method, target, rd)
	if err != nil {
		x.close()
		return nil, p.fail(core.ErrInvalidRequest, "the request to %s could not be made: %v", traceURL(target), err)
	}
	p.setHeaders(req, body != nil, accept)
	if p.tracing() {
		p.traceFn(TraceRecord{Kind: TraceRequest, Method: method, URL: traceURL(target), Body: string(body)})
	}

	x.lim.set(&x.lim.total)
	x.lim.set(&x.lim.first)
	x.lim.set(&x.lim.connect)
	resp, err := p.client.Do(req)
	x.lim.lift(&x.lim.connect)
	if err != nil {
		err = x.failure("sending the request", err)
		x.close()
		return nil, err
	}
	x.resp = resp
	return x, nil
}

// setHeaders writes a request's headers: the provider's first, then the
// configured ones, which win.
func (p *provider) setHeaders(req *http.Request, hasBody bool, accept string) {
	h := req.Header
	h.Set("User-Agent", userAgent)
	h.Set("Accept", accept)
	if hasBody {
		h.Set("Content-Type", "application/json")
	}
	if p.kind == TypeAnthropic {
		h.Set("Anthropic-Version", "2023-06-01")
		if p.key != "" {
			h.Set("X-Api-Key", p.key)
		}
	} else if p.key != "" {
		h.Set("Authorization", "Bearer "+p.key)
	}
	for k, v := range p.headers {
		switch {
		case strings.EqualFold(k, "Host"):
			req.Host = v
		case v == "":
			h.Del(k)
			if strings.EqualFold(k, "User-Agent") {
				// Without this net/http would put its own name in.
				h["User-Agent"] = []string{""}
			}
		default:
			h.Set(k, v)
		}
	}
}

// traceURL is target as a trace may show it: without a query and without
// a user name or password, either of which can hold a key.
func traceURL(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host + u.Path
}

// Read reads the response body. It is how every byte of a response is
// read, so that the idle limit sees every wait.
func (x *exchange) Read(b []byte) (int, error) {
	x.lim.reading()
	n, err := x.resp.Body.Read(b)
	x.lim.readDone()
	return n, err
}

// readAll reads the body of a response that is taken whole, up to limit
// bytes; whole reports whether that was all of it. The time the server
// takes to start counts against the first-token limit, which the headers
// have just met; the rest of the body is held to the idle limit.
func (x *exchange) readAll(limit int) (body []byte, whole bool, err error) {
	x.lim.begun()
	body, err = io.ReadAll(io.LimitReader(x, int64(limit)+1))
	if err != nil {
		return body, false, x.failure("reading the response", err)
	}
	if len(body) > limit {
		return body[:limit], false, nil
	}
	return body, true, nil
}

// ok reports whether the response's status says the request worked.
func (x *exchange) ok() bool {
	return x.resp.StatusCode >= 200 && x.resp.StatusCode <= 299
}

// statusError reads the body of a response whose status is an error and
// returns the error it stands for.
func (x *exchange) statusError() error {
	body, _, err := x.readAll(maxErrorBody)
	x.traceResponse(body)
	if err != nil {
		// The reason did not arrive whole. If that is because the
		// caller gave up or a time limit ran out, say that; otherwise
		// the status and what did arrive still tell most of the story.
		var pe *core.ProviderError
		if !errors.As(err, &pe) || pe.Kind == core.ErrTimeout {
			return err
		}
	}
	return x.p.apiError(x.resp.StatusCode, x.resp.Header, body)
}

// failure turns an error from the round trip, or from reading the body,
// into the error the caller of the package gets: the time limit that ran
// out, the caller's own cancellation, or a broken connection, in that
// order of asking.
func (x *exchange) failure(doing string, err error) error {
	var lim *limitError
	if errors.As(context.Cause(x.ctx), &lim) {
		return x.p.timeoutError(lim)
	}
	if perr := x.parent.Err(); perr != nil {
		return perr
	}
	// A *url.Error spells out the whole URL, which can hold a key in its
	// query. What matters is underneath.
	var uerr *url.Error
	if errors.As(err, &uerr) && uerr.Err != nil {
		err = uerr.Err
	}
	return &core.ProviderError{
		Provider: x.p.name,
		Kind:     core.ErrNetwork,
		Message:  doing + ": " + err.Error(),
		Err:      err,
	}
}

// timeoutError makes the error for a time limit that ran out.
func (p *provider) timeoutError(l *limitError) *core.ProviderError {
	var msg string
	switch l.name {
	case "connect":
		msg = fmt.Sprintf("no connection to the server after %s", l.after)
	case "first_token":
		msg = fmt.Sprintf("the server sent none of its answer in %s", l.after)
	case "idle":
		msg = fmt.Sprintf("the server went silent for %s in the middle of its answer", l.after)
	default:
		msg = fmt.Sprintf("the request was not over after %s", l.after)
	}
	return &core.ProviderError{Provider: p.name, Kind: core.ErrTimeout, Timeout: l.name, Message: msg}
}

// close ends the exchange: the clocks stop, the request is cancelled and
// the connection is given up. It is safe to call more than once.
func (x *exchange) close() {
	x.lim.close()
	x.cancel(nil)
	if x.resp != nil {
		x.resp.Body.Close()
	}
}

// call makes a request whose response is read whole, and returns the
// response's body. A response with an error status comes back as the
// error it stands for.
func (p *provider) call(ctx context.Context, method, target string, body []byte) ([]byte, error) {
	x, err := p.send(ctx, method, target, body, "application/json")
	if err != nil {
		return nil, err
	}
	defer x.close()
	if !x.ok() {
		return nil, x.statusError()
	}
	out, whole, err := x.readAll(maxWholeBody)
	x.traceResponse(out)
	if err != nil {
		return nil, err
	}
	if !whole {
		e := p.fail(core.ErrBadResponse, "the response is larger than %d bytes", maxWholeBody)
		e.Body = string(clip(out, bodyStart))
		return nil, e
	}
	return out, nil
}

// traceResponse tells the trace hook about the response: its status, its
// headers, and its body if that was read whole.
func (x *exchange) traceResponse(body []byte) {
	if x.p.tracing() {
		x.p.traceFn(TraceRecord{Kind: TraceResponse, Status: x.resp.StatusCode, Header: x.resp.Header.Clone(), Body: string(body)})
	}
}

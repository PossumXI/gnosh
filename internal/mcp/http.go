package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"
)

// httpTransport is a server reached at a URL, by MCP's Streamable HTTP
// transport: each message the client sends is a POST of its own. The
// answer comes back as one JSON message (or a list of them), or as a
// stream of server-sent events, each a message, which may hold the
// server's own requests before the answer. The session id the server
// gives with its answer to the handshake goes with every later request,
// and the session is ended with a DELETE when the client closes.
//
// The client opens no stream of its own with a GET, which the protocol
// leaves to it: a server's notifications between requests (that its tool
// list changed, say) are not heard.
type httpTransport struct {
	c       *Client
	url     string
	headers map[string]string
	client  *http.Client
	// auth gives the requests the person's sign-in, when the server's
	// Authorization header is not configured (auth.go).
	auth *Authorizer
	// base ends with the connection: every request is made in it too.
	base context.Context
	end  context.CancelFunc

	mu      sync.Mutex
	session string
	version string
}

// maxHTTPMessage is the most of one answer that is read.
const maxHTTPMessage = 16 << 20

// StartHTTP connects to a server at a URL and makes the handshake.
// headers go with every request.
func StartHTTP(ctx context.Context, name, url string, headers map[string]string, opt Options) (*Client, error) {
	opt = opt.defaults()
	c := &Client{srv: Server{Name: name}, opt: opt, pending: map[string]chan message{}}
	t := &httpTransport{c: c, url: url, headers: headers}
	if opt.Auth != nil && !hasHeader(headers, "Authorization") {
		t.auth = opt.Auth
	}
	t.base, t.end = context.WithCancel(context.Background())
	t.client = &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// A redirect elsewhere would take the configured headers, which
		// may hold a key, to a host nobody named.
		if req.URL.Host != via[0].URL.Host || req.URL.Scheme != via[0].URL.Scheme {
			return fmt.Errorf("the server sent the request on to %s://%s, which is not where it is configured", req.URL.Scheme, req.URL.Host)
		}
		if len(via) >= 5 {
			return errors.New("the server redirected too many times")
		}
		return nil
	}}
	c.t = t
	return c, c.handshake(ctx)
}

func (t *httpTransport) stderr() string { return "" }

// setVersion notes the protocol version the server answered the handshake
// with, which goes with every later request.
func (t *httpTransport) setVersion(v string) {
	t.mu.Lock()
	t.version = v
	t.mu.Unlock()
}

func (t *httpTransport) send(ctx context.Context, m message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	for renewed := false; ; renewed = true {
		resp, done, token, err := t.post(ctx, data)
		if err != nil {
			return err
		}
		if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && t.auth != nil {
			// The server wants a sign-in, or a better one: the token is
			// renewed once, and otherwise the person signs in (/mcp).
			www := resp.Header.Get("WWW-Authenticate")
			resp.Body.Close()
			done()
			if !renewed && token != "" && t.auth.renew(ctx, token) {
				continue
			}
			return &NeedsAuth{Status: resp.StatusCode, WWWAuthenticate: www}
		}
		return t.answered(m, resp, done)
	}
}

// post makes one POST of a message: the answer, how to end the request
// once its answer is read, and the token it carried.
func (t *httpTransport) post(ctx context.Context, data []byte) (*http.Response, func(), string, error) {
	// The request ends with ctx, or with the connection.
	rctx, cancel := context.WithCancel(ctx)
	unhook := context.AfterFunc(t.base, cancel)
	done := func() {
		unhook()
		cancel()
	}
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, t.url, bytes.NewReader(data))
	if err != nil {
		done()
		return nil, nil, "", err
	}
	token := t.header(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := t.client.Do(req)
	if err != nil {
		done()
		if t.base.Err() != nil {
			return nil, nil, "", ErrClosed
		}
		return nil, nil, "", fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	return resp, done, token, nil
}

// answered takes the server's answer to the message m.
func (t *httpTransport) answered(m message, resp *http.Response, done func()) error {
	if id := resp.Header.Get("Mcp-Session-Id"); id != "" && m.Method == "initialize" {
		t.mu.Lock()
		t.session = id
		t.mu.Unlock()
	}
	switch {
	case resp.StatusCode == http.StatusAccepted:
		resp.Body.Close()
		done()
		return nil
	case resp.StatusCode == http.StatusNotFound && t.hasSession():
		resp.Body.Close()
		done()
		return fmt.Errorf("%w (it answered 404 Not Found); reconnect to start another", ErrSessionEnded)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		resp.Body.Close()
		done()
		if text := strings.TrimSpace(string(body)); text != "" {
			return fmt.Errorf("mcp: the server answered %s: %s", resp.Status, text)
		}
		return fmt.Errorf("mcp: the server answered %s", resp.Status)
	}
	media, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if media == "text/event-stream" {
		// The stream may stay open as long as the call takes: it is read
		// beside the request, which waits for its answer as it would over
		// stdio.
		go func() {
			defer done()
			defer resp.Body.Close()
			t.events(resp.Body)
		}()
		return nil
	}
	defer done()
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPMessage))
	if err != nil {
		return fmt.Errorf("mcp: the server's answer could not be read: %w", err)
	}
	return t.take(body)
}

// header puts the configured headers, the session id and the protocol
// version on a request, and the sign-in's token when there is one, which
// it returns.
func (t *httpTransport) header(req *http.Request) (token string) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	if t.auth != nil {
		if token = t.auth.token(); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.session != "" {
		req.Header.Set("Mcp-Session-Id", t.session)
	}
	if t.version != "" {
		req.Header.Set("MCP-Protocol-Version", t.version)
	}
	return token
}

func (t *httpTransport) hasSession() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.session != ""
}

// take dispatches what a JSON answer holds: one message, or a list.
func (t *httpTransport) take(body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil
	}
	if body[0] == '[' {
		var list []message
		if err := json.Unmarshal(body, &list); err != nil {
			return fmt.Errorf("mcp: the server's answer is not JSON-RPC: %w", err)
		}
		for _, m := range list {
			t.c.dispatch(m)
		}
		return nil
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("mcp: the server's answer is not JSON-RPC: %w", err)
	}
	t.c.dispatch(m)
	return nil
}

// events reads a stream of server-sent events, each event's data a
// message.
func (t *httpTransport) events(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxHTTPMessage)
	var data strings.Builder
	flush := func() {
		if data.Len() > 0 {
			_ = t.take([]byte(data.String()))
			data.Reset()
		}
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
		// (event:, id: and retry: lines, and comments, carry nothing the
		// client needs.)
	}
	flush()
}

// close ends the session, if the server gave one, with a DELETE, and every
// request still made.
func (t *httpTransport) close() error {
	t.end()
	t.mu.Lock()
	session := t.session
	t.mu.Unlock()
	if session != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if req, err := http.NewRequestWithContext(ctx, http.MethodDelete, t.url, nil); err == nil {
			t.header(req)
			if resp, err := t.client.Do(req); err == nil {
				resp.Body.Close()
			}
		}
	}
	t.client.CloseIdleConnections()
	return nil
}

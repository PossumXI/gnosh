// Package mcp is a client of MCP servers (the Model Context Protocol):
// outside programs that offer tools. It speaks JSON-RPC 2.0 to a server
// in one of two ways (spec/cc/mcp.md): to a server it starts, over the
// server's standard input and output, one message to a line (the stdio
// transport, stdio.go); or to a server at a URL, a POST to a message
// (the Streamable HTTP transport, http.go).
//
// A Client makes the initialize handshake, lists the server's tools and
// calls them. It answers what a server may ask of a client: ping, and
// roots/list with the folders it was given; anything else is answered
// "method not found". Stopping a started server closes its input and then,
// as Claude Code does, sends SIGINT, SIGTERM 100 ms later and SIGKILL
// 400 ms after that, to the server's whole process group.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ProtocolVersion is the version of MCP the client asks for. A server
// answers with the version it speaks, which the client accepts.
const ProtocolVersion = "2025-06-18"

// Server is how to start one server.
type Server struct {
	// Name is the server's name in the configuration.
	Name string
	// Command, Args and Env start it: Env is added to Environ.
	Command string
	Args    []string
	Env     map[string]string
	// Environ is the environment the server starts from; nil means none
	// at all but Env.
	Environ []string
	// Dir is the folder it starts in.
	Dir string
	// Roots are the folders roots/list names, as file URLs are made of
	// them.
	Roots []string
}

// Options are the client's own settings.
type Options struct {
	// ClientName and ClientVersion are what the handshake says the client
	// is. Default "gnosh" and "devel".
	ClientName, ClientVersion string
	// StartTimeout bounds starting the server and the handshake. Default
	// 30 seconds.
	StartTimeout time.Duration
	// MaxStderr is how much of the server's standard error is kept, for
	// the person to read when it fails. Default 16 KiB.
	MaxStderr int
	// MaxLine is the longest message the server may send. Default 32 MiB.
	MaxLine int
	// OnToolsChanged, when set, is called (in a goroutine of its own) each
	// time the server says its tool list changed.
	OnToolsChanged func()
}

// Tool is one tool a server offers.
type Tool struct {
	Name        string          `json:"name"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
	Annotations struct {
		ReadOnly bool `json:"readOnlyHint,omitempty"`
	} `json:"annotations"`
}

// Content is one piece of what a tool call came to.
type Content struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
	Resource *struct {
		URI      string `json:"uri"`
		Text     string `json:"text,omitempty"`
		MimeType string `json:"mimeType,omitempty"`
	} `json:"resource,omitempty"`
}

// Result is what a tool call came to.
type Result struct {
	Content           []Content       `json:"content"`
	IsError           bool            `json:"isError,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent,omitempty"`
}

// Info is what the server said of itself in the handshake.
type Info struct {
	Name            string
	Version         string
	ProtocolVersion string
	Instructions    string
}

// RPCError is an error a server answered a request with.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("the server answered with an error (%d): %s", e.Code, e.Message)
}

// ErrClosed is returned for a request made, or pending, when the server's
// connection is gone.
var ErrClosed = errors.New("mcp: the server is not running")

// message is one JSON-RPC message, either way.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Client is the connection to one server.
type Client struct {
	srv  Server
	opt  Options
	t    transport
	info Info

	mu      sync.Mutex
	next    int64
	pending map[string]chan message
	closed  bool
	err     error // why the connection ended
	// changed is set when the server says its tool list changed.
	changed bool
}

// transport carries messages to a server and brings its own back, which
// it hands to the client's dispatch.
type transport interface {
	// send hands one message to the server.
	send(ctx context.Context, m message) error
	// stderr is the end of what a started server wrote on its standard
	// error, or "".
	stderr() string
	// close ends the connection, and with a started server the server.
	close() error
}

// Start starts the server and makes the handshake.
func Start(ctx context.Context, srv Server, opt Options) (*Client, error) {
	opt = opt.defaults()
	if strings.TrimSpace(srv.Command) == "" {
		return nil, errors.New("mcp: the server has no command")
	}
	c := &Client{srv: srv, opt: opt, pending: map[string]chan message{}}
	st, err := startStdio(c, srv, opt)
	if err != nil {
		return nil, err
	}
	c.t = st
	return c, c.handshake(ctx)
}

// defaults fills in the options that were left at zero.
func (opt Options) defaults() Options {
	if opt.ClientName == "" {
		opt.ClientName = "gnosh"
	}
	if opt.ClientVersion == "" {
		opt.ClientVersion = "devel"
	}
	if opt.StartTimeout <= 0 {
		opt.StartTimeout = 30 * time.Second
	}
	if opt.MaxStderr <= 0 {
		opt.MaxStderr = 16 << 10
	}
	if opt.MaxLine <= 0 {
		opt.MaxLine = 32 << 20
	}
	return opt
}

// handshake makes the initialize handshake. On failure the connection is
// closed.
func (c *Client) handshake(ctx context.Context) error {
	opt := c.opt
	hctx, cancel := context.WithTimeout(ctx, opt.StartTimeout)
	defer cancel()
	raw, err := c.request(hctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{"roots": map[string]any{"listChanged": false}},
		"clientInfo":      map[string]any{"name": opt.ClientName, "version": opt.ClientVersion},
	})
	if err != nil {
		c.Close()
		return c.failure("the handshake failed", err)
	}
	var hello struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(raw, &hello); err != nil {
		c.Close()
		return fmt.Errorf("mcp: the server's answer to the handshake cannot be read: %w", err)
	}
	c.info = Info{Name: hello.ServerInfo.Name, Version: hello.ServerInfo.Version, ProtocolVersion: hello.ProtocolVersion, Instructions: hello.Instructions}
	if h, ok := c.t.(*httpTransport); ok {
		h.setVersion(hello.ProtocolVersion)
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		c.Close()
		return c.failure("the handshake could not be finished", err)
	}
	return nil
}

// failure says why the server did not answer, with what it wrote on its
// standard error.
func (c *Client) failure(what string, err error) error {
	if tail := strings.TrimSpace(c.t.stderr()); tail != "" {
		return fmt.Errorf("mcp: %s: %w. What the server wrote: %s", what, err, tail)
	}
	return fmt.Errorf("mcp: %s: %w", what, err)
}

// Info returns what the server said of itself.
func (c *Client) Info() Info { return c.info }

// Stderr returns the end of what the server wrote on its standard error.
func (c *Client) Stderr() string { return c.t.stderr() }

// ToolsChanged reports whether the server said its tool list changed since
// the last call of ListTools.
func (c *Client) ToolsChanged() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.changed
}

// ListTools lists the server's tools, every page of them.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	c.mu.Lock()
	c.changed = false
	c.mu.Unlock()
	var all []Tool
	cursor := ""
	for page := 0; page < 100; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.request(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var res struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return nil, fmt.Errorf("mcp: the tool list cannot be read: %w", err)
		}
		all = append(all, res.Tools...)
		if res.NextCursor == "" {
			return all, nil
		}
		cursor = res.NextCursor
	}
	return nil, errors.New("mcp: the server's tool list never ends")
}

// CallTool calls one tool with its arguments, a JSON object.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (Result, error) {
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	raw, err := c.request(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return Result{}, err
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return Result{}, fmt.Errorf("mcp: the tool's result cannot be read: %w", err)
	}
	return res, nil
}

// Close ends the connection. A started server's input is closed, and then
// it is sent SIGINT, SIGTERM 100 ms later and SIGKILL 400 ms after that, as
// long as it runs; an http server's session is ended.
func (c *Client) Close() error {
	c.mu.Lock()
	already := c.closed
	c.closed = true
	c.mu.Unlock()
	if already || c.t == nil {
		return nil
	}
	err := c.t.close()
	c.lost(nil)
	return err
}

// request sends a request and waits for its answer.
func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed || c.err != nil {
		err := c.err
		c.mu.Unlock()
		if err == nil {
			err = ErrClosed
		}
		return nil, err
	}
	c.next++
	id := fmt.Sprintf("%d", c.next)
	ch := make(chan message, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	// The server is told a request that is given up.
	giveUp := func() error {
		_ = c.notify("notifications/cancelled", map[string]any{"requestId": json.RawMessage(id), "reason": "the client stopped waiting"})
		return ctx.Err()
	}
	// (Over http the answer may come back with the request itself: send
	// then returns once it has been handed to the channel.)
	if err := c.t.send(ctx, message{JSONRPC: "2.0", ID: json.RawMessage(id), Method: method, Params: mustJSON(params)}); err != nil {
		if ctx.Err() != nil {
			return nil, giveUp()
		}
		return nil, err
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return nil, c.ended()
		}
		if m.Error != nil {
			return nil, m.Error
		}
		return m.Result, nil
	case <-ctx.Done():
		return nil, giveUp()
	}
}

// ended is why pending requests can no longer be answered.
func (c *Client) ended() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	return ErrClosed
}

func (c *Client) notify(method string, params any) error {
	return c.send(message{JSONRPC: "2.0", Method: method, Params: mustJSON(params)})
}

// send sends what needs no answer: a notification, or the client's answer
// to a server's request.
func (c *Client) send(m message) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.opt.StartTimeout)
	defer cancel()
	return c.t.send(ctx, m)
}

// dispatch takes one message from the server: an answer goes to the request
// that waits for it, a request of the server's is answered, and a
// notification is noted.
func (c *Client) dispatch(m message) {
	switch {
	case m.Method != "" && len(m.ID) > 0:
		go c.answer(m)
	case m.Method != "":
		c.notified(m)
	case len(m.ID) > 0:
		id := strings.Trim(string(m.ID), `"`)
		c.mu.Lock()
		ch := c.pending[id]
		c.mu.Unlock()
		if ch != nil {
			select {
			case ch <- m:
			default:
			}
		}
	}
}

// lost ends the connection for every request that waits: they fail with
// err, or ErrClosed.
func (c *Client) lost(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.err = err
		if c.err == nil {
			c.err = ErrClosed
		}
	}
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

// answer answers a request the server made of the client.
func (c *Client) answer(m message) {
	reply := message{JSONRPC: "2.0", ID: m.ID}
	switch m.Method {
	case "ping":
		reply.Result = json.RawMessage(`{}`)
	case "roots/list":
		roots := make([]map[string]string, 0, len(c.srv.Roots))
		for _, r := range c.srv.Roots {
			roots = append(roots, map[string]string{"uri": "file://" + r, "name": r})
		}
		reply.Result = mustJSON(map[string]any{"roots": roots})
	default:
		reply.Error = &RPCError{Code: -32601, Message: "gnosh does not answer " + m.Method}
	}
	_ = c.send(reply)
}

// notified takes a notification from the server.
func (c *Client) notified(m message) {
	if m.Method == "notifications/tools/list_changed" {
		c.mu.Lock()
		c.changed = true
		c.mu.Unlock()
		if c.opt.OnToolsChanged != nil {
			go c.opt.OnToolsChanged()
		}
	}
}

func mustJSON(v any) json.RawMessage {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// capped keeps the last max bytes written to it.
type capped struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (c *capped) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.buf = append(c.buf, p...)
	if len(c.buf) > c.max {
		c.buf = append([]byte(nil), c.buf[len(c.buf)-c.max:]...)
	}
	return len(p), nil
}

func (c *capped) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return string(c.buf)
}

// Environ returns the process's environment, for Server.Environ.
func Environ() []string { return os.Environ() }

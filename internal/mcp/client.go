// Package mcp is a client of MCP servers (the Model Context Protocol):
// outside programs that offer tools. It speaks JSON-RPC 2.0 to a server it
// starts and talks to over the server's standard input and output, one
// message to a line, which is MCP's stdio transport (spec/cc/mcp.md).
//
// A Client starts the server, makes the initialize handshake, lists the
// server's tools and calls them. It answers what a server may ask of a
// client: ping, and roots/list with the folders it was given; anything
// else is answered "method not found". Stopping a server closes its input
// and then, as Claude Code does, sends SIGINT, SIGTERM 100 ms later and
// SIGKILL 400 ms after that, to the server's whole process group.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
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

// Client is the connection to one started server.
type Client struct {
	srv  Server
	opt  Options
	cmd  *exec.Cmd
	in   io.WriteCloser
	info Info

	writeMu sync.Mutex
	mu      sync.Mutex
	next    int64
	pending map[string]chan message
	closed  bool
	err     error // why the connection ended
	stderr  *capped
	done    chan struct{} // closed when the reader stops
	// changed is set when the server says its tool list changed.
	changed bool
}

// Start starts the server and makes the handshake.
func Start(ctx context.Context, srv Server, opt Options) (*Client, error) {
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
	if strings.TrimSpace(srv.Command) == "" {
		return nil, errors.New("mcp: the server has no command")
	}
	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Dir = srv.Dir
	env := append([]string(nil), srv.Environ...)
	for k, v := range srv.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	// A group of its own, so that what it starts is stopped with it.
	ownGroup(cmd)
	c := &Client{srv: srv, opt: opt, cmd: cmd, pending: map[string]chan message{}, stderr: &capped{max: opt.MaxStderr}, done: make(chan struct{})}
	cmd.Stderr = c.stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: %s could not be started: %w", srv.Command, err)
	}
	c.in = in
	go c.read(out)

	hctx, cancel := context.WithTimeout(ctx, opt.StartTimeout)
	defer cancel()
	raw, err := c.request(hctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{"roots": map[string]any{"listChanged": false}},
		"clientInfo":      map[string]any{"name": opt.ClientName, "version": opt.ClientVersion},
	})
	if err != nil {
		c.Close()
		return nil, c.failure("the handshake failed", err)
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
		return nil, fmt.Errorf("mcp: the server's answer to the handshake cannot be read: %w", err)
	}
	c.info = Info{Name: hello.ServerInfo.Name, Version: hello.ServerInfo.Version, ProtocolVersion: hello.ProtocolVersion, Instructions: hello.Instructions}
	if err := c.notify("notifications/initialized", nil); err != nil {
		c.Close()
		return nil, c.failure("the handshake could not be finished", err)
	}
	return c, nil
}

// failure says why the server did not answer, with what it wrote on its
// standard error.
func (c *Client) failure(what string, err error) error {
	if tail := strings.TrimSpace(c.stderr.String()); tail != "" {
		return fmt.Errorf("mcp: %s: %w. What the server wrote: %s", what, err, tail)
	}
	return fmt.Errorf("mcp: %s: %w", what, err)
}

// Info returns what the server said of itself.
func (c *Client) Info() Info { return c.info }

// Stderr returns the end of what the server wrote on its standard error.
func (c *Client) Stderr() string { return c.stderr.String() }

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

// Close stops the server: its input is closed, and then it is sent SIGINT,
// SIGTERM 100 ms later and SIGKILL 400 ms after that, as long as it runs.
func (c *Client) Close() error {
	c.mu.Lock()
	already := c.closed
	c.closed = true
	c.mu.Unlock()
	if already {
		return nil
	}
	_ = c.in.Close()
	exited := make(chan struct{})
	go func() {
		_ = c.cmd.Wait()
		close(exited)
	}()
	for i, wait := range []time.Duration{100 * time.Millisecond, 400 * time.Millisecond, 2 * time.Second} {
		select {
		case <-exited:
			<-c.done
			return nil
		default:
		}
		stop(c.cmd, i)
		select {
		case <-exited:
			<-c.done
			return nil
		case <-time.After(wait):
		}
	}
	return fmt.Errorf("mcp: %s did not stop", c.srv.Name)
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
	if err := c.send(message{JSONRPC: "2.0", ID: json.RawMessage(id), Method: method, Params: mustJSON(params)}); err != nil {
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
		// The server is told the request is given up.
		_ = c.notify("notifications/cancelled", map[string]any{"requestId": json.RawMessage(id), "reason": "the client stopped waiting"})
		return nil, ctx.Err()
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

func (c *Client) send(m message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if _, err := c.in.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("%w (%v)", ErrClosed, err)
	}
	return nil
}

// read takes the server's messages until its output ends.
func (c *Client) read(out io.Reader) {
	defer close(c.done)
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), c.opt.MaxLine)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m message
		if json.Unmarshal(line, &m) != nil {
			// A server that writes other text on its output is broken, but
			// one line of it is not worth the connection.
			continue
		}
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
				ch <- m
			}
		}
	}
	err := sc.Err()
	c.mu.Lock()
	if err != nil {
		c.err = fmt.Errorf("mcp: the server's output could not be read: %w", err)
	} else {
		c.err = ErrClosed
	}
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()
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

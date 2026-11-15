package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// stdioTransport is a server gnosh starts, spoken to over its standard
// input and output, one message to a line.
type stdioTransport struct {
	c      *Client
	name   string
	cmd    *exec.Cmd
	in     io.WriteCloser
	mu     sync.Mutex
	errs   *capped
	done   chan struct{} // closed when the reader stops
	maxLen int
}

// startStdio starts the server.
func startStdio(c *Client, srv Server, opt Options) (*stdioTransport, error) {
	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Dir = srv.Dir
	env := append([]string(nil), srv.Environ...)
	for k, v := range srv.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	// A group of its own, so that what it starts is stopped with it.
	ownGroup(cmd)
	t := &stdioTransport{c: c, name: srv.Name, cmd: cmd, errs: &capped{max: opt.MaxStderr}, done: make(chan struct{}), maxLen: opt.MaxLine}
	cmd.Stderr = t.errs
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
	t.in = in
	go t.read(out)
	return t, nil
}

func (t *stdioTransport) stderr() string { return t.errs.String() }

func (t *stdioTransport) send(_ context.Context, m message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.in.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("%w (%v)", ErrClosed, err)
	}
	return nil
}

// read takes the server's messages until its output ends.
func (t *stdioTransport) read(out io.Reader) {
	defer close(t.done)
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64<<10), t.maxLen)
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
		t.c.dispatch(m)
	}
	if err := sc.Err(); err != nil {
		t.c.lost(fmt.Errorf("mcp: the server's output could not be read: %w", err))
		return
	}
	t.c.lost(nil)
}

// close closes the server's input, and then sends it SIGINT, SIGTERM 100 ms
// later and SIGKILL 400 ms after that, as long as it runs.
func (t *stdioTransport) close() error {
	_ = t.in.Close()
	exited := make(chan struct{})
	go func() {
		_ = t.cmd.Wait()
		close(exited)
	}()
	for i, wait := range []time.Duration{100 * time.Millisecond, 400 * time.Millisecond, 2 * time.Second} {
		select {
		case <-exited:
			<-t.done
			return nil
		default:
		}
		stop(t.cmd, i)
		select {
		case <-exited:
			<-t.done
			return nil
		case <-time.After(wait):
		}
	}
	return fmt.Errorf("mcp: %s did not stop", t.name)
}

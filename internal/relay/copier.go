package relay

import (
	"context"
	"sync"
	"time"
)

// copier puts the far side's copies on the clipboard one at a time, in
// the background, so that a slow clipboard never holds up the screen. The
// clipboard keeps one text, so a copy still waiting when a newer one comes
// is dropped: the newest is the one that counts.
type copier struct {
	put    func(ctx context.Context, text string) error
	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{}
	done   chan struct{}

	mu      sync.Mutex
	next    string
	waiting bool // next is a copy not made yet
	closing bool
	failed  int
	lastErr error
}

// newCopier starts a copier that copies with put.
func newCopier(put func(ctx context.Context, text string) error) *copier {
	ctx, cancel := context.WithCancel(context.Background())
	c := &copier{put: put, ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go c.run()
	return c
}

// offer hands the copier text to copy.
func (c *copier) offer(text string) {
	c.mu.Lock()
	c.next, c.waiting = text, true
	c.mu.Unlock()
	c.poke()
}

// poke wakes the copier.
func (c *copier) poke() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// run makes the copies, until finish asks it to end and none waits.
func (c *copier) run() {
	defer close(c.done)
	for range c.wake {
		for {
			c.mu.Lock()
			text, waiting, closing := c.next, c.waiting, c.closing
			c.waiting = false
			c.mu.Unlock()
			if !waiting {
				if closing {
					return
				}
				break
			}
			// (One stopped by finish is not counted: finish says so.)
			if err := c.put(c.ctx, text); err != nil && c.ctx.Err() == nil {
				c.mu.Lock()
				c.failed, c.lastErr = c.failed+1, err
				c.mu.Unlock()
			}
		}
	}
}

// finish waits up to wait for the newest copy to be made, and then stops
// the one under way. all says the copier was done by then; failed and err
// say how many copies could not be made, and why the last one could not.
func (c *copier) finish(wait time.Duration) (all bool, failed int, err error) {
	c.mu.Lock()
	c.closing = true
	c.mu.Unlock()
	c.poke()
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-c.done:
		all = true
	case <-t.C:
	}
	c.cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	return all, c.failed, c.lastErr
}

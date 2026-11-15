package shelltool

import (
	"bytes"
	"sync"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// progressSink carries a running command's output to the session's
// Progress function without ever making the command wait for it. The
// command's side only appends to a bounded buffer; a goroutine of the
// sink's own hands the buffer over, whole lines at a time. When the
// listener is slower than the command, the oldest waiting output is
// dropped and the listener is told how much.
type progressSink struct {
	fn     func(core.Progress)
	callID string

	mu      sync.Mutex
	pending []byte   // output not yet handed over
	notes   []string // status lines not yet handed over
	dropped int64    // bytes dropped since the listener was last told
	closed  bool

	wake chan struct{} // holds one token when there is something to do
	done chan struct{} // closed when the goroutine has returned
}

// newProgressSink starts a sink that feeds fn. The caller must close it.
func newProgressSink(fn func(core.Progress), callID string) *progressSink {
	s := &progressSink{
		fn:     fn,
		callID: callID,
		wake:   make(chan struct{}, 1),
		done:   make(chan struct{}),
	}
	go s.run()
	return s
}

// write takes the next piece of output. It never blocks on the listener.
func (s *progressSink) write(p []byte) {
	s.mu.Lock()
	if len(s.pending)+len(p) > progressBacklogBytes {
		// The listener is behind. Keep the newest output.
		s.dropped += int64(len(s.pending))
		s.pending = s.pending[:0]
		if over := len(p) - progressBacklogBytes; over > 0 {
			s.dropped += int64(over)
			p = p[over:]
		}
	}
	s.pending = append(s.pending, p...)
	s.mu.Unlock()
	s.poke()
}

// note queues a status line, such as the news that the command is being
// stopped.
func (s *progressSink) note(text string) {
	s.mu.Lock()
	s.notes = append(s.notes, text)
	s.mu.Unlock()
	s.poke()
}

// close hands over what is left and stops the goroutine. It waits for the
// listener, but not for long: a listener that blocks must not hold up the
// result of the command.
func (s *progressSink) close() { s.closeWithin(250 * time.Millisecond) }

// closeWithin is close, waiting up to d for the listener.
func (s *progressSink) closeWithin(d time.Duration) {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.poke()
	waitFor(s.done, d)
}

func (s *progressSink) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// run hands output over until the sink is closed. A line is handed over
// when its newline arrives. A line that stays unfinished for a moment (a
// prompt, a progress bar), or that grows very long, is handed over as far
// as it has come.
func (s *progressSink) run() {
	defer close(s.done)
	var (
		held   []byte    // the start of a line whose end has not come
		heldAt time.Time // when held last started to wait
		spare  []byte    // a second buffer, swapped with pending, so that none is allocated
		timer  = time.NewTimer(time.Hour)
	)
	defer timer.Stop()
	for {
		var linger <-chan time.Time
		if len(held) > 0 {
			timer.Reset(time.Until(heldAt.Add(progressPartialLinger)))
			linger = timer.C
		}
		waited := false
		select {
		case <-s.wake:
		case <-linger:
			waited = true
		}

		s.mu.Lock()
		fresh := s.pending
		s.pending = spare[:0]
		notes := s.notes
		s.notes = nil
		dropped := s.dropped
		s.dropped = 0
		closed := s.closed
		s.mu.Unlock()

		if dropped > 0 {
			// Whatever was held is the start of output that is gone.
			held = held[:0]
			s.fn(core.Progress{CallID: s.callID, Stream: "status", Text: byteSize(dropped) + " of output went by too fast to show"})
		}
		if len(held) == 0 {
			heldAt = time.Now()
		}
		held = append(held, fresh...)
		spare = fresh

		cut := bytes.LastIndexByte(held, '\n') + 1
		switch {
		case closed:
			cut = len(held)
		case cut == 0 && (waited || len(held) >= progressBacklogBytes/4):
			cut = len(held) - incompleteSuffix(held)
			heldAt = time.Now() // what stays behind waits afresh
		}
		if cut > 0 {
			s.fn(core.Progress{CallID: s.callID, Stream: "stdout", Text: clean(held[:cut])})
			held = append(held[:0], held[cut:]...)
			heldAt = time.Now()
		}
		for _, text := range notes {
			s.fn(core.Progress{CallID: s.callID, Stream: "status", Text: text})
		}
		if closed {
			return
		}
	}
}

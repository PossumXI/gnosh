package agent

import (
	"context"
	"sync"
	"time"
)

// defaultBacklog is how many bytes of undelivered live output a sink keeps
// before it starts to throw the oldest pieces away.
const defaultBacklog = 32 << 20

// drainGrace is how long, after the session is closed, a sink goes on
// offering events to a channel nobody reads before it gives up.
const drainGrace = 5 * time.Second

// sink carries the loop's events to whoever consumes them. Putting an event
// in never blocks, so a slow consumer cannot stall the loop: events wait in
// a list, and one goroutine hands them on in order.
//
// While events wait, pieces of live output that follow each other (the
// answer's text, the reasoning, one call's arguments, one call's output)
// are joined into one event, which loses nothing. Only if the waiting
// output still grows past the backlog are the oldest pieces thrown away,
// and a LostEvent takes their place.
type sink struct {
	session string
	clock   Clock
	limit   int

	mu     sync.Mutex
	wake   *sync.Cond
	q      []Event
	bytes  int // text held by the waiting pieces of live output
	seq    uint64
	closed bool
	busy   bool // the pump is handing an event to the consumer

	// One of these is set. deliver is the consumer's function; ch is the
	// channel of Agent.Events.
	deliver func(Event)
	ch      chan Event

	started bool
	done    chan struct{} // closed when the pump has handed on its last event
	giveUp  chan struct{} // closed when a channel nobody reads is given up on
}

func newSink(session string, clock Clock, deliver func(Event), limit int) *sink {
	if limit <= 0 {
		limit = defaultBacklog
	}
	s := &sink{
		session: session,
		clock:   clock,
		limit:   limit,
		deliver: deliver,
		done:    make(chan struct{}),
		giveUp:  make(chan struct{}),
	}
	s.wake = sync.NewCond(&s.mu)
	if deliver == nil {
		s.ch = make(chan Event, 16)
	}
	return s
}

// start runs the pump. It is called once, when the session starts.
func (s *sink) start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go s.pump()
}

// emit puts ev in line. It may be called from any goroutine and never
// blocks. After close it does nothing.
func (s *sink) emit(ev Event) {
	m := ev.EventMeta()
	if m.Time.IsZero() {
		m.Time = s.clock.Now()
	}
	if m.Session == "" {
		m.Session = s.session
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	n := pieceLen(ev)
	if n >= 0 && len(s.q) > 0 && joinPiece(s.q[len(s.q)-1], ev) {
		s.bytes += n
	} else {
		s.q = append(s.q, ev)
		if n > 0 {
			s.bytes += n
		}
	}
	if s.bytes > s.limit {
		s.shed()
	}
	s.wake.Signal()
}

// pieceLen returns how much text a piece of live output carries, and -1 for
// every other event.
func pieceLen(ev Event) int {
	switch e := ev.(type) {
	case *TextEvent:
		return len(e.Text)
	case *ThinkingEvent:
		return len(e.Text)
	case *ToolInputEvent:
		return len(e.Text)
	case *ToolProgressEvent:
		return len(e.Text)
	}
	return -1
}

// samePlace reports whether two events belong to the same agent and turn.
func samePlace(a, b *Meta) bool {
	return a.Agent == b.Agent && a.Parent == b.Parent && a.Turn == b.Turn && a.Session == b.Session
}

// joinPiece adds next's text to last when both are the same kind of live
// output from the same place, and reports whether it did.
func joinPiece(last, next Event) bool {
	if !samePlace(last.EventMeta(), next.EventMeta()) {
		return false
	}
	switch l := last.(type) {
	case *TextEvent:
		if n, ok := next.(*TextEvent); ok {
			l.Text += n.Text
			return true
		}
	case *ThinkingEvent:
		if n, ok := next.(*ThinkingEvent); ok {
			l.Text += n.Text
			return true
		}
	case *ToolInputEvent:
		if n, ok := next.(*ToolInputEvent); ok && n.CallID == l.CallID {
			l.Text += n.Text
			return true
		}
	case *ToolProgressEvent:
		if n, ok := next.(*ToolProgressEvent); ok && n.CallID == l.CallID && n.Stream == l.Stream {
			l.Text += n.Text
			return true
		}
	}
	return false
}

// shed throws the oldest pieces of live output away until half the backlog
// is free, and puts one LostEvent where the first of them stood. The caller
// holds the lock.
func (s *sink) shed() {
	var lost *LostEvent
	out := s.q[:0]
	for _, ev := range s.q {
		n := pieceLen(ev)
		if n < 0 || s.bytes <= s.limit/2 {
			if l, ok := ev.(*LostEvent); ok && lost == nil {
				// An earlier loss that is still waiting: add to it.
				lost = l
			}
			out = append(out, ev)
			continue
		}
		s.bytes -= n
		if lost == nil {
			m := *ev.EventMeta()
			m.Seq = 0
			lost = &LostEvent{Meta: m}
			out = append(out, lost)
		}
		lost.Events++
		lost.Bytes += n
	}
	for i := len(out); i < len(s.q); i++ {
		s.q[i] = nil
	}
	s.q = out
}

// pump hands the waiting events on, one at a time and in order, until the
// sink is closed and empty.
func (s *sink) pump() {
	defer close(s.done)
	if s.ch != nil {
		defer close(s.ch)
	}
	for {
		s.mu.Lock()
		for len(s.q) == 0 && !s.closed {
			s.wake.Wait()
		}
		if len(s.q) == 0 {
			s.mu.Unlock()
			return
		}
		ev := s.q[0]
		s.q[0] = nil
		s.q = s.q[1:]
		if n := pieceLen(ev); n > 0 {
			s.bytes -= n
		}
		s.seq++
		ev.EventMeta().Seq = s.seq
		s.busy = true
		s.mu.Unlock()

		if s.deliver != nil {
			s.deliver(ev)
		} else {
			select {
			case s.ch <- ev:
			case <-s.giveUp:
				return
			}
		}
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}
}

// close ends the stream: nothing more is taken in, and what waits is still
// handed on. With a consumer function it returns once the function has been
// given the last event. With a channel it returns at once; the channel is
// closed when the consumer has read everything, or when nothing has been
// read for a while.
func (s *sink) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	started := s.started
	s.wake.Broadcast()
	s.mu.Unlock()

	if !started {
		return
	}
	if s.deliver != nil {
		<-s.done
		return
	}
	go func() {
		// Give up on a channel that nobody reads, so that the pump does not
		// wait for ever. Each read restarts the wait.
		last := uint64(0)
		for {
			t := time.NewTimer(drainGrace)
			select {
			case <-s.done:
				t.Stop()
				return
			case <-t.C:
			}
			s.mu.Lock()
			seq := s.seq
			s.mu.Unlock()
			if seq == last {
				close(s.giveUp)
				return
			}
			last = seq
		}
	}()
}

// drain waits until every event put in so far has been handed to the
// consumer, or until ctx ends.
func (s *sink) drain(ctx context.Context) error {
	for {
		s.mu.Lock()
		empty := !s.started || (len(s.q) == 0 && !s.busy)
		s.mu.Unlock()
		if empty {
			return nil
		}
		t := time.NewTimer(2 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-s.done:
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

package memjoin

import (
	"sync"

	"github.com/gnosh-ai/gnosh/internal/agent"
)

// notifier puts lines in front of the person, in the order they were
// handed to it, from a goroutine of its own.
//
// It exists because of where the join learns things. A pop is known to be
// in force when the loop has stored the result of the call that made it,
// and the loop tells the join so while it still holds the session's log.
// Telling the person is itself a write to that log. So the line is handed
// over here and goes out as soon as the log is free, right after the
// result it belongs to.
type notifier struct {
	say func(level agent.Level, source, text string)

	mu    sync.Mutex
	queue []notice
	// idle is closed when the goroutine that tells has nothing left and
	// ends. It is nil while none runs.
	idle chan struct{}
}

// notice is one line for the person.
type notice struct {
	level  agent.Level
	source string
	text   string
}

// newNotifier makes a notifier that tells with say. A nil say tells
// nobody.
func newNotifier(say func(level agent.Level, source, text string)) *notifier {
	return &notifier{say: say}
}

// tell hands a line over. It never waits.
func (n *notifier) tell(level agent.Level, source, text string) {
	if n == nil || n.say == nil || text == "" {
		return
	}
	n.mu.Lock()
	n.queue = append(n.queue, notice{level, source, text})
	if n.idle == nil {
		n.idle = make(chan struct{})
		go n.drain(n.idle)
	}
	n.mu.Unlock()
}

// drain tells the lines that wait, in order, and ends when none is left.
func (n *notifier) drain(idle chan struct{}) {
	for {
		n.mu.Lock()
		if len(n.queue) == 0 {
			n.idle = nil
			n.mu.Unlock()
			close(idle)
			return
		}
		next := n.queue[0]
		n.queue = n.queue[1:]
		n.mu.Unlock()
		func() {
			defer func() { _ = recover() }()
			n.say(next.level, next.source, next.text)
		}()
	}
}

// wait returns once every line handed over so far has been told.
func (n *notifier) wait() {
	if n == nil {
		return
	}
	for {
		n.mu.Lock()
		idle := n.idle
		n.mu.Unlock()
		if idle == nil {
			return
		}
		<-idle
	}
}

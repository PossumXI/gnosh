package shelltool

import (
	"fmt"
	"sync"
)

// notifier passes the news that background commands have ended to
// Options.OnEnd: one call at a time, in the order the commands ended, from
// a goroutine of its own, so that a slow callback holds up nothing but the
// next piece of news.
type notifier struct {
	fn func(Ended)
	wg *sync.WaitGroup // the manager's count of its goroutines

	mu     sync.Mutex
	queue  []*job
	busy   bool // a goroutine is working through queue
	closed bool
}

// newNotifier makes a notifier that calls fn, which may be nil.
func newNotifier(fn func(Ended), wg *sync.WaitGroup) *notifier {
	return &notifier{fn: fn, wg: wg}
}

// send queues the news that j has ended. It is called from the goroutine
// that saw j through, which the manager is already counting.
func (n *notifier) send(j *job) {
	if e := j.ending(); n.fn == nil || e == nil || e.reason == endClosed {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return
	}
	n.queue = append(n.queue, j)
	if !n.busy {
		n.busy = true
		n.wg.Add(1)
		go n.work()
	}
}

// work passes on queued news until there is none left.
func (n *notifier) work() {
	defer n.wg.Done()
	for {
		n.mu.Lock()
		if n.closed || len(n.queue) == 0 {
			n.busy = false
			n.mu.Unlock()
			return
		}
		j := n.queue[0]
		n.queue = n.queue[1:]
		n.mu.Unlock()
		// Each end is announced once: if a TaskOutput or a TaskStop has
		// already shown it to the model, there is nothing to add.
		if j.claim() {
			n.fn(j.ended())
		}
	}
}

// close drops whatever is queued. No new call to the callback starts after
// it returns; one already under way finishes on its own.
func (n *notifier) close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.closed = true
	n.queue = nil
}

// ended makes the news of the command's end.
func (j *job) ended() Ended {
	what := oneLine(j.description, 80)
	if what == "" {
		what = firstLine(j.command, 80)
	}
	return Ended{
		Task:    j.task(j.m.opt.Now()),
		Summary: fmt.Sprintf("%s (%s) %s", j.taskID(), what, j.how(j.ending())),
		Tail:    j.out.end(j.m.opt.TailChars),
	}
}

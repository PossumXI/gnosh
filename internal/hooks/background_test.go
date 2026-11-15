package hooks

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// heard collects what the hooks that ran in the background left.
type heard struct {
	mu   sync.Mutex
	got  []Background
	more chan struct{}
}

func newHeard() *heard { return &heard{more: make(chan struct{}, 16)} }

func (h *heard) take(b Background) {
	h.mu.Lock()
	h.got = append(h.got, b)
	h.mu.Unlock()
	h.more <- struct{}{}
}

// next waits for the next hook to end, and fails the test after a while.
func (h *heard) next(t *testing.T) Background {
	t.Helper()
	select {
	case <-h.more:
	case <-time.After(20 * time.Second):
		t.Fatal("no hook that ran in the background ended")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.got[len(h.got)-1]
}

// A hook in the background (spec/cc/hooks.md 7) is started and not waited
// for, decides nothing, and leaves what it adds for the model, or with
// asyncRewake an exit with 2 that wakes it, for when it ends.
func TestBackgroundHooks(t *testing.T) {
	opts := options(t)
	got := newHeard()
	opts.Background = got.take

	// Run does not wait: the hook sleeps, and Run is back at once.
	r := newRunner(t, opts, Hook{Event: PostToolUse, Async: true,
		Command: `sleep 1; echo '{"hookSpecificOutput": {"hookEventName": "PostToolUse", "additionalContext": "lint: 2 warnings"}, "systemMessage": "lint ran"}'`})
	began := time.Now()
	out := run(t, r, input(PostToolUse))
	if took := time.Since(began); took > 700*time.Millisecond {
		t.Errorf("Run waited %v for a hook in the background", took)
	}
	if len(out.Records) != 1 || out.Records[0].Result != ResultBackground || !out.Records[0].Background || out.Blocked {
		t.Errorf("the outcome while the hook runs: %+v", out)
	}
	if s := out.Records[0].String(); !strings.Contains(s, "runs in the background") {
		t.Errorf("the record reads %q", s)
	}
	b := got.next(t)
	if b.Event != PostToolUse || strings.Join(b.Context, "|") != "lint: 2 warnings|lint ran" || b.Wake != "" || b.Warning != "" || b.Record.ExitCode != 0 {
		t.Errorf("what the hook left: %+v", b)
	}

	// A hook in the background decides nothing: exit 2 on PreToolUse
	// blocks no call, and without asyncRewake wakes no one.
	r = newRunner(t, opts, Hook{Event: PreToolUse, Async: true, Command: "echo no >&2; exit 2"})
	if out := run(t, r, input(PreToolUse)); out.Blocked || out.PermissionDecision != "" {
		t.Errorf("a hook in the background decided: %+v", out)
	}
	if b := got.next(t); b.Wake != "" || len(b.Context) != 0 {
		t.Errorf("an async hook's exit 2: %+v", b)
	}

	// asyncRewake: exit 2 wakes the model with stderr, or stdout when
	// stderr is empty.
	r = newRunner(t, opts,
		Hook{Event: Stop, AsyncRewake: true, Command: "echo 'the tests fail' >&2; exit 2"})
	run(t, r, input(Stop))
	if b := got.next(t); b.Wake != "the tests fail" || b.Record.Result != ResultFeedback || !strings.Contains(b.Record.String(), "in the background, gave feedback") {
		t.Errorf("asyncRewake with stderr: %+v, %s", b, b.Record.String())
	}
	r = newRunner(t, opts, Hook{Event: Stop, AsyncRewake: true, Command: "echo 'from stdout'; exit 2"})
	run(t, r, input(Stop))
	if b := got.next(t); b.Wake != "from stdout" {
		t.Errorf("asyncRewake with stdout alone: %+v", b)
	}
	r = newRunner(t, opts, Hook{Event: Stop, AsyncRewake: true, Command: "exit 0"})
	run(t, r, input(Stop))
	if b := got.next(t); b.Wake != "" {
		t.Errorf("asyncRewake that exited 0 woke the model: %+v", b)
	}

	// One that cannot be evaluated is a warning for the person, and
	// nothing for the model.
	r = newRunner(t, opts, Hook{Event: Stop, Async: true, Command: "nosuchprogram", Args: []string{}})
	run(t, r, input(Stop))
	if b := got.next(t); !strings.Contains(b.Warning, "which ran in the background, could not be evaluated") || len(b.Context) != 0 || b.Record.Result != ResultCouldNotEvaluate {
		t.Errorf("a hook in the background that could not run: %+v", b)
	}

	// The waited-for hooks of the same moment are waited for, and decide.
	r = newRunner(t, opts,
		Hook{Event: PreToolUse, Async: true, Command: "sleep 5"},
		Hook{Event: PreToolUse, Command: "echo stop >&2; exit 2"})
	if out := run(t, r, input(PreToolUse)); !out.Blocked || len(out.Records) != 2 || out.Records[1].Result != ResultBackground {
		t.Errorf("a waited-for hook beside one in the background: %+v", out)
	}

	// Close stops what still runs, with nothing left to anyone, and
	// starts nothing after.
	began = time.Now()
	r.Close(10 * time.Second)
	if took := time.Since(began); took > 4*time.Second {
		t.Errorf("Close took %v to stop a sleeping hook", took)
	}
	select {
	case <-got.more:
		t.Errorf("a hook stopped by Close left something: %+v", got.got[len(got.got)-1])
	case <-time.After(300 * time.Millisecond):
	}
	if out := run(t, r, input(PreToolUse)); len(out.Records) != 2 || out.Records[1].Result != ResultCancelled {
		t.Errorf("after Close: %+v", out.Records)
	}
	if left := leftovers(); len(left) != 0 {
		t.Errorf("processes of the hooks left running: %v", left)
	}
}

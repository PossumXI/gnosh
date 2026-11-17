package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// After a turn failed, the empty prompt box says what to try next, as
// Claude Code 2.1.326 does, by the way it failed; a turn that ends any
// other way puts the usual words back.
func TestFailedTurnHint(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor)
	fail := func(kind string) {
		g.ev(&agent.TurnEndEvent{Result: agent.TurnResult{Reason: agent.EndFailed, ErrorKind: kind,
			Err: errors.New("local/qwen3.8-27b is not running: it has not answered once in 1m 30s")}}, idle())
	}
	g.keys(`"go" <Enter>`).start()
	fail("server_error")
	if !strings.Contains(g.region(), `Send "go on" to try again; /doctor checks the model's server`) {
		t.Fatalf("no hint after a failed turn:\n%s", g.region())
	}
	g.keys(`"go on" <Enter>`).start()
	if strings.Contains(g.region(), "try again") {
		t.Errorf("the hint stayed while a turn runs:\n%s", g.region())
	}
	g.ev(turnEnd(agent.EndCompleted), idle())
	if strings.Contains(g.region(), "try again") || !strings.Contains(g.region(), "Ask for something") {
		t.Errorf("the hint stayed after a turn that ended well:\n%s", g.region())
	}
	g.keys(`"more" <Enter>`).start()
	fail("context_too_long")
	if !strings.Contains(g.region(), "/compact makes room") {
		t.Errorf("no hint to make room:\n%s", g.region())
	}
}

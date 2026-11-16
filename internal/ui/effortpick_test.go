package ui

import (
	"slices"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// /effort with nothing after it is a picker of the thinking levels, the
// one in force marked; Alt+T turns thinking over; Alt+P opens the model
// picker and leaves what is typed in the box.
func TestEffortPickerAndKeys(t *testing.T) {
	g := newRig(t, 100, 30, text.NoColor, func(c *controls) {
		c.levels, c.level = []string{"off", "on"}, "off"
		c.models = []app.ModelChoice{{Ref: "local/qwen", Current: true}}
		c.info.Commands = append(c.info.Commands, "effort", "model")
	})
	ran := func() []string {
		g.c.mu.Lock()
		defer g.c.mu.Unlock()
		return slices.Clone(g.c.commands)
	}
	g.keys(`"/effort" <Enter>`)
	g.liveHas("How much is the model to think?", "off", "as it is now", "on", "auto", "the model's own default", "Never mind")
	g.keys(`<Down> <Enter>`)
	if got := ran(); !slices.Equal(got, []string{"/effort on"}) {
		t.Fatalf("the picker ran %q", got)
	}
	// Alt+T: off and on turn over.
	g.keys(`<Alt+t>`)
	if got := ran(); len(got) != 2 || got[1] != "/effort on" {
		t.Fatalf("Alt+T from off ran %q", got)
	}
	g.c.mu.Lock()
	g.c.level = "on"
	g.c.mu.Unlock()
	g.keys(`<Alt+t>`)
	if got := ran(); len(got) != 3 || got[2] != "/effort off" {
		t.Fatalf("Alt+T from on ran %q", got)
	}
	// Alt+P: the model picker, the box as it was.
	g.keys(`"half a prompt" <Alt+p>`)
	g.liveHas("Which model?")
	g.keys(`<Esc>`)
	if g.box() != "half a prompt" {
		t.Fatalf("after Alt+P the box holds %q", g.box())
	}
}

// Ctrl+X Ctrl+K, as Claude Code's: the first press says what a second
// does; a second within three seconds stops the background sub-agents;
// one later than that only says it again.
func TestStopAgentsKey(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor, func(c *controls) { c.agentsRunning = 2 })
	stopped := func() int {
		g.c.mu.Lock()
		defer g.c.mu.Unlock()
		return g.c.agentsStopped
	}
	g.keys(`<Ctrl+X> <Ctrl+K>`)
	g.liveHas("again within 3 seconds stops every sub-agent")
	if stopped() != 0 {
		t.Fatal("one press stopped the sub-agents")
	}
	g.wait(time.Second).keys(`<Ctrl+X> <Ctrl+K>`)
	if stopped() != 1 {
		t.Fatalf("two presses called StopBackgroundAgents %d times", stopped())
	}
	g.once("2 sub-agents that ran in the background are stopped.")
	g.keys(`<Ctrl+X> <Ctrl+K>`).wait(4 * time.Second).keys(`<Ctrl+X> <Ctrl+K>`)
	if stopped() != 1 {
		t.Fatal("two presses four seconds apart stopped the sub-agents")
	}
}

// Alt+T on a dial goes to the next level, and from the last to the first;
// from the model's own default, to the first.
func TestNextThinking(t *testing.T) {
	dial := []string{"low", "medium", "high"}
	for _, c := range []struct {
		levels []string
		level  string
		want   string
	}{
		{[]string{"off", "on"}, "", "off"},
		{[]string{"off", "on"}, "off", "on"},
		{dial, "low", "medium"},
		{dial, "high", "low"},
		{dial, "", "low"},
		{nil, "", ""},
	} {
		if got := nextThinking(c.levels, c.level); got != c.want {
			t.Errorf("nextThinking(%q, %q) = %q, want %q", c.levels, c.level, got, c.want)
		}
	}
}

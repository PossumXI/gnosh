package ui

import (
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// /model with nothing after it is a picker: the models with the one in
// use marked and the cursor on it; a model with thinking levels then asks
// which; what is picked is switched to with /model provider/model level.
// Esc keeps the model, and /model with a name switches as before.
func TestModelPicker(t *testing.T) {
	g := newRig(t, 100, 30, text.NoColor, func(c *controls) {
		c.models = []app.ModelChoice{
			{Ref: "qwen/qwen3.8-27b", Window: 262144, Levels: []string{"off", "on"}, Level: "off", Current: true},
			{Ref: "big/reasoner", Window: 131072, Levels: []string{"low", "medium", "high"}, Level: "high"},
			{Ref: "plain/model"},
		}
		c.info.Commands = append(c.info.Commands, "model")
	})
	ran := func() []string {
		g.c.mu.Lock()
		defer g.c.mu.Unlock()
		return slices.Clone(g.c.commands)
	}
	g.keys(`"/model" <Enter>`)
	g.liveHas("Which model? The choice is saved", "qwen/qwen3.8-27b", "in use · window 262k · thinking off (of off, on)",
		"big/reasoner", "thinking high (of low, medium, high)", "plain/model", "Keep the model as it is")
	if g.s.q == nil {
		t.Fatal("/model opened no picker")
	}
	// Esc keeps the model: nothing runs.
	g.keys(`<Esc>`)
	if g.s.q != nil || len(ran()) != 0 {
		t.Fatalf("Esc left the picker open or ran %q", ran())
	}
	// The second model, and then its thinking: the cursor starts on the
	// level it had.
	g.keys(`"/model" <Enter> <Down> <Enter>`)
	g.liveHas("How is big/reasoner to think?", "what it would use")
	g.keys(`<Enter>`)
	if got := ran(); len(got) != 1 || got[0] != "/model big/reasoner high" {
		t.Fatalf("the picker ran %q", got)
	}
	// A model with no levels is switched to at once.
	g.keys(`"/model" <Enter> <Down> <Down> <Enter>`)
	if got := ran(); len(got) != 2 || got[1] != "/model plain/model" {
		t.Fatalf("the picker ran %q", got)
	}
	// The model in use, at the level it has: nothing to do.
	g.keys(`"/model" <Enter> <Enter> <Enter>`)
	g.liveHas("is the model in use already, thinking off")
	if got := ran(); len(got) != 2 {
		t.Fatalf("picking what is in use ran %q", got)
	}
	// With a name the command runs as it is typed.
	g.keys(`"/model big/reasoner low" <Enter>`)
	if got := ran(); len(got) != 3 || got[2] != "/model big/reasoner low" {
		t.Fatalf("/model with a name ran %q", got)
	}
}

package ui

import (
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// /config with nothing after it is a picker of the options: Enter turns
// one that is true or false over, asks which of a few words, or asks for
// a value, the one set to begin with; each change runs as /config
// key=value. A list is not changed here, and Esc changes nothing.
func TestConfigPicker(t *testing.T) {
	g := newRig(t, 120, 30, text.NoColor, func(c *controls) {
		c.options = []app.ConfigChoice{
			{Key: "verbose", What: "reasoning and whole outputs as they come", Value: "false", Origin: "the default", Takes: "true or false", Choices: []string{"true", "false"}},
			{Key: "mode", What: "how it starts", Value: "default", Origin: "gnosh.json", Takes: "one of default, plan", Choices: []string{"default", "plan"}, Set: true},
			{Key: "deliverablesServer", What: "where deliverables are served", Value: "127.0.0.1:7777", Origin: "the default", Takes: "an address"},
			{Key: "allowedHosts", What: "hosts", Value: "a, b", Origin: "gnosh.json", List: true},
		}
		c.info.Commands = append(c.info.Commands, "config")
	})
	ran := func() []string {
		g.c.mu.Lock()
		defer g.c.mu.Unlock()
		return slices.Clone(g.c.commands)
	}
	g.keys(`"/config" <Enter>`)
	g.liveHas("gnosh's options", "verbose", "false", "reasoning and whole outputs", "deliverablesServer", "127.0.0.1:7777", "Done")
	g.keys(`<Esc>`)
	if len(ran()) != 0 {
		t.Fatalf("Esc ran %q", ran())
	}
	// True or false: turned over at once.
	g.keys(`"/config" <Enter> <Enter>`)
	if got := ran(); len(got) != 1 || got[0] != "/config verbose=true" {
		t.Fatalf("the picker ran %q", got)
	}
	// A few words: which, the cursor on the one in force.
	g.keys(`"/config" <Enter> <Down> <Enter>`)
	g.liveHas("mode: how it starts?", "as it is now")
	g.keys(`<Down> <Enter>`)
	if got := ran(); len(got) != 2 || got[1] != "/config mode=plan" {
		t.Fatalf("the picker ran %q", got)
	}
	// Anything else: typed, and quoted where it holds a blank.
	g.keys(`"/config" <Enter> <Down> <Down> <Enter>`)
	g.liveHas("deliverablesServer takes an address")
	g.keys(`"off" <Enter>`)
	if got := ran(); len(got) != 3 || got[2] != "/config deliverablesServer=off" {
		t.Fatalf("the picker ran %q", got)
	}
	g.keys(`"/config" <Enter> <Down> <Down> <Enter> "a b" <Enter>`)
	if got := ran(); len(got) != 4 || got[3] != `/config deliverablesServer="a b"` {
		t.Fatalf("the picker ran %q", got)
	}
	// A list is set in gnosh.json.
	g.keys(`"/config" <Enter> <Down> <Down> <Down> <Enter>`)
	g.liveHas("holds a list")
	if got := ran(); len(got) != 4 {
		t.Fatalf("a list ran %q", got)
	}
	// With a key the command runs as it is typed.
	g.keys(`"/config verbose" <Enter>`)
	if got := ran(); len(got) != 5 || got[4] != "/config verbose" {
		t.Fatalf("/config verbose ran %q", got)
	}
}

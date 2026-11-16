package ui

import (
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// /mcp with nothing after it lists the servers with how each stands, and
// Enter on one offers what fits: reconnect or disable, or enable for one
// that is disabled; each runs as the command. With no server listed, the
// command runs as before.
func TestMCPPicker(t *testing.T) {
	g := newRig(t, 120, 24, text.NoColor, func(c *controls) {
		c.servers = []app.MCPChoice{
			{Name: "github", Status: "connected", About: "connected (github 1.0; user), 3 tools: issues, prs, search"},
			{Name: "search", Status: "disabled", About: "disabled for this session (user); /mcp enable search connects it"},
		}
		c.info.Commands = append(c.info.Commands, "mcp")
	})
	ran := func() []string {
		g.c.mu.Lock()
		defer g.c.mu.Unlock()
		return slices.Clone(g.c.commands)
	}
	g.keys(`"/mcp" <Enter>`)
	g.liveHas("The MCP servers", "github  connected", "3 tools: issues, prs, search", "search  disabled", "Done")
	g.keys(`<Enter>`)
	g.liveHas("github: connected. What now?", "Reconnect it", "Disable it for this session")
	g.keys(`<Down> <Enter>`)
	g.keys(`"/mcp" <Enter> <Down> <Enter>`)
	g.liveHas("Enable it")
	g.keys(`<Enter>`)
	g.keys(`"/mcp" <Enter> <Esc>`)
	if got := ran(); !slices.Equal(got, []string{"/mcp disable github", "/mcp enable search"}) {
		t.Fatalf("ran %q", got)
	}

	h := newRig(t, 100, 24, text.NoColor)
	h.keys(`"/mcp" <Enter>`)
	if h.s.q != nil {
		t.Fatal("with no servers /mcp opened a picker")
	}
}

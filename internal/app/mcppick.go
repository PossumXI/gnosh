package app

import "strings"

// MCPChoice is an MCP server as the /mcp picker shows it (/mcp with
// nothing after it, as Claude Code's opens its servers).
type MCPChoice struct {
	// Name is the server's, and Status how it stands: "connected",
	// "failed", "disabled" or "pending".
	Name, Status string
	// About says how it stands, as /mcp says it: its tools, or why it is
	// not connected.
	About string
}

// MCPChoices lists the session's MCP servers as they stand now.
func (c *controls) MCPChoices() []MCPChoice { return c.cur().mcpChoices() }

// mcpChoices is SessionControls.MCPChoices.
func (l *live) mcpChoices() []MCPChoice {
	if l.mcp == nil {
		return nil
	}
	l.mcp.mu.Lock()
	defer l.mcp.mu.Unlock()
	out := make([]MCPChoice, 0, len(l.mcp.servers))
	for _, s := range l.mcp.servers {
		out = append(out, MCPChoice{Name: s.cfg.Name, Status: s.status, About: strings.TrimPrefix(l.mcpLine(s), s.cfg.Name+": ")})
	}
	return out
}

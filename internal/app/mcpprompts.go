package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/mcp"
	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// This file is the prompts of MCP servers as commands (spec/cc/mcp.md 5):
// a connected server's prompts are typed as /mcp__<server>__<prompt>, the
// words after the name are the prompt's arguments in order, and what the
// server writes for them goes to the model as a prompt of the user's,
// under a line of gnosh's that says where it came from. They are read
// when the server connects, and listed among the commands with the
// person's skills.

// mcpPromptCommand is one prompt of a connected server, as a command.
type mcpPromptCommand struct {
	name   string // mcp__<server>__<prompt>
	server string
	client *mcp.Client
	prompt mcp.Prompt
}

// mcpPrompts lists the prompts of the connected servers, as commands.
func (l *live) mcpPrompts() []mcpPromptCommand {
	if l.mcp == nil {
		return nil
	}
	l.mcp.mu.Lock()
	defer l.mcp.mu.Unlock()
	var out []mcpPromptCommand
	for _, s := range l.mcp.servers {
		if s.client == nil {
			continue
		}
		for _, p := range s.prompts {
			out = append(out, mcpPromptCommand{name: mcp.ToolName(s.cfg.Name, p.Name), server: s.cfg.Name, client: s.client, prompt: p})
		}
	}
	return out
}

// mcpPromptInfos lists the prompts of the connected servers for a front
// end's list of commands.
func (l *live) mcpPromptInfos() []CommandInfo {
	var out []CommandInfo
	for _, c := range l.mcpPrompts() {
		var args []string
		for _, a := range c.prompt.Arguments {
			if a.Required {
				args = append(args, "<"+a.Name+">")
			} else {
				args = append(args, "["+a.Name+"]")
			}
		}
		about := firstOf(c.prompt.Title, c.prompt.Description, c.prompt.Name)
		out = append(out, CommandInfo{Name: c.name, Args: strings.Join(args, " "), Summary: oneLine("(MCP "+c.server+") "+about, 200), InRun: true})
	}
	return out
}

// typedMCPPrompt runs a line that names a prompt of a connected server,
// and reports false when it names none.
func (l *live) typedMCPPrompt(word, args, line string) (CommandResult, bool, error) {
	if !strings.HasPrefix(word, "mcp__") {
		return CommandResult{}, false, nil
	}
	var cmd *mcpPromptCommand
	for _, c := range l.mcpPrompts() {
		if c.name == word {
			cmd = &c
			break
		}
	}
	if cmd == nil {
		return CommandResult{}, false, nil
	}
	res := CommandResult{Handled: true, Name: "mcp-prompt"}
	// The words after the name are the arguments, in order; the last
	// argument takes whatever words are left.
	words := strings.Fields(args)
	given := map[string]string{}
	var missing []string
	for i, a := range cmd.prompt.Arguments {
		switch {
		case i < len(words) && i == len(cmd.prompt.Arguments)-1:
			given[a.Name] = strings.Join(words[i:], " ")
		case i < len(words):
			given[a.Name] = words[i]
		case a.Required:
			missing = append(missing, a.Name)
		}
	}
	if len(missing) > 0 {
		return res, true, fmt.Errorf("the prompt %s of %s needs %s: type /%s %s", cmd.prompt.Name, cmd.server, strings.Join(missing, ", "), cmd.name, mcpPromptUsage(cmd.prompt))
	}
	if len(cmd.prompt.Arguments) == 0 && len(words) > 0 {
		return res, true, fmt.Errorf("the prompt %s of %s takes no arguments", cmd.prompt.Name, cmd.server)
	}
	ctx, cancel := context.WithTimeout(l.ctx, l.cfg.Limits.MCPTimeout.V)
	defer cancel()
	msgs, err := cmd.client.GetPrompt(ctx, cmd.prompt.Name, given)
	if err != nil {
		return res, true, fmt.Errorf("the prompt %s of %s could not be got: %v", cmd.prompt.Name, cmd.server, err)
	}
	text := mcpPromptText(msgs)
	if strings.TrimSpace(text) == "" {
		return res, true, fmt.Errorf("the prompt %s of %s came to nothing", cmd.prompt.Name, cmd.server)
	}
	l.nameAfter(line)
	lead := fmt.Sprintf("The user ran the prompt %s of the MCP server %s by typing: %s\nWhat follows is what the server wrote for it: take it as the user's request.", cmd.prompt.Name, cmd.server, oneLine(line, 400))
	id, err := l.agent.Submit(agent.Prompt{Hidden: true, Source: "mcp-prompt", Text: prompt.Wrap(lead) + "\n\n" + prompt.Untrusted(text),
		Notice: fmt.Sprintf("The prompt %s of the MCP server %s is in front of the model.", cmd.prompt.Name, cmd.server)})
	if err != nil {
		return res, true, fmt.Errorf("the prompt could not be sent to the model: %v", err)
	}
	res.Prompt = id
	return res, true, nil
}

// mcpPromptUsage writes a prompt's arguments as its command takes them.
func mcpPromptUsage(p mcp.Prompt) string {
	var out []string
	for _, a := range p.Arguments {
		out = append(out, "<"+a.Name+">")
	}
	return strings.Join(out, " ")
}

// mcpPromptText is the text of a prompt's messages, one after another; a
// message the server wrote as the assistant's says so.
func mcpPromptText(msgs []mcp.PromptMessage) string {
	var parts []string
	for _, m := range msgs {
		var text string
		switch c := m.Content; {
		case c.Type == "text":
			text = c.Text
		case c.Type == "resource" && c.Resource != nil && c.Resource.Text != "":
			text = c.Resource.Text
		default:
			text = "[" + firstOf(c.Type, "a part") + " that is not text, left out]"
		}
		if m.Role == "assistant" {
			text = "(Written as the assistant's words:) " + text
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n\n")
}

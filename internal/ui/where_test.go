package ui

import (
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// A shell result begins with a line written for the model, which says
// which machine the command ran on and where it started. A person reads the
// machine in the call's heading: the line itself, and the words that stand
// in for no output, are not shown as output.
func TestWhereACommandRan(t *testing.T) {
	const here = "<host>local (dev@box)</host><cwd>/home/dev/project</cwd>\n"
	const there = "<host>render (root@203.0.113.10)</host><cwd>/root</cwd>\n"
	result := func(body string, code int) core.ToolResult {
		return core.ToolResult{
			Content: []core.Block{core.Text(body)},
			IsError: code != 0,
			Meta:    map[string]any{"exitCode": code},
		}
	}
	each(t, func(t *testing.T, cols int) {
		g := newRig(t, cols, 24, text.NoColor)
		g.keys(`"look around" <Enter>`).start()

		// A command on this machine: its output, and nothing about where.
		args := map[string]any{"command": "ls"}
		g.ev(toolCall("c1", "Bash"), replyEnd(core.StopToolUse), toolStart("c1", "Bash", args),
			toolEnd("c1", "Bash", args, result(here+"a.txt\nb.txt\n", 0), 2*time.Second))
		g.once("$ ls", "a.txt", "b.txt").committedLacks("<host>", "<cwd>", "dev@box", "[local]")

		// The model may name this machine; the heading still has no label.
		args = map[string]any{"command": "pwd", "host": "local"}
		g.ev(toolStart("c2", "Bash", args),
			toolEnd("c2", "Bash", args, result(here+"/home/dev/project\n", 0), 0))
		g.once("$ pwd").committedLacks("[local]", "<host>")

		// A command on another machine leads with the machine's name.
		args = map[string]any{"command": "uptime", "host": "render"}
		g.ev(toolStart("c3", "Bash", args),
			toolEnd("c3", "Bash", args, result(there+"up 24 days\n", 0), 0))
		g.once("[render] $ uptime", "up 24 days").committedLacks("<host>", "<cwd>", "203.0.113.10")

		// Nothing printed: how it ended stands beside the command, here
		// and there alike.
		args = map[string]any{"command": "mkdir -p out"}
		g.ev(toolStart("c4", "Bash", args),
			toolEnd("c4", "Bash", args, result(here+"(no output)", 0), 0))
		g.once("$ mkdir -p out · exit 0")
		args = map[string]any{"command": "touch done", "host": "render"}
		g.ev(toolStart("c5", "Bash", args),
			toolEnd("c5", "Bash", args, result(there+"(no output)", 0), 0))
		g.once("[render] $ touch done · exit 0").committedLacks("(no output)")

		// A command that failed without a word says only that it failed.
		args = map[string]any{"command": "false"}
		g.ev(toolStart("c6", "Bash", args),
			toolEnd("c6", "Bash", args, result(here+"(no output)\n[exit code 1]", 1), 0))
		g.once("$ false").committedHas("failed · exit 1").committedLacks("(no output)", "[exit code 1]", "<host>")

		// A command that could not reach its machine has no directory in
		// its first line, and what went wrong is shown.
		args = map[string]any{"command": "uptime", "host": "render"}
		g.ev(toolStart("c7", "Bash", args),
			toolEnd("c7", "Bash", args, core.ToolResult{
				Content: []core.Block{core.Text("<host>render (root@203.0.113.10)</host>\n[not run: render refused the connection]")},
				IsError: true,
				Meta:    map[string]any{"reason": "no_connection"},
			}, 0))
		g.committedHas("not run: render refused the connection").committedLacks("<host>")
		g.show("where commands ran")
	})
}

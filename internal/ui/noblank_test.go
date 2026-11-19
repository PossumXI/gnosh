package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// No empty row reaches the scrollback from a call or from gnosh's own
// lines (Jae, 14 November 2026: "in general don't print empty lines"):
// the blank lines of a command's output, of a heredoc, of a failure and of
// a notice are left out (the TUI review of 2026-11-19). The model's own
// paragraphs keep theirs.
func TestNoEmptyRowsFromCallsAndNotices(t *testing.T) {
	g := newRig(t, 80, 40, text.NoColor)
	g.keys(`"show the log" <Enter>`).start()
	heredoc := map[string]any{"command": "cat <<'EOF'\nfirst\n\nsecond\nEOF"}
	logCmd := map[string]any{"command": "git log -2"}
	g.ev(toolCall("c1", "Bash"), toolStart("c1", "Bash", heredoc),
		toolEnd("c1", "Bash", heredoc, core.ToolResult{Content: []core.Block{core.Text("first\n\nsecond\n")}, Meta: map[string]any{"exitCode": 0}}, 0))
	g.ev(toolCall("c2", "Bash"), toolStart("c2", "Bash", logCmd),
		toolEnd("c2", "Bash", logCmd, core.ToolResult{Content: []core.Block{core.Text("commit aaa\n\n    the first\n\ncommit bbb\n\n    the second\n")}, Meta: map[string]any{"exitCode": 0}}, 2*time.Second))
	g.ev(toolCall("c3", "Read"), toolStart("c3", "Read", map[string]any{"file_path": "/x"}),
		toolEnd("c3", "Read", map[string]any{"file_path": "/x"}, core.ToolResult{Content: []core.Block{core.Text("error: no such file\n\nhint: look again")}, IsError: true}, 0))
	g.ev(&agent.NoticeEvent{Level: agent.LevelWarning, Source: "loop", Text: "one thing\n\nand another"})
	g.ev(&agent.TextEvent{Text: "A paragraph.\n\nAnother paragraph.\n"}, turnEnd(agent.EndCompleted))
	var blank []int
	in := false
	for i, row := range g.committed[g.from:] {
		if strings.Contains(row, "A paragraph.") {
			in = true // the answer's own blank line stays
		}
		if !in && strings.TrimSpace(row) == "" {
			blank = append(blank, i)
		}
	}
	if len(blank) > 0 {
		t.Errorf("empty rows %v in the scrollback:\n%s", blank, strings.Join(g.committed[g.from:], "\n"))
	}
	// (Left out, the blank lines no longer take the rows of the tail that
	// is shown: the first commit is there too.)
	g.committedHas("commit aaa", "the second", "error: no such file", "hint: look again", "and another", "Another paragraph.")
}

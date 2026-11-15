package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// The side panel: /panel takes the whole screen, with the conversation at
// the left and, at the right, the files the session changed and its
// deliverables with their addresses; PageUp and the wheel scroll back;
// /panel again gives the screen back, with what was said meanwhile in
// the scrollback. A terminal too narrow for it keeps the screen as it is.
func TestSidePanel(t *testing.T) {
	g := newRig(t, 100, 24, text.NoColor, func(c *controls) {
		c.deliv = []app.Deliverable{{Label: "demo video", Path: "/root/out/demo.mp4", URL: "http://127.0.0.1:7777/tok123/demo.mp4"}}
	})
	g.free = true
	edit := map[string]any{"file_path": "/work/proj/main.go"}
	diff := &core.Display{Kind: "diff", Path: "/work/proj/main.go", Before: "a\n", After: "a\nb\nc\n"}
	g.ev(toolStart("c1", "Edit", edit), toolEnd("c1", "Edit", edit, core.ToolResult{Content: []core.Block{core.Text("edited")}, Display: diff}, time.Second))
	g.s.selectKey = selectKey("Apple_Terminal")
	g.keys(`"/panel" <Enter>`)
	if !g.scr.AltScreenEver() || !g.scr.MouseEver() {
		t.Fatalf("/panel did not take the whole screen")
	}
	screen := strings.Join(g.scr.Lines(), "\n")
	for _, want := range []string{"│ Files changed", "main.go +2 -0", "│ Deliverables", "demo video", "http://127.0.0.1:7777/tok", "/root/out/demo.mp4", "The side panel is on", "hold Fn to"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the screen does not show %q:\n%s", want, screen)
		}
	}

	// What is said goes to the left part, and scrolls back.
	for i := 0; i < 40; i++ {
		g.ev(&agent.NoticeEvent{Level: agent.LevelInfo, Source: "test", Text: fmt.Sprintf("notice %d", i)})
	}
	if screen = strings.Join(g.scr.Lines(), "\n"); !strings.Contains(screen, "notice 39") {
		t.Errorf("the last notice is not on the screen:\n%s", screen)
	}
	g.keys(`<PageUp>`)
	if screen = strings.Join(g.scr.Lines(), "\n"); !strings.Contains(screen, "more rows below") || strings.Contains(screen, "notice 39") {
		t.Errorf("PageUp did not scroll back:\n%s", screen)
	}
	for range 10 {
		g.s.input(term.Mouse{Action: term.MouseWheelDown})
	}
	g.frame()
	if screen = strings.Join(g.scr.Lines(), "\n"); strings.Contains(screen, "more rows below") || !strings.Contains(screen, "notice 39") {
		t.Errorf("the wheel did not scroll forward again:\n%s", screen)
	}

	// /panel again: the screen is given back, and what was said meanwhile
	// is in the scrollback.
	g.keys(`"/panel" <Enter>`)
	all := strings.Join(g.all(), "\n")
	if !strings.Contains(all, "notice 0") || !strings.Contains(all, "notice 39") || !strings.Contains(all, "The side panel is off") {
		t.Errorf("after /panel off, the scrollback lacks what was said:\n%s", all)
	}
	if g.scr.Mode(1000) {
		t.Errorf("the mouse is still reported")
	}

	// /clear: a new conversation, with no files changed yet.
	g.keys(`"/clear" <Enter>`)
	if len(g.s.changed) != 0 {
		t.Errorf("after /clear the panel still lists %d changed files", len(g.s.changed))
	}

	// Each terminal's own key; Shift, with the others named, where the
	// terminal does not say which it is.
	if selectKey("iTerm.app") != "Option" || selectKey("tmux") != "" || selectKey("") != "" {
		t.Errorf("selectKey: iTerm2 %q, tmux %q, none %q", selectKey("iTerm.app"), selectKey("tmux"), selectKey(""))
	}

	// Too narrow: nothing changes but a word.
	n := newRig(t, 60, 20, text.NoColor)
	n.keys(`"/panel" <Enter>`)
	if n.scr.AltScreenEver() || !strings.Contains(strings.Join(n.scr.Lines(), "\n"), "needs a terminal at least 72 columns wide") {
		t.Errorf("a narrow terminal: %q", n.scr.Lines())
	}
}

package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// memTool stands for the tool that brings the part about mem into the
// system prompt.
type memTool struct{}

func (memTool) Spec() core.ToolSpec {
	return core.ToolSpec{Name: "gnovm_run", Description: "Runs Gno.", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}
}

func (memTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	return core.TextResult("ran"), nil
}

// The config says which of gnosh's two texts about mem a model is given:
// the lean one unless the option asks for the full one.
func TestMemoryTextOption(t *testing.T) {
	feature := Feature{
		Name:  "mem-stand-in",
		Tools: []ToolName{{Name: "gnovm_run", Quiet: true}},
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			return &Contribution{Tools: []core.Tool{memTool{}}}, nil
		},
	}
	system := func(lines ...string) string {
		w := apptest.NewWorld(t, apptest.Reply{Text: "ok"})
		w.RC(lines...)
		o := testOptions(w, "-p", "hello")
		o.Features = []Feature{feature}
		runWith(t, t.Context(), o).wantCode(t, 0)
		for _, m := range w.Model.Requests()[0].Messages() {
			if m["role"] == "system" {
				return apptest.Text(m)
			}
		}
		t.Fatal("the request has no system prompt")
		return ""
	}
	tools := []core.ToolSpec{{Name: "gnovm_run"}}
	lean := prompt.Builtin().System(prompt.SystemInput{Tools: tools})[1]
	full := prompt.Builtin().System(prompt.SystemInput{Tools: tools, Memory: prompt.MemoryFull})[1]
	if lean == full {
		t.Fatal("the two texts about mem are the same text")
	}
	for name, tt := range map[string]struct {
		lines     []string
		want, not string
	}{
		"nothing set":     {nil, lean, full},
		"lean asked for":  {[]string{"option memory-text lean"}, lean, full},
		"full asked for":  {[]string{"option memory-text full"}, full, lean},
		"the later wins":  {[]string{"option memory-text full", "option memory-text lean"}, lean, full},
		"reset to unset":  {[]string{"option memory-text full", "option reset memory-text"}, lean, full},
		"full, and hosts": {[]string{"option memory-text full", "host add render --ssh root@203.0.113.7"}, full, lean},
	} {
		got := system(tt.lines...)
		if !strings.Contains(got, tt.want) || strings.Contains(got, tt.not) {
			t.Errorf("%s: the system prompt holds the wrong text about mem:\n%s", name, clip(got))
		}
	}
	// A value that is neither is a mistake, like any other.
	w := apptest.NewWorld(t)
	w.RC("option memory-text long")
	has(t, "stderr", run(t, w, "-p", "hello").wantCode(t, 1).err, "memory-text", "lean, full")
}

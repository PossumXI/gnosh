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
	system := func(settings ...string) string {
		w := apptest.NewWorld(t, apptest.Reply{Text: "ok"})
		args := []string{"-p", "hello"}
		// The first is the person's own file; a second is given for the
		// run, and so is the stronger of the two.
		if len(settings) > 0 {
			w.Settings(settings[0])
		}
		if len(settings) > 1 {
			args = []string{"--settings", settings[1], "-p", "hello"}
		}
		o := testOptions(w, args...)
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
		settings  []string
		want, not string
	}{
		"nothing set":       {nil, lean, full},
		"lean asked for":    {[]string{`{"options": {"memoryText": "lean"}}`}, lean, full},
		"full asked for":    {[]string{`{"options": {"memoryText": "full"}}`}, full, lean},
		"the stronger wins": {[]string{`{"options": {"memoryText": "full"}}`, `{"options": {"memoryText": "lean"}}`}, lean, full},
		"put back by null":  {[]string{`{"options": {"memoryText": "full"}}`, `{"options": {"memoryText": null}}`}, lean, full},
		"full, and hosts":   {[]string{`{"options": {"memoryText": "full"}, "hosts": {"render": {"ssh": "root@203.0.113.7"}}}`}, full, lean},
	} {
		got := system(tt.settings...)
		if !strings.Contains(got, tt.want) || strings.Contains(got, tt.not) {
			t.Errorf("%s: the system prompt holds the wrong text about mem:\n%s", name, clip(got))
		}
	}
	// A value that is neither is a mistake, like any other.
	w := apptest.NewWorld(t)
	w.Settings(`{"options": {"memoryText": "long"}}`)
	has(t, "stderr", run(t, w, "-p", "hello").wantCode(t, 1).err, "options.memoryText", `"lean", "full"`)
}

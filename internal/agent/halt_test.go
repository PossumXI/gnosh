package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// A session start that says stop (a SessionStart hook's "continue": false)
// refuses the prompt it stands before and every one after it, with its
// reason, and nothing goes to the model.
func TestSessionStartHalt(t *testing.T) {
	r := newRig(t, func(o *Options) {
		o.Moments.SessionStart = func(context.Context, StartInfo) ([]Part, error) {
			return nil, &StopWork{Reason: "the license has run out"}
		}
	})
	for _, p := range []string{"first", "second"} {
		res := r.run(p)
		if res.Reason != EndRefused || res.Detail != "the license has run out" {
			t.Errorf("%s prompt: %+v", p, res)
		}
	}
	if n := r.model.count(); n != 0 {
		t.Errorf("the model was asked %d times", n)
	}
}

// The end of a summary made in a turn that says stop (a PostCompact
// hook's "continue": false) ends that turn, once what it gave back is kept.
func TestAfterSummaryHalt(t *testing.T) {
	j := &job{want: 14}
	r := newRig(t, func(o *Options) {
		o.Model.Info.ContextWindow = 8000
		o.System = []string{"You are gnosh."}
		o.Tools = []core.Tool{fileTool(600)}
		o.Moments.AfterSummary = func(context.Context, SummaryInfo) ([]Part, error) {
			return []Part{{Source: "files", Text: "kept after the summary"}}, &StopWork{Reason: "review the summary first"}
		}
	})
	r.model.honest = true
	r.model.window = 8000
	r.model.answer(j.respond)
	res := r.run("read fourteen files")
	if res.Reason != EndHalted || !strings.Contains(res.Detail, "review the summary first") {
		t.Fatalf("result = %+v", res)
	}
	if j.summaryCount() == 0 {
		t.Fatal("no summary was made")
	}
	r.checkLog()
}

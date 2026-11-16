package app

import (
	"context"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// The picker's choices: every model the server has, the one in use marked
// with its window, and after a switch the new one marked.
func TestModelChoices(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Model.SetModels("test-model", "other-model")
	var before, after []ModelChoice
	_, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		before = c.ModelChoices(ctx)
		if _, err := c.Command(ctx, "/model local/other-model"); err != nil {
			t.Errorf("/model: %v", err)
			return 1
		}
		after = c.ModelChoices(ctx)
		return 0
	})
	if r.code != 0 {
		t.Fatalf("exit code %d", r.code)
	}
	current := func(choices []ModelChoice) string {
		for _, ch := range choices {
			if ch.Current {
				return ch.Ref
			}
		}
		return ""
	}
	if len(before) != 2 || before[0].Ref != "local/test-model" || before[1].Ref != "local/other-model" {
		t.Fatalf("the choices: %+v", before)
	}
	// A model not in use offers the level a switch to it would take.
	if before[1].Level != "on" || len(before[1].Levels) != 2 {
		t.Errorf("the other model's thinking: %q of %q", before[1].Level, before[1].Levels)
	}
	if current(before) != "local/test-model" || current(after) != "local/other-model" {
		t.Errorf("in use before %q and after the switch %q", current(before), current(after))
	}
}

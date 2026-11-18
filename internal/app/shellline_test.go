package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// The person's command lines behind "!" are in the prompt history, with
// the "!", in the session that ran them and in the folder's later ones.
// With respondToBashCommands off, the model does not answer one when it
// ends: the command and what it printed go to the model with the next
// prompt.
func TestShellLinesInTheHistoryAndUnanswered(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "It printed a greeting."})
	w.Settings(`{"options": {"respondToBashCommands": false}}`)
	var during []string
	front, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		if err := c.Shell("echo hello from the person"); err != nil {
			t.Errorf("Shell: %v", err)
			return 1
		}
		find(f, "the command's end", func(e *agent.ToolEndEvent) bool { return true })
		l := c.(*controls).cur()
		deadline := time.Now().Add(patience)
		for l.agent.State().Notes == 0 {
			if time.Now().After(deadline) {
				t.Error("the command and its output were not put in front of the model")
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
		during = c.History()
		if _, err := c.Submit(agent.Prompt{Text: "what did it say?"}); err != nil {
			return 1
		}
		_ = c.Wait(ctx)
		c.Quit("prompt_input_exit")
		return 0
	})
	r.wantCode(t, 0)
	if len(w.Model.Requests()) == 0 {
		t.Fatal("the model was never asked")
	}
	// The first request is the prompt's: the command had none of its own.
	first := strings.Join(userTexts(w.Model.Requests()[0]), "\n")
	has(t, "what the model was handed with the prompt", first, "$ echo hello from the person", "hello from the person", "what did it say?")
	for _, ev := range front.events {
		if e, ok := ev.(*agent.PromptEvent); ok && e.Prompt.Source == "shell" {
			t.Errorf("the command went to the model as a prompt of its own: %+v", e.Prompt)
		}
	}
	if got := strings.Join(during, " | "); got != "!echo hello from the person" {
		t.Errorf("the history after the command: %q", got)
	}

	// A later session of the folder.
	var later []string
	_, r = attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		later = c.History()
		c.Quit("prompt_input_exit")
		return 0
	})
	r.wantCode(t, 0)
	if got := strings.Join(later, " | "); got != "!echo hello from the person | what did it say?" {
		t.Errorf("the history of a later session: %q", got)
	}
}

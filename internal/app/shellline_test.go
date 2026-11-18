package app

import (
	"context"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// The person's command lines behind "!" are in the prompt history, with
// the "!", in the session that ran them and in the folder's later ones; a
// prompt sent with a blank before its "!" stays a prompt there. With
// respondToBashCommands off, the model does not answer a command when it
// ends: the command and what it printed are put in the conversation at
// once, before the next prompt, which the model then answers.
func TestShellLinesInTheHistoryAndUnanswered(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "It printed a greeting."}, apptest.Reply{Text: "Words, not a command."})
	w.Settings(`{"options": {"respondToBashCommands": false}}`)
	var during []string
	front, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		if err := c.Shell("echo hello from the person"); err != nil {
			t.Errorf("Shell: %v", err)
			return 1
		}
		find(f, "the command and its output in the conversation", func(e *agent.NoteEvent) bool { return e.Note.Source == "shell" })
		during = c.History()
		if _, err := c.Submit(agent.Prompt{Text: "what did it say?"}); err != nil {
			return 1
		}
		_ = c.Wait(ctx)
		if _, err := c.Submit(agent.Prompt{Text: " !not a command"}); err != nil {
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
	// The first request is the prompt's: the command had none of its own,
	// and what it printed comes before the prompt.
	first := strings.Join(userTexts(w.Model.Requests()[0]), "\n")
	has(t, "what the model was handed with the prompt", first, "$ echo hello from the person", "hello from the person", "what did it say?")
	if out, asked := strings.Index(first, "$ echo hello from the person"), strings.Index(first, "what did it say?"); out < 0 || asked < out {
		t.Errorf("the command's output is not before the prompt:\n%s", first)
	}
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
	if got := strings.Join(later, " | "); got != "!echo hello from the person | what did it say? |  !not a command" {
		t.Errorf("the history of a later session: %q", got)
	}
}

// A command's output that waits for the next prompt is in the session's
// file at once: a session opened again before that prompt still has it.
func TestShellOutputKeptAcrossARestart(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "It said hello."})
	w.Settings(`{"options": {"respondToBashCommands": false}}`)
	_, r := attach(t, w, nil, func(ctx context.Context, c SessionControls, f *scripted) int {
		if err := c.Shell("echo kept for later"); err != nil {
			t.Errorf("Shell: %v", err)
			return 1
		}
		find(f, "the command and its output in the conversation", func(e *agent.NoteEvent) bool { return e.Note.Source == "shell" })
		c.Quit("prompt_input_exit")
		return 0
	})
	r.wantCode(t, 0)
	run(t, w, "-c", "-p", "what did my command print?").wantCode(t, 0)
	if reqs := w.Model.Requests(); len(reqs) != 1 {
		t.Fatalf("the model was asked %d times, want once", len(reqs))
	}
	has(t, "what the resumed session sent", strings.Join(userTexts(w.Model.Requests()[0]), "\n"), "$ echo kept for later", "what did my command print?")
}

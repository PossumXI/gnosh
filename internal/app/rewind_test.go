package app

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// /rewind lists the person's prompts and takes the conversation back to
// before one of them: the model is then sent what came before it and
// nothing after, the session is the same one, and a later run that
// continues it reads the conversation as it was left.
func TestRewindInThePlainSession(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Text: "First answer."}, apptest.Reply{Text: "Second answer."}, apptest.Reply{Text: "Third answer."},
		apptest.Reply{Text: "The answer to the changed prompt."}, apptest.Reply{Text: "Continued."})
	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	for i, p := range []string{"the first prompt", "the second prompt", "the third prompt"} {
		s.typeLine(p)
		s.see([]string{"First answer.", "Second answer.", "Third answer."}[i])
		s.see("-- done in ")
	}
	s.typeLine("/rewind")
	s.see("Your prompts in this conversation:")
	s.see("the first prompt")
	s.see("the third prompt")
	s.see("/rewind N takes the conversation back to before prompt N")
	s.see("Files are not restored")
	s.typeLine("/rewind 9")
	s.see("is not the number of one of your prompts")
	s.typeLine("/rewind 2")
	s.see("The conversation is back to before this prompt of yours")
	s.see("the second prompt")
	s.see("Only the conversation goes back.")

	s.typeLine("the second prompt, changed")
	s.see("The answer to the changed prompt.")
	reqs := w.Model.Requests()
	sent := string(reqs[len(reqs)-1].Body)
	has(t, "what the model was sent after the rewind", sent, "the first prompt", "First answer.", "the second prompt, changed")
	hasNot(t, "what the model was sent after the rewind", sent, "Second answer.", "the third prompt", "Third answer.")
	// The facts that go with a session's first prompt are not given a
	// second time: the first prompt is still in the conversation.
	if n := strings.Count(sent, "Working directory"); n > 1 {
		t.Errorf("the environment was given %d times", n)
	}
	s.typeLine("/rewind")
	s.see("  2. ")
	s.see("the second prompt, changed")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}
	if sessions := w.Sessions(); len(sessions) != 1 {
		t.Fatalf("session files: %v", sessions)
	}
	checkSession(t, w, w.Sessions()[0])

	// Continued in a new process, it is the conversation as it was left.
	run(t, w, "-c", "-p", "go on").wantCode(t, 0)
	reqs = w.Model.Requests()
	sent = string(reqs[len(reqs)-1].Body)
	has(t, "what a later run was sent", sent, "the first prompt", "the second prompt, changed", "The answer to the changed prompt.", "go on")
	hasNot(t, "what a later run was sent", sent, "Second answer.", "Third answer.")
}

// Back to before the very first prompt: the conversation is empty again,
// and the next prompt is a first prompt, with the facts that go with one.
func TestRewindToTheFirstPrompt(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "First answer."}, apptest.Reply{Text: "Afresh."})
	w.Write("AGENTS.md", "Always answer in one sentence.\n")
	s := sit(t, w, "--trust-folder")
	s.see("Type a prompt and Enter.")
	s.typeLine("the first prompt")
	s.see("First answer.")
	s.see("-- done in ")
	s.typeLine("/rewind 1")
	s.see("The conversation is back to before this prompt of yours")
	s.typeLine("another beginning")
	s.see("Afresh.")
	reqs := w.Model.Requests()
	sent := strings.Join(userTexts(reqs[len(reqs)-1]), "\n")
	has(t, "what the model was sent", sent, "another beginning", "Always answer in one sentence.")
	hasNot(t, "what the model was sent", string(reqs[len(reqs)-1].Body), "the first prompt", "First answer.")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}
	checkSession(t, w, w.Sessions()[0])
}

// Through the controls, as an interface does it: the points, the prompt's
// text back, a session that must be idle, and the features told that the
// session is opened again.
func TestRewindThroughTheControls(t *testing.T) {
	hold := make(chan struct{})
	w := apptest.NewWorld(t, apptest.Reply{Text: "One."}, apptest.Reply{Text: "Two."},
		apptest.Reply{Text: "Held.", Hold: hold}, apptest.Reply{Text: "After the rewind."})
	var mu sync.Mutex
	var opened []string
	feature := Feature{
		Name: "watcher",
		Session: func(ctx context.Context, s *Session) (*Contribution, error) {
			mu.Lock()
			opened = append(opened, s.Source)
			mu.Unlock()
			return &Contribution{}, nil
		},
	}
	var points []RewindPoint
	var text string
	var busyErr, goneErr error
	_, r := attach(t, w, func(o *Options) { o.Features = []Feature{feature} }, func(ctx context.Context, c SessionControls, f *scripted) int {
		for _, p := range []string{"prompt one", "prompt two"} {
			if _, err := c.Submit(agent.Prompt{Text: p}); err != nil {
				return 1
			}
			_ = c.Wait(ctx)
		}
		points = c.RewindPoints()
		// Not while a turn runs.
		third, _ := c.Submit(agent.Prompt{Text: "prompt three"})
		find(f, "the third prompt going out", func(e *agent.PromptEvent) bool { return e.ID == third })
		_, busyErr = c.Rewind(ctx, points[1].ID)
		close(hold)
		_ = c.Wait(ctx)

		_, goneErr = c.Rewind(ctx, "no-such-entry")
		var err error
		if text, err = c.Rewind(ctx, points[1].ID); err != nil {
			t.Errorf("Rewind: %v", err)
			return 1
		}
		if got := c.RewindPoints(); len(got) != 1 || got[0].Text != "prompt one" {
			t.Errorf("the points after the rewind: %+v", got)
		}
		if _, err := c.Submit(agent.Prompt{Text: "prompt two, again"}); err != nil {
			t.Errorf("Submit after the rewind: %v", err)
		}
		_ = c.Wait(ctx)
		c.Quit("prompt_input_exit")
		return 0
	})
	r.wantCode(t, 0)
	if len(points) != 2 || points[0].Text != "prompt one" || points[1].Text != "prompt two" || points[0].ID == "" || points[1].Time.IsZero() {
		t.Errorf("the points: %+v", points)
	}
	if text != "prompt two" {
		t.Errorf("the rewind gave back %q", text)
	}
	if busyErr == nil || !strings.Contains(busyErr.Error(), "a turn is running") {
		t.Errorf("a rewind while a turn ran: %v", busyErr)
	}
	if goneErr == nil {
		t.Errorf("a rewind to a prompt that is not there did not fail")
	}
	if got := strings.Join(opened, ","); got != "startup,resume" {
		t.Errorf("the feature was told of sessions opened as %q", got)
	}
	reqs := w.Model.Requests()
	sent := string(reqs[len(reqs)-1].Body)
	has(t, "what the model was sent after the rewind", sent, "prompt one", "One.", "prompt two, again")
	hasNot(t, "what the model was sent after the rewind", sent, "Two.", "prompt three", "Held.")
	checkSession(t, w, w.Sessions()[0])
}

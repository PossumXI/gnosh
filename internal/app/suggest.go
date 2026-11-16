package app

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/permission"
)

// This file is Claude Code's prompt suggestions (promptSuggestionEnabled):
// after a turn of a session a person attends, the session's model is
// asked, beside the session and keeping nothing, what the person would
// most likely send next, and the interface shows it in the empty box,
// where Tab or the right arrow takes it and typing drops it. The option is
// off unless set, as Claude Code has it off for other providers' models:
// each suggestion is a request to the model, and a prompt sent while one
// is being made would wait for it, so a prompt sent stops it.

// SuggestionEvent is the person's likely next prompt, for the interface
// to show in the empty box until they type.
type SuggestionEvent struct {
	Text string
}

// How a suggestion is asked for: a few words, soon, or not at all.
const (
	suggestTokens = 64
	suggestWait   = 60 * time.Second
	suggestChars  = 200
)

// suggester is the suggestion being made, if one is.
type suggester struct {
	mu     sync.Mutex
	cancel context.CancelFunc
}

// start puts a new suggestion in the place of the one being made, if one
// is, and returns the context its request goes by. It is in place before
// the request is sent, so that a prompt sent at once stops it too.
func (s *suggester) start(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = cancel
	return ctx, cancel
}

// stop stops the suggestion being made, if one is: a prompt was sent.
func (s *suggester) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// suggestNext asks for the person's next prompt once a turn has ended,
// when the option is on: in a session a person attends through a front
// end that shows it (Suggester), out of plan mode, after a turn that
// ended as it should and with nothing waiting to go next.
func (l *live) suggestNext(res agent.TurnResult) {
	if !l.suggestOn.Load() || !l.interactive || res.Reason != agent.EndCompleted || l.engine.Mode() == permission.ModePlan {
		return
	}
	if shows, ok := l.front.(Suggester); !ok || !shows.ShowsSuggestions() {
		return
	}
	a := l.agentReady.Load()
	if a == nil || a.State().Queued > 0 {
		return
	}
	ctx, cancel := l.suggest.start(l.ctx)
	l.background(func(context.Context) {
		defer cancel()
		out, err := a.Side(ctx, agent.SideRequest{Purpose: "suggestion", Session: true, Prompt: l.texts.NextPrompt(), MaxTokens: suggestTokens, Timeout: suggestWait})
		if err != nil || ctx.Err() != nil || a.State().Busy {
			return
		}
		if text := suggestionText(out.Text); text != "" {
			l.deliver(&SuggestionEvent{Text: text})
		}
	})
}

// suggestionText is the first line of what the model wrote, without the
// quotes and marks a model puts around such a line, cut short; "" when
// it wrote nothing to offer.
func suggestionText(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "\"'`*>“”‘’ ")
		if line == "" {
			continue
		}
		if utf8.RuneCountInString(line) > suggestChars {
			r := []rune(line)
			line = strings.TrimSpace(string(r[:suggestChars])) + "…"
		}
		return line
	}
	return ""
}

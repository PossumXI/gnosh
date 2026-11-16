package app

import (
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// The end of a session's conversation is shown as it opens again when it
// has one and was written to longer ago than a moment: not after /restart
// or a rewind, which leave it on the screen.
func TestShowPastOf(t *testing.T) {
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	at := func(e transcript.Entry, ago time.Duration) transcript.Entry {
		e.Timestamp = now.Add(-ago)
		return e
	}
	talked := []transcript.Entry{
		at(transcript.PromptEntry("hi"), time.Hour),
		at(transcript.AssistantEntry(core.AssistantText("Hi."), core.StopEndTurn), time.Hour),
	}
	if !showPastOf(talked, now) {
		t.Error("a session last written an hour ago shows nothing of its past")
	}
	if showPastOf(append(talked, at(transcript.MetaEntry("k", "v"), 5*time.Second)), now) {
		t.Error("a session written to moments ago shows its past again")
	}
	if showPastOf([]transcript.Entry{at(transcript.MetaEntry("k", "v"), time.Hour)}, now) || showPastOf(nil, now) {
		t.Error("a session with no prompt shows a past")
	}
}

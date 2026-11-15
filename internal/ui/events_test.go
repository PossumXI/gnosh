package ui

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
)

// Under a prompt, the interface says what each path it mentioned with @
// brought, from the Source of the mention's part.
func TestMentionsSaid(t *testing.T) {
	parts := []agent.Part{
		{Source: "environment"},
		{Source: "mention:file:notes.txt"},
		{Source: "instructions"},
		{Source: "mention:dir:docs"},
		{Source: "mention:denied:.env"},
		{Source: "mention:failed:big.log"},
		{Source: "mention:odd:x"},
	}
	want := "@notes.txt read · @docs listed · @.env denied by a rule · @big.log could not be read"
	if got := mentionsSaid(parts); got != want {
		t.Errorf("mentionsSaid = %q, want %q", got, want)
	}
	if got := mentionsSaid(nil); got != "" {
		t.Errorf("with no mentions: %q", got)
	}
}

package ui

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
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
		{Source: "mention:agent:reviewer"},
		{Source: "mention:odd:x"},
	}
	want := "@notes.txt read · @docs listed · @.env denied by a rule · @big.log could not be read · @agent-reviewer asked for"
	if got := mentionsSaid(parts); got != want {
		t.Errorf("mentionsSaid = %q, want %q", got, want)
	}
	if got := mentionsSaid(nil); got != "" {
		t.Errorf("with no mentions: %q", got)
	}
}

// kindsOnly is a session's controls that can only name kinds of sub-agent.
type kindsOnly struct{ app.Controls }

func (kindsOnly) AgentKinds() []string { return []string{"general-purpose", "Explore", "reviewer"} }

// After an @, the kinds of sub-agent are offered as @agent-<name>, for
// what was typed that "agent-" and a name begin with.
func TestCompleteAgent(t *testing.T) {
	s := &session{c: kindsOnly{}}
	labels := func(typed string) string {
		var out []string
		for _, c := range s.completeAgent(typed, 4) {
			if c.Start != 4 || c.Text != c.Label+" " {
				t.Errorf("%q: candidate %+v", typed, c)
			}
			out = append(out, c.Label)
		}
		return strings.Join(out, " ")
	}
	for typed, want := range map[string]string{
		"a":         "@agent-general-purpose @agent-Explore @agent-reviewer",
		"agent-":    "@agent-general-purpose @agent-Explore @agent-reviewer",
		"agent-rev": "@agent-reviewer",
		"agent-z":   "",
		"src/":      "",
		"":          "",
	} {
		if got := labels(typed); got != want {
			t.Errorf("after @%s: %q, want %q", typed, got, want)
		}
	}
	if got := (&session{}).completeAgent("a", 0); got != nil {
		t.Errorf("controls that cannot name kinds: %+v", got)
	}
}

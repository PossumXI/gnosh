package ui

import (
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
)

// This file is Claude Code's removal of invisible characters from a
// prompt: as Enter sends it, the characters a terminal draws as nothing
// (text.CleanInvisible) are taken out of what was typed and of every
// paste in it, so that a prompt cannot carry words the person never saw.
// When what was taken out could hide text or change its order, or when
// what is left begins as a command or a shell line would and what was
// typed did not, nothing is sent: the prompt is back in the box to be
// read, and Enter sends it as it now stands.

// cleanDraft returns the draft with the invisible characters taken out of
// what was typed and of each paste, the pastes kept where they stand.
func cleanDraft(d editor.Draft) (editor.Draft, text.Invisible) {
	var found text.Invisible
	var b strings.Builder
	var pastes []editor.Span
	add := func(s string) {
		c, f := text.CleanInvisible(s)
		found.Removed += f.Removed
		found.Hiding = found.Hiding || f.Hiding
		b.WriteString(c)
	}
	at := 0
	for _, sp := range d.Pastes {
		add(d.Text[at:sp.Start])
		start := b.Len()
		add(d.Text[sp.Start:sp.End])
		if b.Len() > start {
			pastes = append(pastes, editor.Span{Start: start, End: b.Len()})
		}
		at = sp.End
	}
	add(d.Text[at:])
	if found.Removed == 0 {
		return d, found
	}
	return editor.Draft{Text: b.String(), Pastes: pastes}, found
}

// cleanToSend takes the invisible characters out of a draft Enter is
// sending as mode, and reports whether it goes. When it does not, the
// person was told why and the cleaned draft is back in the box.
func (s *session) cleanToSend(d *editor.Draft, mode editor.Mode) bool {
	clean, found := cleanDraft(*d)
	if found.Removed == 0 {
		return true
	}
	what := count(found.Removed, "invisible character", "invisible characters")
	lead := strings.TrimLeft(clean.Text, " \t\n")
	switch {
	case strings.TrimSpace(clean.Text) == "":
		s.flash("The prompt held only invisible characters, so nothing was sent.")
		return false
	case found.Hiding:
		s.restore(clean)
		s.notice(agent.LevelWarning, "Took out "+what+" that could hide text or reorder it; nothing was sent.\nRead the prompt in the box; Enter sends it.")
		return false
	case mode == editor.ModePrompt && (strings.HasPrefix(lead, "/") || strings.HasPrefix(lead, "!")):
		s.restore(clean)
		s.notice(agent.LevelWarning, "Took out "+what+", and what is left begins as a command; nothing was sent.\nRead it in the box; Enter runs it.")
		return false
	}
	*d = clean
	s.notice(agent.LevelInfo, "Took out "+what+" before sending the prompt.")
	return true
}

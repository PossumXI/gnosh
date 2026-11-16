package ui

import (
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// /skills with nothing after it lists the skills with how far each is
// seen; Enter on one asks how far, the cursor on the setting in force,
// and the choice runs /skills <name> <how>. With no skill, the command
// runs as before.
func TestSkillsPicker(t *testing.T) {
	g := newRig(t, 140, 30, text.NoColor, func(c *controls) {
		c.skills = []app.SkillChoice{
			{Name: "deploy", Description: "Deploys the site.", Visibility: "on", Who: "you type /deploy, and the model may load it"},
			{Name: "notes", Description: "Writes notes.", Visibility: "off", Who: "switched off in your settings"},
		}
		c.info.Commands = append(c.info.Commands, "skills")
	})
	ran := func() []string {
		g.c.mu.Lock()
		defer g.c.mu.Unlock()
		return slices.Clone(g.c.commands)
	}
	g.keys(`"/skills" <Enter>`)
	g.liveHas("The skills.", "/deploy  on", "Deploys the site.", "/notes  off", "Done")
	g.keys(`<Enter>`)
	g.liveHas("How far is /deploy to be seen?", "name-only", "user-invocable-only", "as it is now")
	g.keys(`<Down> <Down> <Down> <Enter>`)
	g.keys(`"/skills" <Enter> <Down> <Enter> <Up> <Up> <Up> <Enter>`)
	if got := ran(); !slices.Equal(got, []string{"/skills deploy off", "/skills notes on"}) {
		t.Fatalf("ran %q", got)
	}

	h := newRig(t, 100, 24, text.NoColor)
	h.keys(`"/skills" <Enter>`)
	if h.s.q != nil {
		t.Fatal("with no skills /skills opened a picker")
	}
}

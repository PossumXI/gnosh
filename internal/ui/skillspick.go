package ui

import (
	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
	"github.com/gnosh-ai/gnosh/internal/ui/menu"
)

// /skills with nothing after it is a picker of the skills, as Claude
// Code's /skills is: each with how far it is seen and who may start it;
// Enter on one asks how far it is to be seen (Claude Code's skillOverrides:
// on, name-only, user-invocable-only, off), which runs the command /skills
// <name> <how>, kept in the project's local settings file.

// skillsLister is a session whose controls list the skills
// (app.SessionControls).
type skillsLister interface {
	SkillChoices() []app.SkillChoice
}

// skillsWords are the lines that open the picker.
var skillsWords = map[string]bool{"/skills": true}

// skillVisibilities are the values a skill can be set to, in the order
// they are offered, with what each does.
var skillVisibilities = []struct{ key, about string }{
	{"on", "as its file says"},
	{"name-only", "the model is told its name alone"},
	{"user-invocable-only", "yours alone to start"},
	{"off", "nobody may start it"},
}

// chooseSkills opens the picker. With no skill, the command runs as
// before and says how to make one.
func (s *session) chooseSkills(line string) bool {
	sl, ok := s.c.(skillsLister)
	if !ok {
		return false
	}
	skills := sl.SkillChoices()
	if len(skills) == 0 {
		return false
	}
	s.ed.AddHistory(editor.Draft{Text: line})
	var opts []menu.Option
	for _, sk := range skills {
		opts = append(opts, menu.Option{Label: "/" + sk.Name + "  " + sk.Visibility, Description: oneLine(sk.Who + s.th.Sep + sk.Description)})
	}
	opts = append(opts, menu.Option{Label: "Done"})
	title := "The skills. Enter on one sets how far it is seen; it is kept for this project."
	pick := &question{q: agent.Question{Kind: "skills", Title: title}}
	pick.ui = menu.NewSelect(menu.SelectOptions{Title: title, Options: opts, CancelHint: "done", Theme: s.th.Menu, Metrics: s.m})
	pick.local = func(res menu.Result) {
		if res.Cancelled || res.Index < 0 || res.Index >= len(skills) {
			return
		}
		s.visibilityMenu(skills[res.Index])
	}
	s.openLocal(pick)
	return true
}

// visibilityMenu asks how far the skill picked is to be seen.
func (s *session) visibilityMenu(sk app.SkillChoice) {
	var opts []menu.Option
	sel := 0
	for i, v := range skillVisibilities {
		o := menu.Option{Label: v.key, Description: v.about}
		if v.key == sk.Visibility {
			o.Description += s.th.Sep + "as it is now"
			sel = i
		}
		opts = append(opts, o)
	}
	opts = append(opts, menu.Option{Label: "Never mind"})
	title := "How far is /" + sk.Name + " to be seen?"
	pick := &question{q: agent.Question{Kind: "skills", Title: title}}
	pick.ui = menu.NewSelect(menu.SelectOptions{Title: title, Options: opts, Selected: sel, CancelHint: "never mind", Theme: s.th.Menu, Metrics: s.m})
	pick.local = func(res menu.Result) {
		if res.Cancelled || res.Index < 0 || res.Index >= len(skillVisibilities) {
			return
		}
		v := skillVisibilities[res.Index].key
		if v == sk.Visibility {
			s.flash("/" + sk.Name + " is " + v + " already.")
			return
		}
		s.run("/skills " + sk.Name + " " + v)
	}
	s.openLocal(pick)
}

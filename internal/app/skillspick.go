package app

// SkillChoice is a skill as the /skills picker lists it.
type SkillChoice struct {
	Name, Description, Source string
	// Visibility is how far the person's settings let it be seen
	// (skillOverrides): "on", "name-only", "user-invocable-only" or
	// "off".
	Visibility string
	// Who says who may start it, as /skills says.
	Who string
}

// SkillChoices lists the skills for the picker that /skills with nothing
// after it opens; how far one is seen is set with the command /skills
// <name> on|name-only|user-invocable-only|off.
func (c *controls) SkillChoices() []SkillChoice { return c.cur().skillChoices() }

// skillChoices is SessionControls.SkillChoices.
func (l *live) skillChoices() []SkillChoice {
	if l.skillsOff() {
		return nil
	}
	var out []SkillChoice
	for _, sk := range l.skills().All() {
		out = append(out, SkillChoice{Name: sk.Name, Description: sk.Description, Source: sk.Source, Visibility: firstOf(sk.Over, "on"), Who: l.skillWho(sk)})
	}
	return out
}

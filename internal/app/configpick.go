package app

// ConfigChoice is one of gnosh's options as the /config picker shows it
// (/config with nothing after it, as Claude Code's opens its settings).
type ConfigChoice struct {
	// Key is the option's name, What what it is for, Value the value in
	// force as text, and Origin where that comes from, in words.
	Key, What, Value, Origin string
	// Set says something other than the default set it.
	Set bool
	// Takes says what values it takes, and Choices lists them where they
	// are few: true and false, or a few words.
	Takes   string
	Choices []string
	// List marks an option that holds a list, which /config does not
	// change.
	List bool
}

// ConfigChoices lists gnosh's options as /config does: as the session
// started, with what /config has put in force since.
func (c *controls) ConfigChoices() []ConfigChoice { return c.cur().configChoices() }

// configChoices is SessionControls.ConfigChoices.
func (l *live) configChoices() []ConfigChoice {
	rows := l.optionRows()
	out := make([]ConfigChoice, 0, len(rows))
	for _, r := range rows {
		out = append(out, ConfigChoice{
			Key: r.Key, What: r.What, Value: r.Text, Origin: l.originText(r), Set: r.Set,
			Takes: r.Takes, Choices: r.Choices, List: r.List,
		})
	}
	return out
}

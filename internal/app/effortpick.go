package app

// Effort returns the thinking levels of the model in use and the level it
// thinks at ("" for its own default), for the /effort picker and Alt+T.
// It asks no server.
func (c *controls) Effort() (levels []string, level string) {
	l := c.cur()
	if l.agent == nil {
		return nil, ""
	}
	m := l.agent.Model()
	return append([]string(nil), m.Levels...), m.Level
}

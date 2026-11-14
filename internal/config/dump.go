package config

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

// String describes the whole configuration, one setting to a line, each
// with where it came from. It is what /status and gnosh doctor can print,
// and it is safe to log: no key, header value, variable value or password
// is in it.
func (c *Config) String() string {
	if c == nil {
		return "no configuration"
	}
	var b strings.Builder
	line := func(format string, args ...any) {
		fmt.Fprintf(&b, format, args...)
		b.WriteByte('\n')
	}
	from := func(o Origin) string { return "  [" + o.String() + "]" }

	line("directories:")
	line("  config   %s", c.Paths.ConfigDir)
	line("  state    %s", c.Paths.StateDir)
	line("  cache    %s", c.Paths.CacheDir)
	line("  project  %s", c.Paths.ProjectRoot)
	line("  data     %s", c.Paths.DataDir)
	line("files read:")
	for _, f := range c.Files {
		note := ""
		if f.Held {
			note = "  (held: the folder is not trusted)"
		}
		line("  %-8s %-12s %s%s", f.Role, f.Scope, f.Path, note)
	}
	if len(c.Files) == 0 {
		line("  none")
	}

	line("providers:")
	for _, p := range c.Providers {
		state := "usable"
		switch {
		case p.Disabled:
			state = "disabled"
		case p.LeftOut != "":
			state = "left out: " + p.LeftOut
		}
		line("  %s (%s) %q: %s%s", p.ID, p.Type, p.Name, state, from(p.Origin))
		for _, e := range p.Endpoints {
			where := "elsewhere"
			if e.Local {
				where = "local"
			}
			line("    %s (%s; first piece %s, between pieces %s)", e.Display, where, limit(e.Timeouts.FirstToken), limit(e.Timeouts.Idle))
		}
		switch {
		case p.KeyFrom != "":
			line("    key: from %s", p.KeyFrom)
		case !p.APIKey.Empty():
			line("    key: set")
		default:
			line("    key: none")
		}
		for _, h := range p.Headers {
			line("    header %s: %s", h.Name, h.Value)
		}
		if len(p.ExtraBody) > 0 {
			line("    extra body fields: %s", list(slices.Sorted(maps.Keys(p.ExtraBody))))
		}
		line("    discover models: %v", p.DiscoverModels.V)
	}
	if len(c.Providers) == 0 {
		line("  none")
	}

	line("models:")
	for _, m := range c.Models {
		window := "ask the server"
		if m.ContextWindow.Set {
			window = fmt.Sprint(m.ContextWindow.V)
		}
		line("  %s  window %s%s", m.Ref(), window, from(m.Origin))
	}
	for _, sl := range []struct {
		name string
		s    Slot
	}{{"large", c.Large}, {"small", c.Small}} {
		if sl.s.Model.Set {
			line("  %s: %s%s  thinking %s%s", sl.name, sl.s.Model.V, from(sl.s.Model.Origin), sl.s.Thinking.V, from(sl.s.Thinking.Origin))
		} else {
			line("  %s: not chosen", sl.name)
		}
	}

	line("options:")
	for _, o := range c.Options.All() {
		line("  %-28s %s%s", o.Key, o.Text, from(o.Origin))
	}

	line("permissions:")
	line("  mode %s%s", c.Permissions.Mode.V, from(c.Permissions.Mode.Origin))
	for _, l := range c.Permissions.Layers {
		for _, set := range []struct {
			name  string
			rules []Rule
		}{{"deny", l.Deny}, {"ask", l.Ask}, {"allow", l.Allow}} {
			for _, r := range set.rules {
				line("  %-5s %s%s", set.name, r.Text, from(r.Origin))
			}
		}
	}
	for _, d := range c.Permissions.Dirs {
		line("  directory %s%s", d.V, from(d.Origin))
	}
	if c.Permissions.Tools.Set {
		line("  only these tools: %s%s", list(c.Permissions.Tools.V), from(c.Permissions.Tools.Origin))
	}

	line("hooks:")
	if c.HooksOff != "" {
		line("  off: %s", c.HooksOff)
	}
	for _, h := range c.AllHooks {
		matcher := ""
		if h.Matcher != "" {
			matcher = " [" + h.Matcher + "]"
		}
		line("  %s%s: %s%s", h.Event, matcher, h.Command, from(h.Origin))
	}
	if len(c.AllHooks) == 0 {
		line("  none")
	}

	if len(c.Hosts) > 0 {
		line("hosts:")
		for _, h := range c.Hosts {
			about := ""
			if h.Description != "" {
				about = ": " + h.Description
			}
			line("  %s → %s%s%s", h.Name, h.SSH, about, from(h.Origin))
		}
	}
	if len(c.MCP) > 0 {
		line("mcp servers:")
		for _, m := range c.MCP {
			what := m.Command
			if m.Type != "stdio" {
				what = displayURL(m.URL)
			}
			line("  %s (%s) %s%s", m.Name, m.Type, what, from(m.Origin))
		}
	}
	if len(c.LSP) > 0 {
		line("language servers:")
		for _, l := range c.LSP {
			line("  %s: %s%s", l.Name, l.Command, from(l.Origin))
		}
	}
	if len(c.Env) > 0 {
		line("variables for commands (values not shown):")
		for _, v := range c.Env {
			line("  %s%s", v.Name, from(v.Origin))
		}
	}

	line("context files:")
	if c.Context.Off {
		line("  off (safe mode)")
	} else {
		line("  in each directory down to %s: %s", c.Context.Cwd, list(slices.Concat(c.Context.ProjectNames, c.Context.LocalNames)))
		for _, g := range c.Context.Global {
			line("  global %s", g.V)
		}
		for _, e := range c.Context.Extra {
			line("  also %s%s", e.V, from(e.Origin))
		}
	}

	if !c.Trusted && len(c.Held) > 0 {
		line("held until the folder is trusted:")
		for _, h := range c.Held {
			line("  %s%s", fmt.Sprintf("%q", h.Text), from(h.Origin))
		}
	}
	return c.secrets.scrub(b.String())
}

// limit shows a time limit, or "no limit".
func limit(d time.Duration) string {
	if d == 0 {
		return "no limit"
	}
	return d.String()
}

// OptionValue is one option as /status shows it.
type OptionValue struct {
	Key string
	// Text is the value in force, as text. A secret shows as "[hidden]".
	Text string
	// Set reports whether something other than the default set it.
	Set    bool
	Origin Origin
}

// All lists every option with its value in force and where it came from,
// in the order of the docs.
func (o *Options) All() []OptionValue {
	out := make([]OptionValue, 0, len(optionSpecs))
	for i := range optionSpecs {
		spec := &optionSpecs[i]
		v := OptionValue{Key: spec.key}
		switch {
		case spec.truth != nil:
			x := spec.truth(o)
			v.Text, v.Set, v.Origin = fmt.Sprint(x.V), x.Set, x.Origin
		case spec.num != nil:
			x := spec.num(o)
			v.Text, v.Set, v.Origin = fmt.Sprint(x.V), x.Set, x.Origin
		case spec.text != nil:
			x := spec.text(o)
			v.Text, v.Set, v.Origin = x.V, x.Set, x.Origin
			if spec.kind == oURL && x.Set {
				// An address can carry a password; it is shown without.
				v.Text = displayURL(x.V)
			}
		case spec.secret != nil:
			x := spec.secret(o)
			v.Text, v.Set, v.Origin = x.V.String(), x.Set, x.Origin
		case spec.list != nil:
			items := *spec.list(o)
			vals := make([]string, len(items))
			for j, it := range items {
				vals[j] = it.V
			}
			v.Text, v.Set = list(vals), len(items) > 0
			if len(items) > 0 {
				v.Origin = items[len(items)-1].Origin
			}
		}
		if !v.Set {
			v.Text = spec.def
		}
		out = append(out, v)
	}
	return out
}

// HooksJSON returns the hooks in force as a hooks block in Claude Code's
// shape (event, then matcher groups, then command handlers), for a hook
// runner that reads that shape. Timeouts are in seconds.
func (c *Config) HooksJSON() []byte {
	type handler struct {
		Type    string  `json:"type"`
		Command string  `json:"command"`
		Timeout float64 `json:"timeout,omitempty"`
	}
	type group struct {
		Matcher string    `json:"matcher,omitempty"`
		Hooks   []handler `json:"hooks"`
	}
	block := map[string][]group{}
	for _, h := range c.Hooks {
		block[h.Event] = append(block[h.Event], group{Matcher: h.Matcher, Hooks: []handler{{Type: "command", Command: h.Command, Timeout: h.Timeout.Seconds()}}})
	}
	data, err := json.Marshal(block)
	if err != nil {
		return []byte("{}")
	}
	return data
}

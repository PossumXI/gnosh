package app

import "github.com/gnosh-ai/gnosh/internal/permission"

// PermissionRule is a rule in force, as the /permissions picker lists it,
// in the order the engine looks at them: deny, then ask, then allow.
type PermissionRule struct {
	// List is "deny", "ask" or "allow", and Rule the rule as the engine
	// writes it.
	List, Rule string
	// Source is where it comes from: a settings file, a flag, or the
	// session's own approvals.
	Source string
	// Held says it waits for the folder to be trusted.
	Held bool
	// Removable says /permissions remove takes it out: it is in the
	// project's local settings file, which gnosh writes, or among the
	// session's own approvals.
	Removable bool
}

// PermissionRules lists the rules in force, and the permission mode, for
// the picker that /permissions with nothing after it opens; adding and
// taking out a rule are the commands /permissions allow, ask, deny and
// remove.
func (c *controls) PermissionRules() (mode string, rules []PermissionRule) {
	return c.cur().permissionRules()
}

// permissionRules is SessionControls.PermissionRules.
func (l *live) permissionRules() (string, []PermissionRule) {
	x := l.engine.Explain()
	var out []PermissionRule
	for _, list := range []permission.List{permission.ListDeny, permission.ListAsk, permission.ListAllow} {
		for _, r := range x.Rules {
			if r.List != list {
				continue
			}
			out = append(out, PermissionRule{
				List: string(r.List), Rule: r.Rule, Source: r.Source, Held: r.Held,
				Removable: r.Source == permission.KeptLayer || r.Source == permission.SessionLayer || r.Source == l.cfg.Paths.LocalSettings,
			})
		}
	}
	return string(x.Mode), out
}

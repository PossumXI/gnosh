package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/sandbox"
)

// slashSandbox is Claude Code's /sandbox: how the sandbox stands (its
// panel's Config and Dependencies), and, with a word after it, the choice
// of its Mode and Overrides tabs, saved to the project's local settings
// as Claude Code saves them: auto (the sandbox on, its commands run
// without asking), regular (on, each asking as it would without it), off,
// strict (the model cannot ask a command out of it) and loose (it can).
// The choice is in force from the session's next start (/restart).
func (l *live) slashSandbox(ctx context.Context, c *SlashContext) error {
	arg := strings.ToLower(strings.TrimSpace(c.Args))
	if arg == "" {
		c.Say("%s", l.sandboxReport())
		return nil
	}
	set := map[string]map[string]bool{
		"auto":    {"enabled": true, "autoAllowBashIfSandboxed": true},
		"regular": {"enabled": true, "autoAllowBashIfSandboxed": false},
		"off":     {"enabled": false},
		"strict":  {"allowUnsandboxedCommands": false},
		"loose":   {"allowUnsandboxedCommands": true},
	}[arg]
	if set == nil {
		return fmt.Errorf("/sandbox takes auto, regular, off, strict or loose, or nothing to say how it stands; %q is none of them", arg)
	}
	err := l.editLocalSettings(func(doc map[string]json.RawMessage) error {
		sb := map[string]json.RawMessage{}
		if raw, ok := doc["sandbox"]; ok {
			if err := json.Unmarshal(raw, &sb); err != nil {
				return fmt.Errorf("its \"sandbox\" is not an object")
			}
		}
		for k, v := range set {
			sb[k], _ = json.Marshal(v)
		}
		raw, err := json.Marshal(sb)
		doc["sandbox"] = raw
		return err
	})
	if err != nil {
		return err
	}
	c.Say("Saved to %s; it is in force from the session's next start (/restart).", l.cfg.Paths.LocalSettings)
	return nil
}

// sandboxReport says how the sandbox stands, in a few lines.
func (l *live) sandboxReport() string {
	sc := l.cfg.Sandbox
	st := sandbox.Check()
	state := l.st.sandboxOf(l.cfg)
	var b strings.Builder
	switch {
	case !sc.Enabled:
		b.WriteString("The sandbox is off: the shell's commands run as they are, each asking as the rules and the mode say. /sandbox auto turns it on for this project, with its commands running without asking; /sandbox regular, asking as before.")
		if !st.OK {
			fmt.Fprintf(&b, "\nIt could not run here as things are: %s.", st.Why)
		}
		return b.String()
	case !state.on:
		fmt.Fprintf(&b, "The sandbox is on (%s) and cannot run here: %s. The shell's commands run outside it, asking as they would without it.", sc.Origin, state.why)
		return b.String()
	}
	fmt.Fprintf(&b, "The sandbox is on (%s): the shell's commands on this machine run in it, under %s.", sc.Origin, st.Tool)
	if sc.AutoAllow {
		b.WriteString("\nMode: auto-allow: a command that runs in it runs without asking, after the deny and ask rules and gnosh's own checks.")
	} else {
		b.WriteString("\nMode: regular permissions: each command asks as it would without the sandbox.")
	}
	if sc.AllowUnsandboxed {
		b.WriteString("\nA command that failed because of it may be run again outside it (dangerouslyDisableSandbox), judged as any command is. /sandbox strict takes that away.")
	} else {
		b.WriteString("\nStrict: the model cannot ask a command out of it. /sandbox loose gives that back.")
	}
	roots := l.roots
	p := l.sandboxPolicy(roots, "", nil)
	fmt.Fprintf(&b, "\nWrites: %s, and a /tmp of its own.", l.shownList(p.Write))
	var guarded []string
	for _, r := range p.ReadOnly {
		if _, err := os.Lstat(r); err == nil && l.insideAny(r, p.Write) && !slices.ContainsFunc(guarded, func(g string) bool { return r == g || strings.HasPrefix(r, g+"/") }) {
			guarded = append(guarded, r)
		}
	}
	if len(guarded) > 0 {
		fmt.Fprintf(&b, "\nRead-only within them: %s.", l.shownList(guarded))
	}
	if len(p.DenyRead) > 0 {
		fmt.Fprintf(&b, "\nNot to be read: %s.", l.shownList(p.DenyRead))
		if len(p.AllowRead) > 0 {
			fmt.Fprintf(&b, " Read after all: %s.", l.shownList(p.AllowRead))
		}
	}
	if len(sc.Excluded) > 0 {
		fmt.Fprintf(&b, "\nRun outside it (excludedCommands): %s.", strings.Join(listValues(sc.Excluded), ", "))
	}
	b.WriteString("\nNetwork: through gnosh's proxy")
	if hosts := listValues(sc.AllowedDomains); len(hosts) > 0 {
		fmt.Fprintf(&b, "; %s let through at once", strings.Join(hosts, ", "))
	}
	if denied := listValues(sc.DeniedDomains); len(denied) > 0 {
		fmt.Fprintf(&b, "; never %s", strings.Join(denied, ", "))
	}
	if sc.Strict {
		b.WriteString("; any other host is refused (strictAllowlist), but those WebFetch(domain:...) allow rules name.")
	} else {
		b.WriteString("; WebFetch(domain:...) allow rules let hosts through too, and any other host is asked about (Yes for the session, or kept as such a rule).")
	}
	return b.String()
}

// shownList is paths as the person is shown them, whole, the home folder
// as ~, joined.
func (l *live) shownList(paths []string) string {
	home := l.cfg.Paths.Home
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
			p = "~" + p[len(home):]
		}
		out = append(out, p)
	}
	return strings.Join(out, ", ")
}

// insideAny reports whether path is inside one of the folders.
func (l *live) insideAny(path string, dirs []string) bool {
	for _, d := range dirs {
		if path != d && strings.HasPrefix(path, strings.TrimSuffix(d, "/")+"/") {
			return true
		}
	}
	return false
}

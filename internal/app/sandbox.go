package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/permission"
	"github.com/gnosh-ai/gnosh/internal/sandbox"
	"github.com/gnosh-ai/gnosh/internal/tools/shelltool"
)

// This file joins the sandbox settings (config.Sandbox) to the shell tool
// and the permission engine (spec/DECISIONS.md 353): a command the shell
// runs on this machine runs in the operating system's sandbox, unless
// excludedCommands names it or the model asked out of it, and with
// autoAllowBashIfSandboxed such a command needs no approval.

// sandboxOn reports whether the sandbox is on and can run here.
func sandboxOn(cfg *config.Config) bool {
	return cfg.Sandbox.Enabled && sandbox.Check().OK
}

// sandboxOutside is the test, shared by the shell tool and the engine, of
// a command that runs outside the sandbox: one the settings exclude, or,
// with asked, one the model asked out of it, where the settings let it.
func sandboxOutside(sc config.Sandbox) func(command string, asked bool) bool {
	return func(command string, asked bool) bool {
		return asked && sc.AllowUnsandboxed || excludedCall(sc.Excluded, command)
	}
}

// sandboxPermission is what the permission engine is told of the sandbox.
func sandboxPermission(cfg *config.Config) permission.Sandbox {
	if !sandboxOn(cfg) {
		return permission.Sandbox{}
	}
	return permission.Sandbox{On: true, AutoAllow: cfg.Sandbox.AutoAllow, Outside: sandboxOutside(cfg.Sandbox)}
}

// shellSandbox is the shell tool's sandbox, or nil when it is off or
// cannot run here (the person was told at startup, sandboxNotice).
func (l *live) shellSandbox() *shelltool.Sandbox {
	sc := l.cfg.Sandbox
	if !sandboxOn(l.cfg) {
		return nil
	}
	temp, err := sandbox.PrivateTemp(os.TempDir())
	if err != nil {
		l.notice(agent.LevelWarning, "sandbox", "The sandbox has no temporary folder ("+err.Error()+"), so commands run outside it, asking as they would without it.")
		return nil
	}
	return &shelltool.Sandbox{
		Wrap: func(dir string, roots, argv []string, expose []sandbox.Exposed) ([]string, []string, error) {
			p := l.sandboxPolicy(roots, temp, expose)
			out, err := sandbox.Wrap(p, dir, argv)
			return out, sandbox.Env(p), err
		},
		Outside:     sandboxOutside(sc),
		Unsandboxed: sc.AllowUnsandboxed,
		Writes:      "the session's folders",
	}
}

// sandboxPolicy is what a sandboxed command may do: write in the session's
// folders and those the settings add, but not in the settings files gnosh
// reads, a repository's hooks or git settings, or what the settings keep
// read-only; read all but what the settings deny.
func (l *live) sandboxPolicy(roots []string, temp string, expose []sandbox.Exposed) sandbox.Policy {
	sc := l.cfg.Sandbox
	p := sandbox.Policy{Temp: temp, Expose: expose}
	p.Write = append(append(p.Write, roots...), listValues(sc.AllowWrite)...)
	for _, root := range roots {
		for _, name := range sandboxGuarded {
			p.ReadOnly = append(p.ReadOnly, filepath.Join(root, name))
		}
	}
	p.ReadOnly = append(append(p.ReadOnly, settingsFiles(l.cfg)...), listValues(sc.DenyWrite)...)
	p.DenyRead, p.AllowRead = listValues(sc.DenyRead), listValues(sc.AllowRead)
	return p
}

// sandboxGuarded are the paths in each of the session's folders that a
// sandboxed command may not change, as the permission engine asks before
// any command changes them: settings files that would widen what runs
// next, a repository's hooks and git settings, which run outside the
// sandbox the next time git does.
var sandboxGuarded = []string{"gnosh.json", ".gnosh/settings.json", ".gnosh/settings.local.json", ".claude/settings.json",
	".claude/settings.local.json", ".mcp.json", ".lsp.json", ".git/hooks", ".git/config"}

// excludedCall reports whether excludedCommands takes a command out of the
// sandbox: a single plain command (no list, pipe, redirection, command
// substitution, subshell or variable: Claude Code keeps those in it) that
// fits one of the entries as the content of a Bash(...) rule fits: "docker"
// is that command alone, "docker *" it with any arguments too, and a * in
// the middle any text.
func excludedCall(entries []config.ListItem, command string) bool {
	c := strings.TrimSpace(command)
	if c == "" || len(entries) == 0 || strings.ContainsAny(c, ";&|<>()$`{}\n\\") {
		return false
	}
	words := strings.Fields(c)
	switch words[0] {
	case "sudo", "eval", "xargs", "cd", "pushd", "popd", "exec", "env", "command", "builtin":
		return false
	case "git":
		// A clone or a new work tree writes a whole tree of files where its
		// path points: one outside the folder stays in the sandbox.
		for _, w := range words[1:] {
			if strings.HasPrefix(w, "/") || strings.HasPrefix(w, "~") || strings.Contains(w, "..") {
				return false
			}
		}
	}
	c = strings.Join(words, " ")
	for _, e := range entries {
		if ruleContentFits(strings.TrimSpace(e.V), c) {
			return true
		}
	}
	return false
}

// ruleContentFits reports whether a command fits a Bash rule's content:
// without a *, exactly; ending in " *", that prefix alone or with
// arguments; otherwise with each * standing for any text.
func ruleContentFits(pattern, command string) bool {
	switch {
	case !strings.Contains(pattern, "*"):
		return command == pattern
	case strings.HasSuffix(pattern, " *") && !strings.Contains(pattern[:len(pattern)-2], "*"):
		prefix := pattern[:len(pattern)-2]
		return command == prefix || strings.HasPrefix(command, prefix+" ")
	}
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(command, parts[0]) {
		return false
	}
	rest := command[len(parts[0]):]
	for i, part := range parts[1:] {
		last := i == len(parts)-2
		switch {
		case last:
			return strings.HasSuffix(rest, part)
		default:
			at := strings.Index(rest, part)
			if at < 0 {
				return false
			}
			rest = rest[at+len(part):]
		}
	}
	return true
}

// sandboxNotice is what the person is told at startup of a sandbox the
// settings turn on that cannot run here, and the error that stops gnosh
// when failIfUnavailable says to.
func sandboxNotice(cfg *config.Config) (notice string, stop error) {
	if !cfg.Sandbox.Enabled {
		return "", nil
	}
	st := sandbox.Check()
	if st.OK {
		return "", nil
	}
	if cfg.Sandbox.FailIfUnavailable {
		return "", &sandboxError{why: st.Why, origin: cfg.Sandbox.Origin.String()}
	}
	return "The sandbox is on (" + cfg.Sandbox.Origin.String() + ") and cannot run here: " + st.Why + ". Commands run outside it, asking as they would without it.", nil
}

// sandboxError stops gnosh where the sandbox must run and cannot.
type sandboxError struct{ why, origin string }

func (e *sandboxError) Error() string {
	return "the sandbox is on (" + e.origin + "), with failIfUnavailable, and cannot run here: " + e.why
}

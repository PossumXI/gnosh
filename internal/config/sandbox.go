package config

import (
	"slices"
	"strconv"
	"strings"
)

// This file is Claude Code's "sandbox" settings key: whether the shell's
// commands run inside the operating system's sandbox (spec/DECISIONS.md
// 352 and 353), and what they may do there. What only takes a power away
// is in force from a repository's file before the folder is trusted
// (turning the sandbox on, failIfUnavailable, allowUnsandboxedCommands
// false, autoAllowBashIfSandboxed false, the deny lists), and a
// repository's file cannot turn the person's sandbox off; what gives one
// waits for the trust (autoAllowBashIfSandboxed true, excludedCommands,
// allowWrite, allowRead).

// Sandbox is what the sandbox settings say, every file's merged.
type Sandbox struct {
	// Enabled runs the shell's commands in the sandbox; Origin is the line
	// that turned it on.
	Enabled bool
	Origin  Origin
	// FailIfUnavailable stops gnosh at startup when the sandbox is on and
	// cannot run here, where otherwise commands run outside it, with a
	// notice.
	FailIfUnavailable bool
	// AutoAllow runs a command that runs in the sandbox without asking,
	// as an allow rule would (autoAllowBashIfSandboxed).
	AutoAllow bool
	// AllowUnsandboxed lets the model ask for a command to run outside the
	// sandbox (the Bash tool's dangerouslyDisableSandbox), which then asks
	// as any command would: true unless a file says false.
	AllowUnsandboxed bool
	// Excluded are the commands that run outside the sandbox, as the
	// content of a Bash(...) rule (excludedCommands).
	Excluded []ListItem
	// AllowWrite and DenyWrite are more paths a sandboxed command may
	// write, and paths it may not; AllowRead and DenyRead, the same for
	// reading. Each is absolute.
	AllowWrite, DenyWrite, AllowRead, DenyRead []ListItem
}

// sandboxBuild is the sandbox settings as the builder gathers them: for
// each switch, the places that set it, so that a project's can be taken
// out again.
type sandboxBuild struct {
	// projectOn are the project's files that turn it on; person, the
	// person's own that turn it on or off, in the order they apply (a
	// later file wins).
	projectOn                                  []Origin
	person                                     []sandboxToggle
	failIfOff, noUnsandboxed, autoOn, autoOff  []Origin
	excluded, allowWrite, denyWrite, allowRead allowList
	denyRead                                   allowList
}

// sandboxToggle is one of the person's files turning the sandbox on or
// off.
type sandboxToggle struct {
	on bool
	o  Origin
}

// sandboxKeys are the keys of "sandbox" gnosh reads.
var sandboxKeys = []keyDef{
	{"enabled", "", "run the shell's commands in the sandbox (bubblewrap on Linux, Seatbelt on macOS)", fromClaudeCode},
	{"autoAllowBashIfSandboxed", "", "run a command that runs in the sandbox without asking, as an allow rule would", fromClaudeCode},
	{"failIfUnavailable", "", "stop at startup when the sandbox is on and cannot run here", fromClaudeCode},
	{"allowUnsandboxedCommands", "", "false: the model cannot ask for a command to run outside the sandbox", fromClaudeCode},
	{"excludedCommands", "", "commands that run outside the sandbox, as a Bash(...) rule's content", fromClaudeCode},
	{"filesystem", "", "allowWrite, denyWrite, allowRead and denyRead: lists of paths", fromClaudeCode},
}

// sandboxNotBuilt are the keys of Claude Code's "sandbox" gnosh has not
// built.
var sandboxNotBuilt = []string{"network", "credentials", "ignoreViolations", "enableWeakerNestedSandbox", "enableWeakerNetworkIsolation",
	"allowAppleEvents", "allowNestedUserNamespaces", "bwrapPath", "socatPath", "ripgrep"}

// sandbox reads the "sandbox" key.
func (s *source) sandbox(f jfield) {
	if f.val.kind != jObject {
		s.errorf(f.line, "\"sandbox\" is an object of the sandbox's settings ({\"enabled\": true, ...}), and here it is %s.", f.val.kind.word())
		return
	}
	theirs := s.scope == "project"
	for _, m := range s.unique(f.val, "sandbox.") {
		o := s.at(m.line)
		switch m.key {
		case "enabled", "failIfUnavailable", "autoAllowBashIfSandboxed", "allowUnsandboxedCommands":
			if m.val.kind != jBool {
				s.errorf(m.line, "\"sandbox.%s\" is true or false, and here it is %s.", m.key, m.val.kind.word())
				continue
			}
			s.sandboxSwitch(m.key, m.val.b, theirs, o)
		case "excludedCommands":
			items, ok := s.sandboxList(m, "commands, each the content of a Bash(...) rule such as \"docker *\"")
			if ok && len(items) > 0 {
				s.add(op{kind: KindAllowRule, text: "sandbox.excludedCommands: " + strings.Join(items, ", "), origin: o, what: "a sandbox setting",
					apply: func(b *builder) { b.sandbox.excluded.add(items, o) }})
			}
		case "filesystem":
			s.sandboxFilesystem(m)
		default:
			if slices.Contains(sandboxNotBuilt, m.key) {
				why := ""
				if m.key == "network" {
					why = " A sandboxed command reaches no network at all; one that needs it runs outside the sandbox, after asking (dangerouslyDisableSandbox)."
				}
				s.errorf(m.line, "\"sandbox.%s\" is a setting Claude Code has and gnosh has not built.%s Take it out of this file.", m.key, why)
				continue
			}
			s.errorf(m.line, "%s is not a key gnosh reads under \"sandbox\". The keys are: %s.", showName(m.key), keyNames(sandboxKeys))
		}
	}
}

// sandboxSwitch puts one of the sandbox's switches in force, by what it
// gives or takes away: a repository's file cannot turn the person's
// sandbox off.
func (s *source) sandboxSwitch(key string, v, theirs bool, o Origin) {
	text := "sandbox." + key + ": " + strconv.FormatBool(v)
	put := func(kind string, set func(b *sandboxBuild) *[]Origin) {
		s.add(op{kind: kind, text: text, origin: o, what: "a sandbox setting",
			apply: func(b *builder) { p := set(&b.sandbox); *p = append(*p, o) }})
	}
	switch {
	case key == "enabled" && theirs && v:
		put(KindRestriction, func(b *sandboxBuild) *[]Origin { return &b.projectOn })
	case key == "enabled" && theirs:
		s.probs.warnf(s.file, o.Line, "\"sandbox.enabled\": false in a project's file does not turn the sandbox off: a repository cannot take away a sandbox the person turned on. It is left out.")
	case key == "enabled":
		s.add(op{kind: KindRestriction, text: text, origin: o, what: "a sandbox setting",
			apply: func(b *builder) { b.sandbox.person = append(b.sandbox.person, sandboxToggle{on: v, o: o}) }})
	case key == "failIfUnavailable" && v:
		put(KindRestriction, func(b *sandboxBuild) *[]Origin { return &b.failIfOff })
	case key == "allowUnsandboxedCommands" && !v:
		put(KindRestriction, func(b *sandboxBuild) *[]Origin { return &b.noUnsandboxed })
	case key == "autoAllowBashIfSandboxed" && v:
		put(KindAllowRule, func(b *sandboxBuild) *[]Origin { return &b.autoOn })
	case key == "autoAllowBashIfSandboxed":
		put(KindRestriction, func(b *sandboxBuild) *[]Origin { return &b.autoOff })
	}
	// (failIfUnavailable false and allowUnsandboxedCommands true are what
	// holds when nothing is said: nothing to put in force.)
}

// sandboxList reads a list of strings, none of them empty.
func (s *source) sandboxList(m jfield, what string) ([]string, bool) {
	items, ok := m.val.strings()
	if !ok {
		s.errorf(m.line, "\"sandbox.%s\" is a list of %s, and here it is not.", m.key, what)
		return nil, false
	}
	var out []string
	for _, it := range items {
		v := strings.TrimSpace(it.str)
		if v == "" {
			s.errorf(it.line, "an entry of \"sandbox.%s\" is empty.", m.key)
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

// sandboxFilesystem reads "sandbox.filesystem": four lists of paths, in
// Claude Code's spelling: /x and //x from the root, ~/x from the home
// folder, and ./x or x from the project's root in a project's file, or
// from the folder of the person's own.
func (s *source) sandboxFilesystem(f jfield) {
	if f.val.kind != jObject {
		s.errorf(f.line, "\"sandbox.filesystem\" is an object of lists of paths (allowWrite, denyWrite, allowRead, denyRead), and here it is %s.", f.val.kind.word())
		return
	}
	base := s.baseDir
	if (s.scope == "project" || s.scope == "local") && s.ruleDir != "" {
		base = s.ruleDir
	}
	if s.scope == "command line" {
		base = s.cwd
	}
	for _, m := range s.unique(f.val, "sandbox.filesystem.") {
		o := s.at(m.line)
		key := "filesystem." + m.key
		switch m.key {
		case "allowWrite", "denyWrite", "allowRead", "denyRead":
		case "disabled", "allowManagedReadPathsOnly":
			s.errorf(m.line, "\"sandbox.filesystem.%s\" is a setting Claude Code has and gnosh has not built. Take it out of this file.", m.key)
			continue
		default:
			s.errorf(m.line, "%s is not a key gnosh reads under \"sandbox.filesystem\". The keys are allowWrite, denyWrite, allowRead and denyRead.", showName(m.key))
			continue
		}
		m.key = key
		items, ok := s.sandboxList(m, "paths")
		if !ok || len(items) == 0 {
			continue
		}
		paths := make([]string, 0, len(items))
		for _, it := range items {
			if strings.HasPrefix(it, "//") {
				it = it[1:]
			}
			// (A trailing /** is the folder with what is in it, which is
			// what a folder's path is to the sandbox.)
			it = strings.TrimSuffix(it, "/**")
			paths = append(paths, absPath(it, s.home, base))
		}
		kind, list := KindRestriction, func(b *builder) *allowList { return &b.sandbox.denyWrite }
		switch m.key {
		case "filesystem.allowWrite":
			kind, list = KindAddDir, func(b *builder) *allowList { return &b.sandbox.allowWrite }
		case "filesystem.allowRead":
			kind, list = KindAddDir, func(b *builder) *allowList { return &b.sandbox.allowRead }
		case "filesystem.denyRead":
			list = func(b *builder) *allowList { return &b.sandbox.denyRead }
		}
		s.add(op{kind: kind, text: "sandbox." + m.key + ": " + strings.Join(paths, ", "), origin: o, what: "a sandbox setting",
			apply: func(b *builder) { list(b).add(paths, o) }})
	}
}

// sweepSandbox takes out of the sandbox settings what a project's file
// put there that the kind of it may not contribute (build.go's sweep).
func (b *sandboxBuild) sweep(ok func(string) bool, from func(Origin) bool, note func(string)) {
	drop := func(kind, what string, list *[]Origin) {
		if ok(kind) {
			return
		}
		n := len(*list)
		*list = slices.DeleteFunc(*list, from)
		if len(*list) != n {
			note(what)
		}
	}
	drop(KindRestriction, "sandbox.enabled", &b.projectOn)
	drop(KindRestriction, "sandbox.failIfUnavailable", &b.failIfOff)
	drop(KindRestriction, "sandbox.allowUnsandboxedCommands", &b.noUnsandboxed)
	drop(KindRestriction, "sandbox.autoAllowBashIfSandboxed", &b.autoOff)
	drop(KindAllowRule, "sandbox.autoAllowBashIfSandboxed", &b.autoOn)
	lists := []struct {
		kind, what string
		l          *allowList
	}{
		{KindAllowRule, "sandbox.excludedCommands", &b.excluded},
		{KindAddDir, "sandbox.filesystem.allowWrite", &b.allowWrite},
		{KindAddDir, "sandbox.filesystem.allowRead", &b.allowRead},
		{KindRestriction, "sandbox.filesystem.denyWrite", &b.denyWrite},
		{KindRestriction, "sandbox.filesystem.denyRead", &b.denyRead},
	}
	for _, l := range lists {
		if !ok(l.kind) && l.l.drop(from) {
			note(l.what)
		}
	}
}

// finish is the merged settings: on when a project's file turned it on,
// or the last of the person's own files to say turned it on.
func (b *sandboxBuild) finish() Sandbox {
	var out Sandbox
	if n := len(b.person); n > 0 && b.person[n-1].on {
		out.Enabled, out.Origin = true, b.person[n-1].o
	}
	if n := len(b.projectOn); n > 0 && !out.Enabled {
		out.Enabled, out.Origin = true, b.projectOn[n-1]
	}
	out.FailIfUnavailable = len(b.failIfOff) > 0
	out.AllowUnsandboxed = len(b.noUnsandboxed) == 0
	out.AutoAllow = len(b.autoOn) > 0 && len(b.autoOff) == 0
	out.Excluded = b.excluded.items
	out.AllowWrite, out.DenyWrite = b.allowWrite.items, b.denyWrite.items
	out.AllowRead, out.DenyRead = b.allowRead.items, b.denyRead.items
	return out
}

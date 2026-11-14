package config

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// This file reads the keys a config file shares with Claude Code's
// settings files, in Claude Code's shape and with its meaning:
// permissions, hooks and env. A block copied out of a Claude Code settings
// file works as it is.

// settingsPermissions reads the permissions object.
func (s *source) settingsPermissions(f jfield) {
	if f.val.kind != jObject {
		s.errorf(f.line, "\"permissions\" is an object holding lists named allow, ask and deny, and here it is %s, so none of it is in force.", f.val.kind.word())
		return
	}
	for _, m := range s.unique(f.val, "permissions.") {
		o := s.at(m.line)
		switch m.key {
		case "allow", "ask", "deny":
			rules, ok := m.val.strings()
			if !ok {
				s.errorf(m.line, "\"permissions.%s\" is a list of rules, each a string such as \"Bash(git diff *)\", and here it is not, so none of its rules is in force.", m.key)
				continue
			}
			for _, r := range rules {
				s.rule(m.key, r.str, s.at(r.line))
			}
		case "defaultMode":
			s.settingsMode(m)
		case "additionalDirectories":
			dirs, ok := m.val.strings()
			if !ok {
				s.errorf(m.line, "\"permissions.additionalDirectories\" is a list of directories, each a string, and here it is not.")
				continue
			}
			for _, d := range dirs {
				if d.str == "" {
					s.errorf(d.line, "an entry of \"permissions.additionalDirectories\" is empty.")
					continue
				}
				dir, at := absPath(d.str, s.home, s.cwd), s.at(d.line)
				s.add(op{kind: KindAddDir, text: "additional directory " + dir, origin: at, feature: FeaturePermissions, what: "an additional directory",
					apply: func(b *builder) { addItem(&b.dirs, dir, at) }})
			}
		case "disableBypassPermissionsMode":
			if m.val.kind != jString || m.val.str != "disable" {
				s.errorf(m.line, "\"permissions.disableBypassPermissionsMode\" takes the one value \"disable\".")
				continue
			}
			s.add(op{kind: KindMode, text: "disableBypassPermissionsMode", origin: o, feature: FeaturePermissions, what: "a permission setting",
				apply: func(b *builder) { b.bypassDisabled.put(true, o) }})
		case "disableAutoMode":
			if m.val.kind != jString || m.val.str != "disable" {
				s.errorf(m.line, "\"permissions.disableAutoMode\" takes the one value \"disable\".")
			}
		case "denyInsteadOfPrompting":
			if m.val.kind != jBool {
				s.errorf(m.line, "\"permissions.denyInsteadOfPrompting\" is true or false, and here it is %s.", m.val.kind.word())
				continue
			}
			if s.scope == "project" || s.scope == "local" {
				s.probs.warnf(s.file, m.line, "\"permissions.denyInsteadOfPrompting\" is not taken from a project's file, so it is not in force here. Set it in your own gnosh.json, or with --settings.")
				continue
			}
			v := m.val.b
			s.add(op{kind: KindMode, text: "denyInsteadOfPrompting", origin: o, feature: FeaturePermissions, what: "a permission setting",
				apply: func(b *builder) { b.denyInstead.put(v, o) }})
		default:
			if slices.Contains(claudeCodePermissionKeys, m.key) {
				s.errorf(m.line, "\"permissions.%s\" is a setting Claude Code has and gnosh has not built, so it would not hold anything back here. Take it out of this file.", m.key)
				continue
			}
			s.errorf(m.line, "%s is not a key gnosh reads under \"permissions\". The keys are: %s.", showName(m.key), keyNames(permissionKeys))
		}
	}
}

// settingsMode reads permissions.defaultMode.
func (s *source) settingsMode(m jfield) {
	if m.val.kind != jString {
		s.errorf(m.line, "\"permissions.defaultMode\" is one of %s, and here it is %s.", list(permissionModes), m.val.kind.word())
		return
	}
	mode, o := m.val.str, s.at(m.line)
	if mode == "manual" {
		mode = "default"
	}
	fromProject := s.scope == "project" || s.scope == "local"
	switch {
	case mode == "auto":
		s.probs.warnf(s.file, m.line, "\"permissions.defaultMode\" is \"auto\", which gnosh does not have (it needs a second model to review each action). The session starts in the default mode, which asks more, not less.")
		return
	case !slices.Contains(permissionModes, mode):
		s.errorf(m.line, "%s is not a permission mode. \"permissions.defaultMode\" is one of %s.", showName(mode), list(permissionModes))
		return
	case mode == "bypassPermissions" && fromProject:
		s.probs.warnf(s.file, m.line, "\"permissions.defaultMode\" is \"bypassPermissions\" in a project's file, and a project cannot switch the questions off, so it is not in force. Set it in your own gnosh.json, or start gnosh with -y.")
		return
	}
	user := s.scope == "user" || s.scope == "command line"
	s.add(op{kind: KindMode, text: "defaultMode " + mode, origin: o, feature: FeaturePermissions, what: "a starting permission mode",
		apply: func(b *builder) {
			b.mode.put(mode, o)
			if b.mode.Origin == o {
				b.modeFromUser = user
			}
		}})
}

// settingsHooks reads the hooks object: event, then matcher groups, then
// handlers.
func (s *source) settingsHooks(f jfield) {
	if f.val.kind != jObject {
		s.errorf(f.line, "\"hooks\" is an object with one key for each event, and here it is %s, so none of the file's hooks is in force.", f.val.kind.word())
		return
	}
	for _, ev := range s.unique(f.val, "hooks.") {
		e := s.hookEventFor(ev.line, ev.key)
		if e == nil {
			continue
		}
		if ev.val.kind != jArray {
			s.errorf(ev.line, "\"hooks.%s\" is a list of matcher groups, and here it is %s, so none of its hooks is in force.", e.name, ev.val.kind.word())
			continue
		}
		for _, group := range ev.val.items {
			if group.kind != jObject {
				s.errorf(group.line, "an entry of \"hooks.%s\" is an object with a \"hooks\" list and, where the event has one, a \"matcher\"; this one is %s.", e.name, group.kind.word())
				continue
			}
			matcher := ""
			var handlers *jnode
			good := true
			for _, m := range s.unique(group, "hooks."+e.name+".") {
				switch m.key {
				case "matcher":
					if m.val.kind != jString {
						s.errorf(m.line, "a \"matcher\" is one string, such as \"Edit|Write\", and here it is %s.", m.val.kind.word())
						good = false
						continue
					}
					matcher = m.val.str
					if why := checkMatcher(e, matcher); why != "" {
						s.errorf(m.line, "%s.", why)
						good = false
					}
				case "hooks":
					if m.val.kind != jArray {
						s.errorf(m.line, "the \"hooks\" of an entry is a list of handlers, and here it is %s.", m.val.kind.word())
						good = false
						continue
					}
					handlers = m.val
				default:
					s.errorf(m.line, "%s is not a key of an entry under \"hooks.%s\". An entry holds \"matcher\" and \"hooks\".", showName(m.key), e.name)
					good = false
				}
			}
			if handlers == nil && good {
				s.errorf(group.line, "an entry of \"hooks.%s\" has no \"hooks\" list, so it would run nothing.", e.name)
			}
			if handlers == nil || !good {
				continue
			}
			for _, h := range handlers.items {
				s.settingsHandler(e, matcher, h)
			}
		}
	}
}

// settingsHandler reads one hook handler.
func (s *source) settingsHandler(e *hookEvent, matcher string, n *jnode) {
	if n.kind != jObject {
		s.errorf(n.line, "a hook handler is an object with \"type\" and \"command\", and here it is %s.", n.kind.word())
		return
	}
	h := Hook{Event: e.name, Matcher: matcher, Scope: s.scope, BaseDir: s.baseDir, Origin: s.at(n.line)}
	typ, good := "", true
	bad := func(line int, format string, args ...any) {
		s.errorf(line, format, args...)
		good = false
	}
	for _, m := range s.unique(n, "") {
		how, known := hookFields[m.key]
		switch {
		case !known:
			bad(m.line, "%s is not a field of a hook handler that gnosh knows, so the hook might not run as it was written. gnosh reads \"type\", \"command\" and \"timeout\".", showName(m.key))
		case m.key == "type":
			if m.val.kind != jString {
				bad(m.line, "a hook's \"type\" is a string, and here it is %s.", m.val.kind.word())
				break
			}
			if typ = m.val.str; typ != "command" {
				bad(m.line, "this hook has \"type\": %s, and gnosh runs hooks of type \"command\" only. A hook it cannot run as written is not run some other way: rewrite it as a command, or take it out.", showName(typ))
			}
		case m.key == "command":
			if m.val.kind != jString || strings.TrimSpace(m.val.str) == "" {
				bad(m.line, "a hook's \"command\" is the command line to run, as a string that is not empty.")
				break
			}
			h.Command = m.val.str
		case m.key == "timeout":
			secs, err := strconv.ParseFloat(m.val.str, 64)
			if m.val.kind != jNumber || err != nil || secs <= 0 || secs > 1e7 {
				bad(m.line, "a hook's \"timeout\" is a number of seconds above zero.")
				break
			}
			h.Timeout = time.Duration(secs * float64(time.Second))
		case how == "ignored":
		case how == "same":
			want := hookFieldSame[m.key]
			got := fmt.Sprintf("%q", m.val.str)
			if m.val.kind == jBool {
				got = strconv.FormatBool(m.val.b)
			}
			if (m.val.kind != jString && m.val.kind != jBool) || got != want {
				bad(m.line, "this hook sets \"%s\", and gnosh can only run a hook with \"%s\": %s (which is what leaving it out means). A hook it cannot run as written is not run some other way: take the field out, or the hook.", m.key, m.key, want)
			}
		default:
			bad(m.line, "this hook has \"%s\". %s, or take the hook out.", m.key, how)
		}
	}
	if good && typ == "" {
		bad(n.line, "this hook handler has no \"type\". Add \"type\": \"command\".")
	}
	if good && h.Command == "" {
		bad(n.line, "this hook handler has no \"command\", so it would run nothing.")
	}
	if !good {
		return
	}
	s.add(op{kind: KindHook, text: "hook " + e.name + ": " + h.Command, origin: h.Origin, feature: FeatureHooks, what: "a hook",
		apply: func(b *builder) { b.addHook(h) }})
}

// settingsEnv reads the env object: variables for the commands and hooks
// gnosh starts.
func (s *source) settingsEnv(f jfield) {
	if f.val.kind != jObject {
		s.errorf(f.line, "\"env\" is an object of variable names and string values, and here it is %s, so none of it is in force.", f.val.kind.word())
		return
	}
	for _, m := range s.unique(f.val, "env.") {
		if !isName(m.key) {
			s.errorf(m.line, "%s is not a usable variable name under \"env\".", showName(m.key))
			continue
		}
		if m.val.kind != jString {
			s.errorf(m.line, "the value of \"env.%s\" is a string (put a number in quotes), and here it is %s.", m.key, m.val.kind.word())
			continue
		}
		s.secrets.add(m.val.str)
		v := EnvVar{Name: m.key, Value: NewSecret(m.val.str), Origin: s.at(m.line)}
		s.add(op{kind: KindEnv, text: "env " + m.key, origin: v.Origin, feature: FeatureEnv, what: "a variable for commands",
			apply: func(b *builder) { b.setEnv(v) }})
	}
}

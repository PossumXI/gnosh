package permission

import (
	"path"
	"strings"
)

// The dangerous-removal check: rm and rmdir commands whose target is the
// filesystem root, a top-level directory, the home directory, the working
// directory or something above it, or a target that could turn into one
// of those when a variable is empty. No allow rule and no hook approves
// one. It is a seat belt, not a wall: it catches the usual accidents.

// topLevelNames are directory names commonly found directly under the
// filesystem root.
var topLevelNames = map[string]bool{
	"bin": true, "boot": true, "dev": true, "etc": true, "home": true, "lib": true,
	"lib32": true, "lib64": true, "media": true, "mnt": true, "opt": true, "proc": true,
	"root": true, "run": true, "sbin": true, "srv": true, "sys": true, "tmp": true,
	"usr": true, "var": true, "Applications": true, "Library": true, "System": true,
	"Users": true, "Volumes": true, "private": true, "cores": true, "snap": true,
}

// criticalDir says why removing the cleaned absolute path p would be
// dangerous, or "".
func (e *Engine) criticalDir(p string) string {
	switch {
	case p == "/":
		return "it is the filesystem root"
	case path.Dir(p) == "/":
		return p + " is a top-level directory"
	case e.home != "" && p == e.home:
		return "it is the home directory"
	case p == e.cwd || p == e.cur:
		return "it is the working directory"
	case inside(p, e.cwd) || inside(p, e.cur):
		return p + " holds the working directory"
	}
	return ""
}

// criticalGlobDir says why removing everything directly under p would be
// dangerous, or "".
func (e *Engine) criticalGlobDir(p string) string {
	if why := e.criticalDir(p); why != "" {
		return "the pattern covers everything in " + p + ": " + why
	}
	for _, d := range e.workDirs() {
		if inside(p, d) {
			return "the pattern covers everything in " + p + ", which is or holds a working directory"
		}
	}
	return ""
}

// removalDanger says why a command is a dangerous removal, or "". dirs
// are the directories the command may be running in; fromSubst names the
// variables that the same line assigned from a command's output.
func (e *Engine) removalDanger(c *simpleCmd, dirs []string, fromSubst map[string]bool) string {
	if len(c.words) < 2 {
		return ""
	}
	full, ok := e.progName(c.words[0])
	if !ok {
		return ""
	}
	name := baseName(full)
	if name != "rm" && name != "rmdir" {
		return ""
	}
	recursive := false
	var targets []word
	rest := false
	for _, w := range c.words[1:] {
		l := w.lit()
		switch {
		case rest || !w.static() || !strings.HasPrefix(l, "-") || w.tilde:
			targets = append(targets, w)
		case l == "--":
			rest = true
		case l == "--recursive" || !strings.HasPrefix(l, "--") && strings.ContainsAny(l, "rR"):
			recursive = true
		}
	}
	for _, w := range targets {
		if why := e.targetDanger(w, dirs, fromSubst, recursive); why != "" {
			return name + " " + w.raw + ": " + why
		}
	}
	return ""
}

// targetDanger judges one target of a removal.
func (e *Engine) targetDanger(w word, dirs []string, fromSubst map[string]bool, recursive bool) string {
	// What the target is when the variables whose values are known are
	// filled in, and every other expansion comes out empty.
	var val strings.Builder
	unknown, guardedOnly, vars, substs := false, true, 0, 0
	for _, p := range w.parts {
		switch p.kind {
		case partLit:
			val.WriteString(p.lit)
		case partVar:
			vars++
			switch {
			case p.name == "HOME" && e.home != "":
				val.WriteString(e.home)
			case p.name == "PWD":
				val.WriteString(e.cur)
			default:
				unknown = true
				if !p.guarded {
					guardedOnly = false
				}
				if fromSubst[p.name] && len(w.parts) <= 2 {
					return "the variable " + p.name + " was just set from a command's output, which may be a directory to keep"
				}
			}
		default:
			substs++
			unknown = true
			guardedOnly = false
		}
	}
	if w.tilde && !w.static() || w.eqHead {
		return ""
	}
	s := val.String()
	if w.tilde {
		s = w.value(e.home)
	}
	if unknown {
		if recursive && w.onlySubst() {
			return "the target is whatever a command prints"
		}
		if guardedOnly && substs == 0 {
			// Every unknown variable is written ${NAME:?}: the shell stops
			// rather than expanding it to nothing.
			return ""
		}
		trimmed := strings.TrimSuffix(s, "/")
		switch {
		case s == "":
			return ""
		case strings.HasSuffix(trimmed, "/*") || (strings.HasSuffix(s, "/") && strings.HasPrefix(s, "/")):
			return "if the variable is empty this removes " + s
		case strings.HasPrefix(s, "/") && !strings.Contains(strings.Trim(s, "/"), "/") && topLevelNames[strings.Trim(s, "/")]:
			return "if the variable is empty this removes " + s
		}
		return ""
	}
	if s == "" {
		return ""
	}
	globbed := w.glob && hasPathMagic(s)
	for _, d := range dirs {
		abs, ok := cleanAbs(s, d)
		if !ok {
			continue
		}
		if !globbed {
			if why := e.criticalDir(abs); why != "" {
				return why
			}
			continue
		}
		// A pattern: find the directory whose entries it sweeps up.
		segs := splitPath(abs)
		for i, seg := range segs {
			if hasPathMagic(seg) {
				// Only a pattern that takes every entry counts: *.tmp
				// names its own files.
				if i == len(segs)-1 && strings.Trim(seg, "*.?") == "" {
					if why := e.criticalGlobDir("/" + strings.Join(segs[:i], "/")); why != "" {
						return why
					}
				}
				break
			}
		}
	}
	return ""
}

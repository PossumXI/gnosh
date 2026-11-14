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
	p = oneDisk(p)
	switch {
	case byNumber(p):
		return "it is named by a number (" + p + "), and what that is cannot be known ahead of time"
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
	p = oneDisk(p)
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
func (e *Engine) removalDanger(v *bashView, c *simpleCmd, dirs []string, fromSubst map[string]bool) string {
	// "rm${s}-rf${s}/" is rm -rf / where s holds a blank: a program's
	// name with something unknown inside it is also read with a blank
	// in each such place.
	if alt := spaceReading(c); alt != nil {
		if why := e.removalDanger(v, alt, dirs, fromSubst); why != "" {
			return why
		}
	}
	if len(c.words) < 2 {
		return ""
	}
	full, ok := e.progName(c.words[0])
	name := progKey(baseName(full))
	if !ok || progIsPattern(c.words[0]) {
		// A program that is not known ahead of time, given what rm is
		// given ("$RM -rf /", "$(which rm) -rf ~"), is taken for rm.
		name = "rm"
		looksLikeRm := false
		for _, w := range c.words[1:] {
			// The letters are the ones some rm takes (GNU's, and -P, -W and
			// -x of the BSDs).
			if l := w.lit(); w.static() && (recursiveLong(l) || len(l) > 1 && l[0] == '-' && l[1] != '-' && strings.Trim(l[1:], "rRfvdiIPWx") == "" && strings.ContainsAny(l, "rR")) {
				looksLikeRm = true
			}
		}
		if !looksLikeRm {
			return ""
		}
	}
	if name == "find" {
		return e.findDanger(v, c, dirs, fromSubst)
	}
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
		case recursiveLong(l) || !strings.HasPrefix(l, "--") && strings.ContainsAny(l, "rR"):
			recursive = true
		}
	}
	// rmdir takes a directory whole, like rm -r: one whose name is only
	// known when the line runs may be one to keep.
	recursive = recursive || name == "rmdir"
	// Braces around a variable are several targets too: "rm -rf {$d,x}".
	for i := 0; i < len(targets); i++ {
		if alts, ok := braceAlts(targets[i]); ok && len(targets)+len(alts) < 256 {
			targets = append(targets, alts...)
		}
	}
	for _, w := range targets {
		if why := e.targetDanger(w, dirs, fromSubst, recursive); why != "" {
			return name + " " + w.raw + ": " + why
		}
		// Braces are several targets: "rm -rf {.,x}".
		if w.brace && w.static() && !w.quoted {
			if alts, ok := braceExpand(w.joined()); ok {
				for _, alt := range alts {
					part := litOf(alt)
					part.glob, part.tilde = w.glob && hasPathMagic(alt), strings.HasPrefix(alt, "~")
					if why := e.targetDanger(part, dirs, fromSubst, recursive); alt != "" && why != "" {
						return name + " " + w.raw + ": " + why
					}
				}
			}
		}
		// A variable the line itself set is also tried as what it was set
		// to: "d=.; rm -rf $d".
		// (Not quoted, its value is cut into words, and a pattern in it
		// is one: "d='*'; rm -rf $d".)
		// (A for loop's list gives it several: each is tried. More of
		// them than gnosh follows are one more doubt.)
		if recursive && e.tooManyFills(v, w) {
			return name + " " + w.raw + ": the line gives the target more values than gnosh follows"
		}
		vals, _, _ := e.filledAll(v, w)
		for _, val := range vals {
			if val.text == "" {
				continue
			}
			fields := []string{val.text}
			if !w.quoted {
				fields = e.fields(v, val.text)
			}
			for _, f := range fields {
				t := litOf(f)
				t.glob = hasPathMagic(f) && (w.glob || !w.quoted || val.glob)
				t.tilde = false
				if why := e.targetDanger(t, dirs, fromSubst, recursive); why != "" {
					return name + " " + w.raw + ": " + why
				}
			}
		}
	}
	return ""
}

// spaceReading returns the command as it is when every part of its
// first word that is not written out holds a blank, where that makes
// more than one word of it: what the shell does with "rm${IFS}-rf" and
// with any variable that holds a blank. It returns nil for a first word
// that is all written out, or all unknown.
func spaceReading(c *simpleCmd) *simpleCmd {
	if len(c.words) == 0 || c.words[0].static() {
		return nil
	}
	var b strings.Builder
	lits, holes := 0, 0
	for _, p := range c.words[0].parts {
		if p.kind == partLit {
			b.WriteString(p.lit)
			lits += len(p.lit)
		} else {
			b.WriteByte(' ')
			holes++
		}
	}
	fields := strings.Fields(b.String())
	if lits == 0 || holes == 0 || len(fields) < 2 {
		return nil
	}
	var ws []word
	for i, f := range fields {
		part := litOf(f)
		part.glob = i > 0 && hasPathMagic(f)
		ws = append(ws, part)
	}
	return &simpleCmd{words: append(ws, c.words[1:]...), redirs: c.redirs, nest: c.nest}
}

// recursiveLong reports whether l is rm's --recursive, or a shorter way
// of writing it that GNU rm takes (--r, --rec, ...).
func recursiveLong(l string) bool {
	return len(l) >= 3 && strings.HasPrefix("--recursive", l)
}

// findNarrows are the tests of find that pick some files and leave
// others: with one of them in front, what an action touches is not
// known ahead of time.
var findNarrows = map[string]bool{
	"-name": true, "-iname": true, "-path": true, "-ipath": true, "-wholename": true, "-iwholename": true,
	"-regex": true, "-iregex": true, "-type": true, "-xtype": true, "-size": true, "-newer": true,
	"-anewer": true, "-cnewer": true, "-mtime": true, "-mmin": true, "-atime": true, "-amin": true,
	"-ctime": true, "-cmin": true, "-empty": true, "-user": true, "-group": true, "-perm": true,
	"-links": true, "-inum": true, "-samefile": true, "-lname": true, "-ilname": true, "-uid": true,
	"-gid": true, "-nouser": true, "-nogroup": true, "-readable": true, "-writable": true,
	"-executable": true, "-fstype": true, "-used": true, "-newermt": true, "-false": true,
}

// findDanger judges a find that removes what it finds: "find . -delete"
// and "find . -exec rm -rf {} +" are "rm -rf ." by another name. It only
// speaks when nothing narrows what find visits, so that the usual
// "find . -name '*.o' -delete" is left alone.
func (e *Engine) findDanger(v *bashView, c *simpleCmd, dirs []string, fromSubst map[string]bool) string {
	args := c.words[1:]
	var starts []word
	i := 0
	for ; i < len(args); i++ {
		l := args[i].lit()
		if l == "-P" || l == "-L" || l == "-H" {
			continue
		}
		if strings.HasPrefix(l, "-") || l == "!" || l == "(" {
			break
		}
		starts = append(starts, args[i])
	}
	if len(starts) == 0 {
		starts = []word{litOf(".")}
	}
	removes := false
	for ; i < len(args); i++ {
		l := args[i].lit()
		switch {
		case findNarrows[l] || strings.HasPrefix(l, "-newer"):
			return ""
		case l == "-delete":
			removes = true
		case l == "-exec" || l == "-execdir" || l == "-ok" || l == "-okdir":
			for k := i + 1; k < len(args) && k < i+4; k++ {
				if n := baseName(args[k].lit()); n == "rm" || n == "rmdir" {
					removes = true
				}
			}
		}
	}
	if !removes {
		return ""
	}
	for _, w := range starts {
		// find removes what is in the starting point, and the starting
		// point too when it can.
		for _, t := range []word{w, {raw: w.raw + "/*", parts: append(append([]wordPart(nil), w.parts...), wordPart{lit: "/*"}), glob: true, quoted: w.quoted, tilde: w.tilde}} {
			if why := e.targetDanger(t, dirs, fromSubst, true); why != "" {
				return "find " + w.raw + " removes what it finds: " + why
			}
		}
	}
	return ""
}

// leadsTo returns where the cleaned absolute path p leads when links
// are followed, if that is somewhere else and can be told.
func (e *Engine) leadsTo(p string) string {
	if e.resolve == nil {
		return ""
	}
	r, err := e.resolve(p)
	if err != nil || !strings.HasPrefix(r, "/") {
		return ""
	}
	if r = cleanPath(r); r == p {
		return ""
	}
	return r
}

// targetDanger judges one target of a removal.
func (e *Engine) targetDanger(w word, dirs []string, fromSubst map[string]bool, recursive bool) string {
	// What the target is when the variables whose values are known are
	// filled in, and every other expansion comes out empty.
	// A ${name:-word} or a ${name:+word} is also judged as the word it
	// may stand for ("rm -rf ${X:+/}", "rm -rf ${X:-$HOME}"), and a
	// ${HOME%/*} as each piece that may be left of the directory.
	for i, p := range w.parts {
		if p.kind != partVar {
			continue
		}
		var readings []word
		switch {
		case p.body != nil:
			readings = append(readings, withBody(w, i, e.home))
		case !p.plainly() && (p.name == "HOME" && e.home != "" || p.name == "PWD"):
			base := e.home
			if p.name == "PWD" {
				base = e.cur
			}
			texts, _ := p.of(base, e.home)
			for _, t := range texts {
				r := w
				r.parts = append(append(append([]wordPart(nil), w.parts[:i]...), wordPart{lit: t}), w.parts[i+1:]...)
				readings = append(readings, r)
			}
		}
		for _, r := range readings {
			if why := e.targetDanger(r, dirs, fromSubst, recursive); why != "" {
				return why
			}
		}
	}
	var val strings.Builder
	unknown, guardedOnly, vars, substs := false, true, 0, 0
	// stated: every variable that is not known says what it stands for
	// when it is not set (${DIR:-/}): the word is then a path like any.
	stated := true
	for _, p := range w.parts {
		switch p.kind {
		case partLit:
			val.WriteString(p.lit)
		case partVar:
			vars++
			switch {
			case p.name == "HOME" && e.home != "" && p.plainly():
				val.WriteString(e.home)
			case p.name == "PWD" && p.plainly():
				val.WriteString(e.cur)
			default:
				unknown = true
				if !p.guarded {
					guardedOnly = false
				}
				if fromSubst[p.name] && len(w.parts) <= 2 {
					return "the variable " + p.name + " was just set from a command's output, which may be a directory to keep"
				}
				// "${DIR:-/}" is / when DIR is not set.
				val.WriteString(p.defIn(e.home))
				stated = stated && p.hasDef && p.def != ""
			}
		default:
			substs++
			unknown = true
			guardedOnly = false
			stated = false
		}
	}
	s := val.String()
	switch {
	case w.eqHead:
		return ""
	case w.tilde && w.static():
		s = w.value(e.home)
	case w.tilde && allLit(w) && s != "~+" && !strings.HasPrefix(s, "~+/") && func() bool { _, ok := dirTilde(w); return ok }():
		// ~- is the directory the shell was in before, and ~2 one that
		// pushd put by: after a cd, one of those the line has been in.
		rest, _ := dirTilde(w)
		for _, d := range dirs {
			t := litOf(d + rest)
			t.glob = w.glob && hasPathMagic(rest)
			if why := e.targetDanger(t, dirs, fromSubst, recursive); why != "" {
				return why
			}
		}
		if rest == "" {
			return "it is a directory the line was in before"
		}
		return ""
	case w.tilde && (s == "~+" || strings.HasPrefix(s, "~+/")):
		// ~+ is the directory the shell is in.
		s = e.cur + s[2:]
	case w.tilde && e.home != "" && w.parts[0].kind == partLit && strings.HasPrefix(w.parts[0].lit, "~/"):
		// "~/$X" is the home directory and whatever X holds, which may
		// be nothing.
		s = e.home + s[1:]
	case w.tilde:
		// ~name is somebody's home directory, and so is "~name/$X" where
		// X holds nothing.
		if rest := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(s, "/"), "/*"), "/"); !strings.Contains(rest, "/") && len(rest) > 1 {
			return "it is the home directory of " + rest[1:]
		}
		return ""
	}
	if unknown {
		if recursive && w.onlySubst() {
			return "the target is whatever a command prints"
		}
		if stated && !w.tilde {
			left := litOf(s)
			left.glob = w.glob
			if why := e.targetDanger(left, dirs, fromSubst, recursive); why != "" {
				return "if the variable is not set this removes " + s + ": " + why
			}
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
		case strings.HasPrefix(s, "/") && strings.Contains(s, "/.."):
			// What is left may climb: "rm -rf ~/$X/.." is the directory
			// above the home directory.
			left := litOf(s)
			left.glob = w.glob
			if why := e.targetDanger(left, dirs, fromSubst, recursive); why != "" {
				return "if the variable is empty this removes " + s + ": " + why
			}
		case !strings.HasPrefix(s, "/") && !w.tilde:
			// What is left of the word when the variable is empty may be
			// the working directory itself, or everything in it:
			// "rm -rf $X." and "rm -rf $PREFIX*".
			left := litOf(s)
			left.glob = w.glob
			if why := e.targetDanger(left, dirs, fromSubst, recursive); why != "" {
				return "if the variable is empty this removes " + s + ": " + why
			}
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
			// Where "link/.." really leads, when that is somewhere else.
			if phys, ok := e.physical(s, d); ok && phys != abs {
				if why := e.criticalDir(phys); why != "" {
					return why
				}
			}
			// And where the target itself leads, when it is a link: with
			// a slash after it, or to the rm of macOS, removing the link
			// is removing what it leads to.
			if to := e.leadsTo(abs); to != "" {
				if why := e.criticalDir(to); why != "" {
					return "it is a link to " + to + ": " + why
				}
			}
			continue
		}
		// A pattern: find the directory whose entries it sweeps up, or
		// the directory to keep that it may take in.
		segs := splitPath(abs)
		first, last := -1, len(segs)-1
		for i, seg := range segs {
			if hasPathMagic(seg) {
				first = i
				break
			}
		}
		if first < 0 {
			continue
		}
		// A part that takes every entry there is: *.tmp names its own
		// files, * names them all.
		sweeps := func(seg string) bool { return hasPathMagic(seg) && strings.Trim(seg, "*.?") == "" }
		dir := "/" + strings.Join(segs[:first], "/")
		all := true
		for _, seg := range segs[first:] {
			all = all && sweeps(seg)
		}
		if all {
			// Everything in the directory, or ("*/*") everything in
			// everything in it.
			if why := e.criticalGlobDir(dir); why != "" {
				return why
			}
			if to := e.leadsTo(dir); to != "" {
				if why := e.criticalGlobDir(to); why != "" {
					return dir + " is a link to " + to + ": " + why
				}
			}
		}
		if last == 0 {
			// "/e*", "/[!x]*": whatever it matches lies right under the
			// root.
			return "the pattern names what lies directly under the filesystem root"
		}
		// The directories to keep, each held against the pattern: it may
		// take one of them in ("/home/u*", "../p*", "/*/u"), or
		// everything in one ("/h*/*").
		for _, keep := range e.keepDirs() {
			ks := splitPath(keep)
			if len(ks) == len(segs) && segsMatch(segs, ks) {
				if why := e.criticalDir(keep); why != "" {
					return "the pattern takes in " + keep + ": " + why
				}
			}
			if sweeps(segs[last]) && len(ks) == last && segsMatch(segs[:last], ks) {
				if why := e.criticalGlobDir(keep); why != "" {
					return why
				}
			}
		}
	}
	return ""
}

// keepDirs returns the directories a pattern is held against by name:
// the home directory, the working directories and everything above
// them, and the usual directories under the root.
func (e *Engine) keepDirs() []string {
	var out []string
	add := func(p string) {
		for p != "" && p != "/" {
			out = unionStrings(out, []string{p})
			p = path.Dir(p)
		}
	}
	add(e.home)
	add(e.cwd)
	add(e.cur)
	for _, d := range e.workDirs() {
		add(d)
	}
	for name := range topLevelNames {
		out = unionStrings(out, []string{"/" + name})
	}
	return out
}

// segsMatch reports whether a path's parts could be what the pattern's
// parts match, one for one. A part it cannot judge matches.
func segsMatch(pats, names []string) bool {
	for i, pat := range pats {
		if !hasPathMagic(pat) {
			if !strings.EqualFold(pat, names[i]) {
				return false
			}
			continue
		}
		// (The shells write [!a] for what Go's matcher writes [^a].)
		if ok, err := path.Match(strings.ReplaceAll(pat, "[!", "[^"), names[i]); err == nil && !ok {
			return false
		}
	}
	return true
}

package permission

import (
	"strings"
)

// lineState follows the shell's directory along one command line.
type lineState struct {
	// dirs are the directories the next command may be running in.
	dirs []string
	// pending is where a cd that just ran would leave the shell. It
	// takes effect for sure only after &&; after anything else the shell
	// may be in either place.
	pending []string
	// moved is set once a cd has come earlier in the line.
	moved bool
	// unknown is set when the directory can no longer be worked out.
	unknown bool
}

// enter is called before judging c: it settles where the shell is.
func (st *lineState) enter(c *simpleCmd) {
	if st.pending == nil {
		return
	}
	if c.sep == sepAnd {
		st.dirs = st.pending
	} else {
		st.dirs = unionStrings(st.dirs, st.pending)
	}
	st.pending = nil
	st.moved = true
}

func unionStrings(a, b []string) []string {
	out := append([]string(nil), a...)
	for _, s := range b {
		found := false
		for _, o := range out {
			if o == s {
				found = true
				break
			}
		}
		if !found {
			out = append(out, s)
		}
	}
	return out
}

// lineDirs returns every directory a command of the line might run in:
// the shell's current one and wherever a cd in the line leads. It is used
// where being wrong must err on the strict side (deny rules, dangerous
// removals), so order and && are ignored.
func (e *Engine) lineDirs(v *bashView) []string {
	dirs := []string{e.cur}
	for _, u := range v.units {
		ws := u.cmd.words
		if len(ws) != 2 || !ws[0].static() || ws[0].joined() != "cd" || !ws[1].static() || len(dirs) >= 16 {
			continue
		}
		for _, d := range append([]string(nil), dirs...) {
			if abs, ok := cleanAbs(ws[1].value(e.home), d); ok {
				dirs = unionStrings(dirs, []string{abs})
			}
		}
	}
	return dirs
}

// absAll returns w's value as an absolute path for each directory the
// command may run in. ok is false when the word is not a plain path.
func (e *Engine) absAll(w word, dirs []string) (out []string, pattern, ok bool) {
	if !w.static() || w.brace {
		return nil, false, false
	}
	val := w.value(e.home)
	if val == "" {
		return nil, false, false
	}
	for _, d := range dirs {
		abs, ok := cleanAbs(val, d)
		if !ok {
			return nil, false, false
		}
		out = unionStrings(out, []string{abs})
	}
	return out, w.glob && hasPathMagic(val), true
}

// shellPaths lists the files a command line names for reading and for
// writing, so that Read and Edit rules and the protected paths can be
// held against them.
func (e *Engine) shellPaths(req *request) {
	v := req.view
	dirs := e.lineDirs(v)
	var reads, writes []string
	add := func(w word, into *[]string) {
		if w.static() && (w.joined() == "-" || w.joined() == "/dev/null") {
			return
		}
		abs, pattern, ok := e.absAll(w, dirs)
		switch {
		case !ok:
			req.pathOpaque = "it names a file with text computed when the command runs (" + w.raw + ")"
		case !pattern:
			*into = append(*into, abs...)
		case e.glob == nil:
			req.pathOpaque = "it names files with a pattern (" + w.raw + ")"
		default:
			for _, a := range abs {
				if ms, err := e.glob(a); err == nil {
					*into = append(*into, ms...)
				} else {
					req.pathOpaque = "it names files with a pattern that could not be expanded (" + w.raw + ")"
				}
			}
		}
	}
	for _, u := range v.units {
		f := facts(u.cmd.words)
		for _, w := range f.reads {
			add(w, &reads)
		}
		for _, w := range f.writes {
			add(w, &writes)
		}
		for _, r := range u.cmd.redirs {
			switch r.op {
			case "<":
				add(r.target, &reads)
			case ">", ">>", ">|", "&>", "&>>", "<>":
				add(r.target, &writes)
			case ">&":
				if !isFdTarget(r.target) {
					add(r.target, &writes)
				}
			}
		}
	}
	req.reads = e.expand(req, reads)
	req.writes = e.expand(req, writes)
	req.unresolved = false // links matter here only to the checks that allow
}

// isFdTarget reports whether a redirection target is a descriptor number
// or "-", as in 2>&1 and >&-.
func isFdTarget(w word) bool {
	if !w.plain() {
		return false
	}
	l := w.joined()
	return l == "-" || isInt(l) && l[0] != '-' && l[0] != '+'
}

// readable reports whether the cleaned absolute path may be read without
// asking: it lies in a working directory, or an allow rule covers it, as
// written and as resolved.
func (e *Engine) readable(p string) bool {
	check := func(p string) bool {
		for _, d := range e.workDirs() {
			if inside(d, p) {
				return true
			}
		}
		return e.allowsPath(p, false) != nil
	}
	if !check(p) {
		return false
	}
	if e.resolve == nil {
		return true
	}
	r, err := e.resolve(p)
	return err == nil && strings.HasPrefix(r, "/") && check(cleanPath(r))
}

// inWork reports whether the cleaned absolute path lies in a working
// directory, as written and as resolved.
func (e *Engine) inWork(p string) bool {
	check := func(p string) bool {
		for _, d := range e.workDirs() {
			if inside(d, p) {
				return true
			}
		}
		return false
	}
	if !check(p) {
		return false
	}
	if e.resolve == nil {
		return true
	}
	r, err := e.resolve(p)
	return err == nil && strings.HasPrefix(r, "/") && check(cleanPath(r))
}

func cleanPath(p string) string {
	c, _ := cleanAbs(p, "")
	return c
}

// wordsOK reports whether every word names something for which ok holds,
// wherever the command may be running. why says what did not.
func (e *Engine) wordsOK(ws []word, st *lineState, ok func(string) bool, what string) string {
	for _, w := range ws {
		if w.static() && w.joined() == "-" {
			continue
		}
		abs, pattern, good := e.absAll(w, st.dirs)
		if !good {
			return "it names a file gnosh cannot know ahead of time (" + w.raw + ")"
		}
		if st.unknown && !strings.HasPrefix(w.value(e.home), "/") {
			return "it names " + w.raw + " after a change of directory gnosh could not follow"
		}
		if pattern {
			if e.glob == nil {
				return "it names files with a pattern (" + w.raw + "), which cannot be checked ahead of time"
			}
			var all []string
			for _, a := range abs {
				ms, err := e.glob(a)
				if err != nil {
					return "the pattern " + w.raw + " could not be expanded"
				}
				all = append(all, ms...)
			}
			// A matched name that starts with a dash would reach the
			// command as a flag.
			if strings.ContainsAny(w.joined()[:1], "*?[") {
				for _, m := range all {
					if strings.HasPrefix(baseName(m), "-") {
						return "the pattern " + w.raw + " matches " + baseName(m) + ", which the command would read as a flag"
					}
				}
			}
			abs = all
		}
		for _, a := range abs {
			if !ok(cleanPath(a)) {
				return a + " is " + what
			}
		}
	}
	return ""
}

// partVerdict is the judgement of one simple command of a line.
type partVerdict struct {
	// ok: the part runs without asking; rule is the allow rule that let
	// it, when one did.
	ok   bool
	rule *crule
	// why says why the part needs approval.
	why string
	// exact and prefix are the rules that would cover the part from now
	// on; both are empty when no rule could.
	exact, prefix string
}

// judgePart decides whether one simple command of a flat line runs
// without asking: an allow rule names it, or it only reads, or it is one
// of the file commands acceptEdits lets run.
func (e *Engine) judgePart(c *simpleCmd, st *lineState) partVerdict {
	plan := e.mode == ModePlan
	if len(c.assigns) > 0 {
		return partVerdict{why: "it sets a variable (" + c.assigns[0].w.raw + "), and gnosh asks about every command that does"}
	}
	staticInput := true
	for _, r := range c.redirs {
		switch r.op {
		case "<<", "<<-":
			staticInput = staticInput && r.bodyStatic
		case "<<<":
			staticInput = staticInput && r.target.static()
		case "<&":
			if !isFdTarget(r.target) {
				return partVerdict{why: "it has a redirection gnosh does not follow (<&" + r.target.raw + ")"}
			}
		case "<":
			if r.target.plain() && r.target.joined() == "/dev/null" {
				continue
			}
			if st.moved && r.target.static() && !strings.HasPrefix(r.target.value(e.home), "/") {
				return partVerdict{why: "it reads " + r.target.raw + " through < after a cd"}
			}
			if r.target.glob {
				return partVerdict{why: "it reads through < from a file name pattern"}
			}
			if why := e.wordsOK([]word{r.target}, st, e.readable, "outside the working directories"); why != "" {
				return partVerdict{why: "its input: " + why}
			}
		default:
			if r.op == ">&" && isFdTarget(r.target) {
				continue
			}
			if r.target.plain() && r.target.joined() == "/dev/null" {
				continue
			}
			return partVerdict{why: "it writes to " + r.target.raw + " with a redirection, and gnosh asks about those"}
		}
	}
	if len(c.words) == 0 {
		return partVerdict{ok: true}
	}

	words := c.words
	why := ""
	viaXargs := false
	var first cmdFacts
	for depth := 0; ; depth++ {
		f := facts(words)
		if depth == 0 {
			first = f
		}
		if !f.plain {
			if why == "" {
				why = orDefault(f.why, "the program name is not written as plain text")
			}
			break
		}
		// After a cd, a program named by a relative path is not the file
		// the rule's author saw: no allow rule names it.
		prog := words[0].joined()
		relProg := strings.Contains(prog, "/") && !strings.HasPrefix(prog, "/")
		if !plan && !(st.moved && relProg) {
			if r := e.allowCommand(words); r != nil {
				e.afterPart(f, st)
				return partVerdict{ok: true, rule: r}
			}
		}
		if !viaXargs {
			if w := e.partRuns(c, f, st, depth, staticInput); w == "" {
				e.afterPart(f, st)
				return partVerdict{ok: true}
			} else if why == "" {
				why = w
			}
		}
		inner, ok := strictStrip(words)
		if !ok || depth >= 8 {
			break
		}
		viaXargs = viaXargs || words[0].joined() == "xargs"
		words = inner
	}
	e.afterPart(first, st)
	pv := partVerdict{why: why}
	if plainName(c.words[0]) {
		pv.exact, pv.prefix = e.suggestRules(c, first, st)
	}
	return pv
}

// afterPart notes what a part that is going to run does to the shell's
// directory.
func (e *Engine) afterPart(f cmdFacts, st *lineState) {
	switch f.name {
	case "cd":
		if f.cd == nil {
			st.unknown = true
			return
		}
		abs, _, ok := e.absAll(*f.cd, st.dirs)
		if !ok || st.unknown {
			st.unknown = true
			return
		}
		st.pending = abs
	case "pushd", "popd":
		st.unknown = true
	}
}

// allowCommand returns the allow rule that names the command, if any.
func (e *Engine) allowCommand(words []word) *crule {
	text := rawJoined(words)
	exactOnly := prefixOnlyExact(words)
	for _, c := range e.rules(ListAllow) {
		if c.kind != kindCommand || exactOnly && !c.cmd.exact() {
			continue
		}
		if c.cmd.match(text) {
			return c
		}
	}
	return nil
}

// partRuns says why the command does not run on the mode alone, or ""
// when it does: it only reads inside the working directories, or the mode
// is acceptEdits and it is a file command that stays inside them.
func (e *Engine) partRuns(c *simpleCmd, f cmdFacts, st *lineState, depth int, staticInput bool) string {
	for _, d := range st.dirs {
		if !e.readable(d) {
			return "it would run in " + d + ", outside the working directories"
		}
	}
	if st.unknown {
		return "it runs after a change of directory gnosh could not follow"
	}
	switch {
	case f.readOnly:
		if !staticInput {
			return "its input is computed when the command runs"
		}
		if f.git && st.moved {
			return "it runs git after a cd, and the other directory's git hooks could run"
		}
		if f.cd != nil {
			return e.wordsOK([]word{*f.cd}, st, e.readable, "outside the working directories")
		}
		return e.wordsOK(f.reads, st, e.readable, "outside the working directories")
	case f.fsEdit && e.mode == ModeAcceptEdits:
		if f.name == "touch" && depth > 0 {
			return "touch behind a wrapper is not one of the commands acceptEdits lets run"
		}
		if why := e.wordsOK(f.reads, st, e.readable, "outside the working directories"); why != "" {
			return why
		}
		return e.wordsOK(f.writes, st, e.inWork, "outside the working directories")
	case f.fsEdit:
		return f.name + " changes files, which asks unless the mode is acceptEdits or a rule allows it"
	}
	return orDefault(f.why, f.name+" is not on the list of commands that only read")
}

// decideShell applies the allow rules and the mode to a command line that
// no deny rule, ask rule or safety check stopped.
func (e *Engine) decideShell(req *request) Decision {
	v := req.view
	plan := e.mode == ModePlan
	ask := func(reason string) Decision {
		if plan {
			reason = "Plan mode: " + reason
		}
		return Decision{Behavior: Ask, Stage: StageMode, Reason: reason, hookAllow: !plan}
	}
	if !plan {
		if c := e.match(ListAllow, req); c != nil {
			return Decision{Behavior: Allow, Stage: StageAllow, Rule: c.Rule.String(), Source: c.layer.Name,
				Reason: "The allow rule " + c.Rule.String() + " from " + c.layer.Name + " covers every use of the tool."}
		}
	}
	switch {
	case !v.sc.ok:
		return ask("gnosh could not read the whole command (" + v.sc.why + "), so a person decides.")
	case v.tooMany:
		return ask("The command holds more nested commands than gnosh will follow, so a person decides.")
	case v.sc.subst:
		return ask("The command runs another command to build part of itself ($(...), backticks or <(...)), and gnosh asks about every command that does.")
	case v.sc.subshell || v.sc.compound:
		return ask("The command holds a subshell, a loop or another compound form, and gnosh asks about those.")
	case v.sc.background:
		return ask("The command starts a background job with &, and gnosh asks about those.")
	case v.sc.exotic:
		return ask("The command holds a ${...} form gnosh does not model, so a person decides.")
	case v.sc.setsVar:
		return ask("The command sets a variable (${name:=value}), and gnosh asks about every command that does.")
	}

	st := &lineState{dirs: []string{e.cur}}
	var rule *crule
	var whys, exact, prefix []string
	coverable := true
	for _, c := range v.sc.cmds {
		st.enter(c)
		pv := e.judgePart(c, st)
		if pv.ok {
			if rule == nil {
				rule = pv.rule
			}
			continue
		}
		whys = append(whys, pv.why)
		if pv.exact == "" {
			coverable = false
			continue
		}
		exact = unionStrings(exact, []string{pv.exact})
		prefix = unionStrings(prefix, []string{orDefault(pv.prefix, pv.exact)})
	}
	if len(whys) == 0 {
		if rule != nil {
			return Decision{Behavior: Allow, Stage: StageAllow, Rule: rule.Rule.String(), Source: rule.layer.Name,
				Reason: "Every part of the command is covered by an allow rule or only reads; the first rule is " + rule.Rule.String() + " from " + rule.layer.Name + "."}
		}
		what := "Every part of the command only reads inside the working directories."
		if e.mode == ModeAcceptEdits {
			what = "Every part of the command only reads, or is a file command acceptEdits lets run, inside the working directories."
		}
		return Decision{Behavior: Allow, Stage: StageMode, Reason: what}
	}
	d := ask("The command needs approval: " + whys[0] + ".")
	if coverable && !plan && len(exact) <= 5 {
		d.Suggestions = append(d.Suggestions, Suggestion{Rules: exact, Scope: GrantLocal})
		if strings.Join(prefix, "\n") != strings.Join(exact, "\n") {
			d.Suggestions = append(d.Suggestions, Suggestion{Rules: prefix, Scope: GrantLocal})
		}
	}
	return d
}

// needsSubcommand lists programs whose first argument picks what they do,
// so that a rule for the bare program would allow far more than the call
// that was approved.
var needsSubcommand = map[string]bool{
	"git": true, "npm": true, "npx": true, "yarn": true, "pnpm": true, "bun": true, "go": true,
	"cargo": true, "docker": true, "kubectl": true, "gh": true, "make": true, "gno": true,
	"gnokey": true, "gnodev": true, "pip": true, "pip3": true, "brew": true, "apt": true,
	"apt-get": true, "dnf": true, "systemctl": true, "terraform": true, "aws": true,
	"gcloud": true, "doctl": true, "helm": true, "poetry": true, "uv": true, "deno": true,
}

// noPrefix lists programs for which only the exact command is ever
// suggested: they run or destroy whatever they are given.
var noPrefix = map[string]bool{
	"rm": true, "rmdir": true, "mv": true, "cp": true, "dd": true, "chmod": true, "chown": true,
	"kill": true, "pkill": true, "killall": true, "sudo": true, "su": true, "doas": true,
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true, "eval": true,
	"exec": true, "env": true, "xargs": true, "find": true, "curl": true, "wget": true,
	"ssh": true, "scp": true, "rsync": true, "nc": true, "python": true, "python3": true,
	"node": true, "ruby": true, "perl": true, "php": true, "lua": true, "awk": true, "sed": true,
	"watch": true, "setsid": true, "ionice": true, "flock": true, "source": true, ".": true,
	"tee": true, "truncate": true, "shred": true, "ln": true, "timeout": true, "nice": true,
	"nohup": true, "time": true, "command": true, "builtin": true, "stdbuf": true, "noglob": true,
	"alias": true, "export": true, "unset": true, "trap": true, "cd": true,
}

// suggestRules returns the rules that would cover a part from now on: the
// exact command, and where it is sensible a prefix rule one step wider.
// For a cd out of the working directories the rule is a Read rule for
// that directory.
func (e *Engine) suggestRules(c *simpleCmd, f cmdFacts, st *lineState) (exact, prefix string) {
	if f.name == "cd" && f.cd != nil {
		if abs, _, ok := e.absAll(*f.cd, st.dirs); ok && len(abs) == 1 {
			return "Read(/" + EscapePath(abs[0]) + "/**)", ""
		}
		return "", ""
	}
	text := rawJoined(c.words)
	if strings.ContainsAny(text, "\n") {
		return "", ""
	}
	// A suggestion is only made when the rule, read back, covers this
	// very command.
	covers := func(rule string) bool {
		r, err := ParseRule(rule)
		if err != nil {
			return false
		}
		cr, err := e.compile(r, ListAllow, e.session)
		return err == nil && cr.kind == kindCommand && cr.cmd.match(text)
	}
	exact = "Bash(" + strings.ReplaceAll(text, "*", "\\*") + ")"
	if !covers(exact) {
		return "", ""
	}
	name := c.words[0].joined()
	if noPrefix[baseName(name)] || strings.Contains(name, "/") || prefixOnlyExact(c.words) || len(c.words) == 1 {
		return exact, ""
	}
	head := name
	if len(c.words) > 1 && subcommandLike(c.words[1]) {
		head += " " + c.words[1].joined()
		if len(c.words) == 2 {
			return exact, ""
		}
	} else if needsSubcommand[name] {
		return exact, ""
	}
	if prefix = "Bash(" + head + " *)"; !covers(prefix) {
		prefix = ""
	}
	return exact, prefix
}

// subcommandLike reports whether a word looks like a subcommand: a short
// plain lower-case word such as "test" or "run-script".
func subcommandLike(w word) bool {
	if !w.plain() {
		return false
	}
	s := w.joined()
	if s == "" || len(s) > 24 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ':') {
			return false
		}
	}
	return true
}

package permission

import (
	"encoding/json"
	"path"
	"strings"
)

// hasGuards reports whether any deny or ask rule of the given kind is in
// force.
func (e *Engine) hasGuards(kind ruleKind) bool {
	for _, list := range []List{ListDeny, ListAsk} {
		for _, c := range e.rules(list) {
			if c.kind == kind {
				return true
			}
		}
	}
	return false
}

// safety runs the checks that no allow rule and no hook approval passes.
// It reports true when one of them settled the call.
func (e *Engine) safety(req *request) (Decision, bool) {
	ask := func(reason string) (Decision, bool) {
		return Decision{Behavior: Ask, Stage: StageSafety, Reason: reason}, true
	}
	bypass := e.mode == ModeBypass
	if v := req.view; v != nil {
		// A guard that could not be checked has not passed.
		if c := e.couldHit(v); c != nil {
			return Decision{Behavior: Ask, Stage: StageSafety, Rule: c.Rule.String(), Source: c.layer.Name,
				Reason: "The " + string(c.list) + " rule " + c.Rule.String() + " from " + c.layer.Name + " could apply to this command, but part of the command is only known when it runs, so the rule could not be checked."}, true
		}
		if v.opaque != "" && e.hasGuards(kindCommand) {
			return ask("Part of this command cannot be read ahead of time (" + v.opaque + "), so the deny and ask rules for commands could not be checked against it.")
		}
		// The same holds for the Read and Edit rules and a file a command
		// names in a way that cannot be known ahead of time. With
		// permissions skipped this one is let go: nearly any variable
		// could name a guarded file, so asking would stop most of what an
		// unattended session runs, and those rules are a guard against
		// the usual way of naming a file, not a wall.
		if req.pathOpaque != "" && !bypass && e.hasGuards(kindPath) {
			return ask("This command cannot be checked against the Read and Edit rules: " + req.pathOpaque + ".")
		}
		// Not so for what the line itself spells out in more values than
		// gnosh follows: padding a line is not a way past a rule.
		if req.pathCapped != "" && e.hasGuards(kindPath) {
			return ask("This command cannot be checked against the Read and Edit rules: " + req.pathCapped + ".")
		}
		dirs := e.lineDirs(v)
		if v.pwds == nil {
			v.pwds = dirs
		}
		fromSubst := map[string]bool{}
		for _, u := range v.units {
			for _, a := range u.cmd.assigns {
				if a.fromSubst {
					fromSubst[a.name] = true
				}
			}
		}
		for _, u := range v.units {
			if why := e.removalDanger(v, u.cmd, dirs, fromSubst); why != "" {
				what := "This removal looks dangerous"
				if !strings.HasPrefix(why, "rm") && !strings.Contains(why, " removes what it finds: ") {
					// (mv, chmod -R, git clean, dd, rsync --delete: not
					// removals by name, and as final.)
					what = "This command could destroy something that has to be kept"
				}
				return Decision{Behavior: Ask, Stage: StageSafety, Critical: true,
					Reason: what + " (" + why + "). No rule approves it; a person decides."}, true
			}
		}
		if v.manyNames != "" {
			// What runs is one of more programs than gnosh follows, each
			// of them written out in the line: an rm among them is not
			// ruled out, and padding a line is not a way past this check.
			return Decision{Behavior: Ask, Stage: StageSafety, Critical: true,
				Reason: "A dangerous removal cannot be ruled out: the line gives the program it runs (" + v.manyNames + ") more values than gnosh follows."}, true
		}
		// A line, or a script inside it that will run (the argument of an
		// eval), that could not be read and that holds an rm: its crude
		// pieces say so, and nothing says what the rm is given.
		crude, why := v.unread, v.opaque
		if !v.sc.ok {
			crude, why = v.frags, v.sc.why
		}
		for _, f := range crude {
			fields := strings.Fields(f)
			for i := 0; i < len(fields) && i < 3; i++ {
				if n := baseName(fields[i]); n == "rm" || n == "rmdir" {
					return Decision{Behavior: Ask, Stage: StageSafety, Critical: true,
						Reason: "A dangerous removal cannot be ruled out: gnosh could not read the whole command (" + why + "), and the shell may read it differently."}, true
				}
			}
			// The other commands that destroy a directory which has to be
			// kept are judged by what the piece says they are handed, when
			// the piece can be read as a line of its own: "coproc mv .git
			// /tmp/g" holds the piece "mv .git /tmp/g".
			if danger := e.crudeDanger(f, dirs); danger != "" {
				return Decision{Behavior: Ask, Stage: StageSafety, Critical: true,
					Reason: "This command could destroy something that has to be kept (" + danger + "). gnosh could not read the whole command (" + why + "), so this is what a piece of it says. No rule approves it; a person decides."}, true
			}
		}
		// And where what could not be read is the very word such a command
		// is handed ("mv ${HOME:0:${#HOME}} /tmp/h"), the pieces say
		// nothing: a line that names one of those commands and the home
		// or the working directory is not ruled out either.
		if len(crude) > 0 && (strings.Contains(v.text, "HOME") || strings.Contains(v.text, "PWD") || strings.Contains(v.text, "~")) {
			for _, f := range crude {
				for _, field := range strings.Fields(f) {
					switch progKey(baseName(field)) {
					case "mv", "chmod", "chown", "chgrp", "truncate", "shred", "rsync":
						return Decision{Behavior: Ask, Stage: StageSafety, Critical: true,
							Reason: "A command that could destroy the home or the working directory cannot be ruled out: gnosh could not read the whole command (" + why + "), and it names " + field + " and one of those directories."}, true
					}
				}
			}
		}
		if !v.sc.ok && e.hasGuards(kindCommand) {
			// The crude pieces of the line met no deny rule (that was
			// checked first), but a crude cut does not undo every way of
			// writing a command. A guard that could not be checked has not
			// passed.
			return ask("gnosh could not read the whole command (" + v.sc.why + "), so the deny and ask rules for commands could not be checked against it.")
		}
		if req.remote != "" && !bypass {
			return Decision{Behavior: Ask, Stage: StageSafety, hookAllow: true,
				Reason: "The command runs on the host " + req.remote + ". Allow rules and the read-only list are about this machine, so a person decides."}, true
		}
	}
	if req.byNumber != "" && (!bypass || e.hasGuards(kindPath)) {
		return ask("This names a file by a number (" + req.byNumber + ") and not by where it is, so the rules about files and the protected paths could not be checked against it.")
	}
	if req.info.class == classFetch && req.host == "" && e.hasGuards(kindDomain) {
		return ask("The URL has no host gnosh can read, so the rules about hosts could not be checked against it.")
	}
	if bypass {
		return Decision{}, false
	}
	for _, p := range req.writes {
		if why := e.protectedWhy(p); why != "" {
			return ask("This would change " + p + ": " + why + ". A change there always asks, whatever rule allows the tool.")
		}
	}
	if req.unresolved {
		return ask("A path in this call has links that could not be followed, so gnosh cannot tell where it really leads.")
	}
	if e.gitRuns && req.view != nil {
		for _, u := range req.view.units {
			if len(u.cmd.words) > 0 {
				if name, ok := e.progName(u.cmd.words[0]); ok && progKey(baseName(name)) == "git" {
					return ask("This repository's own git settings name programs that git would run, and the folder is not trusted, so git commands ask first.")
				}
			}
		}
	}
	return Decision{}, false
}

// crudeDanger reads one crude piece of a line that could not be followed
// and says why a command in it would destroy something that has to be
// kept, or "". The piece is read as a line of its own where it can be;
// where it cannot ("chmod -R 00$[0 ."), its words are taken as they are
// written, from each word on that names such a command.
func (e *Engine) crudeDanger(piece string, dirs []string) string {
	fields := strings.Fields(piece)
	var from []int
	for i, f := range fields {
		switch progKey(baseName(f)) {
		case "mv", "chmod", "chown", "chgrp", "git", "dd", "truncate", "shred", "rsync", "find":
			from = append(from, i)
		}
	}
	if len(from) == 0 {
		return ""
	}
	if pv := e.viewBash(piece); pv.sc.ok {
		for _, u := range pv.units {
			if why := e.removalDanger(pv, u.cmd, dirs, nil); why != "" {
				return why
			}
		}
		return ""
	}
	plain := &bashView{sc: &script{ok: true}, vars: map[string]*lineVar{}}
	for _, i := range from {
		var ws []word
		for _, f := range fields[i:] {
			ws = append(ws, litOf(f))
		}
		if why := e.removalDanger(plain, &simpleCmd{words: ws}, dirs, nil); why != "" {
			return why
		}
	}
	return ""
}

// decideRead judges Read, Grep and Glob: inside the working directories,
// or where an allow rule reaches, they run.
func (e *Engine) decideRead(req *request) Decision {
	if c := e.match(ListAllow, req); c != nil {
		return allowedBy(c)
	}
	var rule *crule
	for _, p := range req.reads {
		in := false
		for _, d := range e.workDirs() {
			in = in || inside(d, p)
		}
		if in {
			continue
		}
		if c := e.allowsPath(p, false); c != nil {
			rule = c
			continue
		}
		dir := path.Dir(p)
		d := Decision{Behavior: Ask, Stage: StageMode, hookAllow: true,
			Reason: p + " is outside the working directories, and reading there asks first."}
		d.Suggestions = []Suggestion{{Rules: []string{"Read(/" + EscapePath(p) + ")"}, Scope: GrantSession}}
		if dir != "/" && e.criticalDir(dir) == "" {
			d.Suggestions = append(d.Suggestions, Suggestion{Rules: []string{"Read(/" + EscapePath(dir) + "/**)"}, Scope: GrantSession})
		}
		return d
	}
	if rule != nil {
		return allowedBy(rule)
	}
	return Decision{Behavior: Allow, Stage: StageMode, Reason: "Reading inside the working directories needs no approval."}
}

// decideEdit judges Edit, Write and NotebookEdit.
func (e *Engine) decideEdit(req *request) Decision {
	if e.mode == ModePlan {
		return Decision{Behavior: Deny, Stage: StageMode,
			Reason: "Plan mode is on: nothing is changed until the plan is approved. Present the plan with ExitPlanMode."}
	}
	if c := e.match(ListAllow, req); c != nil {
		return allowedBy(c)
	}
	var rule *crule
	covered, within := true, true
	for _, p := range req.writes {
		if c := e.allowsPath(p, true); c != nil {
			rule = c
		} else {
			covered = false
		}
		in := false
		for _, d := range e.workDirs() {
			in = in || inside(d, p)
		}
		within = within && in
	}
	if covered && rule != nil {
		return allowedBy(rule)
	}
	if e.mode == ModeAcceptEdits && within {
		return Decision{Behavior: Allow, Stage: StageMode, Reason: "acceptEdits lets file edits inside the working directories run."}
	}
	d := Decision{Behavior: Ask, Stage: StageMode, hookAllow: true, Reason: "Changing a file asks first in this mode."}
	if !within {
		d.Reason = "This changes a file outside the working directories, which asks first."
	}
	var rules []string
	for _, p := range req.writes {
		rules = append(rules, "Edit(/"+EscapePath(p)+")")
	}
	d.Suggestions = []Suggestion{{Rules: rules, Scope: GrantSession}}
	if within {
		d.Suggestions = append(d.Suggestions, Suggestion{Mode: ModeAcceptEdits, Scope: GrantSession})
	}
	return d
}

// decideFetch judges WebFetch.
func (e *Engine) decideFetch(req *request) Decision {
	if c := e.match(ListAllow, req); c != nil {
		return allowedBy(c)
	}
	d := Decision{Behavior: Ask, Stage: StageMode, hookAllow: true, Reason: "Fetching from the web asks first."}
	if req.host == "" {
		d.Reason = "The URL has no host gnosh can read, so a person decides."
		return d
	}
	d.Reason = "Fetching from " + req.host + " asks first."
	d.Suggestions = []Suggestion{{Rules: []string{"WebFetch(domain:" + req.host + ")"}, Scope: GrantLocal}}
	return d
}

// decideOther judges every tool with no rule family of its own: it asks
// unless an allow rule names the tool.
func (e *Engine) decideOther(req *request) Decision {
	plan := e.mode == ModePlan
	// While planning, an approval reaches only what does not change
	// anything: searching the web, and presenting the plan.
	holds := plan && req.canon != toolWebSearch && req.canon != "ExitPlanMode"
	if !holds {
		if c := e.match(ListAllow, req); c != nil {
			return allowedBy(c)
		}
	}
	d := Decision{Behavior: Ask, Stage: StageMode, hookAllow: !holds, Reason: req.canon + " asks first unless an allow rule names it."}
	if holds {
		d.Reason = "Plan mode: " + req.canon + " may change things, so a person decides."
		return d
	}
	switch {
	case req.canon == "ExitPlanMode":
		d.Reason = "The plan is ready; a person approves it or asks for more."
	case req.canon == toolWebSearch:
		d.Suggestions = []Suggestion{{Rules: []string{toolWebSearch}, Scope: GrantLocal}}
	case req.info.class == classSkill:
		name, args := inputString(req.input, "skill"), inputString(req.input, "args")
		name = strings.TrimPrefix(name, "/")
		if name != "" && !strings.ContainsAny(name, " \t*()\\") {
			rule := "Skill(" + name + ")"
			if strings.TrimSpace(args) != "" {
				rule = "Skill(" + name + " *)"
			}
			d.Suggestions = []Suggestion{{Rules: []string{rule}, Scope: GrantSession}}
		}
	default:
		if !hasGlob(req.canon) && !strings.ContainsAny(req.canon, "() \t") {
			d.Suggestions = []Suggestion{{Rules: []string{req.canon}, Scope: GrantSession}}
		}
	}
	return d
}

// inputString returns the string value of one input field, or "".
func inputString(input map[string]json.RawMessage, key string) string {
	var s string
	if raw, ok := input[key]; ok {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

// allowedBy is the decision that an allow rule let the call run.
func allowedBy(c *crule) Decision {
	return Decision{Behavior: Allow, Stage: StageAllow, Rule: c.Rule.String(), Source: c.layer.Name,
		Reason: "The allow rule " + c.Rule.String() + " from " + c.layer.Name + " covers this."}
}

package permission

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// Decision is the engine's answer about one tool call.
type Decision struct {
	// Behavior is Allow, Ask or Deny.
	Behavior Behavior
	// Reason is one plain sentence saying why.
	Reason string
	// Rule is the rule that decided, as written, and Source is where it
	// came from. Both are empty when no rule did.
	Rule   string
	Source string
	// Stage is the step of the decision order that settled the call.
	Stage Stage
	// Critical marks a dangerous removal. No rule and no hook approves
	// one; in ModeBypass the caller asks with a time limit, and refuses
	// where nobody can answer.
	Critical bool
	// Suggestions are the choices a "yes, and don't ask again" answer
	// could make, narrowest first. It is empty when no rule could cover
	// the call, and then only a one-time yes is offered.
	Suggestions []Suggestion
	// Mode is the mode the decision was made in.
	Mode Mode

	// hookAllow says a PreToolUse hook's approval would let the call run.
	hookAllow bool
}

// Suggestion is one way to stop being asked about calls like this one.
type Suggestion struct {
	// Rules are allow rules to add, all of them together.
	Rules []string
	// Scope says where the spec keeps such an approval: for the session
	// only, or in the project's local settings.
	Scope GrantScope
	// Mode, when set, means the suggestion is to switch the session to
	// that mode instead of adding rules.
	Mode Mode
}

// HookVerdict is what PreToolUse hooks said about a call.
type HookVerdict string

const (
	// HookNone: no hook had an opinion.
	HookNone HookVerdict = ""
	// HookAllow: a hook approved the call.
	HookAllow HookVerdict = "allow"
	// HookAsk: a hook wants a person to decide.
	HookAsk HookVerdict = "ask"
	// HookDeny: a hook blocked the call.
	HookDeny HookVerdict = "deny"
)

// WithHook folds in what the PreToolUse hooks said. The engine's own
// decision must have been made on the input the hooks left (a hook may
// rewrite it). A hook's block always wins. A hook's "ask" puts the call
// to a person even when it would have run. A hook's approval skips the
// question the mode would have asked, and nothing else: not a deny rule,
// not an ask rule, not a safety check, not plan mode's hold on changes.
func (d Decision) WithHook(v HookVerdict, reason string) Decision {
	switch v {
	case HookDeny:
		return Decision{Behavior: Deny, Reason: orDefault(reason, "A hook blocked this call."), Stage: StageHook, Mode: d.Mode}
	case HookAsk:
		if d.Behavior == Deny {
			return d
		}
		out := Decision{Behavior: Ask, Reason: orDefault(reason, "A hook asked for a person to decide."), Stage: StageHook, Mode: d.Mode, Critical: d.Critical}
		if d.Mode == ModeDontAsk {
			out.Behavior = Deny
		}
		return out
	case HookAllow:
		if d.hookAllow && d.Behavior != Allow {
			return Decision{Behavior: Allow, Reason: orDefault(reason, "A hook approved this call."), Stage: StageHook, Mode: d.Mode}
		}
	}
	return d
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// request is one tool call with what was worked out about it.
type request struct {
	canon string
	info  toolInfo
	input map[string]json.RawMessage
	// view is the reading of a shell command.
	view *bashView
	// reads and writes are the cleaned absolute paths the call would
	// read and change, each as written and as resolved.
	reads, writes []string
	// unresolved is set when a path's links could not be followed.
	unresolved bool
	// pathOpaque is not empty when a command names a file in a way that
	// cannot be held against the Read and Edit rules. It says how.
	pathOpaque string
	// host is the lower-case host of a fetch; "" when the URL has none.
	host string
	// remote is the declared host a shell command would run on.
	remote string
	// problem is set when the call cannot be judged at all.
	problem string
}

// Decide judges one tool call. tool is the tool's name, call is the
// model's request, and action is the tool's own account of what the call
// would do. The answer for a call that needs approval where nobody can
// give it is the caller's: Ask then means refuse.
func (e *Engine) Decide(tool string, call core.ToolCall, action core.Action) Decision {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.decideLocked(tool, call, action)
}

// DecideAs judges a call as Decide does, but as if the session were in
// another mode. It is for a sub-agent whose definition gives it a mode of
// its own: the rules, the directories and every approval are the
// session's, only the mode differs. A mode gnosh does not have, or
// bypassPermissions where the settings forbid it, refuses the call.
func (e *Engine) DecideAs(mode Mode, tool string, call core.ToolCall, action core.Action) Decision {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if !mode.valid() || mode == ModeBypass && e.noBypass {
		return Decision{Behavior: Deny, Stage: StageMode, Mode: e.mode,
			Reason: "The call was to be judged in the mode \"" + string(mode) + "\", which is not one this session may use."}
	}
	as := *e
	as.mode = mode
	return as.decideLocked(tool, call, action)
}

// decideLocked is Decide with the lock already held.
func (e *Engine) decideLocked(tool string, call core.ToolCall, action core.Action) Decision {
	d := e.decide(CanonicalTool(tool), call, action)
	d.Mode = e.mode
	if d.Behavior == Ask && e.mode == ModeDontAsk {
		d.Behavior = Deny
		d.Reason = strings.TrimSuffix(d.Reason, ".") + "; dontAsk mode refuses what would otherwise ask."
		d.Suggestions = nil
	}
	if d.Behavior != Ask {
		d.Suggestions = nil
	}
	return d
}

func (e *Engine) decide(canon string, call core.ToolCall, action core.Action) Decision {
	if off := e.toolOff(canon); off != nil {
		if off.rule != nil {
			return Decision{Behavior: Deny, Stage: StageTool, Rule: off.rule.Rule.String(), Source: off.rule.layer.Name,
				Reason: "The deny rule " + off.rule.Rule.String() + " switches " + canon + " off."}
		}
		return Decision{Behavior: Deny, Stage: StageTool, Reason: canon + " is switched off: " + off.why + "."}
	}
	req := e.newRequest(canon, call, action)

	// 1. Deny rules, from every layer.
	if c := e.match(ListDeny, req); c != nil {
		return Decision{Behavior: Deny, Stage: StageDeny, Rule: c.Rule.String(), Source: c.layer.Name,
			Reason: "The deny rule " + c.Rule.String() + " from " + c.layer.Name + " refuses this."}
	}
	// 2. Ask rules.
	if c := e.match(ListAsk, req); c != nil {
		return Decision{Behavior: Ask, Stage: StageAsk, Rule: c.Rule.String(), Source: c.layer.Name,
			Reason: "The ask rule " + c.Rule.String() + " from " + c.layer.Name + " wants a person to decide."}
	}
	if req.problem != "" {
		return Decision{Behavior: Ask, Stage: StageSafety, Reason: req.problem}
	}

	// 3. Checks no allow rule passes.
	if d, ok := e.safety(req); ok {
		return d
	}
	if req.info.class == classPerson {
		// The tool is itself a question to the person: no mode approves
		// it on the person's behalf, and no rule needs to.
		if e.mode == ModeDontAsk {
			return Decision{Behavior: Deny, Stage: StageMode, Reason: canon + " asks the person a question, and dontAsk mode never asks."}
		}
		return Decision{Behavior: Allow, Stage: StageMode, Reason: canon + " is itself a question to the person."}
	}
	if e.mode == ModeBypass {
		return Decision{Behavior: Allow, Stage: StageMode, Reason: "Permissions are skipped (bypassPermissions)."}
	}

	// 4. Allow rules, then what the mode lets run.
	switch req.info.class {
	case classShell:
		return e.decideShell(req)
	case classFileRead:
		return e.decideRead(req)
	case classFileEdit:
		return e.decideEdit(req)
	case classFetch:
		return e.decideFetch(req)
	case classAgent, classQuiet:
		return Decision{Behavior: Allow, Stage: StageMode, Reason: canon + " never needs approval; what it starts is judged call by call."}
	}
	return e.decideOther(req)
}

// newRequest works out what a call would touch.
func (e *Engine) newRequest(canon string, call core.ToolCall, action core.Action) *request {
	req := &request{canon: canon}
	req.info, _ = e.names.info(canon)
	if len(call.Input) > 0 {
		_ = json.Unmarshal(call.Input, &req.input)
	}
	str := func(key string) string {
		var s string
		if raw, ok := req.input[key]; ok {
			_ = json.Unmarshal(raw, &s)
		}
		return s
	}
	switch req.info.class {
	case classShell:
		cmd := action.Command
		if in := str("command"); cmd == "" {
			cmd = in
		} else if in != "" && in != cmd {
			req.problem = "The command the tool reports is not the command the model sent, so it cannot be judged."
		}
		req.remote = str("host")
		req.view = e.viewBash(cmd)
		e.shellPaths(req)
		if req.view.sc.ok && len(req.view.sc.cmds) == 0 && req.problem == "" {
			req.problem = canon + " was given no command, so there is nothing to judge it by."
		}
	case classFileRead, classFileEdit:
		paths := action.Paths
		if len(paths) == 0 {
			if p := str(req.info.primary); p != "" {
				paths = []string{p}
			} else if canon == toolGrep || canon == toolGlob {
				paths = []string{e.cur}
			}
		}
		if len(paths) == 0 {
			req.problem = canon + " names no file, so there is nothing to judge it by."
		}
		for _, p := range paths {
			if !strings.HasPrefix(p, "/") {
				req.problem = canon + " was given the path " + p + ", which is not absolute."
			}
		}
		if req.info.class == classFileRead {
			req.reads = e.expand(req, paths)
		} else {
			req.writes = e.expand(req, paths)
		}
	case classFetch:
		raw := action.URL
		if raw == "" {
			raw = str("url")
		}
		if u, err := url.Parse(raw); err == nil {
			req.host = strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
		}
	default:
		// Any other tool that says it reads or changes files is held to
		// the Read and Edit deny and ask rules. Nothing here is ever a
		// reason to allow it.
		switch action.Kind {
		case core.ActionRead:
			req.reads = e.expand(req, absOnly(action.Paths))
		case core.ActionEdit:
			req.writes = e.expand(req, absOnly(action.Paths))
		}
	}
	return req
}

// absOnly keeps the absolute paths.
func absOnly(ps []string) []string {
	var out []string
	for _, p := range ps {
		if strings.HasPrefix(p, "/") {
			out = append(out, p)
		}
	}
	return out
}

// expand cleans each absolute path and adds where it really leads, so
// that a rule is held against the path as written and as resolved: a deny
// rule applies when either matches, an allow rule only when both do.
func (e *Engine) expand(req *request, paths []string) []string {
	var out []string
	add := func(p string) {
		for _, o := range out {
			if o == p {
				return
			}
		}
		out = append(out, p)
	}
	for _, p := range paths {
		if !strings.HasPrefix(p, "/") {
			continue
		}
		p = path.Clean(p)
		add(p)
		if e.resolve == nil {
			continue
		}
		r, err := e.resolve(p)
		if err != nil || !strings.HasPrefix(r, "/") {
			req.unresolved = true
			continue
		}
		add(path.Clean(r))
	}
	return out
}

// match returns the first rule of the list that covers the request.
func (e *Engine) match(list List, req *request) *crule {
	for _, c := range e.rules(list) {
		switch c.kind {
		case kindTool:
			if c.canon == req.canon || c.canon == toolBash && req.canon == toolMonitor {
				return c
			}
		case kindNameGlob:
			if globMatch(c.canon, req.canon) {
				return c
			}
		case kindParam:
			if c.canon == req.canon && paramMatch(req.input, c.param, c.value) {
				return c
			}
		case kindCommand:
			if req.view != nil && list != ListAllow && e.commandHit(c, req.view) {
				return c
			}
		case kindDomain:
			if req.info.class == classFetch && req.host != "" && domainMatch(c.domain, req.host) {
				return c
			}
		case kindAgent:
			if req.info.class == classAgent {
				typ := "general-purpose"
				if raw, ok := req.input["subagent_type"]; ok {
					_ = json.Unmarshal(raw, &typ)
				}
				if typ == c.value {
					return c
				}
			}
		case kindSkill:
			if req.info.class == classSkill && skillMatch(c, req.input, list) {
				return c
			}
		}
	}
	if list != ListAllow {
		// Read rules guard what is read; a Read deny also stops a change
		// to the same path. Edit rules guard what is changed.
		if c := e.pathHit(list, toolRead, append(append([]string(nil), req.reads...), req.writes...)); c != nil {
			if req.canon != toolNotebookEdit || len(req.reads) > 0 {
				return c
			}
		}
		if c := e.pathHit(list, toolEdit, req.writes); c != nil {
			return c
		}
	}
	return nil
}

// commandHit reports whether a deny or ask Bash rule names any command in
// the line: one of its own, one nested in it, or one found inside a
// wrapper or a script.
func (e *Engine) commandHit(c *crule, v *bashView) bool {
	if c.cmd.match(strings.TrimSpace(v.text)) {
		return true
	}
	for _, u := range v.units {
		if u.texts == nil {
			u.texts = e.denyTexts(u.cmd)
		}
		for _, t := range u.texts {
			if c.cmd.match(t) {
				return true
			}
		}
	}
	for _, f := range v.frags {
		if c.cmd.match(f) {
			return true
		}
	}
	return false
}

// pathHit returns the deny or ask rule for tool (Read or Edit) that
// covers one of the paths. Within one layer a "!" pattern takes its
// matches back out of the working-directory rules listed before it,
// except where a rule shut a whole directory above the file.
func (e *Engine) pathHit(list List, tool string, paths []string) *crule {
	for _, ly := range e.all() {
		rs := ly.deny
		if list == ListAsk {
			rs = ly.ask
		}
		for _, p := range paths {
			var hit *crule
			for _, c := range rs {
				if c.kind != kindPath || c.canon != tool {
					continue
				}
				if c.negate {
					if hit != nil && c.path.match(p) {
						hit = nil
					}
					continue
				}
				ok, viaDir := c.path.matchHow(p)
				if !ok {
					continue
				}
				if !c.path.relative || viaDir {
					return c
				}
				hit = c
			}
			if hit != nil {
				return hit
			}
		}
	}
	return nil
}

// allowsPath reports whether an allow rule covers reading (or, with
// edit, changing) the path. An Edit allow also grants reading.
func (e *Engine) allowsPath(p string, edit bool) *crule {
	for _, c := range e.rules(ListAllow) {
		if c.kind != kindPath || edit && c.canon != toolEdit {
			continue
		}
		if c.path.match(p) {
			return c
		}
	}
	return nil
}

// paramMatch reports whether the input has the scalar parameter with a
// value the pattern covers.
func paramMatch(input map[string]json.RawMessage, name, pat string) bool {
	raw, ok := input[name]
	if !ok {
		return false
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s[0] == '{' || s[0] == '[' || s == "null" {
		return false
	}
	if s[0] == '"' {
		var v string
		if json.Unmarshal(raw, &v) != nil {
			return false
		}
		s = v
	}
	return matchValue(pat, s)
}

// skillMatch reports whether a Skill rule covers the call.
func skillMatch(c *crule, input map[string]json.RawMessage, list List) bool {
	var name, args string
	if raw, ok := input["skill"]; ok {
		_ = json.Unmarshal(raw, &name)
	}
	if raw, ok := input["args"]; ok {
		_ = json.Unmarshal(raw, &args)
	}
	if name == "" || strings.TrimPrefix(name, "/") != c.value {
		return false
	}
	// Skill(name) allows the skill with no arguments only. As a deny or
	// ask rule it stops the skill whatever its arguments: a guard must
	// not be dodged by adding a word.
	return c.anyArgs || list != ListAllow || strings.TrimSpace(args) == ""
}

// domainMatch matches a host against a WebFetch pattern. "*" is every
// host; a leading "*." is any subdomain at any depth; any other * stays
// within one label.
func domainMatch(pat, host string) bool {
	if pat == "*" {
		return true
	}
	if rest, ok := strings.CutPrefix(pat, "*."); ok && !strings.Contains(rest, "*") {
		return strings.HasSuffix(host, "."+rest) && len(host) > len(rest)+1
	}
	pl, hl := strings.Split(pat, "."), strings.Split(host, ".")
	if len(pl) != len(hl) {
		return false
	}
	for i := range pl {
		if !globMatch(pl[i], hl[i]) {
			return false
		}
	}
	return true
}

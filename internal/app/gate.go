package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/filelock"
	"github.com/gnosh-ai/gnosh/internal/hooks"
	"github.com/gnosh-ai/gnosh/internal/permission"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/tools/fstools"
	"github.com/gnosh-ai/gnosh/internal/tools/shelltool"
	"github.com/gnosh-ai/gnosh/internal/trust"
)

// gate answers the loop's one question about a tool call: may it run? It
// joins the person's PreToolUse hooks, the permission rules and the person
// in exactly this order:
//
//  1. The PreToolUse hooks, for every call, a sub-agent's included. A
//     block ends it there: the model reads the reason, and no rule is
//     looked at. A hook may also change the call's arguments.
//  2. The permission engine, on the arguments the hooks left, with the
//     hooks' verdict folded in (permission.Decision.WithHook): a deny rule
//     beats a hook's approval, a hook's "ask" asks even when the call
//     would have run, and a hook's approval skips only the question the
//     mode would have asked.
//  3. When that comes to "ask": the PermissionRequest hooks, which may
//     answer in the person's place; and then the person. Where nobody can
//     be asked the call is refused, and the model reads why: a run that
//     nobody attends does not approve its own calls.
//
// A call whose tool says a person must confirm it (core.Action.Confirm:
// it throws away something that cannot be got back) is put to the person
// even when step 2 would let it run, in every mode, skipped permissions
// included. Only the person's own yes runs it: no rule, no hook's
// approval and no PermissionRequest hook stands in, and "don't ask again"
// is not offered. A deny rule still refuses it outright.
//
// A call its tool has already found unable to run is still judged by the
// hooks and the rules, on what it was aimed at, so that a refused path
// does not give away whether it exists; nobody is asked about it.
//
// A call of a feature's tool that reaches beyond what the feature itself
// keeps is judged twice in step 2: as the call of its own tool that it
// is, and as the call of gnosh's own tool that reaches the same thing
// (reach). A call that runs a command on a declared host is also judged
// as the Bash call for that host, so it asks as every command on another
// machine does and the person's deny rules are held against the command
// before it leaves. A call that hands over a file of this machine is also
// judged as the Read of that file, so a file outside the working
// directories asks as reading it does. The stricter answer holds.
type gate struct {
	l *live
}

// critical is how long a person has to answer the question about a
// dangerous removal when permissions are otherwise skipped.
const criticalWait = 2 * time.Minute

// Check decides about one call.
func (g *gate) Check(ctx context.Context, req agent.GateRequest) (agent.Decision, error) {
	l := g.l
	call, action, cannot := req.Call, req.Action, req.CannotRun
	cwd := l.cwd()
	if req.Env != nil && req.Env.Cwd != "" {
		cwd = req.Env.Cwd
	}

	// 1. The PreToolUse hooks.
	verdict, hookReason := permission.HookNone, ""
	var changed json.RawMessage
	if l.hooks.has(hooks.PreToolUse) {
		out, err := l.hooks.run(ctx, hooks.Input{
			Event: hooks.PreToolUse, Cwd: cwd,
			ToolName: call.Name, ToolUseID: call.ID, ToolInput: call.Input,
			AgentID: req.AgentID, AgentType: req.AgentType,
			IfMatch: l.hookIf(cwd, call, action),
		})
		switch {
		case err != nil && ctx.Err() != nil:
			return agent.Decision{}, ctx.Err()
		case err != nil:
			return agent.Decision{}, err
		case out.EndTurn:
			return agent.Decision{Halt: true, Reason: "This call did not run: a PreToolUse hook the user set up ended the turn. " + firstOf(out.StopReason, out.Reason)}, nil
		case out.Blocked:
			return agent.Decision{Reason: "This call did not run: a PreToolUse hook the user set up blocked it. " + firstOf(out.Reason, "It gave no reason.")}, nil
		}
		if out.UpdatedInput != nil && cannot == nil {
			// The rules judge the arguments the tool will run with.
			changed = out.UpdatedInput
			call.Input = changed
			if call.Name == askToolName {
				// A hook may answer the model's questions in the
				// person's place; the tool is told that these arguments
				// are a hook's.
				l.hookChanged.Store(call.ID, true)
			}
			act, err := describe(req.Tool, call, req.Env)
			if err != nil {
				return agent.Decision{Reason: fmt.Sprintf("This call did not run: a PreToolUse hook the user set up changed its arguments, and with them the call cannot run (%v).", err)}, nil
			}
			action = act
		}
		switch out.PermissionDecision {
		case hooks.Allow:
			verdict = permission.HookAllow
		case hooks.Ask:
			verdict = permission.HookAsk
		}
		hookReason = out.Reason
	}

	// 2. The rules and the mode, on the final arguments. A relative path
	// in a command is read from where the shell of the agent that made the
	// call now is: the directory goes with the question, in one step,
	// because the session and its sub-agents share the engine and each has
	// a directory of its own. The engine is given the arguments whole, a
	// shell call's host among them: it judges a call whose host is this
	// machine (left blank, or named "local") as the call for this machine
	// that it is, and any other as a command on another machine, which
	// always asks.
	d := l.reach(cwd, call, action, l.engine.DecideAt(cwd, call.Name, call, action)).WithHook(verdict, hookReason)
	confirm := action.Confirm && cannot == nil
	if confirm {
		// Asked every time: a kept approval would be the rule that this
		// call must not have.
		d.Suggestions = nil
	}
	switch d.Behavior {
	case permission.Allow:
		if !confirm {
			return agent.Decision{Allow: true, Input: changed}, nil
		}
		d.Reason = "This call throws away something that cannot be got back, so gnosh asks before it runs, whatever the rules and the mode allow."
	case permission.Deny:
		return agent.Decision{Reason: "This call did not run. " + d.Reason + " Do the work another way if there is one, or say what you could not do."}, nil
	}

	// 3. Somebody has to say yes.
	if cannot != nil {
		// The call will not run whatever is answered, so nobody is asked.
		// With a person there the model may read the tool's own account of
		// why; without one it reads no more than it would of any call
		// that needs approval.
		if l.interactive {
			return agent.Decision{Allow: true}, nil
		}
		return agent.Decision{Reason: unattended(d), Unattended: true}, nil
	}
	if !confirm && l.hooks.has(hooks.PermissionRequest) {
		out, err := l.hooks.run(ctx, hooks.Input{
			Event: hooks.PermissionRequest, Cwd: cwd,
			ToolName: call.Name, ToolInput: call.Input,
			AgentID: req.AgentID, AgentType: req.AgentType,
			IfMatch: l.hookIf(cwd, call, action),
		})
		switch {
		case err != nil && ctx.Err() != nil:
			return agent.Decision{}, ctx.Err()
		case err != nil:
			l.notice(agent.LevelWarning, "hook:PermissionRequest", fmt.Sprintf("The PermissionRequest hooks could not be run (%v), so the question is put to you.", err))
		case out.PermissionDecision == hooks.Allow:
			return agent.Decision{Allow: true, Input: changed}, nil
		case out.PermissionDecision == hooks.Deny:
			return agent.Decision{Halt: out.EndTurn, Reason: "This call did not run: a PermissionRequest hook the user set up refused it. " + firstOf(out.Reason, out.StopReason)}, nil
		}
	}

	askCtx := ctx
	q := l.permissionQuestion(call, action, d)
	data := QuestionData{Modes: map[string]string{}, CommentOnYes: true}
	for i, s := range d.Suggestions {
		if s.Mode != "" {
			data.Modes["always:"+strconv.Itoa(i)] = string(s.Mode)
		}
	}
	if d.Critical && d.Mode == permission.ModeBypass {
		var cancel context.CancelFunc
		askCtx, cancel = context.WithTimeout(ctx, criticalWait)
		defer cancel()
		data.Until = l.p.now().Add(criticalWait)
	}
	q.Data = data
	ans, err := req.Ask(askCtx, q)
	switch {
	case errors.Is(err, agent.ErrNobodyToAsk):
		return agent.Decision{Reason: unattended(d), Unattended: true}, nil
	case err != nil && ctx.Err() == nil && askCtx.Err() != nil:
		return agent.Decision{Reason: "This call did not run: it is a removal gnosh always asks about, and nobody approved it within two minutes. If it is really wanted, say what you were about to remove and why, and let the user run it."}, nil
	case err != nil:
		return agent.Decision{}, err
	}
	switch {
	case ans.Choice == "yes":
		yesWith(ctx, call, ans)
		return agent.Decision{Allow: true, Input: changed}, nil
	case strings.HasPrefix(ans.Choice, "always:"):
		i, _ := strconv.Atoi(strings.TrimPrefix(ans.Choice, "always:"))
		if i >= 0 && i < len(d.Suggestions) {
			l.keepApproval(d.Suggestions[i])
		}
		yesWith(ctx, call, ans)
		return agent.Decision{Allow: true, Input: changed}, nil
	}
	// No. With words, the model reads them and carries on; without, the
	// person wants the work to stop, not another way round.
	if comment := strings.TrimSpace(ans.Text); comment != "" {
		return agent.Decision{Reason: "This call did not run: the user refused it, and said: " + comment}, nil
	}
	return agent.Decision{Halt: true, Reason: "This call did not run: the user refused it."}, nil
}

// reach judges what a call of a feature's tool reaches beyond the
// feature's own keeping, and returns the answer that holds for the call:
// own, which is the engine's answer for the call as its own tool's, or
// the engine's answer for the call of gnosh's own tool that reaches the
// same thing, whichever is the stricter.
//
// A feature's tool is known to the engine by its name alone: quiet, or
// asking every time. What a call of it touches is said by the tool
// itself, when it describes the call (core.Describer), and is judged as
// the same reach is judged everywhere else:
//
//   - core.ActionExecute with a command, in a call whose "host" argument
//     names another machine (a host that is left out, left blank or named
//     "local" is this machine, as for Bash), is the Bash call that runs
//     that command on that host: it always asks, when anything asks at
//     all; the person's deny rules are held against the command; and with
//     permissions skipped the declaration of the host is the whole gate.
//   - core.ActionRead with paths is the Read of each of those files: one
//     outside the working directories asks, unless a rule allows reading
//     it, exactly as the model's own Read of it would. (Where Read is
//     switched off altogether there is no such reading to judge by, and
//     the tool's own answer stands.)
//
// Nothing here makes an answer milder than the engine's own.
func (l *live) reach(cwd string, call core.ToolCall, action core.Action, own permission.Decision) permission.Decision {
	if own.Behavior == permission.Deny || !l.featureTool(call.Name) {
		return own
	}
	stricter := func(why string, other permission.Decision) {
		if other.Behavior == permission.Allow || own.Behavior == permission.Deny {
			return
		}
		if other.Behavior == permission.Deny || own.Behavior == permission.Allow {
			other.Reason = why + " " + other.Reason
			own = other
		}
	}
	switch action.Kind {
	case core.ActionExecute:
		if strings.TrimSpace(action.Command) == "" {
			return own
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(call.Input, &fields) != nil {
			return own
		}
		host, named := fields["host"]
		if !named {
			return own
		}
		var name string
		if json.Unmarshal(host, &name) == nil {
			if name = strings.TrimSpace(name); name == "" || name == shelltool.LocalName {
				return own
			}
		}
		// Whatever else stands in that place is somewhere else, as the
		// engine reads a Bash call's host: it is handed over as written.
		quoted, err := json.Marshal(action.Command)
		if err != nil {
			return own
		}
		input := json.RawMessage(`{"command":` + string(quoted) + `,"host":` + string(host) + `}`)
		stricter(call.Name+" runs a command on another machine for this call, which is judged as that command is.",
			l.engine.DecideAt(cwd, shelltool.BashName,
				core.ToolCall{ID: call.ID, Name: shelltool.BashName, Input: input},
				core.Action{Kind: core.ActionExecute, Command: action.Command, Summary: action.Summary}))
	case core.ActionRead:
		read := fstools.Defaults().ReadName
		for _, path := range action.Paths {
			if !filepath.IsAbs(path) {
				continue
			}
			quoted, err := json.Marshal(path)
			if err != nil {
				continue
			}
			d := l.engine.DecideAt(cwd, read,
				core.ToolCall{ID: call.ID, Name: read, Input: json.RawMessage(`{"file_path":` + string(quoted) + `}`)},
				core.Action{Kind: core.ActionRead, Paths: []string{path}, Summary: action.Summary})
			if d.Stage == permission.StageTool {
				// Read itself is switched off in this session, which says
				// nothing about this file: the person's deny and ask
				// rules about the path were held against the call
				// already, and the tool's own answer stands.
				continue
			}
			stricter(call.Name+" hands over a file for this call, which is judged as reading that file is.", d)
		}
	}
	return own
}

// featureTool reports whether a tool is one a feature brought, and not
// one of gnosh's own, which the engine knows for what they are.
func (l *live) featureTool(name string) bool {
	for _, f := range l.p.o.Features {
		for _, t := range f.Tools {
			if t.Name == name {
				return true
			}
		}
	}
	return false
}

// yesWith passes on what the person said with a yes: the call runs, and
// the model reads their words with its result.
func yesWith(ctx context.Context, call core.ToolCall, ans agent.Answer) {
	comment := strings.TrimSpace(ans.Text)
	a := agent.FromContext(ctx)
	if comment == "" || a == nil {
		return
	}
	_ = a.Tell(agent.Note{
		Source: "permission",
		Text:   prompt.Wrap("The user approved your " + call.Name + " call, and said with it: " + comment),
	})
}

// hookIf is the "if" filter of the hooks on one call (hooks.Input.IfMatch):
// a hook runs for the call when a deny rule of the same text would refuse
// it or could apply to it, and when the rule cannot be applied at all, of
// which the person was told when the session started. A filter's mistake
// never skips a hook.
func (l *live) hookIf(dir string, call core.ToolCall, action core.Action) func(rule string) bool {
	return func(rule string) bool {
		takes, err := l.engine.Takes(rule, dir, call.Name, call, action)
		return takes || err != nil
	}
}

// checkHookFilters tells the person, as the session starts, of a hook
// whose "if" filter will not do what it seems to: one on an event that is
// not about a tool call, which never runs, and one whose rule cannot be
// applied, which runs for every call its matcher takes.
func (l *live) checkHookFilters() {
	for _, h := range l.cfg.Hooks {
		if h.If == "" {
			continue
		}
		if !hooks.Event(h.Event).OfTool() {
			l.notice(agent.LevelWarning, "hooks", fmt.Sprintf(
				"The %s hook from %s has an \"if\" filter, which only the hooks of tool events have, so it never runs.", h.Event, h.Origin))
			continue
		}
		if err := l.engine.CheckRule(h.If); err != nil {
			var re *permission.RuleError
			if errors.As(err, &re) {
				err = re.Err
			}
			l.notice(agent.LevelWarning, "hooks", fmt.Sprintf(
				"The %s hook from %s has the \"if\" filter %s, which cannot be applied (%s), so it runs for every call its matcher takes.",
				h.Event, h.Origin, h.If, strings.TrimRight(err.Error(), ".")))
		}
	}
}

// describe asks a tool what a call would do, and survives a tool that
// crashes over it.
func describe(tool core.Tool, call core.ToolCall, env *core.ToolEnv) (act core.Action, err error) {
	d, ok := tool.(core.Describer)
	if !ok {
		return core.Action{Kind: core.ActionOther}, nil
	}
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("the tool crashed looking at them: %v", p)
		}
	}()
	return d.Describe(call, env)
}

// unattended is what the model reads when a call needs a person's yes and
// nobody is there to give it.
func unattended(d permission.Decision) string {
	return "This call did not run: it needs a person's approval, and nobody is here to give it (" + strings.TrimSuffix(d.Reason, ".") +
		"). Do the work another way if there is one, or say what you could not do."
}

// permissionQuestion writes the question about one call: exactly what
// would run, why gnosh asks, and the answers there are. What the model and
// the files it read brought is shown escaped, never as terminal control
// sequences.
func (l *live) permissionQuestion(call core.ToolCall, action core.Action, d permission.Decision) agent.Question {
	if call.Name == exitPlanName {
		return l.planQuestion(call, d)
	}
	var b strings.Builder
	// The tool's own line about the call, unless it only repeats the
	// command that is shown in full below it.
	summary := strings.TrimSpace(action.Summary)
	if summary != "" && summary != strings.TrimSpace(action.Command) {
		b.WriteString(showLines(summary) + "\n")
	}
	switch {
	case action.Command != "":
		for _, line := range strings.Split(action.Command, "\n") {
			b.WriteString("  $ " + trust.Escape(line) + "\n")
		}
	case len(action.Paths) > 0:
		for i, p := range action.Paths {
			switch {
			case i > 0:
				b.WriteString("  (which really is " + trust.Escape(p) + ")\n")
			case !strings.Contains(summary, p):
				b.WriteString("  " + trust.Escape(p) + "\n")
			}
		}
	case action.URL != "":
		b.WriteString("  " + trust.Escape(action.URL) + "\n")
	default:
		b.WriteString("  " + trust.Escape(oneLine(string(call.Input), 600)) + "\n")
	}
	if d.Reason != "" {
		b.WriteString("Why gnosh asks: " + d.Reason)
	}
	choices := []agent.Choice{{Key: "yes", Label: "Yes, this once"}}
	for i, s := range d.Suggestions {
		label := ""
		switch {
		case s.Mode != "":
			label = fmt.Sprintf("Yes, and switch to %s for the rest of the session", s.Mode)
		case s.Scope == permission.GrantLocal:
			label = fmt.Sprintf("Yes, and don't ask again for %s (kept in %s)", trust.Escape(strings.Join(s.Rules, ", ")), l.cfg.Paths.LocalSettings)
		default:
			label = fmt.Sprintf("Yes, and don't ask again this session for %s", trust.Escape(strings.Join(s.Rules, ", ")))
		}
		choices = append(choices, agent.Choice{Key: "always:" + strconv.Itoa(i), Label: label})
	}
	choices = append(choices, agent.Choice{Key: "no", Label: "No (alone it stops the turn; with words after it, the model reads them and carries on)"})
	return agent.Question{
		Kind:    "permission",
		Title:   "Allow " + call.Name + "?",
		Text:    strings.TrimRight(b.String(), "\n"),
		Choices: choices,
	}
}

// planQuestion is the question ExitPlanMode puts: the plan, and whether to
// carry it out, asking about each edit or accepting them, or to keep
// planning.
func (l *live) planQuestion(call core.ToolCall, d permission.Decision) agent.Question {
	var in struct {
		Plan string `json:"plan"`
	}
	_ = json.Unmarshal(call.Input, &in)
	plan := strings.TrimSpace(in.Plan)
	if plan == "" {
		plan = "(The model gave no plan.)"
	}
	choices := []agent.Choice{{Key: "yes", Label: "Yes, carry it out, asking about each edit"}}
	for i, s := range d.Suggestions {
		if s.Mode == permission.ModeAcceptEdits {
			choices = append(choices, agent.Choice{Key: "always:" + strconv.Itoa(i), Label: "Yes, carry it out, and accept its edits without asking"})
		}
	}
	choices = append(choices, agent.Choice{Key: "no", Label: "No, keep planning (say what to change after it; alone it stops the turn)"})
	return agent.Question{
		Kind:    "permission",
		Title:   "Carry out this plan?",
		Text:    showLines(plan),
		Choices: choices,
	}
}

// showLines escapes text for the screen a line at a time, so that its line
// breaks stay.
func showLines(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = trust.Escape(line)
	}
	return strings.Join(lines, "\n")
}

// keepApproval acts on a "yes, and don't ask again": the suggested rules
// are in force from the next call on, and the ones that are to be kept are
// written to the settings file the config names for that.
func (l *live) keepApproval(s permission.Suggestion) {
	if s.Mode != "" {
		if err := l.setMode(s.Mode); err != nil {
			l.notice(agent.LevelWarning, "permission", fmt.Sprintf("The permission mode could not be changed (%v).", err))
			return
		}
		l.notice(agent.LevelInfo, "permission", fmt.Sprintf("The permission mode is %s for the rest of the session.", s.Mode))
		return
	}
	var kept []string
	for _, rule := range s.Rules {
		if err := l.engine.Grant(rule, s.Scope); err != nil {
			l.notice(agent.LevelWarning, "permission", fmt.Sprintf("The approval could not be kept (%v), so gnosh will ask again.", err))
			continue
		}
		kept = append(kept, rule)
	}
	if s.Scope != permission.GrantLocal || len(kept) == 0 {
		return
	}
	if err := l.saveRules(kept); err != nil {
		l.notice(agent.LevelWarning, "permission", fmt.Sprintf("The approval holds for this session, but it could not be written down (%v), so gnosh will ask again next time.", err))
	}
}

// saveRules adds allow rules to the project's local settings file, the one
// place gnosh itself writes settings. The file as it would then be is
// checked the way every config file is checked before a byte is written,
// so that gnosh can never write a file it would then refuse to load.
//
// The file is read as every config file is, so comments a person put in it
// and a comma after a last item do not stop an approval from being saved;
// but it is written again whole, as plain JSON, and they are not in what
// is written.
func (l *live) saveRules(rules []string) error {
	path := l.cfg.Paths.LocalSettings
	dir := filepath.Dir(path)
	_, statErr := os.Stat(dir)
	fresh := errors.Is(statErr, os.ErrNotExist)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if fresh {
		// The project's data folder is never committed.
		_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o600)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	release, err := filelock.File(ctx, path+".lock")
	if err != nil {
		return fmt.Errorf("the settings file is being written by another gnosh: %w", err)
	}
	defer release()

	doc := map[string]json.RawMessage{}
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		plain, err := config.StandardJSON(data)
		if err == nil {
			err = json.Unmarshal(plain, &doc)
		}
		if err != nil {
			return fmt.Errorf("%s is not JSON gnosh can read (%v); it was left as it is", path, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	perms := map[string]json.RawMessage{}
	if raw, ok := doc["permissions"]; ok {
		if err := json.Unmarshal(raw, &perms); err != nil {
			return fmt.Errorf("the \"permissions\" of %s are not an object (%v); the file was left as it is", path, err)
		}
	}
	var allow []string
	if raw, ok := perms["allow"]; ok {
		if err := json.Unmarshal(raw, &allow); err != nil {
			return fmt.Errorf("the \"permissions.allow\" of %s is not a list of rules (%v); the file was left as it is", path, err)
		}
	}
	for _, r := range rules {
		if !slices.Contains(allow, r) {
			allow = append(allow, r)
		}
	}
	var err2 error
	if perms["allow"], err2 = json.Marshal(allow); err2 != nil {
		return err2
	}
	if doc["permissions"], err2 = json.Marshal(perms); err2 != nil {
		return err2
	}
	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if _, err := config.CheckWrite(l.st.inputs, path, content); err != nil {
		return fmt.Errorf("gnosh would not load %s with the rule in it, so it was not written: %w", path, err)
	}
	return config.WriteFileAtomic(path, content, 0o600)
}

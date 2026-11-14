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
	// in a command is read from where the session's shell now is.
	l.engine.SetCwd(cwd)
	d := l.engine.Decide(call.Name, call, action).WithHook(verdict, hookReason)
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
		if err := json.Unmarshal(data, &doc); err != nil {
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

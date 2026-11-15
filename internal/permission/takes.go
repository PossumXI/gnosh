package permission

import (
	"path"
	"strings"
	"sync"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// Takes is a hook's "if" filter (spec/cc/hooks.md): it reports whether a
// deny rule written as rule would refuse the call, or could apply to it
// because part of the call is only known when it runs, so that a hook
// meant for some calls is never skipped for one of them. The rule is read
// as the engine reads its own, against the call with the shell in dir (as
// DecideAt has it), and nothing else of the session's plays a part: not
// its other rules, its mode, its approvals or --tools. An error means the
// rule cannot be applied, as a settings file holding it would be refused.
func (e *Engine) Takes(rule, dir, tool string, call core.ToolCall, action core.Action) (bool, error) {
	e.mu.RLock()
	one := *e
	e.mu.RUnlock()
	one.mu = &sync.RWMutex{}
	one.notes = nil
	ly, err := one.newLayer(Layer{Scope: ScopeCLI, Name: `a hook's "if"`, Deny: []string{rule}})
	if err != nil {
		return false, err
	}
	one.layers = []*layer{ly}
	one.session = &layer{Layer: Layer{Scope: ScopeSession}, base: one.cwd}
	one.saved = &layer{Layer: Layer{Scope: ScopeLocal}, base: one.cwd}
	one.restrict, one.only, one.mode = false, nil, ModeDefault
	if strings.HasPrefix(dir, "/") {
		one.cur = path.Clean(dir)
	}
	canon := CanonicalTool(tool)
	if one.toolOff(canon) != nil {
		return true, nil
	}
	req := one.newRequest(canon, call, action)
	if one.match(ListDeny, req) != nil || req.problem != "" {
		return true, nil
	}
	if v := req.view; v != nil && (one.couldHit(v) != nil || v.opaque != "" && one.hasGuards(kindCommand)) {
		return true, nil
	}
	unsure := req.pathOpaque != "" || req.pathCapped != "" || req.byNumber != "" || req.unresolved
	return unsure && one.hasGuards(kindPath), nil
}

// CheckRule returns the error Takes would give for rule, ahead of any
// call: nil when the rule can be applied.
func (e *Engine) CheckRule(rule string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, err := e.newLayer(Layer{Scope: ScopeCLI, Name: `a hook's "if"`, Deny: []string{rule}})
	return err
}

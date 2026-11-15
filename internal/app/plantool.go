package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/permission"
)

// This file is EnterPlanMode and ExitPlanMode (spec/cc/tools.md section
// 14, permissions.md 2.3): the model's own switch into plan mode, where it
// looks and plans and changes nothing, and its way out, which is to
// present the plan. ExitPlanMode always asks (the engine's decideOther):
// the person carries the plan out, asking about each edit (the session
// goes back to the default mode) or accepting them (acceptEdits, the
// question's second choice), or keeps the session planning, and what they
// say comes back to the model. Neither is a sub-agent's.

const (
	enterPlanName = "EnterPlanMode"
	exitPlanName  = "ExitPlanMode"
)

// enterPlanTool is EnterPlanMode.
type enterPlanTool struct{ l *live }

func (t *enterPlanTool) Spec() core.ToolSpec {
	return core.ToolSpec{
		Name:        enterPlanName,
		Description: "Switches to plan mode, for a task that needs a plan first: you look and change nothing, then present the plan with ExitPlanMode. Not for a small, clear change.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (t *enterPlanTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	return core.Action{Kind: core.ActionOther, Summary: "Switch to plan mode"}, nil
}

func (t *enterPlanTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	if t.l.engine.Mode() == permission.ModePlan {
		return core.TextResult("Plan mode is on already. Look and plan; present the plan with ExitPlanMode."), nil
	}
	if err := t.l.setMode(permission.ModePlan); err != nil {
		return core.ErrorResult("Plan mode could not be switched on: " + strings.TrimRight(err.Error(), ".") + "."), nil
	}
	return core.TextResult("Plan mode is on: read, search and think, and change nothing. When the plan is ready, present it with ExitPlanMode; the user approves it or asks for changes."), nil
}

// exitPlanTool is ExitPlanMode.
type exitPlanTool struct{ l *live }

func (t *exitPlanTool) Spec() core.ToolSpec {
	return core.ToolSpec{
		Name:        exitPlanName,
		Description: "In plan mode, presents the finished plan for the user to approve; approved, the session leaves plan mode and you carry it out. The plan is Markdown: what changes where, in what order, and how you will check it.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"plan":{"type":"string"}},"required":["plan"],"additionalProperties":false}`),
	}
}

func (t *exitPlanTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	return core.Action{Kind: core.ActionOther, Summary: "Present the plan for approval"}, nil
}

func (t *exitPlanTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	l := t.l
	// The person said yes. Accepting the edits has switched the mode
	// already; a plain yes goes back to the default mode.
	if l.engine.Mode() == permission.ModePlan {
		if err := l.setMode(permission.ModeDefault); err != nil {
			return core.ErrorResult("The plan was approved, and plan mode could not be left: " + strings.TrimRight(err.Error(), ".") + "."), nil
		}
	}
	return core.TextResult(fmt.Sprintf("The user approved the plan. The session is in the %s mode now: carry the plan out.", l.engine.Mode())), nil
}

package memjoin

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/app/agentdef"
	"github.com/gnosh-ai/gnosh/internal/core"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
)

// This file says who is given which gnovm tools, and when they run
// (integration-contract.md 4.12, 5.1.2 and 7.2; mem.md section 17).
//
// A session's tools are fixed when the session starts: the loop is handed
// one list, and the model server reads that list as part of the prompt it
// keeps. So what a permission mode allows cannot be said by the list. It
// is said the way gnosh says it for its own tools: a tool that may not
// run in plan mode stays in the list and is refused when it is called.

// planMode reports whether the session is in plan mode at this moment.
// The person may switch the mode while the session runs, so nothing
// keeps the answer: whatever depends on it asks when it acts.
func (j *join) planMode() bool {
	if j.s.Mode != nil {
		return j.s.Mode() == "plan"
	}
	return j.s.Config != nil && j.s.Config.Permissions.Mode.V == "plan"
}

// gated is a gnovm tool that runs code or changes a machine, as a session
// has it. In plan mode the model may look and not change, and has
// gnovm_list alone: a call of any other gnovm tool is answered with what
// plan mode means for the machines and for mem, and nothing is run. The
// moment the session leaves plan mode the same call runs.
type gated struct {
	core.Tool
	j *join
}

// Describe says what the call would do, as the tool itself says it. In
// plan mode it says besides that the call cannot run, so that nobody is
// asked about a call that will be refused, and the model reads why.
func (t gated) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	act := core.Action{Kind: core.ActionOther}
	var err error
	if d, ok := t.Tool.(core.Describer); ok {
		act, err = d.Describe(call, env)
	}
	if t.j.planMode() {
		return act, errors.New(t.j.texts.PlanRefusal(t.Spec().Name))
	}
	return act, err
}

// Run runs the call, unless the session is in plan mode.
func (t gated) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	if t.j.planMode() {
		return core.ErrorResult(t.j.texts.PlanRefusal(t.Spec().Name)), nil
	}
	return t.Tool.Run(ctx, call, env)
}

// given returns the gnovm tools of set that the session is given, in the
// order the model is told of them, and reports whether gnovm_run is among
// them.
//
// The session a person is in is given all four. A sub-agent is given the
// ones its definition names, and so the stock kinds, which name none, are
// given none: mem is the plan of the session the person is in, and a
// helper that was not written for it is kept out of it. gnovm_list runs
// in every mode; the other three are gated.
func (j *join) given(set *gnovmtools.Set) (tools []core.Tool, run bool) {
	offered := func(name string) bool { return j.s.Offered == nil || j.s.Offered(name) }
	named := func(string) bool { return true }
	if !j.main {
		def, ok := j.definition()
		named = func(name string) bool {
			return ok && def.ToolsSet && slices.Contains(def.Tools, name) && !slices.Contains(def.Disallowed, name)
		}
	}
	for _, t := range set.Tools() {
		name := t.Spec().Name
		if !named(name) {
			continue
		}
		if name == gnovmtools.RunToolName {
			run = offered(name)
		}
		if name != gnovmtools.ListToolName {
			t = gated{Tool: t, j: j}
		}
		tools = append(tools, t)
	}
	return tools, run
}

// definition returns the definition of the kind of sub-agent this session
// is. It is found where the Agent tool finds it (the wiring's agentDefs):
// gnosh's own kinds, the person's (~/.claude/agents), and, in a folder
// the person trusts, the project's (.claude/agents in the working
// directory, in every folder above it up to where the project begins,
// and in the directories added to the session); in safe mode gnosh's own
// alone. The session's facts name only the kind, so the file is read
// again here; a kind that is not found is given nothing.
func (j *join) definition() (agentdef.Def, bool) {
	s := j.s
	if s.Config != nil && s.Config.Switches.SafeMode.V {
		return agentdef.Load("", nil).Get(s.AgentType)
	}
	var project []string
	if s.Trusted {
		root := s.Dirs.ProjectRoot
		for dir := s.Dirs.Cwd; dir != ""; dir = filepath.Dir(dir) {
			project = append(project, filepath.Join(dir, ".claude", "agents"))
			if dir == root || dir == filepath.Dir(dir) || !strings.HasPrefix(dir, root) {
				break
			}
		}
		for _, r := range j.env().Roots {
			if d := filepath.Join(r, ".claude", "agents"); !slices.Contains(project, d) {
				project = append(project, d)
			}
		}
	}
	user := ""
	if s.Dirs.Home != "" {
		user = filepath.Join(s.Dirs.Home, ".claude", "agents")
	}
	return agentdef.Load(user, project).Get(s.AgentType)
}

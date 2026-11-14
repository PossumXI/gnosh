package app

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/tools/fstools"
	"github.com/gnosh-ai/gnosh/internal/tools/shelltool"
)

// looker is a tool whose calls only look at things, so that the loop may
// run several of them side by side (agent.Concurrent). The file tools that
// read are wrapped in it; the tools that change something are not, and run
// alone and in order.
type looker struct {
	core.Tool
}

// Parallel says a call may run beside other calls that only look.
func (looker) Parallel(core.ToolCall) bool { return true }

// Describe hands the call on to the tool's own account of what it would
// do.
func (t looker) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	if d, ok := t.Tool.(core.Describer); ok {
		return d.Describe(call, env)
	}
	return core.Action{Kind: core.ActionOther}, nil
}

// buildTools makes the session's tools: gnosh's own first, then the
// features', in the fixed order the model is told of them. A tool that is
// switched off (--tools left it out, or a deny rule names the whole tool)
// is not in the list at all.
func (l *live) buildTools() error {
	limits := l.cfg.Limits
	fo := fstools.Options{HomeDir: l.cfg.Paths.Home}
	if limits.FileReadMaxTokens.V > 0 {
		// The limit is given in tokens, and the file tools count bytes: a
		// token is about four of them.
		fo.ReadMaxBytes = limits.FileReadMaxTokens.V * 4
	}
	if limits.GlobTimeout.Set {
		fo.SearchTimeout = limits.GlobTimeout.V
	}
	names := fstools.Defaults()
	var all []core.Tool
	for _, t := range fstools.New(fo) {
		switch t.Spec().Name {
		case names.ReadName, names.GlobName, names.GrepName:
			all = append(all, looker{t})
		default:
			all = append(all, t)
		}
	}

	so := shelltool.Options{
		Environ:             func() []string { return slices.Clone(l.p.o.Environ) },
		Env:                 l.cfg.CommandEnv(),
		SpoolDir:            l.spoolDir(),
		Now:                 l.p.now,
		DefaultTimeout:      limits.BashDefaultTimeout.V,
		MaxTimeout:          limits.BashMaxTimeout.V,
		TimeoutToBackground: true,
		MaxBackground:       l.cfg.Options.MaxBackgroundJobs.V,
		MaxOutputChars:      limits.BashMaxOutput.V,
		NoOutputTool:        true,
	}
	if l.interactive {
		// Only a session somebody attends is told when a background
		// command ends; a run would be gone, and the tool's words to the
		// model must not promise it.
		so.OnEnd = l.backgroundEnded
	}
	l.shell = shelltool.New(so)
	all = append(all, l.shell.Tools()...)
	// gnosh's own tools, as a sub-agent starts from them.
	l.own = l.own[:0]
	for _, t := range all {
		if l.engine.ToolEnabled(t.Spec().Name) {
			l.own = append(l.own, t)
		}
	}
	// The tools a sub-agent never has: its own Agent tool is made for it
	// at its depth, and only the session the person is in may ask them a
	// question.
	all = append(all, &agentTool{l: l}, &askTool{l: l})
	if len(l.forModel()) > 0 {
		// The Skill tool is there when there is a skill a model may
		// load, as the session starts.
		all = append(all, &skillTool{l: l})
	}

	claimed := map[string]string{}
	for _, f := range l.p.o.Features {
		for _, t := range f.Tools {
			claimed[t.Name] = f.Name
		}
	}
	for _, c := range l.contribs {
		for _, t := range c.c.Tools {
			name := t.Spec().Name
			if claimed[name] != c.f.Name {
				return fmt.Errorf("the feature %q gave this session a tool named %q, which is not in its list of tool names. A tool whose name is in no list could not be named by a rule, a flag or a hook, so gnosh does not start with it", c.f.Name, name)
			}
			all = append(all, t)
		}
	}

	l.tools = l.tools[:0]
	seen := map[string]bool{}
	for _, t := range all {
		name := t.Spec().Name
		if seen[name] {
			return fmt.Errorf("two tools are named %q", name)
		}
		seen[name] = true
		if l.engine.ToolEnabled(name) {
			l.tools = append(l.tools, t)
		}
	}
	return nil
}

// toolNames lists the names of the session's tools, in order.
func (l *live) toolNames() []string {
	names := make([]string, len(l.tools))
	for i, t := range l.tools {
		names[i] = t.Spec().Name
	}
	return names
}

// toolSpecs lists what the model is told about the session's tools.
func (l *live) toolSpecs() []core.ToolSpec {
	specs := make([]core.ToolSpec, len(l.tools))
	for i, t := range l.tools {
		specs[i] = t.Spec()
	}
	return specs
}

// closeShell stops every command the session started, with everything
// those commands started, and waits until they are gone. It is safe to
// call more than once.
func (l *live) closeShell() {
	if l.shell == nil {
		return
	}
	done := make(chan error, 1)
	go func() { done <- l.shell.Close() }()
	select {
	case err := <-done:
		if err != nil {
			l.p.warnf("not everything the session's commands started could be stopped: %v", err)
		}
	case <-time.After(60 * time.Second):
		l.p.warnf("the session's commands had not all stopped after a minute; gnosh goes on without waiting for them.")
	}
}

// waitIdle waits until the session has nothing in hand, or until ctx ends.
func (l *live) waitIdle(ctx context.Context) error {
	return l.agent.Wait(ctx)
}

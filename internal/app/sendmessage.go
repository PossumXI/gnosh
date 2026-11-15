package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/app/agentdef"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// This file is SendMessage: the model gives a sub-agent it started, and
// that has ended, more to do (spec/cc/subagents.md 9). The sub-agent runs
// again in the background under the same id, with everything it had from
// its runs before (its session file is its history) and the tools of its
// kind, and its report comes as a notice, as a background sub-agent's
// does. Explore and Plan are one-shot, as Claude Code's are; a sub-agent
// that was stopped is not taken up again; and one that worked in a git
// work tree of its own is not, since the work tree is not opened again.

// sendMessageName is the tool's name, as Claude Code's.
const sendMessageName = "SendMessage"

// The meta keys a sub-agent's session file holds beside its kind.
const (
	metaEnded    = "ended"    // how its last run ended: completed, partial, failed, halted, stopped
	metaWorkTree = "workTree" // the git work tree of its own it worked in
)

// sendMessageTool is SendMessage.
type sendMessageTool struct{ l *live }

// sendMessageInput is what a SendMessage call is given.
type sendMessageInput struct {
	To      string `json:"to"`
	Message string `json:"message"`
	Summary string `json:"summary"`
}

// resumable reports whether a sub-agent of the kind def can be given more
// to do once it ended.
func resumable(def agentdef.Def) bool {
	return def.Name != "Explore" && def.Name != "Plan" && def.Isolation == ""
}

// subagentsDir is the folder of the session's sub-agents' session files.
func (l *live) subagentsDir() string {
	return filepath.Join(l.st.dirs(l.ts.ID()).Session, "subagents")
}

// Spec says what the model is told about the tool.
func (t *sendMessageTool) Spec() core.ToolSpec {
	return core.ToolSpec{
		Name: sendMessageName,
		Description: "Gives a sub-agent you started, which has ended, more to do: it works on your message in the background with everything it knew from before, and its report comes to you as a notice from gnosh. " +
			"to is its id, as its report gave it (agent id: ...). Explore and Plan sub-agents cannot be given more, nor one that was stopped or that worked in a work tree of its own; start a new one with Agent instead.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"to":{"type":"string","description":"The sub-agent's id."},"message":{"type":"string","description":"What it is to do now, complete in itself as far as what it knew does not cover it."},"summary":{"type":"string","description":"Three to five words that say what it is doing now, for the person watching."}},"required":["to","message"],"additionalProperties":false}`),
	}
}

// Describe says what a call would do; it is never asked about, as
// starting a sub-agent is not.
func (t *sendMessageTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	var in sendMessageInput
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return core.Action{Kind: core.ActionOther}, fmt.Errorf("its arguments cannot be read (%v)", err)
	}
	return core.Action{Kind: core.ActionOther, Summary: "Give the sub-agent " + oneLine(in.To, 40) + " more to do: " + oneLine(firstOf(in.Summary, in.Message), 100)}, nil
}

// Run takes the sub-agent up again, in the background.
func (t *sendMessageTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	l := t.l
	var in sendMessageInput
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return core.ErrorResult(fmt.Sprintf("Nothing was sent: the arguments cannot be read (%v).", err)), nil
	}
	to, message := strings.TrimSpace(in.To), strings.TrimSpace(in.Message)
	if to == "" || message == "" {
		return core.ErrorResult("Nothing was sent: give both to, the sub-agent's id, and message, what it is to do."), nil
	}
	refuse := func(format string, args ...any) (core.ToolResult, error) {
		return core.ErrorResult("Nothing was sent: " + fmt.Sprintf(format, args...) + "."), nil
	}
	for _, b := range l.backgroundAgents() {
		if b.id == to {
			return refuse("the sub-agent %s is still running; its report comes as a notice when it ends, and TaskStop stops it", to)
		}
	}
	meta, err := subagentMeta(ctx, l.subagentsDir(), to)
	if err != nil {
		return refuse("%v", err)
	}
	if len(l.backgroundAgents()) >= maxBackgroundAgents {
		return refuse("%d sub-agents run in the background already, which is the most there may be at once", maxBackgroundAgents)
	}
	def, ok := l.agentDefs().Get(meta[metaAgentType])
	switch {
	case !ok:
		return refuse("the kind of sub-agent %s was, %q, is no longer there", to, meta[metaAgentType])
	case meta[metaWorkTree] != "":
		return refuse("the sub-agent %s worked in a git work tree of its own (%s), which is not opened again; start a new one with Agent, isolation worktree, and tell it what the first did", to, meta[metaWorkTree])
	case !resumable(def):
		return refuse("a %s sub-agent does one task and is not given more; start a new one with Agent", def.Name)
	case meta[metaEnded] == "stopped":
		return refuse("the sub-agent %s was stopped, and a stopped sub-agent is not taken up again; start a new one with Agent", to)
	case meta[metaEnded] == "":
		return refuse("the sub-agent %s never ended a run that could be taken up", to)
	}
	if why := l.cannotRunAgent(def); why != "" {
		return refuse("%s", why)
	}
	if why := l.backgroundOff(nil); why != "" {
		return refuse("it would run in the background, and %s", why)
	}
	s, err := l.newSub(l.ctx, l.agent, nil, def, call.ID, "", true, to)
	if err != nil {
		return refuse("%s", strings.TrimRight(err.Error(), "."))
	}
	out := l.startBackground(s, agentInput{Description: firstOf(strings.TrimSpace(in.Summary), "more to do"), Prompt: message})
	return out, nil
}

// metaAgentType is the meta key of a sub-agent's kind.
const metaAgentType = "agentType"

// subagentMeta reads the meta entries of the sub-agent id's session file in
// dir, the last value of each key; an error says, for the model, why there
// is none.
func subagentMeta(ctx context.Context, dir, id string) (map[string]string, error) {
	if strings.ContainsAny(id, `/\`) || strings.HasPrefix(id, ".") {
		return nil, fmt.Errorf("%q is no sub-agent's id", id)
	}
	store := &transcript.Store{Dir: dir}
	ts, err := store.OpenReadOnly(ctx, id)
	if errors.Is(err, transcript.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("no sub-agent of this session has the id %q", id)
	}
	if err != nil {
		return nil, fmt.Errorf("the session file of the sub-agent %s could not be read (%v)", id, err)
	}
	defer ts.Close()
	meta := map[string]string{}
	for _, e := range ts.Entries() {
		if e.Type == transcript.TypeMeta {
			meta[e.Key] = e.Value
		}
	}
	if meta[metaAgentType] == "" {
		return nil, fmt.Errorf("no sub-agent of this session has the id %q", id)
	}
	return meta, nil
}

var _ core.Tool = (*sendMessageTool)(nil)

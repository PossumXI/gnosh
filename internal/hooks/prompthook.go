package hooks

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// This file is prompt hooks, Claude Code's "type": "prompt": in place of a
// command, the hook is a rule or a condition the model judges ("Block any
// Bash command that reads .env files", "All the tests pass"), with the
// event's input put in for $ARGUMENTS (or after the prompt). The model
// answers with {"ok": true} or {"ok": false, "reason": "..."}, and
// "impossible": true beside a false when the condition can never be met.
// The answer is turned into what a command hook would have printed for the
// event, and read as that is:
//
//   - Stop and SubagentStop: "ok": false keeps the turn going, its reason
//     the model's next instruction ("decision": "block"), unless the
//     model said it is impossible, which lets the turn end;
//   - PreToolUse: "ok": false refuses the call and ends the turn, or with
//     continueOnBlock gives the model the reason as the call's error;
//   - PostToolUse: "ok": false ends the turn, or with continueOnBlock
//     gives the model the reason;
//   - PostToolBatch, UserPromptSubmit and UserPromptExpansion: "ok": false
//     blocks, with the reason;
//   - PostToolUseFailure: the reason goes to the model beside the error;
//   - PermissionRequest: an answer decides nothing (a command hook does).
//
// A model that does not answer in time, or not with that JSON, is a hook
// that could not run, as a command that failed is. The model asked is
// the one gnosh asks its own side questions (Options.Prompt).

// promptEvents are the events a prompt hook runs on.
var promptEvents = map[Event]bool{
	PreToolUse: true, PostToolUse: true, PostToolUseFailure: true, PostToolBatch: true,
	Stop: true, SubagentStop: true, UserPromptSubmit: true, UserPromptExpansion: true, PermissionRequest: true,
}

// promptTimeoutDefault is how long a prompt hook's model is given when the
// hook sets no timeout, as in Claude Code.
const promptTimeoutDefault = 30 * time.Second

// judgement is a prompt hook's answer.
type judgement struct {
	OK         *bool  `json:"ok"`
	Reason     string `json:"reason"`
	Impossible bool   `json:"impossible"`
}

// askModel runs a prompt hook: the prompt, with the event in it, goes to
// the model through Options.Prompt, and its answer becomes what a command
// hook would have printed.
func (r *Runner) askModel(ctx context.Context, h Hook, event []byte, limit time.Duration) (p proc) {
	p.exit, p.limit = -1, limit
	began := r.now()
	defer func() { p.took = r.now().Sub(began) }()
	if r.opts.Prompt == nil {
		p.startErr = errors.New("this gnosh asks no model for a hook")
		return p
	}
	text := h.Prompt
	if strings.Contains(text, "$ARGUMENTS") {
		text = strings.ReplaceAll(text, "$ARGUMENTS", string(event))
	} else {
		text += "\n\n" + string(event)
	}
	cctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	answer, err := r.opts.Prompt(cctx, h.Model, text)
	switch {
	case ctx.Err() != nil:
		p.cancelled = true
		return p
	case cctx.Err() != nil:
		p.timedOut = true
		return p
	case err != nil:
		p.startErr = err
		return p
	}
	v, err := readVerdict(answer)
	if err != nil {
		p.unknown = "the model did not answer with the JSON a prompt hook asks for (" + err.Error() + ")"
		p.stderr = answer
		return p
	}
	p.exit, p.stdout = 0, verdictOutput(h.Event, v, h.ContinueOnBlock)
	return p
}

// readVerdict finds the answer in what the model wrote: the whole text, or
// the first JSON object in it that says "ok" (a model may put words or a
// code block around it).
func readVerdict(text string) (judgement, error) {
	try := func(s string) (judgement, bool) {
		var v judgement
		if json.Unmarshal([]byte(s), &v) == nil && v.OK != nil {
			return v, true
		}
		return judgement{}, false
	}
	t := strings.TrimSpace(text)
	if v, ok := try(t); ok {
		return v, nil
	}
	for i := 0; i < len(t); i++ {
		if t[i] != '{' {
			continue
		}
		var raw json.RawMessage
		if json.NewDecoder(strings.NewReader(t[i:])).Decode(&raw) == nil {
			if v, ok := try(string(raw)); ok {
				return v, nil
			}
		}
	}
	return judgement{}, errors.New(`no JSON object with "ok" in it`)
}

// verdictOutput is what a command hook would print for the answer, on the
// event.
func verdictOutput(e Event, v judgement, continueOnBlock bool) string {
	if *v.OK {
		return "{}"
	}
	reason := strings.TrimSpace(v.Reason)
	if reason == "" {
		reason = "the hook's model answered \"ok\": false and gave no reason"
	}
	out := map[string]any{}
	switch e {
	case Stop, SubagentStop:
		if v.Impossible {
			return "{}"
		}
		out["decision"], out["reason"] = "block", reason
	case PreToolUse:
		out["hookSpecificOutput"] = map[string]any{"hookEventName": string(e), "permissionDecision": "deny", "permissionDecisionReason": reason}
		if !continueOnBlock {
			out["continue"], out["stopReason"] = false, reason
		}
	case PostToolUse:
		if continueOnBlock {
			out["decision"], out["reason"] = "block", reason
		} else {
			out["continue"], out["stopReason"] = false, reason
		}
	case PostToolBatch, UserPromptSubmit, UserPromptExpansion:
		out["decision"], out["reason"] = "block", reason
	case PostToolUseFailure:
		out["hookSpecificOutput"] = map[string]any{"hookEventName": string(e), "additionalContext": reason}
	default:
		return "{}"
	}
	data, _ := json.Marshal(out)
	return string(data)
}

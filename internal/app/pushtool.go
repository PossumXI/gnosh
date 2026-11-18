package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// This file is PushNotification, Claude Code's tool by which the model
// calls the person back to the session: a long task done while they were
// away, a decision only they can make. gnosh sends it the ways it already
// tells a person who is away that the session waits: a desktop note
// through the terminal, by their channel (preferredNotifChannel: OSC 9 or
// 99, or the bell), and their Notification hooks, with the type
// push_notification, which a hook can forward to a phone. When the person
// is at the terminal (a key pressed in the last half minute) nothing is
// sent, as in Claude Code: what the model writes reaches them already.

const pushName = "PushNotification"

// pushWait is how long the tool waits for the front end's word.
const pushWait = 2 * time.Second

// pushTool is PushNotification.
type pushTool struct{ l *live }

func (t *pushTool) Spec() core.ToolSpec {
	return core.ToolSpec{
		Name: pushName,
		Description: "Calls the user back to this session with a notification, when they may have walked away and something here is worth coming back for: a long task finished, a build is ready, a decision only they can make. " +
			"It pulls them from whatever they are doing, so do not use it for routine progress or a quick answer they are watching for; use it when they asked to be told. " +
			"One line under 200 characters, no markdown, leading with what they would act on (\"build failed: 2 auth tests\"). When they are at the terminal nothing is sent, and the result says so.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"message":{"type":"string","minLength":1,"description":"The notification: one line, under 200 characters."},"status":{"const":"proactive"}},"required":["message"],"additionalProperties":false}`),
	}
}

func (t *pushTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	return core.Action{Kind: core.ActionOther, Summary: "Notify the user"}, nil
}

func (t *pushTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	var in struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(call.Input, &in); err != nil {
		return core.ErrorResult("The input could not be read: " + err.Error()), nil
	}
	msg := pushText(in.Message)
	if msg == "" {
		return core.ErrorResult("The message is empty: nothing was sent."), nil
	}
	reply := make(chan string, 1)
	t.l.deliver(&PushEvent{Message: msg, Reply: reply})
	select {
	case said := <-reply:
		return core.TextResult("Notification " + said + "."), nil
	case <-time.After(pushWait):
		return core.TextResult("Notification not sent: this session's interface does not send notifications."), nil
	case <-ctx.Done():
		return core.ErrorResult("The call was stopped before the notification was sent."), nil
	}
}

// pushText is a notification's text: one line, its blanks folded, cut to
// 200 characters.
func pushText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= 200 {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:199])) + "…"
}

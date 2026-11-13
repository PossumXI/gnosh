// Copyright (C) 2026 Jae Kwon
//
// This file is part of gnosh's GnoVM machine subsystem. It is licensed under
// the GNO Network General Public License, version 6, as published by
// NewTendermint, LLC or its successor, or any later version of that License
// that the Licensor accepts as the proxy under its section 14, by a statement
// published in the canonical copy of internal/gnovm/LICENSE.md; together with
// the Additional Terms (Strong Attribution) in that file.

package gnovmtools

import (
	"context"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/gnovm"
)

// TodoStatus says where a line of a session's task list stands.
type TodoStatus string

// The three places a line can stand in, spelled as Claude Code spells them.
const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusInProgress TodoStatus = "in_progress"
	TodoStatusCompleted  TodoStatus = "completed"
)

// Todo is one line of a session's task list, as mem's todos are shown in
// it: what the line says, where it stands, and how it reads while it is the
// one being done (for mem's todos, the same words).
type Todo struct {
	Content    string     `json:"content"`
	Status     TodoStatus `json:"status"`
	ActiveForm string     `json:"activeForm"`
}

// TodoSetter is what SessionTodos writes a session's task list with. A
// list that cannot be written is the setter's to tell the person of.
type TodoSetter interface {
	SetTodos(ctx context.Context, sessionID string, todos []Todo)
}

// SessionTodos shows the goal a mem call changed as the session's task
// list, the pills under its prompt: mem is the one task list, and a
// session's todos are only ever written from it. Of several goals one call
// changed, the last is shown.
func SessionTodos(sessions TodoSetter) MemGoalsFunc {
	return func(ctx context.Context, sessionID string, goals []gnovm.MemGoal) {
		if sessionID == "" || len(goals) == 0 {
			return
		}
		sessions.SetTodos(ctx, sessionID, MemTodos(goals[len(goals)-1], sessionID))
	}
}

// MemTodos is goal g as a session's todos: its steps, the one this session
// started, or else the one its next is in, in progress; the other open ones
// pending; the done ones completed. Steps dropped or forgotten are left
// out. A goal with no steps is one todo.
func MemTodos(g gnovm.MemGoal, sessionID string) []Todo {
	if len(g.Steps) == 0 {
		status := TodoStatusInProgress
		if g.Status == "done" {
			status = TodoStatusCompleted
		}
		return []Todo{{Content: g.Title, Status: status, ActiveForm: g.Title}}
	}
	started := false
	for _, s := range g.Steps {
		started = started || s.Status == "" && s.Doing == sessionID
	}
	var todos []Todo
	for _, s := range g.Steps {
		status := TodoStatusPending
		switch {
		case s.Status == "done":
			status = TodoStatusCompleted
		case s.Status != "":
			continue
		case started && s.Doing == sessionID,
			!started && (g.Next == s.Name || strings.HasPrefix(g.Next, s.Name+"/")):
			status = TodoStatusInProgress
		}
		todos = append(todos, Todo{Content: s.Title, Status: status, ActiveForm: s.Title})
	}
	return todos
}

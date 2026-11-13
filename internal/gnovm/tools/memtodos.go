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
	"strconv"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/gnovm"
)

// TaskLine is one line of a session's task list, as a goal of mem's is
// shown in it: a step of the goal, or the goal itself when it has no
// steps.
type TaskLine struct {
	// Title is the todo's words, as mem holds them.
	Title string
	// AtWork is how the line reads while it is the one being worked. For
	// a todo of mem's that is the same words; whoever shows the list may
	// say more in their place, such as where the session stands.
	AtWork string
	// State is how far the todo has come.
	State LineState
}

// LineState is how far the todo of a TaskLine has come, as the session
// that is shown the list sees it.
type LineState int

const (
	// LineOpen is a todo still to be done, and not the one being worked.
	LineOpen LineState = iota
	// LineWorking is the todo being worked: the one the session started,
	// or else the one mem would give it next.
	LineWorking
	// LineDone is a todo closed as done.
	LineDone
)

// String names the state in a word: "open", "working" or "done".
func (s LineState) String() string {
	switch s {
	case LineOpen:
		return "open"
	case LineWorking:
		return "working"
	case LineDone:
		return "done"
	}
	return "LineState(" + strconv.Itoa(int(s)) + ")"
}

// TodoSetter is what SessionTodos writes a session's task list with. A
// list that cannot be written is the setter's to tell the person of.
type TodoSetter interface {
	SetTodos(ctx context.Context, sessionID string, todos []TaskLine)
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
func MemTodos(g gnovm.MemGoal, sessionID string) []TaskLine {
	if len(g.Steps) == 0 {
		state := LineWorking
		if g.Status == "done" {
			state = LineDone
		}
		return []TaskLine{{Title: g.Title, AtWork: g.Title, State: state}}
	}
	started := false
	for _, s := range g.Steps {
		started = started || s.Status == "" && s.Doing == sessionID
	}
	var todos []TaskLine
	for _, s := range g.Steps {
		state := LineOpen
		switch {
		case s.Status == "done":
			state = LineDone
		case s.Status != "":
			continue
		case started && s.Doing == sessionID,
			!started && (g.Next == s.Name || strings.HasPrefix(g.Next, s.Name+"/")):
			state = LineWorking
		}
		todos = append(todos, TaskLine{Title: s.Title, AtWork: s.Title, State: state})
	}
	return todos
}

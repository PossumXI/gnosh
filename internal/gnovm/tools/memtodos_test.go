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
	"sync"
	"testing"
)

// todoSetter keeps what SessionTodos writes.
type todoSetter struct {
	mu    sync.Mutex
	todos map[string][]TaskLine
}

func (s *todoSetter) SetTodos(_ context.Context, sessionID string, todos []TaskLine) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.todos[sessionID] = todos
}

// mem is the one task list: a goal a call to it plans or changes is what
// the session's todos show, its next step in progress; a read changes
// nothing, and dropped steps are left out.
func TestMemGoalIsTheSessionsTodos(t *testing.T) {
	setter := &todoSetter{todos: map[string][]TaskLine{}}
	run := toolsOver(newLazy(t), Options{OnMemGoals: SessionTodos(setter)}).Tool(RunToolName)
	todo := func(title string, state LineState) TaskLine {
		return TaskLine{Title: title, AtWork: title, State: state}
	}

	call(t, run, `{"name": "mem", "code": "s := mem.Todo(\"ship it\"); s.Todo(\"build\"); s.Todo(\"test\"); s.Todo(\"tag\")"}`)
	require.Equal(t, []TaskLine{
		todo("build", LineWorking),
		todo("test", LineOpen),
		todo("tag", LineOpen),
	}, setter.todos["session-1"])

	call(t, run, `{"name": "mem", "code": "mem.Done(\"ship/build\", \"built\")\nmem.Drop(\"ship/tag\", \"no tag this time\")"}`)
	require.Equal(t, []TaskLine{
		todo("build", LineDone),
		todo("test", LineWorking),
	}, setter.todos["session-1"])

	delete(setter.todos, "session-1")
	call(t, run, `{"name": "mem", "code": "mem.Show()"}`)
	require.Empty(t, setter.todos, "a read changes nothing")

	call(t, run, `{"name": "mem", "code": "mem.Done(\"ship/test\", \"ok\")\nmem.Done(\"ship\", \"shipped\")"}`)
	require.Equal(t, []TaskLine{
		todo("build", LineDone),
		todo("test", LineDone),
	}, setter.todos["session-1"])
}

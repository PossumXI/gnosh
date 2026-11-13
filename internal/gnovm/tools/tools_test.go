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
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
)

// The tests share the process-wide client, so none is parallel.

func newLazy(t *testing.T) *gnovm.Lazy {
	t.Helper()
	l := gnovm.NewLazy(gnovm.ClientOptions{Options: gnovm.Options{Dir: t.TempDir()}, InProcess: true})
	t.Cleanup(l.Close)
	return l
}

func call(t *testing.T, tool core.Tool, input string) response {
	t.Helper()
	return callAs(t, context.Background(), "session-1", tool, "call-"+t.Name(), input)
}

func TestRunToolKeepsStateAndShowsValues(t *testing.T) {
	l := newLazy(t)
	run := toolsOver(l, Options{}).Tool(RunToolName)
	resp := call(t, run, `{"code": "x := 20\nprintln(\"hi\")"}`)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "hi")
	require.Contains(t, resp.Content, "kept as entry 1")
	require.Contains(t, resp.Content, "new scratch VM")

	resp = call(t, run, `{"code": "x * 2"}`)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "40\n")
	require.Contains(t, resp.Content, "nothing to keep")
}

func TestRunToolErrorsSayWhatToDo(t *testing.T) {
	l := newLazy(t)
	run := toolsOver(l, Options{}).Tool(RunToolName)
	resp := call(t, run, `{"code": "panic(\"boom\")"}`)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "ERROR — nothing from this call was kept: panic: boom")

	resp = call(t, run, `{"code": "for {}", "gas": 1e6}`)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "out of gas")
	require.Contains(t, resp.Content, `"gas": 4000000`)

	resp = call(t, run, `{"code": "var keep [][]byte\nfor i := 0; i < 2000; i++ { keep = append(keep, make([]byte, 10000)) }"}`)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "memory cap exceeded")
	require.Contains(t, resp.Content, `gnovm_new {"name": "scratch", "memory_mb": 32}`)

	resp = call(t, run, `{"code": "func ("}`)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "ERROR — nothing ran: syntax error")

	resp = call(t, run, `{"code": "1", "gas": -5}`)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "gas must be")
}

func TestNewListDropTools(t *testing.T) {
	l := newLazy(t)
	ts := toolsOver(l, Options{})
	run, drop := ts.Tool(RunToolName), ts.Tool(DropToolName)
	resp := call(t, ts.Tool(NewToolName), `{"name": "notes", "persist": true, "memory_mb": 8, "description": "my notes"}`)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "Made VM notes with memory_mb 8")
	require.Contains(t, resp.Content, "persistent")

	resp = call(t, run, `{"name": "notes", "code": "var items []string"}`)
	require.False(t, resp.IsError, resp.Content)

	// The session's own scratch VM can be resized by the name "scratch".
	call(t, run, `{"code": "y := 1"}`)
	resp = call(t, ts.Tool(NewToolName), `{"name": "scratch", "memory_mb": 24}`)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "memory_mb 24")

	resp = call(t, ts.Tool(ListToolName), `{}`)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "- notes: memory_mb 8")
	require.Contains(t, resp.Content, "my notes")
	require.Contains(t, resp.Content, "importable: fmt, ")
	require.Contains(t, resp.Content, "strings")
	require.NotContains(t, resp.Content, "chain/banker")
	require.Contains(t, resp.Content, "Built on gno.land")

	// Deleting asks the user first: the tool says what would be deleted,
	// and the harness asks before the call comes here.
	resp = call(t, drop, `{"name": "notes", "action": "evict"}`)
	require.False(t, resp.IsError, resp.Content)
	resp = call(t, drop, `{"name": "notes", "action": "delete"}`)
	require.False(t, resp.IsError, resp.Content)
	resp = call(t, run, `{"name": "notes", "code": "1"}`)
	require.True(t, resp.IsError)
	require.Contains(t, resp.Content, "there is no VM named notes")
}

func TestPrograms(t *testing.T) {
	l := newLazy(t)
	resp := call(t, toolsOver(l, Options{}).Tool(RunToolName), `{"code": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"from a program\") }"}`)
	require.False(t, resp.IsError, resp.Content)
	require.Contains(t, resp.Content, "from a program")
	require.Contains(t, resp.Content, "a fresh program VM")
}

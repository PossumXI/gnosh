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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
)

// require and assert are the checks these tests make, on the standard
// library alone: a check of require's that fails ends the test, one of
// assert's lets it go on. Each takes, after what it checks, an optional
// message: one value, or a format and its arguments.
var (
	require = checks{stop: true}
	assert  = checks{}
)

type checks struct{ stop bool }

// fail reports a failed check with what it found and the test's own words.
func (c checks) fail(t *testing.T, found string, msg []any) bool {
	t.Helper()
	switch {
	case len(msg) == 1:
		found += "\n" + fmt.Sprint(msg[0])
	case len(msg) > 1:
		if format, ok := msg[0].(string); ok {
			found += "\n" + fmt.Sprintf(format, msg[1:]...)
		} else {
			found += "\n" + fmt.Sprint(msg...)
		}
	}
	if c.stop {
		t.Fatal(found)
	}
	t.Error(found)
	return false
}

func (c checks) NoError(t *testing.T, err error, msg ...any) bool {
	t.Helper()
	return err == nil || c.fail(t, fmt.Sprintf("an error where none was wanted: %v", err), msg)
}

func (c checks) True(t *testing.T, v bool, msg ...any) bool {
	t.Helper()
	return v || c.fail(t, "false where true was wanted", msg)
}

func (c checks) False(t *testing.T, v bool, msg ...any) bool {
	t.Helper()
	return !v || c.fail(t, "true where false was wanted", msg)
}

func (c checks) Equal(t *testing.T, want, got any, msg ...any) bool {
	t.Helper()
	return reflect.DeepEqual(want, got) || c.fail(t, fmt.Sprintf("not equal:\nwant: %#v\ngot:  %#v", want, got), msg)
}

// Contains checks that a string holds a piece, or a list a value.
func (c checks) Contains(t *testing.T, in, piece any, msg ...any) bool {
	t.Helper()
	return holds(in, piece) || c.fail(t, fmt.Sprintf("%#v does not hold %#v", in, piece), msg)
}

func (c checks) NotContains(t *testing.T, in, piece any, msg ...any) bool {
	t.Helper()
	return !holds(in, piece) || c.fail(t, fmt.Sprintf("%#v should not hold %#v", in, piece), msg)
}

func (c checks) Len(t *testing.T, v any, n int, msg ...any) bool {
	t.Helper()
	got := reflect.ValueOf(v).Len()
	return got == n || c.fail(t, fmt.Sprintf("%#v has %d, where %d were wanted", v, got, n), msg)
}

func (c checks) Empty(t *testing.T, v any, msg ...any) bool {
	t.Helper()
	return empty(v) || c.fail(t, fmt.Sprintf("%#v where nothing was wanted", v), msg)
}

func (c checks) NotEmpty(t *testing.T, v any, msg ...any) bool {
	t.Helper()
	return !empty(v) || c.fail(t, "nothing where something was wanted", msg)
}

func (c checks) Nil(t *testing.T, v any, msg ...any) bool {
	t.Helper()
	return isNil(v) || c.fail(t, fmt.Sprintf("%#v where nil was wanted", v), msg)
}

func (c checks) NotNil(t *testing.T, v any, msg ...any) bool {
	t.Helper()
	return !isNil(v) || c.fail(t, "nil where something was wanted", msg)
}

func (c checks) GreaterOrEqual(t *testing.T, got, least int, msg ...any) bool {
	t.Helper()
	return got >= least || c.fail(t, fmt.Sprintf("%d where at least %d was wanted", got, least), msg)
}

func (c checks) Positive(t *testing.T, got int, msg ...any) bool {
	t.Helper()
	return got > 0 || c.fail(t, fmt.Sprintf("%d where more than none was wanted", got), msg)
}

// holds reports whether in, a string or a list, has piece in it.
func holds(in, piece any) bool {
	if s, ok := in.(string); ok {
		p, ok := piece.(string)
		return ok && strings.Contains(s, p)
	}
	v := reflect.ValueOf(in)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return false
	}
	for i := 0; i < v.Len(); i++ {
		if reflect.DeepEqual(v.Index(i).Interface(), piece) {
			return true
		}
	}
	return false
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	switch r := reflect.ValueOf(v); r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}

func empty(v any) bool {
	if isNil(v) {
		return true
	}
	switch r := reflect.ValueOf(v); r.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array, reflect.Chan:
		return r.Len() == 0
	default:
		return r.IsZero()
	}
}

// response is a tool's answer as these tests read it: the text the model is
// shown, whether it tells of a failure, and what the harness is given with
// it.
type response struct {
	Content string
	IsError bool
	Meta    map[string]any
	Display *core.Display
}

// testEnv is what the harness gives a call of session's.
func testEnv(session string) *core.ToolEnv {
	return &core.ToolEnv{Cwd: "/proj", Roots: []string{"/proj"}, SessionID: session, Store: new(sync.Map)}
}

// callAs runs one call of tool's as session, with ctx, and returns its
// answer. The call must not fail as a harness's own failure does.
func callAs(t *testing.T, ctx context.Context, session string, tool core.Tool, id, input string) response {
	t.Helper()
	res, err := tool.Run(ctx, core.ToolCall{ID: id, Name: tool.Spec().Name, Input: json.RawMessage(input)}, testEnv(session))
	require.NoError(t, err)
	return response{Content: core.TextOf(res.Content), IsError: res.IsError, Meta: res.Meta, Display: res.Display}
}

// toolsOver is the four tools over the client l holds, for the project
// /proj, with whatever else o sets.
func toolsOver(l *gnovm.Lazy, o Options) *Set {
	o.Client, o.Project = Lazy(l), "/proj"
	return New(o)
}

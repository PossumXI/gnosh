package permission

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// rules is a short way to write a test's rule lists.
type rules struct {
	allow, ask, deny []string
}

// testOptions returns options for a project at /proj owned by a user
// whose home is /home/u, with the rules in one user-settings layer.
func testOptions(mode Mode, r rules) Options {
	return Options{
		Mode:    mode,
		Cwd:     "/proj",
		Home:    "/home/u",
		Trusted: true,
		Layers: []Layer{{
			Scope: ScopeUser, Name: "user settings", Base: "/home/u/.config/gnosh",
			Allow: r.allow, Ask: r.ask, Deny: r.deny,
		}},
	}
}

// mustEngine builds an engine or fails the test.
func mustEngine(t testing.TB, o Options) *Engine {
	t.Helper()
	e, err := New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return e
}

// engine builds the usual test engine.
func engine(t testing.TB, mode Mode, r rules) *Engine {
	t.Helper()
	return mustEngine(t, testOptions(mode, r))
}

// input makes a tool call's JSON arguments.
func input(kv ...any) json.RawMessage {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	b, _ := json.Marshal(m)
	return b
}

// bash asks the engine about a shell command.
func bash(e *Engine, cmd string) Decision {
	return e.Decide("Bash", core.ToolCall{Name: "Bash", Input: input("command", cmd)},
		core.Action{Kind: core.ActionExecute, Command: cmd})
}

// file asks the engine about a file tool's call on one path.
func file(e *Engine, tool, path string) Decision {
	kind := core.ActionRead
	key := "file_path"
	switch tool {
	case "Edit", "Write":
		kind = core.ActionEdit
	case "NotebookEdit":
		kind, key = core.ActionEdit, "notebook_path"
	case "Grep", "Glob":
		key = "path"
	}
	return e.Decide(tool, core.ToolCall{Name: tool, Input: input(key, path)},
		core.Action{Kind: kind, Paths: []string{path}})
}

// fetch asks the engine about a WebFetch of a URL.
func fetch(e *Engine, url string) Decision {
	return e.Decide("WebFetch", core.ToolCall{Name: "WebFetch", Input: input("url", url, "prompt", "x")},
		core.Action{Kind: core.ActionNetwork, URL: url})
}

// other asks the engine about a tool with no file, command or URL.
func other(e *Engine, tool string, kv ...any) Decision {
	return e.Decide(tool, core.ToolCall{Name: tool, Input: input(kv...)}, core.Action{Kind: core.ActionOther})
}

// suggested flattens a decision's suggestions for comparison: the rules
// of each suggestion joined by " + ", a mode change written "mode:name".
func suggested(d Decision) []string {
	var out []string
	for _, s := range d.Suggestions {
		if s.Mode != "" {
			out = append(out, "mode:"+string(s.Mode))
			continue
		}
		out = append(out, strings.Join(s.Rules, " + "))
	}
	return out
}

// fakeFS is a made-up file system for the Resolve and Glob options.
type fakeFS struct {
	// links maps a path to where it really leads.
	links map[string]string
	// files are the paths a pattern can match.
	files []string
}

func (f fakeFS) resolve(p string) (string, error) {
	// The longest linked prefix wins, as a link in the middle of a path
	// would.
	best := ""
	for from := range f.links {
		if (p == from || strings.HasPrefix(p, from+"/")) && len(from) > len(best) {
			best = from
		}
	}
	if best == "" {
		return p, nil
	}
	return f.links[best] + p[len(best):], nil
}

func (f fakeFS) glob(pattern string) ([]string, error) {
	pat := pathPattern{segs: splitPath(pattern)}
	var out []string
	for _, file := range f.files {
		if pat.match(file) {
			out = append(out, file)
		}
	}
	return out, nil
}

// coreCall makes a tool call with the given arguments.
func coreCall(tool string, kv ...any) core.ToolCall {
	return core.ToolCall{Name: tool, Input: input(kv...)}
}

// coreAction is a shell tool's account of a command.
func coreAction(cmd string) core.Action {
	return core.Action{Kind: core.ActionExecute, Command: cmd}
}

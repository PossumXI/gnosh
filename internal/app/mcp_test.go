package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// fakeMCPServer is a small MCP server in sh: it answers the handshake, lists
// one tool, greet, and greets whoever the call names.
const fakeMCPServer = `while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"greeter","version":"0.1"},"capabilities":{"tools":{}}}}\n' "$id" ;;
    *'"method":"tools/list"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"greet","description":"Greets someone by name.","inputSchema":{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}}]}}\n' "$id" ;;
    *'"method":"tools/call"'*)
      who=$(printf '%s' "$line" | sed -n 's/.*"arguments":{"name":"\([^"]*\)".*/\1/p')
      printf '{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"hello, %s"}]}}\n' "$id" "$who" ;;
  esac
done`

// fakeMCPHTTP is a small MCP server at a URL: it answers the handshake,
// lists one tool, wave, and waves back. It notes the Authorization header
// of each request.
func fakeMCPHTTP(t *testing.T, auth *[]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*auth = append(*auth, r.Header.Get("Authorization"))
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&m)
		w.Header().Set("Content-Type", "application/json")
		switch m.Method {
		case "initialize":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"far","version":"1"}}}`, m.ID)
		case "tools/list":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"wave","description":"Waves from afar."}]}}`, m.ID)
		case "tools/call":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"waved back"}]}}`, m.ID)
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// MCP servers from the user's mcp.json, one started and one at a URL: their
// tools are offered as mcp__<server>__<tool>, a call reaches each and its
// answer reaches the model; /mcp says how they stand; and stream-json's
// init line names them.
func TestMCPServerTools(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			{ID: "c1", Name: "mcp__greeter__greet", Args: `{"name":"Jae"}`},
			{ID: "c2", Name: "mcp__remote__wave", Args: `{}`},
		}},
		apptest.Reply{Text: "Greeted."},
	)
	server := w.Script(w.Home+"/bin/greeter.sh", fakeMCPServer)
	var auth []string
	far := fakeMCPHTTP(t, &auth)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {
		"greeter": {"command": "`+server+`"},
		"broken": {"command": "/no/such/server"},
		"remote": {"type": "http", "url": "`+far.URL+`/mcp?key=k9", "headers": {"Authorization": "Bearer t0k"}}
	}}`)

	run(t, w, "-y", "-p", "greet Jae").wantCode(t, 0)
	reqs := w.Model.Requests()
	has(t, "the tools the model was offered", string(reqs[0].Body), "mcp__greeter__greet", "Greets someone by name.", "mcp__remote__wave", "Waves from afar.")
	has(t, "what the model read of the call", toolResult(t, w, 1, "c1"), "hello, Jae")
	has(t, "what the model read of the far call", toolResult(t, w, 1, "c2"), "waved back")
	if len(auth) == 0 || auth[0] != "Bearer t0k" {
		t.Errorf("the configured header did not go: %q", auth)
	}

	r := run(t, w, "-p", "/mcp").wantCode(t, 0)
	has(t, "/mcp", r.out, "greeter: connected (greeter 0.1; user, ", "1 tool: greet",
		"broken: not connected", "/no/such/server",
		"remote: connected (far 1; user, ", ", at "+far.URL+"), 1 tool: wave")
	if strings.Contains(r.out, "k9") {
		t.Errorf("/mcp showed the key in a URL:\n%s", r.out)
	}

	r = run(t, w, "-p", "--output-format", "stream-json", "--verbose", "/mcp").wantCode(t, 0)
	has(t, "the init line", r.out, `"mcp_servers":[{"name":"broken","status":"failed"},{"name":"greeter","status":"connected"},{"name":"remote","status":"connected"}]`)
}

// A project's .mcp.json counts only in a folder the person has trusted,
// and a call of a server's tool asks first, like any tool no rule allows.
func TestMCPProjectServersAndPermission(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "mcp__repo__greet", Args: `{"name":"you"}`}}},
		apptest.Reply{Text: "It needs approval."},
	)
	server := w.Script(w.Home+"/bin/greeter.sh", fakeMCPServer)
	w.Write(".mcp.json", `{"mcpServers": {"repo": {"command": "`+server+`"}}}`)

	r := run(t, w, "-p", "/mcp").wantCode(t, 0)
	has(t, "/mcp in a folder that is not trusted", r.out, "No MCP server is configured")

	r = run(t, w, "--trust-folder", "-p", "/mcp").wantCode(t, 0)
	has(t, "/mcp in a trusted folder", r.out, "repo: connected", "project, ")

	run(t, w, "--trust-folder", "-p", "greet").wantCode(t, 0)
	result := toolResult(t, w, 1, "c1")
	if strings.Contains(result, "hello, you") {
		t.Errorf("the server's tool ran without anybody's approval: %s", result)
	}
	has(t, "what the model read of the call", result, "nobody is here to give it")
}

// gnosh mcp add, list, get and remove, from a shell, as Claude Code's
// claude mcp has them; a session then uses what was added.
func TestMCPCommands(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "ok"})
	server := w.Script(w.Home+"/bin/greeter.sh", fakeMCPServer)
	var auth []string
	far := fakeMCPHTTP(t, &auth)

	// add: local unless -s says otherwise.
	r := run(t, w, "mcp", "add", "greeter", "--", server, "--quiet").wantCode(t, 0)
	has(t, "add", r.out, "Added greeter to local ("+w.ConfigDir()+"/mcp.json, for "+w.Dir+").")
	r = run(t, w, "mcp", "add", "-s", "user", "-t", "http", "-H", "Authorization: Bearer t0k", "far", far.URL+"/mcp?key=k9").wantCode(t, 0)
	has(t, "add -t http", r.out, "Added far to user ("+w.ConfigDir()+"/mcp.json).")
	r = run(t, w, "mcp", "add-json", "-s", "project", "shared", `{"command": "`+server+`"}`).wantCode(t, 0)
	has(t, "add-json", r.out, "Added shared to project ("+w.Dir+"/.mcp.json).", "used once its folder is trusted")
	if info, err := os.Stat(w.ConfigDir() + "/mcp.json"); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the user's file: %v, %v; want it its owner's alone", info, err)
	}

	// What would not work is refused, and nothing is written.
	r = run(t, w, "mcp", "add", "greeter", server).wantCode(t, 1)
	has(t, "a second add", r.err, "there is a server named greeter in local", "gnosh mcp remove greeter -s local")
	r = run(t, w, "mcp", "add-json", "bad", `{"type": "http"}`).wantCode(t, 1)
	has(t, "add-json of a bad entry", r.err, "the entry of bad would not be used", "no url")
	r = run(t, w, "mcp", "add", "a.b", server).wantCode(t, 1)
	has(t, "a bad name", r.err, `"a.b" cannot be a server's name`)

	// list: each server and whether it connects; the project's waits for
	// trust. The rest of a URL and the headers' values are not shown.
	r = run(t, w, "mcp", "list").wantCode(t, 0)
	has(t, "list", r.out,
		"greeter: "+server+" --quiet (stdio, local) - connected (greeter 0.1), 1 tool",
		"far: "+far.URL+" (http, user) - connected (far 1), 1 tool",
		"shared: in the project's .mcp.json, not used until this folder is trusted")
	if strings.Contains(r.out, "k9") || strings.Contains(r.out, "t0k") {
		t.Errorf("list showed a key:\n%s", r.out)
	}
	if len(auth) == 0 || auth[0] != "Bearer t0k" {
		t.Errorf("the header did not go to the server: %q", auth)
	}

	r = run(t, w, "mcp", "get", "far").wantCode(t, 0)
	has(t, "get", r.out, "far:\n", "  scope: user (", "  type: http", "  url: "+far.URL+"/mcp?key=k9",
		"  headers: Authorization (values not shown)", "  status: connected (far 1), 1 tool", "To remove it: gnosh mcp remove far -s user")
	if strings.Contains(r.out, "t0k") {
		t.Errorf("get showed a header's value:\n%s", r.out)
	}
	r = run(t, w, "mcp", "get", "shared").wantCode(t, 0)
	has(t, "get of a server waiting for trust", r.out, "not used until this folder is trusted")
	run(t, w, "mcp", "get", "nobody").wantCode(t, 1)

	// A session uses the local server.
	r = run(t, w, "-p", "/mcp").wantCode(t, 0)
	has(t, "/mcp", r.out, "greeter: connected (greeter 0.1; local, ", "far: connected (far 1; user, ")

	// remove: a name in two places needs -s.
	run(t, w, "mcp", "add", "-s", "user", "greeter", server).wantCode(t, 0)
	r = run(t, w, "mcp", "remove", "greeter").wantCode(t, 1)
	has(t, "remove of a name in two places", r.err, "greeter is in more than one place", "Say which with -s (local, user)")
	r = run(t, w, "mcp", "remove", "greeter", "-s", "local").wantCode(t, 0)
	has(t, "remove -s local", r.out, "Removed greeter from local (")
	r = run(t, w, "mcp", "rm", "greeter").wantCode(t, 0)
	has(t, "rm", r.out, "Removed greeter from user (")
	r = run(t, w, "mcp", "remove", "nobody").wantCode(t, 1)
	has(t, "remove of nobody", r.err, "there is no server named nobody here")
	data, _ := os.ReadFile(w.ConfigDir() + "/mcp.json")
	if strings.Contains(string(data), "greeter") || strings.Contains(string(data), "projects") || !strings.Contains(string(data), `"far"`) {
		t.Errorf("the user's file after the removes:\n%s", data)
	}
}

// /mcp reconnect, enable and disable: a reconnect starts the server
// afresh; a disable lets it go for the session; names that are not there,
// and words /mcp does not take, are said to be so.
func TestMCPReconnectAndDisable(t *testing.T) {
	w := apptest.NewWorld(t)
	starts := w.Home + "/starts"
	server := w.Script(w.Home+"/bin/greeter.sh", "echo started >> "+starts+"\n"+fakeMCPServer)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {"greeter": {"command": "`+server+`"}}}`)
	count := func() int {
		data, _ := os.ReadFile(starts)
		return strings.Count(string(data), "started")
	}

	r := run(t, w, "-p", "/mcp disable greeter").wantCode(t, 0)
	has(t, "/mcp disable", r.out, "greeter: disabled for this session (user, ", "/mcp enable greeter connects it")
	r = run(t, w, "-p", "/mcp enable greeter").wantCode(t, 0)
	has(t, "/mcp enable of a server that is not disabled", r.out, "greeter: connected (greeter 0.1; user, ")
	before := count()
	r = run(t, w, "-p", "/mcp reconnect all").wantCode(t, 0)
	has(t, "/mcp reconnect all", r.out, "greeter: connected (greeter 0.1; user, ")
	if got := count() - before; got != 2 {
		t.Errorf("the server was started %d times by a run with a reconnect, want 2", got)
	}
	r = run(t, w, "-p", "/mcp reconnect nobody").wantCode(t, 0)
	has(t, "/mcp reconnect of nobody", r.out, "There is no MCP server named nobody here; the servers are greeter.")
	r = run(t, w, "-p", "/mcp frobnicate greeter").wantCode(t, 0)
	has(t, "/mcp frobnicate", r.out, "/mcp takes nothing, or reconnect, enable or disable")
}

// --mcp-config gives a run servers of its own; --strict-mcp-config makes
// them the only ones; safe mode turns off the files' servers and leaves
// the ones typed.
func TestMCPConfigFlags(t *testing.T) {
	w := apptest.NewWorld(t)
	server := w.Script(w.Home+"/bin/greeter.sh", fakeMCPServer)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {"filed": {"command": "`+server+`"}}}`)
	w.Write("given.json", `{"mcpServers": {"given": {"command": "`+server+`"}}}`)
	inline := `{"mcpServers": {"typed": {"command": "` + server + `"}}}`

	// (JSON with blanks in it ends a list, so it goes first.)
	r := run(t, w, "-p", "--mcp-config", inline, "given.json", "--", "/mcp").wantCode(t, 0)
	has(t, "/mcp with --mcp-config", r.out, "filed: connected", "given: connected (greeter 0.1; command line, "+w.Dir+"/given.json)", "typed: connected (greeter 0.1; command line, --mcp-config)")

	r = run(t, w, "-p", "--strict-mcp-config", "--mcp-config", inline, "--", "/mcp").wantCode(t, 0)
	has(t, "/mcp with --strict-mcp-config", r.out, "typed: connected")
	if strings.Contains(r.out, "filed") {
		t.Errorf("--strict-mcp-config let the user's server in:\n%s", r.out)
	}

	r = run(t, w, "-p", "--safe-mode", "--mcp-config", inline, "--", "/mcp").wantCode(t, 0)
	has(t, "/mcp in safe mode", r.out, "typed: connected")
	has(t, "the safe mode notice", r.err, "MCP servers from files")
	if strings.Contains(r.out, "filed") {
		t.Errorf("safe mode let the user's server in:\n%s", r.out)
	}
}

// A sub-agent has the tools of the MCP servers, as the session does,
// unless its kind names its tools.
func TestMCPToolsInSubAgents(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"greet","prompt":"Greet Jae.","subagent_type":"general-purpose"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "mcp__greeter__greet", Args: `{"name":"Jae"}`}}},
		apptest.Reply{Text: "The server said: hello, Jae."},
		apptest.Reply{Text: "Greeted."},
	)
	server := w.Script(w.Home+"/bin/greeter.sh", fakeMCPServer)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {"greeter": {"command": "`+server+`"}}}`)

	run(t, w, "-y", "-p", "greet Jae through a sub-agent").wantCode(t, 0)
	reqs := w.Model.Requests()
	if len(reqs) != 4 {
		t.Fatalf("the model was sent %d requests, want 4", len(reqs))
	}
	if names := toolNamesOf(reqs[1]); !slices.Contains(names, "mcp__greeter__greet") {
		t.Errorf("the general-purpose sub-agent's tools are %q", names)
	}
	// (toolResult numbers the calls of each request from c1.)
	has(t, "what the sub-agent read of the call", toolResult(t, w, 2, "c1"), "hello, Jae")
	has(t, "the result of the Agent call", toolResult(t, w, 3, "c1"), "The server said: hello, Jae.")

	// Explore names its tools, and no server's is among them.
	w.Model.Add(
		apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "Agent", Args: `{"description":"look","prompt":"Look around.","subagent_type":"Explore"}`}}},
		apptest.Reply{Text: "Nothing much."},
		apptest.Reply{Text: "Looked."},
	)
	run(t, w, "-y", "-p", "look around").wantCode(t, 0)
	reqs = w.Model.Requests()
	if names := toolNamesOf(reqs[len(reqs)-2]); slices.Contains(names, "mcp__greeter__greet") {
		t.Errorf("the Explore sub-agent was given a server's tool: %q", names)
	}
}

// changerMCPServer is a small MCP server in sh whose tool list grows: once
// its tool open has been called, it also has wave, and it says so before
// it answers the call.
const changerMCPServer = `while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"changer","version":"1"},"capabilities":{"tools":{"listChanged":true}}}}\n' "$id" ;;
    *'"method":"tools/list"'*)
      if [ -f "$HOME/opened" ]; then
        printf '{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"open","description":"Opens the door."},{"name":"wave","description":"Waves through the door."}]}}\n' "$id"
      else
        printf '{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"open","description":"Opens the door."}]}}\n' "$id"
      fi ;;
    *'"method":"tools/call"'*)
      touch "$HOME/opened"
      printf '{"jsonrpc":"2.0","method":"notifications/tools/list_changed"}\n'
      printf '{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"the door is open"}]}}\n' "$id" ;;
  esac
done`

// A server that says its tools changed has them read again, and the
// model's next request has the new list.
func TestMCPToolsChange(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "mcp__changer__open", Args: `{}`}}},
		apptest.Reply{Text: "Opened."},
	)
	server := w.Script(w.Home+"/bin/changer.sh", changerMCPServer)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {"changer": {"command": "`+server+`"}}}`)

	run(t, w, "-y", "-p", "open the door").wantCode(t, 0)
	reqs := w.Model.Requests()
	if len(reqs) != 2 {
		t.Fatalf("the model was sent %d requests, want 2", len(reqs))
	}
	if names := toolNamesOf(reqs[0]); !slices.Contains(names, "mcp__changer__open") || slices.Contains(names, "mcp__changer__wave") {
		t.Errorf("the tools before the call: %q", names)
	}
	if names := toolNamesOf(reqs[1]); !slices.Contains(names, "mcp__changer__wave") {
		t.Errorf("the tools after the server said they changed: %q", names)
	}
}

// briefMCPHTTP is an MCP server at a URL whose sessions end after one
// call: a request in an ended session is answered 404, and a handshake
// starts another session.
func briefMCPHTTP(t *testing.T, inits, calls *atomic.Int32) *httptest.Server {
	var mu sync.Mutex
	live := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		defer mu.Unlock()
		sid := r.Header.Get("Mcp-Session-Id")
		switch {
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
			return
		case m.Method == "initialize":
			sid = fmt.Sprintf("s%d", inits.Add(1))
			live[sid] = true
			w.Header().Set("Mcp-Session-Id", sid)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"brief","version":"1"}}}`, m.ID)
			return
		case !live[sid]:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch m.Method {
		case "tools/list":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"ping","description":"Pings."}]}}`, m.ID)
		case "tools/call":
			delete(live, sid)
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"pong %d"}]}}`, m.ID, calls.Add(1))
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A server at a URL that ends the session is connected to again, and the
// call it refused is made again.
func TestMCPRedialsAServerAtAURL(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "mcp__brief__ping", Args: `{}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "mcp__brief__ping", Args: `{}`}}},
		apptest.Reply{Text: "Pinged twice."},
	)
	var inits, calls atomic.Int32
	srv := briefMCPHTTP(t, &inits, &calls)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {"brief": {"type": "http", "url": "`+srv.URL+`/mcp"}}}`)

	run(t, w, "-y", "-p", "ping twice").wantCode(t, 0)
	has(t, "the first call", toolResult(t, w, 1, "c1"), "pong 1")
	has(t, "the second call, made again in a new session", toolResult(t, w, 2, "c2"), "pong 2")
	if inits.Load() != 2 || calls.Load() != 2 {
		t.Errorf("%d handshakes and %d calls, want 2 of each", inits.Load(), calls.Load())
	}
}

// bigMCPServer is a small MCP server in sh whose one tool, dump, returns
// 1,000 characters.
const bigMCPServer = `while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9][0-9]*\).*/\1/p')
  case "$line" in
    *'"method":"initialize"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"big","version":"1"}}}\n' "$id" ;;
    *'"method":"tools/list"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"dump","description":"Dumps a lot."}]}}\n' "$id" ;;
    *'"method":"tools/call"'*)
      printf '{"jsonrpc":"2.0","id":%s,"result":{"content":[{"type":"text","text":"start%01000dend"}]}}\n' "$id" 7 ;;
  esac
done`

// A result longer than MAX_MCP_OUTPUT_TOKENS lets the model have goes to a
// file in the session's own folder, named with the plain characters of the
// call's id, and the model is told where, with the result's beginning.
func TestMCPLargeResult(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "mcp__big__dump", Args: `{}`}}},
		apptest.Reply{Text: "Dumped."},
	)
	server := w.Script(w.Home+"/bin/big.sh", bigMCPServer)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {"big": {"command": "`+server+`"}}}`)
	o := testOptions(w, "-y", "-p", "dump it")
	o.Environ = append(o.Environ, "MAX_MCP_OUTPUT_TOKENS=100")
	runWith(t, context.Background(), o).wantCode(t, 0)

	result := toolResult(t, w, 1, "c1")
	has(t, "what the model read", result, "The result is 1008 characters long, more than the 400 one result may be here", "/tool-results/mcp__big__dump-", "It begins:\nstart000")
	if strings.Contains(result, "7end") {
		t.Errorf("the model was handed the whole result:\n%s", result)
	}
	files, _ := filepath.Glob(filepath.Join(w.DataDir(), "projects", "*", "*", "tool-results", "mcp__big__dump-*.txt"))
	if len(files) != 1 {
		t.Fatalf("the result files: %v", files)
	}
	if data, _ := os.ReadFile(files[0]); !strings.HasSuffix(string(data), "7end") || len(data) != 1008 {
		t.Errorf("the file holds %d bytes: %.40q...", len(data), data)
	}
	// The model's id for the call goes into the name with its plain
	// characters alone.
	if got := plainName("../../out x"); got != "______out_x" {
		t.Errorf("plainName = %q", got)
	}
}

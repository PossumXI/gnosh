package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

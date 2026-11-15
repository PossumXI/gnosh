package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

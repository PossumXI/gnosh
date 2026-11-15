package app

import (
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

// An MCP server from the user's mcp.json: it is started, its tool is
// offered as mcp__<server>__<tool>, a call reaches it and its answer
// reaches the model; /mcp says how it stands; and stream-json's init line
// names it.
func TestMCPServerTools(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "mcp__greeter__greet", Args: `{"name":"Jae"}`}}},
		apptest.Reply{Text: "Greeted."},
	)
	server := w.Script(w.Home+"/bin/greeter.sh", fakeMCPServer)
	w.Write(w.ConfigDir()+"/mcp.json", `{"mcpServers": {
		"greeter": {"command": "`+server+`"},
		"broken": {"command": "/no/such/server"},
		"remote": {"type": "http", "url": "https://mcp.example.org/x"}
	}}`)

	run(t, w, "-y", "-p", "greet Jae").wantCode(t, 0)
	reqs := w.Model.Requests()
	has(t, "the tools the model was offered", string(reqs[0].Body), "mcp__greeter__greet", "Greets someone by name.")
	has(t, "what the model read of the call", toolResult(t, w, 1, "c1"), "hello, Jae")

	r := run(t, w, "-p", "/mcp").wantCode(t, 0)
	has(t, "/mcp", r.out, "greeter: connected (greeter 0.1; user, ", "1 tool: greet",
		"broken: not connected", "/no/such/server",
		"remote: not connected", "the http transport is not built yet")

	r = run(t, w, "-p", "--output-format", "stream-json", "--verbose", "/mcp").wantCode(t, 0)
	has(t, "the init line", r.out, `"mcp_servers":[{"name":"broken","status":"failed"},{"name":"greeter","status":"connected"},{"name":"remote","status":"failed"}]`)
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

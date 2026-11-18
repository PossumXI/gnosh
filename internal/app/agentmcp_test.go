package app

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A definition's tools and disallowedTools hold MCP tools by name, by
// their server (mcp__github__*), and when taking away also as mcp__github
// and mcp__*; a server whose name holds a double underscore is named
// whole.
func TestToolListed(t *testing.T) {
	cases := []struct {
		list []string
		name string
		away bool
		want bool
	}{
		{[]string{"Read"}, "Read", false, true},
		{[]string{"mcp__github__*"}, "mcp__github__create_issue", false, true},
		{[]string{"mcp__github__*"}, "mcp__gitlab__create_issue", false, false},
		{[]string{"mcp__github"}, "mcp__github__create_issue", false, false},
		{[]string{"mcp__github"}, "mcp__github__create_issue", true, true},
		{[]string{"mcp__*"}, "mcp__github__create_issue", true, true},
		{[]string{"mcp__*"}, "Read", true, false},
		{[]string{"mcp__*"}, "mcp__github__create_issue", false, true},
		{[]string{"mcp__reports__v2__*"}, "mcp__reports__v2__run", false, true},
		{[]string{"mcp__reports"}, "mcp__reports__v2__run", true, true},
		{[]string{"mcp__reports__v2"}, "mcp__reports__v2__run", true, false},
		{[]string{"Read*"}, "ReadMore", false, false},
	}
	for _, c := range cases {
		if got := toolListed(c.list, c.name, c.away); got != c.want {
			t.Errorf("toolListed(%q, %q, away %v) = %v, want %v", c.list, c.name, c.away, got, c.want)
		}
	}
}

// A sub-agent whose definition writes an MCP server of its own has that
// server's tools, which the session's model is never offered; the server
// is started when the sub-agent starts and stopped when it ends. A name of
// a server the session lacks is said, and the sub-agent goes without it.
func TestSubAgentOwnMCPServer(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"greet","prompt":"Greet Jae.","subagent_type":"greeter"}`}}},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "mcp__own__greet", Args: `{"name":"Jae"}`}}},
		apptest.Reply{Text: "Greeted."},
		apptest.Reply{Text: "Done."},
	)
	pids := w.Home + "/server-pids"
	server := w.Script(w.Home+"/bin/greeter.sh", "echo $$ >> "+pids+"\n"+fakeMCPServer)
	w.Write(w.ConfigDir()+"/agents/greeter.md", `---
name: greeter
description: Greets people.
mcpServers:
  - own:
      command: "`+server+`"
  - nosuch
---
You greet people with the greet tool.
`)
	r := run(t, w, "-y", "-p", "greet Jae")
	r.wantCode(t, 0)
	reqs := w.Model.Requests()
	if len(reqs) != 4 {
		t.Fatalf("the model was asked %d times, want 4", len(reqs))
	}
	if body := string(reqs[0].Body); strings.Contains(body, "mcp__own__greet") {
		t.Errorf("the session's model was offered the sub-agent's own server's tool")
	}
	has(t, "the sub-agent's tools", string(reqs[1].Body), "mcp__own__greet", "Greets someone by name.")
	// (The sub-agent's conversation has its own calls: its first is c1.)
	has(t, "what the sub-agent read", toolResult(t, w, 2, "c1"), "hello, Jae")
	has(t, "what was said of the server the session lacks", r.out+r.err, `the server "nosuch"`, "goes without it")
	data, _ := os.ReadFile(pids)
	for _, pid := range strings.Fields(string(data)) {
		n := 0
		fmt.Sscan(pid, &n)
		if n > 0 && syscall.Kill(n, 0) == nil {
			t.Errorf("the sub-agent's server (pid %d) still runs after it ended", n)
		}
	}
	if len(strings.Fields(string(data))) != 1 {
		t.Errorf("the server was started %d times, want once", len(strings.Fields(string(data))))
	}
}

// The session that runs as an agent (--agent) has the agent's own servers
// from the start, beside the files', and /mcp shows them.
func TestAgentSessionOwnMCPServer(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "mcp__own__greet", Args: `{"name":"Jae"}`}}},
		apptest.Reply{Text: "Greeted."},
	)
	server := w.Script(w.Home+"/bin/greeter.sh", fakeMCPServer)
	w.Write(w.ConfigDir()+"/agents/greeter.md", `---
name: greeter
description: Greets people.
mcpServers:
  - own:
      command: "`+server+`"
---
You greet people with the greet tool.
`)
	run(t, w, "--agent", "greeter", "-y", "-p", "greet Jae").wantCode(t, 0)
	has(t, "the tools offered", string(w.Model.Requests()[0].Body), "mcp__own__greet")
	has(t, "what the model read", toolResult(t, w, 1, "c1"), "hello, Jae")
	r := run(t, w, "--agent", "greeter", "-p", "/mcp").wantCode(t, 0)
	has(t, "/mcp", r.out, "own: connected", "agent, ")
}

// /status names the agent and skill files it found and does not use, and
// why.
func TestStatusNamesUnusedFiles(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Write(w.ConfigDir()+"/agents/broken.md", "---\nname: broken\n---\nNo description.\n")
	r := run(t, w, "-p", "/status").wantCode(t, 0)
	has(t, "/status", r.out, "not used", "agents/broken.md: no kind of sub-agent")
}

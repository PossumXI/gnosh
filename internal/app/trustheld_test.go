package app

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// The trust question shows what a Yes would turn on beside the folder's
// settings files: the servers of its .mcp.json, and a skill that lets tools
// run without asking. It showed only the settings files', so a Yes to one
// harmless option started a server it never named; and a folder with
// nothing but such files was never asked (the config review of
// 2026-11-19). A No starts none of it.
func TestTrustQuestionShowsWhatAYesTurnsOn(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Hello."})
	w.Write("gnosh.json", `{"options": {"language": "english"}}`+"\n")
	w.Write(".mcp.json", `{"mcpServers": {"helper": {"command": "sh", "args": ["-c", "touch ran.txt"]}}}`+"\n")
	w.Write(".claude/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploys the site.\nallowed-tools: Bash\n---\nDeploy it.\n")
	s := sit(t, w)
	s.see("? Trust this folder?")
	s.seeAnywhere("the MCP server helper, run on this machine as sh -c touch ran.txt")
	s.seeAnywhere("the skill deploy lets these run without asking while it is used: Bash")
	s.see("  1. No")
	s.typeLine("1")
	s.see("Type a prompt and Enter.")
	s.typeLine("hi")
	s.see("Hello.")
	s.typeLine("/exit")
	s.exit()
	if w.Exists("ran.txt") {
		t.Error("the folder's MCP server was started after a No")
	}

	// A folder whose only own content is such a file is asked about too.
	w = apptest.NewWorld(t)
	w.Write(".mcp.json", `{"mcpServers": {"online": {"type": "http", "url": "https://mcp.example.com/x"}}}`+"\n")
	s = sit(t, w)
	s.see("? Trust this folder?")
	s.seeAnywhere("the MCP server online at https://mcp.example.com/x")
	s.typeLine("1")
	s.see("Type a prompt and Enter.")
	s.typeLine("/exit")
	s.exit()
}

package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// Monitor runs a command in the background and passes on what it prints
// (spec/cc/tools.md 16): the lines reach the model as notes from gnosh,
// and after them the news that the command ended. A run waits for it, as
// for any background command.
func TestMonitorTool(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Monitor", Args: `{"description":"the build","command":"echo compiling; sleep 0.6; echo BUILD-OK-42"}`}}},
		apptest.Reply{Text: "Watching."},
	)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		if n >= 2 {
			return &apptest.Reply{Text: "Noted."}
		}
		return nil
	})
	run(t, w, "-y", "-p", "build it").wantCode(t, 0)
	has(t, "what the Monitor call returned", toolResult(t, w, 1, "c1"), "Monitor bg1 watches: the build", "TaskStop stops it (task_id bg1)")
	var notes []string
	for _, req := range w.Model.Requests() {
		notes = userTexts(req)
	}
	all := strings.Join(notes, "\n")
	has(t, "what the model was told", all, "What the command of Monitor bg1 (the build) printed", "compiling", "BUILD-OK-42",
		"A background command that a Monitor watched has ended: the build")
	if strings.Index(all, "BUILD-OK-42") > strings.Index(all, "A background command that a Monitor watched has ended") {
		t.Errorf("the news of the end came before the last lines:\n%s", all)
	}

	// A sub-agent has no Monitor: its lines would go to the session.
	w = apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"look","prompt":"Look.","subagent_type":"general-purpose"}`}}},
		apptest.Reply{Text: "Looked."},
		apptest.Reply{Text: "Done."},
	)
	run(t, w, "-y", "-p", "look").wantCode(t, 0)
	if tools := strings.Join(toolNamesOf(w.Model.Requests()[1]), " "); strings.Contains(tools, "Monitor") || !strings.Contains(strings.Join(toolNamesOf(w.Model.Requests()[0]), " "), "Monitor") {
		t.Errorf("the session's tools %v, the sub-agent's %q", toolNamesOf(w.Model.Requests()[0]), tools)
	}
}

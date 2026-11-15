package app

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A status line command named in the settings runs when the session opens
// and again after a reply, with the session's facts as JSON on its
// standard input and COLUMNS set to the terminal's width less the padding.
func TestStatusLineCommand(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Hello."})
	runs := w.Home + "/sl-runs"
	script := w.Script(w.Home+"/bin/sl.sh", "{ cat; echo; echo \"cols=$COLUMNS\"; } >> "+runs+"\necho \"status of the session\"")
	w.Settings(`{"statusLine": {"type": "command", "command": "` + script + `", "padding": 1}}`)

	// inputs returns the JSON objects the command was given so far.
	inputs := func() []map[string]any {
		data, _ := os.ReadFile(runs)
		var out []map[string]any
		for _, line := range strings.Split(string(data), "\n") {
			var obj map[string]any
			if json.Unmarshal([]byte(line), &obj) == nil {
				out = append(out, obj)
			}
		}
		return out
	}
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); !ok(); {
			if time.Now().After(deadline) {
				data, _ := os.ReadFile(runs)
				t.Fatalf("%s never happened; the command was given:\n%s", what, data)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	waitFor("the run at the session's start", func() bool { return len(inputs()) >= 1 })
	first := inputs()[0]
	if first["session_id"] == "" || !strings.HasSuffix(first["transcript_path"].(string), ".jsonl") || first["cwd"] != w.Dir {
		t.Errorf("the first run was given %v", first)
	}
	if m, _ := first["model"].(map[string]any); m["id"] != "local/test-model" {
		t.Errorf("the model it was given: %v", first["model"])
	}
	if ws, _ := first["workspace"].(map[string]any); ws["current_dir"] != w.Dir || ws["project_dir"] != w.Dir {
		t.Errorf("the workspace it was given: %v", first["workspace"])
	}
	if data, _ := os.ReadFile(runs); !strings.Contains(string(data), "cols=") {
		t.Errorf("COLUMNS was not set:\n%s", data)
	}

	before := len(inputs())
	s.typeLine("say hello")
	s.see("Hello.")
	waitFor("a run after the reply", func() bool { return len(inputs()) > before })
	last := inputs()[len(inputs())-1]
	cw, _ := last["context_window"].(map[string]any)
	if cw == nil || cw["current_usage"] == nil {
		t.Errorf("after a reply, the context window it was given: %v", last["context_window"])
	}
	s.typeLine("/exit")
	s.exit()
}

// With a refreshInterval the command runs again on its own, with nothing
// else happening.
func TestStatusLineRefresh(t *testing.T) {
	w := apptest.NewWorld(t)
	runs := w.Home + "/sl-runs"
	script := w.Script(w.Home+"/bin/sl.sh", "cat > /dev/null; echo run >> "+runs+"\necho line")
	w.Settings(`{"statusLine": {"type": "command", "command": "` + script + `", "refreshInterval": 1}}`)
	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	count := func() int {
		data, _ := os.ReadFile(runs)
		return strings.Count(string(data), "run")
	}
	for deadline := time.Now().Add(10 * time.Second); count() < 3 && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
	if n := count(); n < 3 {
		t.Errorf("the command ran %d times in a session that did nothing, want at least 3 (the start and two refreshes)", n)
	}
	s.typeLine("/exit")
	s.exit()
}

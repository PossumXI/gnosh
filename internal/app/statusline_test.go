package app

import (
	"encoding/json"
	"os"
	"regexp"
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
	// (The script writes its input, then the width: the file is read until
	// both are there, and the width must be a number.)
	waitFor("COLUMNS set to a number", func() bool {
		data, _ := os.ReadFile(runs)
		return regexp.MustCompile(`cols=[0-9]+`).Match(data)
	})

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

// /statusline asks the model, in a turn, to write a status line script
// from the person's words, try it on this session's own JSON, and add the
// setting to their gnosh.json.
func TestStatusLineCommandAsksTheModel(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Written."})
	r := run(t, w, "-p", "/statusline the branch and the model").wantCode(t, 0)
	has(t, "the run", r.out, "Written.")
	has(t, "what the model was asked", requestText(t, w, 0),
		"It is to show: the branch and the model", "statusline.sh", "gnosh.json", `\"session_id\"`, `\"workspace\"`)
	w.Model.Done()
}

// In a --worktree session the status line command is told of the work
// tree, as Claude Code's is: its name, folder and branch, and where the
// session was started.
func TestStatusLineWorktree(t *testing.T) {
	w := repoWorld(t)
	runs := w.Home + "/sl-runs"
	script := w.Script(w.Home+"/bin/sl.sh", "{ cat; echo; } >> "+runs+"\necho status")
	w.Settings(`{"statusLine": {"type": "command", "command": "` + script + `"}}`)
	s := sit(t, w, "--trust-folder", "--worktree", "feat")
	s.see("Type a prompt and Enter.")
	var got map[string]any
	for deadline := time.Now().Add(15 * time.Second); got == nil; {
		data, _ := os.ReadFile(runs)
		for _, line := range strings.Split(string(data), "\n") {
			if json.Unmarshal([]byte(line), &got) == nil {
				break
			}
			got = nil
		}
		if got == nil && time.Now().After(deadline) {
			t.Fatalf("the status line command never ran: %s", data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	tree := w.Dir + "/.gnosh/worktrees/feat"
	wt, _ := got["worktree"].(map[string]any)
	if wt["name"] != "feat" || wt["path"] != tree || wt["branch"] != "worktree-feat" || wt["original_cwd"] != w.Dir || wt["original_branch"] != "main" {
		t.Errorf("the worktree it was given: %v", got["worktree"])
	}
	if ws, _ := got["workspace"].(map[string]any); ws["git_worktree"] != "feat" || ws["current_dir"] != tree {
		t.Errorf("the workspace it was given: %v", got["workspace"])
	}
	s.typeLine("/exit")
	s.exit()
}

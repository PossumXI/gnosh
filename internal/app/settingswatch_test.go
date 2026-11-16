package app

import (
	"os"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A host added to gnosh.json while the session runs is taken in before the
// next request to the model: the model is told of it, and a call naming it
// runs there; a deny rule added by hand is in force from then on.
func TestSettingsChangeTakenIn(t *testing.T) {
	w := apptest.NewWorld(t)
	far := w.FarMachine()
	w.Write("a.txt", "keep me\n")
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		switch n {
		case 0:
			// The person adds a host, and a rule, while the turn runs.
			w.Settings(`{"hosts": {"render": {"ssh": "root@203.0.113.7"}}, "permissions": {"deny": ["Bash(rm *)"]}}`)
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo first"}`}}}
		case 1:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Bash", Args: `{"command":"echo there","host":"render"}`}}}
		case 2:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "Bash", Args: `{"command":"rm -f a.txt"}`}}}
		}
		return &apptest.Reply{Text: "Done."}
	})
	r := run(t, w, "-y", "-p", "go").wantCode(t, 0)
	has(t, "what the model was told", string(w.Model.Requests()[1].Body), "the machines Bash's host may name are now render (root@203.0.113.7)")
	if got := toolResult(t, w, 2, "c2"); !strings.HasPrefix(got, "<host>render (root@203.0.113.7)</host><cwd>"+far.Home+"</cwd>") {
		t.Errorf("the call naming the new host: %q", got)
	}
	has(t, "the rule added by hand", toolResult(t, w, 3, "c3"), "This call did not run", "Bash(rm *)")
	if !w.Exists("a.txt") {
		t.Errorf("the rm ran past the deny rule added while the session ran")
	}
	has(t, "the person's notice", r.err+r.out, "changed: its hosts and its permission rules are in force now")
}

// A ConfigChange hook runs when a settings file changes while the session
// runs, told which file: a block keeps the settings the session had, and
// once it lets a change through the change is in force.
func TestConfigChangeHook(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Write("a.txt", "x\n")
	w.Write("b.txt", "x\n")
	log := w.Home + "/hook-log"
	allow := w.Home + "/allow"
	gate := w.Script(w.Home+"/hooks/gate.sh", "cat >> "+log+"\necho >> "+log+"\nif [ -e "+allow+" ]; then exit 0; fi\necho 'the settings are frozen' >&2\nexit 2")
	hooksBlock := `"hooks": {"ConfigChange": [{"matcher": "user_settings", "hooks": [{"type": "command", "command": "` + gate + `"}]}]}`
	w.Settings(`{` + hooksBlock + `}`)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		switch n {
		case 0:
			w.Settings(`{` + hooksBlock + `, "permissions": {"deny": ["Bash(rm *)"]}}`)
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Bash", Args: `{"command":"echo first"}`}}}
		case 1:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Bash", Args: `{"command":"rm -f a.txt"}`}}}
		case 2:
			if err := os.WriteFile(allow, nil, 0o600); err != nil {
				t.Error(err)
			}
			w.Settings(`{` + hooksBlock + `, "permissions": {"deny": ["Bash(rm *)", "Bash(curl *)"]}}`)
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "Bash", Args: `{"command":"echo second"}`}}}
		case 3:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c4", Name: "Bash", Args: `{"command":"rm -f b.txt"}`}}}
		}
		return &apptest.Reply{Text: "Done."}
	})
	r := run(t, w, "-y", "-p", "go").wantCode(t, 0)
	if w.Exists("a.txt") {
		t.Errorf("the rm ran under a deny rule whose change a ConfigChange hook blocked")
	}
	if !w.Exists("b.txt") {
		t.Errorf("the rm ran past the deny rule the hook let through")
	}
	has(t, "the person's notice", r.err+r.out, "a ConfigChange hook the user set up blocked it: the session keeps the settings it had. the settings are frozen",
		"changed: its permission rules are in force now")
	data, _ := os.ReadFile(log)
	has(t, "what the hook read", string(data), `"hook_event_name":"ConfigChange"`, `"source":"user_settings"`, `"file_path":"`)
	if n := strings.Count(string(data), `"hook_event_name":"ConfigChange"`); n != 2 {
		t.Errorf("the hook ran %d times for two changes:\n%s", n, data)
	}
}

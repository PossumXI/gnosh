package app

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// SendMessage gives a sub-agent that ended more to do: it runs again in
// the background under the same id, with its first task and its answer to
// it in front of it, and its report comes to the session as a notice.
func TestSendMessageResumesASubAgent(t *testing.T) {
	w := apptest.NewWorld(t)
	idRe := regexp.MustCompile(`\[agent id: ([0-9a-f-]+)\]`)
	sent := false
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		body := string(r.Body)
		if strings.Contains(body, "You are a sub-agent inside gnosh") {
			switch {
			case !strings.Contains(body, "now count the oranges"):
				return &apptest.Reply{Text: "There are 3 apples."}
			case strings.Contains(body, "count the apples") && strings.Contains(body, "There are 3 apples."):
				return &apptest.Reply{Text: "There are 5 oranges."}
			default:
				return &apptest.Reply{Text: "There are 5 oranges, and I know of no apples."}
			}
		}
		switch m := idRe.FindStringSubmatch(body); {
		case strings.Contains(body, "There are 5 oranges"):
			return &apptest.Reply{Text: "Counted."}
		case m != nil && !sent:
			sent = true
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "SendMessage", Args: fmt.Sprintf(`{"to":%q,"message":"now count the oranges","summary":"count the oranges"}`, m[1])}}}
		case strings.Contains(body, `"role":"tool"`):
			return &apptest.Reply{Text: "Waiting for it."}
		}
		return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Agent", Args: `{"description":"count apples","prompt":"count the apples"}`}}}
	})
	r := run(t, w, "-p", "count").wantCode(t, 0)
	if !sent {
		t.Fatalf("the report gave no agent id:\n%s", r.out)
	}
	if !anyRequestHas(w, "is working in the background on: count the oranges") {
		t.Errorf("SendMessage did not say the sub-agent works in the background")
	}
	reqs := w.Model.Requests()
	last := string(reqs[len(reqs)-1].Body)
	if !strings.Contains(last, "There are 5 oranges.") || strings.Contains(last, "know of no apples") {
		t.Errorf("the resumed sub-agent's report, as the session was told it: %s", last)
	}
}

// What SendMessage does not take up: an id no sub-agent of the session has,
// and a one-shot kind.
func TestSendMessageRefusals(t *testing.T) {
	w := apptest.NewWorld(t)
	idRe := regexp.MustCompile(`\[agent id: ([0-9a-f-]+)\]`)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		body := string(r.Body)
		if strings.Contains(body, "look around") && !strings.Contains(body, "0000-nope") {
			// The Explore sub-agent (its own prompt is not gnosh's).
			return &apptest.Reply{Text: "Looked."}
		}
		switch {
		case strings.Contains(body, "Looked."):
			if idRe.MatchString(body) {
				return &apptest.Reply{Text: "An Explore report gave an id."}
			}
			return &apptest.Reply{Text: "Done."}
		case strings.Contains(body, "0000-nope"):
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Agent", Args: `{"description":"look","prompt":"look around","subagent_type":"Explore"}`}}}
		}
		return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "SendMessage", Args: `{"to":"0000-nope","message":"x"}`}}}
	})
	run(t, w, "-p", "go").wantCode(t, 0)
	if !anyRequestHas(w, `no sub-agent of this session has the id \"0000-nope\"`) {
		t.Errorf("an unknown id was not refused")
	}
	if !anyRequestHas(w, "Looked.") || anyRequestHas(w, "[agent id:") {
		t.Errorf("an Explore sub-agent's report gave an id to resume it by, or none came")
	}
}

// anyRequestHas reports whether a request the model was sent holds text.
func anyRequestHas(w *apptest.World, text string) bool {
	for _, r := range w.Model.Requests() {
		if strings.Contains(string(r.Body), text) {
			return true
		}
	}
	return false
}

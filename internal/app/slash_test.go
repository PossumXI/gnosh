package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// The slash commands of a session, typed at the terminal: each does what
// it says and says what it did.
func TestSlashCommandsInASession(t *testing.T) {
	w := apptest.NewWorld(t)
	w.Model.SetModels("test-model", "other-model")
	w.RC("permissions deny 'Bash(rm *)'")
	hook := w.Script(w.Home+"/hooks/noop.sh", "cat > /dev/null")
	w.RC("hook add Stop --name tests --command " + hook)
	turns := 0
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		if isSummaryRequest(r) {
			return &apptest.Reply{Text: "SUMMARY: the user asked for the first task, and it is under way."}
		}
		turns++
		switch turns {
		case 1:
			return &apptest.Reply{Text: "Started on the first task."}
		case 2:
			return &apptest.Reply{Text: "One more step is done."}
		case 3:
			return &apptest.Reply{Text: "All of it is done.\nLOOP_DONE"}
		case 4:
			return &apptest.Reply{Text: "The listing shows the three files I made.\nLOOP_DONE"}
		}
		return &apptest.Reply{Text: "An answer from the other model."}
	})
	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	s.typeLine("do the first task")
	s.see("Started on the first task.")
	s.see("-- done in ")

	// /loop keeps the session working until the model shows it is done.
	s.typeLine("/loop")
	s.see("/loop is on")
	s.see("One more step is done.")
	s.see("All of it is done.")
	s.see("The listing shows the three files I made.")
	s.see("/loop is off")
	if turns != 4 {
		t.Errorf("the model was asked %d times under /loop, want 4 in all", turns)
	}

	// /compact makes a summary now, with the person's words for it.
	s.typeLine("/compact keep the file names")
	s.see("The conversation was summarized")
	found := false
	for _, r := range w.Model.Requests() {
		if isSummaryRequest(r) && strings.Contains(string(r.Body), "keep the file names") {
			found = true
		}
	}
	if !found {
		t.Errorf("the summary request does not carry the person's instructions")
	}

	// The reports.
	s.typeLine("/permissions")
	s.see("Permission mode: default")
	s.see("Bash(rm *)")
	s.typeLine("/hooks")
	s.see("Stop:")
	s.see("tests: ")
	s.typeLine("/doctor")
	s.see("large model: local/test-model")
	s.see("A turn can run.")

	// /model lists what there is and switches; the choice is kept.
	s.typeLine("/model")
	s.see("In use: local/test-model")
	s.see("* local/test-model")
	s.see("local/other-model")
	s.typeLine("/model local/other-model")
	s.see("The model is local/other-model")
	s.typeLine("which model are you?")
	s.see("An answer from the other model.")
	s.see("model local/other-model")
	reqs := w.Model.Requests()
	if got := reqs[len(reqs)-1].JSON["model"]; got != "other-model" {
		t.Errorf("the request after /model names the model %v", got)
	}
	has(t, "the state file", w.Read(w.DataDir()+"/state.json"), "local/other-model")
	s.typeLine("/model no-such-model extra words here")
	s.see("error: /model takes a model")

	// /clear starts a fresh session; the one before is kept.
	before := w.Sessions()
	s.typeLine("/clear")
	s.see("A new session is open: ")
	s.typeLine("hello again")
	s.see("An answer from the other model.")
	last := w.Model.Requests()[len(w.Model.Requests())-1]
	if users := userTexts(last); len(users) != 1 || !strings.Contains(users[0], "hello again") || strings.Contains(string(last.Body), "first task") {
		t.Errorf("the session after /clear is not a fresh one: %q", users)
	}
	s.typeLine("/exit")
	s.exit()
	after := w.Sessions()
	if len(before) != 1 || len(after) != 2 {
		t.Fatalf("session files before /clear: %v, after: %v", before, after)
	}
	for _, path := range after {
		checkSession(t, w, path)
	}
}

// A slash command given as the prompt of a run is run, not sent to the
// model, and what it says is the run's result. A command that only makes
// sense with a person there says so.
func TestSlashCommandsAsARunsPrompt(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "The first answer."})
	run(t, w, "-p", "remember this").wantCode(t, 0)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		if isSummaryRequest(r) {
			return &apptest.Reply{Text: "SUMMARY: the user said to remember this."}
		}
		return nil
	})

	r := run(t, w, "-c", "-p", "/status").wantCode(t, 0)
	has(t, "/status", r.out, "session      ", "model        local/test-model", "permissions  mode default", "The configuration in force")
	r = run(t, w, "-p", "--output-format", "json", "/help").wantCode(t, 0)
	obj := jsonLines(t, r.out)[0]
	if obj["subtype"] != "success" || obj["local_command"] != "help" || !strings.Contains(obj["result"].(string), "/compact [instructions]") {
		t.Errorf("/help as json: %s", clip(r.out))
	}
	r = run(t, w, "-c", "-p", "/compact").wantCode(t, 0)
	has(t, "/compact", r.out, "The conversation was summarized")
	r = run(t, w, "-p", "/no_think").wantCode(t, 0)
	has(t, "/no_think", r.out, "Thinking is off for local/test-model")
	r = run(t, w, "-p", "/exit").wantCode(t, 1)
	has(t, "stderr", r.err, "/exit works in an interactive session")
	if r.out != "" {
		t.Errorf("stdout = %q", r.out)
	}
	// Only the first prompt and the summary went to the model.
	if n := len(w.Model.Requests()); n != 2 {
		t.Errorf("the model was sent %d requests, want 2", n)
	}
	// A prompt that merely begins with a slash is a prompt.
	w.Model.Respond(nil)
	w.Model.Add(apptest.Reply{Text: "That is a path."})
	r = run(t, w, "-p", "/usr/local/bin is what?").wantCode(t, 0)
	if r.out != "That is a path.\n" {
		t.Errorf("stdout = %q", r.out)
	}
	// With --disable-slash-commands the command's name goes to the model.
	w.Model.Add(apptest.Reply{Text: "You typed a slash command."})
	r = run(t, w, "--disable-slash-commands", "-p", "/status").wantCode(t, 0)
	if r.out != "You typed a slash command.\n" {
		t.Errorf("stdout = %q", r.out)
	}
}

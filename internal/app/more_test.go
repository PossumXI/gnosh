package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// /context says what fills the model's window, /rename names the session,
// and a session is named after its first prompt until the person names
// it.
func TestContextAndRename(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "It is flaky because of a timer."})
	w.Write("AGENTS.md", "Always answer in one sentence.\n")
	s := sit(t, w, "--trust-folder")
	s.see("Type a prompt and Enter.")
	s.typeLine("/rename")
	s.see("This session has no name yet.")
	s.typeLine("why is the test flaky?")
	s.see("It is flaky because of a timer.")
	s.typeLine("/rename")
	s.see(`This session is named "why is the test flaky?", after its first prompt.`)
	s.typeLine("/context")
	s.see("The model's window is 100.0K tokens")
	s.see("In use at the last count: about ")
	s.see("gnosh's instructions to the model")
	s.see("what the model is told of its 9 tools")
	s.see("the conversation")
	s.see("/context all lists each tool and each instruction file.")
	s.typeLine("/context all")
	s.see("The tools:")
	s.see("AskUserQuestion")
	s.see("The instruction files in effect from the start:")
	s.see(w.Path("AGENTS.md"))
	s.typeLine("/rename   the flaky   timer test ")
	s.see(`The session is named "the flaky timer test". gnosh -r "the flaky timer test" opens it again.`)
	s.typeLine("/rename")
	s.see(`This session is named "the flaky timer test". /rename and another name changes it.`)
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}

	// The name is what the session goes by afterwards.
	r := run(t, w, "session", "list").wantCode(t, 0)
	has(t, "session list", r.out, "the flaky timer test", "1 prompt(s)")
	w.Model.Add(apptest.Reply{Text: "Continued under its name."})
	run(t, w, "--trust-folder", "-r", "the flaky timer test", "-p", "go on").wantCode(t, 0)
	reqs := w.Model.Requests()
	has(t, "what the model was sent on resuming by name", string(reqs[len(reqs)-1].Body), "why is the test flaky?", "go on")
}

// /resume lists the folder's other sessions and opens one in place of the
// session that is open, which is kept.
func TestResumeInsideASession(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "The earlier answer."})
	run(t, w, "-p", "the earlier session's prompt").wantCode(t, 0)
	earlier := w.Sessions()[0]
	id := strings.TrimSuffix(earlier[strings.LastIndex(earlier, "/")+1:], ".jsonl")

	w.Model.Add(apptest.Reply{Text: "A new answer."}, apptest.Reply{Text: "Back in the earlier session."})
	s := sit(t, w)
	s.see("Type a prompt and Enter.")
	s.typeLine("a new session's prompt")
	s.see("A new answer.")
	s.typeLine("/resume")
	s.see("The other sessions of this folder")
	s.see("  1. " + id[:8])
	s.see("the earlier session's prompt")
	s.typeLine("/resume 7")
	s.see("error: ")
	s.typeLine("/resume 1")
	s.see("Session " + id + " is open: the earlier session's prompt. The one before is kept; gnosh -r ")
	s.typeLine("where were we?")
	s.see("Back in the earlier session.")
	reqs := w.Model.Requests()
	sent := string(reqs[len(reqs)-1].Body)
	has(t, "what the model was sent after /resume", sent, "the earlier session's prompt", "The earlier answer.", "where were we?")
	hasNot(t, "what the model was sent after /resume", sent, "a new session's prompt", "A new answer.")
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Errorf("exit code %d", code)
	}
	if n := len(w.Sessions()); n != 2 {
		t.Errorf("%d session files, want the two", n)
	}
	for _, path := range w.Sessions() {
		checkSession(t, w, path)
	}
}

// The subcommands that look at what gnosh keeps, without a model: the
// folders, the log, the models a server offers, and the sessions.
func TestSubcommandsThatLook(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "An answer."})
	run(t, w, "-p", "a prompt to remember").wantCode(t, 0)
	path := w.Sessions()[0]
	id := strings.TrimSuffix(path[strings.LastIndex(path, "/")+1:], ".jsonl")
	asked := len(w.Model.Requests())

	r := run(t, w, "dirs")
	if r.code != 0 {
		t.Fatalf("gnosh dirs: exit code %d\n%s\n%s", r.code, r.out, r.err)
	}
	has(t, "dirs", r.out, "config", w.ConfigDir(), "sessions", "working folder", w.Dir, "gnosh.log", w.ConfigDir()+"/skills")

	r = run(t, w, "logs").wantCode(t, 0)
	has(t, "logs", r.out, "session "+id)
	r = run(t, w, "logs", "--tail", "1").wantCode(t, 0)
	if n := strings.Count(strings.TrimSpace(r.out), "\n"); n != 0 {
		t.Errorf("logs --tail 1 printed %d lines", n+1)
	}

	r = run(t, w, "models").wantCode(t, 0)
	has(t, "models", r.out, "local at ", "local/test-model  (the model sessions use)")

	r = run(t, w, "session", "list").wantCode(t, 0)
	has(t, "session list", r.out, id, "a prompt to remember", "1 prompt(s)")
	r = run(t, w, "session", "list", "--json").wantCode(t, 0)
	has(t, "session list --json", r.out, `"id": "`+id+`"`, `"title": "a prompt to remember"`, `"prompts": 1`)
	r = run(t, w, "session", "last").wantCode(t, 0)
	has(t, "session last", r.out, "session "+id, "> a prompt to remember", "An answer.")
	r = run(t, w, "session", "show", id[:8]).wantCode(t, 0)
	has(t, "session show", r.out, "> a prompt to remember", "An answer.")
	r = run(t, w, "session", "show", "no-such-session").wantCode(t, 1)
	has(t, "session show of nothing", r.err, `no session of this folder matches "no-such-session"`)
	r = run(t, w, "session", "rename", id[:8], "kept", "for", "later").wantCode(t, 0)
	has(t, "session rename", r.out, `The session `+id+` is named "kept for later".`)
	r = run(t, w, "session", "list").wantCode(t, 0)
	has(t, "session list after the rename", r.out, "kept for later")

	if n := len(w.Model.Requests()); n != asked {
		t.Errorf("the subcommands sent the model %d request(s)", n-asked)
	}
}

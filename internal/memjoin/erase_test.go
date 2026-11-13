package memjoin

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/gnovm"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

const token = "tok-live-8842-zebra"

// holders lists the files under dir that hold text, by path relative to
// dir.
func holders(t *testing.T, dir, text string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(data), text) {
			rel, _ := filepath.Rel(dir, path)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

// An erasure, end to end. A secret was kept in mem by mistake in one
// session, and another session was told it in its digest. The person
// redacts it with /mem: mem forgets it, gnosh's own copy in the other
// session's file is written again without it, and the session whose
// visible messages still hold it is named. Nothing of the erasure is
// logged or stored.
func TestErasureScrubsTheDigestInAnotherSessionsFile(t *testing.T) {
	w := newWorld(t)

	// The first session keeps the secret: the call that kept it is a
	// message the person saw.
	w.script.add(
		reply(memCall("c1", `mem.Fact("deploy/token", "the deploy token is `+token+`")`)),
		says("Kept."),
	)
	w.run("-y", "-p", "keep the deploy token").wantCode(t, 0)

	// The second session is told it with its first prompt.
	w.script.add(says("Hello."))
	w.run("-p", "hello there").wantCode(t, 0)
	sessions := w.Sessions()
	if len(sessions) != 2 {
		t.Fatalf("session files: %v", sessions)
	}
	one, two := sessions[0], sessions[1]
	reqs := w.script.requests()
	has(t, "the second session's first prompt", lastUser(t, reqs[len(reqs)-1]), "<mem>", "`deploy/token` = the deploy token is "+token)
	if data, _ := os.ReadFile(two); !strings.Contains(string(data), token) {
		t.Fatalf("the second session's file does not hold the digest's text")
	}

	// The person redacts it, in a third session.
	r := w.run("-p", "/mem redact "+token)
	r.wantCode(t, 0)
	has(t, "what /mem redact says", r.out,
		"what you saw of it is still in 1 session(s), \"keep the deploy token\", and reaches the model again in them: start a new session, or delete those")
	hasNot(t, "what /mem redact says", r.out+r.err, token)

	// The second session's file no longer holds it, and still reads as
	// the conversation it was.
	data, _ := os.ReadFile(two)
	if strings.Contains(string(data), token) {
		t.Errorf("the second session's file still holds the secret")
	}
	has(t, "the second session's file", string(data), "`deploy/token` = the deploy token is (redacted)", "hello there")
	s := w.session(two)
	if warn := s.Warnings(); len(warn) != 0 {
		t.Errorf("the redacted session's file needed mending: %v", warn)
	}
	msgs, _ := s.View(transcript.ViewOptions{})
	if err := transcript.Check(msgs); err != nil {
		t.Errorf("the redacted session does not hold a conversation a provider accepts: %v", err)
	}

	// The secret stands in one place only: the first session's file,
	// where the person saw it. Not in mem, not in a log, not in a
	// leftover of the rewrite.
	// (The project's log, which the program keeps beside the sessions,
	// has one line for each tool call, and so it holds the call that kept
	// the secret, as the first session's file does. No line of it is
	// about the redaction.)
	for _, name := range holders(t, w.Root, token) {
		switch {
		case filepath.Join(w.Root, name) == one:
		case filepath.Base(name) == "gnosh.log":
			data, _ := os.ReadFile(filepath.Join(w.Root, name))
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, token) && !strings.Contains(line, "gnovm_run: mem.Fact(") {
					t.Errorf("the log holds the secret in a line that is not the first session's own call: %s", line)
				}
			}
		default:
			t.Errorf("%s holds the secret after the redaction", name)
		}
	}

	// The second session, taken up again, sends the model the digest
	// without the secret.
	w.script.add(says("Hello again."))
	id := strings.TrimSuffix(filepath.Base(two), ".jsonl")
	w.run("-r", id, "-p", "and again").wantCode(t, 0)
	reqs = w.script.requests()
	body := string(reqs[len(reqs)-1].Body)
	hasNot(t, "the request of the scrubbed session", body, token)
	has(t, "the request of the scrubbed session", body, "the deploy token is (redacted)")

	// Typing the same again finds nothing left in mem, and says so
	// without repeating the secret.
	r = w.run("-p", "/mem redact "+token)
	hasNot(t, "the second /mem redact", r.out+r.err, token)
	w.script.done()
}

// The agent's own erasure is followed the same way, and the person is
// told where the secret is still shown.
func TestAgentRedactIsFollowed(t *testing.T) {
	w := newWorld(t,
		reply(memCall("c1", `mem.Fact("deploy/token", "the deploy token is `+token+`")`)),
		says("Kept."),
	)
	w.run("-y", "-p", "keep the deploy token").wantCode(t, 0)
	w.script.add(says("Hello."))
	w.run("-p", "hello there").wantCode(t, 0)
	two := w.Sessions()[1]

	w.script.add(
		reply(memCall("c2", `mem.Redact("`+token+`")`)),
		says("Redacted."),
	)
	r := w.run("-y", "-p", "that token should not be in mem: take it out")
	r.wantCode(t, 0)
	has(t, "stderr", r.err, "what you saw of it is still in 2 session(s), \"keep the deploy token\", \"that token should not be in mem: take it out\", and reaches the model again in them")
	if data, _ := os.ReadFile(two); strings.Contains(string(data), token) {
		t.Errorf("the session that was told the secret in its digest still holds it in its file")
	}
	w.script.done()
}

// What an erasure does to each kind of entry: gnosh's own copies lose the
// text, what the person saw keeps it, and a session's binding and task
// list follow a rename.
func TestScrubberEdit(t *testing.T) {
	e := gnovm.MemErasure{
		Texts:   map[string]string{token: "(redacted)", "ship the release on friday": "(forgotten)"},
		Names:   []string{"old-fact"},
		Renamed: map[string]string{"deploy-" + "secret": "forgotten-1"},
	}
	sc := &scrubber{e: e}
	hidden := transcript.UserEntry(coreText("<mem>\n- `old-fact` = something\nfacts: the token is "+token+"\n</mem>"), transcript.SourceNotice)
	if !sc.edit(&hidden) {
		t.Fatalf("a hidden part that holds the secret was not changed")
	}
	if got := hidden.Message.Text(); got != "<mem>\nfacts: the token is (redacted)\n</mem>" {
		t.Errorf("the hidden part after the erasure: %q", got)
	}

	typed := transcript.PromptEntry("the token is " + token)
	if sc.edit(&typed) || !strings.Contains(typed.Message.Text(), token) {
		t.Errorf("what the person typed was changed")
	}
	if !sc.shown {
		t.Errorf("a secret in a visible message was not noted")
	}

	summary := transcript.SummaryEntry("We agreed to ship the release on friday.", "")
	if !sc.edit(&summary) || summary.Summary != "We agreed to (forgotten)." {
		t.Errorf("the summary after the erasure: %q", summary.Summary)
	}

	binding := transcript.MetaEntry(metaGoal, encodeGoal(gnovm.GoalBinding{Condition: "use " + token, Name: "deploy-secret", Title: "use " + token, Verdict: "it says " + token}))
	if !sc.edit(&binding) {
		t.Fatalf("a binding that holds the secret was not changed")
	}
	if strings.Contains(binding.Value, token) || !strings.Contains(binding.Value, `"name":"forgotten-1"`) || !strings.Contains(binding.Value, `"condition":"use (redacted)"`) {
		t.Errorf("the binding after the erasure: %s", binding.Value)
	}

	list := []gnovmtools.Todo{{Content: "ship the release on friday", Status: gnovmtools.TodoStatusPending}, {Content: "tag it", Status: gnovmtools.TodoStatusInProgress}}
	tasks := transcript.MetaEntry(metaTasks, mustJSON(t, list))
	if !sc.edit(&tasks) || tasks.Value != mustJSON(t, list[1:]) {
		t.Errorf("the task list after the erasure: %s", tasks.Value)
	}

	frame := transcript.PopEntry("deploy-secret/rotate", "rotate "+token, "rotated "+token, "c1")
	if !sc.edit(&frame) || frame.Frame != "forgotten-1/rotate" || frame.Label != "rotate (redacted)" || frame.Result != "rotated (redacted)" {
		t.Errorf("the frame entry after the erasure: %+v", frame)
	}

	other := transcript.MetaEntry("model", "a model named "+token)
	if sc.edit(&other) {
		t.Errorf("a note that is not the join's own was changed")
	}
}

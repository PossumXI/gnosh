package memjoin

import (
	"encoding/json"
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

	// The call that kept it is in the first session's file and in mem,
	// and not in gnosh's log: what gnovm_run is given and what it answers
	// is kept out of the log, because no erasure could take it out again.
	// The log still says that the tool was called.
	logs, _ := filepath.Glob(filepath.Join(w.DataDir(), "projects", "*", "gnosh.log"))
	if len(logs) != 1 {
		t.Fatalf("the project's logs: %v", logs)
	}
	logged, _ := os.ReadFile(logs[0])
	has(t, "the log", string(logged), "gnovm_run")
	hasNot(t, "the log", string(logged), token, "deploy/token", "mem.Fact")

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
	// where the person saw it. Not in mem, not in the log, which holds
	// neither the call that kept it nor what was typed after /mem
	// redact, and not in a leftover of the rewrite.
	for _, name := range holders(t, w.Root, token) {
		if filepath.Join(w.Root, name) != one {
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

	list := []gnovmtools.TaskLine{
		{Title: "ship the release on friday", AtWork: "ship the release on friday"},
		{Title: "tag it", AtWork: "tag it", State: gnovmtools.LineWorking},
	}
	tasks := transcript.MetaEntry(metaTasks, encodeTasks(list))
	if !sc.edit(&tasks) || tasks.Value != encodeTasks(list[1:]) {
		t.Errorf("the task list after the erasure: %s", tasks.Value)
	}
	// A list kept in a shape this build does not read loses the erased
	// text all the same.
	other := transcript.MetaEntry(metaTasks, `[{"words":"we ship the release on friday","where":"waiting"},{"words":"tag it"}]`)
	if !sc.edit(&other) || other.Value != `[{"where":"waiting","words":"we (forgotten)"},{"words":"tag it"}]` {
		t.Errorf("a task list of another shape after the erasure: %s", other.Value)
	}

	frame := transcript.PopEntry("deploy-secret/rotate", "rotate "+token, "rotated "+token, "c1")
	if !sc.edit(&frame) || frame.Frame != "forgotten-1/rotate" || frame.Label != "rotate (redacted)" || frame.Result != "rotated (redacted)" {
		t.Errorf("the frame entry after the erasure: %+v", frame)
	}

	foreign := transcript.MetaEntry("model", "a model named "+token)
	if sc.edit(&foreign) {
		t.Errorf("a note that is not the join's own was changed")
	}

	// A line gnosh said for itself about mem is a copy of mem's text, kept
	// under the loop's own key. Its text loses what was erased, and the
	// loop's other fields stay as they were, whatever was erased.
	said := func(src, text string) transcript.Entry {
		raw, err := json.Marshal(map[string]string{"kind": "notice", "id": "n1", "src": src, "level": "warning", "text": text})
		if err != nil {
			t.Fatal(err)
		}
		return transcript.Entry{Type: "notice", Extra: map[string]json.RawMessage{"loop": raw}}
	}
	fields := func(en transcript.Entry) map[string]string {
		var f map[string]string
		if err := json.Unmarshal(en.Extra["loop"], &f); err != nil {
			t.Fatalf("the loop's fields no longer read as they were written: %v: %s", err, en.Extra["loop"])
		}
		return f
	}
	sc.held = false
	due := said("mem", `mem due: "ship the release on friday" (ship-release) (since 2026-12-01) <`+token+`>`)
	if !sc.edit(&due) || !sc.held {
		t.Fatalf("a line gnosh said about mem that holds what was erased was not changed")
	}
	if f := fields(due); f["text"] != `mem due: "(forgotten)" (ship-release) (since 2026-12-01) <(redacted)>` || f["kind"] != "notice" || f["id"] != "n1" || f["src"] != "mem" || f["level"] != "warning" {
		t.Errorf("the line after the erasure: %v", f)
	}
	verdict := said("goal", "Goal not met yet: nothing shows that we ship the release on friday")
	if !sc.edit(&verdict) || fields(verdict)["text"] != "Goal not met yet: nothing shows that we (forgotten)" {
		t.Errorf("a goal's line after the erasure: %s", verdict.Extra["loop"])
	}
	// An erased text that is one of the loop's own words changes none of
	// them.
	words := &scrubber{e: gnovm.MemErasure{Texts: map[string]string{"notice": "(redacted)", "warning": "(redacted)"}}}
	plain := said("mem", "mem due: a notice of a warning")
	if !words.edit(&plain) {
		t.Fatalf("a line that holds an erased word was not changed")
	}
	if f := fields(plain); f["kind"] != "notice" || f["level"] != "warning" || f["text"] != "mem due: a (redacted) of a (redacted)" {
		t.Errorf("the loop's own words after an erasure of the same words: %v", f)
	}
	// A line from anywhere else is not mem's text.
	elsewhere := said("config", "the token is "+token)
	if sc.edit(&elsewhere) || !strings.Contains(string(elsewhere.Extra["loop"]), token) {
		t.Errorf("a line that did not come from mem was changed: %s", elsewhere.Extra["loop"])
	}

	// A session is named to the person without what was erased, whether
	// its name is its title or the start of what was first typed in it.
	// The secret stands where the clipping of a long first line would cut
	// it in two: it is taken out before the line is cut.
	named := &scrubber{e: e, prompt: "the deploy token of the staging cluster is now " + token + "\nand more"}
	if got := named.name("0123456789"); got != "the deploy token of the staging cluster is now (redacted)" {
		t.Errorf("a session named by its first prompt: %q", got)
	}
	named = &scrubber{e: e, title: "the " + token + " session"}
	if got := named.name("0123456789"); got != "the (redacted) session" {
		t.Errorf("a session named by its title: %q", got)
	}
	named = &scrubber{e: gnovm.MemErasure{Names: []string{"old-fact"}}, prompt: "forget `old-fact`"}
	if got := named.name("0123456789"); got != "01234567" {
		t.Errorf("a session whose name is all erased: %q", got)
	}
}

// A session that another gnosh has open is not written under its writer.
// An erasure leaves that session's copies as they are, names the session,
// and says what takes a text out of it later. In a session that is
// closed, the line gnosh said at its start about what waits on the person
// loses the text with everything else.
func TestErasureAndTheSessionOpenElsewhere(t *testing.T) {
	const ask = "which key did " + token + " replace?"
	w := newWorld(t,
		reply(memCall("c1", `mem.Todo("rotate the key").Ask("`+ask+`")`)),
		says("Noted."),
	)
	w.run("-y", "-p", "remind me to rotate the key").wantCode(t, 0)

	// A second session starts and ends without a prompt. gnosh said what
	// waits on the person, and the line is in its file.
	r := w.run("-p", "/mem")
	r.wantCode(t, 0)
	has(t, "stderr as the second session starts", r.err, `mem waiting on you: "`+ask+`"`)
	if n := len(w.Sessions()); n != 2 {
		t.Fatalf("%d session files after the second session: %v", n, w.Sessions())
	}
	two := w.Sessions()[1]
	if data, _ := os.ReadFile(two); !strings.Contains(string(data), token) {
		t.Fatalf("the second session's file does not hold the line gnosh said as it started")
	}

	// A person sits in a third, which is told the same with its first
	// prompt, and stays in it.
	w.script.add(says("Hello."))
	s := w.sit()
	s.see("Type a prompt and Enter.")
	s.typeLine("hello there")
	s.see("Hello.")
	if n := len(w.Sessions()); n != 3 {
		t.Fatalf("%d session files with the third session open: %v", n, w.Sessions())
	}
	three := w.Sessions()[2]

	// The secret is redacted from a fourth while the third is open.
	r = w.run("-p", "/mem redact "+token)
	r.wantCode(t, 0)
	has(t, "what /mem redact says", r.out,
		`1 session(s) are open in another gnosh, "hello there", so gnosh's own copies in them were left as they are: to take a text out of them too, close them and run /mem redact with that text`)
	hasNot(t, "what /mem redact says", r.out, token)

	// The closed session's line went; the open session's file is as its
	// writer left it.
	data, _ := os.ReadFile(two)
	hasNot(t, "the second session's file", string(data), token)
	has(t, "the second session's file", string(data), `mem waiting on you: \"which key did (redacted) replace?\"`)
	if data, _ := os.ReadFile(three); !strings.Contains(string(data), token) {
		t.Fatalf("the open session's file was written under its writer")
	}

	// The person leaves the third, and types the same again: now it is
	// reached, and the secret stands only where the person saw it, in the
	// first session's file.
	s.typeLine("/exit")
	if code := s.exit(); code != 0 {
		t.Fatalf("the third session ended with code %d\n%s", code, s.out.String())
	}
	r = w.run("-p", "/mem redact "+token)
	r.wantCode(t, 0)
	has(t, "the second /mem redact", r.out, "It was taken out of gnosh's own copies in 1 session(s).")
	hasNot(t, "the second /mem redact", r.out+r.err, token, "open in another gnosh")
	one := w.Sessions()[0]
	for _, name := range holders(t, w.Root, token) {
		if filepath.Join(w.Root, name) != one {
			t.Errorf("%s holds the secret after the second redaction", name)
		}
	}
	for _, path := range w.Sessions() {
		sess := w.session(path)
		if warn := sess.Warnings(); len(warn) != 0 {
			t.Errorf("%s needed mending after the redactions: %v", filepath.Base(path), warn)
		}
	}
	w.script.done()
}

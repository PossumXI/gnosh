package transcript

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

const secret = "sk-live-4f9a0c17d2"

// scrub is the edit a caller of Redact brings: it takes the secret out of
// the hidden parts, the summaries and the notes of a session, and leaves
// what the person saw.
func scrub(e *Entry) bool {
	changed := false
	swap := func(s *string) {
		if strings.Contains(*s, secret) {
			*s = strings.ReplaceAll(*s, secret, "(redacted)")
			changed = true
		}
	}
	switch e.Type {
	case TypeUser:
		if e.Source == SourcePrompt || e.Source == SourceToolResults {
			return false
		}
		for i := range e.Message.Blocks {
			swap(&e.Message.Blocks[i].Text)
		}
	case TypeSummary:
		swap(&e.Summary)
	case TypeMeta:
		swap(&e.Value)
	case TypeFrame:
		swap(&e.Label)
		swap(&e.Result)
	}
	for name, raw := range e.Extra {
		if text := string(raw); strings.Contains(text, secret) {
			e.Extra[name] = json.RawMessage(strings.ReplaceAll(text, secret, "(redacted)"))
			changed = true
		}
	}
	return changed
}

// A redaction takes the text out of the session's file wherever the edit
// says, keeps every entry in its place, and leaves the session one that
// can be written to, opened again and viewed.
func TestRedact(t *testing.T) {
	st, _ := newStore(t)
	s, err := st.Create(cwd)
	if err != nil {
		t.Fatal(err)
	}
	hidden := UserEntry(core.UserText("<mem>\nfacts: `api-key` = "+secret+"\n</mem>"), SourceNotice)
	hidden.Extra = map[string]json.RawMessage{"loop": json.RawMessage(`{"kind":"note","src":"mem:digest","text":"` + secret + `"}`)}
	unknown := Entry{Type: "from-a-newer-gnosh", Extra: map[string]json.RawMessage{"payload": json.RawMessage(`{"k":"v"}`)}}
	written := mustAppend(t, s,
		PromptEntry("remember the key "+secret),
		hidden,
		AssistantEntry(core.AssistantText("I kept "+secret+" as a fact."), core.StopEndTurn),
		SummaryEntry("The user gave the key "+secret+".", ""),
		MetaEntry("mem.goal", `{"condition":"use `+secret+`"}`),
		PopEntry("keep-key", "keep "+secret, "kept "+secret, "c1"),
		unknown,
		MetaEntry("model", "qwen"),
	)
	before := fileLines(t, s.Path())

	changed, err := s.Redact(scrub)
	if err != nil {
		t.Fatal(err)
	}
	if changed != 4 {
		t.Errorf("Redact changed %d entries, want 4: the hidden part, the summary, the binding and the frame", changed)
	}

	// The file: the same number of lines, the same ids in the same
	// order, and the secret only where the person saw it.
	after := fileLines(t, s.Path())
	if len(after) != len(before) {
		t.Fatalf("the file has %d lines after the redaction and had %d", len(after), len(before))
	}
	for i, line := range after {
		var a, b struct {
			Type, UUID, ParentUUID, SessionID, Timestamp string
		}
		if err := json.Unmarshal([]byte(line), &a); err != nil {
			t.Fatalf("line %d is not JSON after the redaction: %v", i+1, err)
		}
		_ = json.Unmarshal([]byte(before[i]), &b)
		if a != b {
			t.Errorf("line %d: the entry's type, ids or time changed: %+v, was %+v", i+1, a, b)
		}
		holds := strings.Contains(line, secret)
		visible := i == 0 || i == 2 // the prompt and the reply
		if holds != visible {
			t.Errorf("line %d holds the secret: %v, want %v\n%s", i+1, holds, visible, line)
		}
		if !visible && i != 6 && i != 7 && !strings.Contains(line, "(redacted)") {
			t.Errorf("line %d does not say that something was redacted: %s", i+1, line)
		}
	}
	if after[6] != before[6] || after[7] != before[7] {
		t.Errorf("entries the edit did not change were written differently:\n%s\n%s", after[6], after[7])
	}
	if !strings.Contains(after[1], `"loop":{"kind":"note","src":"mem:digest","text":"(redacted)"}`) {
		t.Errorf("a field this version does not know was not written back as the edit left it: %s", after[1])
	}

	// In memory the session holds what the file holds.
	entries := s.Entries()
	if got := entries[1].Message.Text(); got != "<mem>\nfacts: `api-key` = (redacted)\n</mem>" {
		t.Errorf("the hidden part in memory: %q", got)
	}
	if entries[3].Summary != "The user gave the key (redacted)." || entries[5].Result != "kept (redacted)" {
		t.Errorf("the summary and the frame in memory: %q, %q", entries[3].Summary, entries[5].Result)
	}
	for i := range entries {
		if entries[i].UUID != written[i].UUID {
			t.Errorf("entry %d changed its id", i)
		}
	}

	// Nothing was left beside the file.
	names, _ := os.ReadDir(filepath.Dir(s.Path()))
	if len(names) != 1 {
		t.Errorf("the session's directory holds %d files after the redaction, want the session's own alone", len(names))
	}

	// The writer's lock went over to the new file: nobody else can write
	// the session, and this session still can.
	if _, err := st.Open(ctx, s.ID()); err == nil {
		t.Errorf("a second writer could open the session after its redaction")
	} else {
		var locked *LockedError
		if !errors.As(err, &locked) {
			t.Errorf("opening the redacted session for writing: %v, want a *LockedError", err)
		}
	}
	next := mustAppend(t, s, PromptEntry("carry on"))
	if next[0].ParentUUID != written[len(written)-1].UUID {
		t.Errorf("the entry appended after the redaction does not follow the file's last entry")
	}
	if lines := fileLines(t, s.Path()); len(lines) != len(before)+1 {
		t.Errorf("the file has %d lines after one more entry, want %d", len(lines), len(before)+1)
	}

	// A second redaction with nothing to do writes nothing.
	info, _ := os.Stat(s.Path())
	if changed, err := s.Redact(scrub); err != nil || changed != 0 {
		t.Errorf("a redaction with nothing to change: %d, %v", changed, err)
	}
	if again, _ := os.Stat(s.Path()); !os.SameFile(info, again) {
		t.Errorf("a redaction with nothing to change wrote the file again")
	}

	// Opened again, the file reads without a warning and gives the same
	// conversation.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := st.Open(ctx, s.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if w := r.Warnings(); len(w) != 0 {
		t.Errorf("the redacted file needed mending: %v", w)
	}
	if got := r.Entries(); len(got) != len(entries)+1 || got[1].Message.Text() != entries[1].Message.Text() || got[6].Type != "from-a-newer-gnosh" {
		t.Errorf("the redacted file does not read back as the session held it")
	}
	msgs, _ := r.View(ViewOptions{})
	if err := Check(msgs); err != nil {
		t.Errorf("the redacted session does not give a conversation a provider accepts: %v", err)
	}
}

// What a redaction may not do, and what it leaves alone.
func TestRedactRefusals(t *testing.T) {
	st, _ := newStore(t)
	s, err := st.Create(cwd)
	if err != nil {
		t.Fatal(err)
	}
	// A session with no entries has no file, and nothing to change.
	if changed, err := s.Redact(scrub); err != nil || changed != 0 {
		t.Errorf("redacting a session with no entries: %d, %v", changed, err)
	}
	mustAppend(t, s, SummaryEntry("the key is "+secret, ""))

	// The edit cannot move an entry or change what it is.
	changed, err := s.Redact(func(e *Entry) bool {
		e.UUID, e.ParentUUID, e.SessionID, e.Type = "other", "other", "other", TypeMeta
		e.Summary = "the key is gone"
		return true
	})
	if err != nil || changed != 1 {
		t.Fatalf("Redact: %d, %v", changed, err)
	}
	if e := s.Entries()[0]; e.Type != TypeSummary || e.UUID == "other" || e.SessionID != s.ID() || e.Summary != "the key is gone" {
		t.Errorf("the edit changed what the entry is: %+v", e)
	}

	// Another gnosh's session is not rewritten under it.
	if _, err := st.Redact(ctx, s.Path(), scrub); err == nil {
		t.Errorf("a session open for writing elsewhere was redacted")
	} else {
		var locked *LockedError
		if !errors.As(err, &locked) {
			t.Errorf("redacting a session that is open elsewhere: %v, want a *LockedError", err)
		}
	}

	// A read-only session and a closed one are not redacted.
	ro, err := st.OpenReadOnly(ctx, s.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ro.Redact(scrub); !errors.Is(err, ErrReadOnly) {
		t.Errorf("redacting a read-only session: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Redact(scrub); !errors.Is(err, ErrClosed) {
		t.Errorf("redacting a closed session: %v", err)
	}

	// Closed, the file can be redacted through the store.
	mustWrite := func(text string) {
		w, err := st.Open(ctx, s.ID())
		if err != nil {
			t.Fatal(err)
		}
		mustAppend(t, w, SummaryEntry(text, ""))
		w.Close()
	}
	mustWrite("the key is " + secret + " again")
	changed, err = st.Redact(ctx, s.Path(), scrub)
	if err != nil || changed != 1 {
		t.Fatalf("Store.Redact: %d, %v", changed, err)
	}
	data, _ := os.ReadFile(s.Path())
	if strings.Contains(string(data), secret) {
		t.Errorf("the file still holds the secret after Store.Redact:\n%s", data)
	}
	// And the lock was let go with it.
	w, err := st.Open(ctx, s.ID())
	if err != nil {
		t.Fatalf("the session cannot be opened for writing after Store.Redact: %v", err)
	}
	w.Close()
}

// A branch's own file is redacted; what it inherited is changed in memory
// only, and its parent's file is the parent's to redact.
func TestRedactBranch(t *testing.T) {
	st, _ := newStore(t)
	parent, err := st.Create(cwd)
	if err != nil {
		t.Fatal(err)
	}
	mustAppend(t, parent, PromptEntry("hello"), SummaryEntry("the key is "+secret, ""))
	branch, err := st.Branch(parent, "")
	if err != nil {
		t.Fatal(err)
	}
	defer branch.Close()
	mustAppend(t, branch, SummaryEntry("still "+secret, ""))
	parentBefore, _ := os.ReadFile(parent.Path())

	changed, err := branch.Redact(scrub)
	if err != nil || changed != 1 {
		t.Fatalf("Redact of the branch: %d, %v", changed, err)
	}
	if data, _ := os.ReadFile(branch.Path()); strings.Contains(string(data), secret) {
		t.Errorf("the branch's file still holds the secret")
	}
	if data, _ := os.ReadFile(parent.Path()); string(data) != string(parentBefore) {
		t.Errorf("redacting a branch changed its parent's file")
	}
	for _, e := range branch.Entries() {
		if strings.Contains(e.Summary, secret) {
			t.Errorf("the branch still holds the secret in memory, in an entry it inherited or wrote: %q", e.Summary)
		}
	}
	if branch.Inherited() != 2 {
		t.Errorf("Inherited = %d after the redaction", branch.Inherited())
	}

	// The parent's own file is redacted by the parent.
	if changed, err := parent.Redact(scrub); err != nil || changed != 1 {
		t.Fatalf("Redact of the parent: %d, %v", changed, err)
	}
	parent.Close()
	if data, _ := os.ReadFile(parent.Path()); strings.Contains(string(data), secret) {
		t.Errorf("the parent's file still holds the secret")
	}
}

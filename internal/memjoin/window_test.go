package memjoin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/config"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// reviewOff makes a session whose closes are checked by the free checks
// alone.
func reviewOff(s *app.Session) {
	cfg := &config.Config{}
	cfg.Permissions.Mode.V = "default"
	cfg.Options.AutoSummarize.V = true
	cfg.Options.MemReviewCloses.V = false
	s.Config = cfg
}

// What counts as "in the todo" (close-check.md section 3.2), with the
// real mem behind the tool and the free checks alone.
func TestCloseWindow(t *testing.T) {
	b := newBench(t, Options{}, reviewOff)
	write := func(name, text string) string {
		path := filepath.Join(b.project, name)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	readme := write("README.md", "# Docs\n\nPick a color.\n")
	notes := write("NOTES.md", "notes\n")
	b.prompt("tidy the docs")

	// A whole plan worked from its root: the todo's work was done before
	// it was entered. Entering and closing it in one call is then no
	// "entered here", and its claim is backed by the edit made at the
	// root (fix A).
	b.mem(`mem.Todo("tidy the docs", "fix the typo in README.md", "add a note").Push()`)
	b.call("Read", map[string]any{"file_path": readme})
	b.call("Edit", map[string]any{"file_path": readme, "old_string": "color", "new_string": "colour"})
	out := b.mem("mem.Next()\nmem.Pop(\"README.md now says colour\")")
	has(t, "a todo of a plan, entered and closed after its work was done at the plan's root", out, "done `tidy-docs/fix-typo-readme-md`")

	// Closed by its name from the plan's root, with a claim about a file
	// nothing has touched: refused, in the words for a close by name.
	out = b.mem(`mem.Done("tidy-docs/add-note", "CHANGES.md now lists the fix")`)
	if !strings.Contains(out, "ERROR — nothing from this call was kept: not closed: nothing in `tidy-docs/add-note` changed or showed `CHANGES.md`") {
		t.Errorf("a close by name with an unbacked claim:\n%s", out)
	}
	// The same, with the file changed since the plan was made: kept.
	b.call("Read", map[string]any{"file_path": notes})
	b.call("Edit", map[string]any{"file_path": notes, "old_string": "notes", "new_string": "notes: README.md says colour"})
	out = b.mem(`mem.Done("tidy-docs/add-note", "NOTES.md now says what changed")`)
	has(t, "a close by name whose claim is backed", out, "done `tidy-docs/add-note`")

	// A close by its name outside any plan: only what came after the
	// session's last push or pop is the todo's. The exact words of the
	// refusal are the ones for a close by name: "closed", and no word of
	// stepping out.
	b.mem(`mem.Pop("both todos are done: README.md and NOTES.md were edited")`)
	b.mem(`mem.Later("write the changelog")`)
	out = b.mem(`mem.Done("write-changelog", "NOTES.md updated with the changelog")`)
	want := "ERROR — nothing from this call was kept: " + prompt.Builtin().CloseUnbacked("write-changelog", "NOTES.md", true)
	if !strings.HasPrefix(out, want) {
		t.Errorf("a close by name of a todo outside the plan, whose file was changed in a sibling's work:\n%s\nwant it to begin:\n%s", out, want)
	}
	has(t, "the words of a refused close by name", want,
		"not closed: nothing in `write-changelog` changed or showed `NOTES.md`, which your result says changed. If the change is still to make, make it; if it was made before, read it here so it shows; then close `write-changelog` with what came of it.")
	hasNot(t, "the words of a refused close by name", want, "mem.Leave")

	// A result that says a file was made, when there is none.
	out = b.mem(`mem.Done("write-changelog", "created CHANGELOG.md with the fix")`)
	if want := "ERROR — nothing from this call was kept: not closed: nothing in `write-changelog` made `CHANGELOG.md`, which your result says was made, and there is no such file. Make it, then close `write-changelog` with what came of it; or close it with what you did instead."; !strings.HasPrefix(out, want) {
		t.Errorf("a close that says a file was made:\n%s\nwant it to begin:\n%s", out, want)
	}
	// A command that changed the file backs the claim through the file's
	// own times.
	write("CHANGELOG.md", "- the typo fix\n")
	out = b.mem(`mem.Done("write-changelog", "created CHANGELOG.md with the fix")`)
	has(t, "a close whose file was made by something other than the file tools", out, "done `write-changelog`")
}

// A turn that was stopped while a close was being checked is not a pass.
func TestCloseCheckOfAStoppedTurn(t *testing.T) {
	b := newBench(t, Options{}, nil)
	j := b.join()
	b.prompt("fix it")
	b.mem(`mem.Todo("fix the flaky test", "find the cause").Push()`)
	b.mem(`mem.Next()`)
	path := filepath.Join(b.project, "a_test.go")
	os.WriteFile(path, []byte("package a\n"), 0o644)
	b.call("Read", map[string]any{"file_path": path})

	raw, _ := json.Marshal(map[string]string{"name": "mem", "code": `mem.Pop("it races on the port")`})
	b.append(agent.Entry{Kind: agent.EntryAssistant, Message: core.Message{Role: core.RoleAssistant, Blocks: []core.Block{core.ToolUse("closing", gnovmtools.RunToolName, raw)}}, StopReason: core.StopToolUse})
	closes := []gnovm.MemClose{{Id: "fix-flaky-test/find-cause", Title: "find the cause", Result: "it races on the port"}}

	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	if got, want := j.checkCloses(stopped, j.id, "closing", closes), "not popped: the check of `fix-flaky-test/find-cause` was cancelled, so nothing was kept"; got != want {
		t.Errorf("the check of a close in a stopped turn: %q, want %q", got, want)
	}

	// With no model to ask, the close is kept unchecked, and the person
	// is told so.
	if got := j.checkCloses(context.Background(), j.id, "closing", closes); got != "" {
		t.Errorf("a close whose review cannot run was refused: %q", got)
	}
	b.wait()
	has(t, "what the person was told", b.told(), "The close of `fix-flaky-test/find-cause` could not be checked, so it was kept unchecked: there is no model to ask")
}

// A repeated call on mem is found repeated though its answers differ in
// their numbers, and a repeated refused close gets words of its own.
func TestRepeatKeyAndNudge(t *testing.T) {
	b := newBench(t, Options{}, nil)
	j := b.join()
	input, _ := json.Marshal(map[string]string{"name": "mem", "code": "mem.Show()"})
	call := core.ToolCall{Name: gnovmtools.RunToolName, Input: input}
	one, ok1 := j.repeatKey(call, core.TextResult("mem is empty\nok · mem · ran 31ms · nothing to keep\n"))
	two, ok2 := j.repeatKey(call, core.TextResult("mem is empty\nok · mem · ran 178ms · nothing to keep\n"))
	if !ok1 || !ok2 || one != two {
		t.Errorf("two answers of mem that differ in a duration are not the same step:\n%q\n%q", one, two)
	}
	other, _ := j.repeatKey(call, core.TextResult("1 todo open\nok · mem · ran 31ms · nothing to keep\n"))
	if other == one {
		t.Errorf("two answers of mem that differ in their words are the same step")
	}
	scratch, _ := json.Marshal(map[string]string{"code": "1 + 1"})
	if _, ok := j.repeatKey(core.ToolCall{Name: gnovmtools.RunToolName, Input: scratch}, core.TextResult("2")); ok {
		t.Errorf("a call on a scratch machine was given mem's key")
	}
	if _, ok := j.repeatKey(core.ToolCall{Name: "Bash"}, core.TextResult("12 passed in 3.1s")); ok {
		t.Errorf("another tool's call was given mem's key")
	}

	const refusal = "not popped: nothing came between your entering `fix-flaky/find-cause` \"find the cause\" and this pop, so it is not done. Do it, or check that it is done, then pop it."
	pop, _ := json.Marshal(map[string]string{"name": "mem", "code": `mem.Pop("done")`})
	rep := agent.Repetition{Rounds: 6, Window: 10, Calls: []agent.RepeatedCall{{
		Call:   core.ToolCall{Name: gnovmtools.RunToolName, Input: pop},
		Result: core.ErrorResult("ERROR — nothing from this call was kept: " + refusal + "\nfailed · mem · ran 58ms · nothing kept\n"),
	}}}
	if got, want := j.repeatNudge(rep), prompt.Builtin().RepeatRefusedClose(refusal); got != want {
		t.Errorf("the nudge for a repeated refused close:\n%s\nwant:\n%s", got, want)
	}
	rep.Calls[0].Result = core.TextResult("mem is empty")
	if got := j.repeatNudge(rep); got != "" {
		t.Errorf("a repeated call that is no refused close got the join's words: %s", got)
	}
}

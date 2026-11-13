package memjoin

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/tools/fstools"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// keptCall is what the frames of one kept mem call did: the frame entries
// that record them in the session's log, and for each pop what it took out
// of the model's context.
type keptCall struct {
	entries []transcript.Entry
	pops    []popped
}

// popped is one pop as the person is told of it.
type popped struct {
	frame  string
	tokens int
}

// frameEntries turns the frames a mem call reports into the frame entries
// that record them, in the call's order, and returns beside each entry how
// many thoughts its frame held (a pop's only). Only the events this
// session made count: a copy of another session's rows carries that
// session's events, and such a copy starts at the root.
func (j *join) frameEntries(callID string, frames []gnovm.MemFrame) ([]transcript.Entry, []int) {
	var entries []transcript.Entry
	var thoughts []int
	for _, f := range frames {
		if f.Session != j.id || f.Id == "" {
			continue
		}
		switch f.Kind {
		case "push":
			entries = append(entries, transcript.PushEntry(f.Id, f.Purpose, callID))
			thoughts = append(thoughts, 0)
		case "pop":
			entries = append(entries, transcript.PopEntry(f.Id, f.Purpose, f.Result, callID))
			thoughts = append(thoughts, f.Thoughts)
		}
	}
	return entries, thoughts
}

// onMemFrames is the tools' OnMemFrames slot. mem has kept a call, and
// hands back the frames the call pushed and popped. The reply that made
// the call is in the log by now; the call's own result is not, because the
// text returned here goes into it.
//
// For every pop, what it removes is worked out once, by the same code that
// cuts the model's view (transcript.Preview). From that come the clauses
// the model reads in the pop's result, the reads to forget and the number
// the person is told. The frame entries themselves are written beside the
// call's result when the loop stores it (onStored), so that neither is in
// the file without the other.
//
// It is called with a context that stopping the turn does not cancel: what
// mem kept stands, whatever happens to the turn.
func (j *join) onMemFrames(_ context.Context, _ string, callID string, frames []gnovm.MemFrame) string {
	entries, thoughts := j.frameEntries(callID, frames)
	if len(entries) == 0 {
		return ""
	}
	cuts, _ := j.ts.Preview(entries, transcript.ViewOptions{})
	k := &keptCall{entries: entries}
	var lines []string
	next := 0
	for i, e := range entries {
		if e.Op != transcript.OpPop || next >= len(cuts) {
			continue
		}
		cut := cuts[next]
		next++
		lines = append(lines, j.popReceipt(cut, thoughts[i]))
		if cut.Tokens > 0 {
			k.pops = append(k.pops, popped{frame: cut.Frame, tokens: cut.Tokens})
		}
	}

	// The loop stores a call's result once the call has returned. One that
	// did not stop in time after the turn was stopped has had a result
	// stored for it already, which said that it did not stop; its frames
	// are then written here, late, because the stack must have them.
	j.mu.Lock()
	late := j.stored[callID]
	if !late {
		j.kept[callID] = k
	}
	j.mu.Unlock()
	if late {
		if _, err := j.ts.AppendAll(entries...); err != nil {
			j.notify.tell(agent.LevelError, "mem", fmt.Sprintf(
				"mem kept a call that entered or left a todo, and that could not be written to the session's file (%v). mem.Show() says where the session stands.", err))
		}
		j.tellPopped(k)
	}
	return strings.Join(lines, "\n")
}

// popReceipt writes what a pop's own result tells the model about its
// context (frames.md section 5), and forgets the reads the pop made void.
func (j *join) popReceipt(cut transcript.Cut, thoughts int) string {
	if cut.Between == 0 {
		// Nothing came between entering the todo and leaving it, so
		// nothing was removed and nothing is forgotten.
		return j.texts.PopReceipt(prompt.PopReceipt{Frame: cut.Frame, Nothing: true})
	}
	env := j.env()
	r := prompt.PopReceipt{Frame: cut.Frame, Spared: cut.Spared > 0, Thoughts: thoughts}
	names := fstools.Defaults()
	var touched []string
	for _, reply := range cut.Replies {
		for _, c := range reply.Calls {
			if c.Path != "" {
				touched = append(touched, j.expand(c.Path))
			}
			done := c.Answered && !c.IsError
			switch {
			case !done:
			case c.Name == names.EditName || c.Name == names.WriteName:
				if file := j.shown(c.Path, env.Cwd); file != "" && !slices.Contains(r.Files, file) {
					r.Files = append(r.Files, file)
				}
			case c.Name == gnovmtools.RunToolName:
				if code, ok := memCode(c.Input); ok && !failedLine(c.Content) {
					r.Changes = append(r.Changes, memChanges(code)...)
				}
			}
		}
	}
	// The model no longer has what its reads vouched for: every file the
	// removed work touched must be read again before it is changed.
	r.Reads = len(fstools.Forget(env, touched...)) > 0
	return j.texts.PopReceipt(r)
}

// expand makes a path a model wrote one the file tools know a file by: a
// leading ~ is the home directory.
func (j *join) expand(path string) string {
	home := j.s.Dirs.Home
	switch {
	case home == "":
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	}
	return path
}

// shown is a file as the model is told of it: relative to the working
// directory where it lies under it.
func (j *join) shown(path, cwd string) string {
	path = j.expand(path)
	if path == "" {
		return ""
	}
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(filepath.Clean(path))
	}
	path = filepath.Clean(path)
	if cwd != "" {
		if rel, err := filepath.Rel(cwd, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(rel)
		}
	}
	return path
}

// memCode returns the code of a gnovm_run call on mem, and whether the
// call was one.
func memCode(input json.RawMessage) (string, bool) {
	var p gnovmtools.RunParams
	if json.Unmarshal(input, &p) != nil || strings.TrimSpace(p.Name) != gnovm.MemVM {
		return "", false
	}
	return p.Code, true
}

// failedLine reports whether a gnovm_run result says, in its status line,
// that the call failed and kept nothing.
func failedLine(content []core.Block) bool {
	for _, line := range strings.Split(core.TextOf(content), "\n") {
		if strings.HasPrefix(line, "failed · ") {
			return true
		}
	}
	return false
}

// The calls on mem that a pop's receipt does not list as changes: thoughts
// (mem's own lines list those), the frame calls themselves, and reads.
var notChanges = map[string]bool{
	"Think": true, "Thought": true, "Push": true, "Pop": true, "Leave": true, "Frame": true,
	"Recall": true, "Show": true, "Find": true, "Next": true, "JSON": true, "Check": true,
	"String": true, "Bind": true, "Apply": true, "Help": true,
}

// The verbs of a todo, called on mem.Frame("x"), that the receipt lists.
var frameVerbs = map[string]bool{
	"Done": true, "Drop": true, "Reopen": true, "Note": true, "Wait": true, "Ask": true, "Todo": true, "Parallel": true,
}

var (
	// memCallRe is a line that begins with a call on mem, with the call's
	// word and, when its first argument is a string, that string.
	memCallRe = regexp.MustCompile(`^mem\.([A-Z][A-Za-z]*)\(\s*(?:("(?:[^"\\]|\\.)*"|` + "`[^`]*`" + `)\s*([,)]))?`)
	// frameCallRe is a line that begins with a verb on a todo named
	// outright.
	frameCallRe = regexp.MustCompile(`^mem\.Frame\(\s*("(?:[^"\\]|\\.)*"|` + "`[^`]*`" + `)\s*\)\.([A-Z][A-Za-z]*)\(`)
)

// memChanges lists what a mem call's code changed, as a pop's receipt
// names it: the call's word, and its first argument in backquotes when
// that is a string. A line is one statement of the code.
func memChanges(code string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(code, func(r rune) bool { return r == '\n' || r == ';' }) {
		line = strings.TrimSpace(line)
		if m := frameCallRe.FindStringSubmatch(line); m != nil {
			if frameVerbs[m[2]] {
				out = append(out, m[2]+" `"+unquote(m[1])+"`")
			}
			continue
		}
		m := memCallRe.FindStringSubmatch(line)
		if m == nil || notChanges[m[1]] {
			continue
		}
		word, arg, after := m[1], m[2], m[3]
		if word == "Fact" && arg != "" && after == ")" && nameLike(unquote(arg)) {
			continue // mem.Fact("name") alone reads a fact
		}
		if arg != "" {
			out = append(out, word+" `"+unquote(arg)+"`")
		} else {
			out = append(out, word)
		}
	}
	return out
}

// unquote reads a Go string literal, and returns it as it stands when it
// cannot be read.
func unquote(lit string) string {
	if s, err := strconv.Unquote(lit); err == nil {
		return s
	}
	return strings.Trim(lit, "\"`")
}

// nameRe is a name as mem writes one: lowercase words joined by dashes, in
// segments joined by slashes.
var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(/[a-z0-9]+(-[a-z0-9]+)*)*$`)

// nameLike reports whether s reads as a name in mem rather than as text.
func nameLike(s string) bool { return nameRe.MatchString(s) }

// onStored is Contribution.Stored: it is shown every entry the loop
// appends to the session's log, just before it is written, and returns
// the entries that go into the file right after it, in the same write.
//
// For the result of a gnovm_run call that pushed or popped, those are the
// frame entries. From them alone the session's stack and every cut are
// derived, now and after a restart. They are taken from what onMemFrames
// kept for the call, or else from the frames the tool keeps in the
// result's own metadata, which are the same events.
//
// It runs while the log is being written, so it touches nothing but the
// join's own memory.
func (j *join) onStored(e agent.Entry) []transcript.Entry {
	j.count(e)
	if e.Kind != agent.EntryResult || e.ToolName != gnovmtools.RunToolName {
		return j.marks(e)
	}
	j.mu.Lock()
	k := j.kept[e.ToolUseID]
	j.stored[e.ToolUseID] = true
	j.mu.Unlock()
	if k != nil {
		return slices.Clone(k.entries)
	}
	entries, _ := j.frameEntries(e.ToolUseID, gnovmtools.FramesOf(e.Result.Meta))
	return entries
}

// afterStored is Contribution.AfterStored: an entry of the loop's is in
// the session's file, with what was written beside it. A pop that was
// recorded with a call's result is now in force, and the person is told
// what it took out of the model's context.
//
// It runs under the loop's hold on the log, so nothing here may write to
// the log or wait for it: what is told goes through the join's own queue.
func (j *join) afterStored(_ context.Context, e agent.Entry, _ []transcript.Entry) {
	if e.Kind != agent.EntryResult || e.ToolName != gnovmtools.RunToolName {
		return
	}
	j.mu.Lock()
	k := j.kept[e.ToolUseID]
	delete(j.kept, e.ToolUseID)
	j.mu.Unlock()
	if k != nil {
		j.tellPopped(k)
	}
}

// tellPopped tells the person what each pop of a call took out of the
// model's context. The model itself is given no number.
func (j *join) tellPopped(k *keptCall) {
	for _, p := range k.pops {
		j.notify.tell(agent.LevelInfo, "mem", fmt.Sprintf("popped `%s`: about %s tokens out of the model's context", p.frame, tokenCount(p.tokens)))
	}
}

// slid is Contribution.Slid: the wiring's room-maker has slid the oldest
// part of the conversation out of the model's view and recorded it. The
// reads made in what slid out are forgotten, as a pop's are; the wiring
// itself tells the model (in a reminder, or the next frame line does) and
// the person.
func (j *join) slid(_ context.Context, cuts []transcript.Cut) {
	env := j.env()
	var touched []string
	for _, cut := range cuts {
		for _, reply := range cut.Replies {
			for _, c := range reply.Calls {
				if c.Path != "" {
					touched = append(touched, j.expand(c.Path))
				}
			}
		}
	}
	fstools.Forget(env, touched...)
}

// tokenCount writes a number of tokens the way gnosh shows them: 850, 44K,
// 1.2M.
func tokenCount(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(max(n, 0))
	case n < 999500:
		return strconv.Itoa((n+500)/1000) + "K"
	}
	return strconv.FormatFloat(float64(n)/1e6, 'f', 1, 64) + "M"
}

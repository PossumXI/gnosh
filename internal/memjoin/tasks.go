package memjoin

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// taskState is the session's task list: mem's todos for the last task a
// call of the session changed. mem is the one task list, so this is only
// ever written from what mem hands back. It is guarded by the join's
// lock.
type taskState struct {
	list []gnovmtools.TaskLine
}

// load reads the task list from the session's log: the newest one, the
// one a copy of another session carried included.
func (t *taskState) load(entries []transcript.Entry) {
	for i := len(entries) - 1; i >= 0; i-- {
		if e := &entries[i]; e.Type == transcript.TypeMeta && e.Key == metaTasks {
			if list, ok := decodeTasks(e.Value); ok {
				t.list = list
			}
			return
		}
	}
}

// keptLine is a line of the task list as a session's file keeps it, in
// the meta entry mem.tasks: the todo's words, how the line reads while it
// is the one being worked where that is something else, and its state as
// the word the state gives itself.
type keptLine struct {
	Name   string `json:"name,omitempty"`
	Title  string `json:"title"`
	AtWork string `json:"at_work,omitempty"`
	State  string `json:"state"`
}

// encodeTasks writes a task list as a session's file keeps it.
func encodeTasks(list []gnovmtools.TaskLine) string {
	kept := make([]keptLine, len(list))
	for i, l := range list {
		kept[i] = keptLine{Name: l.Name, Title: l.Title, State: l.State.String()}
		if l.AtWork != l.Title {
			kept[i].AtWork = l.AtWork
		}
	}
	raw, _ := json.Marshal(kept) // strings and nothing else: it cannot fail
	return string(raw)
}

// decodeTasks reads a task list as a session's file keeps it. ok is false
// for a value that is not one: a line with no words, or a state that is
// none of the three. A list that is not read is not shown; the session's
// list comes back with its next kept call on mem, which holds it.
func decodeTasks(value string) (list []gnovmtools.TaskLine, ok bool) {
	var kept []keptLine
	if json.Unmarshal([]byte(value), &kept) != nil {
		return nil, false
	}
	list = make([]gnovmtools.TaskLine, 0, len(kept))
	for _, k := range kept {
		state, known := lineState(k.State)
		if k.Title == "" || !known {
			return nil, false
		}
		l := gnovmtools.TaskLine{Name: k.Name, Title: k.Title, AtWork: k.AtWork, State: state}
		if l.AtWork == "" {
			l.AtWork = l.Title
		}
		list = append(list, l)
	}
	return list, true
}

// lineState reads a line's state from the word the state gives itself.
func lineState(word string) (gnovmtools.LineState, bool) {
	for _, s := range []gnovmtools.LineState{gnovmtools.LineOpen, gnovmtools.LineWorking, gnovmtools.LineDone} {
		if s.String() == word {
			return s, true
		}
	}
	return 0, false
}

// taskStatus is a line's state in the words of the front end's task.
func taskStatus(s gnovmtools.LineState) string {
	switch s {
	case gnovmtools.LineWorking:
		return app.TaskInProgress
	case gnovmtools.LineDone:
		return app.TaskCompleted
	}
	return app.TaskPending
}

// onMemGoals is the tools' OnMemGoals slot: a kept call changed top-level
// todos, and mem hands them back as the calling session sees them. The
// last of them becomes the session's task list.
//
// The front end is handed the list once the call's result is in the log
// (afterStored), when the frames the call pushed and popped are in force:
// the line in progress reads the frame the session then stands in. A call
// whose result the loop stored before the call came back has no such
// moment to come, and its list is handed over here.
func (j *join) onMemGoals(_ context.Context, _ string, goals []gnovm.MemGoal) {
	if len(goals) == 0 || !j.main {
		return
	}
	list := gnovmtools.MemTodos(goals[len(goals)-1], j.id)
	j.setTasks(list)
	j.mu.Lock()
	waiting := len(j.inflight) > 0
	j.mu.Unlock()
	if !waiting {
		j.show()
	}
}

// setTasks keeps a task list for the session and writes it to its log, so
// that a session that is opened again, or copied, has it.
func (j *join) setTasks(list []gnovmtools.TaskLine) {
	j.mu.Lock()
	same := sameTasks(j.tasks.list, list)
	j.tasks.list = list
	j.mu.Unlock()
	if same {
		return
	}
	_, _ = j.ts.Append(transcript.MetaEntry(metaTasks, encodeTasks(list)))
}

func sameTasks(a, b []gnovmtools.TaskLine) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// showTasks hands the front end the session's task list, which it shows
// under the prompt: mem's todos for the last task a call of the session
// changed, each with where it stands. While the session stands in a
// frame, the line in progress that holds that frame reads it: in `id`
// "purpose". stack is the session's stack as it stands. The front end's
// task has names of its own for the three parts of a line, and they are
// filled here.
//
// A session may hold several todos at once (mem's holds: a todo entered
// on the way, or started), and each is a line in progress; only the one
// the session stands in reads the frame, and the others their own words
// (Jae, 16 November 2026: the frame stood on three lines at once, "why is
// there repetition").
func (j *join) showTasks(stack []transcript.Frame) {
	if !j.main || j.s.Tasks == nil {
		return
	}
	j.mu.Lock()
	list := slices.Clone(j.tasks.list)
	j.mu.Unlock()
	// The line that reads the frame: the one in progress that is the frame
	// the session stands in, or holds it; else the one line in progress,
	// when there is one (the session stands in the task, the line is its
	// next todo); else none.
	reads, working := -1, 0
	var top transcript.Frame
	if n := len(stack); n > 0 {
		top = stack[n-1]
	}
	for i, l := range list {
		if l.State != gnovmtools.LineWorking {
			continue
		}
		working++
		if reads < 0 && top.ID != "" && l.Name != "" && (l.Name == top.ID || strings.HasPrefix(top.ID, l.Name+"/")) {
			reads = i
		}
	}
	if reads < 0 && working == 1 {
		for i, l := range list {
			if l.State == gnovmtools.LineWorking {
				reads = i
			}
		}
	}
	tasks := make([]app.Task, 0, len(list))
	for i, l := range list {
		task := app.Task{Content: l.Title, ActiveForm: l.AtWork, Status: taskStatus(l.State)}
		if i == reads && top.ID != "" {
			task.ActiveForm = fmt.Sprintf("in `%s` %q", top.ID, top.Label)
		}
		tasks = append(tasks, task)
	}
	j.s.Tasks(tasks)
}

// onMemRules is the tools' OnMemRules slot: a kept call set, changed or
// removed rules. Every session is given the rules as the user's own
// words, so whatever a session does to one, the person is told: a rule
// that a file or a tool's output talked the agent into must not reach
// later sessions unseen.
func (j *join) onMemRules(_ context.Context, _ string, rules []gnovm.MemRule) {
	for _, r := range rules {
		j.tell(agent.LevelWarning, "mem", ruleWarning(r))
	}
}

// ruleWarning is what the person is told of one rule a session touched.
func ruleWarning(r gnovm.MemRule) string {
	switch {
	case r.Was == "":
		return fmt.Sprintf("mem: this session set a rule every session will follow as yours: `%s` = %q. If you did not ask for it, /mem forget %s removes it", r.Name, r.Text, r.Name)
	case r.Text == "":
		return fmt.Sprintf("mem: this session removed the rule `%s`, which every session followed as yours. If you did not ask for it, /mem log shows the change and /mem undo reverses it", r.Name)
	}
	return fmt.Sprintf("mem: this session changed the rule `%s`, which every session follows as yours, to %q (it was %q). If you did not ask for it, /mem log shows the change and /mem undo reverses it", r.Name, r.Text, r.Was)
}

// scrubTasks takes what was erased out of a task list. A line left with
// nothing but the word that stands for what was forgotten goes.
func scrubTasks(list []gnovmtools.TaskLine, e gnovm.MemErasure) ([]gnovmtools.TaskLine, bool) {
	changed := false
	out := list[:0:0]
	for _, l := range list {
		title, atWork := e.Scrub(l.Title), e.Scrub(l.AtWork)
		if title != l.Title || atWork != l.AtWork {
			changed = true
		}
		if strings.TrimSpace(title) == "(forgotten)" || strings.TrimSpace(title) == "" {
			changed = true
			continue
		}
		l.Title, l.AtWork = title, atWork
		out = append(out, l)
	}
	return out, changed
}

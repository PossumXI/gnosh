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
	list []gnovmtools.Todo
}

// load reads the task list from the session's log: the newest one, the
// one a copy of another session carried included.
func (t *taskState) load(entries []transcript.Entry) {
	for i := len(entries) - 1; i >= 0; i-- {
		if e := &entries[i]; e.Type == transcript.TypeMeta && e.Key == metaTasks {
			var list []gnovmtools.Todo
			if json.Unmarshal([]byte(e.Value), &list) == nil {
				t.list = list
			}
			return
		}
	}
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
func (j *join) setTasks(list []gnovmtools.Todo) {
	raw, err := json.Marshal(list)
	if err != nil {
		return
	}
	j.mu.Lock()
	same := sameTasks(j.tasks.list, list)
	j.tasks.list = list
	j.mu.Unlock()
	if same {
		return
	}
	_, _ = j.ts.Append(transcript.MetaEntry(metaTasks, string(raw)))
}

func sameTasks(a, b []gnovmtools.Todo) bool {
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
// frame, the line in progress reads the frame: in `id` "purpose". stack
// is the session's stack as it stands.
func (j *join) showTasks(stack []transcript.Frame) {
	if !j.main || j.s.Tasks == nil {
		return
	}
	j.mu.Lock()
	list := slices.Clone(j.tasks.list)
	j.mu.Unlock()
	tasks := make([]app.Task, 0, len(list))
	for _, t := range list {
		task := app.Task{Content: t.Content, ActiveForm: t.ActiveForm, Status: string(t.Status)}
		if n := len(stack); n > 0 && t.Status == gnovmtools.TodoStatusInProgress {
			task.ActiveForm = fmt.Sprintf("in `%s` %q", stack[n-1].ID, stack[n-1].Label)
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
func scrubTasks(list []gnovmtools.Todo, e gnovm.MemErasure) ([]gnovmtools.Todo, bool) {
	changed := false
	out := list[:0:0]
	for _, t := range list {
		content, active := e.Scrub(t.Content), e.Scrub(t.ActiveForm)
		if content != t.Content || active != t.ActiveForm {
			changed = true
		}
		if strings.TrimSpace(content) == "(forgotten)" || strings.TrimSpace(content) == "" {
			changed = true
			continue
		}
		t.Content, t.ActiveForm = content, active
		out = append(out, t)
	}
	return out, changed
}

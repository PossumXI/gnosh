package memjoin

import (
	"slices"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app"
	gnovmtools "github.com/gnosh-ai/gnosh/internal/gnovm/tools"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// A session that holds several todos at once shows each in its own words,
// and only the one it stands in reads the frame (Jae, 16 November 2026:
// one frame stood on three lines of the task list, "why is there
// repetition"). Standing in the task itself with one todo in progress,
// that todo reads the frame, as before; with several, none does.
func TestShowTasksReadsTheFrameOnce(t *testing.T) {
	var got []app.Task
	j := &join{main: true, s: &app.Session{Tasks: func(ts []app.Task) { got = ts }}}
	line := func(name, title string, state gnovmtools.LineState) gnovmtools.TaskLine {
		return gnovmtools.TaskLine{Name: name, Title: title, AtWork: title, State: state}
	}
	j.tasks.list = []gnovmtools.TaskLine{
		line("forum/research", "research models", gnovmtools.LineDone),
		line("forum/backend", "build the backend and API", gnovmtools.LineWorking),
		line("forum/frontend", "build the frontend", gnovmtools.LineWorking),
		line("forum/deploy", "deploy it", gnovmtools.LineWorking),
		line("forum/matchbox", "build the matchbox", gnovmtools.LineOpen),
	}
	shown := func(stack ...transcript.Frame) []string {
		t.Helper()
		j.showTasks(stack)
		var out []string
		for _, task := range got {
			out = append(out, task.ActiveForm)
		}
		return out
	}
	task := transcript.Frame{ID: "forum", Label: "the forum"}
	api := transcript.Frame{ID: "forum/backend/api", Label: "build the API"}
	if got, want := shown(task, api), []string{"research models", "in `forum/backend/api` \"build the API\"", "build the frontend", "deploy it", "build the matchbox"}; !slices.Equal(got, want) {
		t.Errorf("standing under one of three held todos:\n%q\nwant:\n%q", got, want)
	}
	if got, want := shown(task), []string{"research models", "build the backend and API", "build the frontend", "deploy it", "build the matchbox"}; !slices.Equal(got, want) {
		t.Errorf("standing in the task, three todos held:\n%q\nwant:\n%q", got, want)
	}
	j.tasks.list = j.tasks.list[:2]
	if got, want := shown(task), []string{"research models", "in `forum` \"the forum\""}; !slices.Equal(got, want) {
		t.Errorf("standing in the task, one todo in progress:\n%q\nwant:\n%q", got, want)
	}
}

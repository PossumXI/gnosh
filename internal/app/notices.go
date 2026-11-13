package app

import (
	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/tools/shelltool"
)

// backgroundEnded is told by the shell manager that a background command
// has ended, and passes the news to the model through the loop's one way
// in for such things: a note. A running turn gets it before its next
// request; an idle session starts a turn for it, with one quiet line for
// the person saying why. Each end is announced once: the shell manager
// sees to that. Nothing is announced once the session has begun to close.
//
// It is called on a goroutine of the shell manager's and must not block:
// Tell only puts the note in line.
func (l *live) backgroundEnded(e shelltool.Ended) {
	a := l.agent
	if a == nil || l.closing.Load() {
		return
	}
	l.noteBackground()
	end := prompt.BackgroundEnd{
		ID:      e.ID,
		Command: firstOf(e.Description, firstLine(e.Command)),
		Output:  e.OutputFile,
	}
	switch {
	case e.Reason == shelltool.ReasonExited && e.ExitCode == 0:
		end.State = prompt.BackgroundFinished
	case e.Reason == shelltool.ReasonExited:
		end.State, end.Code = prompt.BackgroundFailed, e.ExitCode
	default:
		// Its time ran out, it printed too much, or it was stopped.
		end.State = prompt.BackgroundStopped
	}
	_ = a.Tell(agent.Note{
		Text:   l.texts.BackgroundEnded(end),
		Source: "task-notification",
		Notice: "A background command ended: " + e.Summary,
		Wake:   true,
	})
}

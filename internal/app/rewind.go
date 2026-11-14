package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// This file is the rewind: taking a conversation back to before one of
// the person's prompts (spec/cc/sessions.md, section 5; spec/gnosh
// frames.md, 12.5).
//
// The points are the person's own prompts, as the session's file holds
// them after earlier rewinds; one inside a frame that was popped since is
// still a point. A rewind writes one entry into the session's file, which
// says the conversation goes on from the entry before the prompt, and
// then opens the session again from its file behind the same controls,
// exactly as leaving and resuming it would: so everything that is worked
// out from the conversation (what the model is sent, the frames the
// session stands in, what the features hold of it, whether the first
// prompt's facts have been given) is worked out afresh, by the code that
// a resume runs. Nothing is taken out of the file: a rewind that was a
// mistake is itself in the record.
//
// What a rewind does not do in this build: it restores no files. gnosh
// keeps no checkpoints of the files a session's tools change yet, so what
// was changed on disk stays as it is, and the rewind says so. Because the
// session is opened again, the commands it had running in the background
// are stopped, as they are when a session ends.

// RewindPoint is one of the person's prompts that the conversation can be
// taken back to.
type RewindPoint struct {
	// ID names the point for Rewind.
	ID string
	// Text is the prompt, and Time when it was sent.
	Text string
	Time time.Time
}

// rewindGroup finds the person's prompts in what is left of the
// conversation, and for each the entry the conversation would go on from
// if it were taken back to before that prompt: the one before the prompt
// and everything that went out with it.
func (l *live) rewindGroup() (points []RewindPoint, before map[string]string) {
	entries := transcript.Conversation(l.ts.Entries())
	before = map[string]string{}
	// first is where each prompt's own entries begin: the parts that rode
	// on it may stand before the prompt itself.
	first := map[string]int{}
	for i := range entries {
		for _, e := range decode(&entries[i]) {
			id := ""
			switch e.Kind {
			case agent.EntryPrompt, agent.EntryQueued:
				id = e.ID
			case agent.EntryNote:
				id = e.Prompt
			}
			if _, seen := first[id]; id != "" && !seen {
				first[id] = i
			}
		}
	}
	for i := range entries {
		for _, e := range decode(&entries[i]) {
			if e.Kind != agent.EntryPrompt || e.Hidden {
				continue
			}
			text := strings.TrimSpace(core.TextOf(e.Blocks))
			if text == "" {
				continue
			}
			points = append(points, RewindPoint{ID: entries[i].UUID, Text: text, Time: e.Time})
			if at, ok := first[e.ID]; ok && at > 0 {
				before[entries[i].UUID] = entries[at-1].UUID
			}
		}
	}
	return points, before
}

// rewindPoints is SessionControls.RewindPoints.
func (l *live) rewindPoints() []RewindPoint {
	points, _ := l.rewindGroup()
	return points
}

// RewindPoints lists the person's prompts of this conversation, the
// oldest first.
func (c *controls) RewindPoints() []RewindPoint { return c.cur().rewindPoints() }

// Rewind takes the conversation back to before the prompt that point
// names, and returns the prompt's text. The session must be idle.
func (c *controls) Rewind(ctx context.Context, point string) (string, error) {
	old := c.cur()
	if old.agent.State().Busy {
		return "", errors.New("a turn is running. Stop it first (Esc, or Ctrl+C in the plain session), then go back")
	}
	points, before := old.rewindGroup()
	text := ""
	for _, p := range points {
		if p.ID == point {
			text = p.Text
		}
	}
	to, ok := before[point]
	switch {
	case text == "":
		return "", errors.New("that prompt is not in this conversation any more")
	case !ok:
		return "", errors.New("that is the very start of this session's file, and there is nothing before it to go back to. /clear starts a fresh session")
	}
	if _, err := old.ts.Append(transcript.RewindEntry(to)); err != nil {
		return "", fmt.Errorf("the rewind could not be written into the session's file (%v), so nothing was changed", err)
	}
	id := old.ts.ID()
	old.flog.logf("turn", "the conversation was taken back to before the prompt in entry %s", point)
	old.close("other")
	next, err := c.p.openSession(ctx, c.st, c.front, "rewind", id)
	if err != nil {
		return "", fmt.Errorf("the conversation was taken back, but the session could not be opened again: %w", err)
	}
	c.mu.Lock()
	c.l = next
	c.mu.Unlock()
	return text, nil
}

// rewindNote is what a person is told wherever a rewind is offered.
const rewindNote = "Only the conversation goes back. Files are not restored: this gnosh keeps no checkpoints of them yet, so what was changed on disk stays as it is. Commands running in the background are stopped."

// slashRewind is /rewind: with no number it lists the points, and with
// one it asks for the conversation to be taken back to before that
// prompt, which the session's controls then do.
func (l *live) slashRewind(ctx context.Context, c *SlashContext) error {
	points := l.rewindPoints()
	if len(points) == 0 {
		c.Say("There is no prompt of yours in this conversation to go back to.")
		return nil
	}
	arg := strings.TrimSpace(c.Args)
	if arg == "" {
		c.Say("Your prompts in this conversation:")
		for i, p := range points {
			c.Say("  %d. %s  %s", i+1, p.Time.Local().Format("15:04"), oneLine(p.Text, 100))
		}
		c.Say("/rewind N takes the conversation back to before prompt N, which you can then send again or change. %s", rewindNote)
		return nil
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 || n > len(points) {
		return fmt.Errorf("%q is not the number of one of your prompts: /rewind lists them, 1 to %d", oneLine(arg, 40), len(points))
	}
	if l.agent.State().Busy {
		return errors.New("a turn is running. Stop it first, then go back")
	}
	l.rewindTo = points[n-1].ID
	return nil
}

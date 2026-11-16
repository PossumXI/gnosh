package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/text"
)

// The prompt mark and the pointer are the blue-green of gnosh.ai, not the
// hot pink, which reads as red on a dark screen.
func TestPromptMarkIsTheSitesBlueGreen(t *testing.T) {
	th := DefaultTheme(text.TrueColor)
	if got := th.Palette.Aqua.String(); got != "#68ffd6" {
		t.Fatalf("the blue-green is %s, and gnosh.ai's is #68ffd6", got)
	}
	const aqua = "38;2;104;255;214"
	for name, styled := range map[string]string{
		"the prompt mark in the scrollback": th.Prompt.Render(">"),
		"the prompt mark in the box":        th.Editor.PromptStyle.Render(">"),
		"the pointer of a list":             th.Menu.PointerStyle.Render(">"),
	} {
		if !strings.Contains(styled, aqua) {
			t.Errorf("%s is drawn as %q, not in the blue-green", name, styled)
		}
	}
}

// While gnosh works the mark before the activity line moves: a comet of
// sparkle marks that takes a step every tenth of a second or so. It stands
// still while a question waits, and with no colour it is the plain mark.
func TestSpinnerMoves(t *testing.T) {
	g := newRig(t, 80, 24, text.TrueColor)
	g.keys(`"go" <Enter>`).start()
	head := func() int {
		t.Helper()
		g.frame()
		for _, row := range g.live {
			if strings.Contains(row, "waiting for") || strings.Contains(row, "is writing") {
				return strings.Index(row, "✦")
			}
		}
		t.Fatalf("the live region has no activity line\n%s", g.dump())
		return 0
	}
	seen := map[int]bool{}
	for range 12 {
		seen[head()] = true
		g.wait(g.s.th.SpinEvery)
	}
	if len(seen) < 5 {
		t.Fatalf("in twelve steps the head of the spinner stood at %d places: %v\n%s", len(seen), seen, g.dump())
	}
	g.liveLacks("* waiting")

	// The session says when it wants to be woken: soon, while it works.
	if at := g.s.wake(); at.IsZero() || at.Sub(g.now) > g.s.th.SpinEvery {
		t.Errorf("while it works the session asks to be woken at %v, %v from now", at, at.Sub(g.now))
	}

	// A question that waits for the person does not move.
	g.ev(&agent.QuestionEvent{Question: permission("q1")})
	g.frame()
	g.liveHas("* waiting for your answer").liveLacks("✦")

	// With no colour the mark is the plain one.
	g = newRig(t, 80, 24, text.NoColor)
	g.keys(`"go" <Enter>`).start()
	g.liveHas("* waiting for").liveLacks("✦")
}

// Folded reasoning shows its last four rows while it arrives, and says
// every time which key writes it out.
func TestThinkingSaysHowToSeeIt(t *testing.T) {
	g := newRig(t, 80, 24, text.NoColor)
	g.keys(`"go" <Enter>`).start()
	g.ev(thought("one\ntwo\nthree\nfour\nfive\nsix\n"))
	g.liveHas("~ thinking · Ctrl+O shows all of it", "three", "four", "five", "six").liveLacks("two")
	g.wait(3*time.Second).ev(said("Done.\n"), replyEnd(core.StopEndTurn))
	g.ev(request(2), thought("again\n"))
	g.wait(2*time.Second).ev(said("Done again.\n"), replyEnd(core.StopEndTurn), turnEnd(agent.EndCompleted), idle())
	if n := strings.Count(flat(g.scroll()), "Ctrl+O shows it"); n != 2 {
		t.Fatalf("two folded thoughts name the key %d times, and should name it each time\n%s", n, g.dump())
	}
	g.once("~ thought for 3s · Ctrl+O shows it", "~ thought for 2s · Ctrl+O shows it")
}

// With the verbose option every thought is written out as it comes, and
// nothing is folded.
func TestVerboseWritesThoughtsOut(t *testing.T) {
	g := newRig(t, 80, 40, text.NoColor)
	g.s.detail = true
	g.keys(`"go" <Enter>`).start()
	g.ev(thought("the first thought\n"))
	g.wait(2*time.Second).ev(said("Looking.\n"), replyEnd(core.StopEndTurn))
	g.ev(request(2), thought("the second thought\n"))
	g.wait(3*time.Second).ev(said("Done.\n"), replyEnd(core.StopEndTurn), turnEnd(agent.EndCompleted), idle())
	g.once("the first thought", "the second thought")
	g.committedLacks("Ctrl+O shows it")
}

// The one word exit, or quit, leaves, as /exit does. A sentence that
// begins with it is a prompt.
func TestTheWordExitLeaves(t *testing.T) {
	for _, word := range []string{"exit", "quit", " Exit "} {
		g := newRig(t, 80, 24, text.NoColor)
		g.keys(`"` + word + `" <Enter>`)
		if !g.s.done || g.s.why != "prompt_input_exit" {
			t.Errorf("%q: done %v, reason %q", word, g.s.done, g.s.why)
		}
	}
	g := newRig(t, 80, 24, text.NoColor)
	g.keys(`"exit the loop early" <Enter>`)
	if g.s.done {
		t.Fatal("a prompt that begins with exit left gnosh")
	}
}

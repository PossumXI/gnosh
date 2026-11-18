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

// strip is the working mark as the live region shows it now: the cells
// before the words of the line that says what gnosh is doing.
func (g *rig) strip() []rune {
	g.t.Helper()
	cells := g.s.th.Fall.Cells
	for _, row := range g.live {
		for _, words := range []string{"waiting for", "is thinking", "is deep in thought", "is writing"} {
			if !strings.Contains(row, words) {
				continue
			}
			r := []rune(row)
			if len(r) <= cells || r[cells] != ' ' {
				g.t.Fatalf("the working mark is not %d cells and a blank before the words: %q", cells, row)
			}
			return r[:cells]
		}
	}
	g.t.Fatalf("the live region has no activity line\n%s", g.dump())
	return nil
}

// While gnosh works the mark before the activity line moves: sparkle
// marks that fall to the right across its cells. It stands still while a
// question waits, and with no colour it is the plain mark.
func TestWorkingMarkFalls(t *testing.T) {
	g := newRig(t, 80, 24, text.TrueColor)
	g.keys(`"go" <Enter>`).start()
	every := g.s.th.Fall.Every
	// The head that is farthest along, frame by frame, for three seconds.
	seen, back, last := map[int]bool{}, 0, -1
	for range int(3 * time.Second / every) {
		head := -1
		for i, r := range g.strip() {
			if r == '✦' {
				head = i
			}
		}
		g.wait(every)
		if head < 0 {
			// A drop whose head has left: its tail is still leaving.
			continue
		}
		seen[head] = true
		if head < last {
			back++
		}
		last = head
	}
	if len(seen) < 6 {
		t.Fatalf("in three seconds the head of a drop stood in %d places: %v\n%s", len(seen), seen, g.dump())
	}
	// A head is farther left than the frame before only when the drop
	// before it has left: in three seconds of a calm fall, a few times.
	if back > 4 {
		t.Errorf("the drops went back %d times in three seconds", back)
	}
	g.liveLacks("* waiting")

	// The session says when it wants to be woken: soon, while it works.
	if at := g.s.wake(); at.IsZero() || at.Sub(g.now) > every {
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

// A thought that goes on deepens, and the working mark shows it: more
// drops fall, the line says so in words, and the session asks to be woken
// sooner. When the thought is over the fall is calm again. (Jae, 18
// November 2026: "when 'deep in thought', more things drop faster with
// higher acceleration".)
func TestADeepThoughtShowsInTheMark(t *testing.T) {
	g := newRig(t, 80, 24, text.TrueColor)
	g.keys(`"go" <Enter>`).start()
	g.c.state = agent.State{Busy: true, Phase: agent.PhaseReply}
	g.ev(thought("Let me think.\n"))
	th := g.s.th.Fall
	// How many marks the strip holds over two seconds of frames.
	marks := func() (n int) {
		for range 40 {
			for _, r := range g.strip() {
				if r != ' ' {
					n++
				}
			}
			g.wait(50 * time.Millisecond)
		}
		return n
	}
	early := marks()
	g.liveHas("qwen3.8-27b is thinking").liveLacks("deep in thought")
	if at := g.s.wake(); at.Sub(g.now) != th.Every {
		t.Errorf("early in a thought the session asks to be woken %v from now, want %v", at.Sub(g.now), th.Every)
	}

	g.wait(40 * time.Second)
	g.liveHas("qwen3.8-27b is deep in thought · 42s")
	late := marks()
	if late < 2*early {
		t.Errorf("a deep thought shows %d marks in two seconds, and a young one %d", late, early)
	}
	if at := g.s.wake(); at.Sub(g.now) != th.EveryDeep {
		t.Errorf("deep in a thought the session asks to be woken %v from now, want %v", at.Sub(g.now), th.EveryDeep)
	}

	// The answer begins: the fall settles, and is calm again.
	g.ev(said("Here it is.\n"))
	g.liveHas("qwen3.8-27b is writing")
	g.wait(time.Second).wait(time.Second)
	if after := marks(); after*3 > late*2 {
		t.Errorf("after the thought the strip still shows %d marks in two seconds, against %d while it was deep", after, late)
	}
	if at := g.s.wake(); at.Sub(g.now) != th.Every {
		t.Errorf("after the thought the session asks to be woken %v from now, want %v", at.Sub(g.now), th.Every)
	}

	// Where the model's name leaves no room for them, the short words.
	g = newRig(t, 60, 24, text.NoColor)
	g.keys(`"go" <Enter>`)
	g.c.state = agent.State{Busy: true, Phase: agent.PhaseReply}
	g.ev(busy(), turnStart(), promptOut("p1", "go"), &agent.RequestEvent{
		Purpose: "main", Round: 1, Attempt: 1, Provider: "local", Model: "a-model-with-a-long-name-of-its-own-v2.5-27b-q8", Messages: 3,
	})
	g.ev(thought("Let me think.\n"))
	g.wait(41 * time.Second)
	g.liveHas("* deep in thought · 41s").liveLacks("is deep in thought")
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

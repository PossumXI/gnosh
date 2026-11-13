package memjoin

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/prompt"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// subagentFiles lists the session files of the world's sub-agents: each
// has one of its own, in the folder of the session that started it.
func subagentFiles(w *world) []string {
	found, _ := filepath.Glob(filepath.Join(w.DataDir(), "projects", "*", "*", "subagents", "projects", "*", "*.jsonl"))
	return found
}

// Sub-agents and mem, end to end, through the Agent tool. A stock
// sub-agent is given no gnovm tool and nothing of mem's context, though
// the session that starts it has a plan in mem and stands in one of its
// todos. A kind whose definition names gnovm_run is given that tool: it is
// a session of its own to mem, every request of its carries the sub-agent
// flag, it takes a free todo of the plan and pops it in its own log, and
// its mem.Next() answers at once where another session's would wait.
func TestSubAgentsAndMem(t *testing.T) {
	const (
		group  = "build-shapes"
		circle = "build-shapes/circle"
		square = "build-shapes/square"
	)
	w := newWorld(t,
		// The session the person is in: a plan of two todos that do not
		// depend on each other, and it steps into one.
		reply(memCall("m1", `mem.Todo("build the shapes", "circle", "square").Parallel()`)),
		reply(memCall("m2", `mem.Next()`)),
		// A stock sub-agent.
		reply(call("m3", "Agent", map[string]any{"description": "look around", "prompt": "Say which tools you have."})),
		says("I have the file tools, the shell and no others."),
		// A sub-agent of a kind that is given gnovm_run.
		reply(call("m4", "Agent", map[string]any{"description": "take a todo", "prompt": "Take the next todo of the plan and do it.", "subagent_type": "shaper"})),
		reply(memCall("s1", `mem.Next()`)),
		reply(call("s2", "Read", map[string]any{"file_path": "square.txt"})),
		reply(memCall("s3", "mem.Pop(\"square.txt holds a square of four lines\")\nmem.Next()")),
		says("The square is done. The circle is another session's."),
		says("Both helpers are back."),
	)
	w.Write("square.txt", "+--+\n|  |\n|  |\n+--+\n")
	w.Write(filepath.Join(w.Home, ".claude", "agents", "shaper.md"),
		"---\nname: shaper\ndescription: Takes a todo of the plan in mem and does it.\ntools: Read, gnovm_run\n---\n\nYou are the shaper. The plan is in mem: call gnovm_run with the name mem.\n")

	r := w.run("-y", "-p", "build the shapes")
	r.wantCode(t, 0)
	w.script.done()
	reqs := w.script.requests()
	if len(reqs) != 10 {
		t.Fatalf("the run made %d requests, want 10", len(reqs))
	}
	mainID := strings.TrimSuffix(filepath.Base(w.Sessions()[0]), ".jsonl")
	memLine := prompt.Builtin().MemLine()
	memory := prompt.Builtin().System(prompt.SystemInput{Tools: []core.ToolSpec{{Name: "gnovm_run"}}})[1]

	// The session itself has the four tools, the part about mem and the
	// mem line; it stands in the circle when it starts the sub-agents.
	has(t, "the session's tools", strings.Join(toolNamesOf(reqs[0]), " "), "Agent gnovm_run gnovm_new gnovm_list gnovm_drop")
	has(t, "the session's system prompt", system(t, reqs[0]), memory)
	has(t, "the session's first prompt", lastUser(t, reqs[0]), memLine)
	has(t, "the session's step into the plan", lastResult(t, reqs[2]), "in `"+circle+"` \"circle\"")

	// The stock sub-agent: no gnovm tool, and nothing of mem's.
	stock := reqs[3]
	for _, name := range toolNamesOf(stock) {
		if strings.HasPrefix(name, "gnovm_") {
			t.Errorf("the stock sub-agent was given %s", name)
		}
	}
	has(t, "the stock sub-agent's tools", strings.Join(toolNamesOf(stock), " "), "Read", "Bash")
	hasNot(t, "the stock sub-agent's request", string(stock.Body), "<mem>", memLine, "You are in `", group, "mem.Next()", "From your memory (mem)")
	hasNot(t, "the stock sub-agent's system prompt", system(t, stock), memory)

	// The defined kind: the one gnovm tool its definition names, its own
	// words for a system prompt, and still nothing of mem's context: no
	// digest, no mem line, no frame line.
	shaper := reqs[5]
	if got := strings.Join(toolNamesOf(shaper), " "); got != "Read gnovm_run" {
		t.Errorf("the tools of the kind that names gnovm_run: %q", got)
	}
	has(t, "the defined sub-agent's system prompt", system(t, shaper), "You are the shaper.")
	hasNot(t, "the defined sub-agent's first request", string(shaper.Body), "<mem>", memLine, "You are in `", "From your memory (mem)")

	// Its calls on mem: a session of its own, marked as a sub-agent's,
	// with its own stack and not the stack of the session that started it.
	// (The call that pops is sent twice: mem hands the close back to be
	// checked before it keeps anything, and then the call is run again.)
	sent := w.memRequests()
	if len(sent) != 5 {
		t.Fatalf("%d requests reached mem, want 5: two of the session's and three of the sub-agent's", len(sent))
	}
	for i, req := range sent[:2] {
		if req.SubAgent || req.Session != mainID {
			t.Errorf("the session's own mem call %d: SubAgent %v, session %q", i, req.SubAgent, req.Session)
		}
	}
	subID := sent[2].Session
	for i, req := range sent[2:] {
		if !req.SubAgent || req.Session != subID || req.Session == mainID || req.Session == "" {
			t.Errorf("the sub-agent's mem call %d: SubAgent %v, session %q (the session that started it is %q)", i, req.SubAgent, req.Session, mainID)
		}
	}
	if len(sent[2].Frames) != 0 || strings.Join(sent[3].Frames, " ") != square || strings.Join(sent[4].Frames, " ") != square {
		t.Errorf("the stacks the sub-agent's calls carried: %q, %q and %q, want none and then %s twice", sent[2].Frames, sent[3].Frames, sent[4].Frames, square)
	}
	var subFile string
	for _, path := range subagentFiles(w) {
		if strings.TrimSuffix(filepath.Base(path), ".jsonl") == subID {
			subFile = path
		}
	}
	if subFile == "" {
		t.Fatalf("no sub-agent's session file is named %s: %v", subID, subagentFiles(w))
	}

	// It took the todo that was free, though the session that started it
	// holds the other one and is at work.
	has(t, "the sub-agent's step into the plan", lastResult(t, reqs[6]), "in `"+square+"` \"square\"")

	// Its pop is its own: the receipt is in its result, the work it did
	// is gone from what it is sent, and its own log holds the push and
	// the pop. And its mem.Next() after the pop answers at once: nothing
	// is free, the circle's session is at work, and a sub-agent never
	// waits for it, since that session is waiting for the sub-agent.
	popped := lastResult(t, reqs[8])
	has(t, "the sub-agent's pop", popped,
		"done `"+square+"` \"square\" → square.txt holds a square of four lines",
		"no next todo: `"+group+"` \"build the shapes\": nothing in it can be done now: `"+circle+"` is held by session ",
		"Everything since you entered `"+square+"` was removed from your context: this result stays; the user still sees all of it. Your reads inside it are forgotten: read a file again before you edit it.",
		"ok · mem · ")
	hasNot(t, "the request after the sub-agent's pop", string(reqs[8].Body), "+--+")
	sub := w.session(subFile)
	if got := stackIDs(sub.Stack()); got != "" {
		t.Errorf("the sub-agent's stack after its pop: %q", got)
	}
	_, info := sub.View(transcript.ViewOptions{Trace: true})
	if len(info.Cuts) != 1 || info.Cuts[0].Frame != square {
		t.Errorf("the cuts of the sub-agent's own log: %+v", info.Cuts)
	}

	// The session that started them stands where it stood: the frames of
	// a sub-agent are no frames of its own.
	if got := stackIDs(w.session(w.Sessions()[0]).Stack()); got != circle {
		t.Errorf("the session's stack after its sub-agents: %q, want %s", got, circle)
	}
	has(t, "the result of the Agent call", lastResult(t, reqs[9]), "The square is done. The circle is another session's.")
	// The person was told of the sub-agent's pop, too.
	has(t, "stderr", r.err, "popped `"+square+"`: about ")
}

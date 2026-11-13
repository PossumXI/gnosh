package prompt

import (
	"strings"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// The tools whose presence decides a part of the system prompt.
const (
	toolGnovmRun = "gnovm_run"
	toolDeliver  = "Deliver"
)

// MemoryForm chooses between the two texts gnosh has about mem. Which of
// them serves a model better is to be settled by measurement on the
// person's own machine, the same tasks run with each; until it is, the
// lean one is sent.
type MemoryForm int

const (
	// MemoryLean is the text "memory", and the default. It is under 1,500
	// tokens, and leaves to mem's own receipts and refusals, and to the
	// frame line, what they say at the moment it matters.
	MemoryLean MemoryForm = iota
	// MemoryFull is the text "memory-full": the lean text with every item
	// the mem spec lists for the system prompt said in it, about 250
	// tokens more.
	MemoryFull
)

// SystemInput is what the system prompt of a session depends on. Nothing
// in it is the date, the working directory, the model or the session:
// those go with the first prompt (Environment), so that the system prompt
// is the same bytes from one session to the next.
type SystemInput struct {
	// Tools are the tools the session's requests carry. The part about
	// mem is sent when gnovm_run is among them, the part about
	// deliverables when Deliver is, and with no tools at all the model is
	// told that it has none.
	Tools []core.ToolSpec

	// Memory chooses which text about mem is sent when the session has
	// gnovm_run. The zero value is MemoryLean.
	Memory MemoryForm

	// GnoProject says the working directory holds Gno code: a gnomod.toml,
	// a gno.mod, a gnowork.toml or a .gno file.
	GnoProject bool

	// Instructions is the project's and the person's own instruction
	// files, already put together by the package that reads them. It is
	// placed as it is, but for control sequences and gnosh's own tag,
	// which are made harmless: these are files, and a stranger's
	// repository can bring one.
	Instructions string

	// Replace, when not empty, stands in place of every part gnosh writes
	// itself. Instructions and Append are still sent. It is what
	// --system-prompt gives.
	Replace string

	// Append, when not empty, is sent last. It is what
	// --append-system-prompt gives.
	Append string
}

// System returns the system prompt as parts, the part that changes least
// first, as core.Request.System wants them. The order never changes; a
// part that does not apply is left out:
//
//  1. the fixed core, with the security rule as its last lines: the same
//     bytes whatever is in SystemInput;
//  2. the note that there are no tools, when there are none;
//  3. the part about mem, when the session has gnovm_run: the lean text,
//     or with Memory set to MemoryFull the full one;
//  4. the Gno rule, in a project with Gno code;
//  5. the project's and the person's instructions;
//  6. the part about deliverables, when the session has Deliver. It stands
//     after the instructions because only some sessions of a project have
//     the tool: placed here, a session with it and one without share
//     everything before it in a model server's cache;
//  7. Append.
//
// With Replace set, parts 1, 3, 4 and 6 give way to it.
func (s *Set) System(in SystemInput) []string {
	var parts []string
	add := func(part string) {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	own := strings.TrimSpace(in.Replace) == ""
	if own {
		add(s.Core())
	} else {
		add(in.Replace)
	}
	if len(in.Tools) == 0 {
		add(s.fill(textNoTools, nil))
	}
	if own && hasTool(in.Tools, toolGnovmRun) {
		if in.Memory == MemoryFull {
			add(s.fill(textMemoryFull, nil))
		} else {
			add(s.fill(textMemory, nil))
		}
	}
	if own && in.GnoProject {
		add(s.fill(textGno, nil))
	}
	add(Untrusted(strings.TrimSpace(in.Instructions)))
	if own && hasTool(in.Tools, toolDeliver) {
		add(s.fill(textDeliver, nil))
	}
	add(in.Append)
	return parts
}

// Core returns the fixed core: the first part of every system prompt gnosh
// writes. It depends on nothing, so it is the same bytes in every session,
// project and machine, and on every day.
func (s *Set) Core() string {
	return s.fill(textCore, nil) + "\n" + s.fill(textSecurity, nil)
}

// hasTool reports whether a tool called name is among tools.
func hasTool(tools []core.ToolSpec, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

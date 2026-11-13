// Copyright (C) 2026 Jae Kwon
//
// This file is part of gnosh's GnoVM machine subsystem. It is licensed under
// the GNO Network General Public License, version 6, as published by
// NewTendermint, LLC or its successor, or any later version of that License
// that the Licensor accepts as the proxy under its section 14, by a statement
// published in the canonical copy of internal/gnovm/LICENSE.md; together with
// the Additional Terms (Strong Attribution) in that file.

package gnovmtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/gnovm"
)

// Client is the GnoVM as the tools use it: the calls of gnovm.Client that
// they make. The process's client is one, whether it runs the VMs in a
// worker or, as tests do, in this process. A test of what the tools do when
// a worker dies puts its own in its place.
type Client interface {
	// Run runs code in a VM, or in mem. The error is for a call that did
	// not run at all; a failure of the code is in the result's Err.
	Run(ctx context.Context, req gnovm.RunRequest) (*gnovm.RunResult, error)
	// New makes a VM, or changes one's settings, and reports whether it
	// made it.
	New(ctx context.Context, req gnovm.NewRequest) (gnovm.VMInfo, bool, error)
	// List describes the host's memory and a project's VMs.
	List(ctx context.Context, project string) (gnovm.ListResult, error)
	// Drop evicts a VM from memory (action "evict") or deletes it
	// ("delete").
	Drop(ctx context.Context, project, name, action string) error
	// Kept reports whether a VM kept the call with request id req.
	Kept(ctx context.Context, project, name, req string) (bool, error)
	// KeptFrames reports whether mem kept the call with request id req,
	// and the frames that call pushed and popped.
	KeptFrames(ctx context.Context, project, req string) ([]gnovm.MemFrame, bool, error)
	// Deaths says why the client's workers ended, newest last.
	Deaths() []string
}

var _ Client = (*gnovm.Client)(nil)

// Lazy makes Options.Client of the program's hold on its client. The client
// is acquired when a tool is first called, so a gnosh that never runs a VM
// never starts a worker; a failure is not remembered, and the next call
// tries again. Tests give it a Lazy whose ClientOptions set InProcess.
func Lazy(l *gnovm.Lazy) func() (Client, error) {
	return func() (Client, error) {
		c, err := l.Get()
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

// Options is everything the tools are given from outside. Client is
// required. Every other field may be left out: the tools then do what a
// first working run of mem needs and no more (no stack is told to mem, no
// names are made, nothing is refused as fresh, no close is checked) and
// tell nobody what a call kept.
//
// Each function is called on the goroutine of the tool call it serves. One
// session's calls come one at a time, since no call runs beside a gnovm_run
// call; calls of different sessions may come at once. The functions that
// are called before a call runs are given the call's own context, which
// ends when the person stops the turn, and must return promptly then. The
// functions that are told what a call kept are given a context that
// cancellation does not reach, with the call's values still in it: what mem
// kept stands whatever happens to the turn, and a stop after a kept push or
// pop must not lose it.
type Options struct {
	// Client returns the way to the GnoVM. It is called at the start of
	// every call's work, after the arguments were checked. When it fails
	// the model reads "the Gno VMs are not available: " and its error, and
	// the next call asks again. Lazy makes one of a *gnovm.Lazy.
	Client func() (Client, error)

	// Project is the session's project directory, an absolute path: VMs
	// belong to a project, and so does mem (which the host keys by the
	// directory's repository). It must stay the same for the length of a
	// session, wherever a shell command takes the working directory. When
	// it is empty the tools take the first of the call's ToolEnv.Roots,
	// and failing that its Cwd.
	Project string

	// Session returns the id of the session a call belongs to: what mem
	// knows the caller by, what its scratch VM is named after, and what
	// every function below is given. Nil means the call's
	// ToolEnv.SessionID. A sub-agent that is given these tools is a session
	// of its own to mem (it may take a free todo under one its parent
	// holds), so where the harness gives a sub-agent's calls the session id
	// of the person's session, this is where the sub-agent's own id comes
	// from. It is called once for each gnovm_run, gnovm_new and gnovm_drop
	// call, once its arguments were checked and before it does anything,
	// and must not return "". gnovm_list needs no session.
	Session func(ctx context.Context, env *core.ToolEnv) string

	// SubAgent reports whether sessionID is a sub-agent's session
	// (RunRequest.SubAgent): its mem.Next() then never waits for working
	// sessions, since it would wait on its parent while its parent waits
	// on it. It is called once before every gnovm_run call. Nil means
	// false.
	SubAgent func(ctx context.Context, sessionID string) bool

	// MemStack returns the session's stack of frames in mem, bottom to
	// top: the ids of the frames the session has pushed and not popped, as
	// the harness derives them from the frames kept with the session's own
	// gnovm_run results (RunRequest.Frames). It is called once before every
	// call whose name is "mem", and its answer stands for that call, the
	// runs the close check repeats included. Nil means no stack: mem then
	// takes every call to be made from the root.
	MemStack func(ctx context.Context, sessionID string) []string

	// BoundGoal returns the name of the todo the session's /goal binds, or
	// "" (RunRequest.Goal): the session's own calls may not drop or forget
	// it. It is called once before every call whose name is "mem", before
	// MemStack. Nil means "".
	BoundGoal func(ctx context.Context, sessionID string) string

	// MemFresh returns the id of the frame on top of the session's stack
	// when that frame is fresh: no work came between the round that entered
	// it and this call's round. Otherwise it returns "". mem refuses to pop
	// a fresh frame (RunRequest.Fresh). It is called after MemStack, and
	// only for a call whose name is "mem", whose stack as MemStack gave it
	// is not empty, and whose code contains "Pop(" or "Done(". callID is
	// the call's id. Nil means nothing is refused as fresh.
	MemFresh func(ctx context.Context, sessionID, callID string) string

	// Namer returns names for the texts a call keeps in mem without naming
	// them, by text (RunRequest.Names): each a short lowercase dashed name.
	// A text it leaves out, or names with something mem will not take, is
	// named by mem from its own words. texts are the unnamed text literals
	// the tool found in the call's code, in order, each once; Namer is
	// called after MemFresh, only for a call whose name is "mem" and whose
	// code holds at least one. The same text must get the same name for as
	// long as the session lasts, so that a refused call sent again makes
	// the same names; the names it returns are also the ones the close
	// check's repeated runs carry. Nil means no names.
	Namer NamerFunc

	// CheckCloses, when not nil, turns the close check on for calls whose
	// name is "mem" (RunRequest.CheckCloses): mem then keeps nothing of an
	// input that closes, as done, a todo the work is done in, until that
	// close is approved here. It is given the closes of one run that are
	// not approved yet, with the call's id, and returns "" to approve them
	// all, or the refusal. Approved, the same request runs again with them
	// named in RunRequest.Checked (each by its id and result, and by its id
	// with any result), which may bring further closes and so another call
	// here. Refused, the refusal is the call's error, the model reads it
	// after "ERROR — nothing from this call was kept: ", and nothing the
	// run printed is shown. A turn that was stopped while it checked is not
	// an approval. Nil means no close is checked.
	CheckCloses CheckCloseFunc

	// ClosesKept is told, after a call in which CheckCloses was asked,
	// whether mem kept the call: false at a refusal, and otherwise once,
	// when the last run has answered, true only if that run failed in no
	// way. It is not called for a call that asked CheckCloses nothing. A
	// call whose worker died before it replied is reported as not kept,
	// whatever mem then says of it.
	ClosesKept func(ctx context.Context, sessionID, callID string, kept bool)

	// OnMemFrames is told the frames a kept call to mem pushed and popped,
	// in order, with the call's id, and returns what the call's result
	// tells the model besides mem's own output: for a pop, what was removed
	// from its context and what that changed; "" for nothing to add. The
	// text goes after the call's value and output and before its status
	// line. It is called first of the four that are told what a call kept,
	// and only when the call pushed or popped. It is also called for a call
	// whose worker died before replying and which mem turns out to have
	// kept, with the frames mem recorded for it; its text then goes on a
	// line of its own after the sentence that says the call was kept.
	// Whether or not it is set, the same frames are kept in the result's
	// Meta under MetaFrames.
	OnMemFrames MemFramesFunc

	// OnMemGoals is told the top-level todos a kept call to mem changed,
	// as the call's session sees them, for the session's task list: the
	// last of them is the one to show (MemTodos maps it). It is called
	// after OnMemFrames, only when the call changed one.
	OnMemGoals MemGoalsFunc

	// OnMemErased is told what a kept call to mem erased, so that gnosh
	// takes it out of its own copies of mem's text too. It holds the very
	// text erased: it must never be logged or stored. It is called after
	// OnMemGoals, only when the call erased something.
	OnMemErased func(ctx context.Context, sessionID string, e gnovm.MemErasure)

	// OnMemRules is told the rules a kept call to mem set, changed or
	// removed, for the user to be told: a rule is given to every session
	// as the user's own, and only the user's words become rules. It is
	// called last, only when the call changed a rule.
	OnMemRules func(ctx context.Context, sessionID string, rules []gnovm.MemRule)
}

// Set is the four tools, as New makes them.
type Set struct {
	o     Options
	tools []core.Tool
}

// New makes the four tools. It starts nothing and reaches nothing: the
// client is asked for when a tool is first called.
func New(o Options) *Set {
	s := &Set{o: o}
	s.tools = []core.Tool{runTool{&s.o}, newTool{&s.o}, listTool{&s.o}, dropTool{&s.o}}
	return s
}

// Tools returns gnovm_run, gnovm_new, gnovm_list and gnovm_drop, in that
// order.
func (s *Set) Tools() []core.Tool { return slices.Clone(s.tools) }

// Tool returns the one of the four whose name is name, or nil: an agent
// that may only look is given Tool(ListToolName) alone.
func (s *Set) Tool(name string) core.Tool {
	for _, t := range s.tools {
		if t.Spec().Name == name {
			return t
		}
	}
	return nil
}

// place is where a call runs: the project whose VMs it uses, and the
// session that makes it (gnovm_list, which needs none, gets ""). An error is
// the harness's own: it gave the tool too little to run with.
func (o *Options) place(ctx context.Context, env *core.ToolEnv, tool string) (project, session string, err error) {
	if o.Client == nil {
		return "", "", fmt.Errorf("gnovmtools: %s was made with no Options.Client, so it has no way to the GnoVM", tool)
	}
	if env == nil {
		return "", "", fmt.Errorf("gnovmtools: %s was given no ToolEnv, which names the session and the project", tool)
	}
	project = o.Project
	if project == "" && len(env.Roots) > 0 {
		project = env.Roots[0]
	}
	if project == "" {
		project = env.Cwd
	}
	if project == "" {
		return "", "", fmt.Errorf("gnovmtools: %s was given no project directory: Options.Project is empty, and so are the ToolEnv's Roots and Cwd", tool)
	}
	if tool == ListToolName {
		return project, "", nil
	}
	session = env.SessionID
	if o.Session != nil {
		session = o.Session(ctx, env)
	}
	if session == "" {
		return "", "", fmt.Errorf("gnovmtools: %s was given no session id, which mem and the scratch VM are named by", tool)
	}
	return project, session, nil
}

// client asks for the way to the GnoVM. Its error is what the model reads
// after "the Gno VMs are not available: ".
func (o *Options) client() (Client, error) {
	c, err := o.Client()
	if err == nil && c == nil {
		err = errors.New("gnosh's wiring returned no client and no reason")
	}
	return c, err
}

// framesFromJSON reads frames back from what a stored result's Meta holds
// once it has been through JSON.
func framesFromJSON(v any) []gnovm.MemFrame {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var frames []gnovm.MemFrame
	if json.Unmarshal(b, &frames) != nil {
		return nil
	}
	return frames
}

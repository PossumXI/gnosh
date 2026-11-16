package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/cli"
)

// This file is /restart (also /update), as Claude Code has it: the
// session ends as /exit ends it, and gnosh starts again in it, from the
// program file there is now, which is a newer gnosh when one has been
// installed since this one started. The new start has the same flags,
// with --resume=<this session> in place of what chose the conversation
// (cli.Table.RelaunchArgs), so the conversation, mem's place in it, the
// permission mode and the work tree go on as they were. A turn that runs,
// and work in the background, would be stopped with the session, so
// /restart waits for them; and a newer gnosh is said once, at the end of
// a turn, so that the person knows /restart has something to bring.

// restartArgs is the command line /restart asked to start again with, or
// nil.
func (p *program) restartArgs() []string {
	p.restartMu.Lock()
	defer p.restartMu.Unlock()
	return p.restart
}

// relaunch starts gnosh again with args, in place of this process. It
// returns only when that could not be done, with the exit code.
func (p *program) relaunch(args []string) int {
	err := p.o.Relaunch(args)
	words := make([]string, len(args))
	for i, a := range args {
		words[i] = shellWord(a)
	}
	p.errf("gnosh could not start again (%v). The session is kept: gnosh %s opens it again.", err, strings.Join(words, " "))
	return cli.ExitFailure
}

// shellWord writes s so that a shell reads it as one word.
func shellWord(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./=:,+@%") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// newBuild reports whether the program file gnosh started from has been
// replaced since: by another file put in its place, as an install by
// rename does, or by other contents written over it.
func (p *program) newBuild() bool {
	if p.started == nil {
		return false
	}
	now, err := os.Stat(p.o.Executable)
	if err != nil {
		// (Between the two steps of an install, say: not yet.)
		return false
	}
	return !os.SameFile(p.started, now) || !now.ModTime().Equal(p.started.ModTime()) || now.Size() != p.started.Size()
}

// tellNewBuild says once, as a turn of an interactive session ends, that a
// newer gnosh is installed than the one running.
func (l *live) tellNewBuild() {
	if !l.interactive || l.p.o.Relaunch == nil || l.newBuildSaid.Load() || !l.p.newBuild() {
		return
	}
	if l.newBuildSaid.CompareAndSwap(false, true) {
		l.notice(agent.LevelInfo, "restart", "A newer gnosh has been installed since this one started: /restart runs it, in this session.")
	}
}

// runningWork lists what runs in the background: commands, Monitors and
// sub-agents, each as its id and what it is.
func (l *live) runningWork() []string {
	var running []string
	if l.shell != nil {
		for _, t := range l.shell.Tasks() {
			if t.Running {
				running = append(running, t.ID+" ("+oneLine(firstOf(t.Description, firstLine(t.Command)), 80)+")")
			}
		}
	}
	for _, b := range l.backgroundAgents() {
		running = append(running, b.id+" ("+oneLine(b.desc, 80)+")")
	}
	return running
}

// slashRestart is /restart: it works out the command line to start again
// with, and ends the session; the program starts again once the session
// is closed (Main).
func (l *live) slashRestart(ctx context.Context, c *SlashContext) error {
	p := l.p
	switch {
	case p.o.Relaunch == nil:
		return errors.New("this gnosh cannot start itself again. /exit ends the session, and " + l.info().Resume + " opens it again")
	case c.Agent.State().Busy:
		return errors.New("a turn is running, and would be stopped: /restart once it has ended, or Esc stops it")
	}
	if work := l.runningWork(); len(work) > 0 {
		return fmt.Errorf("this is still running in the background, and would be stopped: %s. /restart once it has ended, or /tasks stop <id> stops it", strings.Join(work, ", "))
	}
	r := cli.Relaunch{Session: l.ts.ID(), Mode: string(l.engine.Mode()), Effort: l.agent.Model().Level}
	if ref, ok := l.modelRef(); ok {
		r.Model = ref.String()
	}
	if t := l.st.tree; t != nil {
		r.Tree = t.name
	}
	args, err := p.table.RelaunchArgs(p.o.Args, r)
	if err != nil {
		return fmt.Errorf("the command line to start again with could not be made (%v). /exit ends the session, and %s opens it again", err, l.info().Resume)
	}
	p.restartMu.Lock()
	p.restart = args
	p.restartMu.Unlock()
	if p.newBuild() {
		c.Say("gnosh starts again, the newer one, in this session.")
	} else {
		c.Say("gnosh starts again, in this session.")
	}
	return nil
}

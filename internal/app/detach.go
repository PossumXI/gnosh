package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/shellquote"
)

// This file is /background (also /bg), as Claude Code has it: the session
// goes on without this terminal. It ends here as /exit ends it, and gnosh
// starts again in it as a run of its own (gnosh -p, resumed, with the
// session's flags as they stand) in a process that outlives this one, on
// the prompt given; once the run is done, gnosh -r opens the session again
// to read and go on. A run has nobody to ask, so what would ask is
// refused, as in any run; the run's output goes to a file beside the
// session's.

// detachPlan is what /background goes on with once the session is closed.
type detachPlan struct {
	session, prompt, log string
}

// detachedPlan is the plan /background made, or nil.
func (p *program) detachedPlan() *detachPlan {
	p.restartMu.Lock()
	defer p.restartMu.Unlock()
	return p.detached
}

// detach starts the run /background asked for and says how to come back
// to the session, or, when it cannot, how to go on by hand.
func (p *program) detach(args []string, d *detachPlan) int {
	run := append(append(args, "--print", "--"), d.prompt)
	pid, err := p.o.Detach(run, d.log)
	if err != nil {
		p.errf("the session could not go on in the background (%v). It is kept: gnosh %s runs the prompt, and gnosh -r %s opens it.", err, shellquote.Words(run), d.session)
		return cli.ExitFailure
	}
	fmt.Fprintf(p.stderr, "gnosh: the session goes on in the background (process %d). What it prints is in %s; gnosh -r %s opens it again once it is done.\n", pid, d.log, d.session)
	return cli.ExitOK
}

// slashBackground is /background <prompt>.
func (l *live) slashBackground(ctx context.Context, c *SlashContext) error {
	prompt := strings.TrimSpace(c.Args)
	switch {
	case l.p.o.Detach == nil:
		return errors.New("this gnosh cannot go on without its terminal")
	case prompt == "":
		return errors.New("say what the session is to do on its own: /background <prompt>. It goes on as a run of its own, which asks nobody anything, and gnosh -r opens it again once that is done")
	}
	args, err := l.goOnArgs(c, "background")
	if err != nil {
		return err
	}
	id := l.ts.ID()
	d := &detachPlan{session: id, prompt: prompt, log: filepath.Join(l.crashDir(), id+".background.log")}
	p := l.p
	p.restartMu.Lock()
	p.restart, p.detached = args, d
	p.restartMu.Unlock()
	c.Say("The session goes on in the background, on its own, with: %s", oneLine(prompt, 200))
	return nil
}

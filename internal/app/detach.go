package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/shellquote"
	"github.com/gnosh-ai/gnosh/internal/transcript"
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
	p.logHead(d.log, d.session, d.prompt)
	pid, err := p.o.Detach(run, slices.Clone(p.o.Environ), d.log)
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

// slashFork is /fork <prompt>, as Claude Code has it: the conversation so
// far is copied into a new session, which goes on in the background as a
// run of its own on the prompt, while this session goes on here. The copy
// works in the same folder.
func (l *live) slashFork(ctx context.Context, c *SlashContext) error {
	prompt := strings.TrimSpace(c.Args)
	switch {
	case l.p.o.Detach == nil:
		return errors.New("this gnosh cannot start a session that goes on without a terminal")
	case prompt == "":
		return errors.New("say what the copy is to do: /fork <prompt>. It goes on in the background as a run of its own, which asks nobody anything, while this session goes on here")
	case l.busy(ctx):
		return errors.New("a turn is running. Wait for it, or stop it, then fork")
	case !slices.ContainsFunc(l.ts.Entries(), func(e transcript.Entry) bool { return e.Type == transcript.TypeUser }):
		return errors.New("there is nothing to copy yet: the conversation has not begun")
	}
	id, err := l.copySession("fork")
	if err != nil {
		return fmt.Errorf("the conversation could not be copied: %v", err)
	}
	args, err := l.relaunchFor(id, false)
	if err != nil {
		return fmt.Errorf("the copy %s was made, and the command line to start it could not be (%v): gnosh -r %s opens it", id, err, id)
	}
	run := append(args, "--print", "--", prompt)
	log := filepath.Join(l.crashDir(), id+".background.log")
	l.p.logHead(log, id, prompt)
	pid, err := l.p.o.Detach(run, slices.Clone(l.p.o.Environ), log)
	if err != nil {
		return fmt.Errorf("the copy %s was made, and could not be started (%v): gnosh -r %s opens it", id, err, id)
	}
	if t := l.st.tree; t != nil {
		// The run works in this session's work tree, which must outlive
		// this session.
		t.forked.Store(true)
	}
	l.background(func(ctx context.Context) { l.watchRun(ctx, id, log) })
	c.Say("A copy of this conversation, %s, goes on in the background (process %d) with: %s", id, pid, oneLine(prompt, 200))
	c.Say("It works in this same folder, beside this session. gnosh -r %s opens it once it is done; what it prints is in %s.", id, l.tilde(log))
	return nil
}

// watchRun tells the person, while this session is open, when a run /fork
// started is done, as Claude Code's agent view does: the run holds the
// copy's session file while it works and lets go when it ends. A run that
// never took the file is said to have ended without starting, with the
// last line of its log, once it has had runStart to take it.
func (l *live) watchRun(ctx context.Context, id, logPath string) {
	path := filepath.Join(l.crashDir(), id+".jsonl")
	started := l.p.now()
	took := false
	tick := time.NewTicker(runLook)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if transcript.InUse(path) {
			took = true
			continue
		}
		if !took && l.p.now().Sub(started) < runStart {
			continue
		}
		_, last, _ := readRunLog(logPath, started)
		said := ""
		if last != "" {
			said = " It ended with: " + oneLine(last, 160)
		}
		if took {
			l.notice(agent.LevelInfo, "fork", fmt.Sprintf("The fork %s is done (gnosh -r %s opens it).%s", shortID(id), id, said))
		} else {
			l.notice(agent.LevelWarning, "fork", fmt.Sprintf("The fork %s ended without taking its session (what it printed is in %s).%s", shortID(id), l.tilde(logPath), said))
		}
		return
	}
}

// runLook is how often watchRun looks at a run's session, and runStart how
// long a run has to take it before it is said not to have started.
const (
	runLook  = 2 * time.Second
	runStart = 30 * time.Second
)

// copySession copies the conversation into a new session of this folder,
// named after this one with what the copy is for, and returns its id.
func (l *live) copySession(what string) (string, error) {
	copied, err := l.store.Branch(l.ts, "")
	if err != nil {
		return "", err
	}
	if was, _ := l.title(); was != "" {
		_, err = copied.Append(transcript.TitleEntry(was+" ("+what+")", transcript.SourceAuto))
	}
	id := copied.ID()
	if cerr := copied.Close(); err == nil {
		err = cerr
	}
	return id, err
}

// logHead begins a run's log with what it is: when it started, the
// session it goes on with, and its prompt. The run's own output follows.
// A log that cannot be written to is left to the run, which says why.
func (p *program) logHead(log, session, prompt string) {
	f, err := os.OpenFile(log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "gnosh: %s the session %s goes on in the background with: %s\n", p.now().UTC().Format(time.RFC3339), session, oneLine(prompt, 400))
}

// backgroundRun is a run that /background or /fork started from this
// folder: the session it goes on with, its prompt, when it started,
// whether it still runs (it holds its session while it does), and, once
// it is done, the last line it printed.
type backgroundRun struct {
	session, prompt, log string
	started              time.Time
	running              bool
	last                 string
}

// runsShown is how many runs in the background /tasks lists, the newest
// first, and runsKept how old a run's log may be to be among them.
const (
	runsShown = 10
	runsKept  = 7 * 24 * time.Hour
)

// backgroundRuns are the runs in the background of this folder's sessions
// whose logs were written in the last week, the newest first. A log a
// session was sent to the background more than once holds every run; the
// last is the one told.
func (l *live) backgroundRuns() []backgroundRun {
	dir := l.crashDir()
	if dir == "" {
		return nil
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.background.log"))
	var out []backgroundRun
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil || l.p.now().Sub(info.ModTime()) > runsKept {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(p), ".background.log")
		r := backgroundRun{session: id, log: p, started: info.ModTime()}
		r.prompt, r.last, r.started = readRunLog(p, r.started)
		r.running = transcript.InUse(filepath.Join(dir, id+".jsonl"))
		if r.running {
			r.last = ""
		}
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b backgroundRun) int { return b.started.Compare(a.started) })
	if len(out) > runsShown {
		out = out[:runsShown]
	}
	return out
}

// runLogRead is how much of the end of a run's log is read: its last
// run's first line and last line are in it, but for a run that printed
// more, which then shows no prompt.
const runLogRead = 1 << 20

// readRunLog reads the last run in a run's log: the prompt its first line
// gives (logHead), when it started (started, when the line gives no
// time), and the last line it printed after that.
func readRunLog(path string, started time.Time) (prompt, last string, when time.Time) {
	when = started
	f, err := os.Open(path)
	if err != nil {
		return "", "", when
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.Size() > runLogRead {
		_, _ = f.Seek(info.Size()-runLogRead, io.SeekStart)
	}
	data, _ := io.ReadAll(io.LimitReader(f, runLogRead))
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "gnosh: "); ok {
			if at, after, ok := strings.Cut(rest, " the session "); ok {
				if _, p, ok := strings.Cut(after, " goes on in the background with: "); ok {
					prompt, last = p, ""
					if t, err := time.Parse(time.RFC3339, at); err == nil {
						when = t
					}
					continue
				}
			}
		}
		if line != "" {
			last = line
		}
	}
	return prompt, last, when
}

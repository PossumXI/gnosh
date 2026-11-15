package hooks

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// Options is what a Runner is given about the program and the machine
// around it. Nothing is read from the process's own environment, clock or
// working directory unless a field's comment says so.
type Options struct {
	// Fires lists the events the program really fires. It has no
	// default: a hook configured on an event that is not listed would
	// never run, so Check reports it and NewRunner refuses it, and Run
	// refuses an event that is not listed. Events returns all fifteen.
	Fires []Event

	// ProjectDir is the project's root, where the session started. Hooks
	// get it as GNOSH_PROJECT_DIR and as CLAUDE_PROJECT_DIR, so that a
	// command written for Claude Code, such as
	// "$CLAUDE_PROJECT_DIR"/.claude/hooks/check.sh, works unchanged. It
	// is required when there are hooks.
	//
	// The shell fills the variable in. gnosh does not paste the directory
	// into the command's text, so a directory's name is never read as
	// shell code.
	ProjectDir string

	// StartDir is the directory the session started in. Hooks start
	// there when the session's working directory no longer exists.
	StartDir string

	// Shell is the shell that runs hook commands: a program that takes
	// -c and a command line. Empty means /bin/bash when that file exists
	// and /bin/sh when it does not.
	Shell string

	// Environ returns the environment hooks start from, and is called
	// for every event. Nil means os.Environ. The variables this package
	// sets are laid over it (see the package comment).
	Environ func() []string

	// Now is the clock a hook's Duration is measured with. Nil means
	// time.Now. It must be safe to call from several goroutines. Timeouts
	// always run on real time.
	Now func() time.Time

	// MemStatus is the path of the file that says what the project's mem
	// holds. Hooks get it as GNOSH_MEM_STATUS. Empty leaves it unset.
	MemStatus string

	// ToolNames lists the tools the program has. It is used only by
	// Check, to warn about a matcher that fits none of them. Empty means
	// no such warning.
	ToolNames []string

	// Renamed maps an older name of a tool to the name it has now. A
	// matcher that uses the older name fits the tool in full. Claude
	// Code's own renames are always known: Task (now Agent), KillShell
	// and KillBash (now TaskStop), BashOutput (now TaskOutput).
	Renamed map[string]string

	// Guards maps a tool's name to the Claude Code tool names whose
	// hooks also guard it, for tools Claude Code has nothing like: a
	// tool that saves a URL to a file is guarded by WebFetch and Write.
	// A hook that fits a call only this way can block it, put it to the
	// user, add text and change the input, but cannot approve it. Edit
	// is always guarded by MultiEdit.
	Guards map[string][]string

	// KillGrace is how long a hook that is being stopped, because its
	// time ran out or the turn was cancelled, has between SIGTERM and
	// SIGKILL. Default: 2 seconds.
	KillGrace time.Duration

	// PipeWait is how long to wait, once a hook's own process has exited,
	// for a process it left behind to let go of the hook's output. After
	// that the hook counts as one that could not evaluate. Default: 10
	// seconds, as in Claude Code.
	PipeWait time.Duration

	// MaxOutput is how many bytes of a hook's stdout are read. A hook
	// that prints more could not be read in full and so could not
	// evaluate. Default: 4 MiB.
	MaxOutput int

	// MaxText is how many characters of one hook's reason, added context
	// or warning are passed on. Longer text is passed on as its start
	// and its end around a line that says how much was left out.
	// Default: 10,000, as in Claude Code.
	MaxText int
}

const (
	// stderrKeep is how many bytes of a hook's stderr are kept at each
	// end.
	stderrKeep = 64 << 10
	// killWait is how long to wait, after SIGKILL, for a hook's processes
	// to be gone.
	killWait = 5 * time.Second
	// drainWait is how long to wait for the last of a stopped hook's
	// output, which is only wanted for the record.
	drainWait = 200 * time.Millisecond
)

// Runner runs the hooks of a configuration. It holds no state that
// changes, so one Runner serves a whole process, sub-agents included, and
// Run may be called from several goroutines at once.
type Runner struct {
	hooks      map[Event][]compiled
	fires      []Event
	tools      toolNames
	shell      string
	environ    func() []string
	now        func() time.Time
	projectDir string
	startDir   string
	memStatus  string
	killGrace  time.Duration
	pipeWait   time.Duration
	maxOutput  int
	maxText    int
}

// compiled is a hook with its matcher read.
type compiled struct {
	Hook
	m matcher
}

// selected is a hook chosen to run on one occurrence of an event.
type selected struct {
	Hook
	// guardOnly says the hook fits the tool only as a guard for another
	// tool's name.
	guardOnly bool
}

// NewRunner makes a Runner for a configuration. It refuses, with a
// *ConfigError listing every problem, a configuration that Check finds a
// problem with (warnings aside): gnosh then stops at startup instead of
// running with a guard missing.
//
// To run with no hooks at all (safe mode, or "disableAllHooks"), give it
// an empty Config and the same Options.
func NewRunner(cfg Config, opts Options) (*Runner, error) {
	if bad := stopping(Check(cfg, opts)); len(bad) > 0 {
		return nil, &ConfigError{Problems: bad}
	}
	if len(cfg.Hooks) > 0 && opts.ProjectDir == "" {
		return nil, errors.New("hooks cannot be run without the project's directory, which they are given as GNOSH_PROJECT_DIR and CLAUDE_PROJECT_DIR (Options.ProjectDir is empty)")
	}
	r := &Runner{
		hooks:      map[Event][]compiled{},
		fires:      slices.Clone(opts.Fires),
		tools:      newToolNames(opts.Renamed, opts.Guards),
		shell:      opts.Shell,
		environ:    opts.Environ,
		now:        opts.Now,
		projectDir: opts.ProjectDir,
		startDir:   opts.StartDir,
		memStatus:  opts.MemStatus,
		killGrace:  opts.KillGrace,
		pipeWait:   opts.PipeWait,
		maxOutput:  opts.MaxOutput,
		maxText:    opts.MaxText,
	}
	if r.shell == "" {
		r.shell = "/bin/sh"
		if st, err := os.Stat("/bin/bash"); err == nil && !st.IsDir() {
			r.shell = "/bin/bash"
		}
	}
	if r.environ == nil {
		r.environ = os.Environ
	}
	if r.now == nil {
		r.now = time.Now
	}
	if r.killGrace <= 0 {
		r.killGrace = 2 * time.Second
	}
	if r.pipeWait <= 0 {
		r.pipeWait = 10 * time.Second
	}
	if r.maxOutput <= 0 {
		r.maxOutput = 4 << 20
	}
	if r.maxText <= 0 {
		r.maxText = 10000
	}
	for _, h := range cfg.Hooks {
		info, _ := describe(h.Event)
		m, _ := compileMatcher(h.Matcher, info.narrow) // Check has passed it
		r.hooks[h.Event] = append(r.hooks[h.Event], compiled{Hook: h, m: m})
	}
	return r, nil
}

// Has reports whether any hook is configured for the event. The agent
// loop uses it to skip work that only exists for hooks: the timers behind
// Notification, the last message gathered for Stop.
func (r *Runner) Has(event Event) bool {
	return len(r.hooks[event]) > 0
}

// Hooks returns the hooks in force for the event, in the order they run
// and report in.
func (r *Runner) Hooks(event Event) []Hook {
	out := make([]Hook, 0, len(r.hooks[event]))
	for _, h := range r.hooks[event] {
		out = append(out, h.Hook)
	}
	return out
}

// Run runs the hooks of one occurrence of an event and returns what they
// came to.
//
// The hooks whose matcher fits are chosen; two with the same command run
// once. They are started at the same time, each as a shell command in a
// process group and session of its own with no terminal, with the event
// as JSON on stdin, and all are waited for. A hook that passes its
// timeout is stopped with its whole process group and counts as one that
// could not evaluate. Their answers are then combined in the order the
// hooks are configured (see Outcome).
//
// Run returns an error in two cases, and in both the Outcome it returns
// never lets a tool call or a prompt through (Blocked is set for
// PreToolUse and UserPromptSubmit):
//
//   - ctx was cancelled before every hook had finished. The hooks still
//     running are stopped, with their process groups, before Run returns.
//     The error wraps ctx's. A cancelled turn is not a hook's failure, so
//     nothing is reported as blocked by a hook; the Records say which
//     hooks finished and which were cut short.
//   - the Input is wrong: an event that is not one of gnosh's or that
//     Options.Fires does not list, a tool event with no tool name, a
//     value its event cannot have. These are mistakes in the calling
//     program, and no hook is run.
func (r *Runner) Run(ctx context.Context, in Input) (Outcome, error) {
	info, ok := describe(in.Event)
	switch {
	case !ok:
		return r.refuse(in.Event, nil, errors.New("no hooks were run: "+eventProblem(string(in.Event))))
	case !slices.Contains(r.fires, in.Event):
		return r.refuse(in.Event, nil, fmt.Errorf("the %s hooks were not run: this Runner was told that gnosh does not fire %s (it is not in Options.Fires)", in.Event, in.Event))
	}
	if err := in.check(info); err != nil {
		return r.refuse(in.Event, nil, fmt.Errorf("the %s hooks were not run: %w", in.Event, err))
	}
	sel := r.choose(&in, info)
	if len(sel) == 0 {
		return Outcome{}, nil
	}
	if ctx.Err() != nil {
		return r.refuse(in.Event, nil, stopped(ctx, in.Event))
	}

	base := r.environ()
	dir := r.startIn(in.Cwd, base)
	if in.Cwd == "" {
		in.Cwd = dir
	}
	stdin, err := in.payload(info, in.Cwd)
	if err != nil {
		return r.refuse(in.Event, nil, fmt.Errorf("the %s hooks were not run: %w", in.Event, err))
	}
	env := in.environ(info, base, dir, r.projectDir, r.memStatus)

	procs := make([]proc, len(sel))
	var all sync.WaitGroup
	for i, h := range sel {
		all.Go(func() {
			procs[i] = r.exec(ctx, h.Command, stdin, env, dir, timeoutFor(h.Event, h.Timeout))
		})
	}
	all.Wait()

	out := r.combine(&in, info, sel, procs)
	if slices.ContainsFunc(procs, func(p proc) bool { return p.cancelled }) {
		return r.refuse(in.Event, out.Records, stopped(ctx, in.Event))
	}
	return out, nil
}

// refuse returns what Run returns when it has no answer from the hooks:
// the error, and an Outcome that lets nothing through.
func (r *Runner) refuse(event Event, records []Record, err error) (Outcome, error) {
	out := Outcome{Records: records}
	switch event {
	case PreToolUse:
		out.Blocked, out.PermissionDecision, out.Reason = true, Deny, err.Error()
	case UserPromptSubmit:
		out.Blocked, out.Reason = true, err.Error()
	}
	return out, err
}

// stopped is the error for hooks cut short by a cancelled context. It
// wraps the context's own error and, when the context was given one, the
// cause it was cancelled for.
func stopped(ctx context.Context, event Event) error {
	err, cause := ctx.Err(), context.Cause(ctx)
	if cause == nil || cause == err {
		return fmt.Errorf("the %s hooks were stopped before they finished: %w", event, err)
	}
	return fmt.Errorf("the %s hooks were stopped before they finished: %w: %w", event, err, cause)
}

// choose picks the hooks that run on this occurrence of the event, in the
// order they are configured.
func (r *Runner) choose(in *Input, info eventInfo) []selected {
	var out []selected
	for _, h := range r.hooks[in.Event] {
		matched, guardOnly := true, false
		switch info.subject {
		case toolName:
			matched, guardOnly = r.tools.fit(h.m, in.ToolName)
		case ownValue:
			matched = h.m.matches(in.subject())
		}
		if !matched {
			continue
		}
		// An "if" filter: on a tool event the hook runs for the calls the
		// filter takes; on any other event never (spec/cc/hooks.md).
		if h.If != "" && (info.subject != toolName || in.IfMatch != nil && !in.IfMatch(h.If)) {
			continue
		}
		// The same command configured twice runs once. It is a guard
		// only if every hook that brought it here is one.
		if i := slices.IndexFunc(out, func(s selected) bool { return s.Command == h.Command }); i >= 0 {
			out[i].guardOnly = out[i].guardOnly && guardOnly
			continue
		}
		out = append(out, selected{Hook: h.Hook, guardOnly: guardOnly})
	}
	return out
}

// startIn returns the directory hooks start in: the session's working
// directory, or, when that is gone, the directory the session started in,
// the project's, the user's home, and at last the system's temporary one.
func (r *Runner) startIn(cwd string, env []string) string {
	lookup := func(name string) string {
		for _, kv := range env {
			if v, ok := strings.CutPrefix(kv, name+"="); ok {
				return v
			}
		}
		return ""
	}
	for _, dir := range []string{cwd, r.startDir, r.projectDir, lookup("HOME"), lookup("TMPDIR"), "/tmp"} {
		if dir == "" {
			continue
		}
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return "/"
}

// proc is how running one hook's command went.
type proc struct {
	// startErr is why the command could not be started.
	startErr error
	// exit is the exit status, or -1 when there is none: the command
	// never started, was killed, or was stopped by gnosh.
	exit int
	// signal names the signal that killed the command, when one did and
	// gnosh did not send it.
	signal string
	// timedOut and cancelled say that gnosh stopped the command, and
	// why. limit is the time it was given.
	timedOut  bool
	cancelled bool
	limit     time.Duration
	// leftOpen says the command exited but something it started kept its
	// output open past the wait for it.
	leftOpen bool
	// unknown says why gnosh cannot tell how the command ended or what it
	// printed, when it cannot: the system would not say, or reading failed.
	unknown string
	// unstopped says that gnosh stopped the command and could not see all
	// of its processes go.
	unstopped bool
	// stdout is what the command printed, up to the limit; lost counts
	// the bytes past it. stderr is the start and the end of what it wrote
	// to standard error.
	stdout string
	lost   int64
	stderr string
	// took is how long it all took.
	took time.Duration
}

// pipes are the three pipes between gnosh and a hook.
type pipes struct {
	inR, inW   *os.File // the hook's stdin; gnosh writes to inW
	outR, outW *os.File // the hook's stdout; gnosh reads from outR
	errR, errW *os.File // the hook's stderr; gnosh reads from errR
}

// openPipes makes the three pipes.
func openPipes() (*pipes, error) {
	var p pipes
	var err error
	if p.inR, p.inW, err = os.Pipe(); err == nil {
		if p.outR, p.outW, err = os.Pipe(); err == nil {
			if p.errR, p.errW, err = os.Pipe(); err == nil {
				return &p, nil
			}
		}
	}
	p.closeHook()
	p.closeOurs()
	return nil, err
}

// closeHook closes the ends that belong to the hook.
func (p *pipes) closeHook() { closeFiles(p.inR, p.outW, p.errW) }

// closeOurs closes gnosh's own ends. Whoever is reading or writing them
// in another goroutine is let go.
func (p *pipes) closeOurs() { closeFiles(p.inW, p.outR, p.errR) }

func closeFiles(files ...*os.File) {
	for _, f := range files {
		if f != nil {
			f.Close()
		}
	}
}

// exec runs one hook's command and waits for it, for at most limit, or
// until ctx is cancelled. When it stops the hook it stops the hook's whole
// process group, and it does not return before its own goroutines have
// finished (but for the one that waits on a process the system would not
// kill, which ends when that process does).
func (r *Runner) exec(ctx context.Context, command string, stdin []byte, env []string, dir string, limit time.Duration) (p proc) {
	p.exit, p.limit = -1, limit
	began := r.now()
	defer func() { p.took = r.now().Sub(began) }()

	pp, err := openPipes()
	if err != nil {
		p.startErr = err
		return p
	}
	cmd := exec.Command(r.shell, "-c", command)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pp.inR, pp.outW, pp.errW
	ownGroup(cmd)
	if err := cmd.Start(); err != nil {
		pp.closeHook()
		pp.closeOurs()
		p.startErr = err
		return p
	}
	pp.closeHook() // the hook has its own copies now
	pid := cmd.Process.Pid

	// Three things go on beside the command: the event is written to its
	// stdin, its output is read (and bounded), and its exit is waited
	// for. cmd.Wait returns as soon as the command's own process has
	// exited, because the pipes are ours and not the exec package's.
	exited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	wrote := make(chan struct{})
	go func() {
		defer close(wrote)
		// A hook that does not read its input, or exits before it has,
		// is not at fault for that.
		pp.inW.Write(stdin)
		pp.inW.Close()
	}()
	stdout := head{max: r.maxOutput}
	stderr := ends{keep: stderrKeep}
	var outErr, errErr error
	read := make(chan struct{})
	go func() {
		defer close(read)
		var both sync.WaitGroup
		both.Go(func() { _, outErr = io.Copy(&stdout, pp.outR) })
		both.Go(func() { _, errErr = io.Copy(&stderr, pp.errR) })
		both.Wait()
	}()

	deadline := time.NewTimer(limit)
	defer deadline.Stop()
	select {
	case <-exited:
		// The hook's own process has exited. Something it started may
		// still hold its output open, and until that lets go the hook
		// may not have said everything.
		wait := time.NewTimer(r.pipeWait)
		select {
		case <-read:
		case <-wait.C:
			p.leftOpen = true
		case <-deadline.C:
			p.timedOut = true
		case <-ctx.Done():
			p.cancelled = true
		}
		wait.Stop()
	case <-deadline.C:
		p.timedOut = true
	case <-ctx.Done():
		p.cancelled = true
	}
	if p.timedOut || p.cancelled {
		p.unstopped = !r.stop(pid, exited)
		// What it printed is no answer now, but the last of it is worth
		// having for the record.
		select {
		case <-read:
		case <-time.After(drainWait):
		}
	}
	pp.closeOurs()
	<-read
	<-wrote

	p.stdout, p.lost = string(stdout.buf), stdout.lost
	p.stderr = stderr.text()
	if p.timedOut || p.cancelled {
		return p
	}
	// The hook ended by itself. If the system will not say how, or its
	// output could not be read to the end, nothing can be made of it: a
	// hook whose answer is not known has not passed.
	p.exit, p.signal = ending(cmd.ProcessState)
	switch {
	case cmd.ProcessState == nil:
		p.unknown = "the system did not say how it ended"
		if waitErr != nil {
			p.unknown += ": " + waitErr.Error()
		}
	case p.leftOpen:
	case outErr != nil:
		p.unknown = "what it printed could not be read: " + outErr.Error()
	case errErr != nil:
		p.unknown = "what it wrote to standard error could not be read: " + errErr.Error()
	}
	return p
}

// stop ends a hook's whole process group. It asks first (SIGTERM), so
// that a hook that started helpers of its own can stop them, and when the
// grace runs out it kills whatever is left (SIGKILL). It reports whether
// everything is known to be gone.
func (r *Runner) stop(pid int, exited <-chan struct{}) bool {
	terminate(pid, isClosed(exited))
	if settled(pid, exited, r.killGrace) {
		return true
	}
	destroy(pid, isClosed(exited))
	return settled(pid, exited, killWait)
}

// settled waits, for at most d, until the hook's own process has been
// waited for and no process is left in its group.
func settled(pid int, exited <-chan struct{}, d time.Duration) bool {
	end := time.NewTimer(d)
	defer end.Stop()
	select {
	case <-exited:
	case <-end.C:
		return false
	}
	for pause := time.Millisecond; ; pause = min(2*pause, 20*time.Millisecond) {
		if groupGone(pid) {
			return true
		}
		select {
		case <-end.C:
			return false
		case <-time.After(pause):
		}
	}
}

// isClosed reports whether ch has been closed.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

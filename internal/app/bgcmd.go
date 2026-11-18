package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// This file is gnosh --bg and gnosh stop, as Claude Code has claude --bg
// and claude stop: a session started in the background from a shell, and
// stopped from one; gnosh logs <session> (more.go) shows what it printed.
// The run is the kind /background and /fork start (detach.go): gnosh -p
// in a process of its own, its output in <session>.background.log beside
// the session's file, its process id in <session>.background.pid.

// stopWait is how long gnosh stop waits for a run to let go of its session
// after it was asked to stop, before it is made to.
const stopWait = 10 * time.Second

// backgroundCommand is gnosh --bg: a new session on the prompt, as a run in
// the background, and how to follow it.
func (p *program) backgroundCommand(ctx context.Context, inv cli.Invocation) int {
	st, code := p.prepare(ctx, inv, false)
	if st == nil || st.cfg == nil {
		if st != nil {
			st.printProblems()
		}
		return max(code, cli.ExitFailure)
	}
	if p.o.Detach == nil {
		p.errf("this gnosh cannot start a session in the background, so nothing was started.")
		return cli.ExitFailure
	}
	store := st.sessions()
	id := inv.SessionID
	if id == "" {
		var err error
		if id, err = store.NewID(); err != nil {
			p.errf("the session could not be given an ID (%v), so nothing was started.", err)
			return cli.ExitFailure
		}
	}
	args, err := p.table.RelaunchArgs(p.o.Args, cli.Relaunch{Session: id, Fresh: true})
	if err != nil {
		p.errf("the command line for the run could not be made (%v), so nothing was started.", err)
		return cli.ExitFailure
	}
	if inv.Name != "" {
		args = append(args, "--name="+inv.Name)
	}
	run := append(args, "--print", "--", inv.Prompt)
	dir := store.ProjectDir(st.cfg.Paths.Cwd)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		p.errf("the folder for the session's files, %s, could not be made (%v), so nothing was started.", dir, err)
		return cli.ExitFailure
	}
	log := filepath.Join(dir, id+".background.log")
	p.logHead(log, id, filepath.Join(dir, id+".jsonl"), inv.Prompt)
	pid, err := p.o.Detach(run, slices.Clone(p.o.Environ), log)
	if err != nil {
		p.errf("the session could not be started in the background (%v).", err)
		return cli.ExitFailure
	}
	writeRunPID(log, pid)
	short := runShort(id)
	fmt.Fprintf(p.stdout, "gnosh: backgrounded · %s · process %d\n", id, pid)
	hints := [][2]string{
		{"gnosh logs " + short, "what it prints (-f follows it)"},
		{"gnosh stop " + short, "stops it"},
		{"gnosh -r " + id, "opens it once it is done"},
	}
	width := 0
	for _, h := range hints {
		width = max(width, len(h[0]))
	}
	for _, h := range hints {
		fmt.Fprintf(p.stdout, "  %-*s  %s\n", width, h[0], h[1])
	}
	return cli.ExitOK
}

// runShort is the start of a session's id that names its run in the
// folder's runs: up to the end of its second group, the time to the
// millisecond in gnosh's ids, since two runs started within the same
// minute share the first eight characters.
func runShort(id string) string {
	if len(id) > 13 && id[8] == '-' {
		return id[:13]
	}
	return id
}

// pidFile is the file beside a run's log that holds its process id.
func pidFile(log string) string {
	return strings.TrimSuffix(log, ".log") + ".pid"
}

// writeRunPID keeps a run's process id beside its log, for gnosh stop. A
// run whose id was not kept can still be followed and opened again.
func writeRunPID(log string, pid int) {
	_ = os.WriteFile(pidFile(log), []byte(strconv.Itoa(pid)+"\n"), 0o600)
}

// findRun finds the run of this folder that word names: a session's id, or
// the start of one that a single run's has, by its log.
func findRun(dir, word string) (id, log string, err error) {
	word = strings.TrimSpace(word)
	if word == "" || strings.ContainsAny(word, `/\`) {
		return "", "", fmt.Errorf("%q names no session", word)
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.background.log"))
	var found []string
	for _, p := range paths {
		run := strings.TrimSuffix(filepath.Base(p), ".background.log")
		if run == word {
			return run, p, nil
		}
		if strings.HasPrefix(run, word) {
			found = append(found, run)
		}
	}
	switch len(found) {
	case 1:
		return found[0], filepath.Join(dir, found[0]+".background.log"), nil
	case 0:
		return "", "", fmt.Errorf("no session of this folder ran in the background under the ID %s (gnosh --bg, /background and /fork start one; /tasks lists them)", word)
	}
	return "", "", fmt.Errorf("%s is the start of the IDs of %d sessions that ran in the background here (%s): give more of it", word, len(found), strings.Join(found, ", "))
}

// runSession is the session file a run's log names, or the one beside it.
func runSession(dir, id, log string) string {
	_, _, _, file := readRunLog(log, time.Time{})
	return firstOf(file, filepath.Join(dir, id+".jsonl"))
}

// stopCommand is gnosh stop <session>: the run is asked to stop, as Esc
// stops a turn, and made to when it has not let go of its session in
// stopWait. What it did so far stays in its session.
func (p *program) stopCommand(ctx context.Context, inv cli.Invocation) int {
	st, code := p.prepare(ctx, inv, false)
	if st == nil || st.cfg == nil {
		if st != nil {
			st.printProblems()
		}
		return max(code, cli.ExitFailure)
	}
	dir := st.sessions().ProjectDir(st.cfg.Paths.Cwd)
	id, log, err := findRun(dir, inv.Args[0])
	if err != nil {
		p.errf("%v.", err)
		return cli.ExitFailure
	}
	said, err := stopRun(ctx, p.now, dir, id, log)
	if err != nil {
		p.errf("%v.", err)
		return cli.ExitFailure
	}
	fmt.Fprintf(p.stdout, "gnosh: %s\n", said)
	return cli.ExitOK
}

// stopRun stops the run of a session in the background, as gnosh stop
// and /tasks stop do: it is asked to stop, as Esc stops a turn, and made
// to when it has not let go of its session in stopWait. It says what came
// of it, or why nothing was stopped.
func stopRun(ctx context.Context, now func() time.Time, dir, id, log string) (string, error) {
	file := runSession(dir, id, log)
	if !transcript.InUse(file) {
		return fmt.Sprintf("the session %s is not running: gnosh -r %s opens it.", id, id), nil
	}
	data, err := os.ReadFile(pidFile(log))
	pid, perr := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || perr != nil || pid <= 1 {
		return "", fmt.Errorf("the session %s is running, and gnosh has no process ID for its run (an older gnosh started it, or the session is open somewhere else), so nothing was stopped", id)
	}
	if !runIsFor(pid, id) {
		return "", fmt.Errorf("process %d, which ran the session %s, is not running it any more, and the session is open somewhere else; nothing was stopped", pid, id)
	}
	proc, err := os.FindProcess(pid)
	if err == nil {
		err = proc.Signal(syscall.SIGTERM)
	}
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return "", fmt.Errorf("process %d could not be asked to stop: %v", pid, err)
	}
	deadline := now().Add(stopWait)
	for transcript.InUse(file) && now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if transcript.InUse(file) && runIsFor(pid, id) {
		_ = proc.Kill()
		return fmt.Sprintf("the session %s did not stop within %s, and its run was ended. What it did so far is kept: gnosh -r %s opens it.", id, stopWait, id), nil
	}
	return fmt.Sprintf("the session %s was stopped. What it did so far is kept: gnosh -r %s opens it.", id, id), nil
}

// runIsFor reports whether process pid is a gnosh run of the session id:
// its command line names the session. A process id is used again once
// its process has ended, and a stop must not reach another program.
func runIsFor(pid int, id string) bool {
	if data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); err == nil {
		return strings.Contains(string(data), id)
	}
	out, err := exec.Command("ps", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
	return err == nil && strings.Contains(string(out), id)
}

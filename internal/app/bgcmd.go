package app

import (
	"context"
	"encoding/json"
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
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// This file is gnosh --bg, gnosh agents and gnosh stop, as Claude Code has
// claude --bg, claude agents and claude stop: a session started in the
// background from a shell, listed and stopped from one, from any folder;
// gnosh logs <session> (more.go) shows what it printed.
// The run is the kind /background and /fork start (detach.go): gnosh -p
// in a process of its own, its output in <session>.background.log beside
// the session's file, its process id in <session>.background.pid.

// stopWait is how long gnosh stop waits for a run to let go of its session
// after it was asked to stop, before it is made to.
const stopWait = 10 * time.Second

// backgroundCommand is gnosh --bg: a new session on the prompt, as a run in
// the background, and how to follow it.
func (p *program) backgroundCommand(ctx context.Context, inv cli.Invocation) int {
	st, code := p.prepared(ctx, inv)
	if st == nil {
		return code
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
	p.logHead(log, id, st.cfg.Paths.Cwd, filepath.Join(dir, id+".jsonl"), inv.Prompt)
	pid, err := p.o.Detach(run, slices.Clone(p.o.Environ), log)
	if err != nil {
		p.errf("the session could not be started in the background (%v).", err)
		return cli.ExitFailure
	}
	writeRunPID(log, pid)
	short := runShort(id)
	fmt.Fprintf(p.stdout, "gnosh: backgrounded · %s · process %d\n", id, pid)
	hints := [][2]string{
		{"gnosh agents", "lists the sessions in the background"},
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

// findRun finds the run in the background that word names, among those
// whose logs are in dirs: a session's id, or the start of one that a
// single run's has.
func findRun(dirs []string, word string) (id, log string, err error) {
	word = strings.TrimSpace(word)
	if word == "" || strings.ContainsAny(word, `/\`) {
		return "", "", fmt.Errorf("%q names no session", word)
	}
	var found []string // the ids that begin with word, and their logs
	var logs []string
	for _, dir := range dirs {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.background.log"))
		for _, p := range paths {
			run := strings.TrimSuffix(filepath.Base(p), ".background.log")
			if run == word {
				return run, p, nil
			}
			if strings.HasPrefix(run, word) {
				found, logs = append(found, run), append(logs, p)
			}
		}
	}
	switch len(found) {
	case 1:
		return found[0], logs[0], nil
	case 0:
		return "", "", fmt.Errorf("no session ran in the background under the ID %s (gnosh agents lists them; gnosh --bg, /background and /fork start one)", word)
	}
	return "", "", fmt.Errorf("%s is the start of the IDs of %d sessions that ran in the background (%s): give more of it", word, len(found), strings.Join(found, ", "))
}

// runSession is the session file a run's log names, or the one beside it.
func runSession(id, log string) string {
	return firstOf(readRunLog(log, time.Time{}).file, filepath.Join(filepath.Dir(log), id+".jsonl"))
}

// stopCommand is gnosh stop <session>: the run is asked to stop, as Esc
// stops a turn, and made to when it has not let go of its session in
// stopWait. What it did so far stays in its session.
func (p *program) stopCommand(ctx context.Context, inv cli.Invocation) int {
	st, code := p.prepared(ctx, inv)
	if st == nil {
		return code
	}
	id, log, err := findRun(st.sessions().ProjectDirs(), inv.Args[0])
	if err != nil {
		p.errf("%v.", err)
		return cli.ExitFailure
	}
	said, err := stopRun(ctx, p.now, id, log)
	if err != nil {
		p.errf("%v.", err)
		return cli.ExitFailure
	}
	fmt.Fprintf(p.stdout, "gnosh: %s\n", said)
	return cli.ExitOK
}

// stopRun stops the run of a session in the background, as gnosh stop
// and /tasks stop do: it is asked to stop, as Esc stops a turn, and made
// to when it has not let go of its session in stopWait. Its log then says
// it was stopped. stopRun says what came of it, or why nothing was
// stopped.
func stopRun(ctx context.Context, now func() time.Time, id, log string) (string, error) {
	file := runSession(id, log)
	if !transcript.InUse(file) {
		return fmt.Sprintf("the session %s is not running: gnosh -r %s opens it.", id, id), nil
	}
	pid := runPID(log)
	if pid == 0 {
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
	said := fmt.Sprintf("the session %s was stopped. What it did so far is kept: gnosh -r %s opens it.", id, id)
	if transcript.InUse(file) && runIsFor(pid, id) {
		_ = proc.Kill()
		said = fmt.Sprintf("the session %s did not stop within %s, and its run was ended. What it did so far is kept: gnosh -r %s opens it.", id, stopWait, id)
	}
	if f, err := os.OpenFile(log, os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
		fmt.Fprintf(f, "gnosh: %s%s\n", now().UTC().Format(time.RFC3339), runStopped)
		f.Close()
	}
	return said, nil
}

// heldByRun says what to do about a session that may be held by a run in
// the background: it lets go once it is done.
func heldByRun(id string) string {
	short := runShort(id)
	return fmt.Sprintf("If it is a run in the background, it lets go of the session when it is done: gnosh logs -f %s follows it, and gnosh stop %s stops it", short, short)
}

// runPID is the process id kept beside a run's log, or 0.
func runPID(log string) int {
	data, err := os.ReadFile(pidFile(log))
	pid, perr := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || perr != nil || pid <= 1 {
		return 0
	}
	return pid
}

// runState is how a run stands, in the words Claude Code's list has for
// it: working while it holds its session, then stopped when gnosh stop
// stopped it, or else done.
func runState(r backgroundRun) string {
	switch {
	case r.running:
		return "working"
	case r.stopped:
		return "stopped"
	}
	return "done"
}

// agentsCommand is gnosh agents, as Claude Code has claude agents: the
// sessions in the background (gnosh --bg, /background, /fork) of every
// folder, or, with --cwd, of that folder and the folders under it. Claude
// Code's opens a view to work in; gnosh's prints the list, the working
// ones first: those done in the last week follow, or with --all every one
// whose log is kept. --json prints them as JSON, the working ones only
// unless --all is given too, as Claude Code's does.
func (p *program) agentsCommand(ctx context.Context, inv cli.Invocation) int {
	st, code := p.prepared(ctx, inv)
	if st == nil {
		return code
	}
	now := p.now()
	since := now.Add(-runsKept)
	if inv.All {
		since = time.Time{}
	}
	var working, done []backgroundRun
	for _, r := range runsIn(st.sessions().ProjectDirs(), since) {
		switch {
		case inv.Cwd != "" && !within(r.cwd, st.cfg.Paths.Cwd):
		case r.running:
			working = append(working, r)
		case !inv.JSON || inv.All:
			done = append(done, r)
		}
	}
	runs := append(working, done...)
	if inv.JSON {
		type row struct {
			ID        string    `json:"id"`
			Kind      string    `json:"kind"`
			Cwd       string    `json:"cwd,omitempty"`
			StartedAt time.Time `json:"startedAt"`
			State     string    `json:"state"`
			PID       int       `json:"pid,omitempty"`
			Status    string    `json:"status,omitempty"`
			Prompt    string    `json:"prompt,omitempty"`
			Log       string    `json:"log"`
		}
		rows := []row{}
		for _, r := range runs {
			w := row{ID: r.session, Kind: "background", Cwd: r.cwd, StartedAt: r.started.UTC(), State: runState(r),
				Status: oneLine(r.last, 400), Prompt: r.prompt, Log: r.log}
			if pid := runPID(r.log); r.running && pid != 0 && runIsFor(pid, r.session) {
				w.PID = pid
			}
			rows = append(rows, w)
		}
		enc := json.NewEncoder(p.stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(rows)
		return cli.ExitOK
	}
	if len(runs) == 0 {
		said := "No session runs in the background"
		if inv.Cwd != "" {
			said += " from " + homeTilde(st.cfg.Paths.Home, st.cfg.Paths.Cwd) + " or a folder in it"
		}
		if inv.All {
			said += ", and no log of one that ran is kept"
		} else {
			said += ", and none ran in the last week"
		}
		fmt.Fprintf(p.stdout, "%s: gnosh --bg <prompt> starts one.\n", said)
		return cli.ExitOK
	}
	type cells struct{ name, state, dir, prompt string }
	rows := make([]cells, len(runs))
	wState, wDir := 0, 0
	for i, r := range runs {
		state := runState(r)
		if r.running {
			state += " " + roundTime(now.Sub(r.started))
		}
		rows[i] = cells{runShort(r.session) + "  " + r.started.Local().Format("2006-01-02 15:04"), state,
			homeTilde(st.cfg.Paths.Home, firstOf(r.cwd, "(folder not known)")), oneLine(firstOf(r.prompt, "(its prompt is not in its log)"), 80)}
		wState, wDir = max(wState, utf8.RuneCountInString(state)), max(wDir, utf8.RuneCountInString(rows[i].dir))
	}
	for _, c := range rows {
		fmt.Fprintf(p.stdout, "%s  %-*s  %-*s  %s\n", c.name, wState, c.state, wDir, c.dir, c.prompt)
	}
	fmt.Fprintln(p.stdout, "gnosh logs <name> prints what one printed (-f follows it), gnosh stop <name> stops one, and gnosh -r <name> opens one.")
	return cli.ExitOK
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

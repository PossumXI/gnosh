package shelltool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// bashTool is the Bash tool: it runs one shell command.
type bashTool struct{ m *Manager }

// Spec says what the model is told about the tool.
func (t bashTool) Spec() core.ToolSpec { return t.m.bashSpec() }

// Describe says what a call would run, for permission rules and for the
// person who may be asked. Its error is the same refusal Run would give.
func (t bashTool) Describe(call core.ToolCall, env *core.ToolEnv) (core.Action, error) {
	a, err := t.m.bashArgs(call.Input)
	if err != nil {
		return core.Action{}, err
	}
	return core.Action{Kind: core.ActionExecute, Command: a.command, Summary: a.summary()}, nil
}

// Run runs the command. In the foreground it returns when the command has
// ended, been stopped, or been moved to the background; in the background
// it returns as soon as the command has started.
func (t bashTool) Run(ctx context.Context, call core.ToolCall, env *core.ToolEnv) (core.ToolResult, error) {
	m := t.m
	a, err := m.bashArgs(call.Input)
	if err != nil {
		return core.ErrorResult(err.Error()), nil
	}
	if env == nil || !filepath.IsAbs(env.Cwd) {
		return core.ToolResult{}, errors.New("shelltool: the tool was given no working directory to run the command in")
	}
	if ctx.Err() != nil {
		return core.ErrorResult("The command was not started: the call was cancelled first."), nil
	}
	j, err := m.start(a, env, call.ID)
	if err != nil {
		var no refusal
		if errors.As(err, &no) {
			return core.ErrorResult(string(no)), nil
		}
		return core.ToolResult{}, err
	}
	if a.background {
		return m.startedResult(j, a), nil
	}
	outcome := j.attend(ctx, a.limit)
	j.detach()
	m.unlist(j)
	if outcome != attendEnded {
		return m.movedResult(j, a, outcome == attendTimedOut), nil
	}
	return m.finishedResult(j, a, env), nil
}

// start starts a command. A refusal is for the model; any other error is
// the harness's own.
func (m *Manager) start(a bashArgs, env *core.ToolEnv, callID string) (*job, error) {
	if st, err := os.Stat(env.Cwd); err != nil || !st.IsDir() {
		return nil, refusal(fmt.Sprintf("The command was not run: the working directory %s is not there any more. Nothing can run until it exists again.", env.Cwd))
	}
	dir, err := m.spoolDir()
	if err != nil {
		return nil, err
	}
	j := newJob(m, a, env.Cwd)
	name, err := m.admit(j, a.background, callID)
	if err != nil {
		return nil, err
	}
	if env.Progress != nil && !a.background {
		j.sink.Store(newProgressSink(env.Progress, callID))
	}
	if err := j.launch(dir, name); err != nil {
		j.detach()
		m.forget(j)
		close(j.done)
		return nil, err
	}
	if a.background {
		m.publish(j)
		m.wg.Go(func() { j.background(a.limit) })
	}
	return j, nil
}

// finishedResult is the result of a foreground command that has ended.
func (m *Manager) finishedResult(j *job, a bashArgs, env *core.ToolEnv) core.ToolResult {
	e := j.ending()
	failed := e.failed()
	meaning := ""
	if e.reason == endExited && e.code == 1 {
		if meaning = harmlessExit(a.command); meaning != "" {
			failed = false
		}
	}
	limit := m.opt.MaxOutputChars
	if failed {
		limit = m.opt.MaxFailedOutputChars
	}
	body, cut, total, spooled := j.out.view(limit)
	_, _, spoolErr := j.out.size()
	took := e.at.Sub(j.started)

	meta := map[string]any{
		metaExitCode:    e.code,
		metaReason:      e.reason.String(),
		metaInterrupted: e.reason == endCancelled || e.reason == endClosed,
		metaDuration:    took.Milliseconds(),
		metaDir:         j.dir,
		metaOutputBytes: total,
		metaTruncated:   cut,
	}
	res := core.ToolResult{IsError: failed, Meta: meta}

	// What happened to the command, when it did not simply end: this goes
	// above the output.
	headline := ""
	switch e.reason {
	case endTimeout:
		headline = "timed out after " + seconds(e.limit) + " and was stopped; the output so far is below"
		if a.asked > 0 {
			headline += fmt.Sprintf(" (the %s asked for is over the maximum, so it was lowered to %s)", seconds(a.asked), seconds(e.limit))
		}
		if e.limit < time.Second {
			headline += fmt.Sprintf(" (%s is in milliseconds)", argTimeout)
		}
		meta[metaTimedOutAfter] = e.limit.Milliseconds()
	case endCancelled:
		headline = "interrupted after " + seconds(took) + " and stopped; the output so far is below"
	case endClosed:
		headline = "stopped after " + seconds(took) + " because the session is ending; the output so far is below"
	case endOutputLimit:
		headline = "stopped after printing more than " + byteSize(m.opt.MaxOutputBytes) + "; the start and the end of the output are below"
	}

	// The rest goes under the output.
	var notes []string
	switch {
	case !e.reaped:
		notes = append(notes, fmt.Sprintf("the shell (pid %d) had not ended when this was written", j.pid))
	case e.waitErr != nil:
		notes = append(notes, fmt.Sprintf("the exit status could not be read: %v", e.waitErr))
	case e.sig != 0 && e.reason == endExited:
		notes = append(notes, fmt.Sprintf("the shell was killed by %s (%s)", signalName(e.sig), e.sig))
		meta[metaSignal] = signalName(e.sig)
	case e.sig != 0:
		meta[metaSignal] = signalName(e.sig)
	case meaning != "":
		notes = append(notes, "exit code 1: "+meaning)
		meta[metaExitMeaning] = meaning
	case e.code != 0:
		notes = append(notes, fmt.Sprintf("exit code %d", e.code))
	}
	if len(e.alive) > 0 {
		notes = append(notes, "could not stop: "+describeProcs(e.alive)+"; they are still running")
	}
	if e.escaped {
		notes = append(notes, escapedNote)
	}
	if e.held {
		note, pgids := m.leftNote(j, e)
		notes = append(notes, note)
		if len(pgids) > 0 {
			meta[metaLeftRunning] = pgids
		}
	}
	if e.unread && !e.held && !e.escaped {
		notes = append(notes, "the last of the output had not been read when this was written; this machine is very slow just now, or the disk the output file is on has stopped taking it")
	}
	if cut && spoolErr != nil {
		notes = append(notes, fmt.Sprintf("the output file stopped at %s: %v", byteSize(spooled), spoolErr))
	}

	// Where the session is now.
	ownExit := e.reason == endExited && e.reaped && e.sig == 0
	switch where := place(e.finalDir, env); {
	case ownExit && e.finalDir == "":
		notes = append(notes, "the shell did not say where it ended (the command replaced it with exec, or set an EXIT trap of its own), so the working directory is still "+env.Cwd)
	case ownExit && where.to != "":
		res.Cwd = where.to
		notes = append(notes, "working directory is now "+where.to)
	case ownExit && where.outside != "":
		notes = append(notes, fmt.Sprintf("working directory reset to %s: %s is outside the project, so the session does not follow the command there", env.Cwd, where.outside))
	case !ownExit && (where.to != "" || where.outside != ""):
		notes = append(notes, "the command did not end by itself, so its change of directory is not kept: the working directory is still "+env.Cwd)
	}

	// The output file is kept only when it holds something the model was
	// not shown, or is still being written to.
	if cut || e.held {
		meta[metaOutputFile] = j.out.path
		meta[metaOutputFileSize] = spooled
		m.retain(j.out.path, spooled)
	} else {
		os.Remove(j.out.path)
	}

	text := compose(headline, body, notes)
	res.Content = []core.Block{core.Text(text)}
	res.Display = &core.Display{Kind: "command", Title: a.summary(), Text: text}
	return res
}

// escapedNote is the news that stopping a command did not reach everything
// it had started.
const escapedNote = "a process the command started had moved to a session of its own, where stopping the command does not reach; it still held the command's output open, is probably still running, and is no longer listened to"

// leftNote words the news that an ended command left something running
// that still holds its output, and returns the process groups it found.
func (m *Manager) leftNote(j *job, e *ending) (string, []int) {
	groups, pgids := describeGroups(e.left)
	if groups == "" {
		// The session could not be listed, or holds nothing: ask after the
		// shell's own group, which is where such a process usually is.
		if err := m.signal(-j.pid, 0); err == nil {
			groups, pgids = fmt.Sprintf("process group %d", j.pid), []int{j.pid}
		}
	}
	if groups == "" {
		if e.swept {
			return "the command ended, but a process it started, which has since left its session, still held its output open; that process was not stopped, cannot be named, and is no longer listened to", nil
		}
		return "the command ended, but a process it started, which has since left its session, still holds its output open; that process was not stopped and cannot be named", nil
	}
	if e.swept {
		// It was stopped, so it is not reported as running.
		return fmt.Sprintf("the command ended but left a process running that still held its output open: %s. Too many processes left like that are already being listened to, so this one was stopped. For a process that should keep running, use %s.",
			groups, argBackground), nil
	}
	targets := make([]string, len(pgids))
	for i, pgid := range pgids {
		targets[i] = fmt.Sprintf("-%d", pgid)
	}
	return fmt.Sprintf("the command ended but left a process running that still holds its output open: %s. What that prints from now on goes only to %s. It will be stopped when the session ends; to stop it now: kill -- %s",
		groups, j.out.path, strings.Join(targets, " ")), pgids
}

// startedResult is the result of a command started in the background.
func (m *Manager) startedResult(j *job, a bashArgs) core.ToolResult {
	id := j.taskID()
	var b strings.Builder
	fmt.Fprintf(&b, "Started in the background as %s (process group %d).\n", id, j.pid)
	b.WriteString(m.backgroundHelp(j, a.limit))
	text := b.String()
	return core.ToolResult{
		Content: []core.Block{core.Text(text)},
		Display: &core.Display{Kind: "command", Title: a.summary(), Text: text},
		Meta: map[string]any{
			metaTaskID:     id,
			metaOutputFile: j.out.path,
			metaDir:        j.dir,
		},
	}
}

// backgroundHelp says where a background command's output is, how to read
// it and stop it, and whether its end will be announced.
func (m *Manager) backgroundHelp(j *job, limit time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Its output is collected in %s.", j.out.path)
	if m.opt.NoOutputTool {
		fmt.Fprintf(&b, " Read that file to see it; %s stops the command.", StopName)
	} else {
		fmt.Fprintf(&b, " %s returns what it has printed since you last asked; %s stops it.", OutputName, StopName)
	}
	if limit > 0 {
		fmt.Fprintf(&b, " It is stopped if it is still running after %s.", seconds(limit))
	}
	if m.opt.OnEnd != nil {
		b.WriteString(" You will be told when it ends, so there is no need to wait for it or to keep checking.")
	}
	return b.String()
}

// movedResult is the result of a foreground command that was moved to the
// background, where it is still running. timedOut says whether its time
// ran out; otherwise the person asked for the move.
func (m *Manager) movedResult(j *job, a bashArgs, timedOut bool) core.ToolResult {
	id := j.taskID()
	// From here on TaskOutput returns only what the model has not been
	// shown in this result.
	j.readMu.Lock()
	body, cut, total, _ := j.out.view(m.opt.MaxOutputChars)
	j.offset = total
	j.readMu.Unlock()

	meta := map[string]any{
		metaTaskID:      id,
		metaOutputFile:  j.out.path,
		metaDir:         j.dir,
		metaOutputBytes: total,
		metaTruncated:   cut,
	}
	headline := fmt.Sprintf("moved to the background as %s at the person's request, where it is still running; what it has printed so far is below", id)
	if timedOut {
		headline = fmt.Sprintf("still running after %s, so it was moved to the background as %s; what it has printed so far is below", seconds(a.limit), id)
		meta[metaTimedOutAfter] = a.limit.Milliseconds()
	}
	notes := []string{
		m.backgroundHelp(j, m.opt.BackgroundTimeout),
		"a change of directory it makes will not carry over: the working directory is still " + j.dir,
	}
	text := compose(headline, body, notes)
	return core.ToolResult{
		Content: []core.Block{core.Text(text)},
		Display: &core.Display{Kind: "command", Title: a.summary(), Text: text},
		Meta:    meta,
	}
}

// compose lays out a result: a line about what happened to the command,
// if there is one; the output; then the notes, each in brackets so that
// they cannot be taken for output.
func compose(headline, body string, notes []string) string {
	var b strings.Builder
	if headline != "" {
		b.WriteString("[" + headline + "]\n")
	}
	if body = strings.TrimRight(body, "\n"); body == "" {
		b.WriteString("(no output)")
	} else {
		b.WriteString(body)
	}
	if len(notes) > 0 {
		b.WriteByte('\n')
	}
	for _, note := range notes {
		b.WriteString("\n[" + note + "]")
	}
	return b.String()
}

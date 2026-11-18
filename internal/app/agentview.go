package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gnosh-ai/gnosh/internal/cli"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// This file is what gnosh agents shows and does on a terminal: the agent
// view, as claude agents opens Claude Code's
// (code.claude.com/docs/en/agent-view). The interface draws it
// (ui.NewAgentView) and asks for everything through AgentSource. Off a
// terminal, and with --json, gnosh agents prints the list (bgcmd.go).

// AgentSource is what the agent view asks of gnosh.
type AgentSource interface {
	// Runs are the runs in the background as they stand now: the working
	// ones first, then those that are over, the newest first in each.
	Runs() []AgentRun
	// Conversation is the conversation of a run's session so far, as the
	// transcript view shows it.
	Conversation(id string) []TranscriptEntry
	// Stop asks a working run to stop, as gnosh stop does, and waits for
	// it to end. It says what came of it.
	Stop(ctx context.Context, id string) (string, error)
	// Forget takes a run that is over off the list: its log goes, and its
	// session stays.
	Forget(id string) error
	// Dispatch starts a new run on the prompt, as gnosh --bg does, in the
	// folder the view was opened in, and returns its session's id.
	Dispatch(ctx context.Context, prompt string) (string, error)
	// Open opens a run's session as gnosh -r does, in its folder and on
	// this terminal, and returns when the person leaves it. The view has
	// given the terminal up meanwhile.
	Open(id string) error
	// Resume sends a run that is over back to work on the prompt, as gnosh
	// --bg -r does: its session goes on, in its folder, as a run again.
	Resume(ctx context.Context, id, prompt string) error
	// Tell sends words to a working run, which it reads between its steps
	// as the person's (runasks.go); Unread takes back what a run that has
	// ended did not read of them.
	Tell(id, text string) error
	Unread(id string) []string
	// Answer answers the question a run waits on (AgentRun.Question):
	// question is its ID, choice the key of the answer chosen, and text
	// words that go with it ("no" with words lets the model carry on).
	Answer(id, question, choice, text string) error
}

// AgentQuestion is a permission question a run in the background waits on
// for a person to answer in the agent view (runasks.go).
type AgentQuestion struct {
	ID, Title, Text string
	Choices         []QuestionChoice
	Since           time.Time
}

// QuestionChoice is an answer a question offers: Key is what is answered,
// Label what the person reads.
type QuestionChoice struct{ Key, Label string }

// AgentRun is a run in the background as the agent view lists it.
type AgentRun struct {
	// ID is its session's id, Short the start of it that names the run
	// (runShort), and Name the name it was given, or "".
	ID, Short, Name string
	// State is "working", "blocked" (a question waits: Question), "done"
	// or "stopped".
	State string
	// Question is the permission question a blocked run waits on.
	Question *AgentQuestion
	// Cwd is the folder it works in, and Folder the same with ~ for the
	// home folder.
	Cwd, Folder string
	// Prompt is what it was started on.
	Prompt string
	// Output is the end of what it printed, its empty lines left out: for
	// a run that is over, its answer.
	Output []string
	// Started is when it started, and Ended, for a run that is over, when
	// its log was last written to.
	Started, Ended time.Time
}

// AgentViewInfo is what the agent view is opened with.
type AgentViewInfo struct {
	Source AgentSource
	// Cwd is the folder a run started from the view works in, and Home
	// the person's home folder, for showing paths.
	Cwd, Home string
	// Scoped says only the runs of Cwd and the folders in it are listed
	// (--cwd), and All that every run whose log is kept is (--all).
	Scoped, All bool
	// Flags are what a run started from the view is given besides its
	// prompt: the permission mode, model and effort gnosh agents was
	// given for them, as Claude Code's dispatch defaults.
	Flags []string
	// Notify is how the view tells the person that a run needs input or
	// is done: the preferredNotifChannel option ("auto" when not set).
	Notify string
}

// agentView opens the agent view for gnosh agents.
func (p *program) agentView(st *startup, inv cli.Invocation) int {
	cwd, home := st.cfg.Paths.Cwd, st.cfg.Paths.Home
	var flags []string
	for _, f := range [][2]string{{"--permission-mode", inv.PermissionMode}, {"--model", inv.Model}, {"--effort", inv.Effort}} {
		if f[1] != "" {
			flags = append(flags, f[0]+"="+f[1])
		}
	}
	return p.o.AgentView(AgentViewInfo{
		Source: &runSource{p: p, dirs: st.sessions().ProjectDirs(), cwd: cwd, home: home, scoped: inv.Cwd != "", all: inv.All, flags: flags},
		Cwd:    cwd, Home: home, Scoped: inv.Cwd != "", All: inv.All, Flags: flags,
		Notify: firstOf(st.cfg.Options.PreferredNotifChannel.V, "auto"),
	})
}

// runSource is the AgentSource of gnosh agents: the runs whose logs are in
// dirs, the folders of every project's sessions.
type runSource struct {
	p      *program
	dirs   []string
	cwd    string
	home   string
	scoped bool
	all    bool
	// flags go to every run Dispatch starts.
	flags []string

	mu sync.Mutex
	// runs are the runs as last listed, by session id, and convs what was
	// read of their sessions' files, by file.
	runs  map[string]backgroundRun
	convs map[string]*convCache
}

// convCache is what was read of a session's file: the entries up to the
// end of its last whole line, which is size bytes in.
type convCache struct {
	size    int64
	entries []transcript.Entry
}

func (s *runSource) Runs() []AgentRun {
	since := s.p.now().Add(-runsKept)
	if s.all {
		since = time.Time{}
	}
	listed := map[string]backgroundRun{}
	var working, over []AgentRun
	for _, r := range runsIn(s.dirs, since) {
		if s.scoped && !within(r.cwd, s.cwd) {
			continue
		}
		listed[r.session] = r
		a := AgentRun{ID: r.session, Short: runShort(r.session), Name: r.name, State: runState(r),
			Cwd: r.cwd, Folder: homeTilde(s.home, r.cwd), Prompt: r.prompt, Output: r.output, Started: r.started}
		if q := r.asking; q != nil {
			a.Question = &AgentQuestion{ID: q.ID, Title: q.Title, Text: q.Text, Since: q.Since}
			for _, c := range q.Choices {
				a.Question.Choices = append(a.Question.Choices, QuestionChoice{Key: c.Key, Label: c.Label})
			}
		}
		if r.running {
			working = append(working, a)
			continue
		}
		if info, err := os.Stat(r.log); err == nil {
			a.Ended = info.ModTime()
		}
		over = append(over, a)
	}
	s.mu.Lock()
	s.runs = listed
	for path := range s.convs {
		kept := false
		for _, r := range listed {
			kept = kept || r.file == path
		}
		if !kept {
			delete(s.convs, path)
		}
	}
	s.mu.Unlock()
	return append(working, over...)
}

// run is the run of the session id as last listed.
func (s *runSource) run(id string) (backgroundRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return r, fmt.Errorf("the session %s is not among the runs in the background now", id)
	}
	return r, nil
}

func (s *runSource) Conversation(id string) []TranscriptEntry {
	r, err := s.run(id)
	if err != nil || r.file == "" {
		return nil
	}
	return transcriptEntriesOf(s.read(r.file))
}

// read is what the session's file at path holds: what was read of it
// before, and the whole lines written to it since. A file that got
// shorter is read again from its start.
func (s *runSource) read(path string) []transcript.Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.convs == nil {
		s.convs = map[string]*convCache{}
	}
	c := s.convs[path]
	if c == nil {
		c = &convCache{}
		s.convs[path] = c
	}
	f, err := os.Open(path)
	if err != nil {
		return c.entries
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return c.entries
	}
	if info.Size() < c.size {
		*c = convCache{}
	}
	if info.Size() == c.size {
		return c.entries
	}
	if _, err := f.Seek(c.size, io.SeekStart); err != nil {
		return c.entries
	}
	data, _ := io.ReadAll(io.LimitReader(f, info.Size()-c.size))
	end := bytes.LastIndexByte(data, '\n')
	if end < 0 {
		return c.entries
	}
	data = data[:end+1]
	for e, err := range transcript.Read(context.Background(), bytes.NewReader(data), nil) {
		if err != nil {
			break
		}
		c.entries = append(c.entries, e)
	}
	c.size += int64(len(data))
	return c.entries
}

func (s *runSource) Stop(ctx context.Context, id string) (string, error) {
	r, err := s.run(id)
	if err != nil {
		return "", err
	}
	proc, said, err := askStop(id, r.log)
	if err != nil || proc == nil {
		return said, err
	}
	return awaitStop(ctx, s.p.now, id, r.log, proc)
}

func (s *runSource) Answer(id, question, choice, text string) error {
	r, err := s.run(id)
	if err != nil {
		return err
	}
	return answerAsk(r.log, question, choice, text)
}

func (s *runSource) Forget(id string) error {
	r, err := s.run(id)
	if err != nil {
		return err
	}
	// A session sent off from two folders has a log in each.
	var logs []string
	for _, dir := range s.dirs {
		if log := filepath.Join(dir, id+".background.log"); fileExists(log) {
			logs = append(logs, log)
		}
	}
	for _, log := range logs {
		if runAlive(id, log, firstOf(r.file, filepath.Join(filepath.Dir(log), id+".jsonl"))) {
			return fmt.Errorf("the session %s is working: stop it first", id)
		}
	}
	for _, log := range logs {
		if err := os.Remove(log); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		_ = os.Remove(pidFile(log))
	}
	return nil
}

// bgStarted begins the line gnosh --bg prints of the run it started, its
// session's id after it.
const bgStarted = "gnosh: backgrounded · "

func (s *runSource) Dispatch(ctx context.Context, prompt string) (string, error) {
	return s.background(ctx, s.cwd, nil, prompt)
}

func (s *runSource) Tell(id, text string) error {
	r, err := s.run(id)
	if err != nil {
		return err
	}
	return tellRun(r.log, text, s.p.now())
}

func (s *runSource) Unread(id string) []string {
	r, err := s.run(id)
	if err != nil {
		return nil
	}
	return unreadWords(r.log)
}

func (s *runSource) Resume(ctx context.Context, id, prompt string) error {
	r, err := s.run(id)
	if err != nil {
		return err
	}
	dir := s.cwd
	if info, err := os.Stat(r.cwd); err == nil && info.IsDir() {
		dir = r.cwd
	}
	_, err = s.background(ctx, dir, []string{"--resume=" + id}, prompt)
	return err
}

// background runs gnosh --bg in dir, with the view's flags, more, and the
// prompt, and returns the id of the session it sent to the background.
func (s *runSource) background(ctx context.Context, dir string, more []string, prompt string) (string, error) {
	if s.p.o.Executable == "" {
		return "", errors.New("gnosh does not know its own program file, so it cannot start a run")
	}
	args := append(append(append([]string{"--bg"}, s.flags...), more...), "--", prompt)
	cmd := exec.CommandContext(ctx, s.p.o.Executable, args...)
	cmd.Dir, cmd.Env = dir, s.p.o.Environ
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	for _, line := range strings.Split(out.String(), "\n") {
		if rest, ok := strings.CutPrefix(line, bgStarted); ok {
			id, _, _ := strings.Cut(rest, " ")
			return id, nil
		}
	}
	return "", errors.New(childSaid(errs.String(), err, "it printed no session"))
}

// childSaid is why a gnosh started as a child failed: the last thing it
// printed where errors go, or else how it ended, or else none.
func childSaid(stderr string, err error, none string) string {
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if said := strings.TrimSpace(lines[len(lines)-1]); said != "" {
		return strings.TrimSuffix(strings.TrimPrefix(said, "gnosh: "), ".")
	}
	if err != nil {
		return err.Error()
	}
	return none
}

func (s *runSource) Open(id string) error {
	if s.p.o.Executable == "" {
		return errors.New("gnosh does not know its own program file, so it cannot open a session")
	}
	dir := s.cwd
	if r, err := s.run(id); err == nil && r.cwd != "" {
		if info, err := os.Stat(r.cwd); err == nil && info.IsDir() {
			dir = r.cwd
		}
	}
	cmd := exec.Command(s.p.o.Executable, "-r", id)
	cmd.Dir, cmd.Env = dir, s.p.o.Environ
	// Where errors go is read as well, for what to say when it fails.
	var errs tailWriter
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.p.o.Stdin, s.p.o.Stdout, io.MultiWriter(s.p.o.Stderr, &errs)
	if err := cmd.Run(); err != nil {
		return errors.New(childSaid(string(errs.b), err, err.Error()))
	}
	return nil
}

// tailWriter keeps the last tailKept bytes written to it.
type tailWriter struct{ b []byte }

const tailKept = 4 << 10

func (t *tailWriter) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if over := len(t.b) - tailKept; over > 0 {
		t.b = append(t.b[:0], t.b[over:]...)
	}
	return len(p), nil
}

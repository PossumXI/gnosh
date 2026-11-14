package shelltool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// Manager runs the shell commands of one session. It knows every process
// it has started, so that none is left behind: Close stops them all.
// Make one with New. Its methods and its tools may be used from several
// goroutines at once.
type Manager struct {
	opt   Options // with every default filled in
	shell string  // the shell that runs commands

	// The machines a call may name as its host, and this one as results
	// write it.
	hosts     map[string]*Host // the declared hosts by name
	hostNames []string         // their names, in the order declared
	local     string           // who and where this machine is: "dev@box"

	// The two ways the manager touches other processes. They are fields so
	// that a test can stand in for them.
	signal func(pid int, sig syscall.Signal) error
	procs  func(sid int) ([]procInfo, error)

	notes *notifier // tells Options.OnEnd about background commands that ended

	// maxLingering is how many ended commands may have a process they left
	// behind listened to at once. Each costs a goroutine and two open
	// files, so there has to be an end to it.
	maxLingering int

	// pumpDelay holds back the reading of every command's output. It is
	// zero outside tests, which use it to stand for a very busy machine.
	pumpDelay time.Duration

	mu        sync.Mutex
	closed    bool
	dir       string              // the manager's own directory for output files; made when first needed
	seq       int                 // how many commands have been let in
	bgSeq     int                 // how many background ids have been given out
	bgRunning int                 // how many background commands have not ended
	running   map[*job]struct{}   // commands whose end is not final yet
	lingering map[*job][]procInfo // ended commands whose output is still held open, with the processes they left
	tasks     map[string]*job     // background commands by id; ended ones stay
	order     []string            // the ids in tasks, oldest first
	calls     map[string]*job     // foreground commands by the id of the call that waits for them
	kept      []keptFile          // output files of finished foreground commands, oldest first
	keptBytes int64               // how much disk those files take

	wg sync.WaitGroup // every goroutine the manager has started

	closeOnce sync.Once
	closeErr  error
}

// keptFile is the output file of a finished foreground command.
type keptFile struct {
	path string
	size int64
}

// errClosed is returned for anything asked of a closed manager.
var errClosed = errors.New("shelltool: the manager is closed")

// refusal is a failure the model can do something about. It reaches the
// model as the text of a failed tool result; any other error from starting
// a command is the harness's own trouble and goes back as a Go error.
type refusal string

func (r refusal) Error() string { return string(r) }

// New makes a manager. It starts nothing and touches no files until the
// first command runs.
func New(opts Options) *Manager {
	m := &Manager{
		opt:    opts.withDefaults(),
		signal: syscall.Kill,
		procs:  sessionProcs,

		maxLingering: defaultMaxLingering,
		running:      make(map[*job]struct{}),
		lingering:    make(map[*job][]procInfo),
		tasks:        make(map[string]*job),
		calls:        make(map[string]*job),
	}
	m.shell = m.opt.Shell
	if m.shell == "" {
		m.shell = "/bin/sh"
		if st, err := os.Stat("/bin/bash"); err == nil && st.Mode().IsRegular() {
			m.shell = "/bin/bash"
		}
	}
	m.local = m.opt.Local
	if m.local == "" {
		m.local = whoAndWhere()
	}
	m.hosts = make(map[string]*Host, len(m.opt.Hosts))
	for _, h := range m.opt.Hosts {
		// The name of this machine is never a host's, and a name stands
		// for one machine: the config refuses both, and here the first
		// declaration of a name is the one that counts.
		if _, twice := m.hosts[h.Name]; h.Name == "" || h.Name == LocalName || twice {
			continue
		}
		m.hosts[h.Name] = &h
		m.hostNames = append(m.hostNames, h.Name)
	}
	m.notes = newNotifier(m.opt.OnEnd, &m.wg)
	return m
}

// PersonsBash returns the Bash tool for a command the person types into the
// session themselves. It runs the command exactly as Bash does, with the
// same shell, limits and line at the head of the result, and without the
// rules and notes that are there to steer a model: a person may type ssh.
func (m *Manager) PersonsBash() core.Tool {
	return bashTool{m: m, person: true}
}

// Tools returns the tools the manager offers a model: Bash, TaskOutput
// (unless Options.NoOutputTool) and TaskStop. Each is also a
// core.Describer.
func (m *Manager) Tools() []core.Tool {
	tools := []core.Tool{bashTool{m: m}}
	if !m.opt.NoOutputTool {
		tools = append(tools, outputTool{m})
	}
	return append(tools, stopTool{m})
}

// Tasks lists the session's background commands, oldest first, ended ones
// included.
func (m *Manager) Tasks() []Task {
	m.mu.Lock()
	jobs := make([]*job, 0, len(m.order))
	for _, id := range m.order {
		jobs = append(jobs, m.tasks[id])
	}
	m.mu.Unlock()
	now := m.opt.Now()
	out := make([]Task, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, j.task(now))
	}
	return out
}

// Stop stops the background command with the given id, as the TaskStop
// tool does, and returns it as it then is. It is for the person's own
// controls. Stopping a command that has already ended is not an error.
func (m *Manager) Stop(ctx context.Context, id string) (Task, error) {
	j := m.find(id)
	if j == nil {
		return Task{}, fmt.Errorf("shelltool: there is no background command %q", id)
	}
	if err := j.stopAndWait(ctx); err != nil {
		return j.task(m.opt.Now()), err
	}
	return j.task(m.opt.Now()), nil
}

// Background moves the foreground command that the tool call callID is
// waiting for to the background, where it keeps running; the call returns
// at once with the command's task id. It is for the person's own controls.
// It reports whether there was such a command to ask. A command on another
// machine is not one: it runs for as long as its call holds the connection,
// and stays in the foreground.
func (m *Manager) Background(callID string) bool {
	m.mu.Lock()
	j := m.calls[callID]
	m.mu.Unlock()
	if j == nil || j.host != nil {
		return false
	}
	select {
	case j.move <- struct{}{}:
	default:
	}
	return true
}

// Close stops every command the manager started and is still running,
// foreground and background, with everything those commands started; kills
// what a finished command left behind if that still holds the command's
// output open; waits for all of it to be gone; and removes the output
// files. Tool calls in flight return, saying that their command was
// stopped. After Close the manager runs nothing more. Close may be called
// more than once. Its error says what could not be stopped or removed.
func (m *Manager) Close() error {
	m.closeOnce.Do(func() { m.closeErr = m.close() })
	return m.closeErr
}

func (m *Manager) close() error {
	m.mu.Lock()
	m.closed = true
	running := make([]*job, 0, len(m.running))
	for j := range m.running {
		running = append(running, j)
	}
	m.mu.Unlock()

	// Every command is stopped at the same time; each takes at most its
	// graces. The wait below is longer than any of them can take, and is
	// there so that even a fault here cannot make Close hang.
	for _, j := range running {
		j.ask(endClosed)
	}
	var problems []string
	patience := time.NewTimer(m.opt.TermGrace + m.opt.KillWait + 2*m.opt.PipeGrace + drainWait + 5*time.Second)
	defer patience.Stop()
	late, expired := 0, false
	for _, j := range running {
		if !expired {
			select {
			case <-j.done:
				continue
			case <-patience.C:
				expired = true
			}
		}
		select {
		case <-j.done:
		default:
			late++
		}
	}
	if late > 0 {
		problems = append(problems, fmt.Sprintf("%s did not stop in time", plural(int64(late), "command")))
	}
	for _, j := range running {
		if e := j.ending(); e != nil && len(e.alive) > 0 {
			problems = append(problems, fmt.Sprintf("%s could not be stopped: %s", j.label(), describeProcs(e.alive)))
		}
	}

	// What finished commands left behind, still holding their output.
	m.mu.Lock()
	left := make(map[*job][]procInfo, len(m.lingering))
	for j, procs := range m.lingering {
		left[j] = procs
	}
	m.mu.Unlock()
	var sweeps sync.WaitGroup
	for j, procs := range left {
		sweeps.Go(func() {
			m.sweep(j.pid, procs)
			j.pipe.Close()
			waitFor(j.pumped, m.opt.PipeGrace+time.Second)
		})
	}
	sweeps.Wait()

	m.notes.close()

	// Every goroutine should be done by now. One that is not is waiting on
	// a process that would not die, or on a callback that blocks.
	idle := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(idle)
	}()
	if !waitFor(idle, 2*time.Second) {
		problems = append(problems, "some of the manager's goroutines are still waiting on a process or a callback")
	}

	m.mu.Lock()
	dir := m.dir
	m.mu.Unlock()
	if dir != "" {
		if err := os.RemoveAll(dir); err != nil {
			problems = append(problems, fmt.Sprintf("removing the output files in %s: %v", dir, err))
		}
	}
	if len(problems) > 0 {
		return errors.New("shelltool: closing: " + strings.Join(problems, "; "))
	}
	return nil
}

// spoolDir returns the manager's directory for output files, making it if
// it is not there.
func (m *Manager) spoolDir() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", errClosed
	}
	if m.dir != "" {
		// A cleaner of temporary files may have taken it.
		if err := os.MkdirAll(m.dir, 0o700); err != nil {
			return "", fmt.Errorf("shelltool: restoring the directory for output files: %w", err)
		}
		return m.dir, nil
	}
	parent := m.opt.SpoolDir
	if parent == "" {
		parent = os.TempDir()
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", fmt.Errorf("shelltool: making the directory for output files: %w", err)
	}
	dir, err := os.MkdirTemp(parent, "gnosh-shell-")
	if err != nil {
		return "", fmt.Errorf("shelltool: making the directory for output files: %w", err)
	}
	m.dir = dir
	return dir, nil
}

// admit lets a new command in: it names its files, gives a background
// command its id, and puts it among the running ones, so that Close will
// find it from this moment on. callID is the tool call that will wait for
// a foreground command. A background command is not yet listed among the
// tasks: it has not started, and nothing may ask about it until it has
// (see publish).
func (m *Manager) admit(j *job, background bool, callID string) (name string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return "", errClosed
	}
	if background && m.bgRunning >= m.opt.MaxBackground {
		return "", refusal(fmt.Sprintf("%d background commands are already running, which is the most allowed at once. Stop one with %s first, or run this command in the foreground.", m.bgRunning, StopName))
	}
	m.seq++
	name = fmt.Sprintf("cmd-%d", m.seq)
	if background {
		name = m.nextTaskID(j)
	} else if callID != "" {
		j.callID = callID
		m.calls[callID] = j
	}
	m.running[j] = struct{}{}
	return name, nil
}

// nextTaskID makes j a background command by giving it the next id, which
// it returns. It must be called with mu held.
func (m *Manager) nextTaskID(j *job) string {
	m.bgSeq++
	m.bgRunning++
	id := fmt.Sprintf("bg%d", m.bgSeq)
	j.setTaskID(id)
	return id
}

// publish lists a background command that has started among the tasks,
// where the tools and Tasks find it by its id.
func (m *Manager) publish(j *job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.list(j)
}

// list adds a started background command to the tasks. It must be called
// with mu held.
func (m *Manager) list(j *job) {
	id := j.taskID()
	m.tasks[id] = j
	m.order = append(m.order, id)
}

// adopt turns a running foreground command into a background one. It
// reports false, and changes nothing, when the manager is closed or already
// has as many background commands as it may.
func (m *Manager) adopt(j *job) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.bgRunning >= m.opt.MaxBackground {
		return false
	}
	delete(m.calls, j.callID)
	m.nextTaskID(j)
	m.list(j)
	return true
}

// unlist forgets which call waits for a foreground command, once that call
// is about to return.
func (m *Manager) unlist(j *job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls[j.callID] == j {
		delete(m.calls, j.callID)
	}
}

// linger records that an ended command left processes behind, left, that
// still hold its output open, so that Close can deal with them. The command
// stays on that list until its output closes (see settle). It reports false
// when the manager is already closing, or is listening to as many such
// commands as it will; the caller then deals with this one at once.
func (m *Manager) linger(j *job, left []procInfo) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || len(m.lingering) >= m.maxLingering {
		return false
	}
	if !closed(j.pumped) {
		// Otherwise the output closed in the meantime, and nothing is left
		// to deal with.
		m.lingering[j] = left
	}
	return true
}

// settle is called when a command's output has closed for good: nothing of
// it is left behind that the manager need remember.
func (m *Manager) settle(j *job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.lingering, j)
}

// release takes a command that has ended off the list of running ones. It
// is called once for each command that started.
func (m *Manager) release(j *job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, j)
	if j.taskID() != "" {
		m.bgRunning--
	}
}

// forget removes a command that never started.
func (m *Manager) forget(j *job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, j)
	if m.calls[j.callID] == j {
		delete(m.calls, j.callID)
	}
	if j.taskID() != "" {
		m.bgRunning--
	}
}

// find returns the background command with the given id, or nil.
func (m *Manager) find(id string) *job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tasks[id]
}

// retain notes the output file of a finished foreground command as kept,
// and removes the oldest kept files while together they take more disk
// than the budget allows. The newest is never removed.
func (m *Manager) retain(path string, size int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kept = append(m.kept, keptFile{path, size})
	m.keptBytes += size
	for m.opt.SpoolBudget >= 0 && m.keptBytes > m.opt.SpoolBudget && len(m.kept) > 1 {
		old := m.kept[0]
		m.kept = m.kept[1:]
		m.keptBytes -= old.size
		os.Remove(old.path)
	}
}

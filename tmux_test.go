package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// pane is a window of a tmux server of a test's own, in which the real
// gnosh runs as a person would run it. tmux is a real terminal emulator:
// gnosh takes its window over as it would any terminal, and what the
// window shows is what a person would see.
type pane struct {
	t    *testing.T
	tmux func(args ...string) string
}

// quoted is a word as a shell takes it literally.
func quoted(word string) string { return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'" }

// openTmux starts a tmux server of the test's own with one window, in the
// world's folder, that runs a command line. The test is skipped unless
// GNOSH_UI_TMUX=1 and tmux is installed, as the interface's own tests in
// tmux are.
func openTmux(t *testing.T, w *apptest.World, command string) *pane {
	t.Helper()
	if os.Getenv("GNOSH_UI_TMUX") != "1" {
		t.Skip("set GNOSH_UI_TMUX=1 to try gnosh in tmux")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	socket := fmt.Sprintf("gnosh-main-%d", os.Getpid())
	p := &pane{t: t}
	p.tmux = func(args ...string) string {
		cmd := exec.Command("tmux", append([]string{"-L", socket, "-f", "/dev/null"}, args...)...)
		cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8", "TERM=xterm-256color")
		out, _ := cmd.CombinedOutput()
		return strings.TrimRight(string(out), "\n")
	}
	t.Cleanup(func() { p.tmux("kill-server") })
	p.tmux("new-session", "-d", "-s", "main", "-x", "100", "-y", "30", "-c", w.Dir, command)
	return p
}

// gnoshLine is the command line that runs gnosh with the world's
// environment and nothing else but a TERM the full interface draws on.
func gnoshLine(w *apptest.World, args ...string) string {
	command := "env -i"
	for _, kv := range append(w.Env(), "TERM=tmux-256color") {
		command += " " + quoted(kv)
	}
	command += " " + quoted(binary)
	for _, arg := range args {
		command += " " + quoted(arg)
	}
	return command
}

// openPane starts gnosh in tmux as the window's own program. It runs
// under a shell that says how it ended and then stays, so that the window
// can be looked at afterwards.
func openPane(t *testing.T, w *apptest.World, args ...string) *pane {
	t.Helper()
	return openTmux(t, w, gnoshLine(w, args...)+`; echo "gnosh ended with $?"; exec sleep 3600`)
}

// openShell starts a shell with job control in tmux, as a person has one.
// Its prompt is "shell:". gnosh is started in it by typing its command
// line.
func openShell(t *testing.T, w *apptest.World) *pane {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not installed")
	}
	return openTmux(t, w, `env -i TERM="$TERM" PATH="$PATH" PS1='shell: ' bash --norc --noprofile -i`)
}

// screen is everything the window shows and has shown.
func (p *pane) screen() string { return p.tmux("capture-pane", "-p", "-t", "main", "-S", "-") }

// until waits for ok to come true.
func (p *pane) until(what string, ok func() bool) {
	p.t.Helper()
	deadline := time.Now().Add(time.Minute)
	for !ok() {
		if time.Now().After(deadline) {
			p.t.Fatalf("it never came about that %s. tmux shows:\n%s", what, p.screen())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// see waits until the window shows each piece.
func (p *pane) see(pieces ...string) {
	p.t.Helper()
	for _, piece := range pieces {
		p.until(fmt.Sprintf("tmux shows %q", piece), func() bool { return strings.Contains(p.screen(), piece) })
	}
}

// send types into the window: words as they are, then keys by tmux's
// names.
func (p *pane) send(words string, keys ...string) {
	if words != "" {
		p.tmux("send-keys", "-t", "main", "-l", words)
	}
	if len(keys) > 0 {
		p.tmux(append([]string{"send-keys", "-t", "main"}, keys...)...)
	}
}

// raw reports whether the window's terminal is in raw mode, as stty reads
// its settings, and whether that could be told at all.
func (p *pane) raw() (raw, known bool) {
	tty := p.tmux("display-message", "-p", "-t", "main", "#{pane_tty}")
	out, err := exec.Command("stty", "-F", tty, "-a").CombinedOutput()
	if err != nil {
		return false, false
	}
	return strings.Contains(string(out), "-icanon") && strings.Contains(string(out), "-echo "), true
}

// The real program in a real terminal, with the full interface: a person
// types a prompt and reads the answer, is asked before a file is written
// and says yes, runs a command of their own, moves a command of the
// model's to the background, and leaves. It runs only with
// GNOSH_UI_TMUX=1, on a machine with tmux, in a tmux server of its own.
func TestBinaryInTmux(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Thinking: "It is a sum.", Text: "The answer is 4."},
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "Write", Args: `{"file_path":"made.txt","content":"made by the model\n"}`}}},
		apptest.Reply{Text: "The file is written."},
		apptest.Reply{Text: "I see what you ran."},
	)
	// A command that runs until it is stopped, and that the person's
	// rules let the model run without a question.
	job := w.Script(w.Home+"/job.sh", `echo $$ > job.pid; exec sleep 300`)
	w.RC("permissions allow 'Bash(" + job + ")'")
	w.Model.Add(
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "Bash", Args: `{"command":"` + job + `"}`}}},
		apptest.Reply{Text: "It runs in the background now."},
	)
	p := openPane(t, w)
	show := func(when string) {
		if os.Getenv("GNOSH_UI_SHOW") != "" {
			t.Logf("%s, tmux shows:\n%s", when, p.screen())
		}
	}
	// It is the full interface, not the plain session, which would say
	// "Type a prompt and Enter."
	p.see("GNOSH", "test-model", "manual mode", "idle")
	if shown := p.screen(); strings.Contains(shown, "Type a prompt and Enter.") {
		t.Fatalf("the session is the plain one:\n%s", shown)
	}
	if raw, known := p.raw(); known && !raw {
		t.Error("the terminal is not in raw mode while gnosh has it")
	}

	p.send("what is 2+2?", "Enter")
	p.see("> what is 2+2?", "The answer is 4.", "done in")
	show("after the first answer")

	// A tool call that changes a file asks first, with the change in
	// front of the person.
	p.send("write the file", "Enter")
	p.see("Allow Write?", "made.txt", "+ made by the model", "Yes, this once")
	if w.Exists("made.txt") {
		t.Fatal("the file was written before the question was answered")
	}
	show("with the question put")
	// The question takes keys once it has been read: a moment after it
	// was drawn, with the keyboard quiet.
	for quiet := time.Now(); time.Since(quiet) < 1500*time.Millisecond; {
		time.Sleep(20 * time.Millisecond)
	}
	p.send("", "Enter")
	p.see("? Allow Write? Yes, this once", "The file is written.")
	p.until("the second turn is over", func() bool { return strings.Count(p.screen(), "done in") == 2 })
	if got := w.Read("made.txt"); got != "made by the model\n" {
		t.Fatalf("the file holds %q", got)
	}

	// A command of the person's own: it runs without a question, is
	// shown, and is put in front of the model with what it printed.
	p.send("!echo from my own $((40+2)) hands", "Enter")
	p.see("from my own 42 hands", "I see what you ran.")
	p.until("the third turn is over", func() bool { return strings.Count(p.screen(), "done in") == 3 })
	show("after a command of the person's own")
	requests := w.Model.Requests()
	if last := string(requests[len(requests)-1].Body); !strings.Contains(last, "echo from my own $((40+2)) hands") || !strings.Contains(last, "from my own 42 hands") {
		t.Errorf("the model was not handed the person's command and what it printed:\n%s", last)
	}

	// A command of the model's that runs long is moved to the background
	// with Ctrl+B: its call returns, the model goes on, and the status
	// line counts the command.
	p.send("start the long job", "Enter")
	pid := readPid(t, w, "job.pid")
	defer syscall.Kill(pid, syscall.SIGKILL) // whatever gnosh leaves behind, the test does not
	p.until("the command is in the background", func() bool {
		if strings.Contains(p.screen(), "1 in the background") {
			return true
		}
		p.send("", "C-b")
		time.Sleep(300 * time.Millisecond)
		return false
	})
	p.see("It runs in the background now.")
	p.until("the fourth turn is over", func() bool { return strings.Count(p.screen(), "done in") == 4 })
	show("with a command in the background")
	if gone(pid) {
		t.Error("the command that was moved to the background is not running any more")
	}

	// Out, as a person leaves: Ctrl+D twice in an empty box. The command
	// in the background is stopped on the way.
	p.send("", "C-d", "C-d")
	p.see("gnosh ended with ")
	waitFor(t, "the background command to be stopped", func() bool { return gone(pid) })
	end := p.screen()
	show("at the end")
	if !strings.Contains(end, "gnosh ended with 0") {
		t.Errorf("gnosh did not end with exit code 0:\n%s", end)
	}
	if !strings.Contains(end, "To resume this session: gnosh -r ") || strings.Contains(end, "manual mode") {
		t.Errorf("after leaving, tmux shows:\n%s", end)
	}
	if raw, known := p.raw(); known && raw {
		t.Error("the terminal was left in raw mode")
	}
	for _, once := range []string{"The answer is 4.", "The file is written.", "> what is 2+2?", "I see what you ran.", "It runs in the background now."} {
		if n := strings.Count(end, once); n != 1 {
			t.Errorf("%q is on the terminal %d times:\n%s", once, n, end)
		}
	}
	checkSession(t, w)
	w.Model.Done()
}

// A prompt given on the command line is the session's first prompt in the
// full interface too: it is sent as if it had been typed, and the session
// stays open afterwards. Up in an empty box reaches the prompts sent
// before, those of the folder's earlier sessions among them.
func TestBinaryInTmuxWithAPrompt(t *testing.T) {
	w := apptest.NewWorld(t,
		apptest.Reply{Text: "Noted."},
		apptest.Reply{Text: "The answer is 4."},
		apptest.Reply{Text: "And that is 6."},
	)
	gnosh(t, w, "-p", "a prompt of an earlier session").wantCode(t, 0)

	p := openPane(t, w, "what is 2+2?")
	p.see("GNOSH", "> what is 2+2?", "The answer is 4.", "done in", "idle")
	p.send("", "Up", "Up")
	p.see("> a prompt of an earlier session")
	p.send("", "C-u")
	p.send("and 3+3?", "Enter")
	p.see("> and 3+3?", "And that is 6.")
	p.until("the second turn is over", func() bool { return strings.Count(p.screen(), "done in") == 2 })
	p.send("", "C-d", "C-d")
	p.see("gnosh ended with ")
	if end := p.screen(); !strings.Contains(end, "gnosh ended with 0") || !strings.Contains(end, "To resume this session: gnosh -r ") {
		t.Errorf("after leaving, tmux shows:\n%s", end)
	}
	requests := w.Model.Requests()
	if len(requests) != 3 || !strings.Contains(string(requests[1].Body), "what is 2+2?") {
		t.Errorf("the model was sent %d requests, and the second should hold the command line's prompt", len(requests))
	}
	if sessions := w.Sessions(); len(sessions) != 2 {
		t.Errorf("session files: %v", sessions)
	}
	w.Model.Done()
}

// gnosh as a job of a shell: Ctrl+Z gives the terminal back to the shell
// and stops gnosh, with nothing of its live region left behind, and fg
// brings it back with the draft still in the box and the session whole.
func TestBinaryInTmuxSuspend(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "Four."}, apptest.Reply{Text: "Still here."})
	p := openShell(t, w)
	prompted := func() bool { return strings.HasSuffix(strings.TrimRight(p.screen(), " \n"), "shell:") }
	p.until("the shell's prompt is there", prompted)
	p.send(gnoshLine(w), "Enter")
	p.see("manual mode", "idle")
	p.send("what is 2+2?", "Enter")
	p.see("Four.", "done in")
	p.send("are you")
	p.see("> are you")

	p.send("", "C-z")
	p.see("Stopped")
	p.until("the shell's prompt is back", prompted)
	if shown := p.screen(); strings.Contains(shown, "manual mode") || strings.Contains(shown, "> are you") {
		t.Errorf("gnosh is stopped and its live region is still on the terminal:\n%s", shown)
	}
	// (The terminal's settings say nothing here: a shell that waits at
	// its prompt reads keys one at a time too. That the terminal is
	// given back exactly as it was found is checked in internal/term.)
	p.send("echo the shell has the terminal", "Enter")
	p.until("the shell ran a command", func() bool { return strings.Count(p.screen(), "the shell has the terminal") == 2 && prompted() })

	p.send("fg", "Enter")
	p.see("manual mode", "> are you")
	p.send(" still there?", "Enter")
	p.see("> are you still there?", "Still here.")
	p.until("the second turn is over", func() bool { return strings.Count(p.screen(), "done in") == 2 })
	shown := p.screen()
	if os.Getenv("GNOSH_UI_SHOW") != "" {
		t.Logf("after Ctrl+Z and fg, tmux shows:\n%s", shown)
	}
	for _, once := range []string{"Four.", "> what is 2+2?", "manual mode", "Still here."} {
		if n := strings.Count(shown, once); n != 1 {
			t.Errorf("%q is on the terminal %d times after gnosh was brought back:\n%s", once, n, shown)
		}
	}

	p.send("", "C-d", "C-d")
	p.see("To resume this session: gnosh -r ")
	p.until("the shell's prompt is back", prompted)
	p.send("echo gnosh left with $?", "Enter")
	p.see("gnosh left with 0")
	checkSession(t, w)
	w.Model.Done()
}

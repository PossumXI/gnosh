package term

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	xterm "golang.org/x/term"

	"github.com/gnosh-ai/gnosh/internal/term/vtest"
)

// TestMain lets the test binary stand in for a program with a renderer
// when a test starts it inside a real terminal (see TestRendererInTmux).
func TestMain(m *testing.M) {
	if helper, ok := helpers[os.Getenv("GNOSH_TERM_HELPER")]; ok {
		helper()
		return
	}
	os.Exit(m.Run())
}

// helpers are the programs the test binary can stand in for, by the name
// given in GNOSH_TERM_HELPER.
var helpers = map[string]func(){"renderer": rendererHelper}

// rendererHelper reads commands from standard input, one a line, and
// carries them out with a Renderer on standard output:
//
//	C text           commit one line
//	L row col a|b|c  set the live region and the cursor
//	R cols rows      wait for the terminal to be that size, then resize
//	F on|off         say whether the terminal wraps lines again
//	X                clear the live region and exit
//
// After each command it adds a line to the file named by
// GNOSH_TERM_HELPER_LOG, so the test knows the command is done.
func rendererHelper() {
	log, err := os.OpenFile(os.Getenv("GNOSH_TERM_HELPER_LOG"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cols, rows, _ := xterm.GetSize(int(os.Stdout.Fd()))
	r := NewRenderer(teeWriter{log}, cols, rows, vtest.Width)
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		cmd, arg, _ := strings.Cut(in.Text(), " ")
		switch cmd {
		case "C":
			r.Commit(arg)
		case "L":
			f := strings.SplitN(arg, " ", 3)
			row, _ := strconv.Atoi(f[0])
			col, _ := strconv.Atoi(f[1])
			var lines []string
			if len(f) == 3 && f[2] != "" {
				lines = strings.Split(f[2], "|")
			}
			r.SetLive(lines, row, col)
		case "R":
			// tmux tells the pane its new size a little after it has
			// rearranged the screen.
			var w, h int
			fmt.Sscan(arg, &w, &h)
			for i := 0; i < 1000; i++ {
				if cols, rows, _ = xterm.GetSize(int(os.Stdout.Fd())); cols == w && rows == h {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			r.Resize(cols, rows)
		case "F":
			r.SetReflow(arg == "on")
		case "X":
			r.Clear()
			fmt.Fprintln(log, "done")
			return
		}
		fmt.Fprintln(log, "ok", cmd)
	}
}

// teeWriter writes to standard output and notes each write in the log.
type teeWriter struct{ log *os.File }

func (w teeWriter) Write(p []byte) (int, error) {
	fmt.Fprintf(w.log, "wrote %q\n", p)
	return os.Stdout.Write(p)
}

// TestRendererInTmux runs the renderer inside tmux, a real terminal
// emulator, resizes the terminal under it, and checks what tmux shows. It
// runs only when GNOSH_TMUX=1, on a machine with tmux, and uses a tmux
// server of its own.
//
// tmux wraps lines again when a pane is resized and keeps the cursor on
// its character, which is what the renderer assumes by default.
func TestRendererInTmux(t *testing.T) {
	if os.Getenv("GNOSH_TMUX") != "1" {
		t.Skip("set GNOSH_TMUX=1 to try the renderer in tmux")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	socket := fmt.Sprintf("gnosh-term-%d", os.Getpid())
	tmux := func(args ...string) string {
		cmd := exec.Command("tmux", append([]string{"-L", socket, "-f", "/dev/null"}, args...)...)
		cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8", "TERM=xterm-256color")
		out, _ := cmd.CombinedOutput()
		return strings.TrimRight(string(out), "\n")
	}
	defer tmux("kill-server")
	// A session that outlives the others keeps the server running.
	tmux("new-session", "-d", "-s", "keep", "sleep 3600")
	long := "0123456789abcdefghijklmnopqrstuvwxyzABCD"

	type step struct {
		cmd       string   // a command for the helper, or "resize W H"
		committed []string // lines committed by this step
		live      []string // the live region after this step, when it changes
		row, col  int      // the cursor after this step, when live is set
	}
	live := func(row, col int, lines ...string) step {
		return step{cmd: fmt.Sprintf("L %d %d %s", row, col, strings.Join(lines, "|")), live: append([]string{}, lines...), row: row, col: col}
	}
	commit := func(line string) step { return step{cmd: "C " + line, committed: []string{line}} }
	resize := func(w, h int) step { return step{cmd: fmt.Sprintf("resize %d %d", w, h)} }

	// fill commits enough lines to fill the screen, so that the live
	// region is at the bottom of it, where it is in a session of any
	// length. (Near the top of a screen with empty rows below, tmux makes
	// room for lines it wraps again by moving the top rows into its
	// history, and the top of the live region can go with them, out of
	// reach.)
	fill := func(steps ...step) []step {
		var out []step
		for i := 1; i <= 12; i++ {
			out = append(out, commit(fmt.Sprint("filler ", i)))
		}
		return append(out, steps...)
	}
	scripts := map[string][]step{
		"commit, grow and shrink": {
			commit("one"), commit("two"), live(0, 2, "> ", "status"),
			commit("three"), live(1, 0, "streaming a", "> ", "status"), commit("streaming a"),
			live(3, 1, "a", "b", "c", "d", "e", "f"), live(0, 2, "> x"), commit("four"),
			commit("five"), commit("six"), commit("seven"), commit("eight"), commit("nine"),
			live(-1, 0, "working", "status"),
		},
		"narrower and wider, cursor shown": fill(
			commit("one"), commit("two"),
			live(2, 4, long, "second "+long[:25], "> in", "status"),
			resize(30, 12), resize(18, 12), resize(11, 12),
			live(3, 4, long[:11], long[11:22], long[22:33], "> in", "status"),
			resize(24, 12), resize(40, 12), commit("three"),
			live(0, 30, "> "+long[:36], "status"), resize(25, 12), resize(15, 12), resize(40, 12),
		),
		"narrower and wider, cursor hidden": fill(
			commit("one"), commit("two"),
			live(-1, 0, long, "second "+long[:25], "> in", "status"),
			resize(30, 12), resize(18, 12), resize(11, 12),
			live(-1, 0, long[:11], long[11:22], long[22:33], "> in", "status"),
			resize(24, 12), resize(40, 12), commit("three"),
		),
		"shorter and taller": {
			commit("one"), commit("two"), commit("three"), commit("four"), commit("five"),
			live(1, 4, long[:30], "> in", "status"),
			resize(40, 6), resize(40, 4), resize(40, 12), commit("six"),
			resize(22, 5), resize(40, 9),
		},
		"full-width lines": {
			commit(long), live(0, 5, long, long, "> in"), commit(long),
			resize(40, 8), live(1, 40, long, long), commit("after"),
		},
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for name, steps := range scripts {
		t.Run(name, func(t *testing.T) {
			logFile := filepath.Join(t.TempDir(), "log")
			session := "s" + strconv.Itoa(len(name))
			cols, rows := 40, 12
			script := fmt.Sprintf("stty -echo; GNOSH_TERM_HELPER=renderer GNOSH_TERM_HELPER_LOG=%s exec %s", logFile, self)
			tmux("new-session", "-d", "-s", session, "-x", fmt.Sprint(cols), "-y", fmt.Sprint(rows), script)
			defer tmux("kill-session", "-t", session)

			var committed, liveLines []string
			curRow, curCol, done := 0, 0, 0
			r := NewRenderer(nil, cols, rows, vtest.Width) // for cutting lines to the width
			before := ""
			for i, st := range steps {
				if w, h, isResize := parseResize(st.cmd); isResize {
					cols, rows = w, h
					tmux("resize-window", "-t", session, "-x", fmt.Sprint(w), "-y", fmt.Sprint(h))
					r.cols, r.rows = w, h
					time.Sleep(50 * time.Millisecond)
					before = fmt.Sprintf("history %s, cursor row,col %s\n  %s",
						tmux("display-message", "-p", "-t", session, "#{history_size}"),
						tmux("display-message", "-p", "-t", session, "#{cursor_y},#{cursor_x}"),
						strings.ReplaceAll(tmux("capture-pane", "-p", "-t", session), "\n", "\n  "))
					tmux("send-keys", "-t", session, "-l", fmt.Sprintf("R %d %d", w, h))
					tmux("send-keys", "-t", session, "Enter")
				} else {
					tmux("send-keys", "-t", session, "-l", st.cmd)
					tmux("send-keys", "-t", session, "Enter")
				}
				done++
				committed = append(committed, st.committed...)
				if st.live != nil || strings.HasPrefix(st.cmd, "L ") {
					liveLines, curRow, curCol = st.live, st.row, st.col
				}
				// What tmux should show: committed lines wrapped by
				// tmux itself, then the live region cut to the width.
				var want []string
				for _, c := range committed {
					want = append(want, wrapPlain(c, cols)...)
				}
				wantCursorLine := len(want)
				vis := liveLines
				if len(vis) > rows {
					vis = vis[len(vis)-rows:]
				}
				for _, l := range vis {
					want = append(want, strings.TrimRight(r.prepare(l).plain, " "))
				}
				for len(want) > 0 && want[len(want)-1] == "" {
					want = want[:len(want)-1]
				}
				wantCol := 0
				if curRow >= 0 && len(vis) > 0 {
					wantCursorLine += min(max(curRow-(len(liveLines)-len(vis)), 0), len(vis)-1)
					wantCol = min(curCol, cols-1)
				}

				deadline := time.Now().Add(20 * time.Second)
				for {
					logged, _ := os.ReadFile(logFile)
					history := 0
					fmt.Sscan(tmux("display-message", "-p", "-t", session, "#{history_size}"), &history)
					all := strings.Split(tmux("capture-pane", "-p", "-t", session, "-S", fmt.Sprint(-history)), "\n")
					for len(all) > 0 && all[len(all)-1] == "" {
						all = all[:len(all)-1]
					}
					var cy, cx int
					fmt.Sscan(tmux("display-message", "-p", "-t", session, "#{cursor_y} #{cursor_x}"), &cy, &cx)
					ok := strings.Count("\n"+string(logged), "\nok ") >= done &&
						reflect.DeepEqual(all, want) && history+cy == wantCursorLine && cx == wantCol
					if ok {
						break
					}
					if time.Now().After(deadline) {
						tail := strings.Split(strings.TrimSpace(string(logged)), "\n")
						t.Fatalf("step %d (%s) at %dx%d:\ntmux shows (cursor on line %d col %d):\n  %s\nwant (cursor on line %d col %d):\n  %s\nthe helper's last writes:\n  %s\nthe screen after the last resize, before the renderer heard of it: %s",
							i, st.cmd, cols, rows, history+cy, cx, strings.Join(all, "\n  "), wantCursorLine, wantCol, strings.Join(want, "\n  "),
							strings.Join(tail[max(len(tail)-3, 0):], "\n  "), before)
					}
					time.Sleep(20 * time.Millisecond)
				}
			}
		})
	}
}

func parseResize(cmd string) (w, h int, ok bool) {
	n, _ := fmt.Sscanf(cmd, "resize %d %d", &w, &h)
	return w, h, n == 2
}

// wrapPlain cuts a line of ASCII into rows of at most cols characters, as
// a terminal that wraps does.
func wrapPlain(s string, cols int) []string {
	var out []string
	for len(s) > cols {
		out = append(out, s[:cols])
		s = s[cols:]
	}
	return append(out, s)
}

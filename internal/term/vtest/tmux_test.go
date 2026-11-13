package vtest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestAgainstTmux writes the same bytes to this emulator and to a real
// terminal emulator, tmux, and compares what each shows. It runs only when
// GNOSH_TMUX=1, on a machine with tmux, and uses a tmux server of its own.
//
// The cases keep to behaviour xterm and tmux share. Where they differ
// (erasing with a wrap pending, backspace with a wrap pending, what a
// scroll region sends to the history) this emulator follows xterm, and the
// difference is covered in the ordinary tests.
func TestAgainstTmux(t *testing.T) {
	if os.Getenv("GNOSH_TMUX") != "1" {
		t.Skip("set GNOSH_TMUX=1 to compare the emulator with tmux")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux is not installed")
	}
	const cols, rows = 12, 5
	socket := fmt.Sprintf("gnosh-vtest-%d", os.Getpid())
	tmux := func(args ...string) (string, error) {
		cmd := exec.Command("tmux", append([]string{"-L", socket, "-f", "/dev/null"}, args...)...)
		cmd.Env = append(os.Environ(), "LC_ALL=C.UTF-8", "TERM=xterm-256color")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	defer tmux("kill-server")
	// A session that outlives the others keeps the server running.
	tmux("new-session", "-d", "-s", "keep", "sleep 3600")

	cases := []struct{ name, input string }{
		{"text and CR LF", "one\r\ntwo\r\nthree"},
		{"LF keeps the column", "ab\ncd\nef"},
		{"full line then CR LF", "0123456789ab\r\nnext"},
		{"full line then a character", "0123456789abX"},
		{"two full lines", "0123456789ab0123456789abZ"},
		{"cursor up and overwrite", "one\r\ntwo\r\nthree\x1b[2A\rONE"},
		{"cursor up stops at the top", "one\r\ntwo\x1b[9A\rX"},
		{"cursor down stops at the bottom", "a\x1b[99Bb"},
		{"column", "abcdef\x1b[3GX\x1b[GY\x1b[99GZ"},
		{"erase to end of line", "abcdef\x1b[3G\x1b[K"},
		{"erase line", "abcdef\x1b[3G\x1b[2Kx"},
		{"erase down", "one\r\ntwo\r\nthree\r\nfour\x1b[3A\x1b[2G\x1b[J"},
		{"autowrap off", "\x1b[?7l0123456789abcdef\x1b[?7h\r\nnext"},
		{"autowrap off then on, mid line", "\x1b[?7l0123456789abXY\x1b[?7hZ"},
		{"wide characters", "a世界b"},
		{"wide character at the edge", "0123456789a世"},
		{"wide character filling the edge", "0123456789世x"},
		{"wide character overwritten", "世界\rX"},
		{"combining mark", "e\u0301x"},
		{"combining mark at the edge", "0123456789ae\u0301"},
		{"combining mark at the edge, autowrap off", "\x1b[?7l0123456789ae\u0301\x1b[?7h"},
		{"combining mark after a style", "e\x1b[1m\u0301x"},
		{"combining mark at the edge, autowrap on for the last cell", "\x1b[?7l0123456789a\x1b[?7he\u0301\x1b[?7l\x1b[m\r\nx\x1b[?7h"},
		{"scrolling", "1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7"},
		{"scrolling by wrap", "0123456789ab0123456789ab0123456789ab0123456789ab0123456789abEND"},
		{"hidden cursor", "abc\x1b[?25l"},
		{"styles do not move anything", "\x1b[1;31mred\x1b[m \x1b[4mu\x1b[m\x1b[38;2;1;2;3mx\x1b[m"},
		{"insert and delete characters", "abcdef\x1b[3G\x1b[2@\x1b[6G\x1b[P"},
		{"erase characters", "abcdef\x1b[2G\x1b[3X"},
		{"insert and delete lines", "1\r\n2\r\n3\r\n4\x1b[2;1H\x1b[L\x1b[4;1H\x1b[M"},
		{"scroll up and down", "1\r\n2\r\n3\x1b[S\x1b[2T"},
		{"scroll region", "1\r\n2\r\n3\r\n4\r\n5\x1b[2;4r\x1b[4;1H\nX"},
		{"reverse index", "1\r\n2\x1b[1;1H\x1bMtop"},
		{"save and restore", "ab\x1b7\r\ncd\x1b8X"},
		{"tabs", "a\tb\tc"},
		{"erase screen keeps it in the history", "one\r\ntwo\r\nthree\x1b[2J"},
		{"erase down from the corner keeps it in the history", "one\r\ntwo\r\nthree\x1b[H\x1b[J"},
		{"erase down from the second column erases", "one\r\ntwo\r\nthree\x1b[1;2H\x1b[J"},
		{"erase down from the second row erases", "one\r\ntwo\r\nthree\x1b[2;1H\x1b[J"},
		{"erase rows one by one from a saved cursor", "one\r\ntwo\r\nthree\r\nfour\x1b[H\x1b7\x1b[2K\x1b[B\x1b[2K\x1b[B\x1b[2K\x1b[B\x1b[2K\x1b[B\x1b[2K\x1b[B\x1b[2K\x1b[B\x1b8x"},
		{"backspace", "abc\b\bX"},
		{"position", "\x1b[3;4Hx\x1b[1;1Hy\x1b[99;99Hz"},
		{"repeat", "ab\x1b[3b"},
		{"what the renderer writes", "\x1b[?2026h\x1b[?25l\x1b[?7lfirst\x1b[m\x1b[K\r\nsecond\x1b[m\x1b[K\r\n\x1b[J\x1b[1A\x1b[3G\x1b[?7h\x1b[?25h\x1b[?2026l"},
	}
	dir := t.TempDir()
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			want := New(cols, rows)
			want.SetScrollOnClear(true) // tmux's default
			want.WriteString(c.input)
			file := filepath.Join(dir, fmt.Sprintf("case%d", i))
			if err := os.WriteFile(file, []byte(c.input), 0o600); err != nil {
				t.Fatal(err)
			}
			session := fmt.Sprintf("c%d", i)
			script := fmt.Sprintf("stty -opost -echo; cat %s; exec sleep 600", file)
			if out, err := tmux("new-session", "-d", "-s", session, "-x", fmt.Sprint(cols), "-y", fmt.Sprint(rows), script); err != nil {
				t.Fatalf("tmux new-session: %v: %s", err, out)
			}
			defer tmux("kill-session", "-t", session)

			wantRow, wantCol := want.Cursor()
			wantScreen := strings.Join(want.Lines(), "\n")
			wantHistory := strings.Join(want.Scrollback(), "\n")
			var screen, history, cursor string
			deadline := time.Now().Add(15 * time.Second)
			for {
				screen, _ = tmux("capture-pane", "-p", "-t", session)
				screen = strings.TrimSuffix(screen, "\n")
				history = ""
				if n, _ := tmux("display-message", "-p", "-t", session, "#{history_size}"); strings.TrimSpace(n) != "0" {
					history, _ = tmux("capture-pane", "-p", "-t", session, "-S", "-"+strings.TrimSpace(n), "-E", "-1")
					history = strings.TrimSuffix(history, "\n")
				}
				cursor, _ = tmux("display-message", "-p", "-t", session, "#{pane_width}x#{pane_height} #{cursor_y},#{cursor_x} #{cursor_flag}")
				cursor = strings.TrimSpace(cursor)
				flag := 1
				if !want.CursorVisible() {
					flag = 0
				}
				// tmux shows a pending wrap as the cursor one past
				// the last column.
				col := wantCol
				if want.WrapPending() {
					col++
				}
				wantCursor := fmt.Sprintf("%dx%d %d,%d %d", cols, rows, wantRow, col, flag)
				if screen == wantScreen && cursor == wantCursor && (history == wantHistory || strings.Contains(c.name, "region")) {
					return
				}
				if time.Now().After(deadline) {
					t.Errorf("input %q\ntmux screen:\n%s\ntmux history: %q\ntmux size, cursor, shown: %s\nemulator (want %s, history %q):\n%s",
						c.input, screen, history, cursor, wantCursor, wantHistory, want.Dump())
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

package shelltool

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/core"
)

func TestLongOutputIsCutInTheMiddle(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxOutputChars: 2000})
	env := newEnv(t)
	res := bash(t, m, env, "seq 1 20000")
	_, body, _ := parts(res)

	lines := strings.Split(body, "\n")
	gap := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "[") && strings.Contains(line, "omitted") {
			if gap >= 0 {
				t.Fatalf("more than one line stands for omitted output: %q and %q", lines[gap], line)
			}
			gap = i
		}
	}
	if gap < 0 {
		t.Fatalf("no line says that output was omitted:\n%s", body)
	}
	head, tail := lines[:gap], lines[gap+1:]
	if len(head) == 0 || head[0] != "1" || head[1] != "2" {
		t.Errorf("the start of the output is not shown: %q", head)
	}
	if len(tail) == 0 || tail[len(tail)-1] != "20000" || tail[len(tail)-2] != "19999" {
		t.Errorf("the end of the output is not shown: %q", tail)
	}
	omitted := 20000 - len(head) - len(tail)
	file, _ := res.Meta[metaOutputFile].(string)
	if want := fmt.Sprintf("[%d lines omitted, full output in %s]", omitted, file); lines[gap] != want {
		t.Errorf("the line between is %q, want %q", lines[gap], want)
	}
	shown := strings.Join(head, "\n") + "\n" + strings.Join(tail, "\n")
	if n := utf8.RuneCountInString(shown); n > 2000 || n < 1900 {
		t.Errorf("%d characters of output are shown, want close to and at most 2000", n)
	}
	if res.IsError || res.Meta[metaTruncated] != true {
		t.Errorf("IsError %v, truncated %v; want a good result marked as cut", res.IsError, res.Meta[metaTruncated])
	}

	// The whole output is in the file the line names.
	whole, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(whole), "\n"); got != 20000 || !strings.HasSuffix(string(whole), "\n19999\n20000\n") {
		t.Errorf("the output file has %d lines, want all 20000", got)
	}
	if size, _ := res.Meta[metaOutputFileSize].(int64); size != int64(len(whole)) {
		t.Errorf("persistedOutputSize = %v, the file has %d bytes", res.Meta[metaOutputFileSize], len(whole))
	}
	if total, _ := res.Meta[metaOutputBytes].(int64); total != int64(len(whole)) {
		t.Errorf("outputBytes = %v, want %d", res.Meta[metaOutputBytes], len(whole))
	}
}

func TestShortOutputKeepsNoFile(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	res := bash(t, m, newEnv(t), "seq 1 100")
	if got := printed(res); !strings.HasPrefix(got, "1\n2\n") || !strings.HasSuffix(got, "\n99\n100") {
		t.Errorf("output = %q", got)
	}
	if _, ok := res.Meta[metaOutputFile]; ok || res.Meta[metaTruncated] != false {
		t.Errorf("Meta = %v; output shown whole needs no file", res.Meta)
	}
	if files, _ := filepath.Glob(filepath.Join(m.dir, "*")); len(files) != 0 {
		t.Errorf("files left in the output directory: %v", files)
	}
}

func TestFailedCommandShowsLess(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxOutputChars: 4000, MaxFailedOutputChars: 1000})
	env := newEnv(t)
	passed := bash(t, m, env, "seq 1 600")
	failed := bash(t, m, env, "seq 1 600; exit 1")
	if passed.Meta[metaTruncated] != false {
		t.Errorf("2,292 characters from a command that succeeded were cut at a limit of 4,000")
	}
	if failed.Meta[metaTruncated] != true || !strings.Contains(printed(failed), "lines omitted, full output in") {
		t.Errorf("2,292 characters from a command that failed were not cut at a limit of 1,000:\n%s", printed(failed))
	}
	if n := utf8.RuneCountInString(printed(failed)); n > 1200 {
		t.Errorf("%d characters shown for a failed command, want about 1,000", n)
	}
}

func TestLongLinesAreCutInsideTheLine(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxOutputChars: 1000})
	res := bash(t, m, newEnv(t), "head -c 50000 /dev/zero | tr '\\0' 'x'; echo; echo END")
	_, body, _ := parts(res)
	lines := strings.Split(body, "\n")
	if len(lines) != 4 || lines[0] != strings.Repeat("x", 500) || !strings.HasPrefix(lines[1], "[output omitted here, full output in ") || lines[3] != "END" {
		t.Errorf("unexpected shape for one very long line:\n%.200s ...\n%q", body, lines[1:])
	}
	if n := utf8.RuneCountInString(lines[0] + lines[2] + lines[3]); n > 1000 {
		t.Errorf("%d characters shown, want at most 1000", n)
	}
}

// TestHugeOutput sends 300 MB through the tool. It is not parallel: it
// measures how much memory the test binary allocates while it runs.
func TestHugeOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("300 MB of output is not short")
	}
	const size = 300_000_000
	m := newManager(t, Options{MaxSpoolBytes: 8 << 20})
	env := newEnv(t)

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	res := run(t, t.Context(), m, BashName, env, args{
		argCommand: fmt.Sprintf(`yes "0123456789 the quick brown fox jumps over the lazy dog" | head -c %d; echo; echo "the very end"`, size),
		argTimeout: 600_000,
	})
	runtime.ReadMemStats(&after)

	if res.IsError {
		t.Fatalf("the command failed: %.300s", core.TextOf(res.Content))
	}
	if got, want := res.Meta[metaOutputBytes], int64(size+1+len("the very end\n")); got != want {
		t.Errorf("outputBytes = %v, want %d", got, want)
	}
	_, body, _ := parts(res)
	if n := utf8.RuneCountInString(body); n > m.opt.MaxOutputChars+200 {
		t.Errorf("%d characters are shown, want at most %d and one line", n, m.opt.MaxOutputChars)
	}
	if !strings.HasPrefix(body, "0123456789 the quick brown fox") || !strings.HasSuffix(body, "\nthe very end") {
		t.Errorf("the start and the end of the output are not what is shown: %.80q ... %q", body, body[len(body)-40:])
	}
	file, _ := res.Meta[metaOutputFile].(string)
	if !strings.Contains(body, "lines omitted, the first 8 MiB of the output are in "+file+"]") {
		t.Errorf("no line says what was omitted and how much of it the file holds")
	}
	if st, err := os.Stat(file); err != nil || st.Size() != 8<<20 {
		t.Errorf("the output file should hold exactly its limit of 8 MiB: %v, %v", st, err)
	}
	// Reading a gigabyte must not take a gigabyte: everything is read
	// through one fixed buffer, and only the two ends are kept.
	grew := after.TotalAlloc - before.TotalAlloc
	t.Logf("%d KiB were allocated to pass %d MB of output through", grew>>10, size/1_000_000)
	if grew > 8<<20 {
		t.Errorf("%d MiB were allocated to pass %d MB of output through", grew>>20, size/1_000_000)
	}
}

func TestHugeOutputWithASlowListener(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxSpoolBytes: 1 << 20})
	env := newEnv(t)
	// The listener takes its time over every piece. The command must not be
	// slowed to its pace: output the listener cannot keep up with is dropped
	// for the listener, and the listener is told.
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	done := make(chan struct{})
	env.Progress = func(p core.Progress) {
		if p.Stream == "status" && strings.Contains(p.Text, "too fast to show") {
			select {
			case <-done:
			default:
				close(done)
			}
		}
		select {
		case <-release:
		case <-time.After(20 * time.Millisecond):
		}
	}
	start := time.Now()
	res := run(t, t.Context(), m, BashName, env, args{argCommand: "yes 'some output' | head -c 50000000; echo; echo end", argTimeout: 120_000})
	if res.IsError || !strings.HasSuffix(printed(res), "\nend") {
		t.Fatalf("the command did not run to its end: %.200s", core.TextOf(res.Content))
	}
	// At the listener's pace, 50 MB would take hours.
	if took := time.Since(start); took > time.Minute {
		t.Errorf("the command took %v: it was held back by the listener", took)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Error("the listener was never told that output was dropped")
	}
}

func TestListenerThatBlocks(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	env.Progress = func(core.Progress) { <-release }
	start := time.Now()
	res := bash(t, m, env, "seq 1 5000; echo done")
	if !strings.HasSuffix(printed(res), "\ndone") {
		t.Errorf("output = %.100q", printed(res))
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Errorf("a listener that blocks held the result up for %v", took)
	}
}

func TestLiveOutput(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	live := newCollector()
	env.Progress = live.take

	// The command waits for the test between its lines, so each line can
	// be seen to arrive while the command is still running.
	command := `echo first
while [ ! -e second ]; do sleep 0.02; done
echo second >&2
printf 'no newline yet'
while [ ! -e third ]; do sleep 0.02; done
echo ', and now there is'
printf 'the end'`
	done := begin(t, t.Context(), m, BashName, env, args{argCommand: command, argTimeout: 60_000})
	live.waitFor(t, "first\n")
	touch(t, env.Cwd, "second")
	live.waitFor(t, "second\n")
	// A line with no newline is shown after a moment all the same.
	live.waitFor(t, "no newline yet")
	touch(t, env.Cwd, "third")
	res := <-done

	want := "first\nsecond\nno newline yet, and now there is\nthe end"
	if got := printed(res); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if got := live.text(); got != want {
		t.Errorf("live output = %q, want %q", got, want)
	}
}

func TestBinaryOutput(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{})
	env := newEnv(t)
	live := newCollector()
	env.Progress = live.take
	res := bash(t, m, env, `printf 'A\000B\377\376C\n'; printf '\303\251t\303\251 \342\202\254 \360\237\230\200\n'; printf 'cut \342\202'`)
	want := "AB�C\nété € 😀\ncut �"
	if got := printed(res); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if got := live.text(); got != want {
		t.Errorf("live output = %q, want %q", got, want)
	}

	// A flood of bytes that are no text at all.
	res = bash(t, m, env, "head -c 200000 /dev/urandom")
	text := core.TextOf(res.Content)
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		t.Errorf("random bytes came through as invalid text")
	}
	if total := res.Meta[metaOutputBytes]; total != int64(200000) {
		t.Errorf("outputBytes = %v, want 200000", total)
	}
	raw, err := os.ReadFile(res.Meta[metaOutputFile].(string))
	if err != nil || len(raw) != 200000 {
		t.Errorf("the output file should hold the bytes as they were: %d bytes, %v", len(raw), err)
	}
}

func TestOutputFileLimit(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxSpoolBytes: 64 << 10, MaxOutputChars: 1000})
	res := bash(t, m, newEnv(t), "seq 1 100000")
	file, _ := res.Meta[metaOutputFile].(string)
	if !strings.Contains(printed(res), "lines omitted, the first 64 KiB of the output are in "+file+"]") {
		t.Errorf("the result does not say that the file holds only the start:\n%s", printed(res))
	}
	if !strings.HasSuffix(printed(res), "\n99999\n100000") {
		t.Errorf("the end of the output is not shown")
	}
	if st, err := os.Stat(file); err != nil || st.Size() != 64<<10 {
		t.Errorf("the output file: %v, %v; want exactly 64 KiB", st, err)
	}
	if got := res.Meta[metaOutputBytes]; got != int64(588895) {
		t.Errorf("outputBytes = %v, want 588895", got)
	}
}

func TestOutputFilesStayWithinBudget(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxOutputChars: 100, SpoolBudget: 25_000})
	env := newEnv(t)
	var files []string
	for range 4 {
		res := bash(t, m, env, "seq 1 2000") // 8,893 bytes each
		files = append(files, res.Meta[metaOutputFile].(string))
	}
	var there []bool
	for _, file := range files {
		_, err := os.Stat(file)
		there = append(there, err == nil)
	}
	if want := []bool{false, false, true, true}; fmt.Sprint(there) != fmt.Sprint(want) {
		t.Errorf("which output files are still there: %v, want %v (the oldest go first)", there, want)
	}
}

func TestOutputDirectoryIsRestored(t *testing.T) {
	t.Parallel()
	m := newManager(t, Options{MaxOutputChars: 100})
	env := newEnv(t)
	bash(t, m, env, "echo once")
	// A cleaner of temporary files takes the directory away mid-session.
	if err := os.RemoveAll(m.dir); err != nil {
		t.Fatal(err)
	}
	res := bash(t, m, env, "seq 1 500")
	if res.IsError || res.Meta[metaTruncated] != true {
		t.Fatalf("the command after the directory vanished: %s", core.TextOf(res.Content))
	}
	if _, err := os.Stat(res.Meta[metaOutputFile].(string)); err != nil {
		t.Errorf("its output file: %v", err)
	}
}

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

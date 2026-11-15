package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A hook in the exec form runs its program with its arguments as they
// are: no shell reads them, the program is found on the hook's PATH, and
// ${CLAUDE_PROJECT_DIR} is the project's directory (spec/cc/hooks.md 2.2
// and 2.4).
func TestExecForm(t *testing.T) {
	opts := options(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "say"), []byte("#!/bin/sh\nfor a in \"$@\"; do printf '[%s]' \"$a\"; done >&2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	opts.Environ = func() []string { return []string{"PATH=" + bin + ":" + testPath, "HOME=" + opts.ProjectDir, marker} }

	args := []string{"a b", "$HOME", "${CLAUDE_PROJECT_DIR}/x", "${GNOSH_PROJECT_DIR}", "'q'", "", "*"}
	r := newRunner(t, opts, Hook{Event: Stop, Command: "say", Args: args})
	out := run(t, r, input(Stop))
	want := "[a b][$HOME][" + opts.ProjectDir + "/x][" + opts.ProjectDir + "]['q'][][*]"
	if len(out.Records) != 1 || out.Records[0].Stderr != want || out.Records[0].ExitCode != 0 {
		t.Errorf("the exec form ran as %+v, want stderr %s", out.Records, want)
	}
	if got := out.Records[0].Command; got != `say 'a b' '$HOME' '${CLAUDE_PROJECT_DIR}/x' '${GNOSH_PROJECT_DIR}' ''\''q'\''' '' '*'` {
		t.Errorf("the record shows the command as %s", got)
	}

	// The program by its path, from the hook's directory, with no
	// arguments at all.
	r = newRunner(t, opts, Hook{Event: Stop, Command: filepath.Join(bin, "say"), Args: []string{}})
	if out := run(t, r, input(Stop)); len(out.Records) != 1 || out.Records[0].Stderr != "" || out.Records[0].ExitCode != 0 {
		t.Errorf("the exec form with no arguments: %+v", out.Records)
	}

	// A program that is not there is a hook that could not run: on an
	// event that can block, it blocks and says why.
	r = newRunner(t, opts, Hook{Event: UserPromptSubmit, Command: "nosuchprogram", Args: []string{"x"}})
	out = run(t, r, input(UserPromptSubmit))
	if !out.Blocked || !strings.Contains(out.Reason, "nosuchprogram is not a program on the PATH the hook is given") {
		t.Errorf("a program that is not there: blocked %v, %q", out.Blocked, out.Reason)
	}

	// The same program and arguments configured twice run once; the shell
	// form of the same words is another hook.
	r = newRunner(t, opts,
		Hook{Event: Stop, Command: "say", Args: []string{"once"}},
		Hook{Event: Stop, Command: "say", Args: []string{"once"}},
		Hook{Event: Stop, Command: "say once"})
	if out := run(t, r, input(Stop)); len(out.Records) != 2 {
		t.Errorf("%d hooks ran, want 2: %+v", len(out.Records), out.Records)
	}
}

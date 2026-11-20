package sandbox

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Status is whether this machine can sandbox commands.
type Status struct {
	// OK says it can; Why says why not, in a sentence, when it cannot.
	OK  bool
	Why string
	// Tool is the program that makes the sandbox: bwrap's path, or
	// sandbox-exec's.
	Tool string
}

var (
	checkOnce sync.Once
	checked   Status
)

// Check reports whether commands can be sandboxed here, trying the sandbox
// once in the process's life: a bubblewrap that the system will not let
// make its namespaces (Ubuntu's AppArmor rule for unprivileged ones, a
// container) is found by its first try, not by the model's first command.
func Check() Status {
	checkOnce.Do(func() { checked = check() })
	return checked
}

// try runs a sandbox's trial command, and says what went wrong, if
// anything, in its own words.
func try(argv []string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		said := strings.TrimSpace(out.String())
		if i := strings.IndexByte(said, '\n'); i >= 0 {
			said = said[:i]
		}
		if said == "" {
			said = err.Error()
		}
		return said
	}
	return ""
}

// lookFor finds a program in the system's own folders, never on the PATH:
// a folder there a sandboxed command can write (a virtualenv's, a
// project's node_modules/.bin) could hold a program of that name that
// makes no sandbox at all (the sandbox review).
func lookFor(name string) string {
	for _, dir := range []string{"/usr/bin", "/bin"} {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

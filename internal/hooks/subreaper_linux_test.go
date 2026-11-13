//go:build linux

package hooks

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// becomeSubreaper makes this process the one that inherits the processes
// its descendants orphan. The tests never collect those, so from then on a
// hook's dead children stay in the process table for as long as the test
// binary runs: the state of a container whose first process is not an
// init.
func becomeSubreaper() error {
	const prSetChildSubreaper = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0); errno != 0 {
		return errno
	}
	return nil
}

// Stopping a hook must not depend on somebody collecting its dead
// processes. The tests that stop hooks are run again in a copy of this
// test binary where nobody does; there they must pass as they do here,
// and as quickly.
func TestStoppingHooksWhereNobodyCollectsTheDead(t *testing.T) {
	if os.Getenv(subreaperEnv) != "" {
		t.Skip("this is the copy")
	}
	t.Parallel()
	cmd := exec.Command(os.Args[0], "-test.run", "^(TestTimeout|TestHookThatIgnores|TestHookIsAsked|TestCancelStops|TestHookMayLeave)", "-test.count=1")
	cmd.Env = append(os.Environ(), subreaperEnv+"=1")
	began := time.Now()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the tests that stop hooks failed where nobody collects the dead: %v\n%s", err, out)
	}
	if took := time.Since(began); took > 30*time.Second {
		t.Errorf("they took %v", took)
	}
}

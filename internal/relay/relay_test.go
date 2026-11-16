package relay

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// ssh goes on in the background (-f) only where its own reading of its
// arguments says so: flags joined in one word, the value of an option
// that takes one, options after the host, and the command after them.
func TestForks(t *testing.T) {
	for _, c := range []struct {
		args string
		want bool
	}{
		{"box", false},
		{"-f box", true},
		{"-fN -L 8080:localhost:80 box", true},
		{"-NfL 8080:localhost:80 box", true},
		{"-L 8080:localhost:80 -f box", true},
		{"box -f", true},
		{"-p 22 box -N -f", true},
		{"-l f box", false},
		{"-lf box", false},
		{"-F f box", false},
		{"-i ~/.ssh/f -p 22 box", false},
		{"box ls -f", false},
		{"box -- -f", false},
		{"-- box -f", false},
		{"-o ForkAfterAuthentication=yes box", true},
		{"-oforkafterauthentication=YES box", true},
		{"-o ForkAfterAuthentication=no box", false},
		{"-o StrictHostKeyChecking=yes box -f", true},
		{"-t box gnosh", false},
	} {
		if got := forks(strings.Fields(c.args)); got != c.want {
			t.Errorf("ssh %s: forks %v, want %v", c.args, got, c.want)
		}
	}
	if !forks([]string{"-o", "ForkAfterAuthentication yes", "box"}) {
		t.Error(`-o "ForkAfterAuthentication yes" is not read as going on in the background`)
	}
}

// failAfter is a writer that takes n writes and then fails.
type failAfter struct{ n int }

func (w *failAfter) Write(p []byte) (int, error) {
	if w.n == 0 {
		return 0, errors.New("the terminal is gone")
	}
	w.n--
	return len(p), nil
}

// When the terminal takes no more, what ssh writes is still read to its
// end, so ssh is never left waiting, and its copies are still found.
func TestPassReadsOnWhenTheTerminalIsGone(t *testing.T) {
	r, w := io.Pipe()
	var found []string
	passed := make(chan struct{})
	go func() {
		defer close(passed)
		pass(r, &failAfter{n: 1}, func(text string) { found = append(found, text) })
	}()
	writes := []string{"first\r\n", strings.Repeat("x", 1<<20), "\x1b]52;c;aGk=", "\x07", "last"}
	wrote := make(chan struct{})
	go func() {
		defer close(wrote)
		for _, s := range writes {
			if _, err := io.WriteString(w, s); err != nil {
				t.Errorf("ssh could not write: %v", err)
				return
			}
		}
		w.Close()
	}()
	for _, ch := range []chan struct{}{wrote, passed} {
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatal("what ssh wrote was not read to its end")
		}
	}
	if !slices.Equal(found, []string{"hi"}) {
		t.Errorf("found %q, want the copy that came after the terminal went", found)
	}
}

// The clipboard holds one text: a copy that waits while another is made
// gives way to a newer one, and the newest is made.
func TestCopierMakesTheNewest(t *testing.T) {
	var mu sync.Mutex
	var made []string
	release := make(chan struct{})
	c := newCopier(func(_ context.Context, text string) error {
		if text == "first" {
			<-release
		}
		mu.Lock()
		made = append(made, text)
		mu.Unlock()
		return nil
	})
	c.offer("first")
	time.Sleep(50 * time.Millisecond)
	c.offer("second")
	c.offer("third")
	close(release)
	all, failed, err := c.finish(5 * time.Second)
	if !all || failed != 0 || err != nil {
		t.Errorf("finish says %v, %d, %v; want all made", all, failed, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(made, []string{"first", "third"}) {
		t.Errorf("made %q, want first and then the newest", made)
	}
}

// A copy that is not made in the time given is stopped, and finish says
// so; one that fails is counted.
func TestCopierGivesUp(t *testing.T) {
	stopped := make(chan struct{})
	c := newCopier(func(ctx context.Context, text string) error {
		if text == "fails" {
			return errors.New("no clipboard")
		}
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	c.offer("fails")
	time.Sleep(50 * time.Millisecond)
	c.offer("hangs")
	time.Sleep(50 * time.Millisecond)
	all, failed, err := c.finish(100 * time.Millisecond)
	if all || failed != 1 || err == nil || err.Error() != "no clipboard" {
		t.Errorf("finish says %v, %d, %v; want not all, and the one that failed", all, failed, err)
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Error("the copy under way was not stopped")
	}
}

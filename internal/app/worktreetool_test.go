package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// EnterWorktree, asked for, makes a git work tree of the repository's and
// moves the session into it: the turn ends, the session's file goes with
// the tree's sessions, and gnosh starts again with --worktree and the
// session, told what moved. That gnosh works in the tree and goes on by
// itself, and its ExitWorktree takes the session back, removing the tree
// it left as made; the gnosh that starts then goes on in the checkout.
func TestEnterAndExitWorktree(t *testing.T) {
	w := repoWorld(t,
		apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "EnterWorktree", Args: `{"name":"feat"}`}}},
		apptest.Reply{Text: "In the tree now."},
		apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "ExitWorktree", Args: `{"action":"remove"}`}}},
		apptest.Reply{Text: "Back in the checkout."},
	)
	var started, envs [][]string
	relaunch := func(o *Options) {
		o.Relaunch = func(args, env []string) error {
			started, envs = append(started, args), append(envs, env)
			return os.ErrPermission
		}
	}
	s := sitWith(t, w, relaunch, "--trust-folder")
	s.see("Type a prompt and Enter.")
	s.typeLine("work in a worktree")
	s.see("The session moves into the git work tree")
	if code := s.exit(); code != 1 {
		t.Errorf("exit code %d, want 1 for a start that failed", code)
	}
	tree := filepath.Join(w.Dir, ".gnosh", "worktrees", "feat")
	if real, err := filepath.EvalSymlinks(tree); err != nil || !fileExists(filepath.Join(real, "main.go")) {
		t.Fatalf("no work tree at %s (%v)", tree, err)
	}
	if len(started) != 1 {
		t.Fatalf("gnosh started again %d times", len(started))
	}
	id := strings.TrimSuffix(filepath.Base(w.Sessions()[0]), ".jsonl")
	if !slices.Contains(started[0], "--worktree=feat") || !slices.Contains(started[0], "--resume="+id) || !slices.Contains(started[0], "--trust-folder") {
		t.Errorf("gnosh started again with %q, want --worktree=feat and --resume=%s among them", started[0], id)
	}
	st := &transcript.Store{Dir: w.DataDir()}
	if list, _ := st.List(context.Background(), tree); len(list) != 1 {
		t.Errorf("the tree lists %d sessions, want the one that moved", len(list))
	}
	if !slices.ContainsFunc(envs[0], func(kv string) bool {
		return strings.HasPrefix(kv, TreeMoveEnv+"=") && strings.Contains(kv, `"entered"`)
	}) {
		t.Errorf("the gnosh EnterWorktree starts is not told what moved: %q", envs[0])
	}

	// That gnosh works in the tree and goes on by itself; asked, it takes
	// the session back out.
	s = sitWith(t, w, func(o *Options) {
		relaunch(o)
		o.Environ = append(slices.Clone(o.Environ), envs[0][len(envs[0])-2:]...)
	}, started[0]...)
	s.see("In the tree now.")
	s.typeLine("leave it")
	s.see("The session goes back to")
	s.exit()
	reqs := w.Model.Requests()
	has(t, "the request after the move", string(reqs[1].Body), "The session works in the git work tree", "on the branch worktree-feat", "Go on with the task")
	if fileExists(tree) {
		t.Errorf("the tree left as it was made is still there")
	}
	if len(started) != 2 || slices.ContainsFunc(started[1], func(a string) bool { return strings.HasPrefix(a, "--worktree") }) {
		t.Fatalf("after ExitWorktree, gnosh started again with %q", started)
	}

	// The gnosh that starts then goes on in the checkout.
	s = sitWith(t, w, func(o *Options) {
		o.Environ = append(slices.Clone(o.Environ), envs[1][len(envs[1])-2:]...)
	}, started[1]...)
	s.see("Back in the checkout.")
	s.typeLine("/exit")
	s.exit()
	reqs = w.Model.Requests()
	has(t, "the request after going back", string(reqs[len(reqs)-1].Body), "The session works in "+w.Dir+" again", "was removed with its branch worktree-feat")
	if list, _ := st.List(context.Background(), w.Dir); len(list) != 1 || list[0].ID != id {
		t.Errorf("after going back, the checkout lists %+v", list)
	}
}

// Neither tool is offered where it cannot work: in a run, in a folder not
// trusted, out of a git repository, or with nobody to start gnosh again;
// and ExitWorktree only in a tree EnterWorktree moved the session into.
func TestWorktreeToolsOffered(t *testing.T) {
	w := repoWorld(t, apptest.Reply{Text: "ok"}, apptest.Reply{Text: "ok"}, apptest.Reply{Text: "ok"})
	offered := func(i int) string { return string(w.Model.Requests()[i].Body) }
	run(t, w, "--trust-folder", "-p", "hi").wantCode(t, 0)
	if strings.Contains(offered(0), `"EnterWorktree"`) {
		t.Errorf("a run was offered EnterWorktree")
	}
	s := sitWith(t, w, func(o *Options) { o.Relaunch = func(args, env []string) error { return nil } }, "--trust-folder")
	s.see("Type a prompt and Enter.")
	s.typeLine("hi")
	s.see("ok")
	s.typeLine("/exit")
	s.exit()
	if body := offered(1); !strings.Contains(body, `"EnterWorktree"`) || strings.Contains(body, `"ExitWorktree"`) {
		t.Errorf("an interactive session in a trusted repository: EnterWorktree %v, ExitWorktree %v", strings.Contains(body, `"EnterWorktree"`), strings.Contains(body, `"ExitWorktree"`))
	}
	s = sit(t, w, "--trust-folder")
	s.see("Type a prompt and Enter.")
	s.typeLine("hi")
	s.see("ok")
	s.typeLine("/exit")
	s.exit()
	if strings.Contains(offered(2), `"EnterWorktree"`) {
		t.Errorf("a gnosh that cannot start itself again was offered EnterWorktree")
	}
}

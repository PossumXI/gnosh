package app

import (
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// Claude Code's blocks, set in the person's gnosh.json, hold in
// bypassPermissions (-y): the file tools refuse to read or change a file
// outside the project, and to change a settings file in it, and say why,
// while the project's own files are read as before; the work tree tools
// are not offered.
func TestBlocksOutsideTheWorkingDirectories(t *testing.T) {
	w := apptest.NewWorld(t)
	secret := w.Write(w.Home+"/outside/secret.txt", "hidden\n")
	w.Write("a.txt", "mine\n")
	w.Settings(`{"permissions": {"blockReadsOutsideWorkingDirectories": true, "blockWritesOutsideWorkingDirectories": true}}`)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		if n == 0 {
			return &apptest.Reply{Calls: []apptest.Call{
				{ID: "c1", Name: "Read", Args: `{"file_path":"` + secret + `"}`},
				{ID: "c2", Name: "Write", Args: `{"file_path":"` + w.Home + `/outside/new.txt","content":"x\n"}`},
				{ID: "c3", Name: "Read", Args: `{"file_path":"a.txt"}`},
				{ID: "c4", Name: "Write", Args: `{"file_path":"gnosh.json","content":"{}\n"}`},
			}}
		}
		return &apptest.Reply{Text: "Done."}
	})
	run(t, w, "-y", "-p", "look around").wantCode(t, 0)
	has(t, "the read outside", toolResult(t, w, 1, "c1"), "blockReadsOutsideWorkingDirectories", "/add-dir")
	has(t, "the write outside", toolResult(t, w, 1, "c2"), "blockWritesOutsideWorkingDirectories")
	has(t, "the project's file", toolResult(t, w, 1, "c3"), "mine")
	has(t, "the project's settings file", toolResult(t, w, 1, "c4"), "settings file")
	if w.Exists(w.Home+"/outside/new.txt") || w.Exists("gnosh.json") {
		t.Error("a blocked write was made")
	}
	if body := string(w.Model.Requests()[0].Body); strings.Contains(body, `"name":"EnterWorktree"`) {
		t.Error("EnterWorktree is offered under the write block")
	}
	// /permissions says so.
	r := run(t, w, "-p", "/permissions")
	r.wantCode(t, 0)
	has(t, "/permissions", r.out, "The file tools read or change nothing outside these folders, in every mode", "blockReadsOutsideWorkingDirectories")
}

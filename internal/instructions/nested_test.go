package instructions

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
)

func nestedOptions() Options {
	return Options{
		Cwd:          "/work/shop",
		ProjectNames: []string{"CLAUDE.md"},
		LocalNames:   []string{"CLAUDE.local.md"},
		DotDirs:      []string{".claude"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/work/shop/CLAUDE.md":              "shop\n",
			"/work/shop/shared.md":              "shared\n",
			"/work/shop/main.go":                "",
			"/work/shop/api/CLAUDE.md":          "api; @../shared.md @notes.md\n",
			"/work/shop/api/notes.md":           "api notes\n",
			"/work/shop/api/CLAUDE.local.md":    "api private\n",
			"/work/shop/api/.claude/CLAUDE.md":  "api dot\n",
			"/work/shop/api/.claude/rules/r.md": "api rule\n",
			"/work/shop/api/a.go":               "",
			"/work/shop/api/v2/CLAUDE.md":       "v2; @../../shared.md\n",
			"/work/shop/api/v2/h.go":            "",
			"/work/shop/web/CLAUDE.md":          "web\n",
			"/work/shop/web/app.ts":             "",
			"/work/other/CLAUDE.md":             "beside the project\n",
			"/work/other/x.go":                  "",
		}),
	}
}

func TestNestedFilesArriveOnce(t *testing.T) {
	s := load(t, nestedOptions())
	same(t, "at the start", paths(s.Files()), []string{"/work/shop/CLAUDE.md"})

	// The first touch two levels down brings both levels, outermost first,
	// each as starting the session there would have.
	late, warns := touch(s, "/work/shop/api/v2/h.go")
	same(t, "two levels down", brief(late), []string{
		"/work/shop/api/CLAUDE.md Project main nested_traversal",
		"/work/shop/shared.md Project import include",
		"/work/shop/api/notes.md Project import include",
		"/work/shop/api/.claude/CLAUDE.md Project main nested_traversal",
		"/work/shop/api/.claude/rules/r.md Project rule nested_traversal",
		"/work/shop/api/CLAUDE.local.md Local main nested_traversal",
		"/work/shop/api/v2/CLAUDE.md Project main nested_traversal",
	})
	same(t, "warnings", warned(warns), nil)
	for _, f := range late {
		if !f.Late || f.Trigger != "/work/shop/api/v2/h.go" {
			t.Errorf("%s: late %v, trigger %q", f.Path, f.Late, f.Trigger)
		}
	}
	if late[0].Base != "/work/shop/api" || late[3].Base != "/work/shop/api" || late[6].Base != "/work/shop/api/v2" {
		t.Errorf("bases: %q %q %q", late[0].Base, late[3].Base, late[6].Base)
	}

	// After that, nothing in those directories is given again, however
	// the path is written.
	for _, p := range []string{
		"/work/shop/api/v2/h.go",
		"/work/shop/api/a.go",
		"api/a.go",
		"./api/v2/../v2/h.go",
		"/work/shop/api",
		"/work/shop/main.go", // the working directory itself was read at the start
		"/work/shop",
		"/work/other/x.go", // not below the working directory
		"/work",
	} {
		if again, _ := touch(s, p); len(again) != 0 {
			t.Errorf("touching %s gave %v", p, brief(again))
		}
	}

	// Another directory, reached by a relative path.
	late, _ = touch(s, "web/app.ts")
	same(t, "another directory", brief(late), []string{"/work/shop/web/CLAUDE.md Project main nested_traversal"})
	if late[0].Trigger != "/work/shop/web/app.ts" {
		t.Errorf("trigger %q", late[0].Trigger)
	}

	// Files() holds everything in the order it arrived; Render only what
	// was there from the start.
	if n := len(s.Files()); n != 9 {
		t.Errorf("%d files in all, want 9", n)
	}
	if r := s.Render(); strings.Contains(r, "/work/shop/api") || !strings.Contains(r, "=== /work/shop/CLAUDE.md (project instructions) ===") {
		t.Errorf("Render:\n%s", r)
	}
}

func TestNestedDirectoryTouchedAsADirectoryOrBeforeItExists(t *testing.T) {
	s := load(t, nestedOptions())
	late, _ := touch(s, "/work/shop/web")
	same(t, "moving into a directory", paths(late), []string{"/work/shop/web/CLAUDE.md"})

	s = load(t, nestedOptions())
	late, _ = touch(s, "/work/shop/web/new/deeper/file.ts")
	same(t, "a file about to be made", paths(late), []string{"/work/shop/web/CLAUDE.md"})
}

func TestLateFilesAreCut(t *testing.T) {
	long := "0123456789\nabcdef\nKLMNOPQRST\nthe rest\n"
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md"},
		LateMaxChars: 20,
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/CLAUDE.md":     long,
			"/p/sub/CLAUDE.md": long + "@more.md\n",
			"/p/sub/more.md":   "never seen, so never imported\n",
			"/p/sub/x":         "",
		}),
	}
	s := load(t, o)
	if f := s.Files()[0]; f.Truncated || f.Text != long {
		t.Errorf("a file from the start is never cut: %+v", f)
	}
	late, warns := touch(s, "/p/sub/x")
	same(t, "late", paths(late), []string{"/p/sub/CLAUDE.md"})
	f := late[0]
	if !f.Truncated || f.Text != "0123456789\nabcdef\n" || f.Size != int64(len(long)+len("@more.md\n")) {
		t.Errorf("the cut file: %+v", f)
	}
	same(t, "warnings", warned(warns), []string{"cut /p/sub/CLAUDE.md"})
	if !strings.Contains(warns[0].Text, "first 20 characters") {
		t.Errorf("warning: %s", warns[0])
	}
	if r := RenderFiles(late); !strings.HasSuffix(r, "0123456789\nabcdef\n"+cutNote+"\n") {
		t.Errorf("the note is missing:\n%s", r)
	}

	// A negative limit means no cut.
	o.LateMaxChars = -1
	s = load(t, o)
	late, _ = touch(s, "/p/sub/x")
	same(t, "uncut", paths(late), []string{"/p/sub/CLAUDE.md", "/p/sub/more.md"})
}

func TestForget(t *testing.T) {
	fsys := tree(map[string]string{
		"/p/CLAUDE.md":           "top\n",
		"/p/sub/CLAUDE.md":       "sub; @extra.md\n",
		"/p/sub/extra.md":        "extra\n",
		"/p/sub/x":               "",
		"/p/web/CLAUDE.md":       "web\n",
		"/p/web/y":               "",
		"/p/.claude/rules/go.md": "---\npaths: \"*.go\"\n---\ngo rule\n",
		"/p/z.go":                "",
	})
	o := Options{Cwd: "/p", ProjectNames: []string{"CLAUDE.md"}, DotDirs: []string{".claude"}, Trusted: true, FS: fsys}
	s := load(t, o)
	touchAll := func() []string {
		var got []string
		for _, p := range []string{"/p/sub/x", "/p/web/y", "/p/z.go"} {
			late, _ := touch(s, p)
			got = append(got, paths(late)...)
		}
		return got
	}
	same(t, "first", touchAll(), []string{"/p/sub/CLAUDE.md", "/p/sub/extra.md", "/p/web/CLAUDE.md", "/p/.claude/rules/go.md"})
	same(t, "second", touchAll(), nil)

	// Naming an import forgets the file it came with too, and they come
	// back together, read fresh.
	fsys["p/sub/CLAUDE.md"].Data = []byte("sub, edited; @extra.md\n")
	s.Forget("/p/sub/extra.md")
	same(t, "held after forgetting one", paths(s.Files()), []string{"/p/CLAUDE.md", "/p/web/CLAUDE.md", "/p/.claude/rules/go.md"})
	late, _ := touch(s, "/p/sub/x")
	same(t, "given again", paths(late), []string{"/p/sub/CLAUDE.md", "/p/sub/extra.md"})
	if late[0].Text != "sub, edited; @extra.md\n" {
		t.Errorf("not read fresh: %q", late[0].Text)
	}
	same(t, "the others stay given", touchAll(), nil)

	// A rule comes back when its pattern next matches, read again too.
	fsys["p/.claude/rules/go.md"].Data = []byte("---\npaths: \"*.go\"\n---\ngo rule, edited\n")
	s.Forget("/p/.claude/rules/go.md")
	same(t, "the rule again", touchAll(), []string{"/p/.claude/rules/go.md"})
	if f := find(t, s.Files(), "/p/.claude/rules/go.md"); f.Text != "go rule, edited\n" {
		t.Errorf("the rule was not read again: %q", f.Text)
	}

	// A file from the start is never forgotten, and an unknown path is
	// passed over.
	s.Forget("/p/CLAUDE.md", "/nowhere")
	same(t, "nothing forgotten", touchAll(), nil)
	if n := len(s.Files()); n != 5 {
		t.Errorf("%d files held, want 5", n)
	}

	// With no paths, every late file is forgotten.
	s.Forget()
	same(t, "held after forgetting all", paths(s.Files()), []string{"/p/CLAUDE.md"})
	same(t, "everything again", touchAll(), []string{"/p/sub/CLAUDE.md", "/p/sub/extra.md", "/p/web/CLAUDE.md", "/p/.claude/rules/go.md"})
}

func TestNestedImportAlreadyGivenIsNotGivenAgain(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/CLAUDE.md":     "@shared.md\n",
			"/p/shared.md":     "shared\n",
			"/p/sub/CLAUDE.md": "@../shared.md\n",
			"/p/sub/x":         "",
		}),
	}
	s := load(t, o)
	late, _ := touch(s, "/p/sub/x")
	same(t, "late", paths(late), []string{"/p/sub/CLAUDE.md"})
	// Forgetting the late file does not take the start's import with it.
	s.Forget("/p/sub/CLAUDE.md")
	same(t, "held", paths(s.Files()), []string{"/p/CLAUDE.md", "/p/shared.md"})
}

func TestForPathFromManyGoroutines(t *testing.T) {
	files := map[string]string{"/p/CLAUDE.md": "top\n", "/p/.claude/rules/go.md": "---\npaths: \"*.go\"\n---\ngo\n"}
	var touched []string
	for i := range 40 {
		dir := fmt.Sprintf("/p/d%02d", i)
		files[dir+"/CLAUDE.md"] = "nested\n"
		files[dir+"/f.go"] = ""
		touched = append(touched, dir+"/f.go")
	}
	s := load(t, Options{Cwd: "/p", ProjectNames: []string{"CLAUDE.md"}, DotDirs: []string{".claude"}, Trusted: true, FS: tree(files)})

	var mu sync.Mutex
	var got []string
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for _, p := range touched {
				late, _ := s.ForPath(context.Background(), p)
				s.Files()
				s.Warnings()
				s.Render()
				s.Changed(context.Background())
				mu.Lock()
				got = append(got, paths(late)...)
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	// Every nested file and the one rule, each exactly once.
	slices.Sort(got)
	want := []string{"/p/.claude/rules/go.md"}
	for i := range 40 {
		want = append(want, fmt.Sprintf("/p/d%02d/CLAUDE.md", i))
	}
	same(t, "delivered", got, want)
}

func TestForPathStopsWhenCancelled(t *testing.T) {
	s := load(t, nestedOptions())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if late, _ := s.ForPath(ctx, "/work/shop/api/v2/h.go"); len(late) != 0 {
		t.Errorf("a cancelled call gave %v", brief(late))
	}
	// What was not reached comes with the next call.
	late, _ := touch(s, "/work/shop/api/v2/h.go")
	if len(late) != 7 {
		t.Errorf("the next call gave %v", brief(late))
	}
}

func TestRuleBelowTheWorkingDirectoryIsPutByOnce(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md"},
		DotDirs:      []string{".claude"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/sub/CLAUDE.md":           "sub\n",
			"/p/sub/.claude/rules/go.md": "---\npaths: \"*.go\"\n---\nsub's Go rule\n",
			"/p/sub/notes.txt":           "",
			"/p/sub/x.go":                "",
			"/p/sub/y.go":                "",
		}),
	}
	s := load(t, o)
	// The first touch finds the rule and puts it by; it does not match.
	late, _ := touch(s, "/p/sub/notes.txt")
	same(t, "a text file", paths(late), []string{"/p/sub/CLAUDE.md"})
	// After the conversation is cut, the directory is searched again and
	// finds the same rule waiting, not a second one.
	s.Forget()
	late, _ = touch(s, "/p/sub/x.go")
	same(t, "a Go file", brief(late), []string{
		"/p/sub/CLAUDE.md Project main nested_traversal",
		"/p/sub/.claude/rules/go.md Project rule path_glob_match",
	})
	if late[1].Base != "/p/sub" {
		t.Errorf("the rule's base: %q", late[1].Base)
	}
	if again, _ := touch(s, "/p/sub/y.go"); len(again) != 0 {
		t.Errorf("given twice: %v", paths(again))
	}
	if n := len(s.rules); n != 1 {
		t.Errorf("%d rules put by, want 1", n)
	}
}

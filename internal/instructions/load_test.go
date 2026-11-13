package instructions

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// shop is a tree with one of everything, for the tests of order.
func shop() fstest.MapFS {
	return tree(map[string]string{
		"/home/ana/.config/gnosh/GNOSH.md":      "user\n",
		"/home/ana/.config/gnosh/rules/tone.md": "tone\n",
		"/home/ana/.config/gnosh/rules/go.md":   "---\npaths: \"*.go\"\n---\ngo rule\n",
		"/GNOSH.md":                             "top of the disk\n",
		"/work/GNOSH.md":                        "work\n",
		"/work/shop/GNOSH.md":                   "shop\n",
		"/work/shop/.gnosh/GNOSH.md":            "shop, in the dot directory\n",
		"/work/shop/CLAUDE.md":                  "shop, under the other name\n",
		"/work/shop/.claude/CLAUDE.md":          "shop, other name, dot directory\n",
		"/work/shop/CLAUDE.local.md":            "shop, private\n",
		"/work/shop/.claude/CLAUDE.local.md":    "local files are not looked for in dot directories\n",
		"/work/shop/.gnosh/rules/a.md":          "rule a\n",
		"/work/shop/.gnosh/rules/sub/b.md":      "rule b\n",
		"/work/shop/.gnosh/rules/notes.txt":     "not a rule\n",
		"/work/shop/.gnosh/rules/z.md":          "---\npaths:\n  - \"api/**\"\n---\nrule z\n",
		"/work/shop/.claude/rules/c.md":         "rule c\n",
		"/work/shop/api/GNOSH.md":               "api\n",
		"/work/shop/api/CLAUDE.local.md":        "api, private\n",
		"/work/shop/api/v2/GNOSH.md":            "below the working directory\n",
		"/work/shop/api/v2/handler.go":          "package v2\n",
		"/work/other/GNOSH.md":                  "beside the project\n",
	})
}

func shopOptions() Options {
	return Options{
		Cwd:          "/work/shop/api",
		ProjectRoot:  "/work/shop",
		Home:         "/home/ana",
		UserFiles:    []string{"/home/ana/.config/gnosh/GNOSH.md", "/home/ana/.config/gnosh/absent.md", "/home/ana/.config/gnosh/rules"},
		ProjectNames: []string{"GNOSH.md", "CLAUDE.md"},
		LocalNames:   []string{"CLAUDE.local.md"},
		DotDirs:      []string{".gnosh", ".claude"},
		Trusted:      true,
		FS:           shop(),
	}
}

func TestOrderAtStart(t *testing.T) {
	s := load(t, shopOptions())
	same(t, "files", brief(s.Files()), []string{
		"/home/ana/.config/gnosh/GNOSH.md User main session_start",
		"/home/ana/.config/gnosh/rules/tone.md User rule session_start",
		"/GNOSH.md Project main session_start",
		"/work/GNOSH.md Project main session_start",
		"/work/shop/GNOSH.md Project main session_start",
		"/work/shop/.gnosh/GNOSH.md Project main session_start",
		"/work/shop/CLAUDE.md Project main session_start",
		"/work/shop/.claude/CLAUDE.md Project main session_start",
		"/work/shop/.gnosh/rules/a.md Project rule session_start",
		"/work/shop/.gnosh/rules/sub/b.md Project rule session_start",
		"/work/shop/.claude/rules/c.md Project rule session_start",
		"/work/shop/CLAUDE.local.md Local main session_start",
		"/work/shop/api/GNOSH.md Project main session_start",
		"/work/shop/api/CLAUDE.local.md Local main session_start",
	})
	same(t, "warnings", warned(s.Warnings()), nil)

	f := find(t, s.Files(), "/work/shop/.gnosh/GNOSH.md")
	if f.Base != "/work/shop" || f.RealPath != f.Path || f.Late || f.Depth != 0 || f.Parent != "" {
		t.Errorf("the dot-directory file: %+v", f)
	}
	if f.Text != "shop, in the dot directory\n" || f.Size != int64(len(f.Text)) || !f.ModTime.Equal(t0) {
		t.Errorf("text, size and time: %q %d %v", f.Text, f.Size, f.ModTime)
	}
	if u := find(t, s.Files(), "/home/ana/.config/gnosh/GNOSH.md"); u.Base != "" {
		t.Errorf("a user file speaks for no directory, got %q", u.Base)
	}
	if r := find(t, s.Files(), "/home/ana/.config/gnosh/rules/tone.md"); r.Base != "/work/shop/api" {
		t.Errorf("a user rule's base is the working directory, got %q", r.Base)
	}
}

func TestZeroOptionsReadNothingFromTheProject(t *testing.T) {
	o := shopOptions()
	o.Trusted = false
	s := load(t, o)
	same(t, "files", brief(s.Files()), []string{
		"/home/ana/.config/gnosh/GNOSH.md User main session_start",
		"/home/ana/.config/gnosh/rules/tone.md User rule session_start",
	})

	// Nothing below the working directory either, but the person's own
	// rule for Go files still applies.
	files, _ := touch(s, "/work/shop/api/v2/handler.go")
	same(t, "late files", brief(files), []string{
		"/home/ana/.config/gnosh/rules/go.md User rule path_glob_match",
	})

	// Only the working directory is required.
	s = load(t, Options{Cwd: "/work/shop", FS: shop()})
	if n := len(s.Files()); n != 0 {
		t.Errorf("zero options loaded %d files", n)
	}
	if s.Render() != "" {
		t.Errorf("nothing to render, got %q", s.Render())
	}
}

func TestOnlyTheNamesAsked(t *testing.T) {
	o := shopOptions()
	o.ProjectNames = []string{"CLAUDE.md"}
	o.LocalNames = nil
	o.DotDirs = nil
	o.UserFiles = nil
	s := load(t, o)
	same(t, "files", brief(s.Files()), []string{
		"/work/shop/CLAUDE.md Project main session_start",
	})
}

func TestFallbackInstead(t *testing.T) {
	files := map[string]string{
		"/home/ana/GNOSH.md":             "user\n",
		"/AGENTS.md":                     "top\n",
		"/work/AGENTS.md":                "work\n",
		"/work/shop/AGENTS.md":           "shop\n",
		"/work/shop/.gnosh/AGENTS.md":    "shop dot\n",
		"/work/shop/.gnosh/rules/r.md":   "rule\n",
		"/work/shop/api/AGENTS.md":       "api\n",
		"/work/shop/api/x.go":            "x\n",
		"/work/shop/web/AGENTS.md":       "web\n",
		"/work/shop/web/GNOSH.md":        "web main\n",
		"/work/shop/web/y.ts":            "y\n",
		"/work/shop/lib/.gnosh/GNOSH.md": "lib main\n",
		"/work/shop/lib/AGENTS.md":       "lib\n",
		"/work/shop/lib/z.go":            "z\n",
	}
	o := Options{
		Cwd:           "/work/shop",
		UserFiles:     []string{"/home/ana/GNOSH.md"},
		ProjectNames:  []string{"GNOSH.md"},
		LocalNames:    []string{"GNOSH.local.md"},
		FallbackNames: []string{"AGENTS.md"},
		DotDirs:       []string{".gnosh"},
		Trusted:       true,
		FS:            tree(files),
	}

	// No main file from the top of the disk to the working directory: the
	// fallback files stand in. The person's own file and the rules do not
	// count as main files.
	s := load(t, o)
	same(t, "no main file", brief(s.Files()), []string{
		"/home/ana/GNOSH.md User main session_start",
		"/AGENTS.md Project fallback session_start",
		"/work/AGENTS.md Project fallback session_start",
		"/work/shop/AGENTS.md Project fallback session_start",
		"/work/shop/.gnosh/AGENTS.md Project fallback session_start",
		"/work/shop/.gnosh/rules/r.md Project rule session_start",
	})
	// Below the working directory each directory is judged by itself.
	late, _ := touch(s, "/work/shop/api/x.go")
	same(t, "a directory with only a fallback file", brief(late), []string{
		"/work/shop/api/AGENTS.md Project fallback nested_traversal",
	})
	late, _ = touch(s, "/work/shop/web/y.ts")
	same(t, "a directory with a main file", brief(late), []string{
		"/work/shop/web/GNOSH.md Project main nested_traversal",
	})
	late, _ = touch(s, "/work/shop/lib/z.go")
	same(t, "a directory with a main file in its dot directory", brief(late), []string{
		"/work/shop/lib/.gnosh/GNOSH.md Project main nested_traversal",
	})

	// One main file anywhere on the way down, here a private one two
	// levels up, and no fallback file is read at all, not even below.
	for _, main := range []string{"/work/GNOSH.local.md", "/work/shop/GNOSH.md", "/work/shop/.gnosh/GNOSH.md", "/GNOSH.md"} {
		with := map[string]string{main: "main\n"}
		for p, c := range files {
			with[p] = c
		}
		o.FS = tree(with)
		s := load(t, o)
		for _, f := range s.Files() {
			if f.Kind == KindFallback {
				t.Errorf("with %s there, %s was still read", main, f.Path)
			}
		}
		if late, _ := touch(s, "/work/shop/api/x.go"); len(late) != 0 {
			t.Errorf("with %s there, touching api gave %v", main, brief(late))
		}
	}
}

func TestFallbackAlsoAndNever(t *testing.T) {
	o := Options{
		Cwd:           "/work/shop",
		ProjectNames:  []string{"GNOSH.md"},
		LocalNames:    []string{"GNOSH.local.md"},
		FallbackNames: []string{"AGENTS.md"},
		Fallback:      FallbackAlso,
		DotDirs:       []string{".gnosh"},
		Trusted:       true,
		FS: tree(map[string]string{
			"/work/AGENTS.md":              "work\n",
			"/work/shop/GNOSH.md":          "shop main\n",
			"/work/shop/AGENTS.md":         "shop\n",
			"/work/shop/GNOSH.local.md":    "shop private\n",
			"/work/shop/.gnosh/rules/r.md": "rule\n",
			"/work/shop/api/AGENTS.md":     "api\n",
			"/work/shop/api/GNOSH.md":      "api main\n",
			"/work/shop/api/x.go":          "x\n",
		}),
	}
	s := load(t, o)
	same(t, "also", brief(s.Files()), []string{
		"/work/AGENTS.md Project fallback session_start",
		"/work/shop/GNOSH.md Project main session_start",
		"/work/shop/AGENTS.md Project fallback session_start",
		"/work/shop/.gnosh/rules/r.md Project rule session_start",
		"/work/shop/GNOSH.local.md Local main session_start",
	})
	late, _ := touch(s, "/work/shop/api/x.go")
	same(t, "also, below", brief(late), []string{
		"/work/shop/api/GNOSH.md Project main nested_traversal",
		"/work/shop/api/AGENTS.md Project fallback nested_traversal",
	})

	o.Fallback = FallbackNever
	s = load(t, o)
	for _, f := range s.Files() {
		if f.Kind == KindFallback {
			t.Errorf("never: %s was read", f.Path)
		}
	}
}

func TestFallbackFileReachedAnotherWayIsReadOnce(t *testing.T) {
	// A main file that imports the fallback file, and a main file that is
	// a link to it: either way the text is given once.
	for name, main := range map[string]string{"import": "See @AGENTS.md\n", "link": "-> AGENTS.md"} {
		o := Options{
			Cwd:           "/work/shop",
			ProjectNames:  []string{"CLAUDE.md"},
			FallbackNames: []string{"AGENTS.md"},
			Fallback:      FallbackAlso,
			Trusted:       true,
			FS: tree(map[string]string{
				"/work/shop/CLAUDE.md": main,
				"/work/shop/AGENTS.md": "agents\n",
			}),
		}
		s := load(t, o)
		n := 0
		for _, f := range s.Files() {
			if f.RealPath == "/work/shop/AGENTS.md" {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s: AGENTS.md loaded %d times: %v", name, n, brief(s.Files()))
		}
		if name == "link" {
			same(t, "link", brief(s.Files()), []string{"/work/shop/CLAUDE.md Project main session_start"})
			if got := s.Files()[0].Text; got != "agents\n" {
				t.Errorf("the link's text: %q", got)
			}
		}
	}
}

func TestSizeLimit(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"A.md", "B.md", "C.md"},
		MaxBytes:     10,
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/A.md":   "0123456789",  // at the limit
			"/p/B.md":   "0123456789x", // one over
			"/p/C.md":   "@big.md\n",   // small, but what it imports is not
			"/p/big.md": strings.Repeat("y", 50),
		}),
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/A.md", "/p/C.md"})
	same(t, "warnings", warned(s.Warnings()), []string{"too_big /p/B.md", "too_big /p/big.md"})
	if w := s.Warnings()[0]; !strings.Contains(w.Text, "/p/B.md is 11 bytes, over the 10 bytes limit") {
		t.Errorf("warning text: %s", w)
	}
	if w := s.Warnings()[1]; w.From != "/p/C.md" {
		t.Errorf("the import's warning names its importer: %+v", w)
	}

	// The default limit is 4 MiB.
	big := strings.Repeat("z", DefaultMaxBytes)
	o.MaxBytes = 0
	o.FS = tree(map[string]string{"/p/A.md": big, "/p/B.md": big + "z"})
	s = load(t, o)
	same(t, "files at 4 MiB", paths(s.Files()), []string{"/p/A.md"})
	same(t, "warnings at 4 MiB", warned(s.Warnings()), []string{"too_big /p/B.md"})
	if w := s.Warnings()[0]; !strings.Contains(w.Text, "4.0 MiB") {
		t.Errorf("warning text: %s", w)
	}
}

func TestTextIsCleaned(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"latin.md", "binary.md", "crlf.md", "bom.md", "old-mac.md"},
		DotDirs:      []string{".gnosh"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/latin.md":            "caf\xe9 au lait\n",
			"/p/binary.md":           "PK\x03\x04\x00\x00",
			"/p/crlf.md":             "one\r\n@docs/a.md\r\ntwo\r\n",
			"/p/docs/a.md":           "imported\r\n",
			"/p/bom.md":              "\xef\xbb\xbf# Title\n",
			"/p/old-mac.md":          "a\rb\r",
			"/p/.gnosh/rules/win.md": "---\r\npaths: \"*.go\"\r\n---\r\nrule body\r\n",
			"/p/x.go":                "x\n",
		}),
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/latin.md", "/p/crlf.md", "/p/docs/a.md", "/p/bom.md", "/p/old-mac.md"})
	same(t, "warnings", warned(s.Warnings()), []string{"invalid_utf8 /p/latin.md", "not_text /p/binary.md"})
	for p, want := range map[string]string{
		"/p/latin.md":   "caf� au lait\n",
		"/p/crlf.md":    "one\n@docs/a.md\ntwo\n",
		"/p/docs/a.md":  "imported\n",
		"/p/bom.md":     "# Title\n",
		"/p/old-mac.md": "a\nb\n",
	} {
		if got := find(t, s.Files(), p).Text; got != want {
			t.Errorf("%s: text %q, want %q", p, got, want)
		}
	}
	// The frontmatter of a file with Windows line ends is read like any
	// other.
	late, _ := touch(s, "/p/x.go")
	same(t, "rule", paths(late), []string{"/p/.gnosh/rules/win.md"})
	if late[0].Text != "rule body\n" {
		t.Errorf("rule text %q", late[0].Text)
	}
}

func TestHTMLCommentsAreRemoved(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/CLAUDE.md": "# Rules\n<!-- for maintainers: @hidden.md -->\nKeep it short.\n",
			"/p/hidden.md": "an import inside a comment is not followed\n",
		}),
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/CLAUDE.md"})
	if got := s.Files()[0].Text; got != "# Rules\nKeep it short.\n" {
		t.Errorf("text %q", got)
	}
}

func TestNotAFile(t *testing.T) {
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md", "GNOSH.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/CLAUDE.md/inside.txt": "CLAUDE.md is a directory here\n",
			"/p/GNOSH.md":             "see @docs and @docs/\n",
			"/p/docs/a.md":            "a\n",
		}),
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/GNOSH.md"})
	same(t, "warnings", warned(s.Warnings()), []string{"not_file /p/CLAUDE.md", "not_file /p/docs"})
}

func TestOwnershipAboveTheProject(t *testing.T) {
	mine := func(info fs.FileInfo) bool { return info.Sys() != "stranger" }
	m := fstest.MapFS{
		"tmp/CLAUDE.md":                    {Data: []byte("planted\n"), Mode: 0o644, Sys: "stranger"},
		"tmp/.claude/rules/r.md":           {Data: []byte("planted rule\n"), Mode: 0o644, Sys: "stranger"},
		"tmp/work/CLAUDE.md":               {Data: []byte("mine, above the project\n"), Mode: 0o644},
		"tmp/work/GNOSH.md":                {Data: []byte("../mine.md"), Mode: fs.ModeSymlink | 0o777, Sys: "stranger"},
		"tmp/mine.md":                      {Data: []byte("mine, but the link to it is not\n"), Mode: 0o644},
		"tmp/work/shop/CLAUDE.md":          {Data: []byte("in the project, whoever owns it\n"), Mode: 0o644, Sys: "stranger"},
		"tmp/work/shop/.claude/rules/r.md": {Data: []byte("project rule\n"), Mode: 0o644, Sys: "stranger"},
	}
	// A stranger's link to a directory of the person's own: the files in it
	// are the person's, but the stranger chose them.
	m["tmp/.shared"] = &fstest.MapFile{Data: []byte("mine"), Mode: fs.ModeSymlink | 0o777, Sys: "stranger"}
	m["tmp/mine/CLAUDE.md"] = &fstest.MapFile{Data: []byte("mine, picked by a stranger\n"), Mode: 0o644}
	m["tmp/mine/rules/r.md"] = &fstest.MapFile{Data: []byte("my rule, picked by a stranger\n"), Mode: 0o644}
	o := Options{
		Cwd:          "/tmp/work/shop",
		ProjectNames: []string{"CLAUDE.md", "GNOSH.md"},
		DotDirs:      []string{".claude", ".shared"},
		Owned:        mine,
		Trusted:      true,
		FS:           m,
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{
		"/tmp/work/CLAUDE.md",
		"/tmp/work/shop/CLAUDE.md",
		"/tmp/work/shop/.claude/rules/r.md",
	})
	same(t, "warnings", warned(s.Warnings()), []string{
		"not_owned /tmp/CLAUDE.md",
		"not_owned /tmp/.shared/CLAUDE.md",
		"not_owned /tmp/.claude/rules/r.md",
		"not_owned /tmp/.shared/rules",
		"not_owned /tmp/work/GNOSH.md",
	})

	// A stranger's file above the project does not count as a main file
	// either: it cannot switch the project's fallback files off.
	o.ProjectNames = []string{"CLAUDE.md"}
	o.FallbackNames = []string{"AGENTS.md"}
	o.DotDirs = nil
	o.FS = fstest.MapFS{
		"tmp/CLAUDE.md":      {Data: []byte("planted\n"), Mode: 0o644, Sys: "stranger"},
		"tmp/work/AGENTS.md": {Data: []byte("the project's own\n"), Mode: 0o644},
	}
	o.Cwd = "/tmp/work"
	s = load(t, o)
	same(t, "fallback survives", paths(s.Files()), []string{"/tmp/work/AGENTS.md"})
}

func TestExclude(t *testing.T) {
	o := shopOptions()
	o.Exclude = []string{"/work/shop/.claude", "CLAUDE.local.md", "/home/ana/**/tone.md", "/work/GNOSH.md"}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{
		"/home/ana/.config/gnosh/GNOSH.md",
		"/GNOSH.md",
		"/work/shop/GNOSH.md",
		"/work/shop/.gnosh/GNOSH.md",
		"/work/shop/CLAUDE.md",
		"/work/shop/.gnosh/rules/a.md",
		"/work/shop/.gnosh/rules/sub/b.md",
		"/work/shop/api/GNOSH.md",
	})

	// An excluded file is not loaded as an import either, and not through
	// a link that leads to it.
	o = Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md", "LINK.md"},
		Exclude:      []string{"/p/secret.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/CLAUDE.md": "@secret.md\n",
			"/p/LINK.md":   "-> secret.md",
			"/p/secret.md": "left out\n",
		}),
	}
	s = load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/CLAUDE.md"})
	same(t, "warnings", warned(s.Warnings()), nil)

	// An excluded main file is as good as absent: it does not stand in the
	// way of the fallback files.
	o = Options{
		Cwd:           "/p",
		ProjectNames:  []string{"CLAUDE.md"},
		FallbackNames: []string{"AGENTS.md"},
		Exclude:       []string{"/p/CLAUDE.md"},
		Trusted:       true,
		FS:            tree(map[string]string{"/p/CLAUDE.md": "left out\n", "/p/AGENTS.md": "agents\n"}),
	}
	s = load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/AGENTS.md"})
}

func TestOptionsThatMakeNoSense(t *testing.T) {
	ok := Options{Cwd: "/p", FS: tree(map[string]string{"/p/x": "x", "/q/y": "y"})}
	for name, change := range map[string]func(*Options){
		"no working directory":        func(o *Options) { o.Cwd = "" },
		"relative working directory":  func(o *Options) { o.Cwd = "p" },
		"relative project root":       func(o *Options) { o.ProjectRoot = "p" },
		"root beside the cwd":         func(o *Options) { o.ProjectRoot = "/q" },
		"root below the cwd":          func(o *Options) { o.ProjectRoot = "/p/x" },
		"relative home":               func(o *Options) { o.Home = "home" },
		"relative user file":          func(o *Options) { o.UserFiles = []string{"GNOSH.md"} },
		"relative project file":       func(o *Options) { o.ProjectFiles = []string{"docs/AI.md"} },
		"empty name":                  func(o *Options) { o.ProjectNames = []string{""} },
		"absolute name":               func(o *Options) { o.LocalNames = []string{"/etc/passwd"} },
		"name that climbs":            func(o *Options) { o.FallbackNames = []string{"../AGENTS.md"} },
		"dot directory that is dot":   func(o *Options) { o.DotDirs = []string{"."} },
		"rules directory that climbs": func(o *Options) { o.RulesDir = ".." },
		"unknown fallback policy":     func(o *Options) { o.Fallback = FallbackPolicy(7) },
		"exclude pattern":             func(o *Options) { o.Exclude = []string{"docs/[abc"} },
	} {
		o := ok
		change(&o)
		if _, err := Load(context.Background(), o); err == nil {
			t.Errorf("%s: no error", name)
		} else if !strings.HasPrefix(err.Error(), "instructions: ") {
			t.Errorf("%s: error %q does not say where it is from", name, err)
		}
	}
	if _, err := Load(context.Background(), ok); err != nil {
		t.Errorf("the plain options: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Load(ctx, ok); err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("a cancelled context: %v", err)
	}
}

func TestHookValues(t *testing.T) {
	// These strings are what the InstructionsLoaded hook is sent.
	got := []string{
		string(ScopeUser), string(ScopeProject), string(ScopeLocal),
		string(ReasonStart), string(ReasonNested), string(ReasonRuleMatch), string(ReasonImport),
	}
	same(t, "values", got, []string{"User", "Project", "Local", "session_start", "nested_traversal", "path_glob_match", "include"})
}

func TestListedFilesAndFolders(t *testing.T) {
	o := Options{
		Cwd:          "/work/shop/api",
		ProjectRoot:  "/work/shop",
		UserFiles:    []string{"/home/ana/first.md", "/home/ana/more", "/home/ana/gone", "/home/ana/last.md"},
		ProjectNames: []string{"CLAUDE.md"},
		ProjectFiles: []string{"/work/shop/docs/AI.md", "/team/notes.md", "/team/rules", "/nowhere/x.md"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/home/ana/first.md":      "first\n",
			"/home/ana/more/a.md":     "a\n",
			"/home/ana/more/go.md":    "---\npaths: \"*.go\"\n---\nthe person's Go rule\n",
			"/home/ana/more/sub/b.md": "b\n",
			"/home/ana/more/c.txt":    "not Markdown\n",
			"/home/ana/last.md":       "last\n",
			"/work/shop/CLAUDE.md":    "shop\n",
			"/work/shop/docs/AI.md":   "ai; @more.md @/team/secret.md\n",
			"/work/shop/docs/more.md": "more\n",
			"/team/notes.md":          "team notes\n",
			"/team/secret.md":         "secret\n",
			"/team/rules/t.md":        "team rule\n",
			"/team/rules/ts.md":       "---\npaths: \"web/**\"\n---\nthe team's rule for the web\n",
			"/work/shop/web/app.ts":   "",
			"/work/shop/api/x.go":     "",
		}),
	}
	s := load(t, o)
	// The person's list is read in the order given, a folder where it
	// stands. The project's extra files come after the directories. A file
	// an option names is read wherever it lies, but what it imports is
	// held to the project.
	same(t, "files", brief(s.Files()), []string{
		"/home/ana/first.md User main session_start",
		"/home/ana/more/a.md User rule session_start",
		"/home/ana/more/sub/b.md User rule session_start",
		"/home/ana/last.md User main session_start",
		"/work/shop/CLAUDE.md Project main session_start",
		"/work/shop/docs/AI.md Project main session_start",
		"/work/shop/docs/more.md Project import include",
		"/team/notes.md Project main session_start",
		"/team/rules/t.md Project rule session_start",
	})
	same(t, "warnings", warned(s.Warnings()), []string{"import_refused /team/secret.md"})

	// A rule from a listed folder is read against the project root; one
	// from the person's folder against the working directory.
	late, _ := touch(s, "/work/shop/web/app.ts")
	same(t, "the web", brief(late), []string{"/team/rules/ts.md Project rule path_glob_match"})
	if late[0].Base != "/work/shop" {
		t.Errorf("base %q", late[0].Base)
	}
	late, _ = touch(s, "/work/shop/api/x.go")
	same(t, "a Go file", brief(late), []string{"/home/ana/more/go.md User rule path_glob_match"})

	o.Trusted = false
	s = load(t, o)
	same(t, "not trusted", paths(s.Files()), []string{"/home/ana/first.md", "/home/ana/more/a.md", "/home/ana/more/sub/b.md", "/home/ana/last.md"})
}

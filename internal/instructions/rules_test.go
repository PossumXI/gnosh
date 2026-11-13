package instructions

import (
	"slices"
	"strings"
	"testing"
)

func rulesOptions() Options {
	return Options{
		Cwd:         "/work/shop/api",
		ProjectRoot: "/work/shop",
		UserFiles:   []string{"/home/ana/rules"},
		DotDirs:     []string{".gnosh"},
		Trusted:     true,
		FS: tree(map[string]string{
			"/home/ana/rules/always.md":               "user always\n",
			"/home/ana/rules/md.md":                   "---\npaths: \"*.md\"\n---\nuser rule for markdown\n",
			"/work/shop/.gnosh/rules/all.md":          "---\ndescription: no paths here\n---\n\nproject always\n",
			"/work/shop/.gnosh/rules/go.md":           "---\npaths:\n  - \"api/**/*.go\"\n---\n\n# Go\ngo rule, see @../../docs/go.md\n",
			"/work/shop/.gnosh/rules/top.md":          "---\npaths: /*.go\n---\nonly at the top\n",
			"/work/shop/.gnosh/rules/web/ts.md":       "---\npaths: [\"web/**/*.{ts,tsx}\"]\n---\nts rule\n",
			"/work/shop/api/.gnosh/rules/handlers.md": "---\npaths: handlers/*.go\n---\nhandlers rule\n",
			"/work/shop/docs/go.md":                   "go doc\n",
			"/work/shop/api/handlers/user.go":         "",
			"/work/shop/api/handlers/other.go":        "",
			"/work/shop/api/README.md":                "",
			"/work/shop/README.md":                    "",
			"/work/shop/main.go":                      "",
			"/work/shop/web/src/app.tsx":              "",
		}),
	}
}

func TestRulesWithAndWithoutPaths(t *testing.T) {
	s := load(t, rulesOptions())
	// Only the rules with no path patterns are in effect from the start.
	same(t, "at the start", brief(s.Files()), []string{
		"/home/ana/rules/always.md User rule session_start",
		"/work/shop/.gnosh/rules/all.md Project rule session_start",
	})
	if got := find(t, s.Files(), "/work/shop/.gnosh/rules/all.md").Text; got != "project always\n" {
		t.Errorf("the frontmatter is taken off: %q", got)
	}

	// A Go file under api: the project's Go rule (with what it imports)
	// and the rule kept in api's own dot directory.
	late, warns := touch(s, "/work/shop/api/handlers/user.go")
	same(t, "a handler", brief(late), []string{
		"/work/shop/.gnosh/rules/go.md Project rule path_glob_match",
		"/work/shop/docs/go.md Project import include",
		"/work/shop/api/.gnosh/rules/handlers.md Project rule path_glob_match",
	})
	same(t, "warnings", warned(warns), nil)
	g := late[0]
	if !slices.Equal(g.Globs, []string{"api/**/*.go"}) || g.Trigger != "/work/shop/api/handlers/user.go" || g.Base != "/work/shop" || !g.Late {
		t.Errorf("the matched rule: %+v", g)
	}
	if g.Text != "# Go\ngo rule, see @../../docs/go.md\n" {
		t.Errorf("the matched rule's text: %q", g.Text)
	}
	if h := late[2]; h.Base != "/work/shop/api" || !slices.Equal(h.Globs, []string{"handlers/*.go"}) {
		t.Errorf("the rule from the working directory: %+v", h)
	}
	if d := late[1]; !d.Late || d.Trigger != "/work/shop/api/handlers/user.go" || d.Parent != "/work/shop/.gnosh/rules/go.md" {
		t.Errorf("the rule's import: %+v", d)
	}

	// Each rule is given once. Nothing matches a file outside every base,
	// or a directory.
	for _, p := range []string{"/work/shop/api/handlers/user.go", "/work/shop/api/handlers/other.go", "/elsewhere/x.go", "/work/shop/api/handlers"} {
		if again, _ := touch(s, p); len(again) != 0 {
			t.Errorf("touching %s gave %v", p, brief(again))
		}
	}

	// A pattern is read against the directory that holds the rule's dot
	// directory, wherever the session works.
	late, _ = touch(s, "/work/shop/web/src/app.tsx")
	same(t, "a file beside the working directory", paths(late), []string{"/work/shop/.gnosh/rules/web/ts.md"})
	late, _ = touch(s, "/work/shop/main.go")
	same(t, "a file at the top", paths(late), []string{"/work/shop/.gnosh/rules/top.md"})

	// The person's own rule is read against the working directory.
	if late, _ = touch(s, "/work/shop/README.md"); len(late) != 0 {
		t.Errorf("a file above the working directory matched the person's rule: %v", brief(late))
	}
	late, _ = touch(s, "/work/shop/api/README.md")
	same(t, "the person's rule", brief(late), []string{"/home/ana/rules/md.md User rule path_glob_match"})
	if late[0].Base != "/work/shop/api" {
		t.Errorf("the person's rule speaks for the working directory, got %q", late[0].Base)
	}

	same(t, "everything, in order", paths(s.Files()), []string{
		"/home/ana/rules/always.md",
		"/work/shop/.gnosh/rules/all.md",
		"/work/shop/.gnosh/rules/go.md",
		"/work/shop/docs/go.md",
		"/work/shop/api/.gnosh/rules/handlers.md",
		"/work/shop/.gnosh/rules/web/ts.md",
		"/work/shop/.gnosh/rules/top.md",
		"/home/ana/rules/md.md",
	})
}

func TestRuleHeadersThatCannotBeRead(t *testing.T) {
	o := Options{
		Cwd:     "/p",
		DotDirs: []string{".gnosh"},
		Trusted: true,
		FS: tree(map[string]string{
			"/p/.gnosh/rules/badpat.md": "---\npaths:\n  - \"src/[ab\"\n  - \"ok/*.go\"\n---\nbad pattern body\n",
			"/p/.gnosh/rules/broken.md": "---\npaths: [src/**\n---\nbroken body\n",
			"/p/.gnosh/rules/never.md":  "---\npaths: \"[\"\n---\nnever\n",
			"/p/ok/x.go":                "",
			"/p/src/a":                  "",
		}),
	}
	s := load(t, o)
	// A header that cannot be read costs the rule its scope, not its text.
	same(t, "files", paths(s.Files()), []string{"/p/.gnosh/rules/broken.md"})
	if got := s.Files()[0].Text; got != "---\npaths: [src/**\n---\nbroken body\n" {
		t.Errorf("the whole file is kept: %q", got)
	}
	same(t, "warnings", warned(s.Warnings()), []string{
		"pattern /p/.gnosh/rules/badpat.md",
		"frontmatter /p/.gnosh/rules/broken.md",
		"pattern /p/.gnosh/rules/never.md",
	})
	if w := s.Warnings()[1]; !strings.Contains(w.Text, "line 2") || !strings.Contains(w.Text, "applies to every file") {
		t.Errorf("frontmatter warning: %s", w)
	}
	if w := s.Warnings()[0]; !strings.Contains(w.Text, "src/[ab") || !strings.Contains(w.Text, "matches nothing") {
		t.Errorf("pattern warning: %s", w)
	}

	// A pattern that cannot be read matches nothing; the others still do.
	late, _ := touch(s, "/p/src/a")
	same(t, "the bad pattern", paths(late), nil)
	late, _ = touch(s, "/p/ok/x.go")
	same(t, "the good pattern", paths(late), []string{"/p/.gnosh/rules/badpat.md"})
}

func TestRuleImportedByName(t *testing.T) {
	// A file that imports a rule gets it at once and whole; the rule is
	// not given again when its pattern matches.
	o := Options{
		Cwd:          "/p",
		ProjectNames: []string{"CLAUDE.md"},
		DotDirs:      []string{".claude"},
		Trusted:      true,
		FS: tree(map[string]string{
			"/p/CLAUDE.md":           "@.claude/rules/go.md\n",
			"/p/.claude/rules/go.md": "---\npaths: \"*.go\"\n---\ngo rule\n",
			"/p/x.go":                "",
		}),
	}
	s := load(t, o)
	same(t, "files", brief(s.Files()), []string{
		"/p/CLAUDE.md Project main session_start",
		"/p/.claude/rules/go.md Project import include",
	})
	if late, _ := touch(s, "/p/x.go"); len(late) != 0 {
		t.Errorf("given twice: %v", brief(late))
	}
}

func TestRulesDirectoryName(t *testing.T) {
	o := Options{
		Cwd:      "/p",
		DotDirs:  []string{".gnosh"},
		RulesDir: "standing",
		Trusted:  true,
		FS: tree(map[string]string{
			"/p/.gnosh/standing/a.md": "a\n",
			"/p/.gnosh/rules/b.md":    "b\n",
		}),
	}
	s := load(t, o)
	same(t, "files", paths(s.Files()), []string{"/p/.gnosh/standing/a.md"})
}

package ignore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// verdict is what one ignore file says about a path.
type verdict int

const (
	silent  verdict = iota // no pattern matches
	ignores                // the last matching pattern ignores it
	takesIn                // the last matching pattern is a "!" pattern
)

func (v verdict) String() string {
	return [...]string{"says nothing", "ignores", "takes back in"}[v]
}

func fileVerdict(f *File, rel string, isDir bool) verdict {
	ignored, decided := f.Match(rel, isDir)
	switch {
	case !decided:
		return silent
	case ignored:
		return ignores
	}
	return takesIn
}

func TestFileMatch(t *testing.T) {
	type check struct {
		path  string
		isDir bool
		want  verdict
	}
	tests := []struct {
		name   string
		text   string
		checks []check
	}{
		{
			name: "blank lines and comments match nothing",
			text: "\n# a comment\n   \n#foo\n",
			checks: []check{
				{"foo", false, silent},
				{"#foo", false, silent},
				{"# a comment", false, silent},
			},
		},
		{
			name: "a backslash before the first hash makes it a pattern",
			text: "\\#foo\n",
			checks: []check{
				{"#foo", false, ignores},
				{"foo", false, silent},
			},
		},
		{
			name: "trailing spaces are dropped unless a backslash protects them",
			text: "foo   \nbar\\ \nbaz\\  \nmid dle  \n",
			checks: []check{
				{"foo", false, ignores},
				{"foo   ", false, silent},
				{"bar ", false, ignores},
				{"bar", false, silent},
				{"baz ", false, ignores},
				{"baz  ", false, silent},
				{"mid dle", false, ignores},
			},
		},
		{
			name: "leading spaces are part of the pattern",
			text: " lead\n",
			checks: []check{
				{" lead", false, ignores},
				{"lead", false, silent},
			},
		},
		{
			name: "an exclamation mark takes a path back in; the last match decides",
			text: "*.html\n!foo.html\n",
			checks: []check{
				{"gitignore.html", false, ignores},
				{"foo.html", false, takesIn},
				{"sub/foo.html", false, takesIn},
				{"foo.txt", false, silent},
			},
		},
		{
			name: "a later pattern overrules an earlier one",
			text: "!keep.log\n*.log\n",
			checks: []check{
				{"keep.log", false, ignores},
			},
		},
		{
			name: "a backslash before the first exclamation mark makes it a pattern",
			text: "\\!important!.txt\n",
			checks: []check{
				{"!important!.txt", false, ignores},
				{"important!.txt", false, silent},
			},
		},
		{
			name: "a lone exclamation mark or slash is no pattern",
			text: "!\n/\n!/\n",
			checks: []check{
				{"!", false, silent},
				{"a", true, silent},
			},
		},
		{
			name: "a slash at the end matches directories only",
			text: "frotz/\n",
			checks: []check{
				{"frotz", true, ignores},
				{"a/frotz", true, ignores},
				{"frotz", false, silent},
				{"a/frotz", false, silent},
			},
		},
		{
			name: "a slash in the middle ties the pattern to the file's directory",
			text: "doc/frotz/\n",
			checks: []check{
				{"doc/frotz", true, ignores},
				{"a/doc/frotz", true, silent},
				{"doc/frotz", false, silent},
			},
		},
		{
			name: "doc/frotz and /doc/frotz mean the same",
			text: "/doc/frotz\n",
			checks: []check{
				{"doc/frotz", false, ignores},
				{"doc/frotz", true, ignores},
				{"a/doc/frotz", false, silent},
			},
		},
		{
			name: "a pattern without a slash matches at any depth",
			text: "hello.*\n",
			checks: []check{
				{"hello.txt", false, ignores},
				{"hello.c", false, ignores},
				{"a/hello.java", false, ignores},
				{"a/b/hello.d", true, ignores},
				{"hello", false, silent},
				{"xhello.c", false, silent},
			},
		},
		{
			name: "a leading slash keeps it to the file's own directory",
			text: "/hello.*\n",
			checks: []check{
				{"hello.txt", false, ignores},
				{"hello.c", false, ignores},
				{"a/hello.java", false, silent},
			},
		},
		{
			name: "a star does not cross a slash",
			text: "foo/*\n",
			checks: []check{
				{"foo/test.json", false, ignores},
				{"foo/bar", true, ignores},
				{"foo/bar/hello.c", false, silent},
				{"foo", true, silent},
				{"x/foo/test.json", false, silent},
			},
		},
		{
			name: "a question mark and a range match one character",
			text: "file?.txt\n*.[oa]\nlog[0-9]\n[!a-y]zz\n",
			checks: []check{
				{"file1.txt", false, ignores},
				{"file12.txt", false, silent},
				{"file.txt", false, silent},
				{"file.o", false, ignores},
				{"lib.a", false, ignores},
				{"src/internal.o", false, ignores},
				{"main.c", false, silent},
				{"log7", false, ignores},
				{"logx", false, silent},
				{"zzz", false, ignores},
				{"azz", false, silent},
			},
		},
		{
			name: "two stars and a slash at the start match in all directories",
			text: "**/foo\n",
			checks: []check{
				{"foo", false, ignores},
				{"foo", true, ignores},
				{"a/foo", false, ignores},
				{"a/b/foo", true, ignores},
				{"a/foo/b", false, silent},
				{"xfoo", false, silent},
			},
		},
		{
			name: "**/foo/bar matches bar directly under any foo",
			text: "**/foo/bar\n",
			checks: []check{
				{"foo/bar", false, ignores},
				{"a/foo/bar", true, ignores},
				{"a/b/foo/bar", false, ignores},
				{"foo/x/bar", false, silent},
				{"bar", false, silent},
			},
		},
		{
			name: "a trailing /** matches everything inside",
			text: "abc/**\n",
			checks: []check{
				{"abc/x", false, ignores},
				{"abc/x/y/z", false, ignores},
				{"abc/d", true, ignores},
				{"abc", true, silent},
				{"abc", false, silent},
				{"x/abc/y", false, silent},
			},
		},
		{
			name: "/**/ matches zero or more directories",
			text: "a/**/b\n",
			checks: []check{
				{"a/b", false, ignores},
				{"a/x/b", false, ignores},
				{"a/x/y/b", true, ignores},
				{"a/x/y/c", false, silent},
				{"z/a/b", false, silent},
			},
		},
		{
			name: "other runs of stars are ordinary stars",
			text: "a**b\n***.x\nq/**z\n",
			checks: []check{
				{"ab", false, ignores},
				{"aXXb", false, ignores},
				{"sub/aXb", false, ignores},
				{"file.x", false, ignores},
				{"q/z", false, ignores},
				{"q/yz", false, ignores},
				{"q/y/z", false, silent},
			},
		},
		{
			name: "two stars alone match every name",
			text: "**\n",
			checks: []check{
				{"a", false, ignores},
				{"a/b", true, ignores},
			},
		},
		{
			name: "**/ matches every directory",
			text: "**/\n",
			checks: []check{
				{"a", true, ignores},
				{"a/b", true, ignores},
				{"a", false, silent},
			},
		},
		{
			name: "exclude everything except directory foo/bar",
			text: "# exclude everything except directory foo/bar\n/*\n!/foo\n/foo/*\n!/foo/bar\n",
			checks: []check{
				{"README", false, ignores},
				{"src", true, ignores},
				{"foo", true, takesIn},
				{"foo/other", true, ignores},
				{"foo/file", false, ignores},
				{"foo/bar", true, takesIn},
				{"foo/bar/deep.txt", false, silent},
			},
		},
		{
			name: "a pattern that cannot be parsed matches nothing",
			text: "[\nfoo[\nbar\\\n[[:nope:]]\nok\n",
			checks: []check{
				{"[", false, silent},
				{"foo[", false, silent},
				{"bar\\", false, silent},
				{"bar", false, silent},
				{"ok", false, ignores},
			},
		},
		{
			name: "Windows line endings and a byte order mark are not part of patterns",
			text: "\xef\xbb\xbffoo\r\nbar/\r\n!baz\r\n",
			checks: []check{
				{"foo", false, ignores},
				{"bar", true, ignores},
				{"bar", false, silent},
				{"baz", false, takesIn},
			},
		},
		{
			name: "the last line needs no line break",
			text: "one\ntwo",
			checks: []check{
				{"one", false, ignores},
				{"two", false, ignores},
			},
		},
		{
			name: "escaped wildcards are themselves",
			text: "\\*.txt\nwhy\\?\n\\[x\\]\n",
			checks: []check{
				{"*.txt", false, ignores},
				{"a.txt", false, silent},
				{"why?", false, ignores},
				{"whyX", false, silent},
				{"[x]", false, ignores},
				{"x", false, silent},
			},
		},
		{
			name: "character classes",
			text: "[[:digit:]]*.bak\n",
			checks: []check{
				{"1.bak", false, ignores},
				{"22x.bak", false, ignores},
				{"x.bak", false, silent},
			},
		},
		{
			name: "letter case matters",
			text: "Build/\n*.LOG\n",
			checks: []check{
				{"Build", true, ignores},
				{"build", true, silent},
				{"a.LOG", false, ignores},
				{"a.log", false, silent},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Parse([]byte(tt.text))
			for _, c := range tt.checks {
				if got := fileVerdict(f, c.path, c.isDir); got != c.want {
					t.Errorf("path %q (directory: %v): the file %s it, want %s", c.path, c.isDir, got, c.want)
				}
			}
		})
	}
}

func TestParseCountsPatterns(t *testing.T) {
	f := Parse([]byte("# c\n\na\n!b\n[\n/\n c \n"))
	if f.Len() != 3 {
		t.Fatalf("Len() = %d, want 3 (a, !b and \" c\")", f.Len())
	}
	if Parse(nil).Len() != 0 {
		t.Fatal("an empty file has patterns")
	}
}

// tree makes files under a new temp directory and returns its real path.
// A key ending in a slash makes a directory; any other key makes a file
// with the value as its text.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// ignoredBelow walks from the rules of a directory down to rel, the way a
// search would, and reports whether rel is ignored: it is when it or any
// directory on the way to it is.
func ignoredBelow(r *Rules, rel string, isDir bool) bool {
	parts := strings.Split(rel, "/")
	for i, name := range parts {
		last := i == len(parts)-1
		if r.Ignored(name, !last || isDir) {
			return true
		}
		if !last {
			r = r.Enter(name, nil)
		}
	}
	return false
}

type pathCheck struct {
	path    string
	isDir   bool
	ignored bool
}

func checkPaths(t *testing.T, r *Rules, checks []pathCheck) {
	t.Helper()
	for _, c := range checks {
		if got := ignoredBelow(r, c.path, c.isDir); got != c.ignored {
			t.Errorf("%q (directory: %v): ignored = %v, want %v", c.path, c.isDir, got, c.ignored)
		}
	}
}

func TestRulesManualExamples(t *testing.T) {
	t.Run("info/exclude and a nested file", func(t *testing.T) {
		root := tree(t, map[string]string{
			".git/info/exclude":        "# ignore objects and archives, anywhere in the tree.\n*.[oa]\n",
			"Documentation/.gitignore": "# ignore generated html files,\n*.html\n# except foo.html which is maintained by hand\n!foo.html\n",
		})
		checkPaths(t, For(root, Options{}), []pathCheck{
			{"Documentation/foo.html", false, false},
			{"Documentation/gitignore.html", false, true},
			{"file.o", false, true},
			{"lib.a", false, true},
			{"src/internal.o", false, true},
			{"src/internal.c", false, false},
			{"index.html", false, false},
		})
	})
	t.Run("a nested file overrules the one above", func(t *testing.T) {
		root := tree(t, map[string]string{
			".git/":                      "",
			".gitignore":                 "vmlinux*\n",
			"arch/foo/kernel/.gitignore": "!/vmlinux*\n",
		})
		checkPaths(t, For(root, Options{}), []pathCheck{
			{"vmlinux", false, true},
			{"arch/foo/vmlinux.bin", false, true},
			{"arch/foo/kernel/vmlinux.lds.S", false, false},
			// The nested pattern is tied to its own directory.
			{"arch/foo/kernel/sub/vmlinux.lds.S", false, true},
		})
	})
	t.Run("everything except foo/bar", func(t *testing.T) {
		root := tree(t, map[string]string{
			".git/":      "",
			".gitignore": "/*\n!/foo\n/foo/*\n!/foo/bar\n",
		})
		checkPaths(t, For(root, Options{}), []pathCheck{
			{"README", false, true},
			{"src", true, true},
			{"src/main.c", false, true},
			{"foo", true, false},
			{"foo/x", false, true},
			{"foo/bar", true, false},
			{"foo/bar/deep/file", false, false},
		})
	})
	t.Run("a file cannot be taken back in from an ignored directory", func(t *testing.T) {
		root := tree(t, map[string]string{
			".git/":      "",
			".gitignore": "build/\n!build/keep.txt\n",
		})
		checkPaths(t, For(root, Options{}), []pathCheck{
			{"build", true, true},
			{"build/keep.txt", false, true},
			{"build", false, false},
		})
	})
}

func TestRulesPrecedence(t *testing.T) {
	root := tree(t, map[string]string{
		".git/info/exclude": "*.tmp\nlocal/\n!never.o\n",
		".gitignore":        "*.o\n!keep.tmp\nout/\n",
		"a/.gitignore":      "!special.o\nmine\n",
		"a/b/.gitignore":    "special.o\n/only-here\n",
		"a/b/c/":            "",
	})
	r := For(root, Options{})
	checkPaths(t, r, []pathCheck{
		// The top .gitignore.
		{"x.o", false, true},
		{"a/x.o", false, true},
		{"out", true, true},
		{"out/anything", false, true},
		// A nested file wins over the ones above it.
		{"a/special.o", false, false},
		{"a/b/special.o", false, true},
		{"a/b/c/special.o", false, true},
		{"a/mine", false, true},
		{"mine", false, false},
		{"a/b/mine", false, true},
		{"a/b/only-here", false, true},
		{"a/b/c/only-here", false, false},
		// Any .gitignore wins over info/exclude, wherever it is.
		{"x.tmp", false, true},
		{"keep.tmp", false, false},
		{"a/b/keep.tmp", false, false},
		{"never.o", false, true},
		{"local", true, true},
		{"local", false, false},
	})
}

func TestRulesStartBelowTheTop(t *testing.T) {
	root := tree(t, map[string]string{
		".git/info/exclude": "*.tmp\n",
		".gitignore":        "*.o\n/top-only\nsub/deep/gone\n",
		"sub/.gitignore":    "!kept.o\n",
		"sub/deep/":         "",
	})
	// A search that starts in a subdirectory still sees the rules above
	// it, with paths tied to the directories the rules came from.
	r := For(filepath.Join(root, "sub", "deep"), Options{})
	if r.Dir() != filepath.Join(root, "sub", "deep") {
		t.Fatalf("Dir() = %q", r.Dir())
	}
	checkPaths(t, r, []pathCheck{
		{"x.o", false, true},
		{"kept.o", false, false},
		{"x.tmp", false, true},
		{"gone", false, true},
		{"top-only", false, false},
		{"fine.c", false, false},
	})
}

func TestRulesOutsideARepository(t *testing.T) {
	root := tree(t, map[string]string{
		".gitignore":      "*\n",
		"proj/.gitignore": "*.log\n",
		"proj/sub/":       "",
	})
	// Nothing above the starting directory is read when there is no
	// repository: the "*" one level up must not hide the project.
	r := For(filepath.Join(root, "proj"), Options{})
	checkPaths(t, r, []pathCheck{
		{"main.go", false, false},
		{"debug.log", false, true},
		{"sub/debug.log", false, true},
	})
	// Starting at the top, its own file applies.
	checkPaths(t, For(root, Options{}), []pathCheck{
		{"proj", true, true},
		{"anything", false, true},
	})
}

func TestRulesNestedRepositoryStartsAfresh(t *testing.T) {
	root := tree(t, map[string]string{
		".git/":                   "",
		".gitignore":              "*.log\nsecret\n",
		"inner/.git/info/exclude": "*.bak\n",
		"inner/.gitignore":        "tmp/\n",
		"inner/sub/":              "",
		"plain/":                  "",
	})
	outer := For(root, Options{})
	checkPaths(t, outer, []pathCheck{
		{"a.log", false, true},
		{"plain/a.log", false, true},
		// Inside the inner repository only its own rules apply.
		{"inner/a.log", false, false},
		{"inner/secret", false, false},
		{"inner/sub/a.log", false, false},
		{"inner/tmp", true, true},
		{"inner/sub/x.bak", false, true},
		{"plain/x.bak", false, false},
	})
	// Starting inside the inner repository gives the same answers.
	inner := For(filepath.Join(root, "inner", "sub"), Options{})
	checkPaths(t, inner, []pathCheck{
		{"a.log", false, false},
		{"x.bak", false, true},
		{"tmp", true, true},
	})
}

func TestRulesGitFile(t *testing.T) {
	// A linked worktree has a .git file that names its own git directory,
	// and that directory names the main one, where info/exclude lives.
	root := tree(t, map[string]string{
		"main/.git/info/exclude":           "*.cache\n",
		"main/.git/worktrees/wt/commondir": "../..\n",
		"wt/.gitignore":                    "*.o\n",
		"wt/src/":                          "",
		// A submodule's .git file points at a directory with its own
		// info/exclude and no commondir.
		"super/.git/modules/sub/info/exclude": "*.gen\n",
		"super/sub/x/":                        "",
		// A .git file that says nothing useful does not make a repository.
		"odd/.gitignore":        "*.o\n",
		"odd/inner/.git":        "not a pointer\n",
		"odd/inner/deeper/":     "",
		"empty/.git":            "gitdir:\n",
		"abs/.gitignore":        "*.o\n",
		"abs/.git":              "placeholder",
		"abs/real/info/":        "",
		"abs/real/info/exclude": "*.abs\n",
	})
	write := func(rel, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("wt/.git", "gitdir: "+filepath.Join(root, "main", ".git", "worktrees", "wt")+"\n")
	write("super/sub/.git", "gitdir: ../.git/modules/sub\n")
	write("abs/.git", "gitdir:   "+filepath.Join(root, "abs", "real")+"  \nsecond line\n")

	checkPaths(t, For(filepath.Join(root, "wt"), Options{}), []pathCheck{
		{"a.o", false, true},
		{"src/a.cache", false, true},
		{"src/a.c", false, false},
	})
	checkPaths(t, For(filepath.Join(root, "super", "sub", "x"), Options{}), []pathCheck{
		{"a.gen", false, true},
		{"a.o", false, false},
	})
	// odd/inner is not a repository, so odd's rules still apply in it.
	checkPaths(t, For(filepath.Join(root, "odd"), Options{}), []pathCheck{
		{"inner/a.o", false, true},
		{"inner/deeper/a.o", false, true},
	})
	checkPaths(t, For(filepath.Join(root, "abs"), Options{}), []pathCheck{
		{"a.abs", false, true},
		{"a.o", false, true},
	})
	if gitDir(filepath.Join(root, "empty")) != "" {
		t.Error("a .git file with an empty gitdir made a repository")
	}
}

func TestRulesOptions(t *testing.T) {
	files := map[string]string{
		".git/info/exclude": "*.tmp\n",
		".gitignore":        "*.o\n!keep.x\n",
		".ignore":           "*.x\n!special.o\n",
		"sub/.ignore":       "*.y\n",
	}
	t.Run("defaults read .gitignore and info/exclude only", func(t *testing.T) {
		checkPaths(t, For(tree(t, files), Options{}), []pathCheck{
			{"a.o", false, true},
			{"a.tmp", false, true},
			{"a.x", false, false},
			{"sub/a.y", false, false},
			{"special.o", false, true},
		})
	})
	t.Run("extra names; the later name wins", func(t *testing.T) {
		checkPaths(t, For(tree(t, files), Options{Names: []string{".gitignore", ".ignore"}}), []pathCheck{
			{"a.o", false, true},
			{"special.o", false, false},
			{"a.x", false, true},
			{"keep.x", false, true},
			{"sub/a.y", false, true},
			{"a.tmp", false, true},
		})
		checkPaths(t, For(tree(t, files), Options{Names: []string{".ignore", ".gitignore"}}), []pathCheck{
			{"special.o", false, true},
			{"keep.x", false, false},
			{"a.x", false, true},
		})
	})
	t.Run("no names at all", func(t *testing.T) {
		checkPaths(t, For(tree(t, files), Options{Names: []string{}}), []pathCheck{
			{"a.o", false, false},
			{"a.tmp", false, true},
		})
	})
	t.Run("info/exclude turned off", func(t *testing.T) {
		checkPaths(t, For(tree(t, files), Options{NoGitExclude: true}), []pathCheck{
			{"a.o", false, true},
			{"a.tmp", false, false},
		})
	})
	t.Run("a size limit cuts at a whole line", func(t *testing.T) {
		root := tree(t, map[string]string{".git/": "", ".gitignore": "aa\nbb\ncc\n"})
		// Seven bytes hold "aa\nbb\nc": the third line is cut and dropped.
		checkPaths(t, For(root, Options{MaxFileBytes: 7}), []pathCheck{
			{"aa", false, true},
			{"bb", false, true},
			{"cc", false, false},
			{"c", false, false},
		})
		checkPaths(t, For(root, Options{MaxFileBytes: 9}), []pathCheck{
			{"cc", false, true},
		})
		checkPaths(t, For(root, Options{MaxFileBytes: 1}), []pathCheck{
			{"aa", false, false},
		})
	})
}

func TestRulesIgnoreFileMustBeARegularFile(t *testing.T) {
	root := tree(t, map[string]string{
		".git/":           "",
		"real-rules":      "*.o\n",
		"dir/.gitignore/": "",
		"dir/x/":          "",
	})
	if err := os.Symlink("real-rules", filepath.Join(root, ".gitignore")); err != nil {
		t.Skipf("cannot make a symbolic link here: %v", err)
	}
	// Git does not follow a link to an ignore file, and a directory named
	// .gitignore is not one either.
	checkPaths(t, For(root, Options{}), []pathCheck{
		{"a.o", false, false},
		{"dir/a.o", false, false},
		{"dir/x/a.o", false, false},
	})
}

func TestEnterUsesTheListing(t *testing.T) {
	root := tree(t, map[string]string{
		".git/":             "",
		"sub/.gitignore":    "*.o\n",
		"sub/in/.git/":      "",
		"sub/in/.gitignore": "*.c\n",
	})
	r := For(root, Options{})
	// A listing that does not show the ignore file means it is not read.
	blind := r.Enter("sub", func(string) bool { return false })
	if blind.Ignored("a.o", false) {
		t.Error("Enter read an ignore file its listing said was not there")
	}
	seeing := r.Enter("sub", func(name string) bool { return name == ".gitignore" })
	if !seeing.Ignored("a.o", false) {
		t.Error("Enter did not read the ignore file its listing showed")
	}
	// The same goes for noticing a repository.
	notRepo := seeing.Enter("in", func(name string) bool { return name == ".gitignore" })
	if !notRepo.Ignored("a.o", false) || !notRepo.Ignored("a.c", false) {
		t.Error("without .git in the listing, the rules above should still apply")
	}
	repo := seeing.Enter("in", func(name string) bool { return name == ".gitignore" || name == ".git" })
	if repo.Ignored("a.o", false) || !repo.Ignored("a.c", false) {
		t.Error("with .git in the listing, only the inner repository's rules should apply")
	}
}

func FuzzParseAndMatch(f *testing.F) {
	for _, s := range [][2]string{
		{"*.o\n!keep.o\n", "a/keep.o"}, {"/a/**/b/\n", "a/x/b"}, {"[\n\\\n", "["},
		{"**\n", ""}, {"!\n/\n", "x"}, {"a\\ \n b \n", "a "}, {"\xef\xbb\xbfx\r\n", "x"},
		{"[[:alpha:]]*\n[!a-\n", "q"}, {"foo/\n!foo/bar\n", "foo/bar"}, {"\\#\n\\!\n", "#"},
		{"***/**/**\n", "a/b/c"}, {"a/\\/b\n", "a/b"},
	} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, text, path string) {
		file := Parse([]byte(text))
		for _, isDir := range []bool{false, true} {
			ignored, decided := file.Match(path, isDir)
			if ignored && !decided {
				t.Fatalf("Match(%q) says ignored without a deciding pattern", path)
			}
		}
		// A file with only comments and blank lines decides nothing.
		if file.Len() == 0 {
			if _, decided := file.Match(path, true); decided {
				t.Fatalf("a file with no patterns decided %q", path)
			}
		}
		// A pattern that only a directory can match never matches a file.
		if strings.HasSuffix(strings.TrimRight(text, " "), "/") && !strings.Contains(text, "\n") {
			if _, decided := file.Match(path, false); decided {
				t.Fatalf("the directory-only pattern %q matched the file %q", text, path)
			}
		}
	})
}

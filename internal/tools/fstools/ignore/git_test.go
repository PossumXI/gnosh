package ignore

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// These tests ask git itself which paths it ignores and compare the answer
// with this package's. They are skipped where git is not installed.

// gitRunner runs git in one working tree with nothing of the machine's own
// configuration in sight.
type gitRunner struct {
	t    *testing.T
	git  string
	root string
	env  []string
}

func newGitRunner(t *testing.T) *gitRunner {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	g := &gitRunner{t: t, git: git, root: root, env: []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + home,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"LC_ALL=C",
	}}
	g.run(nil, "init", "-q", ".")
	// Whatever the file system, compare names by their exact letters, as
	// this package does.
	g.run(nil, "config", "core.ignorecase", "false")
	return g
}

func (g *gitRunner) run(stdin []byte, args ...string) []byte {
	g.t.Helper()
	cmd := exec.Command(g.git, args...)
	cmd.Dir = g.root
	cmd.Env = g.env
	cmd.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// check-ignore exits 1 when no path is ignored; that is an answer.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 && len(args) > 0 && args[0] == "check-ignore" && stderr.Len() == 0 {
			return out
		}
		g.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return out
}

// ignoredSet asks git which of the paths it ignores.
func (g *gitRunner) ignoredSet(paths []string) map[string]bool {
	g.t.Helper()
	var in bytes.Buffer
	for _, p := range paths {
		in.WriteString(p)
		in.WriteByte(0)
	}
	out := g.run(in.Bytes(), "check-ignore", "--no-index", "-z", "--stdin")
	set := make(map[string]bool)
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			set[string(p)] = true
		}
	}
	return set
}

func (g *gitRunner) write(rel, text string) {
	g.t.Helper()
	path := filepath.Join(g.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		g.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		g.t.Fatal(err)
	}
}

// The names the generated trees and patterns are made of. No name is both
// a directory and a file, and a few are awkward on purpose.
var (
	gitDirNames  = []string{"a", "b", "foo", "build", "d e"}
	gitFileNames = []string{"x.o", "y.txt", "c", "bar", "f g", "#h", "!i", "a.b.c"}
)

// gitTree makes every directory and file of the test tree, three levels
// deep, and returns their paths: directories first within each level.
func gitTree(g *gitRunner) (dirs, files []string) {
	level := []string{""}
	for depth := 0; depth < 3; depth++ {
		var next []string
		for _, parent := range level {
			for _, f := range gitFileNames {
				p := parent + f
				g.write(p, "x")
				files = append(files, p)
			}
			if depth == 2 {
				continue
			}
			for _, d := range gitDirNames {
				p := parent + d
				if err := os.MkdirAll(filepath.Join(g.root, filepath.FromSlash(p)), 0o755); err != nil {
					g.t.Fatal(err)
				}
				dirs = append(dirs, p)
				next = append(next, p+"/")
			}
		}
		level = next
	}
	return dirs, files
}

// randomPattern writes one line of an ignore file.
func randomPattern(r *rand.Rand) string {
	// Now and then, a line that is special in itself.
	fixed := []string{
		"", "# comment", "#h", "\\#h", "\\!i", "!i", "!", "/", "**", "**/", "/**", "***", "*", "/*", "*/", "!*/",
		"[", "a[", "foo\\", "f\\ g", "d\\ e/", "f g", "f g   ", "f\\ g  ", " c", "x.o\r", "a/**", "**/a", "a/**/c",
		"**/**", "a/**/**/c", "/**/c", "**/build/**", "*.[oa]", "[!a]", "[a-c]", "[]]", "?", "??", "a?", "*.*",
		"a.b.c", "*.b.*", "/a/", "a/b", "/a/b/", "!/a/b", "foo/", "!foo/", "foo/*", "!foo/bar", "build", "!build",
		"\\*", "x.o ", "y.txt\\ ", "[[:alpha:]]", "[[:digit:]]*", "**foo", "foo**", "f**o", "a/**b", "a**/b",
		"//a", "a//b", "a/", "a//", "/a", "./a", "a/./b", "a/../b", "**/", "!**/", "\\", "\\\\", "*\\", "!!i", "!#h",
		"[a-", "[]", "a\\", "\\ ", " ", "  ", "\t", "c\t", "c ", "c\\ ", "!c ", "/c", "c/", "/c/", "**/c", "c/**", "!/c",
		"*c", "c*", "[c]", "[[]", "[]c]", "[c-]", "[-c]", "[!]]", "[^a]", "[\\c]", "[c\\]", "*/c", "*/*", "*/*/*", "a/*/c",
		"/*/*", "!/*/*", "a/*", "!a/*", "/a/*/", "foo/**/", "**/foo/", "**/foo/**", "/foo/build/", "foo/build", "x.o/",
		"!x.o", "****", "a/****", "****/c", "a/***/c", "*/**", "**/*", "!**/*", "?*", "*?", "[a-c][a-c]", "[!a-c]*",
		"[[:lower:]]", "[[:upper:]]*", "[[:space:]]", "*[[:space:]]*", "[[:punct:]]*", "[^[:alpha:]]*", "[a[:digit:]]",
		"[[:alpha:]", "[[:nope:]]", "[[:alpha:]-z]", "[a-[:alpha:]]", "\\[a]", "[\\]]", "[\\!a]", "[a\\-c]", "*.\\o",
		"\\a", "\\x.o", "x\\.o", "é", "[é]", "?.o", "x?o", "x.[!o]", "x.[^o]", "#", "\\#", "!\\#h", "\\!", "!\\!i",
	}
	if r.IntN(3) == 0 {
		return fixed[r.IntN(len(fixed))]
	}
	segs := []string{
		"a", "b", "foo", "build", "d e", "d\\ e", "x.o", "y.txt", "c", "bar", "f g", "a.b.c",
		"*", "**", "*.o", "*.txt", "a*", "*a", "?", "[ab]", "[!a]", "[a-f]*", "x.?", "*.*", "f*", "b*r", "***",
	}
	var b strings.Builder
	if r.IntN(4) == 0 {
		b.WriteByte('!')
	}
	if r.IntN(4) == 0 {
		b.WriteByte('/')
	}
	n := 1 + r.IntN(3)
	if r.IntN(2) == 0 {
		n = 1
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte('/')
		}
		b.WriteString(segs[r.IntN(len(segs))])
	}
	if r.IntN(4) == 0 {
		b.WriteByte('/')
	}
	if r.IntN(10) == 0 {
		b.WriteString("  ")
	}
	return b.String()
}

func randomIgnoreFile(r *rand.Rand) string {
	var b strings.Builder
	end := "\n"
	if r.IntN(8) == 0 {
		end = "\r\n"
	}
	for i, n := 0, 1+r.IntN(6); i < n; i++ {
		b.WriteString(randomPattern(r))
		b.WriteString(end)
	}
	return b.String()
}

func TestAgainstGit(t *testing.T) {
	g := newGitRunner(t)
	dirs, files := gitTree(g)
	all := append(append([]string{}, dirs...), files...)
	isDir := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		isDir[d] = true
	}

	// Where ignore files may go, besides the top.
	places := []string{"a", "foo", "a/b", "foo/build", "d e", "build/a"}

	rounds := 400
	if testing.Short() {
		rounds = 40
	}
	seed := uint64(7)
	// GNOSH_IGNORE_ROUNDS and GNOSH_IGNORE_SEED let a person run a longer
	// or a different comparison by hand; the suite itself never sets them.
	if n, err := strconv.Atoi(os.Getenv("GNOSH_IGNORE_ROUNDS")); err == nil && n > 0 {
		rounds = n
	}
	if n, err := strconv.ParseUint(os.Getenv("GNOSH_IGNORE_SEED"), 10, 64); err == nil {
		seed = n
	}
	r := rand.New(rand.NewPCG(20261113, seed))
	for round := 0; round < rounds; round++ {
		// Fresh ignore files for this round.
		written := map[string]string{".gitignore": randomIgnoreFile(r)}
		for _, p := range places {
			if r.IntN(3) == 0 {
				written[p+"/.gitignore"] = randomIgnoreFile(r)
			}
		}
		exclude := ""
		if r.IntN(2) == 0 {
			exclude = randomIgnoreFile(r)
		}
		for rel, text := range written {
			g.write(rel, text)
		}
		g.write(".git/info/exclude", exclude)

		want := g.ignoredSet(all)
		rules := For(g.root, Options{})
		var diffs []string
		for _, p := range all {
			got := ignoredBelow(rules, p, isDir[p])
			if got != want[p] {
				diffs = append(diffs, fmt.Sprintf("  %q (directory: %v): this package says ignored=%v, git says %v", p, isDir[p], got, want[p]))
			}
		}
		if len(diffs) > 0 {
			var b strings.Builder
			var names []string
			for rel := range written {
				names = append(names, rel)
			}
			sort.Strings(names)
			for _, rel := range names {
				fmt.Fprintf(&b, "%s:\n%q\n", rel, written[rel])
			}
			fmt.Fprintf(&b, ".git/info/exclude:\n%q\n", exclude)
			if len(diffs) > 12 {
				diffs = append(diffs[:12], fmt.Sprintf("  ... and %d more", len(diffs)-12))
			}
			t.Fatalf("round %d: disagreement with git %s\n%s%s", round, strings.TrimSpace(string(g.run(nil, "--version"))), b.String(), strings.Join(diffs, "\n"))
		}

		for rel := range written {
			if rel != ".gitignore" {
				if err := os.Remove(filepath.Join(g.root, filepath.FromSlash(rel))); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

// TestAgainstGitManualExamples runs the examples of the gitignore manual
// page past git and this package alike.
func TestAgainstGitManualExamples(t *testing.T) {
	g := newGitRunner(t)
	g.write(".git/info/exclude", "# ignore objects and archives, anywhere in the tree.\n*.[oa]\n")
	g.write(".gitignore", "vmlinux*\nhello.*\n/top.*\nzoo/\ndoc/frotz/\ngrotz/\nabc/**\nq/**/r\n**/deep/leaf\npre/*\n")
	g.write("Documentation/.gitignore", "# ignore generated html files,\n*.html\n# except foo.html which is maintained by hand\n!foo.html\n")
	g.write("arch/foo/kernel/.gitignore", "!/vmlinux*\n")
	g.write("only/.gitignore", "# exclude everything except directory foo/bar\n/*\n!/foo\n/foo/*\n!/foo/bar\n")

	var dirs, files []string
	addFile := func(p string) { g.write(p, "x"); files = append(files, p) }
	addDir := func(p string) {
		if err := os.MkdirAll(filepath.Join(g.root, filepath.FromSlash(p)), 0o755); err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, p)
	}
	for _, p := range []string{
		"Documentation/foo.html", "Documentation/gitignore.html", "file.o", "lib.a", "src/internal.o", "src/internal.c",
		"vmlinux", "arch/foo/kernel/vmlinux.lds.S", "arch/foo/vmlinux.bin",
		"hello.txt", "hello.c", "a/hello.java", "top.x", "a/top.x",
		"a/zoo", "doc/x", "a/doc/x", "a/grotz/x", "b/grotz", "abc/x/y", "x/abc/y",
		"q/r", "q/1/r", "q/1/2/r", "q/1/2/s", "z/deep/leaf", "deep/leaf", "deep/x/leaf",
		"pre/test.json", "pre/bar/hello.c",
		"only/README", "only/src/main.c", "only/foo/x", "only/foo/bar/deep/file",
	} {
		addFile(p)
	}
	for _, p := range []string{
		"zoo", "b/zoo", "doc/frotz", "a/doc/frotz", "grotz", "a/grotz", "abc", "abc/x", "x/abc",
		"pre", "pre/bar", "only/src", "only/foo", "only/foo/bar", "only/foo/bar/deep",
	} {
		addDir(p)
	}
	all := append(append([]string{}, dirs...), files...)
	isDir := make(map[string]bool)
	for _, d := range dirs {
		isDir[d] = true
	}
	want := g.ignoredSet(all)
	rules := For(g.root, Options{})
	for _, p := range all {
		if got := ignoredBelow(rules, p, isDir[p]); got != want[p] {
			t.Errorf("%q (directory: %v): this package says ignored=%v, git says %v", p, isDir[p], got, want[p])
		}
	}
	// And the answers the manual page itself gives.
	for p, ignored := range map[string]bool{
		"Documentation/foo.html":        false,
		"Documentation/gitignore.html":  true,
		"file.o":                        true,
		"lib.a":                         true,
		"src/internal.o":                true,
		"arch/foo/kernel/vmlinux.lds.S": false,
		"hello.txt":                     true,
		"a/hello.java":                  true,
		"top.x":                         true,
		"a/top.x":                       false,
		"doc/frotz":                     true,
		"a/doc/frotz":                   false,
		"grotz":                         true,
		"a/grotz":                       true,
		"b/grotz":                       false,
		"zoo":                           true,
		"b/zoo":                         true,
		"a/zoo":                         false,
		"pre/test.json":                 true,
		"pre/bar":                       true,
		"q/r":                           true,
		"q/1/2/r":                       true,
		"only/foo/bar/deep/file":        false,
		"only/README":                   true,
	} {
		if want[p] != ignored {
			t.Errorf("git says ignored=%v for %q; the manual page says %v", want[p], p, ignored)
		}
	}
}

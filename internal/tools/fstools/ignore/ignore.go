// Package ignore decides which files a search should leave out, by the
// rules of gitignore files.
//
// It follows the pattern format in git's gitignore manual page:
//
//   - A blank line is nothing. A line that starts with "#" is a comment;
//     "\#" starts a pattern with a real hash.
//   - Spaces at the end of a line are dropped unless a backslash protects
//     them. One carriage return before the line break is dropped too, so a
//     file saved with Windows line endings reads the same.
//   - "!" in front turns the pattern round: what it matches is taken back
//     in. "\!" starts a pattern with a real exclamation mark.
//   - A slash at the end makes the pattern match directories only.
//   - A slash at the start or in the middle ties the pattern to the
//     directory of the ignore file. Without one, the pattern is compared
//     with the last name of a path, at any depth.
//   - "*" matches anything but a slash, "?" one character but a slash,
//     "[a-z]" one character of a set. "**/" at the start matches in every
//     directory, "/**" at the end matches everything inside, and "/**/"
//     matches any number of directories, none included.
//
// Within one file the last pattern that matches decides. Between files,
// the one nearest the path decides: an ignore file in a directory comes
// before those in the directories above it, and all of them come before
// the repository's .git/info/exclude.
//
// A file inside an ignored directory is ignored whatever else matches it:
// a walk never enters an ignored directory, so nothing in it can be taken
// back in. That is git's rule too.
//
// Three things differ from git, on purpose:
//
//   - Tracked files are not exempt. Git never ignores a file it tracks;
//     this package does not read git's index and goes by the patterns
//     alone.
//   - The user-wide excludes file (core.excludesFile) is not read, because
//     that would mean reading the home directory and git's configuration.
//   - Letter case always matters, even in a repository where git was told
//     to ignore it (core.ignoreCase).
//
// A pattern that cannot be parsed, such as one with a "[" that is never
// closed, matches nothing.
package ignore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/tools/fstools/wild"
)

// pattern is one line of an ignore file.
type pattern struct {
	// negate is set for a line that began with "!": a match takes the path
	// back in.
	negate bool
	// dirOnly is set for a line that ended with a slash.
	dirOnly bool
	// anchored is set when the line had a slash at its start or in its
	// middle: path is then compared with the whole path below the ignore
	// file's directory. Otherwise name is compared with the last name.
	anchored bool
	name     wild.Segment
	path     *wild.Pattern
}

func (p *pattern) matches(rel, base string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}
	if p.anchored {
		return p.path.Match(rel)
	}
	return p.name.Match(base)
}

// parseLine reads one line of an ignore file, without its line break. It
// reports false for a line that holds no usable pattern.
func parseLine(line string) (pattern, bool) {
	line = strings.TrimSuffix(line, "\r")
	line = trimTrailingSpaces(line)
	if line == "" || line[0] == '#' {
		return pattern{}, false
	}
	var p pattern
	if line[0] == '!' {
		p.negate = true
		line = line[1:]
	}
	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = line[:len(line)-1]
	}
	if line == "" {
		return pattern{}, false
	}
	if strings.Contains(line, "/") {
		p.anchored = true
		line = strings.TrimPrefix(line, "/")
		compiled, err := wild.Compile(line)
		if err != nil {
			return pattern{}, false
		}
		p.path = compiled
		return p, true
	}
	seg, err := wild.CompileSegment(line)
	if err != nil {
		return pattern{}, false
	}
	p.name = seg
	return p, true
}

// trimTrailingSpaces drops the spaces at the end of a line, keeping any
// that a backslash protects.
func trimTrailingSpaces(line string) string {
	end := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			continue
		case '\\':
			// The backslash and the character after it both count.
			if i+1 < len(line) {
				i++
			}
		}
		end = i + 1
	}
	return line[:end]
}

// File is the patterns of one ignore file, in the order written.
type File struct {
	patterns []pattern
}

// Parse reads the text of an ignore file. A byte order mark at the start
// is skipped.
func Parse(data []byte) *File {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	f := &File{}
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		if p, ok := parseLine(string(line)); ok {
			f.patterns = append(f.patterns, p)
		}
	}
	return f
}

// Len returns the number of patterns in the file.
func (f *File) Len() int { return len(f.patterns) }

// Match looks for the last pattern in the file that matches a path. rel is
// the path below the directory the file's patterns are relative to, with
// slashes and no leading slash; isDir says whether the path is a
// directory. decided is false when no pattern matches; otherwise ignored
// says whether the path is ignored (true) or taken back in by a "!"
// pattern (false).
//
// Match looks at this one path only. It does not look at the directories
// above it: a file under an ignored directory is ignored even when Match
// says otherwise, and Rules takes care of that.
func (f *File) Match(rel string, isDir bool) (ignored, decided bool) {
	base := rel
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		base = rel[i+1:]
	}
	for i := len(f.patterns) - 1; i >= 0; i-- {
		if p := &f.patterns[i]; p.matches(rel, base, isDir) {
			return !p.negate, true
		}
	}
	return false, false
}

// Options says which ignore files to read.
type Options struct {
	// Names are the names of the ignore files read in every directory, in
	// rising order of weight: where two of them disagree about a path, the
	// later name wins. Nil means .gitignore alone.
	Names []string

	// NoGitExclude turns off reading a repository's .git/info/exclude.
	NoGitExclude bool

	// MaxFileBytes is the most that is read of one ignore file. Lines past
	// it are not seen. Zero means 1 MiB.
	MaxFileBytes int
}

// config is Options with the defaults filled in. It is shared by every
// Rules made from one call to For.
type config struct {
	names        []string
	gitExclude   bool
	maxFileBytes int
}

// Rules are the ignore rules in force inside one directory: those of its
// own ignore files and of every directory above it, up to the top of the
// repository it is in. A Rules is not changed once made, so any number of
// goroutines may use it.
type Rules struct {
	cfg *config
	// dir is the directory, as an absolute path.
	dir string
	// prefix is the path of dir below the top directory of its chain, with
	// a slash after every name: "" at the top, then "a/", "a/b/".
	prefix string
	// up is the nearest directory above, in the same repository, that has
	// patterns of its own; nil when there is none.
	up *Rules
	// files are the directory's own ignore files, heaviest first.
	files []*File
	// exclude is the repository's info/exclude, when dir is the top of a
	// repository.
	exclude *File
}

// For returns the rules in force inside dir, an absolute path. It reads
// dir's ignore files and, when dir is inside a git repository, those of
// the directories above it up to the repository's top. Outside a
// repository nothing above dir is read: an ignore file in a home directory
// should not hide a project kept under it.
func For(dir string, opts Options) *Rules {
	cfg := &config{names: opts.Names, gitExclude: !opts.NoGitExclude, maxFileBytes: opts.MaxFileBytes}
	if cfg.names == nil {
		cfg.names = []string{".gitignore"}
	}
	if cfg.maxFileBytes <= 0 {
		cfg.maxFileBytes = 1 << 20
	}
	dir = filepath.Clean(dir)
	// Find the nearest directory, dir itself or one above it, that is the
	// top of a repository.
	top := ""
	for d := dir; ; {
		if isRepoTop(d) {
			top = d
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	if top == "" {
		return cfg.load(dir, nil, "", true, nil)
	}
	r := cfg.load(top, nil, "", true, nil)
	rel, err := filepath.Rel(top, dir)
	if err != nil || rel == "." {
		return r
	}
	for _, name := range strings.Split(filepath.ToSlash(rel), "/") {
		r = r.Enter(name, nil)
	}
	return r
}

// Enter returns the rules in force inside the directory called name that
// sits directly in r's directory. has, when not nil, reports whether that
// directory holds an entry of a given name; it lets a walk that has
// already listed the directory save Enter from asking the disk. With a nil
// has, Enter asks the disk.
//
// A directory that is the top of a git repository starts afresh: the rules
// of the directories above it stop applying inside it, as they do for git.
func (r *Rules) Enter(name string, has func(name string) bool) *Rules {
	dir := filepath.Join(r.dir, name)
	if (has == nil || has(".git")) && isRepoTop(dir) {
		return r.cfg.load(dir, nil, "", true, has)
	}
	up := r
	if len(r.files) == 0 && r.exclude == nil {
		up = r.up
	}
	return r.cfg.load(dir, up, r.prefix+name+"/", false, has)
}

// load reads the ignore files of one directory.
func (cfg *config) load(dir string, up *Rules, prefix string, top bool, has func(string) bool) *Rules {
	r := &Rules{cfg: cfg, dir: dir, prefix: prefix, up: up}
	for i := len(cfg.names) - 1; i >= 0; i-- {
		name := cfg.names[i]
		if has != nil && !has(name) {
			continue
		}
		if f := readIgnoreFile(filepath.Join(dir, name), cfg.maxFileBytes); f != nil {
			r.files = append(r.files, f)
		}
	}
	if top && cfg.gitExclude {
		if path := gitExcludePath(dir); path != "" {
			r.exclude = readIgnoreFile(path, cfg.maxFileBytes)
		}
	}
	return r
}

// Ignored reports whether the entry called name, directly inside r's
// directory, is ignored. isDir says whether the entry is a directory.
//
// It trusts that r's directory is not itself ignored: a walk that skips
// ignored directories never asks about anything inside one.
func (r *Rules) Ignored(name string, isDir bool) bool {
	full := r.prefix + name
	for n := r; n != nil; n = n.up {
		rel := full[len(n.prefix):]
		for _, f := range n.files {
			if ignored, decided := f.Match(rel, isDir); decided {
				return ignored
			}
		}
	}
	for n := r; n != nil; n = n.up {
		if n.exclude != nil {
			if ignored, decided := n.exclude.Match(full[len(n.prefix):], isDir); decided {
				return ignored
			}
		}
	}
	return false
}

// Dir returns the directory the rules are for.
func (r *Rules) Dir() string { return r.dir }

// readIgnoreFile reads and parses one ignore file. It returns nil when the
// file is missing, unreadable, empty of patterns, or not a regular file:
// git does not follow a symbolic link to an ignore file, and neither does
// this.
func readIgnoreFile(path string, max int) *File {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	data, err := readUpTo(path, max)
	if err != nil {
		return nil
	}
	if len(data) > max {
		// Cut at the last whole line within the limit.
		data = data[:max]
		if i := bytes.LastIndexByte(data, '\n'); i >= 0 {
			data = data[:i+1]
		} else {
			data = nil
		}
	}
	f := Parse(data)
	if f.Len() == 0 {
		return nil
	}
	return f
}

// readUpTo reads at most max+1 bytes of a file, so that the caller can
// tell a file of exactly max bytes from a longer one.
func readUpTo(path string, max int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, int64(max)+1))
}

// isRepoTop reports whether dir is the top of a git working tree: it holds
// a .git directory, or a .git file that points at one (as a linked
// worktree and a submodule do).
func isRepoTop(dir string) bool {
	return gitDir(dir) != ""
}

// gitDir returns the git directory of the working tree whose top is dir,
// or "" when dir is not the top of one.
func gitDir(dir string) string {
	dotgit := filepath.Join(dir, ".git")
	fi, err := os.Stat(dotgit)
	if err != nil {
		return ""
	}
	if fi.IsDir() {
		return dotgit
	}
	if !fi.Mode().IsRegular() {
		return ""
	}
	data, err := readUpTo(dotgit, 4096)
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(data), "\n")
	target, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return ""
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	return target
}

// gitExcludePath returns the path of the info/exclude file of the
// repository whose working tree has its top at dir, or "". A linked
// worktree keeps a git directory of its own that names, in its commondir
// file, the main one; info/exclude lives in the main one.
func gitExcludePath(dir string) string {
	gd := gitDir(dir)
	if gd == "" {
		return ""
	}
	common := gd
	if data, err := readUpTo(filepath.Join(gd, "commondir"), 4096); err == nil {
		if c := strings.TrimSpace(string(data)); c != "" {
			if !filepath.IsAbs(c) {
				c = filepath.Join(gd, c)
			}
			common = c
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ""
	}
	return filepath.Join(common, "info", "exclude")
}

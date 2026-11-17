package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A new work tree is a checkout of a commit, and what git ignores is not
// in one: a .env, a local settings file. As Claude Code does, the files a
// repository's .worktreeinclude names (in the syntax of .gitignore) that
// git ignores are copied into each new work tree, a session's (--worktree)
// or a sub-agent's, each to the same place in it. A copy the work tree
// still holds as it was copied is no work of the tree's: one whose only
// difference from the repository is such copies is removed at its end as
// one that holds nothing.

// includeFile is the file, in a repository's top folder, that names them.
const includeFile = ".worktreeinclude"

// copyIncluded copies into the work tree wt, which was just made, the
// files of its repository that .worktreeinclude names and git ignores,
// and returns those it copied. A file the work tree has already is left as
// it is, and so is one whose place in the work tree leads out of it (by a
// link the checkout holds). The error says what could not be copied.
func copyIncluded(ctx context.Context, environ []string, wt *worktree) ([]string, error) {
	list := filepath.Join(wt.repo, includeFile)
	if info, err := os.Stat(list); err != nil || !info.Mode().IsRegular() {
		return nil, nil
	}
	// The files it names that git does not track ...
	named, err := gitWith(ctx, environ, wt.repo, "", "ls-files", "-z", "--others", "--ignored", "--exclude-from="+list)
	if err != nil {
		return nil, fmt.Errorf("the files %s names could not be listed (%v)", includeFile, err)
	}
	if strings.Trim(named, "\x00") == "" {
		return nil, nil
	}
	// ... and of those, the ones git ignores.
	ignored, err := gitWith(ctx, environ, wt.repo, named, "check-ignore", "-z", "--stdin")
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee) && ee.ExitCode() == 1:
		return nil, nil // none of them
	case err != nil:
		return nil, fmt.Errorf("which of the files %s names git ignores could not be told (%v)", includeFile, err)
	}
	tree, err := filepath.EvalSymlinks(wt.path)
	if err != nil {
		return nil, err
	}
	var copied, failed []string
	for _, rel := range strings.Split(ignored, "\x00") {
		if rel == "" || !filepath.IsLocal(rel) {
			continue
		}
		dst := filepath.Join(wt.path, rel)
		if fileExists(dst) {
			continue
		}
		if err := copyInto(filepath.Join(wt.repo, rel), dst, tree); err != nil {
			failed = append(failed, rel+" ("+err.Error()+")")
			continue
		}
		copied = append(copied, rel)
	}
	if len(failed) > 0 {
		return copied, fmt.Errorf("of the files %s names, these could not be copied into the work tree: %s", includeFile, strings.Join(failed, ", "))
	}
	return copied, nil
}

// copyInto copies the file or link src to dst, whose folders are made as
// needed, and only inside the folder tree (its real path).
func copyInto(src, dst, tree string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if real, err := filepath.EvalSymlinks(dir); err != nil || real != tree && !strings.HasPrefix(real, tree+string(filepath.Separator)) {
		return errors.New("its folder in the work tree leads out of it")
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case !info.Mode().IsRegular():
		return errors.New("it is no file")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// copiedAsIs reports whether what stands at rel in the work tree wt, a
// path git status gives as ignored, is a copy of what stands there in the
// repository's own folder: a file of the same bytes, a link to the same
// place, a folder of nothing else. Taking it away with the work tree loses
// nothing.
func copiedAsIs(wt *worktree, rel string) bool {
	rel = strings.TrimSuffix(rel, "/")
	if rel == "" || !filepath.IsLocal(rel) {
		return false
	}
	return sameFiles(filepath.Join(wt.path, rel), filepath.Join(wt.repo, rel))
}

// sameFiles reports whether a is what b is: the same bytes, the same link,
// or a folder each of whose entries is the same as b's of that name.
func sameFiles(a, b string) bool {
	ai, err := os.Lstat(a)
	if err != nil {
		return false
	}
	bi, err := os.Lstat(b)
	if err != nil || ai.Mode().Type() != bi.Mode().Type() {
		return false
	}
	switch {
	case ai.Mode()&os.ModeSymlink != 0:
		ta, err1 := os.Readlink(a)
		tb, err2 := os.Readlink(b)
		return err1 == nil && err2 == nil && ta == tb
	case ai.IsDir():
		entries, err := os.ReadDir(a)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if !sameFiles(filepath.Join(a, e.Name()), filepath.Join(b, e.Name())) {
				return false
			}
		}
		return true
	case ai.Mode().IsRegular():
		return ai.Size() == bi.Size() && sameBytes(a, b)
	}
	return false
}

// sameBytes reports whether two files hold the same bytes.
func sameBytes(a, b string) bool {
	fa, err := os.Open(a)
	if err != nil {
		return false
	}
	defer fa.Close()
	fb, err := os.Open(b)
	if err != nil {
		return false
	}
	defer fb.Close()
	// (A read that ends the file early ends it, on both sides at once.)
	ended := func(err error) bool { return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) }
	ba, bb := make([]byte, 64<<10), make([]byte, 64<<10)
	for {
		na, ea := io.ReadFull(fa, ba)
		nb, eb := io.ReadFull(fb, bb)
		if na != nb || !bytes.Equal(ba[:na], bb[:nb]) {
			return false
		}
		if ea != nil || eb != nil {
			return ended(ea) && ended(eb)
		}
	}
}

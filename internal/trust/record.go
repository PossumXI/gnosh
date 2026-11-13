package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Store is the record of the folders a person has trusted. It lives in the
// user's state directory, never in a project.
//
// The record is a directory holding one small file per trusted folder. A
// file is written whole to a temporary name and renamed into place, so a
// reader never sees part of one, and because each folder has a file of its
// own, several gnosh processes can trust and forget folders at the same
// time without one undoing another's work and without any lock.
type Store struct {
	// Dir is the directory the record is kept in: the config package's
	// Paths.TrustDir.
	Dir string
	// Home is the user's home directory. A record of it, or of a folder
	// that holds it, never counts.
	Home string
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
	// UID returns the id of the user gnosh runs as, who must own every
	// file of the record. Nil means os.Getuid.
	UID func() int
}

// Entry is one trusted folder.
type Entry struct {
	// Path is the folder, with links resolved: a Folder's Key.
	Path string
	// Added is when it was trusted.
	Added time.Time
	// Device and Inode are the numbers that told the folder from any other
	// on the day it was trusted.
	Device, Inode uint64
}

// entryVersion is the version written to every file of the record.
const entryVersion = 1

// entryFile is an Entry's shape on disk.
type entryFile struct {
	Version int    `json:"version"`
	Path    string `json:"path"`
	Device  uint64 `json:"device"`
	Inode   uint64 `json:"inode"`
	Added   string `json:"added"`
	// Check is a digest of the fields above. It shows that the file is as
	// gnosh wrote it: a record edited by hand, cut short or patched
	// together does not match. It is not a signature. Anything that can
	// write here as the user can also compute it, and nothing gnosh could
	// keep on the same disk would stop that.
	Check string `json:"check"`
}

// digest computes an entry file's Check.
func (e entryFile) digest() string {
	sum := sha256.Sum256(fmt.Appendf(nil, "gnosh trust record v%d\n%s\n%d\n%d\n%s\n", e.Version, e.Path, e.Device, e.Inode, e.Added))
	return hex.EncodeToString(sum[:])
}

// ErrSessionOnly is returned by Trust for the home directory and for any
// folder that holds it: such a folder is trusted for one session at a time
// and never recorded.
var ErrSessionOnly = errors.New("this folder is the home directory, or holds it, so it is trusted for one session at a time and is never recorded")

// now reads the clock.
func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// uid is the user who must own the record.
func (s *Store) uid() int {
	if s.UID != nil {
		return s.UID()
	}
	return os.Getuid()
}

// fileFor is the file that holds the entry of a folder. Its name is a
// digest of the folder's path, so that every path has one file and no
// path can name a file outside the record.
func (s *Store) fileFor(path string) string {
	sum := sha256.Sum256([]byte(path))
	return filepath.Join(s.Dir, hex.EncodeToString(sum[:16])+".json")
}

// sessionOnly reports whether path is the home directory or holds it.
func (s *Store) sessionOnly(path string) bool {
	if s.Home == "" {
		return false
	}
	home := s.Home
	if real, err := filepath.EvalSymlinks(home); err == nil {
		home = real
	}
	return path == home || holds(path, home)
}

// Trust records a folder as trusted. It records f.Key, so that a
// repository's worktrees share the entry. It refuses the home directory
// with ErrSessionOnly: the caller trusts that for the session and records
// nothing.
func (s *Store) Trust(f Folder) error {
	if f.SessionOnly || s.sessionOnly(f.Key) {
		return ErrSessionOnly
	}
	info, err := os.Stat(f.Key)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("the folder %s could not be recorded as trusted: it is not a folder that is there", f.Key)
	}
	dev, ino, _, ok := identity(info)
	if !ok {
		return fmt.Errorf("the folder %s could not be recorded as trusted: this system does not say which folder is which, so a record could not be checked later", f.Key)
	}
	e := entryFile{Version: entryVersion, Path: f.Key, Device: dev, Inode: ino, Added: s.now().UTC().Format(time.RFC3339)}
	e.Check = e.digest()
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return fmt.Errorf("the folder %s could not be recorded as trusted: %w", f.Key, err)
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("the folder %s could not be recorded as trusted: the record's directory could not be made: %w", f.Key, err)
	}
	if err := writeAtomic(s.fileFor(f.Key), append(data, '\n')); err != nil {
		return fmt.Errorf("the folder %s could not be recorded as trusted: %w", f.Key, err)
	}
	return nil
}

// writeAtomic writes data to a temporary file beside name and renames it
// over name, so that name is never seen half written.
func writeAtomic(name string, data []byte) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(name), ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Chmod(0o600); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), name)
}

// Forget removes a folder from the record. path is the folder as it was
// trusted: a Folder's Key. It reports whether there was an entry.
func (s *Store) Forget(path string) (bool, error) {
	err := os.Remove(s.fileFor(path))
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, fmt.Errorf("the folder %s could not be taken out of the trust record: %w", path, err)
}

// read reads the entry file for a folder and checks it. It returns the
// entry, or the reason the file does not count; both are empty when there
// is no file.
func (s *Store) read(file string) (*Entry, string) {
	li, err := os.Lstat(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ""
	}
	if err != nil {
		return nil, fmt.Sprintf("the trust record %s could not be looked at (%v)", file, err)
	}
	if !li.Mode().IsRegular() {
		return nil, fmt.Sprintf("the trust record %s is not an ordinary file, so it does not count", file)
	}
	_, _, owner, ok := identity(li)
	if !ok || owner != s.uid() {
		return nil, fmt.Sprintf("the trust record %s does not belong to you, so it does not count", file)
	}
	if li.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Sprintf("the trust record %s can be written by other users, so it does not count. If it is yours: chmod 600 %s", file, file)
	}
	text, err := readSmall(file)
	if err != nil {
		return nil, fmt.Sprintf("the trust record %s could not be read (%v)", file, err)
	}
	var e entryFile
	if err := json.Unmarshal([]byte(text), &e); err != nil {
		return nil, fmt.Sprintf("the trust record %s is not as gnosh wrote it (it cannot be read), so it does not count", file)
	}
	added, terr := time.Parse(time.RFC3339, e.Added)
	switch {
	case e.Version != entryVersion:
		return nil, fmt.Sprintf("the trust record %s is of a version this gnosh does not read, so it does not count", file)
	case e.Check != e.digest() || terr != nil:
		return nil, fmt.Sprintf("the trust record %s is not as gnosh wrote it (it was changed by something else), so it does not count", file)
	case !filepath.IsAbs(e.Path) || filepath.Clean(e.Path) != e.Path || s.fileFor(e.Path) != file:
		return nil, fmt.Sprintf("the trust record %s is not the record of the folder it names, so it does not count", file)
	case s.sessionOnly(e.Path):
		return nil, fmt.Sprintf("the trust record %s is for the home directory or a folder that holds it, which is never recorded, so it does not count", file)
	}
	return &Entry{Path: e.Path, Added: added, Device: e.Device, Inode: e.Inode}, ""
}

// current reports whether the folder an entry names is still the folder
// that was trusted. A folder that was moved away leaves its entry behind,
// and whatever is later put at the old path is a different folder: it
// does not inherit the trust.
func (e *Entry) current() (bool, string) {
	info, err := os.Stat(e.Path)
	if err != nil || !info.IsDir() {
		return false, fmt.Sprintf("the folder %s that was trusted is no longer there", e.Path)
	}
	dev, ino, _, ok := identity(info)
	if !ok || dev != e.Device || ino != e.Inode {
		return false, fmt.Sprintf("the folder now at %s is not the one that was trusted (it was replaced, or moved here), so it is asked about again", e.Path)
	}
	return true, ""
}

// Verdict is what the record says about a folder.
type Verdict struct {
	// Trusted reports that the folder is covered by a record that counts.
	Trusted bool
	// By is the recorded folder that covers it: the folder's Key, or, for
	// a folder outside any repository, a recorded folder above it.
	By string
	// Why says, when Trusted is false and a record was found that does not
	// count, why it does not.
	Why string
}

// Check says whether a folder is trusted.
//
// A repository is trusted only by a record of its own key: the main
// checkout. A folder outside any repository is also covered by a record of
// a folder above it, since a trusted folder covers the folders under it
// that are not repositories of their own. The home directory, and anything
// that holds it, is never trusted by the record.
func (s *Store) Check(f Folder) Verdict {
	if f.SessionOnly || s.sessionOnly(f.Key) {
		return Verdict{Why: "the home directory is trusted for one session at a time and is never recorded"}
	}
	var v Verdict
	for path := f.Key; ; path = filepath.Dir(path) {
		e, why := s.read(s.fileFor(path))
		if e != nil {
			if ok, stale := e.current(); ok {
				return Verdict{Trusted: true, By: e.Path}
			} else if v.Why == "" {
				v.Why = stale
			}
		} else if why != "" && v.Why == "" {
			v.Why = why
		}
		if f.Repo || path == filepath.Dir(path) {
			break
		}
	}
	return v
}

// List returns the folders the record holds, sorted by path, and a line
// for each file in the record that does not count, saying why. An entry
// whose folder has gone or been replaced is listed among the lines, not
// among the folders.
func (s *Store) List() ([]Entry, []string) {
	names, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("the trust record in %s could not be read (%v), so no folder counts as trusted", s.Dir, err)}
	}
	var entries []Entry
	var notes []string
	for _, n := range names {
		if strings.HasPrefix(n.Name(), ".") || !strings.HasSuffix(n.Name(), ".json") {
			continue
		}
		e, why := s.read(filepath.Join(s.Dir, n.Name()))
		if e == nil {
			if why != "" {
				notes = append(notes, why)
			}
			continue
		}
		if ok, stale := e.current(); !ok {
			notes = append(notes, stale)
			continue
		}
		entries = append(entries, *e)
	}
	slices.SortFunc(entries, func(a, b Entry) int { return strings.Compare(a.Path, b.Path) })
	slices.Sort(notes)
	return entries, notes
}

package trust

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// store makes a Store in a directory of the test's own, with a clock that
// stands still.
func store(t *testing.T, tmp string) *Store {
	t.Helper()
	return &Store{
		Dir:  filepath.Join(tmp, "state", "gnosh", "trusted-folders"),
		Home: filepath.Join(tmp, "home"),
		Now:  func() time.Time { return time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC) },
	}
}

// trusted reports whether the record covers the folder at dir.
func trusted(t *testing.T, s *Store, dir string) Verdict {
	t.Helper()
	return s.Check(resolve(t, dir, s.Home))
}

// only returns the one entry file in the record.
func only(t *testing.T, s *Store) string {
	t.Helper()
	names, err := os.ReadDir(s.Dir)
	if err != nil || len(names) != 1 {
		t.Fatalf("want one file in the record, got %v (%v)", names, err)
	}
	return filepath.Join(s.Dir, names[0].Name())
}

func TestTrustCheckListForget(t *testing.T) {
	tmp := root(t)
	s := store(t, tmp)
	a := repo(t, filepath.Join(tmp, "a"), "")
	b := mkdir(t, tmp, "b")

	// Nothing recorded: nothing trusted, nothing listed, and no record
	// directory made by looking.
	if v := trusted(t, s, a); v.Trusted || v.Why != "" {
		t.Errorf("before: %+v", v)
	}
	if entries, notes := s.List(); len(entries) != 0 || len(notes) != 0 {
		t.Errorf("before: %v %v", entries, notes)
	}
	if _, err := os.Stat(s.Dir); err == nil {
		t.Errorf("looking made the record directory")
	}

	for _, dir := range []string{a, b} {
		if err := s.Trust(resolve(t, dir, s.Home)); err != nil {
			t.Fatalf("Trust(%s): %v", dir, err)
		}
	}
	if v := trusted(t, s, a); !v.Trusted || v.By != a {
		t.Errorf("after trusting: %+v", v)
	}
	if v := trusted(t, s, mkdir(t, a, "deep", "er")); !v.Trusted || v.By != a {
		t.Errorf("a folder inside a trusted repository: %+v", v)
	}
	entries, notes := s.List()
	if len(notes) != 0 || len(entries) != 2 || entries[0].Path != a || entries[1].Path != b || !entries[0].Added.Equal(s.Now()) {
		t.Errorf("List: %+v %v", entries, notes)
	}

	// The record is the user's alone, and each write is a whole file.
	info, err := os.Stat(s.Dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the record directory: %v %v", info.Mode(), err)
	}
	names, _ := os.ReadDir(s.Dir)
	for _, n := range names {
		fi, _ := n.Info()
		if fi.Mode().Perm() != 0o600 || !strings.HasSuffix(n.Name(), ".json") || strings.HasPrefix(n.Name(), ".") {
			t.Errorf("a file in the record: %s %v", n.Name(), fi.Mode())
		}
	}
	if len(names) != 2 {
		t.Errorf("one file per folder, and nothing left over: %v", names)
	}

	// Trusting twice changes nothing.
	if err := s.Trust(resolve(t, a, s.Home)); err != nil {
		t.Fatal(err)
	}
	if entries, _ := s.List(); len(entries) != 2 {
		t.Errorf("trusting twice: %+v", entries)
	}

	gone, err := s.Forget(a)
	if err != nil || !gone {
		t.Errorf("Forget: %v %v", gone, err)
	}
	if v := trusted(t, s, a); v.Trusted {
		t.Errorf("after forgetting: %+v", v)
	}
	if gone, err := s.Forget(a); err != nil || gone {
		t.Errorf("forgetting what is not there: %v %v", gone, err)
	}
	if v := trusted(t, s, b); !v.Trusted {
		t.Errorf("forgetting one folder left the other: %+v", v)
	}
}

// A folder is the same folder however it is reached.
func TestTrustThroughLinks(t *testing.T) {
	tmp := root(t)
	s := store(t, tmp)
	real := repo(t, filepath.Join(tmp, "real"), "")
	symlink(t, real, filepath.Join(tmp, "link"))
	if err := s.Trust(resolve(t, filepath.Join(tmp, "link"), s.Home)); err != nil {
		t.Fatal(err)
	}
	if v := trusted(t, s, real); !v.Trusted || v.By != real {
		t.Errorf("trusted through a link, asked by its real path: %+v", v)
	}
	if v := trusted(t, s, filepath.Join(tmp, "link", ".")); !v.Trusted {
		t.Errorf("asked through the link: %+v", v)
	}
	// A link that is later pointed somewhere else leads to another
	// folder, which is not trusted.
	other := repo(t, filepath.Join(tmp, "other"), "")
	if err := os.Remove(filepath.Join(tmp, "link")); err != nil {
		t.Fatal(err)
	}
	symlink(t, other, filepath.Join(tmp, "link"))
	if v := trusted(t, s, filepath.Join(tmp, "link")); v.Trusted {
		t.Errorf("a link pointed at another folder carried the trust along: %+v", v)
	}
}

// Worktrees share their repository's record. A folder that only claims to
// be a worktree does not.
func TestTrustAndWorktrees(t *testing.T) {
	tmp := root(t)
	s := store(t, tmp)
	main := repo(t, filepath.Join(tmp, "main"), "")
	wt := worktree(t, main, filepath.Join(tmp, "feature"), "feature")
	if err := s.Trust(resolve(t, wt, s.Home)); err != nil {
		t.Fatal(err)
	}
	if v := trusted(t, s, main); !v.Trusted || v.By != main {
		t.Errorf("trusting a worktree trusts the repository: %+v", v)
	}
	if v := trusted(t, s, wt); !v.Trusted {
		t.Errorf("the worktree: %+v", v)
	}
	if entries, _ := s.List(); len(entries) != 1 || entries[0].Path != main {
		t.Errorf("the record is kept under the main checkout: %+v", entries)
	}

	// SAFE-8 case 3: a .git that is a link to the trusted worktree's .git.
	evil := mkdir(t, tmp, "evil")
	symlink(t, filepath.Join(wt, ".git"), filepath.Join(evil, ".git"))
	if v := trusted(t, s, evil); v.Trusted {
		t.Errorf("a folder borrowed a trusted repository's trust through a linked .git: %+v", v)
	}
	// And one that names the trusted repository's record in a .git of
	// its own.
	copied := mkdir(t, tmp, "copied")
	write(t, filepath.Join(copied, ".git"), "gitdir: "+filepath.Join(main, ".git", "worktrees", "feature")+"\n")
	if v := trusted(t, s, copied); v.Trusted {
		t.Errorf("a folder borrowed a trusted repository's trust by naming it: %+v", v)
	}
	// SAFE-8 case 1: a repository that says its work tree is the trusted
	// folder.
	moved := repo(t, filepath.Join(tmp, "moved"), "\tworktree = "+main+"\n")
	if v := trusted(t, s, moved); v.Trusted {
		t.Errorf("a repository pointed its work tree at a trusted folder and was trusted: %+v", v)
	}
}

// Outside a repository, a trusted folder covers the folders under it that
// are not repositories of their own.
func TestTrustCoversPlainFoldersBelow(t *testing.T) {
	tmp := root(t)
	s := store(t, tmp)
	top := mkdir(t, tmp, "work")
	plain := mkdir(t, top, "notes", "2026")
	inner := repo(t, filepath.Join(top, "cloned"), "")
	if err := s.Trust(resolve(t, top, s.Home)); err != nil {
		t.Fatal(err)
	}
	if v := trusted(t, s, plain); !v.Trusted || v.By != top {
		t.Errorf("a plain folder under a trusted one: %+v", v)
	}
	if v := trusted(t, s, inner); v.Trusted {
		t.Errorf("a repository under a trusted folder is its own: %+v", v)
	}
	if v := trusted(t, s, mkdir(t, inner, "src")); v.Trusted {
		t.Errorf("a folder inside that repository: %+v", v)
	}
	if v := trusted(t, s, tmp); v.Trusted {
		t.Errorf("the folder above a trusted one: %+v", v)
	}
	// A trusted repository does not cover a repository nested in it.
	outer := repo(t, filepath.Join(tmp, "outer"), "")
	nested := repo(t, filepath.Join(outer, "third_party", "lib"), "")
	if err := s.Trust(resolve(t, outer, s.Home)); err != nil {
		t.Fatal(err)
	}
	if v := trusted(t, s, nested); v.Trusted {
		t.Errorf("a repository nested in a trusted one: %+v", v)
	}
}

// A folder that is moved leaves its record behind, and whatever is put at
// the old path afterwards is a different folder.
func TestMovedAndReplacedFolders(t *testing.T) {
	tmp := root(t)
	s := store(t, tmp)
	old := repo(t, filepath.Join(tmp, "proj"), "")
	if err := s.Trust(resolve(t, old, s.Home)); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(tmp, "proj-moved")
	if err := os.Rename(old, moved); err != nil {
		t.Fatal(err)
	}
	if v := trusted(t, s, moved); v.Trusted {
		t.Errorf("a folder moved elsewhere is asked about again: %+v", v)
	}
	if entries, notes := s.List(); len(entries) != 0 || len(notes) != 1 || !strings.Contains(notes[0], "no longer there") {
		t.Errorf("the record of a folder that is gone: %v %q", entries, notes)
	}

	// A stranger's repository lands at the trusted path.
	stranger := repo(t, old, "")
	v := trusted(t, s, stranger)
	if v.Trusted {
		t.Errorf("a different folder at a trusted path inherited the trust: %+v", v)
	}
	if !strings.Contains(v.Why, "not the one that was trusted") {
		t.Errorf("why it is asked about again is said: %q", v.Why)
	}
	if entries, notes := s.List(); len(entries) != 0 || len(notes) != 1 || !strings.Contains(notes[0], "not the one that was trusted") {
		t.Errorf("the record of a replaced folder: %v %q", entries, notes)
	}

	// Saying yes again records the folder that is there now.
	if err := s.Trust(resolve(t, stranger, s.Home)); err != nil {
		t.Fatal(err)
	}
	if v := trusted(t, s, stranger); !v.Trusted {
		t.Errorf("after trusting the new folder: %+v", v)
	}
	// Moved back, the first folder is again a different folder from the
	// one on record.
	if err := os.Rename(stranger, filepath.Join(tmp, "stranger")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(moved, old); err != nil {
		t.Fatal(err)
	}
	if v := trusted(t, s, old); v.Trusted {
		t.Errorf("the record now names the other folder: %+v", v)
	}
}

// A record that is not as gnosh wrote it does not count, and never stops
// anything: the folder is simply not trusted, and why is said.
func TestTamperedRecord(t *testing.T) {
	edit := func(t *testing.T, file string, change func(map[string]any)) {
		t.Helper()
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		change(m)
		out, _ := json.Marshal(m)
		if err := os.WriteFile(file, out, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tricks := []struct {
		name  string
		why   string
		plant func(t *testing.T, s *Store, dir, other string)
	}{
		{"the path edited to another folder", "not as gnosh wrote it", func(t *testing.T, s *Store, dir, other string) {
			edit(t, only(t, s), func(m map[string]any) { m["path"] = other })
		}},
		{"the folder's numbers edited", "not as gnosh wrote it", func(t *testing.T, s *Store, dir, other string) {
			edit(t, only(t, s), func(m map[string]any) { m["inode"] = 1 })
		}},
		{"the date edited", "not as gnosh wrote it", func(t *testing.T, s *Store, dir, other string) {
			edit(t, only(t, s), func(m map[string]any) { m["added"] = "2020-01-01T00:00:00Z" })
		}},
		{"the digest removed", "not as gnosh wrote it", func(t *testing.T, s *Store, dir, other string) {
			edit(t, only(t, s), func(m map[string]any) { delete(m, "check") })
		}},
		{"another version", "version this gnosh does not read", func(t *testing.T, s *Store, dir, other string) {
			edit(t, only(t, s), func(m map[string]any) { m["version"] = 2 })
		}},
		{"cut short", "not as gnosh wrote it", func(t *testing.T, s *Store, dir, other string) {
			file := only(t, s)
			data, _ := os.ReadFile(file)
			os.WriteFile(file, data[:len(data)/2], 0o600)
		}},
		{"emptied", "not as gnosh wrote it", func(t *testing.T, s *Store, dir, other string) {
			os.WriteFile(only(t, s), nil, 0o600)
		}},
		{"writable by others", "can be written by other users", func(t *testing.T, s *Store, dir, other string) {
			os.Chmod(only(t, s), 0o666)
		}},
		{"writable by the group", "chmod 600", func(t *testing.T, s *Store, dir, other string) {
			os.Chmod(only(t, s), 0o660)
		}},
		{"replaced by a link", "not an ordinary file", func(t *testing.T, s *Store, dir, other string) {
			file := only(t, s)
			elsewhere := filepath.Join(filepath.Dir(s.Dir), "elsewhere.json")
			os.Rename(file, elsewhere)
			os.Symlink(elsewhere, file)
		}},
		{"owned by someone else", "does not belong to you", func(t *testing.T, s *Store, dir, other string) {
			me := os.Getuid()
			s.UID = func() int { return me + 1 }
		}},
		{"a record copied to another folder's name", "not the record of the folder it names", func(t *testing.T, s *Store, dir, other string) {
			// The record for dir, put where other's record would be: an
			// attempt to pass one folder's trust to another.
			data, _ := os.ReadFile(only(t, s))
			os.WriteFile(s.fileFor(other), data, 0o600)
			os.Remove(s.fileFor(dir))
		}},
	}
	for _, tt := range tricks {
		t.Run(tt.name, func(t *testing.T) {
			tmp := root(t)
			s := store(t, tmp)
			dir := repo(t, filepath.Join(tmp, "proj"), "")
			other := repo(t, filepath.Join(tmp, "other"), "")
			if err := s.Trust(resolve(t, dir, s.Home)); err != nil {
				t.Fatal(err)
			}
			tt.plant(t, s, dir, other)
			for _, d := range []string{dir, other} {
				if v := trusted(t, s, d); v.Trusted {
					t.Errorf("%s is trusted by a record that is not as gnosh wrote it: %+v", d, v)
				}
			}
			why := trusted(t, s, dir).Why + trusted(t, s, other).Why
			entries, notes := s.List()
			if len(entries) != 0 {
				t.Errorf("List gives an entry that does not count: %+v", entries)
			}
			if !strings.Contains(why+strings.Join(notes, " "), tt.why) {
				t.Errorf("why the record does not count is said (%q): Check %q, List %q", tt.why, why, notes)
			}
			// Saying yes again puts a good record in its place.
			s.UID = nil
			if err := s.Trust(resolve(t, dir, s.Home)); err != nil {
				t.Fatalf("trusting again over a bad record: %v", err)
			}
			if v := trusted(t, s, dir); !v.Trusted {
				t.Errorf("after trusting again: %+v", v)
			}
		})
	}
}

// The home directory, and anything that holds it, is never recorded; a
// record of it written by something else does not count.
func TestHomeIsNeverRecorded(t *testing.T) {
	tmp := root(t)
	s := store(t, tmp)
	home := mkdir(t, tmp, "home")
	for _, dir := range []string{home, tmp} {
		f := resolve(t, dir, home)
		if err := s.Trust(f); !errors.Is(err, ErrSessionOnly) {
			t.Errorf("Trust(%s): %v, want ErrSessionOnly", dir, err)
		}
		if v := s.Check(f); v.Trusted || !strings.Contains(v.Why, "one session at a time") {
			t.Errorf("Check(%s): %+v", dir, v)
		}
	}
	if _, err := os.Stat(s.Dir); err == nil {
		t.Errorf("refusing to record the home directory still wrote something")
	}

	// A record for home, written by a store that did not know where home
	// is: once home is known, it does not count, and it covers nothing.
	blind := &Store{Dir: s.Dir, Now: s.Now}
	if err := blind.Trust(resolve(t, home, "")); err != nil {
		t.Fatal(err)
	}
	child := mkdir(t, home, "downloads", "stranger")
	if v := s.Check(resolve(t, child, home)); v.Trusted {
		t.Errorf("a record of the home directory covered a folder under it: %+v", v)
	}
	if entries, notes := s.List(); len(entries) != 0 || len(notes) != 1 || !strings.Contains(notes[0], "never recorded") {
		t.Errorf("List: %v %q", entries, notes)
	}
}

func TestTrustNeedsAFolder(t *testing.T) {
	tmp := root(t)
	s := store(t, tmp)
	f := resolve(t, mkdir(t, tmp, "gone"), s.Home)
	os.Remove(filepath.Join(tmp, "gone"))
	if err := s.Trust(f); err == nil || !strings.Contains(err.Error(), "not a folder that is there") {
		t.Errorf("trusting a folder that is gone: %v", err)
	}
	// A record directory that cannot be read lists nothing as trusted.
	write(t, s.Dir, "not a directory")
	if entries, notes := s.List(); len(entries) != 0 || len(notes) != 1 {
		t.Errorf("a record directory that is a file: %v %q", entries, notes)
	}
}

// Several gnosh processes trust and forget folders at the same time: no
// answer is lost, and no reader ever sees a record it cannot read.
func TestWritersAtOnce(t *testing.T) {
	tmp := root(t)
	const writers, folders = 8, 6
	dirs := make([][]string, writers)
	for w := range writers {
		for f := range folders {
			dirs[w] = append(dirs[w], repo(t, filepath.Join(tmp, "w", string(rune('a'+w)), string(rune('a'+f))), ""))
		}
	}
	shared := repo(t, filepath.Join(tmp, "shared"), "")
	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each writer is its own Store, as each process would be.
			s := store(t, tmp)
			for _, dir := range dirs[w] {
				f, err := Resolve(dir, s.Home)
				if err != nil {
					t.Error(err)
					return
				}
				if err := s.Trust(f); err != nil {
					t.Errorf("Trust(%s): %v", dir, err)
				}
				// Everyone also trusts, forgets and trusts one folder.
				sf, _ := Resolve(shared, s.Home)
				if err := s.Trust(sf); err != nil {
					t.Errorf("Trust(shared): %v", err)
				}
				if w%2 == 0 {
					s.Forget(shared)
					if err := s.Trust(sf); err != nil {
						t.Errorf("Trust(shared) again: %v", err)
					}
				}
				if v := s.Check(f); !v.Trusted {
					t.Errorf("a folder just trusted is not trusted: %s %+v", dir, v)
				}
				if _, notes := s.List(); len(notes) != 0 {
					t.Errorf("a reader saw a record that does not count: %q", notes)
				}
			}
		}()
	}
	wg.Wait()

	s := store(t, tmp)
	entries, notes := s.List()
	if len(notes) != 0 {
		t.Errorf("after the writers: %q", notes)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Path)
	}
	var want []string
	for _, ds := range dirs {
		want = append(want, ds...)
	}
	want = append(want, shared)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("an answer was lost: %d folders recorded, want %d", len(got), len(want))
	}
	names, _ := os.ReadDir(s.Dir)
	if len(names) != len(want) {
		t.Errorf("files left over in the record: %d files for %d folders", len(names), len(want))
	}
}

package apptest

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// World is a place for one test to run gnosh in: a home directory of its
// own, a project folder, and a scripted model server that the home's
// gnoshrc names as the model to use. Nothing in it is the real machine's:
// HOME and the three XDG directories lie inside a temporary directory.
type World struct {
	// Root is the temporary directory that holds everything. Home is the
	// home directory, and Dir the project folder gnosh is started in.
	Root, Home, Dir string
	// Model is the scripted model server.
	Model *Model
	// PathFirst are directories that stand before the machine's own on
	// the PATH gnosh is run with: where a test puts a stand-in for a
	// program gnosh starts.
	PathFirst []string

	t testing.TB
}

// NewWorld makes a world whose model answers with replies, in order.
func NewWorld(t testing.TB, replies ...Reply) *World {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := &World{Root: root, Home: filepath.Join(root, "home"), Dir: filepath.Join(root, "proj"), t: t}
	for _, d := range []string{w.Home, w.Dir, filepath.Join(root, "tmp"), w.ConfigDir()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	w.Model = NewModel(t, replies...)
	w.RC(
		"provider add local --type llamacpp --base-url "+w.Model.URL,
		"model large local/test-model",
	)
	return w
}

// ConfigDir is the user's config directory, and DataDir gnosh's own data
// directory.
func (w *World) ConfigDir() string { return filepath.Join(w.Home, ".config", "gnosh") }

// DataDir is where gnosh keeps its own data: sessions, state, trust.
func (w *World) DataDir() string { return filepath.Join(w.Home, ".local", "share", "gnosh") }

// Env is the environment gnosh is run with: the home and XDG directories
// of this world, the machine's PATH with PathFirst before it, and nothing
// else.
func (w *World) Env() []string {
	path := strings.Join(append(append([]string{}, w.PathFirst...), os.Getenv("PATH")), string(os.PathListSeparator))
	return []string{
		"HOME=" + w.Home,
		"XDG_CONFIG_HOME=" + filepath.Join(w.Home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(w.Home, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(w.Home, ".cache"),
		"TMPDIR=" + filepath.Join(w.Root, "tmp"),
		"PATH=" + path,
		"LANG=C.UTF-8",
	}
}

// FarMachine is a made-up machine for a world: a stand-in for ssh, first
// on the world's PATH, that writes down which destination it was called
// with and then runs the command it was given in a directory of its own,
// as the login shell of another machine would, or fails as it was told to.
// Nothing ever leaves this machine.
type FarMachine struct {
	// Home is the directory a command starts in on the made-up machine.
	Home string

	w   *World
	dir string
}

// The stand-in. ssh hands the last of its arguments to the login shell of
// the other machine; the destination is the argument after "--".
const standInSSH = `dir=$(dirname "$0")
dest= after=
for arg in "$@"; do
	if [ -n "$after" ] && [ -z "$dest" ]; then dest=$arg; fi
	if [ "$arg" = "--" ]; then after=yes; fi
	last=$arg
done
[ -n "$dest" ] || dest=$1
echo "$dest" >> "$dir/calls"
if [ -f "$dir/fail" ]; then
	cat "$dir/fail" >&2
	exit 255
fi
cd "$dir/home" || exit 255
exec env -i PATH="$PATH" HOME="$dir/home" /bin/sh -c "$last"`

// FarMachine sets the made-up machine up and returns it.
func (w *World) FarMachine() *FarMachine {
	w.t.Helper()
	dir := filepath.Join(w.Root, "far")
	f := &FarMachine{Home: filepath.Join(dir, "home"), w: w, dir: dir}
	if err := os.MkdirAll(f.Home, 0o755); err != nil {
		w.t.Fatal(err)
	}
	w.Script(filepath.Join(dir, "ssh"), standInSSH)
	w.PathFirst = append(w.PathFirst, dir)
	return f
}

// Calls returns the destinations the stand-in was called with, in order.
func (f *FarMachine) Calls() []string {
	data, _ := os.ReadFile(filepath.Join(f.dir, "calls"))
	return strings.Fields(string(data))
}

// Fail makes every later call of the stand-in fail as ssh fails, saying
// words on its standard error. Empty words make it work again.
func (f *FarMachine) Fail(words string) {
	f.w.t.Helper()
	path := filepath.Join(f.dir, "fail")
	if words == "" {
		os.Remove(path)
		return
	}
	if err := os.WriteFile(path, []byte(words+"\n"), 0o644); err != nil {
		f.w.t.Fatal(err)
	}
}

// RC adds lines to the user's gnoshrc.
func (w *World) RC(lines ...string) {
	w.t.Helper()
	path := filepath.Join(w.ConfigDir(), "gnoshrc")
	old, _ := os.ReadFile(path)
	text := string(old) + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		w.t.Fatal(err)
	}
}

// Write writes a file. A relative name is under the project folder.
// Intermediate directories are made. It returns the file's full path.
func (w *World) Write(name, content string) string {
	w.t.Helper()
	path := w.Path(name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		w.t.Fatal(err)
	}
	return path
}

// Script writes an executable shell script and returns its full path.
func (w *World) Script(name, body string) string {
	w.t.Helper()
	path := w.Write(name, "#!/bin/sh\n"+body+"\n")
	if err := os.Chmod(path, 0o755); err != nil {
		w.t.Fatal(err)
	}
	return path
}

// Path makes a name a full path: a relative one is under the project
// folder.
func (w *World) Path(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(w.Dir, name)
}

// Read returns a file's content, or fails the test.
func (w *World) Read(name string) string {
	w.t.Helper()
	data, err := os.ReadFile(w.Path(name))
	if err != nil {
		w.t.Fatal(err)
	}
	return string(data)
}

// Exists reports whether a file is there.
func (w *World) Exists(name string) bool {
	_, err := os.Stat(w.Path(name))
	return err == nil
}

// Sessions returns the session files of this world, sorted by the time
// they were last written, the oldest first.
func (w *World) Sessions() []string {
	w.t.Helper()
	found, _ := filepath.Glob(filepath.Join(w.DataDir(), "projects", "*", "*.jsonl"))
	sort.Slice(found, func(i, j int) bool {
		a, _ := os.Stat(found[i])
		b, _ := os.Stat(found[j])
		if a == nil || b == nil {
			return found[i] < found[j]
		}
		if !a.ModTime().Equal(b.ModTime()) {
			return a.ModTime().Before(b.ModTime())
		}
		return found[i] < found[j]
	})
	return found
}

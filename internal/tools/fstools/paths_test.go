package fstools

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/core"
)

func TestFollowLinks(t *testing.T) {
	f := newFixture(t, Options{})
	f.file("real/file.txt", "x")
	f.mkdir("real/sub")
	f.symlink("real", "dir-link")
	f.symlink("real/file.txt", "file-link")
	f.symlink(f.path("real/file.txt"), "abs-link")
	f.symlink("file-link", "link-to-link")
	f.symlink("../file.txt", "real/sub/up-link")
	f.symlink("nowhere/else", "dangling")
	f.symlink("dangling", "to-dangling")
	f.symlink("b-loop", "a-loop")
	f.symlink("a-loop", "b-loop")
	f.symlink(".", "dot-link")
	f.symlink("/", "root-link")

	tests := []struct {
		path   string
		want   string
		exists bool
	}{
		{"real/file.txt", "real/file.txt", true},
		{"real", "real", true},
		{"file-link", "real/file.txt", true},
		{"abs-link", "real/file.txt", true},
		{"link-to-link", "real/file.txt", true},
		{"dir-link/file.txt", "real/file.txt", true},
		{"dir-link/sub/up-link", "real/file.txt", true},
		{"dir-link", "real", true},
		{"dot-link/dot-link/real/file.txt", "real/file.txt", true},
		// Where the path stops existing, the rest is kept as written.
		{"missing", "missing", false},
		{"missing/deeper/file", "missing/deeper/file", false},
		{"real/new.txt", "real/new.txt", false},
		{"dir-link/new/file.txt", "real/new/file.txt", false},
		// A link to nothing leads to where the something would be.
		{"dangling", "nowhere/else", false},
		{"to-dangling", "nowhere/else", false},
		{"dangling/below", "nowhere/else/below", false},
	}
	for _, tt := range tests {
		got, exists, err := followLinks(f.path(tt.path))
		if err != nil {
			t.Errorf("followLinks(%s): %v", tt.path, err)
			continue
		}
		if got != f.path(tt.want) || exists != tt.exists {
			t.Errorf("followLinks(%s) = %s, %v; want %s, %v", tt.path, strings.TrimPrefix(got, f.dir+"/"), exists, tt.want, tt.exists)
		}
	}
	if got, exists, err := followLinks(f.path("root-link") + f.path("real/file.txt")); err != nil || got != f.path("real/file.txt") || !exists {
		t.Errorf("through a link to the root: %s, %v, %v", got, exists, err)
	}
	if got, exists, err := followLinks("/"); err != nil || got != "/" || !exists {
		t.Errorf("the root: %s, %v, %v", got, exists, err)
	}
	for _, bad := range []string{"a-loop", "b-loop/x", "real/file.txt/below"} {
		if _, _, err := followLinks(f.path(bad)); err == nil {
			t.Errorf("followLinks(%s) gave no error", bad)
		}
	}
	// What it finds is what the system finds, for paths that exist.
	for _, p := range []string{"file-link", "link-to-link", "dir-link/sub/up-link", "dot-link/real", "abs-link"} {
		want, err := filepath.EvalSymlinks(f.path(p))
		if err != nil {
			t.Fatal(err)
		}
		if got, _, _ := followLinks(f.path(p)); got != want {
			t.Errorf("followLinks(%s) = %s; the system says %s", p, got, want)
		}
	}
}

func TestLocate(t *testing.T) {
	f := newFixture(t, Options{})
	o := &f.opts
	p := f.file("a.txt", "x")
	loc, fail := o.locate(f.env, "Read", "file_path", p)
	if fail != nil || loc.abs != p || loc.real != p || !loc.exists || loc.info == nil || loc.info.Size() != 1 || loc.trouble != nil {
		t.Errorf("locate(%s) = %+v, %v", p, loc, fail)
	}
	if got := loc.paths(); len(got) != 1 || got[0] != p {
		t.Errorf("paths = %v", got)
	}
	if loc.viaLink() != "" {
		t.Errorf("viaLink = %q", loc.viaLink())
	}
	// Relative paths, cleaned.
	loc, fail = o.locate(f.env, "Read", "file_path", "x/../a.txt")
	if fail != nil || loc.abs != p {
		t.Errorf("relative: %+v, %v", loc, fail)
	}
	// A link.
	link := f.symlink("a.txt", "l")
	loc, _ = o.locate(f.env, "Read", "file_path", link)
	if loc.abs != link || loc.real != p || loc.info.Mode()&fs.ModeSymlink != 0 {
		t.Errorf("link: %+v", loc)
	}
	if got := loc.paths(); len(got) != 2 || got[0] != link || got[1] != p {
		t.Errorf("paths = %v", got)
	}
	if loc.viaLink() != " (which leads to "+p+")" {
		t.Errorf("viaLink = %q", loc.viaLink())
	}
	// Trouble on the way is carried, not returned: the path is still known.
	f.symlink("loop", "loop")
	loc, fail = o.locate(f.env, "Read", "file_path", f.path("loop"))
	if fail != nil || loc.trouble == nil || loc.abs != f.path("loop") || loc.real != "" {
		t.Errorf("loop: %+v, %v", loc, fail)
	}
	if got := loc.paths(); len(got) != 1 || got[0] != f.path("loop") {
		t.Errorf("paths = %v", got)
	}
	// What cannot be a path at all is a failure.
	for _, bad := range []string{"", "a\x00b", "~", "~/x", "~someone"} {
		if _, fail := o.locate(f.env, "Read", "file_path", bad); fail == nil {
			t.Errorf("locate(%q) gave no failure", bad)
		}
	}
	if _, fail := o.locate(&core.ToolEnv{Cwd: "not/absolute", Store: &sync.Map{}}, "Read", "file_path", "x"); fail == nil {
		t.Error("a relative path with a relative working directory gave no failure")
	}
	if _, fail := o.locate(nil, "Read", "file_path", "x"); fail == nil {
		t.Error("a relative path with no environment gave no failure")
	}
	if _, fail := o.locate(nil, "Read", "file_path", p); fail != nil {
		t.Errorf("an absolute path needs no environment: %v", fail)
	}
	// A home directory, when the options name one.
	home := Options{HomeDir: f.dir}.withDefaults()
	for arg, want := range map[string]string{"~": f.dir, "~/a.txt": p, "~/x/../a.txt": p} {
		loc, fail := home.locate(f.env, "Read", "file_path", arg)
		if fail != nil || loc.abs != want {
			t.Errorf("locate(%q) with a home directory = %s, %v; want %s", arg, loc.abs, fail, want)
		}
	}
	if _, fail := home.locate(f.env, "Read", "file_path", "~other"); fail == nil {
		t.Error("~other was expanded")
	}
}

func TestKindOf(t *testing.T) {
	for mode, want := range map[fs.FileMode]string{
		0o644:                             "a file",
		fs.ModeDir | 0o755:                "a directory",
		fs.ModeNamedPipe:                  "a named pipe (FIFO)",
		fs.ModeSocket:                     "a socket",
		fs.ModeDevice | fs.ModeCharDevice: "a character device",
		fs.ModeDevice:                     "a block device",
		fs.ModeSymlink:                    "a symbolic link",
		fs.ModeIrregular:                  "not a regular file",
	} {
		if got := kindOf(mode); got != want {
			t.Errorf("kindOf(%v) = %q, want %q", mode, got, want)
		}
	}
}

func TestByteSize(t *testing.T) {
	for n, want := range map[int64]string{
		0: "0 bytes", 1: "1 byte", 2: "2 bytes", 1023: "1023 bytes", 1024: "1 KiB", 1536: "1.5 KiB",
		100000: "97.7 KiB", 1 << 20: "1 MiB", 5 << 20: "5 MiB", 40 << 20: "40 MiB", 1 << 30: "1 GiB",
		3 << 29: "1.5 GiB", 150 << 20: "150 MiB", 1 << 40: "1 TiB", 5000 << 40: "5000 TiB",
	} {
		if got := byteSize(n); got != want {
			t.Errorf("byteSize(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestParseArgs(t *testing.T) {
	type result struct {
		n     int
		given bool
	}
	whole := func(raw string) (result, string) {
		a, f := parseArgs("Tool", json.RawMessage(`{"n":`+raw+`}`), "n")
		if f != nil {
			return result{}, f.msg
		}
		n, given, f := a.whole("n")
		if f != nil {
			return result{}, f.msg
		}
		return result{n, given}, ""
	}
	for raw, want := range map[string]result{
		"0": {0, true}, "7": {7, true}, "7.0": {7, true}, "7e1": {70, true}, `"7"`: {7, true}, `" 12 "`: {12, true},
		"null": {0, false}, "99999999999": {1<<31 - 1, true}, `"3.0"`: {3, true},
	} {
		if got, msg := whole(raw); msg != "" || got != want {
			t.Errorf("whole(%s) = %+v, %q; want %+v", raw, got, msg, want)
		}
	}
	for _, raw := range []string{"-1", "1.5", `"x"`, "true", "[1]", "{}", `""`, "1e400", `"NaN"`, `"-0.5"`} {
		if _, msg := whole(raw); msg == "" {
			t.Errorf("whole(%s) was accepted", raw)
		}
	}
	flag := func(raw string) (bool, string) {
		a, f := parseArgs("Tool", json.RawMessage(`{"b":`+raw+`}`), "b")
		if f != nil {
			return false, f.msg
		}
		v, _, f := a.flag("b")
		if f != nil {
			return false, f.msg
		}
		return v, ""
	}
	for raw, want := range map[string]bool{"true": true, "false": false, `"true"`: true, `"FALSE"`: false, `" True "`: true, "null": false} {
		if got, msg := flag(raw); msg != "" || got != want {
			t.Errorf("flag(%s) = %v, %q; want %v", raw, got, msg, want)
		}
	}
	for _, raw := range []string{"1", "0", `"yes"`, `""`, "[]", "{}"} {
		if _, msg := flag(raw); msg == "" {
			t.Errorf("flag(%s) was accepted", raw)
		}
	}
	// Text must be text; null is the same as leaving it out.
	a, f := parseArgs("Tool", json.RawMessage(`{"s":"x","e":"","z":null,"w":3}`), "s", "e", "z", "w", "missing")
	if f != nil {
		t.Fatal(f)
	}
	if v, given, f := a.text("s"); v != "x" || !given || f != nil {
		t.Errorf("text(s) = %q, %v, %v", v, given, f)
	}
	if v, given, f := a.text("e"); v != "" || !given || f != nil {
		t.Errorf("text(e) = %q, %v, %v", v, given, f)
	}
	for _, name := range []string{"z", "missing"} {
		if _, given, f := a.text(name); given || f != nil {
			t.Errorf("text(%s): given %v, %v", name, given, f)
		}
		if _, f := a.required(name); f == nil || !strings.Contains(f.msg, name+" is required") {
			t.Errorf("required(%s) = %v", name, f)
		}
	}
	if _, _, f := a.text("w"); f == nil || !strings.Contains(f.msg, "w must be a string, not a number") {
		t.Errorf("text(w) = %v", f)
	}
	if v, f := a.required("e"); v != "" || f != nil {
		t.Errorf("required(e) = %q, %v: an empty string is still given", v, f)
	}
	// What is not an object of known arguments.
	for raw, want := range map[string]string{
		`[1,2]`:          "not one JSON object",
		`"text"`:         "not one JSON object",
		`{"s":1,"x":2}`:  `takes no argument called "x"`,
		`{"s":`:          "not one JSON object",
		`{"S":"upper"}`:  `takes no argument called "S"`,
		`{"s ":"space"}`: `takes no argument called "s "`,
	} {
		if _, f := parseArgs("Tool", json.RawMessage(raw), "s"); f == nil || !strings.Contains(f.msg, want) {
			t.Errorf("parseArgs(%s) = %v, want a failure about %q", raw, f, want)
		}
	}
	if got := short("héllo wörld", 3); got != "hé…" {
		t.Errorf("short cut inside a character: %q", got)
	}
	if got := short("abc", 3); got != "abc" {
		t.Errorf("short changed a short string: %q", got)
	}
}

func TestMakeParents(t *testing.T) {
	f := newFixture(t, Options{})
	deep := f.path("a/b/c")
	made, err := makeParents(deep)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{f.path("a"), f.path("a/b"), deep}; len(made) != 3 || made[0] != want[0] || made[2] != want[2] {
		t.Errorf("made = %v, want %v", made, want)
	}
	// Nothing to make the second time.
	if made, err := makeParents(deep); err != nil || len(made) != 0 {
		t.Errorf("second time: %v, %v", made, err)
	}
	// Taking them away again leaves what was there before, and never a
	// directory that has gained something.
	os.WriteFile(filepath.Join(f.path("a/b"), "keep"), []byte("x"), 0o644)
	removeDirs(made)
	removeDirs([]string{f.path("a"), f.path("a/b"), deep})
	if _, err := os.Stat(deep); err == nil {
		t.Error("the innermost directory was not removed")
	}
	if _, err := os.Stat(f.path("a/b/keep")); err != nil {
		t.Error("a directory that had gained a file was removed")
	}
	// A file in the way.
	if _, err := makeParents(f.path("a/b/keep/below")); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("a file in the way: err = %v", err)
	}
}

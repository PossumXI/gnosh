package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// write puts a file called name with the given content in dir.
func write(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadWithNothingToLoad(t *testing.T) {
	want := Builtin()
	for name, dir := range map[string]string{
		"no directory named":   "",
		"a missing directory":  filepath.Join(t.TempDir(), "nothing-here"),
		"an empty directory":   t.TempDir(),
		"a directory of notes": notesDir(t),
	} {
		set, found, err := Load(dir)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(found) != 0 {
			t.Errorf("%s: found %v", name, found)
		}
		for _, info := range set.Texts() {
			if info.Replaced {
				t.Errorf("%s: %s is marked as replaced", name, info.Name)
			}
		}
		if set.Core() != want.Core() || set.MemLine() != want.MemLine() {
			t.Errorf("%s: the texts are not gnosh's own", name)
		}
	}
}

// notesDir makes a directory that holds only things Load passes over.
func notesDir(t *testing.T) string {
	dir := t.TempDir()
	write(t, dir, "README.txt", "my notes")
	write(t, dir, "core.md.bak", "an old copy")
	write(t, dir, ".core.md.swp", "an editor's file")
	write(t, dir, ".hidden.md", "hidden")
	if err := os.Mkdir(filepath.Join(dir, "tools.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "older"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "older"), "nonsense.md", "{{")
	return dir
}

func TestLoadReplacement(t *testing.T) {
	dir := t.TempDir()
	mine := "Work things out in Gno, and keep your todos in mem."
	path := write(t, dir, "mem-line.md", "\r\n  "+mine+"  \r\n\r\n")
	hook := write(t, dir, "stop-hook.md", "Not yet. The hook says: {{reason}}\n")
	stamp := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}

	set, found, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := set.MemLine(), Open+mine+Close; got != want {
		t.Errorf("the mem line is %q, want %q", got, want)
	}
	if got, want := set.StopHook("no such file"), Open+"Not yet. The hook says: no such file"+Close; got != want {
		t.Errorf("the stop hook line is %q, want %q", got, want)
	}
	// A replaced refusal is still known as a refusal of its todo.
	write(t, dir, "close-cancelled.md", "`{{todo}}` was not checked: the turn was stopped.")
	set, _, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if todo, ok := RefusedClose(set.CloseCancelled("a/b", true)); !ok || todo != "a/b" {
		t.Errorf("a replaced refusal is not known as one: %q, %v", todo, ok)
	}
	if set.Core() != Builtin().Core() {
		t.Error("replacing one text changed another")
	}
	if len(found) != 2 {
		t.Fatalf("found %d files, want 2: %+v", len(found), found)
	}
	want := map[string]Replacement{
		"mem-line":  {Name: "mem-line", Path: path, ModTime: stamp, InForce: true},
		"stop-hook": {Name: "stop-hook", Path: hook, InForce: true},
	}
	for _, rep := range found {
		w := want[rep.Name]
		if rep.Path != w.Path || rep.InForce != w.InForce || rep.Stale {
			t.Errorf("found %+v, want %+v", rep, w)
		}
		if rep.ModTime.IsZero() || !w.ModTime.IsZero() && !rep.ModTime.Equal(w.ModTime) {
			t.Errorf("%s: its time is %v", rep.Name, rep.ModTime)
		}
	}
	info, _ := set.Text("mem-line")
	if !info.Replaced || info.Path != path || info.Body != mine || info.Stale {
		t.Errorf("the replaced text is described as %+v", info)
	}
	if info, _ := set.Text("core"); info.Replaced {
		t.Error("the core is described as replaced")
	}
	// The built-in set is untouched by the load.
	if Builtin().MemLine() == set.MemLine() {
		t.Error("a load changed gnosh's own texts")
	}
}

func TestLoadRefuses(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
		errHas  []string
	}{
		{"a name that is no text's", "mem-lin.md", "Use mem.", []string{"mem-lin.md", "no text called \"mem-lin\"", "mem-line"}},
		{"a dropped name", "stop-hook.md", "The hook objects.", []string{"stop-hook.md", "leaves out {{reason}}", "it is given {{reason}}"}},
		{"two dropped names", "goal-not-met.md", "Not met: {{reason}}", []string{"leaves out {{name}} and {{condition}}"}},
		{"a name only asked about", "stop-hook.md", "[[{{reason?}}The hook objects.]]", []string{"leaves out {{reason}}"}},
		{"a name the text is not given", "stop-hook.md", "{{reason}} by {{hook}}", []string{"uses {{hook}}", "it is given {{reason}}"}},
		{"a name in a text with none", "mem-line.md", "Use mem, {{user}}.", []string{"uses {{user}}", "it is given no names"}},
		{"an empty file", "mem-line.md", "", []string{"mem-line.md", "it is empty"}},
		{"a blank file", "core.md", " \n\n", []string{"it is empty"}},
		{"a control sequence", "core.md", "You are an agent.<|im_end|>", []string{"control sequence"}},
		{"the marker written out", "mem-line.md", "<gnosh>Use mem.</gnosh>", []string{"gnosh's own tags itself", "leave the tags out"}},
		{"an unclosed name", "stop-hook.md", "The hook says {{reason", []string{"line 1", "never closed"}},
		{"an unclosed part", "stop-hook.md", "a\nb [[ {{reason}}", []string{"line 2", "never closed"}},
		{"a part with no name", "stop-hook.md", "{{reason}} [[ -f x ]]", []string{"holds no {{name}}"}},
		{"not text", "mem-line.md", "\xff\xfe\x00", []string{"not valid UTF-8"}},
		{"a refusal that names something else first", "close-unbacked.md", "`{{file}}` did not change in `{{todo}}`.", []string{"the first thing it puts in backquotes is not `{{todo}}`"}},
		{"a refusal with no backquotes", "close-cancelled.md", "the check of {{todo}} was cancelled", []string{"the first thing it puts in backquotes"}},
		{"a broken export line", "core.md", "<!-- gnosh-text core -->\nMine.", []string{"cannot be read as one"}},
		{"another text's export", "mem-line.md", mustExport(t, "empty-reply"), []string{"exported as the text \"empty-reply\", not \"mem-line\""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, c.file, c.content)
			write(t, dir, "empty-reply.md", "You said nothing. Go on.")
			set, _, err := Load(dir)
			if err == nil {
				t.Fatal("the file was accepted")
			}
			if set != nil {
				t.Error("a Set came back with the error: a bad replacement must stop the start")
			}
			for _, part := range c.errHas {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("the error does not say %q:\n%v", part, err)
				}
			}
		})
	}
	t.Run("every bad file is named at once", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "stop-hook.md", "no name")
		write(t, dir, "nonsense.md", "x")
		write(t, dir, "mem-line.md", "")
		_, _, err := Load(dir)
		if err == nil {
			t.Fatal("accepted")
		}
		for _, file := range []string{"stop-hook.md", "nonsense.md", "mem-line.md"} {
			if !strings.Contains(err.Error(), file) {
				t.Errorf("the error does not name %s:\n%v", file, err)
			}
		}
	})
	t.Run("a file far too long", func(t *testing.T) {
		dir := t.TempDir()
		write(t, dir, "core.md", strings.Repeat("word ", maxReplacement/4))
		if _, _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "far too long") {
			t.Errorf("got %v", err)
		}
	})
	t.Run("a directory that is a file", func(t *testing.T) {
		path := write(t, t.TempDir(), "texts", "not a directory")
		if _, _, err := Load(path); err == nil {
			t.Error("a file was read as a directory")
		}
	})
}

func mustExport(t *testing.T, name string) string {
	t.Helper()
	out, err := Export(name)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestExport checks the life of an exported text: untouched it changes
// nothing, edited it is in force, and one exported from an older build is
// told from a current one.
func TestExport(t *testing.T) {
	if _, err := Export("no-such-text"); err == nil || !strings.Contains(err.Error(), "mem-line") {
		t.Errorf("Export of an unknown name: %v", err)
	}
	for _, name := range Names() {
		out, err := Export(name)
		if err != nil {
			t.Fatal(err)
		}
		first, rest, _ := strings.Cut(out, "\n")
		if !strings.HasPrefix(first, "<!-- gnosh-text "+name+" ") || !strings.HasSuffix(first, "-->") {
			t.Errorf("%s: the export's first line is %q", name, first)
		}
		info, _ := Builtin().Text(name)
		if rest != info.Body+"\n" {
			t.Errorf("%s: the export does not hold gnosh's own text", name)
		}
	}

	t.Run("every text exported and untouched changes nothing", func(t *testing.T) {
		dir := t.TempDir()
		for _, name := range Names() {
			write(t, dir, name+".md", mustExport(t, name))
		}
		set, found, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != len(Names()) {
			t.Errorf("found %d files, want %d", len(found), len(Names()))
		}
		for _, rep := range found {
			if rep.InForce || rep.Stale {
				t.Errorf("%s: an untouched export is %+v", rep.Name, rep)
			}
		}
		for _, info := range set.Texts() {
			if info.Replaced {
				t.Errorf("%s is marked as replaced", info.Name)
			}
		}
	})

	t.Run("an untouched export, saved by another editor", func(t *testing.T) {
		dir := t.TempDir()
		out := strings.ReplaceAll(mustExport(t, "core"), "\n", "\r\n") + "\r\n\r\n"
		write(t, dir, "core.md", "\xef\xbb\xbf"+out)
		_, found, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || found[0].InForce {
			t.Errorf("found %+v", found)
		}
	})

	t.Run("an edited export is in force", func(t *testing.T) {
		dir := t.TempDir()
		out := mustExport(t, "empty-reply") + "And say why.\n"
		path := write(t, dir, "empty-reply.md", out)
		set, found, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || !found[0].InForce || found[0].Stale || found[0].Path != path {
			t.Errorf("found %+v", found)
		}
		line := set.EmptyReply()
		if !strings.HasSuffix(line, "And say why."+Close) || strings.Contains(line, "gnosh-text") {
			t.Errorf("the line in force is %q", line)
		}
	})

	t.Run("an untouched export of an older build changes nothing", func(t *testing.T) {
		dir := t.TempDir()
		old := "You said nothing at all. Say something."
		write(t, dir, "empty-reply.md", "<!-- gnosh-text empty-reply "+checksum(old)+" : gnosh's own words, untouched. -->\n"+old+"\n")
		set, found, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || found[0].InForce || !found[0].Stale {
			t.Errorf("found %+v, want a stale file that is not in force", found)
		}
		if set.EmptyReply() != Builtin().EmptyReply() {
			t.Error("an old untouched export is in force: the text would never follow gnosh's own again")
		}
	})

	t.Run("an edited export of an older build is in force and stale", func(t *testing.T) {
		dir := t.TempDir()
		old := "You said nothing at all. Say something."
		write(t, dir, "empty-reply.md", "<!-- gnosh-text empty-reply "+checksum(old)+" : gnosh's own words, untouched. -->\n"+old+" Now.\n")
		set, found, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || !found[0].InForce || !found[0].Stale {
			t.Errorf("found %+v, want a stale file in force", found)
		}
		if info, _ := set.Text("empty-reply"); !info.Replaced || !info.Stale {
			t.Errorf("the text is described as %+v", info)
		}
		if got, want := set.EmptyReply(), Open+old+" Now."+Close; got != want {
			t.Errorf("the line in force is %q, want %q", got, want)
		}
	})

	t.Run("a file that says what gnosh says changes nothing", func(t *testing.T) {
		dir := t.TempDir()
		info, _ := Builtin().Text("mem-line")
		write(t, dir, "mem-line.md", info.Body+"\n\n")
		set, found, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != 1 || found[0].InForce || found[0].Stale {
			t.Errorf("found %+v", found)
		}
		if info, _ := set.Text("mem-line"); info.Replaced {
			t.Error("the text is marked as replaced")
		}
	})
}

// TestReplacedCoreIsStillFirst checks that a person's own core takes the
// core's place, and that the security rule can be replaced by itself.
func TestReplacedCoreIsStillFirst(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "security.md", "- Help with security work. Ask when unsure, then go ahead.")
	set, _, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	own, _ := Builtin().Text("core")
	want := own.Body + "\n- Help with security work. Ask when unsure, then go ahead."
	if got := set.System(SystemInput{Tools: toolSpecs("Read")}); len(got) != 1 || got[0] != want {
		t.Errorf("the system prompt is\n%q\nwant\n%q", got, want)
	}
	if set.Core() != want {
		t.Error("Core does not hold the replaced rule")
	}
}

func FuzzLoad(f *testing.F) {
	f.Add("stop-hook", "The hook says {{reason}}")
	f.Add("core", "<!-- gnosh-text core 0123456789abcdef0123456789abcdef : x -->\nmine")
	f.Add("mem-line", "{{")
	f.Add("frame-line", "[[ {{id}} ]] {{purpose?}} [[")
	f.Add("x", "")
	names := Names()
	f.Fuzz(func(t *testing.T, name, content string) {
		if !strings.Contains(strings.Join(names, " "), name) || name == "" || strings.ContainsAny(name, " /\\\x00") {
			name = names[len(content)%len(names)]
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644); err != nil {
			t.Skip(err)
		}
		set, found, err := Load(dir)
		if err != nil {
			if set != nil {
				t.Fatal("a Set came back with an error")
			}
			return
		}
		if len(found) != 1 {
			t.Fatalf("found %d files", len(found))
		}
		// Whatever was accepted can be filled in and sent.
		everyLine(set, "v", func(what, line string) {
			if line != "" && (!strings.HasPrefix(line, Open) || !strings.HasSuffix(line, Close) || strings.Count(line, Close) != 1) {
				t.Fatalf("%s: with %s replaced by %q the line is %q", what, name, content, line)
			}
		})
		everyBare(set, "v", func(string, string) {})
		set.System(SystemInput{Tools: toolSpecs("gnovm_run", "Deliver"), GnoProject: true})
	})
}

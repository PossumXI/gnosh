package diff

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	ten := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	twenty := ten + "11\n12\n13\n14\n15\n16\n17\n18\n19\n20\n"
	tests := []struct {
		name          string
		before, after string
		opts          UnifiedOptions
		want          string
	}{
		{"the same", "a\nb\n", "a\nb\n", UnifiedOptions{}, ""},
		{"both empty", "", "", UnifiedOptions{}, ""},
		{
			"one line changed",
			"a\nb\nc\n", "a\nx\nc\n", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,3 +1,3 @@\n a\n-b\n+x\n c\n",
		},
		{
			"names",
			"a\n", "b\n", UnifiedOptions{OldName: "a/main.go", NewName: "b/main.go"},
			"--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-a\n+b\n",
		},
		{
			"a count of one is left out",
			"a\n", "a\nb\n", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1 +1,2 @@\n a\n+b\n",
		},
		{
			"a new file",
			"", "a\nb\n", UnifiedOptions{OldName: "/dev/null", NewName: "b/new.txt"},
			"--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,2 @@\n+a\n+b\n",
		},
		{
			"a deleted file",
			"a\nb\n", "", UnifiedOptions{OldName: "a/old.txt", NewName: "/dev/null"},
			"--- a/old.txt\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n",
		},
		{
			"three lines of context by default",
			ten, strings.Replace(ten, "5\n", "five\n", 1), UnifiedOptions{},
			"--- a\n+++ b\n@@ -2,7 +2,7 @@\n 2\n 3\n 4\n-5\n+five\n 6\n 7\n 8\n",
		},
		{
			"one line of context",
			ten, strings.Replace(ten, "5\n", "five\n", 1), UnifiedOptions{Context: 1},
			"--- a\n+++ b\n@@ -4,3 +4,3 @@\n 4\n-5\n+five\n 6\n",
		},
		{
			"no context",
			ten, strings.Replace(ten, "5\n", "five\n", 1), UnifiedOptions{Context: -1},
			"--- a\n+++ b\n@@ -5 +5 @@\n-5\n+five\n",
		},
		{
			"no context, a line inserted: the range names the line before",
			ten, strings.Replace(ten, "5\n", "5\nand a half\n", 1), UnifiedOptions{Context: -1},
			"--- a\n+++ b\n@@ -5,0 +6 @@\n+and a half\n",
		},
		{
			"no context, a line deleted",
			ten, strings.Replace(ten, "5\n", "", 1), UnifiedOptions{Context: -1},
			"--- a\n+++ b\n@@ -5 +4,0 @@\n-5\n",
		},
		{
			"two hunks",
			twenty, strings.Replace(strings.Replace(twenty, "3\n", "three\n", 1), "15\n", "fifteen\n", 1), UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,6 +1,6 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n@@ -12,7 +12,7 @@\n 12\n 13\n 14\n-15\n+fifteen\n 16\n 17\n 18\n",
		},
		{
			"hunks whose context touches are one hunk",
			twenty, strings.Replace(strings.Replace(twenty, "3\n", "three\n", 1), "10\n", "ten\n", 1), UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,13 +1,13 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n 7\n 8\n 9\n-10\n+ten\n 11\n 12\n 13\n",
		},
		{
			"hunks one line too far apart to touch stay two",
			twenty, strings.Replace(strings.Replace(twenty, "3\n", "three\n", 1), "11\n", "eleven\n", 1), UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,6 +1,6 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n@@ -8,7 +8,7 @@\n 8\n 9\n 10\n-11\n+eleven\n 12\n 13\n 14\n",
		},
		{
			"the final newline removed",
			"a\nb\n", "a\nb", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,2 +1,2 @@\n a\n-b\n+b\n\\ No newline at end of file\n",
		},
		{
			"the final newline added",
			"a\nb", "a\nb\n", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n",
		},
		{
			"no final newline on either side, the last line changed",
			"a\nb", "a\nc", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+c\n\\ No newline at end of file\n",
		},
		{
			"no final newline on either side, the last line kept",
			"a\nb", "x\nb", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,2 +1,2 @@\n-a\n+x\n b\n\\ No newline at end of file\n",
		},
		{
			"an open new file",
			"", "a", UnifiedOptions{},
			"--- a\n+++ b\n@@ -0,0 +1 @@\n+a\n\\ No newline at end of file\n",
		},
		{
			"CRLF lines are written as they are",
			"a\r\nb\r\n", "a\r\nx\r\n", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,2 +1,2 @@\n a\r\n-b\r\n+x\r\n",
		},
		{
			"line endings changed: every line differs",
			"a\nb\n", "a\r\nb\r\n", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,2 +1,2 @@\n-a\n-b\n+a\r\n+b\r\n",
		},
		{
			"a name with a space is followed by a tab",
			"a\n", "b\n", UnifiedOptions{OldName: "my file.txt", NewName: "my file.txt"},
			"--- my file.txt\t\n+++ my file.txt\t\n@@ -1 +1 @@\n-a\n+b\n",
		},
		{
			"a name that would break the header is quoted",
			"a\n", "b\n", UnifiedOptions{OldName: "odd\"name\\\n\x01", NewName: "tab\there"},
			"--- \"odd\\\"name\\\\\\n\\001\"\n+++ \"tab\\there\"\n@@ -1 +1 @@\n-a\n+b\n",
		},
		{
			"lines that look like headers",
			"--- a\n+++ b\n@@ -1 +1 @@\n", "--- a\n++ b\n@@ -1 +1 @@\n", UnifiedOptions{},
			"--- a\n+++ b\n@@ -1,3 +1,3 @@\n --- a\n-+++ b\n+++ b\n @@ -1 +1 @@\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Unified(tt.before, tt.after, tt.opts)
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
			back, err := applyUnified(tt.before, got)
			if err != nil {
				t.Fatalf("the diff does not apply: %v\n%s", err, got)
			}
			if back != tt.after {
				t.Errorf("applied, the diff gives %q, want %q", back, tt.after)
			}
		})
	}
}

// TestUnifiedApplies checks, for texts of every kind and every amount of
// context, that the unified form read back strictly gives the text after.
func TestUnifiedApplies(t *testing.T) {
	s := newSource(21)
	for i := range times(2000) {
		var before, after string
		switch i % 3 {
		case 0:
			before, after = s.pair(80, 1+s.IntN(8))
		case 1:
			before, after = s.text(s.lines(s.IntN(30))), s.text(s.lines(s.IntN(30)))
		case 2:
			before, after = s.noise(s.IntN(300)), s.noise(s.IntN(300))
		}
		for _, context := range []int{0, -1, 1, 2, 7} {
			patch := Unified(before, after, UnifiedOptions{Context: context})
			got, err := applyUnified(before, patch)
			if err != nil {
				t.Fatalf("context %d: the diff does not apply: %v\nbefore: %q\nafter:  %q\n%s", context, err, before, after, patch)
			}
			if got != after {
				t.Fatalf("context %d: applied, the diff gives %q\nbefore: %q\nafter:  %q\n%s", context, got, before, after, patch)
			}
			if (patch == "") != (before == after) {
				t.Fatalf("the diff is empty: %v, the texts are the same: %v", patch == "", before == after)
			}
		}
	}
}

// gnu returns the path of a program if it is installed and is the GNU one,
// and skips the test otherwise: the formats agree everywhere, but what each
// program makes of an odd name or of bytes that are not text does not.
func gnu(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed", name)
	}
	version, err := exec.Command(path, "--version").Output()
	if err != nil || !bytes.Contains(version, []byte("GNU")) {
		t.Skipf("%s is not GNU %s", path, name)
	}
	return path
}

// patched applies a diff to a file holding before, with patch, and returns
// what the file holds afterwards.
func patched(t *testing.T, patch, dir string, n int, before, diff string) string {
	t.Helper()
	file := filepath.Join(dir, "text"+strconv.Itoa(n))
	out := file + ".out"
	if err := os.WriteFile(file, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file+".diff", []byte(diff), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(patch, "--quiet", "--batch", "-o", out, file, file+".diff")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("patch refused the diff: %v\n%s\nbefore: %q\n%s", err, msg, before, diff)
	}
	got, err := os.ReadFile(out)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(got)
}

// TestUnifiedWithPatch gives the unified form to the real patch program and
// checks that it makes the text after out of the text before.
func TestUnifiedWithPatch(t *testing.T) {
	patch := gnu(t, "patch")
	dir := t.TempDir()
	s := newSource(22)
	n := 0
	check := func(before, after string, context int) {
		t.Helper()
		diff := Unified(before, after, UnifiedOptions{Context: context})
		if diff == "" {
			if before != after {
				t.Fatalf("no diff for texts that differ\nbefore: %q\nafter:  %q", before, after)
			}
			return
		}
		n++
		if got := patched(t, patch, dir, n, before, diff); got != after {
			t.Fatalf("context %d: patch made %q\nbefore: %q\nafter:  %q\n%s", context, got, before, after, diff)
		}
	}

	// The cases that have a rule of their own in the format.
	for _, tt := range []struct{ before, after string }{
		{"", "a\n"}, {"", "a"}, {"a\n", ""}, {"a", ""},
		{"a\nb\n", "a\nb"}, {"a\nb", "a\nb\n"}, {"a\nb", "a\nc"}, {"a\nb", "x\nb"},
		{"a\nb\n", "a\r\nb\r\n"}, {"a\r\nb\r\n", "a\nb\n"}, {"a\r\nb\r\n", "a\r\nx\r\n"},
		{"a\nb\nc\n", "a\nb\r\nc\n"}, {"\n", "\n\n"}, {"\n\n", "\n"}, {"a\n", "\n"},
		{"--- a\n+++ b\n", "--- a\n++ b\n"}, {"\\ No newline\n", "\\ No newline"},
		{"x\n", "\\\n"}, {"a\tb \n", "a\tb\n"},
	} {
		for _, context := range []int{0, -1, 1} {
			check(tt.before, tt.after, context)
		}
	}
	// Texts of every kind.
	for i := range 400 {
		var before, after string
		switch i % 3 {
		case 0:
			before, after = s.pair(80, 1+s.IntN(8))
		case 1:
			before, after = s.text(s.lines(s.IntN(30))), s.text(s.lines(s.IntN(30)))
		case 2:
			before, after = s.pair(600, 5)
		}
		check(before, after, []int{0, -1, 1, 5}[i%4])
	}
}

// TestUnifiedNamesWithPatch checks that patch finds the file to change from
// the names in the header, when a name has a space in it or needs quoting.
func TestUnifiedNamesWithPatch(t *testing.T) {
	patch := gnu(t, "patch")
	for _, name := range []string{"plain.txt", "my file.txt", "two  spaces and more.txt", "quote\"d.txt", "back\\slash.txt", "tab\there.txt"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			before, after := "one\ntwo\nthree\n", "one\n2\nthree\n"
			if err := os.WriteFile(filepath.Join(dir, name), []byte(before), 0o600); err != nil {
				t.Fatal(err)
			}
			diff := Unified(before, after, UnifiedOptions{OldName: name, NewName: name})
			cmd := exec.Command(patch, "--quiet", "--batch", "-p0")
			cmd.Dir = dir
			cmd.Stdin = strings.NewReader(diff)
			if msg, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("patch refused the diff: %v\n%s\n%s", err, msg, diff)
			}
			got, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != after {
				t.Errorf("patch made %q of the file, want %q\n%s", got, after, diff)
			}
		})
	}
}

// TestUnifiedAgainstGNUDiff compares with what GNU diff makes of the same
// two texts when it is asked for the smallest change: the two may put a
// change in different places, but they must delete and insert as many
// lines, and GNU's output must mean the same change.
func TestUnifiedAgainstGNUDiff(t *testing.T) {
	diff := gnu(t, "diff")
	dir := t.TempDir()
	s := newSource(23)
	count := func(patch string) (minus, plus int) {
		for i, line := range strings.SplitAfter(patch, "\n") {
			switch {
			case i < 2 || line == "":
			case line[0] == '-':
				minus++
			case line[0] == '+':
				plus++
			}
		}
		return minus, plus
	}
	for i := range 300 {
		var before, after string
		switch i % 3 {
		case 0:
			before, after = s.pair(80, 1+s.IntN(8))
		case 1:
			before, after = s.text(s.lines(s.IntN(30))), s.text(s.lines(s.IntN(30)))
		case 2:
			before, after = s.pair(600, 5)
		}
		if before == after {
			continue
		}
		fa, fb := filepath.Join(dir, "a"), filepath.Join(dir, "b")
		if err := os.WriteFile(fa, []byte(before), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fb, []byte(after), 0o600); err != nil {
			t.Fatal(err)
		}
		// -d asks for the smallest change, -a takes any bytes as text.
		theirs, err := exec.Command(diff, "-u", "-d", "-a", "--label", "a", "--label", "b", fa, fb).Output()
		var exit *exec.ExitError
		if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
			t.Fatalf("diff failed: %v", err)
		}
		ours := Unified(before, after, UnifiedOptions{})
		theirMinus, theirPlus := count(string(theirs))
		ourMinus, ourPlus := count(ours)
		if theirMinus != ourMinus || theirPlus != ourPlus {
			t.Fatalf("GNU diff deletes %d lines and inserts %d; this package deletes %d and inserts %d\nbefore: %q\nafter:  %q\ntheirs:\n%s\nours:\n%s",
				theirMinus, theirPlus, ourMinus, ourPlus, before, after, theirs, ours)
		}
		// The reader these tests use must also read GNU's own output.
		got, err := applyUnified(before, string(theirs))
		if err != nil || got != after {
			t.Fatalf("GNU diff's output does not read back: %v\n%s", err, theirs)
		}
	}
}

func TestHunks(t *testing.T) {
	ten := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
	type head struct{ oldStart, oldLines, newStart, newLines int }
	tests := []struct {
		name          string
		before, after string
		context       int
		want          []head
		edits         []string
	}{
		{"no changes", ten, ten, 0, nil, nil},
		{"both empty", "", "", 0, nil, nil},
		{
			"one change", ten, strings.Replace(ten, "5\n", "five\n", 1), 0,
			[]head{{2, 7, 2, 7}},
			[]string{`=2\n =3\n =4\n -5\n +five\n =6\n =7\n =8\n`},
		},
		{
			"at the top", ten, "one\n" + ten[2:], 2,
			[]head{{1, 3, 1, 3}},
			[]string{`-1\n +one\n =2\n =3\n`},
		},
		{
			"at the bottom", ten, ten[:len(ten)-3] + "ten\n", 2,
			[]head{{8, 3, 8, 3}},
			[]string{`=8\n =9\n -10\n +ten\n`},
		},
		{
			"two hunks", ten, "one\n" + ten[2:len(ten)-3] + "ten\n", 1,
			[]head{{1, 2, 1, 2}, {9, 2, 9, 2}},
			[]string{`-1\n +one\n =2\n`, `=9\n -10\n +ten\n`},
		},
		{
			"joined by their context", ten, "one\n2\n3\nfour\n" + ten[8:], 1,
			[]head{{1, 5, 1, 5}},
			[]string{`-1\n +one\n =2\n =3\n -4\n +four\n =5\n`},
		},
		{
			"no context", ten, strings.Replace(ten, "5\n", "", 1), -1,
			[]head{{5, 1, 4, 0}},
			[]string{`-5\n`},
		},
		{
			"an insert with no context", ten, strings.Replace(ten, "5\n", "5\nmore\n", 1), -1,
			[]head{{5, 0, 6, 1}},
			[]string{`+more\n`},
		},
		{"a new text", "", "a\nb\n", 0, []head{{0, 0, 1, 2}}, []string{`+a\n +b\n`}},
		{"a text emptied", "a\nb\n", "", 0, []head{{1, 2, 0, 0}}, []string{`-a\n -b\n`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hunks := Hunks(Lines(tt.before, tt.after), tt.context)
			if len(hunks) != len(tt.want) {
				t.Fatalf("got %d hunks, want %d: %+v", len(hunks), len(tt.want), hunks)
			}
			for i, h := range hunks {
				if got := (head{h.OldStart, h.OldLines, h.NewStart, h.NewLines}); got != tt.want[i] {
					t.Errorf("hunk %d is at %+v, want %+v", i, got, tt.want[i])
				}
				if got := show(h.Edits); got != tt.edits[i] {
					t.Errorf("hunk %d holds %s, want %s", i, got, tt.edits[i])
				}
			}
		})
	}
}

// TestHunksCoverEveryChange checks that hunks leave no change out, add up to
// what their headers say, and do not overlap.
func TestHunksCoverEveryChange(t *testing.T) {
	s := newSource(24)
	for range 1000 {
		before, after := s.pair(120, 1+s.IntN(10))
		edits := Lines(before, after)
		changed := 0
		for _, e := range edits {
			if e.Op != Keep {
				changed += len(e.Lines)
			}
		}
		for _, context := range []int{-1, 0, 1, 4} {
			inHunks, lastOld, lastNew := 0, 0, 0
			for _, h := range Hunks(edits, context) {
				oldLines, newLines := 0, 0
				for _, e := range h.Edits {
					switch e.Op {
					case Keep:
						oldLines += len(e.Lines)
						newLines += len(e.Lines)
					case Delete:
						oldLines += len(e.Lines)
						inHunks += len(e.Lines)
					case Insert:
						newLines += len(e.Lines)
						inHunks += len(e.Lines)
					}
				}
				if oldLines != h.OldLines || newLines != h.NewLines {
					t.Fatalf("a hunk says %d and %d lines and holds %d and %d", h.OldLines, h.NewLines, oldLines, newLines)
				}
				first := h.OldStart
				if h.OldLines > 0 {
					first--
				}
				if first < lastOld {
					t.Fatalf("a hunk starts at line %d of the old text, before the last one ended at %d", first+1, lastOld)
				}
				lastOld = first + h.OldLines
				first = h.NewStart
				if h.NewLines > 0 {
					first--
				}
				if first < lastNew {
					t.Fatalf("a hunk starts at line %d of the new text, before the last one ended at %d", first+1, lastNew)
				}
				lastNew = first + h.NewLines
			}
			if inHunks != changed {
				t.Fatalf("context %d: the hunks hold %d changed lines of %d", context, inHunks, changed)
			}
		}
	}
}

package diff

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
)

// apply runs an edit script against before: kept lines are copied, deleted
// lines are passed over, inserted lines are written. It fails when the
// script does not fit the text: a kept or deleted line that is not the next
// line of before, or lines of before the script never reaches.
func apply(before string, edits []Edit) (string, error) {
	rest := before
	var out strings.Builder
	for i, e := range edits {
		switch e.Op {
		case Keep, Delete:
			for _, line := range e.Lines {
				if !strings.HasPrefix(rest, line) {
					return "", fmt.Errorf("edit %d (%v): line %q is not what comes next in the text before, %q", i, e.Op, line, clip(rest))
				}
				rest = rest[len(line):]
				if e.Op == Keep {
					out.WriteString(line)
				}
			}
		case Insert:
			for _, line := range e.Lines {
				out.WriteString(line)
			}
		default:
			return "", fmt.Errorf("edit %d has the unknown op %d", i, e.Op)
		}
	}
	if rest != "" {
		return "", fmt.Errorf("the script ends before the text does: %q is left", clip(rest))
	}
	return out.String(), nil
}

// clip shortens a string for an error message.
func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "..."
	}
	return s
}

// checkScript fails the test unless edits is a script that turns before into
// after and keeps every promise Lines makes about its shape. It returns the
// number of lines the script deletes and inserts.
func checkScript(t testing.TB, before, after string, edits []Edit) (changed int) {
	t.Helper()
	got, err := apply(before, edits)
	if err != nil {
		t.Fatalf("the script does not apply: %v\nbefore: %q\nafter:  %q", err, before, after)
	}
	if got != after {
		t.Fatalf("the script gives the wrong text\nbefore: %q\nafter:  %q\ngot:    %q", before, after, got)
	}
	var kept, gone strings.Builder
	for i, e := range edits {
		if len(e.Lines) == 0 {
			t.Fatalf("edit %d (%v) has no lines", i, e.Op)
		}
		if i > 0 && edits[i-1].Op == e.Op {
			t.Fatalf("edits %d and %d are both %v: runs are not joined", i-1, i, e.Op)
		}
		if i > 0 && edits[i-1].Op == Insert && e.Op == Delete {
			t.Fatalf("edit %d is a Delete right after an Insert: the Delete comes first", i)
		}
		for _, line := range e.Lines {
			if line == "" || strings.Contains(line[:len(line)-1], "\n") {
				t.Fatalf("edit %d holds %q, which is not one line", i, line)
			}
		}
		switch e.Op {
		case Keep:
			kept.WriteString(strings.Join(e.Lines, ""))
			gone.WriteString(strings.Join(e.Lines, ""))
		case Delete:
			gone.WriteString(strings.Join(e.Lines, ""))
			changed += len(e.Lines)
		case Insert:
			kept.WriteString(strings.Join(e.Lines, ""))
			changed += len(e.Lines)
		}
	}
	if gone.String() != before {
		t.Fatalf("the Keep and Delete lines do not join to the text before")
	}
	if kept.String() != after {
		t.Fatalf("the Keep and Insert lines do not join to the text after")
	}
	return changed
}

// smallest returns the size of the smallest script between two texts: the
// lines of both, less twice the longest run of lines they have in common in
// order. It is worked out the slow, plain way, a table with one entry for
// every pair of lines, and is what the real comparison is checked against.
func smallest(before, after string) int {
	a, b := splitLines(before), splitLines(after)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				cur[j] = prev[j-1] + 1
			} else {
				cur[j] = max(prev[j], cur[j-1])
			}
		}
		prev, cur = cur, prev
	}
	return len(a) + len(b) - 2*prev[len(b)]
}

// applyUnified applies a unified diff to before, checking every line the
// diff says is there, and returns the result. It reads the format strictly,
// as a second opinion beside patch: ranges must add up and context must
// match where the ranges say it is.
func applyUnified(before, patch string) (string, error) {
	if patch == "" {
		return before, nil
	}
	old := splitLines(before)
	lines := strings.SplitAfter(patch, "\n")
	if n := len(lines); lines[n-1] == "" {
		lines = lines[:n-1]
	} else {
		return "", fmt.Errorf("the diff does not end with a newline")
	}
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "--- ") || !strings.HasPrefix(lines[1], "+++ ") {
		return "", fmt.Errorf("the diff does not start with a --- and a +++ line")
	}
	var out strings.Builder
	at := 0      // lines of old used so far
	written := 0 // lines of the new text written so far
	for i := 2; i < len(lines); {
		head := lines[i]
		i++
		var oldStart, oldCount, newStart, newCount int
		if err := parseHunkHead(head, &oldStart, &oldCount, &newStart, &newCount); err != nil {
			return "", err
		}
		first := oldStart - 1
		if oldCount == 0 {
			first = oldStart
		}
		if first < at || first > len(old) {
			return "", fmt.Errorf("hunk %q starts at line %d, but %d lines are used and the text has %d", strings.TrimSpace(head), oldStart, at, len(old))
		}
		for ; at < first; at++ {
			out.WriteString(old[at])
			written++
		}
		if want := newStart - 1; newCount > 0 && written != want || newCount == 0 && written != newStart {
			return "", fmt.Errorf("hunk %q says it starts at line %d of the new text, but %d lines are written", strings.TrimSpace(head), newStart, written)
		}
		usedOld, usedNew := 0, 0
		for usedOld < oldCount || usedNew < newCount {
			if i >= len(lines) {
				return "", fmt.Errorf("the diff ends inside hunk %q", strings.TrimSpace(head))
			}
			line := lines[i]
			i++
			if line == "" || line[len(line)-1] != '\n' {
				return "", fmt.Errorf("line %q of the diff is not ended", line)
			}
			sign, text := line[0], line[1:]
			// The line after may say that this one has no newline.
			if i < len(lines) && strings.HasPrefix(lines[i], "\\") {
				text = text[:len(text)-1]
				i++
			}
			switch sign {
			case ' ', '-':
				if at >= len(old) || old[at] != text {
					have := "nothing"
					if at < len(old) {
						have = strconv.Quote(old[at])
					}
					return "", fmt.Errorf("hunk %q expects %q at line %d of the old text, which has %s", strings.TrimSpace(head), text, at+1, have)
				}
				at++
				usedOld++
				if sign == ' ' {
					out.WriteString(text)
					usedNew++
					written++
				}
			case '+':
				out.WriteString(text)
				usedNew++
				written++
			default:
				return "", fmt.Errorf("line %q of the diff starts with neither a space, a minus nor a plus", line)
			}
		}
		if usedOld != oldCount || usedNew != newCount {
			return "", fmt.Errorf("hunk %q holds %d old and %d new lines", strings.TrimSpace(head), usedOld, usedNew)
		}
	}
	for ; at < len(old); at++ {
		out.WriteString(old[at])
	}
	return out.String(), nil
}

// parseHunkHead reads "@@ -a,b +c,d @@", where ",b" and ",d" may be left out
// and then mean one line.
func parseHunkHead(head string, oldStart, oldCount, newStart, newCount *int) error {
	fields := strings.Fields(head)
	if len(fields) != 4 || fields[0] != "@@" || fields[3] != "@@" ||
		!strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return fmt.Errorf("%q is not a hunk header", strings.TrimSpace(head))
	}
	read := func(s string, start, count *int) error {
		first, n, both := strings.Cut(s, ",")
		var err error
		if *start, err = strconv.Atoi(first); err != nil {
			return fmt.Errorf("%q is not a hunk header: %v", strings.TrimSpace(head), err)
		}
		*count = 1
		if both {
			if *count, err = strconv.Atoi(n); err != nil {
				return fmt.Errorf("%q is not a hunk header: %v", strings.TrimSpace(head), err)
			}
		}
		return nil
	}
	if err := read(fields[1][1:], oldStart, oldCount); err != nil {
		return err
	}
	return read(fields[2][1:], newStart, newCount)
}

// workBound is the most line comparisons a comparison of texts of n and m
// lines makes under a budget, as Compare's documentation states it.
func workBound(n, m, budget int) int {
	return budget + 1000*(n+m)
}

// times is how often a test repeats a check that it repeats n times in an
// ordinary run: a third as often under the race detector.
func times(n int) int {
	if underRace {
		return n / 3
	}
	return n
}

// A source makes up texts for the tests, the same ones on every run.
type source struct {
	*rand.Rand
}

// newSource returns a source that always gives the same texts for the same
// seed.
func newSource(seed uint64) source {
	return source{rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

// The lines a made-up text is built from. Many of them repeat, as the
// braces and blank lines of real code do: repeated lines are where a
// comparison has choices to make.
var codeLines = []string{
	"", "", "", "}", "}", "\t}", "\t}", "{", "\treturn nil", "\treturn err", "\tif err != nil {",
	"func main() {", "\tfor i := range n {", "\t\tx++", "\t\ty--", "// a comment", "import \"fmt\"",
	"\tdefault:", "\tcase 1:", "var x int", "    indented with spaces", "日本語の行", "emoji 🎉 line",
}

// line makes up one line, without its ending.
func (s source) line() string {
	switch s.IntN(10) {
	case 0, 1, 2:
		return "line " + strconv.Itoa(s.IntN(1000000)) // almost surely like no other
	case 3:
		return strings.Repeat(" ", s.IntN(6)) + "word" + strconv.Itoa(s.IntN(5))
	}
	return codeLines[s.IntN(len(codeLines))]
}

// lines makes up n lines, without their endings.
func (s source) lines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s.line()
	}
	return out
}

// text joins lines into a text. Most texts end every line with LF; some use
// CRLF throughout, some mix the two, and some leave the last line open.
func (s source) text(lines []string) string {
	style := s.IntN(10) // 0: CRLF, 1: mixed, otherwise LF
	var b strings.Builder
	for i, line := range lines {
		b.WriteString(line)
		if i == len(lines)-1 && s.IntN(6) == 0 {
			break
		}
		if style == 0 || style == 1 && s.IntN(3) == 0 {
			b.WriteString("\r\n")
		} else {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// edited returns lines after a few of the things an editor does to a file:
// lines deleted, inserted, replaced, copied and moved, alone or in blocks.
func (s source) edited(lines []string, changes int) []string {
	out := append([]string(nil), lines...)
	for range changes {
		at := 0
		if len(out) > 0 {
			at = s.IntN(len(out) + 1)
		}
		span := min(1+s.IntN(4), len(out)-at)
		switch s.IntN(6) {
		case 0: // delete
			out = append(out[:at], out[at+span:]...)
		case 1: // insert
			out = append(out[:at], append(s.lines(1+s.IntN(4)), out[at:]...)...)
		case 2: // replace
			for k := at; k < at+span; k++ {
				out[k] = s.line()
			}
		case 3: // change a little
			for k := at; k < at+span; k++ {
				out[k] += " // changed"
			}
		case 4: // copy a block to somewhere else
			block := append([]string(nil), out[at:at+span]...)
			to := s.IntN(len(out) + 1)
			out = append(out[:to], append(block, out[to:]...)...)
		case 5: // move a block
			block := append([]string(nil), out[at:at+span]...)
			out = append(out[:at], out[at+span:]...)
			to := s.IntN(len(out) + 1)
			out = append(out[:to], append(block, out[to:]...)...)
		}
	}
	return out
}

// pair makes up a text and an edited form of it.
func (s source) pair(maxLines, changes int) (before, after string) {
	lines := s.lines(s.IntN(maxLines + 1))
	return s.text(lines), s.text(s.edited(lines, changes))
}

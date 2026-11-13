package fstools

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"unicode/utf8"
)

// toLF reads every CRLF as a plain line break, the slow and obvious way.
func toLF(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

// loneCR reports whether s holds a carriage return that is not the first
// half of a CRLF.
func loneCR(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\r' && (i+1 >= len(s) || s[i+1] != '\n') {
			return true
		}
	}
	return false
}

func FuzzEditSubstitute(f *testing.F) {
	seeds := []struct {
		body, old, repl string
		all             bool
	}{
		{"one\ntwo\nthree\n", "two", "2", false},
		{"one\r\ntwo\r\nthree\r\n", "one\ntwo", "a\nb\nc", false},
		{"a\r\nb\nc\r\n", "b", "x\ny", true},
		{"k\r\nv\r\nk\nv\n", "k\n", "k\nextra\n", true},
		{"abc", "b", "b\nB", false},
		{"a\r\nb", "b", "b\nc", false},
		{"aaaa", "aa", "b", true},
		{"x\n\n\ny", "\n\n", "\n", true},
		{"a\r\n\r\nb\r\n", "\n", "\n\n", true},
		{"", "a", "b", true},
		{"\r\n", "\n", "", false},
		{"a\r\r\nb", "b", "c", false},
	}
	for _, s := range seeds {
		f.Add(s.body, s.old, s.repl, s.all)
	}
	f.Fuzz(func(t *testing.T, body, old, repl string, all bool) {
		oldLF, newLF := toLF(old), toLF(repl)
		if oldLF == "" {
			return
		}
		view := newLFView(body)

		// The view is the text with CRLF read as a line break, and every
		// place in it leads back to the same place in the original.
		if view.text != toLF(body) {
			t.Fatalf("view of %q is %q, want %q", body, view.text, toLF(body))
		}
		prev := -1
		for i := 0; i <= len(view.text); i++ {
			raw := view.rawOffset(i)
			if raw <= prev || raw > len(body) {
				t.Fatalf("rawOffset(%d) = %d after %d, in a text of %d bytes", i, raw, prev, len(body))
			}
			prev = raw
			if toLF(body[:raw]) != view.text[:i] {
				t.Fatalf("rawOffset(%d) = %d: the text before it is %q, want %q", i, raw, toLF(body[:raw]), view.text[:i])
			}
		}

		starts := findAll(view.text, oldLF)
		if len(starts) == 0 {
			if strings.Contains(view.text, oldLF) {
				t.Fatalf("findAll missed %q in %q", oldLF, view.text)
			}
			return
		}
		n := -1
		if !all {
			starts, n = starts[:1], 1
		}
		if want := strings.Count(view.text, oldLF); all && len(starts) != want {
			t.Fatalf("findAll found %d places, strings.Count says %d", len(starts), want)
		}
		got := substitute(body, view, starts, oldLF, newLF)

		// Nothing before the first replacement or after the last is
		// touched.
		head := body[:view.rawOffset(starts[0])]
		tail := body[view.rawOffset(starts[len(starts)-1]+len(oldLF)):]
		if !strings.HasPrefix(got, head) || !strings.HasSuffix(got, tail) || len(got) < len(head)+len(tail) {
			t.Fatalf("substitute(%q, %q -> %q) = %q: the text before or after the change was altered", body, oldLF, newLF, got)
		}

		// A carriage return on its own is an ordinary character and makes
		// the plain comparisons below ambiguous; the cases above still
		// hold for it.
		if loneCR(body) || strings.Contains(oldLF, "\r") || strings.Contains(newLF, "\r") {
			return
		}
		wantLF := strings.Replace(view.text, oldLF, newLF, n)
		if toLF(got) != wantLF {
			t.Fatalf("substitute(%q, %q -> %q) reads as %q, want %q", body, oldLF, newLF, toLF(got), wantLF)
		}
		switch {
		case !strings.Contains(body, "\r"):
			// A file with plain line breaks keeps them.
			if got != wantLF {
				t.Fatalf("substitute(%q, %q -> %q) = %q, want %q", body, oldLF, newLF, got, wantLF)
			}
		case strings.Count(body, "\n") == strings.Count(body, "\r\n"):
			// A file with CRLF throughout keeps CRLF throughout.
			if want := strings.ReplaceAll(wantLF, "\n", "\r\n"); got != want {
				t.Fatalf("substitute(%q, %q -> %q) = %q, want %q", body, oldLF, newLF, got, want)
			}
		}
	})
}

// naiveLines splits content into lines the slow and obvious way. It
// returns each line without its line break and whether one ended it.
func naiveLines(content []byte) (lines [][]byte, terminated []bool) {
	if len(content) == 0 {
		return nil, nil
	}
	parts := bytes.Split(content, []byte("\n"))
	for i, p := range parts {
		last := i == len(parts)-1
		if last && len(p) == 0 {
			break
		}
		lines = append(lines, p)
		terminated = append(terminated, !last)
	}
	return lines, terminated
}

func FuzzReadLines(f *testing.F) {
	f.Add([]byte("one\ntwo\nthree\n"), uint16(1), uint16(10), uint8(3))
	f.Add([]byte("one\r\ntwo\r\nthree"), uint16(2), uint16(1), uint8(1))
	f.Add([]byte("\n\n\n"), uint16(0), uint16(2), uint8(2))
	f.Add([]byte("no line break"), uint16(1), uint16(1), uint8(200))
	f.Add([]byte("caf\xe9\n\x00\xff\n"), uint16(1), uint16(5), uint8(1))
	f.Add([]byte(strings.Repeat("日本語", 20)+"\r\n\r"), uint16(1), uint16(5), uint8(4))
	f.Add([]byte(""), uint16(1), uint16(1), uint8(1))
	// Found by fuzzing: a sixth "character" that is a stray byte.
	f.Add([]byte("00000\x80"), uint16(1), uint16(1), uint8(1))
	f.Add([]byte("\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80\x80"), uint16(1), uint16(1), uint8(60))
	f.Fuzz(func(t *testing.T, content []byte, offset, limit uint16, chunk uint8) {
		first := max(int(offset), 1)
		most := max(int(limit), 1)
		size := int(chunk)%61 + 1
		feed := func(r *textRead) {
			for rest := content; len(rest) > 0; {
				n := min(size, len(rest))
				r.feed(rest[:n])
				rest = rest[n:]
			}
			r.end()
		}
		lines, terminated := naiveLines(content)

		// With limits out of the way, the page is exactly the lines asked
		// for, numbered, with a carriage return before a line break left
		// out.
		whole := &textRead{first: first, limit: most, maxBytes: 1 << 30, maxChars: 1 << 20}
		feed(whole)
		if whole.lineNo != len(lines) {
			t.Fatalf("counted %d lines, want %d", whole.lineNo, len(lines))
		}
		var want strings.Builder
		wantShown, wantCRLF, wantLF := 0, 0, 0
		for i := first - 1; i < len(lines) && wantShown < most; i++ {
			line := lines[i]
			if terminated[i] && len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
				wantCRLF++
			} else if terminated[i] {
				wantLF++
			}
			text, _ := showable(line)
			want.WriteString(strconv.Itoa(i + 1))
			want.WriteByte('\t')
			want.WriteString(text)
			want.WriteByte('\n')
			wantShown++
		}
		if got := whole.out.String(); got != want.String() {
			t.Fatalf("offset %d, limit %d, pieces of %d bytes:\ngot  %q\nwant %q", first, most, size, got, want.String())
		}
		if whole.shown != wantShown || whole.crlf != wantCRLF || whole.lf != wantLF {
			t.Fatalf("shown %d, crlf %d, lf %d; want %d, %d, %d", whole.shown, whole.crlf, whole.lf, wantShown, wantCRLF, wantLF)
		}
		if wantShown > 0 && (whole.firstShown != first || whole.lastShown != first+wantShown-1) {
			t.Fatalf("showed lines %d-%d, want %d-%d", whole.firstShown, whole.lastShown, first, first+wantShown-1)
		}
		if !utf8.ValidString(whole.out.String()) {
			t.Fatalf("the page is not valid UTF-8: %q", whole.out.String())
		}

		// With a small limit on line length, the same lines come out, each
		// either whole or cut and marked, and always valid text.
		const maxChars = 5
		cut := &textRead{first: first, limit: most, maxBytes: 1 << 30, maxChars: maxChars}
		feed(cut)
		if cut.shown != wantShown || cut.lineNo != len(lines) {
			t.Fatalf("with lines cut: shown %d of %d lines, want %d of %d", cut.shown, cut.lineNo, wantShown, len(lines))
		}
		gotLines := strings.SplitAfter(cut.out.String(), "\n")
		wantLines := strings.SplitAfter(want.String(), "\n")
		// A line of the file may hold a carriage return or other oddity
		// but never a line break, so both split into the same number.
		if len(gotLines) != len(wantLines) {
			t.Fatalf("with lines cut: %d output lines, want %d", len(gotLines), len(wantLines))
		}
		for i := 0; i < wantShown; i++ {
			if !utf8.ValidString(gotLines[i]) {
				t.Fatalf("a cut line is not valid UTF-8: %q", gotLines[i])
			}
			raw := lines[first-1+i]
			if terminated[first-1+i] && len(raw) > 0 && raw[len(raw)-1] == '\r' {
				raw = raw[:len(raw)-1]
			}
			// A byte that is not valid UTF-8 counts as one character.
			if utf8.RuneCount(raw) <= maxChars {
				if gotLines[i] != wantLines[i] {
					t.Fatalf("a short line was changed: got %q, want %q", gotLines[i], wantLines[i])
				}
			} else if !strings.Contains(gotLines[i], " on this line]") {
				t.Fatalf("a long line was not marked as cut: %q", gotLines[i])
			}
		}

		// With a small limit on bytes, the page is a prefix of the lines
		// and says it stopped early.
		small := &textRead{first: first, limit: most, maxBytes: 16, maxChars: 1 << 20}
		feed(small)
		if !strings.HasPrefix(want.String(), small.out.String()) {
			t.Fatalf("with a byte limit the page is not the start of the full page:\ngot  %q\nfull %q", small.out.String(), want.String())
		}
		if wantShown > 0 && small.shown == 0 {
			t.Fatal("with a byte limit no line was shown at all")
		}
		if small.shown < wantShown && !small.stoppedByBytes {
			t.Fatal("the page was cut short without saying the byte limit did it")
		}
	})
}

func FuzzLineReader(f *testing.F) {
	f.Add([]byte("one\ntwo\nthree\n"), uint16(100), uint8(0))
	f.Add([]byte("a very long line with no break in it at all"), uint16(7), uint8(1))
	f.Add([]byte("\n\nx\n\n"), uint16(1), uint8(2))
	f.Add([]byte("1234567\n12345678\n123456789\n"), uint16(8), uint8(1))
	f.Add([]byte(""), uint16(5), uint8(0))
	f.Fuzz(func(t *testing.T, content []byte, most uint16, how uint8) {
		limit := int(most)%300 + 1
		var r = bytes.NewReader(content)
		lr := &lineReader{max: limit}
		switch how % 3 {
		case 0:
			lr.r = r
		case 1:
			lr.r = iotest.OneByteReader(r)
		case 2:
			lr.r = iotest.DataErrReader(iotest.HalfReader(r))
		}
		// Put the pieces back together: that must give the content back.
		var back bytes.Buffer
		pieces := 0
		for {
			line, terminated, ok := lr.next()
			if !ok {
				break
			}
			pieces++
			if len(line) > limit {
				t.Fatalf("a piece of %d bytes; the most allowed is %d", len(line), limit)
			}
			if bytes.IndexByte(line, '\n') >= 0 {
				t.Fatalf("a line holds a line break: %q", line)
			}
			back.Write(line)
			if terminated {
				back.WriteByte('\n')
			}
			if pieces > len(content)+2 {
				t.Fatal("more pieces than bytes")
			}
		}
		if lr.err != nil {
			t.Fatalf("read error: %v", lr.err)
		}
		if !bytes.Equal(back.Bytes(), content) {
			t.Fatalf("pieces of at most %d bytes give back\n%q\nwant\n%q", limit, back.Bytes(), content)
		}
		// With room for the longest line, the lines are the plain ones.
		longest := 0
		lines, terminated := naiveLines(content)
		for _, l := range lines {
			longest = max(longest, len(l))
		}
		lr = &lineReader{r: bytes.NewReader(content), max: longest + 1}
		for i := range lines {
			line, term, ok := lr.next()
			if !ok || !bytes.Equal(line, lines[i]) || term != terminated[i] {
				t.Fatalf("line %d: got %q (terminated %v, ok %v), want %q (terminated %v)", i+1, line, term, ok, lines[i], terminated[i])
			}
		}
		if _, _, ok := lr.next(); ok {
			t.Fatal("a line after the last")
		}
	})
}

package relay

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"
)

// osc52 is a clipboard write of text, ended by end.
func osc52(sel, text, end string) string {
	return "\x1b]52;" + sel + ";" + base64.StdEncoding.EncodeToString([]byte(text)) + end
}

// scan feeds stream to a new scanner in pieces of size n (all of it at
// once when n is 0) and returns what it found.
func scan(stream string, n int) []string {
	var s scanner
	var got []string
	found := func(text string) { got = append(got, text) }
	if n == 0 {
		s.feed([]byte(stream), found)
		return got
	}
	for len(stream) > 0 {
		k := min(n, len(stream))
		s.feed([]byte(stream[:k]), found)
		stream = stream[k:]
	}
	return got
}

// The scanner finds each clipboard write, however the stream is cut up,
// and nothing else.
func TestScanner(t *testing.T) {
	long := strings.Repeat("x", maxCopy)
	for _, c := range []struct {
		name, stream string
		want         []string
	}{
		{"BEL", "before" + osc52("c", "hello", "\x07") + "after", []string{"hello"}},
		{"ST", osc52("", "two\nlines", "\x1b\\"), []string{"two\nlines"}},
		{"selections", osc52("pc", "both", "\x07"), []string{"both"}},
		{"one after another", osc52("c", "one", "\x07") + "\x1b[1mbold\x1b[0m" + osc52("c", "two", "\x1b\\"), []string{"one", "two"}},
		{"unpadded", "\x1b]52;c;aGk\x07", []string{"hi"}},
		{"query", "\x1b]52;c;?\x07", nil},
		{"clear", "\x1b]52;c;\x07", nil},
		{"no selections field", "\x1b]52;aGk=\x07", nil},
		{"not base64", "\x1b]52;c;!!!\x07", nil},
		{"a title", "\x1b]0;52;c;aGk=\x07\x1b]2;title\x07", nil},
		{"a hyperlink", "\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\", nil},
		{"cancelled", "\x1b]52;c;aGk=\x18" + osc52("c", "after", "\x07"), []string{"after"}},
		{"ESC that is not ST", "\x1b]52;c;aGk=\x1b[31m\x07" + osc52("c", "after", "\x07"), []string{"after"}},
		{"ESC ESC ]", "\x1b\x1b]52;c;aGk=\x07", []string{"hi"}},
		{"the 8-bit form", "\x9d52;c;aGk=\x9c", nil},
		{"too long", osc52("c", long, "\x07") + osc52("c", "short", "\x07"), []string{"short"}},
		{"UTF-8", osc52("c", "✦ gnosh — ok", "\x07"), []string{"✦ gnosh — ok"}},
	} {
		for _, n := range []int{0, 1, 2, 7, 4096} {
			if n > 0 && n < 7 && len(c.stream) > 100000 {
				continue
			}
			if got := scan(c.stream, n); !slices.Equal(got, c.want) {
				short := func(s []string) []string {
					out := []string{}
					for _, x := range s {
						if len(x) > 40 {
							x = x[:40] + "..."
						}
						out = append(out, x)
					}
					return out
				}
				t.Errorf("%s, in pieces of %d: found %q, want %q", c.name, n, short(got), short(c.want))
			}
		}
	}
}

package hooks

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestClip(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Errorf("clip of a short text = %q", got)
	}
	if got := clip("exactly 10", 10); got != "exactly 10" {
		t.Errorf("clip of a text at the limit = %q", got)
	}
	// Characters are counted, not bytes: ten of these are thirty bytes.
	accents := strings.Repeat("\xe2\x82\xac", 10)
	if got := clip(accents, 10); got != accents {
		t.Errorf("clip counted bytes: %q", got)
	}
	long := "START" + strings.Repeat("-", 1000) + "END"
	got := clip(long, 100)
	if !strings.HasPrefix(got, "START") || !strings.HasSuffix(got, "END") {
		t.Errorf("clip lost an end: %q", got)
	}
	contains(t, "the clipped text", got, "\n[... 908 characters left out by gnosh ...]\n")
	if n := utf8.RuneCountInString(got) - len("\n[... 908 characters left out by gnosh ...]\n"); n != 100 {
		t.Errorf("clip kept %d characters, want 100", n)
	}
	// A cut never splits a character.
	wide := strings.Repeat("\xe2\x82\xac", 500)
	if got := clip(wide, 99); !utf8.ValidString(got) {
		t.Error("clip split a character")
	}
}

func TestStartAndShorten(t *testing.T) {
	if got := start("abcdef", 3); got != "abc\n[...]" {
		t.Errorf("start = %q", got)
	}
	if got := start("abc", 3); got != "abc" {
		t.Errorf("start = %q", got)
	}
	// Characters are counted, not bytes.
	if euros := strings.Repeat("\xe2\x82\xac", 3); start(euros, 3) != euros || start(euros+"!", 3) != euros+"\n[...]" {
		t.Errorf("start counted bytes: %q, %q", start(euros, 3), start(euros+"!", 3))
	}
	if got := shorten("  ./check.sh \n  --all\t--strict  ", 40); got != "./check.sh --all --strict" {
		t.Errorf("shorten = %q", got)
	}
	if got := shorten(strings.Repeat("x", 50), 10); got != "xxxxxxx..." {
		t.Errorf("shorten = %q", got)
	}
	// A name from a settings file cannot carry a control sequence into a
	// message.
	if got := quote("evil\x1b[2Jname"); strings.ContainsRune(got, 0x1b) {
		t.Errorf("quote let an escape character through: %q", got)
	}
}

func TestSpellAndSize(t *testing.T) {
	for d, want := range map[time.Duration]string{
		150 * time.Millisecond:  "150ms",
		time.Second:             "1 second",
		1500 * time.Millisecond: "1.5 seconds",
		30 * time.Second:        "30 seconds",
		600 * time.Second:       "600 seconds",
		1800 * time.Second:      "1800 seconds",
	} {
		if got := spell(d); got != want {
			t.Errorf("spell(%v) = %q, want %q", d, got, want)
		}
	}
	for n, want := range map[int]string{4 << 20: "4 MiB", 64 << 10: "64 KiB", 1000: "1000 bytes", 1536: "1536 bytes"} {
		if got := size(n); got != want {
			t.Errorf("size(%d) = %q, want %q", n, got, want)
		}
	}
}

// head keeps the start of a stream and counts the rest, in whatever
// pieces the stream arrives.
func TestHead(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 100)
	for _, piece := range []int{1, 7, 64, 1000, 5000} {
		h := head{max: 250}
		for rest := data; len(rest) > 0; {
			n := min(piece, len(rest))
			if got, err := h.Write(rest[:n]); got != n || err != nil {
				t.Fatalf("Write = %d, %v", got, err)
			}
			rest = rest[n:]
		}
		if !bytes.Equal(h.buf, data[:250]) || h.lost != 750 {
			t.Errorf("in pieces of %d: kept %d bytes, lost %d", piece, len(h.buf), h.lost)
		}
	}
	whole := head{max: 2000}
	whole.Write(data)
	if !bytes.Equal(whole.buf, data) || whole.lost != 0 {
		t.Errorf("a stream under the limit: kept %d, lost %d", len(whole.buf), whole.lost)
	}
}

// ends keeps both ends of a stream, in whatever pieces it arrives, and
// never holds more than a few times what it keeps.
func TestEnds(t *testing.T) {
	var data []byte
	for i := 0; len(data) < 10000; i++ {
		data = append(data, []byte("line "+strings.Repeat("x", i%7)+"\n")...)
	}
	for _, total := range []int{0, 1, 99, 100, 101, 199, 200, 201, 350, 1000, len(data)} {
		for _, piece := range []int{1, 3, 50, 99, 100, 101, 250, 20000} {
			e := ends{keep: 100}
			for rest := data[:total]; len(rest) > 0; {
				n := min(piece, len(rest))
				e.Write(rest[:n])
				rest = rest[n:]
				if len(e.first) > 100 || len(e.last) > 200+piece {
					t.Fatalf("total %d, pieces of %d: holding %d and %d bytes", total, piece, len(e.first), len(e.last))
				}
			}
			got := e.text()
			if total <= 200 {
				if got != string(data[:total]) {
					t.Errorf("total %d, pieces of %d: a stream that fits was changed:\n%q", total, piece, got)
				}
				continue
			}
			if !strings.HasPrefix(got, string(data[:100])) || !strings.HasSuffix(got, string(data[total-100:total])) {
				t.Errorf("total %d, pieces of %d: an end is missing:\n%q", total, piece, got)
			}
			// What is still held may be more than the two ends; whatever
			// is missing is counted exactly.
			middle := got[100 : len(got)-100]
			if i := strings.Index(middle, "\n[... "); i >= 0 {
				kept := i + len(middle) - strings.Index(middle, " ...]\n") - len(" ...]\n")
				var lost int
				for _, c := range middle[i+len("\n[... "):] {
					if c < '0' || c > '9' {
						break
					}
					lost = lost*10 + int(c-'0')
				}
				if 200+kept+lost != total {
					t.Errorf("total %d, pieces of %d: 200 at the ends + %d more + %d said to be left out", total, piece, kept, lost)
				}
			} else if len(got) != total {
				t.Errorf("total %d, pieces of %d: nothing is said to be left out, yet %d bytes are there", total, piece, len(got))
			}
		}
	}
	// Bytes that are not text do not make invalid text.
	e := ends{keep: 4}
	e.Write([]byte{'o', 'k', 0xff, 0xfe, 0xe2, 0x82})
	if got := e.text(); !utf8.ValidString(got) || !strings.HasPrefix(got, "ok") {
		t.Errorf("text() = %q", got)
	}
}

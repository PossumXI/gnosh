package hooks

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// quote puts s in double quotes for a message, with anything unprintable
// escaped, so that a name from a settings file is never read as part of
// the sentence around it or as a terminal control sequence.
func quote(s string) string {
	return strconv.Quote(s)
}

// shorten returns s on one line and at most max characters long, ending in
// "..." when it was cut.
func shorten(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max-3]) + "..."
}

// clip returns s if it has at most max characters. Otherwise it returns
// the start and the end of s, max characters together, around a line that
// says how much was left out. A hook's words go to a model with a window
// to fill and to a person with a screen to read, so they are bounded; the
// end is kept because that is where a failing check usually says what
// failed.
func clip(s string, max int) string {
	if len(s) <= max {
		return s // bytes are at least as many as characters
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	head := max / 2
	tail := max - head
	return strings.TrimRight(string(r[:head]), "\n") +
		fmt.Sprintf("\n[... %d characters left out by gnosh ...]\n", len(r)-max) +
		strings.TrimLeft(string(r[len(r)-tail:]), "\n")
}

// start returns the first max characters of s, with a mark when there was
// more.
func start(s string, max int) string {
	if len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n[...]"
}

// spell writes a length of time the way a person would say it.
func spell(d time.Duration) string {
	if d < time.Second {
		return d.String()
	}
	secs := strconv.FormatFloat(d.Seconds(), 'f', -1, 64)
	if secs == "1" {
		return "1 second"
	}
	return secs + " seconds"
}

// size writes a number of bytes the way a person would say it.
func size(n int) string {
	switch {
	case n >= 1<<20 && n%(1<<20) == 0:
		return strconv.Itoa(n>>20) + " MiB"
	case n >= 1<<10 && n%(1<<10) == 0:
		return strconv.Itoa(n>>10) + " KiB"
	}
	return strconv.Itoa(n) + " bytes"
}

// join puts the texts that are not empty together, a blank line between
// them.
func join(texts []string) string {
	var kept []string
	for _, t := range texts {
		if t != "" {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, "\n\n")
}

// head keeps the first max bytes written to it and counts the rest. It is
// what bounds the memory a hook's stdout can take.
type head struct {
	max  int
	buf  []byte
	lost int64 // bytes that did not fit
}

// Write takes the next piece. It never fails, so the writer on the other
// end is never held up by a full pipe.
func (h *head) Write(p []byte) (int, error) {
	n := len(p)
	if room := h.max - len(h.buf); room > 0 {
		k := min(room, len(p))
		h.buf = append(h.buf, p[:k]...)
		p = p[k:]
	}
	h.lost += int64(len(p))
	return n, nil
}

// ends keeps the first and the last keep bytes written to it and counts
// what went by in between. It is what bounds the memory a hook's stderr
// can take, while keeping both the first thing it said and the last.
type ends struct {
	keep  int
	first []byte
	last  []byte // at most twice keep; its final keep bytes are the ones that count
	total int64
}

// Write takes the next piece. It never fails.
func (e *ends) Write(p []byte) (int, error) {
	n := len(p)
	e.total += int64(n)
	if room := e.keep - len(e.first); room > 0 {
		k := min(room, len(p))
		e.first = append(e.first, p[:k]...)
		p = p[k:]
	}
	switch {
	case len(p) >= e.keep:
		e.last = append(e.last[:0], p[len(p)-e.keep:]...)
	case len(p) > 0:
		e.last = append(e.last, p...)
		if len(e.last) > 2*e.keep {
			e.last = append(e.last[:0], e.last[len(e.last)-e.keep:]...)
		}
	}
	return n, nil
}

// text returns what was kept, with a line where bytes are missing. Bytes
// that are not UTF-8, such as a character cut in half, become the
// replacement character.
func (e *ends) text() string {
	if e.total == int64(len(e.first)+len(e.last)) {
		return valid(string(e.first) + string(e.last))
	}
	last := e.last
	if len(last) > e.keep {
		last = last[len(last)-e.keep:]
	}
	lost := e.total - int64(len(e.first)) - int64(len(last))
	return valid(string(e.first)) +
		fmt.Sprintf("\n[... %d bytes left out by gnosh ...]\n", lost) +
		valid(string(last))
}

// valid returns s with every byte that is not UTF-8 replaced by the
// replacement character.
func valid(s string) string {
	return strings.ToValidUTF8(s, string(utf8.RuneError))
}

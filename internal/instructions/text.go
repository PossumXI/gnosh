package instructions

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// cleanText turns a file's bytes into the text the rest of the package works
// on: UTF-8 with "\n" line ends and no byte-order mark. binary reports a file
// that holds a zero byte and so is not text. repaired reports that bytes that
// are not UTF-8 were replaced with U+FFFD.
func cleanText(data []byte) (text string, binary, repaired bool) {
	if bytes.IndexByte(data, 0) >= 0 {
		return "", true, false
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	text = string(data)
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "�")
		repaired = true
	}
	if strings.IndexByte(text, '\r') >= 0 {
		text = strings.ReplaceAll(text, "\r\n", "\n")
		text = strings.ReplaceAll(text, "\r", "\n")
	}
	return text, false, repaired
}

// lineEnd returns the index just past the line of s that starts at i: past
// its newline, or the end of s.
func lineEnd(s string, i int) int {
	if n := strings.IndexByte(s[i:], '\n'); n >= 0 {
		return i + n + 1
	}
	return len(s)
}

// stripComments removes the HTML comments that stand as blocks of their own
// in Markdown text: a comment that opens at the start of a line (after at
// most three spaces) and runs to its "-->", on that line or a later one.
// Such comments are notes for the people who keep the file, not for the
// model. What follows the comment on its last line is kept; a line left
// empty is dropped.
//
// Left alone: a comment in the middle of a line of prose, a comment inside a
// fenced code block, and a comment that never closes (dropping the rest of
// the file for a missing "-->" would lose instructions in silence).
func stripComments(s string) string {
	if !strings.Contains(s, "<!--") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	var open fence
	inFence := false
	unclosed := false // no "-->" lies ahead, so no later comment can end
	for i := 0; i < len(s); {
		end := lineEnd(s, i)
		line := s[i:end]
		if inFence {
			b.WriteString(line)
			if open.closedBy(line) {
				inFence = false
			}
			i = end
			continue
		}
		if f, ok := openFence(line); ok {
			open, inFence = f, true
			b.WriteString(line)
			i = end
			continue
		}
		indent := commentIndent(line)
		after, ok := 0, false
		if indent >= 0 && !unclosed {
			if after, ok = commentEnd(s, i+indent); !ok {
				unclosed = true
			}
		}
		if !ok {
			b.WriteString(line)
			i = end
			continue
		}
		// More comments may follow on the line where this one ends.
		for !unclosed {
			next := after
			for next < len(s) && (s[next] == ' ' || s[next] == '\t') {
				next++
			}
			if !strings.HasPrefix(s[next:], "<!--") {
				break
			}
			if e, ok := commentEnd(s, next); ok {
				after = e
			} else {
				unclosed = true
			}
		}
		end = lineEnd(s, after)
		if rest := strings.TrimLeft(s[after:end], " \t"); strings.TrimSpace(rest) != "" {
			kept := line[:indent] + rest
			b.WriteString(kept)
			// The text that is left may itself open a code block, and
			// whoever reads the result will see it that way.
			if f, ok := openFence(kept); ok {
				open, inFence = f, true
			}
		}
		i = end
	}
	return b.String()
}

// commentIndent returns how many spaces come before the "<!--" that opens
// line, or -1 if line does not open with a comment.
func commentIndent(line string) int {
	n := 0
	for n < len(line) && line[n] == ' ' {
		n++
	}
	if n > 3 || !strings.HasPrefix(line[n:], "<!--") {
		return -1
	}
	return n
}

// commentEnd returns the index just past the "-->" that closes the comment
// opening at s[at:]. As in HTML, "<!-->" and "<!--->" are whole comments.
func commentEnd(s string, at int) (int, bool) {
	k := strings.Index(s[at+2:], "-->")
	if k < 0 {
		return 0, false
	}
	return at + 2 + k + 3, true
}

// cutText returns the start of s, at most max characters long, and whether
// anything was cut. The cut falls at the end of a line when one ends in the
// last quarter of the allowance, so that a line is not left half-said. A max
// of zero or less means no cut.
func cutText(s string, max int) (string, bool) {
	if max <= 0 || len(s) <= max {
		return s, false
	}
	seen, end := 0, len(s)
	for i := range s {
		if seen == max {
			end = i
			break
		}
		seen++
	}
	if end == len(s) {
		return s, false
	}
	cut := s[:end]
	if nl := strings.LastIndexByte(cut, '\n'); nl >= 0 && nl+1 >= end-end/4 {
		cut = cut[:nl+1]
	}
	return cut, true
}

// plain makes a path or pattern safe to put in a line of text: characters
// that do not print (control characters, direction overrides, line
// separators) are written as escapes, so that a file's name cannot break a
// line or drive a terminal.
func plain(s string) string {
	clean := true
	for _, r := range s {
		if !unicode.IsPrint(r) || r == utf8.RuneError {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, "\\x%02x", s[i])
		case !unicode.IsPrint(r):
			fmt.Fprintf(&b, "\\u{%x}", r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

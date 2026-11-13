package fstools

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// sniffLen is how much of the start of a file is looked at to tell what
// kind of file it is.
const sniffLen = 8192

// utf8BOM is the byte order mark some programs put at the start of a
// UTF-8 file.
const utf8BOM = "\xef\xbb\xbf"

// replacement is the character shown in place of bytes that are not
// valid UTF-8.
const replacement = "\uFFFD"

// looksBinary reports whether the start of a file marks it as binary: it
// holds a NUL byte, which text does not. That is the test git uses.
func looksBinary(head []byte) bool {
	if len(head) > sniffLen {
		head = head[:sniffLen]
	}
	return bytes.IndexByte(head, 0) >= 0
}

// utf16Mark reports whether the file starts with a UTF-16 byte order mark.
// Such a file is text, but every other byte of it is NUL.
func utf16Mark(head []byte) bool {
	return bytes.HasPrefix(head, []byte{0xFF, 0xFE}) || bytes.HasPrefix(head, []byte{0xFE, 0xFF})
}

// imageType returns the media type of the image formats Read hands to a
// model, told from the first bytes of a file, or "" for anything else.
func imageType(head []byte) string {
	switch {
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(head, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(head, []byte("GIF87a")), bytes.HasPrefix(head, []byte("GIF89a")):
		return "image/gif"
	case len(head) >= 12 && bytes.Equal(head[:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}

// isPDF reports whether a file starts the way a PDF does.
func isPDF(head []byte) bool {
	return bytes.HasPrefix(head, []byte("%PDF-"))
}

// byteSize writes a number of bytes for a person: "512 bytes", "1.5 KiB".
func byteSize(n int64) string {
	const unit = 1024
	switch {
	case n == 1:
		return "1 byte"
	case n < unit:
		return fmt.Sprintf("%d bytes", n)
	}
	value := float64(n)
	for _, name := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit || name == "TiB" {
			if value >= 100 || value == float64(int64(value)) {
				return fmt.Sprintf("%.0f %s", value, name)
			}
			return fmt.Sprintf("%.1f %s", value, name)
		}
	}
	return fmt.Sprintf("%d bytes", n)
}

// countOf writes a count with its noun: "1 line", "3 lines".
func countOf(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// lineCount returns how many lines a text has. A last line with no line
// break after it counts; an empty text has none.
func lineCount(s string) int {
	n := strings.Count(s, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// runeStarts counts the bytes of b that begin a character. For valid
// UTF-8 that is the number of characters, and it adds up correctly over
// pieces cut at any byte.
func runeStarts(b []byte) int {
	n := 0
	for _, c := range b {
		if isRuneStart(c) {
			n++
		}
	}
	return n
}

// cutChars cuts b to at most max characters. It returns what is kept and
// how many characters were dropped. A byte that is not valid UTF-8 counts
// as one character, as it does everywhere a character is counted here.
func cutChars(b []byte, max int) (kept []byte, dropped int) {
	i, n := 0, 0
	for i < len(b) && n < max {
		_, size := utf8.DecodeRune(b[i:])
		i += size
		n++
	}
	if i >= len(b) {
		return b, 0
	}
	return b[:i], utf8.RuneCount(b[i:])
}

// cutMark is what is put at the end of a line that was cut, to say how
// much of it is missing.
func cutMark(dropped int) string {
	if dropped == 1 {
		return " [... 1 more character on this line]"
	}
	return fmt.Sprintf(" [... %d more characters on this line]", dropped)
}

// showable makes a line of a file safe to show: bytes that are not valid
// UTF-8, and NUL bytes, become the replacement character. It returns the
// text and how many such stretches it replaced.
func showable(b []byte) (string, int) {
	if utf8.Valid(b) && bytes.IndexByte(b, 0) < 0 {
		return string(b), 0
	}
	var out strings.Builder
	out.Grow(len(b) + 8)
	bad := 0
	inBad := false
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if (r == utf8.RuneError && size <= 1) || r == 0 {
			if !inBad {
				out.WriteString(replacement)
				bad++
				inBad = true
			}
			i += max(size, 1)
			continue
		}
		inBad = false
		out.Write(b[i : i+size])
		i += size
	}
	return out.String(), bad
}

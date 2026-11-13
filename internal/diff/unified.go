package diff

import (
	"strconv"
	"strings"
)

// UnifiedOptions are the choices Unified leaves at their defaults.
type UnifiedOptions struct {
	// OldName and NewName are the file names written after "---" and
	// "+++". Left empty they are "a" and "b". Use "/dev/null" for the side
	// of a file that is created or deleted, as patch expects.
	OldName, NewName string
	// Context is the number of unchanged lines around each change. Zero
	// means DefaultContext, a negative number means none.
	Context int
	// Budget bounds the comparison, as Options.Budget does.
	Budget int
}

// noNewline is the line the unified format puts after a line that ends its
// text without a newline. patch looks only at the backslash; the words are
// the ones GNU diff writes.
const noNewline = "\\ No newline at end of file\n"

// Unified compares two texts and returns the difference in the unified
// format, as POSIX specifies it for "diff -u" and as GNU diff writes it:
//
//	--- a
//	+++ b
//	@@ -12,7 +12,8 @@
//	 an unchanged line
//	-a line of the text before
//	+a line of the text after
//
// Each hunk is headed by its place in both texts: the number of its first
// line and how many lines it covers. The count is left out when it is 1, and
// a hunk that covers no lines of a text names the line it comes after. Hunks
// whose context would touch or overlap are one hunk. A line that ends its
// text without a newline is followed by "\ No newline at end of file". Two
// texts that are the same give the empty string.
//
// The result holds the lines of both texts as they are, carriage returns and
// all, so applying it to before with patch gives exactly after. A name with a
// character that would break the header (a control character, a quote or a
// backslash) is written as a quoted C string, which GNU diff and patch use
// for such names, and a name with a space is followed by a tab, which is how
// patch finds where such a name ends.
//
// The header has no time stamps: this package does not read the clock, and
// patch does not need them.
func Unified(before, after string, opts UnifiedOptions) string {
	c := compare(before, after, limit(opts.Budget))
	hunks := group(c.runs, contextOf(opts.Context))
	if len(hunks) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(before)/4 + len(after)/4 + 256)
	b.WriteString("--- ")
	b.WriteString(headerName(opts.OldName, "a"))
	b.WriteString("\n+++ ")
	b.WriteString(headerName(opts.NewName, "b"))
	b.WriteByte('\n')
	for _, h := range hunks {
		b.WriteString("@@ -")
		writeRange(&b, h.a0, h.a1)
		b.WriteString(" +")
		writeRange(&b, h.b0, h.b1)
		b.WriteString(" @@\n")
		for _, r := range h.runs {
			switch r.op {
			case Keep:
				writeLines(&b, ' ', c.old[r.a0:r.a1])
			case Delete:
				writeLines(&b, '-', c.old[r.a0:r.a1])
			case Insert:
				writeLines(&b, '+', c.new[r.b0:r.b1])
			}
		}
	}
	return b.String()
}

// writeRange writes the place of a hunk in one text, which covers lines
// from up to to counted from zero: "start,count", only "start" when the
// count is one, and for an empty range the number of the line before it.
func writeRange(b *strings.Builder, from, to int) {
	switch n := to - from; n {
	case 0:
		b.WriteString(strconv.Itoa(from))
		b.WriteString(",0")
	case 1:
		b.WriteString(strconv.Itoa(from + 1))
	default:
		b.WriteString(strconv.Itoa(from + 1))
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(n))
	}
}

// writeLines writes lines with sign before each, and the "no newline" line
// after one that is not ended.
func writeLines(b *strings.Builder, sign byte, lines []string) {
	for _, line := range lines {
		b.WriteByte(sign)
		b.WriteString(line)
		if !ended(line) {
			b.WriteByte('\n')
			b.WriteString(noNewline)
		}
	}
}

// headerName returns a file name as it is written in the header, or fallback
// for an empty name.
func headerName(name, fallback string) string {
	if name == "" {
		return fallback
	}
	quote, space := false, false
	for i := 0; i < len(name); i++ {
		switch c := name[i]; {
		case c < ' ' || c == 0x7f || c == '"' || c == '\\':
			quote = true
		case c == ' ':
			space = true
		}
	}
	switch {
	case quote:
		var b strings.Builder
		b.WriteByte('"')
		for i := 0; i < len(name); i++ {
			switch c := name[i]; {
			case c == '"' || c == '\\':
				b.WriteByte('\\')
				b.WriteByte(c)
			case c == '\n':
				b.WriteString(`\n`)
			case c == '\t':
				b.WriteString(`\t`)
			case c == '\r':
				b.WriteString(`\r`)
			case c < ' ' || c == 0x7f:
				b.WriteByte('\\')
				b.WriteByte('0' + c>>6)
				b.WriteByte('0' + c>>3&7)
				b.WriteByte('0' + c&7)
			default:
				b.WriteByte(c)
			}
		}
		b.WriteByte('"')
		return b.String()
	case space:
		return name + "\t"
	}
	return name
}

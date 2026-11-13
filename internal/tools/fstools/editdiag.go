package fstools

import (
	"fmt"
	"strings"
	"unicode"
)

// maxDiagnoseBytes is the largest file in which Edit looks for the text
// closest to an old_string it could not find. Past it the error is the
// plain one.
const maxDiagnoseBytes = 8 << 20

// missingText explains why old was not found in text, and where the
// nearest thing to it is. Both have plain line breaks; hadCRLF says the
// file itself uses CRLF. Nothing here changes what Edit accepts: it only
// helps the model send the right text next time.
func missingText(o *Options, loc *location, text, old string, hadCRLF bool) *failure {
	head := fmt.Sprintf("old_string was not found in %s, so nothing was changed.", loc.abs)
	plain := failf("%s Read the file again with %s and copy the text exactly as it is there.", head, o.ReadName)
	if len(text) > maxDiagnoseBytes || len(old) > maxDiagnoseBytes {
		return plain
	}
	fileLines := strings.Split(text, "\n")
	oldLines := strings.Split(old, "\n")
	show := func(first, count int) string {
		return "\n" + numberedLines(fileLines, first, count, o.ReadMaxLineChars)
	}

	// The line numbers Read shows, copied along with the text.
	if stripped, ok := stripLineNumbers(oldLines); ok {
		if strings.Contains(text, strings.Join(stripped, "\n")) {
			return failf("%s old_string has the line numbers that %s puts in front of each line (a number and a tab). They are not in the file: send the text without them.", head, o.ReadName)
		}
		oldLines = stripped
		old = strings.Join(stripped, "\n")
		head += fmt.Sprintf(" (old_string also has the line numbers %s shows in front of each line; they are not part of the file.)", o.ReadName)
	}

	// A line break after the file's last line, which has none.
	if strings.HasSuffix(old, "\n") && !strings.HasSuffix(text, "\n") && strings.HasSuffix(text, strings.TrimSuffix(old, "\n")) && len(old) > 1 {
		return failf("%s old_string ends with a line break, but the text it matches is the very end of the file, and the file has no line break there. Leave the last line break out of old_string (and add one in new_string only if you mean the file to gain it).", head)
	}

	// Line breaks or tabs written out as backslash sequences.
	if strings.Contains(old, `\n`) || strings.Contains(old, `\t`) {
		unescaped := strings.NewReplacer(`\n`, "\n", `\t`, "\t").Replace(old)
		if unescaped != old && strings.Contains(text, unescaped) {
			return failf("%s old_string holds the two characters backslash and n (or backslash and t) where the file has a real line break (or tab). Send the line breaks and tabs themselves.", head)
		}
	}

	// The same text but for some kind of small difference, most telling
	// kind first.
	kinds := []struct {
		norm func(string) string
		what string
	}{
		{trimLineEnd, "only spaces or tabs at the ends of lines differ"},
		{trimLineStart, "only the indentation differs"},
		{squeezeSpace, "only the spacing differs"},
		{plainPunctuation, "the file has typographic characters (curly quotes, long dashes or non-breaking spaces) where old_string has plain ones, or the other way round"},
		{strings.ToLower, "only the letter case differs"},
		{func(s string) string { return strings.ToLower(squeezeSpace(plainPunctuation(s))) }, "spacing, letter case or typographic characters differ"},
	}
	for _, k := range kinds {
		if at := looseFind(fileLines, oldLines, k.norm); at > 0 {
			count := len(oldLines)
			if oldLines[count-1] == "" && count > 1 {
				count--
			}
			return failf("%s The closest text is at %s, where %s. The file has:%s\nCopy the text exactly as it is in the file.",
				head, lineRange(at, count), k.what, show(at, count))
		}
	}

	// The first line is there, and a later line is not what follows it.
	if at, diff := firstLineThenDiffers(fileLines, oldLines); at > 0 {
		theirs := "the file ends before it"
		// What follows the file's last line break is not a line.
		if k := at + diff - 1; k < len(fileLines) && !(k == len(fileLines)-1 && fileLines[k] == "") {
			theirs = fmt.Sprintf("the file has %q", short(fileLines[k], 200))
		}
		return failf("%s The first line of old_string is at line %d of the file, but line %d of old_string does not match what follows there: old_string has %q, and %s. The file has:%s",
			head, at, diff+1, short(oldLines[diff], 200), theirs, show(at, min(len(oldLines), diff+3)))
	}
	if hadCRLF && strings.Contains(old, "\r") {
		return failf("%s old_string holds a carriage return that is not part of a line break. %s matches the file's CRLF line breaks to a plain line break in old_string, so send plain ones.", head, o.EditName)
	}
	return plain
}

// lineRange writes "line 12" or "lines 12-14".
func lineRange(first, count int) string {
	if count <= 1 {
		return fmt.Sprintf("line %d", first)
	}
	return fmt.Sprintf("lines %d-%d", first, first+count-1)
}

// numberedLines shows up to twelve of the given lines of a file the way
// Read numbers them.
func numberedLines(lines []string, first, count, maxChars int) string {
	const most = 12
	var out strings.Builder
	for i := 0; i < count && i < most; i++ {
		n := first + i
		if n < 1 || n > len(lines) {
			break
		}
		kept, dropped := cutChars([]byte(lines[n-1]), maxChars)
		text, _ := showable(kept)
		if dropped > 0 {
			text += cutMark(dropped)
		}
		fmt.Fprintf(&out, "%d\t%s\n", n, text)
	}
	if count > most {
		fmt.Fprintf(&out, "[... and %d more lines.]\n", count-most)
	}
	return out.String()
}

// stripLineNumbers takes the "12<tab>" that Read puts in front of lines
// off every line. It does so only when every line has one and the numbers
// count up by one from line to line, as Read's do: a file of tab-separated
// numbers must not be mistaken for a numbered listing.
func stripLineNumbers(lines []string) ([]string, bool) {
	out := make([]string, len(lines))
	want := -1
	for i, line := range lines {
		if line == "" && i == len(lines)-1 && i > 0 {
			// What follows the last line break is not a line.
			continue
		}
		rest := strings.TrimLeft(line, " ")
		digits := 0
		n := 0
		for digits < len(rest) && digits < 9 && rest[digits] >= '0' && rest[digits] <= '9' {
			n = n*10 + int(rest[digits]-'0')
			digits++
		}
		if digits == 0 || digits >= len(rest) || rest[digits] != '\t' {
			return nil, false
		}
		if want >= 0 && n != want {
			return nil, false
		}
		want = n + 1
		out[i] = rest[digits+1:]
	}
	return out, want >= 0
}

// looseFind looks for the lines of old in the file's lines, comparing
// them after norm has been applied to both. old may start and end partway
// through a line, so its first line need only end a line of the file, its
// last line need only begin one, and a one-line old need only be inside
// one. It returns the number of the file line where the match starts, or
// zero.
func looseFind(file, old []string, norm func(string) string) int {
	want := make([]string, len(old))
	for i, line := range old {
		want[i] = norm(line)
	}
	if len(want) == 1 {
		if want[0] == "" {
			return 0
		}
		for i, line := range file {
			if strings.Contains(norm(line), want[0]) {
				return i + 1
			}
		}
		return 0
	}
	// With nothing but blank lines to go on there is nothing to find.
	blank := true
	for _, w := range want {
		if w != "" {
			blank = false
			break
		}
	}
	if blank {
		return 0
	}
	last := len(want) - 1
	for i := 0; i+last < len(file); i++ {
		if !strings.HasSuffix(norm(file[i]), want[0]) {
			continue
		}
		ok := true
		for j := 1; j < last && ok; j++ {
			ok = norm(file[i+j]) == want[j]
		}
		if ok && strings.HasPrefix(norm(file[i+last]), want[last]) {
			return i + 1
		}
	}
	return 0
}

// firstLineThenDiffers looks for the place where old's first line is a
// whole line of the file (but for spaces around it) and reports the line
// number there and the index of the first line of old that does not match
// what follows. It gives an answer only when the first line is found in
// exactly one place, since anything else would be a guess.
func firstLineThenDiffers(file, old []string) (at, diff int) {
	if len(old) < 2 {
		return 0, 0
	}
	first := strings.TrimSpace(old[0])
	if first == "" {
		return 0, 0
	}
	found := 0
	for i, line := range file {
		if strings.TrimSpace(line) == first {
			if found != 0 {
				return 0, 0
			}
			found = i + 1
		}
	}
	if found == 0 {
		return 0, 0
	}
	for j := 1; j < len(old); j++ {
		k := found - 1 + j
		if j == len(old)-1 && old[j] == "" {
			break
		}
		if k >= len(file) || strings.TrimSpace(file[k]) != strings.TrimSpace(old[j]) {
			return found, j
		}
	}
	return 0, 0
}

func trimLineEnd(s string) string   { return strings.TrimRight(s, " \t\r") }
func trimLineStart(s string) string { return strings.TrimLeft(s, " \t") }

// squeezeSpace drops the space at both ends of a line and makes every run
// of space inside it one space.
func squeezeSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// plainPunctuation replaces the typographic characters a word processor
// or a model puts in for plain ones: curly quotes, long dashes, the
// ellipsis, and spaces that are not the ordinary space.
func plainPunctuation(s string) string {
	plainASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			plainASCII = false
			break
		}
	}
	if plainASCII {
		return s
	}
	var out strings.Builder
	for _, r := range s {
		switch r {
		case '\u2018', '\u2019', '\u201A', '\u201B', '\u2032':
			out.WriteByte('\'')
		case '\u201C', '\u201D', '\u201E', '\u201F', '\u2033':
			out.WriteByte('"')
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2212':
			out.WriteByte('-')
		case '\u2026':
			out.WriteString("...")
		default:
			if (r != ' ' && r != '\t' && unicode.IsSpace(r)) || r == '\u00A0' || r == '\u202F' || r == '\u2007' {
				out.WriteByte(' ')
			} else {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

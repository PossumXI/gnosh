package shellquote

import (
	"regexp"
	"strings"
)

// heredocAt finds a here-document's start at the beginning of a piece of a
// line: << or <<-, and the word that ends it, quoted or not.
var heredocAt = regexp.MustCompile(`^<<(-?)\s*(?:'([^']*)'|"([^"]*)"|\\?([A-Za-z0-9_]+))`)

// CommandStarts says, for each line of a command, whether the shell starts
// reading a command there: not the text or the end of a here-document, not
// a line that the one before goes on into (a backslash at its end), and
// not a line inside quotes opened on a line before. Quotes are followed
// the way the shell reads them, so that a << inside them opens nothing.
func CommandStarts(command string) []bool {
	lines := strings.Split(command, "\n")
	out := make([]bool, len(lines))
	var ends []string // the words that end the open here-documents, in order
	var dash []bool   // each may stand indented by tabs (<<-)
	var quote byte    // the quote open at the end of the line before, or 0
	goesOn := false   // the line before ends with a backslash
	arith := 0        // the (( )) open, inside which << is a shift
	for i, line := range lines {
		if len(ends) > 0 {
			l := line
			if dash[0] {
				l = strings.TrimLeft(l, "\t")
			}
			if l == ends[0] {
				ends, dash = ends[1:], dash[1:]
			}
			continue
		}
		out[i] = quote == 0 && !goesOn
		goesOn = false
	scan:
		for j := 0; j < len(line); j++ {
			c := line[j]
			switch {
			case quote == '\'':
				if c == '\'' {
					quote = 0
				}
			case quote == '"':
				switch c {
				case '\\':
					j++
				case '"':
					quote = 0
				}
			case c == '\\':
				if j == len(line)-1 {
					goesOn = true
				}
				j++
			case c == '\'' || c == '"':
				quote = c
			case c == '#' && (j == 0 || strings.IndexByte(" \t;&|()", line[j-1]) >= 0):
				break scan
			case strings.HasPrefix(line[j:], "(("):
				arith++
				j++
			case strings.HasPrefix(line[j:], "))") && arith > 0:
				arith--
				j++
			case strings.HasPrefix(line[j:], "<<<"):
				// A here-string: what follows is a word, not a document.
				j += 2
			case c == '<' && arith == 0:
				if m := heredocAt.FindStringSubmatch(line[j:]); m != nil {
					ends = append(ends, m[2]+m[3]+m[4])
					dash = append(dash, m[1] == "-")
					j += len(m[0]) - 1
				}
			}
		}
	}
	return out
}

package prompt

import "strings"

// HasMarkerLine reports whether reply holds marker alone on one of its
// lines. A line that only mentions the marker in a sentence does not
// count: a model once wrote "given time, I'll output LOOP_DONE" to get out
// of a hard step.
//
// Blanks around the marker do not matter, and neither do the backquotes or
// asterisks a model may dress a word in.
func HasMarkerLine(reply, marker string) bool {
	for line := range strings.Lines(reply) {
		if isMarker(line, marker) {
			return true
		}
	}
	return false
}

// EndsWithMarker reports whether the last line of reply that is not blank
// is marker alone. It is to be asked of a finished reply, never of one
// still being written.
func EndsWithMarker(reply, marker string) bool {
	reply = strings.TrimSpace(reply)
	if at := strings.LastIndexByte(reply, '\n'); at >= 0 {
		reply = reply[at+1:]
	}
	return isMarker(reply, marker)
}

// isMarker reports whether line is marker and nothing else.
func isMarker(line, marker string) bool {
	if marker == "" {
		return false
	}
	line = strings.TrimSpace(line)
	if line == marker {
		return true
	}
	return strings.TrimSpace(strings.Trim(line, "`*")) == marker
}

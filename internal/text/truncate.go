package text

import "strings"

// Truncate shortens s to fit in width terminal cells. A string that already
// fits comes back as it is. Otherwise the result is exactly width cells
// wide: as many whole grapheme clusters of s as fit beside the tail, then
// the tail (an ellipsis, say), which stands for what was cut. The tail
// should hold no escape sequence that is cut short.
//
//   - The cut falls between grapheme clusters. Where a character two cells
//     wide would straddle it, the character is dropped and a space takes
//     the cell that is left over, so the result still fills the width.
//   - Escape sequences take no room. Those before the cut are kept. After
//     the tail, Truncate writes what is needed to leave colours, styles and
//     the hyperlink as s itself leaves them, so a style that s opens and
//     closes is closed in the result too, and one that s leaves open stays
//     open. Other escape sequences in the part that was cut are dropped.
//   - An escape sequence that was cut short (see Token.Complete) is always
//     removed, even from a string that fits: it would take in whatever is
//     printed after the result.
//   - The tail is written at the cut, in the style of the text it replaces.
//     A tail wider than width is left out.
//   - s is taken as one line: a newline in it is a zero-width control
//     character like any other.
//
// A width below zero counts as zero. Truncate takes time proportional to
// len(s).
func Truncate(s string, width int, tail string) string {
	return Metrics{}.Truncate(s, width, tail)
}

// Truncate is like the package's Truncate, with widths counted by m's rules.
func (m Metrics) Truncate(s string, width int, tail string) string {
	s = finished(s)
	if m.VisibleWidth(s) <= width {
		return s
	}
	width = max(width, 0)
	tailWidth := m.VisibleWidth(tail)
	if tailWidth > width {
		tail, tailWidth = "", 0
	}
	room := width - tailWidth

	// Keep pieces while the clusters among them fit in the room.
	var at look // the look in effect at the cut
	used := 0
	rest := s
	for rest != "" {
		p, after := m.nextPiece(rest)
		if p.kind == Plain && used+p.width > room {
			break
		}
		used += p.width
		at.applyPiece(p)
		rest = after
	}
	kept := s[:len(s)-len(rest)]

	// Where s ends up, and where the result stands once the tail is written.
	end := at
	end.applyText(rest)
	at.applyText(tail)

	out := make([]byte, 0, len(kept)+(room-used)+len(tail)+len(Reset)+len(linkClose))
	out = append(out, kept...)
	for range room - used {
		out = append(out, ' ')
	}
	out = append(out, tail...)
	out = at.appendChange(out, end)
	return string(out)
}

// PadRight returns s with spaces added after it until it is width cells
// wide, as VisibleWidth counts them. A string that is already that wide or
// wider gets none. Like Truncate, the three padding functions remove escape
// sequences that were cut short, which would otherwise take in the padding.
func PadRight(s string, width int) string { return Metrics{}.PadRight(s, width) }

// PadRight is like the package's PadRight, with widths counted by m's rules.
func (m Metrics) PadRight(s string, width int) string {
	s = finished(s)
	if n := width - m.VisibleWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// PadLeft returns s with spaces added before it until it is width cells
// wide, as VisibleWidth counts them. A string that is already that wide or
// wider gets none.
func PadLeft(s string, width int) string { return Metrics{}.PadLeft(s, width) }

// PadLeft is like the package's PadLeft, with widths counted by m's rules.
func (m Metrics) PadLeft(s string, width int) string {
	s = finished(s)
	if n := width - m.VisibleWidth(s); n > 0 {
		return strings.Repeat(" ", n) + s
	}
	return s
}

// Center returns s with spaces added on both sides until it is width cells
// wide, as VisibleWidth counts them. When the spaces cannot be shared
// evenly the extra one goes on the right. A string that is already that
// wide or wider gets none.
func Center(s string, width int) string { return Metrics{}.Center(s, width) }

// Center is like the package's Center, with widths counted by m's rules.
func (m Metrics) Center(s string, width int) string {
	s = finished(s)
	n := width - m.VisibleWidth(s)
	if n <= 0 {
		return s
	}
	left := n / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", n-left)
}

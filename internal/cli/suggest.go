package cli

import (
	"strings"
	"unicode/utf8"
)

// longestName is the longest text worth comparing with a name of the table.
// Nothing in the table comes near it, and it keeps the comparison cheap
// whatever is typed.
const longestName = 64

// distance counts the single slips between two names: a letter added,
// dropped or changed, or two neighbouring letters swapped. Letter case does
// not count. Text longer than any name is simply far.
func distance(a, b string) int {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	if len(ra) > longestName || len(rb) > longestName {
		return longestName
	}
	// Three rows of the usual table: the row two back is needed for swaps.
	back2 := make([]int, len(rb)+1)
	back1 := make([]int, len(rb)+1)
	row := make([]int, len(rb)+1)
	for j := range back1 {
		back1[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		row[0] = i
		for j := 1; j <= len(rb); j++ {
			change := 1
			if ra[i-1] == rb[j-1] {
				change = 0
			}
			row[j] = min(back1[j]+1, row[j-1]+1, back1[j-1]+change)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				row[j] = min(row[j], back2[j-2]+1)
			}
		}
		back2, back1, row = back1, row, back2
	}
	return back1[len(rb)]
}

// fold reduces a name to what stays the same across the ways people write
// one: lower case, with dashes and underscores taken out.
func fold(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "-", "")
	return strings.ReplaceAll(s, "_", "")
}

// slips is how many slips a mistyped name of n letters may have and still
// be matched to a real one.
func slips(n int) int {
	switch {
	case n <= 4:
		return 1
	case n <= 8:
		return 2
	}
	return 3
}

// nearest returns the name in names that typed most likely meant, or "".
// It tries, in order: the same name written another way (letter case,
// dashes, underscores); the one name that typed is the start of; and the
// name fewest slips away, if that is within limit. Among equals the first
// in names wins, so the answer never depends on anything but its inputs.
func nearest(typed string, names []string, limit int) string {
	n := utf8.RuneCountInString(typed)
	if n == 0 || n > longestName {
		return ""
	}
	if key := fold(typed); key != "" {
		for _, name := range names {
			if fold(name) == key {
				return name
			}
		}
	}
	if n >= 3 {
		lower := strings.ToLower(typed)
		start := ""
		count := 0
		for _, name := range names {
			if strings.HasPrefix(strings.ToLower(name), lower) {
				if count == 0 {
					start = name
				}
				count++
			}
		}
		if count == 1 {
			return start
		}
	}
	best, bestD := "", limit+1
	for _, name := range names {
		if d := distance(typed, name); d < bestD {
			best, bestD = name, d
		}
	}
	if bestD >= n {
		// Every letter would have to change: that is not a slip.
		return ""
	}
	return best
}

// nearCommand returns the subcommand of c that word is one slip away from,
// or nil. It is used to tell a mistyped command from a one-word prompt, so
// it is strict: one slip only, and for a name of three letters or fewer
// only two neighbouring letters swapped, because too many ordinary words
// are one letter away from "mem". One-letter aliases are never matched.
func nearCommand(c *command, word string) (*command, string) {
	if utf8.RuneCountInString(word) < 2 {
		return nil, ""
	}
	for _, s := range c.subs {
		for _, name := range append([]string{s.name}, s.aliases...) {
			n := utf8.RuneCountInString(name)
			if n < 2 || distance(word, name) > 1 {
				continue
			}
			if n <= 3 && !swapped(word, name) && !strings.EqualFold(word, name) {
				continue
			}
			return s, name
		}
	}
	return nil, ""
}

// swapped reports whether a is b with two neighbouring letters changed
// places, letter case aside.
func swapped(a, b string) bool {
	ra, rb := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	if len(ra) != len(rb) {
		return false
	}
	for i := 0; i+1 < len(ra); i++ {
		if ra[i] != rb[i] {
			return ra[i] == rb[i+1] && ra[i+1] == rb[i] && string(ra[i+2:]) == string(rb[i+2:])
		}
	}
	return false
}

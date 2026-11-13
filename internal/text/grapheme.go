package text

import (
	"iter"
	"unicode/utf8"
)

// props packs what the package knows about one code point: its grapheme
// class, its width class, two facts about emoji, and whether a line may end
// before and after it. tables.go lays out the bits.
type props uint16

// class is the grapheme class, one of the gc constants.
func (p props) class() uint8 { return uint8(p & propClassMask) }

// widthClass is one of the width constants.
func (p props) widthClass() uint8 { return uint8(p>>propWidthShift) & 3 }

// emojiSelector reports whether U+FE0F after the character makes an emoji
// presentation sequence.
func (p props) emojiSelector() bool { return p&propEmojiSelector != 0 }

// modifierBase reports whether a skin tone after the character joins it.
func (p props) modifierBase() bool { return p&propModifierBase != 0 }

// wrapBefore and wrapAfter are wrap constants: what the character says about
// ending a line just before it and just after it.
func (p props) wrapBefore() uint8 { return uint8(p>>propBeforeShift) & 3 }
func (p props) wrapAfter() uint8  { return uint8(p>>propAfterShift) & 3 }

// The code points and bytes the rules below name.
const (
	emojiSelector   = 0xFE0F  // variation selector 16: show the character before it as an emoji
	firstSkinTone   = 0x1F3FB // the five emoji modifiers, light to dark
	lastSkinTone    = 0x1F3FF
	replacementRune = 0xFFFD // what a terminal shows for a byte that is not UTF-8
	escapeByte      = 0x1B   // ESC, which starts every escape sequence
	spaceByte       = 0x20   // the first ASCII character that takes a cell
	deleteByte      = 0x7F   // DEL, the one ASCII control above the space
)

// asciiCells is the width of an ASCII byte: one cell, or none for a control.
func asciiCells(b byte) int {
	if b >= spaceByte && b != deleteByte {
		return 1
	}
	return 0
}

// A clusterState follows the rules of UAX #29 through one extended grapheme
// cluster, one code point at a time, and adds up the cluster's width as it
// goes. The zero value is ready to take the first code point.
//
// Every rule that looks back further than one code point looks back only
// within the cluster (a conjunct, an emoji joined by ZWJ, a pair of regional
// indicators), so the state can start fresh at any cluster boundary.
type clusterState struct {
	n     int   // code points taken so far
	prev  uint8 // grapheme class of the last one
	flag  bool  // the last one is a regional indicator still waiting for its pair
	pict  uint8 // rule GB11: 1 after "ExtPict Extend*", 2 once a ZWJ has followed
	conj  uint8 // rule GB9c: 1 after "Consonant {Extend|Linker}*", 2 once a Linker is among them
	first props // properties of the first code point
	width int   // the code points' own widths, added up
	emoji bool  // the cluster is shown as one emoji
}

// take offers the cluster its next code point: r, with grapheme class class,
// properties p and a width of w cells when it stands alone. It reports
// whether the code point belongs to the cluster. If it does not, a cluster
// boundary lies before it and the state is unchanged.
//
// The cases are the rules of UAX #29, in the standard's order.
func (st *clusterState) take(r rune, class uint8, p props, w int) bool {
	joinedByZWJ := false
	if st.n > 0 {
		prev := st.prev
		switch {
		case prev == gcCR && class == gcLF:
			// GB3: CR x LF.
		case prev >= gcCR && prev <= gcControl:
			// GB4: break after controls.
			return false
		case class >= gcCR && class <= gcControl:
			// GB5: break before controls.
			return false
		case prev == gcL && (class == gcL || class == gcV || class == gcLV || class == gcLVT):
			// GB6: Hangul syllable sequences.
		case (prev == gcLV || prev == gcV) && (class == gcV || class == gcT):
			// GB7.
		case (prev == gcLVT || prev == gcT) && class == gcT:
			// GB8.
		case class >= gcExtend && class <= gcZWJ:
			// GB9: no break before extending characters or ZWJ.
		case class == gcSpacingMark:
			// GB9a.
		case prev == gcPrepend:
			// GB9b.
		case class == gcConsonant && st.conj == 2:
			// GB9c: no break inside an Indic conjunct.
		case class == gcPictographic && st.pict == 2:
			// GB11: no break inside an emoji ZWJ sequence.
			joinedByZWJ = true
		case class == gcRegionalIndicator && st.flag:
			// GB12 and GB13: regional indicators pair up into flags.
		default:
			// GB999: break everywhere else.
			return false
		}
	}

	switch {
	case class == gcPictographic:
		st.pict = 1
	case st.pict == 1 && class == gcZWJ:
		st.pict = 2
	case st.pict == 1 && class >= gcExtend && class <= gcLinker:
		// Extend* between the pictograph and the ZWJ.
	default:
		st.pict = 0
	}
	switch {
	case class == gcConsonant:
		st.conj = 1
	case st.conj != 0 && class == gcLinker:
		st.conj = 2
	case st.conj != 0 && (class == gcExtendConjunct || class == gcZWJ):
		// An extender between the consonants.
	default:
		st.conj = 0
	}
	st.flag = class == gcRegionalIndicator && !st.flag

	switch st.n {
	case 0:
		st.first = p
	case 1:
		// An emoji presentation sequence, or a modifier sequence.
		if r == emojiSelector && st.first.emojiSelector() {
			st.emoji = true
		}
		if r >= firstSkinTone && r <= lastSkinTone && st.first.modifierBase() {
			st.emoji = true
		}
	}
	if joinedByZWJ {
		st.emoji = true
	}
	st.width += w
	st.prev = class
	st.n++
	return true
}

// cells is the width of the cluster taken so far. A cluster that is shown as
// one emoji takes two cells whatever it is made of. Any other cluster takes
// the sum of its code points' widths.
func (st *clusterState) cells() int {
	if st.emoji {
		return 2
	}
	return st.width
}

// cells is the width of one code point standing alone.
func (m Metrics) cells(p props) int {
	switch p.widthClass() {
	case widthZero:
		return 0
	case widthWide:
		return 2
	case widthAmbiguous:
		if m.AmbiguousWide {
			return 2
		}
	}
	return 1
}

// scan feeds st the code points at the start of s for as long as they belong
// to its cluster, and returns how many bytes that was. With stopAtEscape,
// scan stops in front of an ESC byte without asking the rules: the caller
// deals with escape sequences itself.
func (m Metrics) scan(st *clusterState, s string, stopAtEscape bool) int {
	i := 0
	for i < len(s) {
		b := s[i]
		if b == escapeByte && stopAtEscape {
			break
		}
		if b < utf8.RuneSelf && st.n > 0 && st.prev != gcPrepend && !(st.prev == gcCR && b == '\n') {
			// An ASCII character never joins the cluster before it, but
			// for these two cases: no need to look it up.
			break
		}
		r, size, class, p, w := m.decode(s[i:], stopAtEscape)
		if !st.take(r, class, p, w) {
			break
		}
		i += size
	}
	return i
}

// decodeAcross decodes the character at the start of s when escape sequences
// may stand between its bytes, as in a string cut and joined by something
// that did not know UTF-8. It returns the character and the number of bytes
// of s it takes up, the sequences inside it included. Taking the sequences
// out of s would leave the same character, which is what makes the width of
// a string with sequences equal to the width of the string without them.
// When the bytes do not make a character even so, it returns
// utf8.RuneError and 1.
func decodeAcross(s string) (r rune, size int) {
	var buf [utf8.UTFMax]byte
	var ends [utf8.UTFMax]int // ends[k] is where in s the first k+1 bytes end
	buf[0], ends[0] = s[0], 1
	n, pos := 1, 1
	for n < len(buf) {
		for pos < len(s) && s[pos] == escapeByte {
			k, _, _ := scanSequence(s[pos:])
			pos += k
		}
		// Only continuation bytes can complete the character.
		if pos >= len(s) || s[pos]&0xC0 != 0x80 {
			break
		}
		buf[n] = s[pos]
		pos++
		ends[n] = pos
		n++
	}
	r, used := utf8.DecodeRune(buf[:n])
	if r == utf8.RuneError && used <= 1 {
		return utf8.RuneError, 1
	}
	return r, ends[used-1]
}

// decode reads the character at the start of s, which must not be empty: the
// code point, its length in bytes, its grapheme class, its properties, and
// its width when it stands alone.
//
// A byte that is not part of valid UTF-8 comes back as utf8.RuneError with a
// length of one. It breaks clusters as a control does, and is as wide as the
// replacement character a terminal shows in its place. With across, escape
// sequences that stand between the bytes of a character are passed over and
// counted in its length (see decodeAcross).
func (m Metrics) decode(s string, across bool) (r rune, size int, class uint8, p props, width int) {
	r, size = rune(s[0]), 1
	if s[0] >= utf8.RuneSelf {
		r, size = utf8.DecodeRuneInString(s)
		if r == utf8.RuneError && size == 1 && across {
			r, size = decodeAcross(s)
		}
		if r == utf8.RuneError && size == 1 {
			return r, 1, gcControl, 0, m.cells(lookup(replacementRune))
		}
	}
	p = lookup(r)
	return r, size, p.class(), p, m.cells(p)
}

// codePoint measures the character at the start of s, which must not be
// empty or begin with ESC: its length in bytes, counting any escape
// sequences that stand between its bytes, and its width when it stands
// alone.
func (m Metrics) codePoint(s string) (size, width int) {
	_, size, _, _, width = m.decode(s, true)
	return size, width
}

// clusterAt returns the length in bytes of the grapheme cluster at the start
// of s, and its width. s must not be empty.
func (m Metrics) clusterAt(s string) (n, width int) {
	// An ASCII byte followed by another ASCII byte, or by nothing, is a
	// whole cluster, except CR LF: no ASCII character extends another.
	if b := s[0]; b < utf8.RuneSelf && (len(s) == 1 || s[1] < utf8.RuneSelf) {
		if b == '\r' && len(s) > 1 && s[1] == '\n' {
			return 2, 0
		}
		return 1, asciiCells(b)
	}
	var st clusterState
	n = m.scan(&st, s, false)
	return n, st.cells()
}

// Graphemes returns an iterator over the extended grapheme clusters of s, as
// UAX #29 defines them: what a reader takes for one character, such as a
// letter with its accents, a Hangul syllable, or an emoji with its skin tone.
// The clusters are substrings of s and, joined, give s back. A byte that is
// not part of valid UTF-8 is a cluster of its own.
//
// Graphemes takes s as plain text. In a string with escape sequences each
// ESC is a cluster of its own, like any control character; use Tokens to
// take the sequences out first.
//
// The iterator does not allocate and takes time proportional to len(s).
func Graphemes(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for len(s) > 0 {
			n, _ := Metrics{}.clusterAt(s)
			if !yield(s[:n]) {
				return
			}
			s = s[n:]
		}
	}
}

// NextGrapheme splits the first grapheme cluster off s. It returns the
// cluster, its width in terminal cells as Width counts it, and what is left
// of s. For the empty string it returns "", 0, "".
//
// It is the stepping form of Graphemes, for loops that also need the width or
// that stop early. It does not allocate and takes time proportional to the
// length of the cluster.
func NextGrapheme(s string) (cluster string, width int, rest string) {
	return Metrics{}.NextGrapheme(s)
}

// NextGrapheme is like the package's NextGrapheme, with the width counted by
// m's rules.
func (m Metrics) NextGrapheme(s string) (cluster string, width int, rest string) {
	if s == "" {
		return "", 0, ""
	}
	n, w := m.clusterAt(s)
	return s[:n], w, s[n:]
}

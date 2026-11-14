package permission

import (
	"net/netip"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A deny rule, an ask rule and the list of protected paths are guards: a
// guard must hold for every name that reaches the same file, and some
// file systems take more than one name for a file. macOS takes "A" for
// "a", a composed "é" for an "e" followed by an accent, and (on its older
// file system) skips a handful of characters that cannot be seen; a
// Windows share drops dots and blanks from the end of a name. guardKey
// turns a name into a key that all of those spellings share. Two
// different files may get the same key: a guard then covers both, which
// is the safe mistake. Allow rules never use it.

// looseMark stands in a key for a run of text that cannot be compared
// letter for letter without Unicode's tables of composed characters,
// which the standard library does not carry: one for each letter of such
// text, together with the accents that follow it.
const looseMark = '\x01'

// guardKey returns the key of one path segment (or of one segment of a
// path pattern, whose * ? [ ] and \ are ASCII and stay as they are).
func guardKey(s string) string {
	plain := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			plain = false
			break
		}
	}
	var key string
	if plain {
		key = strings.ToLower(s)
	} else {
		key = looseKey(s)
	}
	// A name's trailing dots and blanks are dropped by file systems made
	// for Windows.
	if t := strings.TrimRight(key, " ."); t != "" {
		return t
	}
	return key
}

// guardPattern is guardKey for one segment of a path pattern: the same
// key, with the dots and blanks at its end taken off even where the
// pattern escaped them ("name\."), so that the pattern still fits the
// key of the name it was written for.
func guardPattern(seg string) string {
	key := seg
	if plain := guardKey(seg + "x"); len(plain) > 0 {
		key = plain[:len(plain)-1] // the key without the trimming
	}
	t := key
	for {
		switch {
		case strings.HasSuffix(t, "\\.") || strings.HasSuffix(t, "\\ "):
			t = t[:len(t)-2]
		case strings.HasSuffix(t, ".") || strings.HasSuffix(t, " "):
			t = t[:len(t)-1]
		default:
			if t == "" {
				return key
			}
			return t
		}
	}
}

// looseKey is guardKey for text outside ASCII.
func looseKey(s string) string {
	var out []rune
	// exact says the last rune written is one that is compared as it is.
	exact := false
	for _, r := range s {
		switch {
		case hfsIgnored(r):
			// The older macOS file system does not look at these at all:
			// ".git" with a zero-width character (U+200C) in it is ".git" there.
			continue
		case r < utf8.RuneSelf:
			out, exact = append(out, unicode.ToLower(r)), true
		case unicode.Is(unicode.M, r):
			// An accent joins the letter before it. Whether the pair is
			// also a single character somewhere in Unicode cannot be told
			// here, so the letter stops being compared as it is.
			if exact && len(out) > 0 {
				out[len(out)-1] = looseMark
			} else if len(out) == 0 {
				out = append(out, looseMark)
			}
			exact = false
		default:
			if a := asciiFold(r); a != "" {
				// A character that case folding turns into ASCII letters.
				out, exact = append(out, []rune(a)...), true
				continue
			}
			if l, v, t, ok := hangul(r); ok {
				// A Korean syllable is its letters, by arithmetic.
				out = append(out, l, v)
				if t != 0 {
					out = append(out, t)
				}
				exact = true
				continue
			}
			if stableRune(r) {
				out, exact = append(out, r), true
				continue
			}
			out, exact = append(out, looseMark), false
		}
	}
	return string(out)
}

// hfsIgnored reports whether r is one of the characters HFS+ leaves out
// when it compares names.
func hfsIgnored(r rune) bool {
	return r >= 0x200C && r <= 0x200F || r >= 0x202A && r <= 0x202E || r >= 0x206A && r <= 0x206F || r == 0xFEFF
}

// asciiFold returns the ASCII text a character outside ASCII is the same
// as under Unicode's case folding or as a composed form, or "".
func asciiFold(r rune) string {
	switch r {
	case 0x017F: // long s
		return "s"
	case 0x212A: // Kelvin sign
		return "k"
	case 0x00DF, 0x1E9E: // sharp s
		return "ss"
	case 0xFB00:
		return "ff"
	case 0xFB01:
		return "fi"
	case 0xFB02:
		return "fl"
	case 0xFB03:
		return "ffi"
	case 0xFB04:
		return "ffl"
	case 0xFB05, 0xFB06:
		return "st"
	case 0x037E: // Greek question mark, which is a semicolon
		return ";"
	case 0x1FEF: // Greek varia, which is a grave accent
		return "`"
	}
	return ""
}

// hangul takes a composed Korean syllable apart into its leading
// consonant, vowel and trailing consonant (0 when there is none).
func hangul(r rune) (l, v, t rune, ok bool) {
	const base, count = 0xAC00, 11172
	if r < base || r >= base+count {
		return 0, 0, 0, false
	}
	i := r - base
	l, v = 0x1100+i/588, 0x1161+(i%588)/28
	if tt := i % 28; tt != 0 {
		t = 0x11A7 + tt
	}
	return l, v, t, true
}

// stableRune reports whether r is a character that has no composed or
// taken-apart form and no upper or lower case: Korean letters and the
// unified Chinese characters. These are compared as they are.
func stableRune(r rune) bool {
	return r >= 0x1100 && r <= 0x11FF || // Hangul letters
		r >= 0x3400 && r <= 0x4DBF || r >= 0x4E00 && r <= 0x9FFF || // unified ideographs
		r >= 0x20000 && r <= 0x2A6DF
}

// guardPath returns the key of a cleaned absolute path, segment by
// segment.
func guardPath(abs string) string {
	plain := true
	for i := 0; i < len(abs); i++ {
		if c := abs[i]; c >= utf8.RuneSelf || c == '.' || c == ' ' {
			plain = false
			break
		}
	}
	if plain {
		return sameDisk(strings.ToLower(abs))
	}
	segs := strings.Split(abs, "/")
	for i, s := range segs {
		if s != "" {
			segs[i] = guardKey(s)
		}
	}
	return sameDisk(strings.Join(segs, "/"))
}

// macOS has more than one absolute path for a file, and no call that
// follows links says so, because none of these is a link:
//
//	/System/Volumes/Data/Users/x   the disk that holds everybody's files, mounted a second time
//	/.nofollow/Users/x             the same path, read without following links
//	/.resolve/1/Users/x            the same again, with a number that says how to read it
//
// aliasLen returns how much of the front of a path (or of a path's key:
// letter case does not matter here) is one of those ways in, or 0.
func aliasLen(p string) int {
	fits := func(prefix string) bool {
		return len(p) >= len(prefix) && strings.EqualFold(p[:len(prefix)], prefix) && (len(p) == len(prefix) || p[len(prefix)] == '/')
	}
	switch {
	case fits("/system/volumes/data"):
		return len("/system/volumes/data")
	case fits("/.nofollow"):
		return len("/.nofollow")
	case fits("/proc/self/root"):
		return len("/proc/self/root") // Linux: the root of the process that asks
	case fits("/proc/thread-self/root"):
		return len("/proc/thread-self/root")
	case len(p) > len("/.resolve/") && strings.EqualFold(p[:len("/.resolve/")], "/.resolve/"):
		rest := p[len("/.resolve/"):]
		n := strings.IndexByte(rest, '/')
		if n < 0 {
			n = len(rest)
		}
		if isDigits(rest[:n]) {
			return len("/.resolve/") + n
		}
	}
	return 0
}

// sameDisk takes those ways in off the front of a path, so that a guard
// holds for every one of them. (A path that has one and is another file
// than the path without it gets a guard it did not need.)
func sameDisk(p string) string {
	for n := aliasLen(p); n > 0; n = aliasLen(p) {
		if p = p[n:]; p == "" {
			p = "/"
		}
	}
	return p
}

// oneDisk is sameDisk for a path as it is written.
func oneDisk(p string) string { return sameDisk(p) }

// byNumber reports whether a cleaned absolute path names a file by its
// number and not by where it is: macOS opens /.vol/<disk>/<file number>
// for any file, and no rule about paths can be held against that.
func byNumber(p string) bool {
	p = oneDisk(p)
	if len(p) >= 5 && strings.EqualFold(p[:5], "/.vol") && (len(p) == 5 || p[5] == '/') {
		return true
	}
	// Linux has the like for another process's files: /proc/<pid>/root,
	// /proc/<pid>/cwd and /proc/<pid>/fd/<n> lead wherever that process
	// stands or has open. (/proc/self is this process, whose links any
	// call that follows links reads.)
	if rest, ok := strings.CutPrefix(p, "/proc/"); ok {
		pid, after, _ := strings.Cut(rest, "/")
		if isDigits(pid) {
			name, _, _ := strings.Cut(after, "/")
			switch name {
			case "root", "cwd", "fd", "exe", "map_files", "task":
				return true
			}
		}
	}
	return false
}

// progKey is the key of a program's name for the rules that guard. A
// file system that takes no notice of case finds rm for RM, and Bash for
// bash; one that skips the characters nobody can see finds it for a name
// with one of those in it. Deny and ask rules, and the tables of
// programs that run other commands, go by this key. Allow rules and the
// list of commands that only read never do.
func progKey(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			var b strings.Builder
			for _, r := range s {
				if !hfsIgnored(r) {
					b.WriteRune(unicode.ToLower(r))
				}
			}
			return b.String()
		}
	}
	return strings.ToLower(s)
}

// foldFirst returns text with its first word, the program's name, turned
// into its key.
func foldFirst(text string) string {
	i := strings.IndexAny(text, " \t\n")
	if i < 0 {
		i = len(text)
	}
	return progKey(text[:i]) + text[i:]
}

// magicProg reports whether a program's path says nothing about which
// program it is: /proc/self/exe is the shell that reads the line,
// /proc/<pid>/exe whatever that process runs, /dev/fd/3 a file that was
// opened, and macOS's /.vol/<disk>/<number> any file at all. Such a
// program is one nobody knows ahead of time.
func magicProg(name string) bool {
	segs := splitPath(oneDisk(name))
	for i, s := range segs {
		switch {
		case i == 0 && strings.EqualFold(s, ".vol"):
			return true
		case s == "dev" && i+1 < len(segs) && segs[i+1] == "fd":
			return true
		case s == "proc" && i+2 < len(segs) && (segs[i+1] == "self" || segs[i+1] == "thread-self" || isDigits(segs[i+1])):
			switch segs[i+2] {
			case "exe", "fd", "map_files", "task":
				return true
			}
		}
	}
	return false
}

// canonHost returns the form of a host that rules and URLs are compared
// in, and whether the host can be compared at all. Letters are lowered,
// a trailing dot is dropped, and an address is written one way whatever
// way it came: 127.1, 0x7f.0.0.1, 2130706433 and ::ffff:127.0.0.1 are all
// 127.0.0.1 to the programs that will connect to them. A host with
// characters outside ASCII is not compared: the programs that fetch
// translate such names (and some of their dots), and only the translated
// form, which starts xn--, says where the request goes.
func canonHost(h string) (string, bool) {
	h = strings.TrimRight(strings.ToLower(h), ".")
	if h == "" {
		return "", false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if c >= utf8.RuneSelf || c <= ' ' || c == 0x7f || strings.IndexByte("\\/@?#\"<>|^`{}[]", c) >= 0 {
			return "", false
		}
	}
	if strings.Contains(h, ":") {
		zone := ""
		if i := strings.IndexByte(h, '%'); i >= 0 {
			h, zone = h[:i], h[i:]
		}
		a, err := netip.ParseAddr(h)
		if err != nil {
			return "", false
		}
		return a.Unmap().String() + zone, true
	}
	if strings.Contains(h, "%") {
		return "", false
	}
	ip, numeric := atonIPv4(h)
	switch {
	case ip != "":
		return ip, true
	case numeric:
		// All numbers, but not an address any program agrees on.
		return "", false
	}
	return h, true
}

// atonIPv4 reads a host the way the C library's inet_aton does, which is
// how many programs still read one: one to four numbers, each decimal,
// octal (a leading 0) or hexadecimal (0x), the last filling whatever
// bytes are left. It returns the address in the usual form, or "" and
// whether the host was made of numbers all the same.
func atonIPv4(h string) (ip string, numeric bool) {
	parts := strings.Split(h, ".")
	if len(parts) > 4 {
		return "", allNumeric(parts)
	}
	var vals []uint64
	for _, p := range parts {
		base := 10
		switch {
		case strings.HasPrefix(p, "0x"):
			p, base = p[2:], 16
		case len(p) > 1 && p[0] == '0':
			p, base = p[1:], 8
		}
		v, err := strconv.ParseUint(p, base, 32)
		if err != nil {
			return "", allNumeric(parts)
		}
		vals = append(vals, v)
	}
	var addr uint64
	for i, v := range vals {
		last := i == len(vals)-1
		switch {
		case !last && v > 255:
			return "", true
		case !last:
			addr |= v << (8 * (3 - i))
		case v >= 1<<(8*(4-i)):
			return "", true
		default:
			addr |= v
		}
	}
	return netip.AddrFrom4([4]byte{byte(addr >> 24), byte(addr >> 16), byte(addr >> 8), byte(addr)}).String(), true
}

// allNumeric reports whether every label starts with a digit, which no
// real host name's last label does.
func allNumeric(parts []string) bool {
	for _, p := range parts {
		if p == "" || p[0] < '0' || p[0] > '9' {
			return false
		}
	}
	return true
}

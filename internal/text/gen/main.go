// Command gen writes the tables of package text: the Unicode properties the
// package needs, packed for lookup, and the xterm colour palette in Oklab.
//
// It is run by hand when the Unicode version changes, never as part of a
// build:
//
//	go run ./internal/text/gen -fetch > /tmp/tables.go
//
// and its output replaces internal/text/tables.go. The data files are read
// from -dir; with -fetch, any that are missing are first downloaded there
// from unicode.org. Every file is checked against the SHA-256 recorded below
// before it is used, so the tables can only come from the files this program
// names. With -testdata DIR the two official test files the package's tests
// read are copied into DIR as well. With -stats the sizes of the candidate
// table layouts are printed to standard error.
//
// The Unicode version is the one Go 1.27's unicode.Version reports, so that
// the tables and the standard library describe the same characters.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// unicodeVersion is the version of the Unicode Character Database the tables
// are made from.
const unicodeVersion = "17.0.0"

// baseURL is where that version's files are published.
const baseURL = "https://www.unicode.org/Public/" + unicodeVersion + "/"

// A source is one data file: its name in the data directory, its path under
// baseURL, and the SHA-256 its contents must have.
type source struct {
	name   string
	path   string
	sha256 string
}

// tableSources lists the files the tables are made from.
func tableSources() []source {
	return []source{
		{"GraphemeBreakProperty.txt", "ucd/auxiliary/GraphemeBreakProperty.txt", "d6b51d1d2ae5c33b451b7ed994b48f1f4dc62b2272a5831e7fd418514a6bae89"},
		{"DerivedCoreProperties.txt", "ucd/DerivedCoreProperties.txt", "24c7fed1195c482faaefd5c1e7eb821c5ee1fb6de07ecdbaa64b56a99da22c08"},
		{"emoji-data.txt", "ucd/emoji/emoji-data.txt", "2cb2bb9455cda83e8481541ecf5b6dfda66a3bb89efa3fa7c5297eccf607b72b"},
		{"emoji-variation-sequences.txt", "ucd/emoji/emoji-variation-sequences.txt", "bb3d09ef03f206012c7532dd52dc0a21c9efddba0135ea4cf0d9201b8b9bba7e"},
		{"EastAsianWidth.txt", "ucd/EastAsianWidth.txt", "ea7ce50f3444a050333448dffef1cadd9325af55cbb764b4a2280faf52170a33"},
		{"DerivedGeneralCategory.txt", "ucd/extracted/DerivedGeneralCategory.txt", "d62e5bab70ca74f099343f71224fa051cb1fdd61a1ab45c0488c44cfc0b6102e"},
		{"HangulSyllableType.txt", "ucd/HangulSyllableType.txt", "5a57450afde0d082bc5026f7458649eac3b615490cc7e3d916b0367f1593c0e3"},
		{"PropList.txt", "ucd/PropList.txt", "130dcddcaadaf071008bdfce1e7743e04fdfbc910886f017d9f9ac931d8c64dd"},
		{"LineBreak.txt", "ucd/LineBreak.txt", "e6a18fa91f8f6a6f8e534b1d3f128c21ada45bfe152eb6b1bcc5e15fd8ac92e6"},
	}
}

// testSources lists the official test files kept under testdata.
func testSources() []source {
	return []source{
		{"GraphemeBreakTest.txt", "ucd/auxiliary/GraphemeBreakTest.txt", "e2d134d2c52919bace503ebb6a551c1855fe1a1faec18478c78fff254a1793ec"},
		{"emoji-test.txt", "emoji/emoji-test.txt", "1d8a944f88d7952f7ef7c5167fef3c67995bcae24543949710231b03a201acda"},
	}
}

func main() {
	dir := flag.String("dir", filepath.Join(os.TempDir(), "gnosh-ucd", unicodeVersion), "directory that holds the Unicode data files")
	fetch := flag.Bool("fetch", false, "download any data file that is missing from -dir")
	testdata := flag.String("testdata", "", "also copy the official test files into this directory")
	stats := flag.Bool("stats", false, "print the sizes of the candidate table layouts to standard error")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	g := generator{dir: *dir, fetch: *fetch, stats: *stats, client: &http.Client{Timeout: 2 * time.Minute}}
	out, err := g.run(ctx, *testdata)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		fmt.Fprintln(os.Stderr, "gen: writing the tables to standard output:", err)
		os.Exit(1)
	}
}

// A generator holds where the data files live and how to get them.
type generator struct {
	dir    string
	fetch  bool
	stats  bool
	client *http.Client
}

// run reads the data files, builds the tables and returns the Go source that
// holds them.
func (g *generator) run(ctx context.Context, testdata string) ([]byte, error) {
	files := map[string][]byte{}
	for _, src := range tableSources() {
		data, err := g.load(ctx, src)
		if err != nil {
			return nil, err
		}
		files[src.name] = data
	}
	for _, src := range testSources() {
		data, err := g.load(ctx, src)
		if err != nil {
			return nil, err
		}
		if testdata == "" {
			continue
		}
		if err := os.MkdirAll(testdata, 0o755); err != nil {
			return nil, fmt.Errorf("making the test data directory %s: %w", testdata, err)
		}
		dst := filepath.Join(testdata, src.name)
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return nil, fmt.Errorf("copying %s to %s: %w", src.name, dst, err)
		}
	}

	db, err := readDatabase(files)
	if err != nil {
		return nil, err
	}
	values, err := db.pack()
	if err != nil {
		return nil, err
	}
	return g.emit(values)
}

// load returns the contents of one data file from the data directory, after
// downloading it there if it is missing and -fetch was given. The contents
// are refused unless they hash to the recorded SHA-256.
func (g *generator) load(ctx context.Context, src source) ([]byte, error) {
	path := filepath.Join(g.dir, src.name)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && g.fetch {
		data, err = g.download(ctx, src)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(g.dir, 0o755); err != nil {
			return nil, fmt.Errorf("making the data directory %s: %w", g.dir, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, fmt.Errorf("saving %s: %w", path, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("reading %s (run with -fetch to download it from %s): %w", path, baseURL+src.path, err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != src.sha256 {
		return nil, fmt.Errorf("%s has SHA-256 %s, but %s is recorded for %s; the file was not used", path, got, src.sha256, baseURL+src.path)
	}
	return data, nil
}

// download fetches one data file from unicode.org.
func (g *generator) download(ctx context.Context, src source) ([]byte, error) {
	url := baseURL + src.path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing the request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "ucd-table-generator")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading %s: the server answered %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("downloading %s: the body broke off: %w", url, err)
	}
	return data, nil
}

// runes is the number of code points, U+0000 to U+10FFFF.
const runes = 0x110000

// The values of Grapheme_Cluster_Break, in the order the generator numbers
// them.
const (
	gcbOther = iota
	gcbCR
	gcbLF
	gcbControl
	gcbExtend
	gcbZWJ
	gcbRegionalIndicator
	gcbPrepend
	gcbSpacingMark
	gcbL
	gcbV
	gcbT
	gcbLV
	gcbLVT
)

// The values of Indic_Conjunct_Break.
const (
	incbNone = iota
	incbConsonant
	incbExtend
	incbLinker
)

// The values of East_Asian_Width.
const (
	eawN = iota
	eawNa
	eawH
	eawA
	eawW
	eawF
)

// The values of Hangul_Syllable_Type the generator tells apart.
const (
	hstOther = iota
	hstV
	hstT
)

// The binary properties, as bits of database.flags.
const (
	flagPictographic      = 1 << iota // Extended_Pictographic
	flagEmoji                         // Emoji
	flagEmojiPresentation             // Emoji_Presentation
	flagModifier                      // Emoji_Modifier
	flagModifierBase                  // Emoji_Modifier_Base
	flagIgnorable                     // Default_Ignorable_Code_Point
	flagPrepended                     // Prepended_Concatenation_Mark
	flagEmojiSelector                 // listed with U+FE0F in emoji-variation-sequences.txt
	flagTextSelector                  // listed with U+FE0E in emoji-variation-sequences.txt
)

// A database holds, for every code point, the properties read from the data
// files.
type database struct {
	gcb   []uint8  // Grapheme_Cluster_Break
	incb  []uint8  // Indic_Conjunct_Break
	eaw   []uint8  // East_Asian_Width
	hst   []uint8  // Hangul_Syllable_Type, V and T only
	gc    []string // General_Category
	lb    []string // Line_Break
	flags []uint16
}

// readDatabase parses the data files.
func readDatabase(files map[string][]byte) (*database, error) {
	db := &database{
		gcb:   make([]uint8, runes),
		incb:  make([]uint8, runes),
		eaw:   make([]uint8, runes),
		hst:   make([]uint8, runes),
		gc:    make([]string, runes),
		lb:    make([]string, runes),
		flags: make([]uint16, runes),
	}
	for i := range db.gc {
		db.gc[i] = "Cn" // UAX #44: code points that are not listed are unassigned.
		db.lb[i] = "XX"
	}

	gcbNames := map[string]uint8{
		"CR": gcbCR, "LF": gcbLF, "Control": gcbControl, "Extend": gcbExtend, "ZWJ": gcbZWJ,
		"Regional_Indicator": gcbRegionalIndicator, "Prepend": gcbPrepend, "SpacingMark": gcbSpacingMark,
		"L": gcbL, "V": gcbV, "T": gcbT, "LV": gcbLV, "LVT": gcbLVT,
	}
	name := "GraphemeBreakProperty.txt"
	if err := wantMissing(name, files[name], "Other"); err != nil {
		return nil, err
	}
	err := eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		v, ok := gcbNames[field(fields, 0)]
		if !ok {
			return fmt.Errorf("unknown Grapheme_Cluster_Break value %q", field(fields, 0))
		}
		fill(db.gcb, lo, hi, v)
		return nil
	})
	if err != nil {
		return nil, err
	}

	incbNames := map[string]uint8{"Consonant": incbConsonant, "Extend": incbExtend, "Linker": incbLinker}
	name = "DerivedCoreProperties.txt"
	if err := wantMissing(name, files[name], "InCB; None"); err != nil {
		return nil, err
	}
	err = eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		switch field(fields, 0) {
		case "InCB":
			v, ok := incbNames[field(fields, 1)]
			if !ok {
				return fmt.Errorf("unknown Indic_Conjunct_Break value %q", field(fields, 1))
			}
			fill(db.incb, lo, hi, v)
		case "Default_Ignorable_Code_Point":
			db.setFlag(lo, hi, flagIgnorable)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	emojiFlags := map[string]uint16{
		"Emoji": flagEmoji, "Emoji_Presentation": flagEmojiPresentation, "Emoji_Modifier": flagModifier,
		"Emoji_Modifier_Base": flagModifierBase, "Emoji_Component": 0, "Extended_Pictographic": flagPictographic,
	}
	name = "emoji-data.txt"
	err = eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		f, ok := emojiFlags[field(fields, 0)]
		if !ok {
			return fmt.Errorf("unknown emoji property %q", field(fields, 0))
		}
		db.setFlag(lo, hi, f)
		return nil
	})
	if err != nil {
		return nil, err
	}

	name = "emoji-variation-sequences.txt"
	if err := db.readVariationSequences(name, files[name]); err != nil {
		return nil, err
	}

	eawNames := map[string]uint8{"N": eawN, "Na": eawNa, "H": eawH, "A": eawA, "W": eawW, "F": eawF}
	name = "EastAsianWidth.txt"
	if err := wantMissing(name, files[name], "N"); err != nil {
		return nil, err
	}
	err = eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		v, ok := eawNames[field(fields, 0)]
		if !ok {
			return fmt.Errorf("unknown East_Asian_Width value %q", field(fields, 0))
		}
		fill(db.eaw, lo, hi, v)
		return nil
	})
	if err != nil {
		return nil, err
	}

	name = "DerivedGeneralCategory.txt"
	err = eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		v := field(fields, 0)
		if len(v) != 2 {
			return fmt.Errorf("unknown General_Category value %q", v)
		}
		fill(db.gc, lo, hi, v)
		return nil
	})
	if err != nil {
		return nil, err
	}

	name = "HangulSyllableType.txt"
	if err := wantMissing(name, files[name], "Not_Applicable"); err != nil {
		return nil, err
	}
	err = eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		switch v := field(fields, 0); v {
		case "V":
			fill(db.hst, lo, hi, hstV)
		case "T":
			fill(db.hst, lo, hi, hstT)
		case "L", "LV", "LVT":
		default:
			return fmt.Errorf("unknown Hangul_Syllable_Type value %q", v)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	name = "PropList.txt"
	err = eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		if field(fields, 0) == "Prepended_Concatenation_Mark" {
			db.setFlag(lo, hi, flagPrepended)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	name = "LineBreak.txt"
	if err := wantMissing(name, files[name], "XX"); err != nil {
		return nil, err
	}
	err = eachEntry(name, files[name], func(lo, hi rune, fields []string) error {
		v := field(fields, 0)
		if _, _, ok := wrapOf(v); !ok {
			return fmt.Errorf("unknown Line_Break value %q", v)
		}
		fill(db.lb, lo, hi, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return db, nil
}

// readVariationSequences records which characters emoji-variation-sequences.txt
// lists with the emoji presentation selector U+FE0F and with the text
// presentation selector U+FE0E.
func (db *database) readVariationSequences(name string, data []byte) error {
	for n, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		seq, _, _ := strings.Cut(line, ";")
		points := strings.Fields(seq)
		if len(points) != 2 {
			return fmt.Errorf("%s line %d: want a character and a selector, found %q", name, n+1, seq)
		}
		base, err1 := strconv.ParseUint(points[0], 16, 32)
		selector, err2 := strconv.ParseUint(points[1], 16, 32)
		if err1 != nil || err2 != nil || base >= runes {
			return fmt.Errorf("%s line %d: cannot read the code points in %q", name, n+1, seq)
		}
		switch selector {
		case 0xFE0F:
			db.flags[base] |= flagEmojiSelector
		case 0xFE0E:
			db.flags[base] |= flagTextSelector
		default:
			return fmt.Errorf("%s line %d: unknown selector U+%04X", name, n+1, selector)
		}
	}
	return nil
}

func (db *database) setFlag(lo, hi rune, f uint16) {
	for r := lo; r <= hi; r++ {
		db.flags[r] |= f
	}
}

func (db *database) has(r rune, f uint16) bool { return db.flags[r]&f != 0 }

// fill sets the entries lo through hi of a property to v.
func fill[T any](prop []T, lo, hi rune, v T) {
	for r := lo; r <= hi; r++ {
		prop[r] = v
	}
}

// field returns the i-th field of a line, or "" if the line has too few.
func field(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return ""
}

// eachEntry calls fn for every data line of a Unicode Character Database
// file: the code points named by the line's first field, and its other
// fields with the space around them removed.
func eachEntry(name string, data []byte, fn func(lo, hi rune, fields []string) error) error {
	for n, line := range strings.Split(string(data), "\n") {
		line, _, _ = strings.Cut(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ";")
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		first, last, isRange := strings.Cut(fields[0], "..")
		if !isRange {
			last = first
		}
		lo, err1 := strconv.ParseUint(first, 16, 32)
		hi, err2 := strconv.ParseUint(last, 16, 32)
		if err1 != nil || err2 != nil || lo > hi || hi >= runes {
			return fmt.Errorf("%s line %d: cannot read the code points in %q", name, n+1, fields[0])
		}
		if err := fn(rune(lo), rune(hi), fields[1:]); err != nil {
			return fmt.Errorf("%s line %d: %w", name, n+1, err)
		}
	}
	return nil
}

// wantMissing checks that a file's "@missing" line, which gives the value of
// the code points the file does not list, names the default the generator
// assumes.
func wantMissing(name string, data []byte, value string) error {
	want := "# @missing: 0000..10FFFF; " + value
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == want {
			return nil
		}
	}
	return fmt.Errorf("%s: no line %q; the default for unlisted code points may have changed", name, want)
}

// The grapheme classes of package text. A class is a value of
// Grapheme_Cluster_Break, split further where the rules of UAX #29 look at
// Extended_Pictographic or Indic_Conjunct_Break. The names are written into
// the generated file in this order.
const (
	classOther = iota
	classCR
	classLF
	classControl
	classExtend
	classExtendConjunct
	classLinker
	classZWJ
	classRegionalIndicator
	classPrepend
	classSpacingMark
	classL
	classV
	classT
	classLV
	classLVT
	classPictographic
	classConsonant
	classCount
)

// classNames gives each grapheme class its name and comment in the generated
// file.
func classNames() [classCount][2]string {
	return [classCount][2]string{
		{"gcOther", "anything not named below"},
		{"gcCR", "carriage return"},
		{"gcLF", "line feed"},
		{"gcControl", "the other controls, format characters and separators"},
		{"gcExtend", "Extend with Indic_Conjunct_Break=None"},
		{"gcExtendConjunct", "Extend with Indic_Conjunct_Break=Extend"},
		{"gcLinker", "Extend with Indic_Conjunct_Break=Linker"},
		{"gcZWJ", "zero width joiner, which has Indic_Conjunct_Break=Extend"},
		{"gcRegionalIndicator", "one half of a flag"},
		{"gcPrepend", "Prepend"},
		{"gcSpacingMark", "SpacingMark"},
		{"gcL", "Hangul leading consonant"},
		{"gcV", "Hangul vowel"},
		{"gcT", "Hangul trailing consonant"},
		{"gcLV", "Hangul syllable without a trailing consonant"},
		{"gcLVT", "Hangul syllable with a trailing consonant"},
		{"gcPictographic", "Other with Extended_Pictographic"},
		{"gcConsonant", "Other with Indic_Conjunct_Break=Consonant"},
	}
}

// The width classes of package text.
const (
	widthZero = iota
	widthNarrow
	widthWide
	widthAmbiguous
)

// What a character says about ending a line next to it, when no space lies
// between it and its neighbour.
const (
	wrapNeutral = iota // no opinion: the neighbour decides
	wrapAllow          // a line may end here unless the neighbour forbids it
	wrapForbid         // a line may not end here
	wrapHyphen         // after only: a hyphen or dash, see package text's Wrap
)

// The layout of the property value of package text.
const (
	propClassBits     = 5
	propWidthShift    = 5
	propEmojiSelector = 1 << 7
	propModifierBase  = 1 << 8
	propBeforeShift   = 9
	propAfterShift    = 11
)

// wrapOf maps a Line_Break value to what package text needs to know about it:
// whether a line may end before and after the character. It is a small part
// of UAX #14: only the classes that open a break without a space (ideographs
// and their like) and the ones that forbid one beside them.
func wrapOf(lineBreak string) (before, after uint8, ok bool) {
	switch lineBreak {
	case "ID", "EB", "EM", "H2", "H3", "JL", "JV", "JT", "RI":
		// Ideographs, emoji, Hangul and flags: a line may end on either side.
		return wrapAllow, wrapAllow, true
	case "CJ", "NS":
		// Small kana and other nonstarters: never at the start of a line.
		return wrapForbid, wrapAllow, true
	case "ZW":
		// Zero width space exists to allow a break after it.
		return wrapForbid, wrapAllow, true
	case "CL", "CP", "EX", "IS", "SY", "IN", "PO", "BA":
		// Closing punctuation and its like stay with what they follow.
		return wrapForbid, wrapNeutral, true
	case "HY", "HH":
		// Hyphens stay with what they follow; after them Wrap decides.
		return wrapForbid, wrapHyphen, true
	case "B2":
		// Em dashes: after them Wrap decides.
		return wrapNeutral, wrapHyphen, true
	case "OP", "PR", "BB":
		// Opening punctuation and its like stay with what follows them.
		return wrapNeutral, wrapForbid, true
	case "QU", "GL", "WJ", "ZWJ", "CM":
		// Quotation marks, glue and combining marks: no break on either side.
		return wrapForbid, wrapForbid, true
	case "AI", "AK", "AL", "AP", "AS", "BK", "CB", "CR", "HL", "LF", "NL", "NU", "SA", "SG", "SP", "VF", "VI", "XX":
		return wrapNeutral, wrapNeutral, true
	}
	return 0, 0, false
}

// pack computes the property value of every code point, after checking the
// facts about the data that package text's algorithms take for granted.
func (db *database) pack() ([]uint16, error) {
	values := make([]uint16, runes)
	for i := range values {
		r := rune(i)
		class, err := db.class(r)
		if err != nil {
			return nil, err
		}
		width, err := db.width(r)
		if err != nil {
			return nil, err
		}
		before, after, _ := wrapOf(db.lb[r])
		if wide := db.eaw[r] == eawW || db.eaw[r] == eawF; wide {
			// Between East Asian wide characters a line may end, whatever
			// their line breaking class, unless one of them forbids it.
			if before == wrapNeutral {
				before = wrapAllow
			}
			if after == wrapNeutral {
				after = wrapAllow
			}
		}
		v := uint16(class) | uint16(width)<<propWidthShift | uint16(before)<<propBeforeShift | uint16(after)<<propAfterShift
		if db.has(r, flagEmojiSelector) {
			if !db.has(r, flagEmoji) {
				return nil, fmt.Errorf("U+%04X takes the emoji presentation selector but is not an emoji", r)
			}
			v |= propEmojiSelector
		}
		if db.has(r, flagModifierBase) {
			if class != classPictographic {
				return nil, fmt.Errorf("U+%04X is an emoji modifier base but not Extended_Pictographic", r)
			}
			v |= propModifierBase
		}
		if db.has(r, flagModifier) != (r >= 0x1F3FB && r <= 0x1F3FF) {
			return nil, fmt.Errorf("U+%04X: the emoji modifiers are no longer exactly U+1F3FB to U+1F3FF", r)
		}
		if db.has(r, flagModifier) && class != classExtend && class != classExtendConjunct {
			return nil, fmt.Errorf("U+%04X is an emoji modifier but its Grapheme_Cluster_Break is not Extend", r)
		}
		if db.has(r, flagEmojiPresentation) && width != widthWide && db.gcb[r] != gcbRegionalIndicator {
			return nil, fmt.Errorf("U+%04X has Emoji_Presentation but is not East Asian wide", r)
		}
		if db.has(r, flagEmoji) && !db.has(r, flagEmojiPresentation) && !db.has(r, flagEmojiSelector) {
			return nil, fmt.Errorf("U+%04X is a text-style emoji that does not take the emoji presentation selector", r)
		}
		values[i] = v
	}
	return values, nil
}

// class returns the grapheme class of r.
func (db *database) class(r rune) (uint8, error) {
	pictographic := db.has(r, flagPictographic)
	incb := db.incb[r]
	bad := func() (uint8, error) {
		return 0, fmt.Errorf("U+%04X: Grapheme_Cluster_Break %d with Indic_Conjunct_Break %d and Extended_Pictographic %t is a combination package text has no class for", r, db.gcb[r], incb, pictographic)
	}
	switch db.gcb[r] {
	case gcbOther:
		switch {
		case pictographic && incb == incbNone:
			return classPictographic, nil
		case !pictographic && incb == incbConsonant:
			return classConsonant, nil
		case !pictographic && incb == incbNone:
			return classOther, nil
		}
		return bad()
	case gcbExtend:
		if pictographic || incb == incbConsonant {
			return bad()
		}
		switch incb {
		case incbLinker:
			return classLinker, nil
		case incbExtend:
			return classExtendConjunct, nil
		}
		return classExtend, nil
	case gcbZWJ:
		if pictographic || incb != incbExtend {
			return bad()
		}
		return classZWJ, nil
	}
	if pictographic || incb != incbNone {
		return bad()
	}
	switch db.gcb[r] {
	case gcbCR:
		return classCR, nil
	case gcbLF:
		return classLF, nil
	case gcbControl:
		return classControl, nil
	case gcbRegionalIndicator:
		return classRegionalIndicator, nil
	case gcbPrepend:
		return classPrepend, nil
	case gcbSpacingMark:
		return classSpacingMark, nil
	case gcbL:
		return classL, nil
	case gcbV:
		return classV, nil
	case gcbT:
		return classT, nil
	case gcbLV:
		return classLV, nil
	case gcbLVT:
		return classLVT, nil
	}
	return bad()
}

// width returns the width class of r: how many terminal cells the character
// takes when it stands alone.
//
// Zero: nonspacing and enclosing marks; controls, format characters, line and
// paragraph separators and surrogates; unassigned code points that are
// default-ignorable; and the Hangul vowels and trailing consonants that join
// the syllable before them. Two exceptions keep a cell, as terminals have
// always given them one: the soft hyphen, and the Arabic and Kaithi signs
// that are format characters in name but are drawn (Prepended_Concatenation_Mark).
// Everything else goes by East_Asian_Width: wide and fullwidth take two
// cells, ambiguous is left for the caller to decide, the rest take one.
func (db *database) width(r rune) (uint8, error) {
	zero := false
	switch db.gc[r] {
	case "Mn", "Me", "Cc", "Cf", "Zl", "Zp", "Cs":
		zero = true
	case "Cn":
		zero = db.has(r, flagIgnorable)
	}
	if db.hst[r] != hstOther {
		zero = true
	}
	if r == 0x00AD || db.has(r, flagPrepended) {
		zero = false
	}
	// Whatever UAX #29 treats as a control must not take a cell, except the
	// soft hyphen.
	if c := db.gcb[r]; (c == gcbControl || c == gcbCR || c == gcbLF) && !zero && r != 0x00AD {
		return 0, fmt.Errorf("U+%04X is a control for grapheme breaking but would be given a width", r)
	}
	if zero {
		return widthZero, nil
	}
	switch db.eaw[r] {
	case eawW, eawF:
		return widthWide, nil
	case eawA:
		return widthAmbiguous, nil
	}
	return widthNarrow, nil
}

// A layout is one way to store the per-code-point classes as a two-stage
// table: the code point's high bits pick a block through the index, its low
// bits pick an entry in the block. Equal blocks are stored once.
type layout struct {
	shift  uint
	index  []uint16 // block number for each run of 1<<shift code points
	blocks []uint8  // the distinct blocks, one after another
}

// size is the number of bytes the layout takes.
func (l layout) size() int {
	return len(l.index)*l.indexBytes() + len(l.blocks)
}

// indexBytes is the number of bytes one index entry takes.
func (l layout) indexBytes() int {
	if len(l.blocks)>>l.shift <= 256 {
		return 1
	}
	return 2
}

// newLayout builds the two-stage table for a block size of 1<<shift.
func newLayout(ids []uint8, shift uint) layout {
	l := layout{shift: shift}
	size := 1 << shift
	seen := map[string]uint16{}
	for start := 0; start < len(ids); start += size {
		block := string(ids[start : start+size])
		n, ok := seen[block]
		if !ok {
			n = uint16(len(l.blocks) >> shift)
			seen[block] = n
			l.blocks = append(l.blocks, block...)
		}
		l.index = append(l.index, n)
	}
	return l
}

// emit writes the generated file.
func (g *generator) emit(values []uint16) ([]byte, error) {
	// Number the distinct property values; the table stores those numbers.
	var distinct []uint16
	numbers := map[uint16]uint8{}
	ids := make([]uint8, len(values))
	for i, v := range values {
		n, ok := numbers[v]
		if !ok {
			if len(distinct) == 256 {
				return nil, errors.New("more than 256 distinct property values: the table's entries no longer fit in a byte")
			}
			n = uint8(len(distinct))
			numbers[v] = n
			distinct = append(distinct, v)
		}
		ids[i] = n
	}

	best := newLayout(ids, 4)
	for shift := uint(4); shift <= 10; shift++ {
		l := newLayout(ids, shift)
		if g.stats {
			fmt.Fprintf(os.Stderr, "block size %4d: %5d blocks, %d-byte index entries, %6d bytes\n", 1<<shift, len(l.blocks)>>shift, l.indexBytes(), l.size())
		}
		if l.size() < best.size() {
			best = l
		}
	}
	if g.stats {
		fmt.Fprintf(os.Stderr, "chosen block size %d; %d distinct property values\n", 1<<best.shift, len(distinct))
	}

	var b bytes.Buffer
	b.WriteString("// Code generated by internal/text/gen from the Unicode Character Database. DO NOT EDIT.\n")
	b.WriteString("//\n")
	fmt.Fprintf(&b, "// Unicode version %s. The tables were made from these files, each checked\n", unicodeVersion)
	b.WriteString("// against its SHA-256 before it was read:\n")
	b.WriteString("//\n")
	for _, src := range tableSources() {
		fmt.Fprintf(&b, "//\t%s\n//\t\tsha256 %s\n", baseURL+src.path, src.sha256)
	}
	b.WriteString("//\n")
	b.WriteString("// The tests read these files, kept under testdata:\n")
	b.WriteString("//\n")
	for _, src := range testSources() {
		fmt.Fprintf(&b, "//\t%s\n//\t\tsha256 %s\n", baseURL+src.path, src.sha256)
	}
	b.WriteString("\npackage text\n\n")

	b.WriteString("// unicodeVersion is the version of the Unicode Character Database the tables\n// below were made from.\n")
	fmt.Fprintf(&b, "const unicodeVersion = %q\n\n", unicodeVersion)

	b.WriteString("// The grapheme classes: the values of Grapheme_Cluster_Break, split further\n")
	b.WriteString("// where the rules of UAX #29 look at Extended_Pictographic or\n")
	b.WriteString("// Indic_Conjunct_Break. The order matters to the code that reads them: gcCR,\n")
	b.WriteString("// gcLF and gcControl stand together, and so do the three kinds of Extend,\n")
	b.WriteString("// with gcZWJ right after them, so that one comparison finds each group.\n")
	b.WriteString("const (\n")
	for i, nc := range classNames() {
		if i == 0 {
			fmt.Fprintf(&b, "\t%s = iota // %s\n", nc[0], nc[1])
		} else {
			fmt.Fprintf(&b, "\t%s // %s\n", nc[0], nc[1])
		}
	}
	b.WriteString(")\n\n")

	b.WriteString("// The width classes: how many cells a character takes when it stands alone.\n")
	b.WriteString("const (\n")
	b.WriteString("\twidthZero = iota // no cell: marks, controls, format characters, joining Hangul letters\n")
	b.WriteString("\twidthNarrow // one cell\n")
	b.WriteString("\twidthWide // two cells: East Asian wide and fullwidth\n")
	b.WriteString("\twidthAmbiguous // East Asian ambiguous: one cell unless the caller says two\n")
	b.WriteString(")\n\n")

	b.WriteString("// What a character says about ending a line next to it when no space lies\n")
	b.WriteString("// between it and its neighbour. It is taken from the character's Line_Break\n")
	b.WriteString("// class and East_Asian_Width.\n")
	b.WriteString("const (\n")
	b.WriteString("\twrapNeutral = iota // no opinion: the neighbour decides\n")
	b.WriteString("\twrapAllow // a line may end here unless the neighbour forbids it\n")
	b.WriteString("\twrapForbid // a line may not end here\n")
	b.WriteString("\twrapHyphen // after the character only: a hyphen or dash\n")
	b.WriteString(")\n\n")

	b.WriteString("// The layout of a props value.\n")
	b.WriteString("const (\n")
	fmt.Fprintf(&b, "\tpropClassMask = %#x // the grapheme class\n", 1<<propClassBits-1)
	fmt.Fprintf(&b, "\tpropWidthShift = %d // then two bits of width class\n", propWidthShift)
	fmt.Fprintf(&b, "\tpropEmojiSelector = %#x // U+FE0F after the character makes an emoji presentation sequence\n", propEmojiSelector)
	fmt.Fprintf(&b, "\tpropModifierBase = %#x // the character is an emoji modifier base\n", propModifierBase)
	fmt.Fprintf(&b, "\tpropBeforeShift = %d // two bits: may a line end before the character\n", propBeforeShift)
	fmt.Fprintf(&b, "\tpropAfterShift = %d // two bits: may a line end after the character\n", propAfterShift)
	b.WriteString(")\n\n")

	indexBytes := best.indexBytes()
	index := make([]byte, 0, len(best.index)*indexBytes)
	for _, n := range best.index {
		if indexBytes == 2 {
			index = append(index, byte(n>>8))
		}
		index = append(index, byte(n))
	}
	props := make([]byte, 0, 2*len(distinct))
	for _, v := range distinct {
		props = append(props, byte(v>>8), byte(v))
	}

	fmt.Fprintf(&b, "// lookup returns the properties of r. A value that is not a code point is\n// given the properties of U+FFFD, the replacement character.\n")
	b.WriteString("func lookup(r rune) props {\n")
	b.WriteString("\tif uint32(r) > 0x10FFFF {\n\t\tr = 0xFFFD\n\t}\n")
	if indexBytes == 2 {
		fmt.Fprintf(&b, "\ti := 2 * (uint(r) >> %d)\n", best.shift)
		b.WriteString("\tblock := uint(tableIndex[i])<<8 | uint(tableIndex[i+1])\n")
	} else {
		fmt.Fprintf(&b, "\tblock := uint(tableIndex[uint(r)>>%d])\n", best.shift)
	}
	fmt.Fprintf(&b, "\tn := 2 * uint(tableBlocks[block<<%d|uint(r)&%#x])\n", best.shift, 1<<best.shift-1)
	b.WriteString("\treturn props(tableProps[n])<<8 | props(tableProps[n+1])\n")
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "// tableIndex gives, for each run of %d code points, the number of the block\n// in tableBlocks that describes it", 1<<best.shift)
	if indexBytes == 2 {
		b.WriteString(", as two bytes, high byte first")
	}
	fmt.Fprintf(&b, ". %d bytes.\n", len(index))
	writeBytes(&b, "tableIndex", index)

	fmt.Fprintf(&b, "// tableBlocks holds the %d distinct blocks of %d code points each. An entry\n// is the number of the code point's property value in tableProps. %d bytes.\n", len(best.blocks)>>best.shift, 1<<best.shift, len(best.blocks))
	writeBytes(&b, "tableBlocks", best.blocks)

	fmt.Fprintf(&b, "// tableProps holds the %d distinct property values, two bytes each, high byte\n// first.\n", len(distinct))
	writeBytes(&b, "tableProps", props)

	fmt.Fprintf(&b, "// labScale is what an Oklab coordinate is multiplied by to store it as a whole\n// number.\n")
	fmt.Fprintf(&b, "const labScale = %d\n\n", labScale)
	b.WriteString("// paletteLab holds the 256 colours of the xterm palette in Oklab: for each\n")
	b.WriteString("// colour L, a and b, multiplied by labScale and rounded, as two-byte signed\n")
	b.WriteString("// numbers, high byte first. Colours 0 to 15 are xterm's defaults, 16 to 231\n")
	b.WriteString("// the 6x6x6 cube, 232 to 255 the grey ramp.\n")
	writeBytes(&b, "paletteLab", paletteLab())

	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting the generated source: %w", err)
	}
	return out, nil
}

// writeBytes writes data as a string constant, a fixed number of bytes to a
// line.
func writeBytes(b *bytes.Buffer, name string, data []byte) {
	const perLine = 32
	fmt.Fprintf(b, "const %s = \"\" +\n", name)
	for start := 0; start < len(data); start += perLine {
		end := min(start+perLine, len(data))
		b.WriteString("\t\"")
		for _, c := range data[start:end] {
			fmt.Fprintf(b, "\\x%02x", c)
		}
		if end == len(data) {
			b.WriteString("\"\n\n")
		} else {
			b.WriteString("\" +\n")
		}
	}
}

// labScale is what an Oklab coordinate is multiplied by to store it as a
// whole number that fits in sixteen bits.
const labScale = 30000

// paletteLab returns the xterm palette in Oklab, packed as described in the
// generated file.
func paletteLab() []byte {
	out := make([]byte, 0, 256*6)
	for i := range 256 {
		r, g, b := paletteRGB(i)
		for _, v := range oklab(r, g, b) {
			n := int16(math.Round(v * labScale))
			out = append(out, byte(uint16(n)>>8), byte(uint16(n)))
		}
	}
	return out
}

// paletteRGB returns colour i of the xterm 256-colour palette: xterm's
// default values for the sixteen ANSI colours, the 6x6x6 colour cube, and the
// grey ramp, as documented in xterm's manual and control-sequence notes.
func paletteRGB(i int) (r, g, b uint8) {
	ansi := [16][3]uint8{
		{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0}, {0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
		{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0}, {92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
	}
	levels := [6]uint8{0, 95, 135, 175, 215, 255}
	switch {
	case i < 16:
		return ansi[i][0], ansi[i][1], ansi[i][2]
	case i < 232:
		i -= 16
		return levels[i/36], levels[i/6%6], levels[i%6]
	}
	v := uint8(8 + 10*(i-232))
	return v, v, v
}

// oklab converts an sRGB colour to Oklab (Björn Ottosson, "A perceptual color
// space for image processing", 2020).
func oklab(r, g, b uint8) [3]float64 {
	linear := func(c uint8) float64 {
		v := float64(c) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	lr, lg, lb := linear(r), linear(g), linear(b)
	l := math.Cbrt(0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb)
	m := math.Cbrt(0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb)
	s := math.Cbrt(0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb)
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

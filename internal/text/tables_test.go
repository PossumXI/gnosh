package text

import (
	"crypto/sha256"
	"encoding/hex"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"strings"
	"testing"
	"unicode"
)

// TestUnicodeVersion checks that the tables describe the same Unicode version
// as the standard library. If this fails after a Go upgrade, the tables are
// due to be made again: see gen/main.go.
func TestUnicodeVersion(t *testing.T) {
	if unicodeVersion != unicode.Version {
		t.Errorf("the tables are for Unicode %s but the unicode package is for %s; run internal/text/gen for the new version", unicodeVersion, unicode.Version)
	}
}

// TestTestdataChecksums checks that the official test files are the ones the
// generated tables name.
func TestTestdataChecksums(t *testing.T) {
	sums := map[string]string{
		// https://www.unicode.org/Public/17.0.0/ucd/auxiliary/GraphemeBreakTest.txt
		"GraphemeBreakTest.txt": "e2d134d2c52919bace503ebb6a551c1855fe1a1faec18478c78fff254a1793ec",
		// https://www.unicode.org/Public/17.0.0/emoji/emoji-test.txt
		"emoji-test.txt": "1d8a944f88d7952f7ef7c5167fef3c67995bcae24543949710231b03a201acda",
	}
	generated := readFile(t, "tables.go")
	for name, want := range sums {
		sum := sha256.Sum256([]byte(readTestdata(t, name)))
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Errorf("testdata/%s has SHA-256 %s, want %s", name, got, want)
		}
		if !strings.Contains(generated, want) {
			t.Errorf("tables.go does not name the checksum of %s", name)
		}
	}
	if !strings.Contains(readTestdata(t, "GraphemeBreakTest.txt"), "GraphemeBreakTest-"+unicodeVersion+".txt") {
		t.Errorf("testdata/GraphemeBreakTest.txt is not the file for Unicode %s", unicodeVersion)
	}
}

// TestNoPackageVariables holds the package to its promise of keeping no
// state: outside the tests it declares no package-level variable at all. The
// tables are string constants.
func TestNoPackageVariables(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("listing the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files++
		for _, decl := range file.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
				t.Errorf("%s declares a package-level variable", fset.Position(gen.Pos()))
			}
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" {
				t.Errorf("%s declares an init function", fset.Position(fn.Pos()))
			}
		}
	}
	if files == 0 {
		t.Error("found no source files to check")
	}
}

// TestLookup spot-checks the generated table against facts from the Unicode
// Character Database.
func TestLookup(t *testing.T) {
	tests := []struct {
		r     rune
		class uint8
		width uint8
	}{
		{'a', gcOther, widthNarrow},
		{'\r', gcCR, widthZero},
		{'\n', gcLF, widthZero},
		{0x00, gcControl, widthZero},
		{0x1B, gcControl, widthZero},
		{0x7F, gcControl, widthZero},
		{0x00AD, gcControl, widthAmbiguous}, // soft hyphen keeps a cell
		{0x0300, gcExtendConjunct, widthZero},
		{0x200C, gcExtend, widthZero},
		{0x200D, gcZWJ, widthZero},
		{0x094D, gcLinker, widthZero},
		{0x0915, gcConsonant, widthNarrow},
		{0x0903, gcSpacingMark, widthNarrow},
		{0x0600, gcPrepend, widthNarrow},
		{0x1100, gcL, widthWide},
		{0x1160, gcV, widthZero},
		{0x11A8, gcT, widthZero},
		{0xAC00, gcLV, widthWide},
		{0xAC01, gcLVT, widthWide},
		{0x16D63, gcV, widthNarrow}, // a Kirat Rai vowel: V for breaking, but not a Hangul letter
		{0x1F1E6, gcRegionalIndicator, widthNarrow},
		{0x1F600, gcPictographic, widthWide},
		{0x00A9, gcPictographic, widthNarrow},
		{0x1F3FB, gcExtendConjunct, widthWide},
		{0x4E00, gcOther, widthWide},
		{0xFF21, gcOther, widthWide},
		{0xFF71, gcOther, widthNarrow},
		{0x03B1, gcOther, widthAmbiguous},
		{0xE000, gcOther, widthAmbiguous},
		{0xFFFD, gcOther, widthAmbiguous},
		{0x2065, gcControl, widthZero},         // unassigned, default-ignorable
		{0x0378, gcOther, widthNarrow},         // unassigned
		{0x2FFFD, gcOther, widthWide},          // unassigned, in a plane of ideographs
		{0xE0001, gcControl, widthZero},        // language tag
		{0xE0020, gcExtendConjunct, widthZero}, // tag space
		{0x10FFFD, gcOther, widthAmbiguous},    // private use
		{0x10FFFF, gcOther, widthNarrow},       // a noncharacter
	}
	for _, tt := range tests {
		p := lookup(tt.r)
		if p.class() != tt.class || p.widthClass() != tt.width {
			t.Errorf("lookup(U+%04X): class %d width %d, want class %d width %d", tt.r, p.class(), p.widthClass(), tt.class, tt.width)
		}
	}
	// Values that are not code points are treated as U+FFFD.
	for _, r := range []rune{-1, 0x110000, math.MaxInt32} {
		if lookup(r) != lookup(0xFFFD) {
			t.Errorf("lookup(%#x) differs from lookup(U+FFFD)", r)
		}
	}
	// Emoji facts.
	if !lookup('#').emojiSelector() || !lookup(0x2764).emojiSelector() || lookup('a').emojiSelector() {
		t.Error("the emoji selector flag is wrong for #, U+2764 or a")
	}
	if !lookup(0x1F44D).modifierBase() || !lookup(0x261D).modifierBase() || lookup(0x1F600).modifierBase() {
		t.Error("the modifier base flag is wrong for U+1F44D, U+261D or U+1F600")
	}
	// Every ASCII character is plain or a control, which is what the fast
	// paths for ASCII rely on.
	for r := rune(0); r < 0x80; r++ {
		p := lookup(r)
		control := r < 0x20 || r == 0x7F
		wantWidth := uint8(widthNarrow)
		if control {
			wantWidth = widthZero
		}
		isControl := p.class() >= gcCR && p.class() <= gcControl
		if (p.class() != gcOther && !isControl) || isControl != control || p.widthClass() != wantWidth {
			t.Errorf("lookup(%#x): class %d width %d does not suit the ASCII fast path", r, p.class(), p.widthClass())
		}
	}
}

// TestLookupWrap spot-checks what the table says about ending a line
// before and after a character.
func TestLookupWrap(t *testing.T) {
	tests := []struct {
		r             rune
		before, after uint8
	}{
		{'a', wrapNeutral, wrapNeutral},
		{'(', wrapNeutral, wrapForbid},
		{')', wrapForbid, wrapNeutral},
		{'.', wrapForbid, wrapNeutral},
		{'"', wrapForbid, wrapForbid},
		{'-', wrapForbid, wrapHyphen},
		{0x2010, wrapForbid, wrapHyphen},   // hyphen
		{0x2014, wrapNeutral, wrapHyphen},  // em dash
		{0x00A0, wrapForbid, wrapForbid},   // no-break space
		{0x200B, wrapForbid, wrapAllow},    // zero width space
		{0x4E00, wrapAllow, wrapAllow},     // an ideograph
		{0xAC00, wrapAllow, wrapAllow},     // a Hangul syllable
		{0x3042, wrapAllow, wrapAllow},     // hiragana a
		{0x3063, wrapForbid, wrapAllow},    // small hiragana tsu
		{0x3002, wrapForbid, wrapAllow},    // ideographic full stop
		{0x300C, wrapAllow, wrapForbid},    // left corner bracket
		{0x300D, wrapForbid, wrapAllow},    // right corner bracket
		{0xFF21, wrapAllow, wrapAllow},     // fullwidth A
		{0x1F600, wrapAllow, wrapAllow},    // an emoji
		{0x1F1E6, wrapAllow, wrapAllow},    // a regional indicator
		{0x0301, wrapForbid, wrapForbid},   // a combining mark
		{0x03B1, wrapNeutral, wrapNeutral}, // Greek alpha: ambiguous width is not wide
	}
	for _, tt := range tests {
		p := lookup(tt.r)
		if p.wrapBefore() != tt.before || p.wrapAfter() != tt.after {
			t.Errorf("lookup(U+%04X): before %d after %d, want before %d after %d", tt.r, p.wrapBefore(), p.wrapAfter(), tt.before, tt.after)
		}
	}
	// Wrap does not look up the letters and digits of ASCII, taking them
	// to have no opinion.
	for r := rune(0); r < 0x80; r++ {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if p := lookup(r); p.wrapBefore() != wrapNeutral || p.wrapAfter() != wrapNeutral {
				t.Errorf("lookup(%q): before %d after %d, want neutral", r, p.wrapBefore(), p.wrapAfter())
			}
		}
	}
}

// TestLookupAgreesWithUnicodePackage compares the table with the standard
// library wherever the two describe the same thing.
func TestLookupAgreesWithUnicodePackage(t *testing.T) {
	for r := rune(0); r <= unicode.MaxRune; r++ {
		p := lookup(r)
		// Nonspacing and enclosing marks are extenders and take no cell.
		if unicode.In(r, unicode.Mn, unicode.Me) {
			if c := p.class(); c < gcExtend || c > gcLinker {
				t.Fatalf("U+%04X is a nonspacing or enclosing mark but has grapheme class %d", r, c)
			}
			if p.widthClass() != widthZero {
				t.Fatalf("U+%04X is a nonspacing or enclosing mark but has width class %d", r, p.widthClass())
			}
		}
		// Controls take no cell and break clusters.
		if unicode.Is(unicode.Cc, r) {
			if c := p.class(); c < gcCR || c > gcControl || p.widthClass() != widthZero {
				t.Fatalf("U+%04X is a control but has grapheme class %d and width class %d", r, c, p.widthClass())
			}
		}
		// Regional indicators.
		if unicode.Is(unicode.Regional_Indicator, r) != (p.class() == gcRegionalIndicator) {
			t.Fatalf("U+%04X: the table and the unicode package disagree on Regional_Indicator", r)
		}
		// Letters, digits and symbols that are not marks take a cell,
		// except the Hangul letters that join the syllable before them.
		if unicode.In(r, unicode.L, unicode.Nd, unicode.S, unicode.P) && p.widthClass() == widthZero {
			if !(r >= 0x1160 && r <= 0x11FF) && !(r >= 0xD7B0 && r <= 0xD7FF) {
				t.Fatalf("U+%04X is a letter, digit, symbol or punctuation but takes no cell", r)
			}
		}
	}
}

// TestPaletteTable checks the generated Oklab palette against the package's
// own conversion.
func TestPaletteTable(t *testing.T) {
	if len(paletteLab) != 256*6 {
		t.Fatalf("paletteLab holds %d bytes, want %d", len(paletteLab), 256*6)
	}
	for i := range 256 {
		r, g, b := paletteRGB(i)
		c := toLab(r, g, b)
		want := [3]float64{c.l, c.a, c.b}
		for k, v := range want {
			got := paletteCoordinate(6*i + 2*k)
			if math.Abs(float64(got)-v*labScale) > 0.5001 {
				t.Errorf("palette colour %d coordinate %d: table has %d, the conversion gives %.3f", i, k, got, v*labScale)
			}
		}
	}
	// The palette itself: a few values from xterm's documentation.
	tests := []struct {
		i       int
		r, g, b uint8
	}{
		{0, 0, 0, 0}, {1, 205, 0, 0}, {4, 0, 0, 238}, {7, 229, 229, 229}, {8, 127, 127, 127}, {12, 92, 92, 255}, {15, 255, 255, 255},
		{16, 0, 0, 0}, {17, 0, 0, 95}, {21, 0, 0, 255}, {46, 0, 255, 0}, {196, 255, 0, 0}, {206, 255, 95, 215}, {231, 255, 255, 255},
		{232, 8, 8, 8}, {244, 128, 128, 128}, {255, 238, 238, 238},
	}
	for _, tt := range tests {
		if r, g, b := paletteRGB(tt.i); r != tt.r || g != tt.g || b != tt.b {
			t.Errorf("palette colour %d is %d,%d,%d, want %d,%d,%d", tt.i, r, g, b, tt.r, tt.g, tt.b)
		}
	}
}

// readFile returns the contents of a file in the package directory.
func readFile(t testing.TB, name string) string {
	t.Helper()
	return readTestdata(t, "../"+name)
}

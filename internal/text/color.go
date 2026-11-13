package text

import (
	"math"
	"strconv"
)

// A Color is a colour a terminal can show: its own default, one of the
// sixteen ANSI colours, one of the 256 colours of the xterm palette, or a
// 24-bit RGB colour. The zero value is Default. Colors compare with ==.
type Color uint32

// The kinds of Color, kept in its top byte.
const (
	colorDefault = iota
	colorANSI
	colorIndexed
	colorRGB
)

// Default is the terminal's own foreground or background colour.
const Default Color = 0

// The sixteen ANSI colours. What they look like is up to the terminal's
// theme.
const (
	Black Color = colorANSI<<24 + iota
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White
	BrightBlack
	BrightRed
	BrightGreen
	BrightYellow
	BrightBlue
	BrightMagenta
	BrightCyan
	BrightWhite
)

// Indexed returns colour n of the 256-colour palette: 0 to 15 are the ANSI
// colours, 16 to 231 a 6x6x6 colour cube, 232 to 255 a ramp of greys.
func Indexed(n uint8) Color { return colorIndexed<<24 | Color(n) }

// RGB returns the 24-bit colour with the given red, green and blue.
func RGB(r, g, b uint8) Color {
	return colorRGB<<24 | Color(r)<<16 | Color(g)<<8 | Color(b)
}

// ansiColor returns ANSI colour n, 0 to 15.
func ansiColor(n int) Color { return colorANSI<<24 | Color(n&0xF) }

// kind is one of the color constants.
func (c Color) kind() uint8 { return uint8(c >> 24) }

// valid returns c, or Default if c is not a value this package's
// constructors make.
func (c Color) valid() Color {
	switch c.kind() {
	case colorANSI:
		return ansiColor(int(c))
	case colorIndexed:
		return Indexed(uint8(c))
	case colorRGB:
		return c
	}
	return Default
}

// String describes the colour for logs and test failures: "default",
// "ansi:9", "indexed:206" or "#ff5fd7".
func (c Color) String() string {
	switch c.kind() {
	case colorANSI:
		return "ansi:" + strconv.Itoa(int(c&0xF))
	case colorIndexed:
		return "indexed:" + strconv.Itoa(int(c&0xFF))
	case colorRGB:
		const hex = "0123456789abcdef"
		b := []byte("#000000")
		for i := range 6 {
			b[6-i] = hex[c>>(4*i)&0xF]
		}
		return string(b)
	}
	return "default"
}

// rgb returns the colour's red, green and blue. An ANSI or palette colour is
// looked up in the xterm palette, with xterm's defaults standing in for the
// sixteen colours a theme may change. Default has no RGB value: ok is false.
func (c Color) rgb() (r, g, b uint8, ok bool) {
	switch c.kind() {
	case colorANSI:
		r, g, b = paletteRGB(int(c & 0xF))
		return r, g, b, true
	case colorIndexed:
		r, g, b = paletteRGB(int(c & 0xFF))
		return r, g, b, true
	case colorRGB:
		return uint8(c >> 16), uint8(c >> 8), uint8(c), true
	}
	return 0, 0, 0, false
}

// paletteRGB returns colour i of the xterm 256-colour palette. The sixteen
// ANSI colours have xterm's default values; the colour cube and the grey
// ramp are fixed by xterm's documentation.
func paletteRGB(i int) (r, g, b uint8) {
	switch {
	case i < 16:
		// xterm's defaults: black, red3, green3, yellow3, blue2, magenta3,
		// cyan3, gray90, then gray50, red, green, yellow, rgb:5c/5c/ff,
		// magenta, cyan, white.
		const ansi = "\x00\x00\x00\xcd\x00\x00\x00\xcd\x00\xcd\xcd\x00\x00\x00\xee\xcd\x00\xcd\x00\xcd\xcd\xe5\xe5\xe5" +
			"\x7f\x7f\x7f\xff\x00\x00\x00\xff\x00\xff\xff\x00\x5c\x5c\xff\xff\x00\xff\x00\xff\xff\xff\xff\xff"
		return ansi[3*i], ansi[3*i+1], ansi[3*i+2]
	case i < 232:
		const levels = "\x00\x5f\x87\xaf\xd7\xff"
		i -= 16
		return levels[i/36], levels[i/6%6], levels[i%6]
	}
	v := uint8(8 + 10*(i-232))
	return v, v, v
}

// A lab is a colour in Oklab (Björn Ottosson, "A perceptual color space for
// image processing", 2020): l is lightness from 0 (black) to 1 (white), a
// runs from green to red and b from blue to yellow. Equal distances in it
// look like roughly equal differences, which RGB does not offer. The package
// uses it for two things: finding the nearest colour of a smaller palette,
// and blending the stops of a gradient.
type lab struct{ l, a, b float64 }

// toLab converts an sRGB colour to Oklab.
func toLab(r, g, b uint8) lab {
	lr, lg, lb := srgbToLinear(r), srgbToLinear(g), srgbToLinear(b)
	l := math.Cbrt(0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb)
	m := math.Cbrt(0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb)
	s := math.Cbrt(0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb)
	return lab{
		l: 0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		a: 1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		b: 0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// rgb converts the colour back to sRGB, clipping what a screen cannot show.
func (c lab) rgb() (r, g, b uint8) {
	l := c.l + 0.3963377774*c.a + 0.2158037573*c.b
	m := c.l - 0.1055613458*c.a - 0.0638541728*c.b
	s := c.l - 0.0894841775*c.a - 1.2914855480*c.b
	l, m, s = l*l*l, m*m*m, s*s*s
	return linearToSRGB(4.0767416621*l - 3.3077115913*m + 0.2309699292*s),
		linearToSRGB(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		linearToSRGB(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s)
}

// blend returns the colour a fraction t of the way from c to d along the
// straight line between them.
func (c lab) blend(d lab, t float64) lab {
	return lab{c.l + (d.l-c.l)*t, c.a + (d.a-c.a)*t, c.b + (d.b-c.b)*t}
}

// srgbToLinear undoes the sRGB transfer curve for one channel.
func srgbToLinear(c uint8) float64 {
	v := float64(c) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// linearToSRGB applies the sRGB transfer curve to one channel and rounds it
// to eight bits.
func linearToSRGB(v float64) uint8 {
	if v <= 0.0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - 0.055
	}
	return uint8(math.Round(255 * min(max(v, 0), 1)))
}

// nearestInPalette returns the number of the palette colour, among those
// numbered from first up to but not including end, that is nearest to c:
// the one at the smallest straight-line distance in Oklab. Of two equally
// near colours the lower-numbered wins.
//
// The palette's Oklab coordinates come ready-made from the generated table,
// as whole numbers, so the search is whole-number arithmetic and gives the
// same answer on every machine.
func nearestInPalette(c lab, first, end int) int {
	l := int64(math.Round(c.l * labScale))
	a := int64(math.Round(c.a * labScale))
	b := int64(math.Round(c.b * labScale))
	best, bestDistance := first, int64(math.MaxInt64)
	for i := first; i < end; i++ {
		dl := l - paletteCoordinate(6*i)
		da := a - paletteCoordinate(6*i+2)
		db := b - paletteCoordinate(6*i+4)
		if d := dl*dl + da*da + db*db; d < bestDistance {
			best, bestDistance = i, d
		}
	}
	return best
}

// paletteCoordinate reads the two-byte signed number at offset i of
// paletteLab.
func paletteCoordinate(i int) int64 {
	return int64(int16(uint16(paletteLab[i])<<8 | uint16(paletteLab[i+1])))
}

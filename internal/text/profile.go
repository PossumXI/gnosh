package text

import "strings"

// A Profile says how much colour the output can take. The zero value is
// NoColor, so a profile that was never set writes plain text.
type Profile uint8

const (
	// NoColor is for output that must stay plain: a file, a pipe, a dumb
	// terminal, or a person who asked for no colour. Nothing is written
	// but the text itself: no colours, no bold, no hyperlinks.
	NoColor Profile = iota
	// ANSI16 is the sixteen ANSI colours.
	ANSI16
	// ANSI256 is the 256-colour xterm palette.
	ANSI256
	// TrueColor is 24-bit RGB.
	TrueColor
)

// String names the profile: "none", "16", "256" or "truecolor".
func (p Profile) String() string {
	switch p {
	case ANSI16:
		return "16"
	case ANSI256:
		return "256"
	case TrueColor:
		return "truecolor"
	}
	return "none"
}

// Convert returns the colour that stands in for c under the profile.
//
// TrueColor keeps every colour. ANSI256 turns an RGB colour into the nearest
// of palette colours 16 to 255 (the cube and the greys; the first sixteen
// are left out because a theme may have changed them). ANSI16 turns an RGB
// colour or a palette colour into the nearest of the sixteen ANSI colours,
// taking those to have xterm's default values. NoColor turns everything into
// Default.
//
// "Nearest" is the smallest straight-line distance in Oklab, a colour space
// built so that distance matches how different two colours look. An RGB
// colour goes to the nearest ANSI colour directly, not by way of the
// 256-colour palette.
func (p Profile) Convert(c Color) Color {
	c = c.valid()
	switch {
	case p >= TrueColor:
		return c
	case p == ANSI256:
		if c.kind() == colorRGB {
			r, g, b, _ := c.rgb()
			return Indexed(uint8(nearestInPalette(toLab(r, g, b), 16, 256)))
		}
		return c
	case p == ANSI16:
		switch c.kind() {
		case colorIndexed:
			if n := int(c & 0xFF); n < 16 {
				return ansiColor(n)
			}
			fallthrough
		case colorRGB:
			r, g, b, _ := c.rgb()
			return ansiColor(nearestInPalette(toLab(r, g, b), 0, 16))
		}
		return c
	}
	return Default
}

// DetectProfile works out the colour profile from the environment. lookup
// reports the value of an environment variable and whether it is set, as
// os.LookupEnv does (a nil lookup is an empty environment); isTerminal says
// whether the output goes to a terminal.
//
// The rules, in order. The first that applies decides.
//
//  1. NO_COLOR set to anything but the empty string: NoColor
//     (https://no-color.org).
//  2. FORCE_COLOR set to "0" or "false": NoColor.
//  3. FORCE_COLOR set to anything else but the empty string, or
//     CLICOLOR_FORCE set to anything but the empty string and "0": colour
//     is forced. The profile is the one the terminal's own variables give
//     (rule 7), but at least ANSI16, at least ANSI256 for FORCE_COLOR=2 and
//     TrueColor for FORCE_COLOR=3, and it holds even when the output is not
//     a terminal.
//  4. The output is not a terminal: NoColor.
//  5. CLICOLOR set to "0": NoColor (https://bixense.com/clicolors).
//  6. TERM set to "dumb": NoColor.
//  7. Otherwise the terminal's own variables decide. COLORTERM set to
//     "truecolor" or "24bit" means TrueColor. So does a TERM that ends in
//     "-direct", or that names a terminal known to take 24-bit colour
//     whose COLORTERM is often lost on the way through ssh: xterm-kitty,
//     xterm-ghostty, alacritty, wezterm, foot, contour. A TERM that
//     contains "256color" means ANSI256. Any other TERM that is set, or
//     any other COLORTERM that is set, means ANSI16. With neither set
//     there is nothing to say the terminal takes colour: NoColor.
//
// Names of terminals and of COLORTERM values are compared without regard to
// case.
func DetectProfile(lookup func(name string) (value string, ok bool), isTerminal bool) Profile {
	get := func(name string) string {
		if lookup == nil {
			return ""
		}
		value, _ := lookup(name)
		return value
	}
	if get("NO_COLOR") != "" {
		return NoColor
	}
	fromTerminal := terminalProfile(strings.ToLower(get("COLORTERM")), strings.ToLower(get("TERM")))

	forced, floor := false, ANSI16
	switch force := strings.ToLower(get("FORCE_COLOR")); force {
	case "":
	case "0", "false":
		return NoColor
	case "2":
		forced, floor = true, ANSI256
	case "3":
		forced, floor = true, TrueColor
	default:
		forced = true
	}
	if force := get("CLICOLOR_FORCE"); force != "" && force != "0" {
		forced = true
	}
	if forced {
		return max(fromTerminal, floor)
	}

	if !isTerminal || get("CLICOLOR") == "0" || strings.EqualFold(get("TERM"), "dumb") {
		return NoColor
	}
	return fromTerminal
}

// terminalProfile is rule 7 of DetectProfile: what COLORTERM and TERM, both
// in lower case, say the terminal can show.
func terminalProfile(colorterm, term string) Profile {
	if colorterm == "truecolor" || colorterm == "24bit" {
		return TrueColor
	}
	switch term {
	case "xterm-kitty", "xterm-ghostty", "alacritty", "wezterm", "foot", "contour":
		return TrueColor
	}
	if term == "dumb" {
		// Only reached when colour is forced: the name says nothing.
		term = ""
	}
	switch {
	case strings.HasSuffix(term, "-direct"):
		return TrueColor
	case strings.Contains(term, "256color"):
		return ANSI256
	case term != "" || colorterm != "":
		return ANSI16
	}
	return NoColor
}

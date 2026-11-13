package diff

import "github.com/gnosh-ai/gnosh/internal/text"

// A Theme is the look of a rendered diff: one style for each kind of thing
// Render writes. The styles need no colour profile; Render sets the one it
// is given. A style left at its zero value writes plain text.
type Theme struct {
	// Path is the file's name in the header.
	Path text.Style
	// Note is the words about the change as a whole: "new file", what
	// happened to the line endings, how many lines lie between two hunks,
	// what was left out.
	Note text.Style
	// Number is the line numbers and the marks in the gutter.
	Number text.Style
	// Context is the unchanged lines.
	Context text.Style
	// Deleted is the lines that went, their minus sign, and the count of
	// them in the header. Inserted is the same for the lines that came.
	Deleted, Inserted text.Style
	// DeletedSpan and InsertedSpan are the words inside a deleted or an
	// inserted line that changed. They should stand out from Deleted and
	// Inserted.
	DeletedSpan, InsertedSpan text.Style
	// Symbol is the stand-ins for what cannot be shown as it is: a control
	// character, an invisible character, a space at the end of a line.
	Symbol text.Style
}

// DefaultTheme returns gnosh's own look for a diff, in the colours that suit
// the profile p.
//
// It is made of foreground colours only. Deleted lines are a rose, inserted
// lines a green; unchanged lines keep the terminal's own colour, so the eye
// goes to what changed; line numbers and notes are grey. The words that
// changed inside a line are underlined in the line's colour: a crisp mark
// that also shows on spaces, and the one emphasis that works the same on a
// dark and on a light background. Nothing is drawn as a filled block. A
// faint background behind the changed words is a matter of taste that
// depends on the terminal's background colour, which this package cannot
// know; a caller that knows it can add one to DeletedSpan and InsertedSpan.
//
// Without colour nothing is lost but the emphasis: the signs, the numbers
// and the notes say everything the colours do.
//
// With 24-bit colour, and with 256 colours where each is written as the
// nearest the palette has, the three colours are middle tones, about as far
// in brightness from a black background as from a white one, so that each
// reads on both: a rose (gnosh's hot pink taken towards red) for what goes,
// a green (gnosh's mint taken darker) for what comes, and a grey for
// everything that is not the text itself.
//
// With sixteen colours the palette is the terminal's own, and its red and
// green are the ones its owner reads every day: those are used as they are.
// (The nearest of the sixteen to the middle green would be a grey.) There is
// no safe grey among the sixteen, since some palettes make their dark grey
// the background colour; numbers and notes are written faint instead, which
// a terminal that has no faint shows as ordinary text.
func DefaultTheme(p text.Profile) Theme {
	rose := text.Style{}.Fg(text.RGB(0xD6, 0x3A, 0x70))
	green := text.Style{}.Fg(text.RGB(0x18, 0x90, 0x5C))
	grey := text.Style{}.Fg(text.RGB(0x7A, 0x7A, 0x7A))
	if p == text.ANSI16 {
		rose = text.Style{}.Fg(text.Red)
		green = text.Style{}.Fg(text.Green)
		grey = text.Style{}.Dim()
	}
	return Theme{
		Path:         text.Style{}.Bold(),
		Note:         grey,
		Number:       grey,
		Deleted:      rose,
		Inserted:     green,
		DeletedSpan:  rose.Underline(),
		InsertedSpan: green.Underline(),
		Symbol:       grey,
	}
}

// in returns the theme with every style set to write for the profile p.
func (t Theme) in(p text.Profile) Theme {
	return Theme{
		Path:         t.Path.In(p),
		Note:         t.Note.In(p),
		Number:       t.Number.In(p),
		Context:      t.Context.In(p),
		Deleted:      t.Deleted.In(p),
		Inserted:     t.Inserted.In(p),
		DeletedSpan:  t.DeletedSpan.In(p),
		InsertedSpan: t.InsertedSpan.In(p),
		Symbol:       t.Symbol.In(p),
	}
}

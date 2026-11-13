package menu

import (
	"strings"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
)

// View is what a question looks like at one width: its rows, styled and
// cut to fit, and where the cursor is (CursorRow is -1 when it is hidden).
// It is the editor's view, and what the renderer's live region takes.
type View = editor.View

// Result is what an event did to a question.
type Result struct {
	// Handled is false when the event meant nothing to the question, so
	// the application may give the key a meaning of its own.
	Handled bool
	// Done is true when the question is over: it was answered, or it
	// was cancelled. A question that is over takes no more events and
	// draws nothing.
	Done bool
	// Cancelled is true when the question ended without an answer (Esc
	// or Ctrl+C). It is never set together with a choice.
	Cancelled bool
	// Index is the option chosen in a Select, counted from 0. It means
	// something only when Done is true and Cancelled is false; in every
	// other result of a Select it is -1.
	Index int
	// Text is what was typed: the note that goes with the choice of a
	// Select, the answer to an Input, or the answer to a Secret. A
	// secret is in the result that has Done set and in no other.
	Text string
	// Answers are the answers to the questions of a Form, one for each
	// question in order.
	Answers []Answer
}

// Yes reports whether the result is a yes to a question made with
// NewConfirm: it was answered, and the answer was the first option.
func (r Result) Yes() bool { return r.Done && !r.Cancelled && r.Index == 0 }

// Prompt is what every question in this package is: something that takes
// the terminal's events one at a time and can be drawn at any width.
type Prompt interface {
	// Handle takes one event and says what it did.
	Handle(ev term.Event) Result
	// Render draws the question for a terminal width cells wide.
	Render(width int) View
}

// Theme is the look of the questions: the marks they draw and the style
// of each kind of thing in them. The option chosen is always marked with
// Pointer, a ticked option with Checked, the tab in front with brackets:
// nothing is said by colour alone, so the questions read the same under
// text.NoColor.
type Theme struct {
	// Pointer marks the option the cursor keys are on.
	Pointer string
	// MoreAbove and MoreBelow mark, beside the first or last row shown,
	// that there are rows hidden beyond it.
	MoreAbove, MoreBelow string
	// Checked and Unchecked stand before the options of a question that
	// takes several answers; Picked and Unpicked before those of one
	// that takes a single answer, in a Form.
	Checked, Unchecked, Picked, Unpicked string
	// Answered marks the tab of a question that has an answer.
	Answered string
	// Mask is drawn for each character of a secret.
	Mask string

	// Title is the look of the question itself.
	Title text.Style
	// Label and Selected are the looks of an option and of the one the
	// cursor keys are on. PointerStyle is the look of the pointer.
	Label, Selected, PointerStyle text.Style
	// Description is the look of the line that explains an option, and
	// of the number and the key that pick it.
	Description text.Style
	// Hint is the look of the line that names the keys.
	Hint text.Style
	// More is the look of the marks for hidden rows.
	More text.Style
	// Tab and TabCurrent are the looks of the tabs of a Form.
	Tab, TabCurrent text.Style
	// Missing is the look of what stands for an answer not given.
	Missing text.Style
	// Field is the look of the text fields. Its Prompt stands before the
	// first row of a field.
	Field editor.Theme
}

// DefaultTheme is gnosh's look for questions, written for the colour
// profile p. Under text.NoColor it is marks alone.
func DefaultTheme(p text.Profile) Theme {
	// The colours of the editor's default look.
	var (
		purple = text.RGB(0xA8, 0x78, 0xFF)
		pink   = text.RGB(0xFF, 0x4F, 0xB6)
		gold   = text.RGB(0xFB, 0xD2, 0x37)
	)
	plain := text.Style{}.In(p)
	field := editor.DefaultTheme(p)
	field.Prompt, field.PromptStyle = "| ", plain.Fg(pink)
	return Theme{
		Pointer:   ">",
		MoreAbove: "↑",
		MoreBelow: "↓",
		Checked:   "[x]",
		Unchecked: "[ ]",
		Picked:    "(*)",
		Unpicked:  "( )",
		Answered:  "✓",
		Mask:      "*",

		Title:        plain.Bold(),
		Label:        plain,
		Selected:     plain.Fg(pink).Bold(),
		PointerStyle: plain.Fg(pink).Bold(),
		Description:  plain.Dim(),
		Hint:         plain.Dim(),
		More:         plain.Fg(purple),
		Tab:          plain.Dim(),
		TabCurrent:   plain.Fg(pink).Bold(),
		Missing:      plain.Fg(gold),
		Field:        field,
	}
}

// What the application passes in is drawn, and some of it comes from
// further away than the application: the model writes the questions of a
// Form, and a command to be approved may quote a stranger's file. None of
// it may reach the terminal as a control sequence. So everything passed in
// is made safe once, when the question is made:
//
//   - oneLine is for what stands on one row (a label, a description, a
//     header): every control character becomes its stand-in, line feeds
//     included.
//   - lines is for what may have several lines (a title, a question): the
//     same, but its line feeds stay.
//   - styled is for the body, which the caller has already styled: colours,
//     styles and hyperlinks stay, with line feeds and tabs, and every other
//     control character and escape sequence becomes its stand-in.

func oneLine(s string) string { return editor.Visible(s) }

func lines(s string) string {
	if !strings.Contains(s, "\n") {
		return editor.Visible(s)
	}
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		parts[i] = editor.Visible(p)
	}
	return strings.Join(parts, "\n")
}

func styled(s string) string {
	var b strings.Builder
	for tok := range text.Tokens(s) {
		switch {
		case tok.IsSGR() || tok.IsHyperlink():
			b.WriteString(tok.Text)
		case tok.Kind != text.Plain:
			b.WriteString(editor.Visible(tok.Text))
		default:
			for _, r := range tok.Text {
				if r == '\n' || r == '\t' {
					b.WriteRune(r)
				} else {
					b.WriteString(editor.Visible(string(r)))
				}
			}
		}
	}
	return b.String()
}

// styledAll is styled for every line of a body.
func styledAll(body []string) []string {
	out := make([]string, len(body))
	for i, l := range body {
		out[i] = styled(l)
	}
	return out
}

// safeOptions is the options with their words made safe to draw.
func safeOptions(opts []Option) []Option {
	out := make([]Option, len(opts))
	for i, o := range opts {
		o.Label, o.Description = oneLine(o.Label), oneLine(o.Description)
		out[i] = o
	}
	return out
}

// defaultMaxHeight is the most rows a question takes unless told
// otherwise: it lives in the few rows at the bottom of the terminal that
// are redrawn in place.
const defaultMaxHeight = 20

// frame is what every question shares: its look, how wide characters are
// counted, and how many rows it may take.
type frame struct {
	th        Theme
	m         text.Metrics
	maxHeight int
	done      bool
}

func newFrame(th Theme, m text.Metrics, maxHeight int) frame {
	if th == (Theme{}) {
		th = DefaultTheme(text.NoColor)
	}
	if maxHeight < 1 {
		maxHeight = defaultMaxHeight
	}
	return frame{th: th, m: m, maxHeight: maxHeight}
}

// SetMaxHeight sets the most rows the question takes. What does not fit
// scrolls. Zero or less means 20.
func (f *frame) SetMaxHeight(rows int) {
	if rows < 1 {
		rows = defaultMaxHeight
	}
	f.maxHeight = rows
}

// field makes a text field: the editor, as a plain field with the
// question's look.
func (f *frame) field(placeholder string) *editor.Editor {
	return editor.New(editor.Options{
		Theme:       f.th.Field,
		Plain:       true,
		Placeholder: placeholder,
		MaxHeight:   4,
		Metrics:     f.m,
	})
}

// gutter is how many cells are kept at the left of every row for the
// pointer or the mark for hidden rows, and the space after it.
func (f *frame) gutter() int {
	return max(f.m.Width(f.th.Pointer), f.m.Width(f.th.MoreAbove), f.m.Width(f.th.MoreBelow), 1) + 1
}

// wrap breaks styled text into rows of at most width cells. Tabs are
// expanded first, and a row that text.Wrap could not make fit (a wide
// character in a single cell) is cut.
func (f *frame) wrap(s string, width int, first, rest string) []string {
	width = max(width, 1)
	lines := f.m.Wrap(f.m.ExpandTabs(s, 8, 0), width, text.WrapOptions{First: first, Rest: rest})
	for i, l := range lines {
		if f.m.VisibleWidth(l) > width {
			lines[i] = f.m.Truncate(l, width, "")
		}
	}
	return lines
}

// block is a run of rows that scrolls as one: the body of a question, or
// its options.
type block struct {
	rows []brow
}

// brow is one row of a block.
type brow struct {
	text string
	// pointer is true on the first row of the option the cursor keys
	// are on.
	pointer bool
	// cursor is the cell on the row where the terminal's cursor goes,
	// counted from the end of the gutter, or -1.
	cursor int
}

func (b *block) add(text string) { b.rows = append(b.rows, brow{text: text, cursor: -1}) }

// show draws the rows of the block from top, h of them at most, each
// after a gutter. The gutter holds the pointer, on the row that has it;
// on the first and last rows shown it holds the mark that says rows are
// hidden beyond, unless the pointer is there. It returns the rows and,
// when one of them holds the cursor, which and at what cell.
func (f *frame) show(b *block, top, h int) (lines []string, curRow, curCol int) {
	gw := f.gutter()
	curRow = -1
	end := min(top+h, len(b.rows))
	for i := top; i < end; i++ {
		r := b.rows[i]
		mark, look := "", f.th.More
		switch {
		case r.pointer:
			mark, look = f.th.Pointer, f.th.PointerStyle
		case i == top && top > 0:
			mark = f.th.MoreAbove
		case i == end-1 && end < len(b.rows):
			mark = f.th.MoreBelow
		}
		lines = append(lines, look.Render(mark)+strings.Repeat(" ", gw-f.m.Width(mark))+r.text)
		if r.cursor >= 0 {
			curRow, curCol = i-top, gw+r.cursor
		}
	}
	return lines, curRow, curCol
}

// scrollTo returns where a block of n rows shown h at a time should
// start, given where it started, so that rows from through to are in
// view; when they do not all fit, from wins. Where there is room it
// keeps one more row in view on each side, so that the row with the
// pointer is not the first or last shown while rows are hidden beyond
// it: the mark for hidden rows goes where the pointer would be.
func scrollTo(top, n, h, from, to int) int {
	if h >= n {
		return 0
	}
	if h >= to-from+3 {
		from, to = max(from-1, 0), min(to+1, n-1)
	}
	if to >= top+h {
		top = to - h + 1
	}
	if from < top {
		top = from
	}
	return min(max(top, 0), n-h)
}

// hint joins as many of parts, from the first, as fit in width cells: the
// line that names the keys, most useful first.
func (f *frame) hint(parts []string, width int) string {
	const sep = " · "
	out := ""
	for i, p := range parts {
		next := p
		if i > 0 {
			next = out + sep + p
		}
		if f.m.Width(next) > width {
			break
		}
		out = next
	}
	if out == "" {
		return ""
	}
	return f.th.Hint.Render(out)
}

// fit cuts every row to width cells, as a last guard: nothing a question
// draws may be wider than the terminal.
func (f *frame) fit(v View, width int) View {
	for i, l := range v.Lines {
		if f.m.VisibleWidth(l) > width {
			v.Lines[i] = f.m.Truncate(l, width, "")
		}
	}
	if v.CursorRow >= len(v.Lines) {
		v.CursorRow = -1
	}
	if v.CursorRow >= 0 {
		v.CursorCol = min(max(v.CursorCol, 0), width-1)
	}
	return v
}

// The keys every question knows.
type keyKind int

const (
	keyOther keyKind = iota
	keyEnter
	keyCancel // Esc
	keyBreak  // Ctrl+C
	keyUp
	keyDown
	keyLeft
	keyRight
	keyTab
	keyBackTab
	keyHome
	keyEnd
	keyPageUp
	keyPageDown
	keyLineUp   // Shift+Up: scroll what is shown above the options
	keyLineDown // Shift+Down
	keySpace
	keyBackspace
	keyClear // Ctrl+U
	keyRune  // a character typed
)

// kindOf says which of the keys every question knows k is.
func kindOf(k term.Key) keyKind {
	const ctrl, shift = term.ModCtrl, term.ModShift
	mod := k.Mod &^ (term.ModCapsLock | term.ModNumLock)
	switch k.Code {
	case term.KeyEnter:
		if mod == 0 {
			return keyEnter
		}
	case term.KeyEscape:
		if mod == 0 {
			return keyCancel
		}
	case term.KeyUp:
		switch mod {
		case 0:
			return keyUp
		case shift:
			return keyLineUp
		}
	case term.KeyDown:
		switch mod {
		case 0:
			return keyDown
		case shift:
			return keyLineDown
		}
	case term.KeyLeft:
		if mod == 0 {
			return keyLeft
		}
	case term.KeyRight:
		if mod == 0 {
			return keyRight
		}
	case term.KeyTab:
		switch mod {
		case 0:
			return keyTab
		case shift:
			return keyBackTab
		}
	case term.KeyHome:
		if mod == 0 {
			return keyHome
		}
	case term.KeyEnd:
		if mod == 0 {
			return keyEnd
		}
	case term.KeyPageUp:
		if mod == 0 {
			return keyPageUp
		}
	case term.KeyPageDown:
		if mod == 0 {
			return keyPageDown
		}
	case term.KeyBackspace:
		if mod == 0 || mod == shift {
			return keyBackspace
		}
	case term.KeyRune:
		if k.Text != "" && mod&^shift == 0 {
			if k.Text == " " {
				return keySpace
			}
			return keyRune
		}
		if mod == ctrl {
			switch k.Rune {
			case 'c':
				return keyBreak
			case 'm', 'j':
				return keyEnter
			case 'i':
				return keyTab
			case 'p':
				return keyUp
			case 'n':
				return keyDown
			case 'h':
				return keyBackspace
			case 'u':
				return keyClear
			}
		}
	}
	return keyOther
}

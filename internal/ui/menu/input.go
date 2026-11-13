package menu

import (
	"strings"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
)

// InputOptions are what a question answered in the person's own words is
// made from.
type InputOptions struct {
	// Title is the question, and Body what it is about, as in a Select.
	Title string
	Body  []string
	// Placeholder is shown, dimmed, while nothing is typed.
	Placeholder string
	// Initial is text put in the field to begin with.
	Initial string
	// AllowEmpty lets Enter answer with nothing typed. Without it Enter
	// on an empty field does nothing.
	AllowEmpty bool
	// Theme, MaxHeight and Metrics are as in a Select.
	Theme     Theme
	MaxHeight int
	Metrics   text.Metrics
}

// Input asks a question that is answered by typing. The field is the
// editor's: the same keys move and delete in it, a paste goes in whole,
// and a line too long for the terminal wraps, so the field grows.
type Input struct {
	frame
	title      string
	body       []string
	allowEmpty bool
	f          *editor.Editor
}

// NewInput makes a question answered by typing.
func NewInput(o InputOptions) *Input {
	in := &Input{
		frame:      newFrame(o.Theme, o.Metrics, o.MaxHeight),
		title:      lines(o.Title),
		body:       styledAll(o.Body),
		allowEmpty: o.AllowEmpty,
	}
	in.f = in.field(o.Placeholder)
	if o.Initial != "" {
		in.f.SetText(o.Initial)
	}
	return in
}

// Handle takes one event from the terminal. Enter answers; Esc cancels;
// Ctrl+C empties the field, and cancels when it is empty already.
func (in *Input) Handle(ev term.Event) Result {
	if in.done {
		return unhandled()
	}
	k, isKey := ev.(term.Key)
	if isKey {
		switch kindOf(k) {
		case keyCancel:
			in.done = true
			return Result{Handled: true, Done: true, Cancelled: true, Index: -1}
		case keyBreak:
			if !in.f.IsEmpty() {
				in.f.Reset()
				return handled()
			}
			in.done = true
			return Result{Handled: true, Done: true, Cancelled: true, Index: -1}
		}
	}
	res := in.f.Handle(ev)
	switch {
	case res.Action == editor.ActionSubmit:
		in.done = true
		return Result{Handled: true, Done: true, Index: -1, Text: res.Text}
	case res.Handled:
		// The field declined to hand over nothing; that is an answer
		// when an empty one is allowed.
		if isKey && kindOf(k) == keyEnter && !res.Changed && in.allowEmpty && strings.TrimSpace(in.f.Text()) == "" {
			in.done = true
			return Result{Handled: true, Done: true, Index: -1}
		}
		return handled()
	}
	return unhandled()
}

// Render draws the question: the title, the body, the field and a dim
// line that names the keys.
func (in *Input) Render(width int) View {
	v := View{CursorRow: -1}
	if in.done || width < 1 {
		return v
	}
	inner := max(width-in.gutter(), 1)
	v.Lines = in.head(in.title, in.body, width, inner, in.maxHeight-2)
	fv := in.f.Render(inner)
	var b block
	for i, l := range fv.Lines {
		row := brow{text: l, cursor: -1}
		if i == fv.CursorRow {
			row.cursor = fv.CursorCol
		}
		b.rows = append(b.rows, row)
	}
	lines, row, col := in.show(&b, 0, len(b.rows))
	if row >= 0 {
		v.CursorRow, v.CursorCol = len(v.Lines)+row, col
	}
	v.Lines = append(v.Lines, lines...)
	if keys := in.hint([]string{"Enter answer", "Esc cancel", "Shift+Enter new line", "Ctrl+C empty"}, width); keys != "" {
		v.Lines = append(v.Lines, keys)
	}
	return in.fit(v, width)
}

// head draws what stands above the answer of a question that is typed:
// its title, and as much of its body as fits in room rows, with a mark
// when some of the body is left out.
func (f *frame) head(title string, body []string, width, inner, room int) []string {
	var lines []string
	if title != "" {
		lines = f.wrap(f.th.Title.Render(title), width, "", "")
	}
	var b block
	for _, l := range body {
		for _, row := range f.wrap(l, inner, "", "") {
			b.add(row)
		}
	}
	shown, _, _ := f.show(&b, 0, max(room-len(lines), 1))
	return append(lines, shown...)
}

// SecretOptions are what a question with a secret answer is made from.
type SecretOptions struct {
	// Title is the question, and Body what it is about: for a key's
	// password, which key, the exact command that will be signed, and
	// the chain.
	Title string
	Body  []string
	// Label stands before the hidden answer: "Password" when empty.
	Label string
	// Theme, MaxHeight and Metrics are as in a Select.
	Theme     Theme
	MaxHeight int
	Metrics   text.Metrics
}

// Secret asks for something that must not be seen: a key's password.
//
// What is typed is never drawn: the row shows one mark for each character
// and nothing else. It is in no Result but the one that ends the question
// with an answer, and in nothing the question draws. There is no history,
// no undo, no completion and no way to move back into what was typed:
// only typing, Backspace and Ctrl+U. Nothing is trimmed, and an empty
// answer is not accepted.
//
// The answer is held in one buffer, which is overwritten with zeros when
// the question ends, whichever way it ends. Go's strings cannot be
// erased, so two things are beyond this type's reach: the events that
// carried the keys, and the answer itself once it has been handed over.
// Both are let go of at once and are the garbage collector's from then
// on.
type Secret struct {
	frame
	title string
	body  []string
	label string
	// buf is what was typed, and ends is where each character of it
	// (each grapheme cluster) ends.
	buf  []byte
	ends []int
	// empty is true after Enter was pressed with nothing typed, until
	// the next key.
	empty bool
}

// NewSecret makes a question with a secret answer.
func NewSecret(o SecretOptions) *Secret {
	s := &Secret{
		frame: newFrame(o.Theme, o.Metrics, o.MaxHeight),
		title: lines(o.Title),
		body:  styledAll(o.Body),
		label: oneLine(o.Label),
	}
	if s.label == "" {
		s.label = "Password"
	}
	return s
}

// String names the type and nothing else, so that a Secret printed by
// mistake shows none of what it holds.
func (s *Secret) String() string { return "menu.Secret" }

// GoString is String, for the same reason.
func (s *Secret) GoString() string { return "menu.Secret" }

// Handle takes one event from the terminal. Enter answers with what was
// typed, when something was; Esc and Ctrl+C cancel.
func (s *Secret) Handle(ev term.Event) Result {
	if s.done {
		return unhandled()
	}
	wasEmpty := s.empty
	s.empty = false
	switch ev := ev.(type) {
	case term.Paste:
		// A pasted password is its first line: the line break that
		// came with it is not part of it.
		t := ev.Text
		if i := strings.IndexByte(t, '\n'); i >= 0 {
			t = t[:i]
		}
		s.put(t)
		return handled()
	case term.Key:
		switch kindOf(ev) {
		case keyRune, keySpace:
			s.put(ev.Text)
		case keyBackspace:
			if n := len(s.ends); n > 0 {
				start := 0
				if n > 1 {
					start = s.ends[n-2]
				}
				clear(s.buf[start:])
				s.buf, s.ends = s.buf[:start], s.ends[:n-1]
			}
		case keyClear:
			s.wipe()
		case keyEnter:
			if len(s.buf) == 0 {
				s.empty = true
				return handled()
			}
			res := Result{Handled: true, Done: true, Index: -1, Text: string(s.buf)}
			s.wipe()
			s.done = true
			return res
		case keyCancel, keyBreak:
			s.wipe()
			s.done = true
			return Result{Handled: true, Done: true, Cancelled: true, Index: -1}
		default:
			s.empty = wasEmpty
			return unhandled()
		}
		return handled()
	}
	s.empty = wasEmpty
	return unhandled()
}

// put adds typed text to the answer.
func (s *Secret) put(t string) {
	if t == "" {
		return
	}
	if len(s.buf)+len(t) > cap(s.buf) {
		// Grow by hand, so that the old buffer can be wiped: append
		// would leave a copy of the answer behind.
		bigger := make([]byte, len(s.buf), max(2*cap(s.buf), len(s.buf)+len(t), 128))
		copy(bigger, s.buf)
		clear(s.buf[:cap(s.buf)])
		s.buf = bigger
	}
	// What was just typed may join the character before it (an accent
	// typed after its letter), so the last character is measured again
	// together with the new text.
	start := 0
	if n := len(s.ends); n > 0 {
		s.ends = s.ends[:n-1]
		if n > 1 {
			start = s.ends[n-2]
		}
	}
	s.buf = append(s.buf, t...)
	for rest := string(s.buf[start:]); rest != ""; {
		cluster, _, after := s.m.NextGrapheme(rest)
		start += len(cluster)
		s.ends = append(s.ends, start)
		rest = after
	}
}

// wipe overwrites the answer with zeros and forgets it.
func (s *Secret) wipe() {
	clear(s.buf[:cap(s.buf)])
	s.buf, s.ends = s.buf[:0], s.ends[:0]
}

// Render draws the question: the title, the body, the row with one mark
// for each character typed, and a dim line that names the keys.
func (s *Secret) Render(width int) View {
	v := View{CursorRow: -1}
	if s.done || width < 1 {
		return v
	}
	inner := max(width-s.gutter(), 1)
	v.Lines = s.head(s.title, s.body, width, inner, s.maxHeight-2)

	label := s.label + ": "
	if s.m.Width(label) > inner/2 {
		label = ""
	}
	// One mark for each character, the last of them when there are more
	// than fit, and a cell kept for the cursor.
	mark := s.th.Mask
	markW := max(s.m.Width(mark), 1)
	if s.m.Width(mark) == 0 {
		mark = "*"
	}
	shown := min(len(s.ends), max(inner-s.m.Width(label)-1, 0)/markW)
	row := s.th.Label.Render(label) + strings.Repeat(mark, shown)
	var b block
	b.rows = append(b.rows, brow{text: row, pointer: true, cursor: s.m.Width(label) + shown*markW})
	lines, r, c := s.show(&b, 0, 1)
	v.CursorRow, v.CursorCol = len(v.Lines)+r, c
	v.Lines = append(v.Lines, lines...)

	parts := []string{"Enter confirm", "Esc cancel", "what you type is not shown"}
	if s.empty {
		parts = []string{"Nothing typed yet", "Esc cancel"}
	}
	if keys := s.hint(parts, width); keys != "" {
		v.Lines = append(v.Lines, keys)
	}
	return s.fit(v, width)
}

package editor

import (
	"strings"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/term"
)

// command is what a key means in the box. The key table in doc.go lists
// them with their keys and where each came from.
type command int

const (
	cmdNone command = iota
	cmdInsert
	cmdSubmit
	cmdSubmitNow
	cmdNewline
	cmdLeft
	cmdRight
	cmdWordLeft
	cmdWordRight
	cmdUp
	cmdDown
	cmdPageUp
	cmdPageDown
	cmdLineStart
	cmdLineEnd
	cmdTextStart
	cmdTextEnd
	cmdBackspace
	cmdDelete
	cmdKillWordLeft
	cmdKillSpaceLeft
	cmdKillWordRight
	cmdKillLineEnd
	cmdKillLineStart
	cmdYank
	cmdYankPop
	cmdUndo
	cmdRedo
	cmdSearch
	cmdStash
	cmdEditor
	cmdTab
	cmdEscape
	cmdInterrupt
	cmdEOF
)

// commandFor is the key table: what each key means in the box.
//
// Under the kitty keyboard protocol Ctrl+H, Ctrl+I and Ctrl+M arrive as
// themselves; they are given the meanings they have in every other
// terminal, where they cannot be told from Backspace, Tab and Enter.
func commandFor(k term.Key) command {
	const ctrl, alt, shift = term.ModCtrl, term.ModAlt, term.ModShift
	mod := k.Mod &^ (term.ModCapsLock | term.ModNumLock)
	switch k.Code {
	case term.KeyEnter:
		switch {
		case mod == 0:
			return cmdSubmit
		case mod&^shift == ctrl:
			return cmdSubmitNow
		case mod&^(shift|alt) == 0:
			return cmdNewline
		}
	case term.KeyTab:
		if mod == 0 {
			return cmdTab
		}
	case term.KeyBackspace:
		switch mod {
		case 0, shift:
			return cmdBackspace
		case alt, ctrl:
			return cmdKillWordLeft
		}
	case term.KeyDelete:
		switch mod {
		case 0:
			return cmdDelete
		case alt, ctrl:
			return cmdKillWordRight
		}
	case term.KeyEscape:
		if mod == 0 {
			return cmdEscape
		}
	case term.KeyLeft:
		switch mod {
		case 0:
			return cmdLeft
		case alt, ctrl:
			return cmdWordLeft
		}
	case term.KeyRight:
		switch mod {
		case 0:
			return cmdRight
		case alt, ctrl:
			return cmdWordRight
		}
	case term.KeyUp:
		if mod == 0 {
			return cmdUp
		}
	case term.KeyDown:
		if mod == 0 {
			return cmdDown
		}
	case term.KeyPageUp:
		if mod == 0 {
			return cmdPageUp
		}
	case term.KeyPageDown:
		if mod == 0 {
			return cmdPageDown
		}
	case term.KeyHome:
		switch mod {
		case 0:
			return cmdLineStart
		case ctrl:
			return cmdTextStart
		}
	case term.KeyEnd:
		switch mod {
		case 0:
			return cmdLineEnd
		case ctrl:
			return cmdTextEnd
		}
	case term.KeyRune:
		if k.Text != "" && mod&^shift == 0 {
			return cmdInsert
		}
		switch mod {
		case ctrl:
			switch k.Rune {
			case 'a':
				return cmdLineStart
			case 'e':
				return cmdLineEnd
			case 'f':
				return cmdRight
			case 'p':
				return cmdUp
			case 'n':
				return cmdDown
			case 'h':
				return cmdBackspace
			case 'd':
				return cmdEOF
			case 'k':
				return cmdKillLineEnd
			case 'u':
				return cmdKillLineStart
			case 'w':
				return cmdKillSpaceLeft
			case 'y':
				return cmdYank
			case '_', '/':
				return cmdUndo
			case 'j':
				return cmdNewline
			case 'm':
				return cmdSubmit
			case 'i':
				return cmdTab
			case 'r':
				return cmdSearch
			case 's':
				return cmdStash
			case 'g':
				return cmdEditor
			case 'c':
				return cmdInterrupt
			}
		case ctrl | shift:
			// Ctrl+_ as a keyboard that needs Shift for "_" reports it
			// under the kitty protocol.
			if k.Rune == '-' || k.Rune == '_' {
				return cmdUndo
			}
		case alt:
			switch k.Rune {
			case 'b':
				return cmdWordLeft
			case 'f':
				return cmdWordRight
			case 'd':
				return cmdKillWordRight
			case 'y':
				return cmdYankPop
			case '/':
				return cmdRedo
			case '<':
				return cmdTextStart
			case '>':
				return cmdTextEnd
			}
		case alt | shift:
			switch k.Rune {
			case ',', '<':
				return cmdTextStart
			case '.', '>':
				return cmdTextEnd
			}
		}
	}
	return cmdNone
}

// key handles one key press.
func (e *Editor) key(k term.Key) Result {
	cmd := commandFor(k)
	armed := e.armed
	e.armed = press{}
	if e.srch != nil {
		return e.searchKey(k, cmd)
	}
	if e.comp != nil {
		if res, done := e.menuKey(cmd); done {
			return res
		}
	}
	if e.vim.mode != VimOff && !e.plain {
		if res, done := e.vimKey(k, cmd, armed); done {
			return res
		}
	}
	return e.run(k, cmd, armed)
}

// run carries out a command in the box itself. armed is the key that was
// waiting for its second press when this one came.
func (e *Editor) run(k term.Key, cmd command, armed press) Result {
	handled := Result{Handled: true}
	switch cmd {
	case cmdNone:
		return Result{}

	case cmdInsert:
		t := clean(k.Text)
		if !e.plain && e.IsEmpty() && t == "?" {
			return Result{Handled: true, Action: ActionHelp}
		}
		e.edit(e.cur, e.cur, t, kindType)
		e.typed = true

	case cmdNewline:
		e.edit(e.cur, e.cur, "\n", kindOther)
		e.typed = true

	case cmdSubmit, cmdSubmitNow:
		if cmd == cmdSubmit && e.afterBackslash() {
			// A backslash and then Enter is a new line, for terminals
			// that cannot tell Shift+Enter from Enter. A shell line
			// keeps the backslash, which is how a shell is told that a
			// command goes on.
			from := pos{e.cur.line, e.cur.col - 1}
			if e.Mode() == ModeShell {
				from = e.cur
			}
			e.edit(from, e.cur, "\n", kindOther)
			e.typed = true
			return handled
		}
		return e.submit(cmd == cmdSubmitNow)

	case cmdLeft:
		e.cur = e.leftOf(e.cur)
	case cmdRight:
		e.cur = e.rightOf(e.cur)
	case cmdWordLeft:
		e.cur = e.wordLeft(e.cur)
	case cmdWordRight:
		e.cur = e.wordRight(e.cur)
	case cmdLineStart:
		e.cur.col = e.home(e.cur.line)
	case cmdLineEnd:
		e.cur.col = len(e.lines[e.cur.line].s)
	case cmdTextStart:
		e.cur = e.start()
	case cmdTextEnd:
		e.cur = e.end()

	case cmdUp:
		if e.vertical(-1) {
			return handled
		}
		if e.plain {
			return Result{}
		}
		if e.queued && e.IsEmpty() {
			return Result{Handled: true, Action: ActionQueue}
		}
		e.older()
	case cmdDown:
		if e.vertical(1) {
			return handled
		}
		if e.plain {
			return Result{}
		}
		e.newer()
	case cmdPageUp:
		for range e.maxHeight {
			if !e.vertical(-1) {
				break
			}
		}
	case cmdPageDown:
		for range e.maxHeight {
			if !e.vertical(1) {
				break
			}
		}

	case cmdBackspace:
		e.deleteBack(e.leftOf(e.cur), kindBack, false)
		e.typed = true
	case cmdDelete:
		e.edit(e.cur, e.rightOf(e.cur), "", kindDel)
		e.typed = true
	case cmdKillWordLeft:
		e.deleteBack(e.wordLeft(e.cur), kindOther, true)
	case cmdKillSpaceLeft:
		e.deleteBack(e.spaceLeft(e.cur), kindOther, true)
	case cmdKillWordRight:
		e.kill(e.cur, e.wordRight(e.cur), true)
	case cmdKillLineEnd:
		to := pos{e.cur.line, len(e.lines[e.cur.line].s)}
		if to == e.cur && e.cur.line+1 < len(e.lines) {
			// At the end of a line it is the line break that goes.
			to = pos{e.cur.line + 1, 0}
		}
		e.kill(e.cur, to, true)
	case cmdKillLineStart:
		from := pos{e.cur.line, e.home(e.cur.line)}
		if from == e.cur && e.cur.line > 0 {
			// At the start of a line it is the line break and the line
			// before it that go, so that repeating the key clears
			// upwards a line at a time.
			from = pos{e.cur.line - 1, e.home(e.cur.line - 1)}
		}
		e.deleteBack(from, kindOther, true)

	case cmdYank:
		e.yank()
	case cmdYankPop:
		e.yankPop()
	case cmdUndo:
		e.undoOne()
	case cmdRedo:
		e.redoOne()

	case cmdTab:
		switch {
		case e.ExpandPaste():
		case e.complete != nil:
			e.refresh(true, false)
			e.skipRefresh = true
		}

	case cmdSearch:
		if e.plain {
			return Result{}
		}
		e.openSearch()
	case cmdStash:
		if e.plain {
			return Result{}
		}
		return e.stash()
	case cmdEditor:
		if e.plain {
			return Result{}
		}
		return Result{Handled: true, Action: ActionEditor}

	case cmdEscape:
		switch {
		case e.plain:
			return Result{}
		case e.busy:
			return Result{Handled: true, Action: ActionInterrupt}
		case e.bare():
			// Nothing but the character that sets the mode: leave the
			// mode.
			e.edit(pos{}, pos{0, 1}, "", kindOther)
		case e.IsEmpty():
			if e.again(armed, cmdEscape) {
				return Result{Handled: true, Action: ActionRewind}
			}
			e.arm(cmdEscape)
		case e.again(armed, cmdEscape):
			e.clear()
			return Result{Handled: true, Cleared: true}
		default:
			e.arm(cmdEscape)
			return Result{Handled: true, Hint: HintEscClear}
		}

	case cmdInterrupt:
		switch {
		case e.plain:
			return Result{}
		case e.busy:
			return Result{Handled: true, Action: ActionInterrupt}
		case e.again(armed, cmdInterrupt):
			return Result{Handled: true, Action: ActionExit}
		}
		res := Result{Handled: true, Hint: HintCtrlCExit}
		if !e.IsEmpty() {
			e.clear()
			res.Cleared = true
		}
		e.arm(cmdInterrupt)
		return res

	case cmdEOF:
		switch {
		case !e.IsEmpty():
			// With text in the box Ctrl+D is Delete.
			e.edit(e.cur, e.rightOf(e.cur), "", kindDel)
			e.typed = true
		case e.plain:
			return Result{}
		case e.again(armed, cmdEOF):
			return Result{Handled: true, Action: ActionExit}
		default:
			e.arm(cmdEOF)
			return Result{Handled: true, Hint: HintCtrlDExit}
		}
	}
	return handled
}

// afterBackslash reports whether the character just before the cursor is
// a backslash.
func (e *Editor) afterBackslash() bool {
	return e.cur.col > e.home(e.cur.line) && e.lines[e.cur.line].s[e.cur.col-1] == '\\'
}

// bare reports whether the box holds nothing but the character that sets
// its mode.
func (e *Editor) bare() bool {
	return e.Mode() != ModePrompt && len(e.lines) == 1 && len(e.lines[0].s) == 1
}

// blank reports whether there is nothing in the box worth sending: only
// spaces and line breaks, after the "!" of a shell line.
func (e *Editor) blank() bool {
	for i := range e.lines {
		s := e.lines[i].s
		if i == 0 && e.Mode() == ModeShell {
			s = s[1:]
		}
		if strings.TrimSpace(plain(s)) != "" {
			return false
		}
	}
	return true
}

// submit hands the text over and empties the box. A box with nothing
// worth sending stays as it is.
func (e *Editor) submit(now bool) Result {
	if e.blank() {
		return Result{Handled: true}
	}
	d := e.Draft()
	res := Result{Handled: true, Changed: true, Action: ActionSubmit, Text: d.Text, Pastes: d.Pastes, Mode: e.Mode(), Now: now}
	e.Reset()
	return res
}

// leftOf returns the place one item before p, or p at the start of the
// text.
func (e *Editor) leftOf(p pos) pos {
	if floor := e.home(p.line); p.col > floor {
		return pos{p.line, e.itemStart(e.lines[p.line].s, p.col, floor)}
	}
	if p.line > 0 {
		return pos{p.line - 1, len(e.lines[p.line-1].s)}
	}
	return p
}

// rightOf returns the place one item after p, or p at the end of the
// text.
func (e *Editor) rightOf(p pos) pos {
	if s := e.lines[p.line].s; p.col < len(s) {
		return pos{p.line, e.itemEnd(s, p.col)}
	}
	if p.line+1 < len(e.lines) {
		return pos{p.line + 1, 0}
	}
	return p
}

// wordRight returns the place after the next word: a run of letters and
// digits, or one collapsed paste. Everything else, line breaks included,
// separates words.
func (e *Editor) wordRight(p pos) pos {
	inWord := false
	for {
		s := e.lines[p.line].s
		if p.col >= len(s) {
			if inWord || p.line+1 >= len(e.lines) {
				return p
			}
			p = pos{p.line + 1, 0}
			continue
		}
		end := e.itemEnd(s, p.col)
		switch classOf(s[p.col:end]) {
		case classWord:
			inWord = true
		case classPaste:
			if inWord {
				return p
			}
			return pos{p.line, end}
		default:
			if inWord {
				return p
			}
		}
		p.col = end
	}
}

// wordLeft returns the place where the word before p starts.
func (e *Editor) wordLeft(p pos) pos {
	inWord := false
	for {
		floor := e.home(p.line)
		if p.col <= floor {
			if inWord || p.line == 0 {
				return p
			}
			p = pos{p.line - 1, len(e.lines[p.line-1].s)}
			continue
		}
		s := e.lines[p.line].s
		start := e.itemStart(s, p.col, floor)
		switch classOf(s[start:p.col]) {
		case classWord:
			inWord = true
		case classPaste:
			if inWord {
				return p
			}
			return pos{p.line, start}
		default:
			if inWord {
				return p
			}
		}
		p.col = start
	}
}

// spaceLeft returns the place where the run of characters before p that
// are not spaces starts, after passing over any spaces just before p. It
// is how far Ctrl+W deletes: a whole path or --flag=value at once. A line
// break counts as a space.
func (e *Editor) spaceLeft(p pos) pos {
	inWord := false
	for {
		floor := e.home(p.line)
		if p.col <= floor {
			if inWord || p.line == 0 {
				return p
			}
			p = pos{p.line - 1, len(e.lines[p.line-1].s)}
			continue
		}
		s := e.lines[p.line].s
		start := e.itemStart(s, p.col, floor)
		if classOf(s[start:p.col]) == classSpace {
			if inWord {
				return p
			}
		} else {
			inWord = true
		}
		p.col = start
	}
}

// vertical moves the cursor one row up (-1) or down (1), keeping to the
// cell it started from. It reports false, and moves nothing, when the
// cursor is already on the first or last row.
func (e *Editor) vertical(dir int) bool {
	l := e.laid(e.cur.line)
	ri := l.rowOf(e.cur.col)
	li := e.cur.line
	switch {
	case dir < 0 && ri > 0:
		ri--
	case dir < 0 && li > 0:
		li--
		ri = len(e.laid(li).rows) - 1
	case dir > 0 && ri+1 < len(l.rows):
		ri++
	case dir > 0 && li+1 < len(e.lines):
		li++
		ri = 0
	default:
		return false
	}
	if e.want < 0 {
		e.want = e.cursorCell()
	}
	e.keepWant = true
	t := e.laid(li)
	e.cur = pos{li, e.offsetAt(t, t.rows[ri], ri == len(t.rows)-1, e.want)}
	return true
}

// deleteBack deletes from the place from up to the cursor. In front of
// the character that sets the mode, the only thing a backward delete can
// take is that character, which leaves the mode. ring says whether what
// is deleted is kept for Ctrl+Y.
func (e *Editor) deleteBack(from pos, kind editKind, ring bool) {
	if from == e.cur && e.cur == e.start() && e.home(0) == 1 {
		e.edit(pos{}, e.cur, "", kindOther)
		return
	}
	if ring {
		e.kill(from, e.cur, false)
		return
	}
	e.edit(from, e.cur, "", kind)
}

// kill deletes the text from a up to b and keeps it for Ctrl+Y. Deletions
// made one after another are kept as one piece, in the order they stood
// in the text: forward says the deleted text was after the cursor.
func (e *Editor) kill(a, b pos, forward bool) {
	text := e.between(a, b)
	if text == "" {
		return
	}
	e.edit(a, b, "", kindOther)
	e.killing = true
	switch n := len(e.ring); {
	case e.wasKill && n > 0 && forward:
		e.ring[n-1] += text
	case e.wasKill && n > 0:
		e.ring[n-1] = text + e.ring[n-1]
	default:
		e.ring = append(e.ring, text)
		if len(e.ring) > killRingSize {
			e.ring = e.ring[1:]
		}
	}
}

// yank puts the text deleted last back in at the cursor.
func (e *Editor) yank() {
	if len(e.ring) == 0 {
		return
	}
	e.yankIdx = len(e.ring) - 1
	e.yankAt = e.cur
	e.edit(e.cur, e.cur, e.ring[e.yankIdx], kindOther)
	// Where the text ends, which is not always where the cursor is: see
	// settle.
	e.yankEnd = advance(e.yankAt, e.ring[e.yankIdx])
	e.yanking = true
}

// yankPop, straight after Ctrl+Y or another Alt+Y, swaps the text just
// put back for the deletion before it, going round.
func (e *Editor) yankPop() {
	if !e.wasYank || len(e.ring) < 2 {
		e.yanking = e.wasYank
		return
	}
	e.yankIdx = (e.yankIdx + len(e.ring) - 1) % len(e.ring)
	e.edit(e.yankAt, e.yankEnd, e.ring[e.yankIdx], kindOther)
	e.yankEnd = advance(e.yankAt, e.ring[e.yankIdx])
	e.yanking = true
}

// paste takes a bracketed paste: the text goes in as it is, line breaks
// and all, as one step of the undo history, and never sends anything. A
// large paste goes in collapsed.
func (e *Editor) paste(p term.Paste) Result {
	res := Result{Handled: true, Truncated: p.Truncated}
	t := clean(p.Text)
	if e.srch != nil {
		e.searchType(t)
		return res
	}
	if t == "" {
		return res
	}
	if e.collapses(t) {
		t = encodePaste(t)
	}
	e.edit(e.cur, e.cur, t, kindOther)
	e.typed = true
	return res
}

// collapses reports whether a paste is large enough to be shown as one
// placeholder.
func (e *Editor) collapses(t string) bool {
	if e.pasteLines > 0 && countLines(t) > e.pasteLines {
		return true
	}
	return e.pasteChars > 0 && utf8.RuneCountInString(t) > e.pasteChars
}

// ExpandPaste turns the collapsed paste the cursor is touching (the one
// just before it, else the one just after) back into its text, with the
// cursor at its end. It reports whether there was one. Undo collapses it
// again.
func (e *Editor) ExpandPaste() bool {
	s := e.lines[e.cur.line].s
	from, to := -1, -1
	switch {
	case strings.HasSuffix(s[:e.cur.col], pasteClose):
		from, to = strings.LastIndex(s[:e.cur.col], pasteOpen), e.cur.col
	case strings.HasPrefix(s[e.cur.col:], pasteOpen):
		from, to = e.cur.col, e.itemEnd(s, e.cur.col)
	}
	if from < 0 {
		return false
	}
	e.edit(pos{e.cur.line, from}, pos{e.cur.line, to}, pasteContent(s[from:to]), kindOther)
	return true
}

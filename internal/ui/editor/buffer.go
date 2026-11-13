package editor

import (
	"slices"
	"strings"
)

// pos is a place in the text: a line, and a byte offset in that line that
// is the start of an item (or the line's end).
type pos struct{ line, col int }

// before reports whether p comes before q in the text.
func (p pos) before(q pos) bool {
	return p.line < q.line || p.line == q.line && p.col < q.col
}

// editKind says what sort of change an edit is, for deciding which edits
// one press of undo takes back together.
type editKind uint8

const (
	kindNone  editKind = iota // no edit
	kindOther                 // an edit that stands alone
	kindType                  // typing: joins the typing just before it
	kindBack                  // Backspace: joins the Backspace just before it
	kindDel                   // Delete: joins the Delete just before it
)

// change is one step of the undo history: at the place at, the text del
// was replaced by the text ins. Both are text as the editor keeps it, so
// a collapsed paste comes back collapsed.
type change struct {
	at       pos
	del, ins string
	cb, ca   pos // the cursor before and after
	wb, wa   int // the history entry shown before and after (see history.go)
	kind     editKind
}

// start is the first place in the text the cursor can be.
func (e *Editor) start() pos { return pos{0, e.home(0)} }

// end is the place after the last item of the text.
func (e *Editor) end() pos {
	n := len(e.lines) - 1
	return pos{n, len(e.lines[n].s)}
}

// home is the first offset of line i at which the cursor can be: 1 on the
// first line when its first character sets the mode, otherwise 0.
func (e *Editor) home(i int) int {
	if i == 0 && e.Mode() != ModePrompt {
		return 1
	}
	return 0
}

// advance returns the place where text ends when it is put in at at.
func advance(at pos, text string) pos {
	n := strings.Count(text, "\n")
	if n == 0 {
		return pos{at.line, at.col + len(text)}
	}
	return pos{at.line + n, len(text) - strings.LastIndexByte(text, '\n') - 1}
}

// between returns the text from a up to b, as the editor keeps it.
func (e *Editor) between(a, b pos) string {
	if a.line == b.line {
		return e.lines[a.line].s[a.col:b.col]
	}
	var sb strings.Builder
	sb.WriteString(e.lines[a.line].s[a.col:])
	for i := a.line + 1; i < b.line; i++ {
		sb.WriteByte('\n')
		sb.WriteString(e.lines[i].s)
	}
	sb.WriteByte('\n')
	sb.WriteString(e.lines[b.line].s[:b.col])
	return sb.String()
}

// whole returns all of the text, as the editor keeps it.
func (e *Editor) whole() string {
	if len(e.lines) == 1 {
		return e.lines[0].s
	}
	return e.between(pos{}, e.end())
}

// splice replaces the text from a up to b with ins and returns the place
// where ins ends. It is the one function that changes the text; it keeps
// no record of what it did. The lines it touches are made anew, so what
// was worked out for drawing the old ones goes with them.
func (e *Editor) splice(a, b pos, ins string) pos {
	head := e.lines[a.line].s[:a.col]
	tail := e.lines[b.line].s[b.col:]
	end := advance(a, ins)
	if a.line == b.line && strings.IndexByte(ins, '\n') < 0 {
		fresh := line{s: head + ins + tail}
		// What is known about the line's collapsed pastes still holds
		// for those the edit did not touch: the ones after it have
		// moved, that is all. So typing beside a large paste does not
		// measure the paste again at every key.
		if old := e.lines[a.line].pastes; len(old) > 0 {
			moved := len(ins) - (b.col - a.col)
			fresh.pastes = make(map[int]pasteInfo, len(old))
			for off, p := range old {
				switch {
				case p.end <= a.col:
					fresh.pastes[off] = p
				case off >= b.col:
					fresh.pastes[off+moved] = pasteInfo{end: p.end + moved, label: p.label}
				}
			}
		}
		e.lines[a.line] = fresh
	} else {
		parts := strings.Split(ins, "\n")
		fresh := make([]line, len(parts))
		for i, p := range parts {
			fresh[i].s = p
		}
		fresh[0].s = head + fresh[0].s
		fresh[len(fresh)-1].s += tail
		e.lines = slices.Replace(e.lines, a.line, b.line+1, fresh...)
	}
	e.changed = true
	return end
}

// edit replaces the text from a up to b with ins, leaves the cursor after
// ins, and records the change so that undo can take it back. It reports
// whether there was anything to change.
func (e *Editor) edit(a, b pos, ins string, kind editKind) bool {
	del := e.between(a, b)
	if del == "" && ins == "" {
		return false
	}
	cb := e.cur
	e.cur = e.splice(a, b, ins)
	e.settle()
	e.fix()
	e.redo = nil
	e.thisKind = kind
	if n := len(e.undo); n > 0 && kind == e.lastKind && e.undo[n-1].kind == kind {
		u := &e.undo[n-1]
		switch {
		case kind == kindType && del == "" && a == advance(u.at, u.ins) && joins(u.ins, ins):
			u.ins += ins
			u.ca = e.cur
			return true
		case kind == kindBack && ins == "" && b == u.at:
			u.del = del + u.del
			u.at, u.ca = a, e.cur
			return true
		case kind == kindDel && ins == "" && a == u.at:
			u.del += del
			return true
		}
	}
	e.undo = append(e.undo, change{at: a, del: del, ins: ins, cb: cb, ca: e.cur, wb: e.walk, wa: e.walk, kind: kind})
	if extra := len(e.undo) - e.undoLimit; extra > 0 {
		e.undo = slices.Delete(e.undo, 0, extra)
	}
	return true
}

// settle moves the cursor to the end of the item it is in, when an edit
// has left it inside one. Text put in front of a combining mark joins
// with the mark, and text taken out from between two characters can let
// them join: either way the place where the edit ended is no longer
// between two items.
func (e *Editor) settle() {
	s := e.lines[e.cur.line].s
	col, home := e.cur.col, e.home(e.cur.line)
	if col <= home || col >= len(s) {
		return
	}
	for start := safeStart(s, col, home); start < col; {
		next := e.itemEnd(s, start)
		if next > col {
			e.cur.col = next
			return
		}
		start = next
	}
}

// joins reports whether typing more after typed belongs to the same undo
// step: it does until a word and the spaces after it are finished, and
// never across a line break.
func joins(typed, more string) bool {
	if strings.IndexByte(typed, '\n') >= 0 || strings.IndexByte(more, '\n') >= 0 {
		return false
	}
	last := typed[len(typed)-1]
	return !(last == ' ' || last == '\t') || more[0] == ' ' || more[0] == '\t'
}

// replaceAll puts text in place of everything in the box as one step of
// the undo history, and makes walk the history entry being shown.
func (e *Editor) replaceAll(text string, walk int) {
	if e.edit(pos{}, e.end(), text, kindOther) {
		// An edit that stands alone is always the newest step.
		e.undo[len(e.undo)-1].wa = walk
	}
	e.walk = walk
}

// undoOne takes back the last change. It reports whether there was one.
func (e *Editor) undoOne() bool {
	n := len(e.undo)
	if n == 0 {
		return false
	}
	u := e.undo[n-1]
	e.undo = e.undo[:n-1]
	e.splice(u.at, advance(u.at, u.ins), u.del)
	e.cur, e.walk = u.cb, e.slot(u.wb)
	e.fix()
	e.redo = append(e.redo, u)
	return true
}

// redoOne makes again the change that undo last took back. It reports
// whether there was one.
func (e *Editor) redoOne() bool {
	n := len(e.redo)
	if n == 0 {
		return false
	}
	r := e.redo[n-1]
	e.redo = e.redo[:n-1]
	e.splice(r.at, advance(r.at, r.del), r.ins)
	e.cur, e.walk = r.ca, e.slot(r.wa)
	e.fix()
	e.undo = append(e.undo, r)
	return true
}

// slot makes a remembered history index safe to use: one that no longer
// names an entry becomes the draft.
func (e *Editor) slot(w int) int {
	if w >= len(e.hist) || w < -1 {
		return -1
	}
	return w
}

// fix puts the cursor somewhere it is allowed to be: inside the text, and
// never in front of the character that sets the mode.
func (e *Editor) fix() {
	e.cur.line = min(max(e.cur.line, 0), len(e.lines)-1)
	e.cur.col = min(max(e.cur.col, e.home(e.cur.line)), len(e.lines[e.cur.line].s))
}

// setLines replaces the text without recording anything, and puts the
// cursor at its end. It is for showing a history entry while searching.
func (e *Editor) setLines(text string) {
	parts := strings.Split(text, "\n")
	e.lines = make([]line, len(parts))
	for i, p := range parts {
		e.lines[i].s = p
	}
	e.cur = e.end()
	e.top = 0
}

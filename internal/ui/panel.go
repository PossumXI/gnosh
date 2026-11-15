package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/diff"
	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
)

// The side panel (Jae, 15 November 2026: "i want a real side panel as an
// option"): a way of drawing the session on the whole screen, with a
// panel at the right that shows the files the session changed and the
// deliverables with their addresses. It never comes on by itself: /panel
// turns it on and off, and the sidePanel option starts the session with
// it. While it is on, the conversation is drawn at the left
// (term.Panel), the wheel and PageUp/PageDown scroll back through it, and
// the terminal selects text with a modifier key held (Option in Apple's
// Terminal and iTerm2, Shift in most others). Turning it off gives the
// terminal back as it was, and what was said meanwhile is written into
// the scrollback, where it would have been.

// panelWords are the lines that turn the panel on and off.
var panelWords = map[string]bool{"/panel": true, "/sidebar": true}

// panelFiles and panelDeliverables are the most of each the panel lists,
// the newest first.
const (
	panelFiles        = 12
	panelDeliverables = 8
)

// changedFile is a file the session's calls changed, with what the last
// call did to it and what all of them did in all.
type changedFile struct {
	path           string
	added, removed int
}

// noteChange takes in a call that changed a file: an edit or a write that
// went well, whose result shows the change.
func (s *session) noteChange(ev *agent.ToolEndEvent) {
	d := ev.Result.Display
	if ev.Result.IsError || d == nil || d.Kind != "diff" || d.Path == "" {
		return
	}
	st := diff.Compare(d.Before, d.After, diff.Options{}).Stats
	for i, f := range s.changed {
		if f.path == d.Path {
			f.added += st.Added
			f.removed += st.Removed
			// The newest change first.
			s.changed = append(append([]changedFile{f}, s.changed[:i]...), s.changed[i+1:]...)
			return
		}
	}
	s.changed = append([]changedFile{{path: d.Path, added: st.Added, removed: st.Removed}}, s.changed...)
}

// togglePanel turns the panel on, or off when it is on.
func (s *session) togglePanel() {
	if s.panel != nil {
		s.closePanel()
		s.flash("The side panel is off; what was said meanwhile is in the scrollback.")
		return
	}
	if s.openPanel() {
		s.flash("The side panel is on: the wheel and PageUp scroll back; hold Option (Shift in most terminals) to select text. /panel turns it off.")
	}
}

// openPanel takes the screen for the panel, and reports whether it could.
func (s *session) openPanel() bool {
	if s.panel != nil {
		return true
	}
	cols, rows, ok := s.host.size()
	if !ok {
		cols, rows = s.termCols, s.rows
	}
	if !term.PanelFits(cols) {
		s.flash(fmt.Sprintf("The side panel needs a terminal at least %d columns wide, and this one has %d.", term.PanelMinCols, cols))
		return false
	}
	p, err := s.host.openPanel(cols, rows)
	if err != nil {
		s.notice(agent.LevelWarning, "The side panel could not be opened: "+err.Error())
		return false
	}
	s.panel, s.r = p, p
	s.termCols, s.rows = cols, rows
	left, _ := p.Size()
	s.relayout(left)
	s.dirty = true
	return true
}

// closePanel gives the screen back and writes what was committed while the
// panel was on into the scrollback.
func (s *session) closePanel() {
	p := s.panel
	if p == nil {
		return
	}
	since, err := p.Close()
	s.keep(err)
	s.panel, s.r = nil, s.inline
	if cols, rows, ok := s.host.size(); ok {
		s.termCols, s.rows = cols, rows
	}
	s.keep(s.inline.Resize(s.termCols, s.rows))
	if len(since) > 0 {
		s.keep(s.inline.Commit(since...))
	}
	s.relayout(s.termCols)
	s.dirty = true
}

// scrollPanel scrolls the panel's view rows rows back (forward when rows
// is negative).
func (s *session) scrollPanel(rows int) {
	if s.panel != nil {
		s.panel.Scroll(rows)
	}
}

// panelLines are what the panel shows, from its top: the files the
// session changed, then the deliverables, each with its address and its
// file. width is the room for text.
func (s *session) panelLines(width int) []string {
	th := &s.th
	var out []string
	out = append(out, th.ToolName.Render("Files changed"))
	if len(s.changed) == 0 {
		out = append(out, th.Quiet.Render("  none yet"))
	}
	for i, f := range s.changed {
		if i == panelFiles {
			out = append(out, th.Quiet.Render(fmt.Sprintf("  and %d more", len(s.changed)-panelFiles)))
			break
		}
		counts := th.DeliverFile.Render(fmt.Sprintf("+%d", f.added)) + " " + th.Fail.Render(fmt.Sprintf("-%d", f.removed))
		name := showPath(s.cwd, s.home, f.path)
		room := width - 2 - s.m.Width(fmt.Sprintf("+%d -%d", f.added, f.removed)) - 1
		out = append(out, "  "+cutStart(name, room, s.m.Width)+" "+counts)
	}
	out = append(out, th.ToolName.Render("Deliverables"))
	if len(s.deliv) == 0 {
		out = append(out, th.Quiet.Render("  none yet"))
	}
	shown := 0
	for i := len(s.deliv) - 1; i >= 0; i-- {
		if shown == panelDeliverables {
			out = append(out, th.Quiet.Render(fmt.Sprintf("  and %d more: /deliverables lists them", len(s.deliv)-panelDeliverables)))
			break
		}
		shown++
		out = append(out, s.panelDeliverable(s.deliv[i], width)...)
	}
	return out
}

// panelDeliverable is one deliverable in the panel: its mark and label,
// then its address, whole, over as many rows as it takes, each row a link
// to the whole address; then its file.
func (s *session) panelDeliverable(d app.Deliverable, width int) []string {
	th := &s.th
	mark := th.DeliverFile.Render(th.DeliverMark)
	switch {
	case !d.Delivered():
		mark = th.Quiet.Render(th.PendingMark)
	case d.Link():
		mark = th.DeliverLink.Render(th.DeliverMark)
	}
	head := "  " + mark + " " + th.DeliverLabel.Render(oneLine(d.Label))
	if d.Pending {
		head += th.Quiet.Render(th.Sep + "pending")
	}
	out := []string{head}
	room := max(width-4, 8)
	if d.URL != "" {
		shown := editor.Visible(d.URL)
		for _, chunk := range chunks(shown, room) {
			text := th.Link.Render(chunk)
			if strings.HasPrefix(d.URL, "http://") || strings.HasPrefix(d.URL, "https://") {
				text = th.Profile.Hyperlink(d.URL, text)
			}
			out = append(out, "    "+text)
		}
	}
	if d.Path != "" {
		where := showPath("", s.home, d.Path)
		if d.Host != "" {
			where = editor.Visible(d.Path) + " on " + oneLine(d.Host)
		}
		out = append(out, "    "+th.Quiet.Render(cutStart(where, room, s.m.Width)))
	}
	return out
}

// chunks cuts text into pieces of at most n characters.
func chunks(text string, n int) []string {
	var out []string
	for text != "" {
		if utf8.RuneCountInString(text) <= n {
			return append(out, text)
		}
		i, count := 0, 0
		for i < len(text) && count < n {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
			count++
		}
		out = append(out, text[:i])
		text = text[i:]
	}
	return out
}

// cutStart keeps the end of a path that is too long for room cells, with
// "…" in front: the file's own name is what matters.
func cutStart(text string, room int, width func(string) int) string {
	if room < 2 || width(text) <= room {
		return text
	}
	for i := range text {
		if i > 0 && width(text[i:])+1 <= room {
			return "…" + text[i:]
		}
	}
	return "…"
}

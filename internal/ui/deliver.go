package ui

import (
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
)

// How a deliverable is shown.
//
// A deliverable is the thing the person asked for: the model names it with
// its Deliver tool, first by a label alone, which claims it, and then with
// the file or the address, which delivers it. The interface writes one
// entry into the scrollback each time, in place of the call's own block:
//
//	✧ demo reel · pending
//
//	✦ hebrew ad video · delivered
//	  http://127.0.0.1:7777/<token>/ad.mp4
//	  /root/renders/ad.mp4 on render
//
// The label is in one plain colour. The mark before it is the one touch
// of colour: green for a file, gold for an address, and a hollow grey one
// for what is not there yet. The address and the path are written whole
// and never cut: on one row with the label when all of it fits, and
// otherwise each on a row of its own, so that a triple click takes
// exactly the address. An address is also a real link (OSC 8) wherever
// the terminal knows links, and with no colour it is the text alone.
//
// The status line counts what is delivered and what is pending, and the
// command /deliverables, which is also /artifacts, lists the latest
// version of each.

// deliverer is what a session's controls may also be: able to give the
// session's deliverables as they stand. The app's own controls are
// (app.SessionControls); what stands in for them in a test need not be.
type deliverer interface {
	Deliverables() []app.Deliverable
}

// deliverWords are the lines that list the deliverables.
var deliverWords = map[string]bool{"/deliverables": true, "/artifacts": true}

// readDeliverables takes the session's deliverables as they stand: when
// the interface starts, and whenever another session may have come to
// stand behind the controls.
func (s *session) readDeliverables() {
	s.deliv = nil
	if d, ok := s.c.(deliverer); ok {
		s.deliv = d.Deliverables()
	}
	s.dirty = true
}

// listsDeliverables reports whether the list is the interface's own to
// draw: the session has the command, and can give the list.
func (s *session) listsDeliverables() bool {
	_, can := s.c.(deliverer)
	return can && slices.Contains(s.info.Commands, "deliverables")
}

// link draws an address: underlined, whole, and as a real link where the
// terminal shows links. The address is outside text: what is drawn of it
// is made safe, and what the terminal is told to open is written by the
// text package, which lets nothing through that could end the sequence.
func (s *session) link(address string) string {
	shown := s.th.Link.Render(editor.Visible(address))
	if strings.HasPrefix(address, "http://") || strings.HasPrefix(address, "https://") {
		return s.th.Profile.Hyperlink(address, shown)
	}
	return shown
}

// deliverableRows draws one deliverable for the scrollback: its mark, its
// label, and words after the label that say where it stands, when there
// are any; then the address and the file. Everything stands on one row
// when it fits there, and otherwise the address and the file each get a
// row of their own under the label. Nothing is cut.
func (s *session) deliverableRows(d app.Deliverable, words string) []string {
	th := &s.th
	mark := th.DeliverFile.Render(th.DeliverMark)
	switch {
	case !d.Delivered():
		mark = th.Quiet.Render(th.PendingMark)
	case d.Link():
		mark = th.DeliverLink.Render(th.DeliverMark)
	}
	head := mark + " " + th.DeliverLabel.Render(oneLine(d.Label))
	if words != "" {
		head += th.Quiet.Render(th.Sep + words)
	}
	var targets []string
	if d.URL != "" {
		targets = append(targets, s.link(d.URL))
	}
	switch {
	case d.Path == "":
	case d.Host != "":
		// A path on another machine is shown as it was named there.
		targets = append(targets, editor.Visible(d.Path)+th.Quiet.Render(" on ")+oneLine(d.Host))
	default:
		targets = append(targets, showPath("", s.home, d.Path))
	}
	if len(targets) == 0 {
		return wrap(s.m, head, s.cols, "", "  ")
	}
	sep := th.Quiet.Render(th.Sep)
	if one := head + sep + strings.Join(targets, sep); s.m.VisibleWidth(one) <= s.cols {
		return []string{one}
	}
	rows := wrap(s.m, head, s.cols, "", "  ")
	for _, t := range targets {
		rows = append(rows, wrap(s.m, t, s.cols, "  ", "  ")...)
	}
	return rows
}

// listDeliverables answers /deliverables: the latest version of each
// thing the agent produced, with its address and its file, the pending
// ones marked, and how an address that answers on this machine only is
// reached from another.
func (s *session) listDeliverables(line string) {
	th := &s.th
	s.readDeliverables()
	s.begin(blkPrompt)
	s.put(wrap(s.m, th.PromptText.Render(oneLine(line)), s.cols, th.Prompt.Render(th.PromptMark), strings.Repeat(" ", s.m.Width(th.PromptMark)))...)
	s.ed.AddHistory(editor.Draft{Text: line})
	if len(s.deliv) == 0 {
		for _, l := range app.DeliverablesText(nil) {
			s.put(wrap(s.m, l, s.cols, "", "")...)
		}
		return
	}
	s.put(wrap(s.m, th.Quiet.Render("Deliverables, the latest version of each: "+app.DeliverableCount(s.deliv, th.Sep)), s.cols, "", "")...)
	for _, d := range s.deliv {
		words := ""
		if d.Pending {
			words = d.State(false)
		}
		s.put(s.deliverableRows(d, words)...)
	}
	if hint := app.DeliverablesHint(s.deliv); hint != "" {
		s.put(wrap(s.m, th.Quiet.Render(hint), s.cols, "", "")...)
	}
}

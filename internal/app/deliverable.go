package app

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// What the agent produced for the person: the list a front end shows.
//
// gnosh often runs on a machine the person is not sitting at, and they do
// not follow the tool calls. So the model names the thing they asked for
// (a rendered video, a report, a page) with a tool of its own, Deliver:
// first by its label alone, before the work, which claims it; and again
// with the file or the address, when it exists, which delivers it. A
// feature keeps the list, the latest version of each label, and hands it
// to the session (Session.Deliverables). This file is the vocabulary the
// feature and the front ends share: the entry, the event that says the
// list changed, and the plain words for both.

// Deliverable is one thing the agent produced for the person, or has said
// it will produce: the latest version of it.
type Deliverable struct {
	// Label is the short name the model gave it. It is the model's text,
	// and a front end makes it safe before it draws it.
	Label string `json:"label"`
	// Pending says the model has claimed it and has not delivered it
	// since. With nothing else set it does not exist yet; with a Path or
	// a URL, that version was delivered before and a newer one is on its
	// way.
	Pending bool `json:"pending,omitempty"`
	// Path is the file: an absolute path on this machine, or the path as
	// it was named on the machine Host names. It is empty for a link, and
	// for a claim nothing was delivered under yet.
	Path string `json:"path,omitempty"`
	// Host is the declared machine the file is on, or empty for this one.
	Host string `json:"host,omitempty"`
	// URL is the address to open. For a file it is where gnosh serves it,
	// and empty when it does not; with no Path it is the link that was
	// delivered.
	URL string `json:"url,omitempty"`
}

// Delivered reports whether there is something to open: a file or a link.
// A claim under which nothing was delivered yet has neither.
func (d Deliverable) Delivered() bool { return d.Path != "" || d.URL != "" }

// Link reports whether what was delivered is an address and not a file.
func (d Deliverable) Link() bool { return d.Path == "" && d.URL != "" }

// Where says where the file is, in words: its path, and the machine when
// that is not this one. It is empty for a link and for a bare claim.
func (d Deliverable) Where() string {
	if d.Path == "" || d.Host == "" {
		return d.Path
	}
	return d.Path + " on " + d.Host
}

// State says where a deliverable stands, in a word or a few, for the line
// a front end writes when the Deliver tool has claimed or delivered it.
// replaced says the call put a version in the place of one delivered
// before.
func (d Deliverable) State(replaced bool) string {
	switch {
	case d.Pending && d.Delivered():
		return "a new version is pending"
	case d.Pending:
		return "pending"
	case replaced:
		return "new version"
	}
	return "delivered"
}

// Line writes a deliverable on one plain line, for a front end that draws
// nothing: where it stands, the label, and then the address and the file,
// each whole.
//
//	deliverable pending: demo reel
//	deliverable: hebrew ad video | http://127.0.0.1:7777/<token>/ad.mp4 | /root/renders/ad.mp4 on render
//
// The words are not made safe for a terminal: the caller does that.
func (d Deliverable) Line(replaced bool) string {
	head := "deliverable"
	switch state := d.State(replaced); state {
	case "delivered":
	case "pending":
		head += " pending"
	default:
		head += ", " + state
	}
	return d.withTargets(head + ": " + d.label())
}

// label is the label on one line.
func (d Deliverable) label() string { return strings.Join(strings.Fields(d.Label), " ") }

// withTargets writes lead and after it the address and the file, when
// there are any, with a bar between them.
func (d Deliverable) withTargets(lead string) string {
	parts := []string{lead}
	if d.URL != "" {
		parts = append(parts, d.URL)
	}
	if where := d.Where(); where != "" {
		parts = append(parts, where)
	}
	return strings.Join(parts, " | ")
}

// CountDeliverables says how many entries of a list are delivered and how
// many are pending. An entry whose newer version is on its way counts as
// pending: a promise is open.
func CountDeliverables(list []Deliverable) (delivered, pending int) {
	for _, d := range list {
		switch {
		case d.Pending:
			pending++
		case d.Delivered():
			delivered++
		}
	}
	return delivered, pending
}

// DeliverableCount writes the two counts for a status line, with sep
// between them: "2 delivered · 1 pending". A count of none is left out,
// and a list with nothing in it gives "".
func DeliverableCount(list []Deliverable, sep string) string {
	delivered, pending := CountDeliverables(list)
	var parts []string
	if delivered > 0 {
		parts = append(parts, fmt.Sprintf("%d delivered", delivered))
	}
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", pending))
	}
	return strings.Join(parts, sep)
}

// DeliverablesHint says, for a list in which gnosh serves a file on this
// machine alone, how the address is reached from another machine. It is
// empty when no entry has such an address.
func DeliverablesHint(list []Deliverable) string {
	for _, d := range list {
		if d.Path == "" || d.URL == "" {
			continue
		}
		u, err := url.Parse(d.URL)
		if err != nil || u.Port() == "" {
			continue
		}
		if ip := net.ParseIP(u.Hostname()); (ip != nil && ip.IsLoopback()) || u.Hostname() == "localhost" {
			return fmt.Sprintf("An address at %s answers on this machine only. From another machine, forward the port first: ssh -L %s:%s <this machine>", u.Host, u.Port(), u.Host)
		}
	}
	return ""
}

// DeliverablesText writes a list as plain lines, the way /deliverables
// says it where nothing is drawn: a count, every entry on a line of its
// own, and how to reach an address that answers on this machine only.
func DeliverablesText(list []Deliverable) []string {
	if len(list) == 0 {
		return []string{"Nothing has been claimed or delivered in this session yet. The model names what you asked for with its Deliver tool, and each one is listed here with its file or its address."}
	}
	lines := []string{"Deliverables, the latest version of each (" + DeliverableCount(list, ", ") + "):"}
	for _, d := range list {
		lead := "  " + d.label()
		if d.Pending {
			lead += " (" + d.State(false) + ")"
		}
		lines = append(lines, d.withTargets(lead))
	}
	if hint := DeliverablesHint(list); hint != "" {
		lines = append(lines, hint)
	}
	return lines
}

// DeliverToolName is the name of the tool by which the model claims and
// delivers: the one whose results carry a Deliverable.
const DeliverToolName = "Deliver"

// The keys of core.ToolResult.Meta under which the Deliver tool keeps what
// its call did, for the front ends, for hooks and for the session's file.
const (
	// MetaDeliverable holds the Deliverable the call claimed or delivered,
	// as it stands after the call.
	MetaDeliverable = "deliverable"
	// MetaReplaced holds true when the call delivered a version in the
	// place of one delivered before.
	MetaReplaced = "replaced"
)

// DeliverableOf reads what a call of the Deliver tool claimed or delivered
// out of the facts its result keeps (core.ToolResult.Meta): as the tool
// put them there, or as JSON gave them back from a session's file. ok is
// false for a result that holds none, which is every result of another
// tool and every call of Deliver that failed.
func DeliverableOf(meta map[string]any) (d Deliverable, replaced, ok bool) {
	switch v := meta[MetaDeliverable].(type) {
	case Deliverable:
		d = v
	case *Deliverable:
		if v == nil {
			return Deliverable{}, false, false
		}
		d = *v
	case map[string]any:
		raw, err := json.Marshal(v)
		if err != nil || json.Unmarshal(raw, &d) != nil {
			return Deliverable{}, false, false
		}
	default:
		return Deliverable{}, false, false
	}
	if strings.TrimSpace(d.Label) == "" {
		return Deliverable{}, false, false
	}
	replaced, _ = meta[MetaReplaced].(bool)
	return d, replaced, true
}

// ResultMeta reads, from an entry of a session's file, the result of a
// tool call that the loop stored there: the tool's name and the facts the
// tool kept about the call (core.ToolResult.Meta), as JSON gave them back.
// ok is false for an entry that is not the result of a call that ran: any
// other kind of entry, and a call that was refused or stopped.
func ResultMeta(te *transcript.Entry) (tool string, meta map[string]any, ok bool) {
	f, is := loopOf(te)
	if !is || f.Kind != agent.EntryResult || f.Denied || f.Interrupted {
		return "", nil, false
	}
	return f.ToolName, f.Meta, true
}

// DeliverablesEvent says the session's list of deliverables has changed,
// and carries all of it, in the order the entries last changed, the one
// that changed last at the end. An empty list means there is none now. One
// is sent when a session that already has a list is opened, too.
type DeliverablesEvent struct {
	// List is the whole list as it now stands.
	List []Deliverable
}

// setDeliverables is Session.Deliverables: the session's list is replaced,
// and the front end is told when it changed.
func (l *live) setDeliverables(list []Deliverable) {
	list = slices.Clone(list)
	l.delivMu.Lock()
	same := slices.Equal(l.deliv, list)
	l.deliv = list
	l.delivMu.Unlock()
	if !same {
		l.deliver(&DeliverablesEvent{List: slices.Clone(list)})
	}
}

// deliverables returns the session's list of deliverables as it stands.
func (l *live) deliverables() []Deliverable {
	l.delivMu.Lock()
	defer l.delivMu.Unlock()
	return slices.Clone(l.deliv)
}

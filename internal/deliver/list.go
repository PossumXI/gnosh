package deliver

import (
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/transcript"
)

// list is a session's deliverables: the latest version of each label, in
// the order the entries last changed, the one that changed last at the
// end. It holds nothing that the session's file does not: every entry is
// what a result of the Deliver tool recorded.
type list struct {
	mu      sync.Mutex
	entries []app.Deliverable
}

// labelKey is what two labels are compared by. Letter case and the blanks
// between words do not tell two deliverables apart: a model that writes
// "Demo video" once and "demo  video" later means the same thing.
func labelKey(label string) string {
	return strings.ToLower(strings.Join(strings.Fields(label), " "))
}

// cleanLabel puts a label on one line with single blanks, and takes out
// what cannot be seen: control characters and the characters that only
// steer how text is laid out.
func cleanLabel(label string) string {
	label = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, label)
	return strings.Join(strings.Fields(label), " ")
}

// put records an entry: it takes the place of the entry with the same
// label, if there is one, and stands last.
func (l *list) put(d app.Deliverable) {
	key := labelKey(d.Label)
	if key == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = slices.DeleteFunc(l.entries, func(e app.Deliverable) bool { return labelKey(e.Label) == key })
	l.entries = append(l.entries, d)
}

// get returns the entry with that label.
func (l *list) get(label string) (app.Deliverable, bool) {
	key := labelKey(label)
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if labelKey(e.Label) == key {
			return e, true
		}
	}
	return app.Deliverable{}, false
}

// all returns the entries as they stand.
func (l *list) all() []app.Deliverable {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.entries)
}

// fromEntries builds the list a session's file holds: for every result of
// a Deliver call that went well, in the order they were stored, the entry
// the result recorded. A call that failed, was refused or was stopped
// recorded none. The conversation being taken back to an earlier point
// (a rewind) takes nothing off the list: what was delivered was delivered,
// and nothing is ever deleted from a session's file.
func fromEntries(entries []transcript.Entry) *list {
	l := &list{}
	for i := range entries {
		tool, meta, ok := app.ResultMeta(&entries[i])
		if !ok || tool != app.DeliverToolName {
			continue
		}
		if d, _, ok := app.DeliverableOf(meta); ok {
			l.put(d)
		}
	}
	return l
}

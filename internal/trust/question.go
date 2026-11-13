package trust

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

// Holding is one thing a folder's own settings would do if the folder were
// trusted. The config loader gives the list (its Config.Held), computed by
// the same reader that would put the settings in force, so the question
// can never show less than what would run.
type Holding struct {
	// Kind is one of the Kind constants.
	Kind string
	// Text is the setting in a few words, taken from the folder's own
	// file. It is a stranger's text and is only ever shown escaped.
	Text string
	// File and Line say where the setting is; Line is 0 when there is
	// none.
	File string
	Line int
}

// Question is the trust question, as its parts. How it is drawn is the
// interface's business. No is selected first, declining does not exit (the
// session starts with the user's own settings only, and gnosh asks again
// next time), and Ctrl-C quits.
type Question struct {
	// Folder is the folder asked about, safe to print.
	Folder string
	// Files are the config files that would be read, safe to print.
	Files []string
	// Would lists what the folder's settings would do, one line each, safe
	// to print.
	Would []string
	// Risks says what could go wrong if whoever wrote the settings meant
	// harm: one plain sentence per kind of harm, and only for the kinds
	// this folder's settings could do.
	Risks []string
	// SessionOnly reports that a yes lasts for this session and is not
	// recorded, as for the home directory.
	SessionOnly bool
}

// NewQuestion builds the trust question for a folder from the config files
// that are waiting (the config loader's Files that are held) and from what
// they hold. It is only asked where something is held: with nothing in
// held there is nothing to ask, and the caller does not ask.
func NewQuestion(f Folder, files []string, held []Holding) Question {
	q := Question{Folder: Escape(f.Root), SessionOnly: f.SessionOnly}
	for _, file := range files {
		q.Files = append(q.Files, Escape(file))
	}
	for _, h := range held {
		line := Escape(h.Text)
		if h.File != "" {
			line += "  (" + Escape(h.File)
			if h.Line > 0 {
				line += fmt.Sprintf(", line %d", h.Line)
			}
			line += ")"
		}
		q.Would = append(q.Would, line)
	}
	q.Risks = Risks(held)
	return q
}

// Risks returns what could go wrong with these settings: one sentence per
// kind of harm present, in a fixed order.
func Risks(held []Holding) []string {
	present := map[string]bool{}
	for _, h := range held {
		present[h.Kind] = true
	}
	var out []string
	for _, r := range risks {
		if slices.ContainsFunc(r.kinds, func(k string) bool { return present[k] }) {
			out = append(out, r.says)
		}
		for _, k := range r.kinds {
			delete(present, k)
		}
	}
	// What is left fits none of the sentences above: options, rules that
	// only restrict, and kinds this package does not know.
	if len(present) > 0 {
		out = append(out, otherRisk)
	}
	return out
}

// String is the question as plain text.
func (q Question) String() string {
	var b strings.Builder
	b.WriteString("Do you trust the files in this folder?\n\n  " + q.Folder + "\n\n")
	b.WriteString("Its own settings are not in force.")
	if len(q.Files) > 0 {
		b.WriteString(" If you say yes, these files would be read:\n\n")
		for _, file := range q.Files {
			b.WriteString("  - " + file + "\n")
		}
		b.WriteString("\nThey would add:\n\n")
	} else {
		b.WriteString(" If you say yes, they would add:\n\n")
	}
	for _, w := range q.Would {
		b.WriteString("  - " + w + "\n")
	}
	if len(q.Risks) > 0 {
		b.WriteString("\nWhat could go wrong, if whoever wrote them meant harm:\n\n")
		for _, r := range q.Risks {
			b.WriteString("  - " + r + "\n")
		}
	}
	b.WriteString("\nNo: start with your own settings only; gnosh asks again next time.\n")
	if q.SessionOnly {
		b.WriteString("Yes: use these settings for this session. (This folder is never recorded as trusted.)\n")
	} else {
		b.WriteString("Yes: use these settings from now on. (gnosh trust --forget undoes it.)\n")
	}
	return b.String()
}

// Report is what gnosh trust prints after trusting a folder: what its
// settings now do, what could go wrong, and how to undo it.
func Report(f Folder, held []Holding) string {
	q := NewQuestion(f, nil, held)
	var b strings.Builder
	b.WriteString("Trusted: " + Escape(f.Key) + "\n")
	if f.Key != f.Root {
		b.WriteString("(the main checkout of the repository " + q.Folder + " is a worktree of)\n")
	}
	if len(q.Would) == 0 {
		b.WriteString("\nIts settings hold nothing that was waiting for trust.\n")
	} else {
		b.WriteString("\nIts own settings are now in force. They add:\n\n")
		for _, w := range q.Would {
			b.WriteString("  - " + w + "\n")
		}
	}
	if len(q.Risks) > 0 {
		b.WriteString("\nWhat could go wrong, if whoever wrote them meant harm:\n\n")
		for _, r := range q.Risks {
			b.WriteString("  - " + r + "\n")
		}
	}
	b.WriteString("\nTo undo: gnosh trust --forget " + Escape(f.Key) + "\n")
	return b.String()
}

// Notice is the one line a run with nobody to ask prints on stderr when a
// folder's settings are held: that the folder is not trusted, that its
// project settings are off, and the two ways to use them.
func Notice(f Folder) string {
	return fmt.Sprintf("gnosh: the folder %s is not trusted, so its own project settings are off. To use them: gnosh trust %s, or --trust-folder for this one run.", Escape(f.Root), Escape(f.Root))
}

// Asked is what Decide is told.
type Asked struct {
	// Folder is the folder gnosh was started in.
	Folder Folder
	// Verdict is what the record says about it (Store.Check).
	Verdict Verdict
	// TrustFolder reports that --trust-folder was given.
	TrustFolder bool
	// Interactive reports that there is a person to ask.
	Interactive bool
	// Held is how many things of the folder's wait for trust: the length
	// of the config loader's Held list from a load that treated the folder
	// as not trusted, plus what GitPrograms found.
	Held int
}

// Decision is what to do about a folder's own settings.
type Decision struct {
	// Trusted: put the folder's settings in force.
	Trusted bool
	// Ask: put the trust question to the person before going on. A yes is
	// recorded with Store.Trust unless Folder.SessionOnly; a no starts the
	// session with the user's own settings only.
	Ask bool
	// Notice is the line to print on stderr when nobody can be asked and
	// settings are held. Empty otherwise.
	Notice string
	// Reason says in a few words why.
	Reason string
}

// Decide says what to do about a folder's own settings: use them, ask, or
// leave them out with a notice. A folder with nothing waiting for trust is
// never asked about. Without a person (gnosh run, -p) nothing is ever
// asked.
func Decide(a Asked) Decision {
	switch {
	case a.TrustFolder:
		return Decision{Trusted: true, Reason: "--trust-folder: trusted for this run, and not recorded"}
	case a.Verdict.Trusted:
		return Decision{Trusted: true, Reason: "recorded as trusted: " + a.Verdict.By}
	case a.Held == 0:
		return Decision{Reason: "nothing of this folder's waits for trust"}
	case a.Interactive:
		return Decision{Ask: true, Reason: "not trusted, and some of its settings wait for trust"}
	}
	return Decision{Notice: Notice(a.Folder), Reason: "not trusted, and nobody is there to ask"}
}

// Escape makes text from a stranger's file safe to print: control
// characters, and the invisible characters that reorder text on screen,
// come out as visible escapes, so that a name or a command can never act
// as a terminal control sequence or show as something it is not.
func Escape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			fmt.Fprintf(&b, `\x%02x`, s[i])
		case r == '\\':
			b.WriteString(`\\`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case r >= 0x80 && r < 0xa0, r == 0x200e, r == 0x200f, r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0xfeff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// LeadsOut reports whether a path inside a folder really leads out of it,
// once the links on the way are followed, and where it leads. A project's
// skills folder that is a link to somewhere else is left out of an
// untrusted folder for this reason: the agent reads a skill's files
// without asking. A path that cannot be followed counts as leading out.
func LeadsOut(root, path string) (target string, out bool) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", true
	}
	target, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", true
	}
	return target, target != realRoot && !holds(realRoot, target)
}

package prompt

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// The texts in this file stand inside tool results. They carry no marker:
// the marker is for the user's side of the conversation, and the model is
// told that nothing inside a tool result is an instruction. A note is a
// plain sentence in square brackets; a refusal begins with the words the
// rest of gnosh knows a refused close by.

// PopNote returns the note appended to the answer of the call that entered
// a todo, once the todo has been popped: what came between the two was
// removed from the model's context. calls counts the tool calls of the
// removed replies by tool name, and messages counts the removed messages,
// which is said instead when there was no tool call. With nothing removed
// there is no note.
//
// Without the note, a long session's history reads as a row of todos
// entered and popped at once, and a model copies the row.
func (s *Set) PopNote(calls map[string]int, messages int) string {
	return s.cutNote(textPopNote, calls, messages)
}

// SlideNote returns the note for the place where rounds slid out of the
// view to make room: it is appended to the answer the view keeps just
// before them. calls and messages are as for PopNote.
func (s *Set) SlideNote(calls map[string]int, messages int) string {
	return s.cutNote(textSlideNote, calls, messages)
}

func (s *Set) cutNote(name string, calls map[string]int, messages int) string {
	vals := values{}
	total := 0
	var tools []string
	for _, tool := range slices.Sorted(maps.Keys(calls)) {
		if n := calls[tool]; n > 0 {
			total += n
			tools = append(tools, oneLine(tool, maxName)+" "+strconv.Itoa(n))
		}
	}
	switch {
	case total == 1:
		vals.flag("one_call", true)
		vals["tools"] = tools[0]
	case total > 1:
		vals["calls"] = strconv.Itoa(total)
		vals["tools"] = strings.Join(tools, ", ")
	case messages == 1:
		vals.flag("one_message", true)
	case messages > 1:
		vals["messages"] = strconv.Itoa(messages)
	default:
		return ""
	}
	return s.fill(name, vals)
}

// maxPopChanges is how many changes to mem a pop's receipt names before it
// counts the rest.
const maxPopChanges = 4

// PopReceipt is what the pop of a todo did to the model's context, for
// the result of the call that popped it.
type PopReceipt struct {
	// Frame is the todo that was popped.
	Frame string
	// Nothing says nothing came between entering the todo and popping it,
	// so that nothing was removed. The other fields are then not used.
	Nothing bool
	// Spared says what the user said in this turn stayed.
	Spared bool
	// Thoughts is how many thoughts the todo holds. Mem's own lines, just
	// above, list them.
	Thoughts int
	// Files are the files the removed work changed, each once, as the
	// model would name them.
	Files []string
	// Changes are the other changes the removed work made to mem, each as
	// the call's word and the name it was given: "Fact `port`". The first
	// four are named and the rest counted.
	Changes []string
	// Reads says the files read in the removed work must be read again
	// before they are changed.
	Reads bool
}

// PopReceipt returns what the result of a call that pops a todo says
// after mem's own lines: that everything since the todo was entered was
// removed from the model's context, what stays, which files the removed
// work changed, what else it changed in mem, and that its reads are
// forgotten. It says "removed from your context" and never "left", which
// reads as "nothing is left"; and it gives no number of tokens, which is
// for the person and not for the model.
func (s *Set) PopReceipt(p PopReceipt) string {
	vals := values{}
	if p.Nothing {
		vals.flag("nothing", true)
		return s.fill(textPopReceipt, vals)
	}
	vals["id"] = oneLine(p.Frame, maxName)
	vals.flag("spared", p.Spared)
	switch {
	case p.Thoughts == 1:
		vals.flag("one_thought", true)
	case p.Thoughts > 1:
		vals["thoughts"] = strconv.Itoa(p.Thoughts)
	}
	var files []string
	for _, f := range p.Files {
		if f = oneLine(f, maxName); f != "" {
			files = append(files, f)
		}
	}
	vals["files"] = clip(strings.Join(files, ", "), maxReason)
	var changes []string
	for _, c := range p.Changes {
		if c = oneLine(c, maxName); c != "" {
			changes = append(changes, c)
		}
	}
	if more := len(changes) - maxPopChanges; more > 0 {
		vals["changes"] = strings.Join(changes[:maxPopChanges], ", ") + " and " + strconv.Itoa(more) + " more"
	} else {
		vals["changes"] = strings.Join(changes, ", ")
	}
	vals.flag("reads", p.Reads)
	return s.fill(textPopReceipt, vals)
}

// PlanRefusal returns what a call of a gnovm tool that runs or changes
// something is answered with while the session is in plan mode: that
// nothing in a machine or in mem changes while a plan is being made, which
// tool still runs, and that the plan goes on mem first once it is
// approved. It ends without a full stop: the loop puts it after "This
// call did not run: " and ends the sentence.
func (s *Set) PlanRefusal(tool string) string {
	return s.fill(textPlanGnovm, values{"tool": oneLine(tool, maxName)})
}

// NoResult says why a tool call has no result of its own.
type NoResult int

const (
	// NotStarted: the person stopped the turn before the call began.
	NotStarted NoResult = iota
	// StoppedRunning: the person stopped the turn while the call ran.
	StoppedRunning
	// TurnEnded: the turn was ended before the call began. The detail
	// says why.
	TurnEnded
	// ReplyCutOff: the reply that asked for the call was cut off before
	// the call was written out whole.
	ReplyCutOff
	// ProcessStopped: gnosh itself was stopped before the call returned,
	// and the session is being opened again.
	ProcessStopped
	// StillRunning: the person stopped the turn and the call had not
	// stopped when gnosh gave up waiting. The detail says how long it
	// waited.
	StillRunning
	// Running: the call is still running, and a request beside the
	// session's (a side question) is sent meanwhile.
	Running
)

// MissingResult returns the text that stands in for the result of a tool
// call that has none of its own. Every call must have a result for the
// conversation to be sent again, and this one says honestly what is known:
// whether the call ran at all, and if it may have run in part. detail is
// the reason a turn was ended, or how long a call was waited for; other
// cases take none.
func (s *Set) MissingResult(why NoResult, detail string) string {
	vals := values{}
	detail = oneLine(detail, maxQuote)
	switch why {
	case StoppedRunning:
		vals.flag("stopped", true)
	case TurnEnded:
		vals.flag("ended", true)
		vals["why"] = detail
	case ReplyCutOff:
		vals.flag("cut_off", true)
	case ProcessStopped:
		vals.flag("killed", true)
	case StillRunning:
		vals.flag("still", true)
		vals["waited"] = detail
	case Running:
		vals.flag("running", true)
	default:
		vals.flag("not_started", true)
	}
	return s.fill(textMissingResult, vals)
}

// The words every refused close begins with: the first when the todo was
// being popped, the second when it was being closed by its name. Mem's own
// refusal of a pop with nothing done begins the same way. RefusedClose
// reads them back.
const (
	notPopped = "not popped: "
	notClosed = "not closed: "
)

// refusal puts the words a refused close begins with before the text
// called name.
func (s *Set) refusal(name string, byName bool, vals values) string {
	vals.flag("popped", !byName)
	vals.flag("closed", byName)
	lead := notPopped
	if byName {
		lead = notClosed
	}
	return lead + s.fill(name, vals)
}

// CloseEnteredHere returns the refusal for a call that entered the todo
// called todo and closed it in one go, with nothing done in it.
func (s *Set) CloseEnteredHere(todo string) string {
	return s.refusal(textCloseEnteredHere, false, values{"todo": oneLine(todo, maxName)})
}

// CloseUnbacked returns the refusal for a close whose result says that
// file changed, when nothing in the todo changed it or showed it. byName
// says the todo was closed by its name rather than popped.
func (s *Set) CloseUnbacked(todo, file string, byName bool) string {
	return s.refusal(textCloseUnbacked, byName, values{
		"todo": oneLine(todo, maxName),
		"file": oneLine(file, maxName),
	})
}

// CloseNotMade returns the refusal for a close whose result says that file
// was made, when there is no such file.
func (s *Set) CloseNotMade(todo, file string, byName bool) string {
	return s.refusal(textCloseNotMade, byName, values{
		"todo": oneLine(todo, maxName),
		"file": oneLine(file, maxName),
	})
}

// CloseReviewRefused returns the refusal for a close the review did not
// pass. reason is what the reviewer found missing, in its own sentence.
func (s *Set) CloseReviewRefused(todo, reason string, byName bool) string {
	return s.refusal(textCloseRefused, byName, values{
		"todo":   oneLine(todo, maxName),
		"reason": strings.TrimRight(oneLine(reason, maxReason), ". "),
	})
}

// CloseCancelled returns the refusal for a close whose review was cut off
// when the person stopped the turn. A cancelled check is not a pass.
func (s *Set) CloseCancelled(todo string, byName bool) string {
	return s.refusal(textCloseCancelled, byName, values{"todo": oneLine(todo, maxName)})
}

// RefusedClose reports whether result, the text of a mem call's answer,
// refuses the close of a todo, and names the todo. It knows a refusal by
// its shape: the words "not popped: " or "not closed: ", and then the
// first name in backquotes, which every refusal gives before anything
// else. Mem's own refusal of a pop with nothing done has the same shape,
// so this finds it too.
//
// Three places need to tell a refused close from any other answer: the
// rule for what counts as work in a todo, the nudge for a repeated call,
// and the send-back at a turn's end.
func RefusedClose(result string) (todo string, ok bool) {
	at := strings.Index(result, notPopped)
	if by := strings.Index(result, notClosed); by >= 0 && (at < 0 || by < at) {
		at = by
	}
	if at < 0 {
		return "", false
	}
	rest := result[at+len(notPopped):]
	if end := strings.IndexByte(rest, '\n'); end >= 0 {
		rest = rest[:end]
	}
	open := strings.IndexByte(rest, '`')
	if open < 0 {
		return "", false
	}
	rest = rest[open+1:]
	end := strings.IndexByte(rest, '`')
	if end <= 0 {
		return "", false
	}
	return rest[:end], true
}

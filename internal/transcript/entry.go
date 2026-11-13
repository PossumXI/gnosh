package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// Version is the format version written in every entry's "v" field.
const Version = 1

// EntryType says what an Entry records.
type EntryType string

const (
	// TypeUser is a user-side message: a prompt, tool results, hook context
	// or a notice from the harness. Source says which.
	TypeUser EntryType = "user"
	// TypeAssistant is one reply from the model.
	TypeAssistant EntryType = "assistant"
	// TypeSummary is text that stands in for the conversation before it.
	TypeSummary EntryType = "summary"
	// TypeRewind says the conversation continues from an earlier entry.
	TypeRewind EntryType = "rewind"
	// TypeFrame is a frame boundary: a push, a pop or a slide.
	TypeFrame EntryType = "frame"
	// TypeTitle is the session's name.
	TypeTitle EntryType = "title"
	// TypeMeta is a small key/value note about the session.
	TypeMeta EntryType = "meta"
	// TypeUsage is the token count of one request.
	TypeUsage EntryType = "usage"
	// TypeBranch is the first entry of a branched session: it points at the
	// session and entry the branch continues from.
	TypeBranch EntryType = "branch"
)

// Source says where a user entry's message came from, or who named a title.
type Source string

const (
	// SourcePrompt is a prompt the person typed.
	SourcePrompt Source = "prompt"
	// SourceToolResults is the results of the tool calls of the reply before.
	SourceToolResults Source = "tool_results"
	// SourceHook is context a hook added.
	SourceHook Source = "hook"
	// SourceNotice is something the harness says for itself: a background
	// command ended, a reminder, a continuation.
	SourceNotice Source = "notice"
	// SourceUser marks a title the person gave. It outranks an automatic one.
	SourceUser Source = "user"
	// SourceAuto marks a title that was generated.
	SourceAuto Source = "auto"
)

// FrameOp says what a frame entry does.
type FrameOp string

const (
	// OpPush enters a frame.
	OpPush FrameOp = "push"
	// OpPop leaves a frame with a result.
	OpPop FrameOp = "pop"
	// OpSlide records that the oldest rounds of a frame were left out of the
	// view to make room, without leaving the frame.
	OpSlide FrameOp = "slide"
)

// Entry is one line of a session file. The first seven fields are on every
// entry; which of the others are set depends on Type. Fields this version
// does not know are kept in Extra and written back as they were, so a file
// written by a newer gnosh survives being loaded and copied by this one.
//
// An Entry returned by a Session is shared with it: treat it as read-only.
type Entry struct {
	// Type says what the entry records. A type this version does not know
	// is kept and ignored by the view.
	Type EntryType
	// UUID names the entry.
	UUID string
	// ParentUUID is the UUID of the entry written before this one in the
	// same session file. It is empty for the first.
	ParentUUID string
	// SessionID is the session whose file the entry was written to.
	SessionID string
	// Timestamp is when the entry was written, in UTC.
	Timestamp time.Time
	// Cwd is the session's working directory when the entry was written.
	Cwd string
	// V is the format version.
	V int

	// Message is the message of a user or assistant entry.
	Message core.Message
	// Source says where a user entry's message came from, or, on a title
	// entry, whether the person or gnosh chose the title.
	Source Source
	// Stop says how an assistant entry's reply ended: one of core's stop
	// reasons. core.StopCancelled and core.StopError mark a reply that was
	// cut off; the turn it belongs to ended there. Empty means unknown.
	Stop core.StopReason

	// Summary is a summary entry's text.
	Summary string
	// Covers is the UUID of the last entry a summary entry stands in for.
	// Empty means everything before the summary entry.
	Covers string

	// To is the UUID of the entry a rewind entry continues from.
	To string

	// Op, Frame, Label, Result and ToolUseID describe a frame entry: what
	// happened, the frame's id, its title, a pop's result, and the id of
	// the tool call that caused a push or a pop.
	Op        FrameOp
	Frame     string
	Label     string
	Result    string
	ToolUseID string
	// Anchor and Until bound a slide: the UUID of the assistant entry whose
	// round stays in front of the cut, and the UUID of the assistant entry
	// the cut ends before.
	Anchor string
	Until  string

	// Title is a title entry's text.
	Title string

	// Key and Value are a meta entry's note.
	Key   string
	Value string

	// Usage, Model and DurationMs are a usage entry: one request's token
	// counts, the model that served it and how long it took.
	Usage      core.Usage
	Model      string
	DurationMs int64

	// FromSession and FromUUID are a branch entry's pointer: the session
	// and the entry the branch continues from.
	FromSession string
	FromUUID    string

	// Extra holds every field this version does not define, by name, as raw
	// JSON. A defined field whose value has an unexpected shape is kept
	// here too, and its typed field stays zero.
	Extra map[string]json.RawMessage

	// kept holds the raw JSON of a defined object field (message, usage)
	// that had members this version does not know, so that writing the
	// entry again loses nothing.
	kept map[string]json.RawMessage
}

// UserEntry makes a user entry holding m, which came from src.
func UserEntry(m core.Message, src Source) Entry {
	m.Role = core.RoleUser
	return Entry{Type: TypeUser, Message: m, Source: src}
}

// PromptEntry makes the user entry for a prompt the person typed.
func PromptEntry(text string) Entry {
	return UserEntry(core.UserText(text), SourcePrompt)
}

// AssistantEntry makes an assistant entry holding the reply m, which ended
// with stop.
func AssistantEntry(m core.Message, stop core.StopReason) Entry {
	m.Role = core.RoleAssistant
	return Entry{Type: TypeAssistant, Message: m, Stop: stop}
}

// SummaryEntry makes a summary entry: text stands in for the conversation
// up to and including the entry named covers, or for everything before the
// summary entry when covers is empty.
func SummaryEntry(text, covers string) Entry {
	return Entry{Type: TypeSummary, Summary: text, Covers: covers}
}

// RewindEntry makes a rewind entry: the conversation continues from the
// entry named to, as it stood right after that entry was written.
func RewindEntry(to string) Entry {
	return Entry{Type: TypeRewind, To: to}
}

// PushEntry makes the frame entry for entering frame id, titled label, by
// the tool call toolUseID.
func PushEntry(id, label, toolUseID string) Entry {
	return Entry{Type: TypeFrame, Op: OpPush, Frame: id, Label: label, ToolUseID: toolUseID}
}

// PopEntry makes the frame entry for leaving frame id with result, by the
// tool call toolUseID.
func PopEntry(id, label, result, toolUseID string) Entry {
	return Entry{Type: TypeFrame, Op: OpPop, Frame: id, Label: label, Result: result, ToolUseID: toolUseID}
}

// TitleEntry makes a title entry. src is SourceUser for a name the person
// gave and SourceAuto for a generated one.
func TitleEntry(title string, src Source) Entry {
	return Entry{Type: TypeTitle, Title: title, Source: src}
}

// MetaEntry makes a meta entry.
func MetaEntry(key, value string) Entry {
	return Entry{Type: TypeMeta, Key: key, Value: value}
}

// UsageEntry makes a usage entry.
func UsageEntry(u core.Usage, model string, d time.Duration) Entry {
	return Entry{Type: TypeUsage, Usage: u, Model: model, DurationMs: d.Milliseconds()}
}

// IsMessage reports whether the entry is a user or assistant entry with
// something in its message.
func (e *Entry) IsMessage() bool {
	return (e.Type == TypeUser || e.Type == TypeAssistant) && len(e.Message.Blocks) > 0
}

// timeLayout is RFC 3339 in UTC with all nine fractional digits, so that
// timestamps sort as text.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

// knownKey reports whether name is a field this version of the format
// defines.
func knownKey(name string) bool {
	switch name {
	case "type", "uuid", "parentUuid", "sessionId", "timestamp", "cwd", "v",
		"message", "source", "stop", "summary", "covers", "to",
		"op", "frame", "label", "result", "toolUseId", "anchor", "until",
		"title", "key", "value", "usage", "model", "durationMs",
		"fromSession", "fromUuid":
		return true
	}
	return false
}

// lineBuilder writes one JSON object, field by field, without the HTML
// escaping encoding/json applies by default: transcripts are full of code.
type lineBuilder struct {
	buf bytes.Buffer
	enc *json.Encoder
	n   int
	err error
}

func newLineBuilder() *lineBuilder {
	b := &lineBuilder{}
	b.enc = json.NewEncoder(&b.buf)
	b.enc.SetEscapeHTML(false)
	return b
}

// value writes v as JSON, without the newline the encoder adds.
func (b *lineBuilder) value(v any) {
	if b.err != nil {
		return
	}
	if err := b.enc.Encode(v); err != nil {
		b.err = err
		return
	}
	b.buf.Truncate(b.buf.Len() - 1)
}

// key starts the field name.
func (b *lineBuilder) key(name string) {
	if b.n == 0 {
		b.buf.WriteByte('{')
	} else {
		b.buf.WriteByte(',')
	}
	b.n++
	b.value(name)
	b.buf.WriteByte(':')
}

// field writes the field name with value v.
func (b *lineBuilder) field(name string, v any) {
	b.key(name)
	b.value(v)
}

// raw writes the field name with a value that is JSON already, compacted
// so that it cannot hold a line break.
func (b *lineBuilder) raw(name string, v json.RawMessage) {
	b.key(name)
	if b.err != nil {
		return
	}
	if err := json.Compact(&b.buf, v); err != nil {
		b.err = fmt.Errorf("field %q does not hold valid JSON: %w", name, err)
	}
}

// str writes a string field when it is not empty, or the raw value Extra
// holds under the same name when the typed field is empty.
func (b *lineBuilder) str(e *Entry, name, s string) {
	if s != "" {
		b.field(name, s)
	} else if raw, ok := e.Extra[name]; ok {
		b.raw(name, raw)
	}
}

// header writes a field that every entry has: the typed value, or the raw
// value Extra holds under the same name when the typed one is zero.
func (b *lineBuilder) header(e *Entry, name string, zero bool, v any) {
	if raw, ok := e.Extra[name]; ok && zero {
		b.raw(name, raw)
		return
	}
	b.field(name, v)
}

// object writes a defined object field. When the entry was loaded with
// members this version does not know and the value has not been changed
// since, the original JSON is written back.
func (b *lineBuilder) object(e *Entry, name string, cur, fresh any) {
	if raw, ok := e.kept[name]; ok {
		if json.Unmarshal(raw, fresh) == nil && reflect.DeepEqual(reflect.ValueOf(fresh).Elem().Interface(), cur) {
			b.raw(name, raw)
			return
		}
	}
	b.field(name, cur)
}

// wireMessage is core.Message with a content list that is never null.
type wireMessage struct {
	Role   core.Role    `json:"role"`
	Blocks []core.Block `json:"content"`
}

// line returns the entry as one line of JSON, without a line break.
func (e *Entry) line() ([]byte, error) {
	b := newLineBuilder()
	b.field("type", string(e.Type))
	b.header(e, "uuid", e.UUID == "", e.UUID)
	b.header(e, "parentUuid", e.ParentUUID == "", e.ParentUUID)
	b.header(e, "sessionId", e.SessionID == "", e.SessionID)
	b.header(e, "timestamp", e.Timestamp.IsZero(), e.Timestamp.UTC().Format(timeLayout))
	b.header(e, "cwd", e.Cwd == "", e.Cwd)
	b.header(e, "v", e.V == 0, e.V)

	b.str(e, "source", string(e.Source))
	b.str(e, "stop", string(e.Stop))
	switch {
	case e.Message.Role != "" || len(e.Message.Blocks) > 0 || e.Type == TypeUser || e.Type == TypeAssistant:
		if _, misread := e.Extra["message"]; misread && e.Message.Role == "" && len(e.Message.Blocks) == 0 {
			b.raw("message", e.Extra["message"])
			break
		}
		m := wireMessage{Role: e.Message.Role, Blocks: e.Message.Blocks}
		if m.Blocks == nil {
			m.Blocks = []core.Block{}
		}
		if raw, ok := e.kept["message"]; ok {
			var old core.Message
			if json.Unmarshal(raw, &old) == nil && reflect.DeepEqual(old, e.Message) {
				b.raw("message", raw)
				break
			}
		}
		b.field("message", m)
	default:
		if raw, ok := e.Extra["message"]; ok {
			b.raw("message", raw)
		}
	}
	b.str(e, "summary", e.Summary)
	b.str(e, "covers", e.Covers)
	b.str(e, "to", e.To)
	b.str(e, "op", string(e.Op))
	b.str(e, "frame", e.Frame)
	b.str(e, "label", e.Label)
	b.str(e, "result", e.Result)
	b.str(e, "toolUseId", e.ToolUseID)
	b.str(e, "anchor", e.Anchor)
	b.str(e, "until", e.Until)
	b.str(e, "title", e.Title)
	b.str(e, "key", e.Key)
	b.str(e, "value", e.Value)
	if raw, misread := e.Extra["usage"]; misread && e.Usage == (core.Usage{}) {
		b.raw("usage", raw)
	} else if e.Usage != (core.Usage{}) || e.Type == TypeUsage {
		b.object(e, "usage", e.Usage, new(core.Usage))
	}
	b.str(e, "model", e.Model)
	if e.DurationMs != 0 {
		b.field("durationMs", e.DurationMs)
	} else if raw, ok := e.Extra["durationMs"]; ok {
		b.raw("durationMs", raw)
	}
	b.str(e, "fromSession", e.FromSession)
	b.str(e, "fromUuid", e.FromUUID)

	names := make([]string, 0, len(e.Extra))
	for name := range e.Extra {
		if !knownKey(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	for _, name := range names {
		b.raw(name, e.Extra[name])
	}
	if b.err != nil {
		return nil, fmt.Errorf("transcript: writing a %q entry as JSON: %w", e.Type, b.err)
	}
	b.buf.WriteByte('}')
	return b.buf.Bytes(), nil
}

// MarshalJSON writes the entry as the JSON object a session file holds.
func (e Entry) MarshalJSON() ([]byte, error) {
	return e.line()
}

// errNotObject is what reading a line that is not a JSON object gives.
var errNotObject = errors.New("not a JSON object")

// UnmarshalJSON reads an entry from the JSON object a session file holds.
// It fails only when data is not a JSON object or its "type" is not a
// string; everything else is kept, typed where it can be. The fields of an
// entry whose type this version does not define are all kept in Extra,
// apart from the seven every entry has.
func (e *Entry) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errNotObject
	}
	*e = Entry{}
	if raw, ok := fields["type"]; ok {
		if err := json.Unmarshal(raw, &e.Type); err != nil {
			return fmt.Errorf(`"type" is not a string`)
		}
	}
	defined := knownType(e.Type)
	for name, raw := range fields {
		ok := true
		switch name {
		case "type":
		case "uuid":
			ok = json.Unmarshal(raw, &e.UUID) == nil
		case "parentUuid":
			ok = json.Unmarshal(raw, &e.ParentUUID) == nil
		case "sessionId":
			ok = json.Unmarshal(raw, &e.SessionID) == nil
		case "timestamp":
			var s string
			if ok = json.Unmarshal(raw, &s) == nil; ok {
				t, err := time.Parse(time.RFC3339Nano, s)
				if ok = err == nil; ok {
					e.Timestamp = t.UTC()
				}
			}
		case "cwd":
			ok = json.Unmarshal(raw, &e.Cwd) == nil
		case "v":
			ok = json.Unmarshal(raw, &e.V) == nil
		default:
			ok = defined && e.readField(name, raw)
		}
		if !ok {
			if e.Extra == nil {
				e.Extra = make(map[string]json.RawMessage)
			}
			e.Extra[name] = raw
		}
	}
	return nil
}

// knownType reports whether t is an entry type this version defines.
func knownType(t EntryType) bool {
	switch t {
	case TypeUser, TypeAssistant, TypeSummary, TypeRewind, TypeFrame, TypeTitle, TypeMeta, TypeUsage, TypeBranch:
		return true
	}
	return false
}

// readField reads one of the fields that depend on the entry's type. It
// reports false when the field is not defined or its value has an
// unexpected shape.
func (e *Entry) readField(name string, raw json.RawMessage) bool {
	switch name {
	case "message":
		return e.readObject(name, raw, &e.Message)
	case "source":
		return json.Unmarshal(raw, &e.Source) == nil
	case "stop":
		return json.Unmarshal(raw, &e.Stop) == nil
	case "summary":
		return json.Unmarshal(raw, &e.Summary) == nil
	case "covers":
		return json.Unmarshal(raw, &e.Covers) == nil
	case "to":
		return json.Unmarshal(raw, &e.To) == nil
	case "op":
		return json.Unmarshal(raw, &e.Op) == nil
	case "frame":
		return json.Unmarshal(raw, &e.Frame) == nil
	case "label":
		return json.Unmarshal(raw, &e.Label) == nil
	case "result":
		return json.Unmarshal(raw, &e.Result) == nil
	case "toolUseId":
		return json.Unmarshal(raw, &e.ToolUseID) == nil
	case "anchor":
		return json.Unmarshal(raw, &e.Anchor) == nil
	case "until":
		return json.Unmarshal(raw, &e.Until) == nil
	case "title":
		return json.Unmarshal(raw, &e.Title) == nil
	case "key":
		return json.Unmarshal(raw, &e.Key) == nil
	case "value":
		return json.Unmarshal(raw, &e.Value) == nil
	case "usage":
		return e.readObject(name, raw, &e.Usage)
	case "model":
		return json.Unmarshal(raw, &e.Model) == nil
	case "durationMs":
		return json.Unmarshal(raw, &e.DurationMs) == nil
	case "fromSession":
		return json.Unmarshal(raw, &e.FromSession) == nil
	case "fromUuid":
		return json.Unmarshal(raw, &e.FromUUID) == nil
	}
	return false
}

// readObject reads a defined object field into dst. It first reads
// strictly; when that fails because of members this version does not know,
// it reads again loosely and keeps the raw JSON beside the typed value. It
// reports false, leaving dst zero, when the value cannot be read at all.
func (e *Entry) readObject(name string, raw json.RawMessage, dst any) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(dst) == nil {
		return true
	}
	reflect.ValueOf(dst).Elem().SetZero()
	if json.Unmarshal(raw, dst) != nil {
		reflect.ValueOf(dst).Elem().SetZero()
		return false
	}
	if e.kept == nil {
		e.kept = make(map[string]json.RawMessage)
	}
	e.kept[name] = raw
	return true
}

// misread lists the defined fields whose values had an unexpected shape
// when the entry was read, sorted. They are in Extra and are ignored. An
// entry of a type this version does not define has none: nothing of it is
// read.
func (e *Entry) misread() []string {
	var names []string
	for name := range e.Extra {
		switch name {
		case "uuid", "parentUuid", "sessionId", "timestamp", "cwd", "v":
			names = append(names, name)
		default:
			if knownKey(name) && knownType(e.Type) {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return names
}

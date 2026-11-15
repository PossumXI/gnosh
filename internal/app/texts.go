package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/core"
	"github.com/gnosh-ai/gnosh/internal/prompt"
)

// loopTexts makes the loop's own words out of the prompt texts in force:
// gnosh's own, with the person's replacements. The loop fills its words in
// with fmt.Sprintf, so each text is turned into a format with a verb where
// the loop puts a value. A text the prompt package has no word for keeps
// the loop's own default.
//
// mem says the session has mem, so that a summary leaves out what mem
// hands back by itself.
func loopTexts(set *prompt.Set, mem bool) (agent.Texts, error) {
	var t agent.Texts
	var firstErr error
	format := func(dst *string, name string, holes map[string]string) {
		s, err := set.Format(name, holes)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("the text %q could not be made ready for the loop: %w", name, err)
			}
			return
		}
		*dst = s
	}
	yes := "yes"

	format(&t.EmptyNudge, "empty-reply", nil)
	// The loop gives: how many rounds were the same, how many were looked
	// at, and the calls.
	format(&t.RepeatNudge, "repeat", map[string]string{"calls": "%[3]s"})
	// The loop gives: how many rounds were looked at, and the two calls.
	format(&t.RepeatPair, "repeat", map[string]string{"calls": "%[2]s"})
	// The loop gives: the marker, and how many times in a row.
	format(&t.Checkpoint, "checkpoint", map[string]string{"marker": "%[1]s", "count": "%[2]d"})
	// The loop gives: the check's name, and the error.
	format(&t.DriverFailed, "check-could-not-run", map[string]string{"name": "%[1]s", "error": "%[2]v"})

	format(&t.StoppedBefore, "missing-result", map[string]string{"not_started": yes})
	format(&t.StoppedDuring, "missing-result", map[string]string{"stopped": yes})
	format(&t.HaltedBefore, "missing-result", map[string]string{"ended": yes, "why": "%[1]s"})
	format(&t.ReplyCut, "missing-result", map[string]string{"cut_off": yes})
	format(&t.Killed, "missing-result", map[string]string{"killed": yes})
	format(&t.DidNotStop, "missing-result", map[string]string{"still": yes, "waited": "%[1]s"})

	summary := map[string]string{"words": fmt.Sprint(prompt.SummaryWords)}
	if mem {
		summary["mem"] = yes
	}
	format(&t.SummaryRequest, "summary", summary)
	format(&t.SummaryExtra, "summary-extra", map[string]string{"instructions": "%[1]s"})
	format(&t.SummaryIntro, "summary-intro", nil)
	format(&t.SummaryAfter, "after-summary", nil)
	format(&t.TrimNote, "earlier-left-out", nil)

	format(&t.LoopContinue, "loop", map[string]string{"marker": prompt.LoopDone})
	format(&t.LoopQuestion, "loop-prove", map[string]string{"marker": prompt.LoopDone})
	// The loop gives: the word that stops the loop, and the turn's number.
	format(&t.LoopCheckin, "loop-check-in", map[string]string{"marker": "%[1]s", "turns": "%[2]d"})
	return t, firstErr
}

// The three text filters, at the three points the loop gives for them.
//
// What a tool brings (a file, a command's output, a web page) can hold the
// character sequences a model server reads as structure, and gnosh's own
// tags. On its way to the model such text is made harmless with one
// invisible mark (cleanResult, and defuseRequest over the whole request).
// A model that copies a marked line into an edit or a command copies the
// mark with it, so the marks are taken out again from what the model
// writes before a tool acts on it (cleanInput). gnosh's own lines are
// never passed through the stricter filter, which would break their tags.

// maxResultBytes is the most text one tool result may carry to the model,
// whatever the tool itself let through. Every tool of gnosh's own has a
// smaller limit; this one is the net under them all.
const maxResultBytes = 400_000

// markEscape is the mark written as a JSON escape, which is how a model
// may spell it in a tool call's arguments. It is put together from two
// pieces, so that nothing that rewrites escapes on the way into this file
// can turn it into the character itself, which a reader could not see.
const markEscape = `\` + `u200b`

// cleanInput is the loop's filter for a tool call's arguments: the marks
// that made text harmless on its way to the model are taken out of every
// string the model wrote, so that the tool acts on the text as it was.
func cleanInput(call core.ToolCall) json.RawMessage {
	out, ok := rewriteJSONStrings(call.Input, prompt.Undefuse, func(raw []byte) bool {
		return bytes.Contains(raw, []byte(prompt.Mark)) || containsFold(raw, markEscape)
	})
	if !ok {
		return call.Input
	}
	return out
}

// cleanResult is the loop's filter for a tool result on its way to the
// model and to the person: text that came from outside is made harmless,
// and a result of absurd size is cut to its start and its end.
func cleanResult(_ core.ToolCall, res core.ToolResult) core.ToolResult {
	res.Content = cleanBlocks(res.Content)
	return res
}

// cleanBlocks makes the text blocks of a tool result harmless and bounded.
func cleanBlocks(blocks []core.Block) []core.Block {
	changed := false
	out := make([]core.Block, len(blocks))
	for i, b := range blocks {
		if b.Type == core.BlockText {
			text := prompt.Untrusted(capText(b.Text, maxResultBytes))
			if text != b.Text {
				b.Text, changed = text, true
			}
		}
		out[i] = b
	}
	if !changed {
		return blocks
	}
	return out
}

// capText cuts text that is longer than max bytes to its start and its
// end, on character boundaries, around a line that says how much was left
// out.
func capText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	head, tail := max/2, max/2
	for head > 0 && !utf8.RuneStart(s[head]) {
		head--
	}
	from := len(s) - tail
	for from < len(s) && !utf8.RuneStart(s[from]) {
		from++
	}
	return s[:head] + fmt.Sprintf("\n[... %d bytes of this result were left out by gnosh: it was too long to give whole ...]\n", from-head) + s[from:]
}

// defuseRequest makes every message of an outgoing request harmless:
// whatever a person typed, a tool returned or the model itself wrote
// earlier can hold a sequence the model server would read as structure.
// The model's own earlier text has gnosh's tags made harmless too: a model
// that writes a <gnosh> note of its own (one did: "Done. Nothing left in
// mem.") would read it back later as gnosh's word. The system prompt and
// the tool definitions are gnosh's own and are left as they are. The same
// text always comes out the same, so the bytes of the earlier part of a
// conversation never change from one request to the next.
func defuseRequest(req *core.Request) {
	for i := range req.Messages {
		defuseBlocks(req.Messages[i].Blocks, req.Messages[i].Role == core.RoleAssistant)
	}
}

// defuseBlocks makes blocks harmless; with own, gnosh's tags as well.
func defuseBlocks(blocks []core.Block, own bool) {
	for i := range blocks {
		b := &blocks[i]
		switch b.Type {
		case core.BlockText, core.BlockThinking:
			if own {
				b.Text = prompt.Untrusted(b.Text)
			} else {
				b.Text = prompt.Defuse(b.Text)
			}
		case core.BlockToolUse:
			if out, ok := rewriteJSONStrings(b.Input, prompt.Defuse, func(raw []byte) bool {
				return bytes.IndexByte(raw, '<') >= 0 || containsFold(raw, `<`)
			}); ok {
				b.Input = out
			}
		case core.BlockToolResult:
			defuseBlocks(b.Content, false)
		}
	}
}

// containsFold reports whether raw holds s in any letter case. s is short
// and plain.
func containsFold(raw []byte, s string) bool {
	return bytes.Contains(bytes.ToLower(raw), []byte(strings.ToLower(s)))
}

// rewriteJSONStrings returns raw, one JSON value, with f applied to every
// string in it, keys included. The value is written again token by token,
// so that an object that names a key twice still names it twice: the
// loop's own check for that comes afterwards and must see it. needed is
// asked first whether raw can hold anything f would change; when it says
// no, or raw cannot be read, ok is false and raw is to be used as it is.
func rewriteJSONStrings(raw json.RawMessage, f func(string) string, needed func([]byte) bool) (json.RawMessage, bool) {
	if len(raw) == 0 || !needed(raw) {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	str := func(s string) error {
		if err := enc.Encode(s); err != nil {
			return err
		}
		out.Truncate(out.Len() - 1) // the line break Encode adds
		return nil
	}
	type frame struct {
		object bool
		n      int  // how many members or items have been started
		key    bool // the next string is a key
	}
	var stack []*frame
	// value is called before a value is written: it writes the comma that
	// parts the items of a list, and notes that an object's next string
	// is a key again.
	value := func() {
		if len(stack) == 0 {
			return
		}
		top := stack[len(stack)-1]
		if top.object {
			top.key = true
			return
		}
		if top.n > 0 {
			out.WriteByte(',')
		}
		top.n++
	}
	changed := false
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch v := tok.(type) {
		case json.Delim:
			switch v {
			case '{', '[':
				value()
				out.WriteByte(byte(v))
				stack = append(stack, &frame{object: v == '{', key: v == '{'})
			default:
				if len(stack) == 0 {
					return nil, false
				}
				stack = stack[:len(stack)-1]
				out.WriteByte(byte(v))
			}
		case string:
			s := f(v)
			changed = changed || s != v
			if n := len(stack); n > 0 && stack[n-1].object && stack[n-1].key {
				top := stack[n-1]
				if top.n > 0 {
					out.WriteByte(',')
				}
				top.n++
				top.key = false
				if err := str(s); err != nil {
					return nil, false
				}
				out.WriteByte(':')
				continue
			}
			value()
			if err := str(s); err != nil {
				return nil, false
			}
		case json.Number:
			value()
			out.WriteString(v.String())
		case bool:
			value()
			if v {
				out.WriteString("true")
			} else {
				out.WriteString("false")
			}
		case nil:
			value()
			out.WriteString("null")
		}
	}
	if len(stack) != 0 || !changed || !json.Valid(out.Bytes()) {
		return nil, false
	}
	return out.Bytes(), true
}

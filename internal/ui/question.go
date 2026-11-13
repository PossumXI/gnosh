package ui

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/diff"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/menu"
)

// How a question of the session's becomes a question on the screen.
//
// The session puts every question as an agent.Question, whose Kind says
// what sort it is. Each sort is drawn with the piece of the menu package
// that fits it:
//
//	"permission"  menu.Select: what the call would do (the command, or the
//	              change as a diff drawn to the terminal's width), why gnosh
//	              asks, and the answers the gate offers. "No" is offered
//	              twice: alone, which stops the turn, and with words, which
//	              the model reads before it carries on.
//	"trust"       menu.Select: the facts as the session wrote them, with the
//	              choices in the order given, so that no is first.
//	"question"    menu.Form when Question.Data holds the model's questions
//	              (see formSpec), and otherwise a Select of the choices or,
//	              with none, a menu.Input.
//	"secret"      menu.Secret: a masked line, whose answer goes to the tool
//	              that asked and is kept nowhere here.
//	anything else a Select of the choices, or an Input when there are none.
//
// A question whose Data has a method Deadline() time.Time is taken back by
// its asker at that time, and the screen counts down to it; such a
// question is about something dangerous, so the cursor starts on no.

// The guard that keeps keys typed before a question was seen from
// answering it. A question takes no key until guardShown has passed since
// it was drawn and guardPause since the last key: someone in the middle of
// a sentence goes on typing into the prompt box, and the question waits
// for them to stop.
const (
	guardShown = 400 * time.Millisecond
	guardPause = 600 * time.Millisecond
)

// asker is a question of the menu package, as the session holds one.
type asker interface {
	menu.Prompt
	// SetMaxHeight sets the most rows the question takes.
	SetMaxHeight(rows int)
}

// question is a question that is on the screen or waits to be.
type question struct {
	q agent.Question
	// sub is the id of the sub-agent that asks, or empty.
	sub string
	ui  asker
	// keys are the answer keys of a Select's options, in order.
	keys []string
	// labels are what each option of a Select says, for the record.
	labels []string
	// form holds the questions of a Form, for the answer.
	form []menu.Question
	// secret says the answer must be shown nowhere.
	secret bool
	// opened is when the question was first drawn, and quiet the moment
	// before which keys still go to the prompt box. guarded says that
	// moment has not passed.
	opened  time.Time
	quiet   time.Time
	guarded bool
	// deadline is when the asker takes the question back, or zero.
	deadline time.Time
	// width is the terminal width the question's body was laid out for.
	width int
	// notified says the Notification hooks were told that it waits.
	notified bool
	// answered says an answer was given and its AnswerEvent is on its
	// way.
	answered bool
}

// deadliner is what the Data of a question with a time limit has.
type deadliner interface {
	Deadline() time.Time
}

// menuGutter is how many cells a question keeps at the left of the rows of
// its body, which the body must be laid out for.
func menuGutter(th *Theme, m text.Metrics) int {
	return max(m.Width(th.Menu.Pointer), m.Width(th.Menu.MoreAbove), m.Width(th.Menu.MoreBelow), 1) + 1
}

// newQuestion makes the question on the screen for q, laid out for a
// terminal width cells wide. at is the option the cursor starts on, or -1
// for the one the question itself calls for.
func newQuestion(th *Theme, m text.Metrics, width int, q agent.Question, at int) *question {
	qs := &question{q: q, width: width}
	switch d := q.Data.(type) {
	case deadliner:
		qs.deadline = d.Deadline()
	case time.Time:
		qs.deadline = d
	}
	title := strings.TrimSpace(q.Title)
	if title == "" {
		title, _ = firstLine(q.Text)
	}
	body := func() []string {
		var out []string
		if strings.TrimSpace(q.Text) != "" && q.Text != q.Title {
			for _, l := range strings.Split(strings.TrimRight(q.Text, "\n"), "\n") {
				out = append(out, safeLine(l))
			}
		}
		// The change a person is asked to approve is shown to them: every
		// hunk, drawn to the width the question has for its body, so that
		// its numbers and signs stay in their columns.
		if a := q.Action; a != nil && a.Preview != nil && a.Preview.Kind == "diff" {
			if len(out) > 0 {
				out = append(out, "")
			}
			out = append(out, diff.Render(*a.Preview, diff.RenderOptions{
				Width: max(width-menuGutter(th, m), 8), Profile: th.Profile, Theme: &th.Diff, Metrics: m, MaxLines: 1000,
			})...)
		}
		return out
	}

	switch {
	case q.Kind == "secret":
		qs.secret = true
		qs.ui = menu.NewSecret(menu.SecretOptions{Title: title, Body: body(), Theme: th.Menu, Metrics: m})
	case q.Kind == "question" && formSpecOf(q.Data) != nil:
		spec := formSpecOf(q.Data)
		qs.form = spec.questions
		qs.ui = menu.NewForm(menu.FormOptions{
			Questions: spec.questions, ReviewTitle: spec.reviewTitle, ReviewNote: spec.reviewNote,
			Theme: th.Menu, Metrics: m,
		})
	case len(q.Choices) == 0:
		qs.ui = menu.NewInput(menu.InputOptions{Title: title, Body: body(), Theme: th.Menu, Metrics: m})
	default:
		var opts []menu.Option
		selected, no := 0, -1
		for _, ch := range q.Choices {
			if ch.Key == "no" && q.Kind == "permission" {
				// The gate's one "no" means two things, by whether words
				// come with it. Each gets a row of its own, and Tab on
				// the first opens a note as well.
				no = len(opts)
				opts = append(opts,
					menu.Option{Label: "No", Description: "the call does not run, and the turn stops", Text: menu.TextOptional,
						Placeholder: "or say what to do instead, and the model carries on"},
					menu.Option{Label: "No, and tell the model what to do instead", Text: menu.TextRequired,
						Placeholder: "the model reads this and carries on"})
				qs.keys = append(qs.keys, "no", "no")
				qs.labels = append(qs.labels, "No", "No")
				continue
			}
			if ch.Key == "no" {
				no = len(opts)
			}
			opts = append(opts, menu.Option{Label: ch.Label})
			qs.keys = append(qs.keys, ch.Key)
			qs.labels = append(qs.labels, oneLine(ch.Label))
		}
		if q.Kind == "question" {
			// The model's own question may always be answered in the
			// person's own words.
			opts = append(opts, menu.Option{Label: "Something else", Text: menu.TextRequired, Placeholder: "your answer"})
			qs.keys = append(qs.keys, "")
			qs.labels = append(qs.labels, "")
		}
		if !qs.deadline.IsZero() && no >= 0 {
			selected = no
		}
		if at >= 0 {
			selected = at
		}
		hint := ""
		if q.Kind == "permission" {
			hint = "no"
		}
		qs.ui = menu.NewSelect(menu.SelectOptions{
			Title: title, Body: body(), Options: opts, Selected: selected, CancelHint: hint,
			Theme: th.Menu, Metrics: m,
		})
	}
	return qs
}

// rebuild lays the question out again for a new width, where its body
// was drawn for the old one: a change shown as a diff, whose columns are
// laid out for one width. The cursor stays on its option. A question on
// which a text field is open is left as it is, so that nothing typed is
// lost; its body is then wrapped by the question itself.
func (qs *question) rebuild(th *Theme, m text.Metrics, width int) {
	sel, ok := qs.ui.(*menu.Select)
	if !ok || sel.Typing() || qs.width == width || qs.q.Action == nil || qs.q.Action.Preview == nil {
		return
	}
	fresh := newQuestion(th, m, width, qs.q, sel.Selected())
	qs.ui, qs.width = fresh.ui, width
}

// How a question ended on the screen.
const (
	endAnswered = iota // an answer was chosen or typed
	endRefused         // Esc on a question that takes no for an answer
	endLeft            // Esc on any other question: nothing was answered
)

// answer turns what the menu reported into the session's answer, and into
// the words that stand in the scrollback as the record of it.
func (qs *question) answer(res menu.Result) (ans agent.Answer, record string, how int) {
	if res.Cancelled {
		for _, k := range qs.keys {
			if k == "no" {
				return agent.Answer{Choice: "no"}, "No", endRefused
			}
		}
		return agent.Answer{}, "not answered", endLeft
	}
	switch {
	case qs.secret:
		return agent.Answer{Text: res.Text}, "answered (what was typed is not shown)", endAnswered
	case qs.form != nil:
		byQuestion := map[string]string{}
		var lines []string
		for i, a := range res.Answers {
			if i >= len(qs.form) {
				break
			}
			var parts []string
			for _, p := range a.Picked {
				if p >= 0 && p < len(qs.form[i].Options) {
					parts = append(parts, qs.form[i].Options[p].Label)
				}
			}
			if other := strings.TrimSpace(a.Other); other != "" {
				parts = append(parts, other)
			}
			said := strings.Join(parts, ", ")
			byQuestion[qs.form[i].Text] = said
			if said == "" {
				said = "(no answer)"
			}
			lines = append(lines, firstNonEmpty(qs.form[i].Header, qs.form[i].Text)+": "+said)
		}
		return agent.Answer{Text: strings.Join(lines, "\n"), Data: byQuestion}, strings.Join(lines, "\n"), endAnswered
	case len(qs.keys) > 0 && res.Index >= 0 && res.Index < len(qs.keys):
		words := strings.TrimSpace(res.Text)
		record = qs.labels[res.Index]
		switch {
		case record == "":
			record = words
		case words != "":
			record += ": " + words
		}
		return agent.Answer{Choice: qs.keys[res.Index], Text: words}, record, endAnswered
	}
	return agent.Answer{Text: res.Text}, res.Text, endAnswered
}

// firstNonEmpty returns the first of its arguments that is not empty.
func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// formSpec is the questions of the model's question tool, as its call
// writes them.
//
// The session hands them over in Question.Data: the call's arguments as
// JSON (a json.RawMessage, a []byte or a string) or decoded
// (map[string]any). Both spellings the specs give are read: Claude Code's
// AskUserQuestion ("questions", each with "question", "header", "options"
// of "label" and "description", and "multiSelect") and the earlier gnosh's
// ("label", "type", "choices", "confirm_title", "confirm_description").
// The answer goes back as Answer.Data, a map[string]string from each
// question's text to what was answered (several answers joined with
// commas, as Claude Code does), and as the same in Answer.Text, one line
// for each question.
type formSpec struct {
	questions               []menu.Question
	reviewTitle, reviewNote string
}

// formSpecOf reads the questions out of a question's Data, or returns nil
// when it holds none.
func formSpecOf(data any) *formSpec {
	var raw []byte
	switch d := data.(type) {
	case json.RawMessage:
		raw = d
	case []byte:
		raw = d
	case string:
		raw = []byte(d)
	case map[string]any:
		raw, _ = json.Marshal(d)
	default:
		return nil
	}
	var in struct {
		Questions []struct {
			Question    string            `json:"question"`
			Header      string            `json:"header"`
			Label       string            `json:"label"`
			Description string            `json:"description"`
			Options     []json.RawMessage `json:"options"`
			Choices     []json.RawMessage `json:"choices"`
			MultiSelect bool              `json:"multiSelect"`
			Type        string            `json:"type"`
		} `json:"questions"`
		ConfirmTitle       string `json:"confirm_title"`
		ConfirmDescription string `json:"confirm_description"`
	}
	if json.Unmarshal(raw, &in) != nil || len(in.Questions) == 0 {
		return nil
	}
	spec := &formSpec{reviewTitle: in.ConfirmTitle, reviewNote: in.ConfirmDescription}
	for _, q := range in.Questions {
		mq := menu.Question{
			Header:      firstNonEmpty(q.Header, q.Label),
			Text:        firstNonEmpty(q.Question, q.Label, q.Header),
			Description: q.Description,
		}
		switch strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(q.Type)) {
		case "multi", "multiselect", "multiple", "checkbox":
			mq.Multi = true
		}
		mq.Multi = mq.Multi || q.MultiSelect
		for _, o := range append(q.Options, q.Choices...) {
			var label string
			var full struct {
				Label       string `json:"label"`
				Description string `json:"description"`
			}
			switch {
			case json.Unmarshal(o, &label) == nil:
				mq.Options = append(mq.Options, menu.Option{Label: label})
			case json.Unmarshal(o, &full) == nil && full.Label != "":
				mq.Options = append(mq.Options, menu.Option{Label: full.Label, Description: full.Description})
			}
		}
		spec.questions = append(spec.questions, mq)
	}
	return spec
}

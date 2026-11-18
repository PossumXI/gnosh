package app

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/mcp"
)

// This file puts an MCP server's elicitation to the person (mcp/elicit.go),
// as Claude Code does: the server's message and one question for each field
// it asks for, in the form the model's own questions use (a boolean is Yes
// or No, a field with a list of values offers them, anything else is typed
// in). What comes back is held to the field's kind and bounds; an answer
// that does not fit is said and asked again, twice at most, and then the
// request is declined. Esc on the form cancels it and the turn goes on (the
// server decides what follows), stopping the turn takes the form back (and
// cancels it), and a run with nobody to ask declines it. The server's
// message stands above the questions, under the server's name.

// elicitTries is how many times the form is put before a request whose
// answers do not fit is declined.
const elicitTries = 3

// mcpElicit is the elicitation handler of the server name.
func (l *live) mcpElicit(name string) func(context.Context, mcp.Elicitation) (mcp.ElicitResult, error) {
	return func(ctx context.Context, e mcp.Elicitation) (mcp.ElicitResult, error) {
		if !l.interactive || l.agent == nil {
			return mcp.ElicitResult{Action: "decline"}, nil
		}
		if turn := l.agent.TurnContext(); turn != nil {
			// What a server asks in the middle of a turn is about that
			// turn's work: stopping the turn takes it back.
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			defer context.AfterFunc(turn, cancel)()
		}
		form := elicitForm(e)
		form.Lead = "The MCP server " + name + " asks: " + oneLine(e.Message, 600)
		form.EscCancels = true
		title := "The MCP server " + name + " asks for something"
		if len(e.Fields) == 0 {
			title = "The MCP server " + name + " asks you to confirm"
		}
		for try := 1; ; try++ {
			ans, err := l.agent.Ask(ctx, agent.Question{Kind: questionKind, Title: title, Text: oneLine(e.Message, 600), Data: form})
			switch {
			case errors.Is(err, agent.ErrNobodyToAsk):
				return mcp.ElicitResult{Action: "decline"}, nil
			case err != nil:
				return mcp.ElicitResult{Action: "cancel"}, nil
			}
			if ans.Choice == "cancel" {
				return mcp.ElicitResult{Action: "cancel"}, nil
			}
			values, _ := ans.Data.(map[string]string)
			if len(values) == 0 && strings.TrimSpace(ans.Text) == "" && len(e.Fields) > 0 {
				return mcp.ElicitResult{Action: "cancel"}, nil
			}
			content, why := elicitContent(e.Fields, form, values)
			if why == "" {
				l.flog.logf("mcp", "the MCP server %s was given what it asked for (%d fields)", name, len(content))
				return mcp.ElicitResult{Action: "accept", Content: content}, nil
			}
			if try >= elicitTries {
				l.notice(agent.LevelWarning, "mcp", fmt.Sprintf("What the MCP server %s asked for was declined: %s.", name, why))
				return mcp.ElicitResult{Action: "decline"}, nil
			}
			l.notice(agent.LevelWarning, "mcp", fmt.Sprintf("That does not fit what the MCP server %s asked for: %s. It is asked again.", name, why))
		}
	}
}

// elicitForm is the form an elicitation is put as: one question for each
// field, the field's name as its tab.
func elicitForm(e mcp.Elicitation) QuestionForm {
	var form QuestionForm
	asked := map[string]bool{}
	for _, f := range e.Fields {
		q := FormQuestion{Question: elicitQuestion(f), Header: elicitHeader(f.Name)}
		if asked[q.Question] {
			// (The answers are keyed by the question: two fields that read
			// alike are told apart by their names.)
			q.Question += " [" + oneLine(f.Name, 40) + "]"
		}
		asked[q.Question] = true
		switch {
		case f.Type == "boolean":
			q.Options = []FormOption{{Label: "Yes"}, {Label: "No"}}
		case len(f.Enum) > 0:
			for i, v := range f.Enum {
				label := v
				if i < len(f.EnumNames) && strings.TrimSpace(f.EnumNames[i]) != "" {
					label = f.EnumNames[i]
				}
				q.Options = append(q.Options, FormOption{Label: oneLine(label, 80)})
			}
		}
		form.Questions = append(form.Questions, q)
	}
	if len(form.Questions) == 0 {
		// Nothing to fill in: the question is whether to go on.
		form.Questions = []FormQuestion{{Question: "Go on?", Header: "Confirm", Options: []FormOption{{Label: "Yes"}, {Label: "No"}}}}
	}
	return form
}

// elicitQuestion is a field's question: its title or name, what the server
// says of it, and what kind of answer it takes.
func elicitQuestion(f mcp.ElicitField) string {
	q := oneLine(firstOf(f.Title, f.Name), 120)
	if d := oneLine(f.Description, 300); d != "" {
		q += ": " + d
	}
	var kind []string
	switch f.Type {
	case "number":
		kind = append(kind, "a number")
	case "integer":
		kind = append(kind, "a whole number")
	}
	if f.Format != "" && f.Type == "string" {
		kind = append(kind, "a "+f.Format)
	}
	if f.Minimum != nil {
		kind = append(kind, "at least "+strconv.FormatFloat(*f.Minimum, 'g', -1, 64))
	}
	if f.Maximum != nil {
		kind = append(kind, "at most "+strconv.FormatFloat(*f.Maximum, 'g', -1, 64))
	}
	if !f.Required {
		kind = append(kind, "may be left empty")
	}
	if len(kind) > 0 {
		q += " (" + strings.Join(kind, ", ") + ")"
	}
	return q
}

// elicitHeader is a field's tab: its name, cut to twelve characters.
func elicitHeader(name string) string {
	if utf8.RuneCountInString(name) <= 12 {
		return name
	}
	return string([]rune(name)[:11]) + "…"
}

// elicitContent turns the answers into the values the server asked for,
// each of its field's kind, or says why one does not fit.
func elicitContent(fields []mcp.ElicitField, form QuestionForm, values map[string]string) (map[string]any, string) {
	content := map[string]any{}
	if len(fields) == 0 {
		// A confirmation: Yes accepts.
		if strings.TrimSpace(values[form.Questions[0].Question]) != "Yes" {
			return nil, "it was not confirmed"
		}
		return content, ""
	}
	for i, f := range fields {
		q := form.Questions[i]
		got := strings.TrimSpace(values[q.Question])
		name := firstOf(f.Title, f.Name)
		if got == "" {
			if f.Required {
				return nil, name + " is needed"
			}
			continue
		}
		switch f.Type {
		case "boolean":
			switch strings.ToLower(got) {
			case "yes", "true":
				content[f.Name] = true
			case "no", "false":
				content[f.Name] = false
			default:
				return nil, name + " is Yes or No"
			}
		case "number", "integer":
			n, err := strconv.ParseFloat(got, 64)
			if err != nil || f.Type == "integer" && n != float64(int64(n)) {
				return nil, name + " is " + map[string]string{"number": "a number", "integer": "a whole number"}[f.Type]
			}
			if f.Minimum != nil && n < *f.Minimum || f.Maximum != nil && n > *f.Maximum {
				return nil, name + " is out of its bounds"
			}
			if f.Type == "integer" {
				content[f.Name] = int64(n)
			} else {
				content[f.Name] = n
			}
		default:
			value := got
			if len(f.Enum) > 0 {
				value = ""
				for j, v := range f.Enum {
					label := v
					if j < len(f.EnumNames) && strings.TrimSpace(f.EnumNames[j]) != "" {
						label = f.EnumNames[j]
					}
					if got == oneLine(label, 80) || got == v {
						value = v
					}
				}
				if value == "" {
					return nil, name + " is one of " + strings.Join(f.Enum, ", ")
				}
			}
			n := utf8.RuneCountInString(value)
			if f.MinLength != nil && n < *f.MinLength || f.MaxLength != nil && n > *f.MaxLength {
				return nil, name + " is not of the length it takes"
			}
			if why := elicitFormat(f.Format, value); why != "" {
				return nil, name + " " + why
			}
			content[f.Name] = value
		}
	}
	return content, ""
}

// elicitFormat says why a string is not of its format, or "".
func elicitFormat(format, v string) string {
	switch format {
	case "email":
		if a, err := mail.ParseAddress(v); err != nil || a.Address != v {
			return "is an email address"
		}
	case "uri":
		if u, err := url.Parse(v); err != nil || u.Scheme == "" {
			return "is an address with a scheme (https://...)"
		}
	case "date":
		if _, err := time.Parse("2006-01-02", v); err != nil {
			return "is a date, as 2026-11-18"
		}
	case "date-time":
		if _, err := time.Parse(time.RFC3339, v); err != nil {
			return "is a date and time, as 2026-11-18T09:30:00Z"
		}
	}
	return ""
}

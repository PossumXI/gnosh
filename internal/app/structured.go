package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/jsonschema"
)

// This file is --json-schema (spec/cc/cli.md 3.2, output-formats.md 4.5):
// once a run's turn has ended well, the model is asked, beside the
// session's request as /btw asks (agent.SideRequest.Session, so that a
// local server reads only the question), for the result as one JSON value
// that conforms to the schema. What it gives is checked against the schema
// and asked for again, with what was wrong, up to structuredTries times in
// all. The value goes in the result's structured_output; a run that never
// gets one ends with error_max_structured_output_retries.

// structuredTries is how many times the value is asked for.
const structuredTries = 3

// structurer is what a session's controls may also be: able to ask the
// model for the run's result as JSON.
type structurer interface {
	Structured(ctx context.Context, schema, problem string) (string, error)
}

// Structured asks the session's model for the result as JSON that
// conforms to schema; problem is what was wrong with the last value.
func (c *controls) Structured(ctx context.Context, schema, problem string) (string, error) {
	l := c.cur()
	res, err := l.agent.Side(ctx, agent.SideRequest{
		Purpose: "structured-output", Session: true, Prompt: l.texts.StructuredOutput(schema, problem),
		MaxTokens: 16384, Timeout: btwWait,
	})
	return res.Text, err
}

// structuredOutput asks for the run's result as JSON that conforms to the
// schema, and checks it, up to structuredTries times. It returns the
// value, or what was wrong with the last one.
func (f *printer) structuredOutput(ctx context.Context, c Controls) (json.RawMessage, string) {
	s, err := jsonschema.Compile(f.inv.JSONSchema)
	if err != nil {
		return nil, "--json-schema is not a valid JSON Schema: " + err.Error()
	}
	st, ok := c.(structurer)
	if !ok {
		return nil, "this session cannot be asked for structured output"
	}
	problem := ""
	for try := 0; try < structuredTries; try++ {
		text, err := st.Structured(ctx, f.inv.JSONSchema, problem)
		if err != nil {
			problem = "the model could not be asked: " + err.Error()
			if ctx.Err() != nil {
				break
			}
			continue
		}
		raw, value, err := firstJSON(text)
		if err != nil {
			problem = err.Error()
			continue
		}
		if err := s.Validate(value); err != nil {
			problem = err.Error()
			continue
		}
		return raw, ""
	}
	return nil, fmt.Sprintf("no value that conforms to the schema came in %d tries; the last: %s", structuredTries, problem)
}

// firstJSON finds the JSON value in a model's answer: all of it, or what a
// code fence holds, or what runs from the first { or [ to the last } or ].
func firstJSON(text string) (json.RawMessage, any, error) {
	text = strings.TrimSpace(text)
	tries := []string{text}
	if i := strings.Index(text, "```"); i >= 0 {
		rest := text[i+3:]
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		if j := strings.Index(rest, "```"); j >= 0 {
			tries = append(tries, strings.TrimSpace(rest[:j]))
		}
	}
	for _, pair := range [][2]string{{"{", "}"}, {"[", "]"}} {
		i, j := strings.Index(text, pair[0]), strings.LastIndex(text, pair[1])
		if i >= 0 && j > i {
			tries = append(tries, text[i:j+1])
		}
	}
	for _, t := range tries {
		if v, err := jsonschema.Decode([]byte(t)); err == nil {
			var buf bytes.Buffer
			if json.Compact(&buf, []byte(t)) == nil {
				return json.RawMessage(buf.Bytes()), v, nil
			}
		}
	}
	if text == "" {
		return nil, nil, errors.New("the answer was empty")
	}
	return nil, nil, errors.New("the answer holds no JSON value")
}

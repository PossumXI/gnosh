// Copyright (C) 2026 Jae Kwon
//
// This file is part of gnosh's GnoVM machine subsystem. It is licensed under
// the GNO Network General Public License, version 6, as published by
// NewTendermint, LLC or its successor, or any later version of that License
// that the Licensor accepts as the proxy under its section 14, by a statement
// published in the canonical copy of internal/gnovm/LICENSE.md; together with
// the Additional Terms (Strong Attribution) in that file.

package gnovmtools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// A tool's arguments are declared once, as a struct (RunParams, NewParams,
// ListParams, DropParams): a field's json tag is the argument's name, its
// type the argument's type, and its description tag what the model is told
// of it. This file reads both things off such a struct: the schema the
// model is given, and a call's arguments.

// schemaOf is the JSON Schema of a tool's arguments, from the struct that
// holds them. A string field is a string, a float64 a number and a bool a
// boolean, behind a pointer or not. A field whose json tag does not say
// omitempty is required. An enum tag lists, between commas, the values a
// field takes. The properties stand in the struct's order.
func schemaOf(params any) json.RawMessage {
	t := reflect.TypeOf(params)
	required := []string{}
	var b bytes.Buffer
	b.WriteString(`{"type":"object","properties":{`)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(jsonText(name))
		b.WriteString(`:{"type":`)
		b.Write(jsonText(schemaType(f.Type)))
		if about := f.Tag.Get("description"); about != "" {
			b.WriteString(`,"description":`)
			b.Write(jsonText(about))
		}
		if enum := f.Tag.Get("enum"); enum != "" {
			b.WriteString(`,"enum":`)
			b.Write(jsonText(strings.Split(enum, ",")))
		}
		b.WriteByte('}')
		if !strings.Contains(","+opts+",", ",omitempty,") {
			required = append(required, name)
		}
	}
	b.WriteString(`},"required":`)
	b.Write(jsonText(required))
	b.WriteString(`,"additionalProperties":false}`)
	return b.Bytes()
}

// schemaType is the JSON Schema type of an argument held in a field of
// type t.
func schemaType(t reflect.Type) string {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	}
	return "string"
}

// jsonText is v as JSON, with <, > and & left as they are.
func jsonText(v any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		// It is given strings and lists of strings, which always encode.
		return []byte(`""`)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// decodeParams reads a call's arguments into params, a pointer to the
// struct that holds the tool's arguments. No arguments at all, and null,
// leave every field as it is; so does a field that is left out or null. A
// field the tool does not take is passed over. A number or a boolean
// written in quotes is read too, as models now and then write them. The
// error is worded for the model and is about the first field that cannot
// be read; every field that can be is set all the same.
func decodeParams(raw json.RawMessage, params any) error {
	var fields map[string]json.RawMessage
	if raw = bytes.TrimSpace(raw); len(raw) > 0 {
		if err := json.Unmarshal(raw, &fields); err != nil {
			return errors.New("the arguments must be one JSON object")
		}
	}
	var first error
	v := reflect.ValueOf(params).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		val := bytes.TrimSpace(fields[name])
		if len(val) == 0 || bytes.Equal(val, []byte("null")) {
			continue
		}
		kind := t.Field(i).Type
		if kind.Kind() == reflect.Pointer {
			kind = kind.Elem()
		}
		got := reflect.New(kind).Elem()
		var err error
		switch kind.Kind() {
		case reflect.String:
			var s string
			if json.Unmarshal(val, &s) != nil {
				err = fmt.Errorf("%s must be a string", name)
			}
			got.SetString(s)
		case reflect.Float64:
			n, ok := numberArg(val)
			if !ok {
				err = fmt.Errorf("%s must be a number", name)
			}
			got.SetFloat(n)
		case reflect.Bool:
			b, ok := boolArg(val)
			if !ok {
				err = fmt.Errorf("%s must be true or false", name)
			}
			got.SetBool(b)
		}
		if err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if f := v.Field(i); f.Kind() == reflect.Pointer {
			f.Set(got.Addr())
		} else {
			f.Set(got)
		}
	}
	return first
}

// numberArg reads a JSON number, or a string that holds one: a model writes
// 1e9, and now and then "1e9". A number too large for a float64 is read as
// a very large one, far past anything a setting takes, so that it is
// refused for its size by whoever checks the range.
func numberArg(val []byte) (float64, bool) {
	text := string(val)
	if val[0] == '"' {
		if json.Unmarshal(val, &text) != nil {
			return 0, false
		}
		text = strings.TrimSpace(text)
	}
	n, err := strconv.ParseFloat(text, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) || math.IsNaN(n) {
		return 0, false
	}
	return min(max(n, -1e18), 1e18), true
}

// boolArg reads true or false, bare or in quotes.
func boolArg(val []byte) (bool, bool) {
	text := string(val)
	if val[0] == '"' {
		if json.Unmarshal(val, &text) != nil {
			return false, false
		}
		text = strings.ToLower(strings.TrimSpace(text))
	}
	switch text {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

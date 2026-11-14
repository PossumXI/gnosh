package config

import (
	"fmt"
	"log/slog"
	"strconv"
)

// Layer is one of the six places a setting can come from. Each outranks the
// ones before it.
type Layer int

const (
	// LayerDefault is gnosh's built-in default.
	LayerDefault Layer = iota
	// LayerUser is the user's own gnosh.json.
	LayerUser
	// LayerState is the user's state file: what was chosen inside gnosh
	// with /model, /think and /autocompact.
	LayerState
	// LayerProject is a file of the project's: its gnosh.json, or the
	// settings file in its data folder.
	LayerProject
	// LayerEnv is an environment variable.
	LayerEnv
	// LayerFlag is the command line. It holds for the one run and is never
	// saved.
	LayerFlag
)

// String names the layer in a word or two.
func (l Layer) String() string {
	switch l {
	case LayerUser:
		return "user"
	case LayerState:
		return "state"
	case LayerProject:
		return "project"
	case LayerEnv:
		return "environment"
	case LayerFlag:
		return "command line"
	}
	return "default"
}

// Origin says where a value came from, for /status and doctor.
type Origin struct {
	Layer Layer
	// File is the file that set the value, when a file did.
	File string
	// Line is the line in File, counted from 1, or 0.
	Line int
	// Name is the flag ("--model") or the variable ("$API_TIMEOUT_MS") that
	// set the value, when one did.
	Name string
}

// String is short enough for one column: "default", "gnosh.json:3",
// "$API_TIMEOUT_MS", "--model".
func (o Origin) String() string {
	switch {
	case o.File != "" && o.Line > 0:
		return o.File + ":" + strconv.Itoa(o.Line)
	case o.File != "":
		return o.File
	case o.Name != "":
		return o.Name
	}
	return o.Layer.String()
}

// Value is a setting together with where it came from.
type Value[T any] struct {
	// V is the value in force. When nothing set it, V is the default, or
	// the zero value for a setting that has none.
	V T
	// Set reports whether something other than the default set V.
	Set bool
	// Origin says what set V.
	Origin Origin
}

// put sets the value from o. A setting from a weaker layer than the one in
// force is left alone, so the order in which layers are read never decides.
func (v *Value[T]) put(x T, o Origin) {
	if v.Set && o.Layer < v.Origin.Layer {
		return
	}
	v.V, v.Set, v.Origin = x, true, o
}

// Secret is a value that is never shown: a key, a header, a variable for a
// command. Printing one with any fmt verb, logging it, or encoding it as
// JSON gives "[hidden]". Reveal returns the value itself.
type Secret struct {
	v string
}

// hidden is what a Secret prints as.
const hidden = "[hidden]"

// NewSecret wraps s.
func NewSecret(s string) Secret { return Secret{v: s} }

// Reveal returns the value. Only the code that sends it where it belongs
// should call this.
func (s Secret) Reveal() string { return s.v }

// Empty reports whether there is no value.
func (s Secret) Empty() bool { return s.v == "" }

// String is "[hidden]", or "" for an empty Secret.
func (s Secret) String() string {
	if s.v == "" {
		return ""
	}
	return hidden
}

// GoString keeps %#v from showing the value.
func (s Secret) GoString() string { return "config.Secret{" + s.String() + "}" }

// Format keeps every fmt verb from showing the value.
func (s Secret) Format(f fmt.State, verb rune) { fmt.Fprint(f, s.String()) }

// MarshalText keeps encoders from writing the value.
func (s Secret) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// LogValue keeps log/slog from writing the value.
func (s Secret) LogValue() slog.Value { return slog.StringValue(s.String()) }

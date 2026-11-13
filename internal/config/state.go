package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
)

// State is what the user chose inside gnosh, kept between sessions: the
// models picked with /model, the thinking level picked for each model with
// /think, /no_think or /model, and the window picked with /autocompact. It
// lives in the user's state directory, never in a project, and it outranks
// the user's gnoshrc: a choice made inside gnosh sticks. A model given on
// the command line is never written here.
type State struct {
	// Large and Small are the models chosen, as provider/id. Empty means
	// no choice.
	Large, Small string
	// Thinking is the level chosen for each model, keyed by provider/id.
	Thinking map[string]string
	// AutoCompactWindow is the window chosen, in tokens. Zero means no
	// choice.
	AutoCompactWindow int

	// extra holds keys this gnosh does not know, so that writing the file
	// back does not drop what another version put there.
	extra map[string]json.RawMessage
}

// stateVersion is the version written to the state file.
const stateVersion = 1

// stateFile is the state file's shape on disk.
type stateFile struct {
	Version int `json:"version"`
	Models  struct {
		Large string `json:"large,omitempty"`
		Small string `json:"small,omitempty"`
	} `json:"models"`
	Thinking          map[string]string `json:"thinking,omitempty"`
	AutoCompactWindow int               `json:"autoCompactWindow,omitempty"`
}

// stateKeys are the keys of stateFile, which extra never holds.
var stateKeys = []string{"version", "models", "thinking", "autoCompactWindow"}

// ReadState reads the state file. A file that is not there gives an empty
// State. A file that cannot be read or understood also gives an empty
// State, with a warning that says so: what gnosh wrote itself must never
// stop it from starting.
func ReadState(fsys FS, path string) (State, []Problem) {
	data, err := fsys.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return State{}, nil
	}
	warn := func(format string, args ...any) (State, []Problem) {
		return State{}, []Problem{{Severity: Warning, File: path, Message: fmt.Sprintf(format, args...)}}
	}
	if err != nil {
		return warn("the state file could not be read (%v), so the model and thinking level chosen earlier are not in force. Choose them again with /model.", err)
	}
	var f stateFile
	if err := json.Unmarshal(data, &f); err != nil {
		return warn("the state file is not JSON gnosh can read, so the model and thinking level chosen earlier are not in force. Choosing them again with /model writes a new file.")
	}
	st := State{Large: f.Models.Large, Small: f.Models.Small, Thinking: f.Thinking, AutoCompactWindow: f.AutoCompactWindow}
	var all map[string]json.RawMessage
	if json.Unmarshal(data, &all) == nil {
		for _, k := range stateKeys {
			delete(all, k)
		}
		if len(all) > 0 {
			st.extra = all
		}
	}
	return st, nil
}

// check says what is wrong with a State that is about to be written, or
// nil. A writer is held to what the reader accepts.
func (st State) check() error {
	for _, m := range []string{st.Large, st.Small} {
		if m != "" && !validModelRef(m) {
			return fmt.Errorf("%q is not a model name of the form provider/id", m)
		}
	}
	for m, level := range st.Thinking {
		if !validModelRef(m) {
			return fmt.Errorf("%q is not a model name of the form provider/id", m)
		}
		if !levelPattern.MatchString(level) {
			return fmt.Errorf("%q is not a thinking level", level)
		}
	}
	if w := st.AutoCompactWindow; w != 0 && (w < 100000 || w > 1000000) {
		return fmt.Errorf("the auto-compact window %d is not between 100000 and 1000000", w)
	}
	return nil
}

// validModelRef reports whether s has the form provider/id.
func validModelRef(s string) bool {
	prov, id, ok := strings.Cut(s, "/")
	return ok && idPattern.MatchString(prov) && id != "" && !strings.ContainsAny(id, " \t\n")
}

// WriteState writes the state file in one step, making its directory when
// it is not there. It refuses a State the reader would not accept, and
// then nothing is written.
func WriteState(fsys WriteFS, path string, st State) error {
	if err := st.check(); err != nil {
		return fmt.Errorf("the state file %s was not written: %w", path, err)
	}
	var f stateFile
	f.Version = stateVersion
	f.Models.Large, f.Models.Small = st.Large, st.Small
	f.Thinking, f.AutoCompactWindow = st.Thinking, st.AutoCompactWindow
	known, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("the state file %s was not written: %w", path, err)
	}
	all := map[string]json.RawMessage{}
	if err := json.Unmarshal(known, &all); err != nil {
		return fmt.Errorf("the state file %s was not written: %w", path, err)
	}
	for k, v := range st.extra {
		if !slices.Contains(stateKeys, k) {
			all[k] = v
		}
	}
	// A map is written with its keys in order, so the same State always
	// gives the same bytes.
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return fmt.Errorf("the state file %s was not written: %w", path, err)
	}
	if err := fsys.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("the state file %s was not written: its directory could not be made: %w", path, err)
	}
	if err := fsys.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("the state file %s was not written: %w", path, err)
	}
	return nil
}

// UpdateState changes the state file: it reads the file as it is now,
// hands the State to change, and writes the result. Reading again just
// before writing keeps what another session saved a moment ago. If the
// file could not be understood, change starts from an empty State and the
// unreadable file is replaced.
func UpdateState(fsys WriteFS, path string, change func(*State)) error {
	st, _ := ReadState(fsys, path)
	st.Thinking = maps.Clone(st.Thinking)
	change(&st)
	return WriteState(fsys, path, st)
}

// SetModel records a model choice for a slot, "large" or "small". An empty
// ref clears the choice.
func (st *State) SetModel(slot string, ref ModelRef) error {
	name := ""
	if ref.ID != "" {
		name = ref.String()
		if !validModelRef(name) {
			return fmt.Errorf("%q is not a model name of the form provider/id", name)
		}
	}
	switch slot {
	case "large":
		st.Large = name
	case "small":
		st.Small = name
	default:
		return fmt.Errorf("%q is not a model slot; the slots are large and small", slot)
	}
	return nil
}

// SetThinking records the thinking level chosen for a model. An empty
// level clears the choice.
func (st *State) SetThinking(ref ModelRef, level string) error {
	name := ref.String()
	if !validModelRef(name) {
		return fmt.Errorf("%q is not a model name of the form provider/id", name)
	}
	if level == "" {
		delete(st.Thinking, name)
		return nil
	}
	if !levelPattern.MatchString(level) {
		return fmt.Errorf("%q is not a thinking level", level)
	}
	if st.Thinking == nil {
		st.Thinking = map[string]string{}
	}
	st.Thinking[name] = level
	return nil
}

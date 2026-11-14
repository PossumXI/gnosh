package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	// The real file system, in a directory of the test's own: the write
	// must be a rename, and must make its directory.
	path := filepath.Join(t.TempDir(), "state", "gnosh", "state.json")
	want := State{Large: "qwen/qwen3.8-27b", Small: "qwen/qwen3.8-27b", Thinking: map[string]string{"qwen/qwen3.8-27b": "off", "gptoss/gpt-oss-120b": "low"}, AutoCompactWindow: 200000}
	if err := WriteState(OS{}, path, want); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	got, problems := ReadState(OS{}, path)
	if len(problems) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("read back %+v with %v, want %+v", got, problems, want)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the state file is the user's alone: %v %v", info.Mode(), err)
	}
	// Nothing is left beside it: the temporary file was renamed into
	// place.
	names, _ := os.ReadDir(filepath.Dir(path))
	if len(names) != 1 || names[0].Name() != "state.json" {
		t.Errorf("files left in the state directory: %v", names)
	}
	// Writing the same State gives the same bytes.
	first, _ := os.ReadFile(path)
	if err := WriteState(OS{}, path, want); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Errorf("the same State gave different bytes:\n%s\n%s", first, second)
	}

	// A file that is not there is an empty State and no problem.
	got, problems = ReadState(OS{}, filepath.Join(t.TempDir(), "nope.json"))
	if len(problems) != 0 || !reflect.DeepEqual(got, State{}) {
		t.Errorf("no file: %+v %v", got, problems)
	}
}

// A state file written by another version of gnosh keeps the keys this
// one does not know.
func TestStateKeepsUnknownKeys(t *testing.T) {
	fsys := newFS().put(statePath, `{"version": 1, "models": {"large": "p/m"}, "recentModels": ["p/a", "p/b"], "future": {"x": 1}}`)
	err := UpdateState(fsys, statePath, func(st *State) {
		if st.Large != "p/m" {
			t.Errorf("UpdateState hands over the file as it is: %+v", st)
		}
		if err := st.SetThinking(ModelRef{Provider: "p", ID: "m"}, "off"); err != nil {
			t.Error(err)
		}
	})
	if err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	data, _ := fsys.ReadFile(statePath)
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		t.Fatalf("the state file is not JSON: %v\n%s", err, data)
	}
	if string(all["recentModels"]) != `["p/a","p/b"]` && !strings.Contains(string(all["recentModels"]), `"p/a"`) || all["future"] == nil {
		t.Errorf("unknown keys were dropped: %s", data)
	}
	st, _ := ReadState(fsys, statePath)
	if st.Large != "p/m" || st.Thinking["p/m"] != "off" {
		t.Errorf("after the update: %+v", st)
	}
}

// What gnosh wrote itself never stops it from starting: a state file that
// cannot be understood, or that names things that are gone, is passed
// over with a word.
func TestStaleStateNeverStopsStartup(t *testing.T) {
	const text = "{" + one + `, "models": {"p/m": {"thinkingLevels": ["off", "on"]}}, "model": "p/m"}`
	cases := []struct {
		name, state string
		pieces      []string
		check       func(*Config)
	}{
		{"not JSON", "{{{", []string{"not JSON gnosh can read", "not in force"}, nil},
		{"the wrong shape", `{"models": "p/m"}`, []string{"not JSON gnosh can read"}, nil},
		{"a provider that is gone", `{"models": {"large": "gone/m"}}`, []string{"gone/m", "provider that is not configured", "Choose a model"},
			func(c *Config) {
				if m := c.Large.Model; m.V.String() != "p/m" || m.Origin.Layer != LayerUser {
					t.Errorf("a stale choice is dropped, and the user's own file counts again: %+v", m)
				}
			}},
		{"a model name of the wrong form", `{"models": {"small": "justaname"}}`, []string{"not of the form provider/id"}, nil},
		{"a window out of range", `{"autoCompactWindow": 7}`, []string{"not between 100000 and 1000000"}, nil},
		{"a level the model does not have", `{"thinking": {"p/m": "extreme"}}`, []string{"extreme", "not one of that model's levels"},
			func(c *Config) {
				if c.Large.Thinking.V != "on" || c.Large.Thinking.Set {
					t.Errorf("a stale level is dropped: %+v", c.Large.Thinking)
				}
			}},
		{"a level that is not a word", `{"thinking": {"p/m": "LOUD!"}}`, []string{"not a level gnosh can read"}, nil},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfg, problems := load(t, inputs(newFS().put(userJSON, text).put(statePath, tt.state)))
			if cfg == nil {
				t.Fatalf("a stale state file stopped startup:\n%s", showProblems(problems))
			}
			wantProblem(t, problems, Warning, statePath, 0, tt.pieces...)
			if tt.check != nil {
				tt.check(cfg)
			}
		})
	}
}

// The thinking level chosen inside gnosh is kept per model, and sits
// between the user's gnosh.json and the project's.
func TestStateThinking(t *testing.T) {
	const text = "{" + one + `, "model": {"id": "p/m", "thinking": true}}`
	fsys := newFS().put(userJSON, text).put(statePath, `{"thinking": {"p/m": "off", "p/other": "low"}}`)
	cfg := mustLoad(t, inputs(fsys))
	if th := cfg.Large.Thinking; th.V != "off" || th.Origin.Layer != LayerState {
		t.Errorf("state over the user's gnosh.json: %+v", th)
	}
	fsys.put(projJSON, `{"model": {"thinking": true}}`)
	cfg = mustLoad(t, inputs(fsys))
	if th := cfg.Large.Thinking; th.V != "on" || th.Origin.Layer != LayerProject {
		t.Errorf("the project's gnosh.json over state: %+v", th)
	}
	in := inputs(fsys)
	in.Flags.Model = "p/other"
	cfg = mustLoad(t, in)
	if th := cfg.Large.Thinking; th.V != "on" {
		// The project's file set the level, which outranks the state.
		t.Errorf("another model, the project's level still holds: %+v", th)
	}
	in = inputs(newFS().put(userJSON, "{"+one+"}").put(statePath, `{"thinking": {"p/m": "off", "p/other": "low"}}`))
	in.Flags.Model = "p/other"
	cfg = mustLoad(t, in)
	if th := cfg.Large.Thinking; th.V != "low" || th.Origin.Layer != LayerState {
		t.Errorf("the level saved for the model in use: %+v", th)
	}
}

// A writer is held to what the reader accepts: a State the loader would
// drop is not written.
func TestWriteStateRefusesWhatTheReaderWould(t *testing.T) {
	fsys := newFS()
	for name, st := range map[string]State{
		"a model with no provider": {Large: "justaname"},
		"a level that is no word":  {Thinking: map[string]string{"p/m": "LOUD!"}},
		"a window out of range":    {AutoCompactWindow: 5},
	} {
		if err := WriteState(fsys, statePath, st); err == nil {
			t.Errorf("%s: written", name)
		}
	}
	if _, err := fsys.Stat(statePath); err == nil {
		t.Errorf("a refused State left a file behind")
	}
	var st State
	if err := st.SetModel("medium", ModelRef{Provider: "p", ID: "m"}); err == nil {
		t.Errorf("a slot other than large or small was taken")
	}
	if err := st.SetModel("large", ModelRef{ID: "m"}); err == nil {
		t.Errorf("a model with no provider was taken")
	}
	if err := st.SetModel("large", ModelRef{Provider: "p", ID: "m"}); err != nil || st.Large != "p/m" {
		t.Errorf("SetModel: %v %+v", err, st)
	}
	if err := st.SetModel("large", ModelRef{}); err != nil || st.Large != "" {
		t.Errorf("an empty model clears the choice: %v %+v", err, st)
	}
	if err := st.SetThinking(ModelRef{Provider: "p", ID: "m"}, "LOUD"); err == nil {
		t.Errorf("a level that is no word was taken")
	}
}

// Several sessions saving at once never leave a file that cannot be read.
func TestStateWritersAtOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				err := UpdateState(OS{}, path, func(st *State) {
					_ = st.SetThinking(ModelRef{Provider: "p", ID: string(rune('a' + i))}, "off")
				})
				if err != nil {
					t.Error(err)
				}
				if _, problems := ReadState(OS{}, path); len(problems) != 0 {
					t.Errorf("a reader saw a state file it could not read: %v", problems)
				}
			}
		}()
	}
	wg.Wait()
	st, problems := ReadState(OS{}, path)
	if len(problems) != 0 || len(st.Thinking) == 0 {
		t.Errorf("after the writers: %+v %v", st, problems)
	}
	names, _ := os.ReadDir(filepath.Dir(path))
	if len(names) != 1 {
		t.Errorf("temporary files were left behind: %v", names)
	}
}

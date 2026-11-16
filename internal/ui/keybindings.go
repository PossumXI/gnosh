package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gnosh-ai/gnosh/internal/agent"
	"github.com/gnosh-ai/gnosh/internal/app"
	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/ui/editor"
)

// This file is Claude Code's keybindings.json, for the parts of the
// interface gnosh has. A key the file binds to an action is taken as the
// key gnosh knows that action by, null takes a key away, and
// command:<name> runs the slash command /<name>. The contexts are Global,
// Chat (with Task, whose one action gnosh takes in the box) and Transcript;
// while a question is open no key is changed, so that an answer is always
// given with the keys the question shows. The file is read again when it
// changes, and what in it cannot be used is said once.

// keyAction is an action of Claude Code's keybindings that gnosh carries
// out: the keys gnosh's own handling knows it by, one key or Ctrl+X and a
// second.
type keyAction []term.Key

// The keys gnosh knows the actions by.
func ctrlKey(r rune) term.Key { return term.Key{Code: term.KeyRune, Rune: r, Mod: term.ModCtrl} }
func altKey(r rune) term.Key  { return term.Key{Code: term.KeyRune, Rune: r, Mod: term.ModAlt} }
func namedKey(c term.KeyCode, m term.Mod) term.Key {
	return term.Key{Code: c, Mod: m}
}

// keyActions are the actions gnosh carries out, by Claude Code's names.
var keyActions = map[string]keyAction{
	"app:redraw":               {ctrlKey('l')},
	"app:toggleTodos":          {ctrlKey('t')},
	"app:toggleTranscript":     {ctrlKey('o')},
	"history:search":           {ctrlKey('r')},
	"history:previous":         {namedKey(term.KeyUp, 0)},
	"history:next":             {namedKey(term.KeyDown, 0)},
	"chat:cancel":              {namedKey(term.KeyEscape, 0)},
	"chat:clearInput":          {ctrlKey('l')},
	"chat:killAgents":          {ctrlKey('x'), ctrlKey('k')},
	"chat:cycleMode":           {namedKey(term.KeyTab, term.ModShift)},
	"chat:modelPicker":         {altKey('p')},
	"chat:thinkingToggle":      {altKey('t')},
	"chat:submit":              {namedKey(term.KeyEnter, 0)},
	"chat:sendNow":             {namedKey(term.KeyEnter, term.ModCtrl)},
	"chat:newline":             {ctrlKey('j')},
	"chat:undo":                {ctrlKey('_')},
	"chat:externalEditor":      {ctrlKey('g')},
	"chat:stash":               {ctrlKey('s')},
	"task:background":          {ctrlKey('b')},
	"transcript:exit":          {{Code: term.KeyRune, Rune: 'q', Text: "q"}},
	"transcript:toggleShowAll": {ctrlKey('e')},
}

// keyContexts are the contexts gnosh takes bindings for, and the one each
// stands for here.
var keyContexts = map[string]string{"Global": "Global", "Chat": "Chat", "Task": "Chat", "Transcript": "Transcript"}

// reservedKeys are the keys no binding takes: Ctrl+C and Ctrl+D always do
// what they do, and the others arrive as Enter, Esc, Tab and Backspace.
var reservedKeys = []term.Key{ctrlKey('c'), ctrlKey('d'), ctrlKey('m'), ctrlKey('['), ctrlKey('i'), ctrlKey('h')}

// keyBindings is what keybindings.json says, by context and key: the action
// a key does, "" for a key taken away.
type keyBindings struct {
	byCtx map[string]map[term.Key]string
}

// keyFile is the file as it was last read.
type keyFile struct {
	path    string
	mod     time.Time
	size    int64
	checked time.Time
	b       keyBindings
}

// canonKey is a key as bindings are looked up by: no lock keys, and Shift
// with a letter as the capital letter. (A letter held with Ctrl arrives in
// lower case, and a binding is read in lower case.)
func canonKey(k term.Key) term.Key {
	k.Mod &^= term.ModCapsLock | term.ModNumLock
	k.Repeat, k.Text = false, ""
	if k.Code == term.KeyRune {
		if k.Mod&term.ModShift != 0 && unicode.IsLetter(k.Rune) {
			k.Rune, k.Mod = unicode.ToUpper(k.Rune), k.Mod&^term.ModShift
		}
	}
	return k
}

// strokeNames are the names of keys that are not characters.
var strokeNames = map[string]term.KeyCode{
	"escape": term.KeyEscape, "esc": term.KeyEscape, "enter": term.KeyEnter, "return": term.KeyEnter,
	"tab": term.KeyTab, "up": term.KeyUp, "down": term.KeyDown, "left": term.KeyLeft, "right": term.KeyRight,
	"pageup": term.KeyPageUp, "pagedown": term.KeyPageDown, "home": term.KeyHome, "end": term.KeyEnd,
	"backspace": term.KeyBackspace, "delete": term.KeyDelete, "insert": term.KeyInsert,
}

// parseStroke reads one keystroke as Claude Code writes it: modifiers and a
// key joined by +, in any case ("ctrl+shift+c", "meta+p", "escape", "K").
func parseStroke(s string) (term.Key, error) {
	parts := strings.Split(strings.ToLower(s), "+")
	name := parts[len(parts)-1]
	if name == "" && len(parts) > 1 && parts[len(parts)-2] == "" {
		// "ctrl++": the plus key.
		name, parts = "+", parts[:len(parts)-1]
	}
	var mod term.Mod
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "ctrl", "control":
			mod |= term.ModCtrl
		case "shift":
			mod |= term.ModShift
		case "alt", "opt", "option", "meta":
			mod |= term.ModAlt
		case "cmd", "command", "super", "win":
			mod |= term.ModSuper
		default:
			return term.Key{}, fmt.Errorf("%q is no modifier (ctrl, shift, alt or meta, cmd)", m)
		}
	}
	k := term.Key{Mod: mod}
	code, named := strokeNames[name]
	switch {
	case name == "":
		return term.Key{}, errors.New("no key is named")
	case name == "space":
		k.Code, k.Rune = term.KeyRune, ' '
	case named:
		k.Code = code
	case len(name) > 1 && name[0] == 'f' && strings.Trim(name[1:], "0123456789") == "":
		var n int
		fmt.Sscanf(name[1:], "%d", &n)
		if n < 1 || n > 35 {
			return term.Key{}, fmt.Errorf("%q is no key", name)
		}
		k.Code = term.KeyF1 + term.KeyCode(n-1)
	case utf8.RuneCountInString(name) == 1:
		k.Code, k.Rune = term.KeyRune, []rune(name)[0]
	default:
		return term.Key{}, fmt.Errorf("%q is no key", name)
	}
	return canonKey(k), nil
}

// parseKeyBindings reads keybindings.json. What cannot be used is left out
// and said in problems; the rest is in force.
func parseKeyBindings(data []byte) (keyBindings, []string) {
	b := keyBindings{byCtx: map[string]map[term.Key]string{}}
	var file struct {
		Bindings []struct {
			Context  string                     `json:"context"`
			Bindings map[string]json.RawMessage `json:"bindings"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return b, []string{"it is not the JSON it should be (" + err.Error() + "), so no key is changed"}
	}
	var problems []string
	for _, block := range file.Bindings {
		ctx, ok := keyContexts[block.Context]
		if !ok {
			problems = append(problems, fmt.Sprintf("the context %q is not one gnosh takes keys for (Global, Chat, Task, Transcript), so its %d bindings are left out", block.Context, len(block.Bindings)))
			continue
		}
		if b.byCtx[ctx] == nil {
			b.byCtx[ctx] = map[term.Key]string{}
		}
		keys := make([]string, 0, len(block.Bindings))
		for k := range block.Bindings {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, stroke := range keys {
			raw := block.Bindings[stroke]
			action := ""
			if string(raw) != "null" {
				if err := json.Unmarshal(raw, &action); err != nil || action == "" {
					problems = append(problems, fmt.Sprintf("%s: %s is neither an action's name nor null", block.Context, stroke))
					continue
				}
			}
			if strings.Contains(strings.TrimSpace(stroke), " ") {
				problems = append(problems, fmt.Sprintf("%s: %q is a chord, and gnosh takes single keys only", block.Context, stroke))
				continue
			}
			k, err := parseStroke(strings.TrimSpace(stroke))
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %q: %v", block.Context, stroke, err))
				continue
			}
			if slices.Contains(reservedKeys, k) {
				problems = append(problems, fmt.Sprintf("%s: %s cannot be bound: it always does what it does", block.Context, stroke))
				continue
			}
			if name, ok := strings.CutPrefix(action, "command:"); ok {
				if !commandWord.MatchString("/" + name) {
					problems = append(problems, fmt.Sprintf("%s: %s: %q names no command", block.Context, stroke, action))
					continue
				}
			} else if _, known := keyActions[action]; action != "" && !known {
				problems = append(problems, fmt.Sprintf("%s: %s: %q is not an action gnosh carries out", block.Context, stroke, action))
				continue
			}
			b.byCtx[ctx][k] = action
		}
	}
	return b, problems
}

// keyFileCheck is how often at most the file is looked at for a change.
const keyFileCheck = time.Second

// keysNow reads the keybindings file again when it changed, and says once
// what in it cannot be used.
func (s *session) keysNow() {
	path := s.info.Keybindings
	if path == "" {
		s.keys = nil
		return
	}
	now := s.now()
	if s.keys != nil && s.keys.path == path && now.Sub(s.keys.checked) < keyFileCheck {
		return
	}
	info, err := os.Stat(path)
	if s.keys != nil && s.keys.path == path && err == nil && info.ModTime().Equal(s.keys.mod) && info.Size() == s.keys.size {
		s.keys.checked = now
		return
	}
	f := &keyFile{path: path, checked: now}
	if err != nil {
		s.keys = f
		return
	}
	f.mod, f.size = info.ModTime(), info.Size()
	data, err := os.ReadFile(path)
	if err != nil {
		s.keys = f
		return
	}
	var problems []string
	f.b, problems = parseKeyBindings(data)
	s.keys = f
	if len(problems) > 0 {
		s.notice(agent.LevelWarning, showPath(s.cwd, s.home, path)+": "+strings.Join(problems, "; ")+".")
	}
}

// rebound carries out a key the keybindings file binds, and reports
// whether it did: the action's own keys go to where they are taken, past
// the bindings, and a key taken away does nothing.
func (s *session) rebound(k term.Key) bool {
	if s.q != nil {
		return false
	}
	s.keysNow()
	if s.keys == nil || len(s.keys.b.byCtx) == 0 {
		return false
	}
	ctx := "Chat"
	if s.view != nil {
		ctx = "Transcript"
	}
	c := canonKey(k)
	action, ok := s.keys.b.byCtx[ctx][c]
	if !ok {
		action, ok = s.keys.b.byCtx["Global"][c]
	}
	if !ok {
		return false
	}
	switch name, command := strings.CutPrefix(action, "command:"); {
	case action == "":
		// Taken away.
	case command:
		if s.view == nil {
			s.command(editor.Draft{Text: "/" + name})
		}
	default:
		for _, key := range keyActions[action] {
			if s.view != nil {
				s.viewInput(key)
			} else {
				s.typedOrMouse(key)
			}
		}
	}
	s.dirty = true
	return true
}

// keybindingsWords are the line that opens the keybindings file.
var keybindingsWords = map[string]bool{"/keybindings": true}

// openKeybindings is /keybindings typed alone: the person's keybindings
// file, made with nothing bound when it is not there, opens in their
// editor, and is read again as they leave it.
func (s *session) openKeybindings(line string) bool {
	path := s.info.Keybindings
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return false
		}
		if err := os.WriteFile(path, []byte(app.KeybindingsTemplate), 0o600); err != nil {
			return false
		}
	}
	s.ed.AddHistory(editor.Draft{Text: line})
	if s.openInEditor(path, "gnosh takes it in at once") {
		s.keys = nil
		s.keysNow()
	}
	return true
}

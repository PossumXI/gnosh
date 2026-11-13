package keyscript

import (
	"reflect"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term"
)

func TestParse(t *testing.T) {
	ch := func(r rune) term.Event { return term.Key{Code: term.KeyRune, Rune: r, Text: string(r)} }
	cases := []struct {
		script string
		want   []term.Event
	}{
		{``, nil},
		{`"hi"`, []term.Event{ch('h'), ch('i')}},
		{"`a\\`", []term.Event{ch('a'), ch('\\')}},
		{`"é世"`, []term.Event{ch('é'), ch('世')}},
		{`"\t\r\n\x7f\x1b\x00"`, []term.Event{
			term.Key{Code: term.KeyTab},
			term.Key{Code: term.KeyEnter},
			term.Key{Code: term.KeyRune, Rune: 'j', Mod: term.ModCtrl},
			term.Key{Code: term.KeyBackspace},
			term.Key{Code: term.KeyEscape},
			term.Key{Code: term.KeyRune, Rune: ' ', Mod: term.ModCtrl},
		}},
		{`<Left> <left>*2`, []term.Event{term.Key{Code: term.KeyLeft}, term.Key{Code: term.KeyLeft}, term.Key{Code: term.KeyLeft}}},
		{`<Shift+Enter>`, []term.Event{term.Key{Code: term.KeyEnter, Mod: term.ModShift}}},
		{`<Ctrl+A>`, []term.Event{term.Key{Code: term.KeyRune, Rune: 'a', Mod: term.ModCtrl}}},
		{`<Alt+Shift+B>`, []term.Event{term.Key{Code: term.KeyRune, Rune: 'b', Mod: term.ModAlt | term.ModShift}}},
		{`<Ctrl+Alt+Up>`, []term.Event{term.Key{Code: term.KeyUp, Mod: term.ModCtrl | term.ModAlt}}},
		{`<Ctrl+_>`, []term.Event{term.Key{Code: term.KeyRune, Rune: '_', Mod: term.ModCtrl}}},
		{`<Ctrl++>`, []term.Event{term.Key{Code: term.KeyRune, Rune: '+', Mod: term.ModCtrl}}},
		{`<+>`, []term.Event{ch('+')}},
		{`<Alt+>>`, []term.Event{term.Key{Code: term.KeyRune, Rune: '>', Mod: term.ModAlt}}},
		{`<Alt+<>`, []term.Event{term.Key{Code: term.KeyRune, Rune: '<', Mod: term.ModAlt}}},
		{`<Shift+a>`, []term.Event{term.Key{Code: term.KeyRune, Rune: 'a', Mod: term.ModShift, Text: "A"}}},
		{`<Space>`, []term.Event{ch(' ')}},
		{`<Ctrl+Space>`, []term.Event{term.Key{Code: term.KeyRune, Rune: ' ', Mod: term.ModCtrl}}},
		{`<F5> <PgUp> <Del> <Esc>`, []term.Event{
			term.Key{Code: term.KeyF5}, term.Key{Code: term.KeyPageUp}, term.Key{Code: term.KeyDelete}, term.Key{Code: term.KeyEscape},
		}},
		{`<Paste "a\nb > c">`, []term.Event{term.Paste{Text: "a\nb > c"}}},
		{`<PasteCut "half">`, []term.Event{term.Paste{Text: "half", Truncated: true}}},
		{`<Resize 40x10> <Focus> <Blur> <Resume>`, []term.Event{
			term.Resize{Cols: 40, Rows: 10}, term.Focus{In: true}, term.Focus{}, term.Resume{},
		}},
		{`"ab"*2 <Tab>`, []term.Event{ch('a'), ch('b'), ch('a'), ch('b'), term.Key{Code: term.KeyTab}}},
	}
	for _, c := range cases {
		got, err := Parse(c.script)
		if err != nil {
			t.Errorf("Parse(%s): %v", c.script, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("Parse(%s)\n got %v\nwant %v", c.script, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, script := range []string{
		`hello`, `"open`, `<Left`, `<Nope>`, `<Hyper+Wrong+Left+x>`, `<Bogus+A>`, `<Left>*`, `<Left>x`,
		`<Paste>`, `<Paste "a" "b">`, `<Resize big>`, `<>`, `<Left extra>`,
	} {
		if evs, err := Parse(script); err == nil {
			t.Errorf("Parse(%s) gave %v, want an error", script, evs)
		}
	}
}

func TestMustParsePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParse did not panic on a bad script")
		}
	}()
	MustParse(`<Nope>`)
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{`"a" <Left>*3 <Paste "x">`, `<Ctrl++>`, `<<>>`, "`raw`", `<Resize 1x1>`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, script string) {
		// It may refuse a script, but it must not panic, and a script it
		// takes must hold only events of the kinds it documents.
		evs, err := Parse(script)
		if err != nil {
			return
		}
		for _, ev := range evs {
			switch ev.(type) {
			case term.Key, term.Paste, term.Resize, term.Focus, term.Resume:
			default:
				t.Fatalf("Parse(%q) made a %T", script, ev)
			}
		}
	})
}

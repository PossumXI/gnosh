package menu

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/text"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

func TestInput(t *testing.T) {
	const keys = "Enter answer · Esc cancel · Shift+Enter new line"
	g := newRig(t, 50, NewInput(InputOptions{Title: "Name this session", Placeholder: "a few words"}))
	g.screen(1, 4, "Name this session", "  | a few words", keys)
	// Enter with nothing typed is not an answer.
	g.do(`<Enter>`).open().do(`"  " <Enter>`).open().do(`<Ctrl+U>`)
	g.do(`"nightly check"`).open().screen(1, 17, "Name this session", "  | nightly check", keys)
	g.do(`<Enter>`).chose(-1, "nightly check")

	// Unless the caller says an empty answer will do.
	newRig(t, 50, NewInput(InputOptions{Title: "Why?", AllowEmpty: true})).do(`<Enter>`).chose(-1, "")
	newRig(t, 50, NewInput(InputOptions{Title: "Why?", AllowEmpty: true})).do(`<Ctrl+J> <Enter>`).chose(-1, "")

	// Esc cancels, whatever is typed. Ctrl+C empties the field first.
	newRig(t, 50, NewInput(InputOptions{Title: "Why?"})).do(`<Esc>`).cancelled()
	newRig(t, 50, NewInput(InputOptions{Title: "Why?"})).do(`"typed" <Esc>`).cancelled()
	newRig(t, 50, NewInput(InputOptions{Title: "Why?"})).do(`<Ctrl+C>`).cancelled()
	g = newRig(t, 50, NewInput(InputOptions{Title: "Why?"}))
	g.do(`"typed" <Ctrl+C>`).open().screen(1, 4, "Why?", "  |", keys)
	g.do(`<Ctrl+C>`).cancelled()

	// The field is the editor's: its keys work, text can be there to
	// begin with, a paste goes in whole, and it grows.
	g = newRig(t, 30, NewInput(InputOptions{Title: "Rename", Initial: "old name", Body: []string{"session 4f2a"}}))
	g.screen(2, 12, "Rename", "  session 4f2a", "  | old name", "Enter answer · Esc cancel")
	g.do(`<Ctrl+A> <Alt+D> "new" <Ctrl+E> <Paste " of\nthe session, at some length"> <Shift+Enter> "end"`).open().screen(5, 7,
		"Rename",
		"  session 4f2a",
		"  | new name of",
		"    the session, at some",
		"    length",
		"    end",
		"Enter answer · Esc cancel")
	g.do(`<Enter>`).chose(-1, "new name of\nthe session, at some length\nend")

	// Keys that mean nothing to a field are handed back.
	g = newRig(t, 50, NewInput(InputOptions{Title: "Why?"}))
	for _, k := range []string{`<F5>`, `<Up>`, `<Down>`, `<Shift+Tab>`, `<Ctrl+L>`, `<Ctrl+R>`, `<Focus>`} {
		if g.do(k); g.last.Handled || g.last.Done {
			t.Errorf("%s: %+v", k, g.last)
		}
	}
	// A question mark, a slash and a bang are text here.
	g.do(`"?/!" <Home> "!" <Enter>`).chose(-1, "!?/!")
}

// secretAlphabet is what the tests type as secrets: characters that are
// in none of the words a Secret draws, so that finding one of them in a
// row means the secret leaked.
const secretAlphabet = "QZ7%ß世👍"

// leaks reports whether any of the characters of a secret is in s.
func leaks(s string) bool { return strings.ContainsAny(s, secretAlphabet) }

// A secret: what is typed is never drawn, and is in no result but the
// last.
func TestSecret(t *testing.T) {
	const keys = "Enter confirm · Esc cancel · what you type is not shown"
	newSecret := func() *Secret {
		return NewSecret(SecretOptions{
			Title: "Password for the key test1",
			Body:  []string{"gnokey maketx call -pkgpath gno.land/r/demo/boards", "chain: gnoland-1 (mainnet)"},
		})
	}
	s := newSecret()
	g := newRig(t, 60, s)
	g.screen(3, 12,
		"Password for the key test1",
		"  gnokey maketx call -pkgpath gno.land/r/demo/boards",
		"  chain: gnoland-1 (mainnet)",
		"> Password:",
		keys)

	// Every event of a session is watched: nothing drawn and no result
	// before the last may hold any part of what was typed.
	watch := func(g *rig, script string) {
		t.Helper()
		for _, ev := range keyscript.MustParse(script) {
			g.last = g.p.Handle(ev)
			if !g.last.Done && (g.last.Text != "" || leaks(fmt.Sprintf("%+v", g.last))) {
				t.Fatalf("after %v: a result that does not end the question holds text: %+v", ev, g.last)
			}
			v := checkView(t, g.p, g.width, fmt.Sprintf("after %v", ev))
			if leaks(strings.Join(v.Lines, "\n")) {
				t.Fatalf("after %v: the secret is on screen: %q", ev, v.Lines)
			}
			if printed := fmt.Sprintf("%v %+v %#v %s", g.p, g.p, g.p, g.p); leaks(printed) {
				t.Fatalf("after %v: printing the question shows the secret: %s", ev, printed)
			}
		}
	}
	watch(g, `"QZ7%"`)
	g.screen(3, 16,
		"Password for the key test1",
		"  gnokey maketx call -pkgpath gno.land/r/demo/boards",
		"  chain: gnoland-1 (mainnet)",
		"> Password: ****",
		keys)
	watch(g, `<Enter>`)
	g.chose(-1, "QZ7%")

	// One mark and one Backspace for each character, whatever it is
	// made of.
	s = newSecret()
	g = newRig(t, 60, s)
	watch(g, `"Q" "ß" "\u0301" "世" "👍" "\u200D" "👍" " "`)
	g.screen(3, 17, "Password for the key test1", "  gnokey maketx call -pkgpath gno.land/r/demo/boards", "  chain: gnoland-1 (mainnet)", "> Password: *****", keys)
	watch(g, `<Backspace>*2`)
	g.screen(3, 15, "Password for the key test1", "  gnokey maketx call -pkgpath gno.land/r/demo/boards", "  chain: gnoland-1 (mainnet)", "> Password: ***", keys)
	watch(g, `<Ctrl+H> "Z" <Enter>`)
	g.chose(-1, "Qß\u0301Z")

	// Nothing is trimmed, and an empty answer is not accepted.
	g = newRig(t, 60, newSecret())
	watch(g, `<Enter>`)
	g.open().screen(3, 12,
		"Password for the key test1",
		"  gnokey maketx call -pkgpath gno.land/r/demo/boards",
		"  chain: gnoland-1 (mainnet)",
		"> Password:",
		"Nothing typed yet · Esc cancel")
	watch(g, `" Q " <Backspace>*9 <Enter> "  Z7 " <Enter>`)
	g.chose(-1, "  Z7 ")

	// Ctrl+U starts again.
	g = newRig(t, 60, newSecret())
	watch(g, `"QQQ" <Ctrl+U> "%" <Enter>`)
	g.chose(-1, "%")

	// A paste is its first line, as it is.
	g = newRig(t, 60, newSecret())
	watch(g, `<Paste " Z Q \nsecond line"> <Enter>`)
	g.chose(-1, " Z Q ")
	g = newRig(t, 60, newSecret())
	watch(g, `<Paste "\nQ"> <Paste ""> <Enter>`)
	g.open()

	// Esc and Ctrl+C cancel.
	g = newRig(t, 60, newSecret())
	watch(g, `"QZ" <Esc>`)
	g.cancelled()
	g = newRig(t, 60, newSecret())
	watch(g, `"QZ" <Ctrl+C>`)
	g.cancelled()

	// There is no moving about in a secret, no history, no undo, no
	// putting back and no completion: those keys do nothing.
	s = newSecret()
	g = newRig(t, 60, s)
	watch(g, `"QZ7"`)
	for _, k := range []string{
		`<Left>`, `<Right>`, `<Up>`, `<Down>`, `<Home>`, `<End>`, `<Tab>`, `<Shift+Tab>`, `<Delete>`, `<Ctrl+A>`, `<Ctrl+E>`,
		`<Ctrl+R>`, `<Ctrl+Y>`, `<Alt+Y>`, `<Ctrl+_>`, `<Alt+/>`, `<Ctrl+S>`, `<Ctrl+W>`, `<Ctrl+K>`, `<Ctrl+G>`, `<Ctrl+D>`,
		`<Shift+Enter>`, `<Alt+Enter>`, `<F5>`, `<Focus>`, `<Resize 20x5>`,
	} {
		watch(g, k)
		if g.last.Handled || string(s.buf) != "QZ7" {
			t.Errorf("%s: %+v, and the answer is now %d bytes", k, g.last, len(s.buf))
		}
	}
	watch(g, `<Enter>`)
	g.chose(-1, "QZ7")
}

// When the question ends, whichever way, the buffer that held the answer
// is overwritten, and so is every buffer it outgrew on the way.
func TestSecretIsWiped(t *testing.T) {
	zero := func(b []byte) bool {
		for _, c := range b {
			if c != 0 {
				return false
			}
		}
		return true
	}
	for _, ending := range []string{"Enter", "Esc", "Ctrl+C"} {
		s := NewSecret(SecretOptions{Title: "Password"})
		s.Handle(key("Q"))
		first := s.buf[:cap(s.buf)]
		for range 80 {
			s.Handle(term.Paste{Text: "Z7%Q"})
		}
		second := s.buf[:cap(s.buf)]
		if &first[0] == &second[0] {
			t.Fatal("the answer did not outgrow its first buffer")
		}
		if !zero(first) {
			t.Errorf("the buffer the answer outgrew still holds it")
		}
		if len(s.ends) != 321 || len(s.buf) != 321 {
			t.Fatalf("%d characters in %d bytes, want 321 of each", len(s.ends), len(s.buf))
		}
		res := s.Handle(key(ending))
		if !res.Done || (ending == "Enter") == res.Cancelled || (ending == "Enter") != (len(res.Text) == 321) {
			t.Fatalf("%s: done %v, cancelled %v, %d bytes of text", ending, res.Done, res.Cancelled, len(res.Text))
		}
		if !zero(second) || len(s.buf) != 0 || len(s.ends) != 0 {
			t.Errorf("after %s the buffer still holds the answer", ending)
		}
		if v := s.Render(40); len(v.Lines) != 0 {
			t.Errorf("after %s the question still draws: %q", ending, v.Lines)
		}
	}
	// Backspace and Ctrl+U overwrite what they take away.
	s := NewSecret(SecretOptions{Title: "Password"})
	for _, r := range "QZ7%" {
		s.Handle(key(string(r)))
	}
	whole := s.buf[:4]
	s.Handle(key("Backspace"))
	if string(whole) != "QZ7\x00" {
		t.Errorf("after Backspace the buffer holds %q", whole)
	}
	s.Handle(key("Ctrl+U"))
	if !zero(whole) {
		t.Errorf("after Ctrl+U the buffer holds %q", whole)
	}
}

// How a secret is drawn: a mark for each character, the last of them
// when there are more than fit, the theme's own mark, and a label that
// gives way in a narrow terminal.
func TestSecretView(t *testing.T) {
	type_ := func(s *Secret, n int) {
		for range n {
			s.Handle(key("Q"))
		}
	}
	s := NewSecret(SecretOptions{Title: "Unlock", Label: "Passphrase"})
	type_(s, 40)
	g := newRig(t, 30, s)
	g.screen(1, 29, "Unlock", "> Passphrase: "+strings.Repeat("*", 15), "Enter confirm · Esc cancel")
	g.width = 16
	g.screen(1, 15, "Unlock", "> "+strings.Repeat("*", 13), "Enter confirm")
	g.width = 3
	g.screen(2, 2, "Unl", "ock", ">")

	th := DefaultTheme(text.NoColor)
	th.Mask = "●"
	s = NewSecret(SecretOptions{Title: "Unlock", Theme: th})
	type_(s, 3)
	newRig(t, 40, s).screen(1, 15, "Unlock", "> Password: ●●●", "Enter confirm · Esc cancel")
	// A theme with no mark still shows that keys are arriving.
	th.Mask = ""
	s = NewSecret(SecretOptions{Title: "Unlock", Theme: th})
	type_(s, 3)
	newRig(t, 40, s).screen(1, 15, "Unlock", "> Password: ***", "Enter confirm · Esc cancel")
}

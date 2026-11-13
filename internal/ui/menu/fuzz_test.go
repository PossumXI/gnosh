package menu

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/term"
	"github.com/gnosh-ai/gnosh/internal/ui/editor/keyscript"
)

// pool is every kind of event a session with a question can hold. A byte
// of fuzz input picks one. The keys that end a question are few among
// them, so that sessions last.
var pool = keyscript.MustParse(`
	"a" "j" "k" "y" "n" "x" "1" "2" "3" "4" "9" "0" "世" "\u0301" "\u200D" "👍"
	"j" "k" "j" "k" <Up> <Down> <Up> <Down> <Left> <Right> <Tab> <Shift+Tab>
	<Home> <End> <PgUp> <PgDn> <Shift+Up> <Shift+Down> <Space> <Space> <Backspace>
	<Ctrl+U> <Ctrl+J> <Shift+Enter> <Ctrl+A> <Ctrl+E> <Ctrl+W> <Ctrl+K> <Ctrl+Y> <Ctrl+_> <Alt+B> <Delete>
	<Ctrl+P> <Ctrl+N> <Ctrl+I> <F5> <Ctrl+L> <Focus> <Resize 30x10>
	<Paste "p"> <Paste "two\nlines"> <Paste "\x1b[31m\ttabbed"> <Paste "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12">
	<Enter> <Ctrl+M> <Esc> <Ctrl+C>
`)

// secretPool is what a session with a Secret holds: only characters that
// no word of the question has, so that one of them on screen is a leak.
var secretPool = keyscript.MustParse(`
	"Q" "Z" "7" "%" "ß" "世" "👍" "\u0301" "\u200D" "Q" "Z" "7"
	<Backspace> <Backspace> <Ctrl+H> <Ctrl+U> <Left> <Up> <Tab> <Ctrl+Y> <Ctrl+_> <Ctrl+R> <F5> <Focus>
	<Paste "Q7"> <Paste "Z%\nQQ"> <Paste "">
	<Enter> <Esc> <Ctrl+C>
`)

// asked makes the question a session is about.
func asked(kind int) Prompt {
	var body []string
	for i := range 25 {
		body = append(body, fmt.Sprintf("+ line %d of a change that is rather long for a narrow terminal", i))
	}
	switch kind % 7 {
	case 0:
		return permission()
	case 1:
		return NewConfirm(ConfirmOptions{Title: "Apply this change?", Body: body, MaxHeight: 10})
	case 2:
		return survey(FormOptions{MaxHeight: 9})
	case 3:
		return NewInput(InputOptions{Title: "Name this session", Body: body[:2], Placeholder: "a few words"})
	case 4:
		return NewSecret(SecretOptions{Title: "Unlock", Body: body[:3], MaxHeight: 5})
	case 5:
		return numbered(15, SelectOptions{MaxHeight: 6, Body: body})
	}
	return NewForm(FormOptions{MaxHeight: 6, Questions: []Question{
		{Text: "Which of these tools may the agent use for the rest of this session?", Multi: true,
			Options: []Option{{Label: "Read", Description: "files in this folder"}, {Label: "Write"}, {Label: "Bash", Description: "any command at all, which is a lot to allow"}}},
		{Header: "Why", Text: "Why?"},
	}})
}

// session runs the events that data picks through a question, drawing it
// after each at a width that data also changes, and checks that what is
// drawn is always sound, that nothing typed comes out before the question
// ends, and that a question that has ended stays ended.
func session(t *testing.T, kind int, data []byte, width int) {
	p := asked(kind)
	_, secret := p.(*Secret)
	events := pool
	if secret {
		events = secretPool
	}
	width = width%70 + 1
	over := false
	for i, b := range data {
		if b >= 250 {
			width = int(b-250)*13 + 1 + i%9
			if s, ok := p.(interface{ SetMaxHeight(int) }); ok {
				s.SetMaxHeight(i%12 + 1)
			}
			continue
		}
		ev := events[int(b)%len(events)]
		when := fmt.Sprintf("kind %d, event %d (%v) at width %d", kind%7, i, ev, width)
		res := p.Handle(ev)
		switch {
		case over:
			if res.Handled || res.Done || res.Cancelled || res.Text != "" || res.Answers != nil {
				t.Fatalf("%s: a question that had ended answered again: %+v", when, res)
			}
		case res.Done:
			over = true
			if res.Cancelled && (res.Index != -1 || res.Text != "" || res.Answers != nil) {
				t.Fatalf("%s: cancelled with an answer: %+v", when, res)
			}
			if !res.Handled {
				t.Fatalf("%s: ended by an event that was not handled: %+v", when, res)
			}
			if secret && !res.Cancelled && res.Text == "" {
				t.Fatalf("%s: an empty secret was accepted", when)
			}
		default:
			if res.Cancelled || res.Text != "" || res.Answers != nil || res.Index != -1 {
				t.Fatalf("%s: an answer came out before the question ended: %+v", when, res)
			}
		}
		v := checkView(t, p, width, when)
		if over && len(v.Lines) != 0 {
			t.Fatalf("%s: a question that has ended still draws", when)
		}
		if secret && leaks(strings.Join(v.Lines, "\n")) {
			t.Fatalf("%s: the secret is on screen: %q", when, v.Lines)
		}
		if secret && leaks(fmt.Sprintf("%v %+v", p, p)) {
			t.Fatalf("%s: printing the question shows the secret", when)
		}
	}
}

func FuzzPrompts(f *testing.F) {
	for kind := range 7 {
		f.Add(uint8(kind), []byte("some keys"), uint8(40))
		rng := rand.New(rand.NewSource(int64(kind)))
		data := make([]byte, 80)
		rng.Read(data)
		f.Add(uint8(kind), data, uint8(kind*9))
	}
	f.Fuzz(func(t *testing.T, kind uint8, data []byte, width uint8) {
		session(t, int(kind), data, int(width))
	})
}

// The same over a fixed set of random sessions, so that it is checked on
// every run and not only while fuzzing.
func TestRandomSessions(t *testing.T) {
	n := 700
	if testing.Short() {
		n = 70
	}
	enders := map[term.Event]bool{}
	for _, ev := range keyscript.MustParse(`<Enter> <Ctrl+M> <Esc> <Ctrl+C>`) {
		enders[ev] = true
	}
	for seed := range n {
		rng := rand.New(rand.NewSource(int64(seed) + 7))
		data := make([]byte, 30+rng.Intn(200))
		rng.Read(data)
		// Keep most sessions going: take out most of the keys that end
		// a question, and most changes of width.
		for i := range data {
			ends := data[i] < 250 && enders[pool[int(data[i])%len(pool)]]
			if (ends || data[i] >= 250) && rng.Intn(6) > 0 {
				data[i] = byte(rng.Intn(40))
			}
		}
		session(t, seed, data, rng.Intn(256))
	}
}

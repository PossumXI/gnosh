package menu

import (
	"fmt"
	"reflect"
	"testing"
)

// survey is a form of three questions: one answer, several answers, and
// an answer that is typed.
func survey(o FormOptions) *Form {
	o.Questions = []Question{
		{Header: "Color", Text: "Which color?", Options: []Option{{Label: "Red", Description: "the warm one"}, {Label: "Blue"}}},
		{Header: "Sizes", Text: "Which sizes?", Description: "Any number.", Multi: true, NoOther: true,
			Options: []Option{{Label: "S"}, {Label: "M"}, {Label: "L"}}},
		{Header: "Notes", Text: "Anything else?", Placeholder: "type here"},
	}
	return NewForm(o)
}

// answered checks that the last event sent the form with these answers.
func (g *rig) answered(want ...Answer) *rig {
	g.t.Helper()
	if !g.last.Handled || !g.last.Done || g.last.Cancelled || !reflect.DeepEqual(g.last.Answers, want) {
		g.t.Fatalf("last event: %+v, want the form sent with %+v", g.last, want)
	}
	g.over()
	return g
}

// The whole of a form: each question on its tab, the review, and the
// answers that come out.
func TestFormWalkthrough(t *testing.T) {
	f := survey(FormOptions{})
	g := newRig(t, 70, f)
	g.screen(-1, 0,
		"[Color] Sizes  Notes  Review",
		"Which color?",
		"> 1. ( ) Red  the warm one",
		"  2. ( ) Blue",
		"  3. ( ) Other, in your own words",
		"Enter choose · ↑↓ move · Esc cancel · ←/→ questions")
	// Enter answers a question that takes one answer and goes on.
	g.do(`<Enter>`).open().screen(-1, 0,
		" Color ✓ [Sizes] Notes  Review",
		"Which sizes?",
		"Any number.",
		"> 1. [ ] S",
		"  2. [ ] M",
		"  3. [ ] L",
		"Space tick · Enter next · ↑↓ move · Esc cancel · ←/→ questions")
	// Space and the numbers tick and untick.
	g.do(`<Space> <Down>*2 <Space> "2" "2"`).open().screen(-1, 0,
		" Color ✓ [Sizes ✓] Notes  Review",
		"Which sizes?",
		"Any number.",
		"  1. [x] S",
		"> 2. [ ] M",
		"  3. [x] L",
		"Space tick · Enter next · ↑↓ move · Esc cancel · ←/→ questions")
	// A question with no options is a field, open at once.
	g.do(`<Enter>`).open().screen(2, 4,
		" Color ✓  Sizes ✓ [Notes] Review",
		"Anything else?",
		"  | type here",
		"Enter confirm · Esc cancel · Tab next question")
	if !f.Typing() || f.Tab() != 2 {
		t.Fatalf("on tab %d, typing %v", f.Tab(), f.Typing())
	}
	g.do(`"make it pop"`).open().screen(2, 15,
		" Color ✓  Sizes ✓ [Notes] Review",
		"Anything else?",
		"  | make it pop",
		"Enter confirm · Esc cancel · Tab next question")
	// The review.
	g.do(`<Enter>`).open().screen(-1, 0,
		" Color ✓  Sizes ✓  Notes ✓ [Review]",
		"Review your answers",
		"  Color: Red",
		"  Sizes: S, L",
		"  Notes: make it pop",
		"Enter send · ←/→ change an answer · Esc cancel")
	want := []Answer{{Picked: []int{0}}, {Picked: []int{0, 2}}, {Other: "make it pop"}}
	if got := f.Answers(); !reflect.DeepEqual(got, want) {
		t.Fatalf("answers so far: %+v", got)
	}
	g.do(`<Enter>`).answered(want...)
}

// Left, Right, Tab and Shift+Tab go between the questions, round and
// round, and an answer can be changed on the way.
func TestFormTabs(t *testing.T) {
	f := survey(FormOptions{})
	g := newRig(t, 70, f)
	for _, c := range []struct {
		key string
		tab int
	}{
		{`<Right>`, 1}, {`<Tab>`, 2}, {`<Tab>`, 3}, {`<Right>`, 0}, {`<Left>`, 3}, {`<Shift+Tab>`, 2},
		{`<Shift+Tab>`, 1}, {`<Left>`, 0}, {`<Ctrl+I>`, 1},
	} {
		g.do(c.key).open()
		if f.Tab() != c.tab {
			t.Fatalf("after %s on tab %d, want %d", c.key, f.Tab(), c.tab)
		}
	}
	// In the field Left and Right move in the text; Tab still goes on.
	g.do(`<Tab> "ab" <Left> "x" <Right> "y"`).open()
	if f.Tab() != 2 {
		t.Fatalf("typing in the field left tab 2 for %d", f.Tab())
	}
	g.do(`<Tab>`).screen(-1, 0,
		" Color  Sizes  Notes ✓ [Review]",
		"Review your answers",
		"  Color: (no answer)",
		"  Sizes: (no answer)",
		"  Notes: axby",
		"Enter send · ←/→ change an answer · Esc cancel")
	// Back to a question: its answer is still there, and can be changed.
	g.do(`<Right> "1"`).open().do(`<Left> <Down> <Enter>`).open()
	g.do(`<Left>*2 <Enter>`).answered(Answer{Picked: []int{1}}, Answer{}, Answer{Other: "axby"})
}

// A question that takes one answer: Enter and the number answer and go
// on; Space answers and stays; a new answer takes the place of the old.
func TestFormSingle(t *testing.T) {
	f := survey(FormOptions{})
	g := newRig(t, 70, f)
	g.do(`<Down> <Space>`).open().screen(-1, 0,
		"[Color ✓] Sizes  Notes  Review",
		"Which color?",
		"  1. ( ) Red  the warm one",
		"> 2. (*) Blue",
		"  3. ( ) Other, in your own words",
		"Enter choose · ↑↓ move · Esc cancel · ←/→ questions")
	g.do(`"k" <Space>`).open()
	if got := f.Answers()[0]; !reflect.DeepEqual(got, Answer{Picked: []int{0}}) {
		t.Fatalf("after choosing again: %+v", got)
	}
	g.do(`"2"`).open()
	if f.Tab() != 1 || !reflect.DeepEqual(f.Answers()[0], Answer{Picked: []int{1}}) {
		t.Fatalf("after 2: tab %d, answer %+v", f.Tab(), f.Answers()[0])
	}
	// j and k, Home and End, round and round.
	g.do(`<Left> <Home> "k"`).open().do(`<Space>`)
	if f.Typing() != true {
		t.Fatal("Space on the empty row for the person's own words did not open it")
	}
	g.do(`<Esc> <End> "j" <Enter>`)
	if f.Tab() != 1 || !reflect.DeepEqual(f.Answers()[0], Answer{Picked: []int{0}}) {
		t.Fatalf("after going round: tab %d, answer %+v", f.Tab(), f.Answers()[0])
	}
}

// The row for the person's own words.
func TestFormOther(t *testing.T) {
	f := survey(FormOptions{})
	g := newRig(t, 70, f)
	g.do(`<Down>*2 <Enter>`).open().screen(5, 11,
		"[Color] Sizes  Notes  Review",
		"Which color?",
		"  1. ( ) Red  the warm one",
		"  2. ( ) Blue",
		"> 3. ( ) Other, in your own words",
		"         |",
		"Enter confirm · Esc back · Tab next question")
	// Letters and digits are text there.
	g.do(`"teal 2"`).open().screen(5, 17,
		"[Color] Sizes  Notes  Review",
		"Which color?",
		"  1. ( ) Red  the warm one",
		"  2. ( ) Blue",
		"> 3. ( ) Other, in your own words",
		"         | teal 2",
		"Enter confirm · Esc back · Tab next question")
	g.do(`<Enter>`).open()
	if f.Tab() != 1 || !reflect.DeepEqual(f.Answers()[0], Answer{Other: "teal 2"}) {
		t.Fatalf("after Enter in the field: tab %d, answer %+v", f.Tab(), f.Answers()[0])
	}
	// Back on the question the words show, and an option takes their
	// place as the answer without throwing them away.
	g.do(`<Left>`).screen(-1, 0,
		"[Color ✓] Sizes  Notes  Review",
		"Which color?",
		"  1. ( ) Red  the warm one",
		"  2. ( ) Blue",
		"> 3. (*) Other, in your own words",
		"         | teal 2",
		"Enter choose · ↑↓ move · Esc cancel · ←/→ questions")
	g.do(`<Home> <Space>`).screen(-1, 0,
		"[Color ✓] Sizes  Notes  Review",
		"Which color?",
		"> 1. (*) Red  the warm one",
		"  2. ( ) Blue",
		"  3. ( ) Other, in your own words",
		"         | teal 2",
		"Enter choose · ↑↓ move · Esc cancel · ←/→ questions")
	// Space on the row, now that it has words, makes them the answer.
	g.do(`<End> <Space>`)
	if !reflect.DeepEqual(f.Answers()[0], Answer{Other: "teal 2"}) {
		t.Fatalf("after Space on the row: %+v", f.Answers()[0])
	}
	// Opening the field again and leaving with Esc keeps the words; they
	// do not push out an option that was chosen meanwhile.
	g.do(`<Home> <Space> "3" "!" <Esc>`).open()
	if f.Typing() || !reflect.DeepEqual(f.Answers()[0], Answer{Picked: []int{0}}) {
		t.Fatalf("after Esc in the field: typing %v, answer %+v", f.Typing(), f.Answers()[0])
	}
	// Enter in the field makes them the answer.
	g.do(`<Enter> <Enter>`)
	if !reflect.DeepEqual(f.Answers()[0], Answer{Other: "teal 2!"}) {
		t.Fatalf("after Enter in the field: %+v", f.Answers()[0])
	}
	// Ctrl+C in the field throws the words away.
	g.do(`<Left> <Enter> <Ctrl+C>`).open()
	if f.Typing() || !f.Answers()[0].Empty() {
		t.Fatalf("after Ctrl+C in the field: typing %v, answer %+v", f.Typing(), f.Answers()[0])
	}
	// Enter with nothing typed closes the field and answers nothing.
	g.do(`<Enter> <Enter>`).open()
	if f.Typing() || f.Tab() != 0 || !f.Answers()[0].Empty() {
		t.Fatalf("after Enter in the empty field: typing %v, tab %d", f.Typing(), f.Tab())
	}
	// Up and Down leave the field for the options.
	g.do(`<Enter> "x" <Up>`).open()
	if f.Typing() || !reflect.DeepEqual(f.Answers()[0], Answer{Other: "x"}) {
		t.Fatalf("after Up in the field: typing %v, answer %+v", f.Typing(), f.Answers()[0])
	}
	g.do(`<Esc>`).cancelled()
}

// A question that takes several answers.
func TestFormMulti(t *testing.T) {
	f := NewForm(FormOptions{Questions: []Question{
		{Header: "Tools", Text: "Which tools may it use?", Multi: true, Options: []Option{{Label: "Read"}, {Label: "Write"}, {Label: "Bash"}}},
		{Header: "Why", Text: "Why?"},
	}})
	g := newRig(t, 70, f)
	// Enter with nothing ticked ticks the row it is on, and goes on.
	g.do(`<Down> <Enter>`).open()
	if f.Tab() != 1 || !reflect.DeepEqual(f.Answers()[0], Answer{Picked: []int{1}}) {
		t.Fatalf("after Enter: tab %d, answer %+v", f.Tab(), f.Answers()[0])
	}
	// With something ticked, Enter only goes on.
	g.do(`<Shift+Tab> <Home> <Enter>`)
	if f.Tab() != 1 || !reflect.DeepEqual(f.Answers()[0], Answer{Picked: []int{1}}) {
		t.Fatalf("after a second Enter: tab %d, answer %+v", f.Tab(), f.Answers()[0])
	}
	// The person's own words are one more answer among the others.
	g.do(`<Shift+Tab> "1" "4" "and curl" <Enter>`).open()
	if !reflect.DeepEqual(f.Answers()[0], Answer{Picked: []int{0, 1}, Other: "and curl"}) {
		t.Fatalf("with words: %+v", f.Answers()[0])
	}
	g.do(`<Shift+Tab>`).screen(-1, 0,
		"[Tools ✓] Why  Review",
		"Which tools may it use?",
		"  1. [x] Read",
		"  2. [x] Write",
		"  3. [ ] Bash",
		"> 4. [x] Other, in your own words",
		"         | and curl",
		"Space tick · Enter next · ↑↓ move · Esc cancel · ←/→ questions")
	// Space unticks them without throwing them away.
	g.do(`<Space> "2"`)
	if !reflect.DeepEqual(f.Answers()[0], Answer{Picked: []int{0}}) {
		t.Fatalf("after unticking: %+v", f.Answers()[0])
	}
	g.do(`<Right> "because" <Enter> <Enter>`).answered(Answer{Picked: []int{0}}, Answer{Other: "because"})
}

// The review says which questions have no answer, and sends all the
// same: the person can see what is missing and decide.
func TestFormReview(t *testing.T) {
	f := survey(FormOptions{ReviewTitle: "Send these?", ReviewNote: "The model will see them."})
	g := newRig(t, 70, f)
	g.do(`<Left>`).screen(-1, 0,
		" Color  Sizes  Notes [Review]",
		"Send these?",
		"The model will see them.",
		"  Color: (no answer)",
		"  Sizes: (no answer)",
		"  Notes: (no answer)",
		"Enter send · ←/→ change an answer · Esc cancel")
	for _, k := range []string{`<Up>`, `<Down>`, `"1"`, `<Space>`, `"j"`} {
		if g.do(k); g.last.Handled {
			t.Errorf("%s on the review: %+v", k, g.last)
		}
	}
	g.do(`<Enter>`).answered(Answer{}, Answer{}, Answer{})

	// Long answers wrap under themselves, and words typed on several
	// lines are shown on one.
	f = survey(FormOptions{})
	g = newRig(t, 30, f)
	g.do(`<Left>*2 "first line" <Shift+Enter> "and a second, rather longer" <Enter>`).screen(-1, 0,
		"[Review]",
		"Review your answers",
		"  Color: (no answer)",
		"  Sizes: (no answer)",
		"  Notes: first line and a",
		"    second, rather longer",
		"Enter send")
	g.do(`<Enter>`).answered(Answer{}, Answer{}, Answer{Other: "first line\nand a second, rather longer"})
}

// Esc and Ctrl+C cancel the form from anywhere but an open field of a
// question that has options, and no answers come out.
func TestFormCancels(t *testing.T) {
	for _, k := range []string{
		`<Esc>`, `<Ctrl+C>`, `<Enter> <Esc>`, `<Left> <Esc>`, `<Left> <Ctrl+C>`,
		`<Right>*2 <Esc>`, `<Right>*2 "typed" <Esc>`, `<Right>*2 <Ctrl+C>`,
		`<Down>*2 <Enter> "typed" <Esc> <Esc>`, `<Down>*2 <Enter> "typed" <Ctrl+C> <Ctrl+C>`,
	} {
		newRig(t, 70, survey(FormOptions{})).do(k).cancelled()
	}
	// Ctrl+C in a question that is only a field empties the field first.
	f := survey(FormOptions{})
	g := newRig(t, 70, f)
	g.do(`<Right>*2 "typed" <Ctrl+C>`).open()
	if !f.Typing() || !f.Answers()[2].Empty() {
		t.Fatalf("after Ctrl+C: typing %v, answers %+v", f.Typing(), f.Answers())
	}
	g.do(`<Ctrl+C>`).cancelled()
}

// A form of one question has no tabs and no review: the answer ends it.
func TestFormOfOne(t *testing.T) {
	one := func(q Question) *rig { return newRig(t, 70, NewForm(FormOptions{Questions: []Question{q}})) }
	choice := Question{Text: "Which?", Options: []Option{{Label: "This"}, {Label: "That"}}}
	g := one(choice)
	g.screen(-1, 0, "Which?", "> 1. ( ) This", "  2. ( ) That", "  3. ( ) Other, in your own words", "Enter choose · ↑↓ move · Esc cancel")
	g.do(`<Down> <Enter>`).answered(Answer{Picked: []int{1}})
	one(choice).do(`"1"`).answered(Answer{Picked: []int{0}})
	one(choice).do(`"3" "neither" <Enter>`).answered(Answer{Other: "neither"})
	one(choice).do(`<Esc>`).cancelled()
	// There are no other questions to go to.
	for _, k := range []string{`<Left>`, `<Right>`, `<Tab>`, `<Shift+Tab>`} {
		if g := one(choice).do(k); g.last.Handled {
			t.Errorf("%s in a form of one question: %+v", k, g.last)
		}
	}

	several := Question{Text: "Which?", Multi: true, NoOther: true, Options: []Option{{Label: "A"}, {Label: "B"}, {Label: "C"}}}
	one(several).do(`<Space> "3" <Enter>`).answered(Answer{Picked: []int{0, 2}})

	typed := Question{Text: "Name it"}
	g = one(typed)
	g.screen(1, 4, "Name it", "  |", "Enter confirm · Esc cancel")
	g.do(`<Enter>`).open().do(`"gnosh" <Enter>`).answered(Answer{Other: "gnosh"})
	one(typed).do(`"x" <Esc>`).cancelled()

	// No questions: nothing to ask.
	g = newRig(t, 70, NewForm(FormOptions{}))
	g.screen(-1, 0)
	if g.do(`<Enter>`); g.last.Handled || g.last.Done {
		t.Errorf("Enter on a form with no questions: %+v", g.last)
	}
}

// Narrow terminals: the tabs give way to the one in front, and long
// labels wrap under themselves.
func TestFormNarrow(t *testing.T) {
	f := NewForm(FormOptions{Questions: []Question{
		{Header: "Deploy", Text: "Where should this be deployed to?", Options: []Option{
			{Label: "The staging droplet", Description: "safe, and wiped nightly"},
			{Label: "Production", Description: "what people use"},
		}},
		{Text: "When?"},
	}})
	g := newRig(t, 24, f)
	g.screen(-1, 0,
		"[1 of 2: Deploy]",
		"Where should this be",
		"deployed to?",
		"> 1. ( ) The staging",
		"         droplet",
		"         safe, and wiped",
		"         nightly",
		"  2. ( ) Production",
		"         what people use",
		"  3. ( ) Other, in your",
		"         own words",
		"Enter choose · ↑↓ move")
	g.do(`"2"`).screen(2, 4, "[2 of 2: Question 2]", "When?", "  |", "Enter confirm")
	g.do(`<Tab>`).screen(-1, 0, "[Review]", "Review your answers", "  Deploy: Production", "  Question 2: (no", "    answer)", "Enter send")
}

// More options than fit: the rows scroll with the cursor.
func TestFormLongQuestion(t *testing.T) {
	q := Question{Text: "Pick", NoOther: true}
	for i := 1; i <= 12; i++ {
		q.Options = append(q.Options, Option{Label: fmt.Sprintf("Choice %d", i)})
	}
	f := NewForm(FormOptions{Questions: []Question{q}, MaxHeight: 7})
	g := newRig(t, 40, f)
	const keys = "Enter choose · ↑↓ move · Esc cancel"
	g.screen(-1, 0, "Pick", "> 1. ( ) Choice 1", "  2. ( ) Choice 2", "  3. ( ) Choice 3", "  4. ( ) Choice 4", "↓ 5. ( ) Choice 5", keys)
	g.do(`<Down>*6`).screen(-1, 0, "Pick", "↑ 4. ( ) Choice 4", "  5. ( ) Choice 5", "  6. ( ) Choice 6", "> 7. ( ) Choice 7", "↓ 8. ( ) Choice 8", keys)
	g.do(`<End>`).screen(-1, 0, "Pick", "↑ 8. ( ) Choice 8", "  9. ( ) Choice 9", "     ( ) Choice 10", "     ( ) Choice 11", ">    ( ) Choice 12", keys)
	f.SetMaxHeight(4)
	g.screen(-1, 0, "Pick", "↑    ( ) Choice 11", ">    ( ) Choice 12", keys)
	g.do(`<Enter>`).answered(Answer{Picked: []int{11}})
}

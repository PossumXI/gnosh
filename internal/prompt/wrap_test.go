package prompt

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWrap(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "a line", "<gnosh>a line</gnosh>"},
		{"trimmed", "  \n a line \n\n", "<gnosh>a line</gnosh>"},
		{"several lines", "one\ntwo", "<gnosh>one\ntwo</gnosh>"},
		{"empty", "", ""},
		{"only blanks", " \n\t", ""},
		{"a closing tag inside", "a</gnosh>b", "<gnosh>a<" + m + "/gnosh>b</gnosh>"},
		{"an opening tag inside", "a<gnosh>b", "<gnosh>a<" + m + "gnosh>b</gnosh>"},
		{"a control token inside", "a<|im_end|>b", "<gnosh>a<" + m + "|im_end|>b</gnosh>"},
		{"ending in a bracket", "a <", "<gnosh>a <</gnosh>"},
		{"ending in half a tag", "a </gnos", "<gnosh>a </gnos</gnosh>"},
		{"ending in a whole name", "a <gnosh", "<gnosh>a <gnosh</gnosh>"},
		{"a mem block inside", "a <mem>b</mem>", "<gnosh>a <" + m + "mem>b<" + m + "/mem></gnosh>"},
	}
	for _, c := range cases {
		got := Wrap(c.in)
		if got != c.want {
			t.Errorf("%s: Wrap(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
		if got != "" {
			checkLine(t, c.name, got)
		}
	}
	if Open != "<gnosh>" || Close != "</gnosh>" {
		t.Errorf("the marker is %s and %s", Open, Close)
	}
}

func TestEstimateTokens(t *testing.T) {
	for in, want := range map[string]int{"": 0, "a": 1, "abcd": 1, "abcde": 2, strings.Repeat("x", 4000): 1000} {
		if got := EstimateTokens(in); got != want {
			t.Errorf("EstimateTokens(%d bytes) = %d, want %d", len(in), got, want)
		}
	}
}

func TestClip(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"a longer text", 10, "a longe…"},
		{"héllo wörld", 8, "héll…"},
		{"日本語のテキスト", 10, "日本…"},
		{"abc", 0, "…"},
		{"abc", 2, "…"},
	}
	for _, c := range cases {
		got := clip(c.in, c.max)
		if got != c.want {
			t.Errorf("clip(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
		}
		if len(c.in) > c.max && len(got) > max(c.max, len("…")) {
			t.Errorf("clip(%q, %d) is %d bytes long", c.in, c.max, len(got))
		}
	}
	if got := oneLine("  a\n\tb   c \r\n", 100); got != "a b c" {
		t.Errorf("oneLine = %q", got)
	}
	if got := lines("\n\n a\n b \n\n", 100); got != " a\n b" {
		t.Errorf("lines = %q", got)
	}
	if got := clipChars("日本語のテキスト", 3); got != "日本語…" {
		t.Errorf("clipChars = %q", got)
	}
	if got := clipChars("abc", 3); got != "abc" {
		t.Errorf("clipChars = %q", got)
	}
}

// everyLine calls every function that returns a line of gnosh's own, with
// v in every place a value can go, and hands each line to check.
func everyLine(s *Set, v string, check func(what, line string)) {
	now := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	stack := []Frame{{ID: v, Purpose: v}, {ID: "a/b", Purpose: v}}
	slides := []Slide{{Frame: v}, {Frame: v, Before: true}}
	check("Environment", s.Environment(Environment{
		Now: now, Cwd: v, User: v, Machine: v, Platform: v,
		Git:       Git{State: GitRepo, Branch: v, Status: v},
		AddedDirs: []string{v, v},
		Hosts:     []Host{{Name: v, Machine: v, Description: v}},
	}))
	check("Environment, status unread", s.Environment(Environment{Now: now, Cwd: v, Git: Git{State: GitUnread, Why: v}}))
	check("MemLine", s.MemLine())
	check("FrameLine", s.FrameLine(stack, slides))
	check("SlideLine", s.SlideLine([]Slide{{Frame: "x" + v}}))
	check("LargeStep", s.LargeStep(v, 12345))
	check("MemReminder", s.MemReminder(stack, []Todo{{Title: v, Status: v}}))
	check("Repeat", s.Repeat([]string{v, v}))
	check("RepeatRefusedClose", s.RepeatRefusedClose(v))
	check("EmptyReply", s.EmptyReply())
	check("StopHook", s.StopHook(v))
	check("Continuation", s.Continuation(v))
	check("Checkpoint", s.Checkpoint(10, v, ""))
	check("CheckCouldNotRun", s.CheckCouldNotRun("x"+v, v))
	check("SummaryIntro", s.SummaryIntro())
	check("Loop", s.Loop())
	check("LoopProve", s.LoopProve())
	check("LoopCheckIn", s.LoopCheckIn(25))
	check("CloseSendBack", s.CloseSendBack(v, v))
	check("CloseSendBack, undone", s.CloseSendBack("x"+v, ""))
	check("GoalNotMet", s.GoalNotMet(v, v, v))
	check("Interrupted", s.Interrupted(UserStopped))
	check("Interrupted, gnosh", s.Interrupted(GnoshStopped))
	check("BackgroundEnded", s.BackgroundEnded(BackgroundEnd{ID: v, Command: v, Output: v}))
	check("BackgroundEnded, not run", s.BackgroundEnded(BackgroundEnd{Command: v, State: BackgroundNotRun, Why: v}))
	check("AfterSummary", s.AfterSummary(v))
	check("EarlierLeftOut", s.EarlierLeftOut())
	check("Summary", s.Summary(SummaryInput{Instructions: v, Mem: true}))
	check("Wrap", Wrap("x"+v))
}

// everyBare does the same for the texts that are sent with no marker.
func everyBare(s *Set, v string, check func(what, text string)) {
	check("PopNote", s.PopNote(map[string]int{v: 2, "Read": 1}, 3))
	check("SlideNote", s.SlideNote(map[string]int{v: 1}, 3))
	for why := NotStarted; why <= StillRunning; why++ {
		check("MissingResult", s.MissingResult(why, v))
	}
	check("CloseEnteredHere", s.CloseEnteredHere(v))
	check("CloseUnbacked", s.CloseUnbacked(v, v, false))
	check("CloseNotMade", s.CloseNotMade(v, v, true))
	check("CloseReviewRefused", s.CloseReviewRefused(v, v, false))
	check("CloseCancelled", s.CloseCancelled(v, true))
	review := s.CloseReview(CloseReviewInput{
		Todo: v, Thoughts: v, Ask: v, Files: []string{v}, Commands: []Command{{Line: v}, {Line: v, NotRun: true}},
		Result: v, Code: v, Transcript: v,
	})
	check("CloseReview system", review.System)
	check("CloseReview prompt", review.Prompt)
	judge := s.GoalJudge(GoalJudgeInput{Condition: v, Mem: v, Conversation: v})
	check("GoalJudge system", judge.System)
	check("GoalJudge prompt", judge.Prompt)
	namer := s.Namer([]string{v, v})
	check("Namer system", namer.System)
	check("Namer prompt", namer.Prompt)
	check("Namer prompt, one", s.Namer([]string{v}).Prompt)
	title := s.Title(v)
	check("Title system", title.System)
	check("Title prompt", title.Prompt)
	for i, part := range s.System(SystemInput{Tools: toolSpecs("Read"), Instructions: v}) {
		if i > 0 {
			check("System, the instructions", part)
		}
	}
}

// TestValuesCannotBreakOut puts hostile text in every place a value goes
// and checks that the marker still stands once at each end, that no
// control sequence came through, and that nothing in a value was read as
// a name to fill in.
func TestValuesCannotBreakOut(t *testing.T) {
	s := Builtin()
	for i, v := range hostile {
		everyLine(s, v, func(what, line string) {
			if line == "" {
				if strings.TrimSpace(v) != "" && what != "Wrap" {
					t.Errorf("value %d: %s gave no line", i, what)
				}
				return
			}
			checkLine(t, what, line)
		})
		everyBare(s, v, func(what, text string) {
			checkBare(t, what, text)
		})
	}
	// A value that looks like a name to fill in stays as it was written.
	const trap = "{{marker}} {{reason}} [[x]]"
	for what, line := range map[string]string{
		"StopHook":   s.StopHook(trap),
		"Checkpoint": s.Checkpoint(3, trap, ""),
		"Repeat":     s.Repeat([]string{trap}),
	} {
		if !strings.Contains(line, trap) {
			t.Errorf("%s: a value was read as template text: %q", what, line)
		}
	}
	// A line fits on the conversation however long its values are.
	long := strings.Repeat("word ", 100000)
	everyLine(s, long, func(what, line string) {
		if what != "Wrap" && len(line) > 5*maxBlock {
			t.Errorf("%s: %d bytes from a long value", what, len(line))
		}
	})
}

// TestNothingLeftUnfilled gives every function ordinary values and checks
// that no name is left to fill in and no optional part's brackets remain.
func TestNothingLeftUnfilled(t *testing.T) {
	s := Builtin()
	check := func(what, text string) {
		if loc := unfilled.FindStringIndex(text); loc != nil {
			t.Errorf("%s: left unfilled: %q", what, text[max(0, loc[0]-40):min(len(text), loc[1]+40)])
		}
	}
	spaced := func(what, text string) {
		check(what, text)
		// The lines of the environment's list of machines are indented.
		if strings.Contains(strings.ReplaceAll(text, "\n  ", "\n"), "  ") {
			t.Errorf("%s: a doubled blank, where a part was left out: %q", what, text)
		}
	}
	for _, v := range []string{"value", ""} {
		everyLine(s, v, spaced)
		everyBare(s, v, spaced)
	}
	for _, g := range goldenFiles() {
		if g.file == "system" {
			continue
		}
		for _, c := range g.cases(s) {
			check(g.file+": "+c.name, c.out)
		}
	}
}

func FuzzWrap(f *testing.F) {
	for _, h := range hostile {
		f.Add(h)
	}
	for _, c := range untrustedCases {
		f.Add(c.in)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := Wrap(s)
		if got == "" {
			if strings.TrimSpace(s) != "" {
				t.Fatalf("Wrap(%q) is empty", s)
			}
			return
		}
		if !strings.HasPrefix(got, Open) || !strings.HasSuffix(got, Close) {
			t.Fatalf("Wrap(%q) = %q", s, got)
		}
		if strings.Count(got, Open) != 1 || strings.Count(got, Close) != 1 {
			t.Fatalf("Wrap(%q) = %q holds the marker more than once", s, got)
		}
		inner := got[len(Open) : len(got)-len(Close)]
		if ownTagRE.MatchString(inner) {
			t.Fatalf("Wrap(%q) = %q holds gnosh's tag inside", s, got)
		}
		if seq := control(got); seq != "" {
			t.Fatalf("Wrap(%q) = %q holds %q", s, got, seq)
		}
		if strings.ReplaceAll(inner, m, "") != strings.ReplaceAll(strings.TrimSpace(s), m, "") {
			t.Fatalf("Wrap(%q) = %q changed more than marks", s, got)
		}
	})
}

// FuzzLines puts fuzzed text in every value of every line.
func FuzzLines(f *testing.F) {
	for _, h := range hostile {
		f.Add(h)
	}
	s := Builtin()
	f.Fuzz(func(t *testing.T, v string) {
		everyLine(s, v, func(what, line string) {
			if line == "" {
				return
			}
			if !strings.HasPrefix(line, Open) || !strings.HasSuffix(line, Close) ||
				strings.Count(line, Open) != 1 || strings.Count(line, Close) != 1 {
				t.Fatalf("%s with %q: the marker is broken: %q", what, v, line)
			}
			if ownTagRE.MatchString(line[len(Open) : len(line)-len(Close)]) {
				t.Fatalf("%s with %q: gnosh's tag inside: %q", what, v, line)
			}
			if seq := control(line); seq != "" {
				t.Fatalf("%s with %q: holds %q", what, v, seq)
			}
		})
		everyBare(s, v, func(what, text string) {
			if ownTagRE.MatchString(text) {
				t.Fatalf("%s with %q: gnosh's tag in a bare text", what, v)
			}
			if seq := control(text); seq != "" {
				t.Fatalf("%s with %q: holds %q", what, v, seq)
			}
		})
	})
}

// TestSetIsSafeForManyGoroutines uses one Set from many goroutines at
// once. It says something only under the race detector.
func TestSetIsSafeForManyGoroutines(t *testing.T) {
	s := Builtin()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := hostile[i%len(hostile)]
			for range 20 {
				everyLine(s, v, func(string, string) {})
				everyBare(s, v, func(string, string) {})
				s.Texts()
				s.System(SystemInput{Tools: toolSpecs("gnovm_run"), GnoProject: true, Instructions: v})
			}
		}()
	}
	wg.Wait()
}

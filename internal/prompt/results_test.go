package prompt

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestRefusedClose checks that every refusal of a close this package
// writes is known as one, with its todo, and that mem's own refusal, which
// has the same shape, is known too.
func TestRefusedClose(t *testing.T) {
	s := Builtin()
	const todo = "fix-flaky/rerun"
	refusals := map[string]string{
		"entered here":           s.CloseEnteredHere(todo),
		"unbacked, popped":       s.CloseUnbacked(todo, "README.md", false),
		"unbacked, by name":      s.CloseUnbacked(todo, "README.md", true),
		"not made":               s.CloseNotMade(todo, "x_test.go", false),
		"not made, by name":      s.CloseNotMade(todo, "x_test.go", true),
		"review":                 s.CloseReviewRefused(todo, "nothing shows it", false),
		"review, by name":        s.CloseReviewRefused(todo, "nothing shows it", true),
		"cancelled":              s.CloseCancelled(todo, false),
		"cancelled, by name":     s.CloseCancelled(todo, true),
		"mem's own":              "not popped: nothing came between your entering `fix-flaky/rerun` \"rerun the suite 20 times\" and this pop, so it is not done. Do it, or check that it is done, then pop it.",
		"behind mem's own words": "ERROR — nothing from this call was kept: not popped: nothing came between your entering `fix-flaky/rerun` \"…\" and this pop\nfailed · mem · ran 31ms · nothing kept",
	}
	for name, text := range refusals {
		got, ok := RefusedClose(text)
		if !ok || got != todo {
			t.Errorf("%s: RefusedClose = %q, %v; want %q:\n%s", name, got, ok, todo, text)
		}
		byName := strings.Contains(name, "by name")
		if strings.HasPrefix(text, "not closed: ") != byName && !strings.HasPrefix(text, "ERROR") {
			t.Errorf("%s: begins %q", name, clip(text, 20))
		}
		if strings.Contains(text, "\n") && !strings.HasPrefix(text, "ERROR") {
			t.Errorf("%s: a refusal is one line", name)
		}
	}
	for name, text := range map[string]string{
		"a kept pop":         "done `fix-flaky/rerun` \"rerun the suite 20 times\" → ok 20 times",
		"another refusal":    "ERROR — nothing from this call was kept: `x` is already on your stack",
		"no name":            "not popped: nothing to pop",
		"an empty name":      "not popped: `` is nothing",
		"a name on the next": "not popped: something\nthen `x`",
		"an unclosed name":   "not closed: `x",
		"nothing":            "",
	} {
		if got, ok := RefusedClose(text); ok {
			t.Errorf("%s: RefusedClose finds %q in %q", name, got, text)
		}
	}
	// The send-back and the nudge quote a refusal; both must still let it
	// be known.
	for name, line := range map[string]string{
		"send-back": s.CloseSendBack(todo, refusals["review"]),
		"nudge":     s.RepeatRefusedClose(refusals["mem's own"]),
	} {
		if got, ok := RefusedClose(line); !ok || got != todo {
			t.Errorf("%s: the refusal it quotes is lost: %q, %v", name, got, ok)
		}
	}
}

func FuzzRefusedClose(f *testing.F) {
	f.Add("not popped: the check of `x` found: y")
	f.Add("not closed: `")
	f.Add("not popped: not closed: ``x`")
	f.Fuzz(func(t *testing.T, s string) {
		todo, ok := RefusedClose(s)
		if ok && (todo == "" || strings.ContainsAny(todo, "`\n") || !strings.Contains(s, "`"+todo+"`")) {
			t.Fatalf("RefusedClose(%q) = %q", s, todo)
		}
		if !ok && todo != "" {
			t.Fatalf("RefusedClose(%q) = %q, false", s, todo)
		}
	})
}

// TestCloseReviewClips checks the two limits the review's request has of
// its own: what the person asked is cut at 2,000 characters and the call's
// code at 4,000, counted in characters and not in bytes.
func TestCloseReviewClips(t *testing.T) {
	s := Builtin()
	ask := strings.Repeat("é", 2500)
	code := strings.Repeat("語", 4500)
	got := s.CloseReview(CloseReviewInput{Todo: "t", Ask: ask, Code: code, Result: "r", Transcript: "x"}).Prompt
	if !utf8.ValidString(got) {
		t.Fatal("a cut fell inside a character")
	}
	if n := strings.Count(got, "é"); n != 2000 {
		t.Errorf("%d characters of what the person asked, want 2000", n)
	}
	if n := strings.Count(got, "語"); n != 4000 {
		t.Errorf("%d characters of the call's code, want 4000", n)
	}
	if strings.Count(got, "…") != 2 {
		t.Errorf("want two cuts marked, got %d", strings.Count(got, "…"))
	}
	// The question names the todo, and comes last.
	prompt := s.CloseReview(CloseReviewInput{Todo: "rerun the suite 20 times", Result: "done"}).Prompt
	last := prompt[strings.LastIndex(prompt, "\n")+1:]
	if !strings.Contains(last, "rerun the suite 20 times") || !strings.HasSuffix(last, "?") {
		t.Errorf("the request ends with %q", last)
	}
	if !strings.Contains(last, "not of other work") {
		t.Errorf("the question does not ask first whether the result is this todo's own: %q", last)
	}
}

// TestJudgesAnswerInJSON ties the two judging prompts to the answer the
// harness reads: one JSON object with "ok" and "reason", and for a goal
// the "basis" field.
func TestJudgesAnswerInJSON(t *testing.T) {
	s := Builtin()
	review := s.CloseReview(CloseReviewInput{Todo: "t"}).System
	for _, want := range []string{`{"ok": true, "reason": "`, `{"ok": false, "reason": "`, "on one line"} {
		if !strings.Contains(review, want) {
			t.Errorf("the review's instructions do not show %q", want)
		}
	}
	judge := s.GoalJudge(GoalJudgeInput{Condition: "c"}).System
	for _, want := range []string{`"basis": "conversation"`, `"basis": "user"`, `{"ok": false, "reason": "`, "on one line"} {
		if !strings.Contains(judge, want) {
			t.Errorf("the judge's instructions do not show %q", want)
		}
	}
}

func TestTokenCount(t *testing.T) {
	for n, want := range map[int]string{
		-5: "0", 0: "0", 850: "850", 999: "999", 1000: "1K", 1499: "1K", 1500: "2K", 12345: "12K", 44000: "44K",
		999499: "999K", 999500: "1.0M", 1234567: "1.2M", 20000000: "20.0M",
	} {
		if got := tokenCount(n); got != want {
			t.Errorf("tokenCount(%d) = %q, want %q", n, got, want)
		}
	}
}

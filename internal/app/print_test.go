package app

import (
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// A reply's last line in gnosh's own marker, after the answer, is a model
// repeating the line gnosh put after the prompt: it is not part of a
// run's result. Anything else is left as the model wrote it.
func TestDropEchoedLine(t *testing.T) {
	cases := map[string]string{
		"ready\n<gnosh>Use mem and Gno whenever reasonable: work things out": "ready",
		"ready\n\n<gnosh>Use mem and Gno whenever reasonable.</gnosh>\n":     "ready",
		"<gnosh>Use mem and Gno whenever reasonable.</gnosh>":                "<gnosh>Use mem and Gno whenever reasonable.</gnosh>",
		"It is written <gnosh>.":                     "It is written <gnosh>.",
		"first\n<gnosh>a note\nthat runs on</gnosh>": "first\n<gnosh>a note\nthat runs on</gnosh>",
		"plain answer\n":                             "plain answer\n",
		"two lines\nof answer":                       "two lines\nof answer",
	}
	for in, want := range cases {
		if got := dropEchoedLine(in); got != want {
			t.Errorf("dropEchoedLine(%q) = %q, want %q", in, got, want)
		}
	}
}

// In a run, the echo is left out of what is printed and of the result.
func TestRunLeavesOutAnEchoedLine(t *testing.T) {
	w := apptest.NewWorld(t, apptest.Reply{Text: "ready\n<gnosh>Use mem whenever reasonable.</gnosh>"})
	r := run(t, w, "-p", "Reply with the one word: ready").wantCode(t, 0)
	if r.out != "ready\n" {
		t.Errorf("text output = %q", r.out)
	}
	w.Model.Add(apptest.Reply{Text: "ready\n<gnosh>Use mem whenever reasonable.</gnosh>"})
	r = run(t, w, "-p", "--output-format", "json", "again").wantCode(t, 0)
	if obj := jsonLines(t, r.out)[0]; obj["result"] != "ready" {
		t.Errorf("json result = %v", obj["result"])
	}
}

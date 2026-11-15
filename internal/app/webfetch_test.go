package app

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
	"github.com/gnosh-ai/gnosh/internal/tools/webtool"
)

// WebFetch: the page is read by a side request with the model's request,
// and the model reads the answer; a page is kept for the next call; a long
// page says how to read on; what is not on the open web is refused before
// anybody is asked.
func TestWebFetch(t *testing.T) {
	var fetched atomic.Int32
	long := strings.Repeat("filler text. ", 9000) + "THE END"
	old := fetchPage
	fetchPage = func(ctx context.Context, raw string, o webtool.Options) (webtool.Page, error) {
		fetched.Add(1)
		if !strings.HasPrefix(o.UserAgent, "gnosh/") {
			t.Errorf("the user agent is %q", o.UserAgent)
		}
		text := "# The docs\n\nThe answer is 42, and the command is `make it`.\n"
		if strings.Contains(raw, "long") {
			text = long
		}
		return webtool.Page{URL: raw, Code: 200, Status: "OK", ContentType: "text/html", Bytes: len(text), Text: text}, nil
	}
	defer func() { fetchPage = old }()

	var sidePrompts []string
	main := 0
	w := apptest.NewWorld(t)
	w.Model.Respond(func(n int, r apptest.Request) *apptest.Reply {
		body := string(r.Body)
		if strings.Contains(body, "<page>") {
			sidePrompts = append(sidePrompts, body)
			if strings.Contains(body, "THE END") || strings.Contains(body, "filler") {
				return &apptest.Reply{Text: "It is filler."}
			}
			return &apptest.Reply{Text: "The answer is 42; the command is `make it`."}
		}
		main++
		switch main {
		case 1:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c1", Name: "WebFetch", Args: `{"url":"http://docs.example.org/page","prompt":"what is the answer?"}`}}}
		case 2:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c2", Name: "WebFetch", Args: `{"url":"https://docs.example.org/page","prompt":"and the command?"}`}}}
		case 3:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c3", Name: "WebFetch", Args: `{"url":"https://docs.example.org/long","prompt":"summarize"}`}}}
		case 4:
			return &apptest.Reply{Calls: []apptest.Call{{ID: "c4", Name: "WebFetch", Args: `{"url":"http://localhost:8080/admin","prompt":"look"}`}}}
		}
		return &apptest.Reply{Text: "Done."}
	})
	run(t, w, "-y", "-p", "look up the docs").wantCode(t, 0)

	first := toolResult(t, w, 2, "c1")
	has(t, "the first fetch's result", first, "The answer is 42")
	if len(sidePrompts) < 1 || !strings.Contains(sidePrompts[0], "what is the answer?") || !strings.Contains(sidePrompts[0], "The docs") || !strings.Contains(sidePrompts[0], "https://docs.example.org/page") {
		t.Fatalf("the side request: %v", sidePrompts)
	}
	// The same page again, from the cache: fetched once for both calls,
	// and http was asked for as https.
	has(t, "the second fetch's result", toolResult(t, w, 4, "c2"), "make it")
	// The long page: what is left, and the offset that reads on.
	has(t, "the long page's result", toolResult(t, w, 6, "c3"), "It is filler.", "characters were read", "WebFetch with offset 100000 reads on")
	if n := fetched.Load(); n != 2 {
		t.Errorf("the pages were fetched %d times, want 2 (the first page once, from the cache the second time, and the long one)", n)
	}
	// localhost: refused before anything is fetched or anybody is asked.
	has(t, "the fetch from localhost", toolResult(t, w, 7, "c4"), "use curl through the shell")
}

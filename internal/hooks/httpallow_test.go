package hooks

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// allowedHttpHookUrls' patterns: * is any run of characters; the scheme
// and host match whatever their case, and a host's trailing dot is no part
// of it; a URL with a user name in it fits none.
func TestURLAllowed(t *testing.T) {
	patterns := []string{"https://hooks.example.com/*", "http://localhost:*", "HTTPS://Upper.Example.org./x"}
	for _, c := range []struct {
		url  string
		want bool
	}{
		{"https://hooks.example.com/check", true},
		{"https://HOOKS.Example.COM./check?a=1", true},
		{"https://hooks.example.com", false},
		{"https://hooks.example.com.evil.example/check", false},
		{"http://hooks.example.com/check", false},
		{"https://hooks.example.com/Check", true},
		{"http://localhost:8080/x", true},
		{"http://localhost/x", false},
		{"http://localhost:80@elsewhere.example/x", false},
		{"https://user@hooks.example.com/check", false},
		{"https://upper.example.org/x", true},
		{"https://upper.example.org/X", false},
		{"not a url", false},
		{"/relative/only", false},
	} {
		if got := URLAllowed(c.url, patterns); got != c.want {
			t.Errorf("URLAllowed(%q) = %v, want %v", c.url, got, c.want)
		}
	}
	if URLAllowed("https://hooks.example.com/check", []string{}) {
		t.Error("an empty list let a URL through")
	}
	for _, c := range []struct {
		pattern, s string
		want       bool
	}{
		{"*", "", true}, {"a*b*c", "aXbYc", true}, {"a*b*c", "aXbY", false}, {"**", "x", true},
		{"a*a", "aaa", true}, {"a*ab", "aaab", true}, {"", "", true}, {"", "x", false}, {"x*", "x", true},
	} {
		if got := wildMatch(c.pattern, c.s); got != c.want {
			t.Errorf("wildMatch(%q, %q) = %v", c.pattern, c.s, got)
		}
	}
}

// An http hook whose URL allowedHttpHookUrls does not let it post to is not
// run, as if it were not there, and Check warns of it; an empty list runs
// no http hook. httpHookAllowedEnvVars narrows the variables its headers
// take to those both lists name.
func TestHTTPHookLimits(t *testing.T) {
	var mu sync.Mutex
	var calls int
	var auth, other string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		mu.Lock()
		calls++
		auth, other = r.Header.Get("Authorization"), r.Header.Get("X-Other")
		mu.Unlock()
		_, _ = io.WriteString(w, `{"hookSpecificOutput": {"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": "no"}}`)
	}))
	defer srv.Close()
	opts := options(t)
	env := opts.Environ
	opts.Environ = func() []string { return append(env(), "TOKEN=s3cret", "OTHER=hidden") }
	hook := Hook{Event: PreToolUse, Matcher: "Bash", URL: srv.URL + "/check",
		Headers: map[string]string{"Authorization": "Bearer ${TOKEN}", "X-Other": "[$OTHER]"}, AllowedEnvVars: []string{"TOKEN", "OTHER"}}

	for _, list := range [][]string{{"https://hooks.example.com/*"}, {}} {
		opts.HTTPURLs = list
		problems := Check(Config{Hooks: []Hook{hook}}, opts)
		if len(problems) != 1 || !problems[0].Warning || !strings.Contains(problems[0].Message, "allowedHttpHookUrls does not let an http hook post to, so it never runs") {
			t.Errorf("Check with %q: %+v", list, problems)
		}
		out := run(t, newRunner(t, opts, hook), input(PreToolUse))
		if out.Blocked || out.PermissionDecision != "" || len(out.Records) != 0 {
			t.Errorf("a hook allowedHttpHookUrls %q does not let post ran: %+v", list, out)
		}
	}
	mu.Lock()
	if calls != 0 {
		t.Errorf("the server was called %d times", calls)
	}
	mu.Unlock()

	// A list it fits lets it run; httpHookAllowedEnvVars lets TOKEN alone
	// into its headers.
	opts.HTTPURLs = []string{"http://127.0.0.1:*"}
	opts.HTTPEnvVars = []string{"TOKEN", "UNUSED"}
	if problems := Check(Config{Hooks: []Hook{hook}}, opts); len(problems) != 0 {
		t.Errorf("Check: %+v", problems)
	}
	out := run(t, newRunner(t, opts, hook), input(PreToolUse))
	if out.PermissionDecision != Deny {
		t.Errorf("the hook's answer: %+v", out)
	}
	mu.Lock()
	if calls != 1 || auth != "Bearer s3cret" || other != "[]" {
		t.Errorf("calls %d, headers %q %q", calls, auth, other)
	}
	mu.Unlock()

	// An empty httpHookAllowedEnvVars lets none in.
	opts.HTTPEnvVars = []string{}
	run(t, newRunner(t, opts, hook), input(PreToolUse))
	mu.Lock()
	if calls != 2 || auth != "Bearer" && auth != "Bearer " || other != "[]" {
		t.Errorf("calls %d, headers %q %q", calls, auth, other)
	}
	mu.Unlock()
}

package deliver

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// Whatever a model writes as the tool's arguments, reading them never
// crashes, and what is read is fit to show and to act on: a label of one
// line and of the length a label may have, a target with nothing in it
// that cannot be seen, and a link only where the target is an http or
// https address.
func FuzzReadArguments(f *testing.F) {
	for _, seed := range []string{
		`{"label":"demo video"}`,
		`{"label":"demo video","target":"out/demo.mp4"}`,
		`{"label":"demo video","target":"renders/demo.mp4","host":"render"}`,
		`{"label":"page","target":"HTTPS://example.com/landing"}`,
		`{"label":"notes","target":"file:///home/dev/notes.md"}`,
		`{"label":"x","target":"s3://bucket/x"}`,
		`{"label":"a\nb\u001b[31m","target":" a\tb "}`,
		`{"label":7}`, `{"label":"x","path":"y"}`, `{"label":"x","host":["render"]}`,
		`{"label":"x","target":null,"host":null}`,
		``, `null`, `[]`, `{`, `{"label":"x","label":"y"}`,
	} {
		f.Add(seed)
	}
	tl := &tool{j: &join{hosts: map[string]string{}}}
	f.Fuzz(func(t *testing.T, raw string) {
		req, err := tl.read([]byte(raw))
		if err != nil {
			if strings.TrimSpace(err.Error()) == "" {
				t.Errorf("a refusal with no words")
			}
			return
		}
		if n := utf8.RuneCountInString(req.label); n == 0 || n > maxLabel || strings.ContainsFunc(req.label, unicode.IsControl) || req.label != strings.TrimSpace(req.label) {
			t.Errorf("the label %q was let through", req.label)
		}
		if strings.ContainsFunc(req.target, unicode.IsControl) {
			t.Errorf("the target %q was let through", req.target)
		}
		if req.link != (strings.HasPrefix(req.target, "http://") || strings.HasPrefix(req.target, "https://")) {
			t.Errorf("the target %q is read as a link: %v", req.target, req.link)
		}
		if req.target == "" && req.host != "" || req.link && req.host != "" {
			t.Errorf("a host (%q) is kept for a call that names no file", req.host)
		}
	})
}

// Whatever path a request names, it is read as a token and at most a
// name, neither of which holds a slash, or it is refused.
func FuzzRequestPath(f *testing.F) {
	for _, seed := range []string{
		"/", "", "/token", "/token/ad.mp4", "/token/", "//token/ad.mp4", "/token//ad.mp4",
		"/token/../../etc/passwd", "/../token", "token/ad.mp4", "/token/a/b", "/\x00/\xff",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, path string) {
		token, name, ok := splitPath(path)
		if !ok {
			if token != "" || name != "" {
				t.Errorf("a refused path %q gave %q and %q", path, token, name)
			}
			return
		}
		if token == "" || strings.Contains(token, "/") || strings.Contains(name, "/") {
			t.Errorf("the path %q is read as the token %q and the name %q", path, token, name)
		}
		if want := "/" + token; path != want && path != want+"/"+name {
			t.Errorf("the path %q is read as the token %q and the name %q, which do not make it up", path, token, name)
		}
	})
}

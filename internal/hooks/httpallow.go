package hooks

import (
	"net/url"
	"strings"
)

// This file is Claude Code's allowedHttpHookUrls (Options.HTTPURLs): the
// URL patterns an http hook may post to.

// urlAllowed reports whether an http hook may post to u: always when no
// list is in force.
func (r *Runner) urlAllowed(u string) bool {
	return r.opts.HTTPURLs == nil || URLAllowed(u, r.opts.HTTPURLs)
}

// URLAllowed reports whether the URL u fits one of the patterns, as
// Claude Code's allowedHttpHookUrls fits them: * stands for any run of
// characters, and anything else for itself; the scheme and the host are
// matched whatever their case, and a host's trailing dot (a fully
// qualified name's) is no part of it, as DNS has it.
//
// A URL with a user name in it fits none: in
// "http://localhost:80@elsewhere.example/" the host is elsewhere.example,
// though "http://localhost:*" would fit the text.
func URLAllowed(u string, patterns []string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed.User != nil || parsed.Host == "" {
		return false
	}
	nu := normalURL(u)
	for _, p := range patterns {
		if wildMatch(normalURL(p), nu) {
			return true
		}
	}
	return false
}

// normalURL lowers the case of a URL's (or a pattern's) scheme and host,
// and takes a trailing dot off the host; the rest is left as it is.
func normalURL(s string) string {
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok {
		return s
	}
	auth, tail := rest, ""
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		auth, tail = rest[:i], rest[i:]
	}
	user := ""
	if i := strings.LastIndex(auth, "@"); i >= 0 {
		user, auth = auth[:i+1], auth[i+1:]
	}
	host, port := auth, ""
	if strings.HasPrefix(host, "[") {
		if i := strings.Index(host, "]"); i >= 0 {
			host, port = host[:i+1], host[i+1:]
		}
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host, port = host[:i], host[i:]
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return strings.ToLower(scheme) + "://" + user + host + port + tail
}

// wildMatch reports whether s fits pattern, where * stands for any run of
// characters, none included, and every other byte for itself.
func wildMatch(pattern, s string) bool {
	p, i := 0, 0
	star, mark := -1, 0
	for i < len(s) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, i
			p++
		case p < len(pattern) && pattern[p] == s[i]:
			p++
			i++
		case star >= 0:
			// Back to the last *, which takes one more byte.
			p = star + 1
			mark++
			i = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}

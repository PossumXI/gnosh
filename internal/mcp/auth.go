package mcp

import (
	"context"
	"net/http"
	"strings"
	"sync"
)

// Authorizer gives the requests to a server reached at a URL the token of
// the person's sign-in to it (LoginStore), and renews it when the server
// refuses it: with the refresh token, when there is one. A server whose
// Authorization header is configured is never given one: the configured
// credential is the one in force (StartHTTP).
type Authorizer struct {
	// Logins is where the sign-ins are kept, and Server the server's URL,
	// the sign-in's key.
	Logins *LoginStore
	Server string
	// HTTP makes the refresh's request; nil is Go's default client.
	HTTP *http.Client

	mu sync.Mutex
}

// token is the access token to send, or "" when there is no sign-in.
func (a *Authorizer) token() string {
	l, ok := a.Logins.Get(a.Server)
	if !ok {
		return ""
	}
	return l.AccessToken
}

// renew is told that the server refused the token refused, and reports
// whether there is another to try: one another session got meanwhile, or
// a refreshed one. A token that cannot be renewed is forgotten (the
// client's registration is kept), and the server then needs a sign-in.
func (a *Authorizer) renew(ctx context.Context, refused string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	l, ok := a.Logins.Get(a.Server)
	switch {
	case !ok:
		return false
	case l.AccessToken != refused:
		return true
	case l.RefreshToken == "":
		l.AccessToken = ""
		_ = a.Logins.Put(l)
		return false
	}
	t, err := Refresh(ctx, a.HTTP, l)
	if err != nil {
		l.AccessToken, l.RefreshToken = "", ""
		_ = a.Logins.Put(l)
		return false
	}
	l.Token = t
	return a.Logins.Put(l) == nil
}

// hasHeader reports whether headers name the header key, in any case.
func hasHeader(headers map[string]string, key string) bool {
	for k := range headers {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}

package sandbox

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Claude Code's allowedDomains and deniedDomains: a name, *.name for the
// names below it, an address (IPv6 in brackets), each with :port for that
// port alone; denied wins.
func TestDomains(t *testing.T) {
	d := Domains{Allowed: []string{"github.com", "*.npmjs.org", "api.example.com:443", "[::1]:8080", "10.0.0.5"}, Denied: []string{"bad.npmjs.org", "github.com:22"}}
	for _, c := range []struct {
		host string
		port int
		want bool
	}{
		{"github.com", 443, true}, {"GitHub.com.", 80, true}, {"github.com", 22, false}, {"api.github.com", 443, false},
		{"registry.npmjs.org", 443, true}, {"npmjs.org", 443, false}, {"bad.npmjs.org", 443, false}, {"xnpmjs.org", 443, false},
		{"api.example.com", 443, true}, {"api.example.com", 80, false}, {"[::1]", 8080, true}, {"::1", 8080, true}, {"::1", 80, false},
		{"10.0.0.5", 1, true}, {"", 443, false}, {"elsewhere.example", 443, false},
	} {
		if got, why := d.Allows(c.host, c.port); got != c.want {
			t.Errorf("Allows(%q, %d) = %v (%s)", c.host, c.port, got, why)
		}
	}
	if _, why := d.Allows("elsewhere.example", 443); !strings.Contains(why, "not in sandbox.network.allowedDomains") {
		t.Errorf("the reason: %s", why)
	}
}

// The proxy carries a CONNECT and a plain request to an allowed host,
// refuses another with a 403 that says why, and refuses a name of the
// internet's that leads to the machine itself.
func TestProxy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "hello from "+r.URL.Path) }))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	var told []string
	p, err := ListenProxy("tcp", "127.0.0.1:0", Domains{Allowed: []string{"127.0.0.1", "rebound.example"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	p.Told = func(host, why string) { told = append(told, host) }
	p.lookup = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		if host == "rebound.example" {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return net.DefaultResolver.LookupIPAddr(ctx, host)
	}
	proxyURL, _ := url.Parse("http://" + p.Addr().String())
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}

	// A plain request.
	resp, err := client.Get(srv.URL + "/plain")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "hello from /plain" {
		t.Errorf("a plain request: %d %q", resp.StatusCode, body)
	}

	// A CONNECT, by hand.
	c, err := net.Dial("tcp", p.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(c, "CONNECT "+u.Host+" HTTP/1.1\r\nHost: "+u.Host+"\r\n\r\n")
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	if !strings.Contains(line, "200") {
		t.Fatalf("CONNECT: %q", line)
	}
	_, _ = br.ReadString('\n')
	_, _ = io.WriteString(c, "GET /tunneled HTTP/1.1\r\nHost: "+u.Host+"\r\nConnection: close\r\n\r\n")
	rest, _ := io.ReadAll(br)
	c.Close()
	if !strings.Contains(string(rest), "hello from /tunneled") {
		t.Errorf("through the tunnel: %q", rest)
	}

	// A host not allowed, and a name that leads to the machine itself.
	for _, target := range []string{"http://elsewhere.example/x", "http://rebound.example:" + u.Port() + "/x"} {
		resp, err := client.Get(target)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden || !strings.Contains(string(body), "gnosh's sandbox did not let the command reach") {
			t.Errorf("%s: %d %q", target, resp.StatusCode, body)
		}
	}
	if len(told) != 2 || told[0] != "elsewhere.example" || told[1] != "rebound.example" {
		t.Errorf("told: %q", told)
	}
}

// A host neither list names is asked about once for the session, however
// many connections reach for it at once, and the answer kept; a denied
// host is never asked about.
func TestProxyAsks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer srv.Close()
	p, err := ListenProxy("tcp", "127.0.0.1:0", Domains{Denied: []string{"denied.example"}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var mu sync.Mutex
	asked := map[string]int{}
	p.Ask = func(ctx context.Context, host string, port int) (bool, string) {
		mu.Lock()
		asked[host]++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		return host == "127.0.0.1", "the person said no to " + host
	}
	proxyURL, _ := url.Parse("http://" + p.Addr().String())
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	get := func(target string) (int, string) {
		resp, err := client.Get(target)
		if err != nil {
			return 0, err.Error()
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, body := get(srv.URL + "/x"); code != 200 || body != "ok" {
				t.Errorf("an approved host: %d %q", code, body)
			}
		}()
	}
	wg.Wait()
	if code, body := get("http://other.example/x"); code != http.StatusForbidden || !strings.Contains(body, "the person said no to other.example") {
		t.Errorf("a refused host: %d %q", code, body)
	}
	get("http://other.example/y")
	get("http://denied.example/")
	mu.Lock()
	defer mu.Unlock()
	if asked["127.0.0.1"] != 1 || asked["other.example"] != 1 || asked["denied.example"] != 0 {
		t.Errorf("asked: %v", asked)
	}
}

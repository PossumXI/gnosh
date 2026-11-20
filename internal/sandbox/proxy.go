package sandbox

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Proxy is a sandboxed command's one way out to the network: an HTTP proxy
// outside the sandbox that takes its requests (CONNECT, for HTTPS and
// anything else over TCP, and plain HTTP for http:// addresses) to the
// hosts Domains allows, and refuses the rest with a 403 that says why. A
// host whose every address is the machine's own or its network's (a name
// that leads to 127.0.0.1, a cloud's metadata address) is refused unless
// it is named as such: a command may not reach the machine's own servers
// through a name of the internet's.
type Proxy struct {
	domains Domains
	ln      net.Listener
	srv     *http.Server
	// lookup finds a host's addresses; net's own, but in tests.
	lookup func(ctx context.Context, host string) ([]net.IPAddr, error)
	// Told is called with each host refused, for the person; nil is
	// nothing.
	Told func(host, why string)
	// Also lets a host neither list names through all the same (a
	// WebFetch(domain:...) allow rule, as Claude Code takes them); Ask
	// asks the person about a host nothing lets through or refuses, once
	// per host for the session, the answer kept. Nil: neither.
	Also func(host string) bool
	Ask  func(ctx context.Context, host string, port int) (yes bool, why string)

	mu       sync.Mutex
	answered map[string]answer
	asking   map[string]*sync.Mutex
}

// proxyDialWait bounds the making of one connection outward.
const proxyDialWait = 30 * time.Second

// ListenProxy starts a proxy on network and address ("unix" and a socket's
// path, or "tcp" and "127.0.0.1:0").
func ListenProxy(network, address string, d Domains) (*Proxy, error) {
	ln, err := net.Listen(network, address)
	if err != nil {
		return nil, err
	}
	p := &Proxy{domains: d, ln: ln, lookup: net.DefaultResolver.LookupIPAddr}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = p.srv.Serve(ln) }()
	return p, nil
}

// Addr is where the proxy listens.
func (p *Proxy) Addr() net.Addr { return p.ln.Addr() }

// Close stops the proxy and the connections it carries.
func (p *Proxy) Close() error { return p.srv.Close() }

// ServeHTTP is one request of a sandboxed command's.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, port := r.URL.Hostname(), r.URL.Port()
	if r.Method == http.MethodConnect {
		h, pt, err := net.SplitHostPort(r.Host)
		if err != nil {
			p.refuse(w, r.Host, "a CONNECT names a host and a port")
			return
		}
		host, port = h, pt
	}
	if port == "" {
		port = "80"
		if r.URL.Scheme == "https" {
			port = "443"
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		p.refuse(w, host, "the port "+port+" is not one")
		return
	}
	if ok, why := p.lets(r.Context(), host, n); !ok {
		p.refuse(w, host, why)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), proxyDialWait)
	conn, why := p.dial(ctx, host, port)
	cancel()
	if conn == nil {
		p.refuse(w, host, why)
		return
	}
	if r.Method == http.MethodConnect {
		p.tunnel(w, conn)
		return
	}
	p.forward(w, r, conn)
}

// dial reaches host:port, refusing a host whose every address is local
// unless the host is named as an address or as localhost; or says why
// not.
func (p *Proxy) dial(ctx context.Context, host, port string) (net.Conn, string) {
	h := normalHost(host)
	named := h == "localhost" || net.ParseIP(h) != nil
	addrs, err := p.lookup(ctx, h)
	if err != nil || len(addrs) == 0 {
		return nil, h + " could not be found (" + errText(err) + ")"
	}
	var d net.Dialer
	tried := ""
	for _, a := range addrs {
		if !named && local(a.IP) {
			tried = h + " leads to " + a.IP.String() + ", an address of this machine or its network"
			continue
		}
		conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(a.IP.String(), port))
		if err == nil {
			return conn, ""
		}
		tried = h + " could not be reached (" + err.Error() + ")"
	}
	return nil, tried
}

// local reports whether an address is the machine's own, its network's,
// or no address at all.
func local(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// errText is an error's words, or nothing.
func errText(err error) string {
	if err == nil {
		return "no address"
	}
	return err.Error()
}

// tunnel answers a CONNECT and carries bytes both ways.
func (p *Proxy) tunnel(w http.ResponseWriter, out net.Conn) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		out.Close()
		http.Error(w, "the proxy cannot carry this connection", http.StatusInternalServerError)
		return
	}
	in, buf, err := hj.Hijack()
	if err != nil {
		out.Close()
		return
	}
	_, _ = in.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	carry(in, buf, out)
}

// carry copies between the two until both are done.
func carry(in net.Conn, buf *bufio.ReadWriter, out net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(out, buf)
		closeWrite(out)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(in, out)
		closeWrite(in)
	}()
	wg.Wait()
	in.Close()
	out.Close()
}

// closeWrite ends the writing half of a connection, where it has one.
func closeWrite(c net.Conn) {
	if cw, ok := c.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	}
}

// forward sends a plain HTTP request on over conn, and its answer back,
// following no redirect.
func (p *Proxy) forward(w http.ResponseWriter, r *http.Request, conn net.Conn) {
	out := r.Clone(r.Context())
	out.RequestURI = ""
	for _, h := range []string{"Proxy-Connection", "Proxy-Authorization", "Connection", "Keep-Alive", "Te", "Trailer", "Upgrade"} {
		out.Header.Del(h)
	}
	one := make(chan net.Conn, 1)
	one <- conn
	tr := &http.Transport{
		DialContext: func(context.Context, string, string) (net.Conn, error) {
			select {
			case c := <-one:
				return c, nil
			default:
				return nil, errors.New("one connection per request")
			}
		},
		DisableKeepAlives: true,
	}
	defer tr.CloseIdleConnections()
	resp, err := tr.RoundTrip(out)
	if err != nil {
		conn.Close()
		http.Error(w, "the proxy could not send the request on: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// refuse answers that the host may not be reached, and why.
func (p *Proxy) refuse(w http.ResponseWriter, host, why string) {
	if p.Told != nil {
		p.Told(host, why)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, "gnosh's sandbox did not let the command reach "+strings.TrimSpace(host)+": "+why+
		". The person may add it to sandbox.network.allowedDomains, or the command may run outside the sandbox (dangerouslyDisableSandbox), after asking.\n")
}

// lets decides whether the host may be reached: never one deniedDomains
// names; one allowedDomains names, or Also lets through; else as the
// person answered, asked once per host for the session (several
// connections to a new host at once ask once).
func (p *Proxy) lets(ctx context.Context, host string, port int) (bool, string) {
	v, why := p.domains.verdict(host, port)
	switch {
	case v == denied:
		return false, why
	case v == listed:
		return true, ""
	}
	h := normalHost(host)
	if p.Also != nil && p.Also(h) {
		return true, ""
	}
	if p.Ask == nil {
		return false, why
	}
	p.mu.Lock()
	if p.asking == nil {
		p.asking, p.answered = map[string]*sync.Mutex{}, map[string]answer{}
	}
	one := p.asking[h]
	if one == nil {
		one = &sync.Mutex{}
		p.asking[h] = one
	}
	p.mu.Unlock()
	one.Lock()
	defer one.Unlock()
	p.mu.Lock()
	a, done := p.answered[h]
	p.mu.Unlock()
	if !done {
		a.yes, a.why = p.Ask(ctx, h, port)
		if ctx.Err() != nil {
			// (Taken back before it was answered: nothing kept.)
			return false, h + " was not answered for"
		}
		p.mu.Lock()
		p.answered[h] = a
		p.mu.Unlock()
	}
	if !a.yes {
		return false, a.why
	}
	return true, ""
}

// answer is what Ask said of a host, kept for the session.
type answer struct {
	yes bool
	why string
}

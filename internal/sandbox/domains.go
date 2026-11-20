package sandbox

import (
	"net"
	"strconv"
	"strings"
)

// Domains is which hosts a sandboxed command may reach through the
// sandbox's proxy, as Claude Code's sandbox.network.allowedDomains and
// deniedDomains say: an entry is a host name, "*.example.com" for its
// names below it (not itself), or an address, IPv6 in brackets, each with
// ":port" for that port alone. A denied entry wins over an allowed one.
type Domains struct {
	Allowed, Denied []string
}

// Allows reports whether the host, at the port, may be reached, and says
// why not when it may not.
func (d Domains) Allows(host string, port int) (bool, string) {
	host = normalHost(host)
	if host == "" {
		return false, "no host was named"
	}
	for _, e := range d.Denied {
		if entryFits(e, host, port) {
			return false, host + " is in sandbox.network.deniedDomains"
		}
	}
	for _, e := range d.Allowed {
		if entryFits(e, host, port) {
			return true, ""
		}
	}
	return false, host + " is not in sandbox.network.allowedDomains"
}

// normalHost is a host as it is compared: lower case, no trailing dot, no
// brackets around an IPv6 address.
func normalHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	return strings.TrimSuffix(h, ".")
}

// entryFits reports whether one entry of the lists covers the host and
// port.
func entryFits(entry, host string, port int) bool {
	e := strings.ToLower(strings.TrimSpace(entry))
	want := 0
	if h, p, err := net.SplitHostPort(e); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil {
			return false
		}
		e, want = h, n
	} else if strings.HasPrefix(e, "[") && strings.HasSuffix(e, "]") {
		e = e[1 : len(e)-1]
	}
	if want != 0 && want != port {
		return false
	}
	e = strings.TrimSuffix(e, ".")
	if rest, ok := strings.CutPrefix(e, "*."); ok {
		return rest != "" && strings.HasSuffix(host, "."+rest)
	}
	return e == host
}

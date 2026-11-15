package deliver

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// server hands the delivered files of one project over HTTP, on this
// machine only. A file is asked for by its token; the request names no
// path, so nothing that is not in the register can be reached. It reads
// and never writes.
type server struct {
	reg *register
	ln  net.Listener
	srv *http.Server
	// base is where the server answers, as an address begins:
	// "http://127.0.0.1:7777".
	base string
	// asked is the address it was to listen on, and moved says that one
	// was taken and a free port is used in its place.
	asked string
	moved bool
}

// serve starts a server for the register on addr, an address of this
// machine with a port. When that port cannot be had, any free port of the
// same address is taken, and the server says so (moved). An error means
// there is no server.
func serve(reg *register, addr string) (*server, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("%q is not an address with a port", addr)
	}
	ln, err := net.Listen("tcp", addr)
	moved := false
	if err != nil {
		other, err2 := net.Listen("tcp", net.JoinHostPort(host, "0"))
		if err2 != nil {
			return nil, fmt.Errorf("%s cannot be listened on (%v), and no other port of %s can either (%v)", addr, err, host, err2)
		}
		ln, moved = other, true
	}
	// The option only takes an address of this machine alone; this is the
	// last look, at the address the system really gave.
	if tcp, ok := ln.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		ln.Close()
		return nil, fmt.Errorf("%s is not an address of this machine alone, and delivered files are served to this machine only", ln.Addr())
	}
	s := &server{reg: reg, ln: ln, base: "http://" + ln.Addr().String(), asked: addr, moved: moved}
	s.srv = &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 10 * time.Second,
		// What the server has to say of a broken connection goes
		// nowhere: its own log would write into the terminal the
		// interface draws on.
		ErrorLog: log.New(io.Discard, "", 0),
	}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

// close stops the server and whatever it is sending.
func (s *server) close() {
	if s != nil {
		_ = s.srv.Close()
	}
}

// address is where an entry of the register is served: the server's own
// address, the token, and the file's name.
func (s *server) address(rec record) string {
	return s.base + "/" + rec.Token + "/" + url.PathEscape(rec.Name)
}

// ServeHTTP answers one request. The path is /<token> or
// /<token>/<name>, where the name is the one the file was delivered
// under; anything else, and a token that is not in the register, is not
// found, in the same words, so that an answer never says how close a
// guess was.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-cache")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		h.Set("Allow", "GET, HEAD")
		http.Error(w, "gnosh serves delivered files to be read: only GET and HEAD are answered here.", http.StatusMethodNotAllowed)
		return
	}
	const none = "There is nothing at this address. gnosh serves each delivered file at an address of its own, which the session that delivered it shows; nothing is listed."
	token, name, ok := splitPath(r.URL.Path)
	if !ok {
		http.Error(w, none, http.StatusNotFound)
		return
	}
	rec, ok := s.reg.byToken(token)
	if !ok || (name != "" && name != rec.Name) {
		http.Error(w, none, http.StatusNotFound)
		return
	}
	f, st, err := openRegular(rec.File)
	if err != nil {
		http.Error(w, "The file that was delivered at this address is not there any more, or is no longer a file that can be read.", http.StatusNotFound)
		return
	}
	defer f.Close()
	h.Set("Content-Type", mediaType(rec.Name))
	// ServeContent answers ranges, which a browser needs to play and to
	// seek in a video, and conditional requests; it writes no body for
	// HEAD, and guesses no type, because one is set.
	http.ServeContent(w, r, "", st.ModTime(), f)
}

// splitPath reads a request's path as a token and a name. The name is
// empty when the path is the token alone.
func splitPath(p string) (token, name string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
	switch {
	case !strings.HasPrefix(p, "/") || parts[0] == "" || len(parts) > 2:
		return "", "", false
	case len(parts) == 2:
		name = parts[1]
	}
	return parts[0], name, true
}

// openRegular opens a file to serve it, and returns it with what the
// system says of it. Only a regular file is opened, and never through a
// link: a path that has become a link since the file was delivered leads
// wherever whoever made the link wants, which is not what was delivered.
func openRegular(path string) (*os.File, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, errors.New("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || !os.SameFile(before, st) {
		f.Close()
		return nil, nil, errors.New("not the regular file that was there a moment ago")
	}
	return f, st, nil
}

package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// bwrapArgs is the bubblewrap command line that runs argv in dir under the
// policy. stat says whether a path is there, and whether it is a folder.
//
// The machine is mounted read-only, with a /dev and a /proc of the
// sandbox's own; the sandbox's temporary folder is /tmp; the folders it
// may write in are mounted over that writable, then the read-only paths
// inside them, then the denied ones hidden (an empty folder, or the null
// device for a file), then the paths inside those it may read after all.
// A later mount covers an earlier one, which is what makes that order the
// rule. It has its own processes (so it can signal no other) and its own
// network, where nothing is reachable, unless the policy gives it the
// machine's; it may make no user namespace of its own where bubblewrap
// can refuse it one. It dies with gnosh.
//
// With the filesystem left open (FilesystemOff) the machine is mounted as
// it is, writable, and nothing of the files is confined.
func bwrapArgs(bwrap string, p Policy, dir string, argv []string, stat func(string) (isDir, ok bool)) []string {
	there := func(s string) bool { _, ok := stat(s); return ok }
	a := []string{bwrap, "--die-with-parent"}
	if p.blockUserns {
		a = append(a, "--unshare-user", "--disable-userns")
	}
	a = append(a, "--unshare-pid", "--unshare-ipc")
	if !p.Network {
		a = append(a, "--unshare-net")
	}
	if p.FilesystemOff {
		a = append(a, "--bind", "/", "/", "--dev", "/dev")
	} else {
		a = append(a, "--ro-bind", "/", "/", "--dev", "/dev")
	}
	if !p.HostProc {
		// (Else the machine's /proc stays, mounted with the rest.)
		a = append(a, "--proc", "/proc")
	}
	// The machine's sockets out of sight: a read-only mount does not stop
	// a connection to one, and systemd's (a job outside, as root with its
	// private socket), D-Bus's and docker's lie under /run (the sandbox
	// review). (/var/run is a link to it where it is not a folder.)
	for _, r := range []string{"/run", "/var/run"} {
		if isDir, ok := stat(r); ok && isDir && !isLink(r) && !p.AllUnixSockets {
			a = append(a, "--tmpfs", r)
		}
	}
	if p.FilesystemOff {
		a = append(a, "--chdir", filepath.Clean(dir), "--")
		if p.Proxy != "" && p.Relay != "" {
			// (The proxy's socket where it is: the machine's /tmp is the
			// sandbox's.)
			a = append(a, p.Relay, RelayWord, p.Proxy)
		}
		return append(a, argv...)
	}
	if p.Temp != "" && there(p.Temp) {
		a = append(a, "--bind", p.Temp, "/tmp")
	}
	for _, w := range kept(p.Write, there) {
		a = append(a, "--bind", w, w)
	}
	if _, ok := insideTemp(p); ok && p.Relay != "" && there(p.Relay) {
		// (gnosh may itself be under the real /tmp, out of sight inside.)
		a = append(a, "--ro-bind", p.Relay, p.Relay)
	}
	for _, r := range kept(p.ReadOnly, there) {
		a = append(a, "--ro-bind", r, r)
	}
	// gnosh's own files after the read-only paths, which may hold them
	// (the session's folder under gnosh's data folder): the shell must
	// write where it ends up.
	for _, e := range p.Expose {
		if e.Path == "" || !there(e.Path) {
			continue
		}
		if e.Write {
			a = append(a, "--bind", e.Path, e.Path)
		} else {
			a = append(a, "--ro-bind", e.Path, e.Path)
		}
	}
	denied := kept(p.DenyRead, there)
	// A denied folder is an empty one, made read-only last: the paths
	// opened again inside it need their places made in it first.
	var emptied []string
	hide := func(d string) {
		if isDir, _ := stat(d); isDir {
			a = append(a, "--tmpfs", d)
			emptied = append(emptied, d)
		} else {
			a = append(a, "--ro-bind", "/dev/null", d)
		}
	}
	for _, d := range denied {
		hide(d)
	}
	var opened []string
	for _, r := range kept(p.AllowRead, there) {
		// (Only a path inside a denied one needs opening again.)
		if slices.ContainsFunc(denied, func(d string) bool { return r != d && under(r, d) }) {
			a = append(a, "--ro-bind", r, r)
			opened = append(opened, r)
		}
	}
	// The folders it writes in, and gnosh's own files, inside a denied
	// path come back as they were: a denyRead of the home folder had taken
	// the project from it, and the files its shell needs (the sandbox
	// review).
	for _, w := range kept(p.Write, there) {
		if slices.ContainsFunc(denied, func(d string) bool { return under(w, d) }) {
			a = append(a, "--bind", w, w)
			opened = append(opened, w)
		}
	}
	for _, e := range p.Expose {
		if e.Path != "" && there(e.Path) && slices.ContainsFunc(denied, func(d string) bool { return under(e.Path, d) }) {
			if e.Write {
				a = append(a, "--bind", e.Path, e.Path)
			} else {
				a = append(a, "--ro-bind", e.Path, e.Path)
			}
		}
	}
	if len(opened) > 0 {
		// (And the read-only paths inside those, again.)
		for _, r := range kept(p.ReadOnly, there) {
			if slices.ContainsFunc(opened, func(o string) bool { return r != o && under(r, o) }) {
				a = append(a, "--ro-bind", r, r)
			}
		}
	}
	// A denied path inside one opened again stays denied: the narrower
	// wins, as in Claude Code.
	for _, d := range present(p.DenyRead, there) {
		if slices.ContainsFunc(opened, func(r string) bool { return d != r && under(d, r) }) {
			hide(d)
		}
	}
	for _, d := range emptied {
		a = append(a, "--remount-ro", d)
	}
	a = append(a, "--chdir", filepath.Clean(dir), "--")
	if inside, ok := insideTemp(p); ok && p.Relay != "" {
		// The way out to the proxy: gnosh, run inside, relays the
		// sandbox's own loopback to the proxy's socket, which is in the
		// sandbox's /tmp.
		a = append(a, p.Relay, RelayWord, inside)
	}
	return append(a, argv...)
}

// insideTemp is the proxy's socket as the sandbox sees it: in its /tmp,
// which is Temp.
func insideTemp(p Policy) (string, bool) {
	if p.Proxy == "" || p.Temp == "" {
		return "", false
	}
	rel, err := filepath.Rel(p.Temp, p.Proxy)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.Join("/tmp", rel), true
}

// isLink reports whether the path is a symbolic link.
func isLink(p string) bool {
	info, err := os.Lstat(p)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

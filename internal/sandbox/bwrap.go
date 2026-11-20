package sandbox

import (
	"path/filepath"
	"slices"
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
// machine's. It dies with gnosh.
func bwrapArgs(bwrap string, p Policy, dir string, argv []string, stat func(string) (isDir, ok bool)) []string {
	there := func(s string) bool { _, ok := stat(s); return ok }
	a := []string{bwrap, "--die-with-parent", "--unshare-pid", "--unshare-ipc"}
	if !p.Network {
		a = append(a, "--unshare-net")
	}
	a = append(a, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc")
	if p.Temp != "" && there(p.Temp) {
		a = append(a, "--bind", p.Temp, "/tmp")
	}
	for _, w := range kept(p.Write, there) {
		a = append(a, "--bind", w, w)
	}
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
	for _, r := range kept(p.ReadOnly, there) {
		a = append(a, "--ro-bind", r, r)
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
	return append(a, argv...)
}

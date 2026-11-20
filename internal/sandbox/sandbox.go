// Package sandbox runs a shell command inside the operating system's own
// sandbox, as Claude Code's Bash sandbox does: on Linux under bubblewrap
// (bwrap), on macOS under Seatbelt (sandbox-exec). A sandboxed command
// writes only in the folders it is given and in the sandbox's own
// temporary folder, reads everything but the paths it is denied, sees
// only its own processes, and reaches no network (spec/DECISIONS.md 352).
//
// The package builds the command line and checks that the sandbox can
// run; which commands run in it, and what they may then do without
// asking, is the caller's.
package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Policy is what a sandboxed command may do.
type Policy struct {
	// Write are the folders, or files, it may write in: the session's
	// working directories and those the settings add. A path that is not
	// there is left out: bubblewrap binds only what exists.
	Write []string
	// ReadOnly are paths inside those that stay read-only: the settings
	// files gnosh guards, and the settings' denyWrite.
	ReadOnly []string
	// DenyRead are paths it may not read, and AllowRead paths inside them
	// it may read after all.
	DenyRead, AllowRead []string
	// Temp is the sandbox's own temporary folder, the person's, kept from
	// one command to the next: /tmp inside the sandbox on Linux (the real
	// /tmp out of sight), and a folder it may write in on macOS, where
	// TMPDIR names it.
	Temp string
	// Expose are files of gnosh's own the command's shell must reach: the
	// one it writes its last working directory to (Write), and the scripts
	// it reads.
	Expose []Exposed
	// Network lets it reach the network. Without it there is none, but
	// for the loopback of its own (Linux) where nothing listens.
	Network bool
}

// Exposed is a file of gnosh's own a sandboxed command reaches.
type Exposed struct {
	Path  string
	Write bool
}

// ErrUnsupported is Wrap's error where this system has no sandbox gnosh
// can use.
var ErrUnsupported = errors.New("gnosh has no sandbox for this system: it sandboxes commands on Linux (bubblewrap) and macOS (Seatbelt)")

// Env is what the command is given beside its own environment: where its
// temporary files go, and its caches, which the person's own folders
// (read-only to it) cannot hold.
func Env(p Policy) []string {
	tmp := sandboxTmp(p)
	return []string{"TMPDIR=" + tmp, "XDG_CACHE_HOME=" + filepath.Join(tmp, ".cache")}
}

// PrivateTemp makes, or checks, the person's own temporary folder for
// sandboxed commands under dir (the system's temporary folder): one the
// person owns, which nobody else can open, and no link. It returns its
// path.
func PrivateTemp(dir string) (string, error) {
	p := filepath.Join(dir, "gnosh-sandbox-"+uidText())
	if err := os.Mkdir(p, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 || !ownedByMe(info) {
		return "", errors.New(p + " is not a folder of the person's own that nobody else can open, so it cannot be the sandbox's temporary folder")
	}
	return p, nil
}

// present is the paths of a list that are there, absolute, cleaned, each
// once, the shortest first.
func present(paths []string, there func(string) bool) []string {
	var out []string
	for _, p := range paths {
		if p == "" || !filepath.IsAbs(p) {
			continue
		}
		p = filepath.Clean(p)
		if there(p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	slices.SortStableFunc(out, func(a, b string) int { return len(a) - len(b) })
	return out
}

// kept is present's paths without one inside another of them: bwrap
// mounts a folder with what is under it.
func kept(paths []string, there func(string) bool) []string {
	var flat []string
	for _, p := range present(paths, there) {
		if !slices.ContainsFunc(flat, func(q string) bool { return under(p, q) }) {
			flat = append(flat, p)
		}
	}
	return flat
}

// under reports whether p is q or a path inside it.
func under(p, q string) bool {
	return p == q || q == "/" || strings.HasPrefix(p, q+"/")
}

// sandboxTmp is the temporary folder as the command sees it.
func sandboxTmp(p Policy) string {
	if runtime.GOOS == "linux" {
		return "/tmp"
	}
	return p.Temp
}

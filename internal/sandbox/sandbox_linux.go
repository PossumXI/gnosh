package sandbox

import (
	"errors"
	"os"
	"path/filepath"
)

// check tries bubblewrap once for the setup: first as the sandbox is
// made, its commands refused user namespaces of their own; where this
// bubblewrap cannot do that (before 0.8), without, which the person is
// told.
func check(s Setup) Status {
	bwrap := lookFor("bwrap")
	if s.Bwrap != "" {
		if !filepath.IsAbs(s.Bwrap) || !isProgram(s.Bwrap) {
			return Status{Tool: s.Bwrap, Why: "sandbox.bwrapPath names " + s.Bwrap + ", which is no program here"}
		}
		bwrap = s.Bwrap
	}
	if bwrap == "" {
		return Status{Why: "bubblewrap is not installed (apt-get install bubblewrap, or dnf install bubblewrap)"}
	}
	dir, err := os.Getwd()
	if err != nil {
		dir = "/"
	}
	trial := []string{"/bin/sh", "-c", ":"}
	p := Policy{HostProc: s.HostProc, blockUserns: !s.HostProc}
	if p.blockUserns && try(bwrapArgs(bwrap, p, dir, trial, statPath)) == "" {
		return withFilter(Status{OK: true, Tool: bwrap, UsernsRefused: true})
	}
	p.blockUserns = false
	if said := try(bwrapArgs(bwrap, p, dir, trial, statPath)); said != "" {
		why := "bubblewrap cannot make its sandbox here (" + said + "); on Ubuntu 24.04 and later, the AppArmor rule on unprivileged namespaces is the usual cause"
		if !s.HostProc {
			why += ", and in a container without its own /proc, sandbox.enableWeakerNestedSandbox"
		}
		return Status{Tool: bwrap, Why: why}
	}
	st := Status{OK: true, Tool: bwrap}
	if !s.HostProc {
		// (With the machine's /proc, bubblewrap has no /proc of the
		// sandbox's own to set the limit in: the setting's own cost.)
		st.Weaker = "this bubblewrap cannot keep sandboxed commands from making user namespaces of their own (that takes bubblewrap 0.8 or later)"
	}
	return withFilter(st)
}

// withFilter adds to the status what the sandbox cannot do here for want
// of a filter for the machine.
func withFilter(st Status) Status {
	if st.OK && !canFilter() {
		no := "nothing keeps sandboxed commands from opening Unix sockets on this machine (gnosh's filter is for amd64 and arm64)"
		if st.Weaker != "" {
			no = st.Weaker + ", and " + no
		}
		st.Weaker = no
	}
	return st
}

// Wrap returns the program and arguments that run argv, a program and its
// arguments, in the folder dir inside the sandbox the policy describes.
func Wrap(p Policy, dir string, argv []string) ([]string, error) {
	st := CheckWith(SetupOf(p))
	if !st.OK {
		return nil, errors.New(st.Why)
	}
	p.blockUserns = st.UsernsRefused && !p.UserNamespaces && !p.HostProc
	return bwrapArgs(st.Tool, p, dir, argv, statPath), nil
}

// statPath says whether a path is there, and whether it is a folder,
// following links as bubblewrap's mounts do.
func statPath(p string) (isDir, ok bool) {
	info, err := os.Stat(p)
	if err != nil {
		return false, false
	}
	return info.IsDir(), true
}

package sandbox

import (
	"errors"
	"os"
)

// check tries bubblewrap once.
func check() Status {
	bwrap := lookFor("bwrap")
	if bwrap == "" {
		return Status{Why: "bubblewrap is not installed (apt-get install bubblewrap, or dnf install bubblewrap)"}
	}
	dir, err := os.Getwd()
	if err != nil {
		dir = "/"
	}
	if said := try(bwrapArgs(bwrap, Policy{}, dir, []string{"/bin/sh", "-c", ":"}, statPath)); said != "" {
		return Status{Tool: bwrap, Why: "bubblewrap cannot make its sandbox here (" + said + "); on Ubuntu 24.04 and later, the AppArmor rule on unprivileged namespaces is the usual cause"}
	}
	return Status{OK: true, Tool: bwrap}
}

// Wrap returns the program and arguments that run argv, a program and its
// arguments, in the folder dir inside the sandbox the policy describes.
func Wrap(p Policy, dir string, argv []string) ([]string, error) {
	st := Check()
	if !st.OK {
		return nil, errors.New(st.Why)
	}
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

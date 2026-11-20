package sandbox

import (
	"errors"
)

// check tries Seatbelt once (the setup is bubblewrap's).
func check(Setup) Status {
	tool := lookFor("sandbox-exec")
	if tool == "" {
		return Status{Why: "this macOS has no sandbox-exec"}
	}
	if said := try([]string{tool, "-p", "(version 1)(allow default)", "/usr/bin/true"}); said != "" {
		return Status{Tool: tool, Why: "Seatbelt cannot make its sandbox here (" + said + ")"}
	}
	return Status{OK: true, Tool: tool}
}

// Wrap returns the program and arguments that run argv, a program and its
// arguments, in the folder dir inside the sandbox the policy describes.
// The paths are put in the system's own spelling first, links resolved
// (/private/tmp for /tmp), which is what Seatbelt matches.
func Wrap(p Policy, dir string, argv []string) ([]string, error) {
	st := Check()
	if !st.OK {
		return nil, errors.New(st.Why)
	}
	real := func(list []string) []string {
		out := make([]string, 0, len(list))
		for _, s := range list {
			if s != "" {
				out = append(out, realPath(s))
			}
		}
		return out
	}
	p.Write, p.ReadOnly, p.DenyRead, p.AllowRead = real(p.Write), real(p.ReadOnly), real(p.DenyRead), real(p.AllowRead)
	p.UnixSockets, p.Hidden = real(p.UnixSockets), real(p.Hidden)
	if t := real([]string{p.Temp}); len(t) == 1 {
		p.Temp = t[0]
	}
	for i := range p.Expose {
		if r := real([]string{p.Expose[i].Path}); len(r) == 1 {
			p.Expose[i].Path = r[0]
		}
	}
	return append([]string{st.Tool, "-p", seatbeltProfile(p)}, argv...), nil
}

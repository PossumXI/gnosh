package sandbox

import "path/filepath"

// realPath is the path in the system's own spelling, links resolved, as
// Seatbelt matches it; for a path not there yet (a socket made later, a
// file denied before it is written, in folders not made yet), the nearest
// folder above it that is there, resolved, with the rest as written: what
// it will be when it is there. (Only its folder had been resolved, so a
// denied path two folders down from one not there yet, under a link such
// as /tmp, matched neither spelling: the third sandbox review.)
func realPath(s string) string {
	s = filepath.Clean(s)
	rest := ""
	for dir := s; ; {
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(r, rest)
		}
		up := filepath.Dir(dir)
		if up == dir {
			return s
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = up
	}
}

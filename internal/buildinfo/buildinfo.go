// Package buildinfo says which build of gnosh this program is.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// BuildID names this build of gnosh. Two programs with the same BuildID were
// built from the same source: a gnosh process and the GnoVM worker it starts
// compare theirs, and refuse to work together when they differ.
//
// A build can be given a name of its choosing at link time:
//
//	go build -ldflags "-X github.com/gnosh-ai/gnosh/internal/buildinfo.BuildID=NAME" .
//
// When the linker leaves it empty, it is filled in, before main runs, from
// what the go command recorded in the binary: the commit it was built from,
// with "+dirty" when the tree had uncommitted changes; failing that, the
// version of the main module; failing that, "devel". Builds that the go
// command could not tell apart (two different builds of an uncommitted tree,
// or builds with no version control beside them) share a BuildID; name such
// builds at link time where it matters.
//
// It is a package variable, set once at start and not changed after, because
// the linker's -X can set nothing else.
var BuildID string

// BuildTime is the time of the commit a build was made from, in RFC 3339,
// when it is given at link time: the go command records it only for a
// build made inside a git checkout, and a build made from an archive of
// the tree (git archive) has none.
//
//	go build -ldflags "-X github.com/gnosh-ai/gnosh/internal/buildinfo.BuildTime=2026-11-15T09:30:00Z" .
var BuildTime string

func init() {
	BuildID = resolve(BuildID, debug.ReadBuildInfo)
}

// resolve is the BuildID for a program whose linker set it to linked: linked
// itself when it is not blank, or else what the build info says.
func resolve(linked string, read func() (*debug.BuildInfo, bool)) string {
	if id := strings.TrimSpace(linked); id != "" {
		return id
	}
	info, ok := read()
	if !ok {
		info = nil
	}
	return fromBuildInfo(info)
}

// revisionLength is how many characters of a commit's hash name a build:
// enough to tell commits apart, short enough to read in a message.
const revisionLength = 12

// fromBuildInfo is the name of the build that info describes; info is nil
// when the binary holds no build info.
func fromBuildInfo(info *debug.BuildInfo) string {
	const unknown = "devel"
	if info == nil {
		return unknown
	}
	var revision string
	var modified bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(s.Value)
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision != "" {
		if len(revision) > revisionLength {
			revision = revision[:revisionLength]
		}
		if modified {
			revision += "+dirty"
		}
		return revision
	}
	// No commit is recorded when the program was installed from a module
	// proxy; its module version then names the build. "(devel)" is what
	// the go command writes when it knows no version.
	if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
		return v
	}
	return unknown
}

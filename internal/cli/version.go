package cli

import (
	"runtime/debug"
	"time"
)

// Build says which build of gnosh is running. The program fills it in once,
// when it starts, from what was compiled into it, so that a session started
// before a newer build was installed still reports its own build and not
// the file now on disk.
type Build struct {
	// Time is the build's time: when its commit was made, or when it was
	// built. The zero time means it is not known.
	Time time.Time
	// Commit is the commit the build was made from, as much of its hash as
	// is to be shown. Empty means it is not known.
	Commit string
	// Modified says the source had changes that were not committed.
	Modified bool
}

// VersionLine returns what `gnosh --version` prints, without a newline: the
// build's time and its commit, and then the program's name in brackets, the
// way Claude Code prints "2.1.323 (Claude Code)":
//
//	2026-11-13T09:15:00Z bfdb0d9 (gnosh)
//
// The time is in UTC, to the second. A build made from source with
// uncommitted changes has "+changes" after its commit. A time or commit
// that is not known is written "unknown-time" or "unknown-commit", so the
// line always has the same three parts.
func VersionLine(b Build) string {
	when := "unknown-time"
	if !b.Time.IsZero() {
		when = b.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	commit := "unknown-commit"
	if b.Commit != "" {
		commit = b.Commit
	}
	if b.Modified {
		commit += "+changes"
	}
	return when + " " + commit + " (gnosh)"
}

// WithLinkedTime returns b with its time read from linked, an RFC 3339
// time given at link time, when b has none and linked can be read.
func (b Build) WithLinkedTime(linked string) Build {
	if !b.Time.IsZero() || linked == "" {
		return b
	}
	if t, err := time.Parse(time.RFC3339, linked); err == nil {
		b.Time = t
	}
	return b
}

// commitLength is how many characters of a commit's hash BuildFromInfo
// keeps.
const commitLength = 12

// BuildFromInfo reads a Build out of what the Go toolchain records in a
// program (runtime/debug.ReadBuildInfo): the commit, its time, and whether
// the source was modified. A nil info, or one without those records, gives
// a Build that says so.
func BuildFromInfo(info *debug.BuildInfo) Build {
	var b Build
	if info == nil {
		return b
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Commit = s.Value
			if len(b.Commit) > commitLength {
				b.Commit = b.Commit[:commitLength]
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				b.Time = t
			}
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}
	return b
}

package cli

import (
	"runtime/debug"
	"testing"
	"time"
)

func TestVersionLine(t *testing.T) {
	at := time.Date(2026, 11, 13, 1, 5, 22, 0, time.FixedZone("PST", -8*3600))
	for _, c := range []struct {
		b    Build
		want string
	}{
		{Build{Time: at, Commit: "bfdb0d9"}, "2026-11-13T09:05:22Z bfdb0d9 (gnosh)"},
		{Build{Time: at, Commit: "bfdb0d9", Modified: true}, "2026-11-13T09:05:22Z bfdb0d9+changes (gnosh)"},
		{Build{Commit: "bfdb0d9"}, "unknown-time bfdb0d9 (gnosh)"},
		{Build{Time: at}, "2026-11-13T09:05:22Z unknown-commit (gnosh)"},
		{Build{}, "unknown-time unknown-commit (gnosh)"},
		{Build{Modified: true}, "unknown-time unknown-commit+changes (gnosh)"},
	} {
		if got := VersionLine(c.b); got != c.want {
			t.Errorf("VersionLine(%+v) = %q, want %q", c.b, got, c.want)
		}
	}
}

func TestBuildFromInfo(t *testing.T) {
	if got := BuildFromInfo(nil); got != (Build{}) {
		t.Errorf("BuildFromInfo(nil) = %+v, want the empty Build", got)
	}
	info := &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "GOOS", Value: "linux"},
		{Key: "vcs", Value: "git"},
		{Key: "vcs.revision", Value: "bfdb0d9c0ffee1234567890abcdef1234567890a"},
		{Key: "vcs.time", Value: "2026-11-13T09:05:22Z"},
		{Key: "vcs.modified", Value: "true"},
	}}
	got := BuildFromInfo(info)
	want := Build{Time: time.Date(2026, 11, 13, 9, 5, 22, 0, time.UTC), Commit: "bfdb0d9c0ffe", Modified: true}
	if !got.Time.Equal(want.Time) || got.Commit != want.Commit || got.Modified != want.Modified {
		t.Errorf("BuildFromInfo = %+v, want %+v", got, want)
	}
	if line := VersionLine(got); line != "2026-11-13T09:05:22Z bfdb0d9c0ffe+changes (gnosh)" {
		t.Errorf("VersionLine = %q", line)
	}
	// Records that are missing or unreadable leave the Build saying so.
	got = BuildFromInfo(&debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: "abc"},
		{Key: "vcs.time", Value: "yesterday"},
		{Key: "vcs.modified", Value: "false"},
	}})
	if !got.Time.IsZero() || got.Commit != "abc" || got.Modified {
		t.Errorf("BuildFromInfo = %+v", got)
	}
}

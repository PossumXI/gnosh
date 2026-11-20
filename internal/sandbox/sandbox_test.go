package sandbox

import (
	"strings"
	"testing"
)

// fakeFS is a stat over a fixed set of paths: true for a folder.
func fakeFS(paths map[string]bool) func(string) (bool, bool) {
	return func(p string) (bool, bool) {
		isDir, ok := paths[p]
		return isDir, ok
	}
}

// The bubblewrap command line: the machine read-only, the sandbox's own
// /dev, /proc and /tmp, the folders it writes in, the read-only paths in
// them, the denied ones hidden, those opened again, and a denied path
// inside one opened again hidden again; no network unless given; only
// what is there, each folder once.
func TestBwrapArgs(t *testing.T) {
	fs := fakeFS(map[string]bool{"/w": true, "/w/sub": true, "/w/.gnosh": true, "/w/gnosh.json": false, "/t": true,
		"/home/u": true, "/home/u/proj": true, "/home/u/proj/.env": false, "/x/cwd": false, "/x/env.sh": false})
	p := Policy{
		Write:     []string{"/w", "/w/sub", "/w/", "/missing"},
		ReadOnly:  []string{"/w/.gnosh", "/w/gnosh.json", "/w/none"},
		DenyRead:  []string{"/home/u", "/home/u/proj/.env"},
		AllowRead: []string{"/home/u/proj", "/elsewhere"},
		Temp:      "/t",
		Expose:    []Exposed{{Path: "/x/cwd", Write: true}, {Path: "/x/env.sh"}, {Path: "/x/gone"}},
	}
	got := strings.Join(bwrapArgs("bwrap", p, "/w/sub/", []string{"/bin/bash", "-c", "x"}, fs), " ")
	want := "bwrap --die-with-parent --unshare-pid --unshare-ipc --unshare-net --ro-bind / / --dev /dev --proc /proc" +
		" --bind /t /tmp --bind /w /w" +
		" --ro-bind /w/.gnosh /w/.gnosh --ro-bind /w/gnosh.json /w/gnosh.json --bind /x/cwd /x/cwd --ro-bind /x/env.sh /x/env.sh" +
		" --tmpfs /home/u --ro-bind /home/u/proj /home/u/proj" +
		" --ro-bind /dev/null /home/u/proj/.env --remount-ro /home/u --chdir /w/sub -- /bin/bash -c x"
	if got != want {
		t.Errorf("bwrap args:\n got %s\nwant %s", got, want)
	}
	p.Network = true
	if got := strings.Join(bwrapArgs("bwrap", Policy{Network: true}, "/", []string{"true"}, fs), " "); strings.Contains(got, "--unshare-net") {
		t.Errorf("with the network: %s", got)
	}
}

// The Seatbelt profile: writes only in the folders named, the temporary
// one and the devices a shell needs; the read-only and denied paths after
// them, a denied path inside one opened again denied again; strings
// quoted.
func TestSeatbeltProfile(t *testing.T) {
	p := Policy{
		Write:     []string{"/Users/u/proj", `/Users/u/we"ird\dir`},
		ReadOnly:  []string{"/Users/u/proj/.gnosh"},
		DenyRead:  []string{"/Users/u", "/Users/u/proj/.env"},
		AllowRead: []string{"/Users/u/proj"},
		Temp:      "/private/tmp/gnosh-sandbox-501",
		Expose:    []Exposed{{Path: "/private/var/x/cwd", Write: true}, {Path: "/private/var/x/env.sh"}},
	}
	got := seatbeltProfile(p)
	for _, piece := range []string{
		"(version 1)\n(allow default)\n", "(deny network*)\n(deny file-write*)\n(allow file-write*\n",
		`(subpath "/Users/u/proj")`, `(subpath "/Users/u/we\"ird\\dir")`, `(subpath "/private/tmp/gnosh-sandbox-501")`,
		`(literal "/private/var/x/cwd")`, `(literal "/dev/null")`, `(regex #"^/dev/fd/[0-9]+$")`,
		"(deny appleevent-send)\n(deny job-creation)\n", `(global-name "com.apple.coreservices.launchservicesd")`,
		`(deny file-read* file-write* (literal "/dev/tty") (regex #"^/dev/ttys[0-9]+$"))`,
		"(deny file-write*\n  (subpath \"/Users/u/proj/.gnosh\"))",
		"(deny file-read*\n  (subpath \"/Users/u\")\n  (subpath \"/Users/u/proj/.env\"))\n(allow file-read*\n  (subpath \"/Users/u/proj\")\n",
		"(deny file-read*\n  (subpath \"/Users/u/proj/.env\"))\n",
	} {
		if !strings.Contains(got, piece) {
			t.Errorf("the profile has no %q:\n%s", piece, got)
		}
	}
	if strings.Contains(got, `(literal "/private/var/x/env.sh")`) {
		t.Errorf("a file only read was made writable:\n%s", got)
	}
	if strings.Index(got, `(literal "/private/var/x/cwd")`) < strings.Index(got, `(subpath "/Users/u/proj/.gnosh")`) {
		t.Errorf("gnosh's own file is made writable before the read-only paths, which would take it back:\n%s", got)
	}
	if strings.Contains(seatbeltProfile(Policy{Network: true}), "network") {
		t.Error("the network was denied though the policy gives it")
	}
}

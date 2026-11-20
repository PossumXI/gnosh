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
		"(version 1)\n(allow default)\n", "(deny network*)\n(deny file-read* file-write* (literal \"/dev/tty\") (regex #\"^/dev/ttys[0-9]+$\"))\n(deny file-write*)\n(allow file-write*\n",
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

// What the settings loosen, on bubblewrap's command line: user namespaces
// refused (where bubblewrap can, Wrap says) unless let; the machine's
// /proc for an unprivileged container; /run left in sight for every Unix
// socket; and with the files left open, the machine mounted writable,
// nothing of the files confined, the relay given the proxy's socket where
// it is.
func TestBwrapLoosenings(t *testing.T) {
	fs := fakeFS(map[string]bool{"/w": true, "/t": true, "/run": true, "/home/u": true, "/x/cwd": false, "/t/proxy.sock": false})
	argv := []string{"/bin/sh", "-c", "x"}
	args := func(p Policy) string { return strings.Join(bwrapArgs("bwrap", p, "/w", argv, fs), " ") }
	if got := args(Policy{blockUserns: true}); !strings.HasPrefix(got, "bwrap --die-with-parent --unshare-user --disable-userns --unshare-pid ") {
		t.Errorf("user namespaces refused: %s", got)
	}
	if got := args(Policy{}); strings.Contains(got, "userns") || !strings.Contains(got, "--proc /proc") || !strings.Contains(got, "--tmpfs /run") {
		t.Errorf("the default: %s", got)
	}
	if got := args(Policy{HostProc: true}); strings.Contains(got, "--proc") {
		t.Errorf("the machine's /proc: %s", got)
	}
	if got := args(Policy{AllUnixSockets: true}); strings.Contains(got, "/run") {
		t.Errorf("every Unix socket: %s", got)
	}
	open := Policy{FilesystemOff: true, Write: []string{"/w"}, DenyRead: []string{"/home/u"}, Temp: "/t",
		Expose: []Exposed{{Path: "/x/cwd", Write: true}}, Proxy: "/t/proxy.sock", Relay: "/gnosh"}
	want := "bwrap --die-with-parent --unshare-pid --unshare-ipc --unshare-net --bind / / --dev /dev --proc /proc --tmpfs /run" +
		" --ro-bind /t/proxy.sock /t/proxy.sock --chdir /w -- /gnosh " + RelayWord + " /t/proxy.sock /bin/sh -c x"
	if got := args(open); got != want {
		t.Errorf("the files left open:\n got %s\nwant %s", got, want)
	}
	if env := strings.Join(Env(open), " "); strings.Contains(env, "TMPDIR") || strings.Contains(env, "XDG_CACHE_HOME") {
		t.Errorf("the files left open, the environment: %s", env)
	}
}

// What the settings loosen, in the Seatbelt profile: the trust service
// closed unless let; Apple Events; Unix sockets by path or all; a port of
// its own and the machine's; more of macOS's services by name, prefix or
// all; and with the files left open, no rule of the files but the
// terminals'.
func TestSeatbeltLoosenings(t *testing.T) {
	base := seatbeltProfile(Policy{})
	for _, piece := range []string{`(global-name "com.apple.trustd.agent")`, "(deny appleevent-send)", "(deny network*)"} {
		if !strings.Contains(base, piece) {
			t.Errorf("the default profile has no %q:\n%s", piece, base)
		}
	}
	if strings.Contains(base, "unix-socket") || strings.Contains(base, "network-bind") || strings.Contains(base, "(allow mach-lookup") {
		t.Errorf("the default profile loosens:\n%s", base)
	}
	got := seatbeltProfile(Policy{TrustService: true, AppleEvents: true, UnixSockets: []string{"/Users/u/.ssh/agent.sock"}, LocalBinding: true,
		MachLookup: []string{"com.apple.CoreSimulator.*", "com.example.one"}})
	for _, piece := range []string{
		"(allow network-outbound\n  (remote unix-socket (path-literal \"/Users/u/.ssh/agent.sock\")))",
		`(allow network-bind (local ip "*:*"))`, `(allow network-inbound (local ip "*:*"))`, `(allow network-outbound (remote ip "localhost:*"))`,
		`(allow mach-lookup (global-name-prefix "com.apple.CoreSimulator.") (global-name "com.example.one"))`,
	} {
		if !strings.Contains(got, piece) {
			t.Errorf("the loosened profile has no %q:\n%s", piece, got)
		}
	}
	if strings.Contains(got, "trustd") || strings.Contains(got, "appleevent") {
		t.Errorf("the trust service or Apple Events still closed:\n%s", got)
	}
	if got := seatbeltProfile(Policy{AllUnixSockets: true, MachLookup: []string{"x.*", "*"}}); !strings.Contains(got, "(allow network-outbound (remote unix-socket))") || !strings.Contains(got, "(allow mach-lookup)\n") {
		t.Errorf("every socket and service:\n%s", got)
	}
	open := seatbeltProfile(Policy{FilesystemOff: true, Write: []string{"/Users/u/proj"}, DenyRead: []string{"/Users/u"}})
	if strings.Contains(open, "file-write*)") || strings.Contains(open, "/Users/u") || !strings.Contains(open, `(literal "/dev/tty")`) || !strings.Contains(open, "(deny network*)") {
		t.Errorf("the files left open:\n%s", open)
	}
}

// A credential the settings hide is hidden last: an allowRead of it, a
// folder written in that holds it, or the folder itself among those
// written in, brings nothing back (the sandbox review). The variables
// the settings hide go first, so that hiding one of gnosh's own cannot
// take the sandbox's.
func TestHiddenLast(t *testing.T) {
	fs := fakeFS(map[string]bool{"/home/u": true, "/home/u/.ssh": true, "/home/u/.ssh/id_ed25519": false, "/w": true, "/w/.env": false, "/t": true})
	p := Policy{Write: []string{"/w", "/home/u/.ssh"}, AllowRead: []string{"/home/u/.ssh/id_ed25519"}, DenyRead: []string{"/home/u"},
		Hidden: []string{"/home/u/.ssh", "/w/.env"}, Temp: "/t"}
	got := strings.Join(bwrapArgs("bwrap", p, "/w", []string{"true"}, fs), " ")
	last := strings.LastIndex(got, "--bind /home/u/.ssh /home/u/.ssh")
	hide := strings.LastIndex(got, "--tmpfs /home/u/.ssh")
	envHide := strings.LastIndex(got, "--ro-bind /dev/null /w/.env")
	if last < 0 || hide < last || envHide < strings.LastIndex(got, "--bind /w /w") || !strings.Contains(got, "--remount-ro /home/u/.ssh") {
		t.Errorf("the hidden paths are not hidden after what opens them:\n%s", got)
	}
	prof := seatbeltProfile(p)
	if i := strings.LastIndex(prof, "(deny file-read* file-write*\n  (subpath \"/home/u/.ssh\")\n  (subpath \"/w/.env\"))"); i < 0 || i < strings.LastIndex(prof, "(allow file-read*") {
		t.Errorf("the hidden paths are not denied last:\n%s", prof)
	}
	p.HideVars = []string{"HTTPS_PROXY", "GITHUB_TOKEN"}
	p.Proxy = "/t/proxy.sock"
	env := strings.Join(Env(p), " ")
	if !strings.HasPrefix(env, "HTTPS_PROXY GITHUB_TOKEN ") || !strings.Contains(env, "HTTPS_PROXY=http://") {
		t.Errorf("the variables: %s", env)
	}
}

// Seatbelt matches paths, so a folder renamed takes what is in it out
// from under the rules: nothing may be moved out of a path the command may
// not read (but out of what is opened again in one), and no folder that
// holds a path kept from it may be renamed or taken away, up to the root
// (the sandbox review).
func TestSeatbeltKeepsFoldersInPlace(t *testing.T) {
	p := Policy{
		Write:     []string{"/Users/u/proj"},
		ReadOnly:  []string{"/Users/u/proj/.gnosh"},
		DenyRead:  []string{"/Users/u", "/Users/u/proj/conf/secret.yml"},
		AllowRead: []string{"/Users/u/proj"},
	}
	got := seatbeltProfile(p)
	for _, piece := range []string{
		"(deny file-write-unlink\n  (subpath \"/Users/u\")\n  (subpath \"/Users/u/proj/conf/secret.yml\"))",
		"(allow file-write-unlink\n  (subpath \"/Users/u/proj\")",
		"(deny file-write-unlink\n  (subpath \"/Users/u/proj/conf/secret.yml\"))",
		"(deny file-write-unlink\n  (literal \"/Users\")\n  (literal \"/Users/u\")\n  (literal \"/Users/u/proj\")\n  (literal \"/Users/u/proj/conf\"))",
	} {
		if !strings.Contains(got, piece) {
			t.Errorf("the profile has no %q:\n%s", piece, got)
		}
	}
	if strings.Index(got, "(allow file-write-unlink") > strings.Index(got, "(deny file-write*\n  (subpath \"/Users/u/proj/.gnosh\"))") {
		t.Errorf("the unlinks opened again come after the read-only paths, and open them:\n%s", got)
	}
	// No folder cloned whole, which would take a denied file in it along;
	// and what is opened again only to be read is not moved or deleted
	// either (the third review).
	p.AllowRead = append(p.AllowRead, "/Users/u/notes")
	got = seatbeltProfile(p)
	if !strings.Contains(got, "(deny file-clone (vnode-type DIRECTORY))") {
		t.Errorf("folders may be cloned:\n%s", got)
	}
	if i := strings.Index(got, "(allow file-write-unlink"); i < 0 || strings.Contains(got[i:i+strings.Index(got[i:], ")\n")], "/Users/u/notes") {
		t.Errorf("a path opened again to be read may be moved or deleted:\n%s", got)
	}
	if strings.Contains(seatbeltProfile(Policy{Write: []string{"/w"}}), "file-clone") {
		t.Errorf("folders refused cloning with nothing denied or hidden")
	}
}

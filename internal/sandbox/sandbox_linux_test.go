package sandbox

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// A command under real bubblewrap: it writes in the folder it is given and
// in its own /tmp (kept for the next command, the real one out of sight),
// and nowhere else; the read-only path in its folder stays so; a denied
// file reads as nothing; it reaches no network, the machine's loopback
// neither; it sees only its own processes. Skipped where bubblewrap cannot
// run.
func TestBubblewrapForReal(t *testing.T) {
	if st := Check(); !st.OK {
		t.Skip(st.Why)
	}
	work, outside, home := t.TempDir(), t.TempDir(), t.TempDir()
	temp, err := PrivateTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, ".gnosh"), 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(home, "secret.txt")
	if err := os.WriteFile(secret, []byte("hunter2"), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	p := Policy{Write: []string{work}, ReadOnly: []string{filepath.Join(work, ".gnosh")}, DenyRead: []string{secret}, Temp: temp}
	run := func(script string) string {
		t.Helper()
		argv, err := Wrap(p, work, []string{"/bin/sh", "-c", script})
		if err != nil {
			t.Fatal(err)
		}
		out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	got := run(`echo in > in.txt && echo ok-in; echo out > ` + outside + `/out.txt 2>/dev/null || echo no-out;` +
		` echo g > .gnosh/x 2>/dev/null || echo no-gnosh; cat ` + secret + `; echo [;` +
		` echo t > /tmp/kept.txt && echo ok-tmp; ls /tmp | grep -c . ; kill -0 1 && echo pid1-ours`)
	for _, want := range []string{"ok-in", "no-out", "no-gnosh", "[", "ok-tmp", "pid1-ours"} {
		if !strings.Contains(got, want) {
			t.Errorf("the sandboxed command said %q, without %q", got, want)
		}
	}
	if strings.Contains(got, "hunter2") {
		t.Errorf("a denied file was read: %s", got)
	}
	if _, err := os.Stat(filepath.Join(outside, "out.txt")); err == nil {
		t.Error("a file was written outside the folders given")
	}
	if data, err := os.ReadFile(filepath.Join(temp, "kept.txt")); err != nil || string(data) != "t\n" {
		t.Errorf("the sandbox's /tmp is not the private folder: %q, %v", data, err)
	}
	if got := run(`cat /tmp/kept.txt`); got != "t" {
		t.Errorf("the next command's /tmp: %q", got)
	}
	// (bash, for its /dev/tcp.)
	if bash, err := exec.LookPath("bash"); err == nil {
		probe := "(exec 3<>/dev/tcp/127.0.0.1/" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) + ") 2>/dev/null && echo reached || echo unreached"
		argv, err := Wrap(p, work, []string{bash, "-c", probe})
		if err != nil {
			t.Fatal(err)
		}
		if out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput(); strings.TrimSpace(string(out)) != "unreached" {
			t.Errorf("the sandboxed command reached the machine's loopback: %s", out)
		}
		if out, _ := exec.Command(bash, "-c", probe).CombinedOutput(); strings.TrimSpace(string(out)) != "reached" {
			t.Errorf("the listener is not reachable even outside the sandbox: %s", out)
		}
	}
}

// TestMain runs the test binary as the relay when bubblewrap starts it so,
// as gnosh's own main does.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == RelayWord {
		os.Exit(RelayMain(os.Args[2:]))
	}
	os.Exit(m.Run())
}

// With a proxy, a sandboxed command reaches an allowed host through it
// (the relay, gnosh itself, carrying the sandbox's own loopback to the
// proxy's socket), and no other: the proxy says why. Skipped where
// bubblewrap or curl cannot run.
func TestSandboxReachesAllowedHostsThroughTheProxy(t *testing.T) {
	if st := Check(); !st.OK {
		t.Skip(st.Why)
	}
	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("no curl")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("reached")) }))
	defer srv.Close()
	temp, err := PrivateTemp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(temp, "proxy-test.sock")
	px, err := ListenProxy("unix", sock, Domains{Allowed: []string{"127.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer px.Close()
	work := t.TempDir()
	p := Policy{Write: []string{work}, Temp: temp, Proxy: sock, Relay: exe}
	fetch := func(url string) string {
		t.Helper()
		argv, err := Wrap(p, work, []string{curl, "-s", "-m", "10", url})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env = append(os.Environ(), Env(p)...)
		out, _ := cmd.CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	if got := fetch(srv.URL); got != "reached" {
		t.Errorf("an allowed host through the proxy: %q", got)
	}
	if got := fetch("http://elsewhere.example/"); !strings.Contains(got, "not in sandbox.network.allowedDomains") {
		t.Errorf("a host not allowed: %q", got)
	}
}

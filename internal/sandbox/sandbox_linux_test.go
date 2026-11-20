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
	temp, err := os.MkdirTemp(t.TempDir(), "sandbox-")
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
	// The machine's sockets under /run are out of sight.
	if _, err := os.Stat("/run"); err == nil {
		if got := run(`ls -A /run | wc -l`); got != "0" {
			t.Errorf("/run inside the sandbox holds %s entries", got)
		}
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

// The loosenings under real bubblewrap: a command may make no user
// namespace of its own (where this bubblewrap can refuse one), and may
// when let; with the machine's /proc it sees the machine's processes;
// with every Unix socket let, the machine's /run is in sight; with the
// files left open it writes anywhere the person may, and reaches the
// network no more. Skipped where bubblewrap cannot run.
func TestBubblewrapLoosenings(t *testing.T) {
	st := Check()
	if !st.OK {
		t.Skip(st.Why)
	}
	work, outside := t.TempDir(), t.TempDir()
	temp, err := os.MkdirTemp(t.TempDir(), "sandbox-")
	if err != nil {
		t.Fatal(err)
	}
	run := func(p Policy, script string) string {
		t.Helper()
		argv, err := Wrap(p, work, []string{"/bin/sh", "-c", script})
		if err != nil {
			t.Fatal(err)
		}
		out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	base := Policy{Write: []string{work}, Temp: temp}
	if unshare, err := exec.LookPath("unshare"); err == nil {
		probe := unshare + " -U true 2>/dev/null && echo made || echo refused"
		if st.UsernsRefused {
			if got := run(base, probe); got != "refused" {
				t.Errorf("a user namespace in the sandbox: %q", got)
			}
		} else {
			t.Logf("this bubblewrap cannot refuse user namespaces: %s", st.Weaker)
		}
		let := base
		let.UserNamespaces = true
		if got := run(let, probe); got != "made" {
			t.Errorf("a user namespace, let: %q", got)
		}
	}
	count := `ls /proc | grep -c '^[0-9]'`
	own, _ := strconv.Atoi(run(base, count))
	host := base
	host.HostProc = true
	machine, _ := strconv.Atoi(run(host, count))
	if own < 1 || own > 5 || machine <= own {
		t.Errorf("processes in sight: %d with its own /proc, %d with the machine's", own, machine)
	}
	if _, err := os.Stat("/run"); err == nil {
		all := base
		all.AllUnixSockets = true
		if got := run(all, `ls -A /run | wc -l`); got == "0" {
			t.Error("with every Unix socket let, /run is still out of sight")
		}
	}
	open := base
	open.FilesystemOff = true
	got := run(open, `echo out > `+outside+`/out.txt && echo wrote; ls /tmp >/dev/null && echo tmp`)
	if !strings.Contains(got, "wrote") {
		t.Errorf("the files left open, a write outside: %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(outside, "out.txt")); err != nil || string(data) != "out\n" {
		t.Errorf("the files left open, what was written: %q, %v", data, err)
	}
	if bash, err := exec.LookPath("bash"); err == nil {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		probe := "(exec 3<>/dev/tcp/127.0.0.1/" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) + ") 2>/dev/null && echo reached || echo unreached"
		argv, err := Wrap(open, work, []string{bash, "-c", probe})
		if err != nil {
			t.Fatal(err)
		}
		if out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput(); strings.TrimSpace(string(out)) != "unreached" {
			t.Errorf("the files left open, the network: %s", out)
		}
	}
}

// Under real bubblewrap a denied file in a folder it writes in stays
// denied when the file or its folder is renamed (a mount moves with its
// folder, and cannot itself be renamed), and a hidden credential stays
// hidden though allowRead names it and its folder is one it writes in
// (the sandbox review). Skipped where bubblewrap cannot run.
func TestBubblewrapHiddenStaysHidden(t *testing.T) {
	if st := Check(); !st.OK {
		t.Skip(st.Why)
	}
	work, home := t.TempDir(), t.TempDir()
	temp, err := os.MkdirTemp(t.TempDir(), "sandbox-")
	if err != nil {
		t.Fatal(err)
	}
	for path, text := range map[string]string{
		filepath.Join(work, "conf", "secret.yml"): "hunter2", filepath.Join(work, ".env"): "hunter3",
		filepath.Join(home, ".ssh", "id_ed25519"): "hunter4",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ssh := filepath.Join(home, ".ssh")
	p := Policy{Write: []string{work, ssh}, AllowRead: []string{filepath.Join(ssh, "id_ed25519")}, Temp: temp,
		DenyRead: []string{filepath.Join(work, "conf", "secret.yml")}, Hidden: []string{filepath.Join(work, ".env"), ssh}}
	argv, err := Wrap(p, work, []string{"/bin/sh", "-c", `cat conf/secret.yml; mv conf/secret.yml s 2>/dev/null; cat s;
		mv conf c2 && cat c2/secret.yml; mv c2 conf; cat .env; mv .env e2 2>/dev/null; cat e2; cat ` + ssh + `/id_ed25519; echo done`})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if strings.Contains(string(out), "hunter") || !strings.Contains(string(out), "done") {
		t.Errorf("a denied or hidden file was read: %s", out)
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
	temp, err := os.MkdirTemp(t.TempDir(), "sandbox-")
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

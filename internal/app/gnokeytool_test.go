package app

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gnosh-ai/gnosh/internal/app/apptest"
)

// gnokeyCall is one call of the gnokey tool, scripted.
func gnokeyCallOf(id, command string) apptest.Call {
	args, _ := json.Marshal(map[string]string{"command": command, "description": "test"})
	return apptest.Call{ID: id, Name: "gnokey", Args: string(args)}
}

// gnokeyWorld is a world with a stand-in gnokey first on the PATH, which
// says what it was run with and what it read on its standard input.
func gnokeyWorld(t *testing.T, replies ...apptest.Reply) *apptest.World {
	w := apptest.NewWorld(t, replies...)
	w.Write(w.Dir+"/gnomod.toml", "module = \"gno.land/r/demo/counter\"\n") // a Gno project
	w.Script(w.Root+"/gnobin/gnokey", `echo "args: $*"
case " $* " in *" -insecure-password-stdin "*) read pw; echo "read: $pw";; esac`)
	w.PathFirst = append(w.PathFirst, w.Root+"/gnobin")
	return w
}

// The gnokey tool runs a query as it is, and asks the person for the key's
// password before a command that signs: at a masked prompt that names the
// key and shows the command, the password then going to gnokey on its
// standard input by the flag gnosh puts before the key. What gnokey
// printed reaches the model without the password, and the session's file
// holds it nowhere.
func TestGnokeyToolSigns(t *testing.T) {
	const sign = "gnokey maketx call -pkgpath gno.land/r/demo/counter -func Increment -gas-fee 1000000ugnot -gas-wanted 2000000 -broadcast -chainid dev -remote 127.0.0.1:26657 testkey"
	w := gnokeyWorld(t,
		apptest.Reply{Calls: []apptest.Call{gnokeyCallOf("c1", "gnokey query vm/qrender -data 'gno.land/r/demo/counter:'")}},
		apptest.Reply{Calls: []apptest.Call{gnokeyCallOf("c2", sign)}},
		apptest.Reply{Text: "Incremented."})
	s := sit(t, w, "-y")
	s.see("Type a prompt and Enter.")
	s.typeLine("increment the counter")
	s.see("Password for the key testkey")
	s.see("gnokey is to sign with the key testkey:")
	s.see("gnokey maketx call -pkgpath gno.land/r/demo/counter")
	s.see("The password goes to gnokey alone")
	s.typeLine("hunter22")
	s.see("Incremented.")
	s.typeLine("/exit")
	s.exit()

	has(t, "the query", toolResult(t, w, 1, "c1"), "args: query vm/qrender -data gno.land/r/demo/counter:")
	got := toolResult(t, w, 2, "c2")
	has(t, "what the model read", got, "args: maketx call -pkgpath gno.land/r/demo/counter", "-broadcast -chainid dev -remote 127.0.0.1:26657 -insecure-password-stdin testkey", "read: [the password]")
	for what, text := range map[string]string{"what the model read": got, "the screen": s.out.String()} {
		if strings.Contains(text, "hunter22") {
			t.Errorf("%s holds the password:\n%s", what, text)
		}
	}
	for _, path := range w.Sessions() {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "hunter22") {
			t.Errorf("the session's file %s holds the password", filepath.Base(path))
		}
	}
}

// What the gnokey tool refuses: key management, which stays in the
// person's terminal; the password flag passed by the model; a shell's
// words; anything that is not gnokey. A run, with nobody to type a
// password, has no gnokey tool at all.
func TestGnokeyToolRefuses(t *testing.T) {
	w := gnokeyWorld(t,
		apptest.Reply{Calls: []apptest.Call{
			gnokeyCallOf("c1", "gnokey add mykey"),
			gnokeyCallOf("c2", "gnokey maketx send -broadcast -insecure-password-stdin k"),
			gnokeyCallOf("c3", "gnokey list | cat"),
			gnokeyCallOf("c4", "rm -rf /"),
			gnokeyCallOf("c5", "gnokey maketx send -to g1x -send 1ugnot -broadcast"),
			gnokeyCallOf("c6", "gnokey maketx send -to g1x -send 1ugnot -broadcast=false k"),
		}},
		apptest.Reply{Text: "Done."})
	s := sit(t, w, "-y")
	s.see("Type a prompt and Enter.")
	s.typeLine("try it all")
	s.see("Done.")
	s.typeLine("/exit")
	s.exit()
	for id, want := range map[string]string{
		"c1": "gnokey add makes, changes or hands out a key, which stays in the user's own terminal",
		"c2": "passes gnokey's -insecure-password-stdin itself, which only gnosh may do",
		"c3": "is a shell's to read, and this command is run with none",
		"c4": "the command must begin with gnokey",
		"c5": "its last word, the key's name, is missing",
	} {
		got := toolResult(t, w, 1, id)
		has(t, id, got, want)
		hasNot(t, id, got, "args:")
	}
	// An unsigned transaction is made with no password.
	has(t, "c6", toolResult(t, w, 1, "c6"), "args: maketx send -to g1x -send 1ugnot -broadcast=false k")
	if strings.Contains(s.out.String(), "Password for the key") {
		t.Error("a refused call asked for a password")
	}

	// A run has no gnokey tool.
	w2 := gnokeyWorld(t, apptest.Reply{Text: "ok"})
	run(t, w2, "-p", "hi").wantCode(t, 0)
	reqs := w2.Model.Requests()
	if len(reqs) == 0 || slices.Contains(toolNamesOf(reqs[0]), "gnokey") {
		t.Errorf("a run was given the gnokey tool")
	}
	// Nor has a session in a project with no Gno code, where gnokey is
	// installed,
	w4 := gnokeyWorld(t, apptest.Reply{Text: "ok"})
	os.Remove(w4.Dir + "/gnomod.toml")
	s4 := sit(t, w4)
	s4.see("Type a prompt and Enter.")
	s4.typeLine("hi")
	s4.see("ok")
	s4.typeLine("/exit")
	s4.exit()
	if reqs := w4.Model.Requests(); len(reqs) == 0 || slices.Contains(toolNamesOf(reqs[0]), "gnokey") {
		t.Errorf("a session in a project with no Gno code was given the gnokey tool")
	}
	// nor one where gnokey is not installed (on a PATH that leaves out
	// whatever gnokey this machine has).
	w3 := apptest.NewWorld(t, apptest.Reply{Text: "ok"})
	w3.Write(w3.Dir+"/gnomod.toml", "module = \"gno.land/r/x\"\n")
	empty := filepath.Join(w3.Root, "emptybin")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	s3 := sitWith(t, w3, func(o *Options) {
		for i, kv := range o.Environ {
			if strings.HasPrefix(kv, "PATH=") {
				o.Environ[i] = "PATH=" + empty
			}
		}
	})
	s3.see("Type a prompt and Enter.")
	s3.typeLine("hi")
	s3.see("ok")
	s3.typeLine("/exit")
	s3.exit()
	if reqs := w3.Model.Requests(); len(reqs) == 0 || slices.Contains(toolNamesOf(reqs[0]), "gnokey") {
		t.Errorf("a session without gnokey installed was given the gnokey tool")
	}
}

// With the real gnokey, where this machine has it: a transaction made
// offline is signed with the password typed at the prompt, and a wrong
// password is reported as gnokey reports it, nothing signed.
func TestGnokeyToolWithTheRealGnokey(t *testing.T) {
	if _, err := exec.LookPath("gnokey"); err != nil {
		t.Skip("gnokey is not installed here")
	}
	home := t.TempDir()
	add := exec.Command("gnokey", "add", "e2ekey", "-home", home, "-insecure-password-stdin")
	add.Stdin = strings.NewReader("pw12345678\npw12345678\n")
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("gnokey add: %v\n%s", err, out)
	}
	list, _ := exec.Command("gnokey", "list", "-home", home).Output()
	addr := regexp.MustCompile(`g1[a-z0-9]+`).FindString(string(list))
	unsigned, err := exec.Command("gnokey", "maketx", "send", "-send", "1ugnot", "-to", addr, "-gas-fee", "1000ugnot", "-gas-wanted", "100000", "-broadcast=false", "-home", home, "e2ekey").Output()
	if err != nil || addr == "" {
		t.Fatalf("gnokey maketx: %v (%s)", err, addr)
	}
	tx := filepath.Join(home, "tx.json")
	if err := os.WriteFile(tx, unsigned, 0o600); err != nil {
		t.Fatal(err)
	}
	sign := "gnokey sign -tx-path " + tx + " -chainid dev -account-number 0 -account-sequence 0 -home " + home + " e2ekey"
	w := apptest.NewWorld(t,
		apptest.Reply{Calls: []apptest.Call{gnokeyCallOf("c1", sign)}},
		apptest.Reply{Calls: []apptest.Call{gnokeyCallOf("c2", sign)}},
		apptest.Reply{Text: "Signed."})
	w.Write(w.Dir+"/gnomod.toml", "module = \"gno.land/r/demo/counter\"\n")
	s := sit(t, w, "-y")
	s.see("Type a prompt and Enter.")
	s.typeLine("sign it")
	s.see("Password for the key e2ekey")
	s.typeLine("wrong-password")
	s.see("Password for the key e2ekey")
	s.typeLine("pw12345678")
	s.see("Signed.")
	s.typeLine("/exit")
	s.exit()
	has(t, "a wrong password", toolResult(t, w, 1, "c1"), "invalid account password")
	has(t, "the password", toolResult(t, w, 2, "c2"), "Tx successfully signed")
	if data, _ := os.ReadFile(tx); !strings.Contains(string(data), `"signature":`) {
		t.Errorf("the transaction was not signed:\n%s", data)
	}
}

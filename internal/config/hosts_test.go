package config

import (
	"strings"
	"testing"
)

// The check of declared hosts reads plain values: no file format is in
// these tests but for the place a declaration says it came from. Each
// mistake is found, names the host, and stands at the declaration's own
// place.
func TestCheckHosts(t *testing.T) {
	at := func(line int) Origin {
		return Origin{Layer: LayerUser, File: "/home/dev/.config/gnosh/settings", Line: line}
	}
	good := []Host{
		{Name: "render", SSH: "root@203.0.113.10", Description: "the render box", Origin: at(1)},
		{Name: "web", SSH: "deploy@203.0.113.11", Origin: at(2)},
		{Name: "alias", SSH: "my-alias", Origin: at(3)},
		{Name: "six", SSH: "root@2001:db8::7", Origin: at(4)},
		{Name: "name.with-all_3", SSH: "a_b-c.d@box.example.com", Origin: at(5)},
		{Name: "localhost", SSH: "dev@localhost", Origin: at(6)},
	}
	if problems := checkHosts(good); len(problems) != 0 {
		t.Fatalf("declarations with nothing wrong were refused:\n%s", showProblems(problems))
	}
	if problems := checkHosts(nil); len(problems) != 0 {
		t.Errorf("no hosts at all gave problems: %v", problems)
	}

	tests := []struct {
		name   string
		host   Host
		pieces []string
	}{
		{"a name declared twice", Host{Name: "render", SSH: "root@203.0.113.99"},
			[]string{"the host render is declared twice", `here as "root@203.0.113.99"`, `at /home/dev/.config/gnosh/settings:1 as "root@203.0.113.10"`, "A name stands for one machine", "take one of the two declarations out"}},
		{"the same declaration twice", Host{Name: "web", SSH: "deploy@203.0.113.11"}, []string{"the host web is declared twice"}},
		{"the local machine's name", Host{Name: "local", SSH: "a@203.0.113.9"},
			[]string{"a host cannot be named local", "local is the name of this machine", "as the host of a call that means here", "Give the host another name"}},
		{"the local machine's name in another case", Host{Name: "LOCAL", SSH: "a@203.0.113.9"}, []string{"a host cannot be named LOCAL", "local is the name of this machine"}},
		{"no name", Host{SSH: "a@203.0.113.9"}, []string{"a host is declared without a name"}},
		{"a name with a blank", Host{Name: "my host", SSH: "a@203.0.113.9"}, []string{"is not a usable name for a host", "letters, digits, dots, dashes and underscores"}},
		{"a name that starts with a dash", Host{Name: "-x", SSH: "a@203.0.113.9"}, []string{`"-x"`, "is not a usable name for a host"}},
		{"no ssh destination", Host{Name: "h"}, []string{"the host h has no ssh destination", "user@address, or a Host alias from your ssh configuration"}},
		{"a destination that starts with a dash", Host{Name: "h", SSH: "-oProxyCommand=x"}, []string{"the ssh destination of the host h starts with a dash", "ssh would read it as an option"}},
		{"a destination with shell syntax", Host{Name: "h", SSH: "a@b; rm -rf ~"}, []string{"the ssh destination of the host h holds blanks, or characters a shell would act on"}},
		{"a destination with options after it", Host{Name: "h", SSH: "a@b -p 22"}, []string{"holds blanks, or characters a shell would act on", "keys, ports and jump hosts belong in ~/.ssh/config"}},
		{"a destination written as an address with a path", Host{Name: "h", SSH: "ssh://a@203.0.113.9/"}, []string{"characters a shell would act on"}},
		{"a destination with a line break", Host{Name: "h", SSH: "a@b\nc"}, []string{"holds blanks, or characters a shell would act on"}},
		{"no user before the @", Host{Name: "h", SSH: "@203.0.113.9"}, []string{`the ssh destination of the host h, "@203.0.113.9", is not a user and an address with one @ between them`}},
		{"no address after the @", Host{Name: "h", SSH: "root@"}, []string{`"root@"`, "is not a user and an address with one @ between them"}},
		{"two @", Host{Name: "h", SSH: "a@b@203.0.113.9"}, []string{"is not a user and an address with one @ between them"}},
		{"a port after the address", Host{Name: "h", SSH: "root@203.0.113.9:2222"},
			[]string{`the ssh destination of the host h, "root@203.0.113.9:2222", ends in a port number`, "a port belongs in ~/.ssh/config, under a Host alias"}},
		{"a port after an alias", Host{Name: "h", SSH: "render:22"}, []string{"ends in a port number"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.host.Origin = at(7)
			problems := checkHosts(append(append([]Host{}, good...), tt.host))
			if len(problems) != 1 {
				t.Fatalf("want the one mistake, got %d:\n%s", len(problems), showProblems(problems))
			}
			p := problems[0]
			if p.Severity != Error || p.File != "/home/dev/.config/gnosh/settings" || p.Line != 7 {
				t.Errorf("the mistake is not an error at the declaration's own place: %+v", p)
			}
			for _, piece := range tt.pieces {
				if !strings.Contains(p.Message, piece) {
					t.Errorf("the message does not say %q:\n%s", piece, p.Message)
				}
			}
		})
	}

	// A declaration that came from somewhere that is not a file says
	// where, and every bad declaration is reported, not only the first.
	problems := checkHosts([]Host{
		{Name: "local", SSH: "a@b", Origin: Origin{Layer: LayerFlag, Name: "--settings"}},
		{Name: "h", Origin: at(2)},
		{Name: "h", SSH: "a@b", Origin: at(3)},
		{Name: "h", SSH: "c@d", Origin: at(4)},
	})
	if len(problems) != 3 || problems[0].Where != "--settings" || problems[0].File != "" || problems[1].Line != 2 || problems[2].Line != 4 ||
		!strings.Contains(problems[2].Message, "at /home/dev/.config/gnosh/settings:3 as") {
		t.Errorf("problems:\n%s", showProblems(problems))
	}
}

// The check is part of every load: a mistake in a host stops startup with
// the file and line of the declaration, whichever file made it, and a
// project's declaration is weighed against the person's only once the
// folder is trusted.
func TestHostMistakesStopStartup(t *testing.T) {
	_, problems := load(t, withRC(`
host add render --ssh root@203.0.113.10
host add web --ssh deploy@203.0.113.11
host add render --ssh root@203.0.113.99 --description "another machine"
host add far --ssh root@203.0.113.12:2222
`))
	wantProblem(t, problems, Error, userRC, 4, "the host render is declared twice", `here as "root@203.0.113.99"`, "at "+userRC+`:2 as "root@203.0.113.10"`)
	wantProblem(t, problems, Error, userRC, 5, "the ssh destination of the host far", "ends in a port number")
	if n := len(Errors(problems)); n != 2 {
		t.Errorf("want the two mistakes, got %d:\n%s", n, showProblems(problems))
	}

	// A project cannot quietly point one of the person's names at another
	// machine: in a trusted folder its declaration of the name is a
	// mistake, with both places named.
	in := inputs(newFS().
		put(userRC, "host add render --ssh root@203.0.113.10\n").
		put(projRC, "host add render --ssh root@203.0.113.66\n"))
	cfg, problems := load(t, in)
	if cfg != nil {
		t.Fatalf("a project's host of the same name as the person's loaded: %+v", cfg.Hosts)
	}
	wantProblem(t, problems, Error, projRC, 1, "the host render is declared twice", `here as "root@203.0.113.66"`, "at "+userRC+`:1 as "root@203.0.113.10"`)

	// While the folder is not trusted its declaration waits, and the
	// person's host is the one in force.
	in.Trusted = false
	cfg = mustLoad(t, in)
	if len(cfg.Hosts) != 1 || cfg.Hosts[0].SSH != "root@203.0.113.10" || len(cfg.Held) != 1 || cfg.Held[0].Kind != KindHost {
		t.Errorf("in an untrusted folder: hosts %+v, held %+v", cfg.Hosts, cfg.Held)
	}

	// A file that would not load is never written by gnosh itself.
	if _, err := CheckWrite(inputs(newFS().put(userRC, "host add render --ssh root@203.0.113.10\n")), userRC,
		[]byte("host add render --ssh root@203.0.113.10\nhost add render --ssh root@203.0.113.11\n")); err == nil {
		t.Errorf("a config file that declares a host twice passed the check before a write")
	}

	// The listing of the configuration shows each host with what was
	// written about it and where it came from.
	cfg = mustLoad(t, withRC("host add render --ssh root@203.0.113.10 --description \"the render box\"\n"))
	if text := cfg.String(); !strings.Contains(text, "render → root@203.0.113.10: the render box  ["+userRC+":1]") {
		t.Errorf("the listing does not show the host:\n%s", text)
	}
}

// A declaration that passes the check can be handed to ssh: whatever the
// name and the destination are, either a mistake is reported, at the
// declaration's own place, or the destination is one word of plain
// characters that does not begin with a dash.
func FuzzCheckHosts(f *testing.F) {
	for _, seed := range [][2]string{
		{"render", "root@203.0.113.10"}, {"h", "alias"}, {"local", "a@b"}, {"h", ""}, {"h", "-oProxyCommand=x"}, {"h", "a@b c"},
		{"h", "a@b\n-oProxyCommand=x"}, {"h", "a@"}, {"h", "@b"}, {"h", "a@b@c"}, {"h", "a:22"}, {"h", "root@2001:db8::7"}, {"", "a@b"},
		{"my host", "a@b"}, {"h", "a@b;id"}, {"h", "$(id)@b"}, {"h", "a@b\x00"}, {"h", "\xff@b"}, {"LoCaL", "x"},
	} {
		f.Add(seed[0], seed[1])
	}
	f.Fuzz(func(t *testing.T, name, ssh string) {
		at := Origin{Layer: LayerUser, File: "/settings", Line: 3}
		problems := checkHosts([]Host{{Name: name, SSH: ssh, Origin: at}})
		for _, p := range problems {
			if p.Severity != Error || p.File != "/settings" || p.Line != 3 || strings.TrimSpace(p.Message) == "" || strings.Contains(p.Message, "\n") {
				t.Fatalf("a mistake that is not one plain line at the declaration's place: %+v", p)
			}
		}
		if len(problems) > 0 {
			return
		}
		if ssh == "" || ssh[0] == '-' || strings.EqualFold(name, "local") || name == "" {
			t.Fatalf("the host %q with the destination %q passed the check", name, ssh)
		}
		for i := 0; i < len(ssh); i++ {
			c := ssh[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("_.[]:%@-", c) >= 0) {
				t.Fatalf("the destination %q passed the check with the byte %q in it", ssh, c)
			}
		}
	})
}

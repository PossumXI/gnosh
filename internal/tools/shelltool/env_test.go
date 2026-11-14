package shelltool

import (
	"slices"
	"strings"
	"testing"
)

// envOf returns the value a variable has in an environment, and whether
// it is there.
func envOf(env []string, name string) (string, bool) {
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, name+"="); ok {
			return v, true
		}
	}
	return "", false
}

// TestEnvNotHandedOn: what changes how a shell reads a command line is not
// inherited, whatever value it has.
func TestEnvNotHandedOn(t *testing.T) {
	t.Parallel()
	parent := []string{"PATH=/usr/bin:/bin", "HOME=/home/u"}
	for name := range notHandedOn {
		parent = append(parent, name+"=/tmp/evil")
	}
	parent = append(parent,
		"BASH_FUNC_ls%%=() { rm -rf \"$@\"; }",
		"BASH_FUNC_cd()=() { rm -rf \"$@\"; }",
		"BASH_FUNC_=() { :; }",
		"ls=() { rm -rf \"$@\"; }",
		"X=() { :;}; rm -rf /",
	)
	got := commandEnv(parent, "/work", nil)
	for _, kv := range got {
		name, value, _ := strings.Cut(kv, "=")
		if _, drop := notHandedOn[name]; drop || strings.HasPrefix(name, "BASH_FUNC_") || strings.HasPrefix(value, "() {") {
			t.Errorf("a command was handed %q", kv)
		}
	}
	for _, want := range []string{"PATH=/usr/bin:/bin", "HOME=/home/u", "PWD=/work", "PAGER=cat", "TERM=dumb", "GNOSH=1"} {
		if !slices.Contains(got, want) {
			t.Errorf("the environment lacks %s: %q", want, got)
		}
	}
	// The names the brief for this file lists, each by itself, so that
	// taking one out of the table is seen here.
	for _, name := range []string{"BASH_ENV", "ENV", "BASHOPTS", "SHELLOPTS", "GLOBIGNORE", "CDPATH", "POSIXLY_CORRECT", "IFS", "PS4",
		"PS0", "PS1", "PS2", "PROMPT_COMMAND", "BASH_XTRACEFD", "BASH_COMPAT"} {
		env := commandEnv([]string{"A=1", name + "=x", "Z=1"}, "/work", nil)
		if _, has := envOf(env, name); has {
			t.Errorf("%s was inherited", name)
		}
		if _, has := envOf(env, "A"); !has {
			t.Errorf("dropping %s took A with it: %q", name, env)
		}
		if why := notHandedOn[name]; len(why) < 20 {
			t.Errorf("%s is dropped and the table does not say why", name)
		}
	}
}

// TestEnvHandedOn: what a program needs, and what only changes what a
// program does once it runs, is inherited as it is.
func TestEnvHandedOn(t *testing.T) {
	t.Parallel()
	parent := []string{
		"PATH=/opt/bin:/usr/bin", "HOME=/home/u", "USER=u", "LANG=en_US.UTF-8", "LC_ALL=C", "TMPDIR=/tmp/u", "SHELL=/bin/zsh",
		"LD_LIBRARY_PATH=/opt/lib", "LD_PRELOAD=/opt/lib/x.so", "DYLD_LIBRARY_PATH=/opt/lib", "GIT_SSH_COMMAND=ssh -i key",
		"GIT_DIR=/repo/.git", "GOFLAGS=-mod=mod", "CC=clang", "EDITOR=vi", "ZDOTDIR=/home/u/.config/zsh", "TMOUT=5", "HISTFILE=/home/u/.h",
		"SSH_AUTH_SOCK=/tmp/agent", "DISPLAY=:0", "XDG_CONFIG_HOME=/home/u/.config", "NODE_OPTIONS=--max-old-space-size=4096",
		// Names that only look like the ones that are dropped.
		"bash_env=x", "Env=x", "ENVIRONMENT=prod", "MY_BASH_ENV=x", "PS3=choose: ", "PS5=x", "IFS_BACKUP=x", "CDPATHS=x", "BASH_FUNC=x",
		"XBASH_FUNC_ls%%=y", "NOTE=a () { b; }", "PAREN=( ) { x }",
	}
	got := commandEnv(parent, "/work", nil)
	for _, kv := range parent {
		if !slices.Contains(got, kv) {
			t.Errorf("%s was not handed on", kv)
		}
	}
}

// TestEnvTheLastOneCounts: a name that stands twice in the parent's
// environment is the last of the two, as it is to a program; a dropped one
// after a kept one of the same name leaves neither.
func TestEnvTheLastOneCounts(t *testing.T) {
	t.Parallel()
	got := commandEnv([]string{"F=plain", "F=() { evil; }", "G=() { evil; }", "G=plain", "PATH=/bin"}, "/work", nil)
	if v, has := envOf(got, "F"); has {
		t.Errorf("F is %q: the function that came last was dropped, and the value before it must not come back", v)
	}
	if v, _ := envOf(got, "G"); v != "plain" {
		t.Errorf("G is %q, want the plain value that came last", v)
	}
}

// TestEnvSettingsStillApply: the settings' env block is the person's word.
// It is laid over the result after the drop, the dropped names included,
// and it can still take a variable away.
func TestEnvSettingsStillApply(t *testing.T) {
	t.Parallel()
	parent := []string{"PATH=/usr/bin", "POSIXLY_CORRECT=inherited", "CDPATH=/inherited", "BASH_ENV=/inherited/rc", "KEEP=1", "GONE=1"}
	extra := []string{"POSIXLY_CORRECT=1", "CDPATH=/srv", "BASH_ENV=/home/u/agent.rc", "IFS=,", "PS4=+ ", "BASH_FUNC_hi%%=() { echo hi; }", "GONE", "TERM=xterm"}
	got := commandEnv(parent, "/work", extra)
	for name, want := range map[string]string{
		"POSIXLY_CORRECT": "1", "CDPATH": "/srv", "BASH_ENV": "/home/u/agent.rc", "IFS": ",", "PS4": "+ ",
		"BASH_FUNC_hi%%": "() { echo hi; }", "KEEP": "1", "TERM": "xterm", "PATH": "/usr/bin",
	} {
		if v, has := envOf(got, name); !has || v != want {
			t.Errorf("%s is %q (there: %v), want %q", name, v, has, want)
		}
	}
	if _, has := envOf(got, "GONE"); has {
		t.Error("a name with no = in the settings' env block did not take the variable away")
	}
	if !slices.IsSorted(got) {
		t.Errorf("the environment is not sorted: %q", got)
	}
	// And the exported way in is the same builder.
	if again := CommandEnv(parent, "/work", extra); !slices.Equal(again, got) {
		t.Errorf("CommandEnv and the manager's own builder differ:\n%q\n%q", again, got)
	}
}

// TestEnvOfAnEmptyParent: with nothing inherited the result is what is
// fixed, and nothing else.
func TestEnvOfAnEmptyParent(t *testing.T) {
	t.Parallel()
	got := commandEnv(nil, "/work", nil)
	want := append(fixedEnv(), "PWD=/work")
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

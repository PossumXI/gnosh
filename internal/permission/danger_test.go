package permission

import (
	"testing"
)

func TestDangerousRemovals(t *testing.T) {
	// Every Bash command is allowed, and there are no deny rules, so the
	// only thing that can stop these is the removal check itself.
	o := testOptions(ModeDefault, rules{allow: []string{"Bash"}})
	o.Layers = append(o.Layers, Layer{Scope: ScopeCLI, Name: "--add-dir", Dirs: []string{"/data/extra"}})
	e := mustEngine(t, o)

	dangerous := []string{
		// The root and its direct children.
		"rm -rf /", "rm -rf /*", "rm -rf /usr", "rm -rf /etc/", "rm -r /var", "rmdir /opt", "rm -rf //",
		"rm -rf /usr/../etc", "rm -rf --no-preserve-root /", "rm -rf -- /", "rm -fr /home",
		// The home directory.
		"rm -rf ~", "rm -rf ~/", "rm -rf ~/*", "rm -rf $HOME", "rm -rf \"$HOME\"", "rm -rf ${HOME}", "rm -rf $HOME/*",
		"rm -rf /home/u", "rm -rf /home/u/",
		// The working directory and what is above it.
		"rm -rf .", "rm -rf ./", "rm -rf ..", "rm -rf /proj", "rm -rf *", "rm -rf ./*", "rm -rf .*", "rm -rf /proj/*",
		"rm -rf $PWD", "rm -rf \"$PWD\"/*", "cd src && rm -rf ..", "rm -rf src/../..",
		// An additional directory or its parents, swept by a pattern.
		"rm -rf /data/extra/*", "rm -rf /data/*",
		// A pattern or trailing slash right under a variable.
		"rm -rf \"$DIR\"/*", "rm -rf $DIR/*", "rm -rf ${DIR}/*", "rm -rf \"$DIR\"/", "rm -rf $DIR/", "rm -rf $DIR/*/",
		"rm -rf \"${DIR:-}\"/*", "rm -rf $A$B/*", "rm -rf \"$DIR/sub\"/*",
		// A variable followed by one common top-level name.
		"rm -rf \"$TMPDIR/mnt\"", "rm -rf $ROOT/usr", "rm -rf $X/etc/",
		// A variable set in the same line from a command's output.
		"D=$(pwd); rm -rf \"$D\"", "D=`pwd` && rm -rf $D", "D=$(dirname x); rm -r $D",
		// A recursive removal of whatever a command prints.
		"rm -rf \"$(pwd)\"", "rm -rf $(pwd)", "rm -rf `pwd`", "rm -rf \"$(git rev-parse --show-toplevel)\"",
		"rm -rf $(pwd)/*", "rm -rf \"$(pwd)\"/*/",
		// The check looks inside.
		"(rm -rf /)", "{ rm -rf ~; }", "echo $(rm -rf /)", "echo `rm -rf ~`", "cat <(rm -rf /)",
		"sh -c 'rm -rf /'", "bash -c \"rm -rf ~\"", "sudo rm -rf /", "env rm -rf /", "timeout 5 rm -rf ~",
		"xargs rm -rf /", "if true; then rm -rf /; fi", "for d in a; do rm -rf ~; done", "eval 'rm -rf /'",
		"ls && rm -rf / || true", "find . -exec rm -rf / \\;", "/bin/rm -rf /", "nohup rm -rf ~ &",
		// A removal in a line the reader cannot follow is not ruled out.
		"rm -rf $x &&", "select f in a b; do rm -rf $f; done", "rm -rf ${x:-\"}\"}", "echo 'x; rm -rf y",
	}
	for _, cmd := range dangerous {
		d := bash(e, cmd)
		if d.Behavior != Ask || !d.Critical || d.Stage != StageSafety || len(d.Suggestions) != 0 {
			t.Errorf("%q: got %s critical=%v at %s (%s), want a critical ask", cmd, d.Behavior, d.Critical, d.Stage, d.Reason)
		}
		if h := d.WithHook(HookAllow, ""); h.Behavior != Ask {
			t.Errorf("%q: a hook approved a dangerous removal", cmd)
		}
	}

	safe := []string{
		"rm a.txt", "rm -rf build", "rm -rf ./build", "rm -rf /proj/build", "rm -rf src/gen", "rm *.tmp", "rm -rf build/*",
		"rm -rf /usr/local/x", "rm -rf /tmp/x", "rm -rf ~/x", "rm -rf $HOME/x", "rm -rf ~/.cache/thing",
		"rm -rf /data/extra/sub", "rm -rf /data/extra/sub/*", "rmdir empty",
		"rm -rf \"${DIR:?}\"/*", "rm -rf \"${DIR:?must be set}/\"", "rm -rf $DIR", "rm -rf \"$DIR/build\"", "rm -rf $BUILD/out",
		"rm \"$(pwd)/a.txt\"", "rm $(ls *.tmp)", "D=x; rm -rf \"$D\"", "echo rm -rf /", "echo 'rm -rf /'", "grep -r 'rm -rf /' .",
		"ls /", "rm -- -rf", "git rm -r --cached .",
		// Forms the reader follows are judged on what they remove.
		"[[ -d build ]] && rm -rf build", "rm -rf \"${dirs[@]}\"", "for ((i=0;i<3;i++)); do rm -f tmp$i; done",
		"(( n > 0 )) && rm -rf build",
	}
	for _, cmd := range safe {
		if d := bash(e, cmd); d.Critical {
			t.Errorf("%q: flagged as a dangerous removal (%s)", cmd, d.Reason)
		}
	}

	// The per-mode table of section 10.
	for mode, want := range map[Mode]Behavior{
		ModeDefault: Ask, ModeAcceptEdits: Ask, ModePlan: Ask, ModeDontAsk: Deny, ModeBypass: Ask,
	} {
		if err := e.SetMode(mode); err != nil {
			t.Fatal(err)
		}
		d := bash(e, "rm -rf /")
		if d.Behavior != want || !d.Critical {
			t.Errorf("rm -rf / in %s: got %s critical=%v, want %s", mode, d.Behavior, d.Critical, want)
		}
	}
	// A deny rule still comes first.
	e = engine(t, ModeBypass, rules{deny: []string{"Bash(rm *)"}})
	if d := bash(e, "rm -rf /"); d.Behavior != Deny || d.Stage != StageDeny {
		t.Errorf("deny before the removal check: %+v", d)
	}
	// The shell's current directory counts as well as the primary one.
	e = engine(t, ModeBypass, rules{})
	e.SetCwd("/proj/src/deep")
	for _, cmd := range []string{"rm -rf .", "rm -rf ..", "rm -rf ../..", "rm -rf /proj/src"} {
		if d := bash(e, cmd); !d.Critical {
			t.Errorf("%q from /proj/src/deep: not flagged", cmd)
		}
	}
	if d := bash(e, "rm -rf ../other"); d.Critical {
		t.Errorf("a sibling directory was flagged: %s", d.Reason)
	}
}

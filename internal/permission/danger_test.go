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

// TestOtherDestroyers: the other commands that destroy a directory which
// has to be kept are asked about like rm, in every mode, and no rule or
// hook approves one; their ordinary uses run as before.
func TestOtherDestroyers(t *testing.T) {
	o := testOptions(ModeBypass, rules{})
	o.Protected = []string{"/home/u/.config/gnosh", "/home/u/.local/share/gnosh"}
	e := mustEngine(t, o)
	destroys := []string{
		// mv takes it away.
		"mv ~ /tmp/h", "mv .git /tmp/g", "mv .git .git.bak", "mv * /tmp/", "mv ./* ../elsewhere", "mv -t /tmp .git src", "mv -t /tmp *",
		"mv --target-directory=/tmp *", "mv -vt /tmp .git", "mv /proj /tmp/p", "mv . /tmp/p", "mv ~/.config/gnosh /tmp/c", "mv ~/* /tmp/",
		"mv /home /x", "sudo mv /etc /x", "mv ${HOME} /tmp/h", "d=.git; mv $d /tmp", "mv ../proj /tmp", "mv \"$X\"/* /tmp", "mv $X/ /tmp",
		"mv -T /tmp/new ~", "mv --no-target-directory /tmp/new .git", "for d in .git src; do mv $d /tmp/; done", "mv \"$(pwd)\" /tmp/x",
		"mv ~/.local/share/gnosh /tmp", "mv ~/.config /tmp", "mv .gi* /tmp", "mv {.git,x} /tmp",
		// chmod, chown and chgrp over all of it.
		"chmod -R 000 .", "chmod -R 777 ~", "chmod -R a-w /", "chmod -R -w .", "chown -R nobody /", "chown -R u:g ~/",
		"chown -R --reference=a.txt .", "chgrp -R x ..", "chmod --recursive 700 .git", "chmod -fR 000 *", "chown -R u \"$X\"/",
		"chown -Rv u /etc", "chmod -R 000 ~/.config/gnosh", "chmod -R u+w ./*", "chown -hR u /usr", "chmod -R 644 $PWD",
		// git clean of what was never committed.
		"git clean -fdx", "git clean -xdf", "git clean -f -d -x", "git clean -fdX", "git clean --force -dx", "git -C .. clean -fdx",
		"git -C ~ clean -fdx", "git clean -fdx .", "git clean -fdx ~", "git -c core.x=y clean -ffdx", "cd ~ && git clean -fdx",
		"git --no-pager clean -xfd", "git clean -fdx -e keep", "git clean -fdx -- .",
		// dd, truncate and shred onto it.
		"dd if=/dev/zero of=/dev/sda", "dd of=/dev/nvme0n1 if=x", "shred -u *", "shred -u ~/*", "truncate -s 0 *", "truncate -s0 ./*",
		"shred /dev/sda1", "truncate -s 0 .git/*", "shred -u \"$DIR\"/*", "truncate --size=0 ~/.config/gnosh/*", "dd if=x of=/dev/disk2",
		"shred -n 3 -u /proj/*",
		// rsync that deletes what is in the way, or takes its sources away.
		"rsync -a --delete src/ .", "rsync -a --delete empty/ ~/", "rsync --del a/ /", "rsync -av --delete-after src/ ..",
		"rsync -a --delete x/ ~/.config/gnosh", "rsync -a --delete -e ssh host:dir/ .", "rsync -a --remove-source-files * host:/backup/",
		"rsync -a --remove-source-files ~ host:x", "rsync -a --delete src/ \"$DEST\"/", "rsync -a --delete --exclude .git src/ ./",
		"rsync --delete-excluded -a a/ /proj",
		// find that shreds what it finds.
		"find . -exec shred -u {} +", "find ~ -delete",
		// The folders that are kept, by rm as well.
		"rm -rf .git", "rm -rf ./.git/", "rm -rf .git/*", "rm -rf ../proj/.git", "rm -rf ~/.config/gnosh", "rm -rf ~/.config",
		"rm -rf ~/.local/share/gnosh/*", "rm -rf ~/.local", "rmdir .git", "rm -rf .gi*", "rm -rf .[!.]*", "rm -rf /proj/.GIT",
	}
	for _, cmd := range destroys {
		d := bash(e, cmd)
		if d.Behavior != Ask || !d.Critical || d.Stage != StageSafety || len(d.Suggestions) != 0 {
			t.Errorf("%q: got %s critical=%v at %s (%s), want a critical ask", cmd, d.Behavior, d.Critical, d.Stage, d.Reason)
		}
		if h := d.WithHook(HookAllow, ""); h.Behavior != Ask {
			t.Errorf("%q: a hook approved it", cmd)
		}
	}
	ordinary := []string{
		"mv a.txt b.txt", "mv build/out dist/", "mv *.log logs/", "mv src/a.go src/b.go", "mv x ~/", "mv x .", "mv -t build a.txt b.txt",
		"mv \"$tmp\" out.txt", "mv \"$f\" \"$f.bak\"", "mv vendor/x/.git /tmp/g", "mv -T /tmp/new build", "mv -f a b", "mv src/* dst/",
		"mv $tmp $dest", "mv -n a.txt ~/Documents/",
		"chmod +x script.sh", "chmod 644 a.txt", "chmod -R u+w build", "chown -R me:me node_modules", "chmod 700 ~", "chmod -R 755 src",
		"chown u .", "chmod -R go-w \"$dir\"", "chmod -x a.sh", "chgrp staff a.txt", "chmod -R 755 ~/bin",
		"git clean -fd", "git clean -n -fdx", "git clean -fdx --dry-run", "git clean -fdx build", "git clean -fX build/", "git clean -f",
		"git status", "git -C build clean -fdx", "git clean -fdx \"$dir\"", "git clean -nfdx", "git commit -m 'clean -fdx'",
		"dd if=a of=b", "dd if=/dev/zero of=build/img bs=1M count=1", "truncate -s 0 build.log", "shred -u secret.key",
		"truncate -s 0 \"$f\"", "shred -u \"$tmp\"", "dd if=/dev/sda of=disk.img", "truncate -s 1G build/sparse",
		"rsync -a src/ build/", "rsync -a --delete src/ dist/", "rsync -a --delete src/ host:/srv/app/", "rsync -a . host:backup/",
		"rsync -a --delete src/ \"$dest\"", "rsync -a ~/notes/ backup/", "rsync -a --delete build/ ~/public_html/",
		"rm .git/index.lock", "rm -rf .git/hooks/x", "rm -rf vendor/x/.git", "rm ~/.config/gnosh/old.json", "rm -rf ~/.cache",
		"rm -rf node_modules", "rm -rf ~/.config/other", "rm -rf ~/.local/share/other",
		"find . -name '*.orig' -exec shred -u {} +",
	}
	for _, cmd := range ordinary {
		if d := bash(e, cmd); d.Behavior != Allow {
			t.Errorf("%q with permissions skipped: got %s critical=%v (%s), want allow", cmd, d.Behavior, d.Critical, d.Reason)
		}
	}
	// A rule that allows the command does not approve the destruction,
	// and dontAsk refuses it.
	for mode, want := range map[Mode]Behavior{ModeDefault: Ask, ModeAcceptEdits: Ask, ModePlan: Ask, ModeDontAsk: Deny, ModeBypass: Ask} {
		o := testOptions(mode, rules{allow: []string{"Bash(mv *)", "Bash(chmod *)", "Bash(git clean *)", "Bash(rsync *)", "Bash"}})
		e := mustEngine(t, o)
		for _, cmd := range []string{"mv .git /tmp/g", "chmod -R 000 .", "git clean -fdx", "rsync -a --delete src/ .", "shred -u *"} {
			if d := bash(e, cmd); d.Behavior != want || !d.Critical {
				t.Errorf("%q in %s: got %s critical=%v, want %s and critical", cmd, mode, d.Behavior, d.Critical, want)
			}
		}
	}
}

// TestTakesAll: with a way to see what is in a directory, targets that
// between them are everything in one that has to be kept are a sweep,
// however they are written.
func TestTakesAll(t *testing.T) {
	// (The made-up file system matches only the names it is given: the
	// directories are listed beside what is in them.)
	fs := fakeFS{files: []string{"/proj/a.txt", "/proj/b.txt", "/proj/Makefile", "/proj/src", "/proj/src/c.go", "/proj/build", "/proj/build/out.o",
		"/proj/.env", "/proj/.git", "/proj/.git/HEAD", "/home/u/notes.txt", "/home/u/Documents", "/home/u/Documents/d", "/home/u/.ssh"}}
	o := testOptions(ModeBypass, rules{})
	o.Resolve, o.Glob = fs.resolve, fs.goGlob
	e := mustEngine(t, o)
	for _, cmd := range []string{
		"rm -rf [!.]*", "rm -rf *?", "rm -rf [a-zA-Z]*", "rm -rf [a-m]* [n-z]* [A-Z]*", "rm -rf a.txt b.txt Makefile src build",
		"rm -rf ./[!.]*", "rm -rf /proj/[^.]*", "rm -rf ~/[!.]*", "rm -rf {a,b}.txt Makefile src build", "rm -rf ?* x",
		"mv [!.]* /tmp/", "mv -t /tmp a.txt b.txt Makefile src build", "chmod -R 000 [!.]*", "shred -u [!.]*", "truncate -s 0 [a-zA-Z]*",
		"rm a.txt b.txt Makefile src build", "rm -rf *.txt Makefile src build",
	} {
		if d := bash(e, cmd); d.Behavior != Ask || !d.Critical {
			t.Errorf("%q: got %s critical=%v (%s), want a critical ask", cmd, d.Behavior, d.Critical, d.Reason)
		}
	}
	for _, cmd := range []string{
		"rm -rf *.txt", "rm -rf [ab]*", "rm -rf a.txt b.txt", "rm -rf src build", "rm -rf src/*", "rm -rf build/[!.]*", "rm -f *.txt Makefile",
		"mv *.txt src/", "chmod -R u+w src build", "shred -u a.txt", "rm -rf a.txt b.txt Makefile src", "rm -rf ~/Documents/*",
	} {
		if d := bash(e, cmd); d.Behavior != Allow {
			t.Errorf("%q with permissions skipped: got %s critical=%v (%s), want allow", cmd, d.Behavior, d.Critical, d.Reason)
		}
	}
}

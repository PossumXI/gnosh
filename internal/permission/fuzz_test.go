package permission

import (
	"strings"
	"testing"
	"time"

	"github.com/gnosh-ai/gnosh/internal/core"
)

// seedCommands start every fuzz target that takes a command line.
var seedCommands = []string{
	"ls", "ls -la src", "echo hello world", "cat a | grep b | wc -l", "git status && npm run build",
	"npm run build; rm -rf x", "echo $(rm -rf x)", "echo `id`", "(cd src && ls)", "{ ls; }",
	"if true; then ls; else rm x; fi", "for f in a b; do cat $f; done", "case $x in a) ls;; esac",
	"while read l; do echo $l; done < file", "f() { ls; }; f", "FOO=bar ls", "ls > out 2>&1",
	"cat <<EOF\nhello $USER\nEOF", "cat <<'EOF'\n$(rm x)\nEOF\nls", "sh -c 'rm -rf x'",
	"eval \"ls; rm x\"", "echo 'a; b' \"c && d\" e\\;f", "echo $'\\x72\\x6d'", "timeout 5 nice -n 3 ls",
	"find . -name '*.go' -exec rm {} \\;", "ls # comment", "ls # ; rm x", "npm test &&", "a | | b",
	"echo ${X:-$(id)} ${Y:?} $((1+2))", "diff <(ls a) <(ls b)", "arr=(a b c); echo ${arr[1]}",
	"xargs -n1 rm", "sudo -u root rm -rf /", "rm -rf \"$DIR\"/*", "cd /tmp && git status", "echo hi &",
	"ls;;", "ls &> out", "ls >| out", "x=1 y=2", "! true", "[[ -f x ]] && echo y", "echo \\", "echo '",
	"echo \"$(echo \"$(echo x)\")\"", "watch -n 1 'ls; rm x'", "a\\\nb", "ls\t-la\r\n", "ls \x00",
	" ", " \\\n ", "\\\n", "ls < /dev/null", "{a,b}", "~/bin/x", "=ls", "ls *(e:'id':)", "echo a >! b", "echo $[1+2]", "cat <<-EOF\n\tx\n\tEOF",
}

func addSeeds(f *testing.F) {
	for _, s := range seedCommands {
		f.Add(s)
	}
}

// FuzzShell: the reader never panics and always comes back, and what it
// reads holds together.
func FuzzShell(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, src string) {
		start := time.Now()
		sc := parseScript(src)
		if !sc.ok && sc.why == "" {
			t.Errorf("%q: refused without a reason", src)
		}
		for _, c := range sc.cmds {
			if sc.ok && len(c.words) == 0 && len(c.assigns) == 0 && len(c.redirs) == 0 {
				t.Errorf("%q: an empty command was read", src)
			}
			for _, w := range c.words {
				_ = w.static()
				_ = w.lit()
				_ = w.value("/home/u")
				_ = w.onlySubst()
				if len(w.parts) == 0 {
					t.Errorf("%q: a word with no parts", src)
				}
			}
		}
		crudeFragments(src)
		e := engine(t, ModeDefault, rules{})
		v := e.viewBash(src)
		for _, u := range v.units {
			e.denyTexts(u.cmd)
			facts(u.cmd.words)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%q (%d bytes): took %s", src, len(src), d)
		}
	})
}

// simpleProgName reports whether a program name can stand in a rule as it
// is.
func simpleProgName(s string) bool {
	if s == "" || s[0] == '-' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(isNameChar(c) || c == '.' || c == '/' || c == '-') {
			return false
		}
	}
	return true
}

// FuzzDecide holds the engine to its promises about shell commands.
func FuzzDecide(f *testing.F) {
	addSeeds(f)
	allow := []string{"Bash(echo *)", "Bash(ls *)", "Bash(git status)", "Bash(npm run *)", "Bash(cat *)", "Bash(make:*)"}
	f.Fuzz(func(t *testing.T, cmd string) {
		base := engine(t, ModeDefault, rules{allow: allow})
		d := bash(base, cmd)
		if d.Reason == "" {
			t.Errorf("%q: no reason", cmd)
		}
		if d.Behavior != Ask && len(d.Suggestions) != 0 {
			t.Errorf("%q: suggestions without a question", cmd)
		}

		// A deny rule that names any part of the command wins, whatever
		// else is allowed and whatever the mode.
		sc := parseScript(cmd)
		tried := 0
		for _, c := range sc.cmds {
			if len(c.words) == 0 || !c.words[0].static() || tried >= 4 {
				continue
			}
			name := c.words[0].joined()
			if !simpleProgName(name) {
				continue
			}
			tried++
			for _, mode := range []Mode{ModeDefault, ModeBypass} {
				e := engine(t, mode, rules{allow: append([]string{"Bash"}, allow...), deny: []string{"Bash(" + name + " *)"}})
				if got := bash(e, cmd); got.Behavior != Deny {
					t.Errorf("%q in %s: a deny rule for %q gave %s (%s)", cmd, mode, name, got.Behavior, got.Reason)
				}
			}
		}

		// Adding a denied command to an allowed one never yields Allow,
		// with the deny rule or without it (nothing allows rm here).
		if d.Behavior == Allow {
			guarded := engine(t, ModeDefault, rules{allow: allow, deny: []string{"Bash(rm *)"}})
			suffixes := []string{
				"; rm -rf zzz", " && rm -rf zzz", " || rm -rf zzz", " | rm -rf zzz", " & rm -rf zzz",
				"\nrm -rf zzz", " $(rm -rf zzz)", " `rm -rf zzz`", " <(rm -rf zzz)", "; (rm -rf zzz)",
			}
			if len(sc.comments) > 0 {
				// What is added to a line that ends in a comment is part of
				// the comment, which no shell runs. A new line is not.
				suffixes = []string{"\nrm -rf zzz", "\n(rm -rf zzz)", "\necho $(rm -rf zzz)"}
			}
			for _, suffix := range suffixes {
				if got := bash(guarded, cmd+suffix); got.Behavior == Allow {
					t.Errorf("%q + %q: allowed past a deny rule (%s)", cmd, suffix, got.Reason)
				}
				if got := bash(base, cmd+suffix); got.Behavior == Allow {
					t.Errorf("%q + %q: allowed though nothing allows rm (%s)", cmd, suffix, got.Reason)
				}
			}
			// Under a deny rule for it, the same holds in front.
			if got := bash(guarded, "rm -rf zzz; "+cmd); got.Behavior != Deny {
				t.Errorf("rm in front of %q: %s", cmd, got.Behavior)
			}
		}

		// Changing the white space around a command that is not allowed
		// never makes it allowed.
		if d.Behavior != Allow {
			variants := []string{" " + cmd, "\t" + cmd, "   " + cmd}
			if !strings.HasSuffix(cmd, "\\") {
				variants = append(variants, cmd+" ", cmd+"\t")
			}
			if !strings.ContainsAny(cmd, "'\"\\#<\n") {
				variants = append(variants,
					strings.ReplaceAll(cmd, " ", "  "),
					strings.ReplaceAll(cmd, " ", "\t"),
					strings.ReplaceAll(cmd, " ", " \\\n "))
			}
			for _, v := range variants {
				if got := bash(base, v); got.Behavior == Allow {
					t.Errorf("%q was %s, but %q is allowed (%s)", cmd, d.Behavior, v, got.Reason)
				}
			}
		}

		// The other modes never panic either, and bypass never turns a
		// refusal into a run.
		for _, mode := range []Mode{ModeAcceptEdits, ModePlan, ModeDontAsk, ModeBypass} {
			e := engine(t, mode, rules{allow: allow, deny: []string{"Bash(rm *)"}, ask: []string{"Bash(git push *)"}})
			got := bash(e, cmd)
			if mode == ModeDontAsk && got.Behavior == Ask {
				t.Errorf("%q: dontAsk asked", cmd)
			}
			if mode == ModePlan && got.Behavior == Allow && d.Behavior != Allow {
				t.Errorf("%q: plan mode allowed what default mode did not", cmd)
			}
		}
	})
}

// FuzzRule: reading a rule never panics; a rule that reads prints back to
// itself; New either takes a rule or names it in its error.
func FuzzRule(f *testing.F) {
	for _, s := range []string{
		"Bash", "Bash(ls *)", "Bash(ls:*)", "Read(.env)", "Edit(/src/**)", "Read(//a/**/b)", "Read(~/x)",
		"Read(!x)", "WebFetch(domain:*.example.com)", "mcp__a__b", "mcp__*", "*", "Agent(Explore)",
		"Skill(x *)", "Bash(a:b)", "Bash(run_in_background:true)", "Bash(", "Bash()", "(", ")", "A(B(C))",
		"Read([)", "Read(\\)", "Read(a/../b)", "Bash(\\*)", "Bash(**)", "Write(x)", "Nope",
	} {
		f.Add(s, "/proj/a/b.txt")
	}
	f.Fuzz(func(t *testing.T, rule, target string) {
		r, err := ParseRule(rule)
		if err == nil {
			again, err2 := ParseRule(r.String())
			if err2 != nil || again != r {
				t.Errorf("%q printed as %q, which reads as %+v (%v)", rule, r.String(), again, err2)
			}
		}
		for _, list := range []List{ListAllow, ListAsk, ListDeny} {
			o := testOptions(ModeDefault, rules{})
			switch list {
			case ListAllow:
				o.Layers[0].Allow = []string{rule}
			case ListAsk:
				o.Layers[0].Ask = []string{rule}
			default:
				o.Layers[0].Deny = []string{rule}
			}
			e, err := New(o)
			if err != nil {
				if !strings.Contains(err.Error(), "user settings") {
					t.Errorf("%q: the error does not name the source: %v", rule, err)
				}
				continue
			}
			// The engine built: every kind of call must come back.
			bash(e, target)
			if strings.HasPrefix(target, "/") {
				file(e, "Read", target)
				file(e, "Edit", target)
			}
			fetch(e, target)
			other(e, "mcp__a__b", "x", target)
			e.ToolEnabled(target)
			e.Explain()
		}
	})
}

// FuzzPath: the path matcher never panics, comes back quickly whatever
// the pattern, and an escaped path matches itself and only itself.
func FuzzPath(f *testing.F) {
	f.Add("**/a*b?c[d-f]/**", "/proj/x/aXbYce/z")
	f.Add("*a*a*a*a*a*a*a*a*b", "/proj/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	f.Add("**/**/**/**/**/**/x", "/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/a/y")
	f.Add("[", "/proj/[")
	f.Add("a\\", "/proj/a\\")
	f.Fuzz(func(t *testing.T, pattern, target string) {
		start := time.Now()
		for _, list := range []List{ListAllow, ListDeny} {
			o := testOptions(ModeDefault, rules{})
			rule := "Read(" + pattern + ")"
			if list == ListAllow {
				o.Layers[0].Allow = []string{rule}
			} else {
				o.Layers[0].Deny = []string{rule}
			}
			e, err := New(o)
			if err != nil {
				continue
			}
			if strings.HasPrefix(target, "/") {
				file(e, "Read", target)
			}
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("pattern %q on %q took %s", pattern, target, d)
		}
		if !strings.HasPrefix(target, "/") || strings.ContainsAny(target, "()\x00") {
			return
		}
		abs := cleanPath(target)
		if abs == "/" {
			return
		}
		e := engine(t, ModeDefault, rules{allow: []string{"Read(/" + EscapePath(abs) + ")"}})
		c := e.rules(ListAllow)[0]
		if !c.path.match(abs) {
			t.Errorf("the escaped rule for %q does not match it (segments %q)", abs, c.path.segs)
		}
		if c.path.match(abs + "x") {
			t.Errorf("the escaped rule for %q matches another path", abs)
		}
		// A deny rule for a path always covers that path, whatever its
		// name is made of, and every other spelling of it that a file
		// system may fold into it.
		guard, err := New(testOptions(ModeBypass, rules{deny: []string{"Read(/" + EscapePath(abs) + ")"}}))
		if err != nil {
			t.Errorf("the escaped deny rule for %q is refused: %v", abs, err)
			return
		}
		upper := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r - 'a' + 'A'
			}
			return r
		}, abs)
		spellings := []string{abs, upper, abs + "/", abs + "/."}
		if strings.Trim(baseName(abs), " .") != "" {
			// (A name of nothing but dots and blanks has no shorter form.)
			spellings = append(spellings, abs+".", abs+" ")
		}
		for _, spelling := range spellings {
			if !guard.ReadDenied(spelling) {
				t.Errorf("the deny rule for %q does not cover %q", abs, spelling)
			}
		}
	})
}

// FuzzAnyCall: no tool name, input or action makes the engine panic, and
// a tool this package does not know never runs without a rule.
func FuzzAnyCall(f *testing.F) {
	f.Add("Bash", `{"command":"ls"}`, "ls", "/proj/a", "https://example.com")
	f.Add("Read", `{"file_path":"/etc/passwd"}`, "", "/etc/passwd", "")
	f.Add("mcp__x__y", `{"a":[1,2]}`, "", "", "")
	f.Add("Nope", `not json`, "rm -rf /", "relative", "::::")
	f.Add("Agent", `{"subagent_type":7}`, "", "", "")
	f.Fuzz(func(t *testing.T, tool, input, cmd, p, url string) {
		for _, mode := range Modes {
			e := engine(t, mode, rules{deny: []string{"Read(.env)", "Bash(rm *)"}})
			for _, kind := range []core.ActionKind{core.ActionRead, core.ActionEdit, core.ActionExecute, core.ActionNetwork, core.ActionOther} {
				d := e.Decide(tool, core.ToolCall{Name: tool, Input: []byte(input)},
					core.Action{Kind: kind, Paths: []string{p}, Command: cmd, URL: url})
				if d.Reason == "" || d.Behavior == "" {
					t.Errorf("%s: an empty decision", tool)
				}
				if _, known := builtinTools[CanonicalTool(tool)]; !known && d.Behavior == Allow && mode != ModeBypass {
					t.Errorf("unknown tool %q ran in %s without a rule", tool, mode)
				}
			}
		}
	})
}

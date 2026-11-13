package permission

import (
	"strings"
	"testing"
)

// cmdsOf prints the simple commands of a script, one per item: the
// literal words joined by spaces.
func cmdsOf(sc *script) []string {
	var out []string
	for _, c := range sc.cmds {
		out = append(out, strings.Join(litsOf(c.words), " "))
	}
	return out
}

func TestParseSplitsCommands(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"ls", []string{"ls"}},
		{"ls -la /tmp", []string{"ls -la /tmp"}},
		{"a && b || c ; d | e & f", []string{"a", "b", "c", "d", "e", "f"}},
		{"a\nb", []string{"a", "b"}},
		{"a |& b", []string{"a", "b"}},
		{"ls  \t -la", []string{"ls -la"}},
		{"ls \\\n -la", []string{"ls -la"}},
		{"l\\\ns", []string{"ls"}},
		{"echo 'a; b' \"c && d\"", []string{"echo a; b c && d"}},
		{"echo a\\;b", []string{"echo a;b"}},
		{"echo $'\\x72\\x6d' $'a\\nb'", []string{"echo rm a\nb"}},
		{"'r'm -rf x", []string{"rm -rf x"}},
		{"r\"m\" -rf x", []string{"rm -rf x"}},
		{"\\rm -rf x", []string{"rm -rf x"}},
		{"echo $(rm x)", []string{"rm x", "echo $(rm x)"}},
		{"echo `rm x`", []string{"rm x", "echo `rm x`"}},
		{"echo \"$(rm x)\"", []string{"rm x", "echo \"$(rm x)\""}},
		{"cat <(rm x)", []string{"rm x", "cat <(rm x)"}},
		{"(rm x)", []string{"rm x"}},
		{"{ rm x; }", []string{"rm x"}},
		{"if a; then b; elif c; then d; else e; fi", []string{"a", "b", "c", "d", "e"}},
		{"while a; do b; done", []string{"a", "b"}},
		{"until a; do b; done", []string{"a", "b"}},
		{"for f in 1 2; do rm $f; done", []string{"rm $f"}},
		{"for f\ndo rm $f\ndone", []string{"rm $f"}},
		{"case $x in a|b) rm x;; *) ls;; esac", []string{"rm x", "ls"}},
		{"f() { rm x; }; f", []string{"rm x", "f"}},
		{"function f { rm x; }", []string{"rm x"}},
		{"! grep -q x f", []string{"grep -q x f"}},
		{"FOO=bar rm x", []string{"rm x"}},
		{"FOO=bar BAZ=$(id) rm x", []string{"id", "rm x"}},
		{"ls # ignored words", []string{"ls"}},
		{"ls > out.txt 2>&1", []string{"ls"}},
		{"cat <<EOF\nrm x; ls\nEOF\necho done", []string{"cat", "echo done"}},
		{"cat <<'EOF'\n$(rm x)\nEOF", []string{"cat"}},
		{"cat <<EOF\n$(rm x)\nEOF", []string{"cat", "rm x"}},
		{"cat <<-EOF\n\tbody\n\tEOF\nls", []string{"cat", "ls"}},
		{"echo ${X:-$(rm x)}", []string{"rm x", "echo ${X:-$(rm x)}"}},
		{"echo $((1+2))", []string{"echo $((1+2))"}},
		{"arr=(a b $(rm x)) ls", []string{"rm x", "ls"}},
		{"find . -name '*.go' -exec rm {} \\;", []string{"find . -name *.go -exec rm {} ;"}},
		{"[[ -f x && -d y ]]", []string{"[[ -f x -d y ]]"}},
		{"[[ -f x ]] && rm y || ls", []string{"[[ -f x ]]", "rm y", "ls"}},
		{"[[ -f <(rm x) ]]", []string{"rm x", "[[ -f <(rm x) ]]"}},
		{"[[ $x =~ ^(a|b)$ ]] && ls", []string{"[[ $x =~ ^ a b $ ]]", "ls"}},
		{"[[ -n $(rm x) ]]", []string{"rm x", "[[ -n $(rm x) ]]"}},
		{"(( x = 1 ))", nil},
		{"(( n > 0 )) && rm y", []string{"rm y"}},
		{"for ((i=0;i<3;i++)); do rm $i; done", []string{"rm $i"}},
		{"echo ${x[1]} ${#x} ${x:0:3} ${x//a/b} ${x^^} ${!x} ${arr[@]}", []string{"echo ${x[1]} ${#x} ${x:0:3} ${x//a/b} ${x^^} ${!x} ${arr[@]}"}},
	}
	for _, tt := range tests {
		sc := parseScript(tt.src)
		if !sc.ok {
			t.Errorf("%q: not read: %s", tt.src, sc.why)
			continue
		}
		got := cmdsOf(sc)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") {
			t.Errorf("%q:\n got  %q\n want %q", tt.src, got, tt.want)
		}
	}
}

func TestParseRefusesWhatItCannotFollow(t *testing.T) {
	for _, src := range []string{
		"npm test &&",
		"&& ls",
		"ls |",
		"ls ||",
		"ls ; ; ls",
		"ls\n&& rm x",
		"echo 'unclosed",
		"echo \"unclosed",
		"echo `unclosed",
		"echo $(unclosed",
		"echo ${unclosed",
		"echo trailing\\",
		"(ls",
		"ls)",
		"{ ls",
		"}",
		"if true; then ls",
		"for x in a; do ls",
		"case x in a) ls",
		"[[ -f x",
		"[[ a; b ]]",
		"[[ a & b ]]",
		"(( a['$(id)'] ))",
		"(( x = 1 )",
		"for ((i=0;i<3;i++) do ls; done",
		"echo ${x[$(id)]}",
		"echo ${x:0:`id`}",
		"echo ${x/a/'b'}",
		"echo ${x/a/(b)}",
		"select x in a b; do ls; done",
		"cat <<EOF\nnever closed",
		"cat <<EOF",
		"echo ${(e)x}",
		"echo ${x:-\"}\"}",
		"echo $[1+2]",
		"echo $((a[$(id)]))",
		"ls *(e:'id':)",
		"ls foo(bar)",
		"echo x >! file",
		"echo a\x00b",
		"echo a\rb",
		"echo $'a\\0b'",
		"ls # ; rm -rf x",
		"ls & | cat",
		"ls > ",
		"time { ls; }",
		"coproc ls",
		"then ls",
	} {
		if sc := parseScript(src); sc.ok {
			t.Errorf("%q: read as %q, want it refused", src, cmdsOf(sc))
		} else if sc.why == "" {
			t.Errorf("%q: refused without a reason", src)
		}
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		src                                 string
		subst, subshell, compound, backgrnd bool
	}{
		{"ls", false, false, false, false},
		{"echo $(ls)", true, false, false, false},
		{"echo `ls`", true, false, false, false},
		{"diff <(ls) <(ls)", true, false, false, false},
		{"(ls)", false, true, false, false},
		{"{ ls; }", false, false, true, false},
		{"for x in a; do ls; done", false, false, true, false},
		{"f() { ls; }", false, false, true, false},
		{"ls &", false, false, false, true},
		{"ls & ls", false, false, false, true},
		{"ls && ls", false, false, false, false},
		{"ls 2>&1", false, false, false, false},
		{"ls &> out", false, false, false, false},
		{"cat <<EOF\n`ls`\nEOF", true, false, false, false},
	}
	for _, tt := range tests {
		sc := parseScript(tt.src)
		if !sc.ok {
			t.Errorf("%q: not read: %s", tt.src, sc.why)
			continue
		}
		if sc.subst != tt.subst || sc.subshell != tt.subshell || sc.compound != tt.compound || sc.background != tt.backgrnd {
			t.Errorf("%q: subst=%v subshell=%v compound=%v background=%v", tt.src, sc.subst, sc.subshell, sc.compound, sc.background)
		}
	}
}

func TestParseWords(t *testing.T) {
	sc := parseScript(`cmd plain 'quoted' "dq $V" $V ${V:?} ~/x ~user *.go {a,b} =ls "$(id)" x$(id)`)
	if !sc.ok {
		t.Fatal(sc.why)
	}
	c := sc.cmds[len(sc.cmds)-1]
	type wd struct {
		static, plain, quoted, glob, brace, tilde, onlySubst bool
	}
	want := []wd{
		{static: true, plain: true},     // cmd
		{static: true, plain: true},     // plain
		{static: true, quoted: true},    // 'quoted'
		{quoted: true},                  // "dq $V"
		{},                              // $V
		{},                              // ${V:?}
		{static: true, tilde: true},     // ~/x
		{tilde: true},                   // ~user
		{static: true, glob: true},      // *.go
		{static: true, brace: true},     // {a,b}
		{},                              // =ls
		{quoted: true, onlySubst: true}, // "$(id)"
		{},                              // x$(id)
	}
	if len(c.words) != len(want) {
		t.Fatalf("got %d words, want %d: %q", len(c.words), len(want), litsOf(c.words))
	}
	for i, w := range c.words {
		got := wd{w.static(), w.plain(), w.quoted, w.glob, w.brace, w.tilde, w.onlySubst()}
		if got != want[i] {
			t.Errorf("word %d %q: got %+v, want %+v", i, w.raw, got, want[i])
		}
	}
	if !c.words[5].parts[0].guarded {
		t.Errorf("${V:?} is not seen as guarded")
	}
	if got := c.words[6].value("/home/u"); got != "/home/u/x" {
		t.Errorf("~/x became %q", got)
	}
}

func TestParseRedirections(t *testing.T) {
	sc := parseScript("cmd <in >out 2>>err 2>&1 &>all <<<word >&2 3<&-")
	if !sc.ok {
		t.Fatal(sc.why)
	}
	var got []string
	for _, r := range sc.cmds[0].redirs {
		got = append(got, r.fd+r.op+r.target.raw)
	}
	want := "<in >out 2>>err 2>&1 &>all <<<word >&2 3<&-"
	if strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

func TestCrudeFragments(t *testing.T) {
	got := crudeFragments("ls; if x; then FOO=1 'r'm -rf y; fi | (sudo rm z)")
	for _, want := range []string{"ls", "'r'm -rf y", "rm -rf y", "sudo rm z", "FOO=1 'r'm -rf y"} {
		found := false
		for _, g := range got {
			found = found || g == want
		}
		if !found {
			t.Errorf("fragment %q missing from %q", want, got)
		}
	}
}

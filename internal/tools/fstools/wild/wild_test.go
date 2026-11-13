package wild

import (
	"errors"
	"path"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSegmentMatch(t *testing.T) {
	tests := []struct {
		pattern, name string
		want          bool
	}{
		// Literals.
		{"", "", true},
		{"", "a", false},
		{"abc", "abc", true},
		{"abc", "abd", false},
		{"abc", "ab", false},
		{"abc", "abcd", false},
		{"ABC", "abc", false},
		// Star.
		{"*", "", true},
		{"*", "anything.go", true},
		{"*.go", "main.go", true},
		{"*.go", ".go", true},
		{"*.go", "main.goo", false},
		{"*.go", "main.go.bak", false},
		{"main*", "main", true},
		{"main*", "main_test.go", true},
		{"a*b*c", "abc", true},
		{"a*b*c", "aXbXc", true},
		{"a*b*c", "aXbXcX", false},
		{"a*b*c", "abcbc", true},
		{"*a*", "banana", true},
		{"*a*", "bnn", false},
		{"**", "x", true},
		{"a**b", "ab", true},
		{"a**b", "aXXb", true},
		{"a***b", "ab", true},
		{"*ab", "aab", true},
		{"*aab", "aaab", true},
		{"*?", "", false},
		{"*?", "a", true},
		// Question mark.
		{"?", "", false},
		{"?", "a", true},
		{"?", "ab", false},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"??", "é", false},
		{"?", "é", true},
		{"?", "日", true},
		{"*??", "日", false},
		{"*??", "日本", true},
		// Brackets.
		{"[abc]", "b", true},
		{"[abc]", "d", false},
		{"[abc]", "", false},
		{"[a-c]x", "bx", true},
		{"[a-c]x", "dx", false},
		{"[!a-c]", "d", true},
		{"[!a-c]", "b", false},
		{"[^a-c]", "d", true},
		{"[^a-c]", "b", false},
		{"[]]", "]", true},
		{"[]a]", "a", true},
		{"[]a]", "]", true},
		{"[!]]", "]", false},
		{"[!]]", "x", true},
		{"[a-]", "-", true},
		{"[a-]", "a", true},
		{"[a-]", "b", false},
		{"[-a]", "-", true},
		{"[-a]", "a", true},
		{"[a\\-c]", "-", true},
		{"[a\\-c]", "b", false},
		{"[\\]]", "]", true},
		{"[[]", "[", true},
		{"[z-a]", "m", false},
		{"[é-ü]", "ö", true},
		{"[é-ü]", "e", false},
		{"[[:digit:]]", "7", true},
		{"[[:digit:]]", "x", false},
		{"[[:alpha:][:digit:]]", "x", true},
		{"[[:alpha:][:digit:]]", "7", true},
		{"[[:alpha:][:digit:]]", "-", false},
		{"[![:space:]]", " ", false},
		{"[![:space:]]", "x", true},
		{"[[:upper:]]*", "Makefile", true},
		{"[[:upper:]]*", "makefile", false},
		{"[[:xdigit:]]", "F", true},
		{"[[:xdigit:]]", "g", false},
		{"[[:punct:]]", "!", true},
		{"[[:punct:]]", "a", false},
		{"[[:alnum:]_]", "_", true},
		{"*.[oa]", "lib.a", true},
		{"*.[oa]", "file.o", true},
		{"*.[oa]", "file.c", false},
		// Escapes.
		{"\\*", "*", true},
		{"\\*", "a", false},
		{"\\?", "?", true},
		{"\\?", "a", false},
		{"\\[a]", "[a]", true},
		{"a\\b", "ab", true},
		{"\\\\", "\\", true},
		{"\\#", "#", true},
		{"\\!x", "!x", true},
		{"foo\\ ", "foo ", true},
		// A stray closing bracket or brace is itself.
		{"a]", "a]", true},
		{"a}", "a}", true},
		{"{a,b}", "{a,b}", true},
	}
	for _, tt := range tests {
		seg, err := CompileSegment(tt.pattern)
		if err != nil {
			t.Errorf("CompileSegment(%q): %v", tt.pattern, err)
			continue
		}
		if got := seg.Match(tt.name); got != tt.want {
			t.Errorf("CompileSegment(%q).Match(%q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestSyntaxErrors(t *testing.T) {
	tests := []struct {
		pattern string
		problem string
	}{
		{"[", "no closing ]"},
		{"[a", "no closing ]"},
		{"[]", "no closing ]"},
		{"[!]", "no closing ]"},
		{"[^", "no closing ]"},
		{"a[b-", "no closing ]"},
		{"\\", "nothing after it"},
		{"abc\\", "nothing after it"},
		{"[a\\", "nothing after it"},
		{"[[:nope:]]", "not a character class"},
		{"[\xff]", "not valid UTF-8"},
	}
	for _, tt := range tests {
		_, err := Compile(tt.pattern)
		var se *SyntaxError
		if !errors.As(err, &se) {
			t.Errorf("Compile(%q) error = %v, want a *SyntaxError", tt.pattern, err)
			continue
		}
		if se.Pattern != tt.pattern || !strings.Contains(se.Problem, tt.problem) {
			t.Errorf("Compile(%q) error = %v, want one about %q", tt.pattern, err, tt.problem)
		}
		if _, err := CompileSegment(tt.pattern); err == nil {
			t.Errorf("CompileSegment(%q) succeeded", tt.pattern)
		}
		if _, err := CompileGlob(tt.pattern); err == nil {
			t.Errorf("CompileGlob(%q) succeeded", tt.pattern)
		}
	}
	if _, err := CompileSegment("a/b"); err == nil {
		t.Error(`CompileSegment("a/b") succeeded; a segment cannot hold a slash`)
	}
	if _, err := CompileSegment("a\\/b"); err == nil {
		t.Error(`CompileSegment("a\\/b") succeeded; an escaped slash is still a slash`)
	}
}

func TestPatternMatch(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"", "", true},
		{"a", "a", true},
		{"a/b", "a/b", true},
		{"a/b", "a/c", false},
		{"a/b", "a", false},
		{"a/b", "a/b/c", false},
		{"a\\/b", "a/b", true},
		{"*", "a", true},
		{"*", "a/b", false},
		{"*/*", "a/b", true},
		{"*/*", "a", false},
		{"*/*", "a/b/c", false},
		{"a/*/c", "a/b/c", true},
		{"a/*/c", "a/b/d/c", false},
		{"a?c", "a/c", false},
		{"a[/]c", "a/c", false},
		{"a[!x]c", "a/c", false},
		{"*.go", "main.go", true},
		{"*.go", "sub/main.go", false},
		// Empty segments are segments.
		{"a//b", "a//b", true},
		{"a//b", "a/b", false},
		{"/a", "/a", true},
		{"/a", "a", false},
		{"a/", "a/", true},
		{"a/", "a", false},
		// "**" before something: no segments or more.
		{"**/a", "a", true},
		{"**/a", "x/a", true},
		{"**/a", "x/y/a", true},
		{"**/a", "x/ab", false},
		{"**/a", "a/x", false},
		{"**/*.go", "main.go", true},
		{"**/*.go", "a/b/main.go", true},
		{"**/*.go", "a/b/main.c", false},
		{"**/foo/bar", "foo/bar", true},
		{"**/foo/bar", "x/foo/bar", true},
		{"**/foo/bar", "foo/x/bar", false},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/c", false},
		{"a/**/b", "b", false},
		{"a/**/b", "x/a/b", false},
		{"**/**/a", "a", true},
		{"**/**/a", "x/y/z/a", true},
		{"a/**/**/b", "a/b", true},
		{"src/**/*.ts", "src/a.ts", true},
		{"src/**/*.ts", "src/x/y/a.ts", true},
		{"src/**/*.ts", "lib/a.ts", false},
		{"src/**/*.ts", "src/a.js", false},
		{"**/a/**/b", "a/b", true},
		{"**/a/**/b", "x/a/y/b", true},
		{"**/a/**/b", "x/b", false},
		// "**" last: one segment or more.
		{"**", "a", true},
		{"**", "a/b/c", true},
		{"a/**", "a/b", true},
		{"a/**", "a/b/c", true},
		{"a/**", "a", false},
		{"a/**", "b/c", false},
		{"**/**", "a", true},
		{"**/**", "a/b", true},
		{"**/a/**", "a/b", true},
		{"**/a/**", "x/a/b/c", true},
		{"**/a/**", "x/a", false},
		// Two stars that are not a whole segment are one star.
		{"a**/b", "ax/b", true},
		{"a**/b", "a/x/b", false},
		{"**a/b", "xa/b", true},
		{"**a/b", "x/a/b", false},
		// Three stars as a whole segment are read as two.
		{"***/b", "x/b", true},
		{"***/b", "x/y/b", true},
		{"***/b", "b", true},
		{"a/****", "a/x/y", true},
		{"a/****", "a", false},
		{"a/**b", "a/xb", true},
		{"a/**b", "a/x/b", false},
		// Many segments: more than the fixed buffer holds.
		{"a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s", "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s", true},
		{"a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/**/s", "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/x/y/s", true},
		{"a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s", "a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/t", false},
	}
	for _, tt := range tests {
		p, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", tt.pattern, err)
			continue
		}
		if got := p.Match(tt.path); got != tt.want {
			t.Errorf("Compile(%q).Match(%q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}

func TestMatchPrefix(t *testing.T) {
	tests := []struct {
		pattern, dir string
		want         bool
	}{
		{"*.go", "sub", false},
		{"a/*.go", "a", true},
		{"a/*.go", "b", false},
		{"a/*.go", "a/b", false},
		{"a/b/c", "a", true},
		{"a/b/c", "a/b", true},
		{"a/b/c", "a/b/c", false},
		{"a/b/c", "a/x", false},
		{"**/*.go", "anything", true},
		{"**/*.go", "any/depth/at/all", true},
		{"src/**/*.go", "src", true},
		{"src/**/*.go", "src/x/y", true},
		{"src/**/*.go", "lib", false},
		{"a/**", "a", true},
		{"a/**", "a/b/c", true},
		{"a/**", "b", false},
		{"**", "a/b", true},
		{"*/b/*", "x", true},
		{"*/b/*", "x/b", true},
		{"*/b/*", "x/c", false},
		{"**/node_modules/*.js", "a/node_modules", true},
		{"**/node_modules/*.js", "a/b/c", true},
	}
	for _, tt := range tests {
		p, err := Compile(tt.pattern)
		if err != nil {
			t.Errorf("Compile(%q): %v", tt.pattern, err)
			continue
		}
		if got := p.MatchPrefix(tt.dir); got != tt.want {
			t.Errorf("Compile(%q).MatchPrefix(%q) = %v, want %v", tt.pattern, tt.dir, got, tt.want)
		}
	}
}

func TestExpand(t *testing.T) {
	tests := []struct {
		pattern string
		want    []string
	}{
		{"abc", []string{"abc"}},
		{"", []string{""}},
		{"{a,b}", []string{"a", "b"}},
		{"*.{json,yaml}", []string{"*.json", "*.yaml"}},
		{"{a,b}{c,d}", []string{"ac", "ad", "bc", "bd"}},
		{"x{a,b{c,d}}y", []string{"xay", "xbcy", "xbdy"}},
		{"{src,test}/**/*.go", []string{"src/**/*.go", "test/**/*.go"}},
		{"{a/b,c}/d", []string{"a/b/d", "c/d"}},
		{"a{,b}", []string{"a", "ab"}},
		{"a{b}", []string{"ab"}},
		{"a{}", []string{"a"}},
		{"{,}", []string{"", ""}},
		{"a}", []string{"a}"}},
		{"a,b", []string{"a,b"}},
		{"\\{a,b\\}", []string{"\\{a,b\\}"}},
		{"{a\\,b,c}", []string{"a\\,b", "c"}},
		{"{a\\},b}", []string{"a\\}", "b"}},
		{"[{]a,b}", []string{"[{]a,b}"}},
		{"{[,]a,b}", []string{"[,]a", "b"}},
		{"{[}]a,b}", []string{"[}]a", "b"}},
		{"{{a,b},c}", []string{"a", "b", "c"}},
		{"{a,{b,{c,d}}}", []string{"a", "b", "c", "d"}},
	}
	for _, tt := range tests {
		got, err := Expand(tt.pattern, MaxAlternatives)
		if err != nil {
			t.Errorf("Expand(%q): %v", tt.pattern, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Expand(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
	for _, bad := range []string{"{", "a{b", "{a,b", "{a,{b}", "x{a,[}]"} {
		if _, err := Expand(bad, MaxAlternatives); err == nil {
			t.Errorf("Expand(%q) succeeded; the brace is never closed", bad)
		}
	}
}

func TestExpandLimit(t *testing.T) {
	// Ten groups of two are 1024 patterns: exactly the limit.
	if got, err := Expand(strings.Repeat("{a,b}", 10), MaxAlternatives); err != nil || len(got) != 1024 {
		t.Fatalf("ten groups: %d patterns, err %v; want 1024, nil", len(got), err)
	}
	// Eleven are 2048.
	_, err := Expand(strings.Repeat("{a,b}", 11), MaxAlternatives)
	if err == nil || !strings.Contains(err.Error(), "more than 1024") {
		t.Fatalf("eleven groups: err = %v, want a refusal that names the limit", err)
	}
	// A pattern that would grow without end must be refused quickly.
	if _, err := CompileGlob(strings.Repeat("{a,b,c,d}", 200)); err == nil {
		t.Fatal("200 groups of four compiled")
	}
	// Many groups of one alternative are fine: they stand for one pattern.
	got, err := Expand(strings.Repeat("{a}", 500), MaxAlternatives)
	if err != nil || len(got) != 1 || got[0] != strings.Repeat("a", 500) {
		t.Fatalf("500 groups of one: %d patterns, err %v", len(got), err)
	}
	// So is deep nesting.
	deep := strings.Repeat("{", 300) + "a" + strings.Repeat("}", 300)
	got, err = Expand(deep, MaxAlternatives)
	if err != nil || len(got) != 1 || got[0] != "a" {
		t.Fatalf("300 nested groups: %q, err %v", got, err)
	}
}

func TestGlob(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"*.{json,yaml}", "a.json", true},
		{"*.{json,yaml}", "a.yaml", true},
		{"*.{json,yaml}", "a.yml", false},
		{"*.{json,yaml}", "sub/a.json", false},
		{"**/*.{ts,tsx}", "src/app/page.tsx", true},
		{"**/*.{ts,tsx}", "src/app/page.jsx", false},
		{"{src,test}/**/*.go", "test/a/b.go", true},
		{"{src,test}/**/*.go", "lib/a/b.go", false},
		{"**/{Makefile,*.mk}", "Makefile", true},
		{"**/{Makefile,*.mk}", "build/rules.mk", true},
		{"**/{Makefile,*.mk}", "build/rules.cmake", false},
		{"\\{a\\}", "{a}", true},
		{"**/*_test.go", "internal/core/accumulate_test.go", true},
		{"**/*_test.go", "internal/core/accumulate.go", false},
		{"internal/**", "internal/core/tool.go", true},
		{"internal/**", "cmd/main.go", false},
	}
	for _, tt := range tests {
		g, err := CompileGlob(tt.pattern)
		if err != nil {
			t.Errorf("CompileGlob(%q): %v", tt.pattern, err)
			continue
		}
		if got := g.Match(tt.path); got != tt.want {
			t.Errorf("CompileGlob(%q).Match(%q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
	_, err := CompileGlob("{a,[}")
	var se *SyntaxError
	if !errors.As(err, &se) || se.Pattern != "{a,[}" {
		t.Errorf(`CompileGlob("{a,[}") error = %v; want a *SyntaxError naming the pattern as written`, err)
	}
}

func TestSplitDir(t *testing.T) {
	tests := []struct {
		pattern  string
		wantDir  string
		restOK   []string // paths below the directory that the rest must match
		restFail []string
	}{
		{"*.go", "", []string{"a.go"}, []string{"x/a.go"}},
		{"**/*.go", "", []string{"a.go", "x/a.go"}, nil},
		{"src/*.go", "src", []string{"a.go"}, []string{"src/a.go", "x/a.go"}},
		{"src/pkg/**/*.go", "src/pkg", []string{"a.go", "x/y/a.go"}, []string{"a.c"}},
		{"src/main.go", "src", []string{"main.go"}, []string{"other.go"}},
		{"main.go", "", []string{"main.go"}, nil},
		{"a/b/c/d", "a/b/c", []string{"d"}, []string{"c/d"}},
		{"{a,b}/x.go", "", []string{"a/x.go", "b/x.go"}, []string{"x.go"}},
		{"src/{a,b}/x.go", "src", []string{"a/x.go", "b/x.go"}, []string{"c/x.go"}},
		{"src/{a/x,a/y}/z.go", "src/a", []string{"x/z.go", "y/z.go"}, []string{"z.go"}},
		{"{src/a,src/a}", "src", []string{"a"}, nil},
		{"a/*/c", "a", []string{"b/c"}, []string{"c"}},
		{"a\\*/c", "a*", []string{"c"}, nil},
		{"../x/*.go", "../x", []string{"a.go"}, nil},
		{"./src/*.go", "./src", []string{"a.go"}, nil},
		{"a//b", "a", nil, []string{"b"}},
		{"/a/b", "", nil, []string{"a/b", "b"}},
	}
	for _, tt := range tests {
		g, err := CompileGlob(tt.pattern)
		if err != nil {
			t.Errorf("CompileGlob(%q): %v", tt.pattern, err)
			continue
		}
		dir, rest := g.SplitDir()
		if dir != tt.wantDir {
			t.Errorf("CompileGlob(%q).SplitDir() dir = %q, want %q", tt.pattern, dir, tt.wantDir)
		}
		for _, p := range tt.restOK {
			if !rest.Match(p) {
				t.Errorf("CompileGlob(%q): rest after %q does not match %q", tt.pattern, dir, p)
			}
		}
		for _, p := range tt.restFail {
			if rest.Match(p) {
				t.Errorf("CompileGlob(%q): rest after %q matches %q", tt.pattern, dir, p)
			}
		}
	}
}

func TestLiteral(t *testing.T) {
	for pattern, want := range map[string]string{"abc": "abc", "": "", "a\\*b": "a*b", "a.b": "a.b"} {
		seg, err := CompileSegment(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := seg.Literal(); !ok || got != want {
			t.Errorf("CompileSegment(%q).Literal() = %q, %v; want %q, true", pattern, got, ok, want)
		}
	}
	for _, pattern := range []string{"a*", "?", "[ab]", "*"} {
		seg, err := CompileSegment(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := seg.Literal(); ok {
			t.Errorf("CompileSegment(%q).Literal() says it has no wildcard", pattern)
		}
	}
}

// comparableWithPathMatch reports whether path.Match and Pattern.Match are
// meant to give the same answer for this pattern and name. The cases left
// out are the ones the package comment lists.
func comparableWithPathMatch(pattern, name string) bool {
	if !utf8.ValidString(pattern) || !utf8.ValidString(name) {
		return false
	}
	for _, s := range []string{"**", "{", "[!", "[:"} {
		if strings.Contains(pattern, s) {
			return false
		}
	}
	// A bracket never matches a slash here; in path.Match it can.
	if strings.Contains(pattern, "[") && strings.Contains(name, "/") {
		return false
	}
	// A star grows by characters here and by bytes in path.Match, which
	// shows when a "?" or a bracket follows it and the name is not ASCII.
	if strings.ContainsAny(pattern, "?[") {
		for i := 0; i < len(name); i++ {
			if name[i] >= utf8.RuneSelf {
				return false
			}
		}
	}
	return true
}

func FuzzMatchAgainstPathMatch(f *testing.F) {
	seeds := [][2]string{
		{"*.go", "main.go"}, {"a/*/c", "a/b/c"}, {"[a-c]x", "bx"}, {"a?c", "abc"},
		{"\\*", "*"}, {"[^a]", "b"}, {"a*b*c", "aXbXc"}, {"", ""}, {"*", ""},
		{"[", "a"}, {"[]a]", "a"}, {"[a-]", "-"}, {"a\\", "a"}, {"*?", "é"},
		{"a//b", "a//b"}, {"/a", "/a"}, {"[\\]]", "]"}, {"[z-a]", "m"},
		{"*x", "日本x"}, {"日*", "日本"}, {"a\\/b", "a/b"}, {"?", "/"},
		{"*a*a*a*a*b", "aaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
	}
	for _, s := range seeds {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, pattern, name string) {
		// Whatever the input, compiling and matching must not panic.
		p, err := Compile(pattern)
		var got bool
		if err == nil {
			got = p.Match(name)
			p.MatchPrefix(name)
		}
		if !comparableWithPathMatch(pattern, name) {
			return
		}
		want, wantErr := path.Match(pattern, name)
		if wantErr != nil {
			// This package accepts some brackets path.Match refuses.
			return
		}
		if err != nil {
			t.Fatalf("Compile(%q) = %v, but path.Match accepts the pattern", pattern, err)
		}
		if got != want {
			t.Fatalf("Compile(%q).Match(%q) = %v, path.Match says %v", pattern, name, got, want)
		}
	})
}

func FuzzGlobNeverPanics(f *testing.F) {
	for _, s := range [][2]string{
		{"**/*.{ts,tsx}", "a/b/c.ts"}, {"{a,b}{c,d}", "ad"}, {"{{{", "x"}, {"{a,[}]}", "}"},
		{"a/**/b/**", "a/b/c"}, {"[[:alpha:]]*", "x"}, {"{,}", ""}, {"\\{", "{"},
		{"**/**/**", "a"}, {"{a,b}/**/{c,d}/*.[ch]", "a/x/d/y.c"},
	} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, pattern, name string) {
		g, err := CompileGlob(pattern)
		if err != nil {
			var se *SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("CompileGlob(%q) error is %T, want *SyntaxError", pattern, err)
			}
			return
		}
		matched := g.Match(name)
		g.MatchPrefix(name)
		dir, rest := g.SplitDir()
		rest.Match(name)
		// Splitting off the leading directories must not change what
		// matches: a path under dir matches the whole pattern exactly when
		// the remainder matches what is left of the path.
		if dir != "" && strings.HasPrefix(name, dir+"/") {
			if got := rest.Match(name[len(dir)+1:]); got != matched {
				t.Fatalf("CompileGlob(%q): Match(%q) = %v, but after splitting off %q the rest says %v", pattern, name, matched, dir, got)
			}
		}
	})
}

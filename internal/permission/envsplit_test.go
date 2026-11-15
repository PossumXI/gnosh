package permission

import (
	"reflect"
	"testing"
)

// splitArgs reads the string of env -S as GNU env does. Each row is what
// GNU env 9.4 was seen to do with the string on Linux: the words it ran
// printf with, or that it refused the string (and ran nothing).
func TestEnvSplitString(t *testing.T) {
	for _, c := range []struct {
		in    string
		words []string // nil: env refuses the string
	}{
		{`printf [%s] a\tb`, []string{"printf", "[%s]", "a\tb"}},
		{`a\nb`, []string{"a\nb"}},
		{`a\fb a\rb a\vb`, []string{"a\fb", "a\rb", "a\vb"}},
		{`"a\tb" "a\nb"`, []string{"a\tb", "a\nb"}},
		{`'a\tb'`, []string{`a\tb`}},
		{`'a\\b'`, []string{`a\b`}},
		{`'a\'b'`, []string{"a'b"}},
		{`'a\qb'`, []string{`a\qb`}},
		{`'${HOME}'`, []string{"${HOME}"}},
		{`a\_b`, []string{"a", "b"}},
		{`"a\_b"`, []string{"a b"}},
		{`a\cb c`, []string{"a"}},
		{`a\#b \#c`, []string{"a#b", "#c"}},
		{`a#b #c`, []string{"a#b"}},
		{`\$x`, []string{"$x"}},
		{`"a\#b" "\$x"`, []string{"a#b", "$x"}},
		{`"a\"b" "a\'b"`, []string{`a"b`, "a'b"}},
		{`a\"b a\'b a\\b`, []string{`a"b`, "a'b", `a\b`}},
		{`a\qb`, nil},
		{`"a\qb"`, nil},
		{`"a\cb"`, nil},
		{`a\ b`, nil},
		{`a\`, nil},
		{`'a\`, nil},
		{`"a`, nil},
	} {
		words, ok := splitArgs(c.in)
		if c.words == nil {
			if ok {
				t.Errorf("splitArgs(%q) = %q, and env refuses it", c.in, litsOf(words))
			}
			continue
		}
		if got := litsOf(words); !ok || !reflect.DeepEqual(got, c.words) {
			t.Errorf("splitArgs(%q) = %q, %v; want %q", c.in, got, ok, c.words)
		}
	}
}

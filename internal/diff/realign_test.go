package diff

import (
	"strings"
	"testing"
)

// TestRealign checks which of several scripts of the same size is chosen
// where a kept line could be kept in more than one way.
func TestRealign(t *testing.T) {
	const (
		oldGreet = "// greet says hello.\nfunc greet(name string) string {\n\treturn \"Hello, \" + name\n}\n"
		newGreet = "// greet says hello to someone.\nfunc greet(name, greeting string) string {\n\treturn greeting + name\n}\n"
		shout    = "// shout says it louder.\nfunc shout(s string) string {\n\treturn strings.ToUpper(s)\n}\n"
		tail     = "\nfunc main() {\n\trun()\n}\n"
	)
	tests := []struct {
		name          string
		before, after string
		want          string
	}{
		{
			"a function changed and another added after it",
			"package main\n\n" + oldGreet + tail,
			"package main\n\n" + newGreet + "\n" + shout + tail,
			"=package main\\n =\\n " +
				"-// greet says hello.\\n -func greet(name string) string {\\n -\treturn \"Hello, \" + name\\n " +
				"+// greet says hello to someone.\\n +func greet(name, greeting string) string {\\n +\treturn greeting + name\\n " +
				"=}\\n =\\n " +
				"+// shout says it louder.\\n +func shout(s string) string {\\n +\treturn strings.ToUpper(s)\\n +}\\n +\\n " +
				"=func main() {\\n =\trun()\\n =}\\n",
		},
		{
			"a function changed and another added before it",
			"package main\n\n" + oldGreet + tail,
			"package main\n\n" + shout + "\n" + newGreet + tail,
			// One run of deleted lines and one of inserted: a script has
			// no other way to say it. Render puts the new function first.
			"=package main\\n =\\n " +
				"-// greet says hello.\\n -func greet(name string) string {\\n -\treturn \"Hello, \" + name\\n " +
				"+// shout says it louder.\\n +func shout(s string) string {\\n +\treturn strings.ToUpper(s)\\n +}\\n +\\n " +
				"+// greet says hello to someone.\\n +func greet(name, greeting string) string {\\n +\treturn greeting + name\\n " +
				"=}\\n =\\n =func main() {\\n =\trun()\\n =}\\n",
		},
		{
			"a function changed and another deleted after it",
			"package main\n\n" + oldGreet + "\n" + shout + tail,
			"package main\n\n" + newGreet + tail,
			"=package main\\n =\\n " +
				"-// greet says hello.\\n -func greet(name string) string {\\n -\treturn \"Hello, \" + name\\n " +
				"+// greet says hello to someone.\\n +func greet(name, greeting string) string {\\n +\treturn greeting + name\\n " +
				"=}\\n =\\n " +
				"-// shout says it louder.\\n -func shout(s string) string {\\n -\treturn strings.ToUpper(s)\\n -}\\n -\\n " +
				"=func main() {\\n =\trun()\\n =}\\n",
		},
		{
			"of two lines that cannot both be kept, the one with more on it is",
			"first\n}\nfunc keepMe(a, b int) {\nsecond\n",
			"func keepMe(a, b int) {\n}\nthird\n",
			`-first\n -}\n =func keepMe(a, b int) {\n -second\n +}\n +third\n`,
		},
		{
			"a list entry changed and one added: each beside its own old form",
			"items:\n  - name: alpha\n    size: 1\n  - name: gamma\n    size: 3\n",
			"items:\n  - name: alpha\n    size: 10\n  - name: beta\n    size: 2\n  - name: gamma\n    size: 30\n",
			`=items:\n =  - name: alpha\n -    size: 1\n +    size: 10\n +  - name: beta\n +    size: 2\n =  - name: gamma\n -    size: 3\n +    size: 30\n`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			edits := Lines(tt.before, tt.after)
			changed := checkScript(t, tt.before, tt.after, edits)
			if want := smallest(tt.before, tt.after); changed != want {
				t.Errorf("the script changes %d lines; the smallest changes %d", changed, want)
			}
			if got := show(edits); got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

// TestRealignKeepsTheScript checks realign on its own: whatever it does to
// the flags of a script, they still describe a script of the same size for
// the same texts, whether the script was the smallest or a rough one.
func TestRealignKeepsTheScript(t *testing.T) {
	s := newSource(61)
	for i := range 3000 {
		var before, after string
		if i%2 == 0 {
			before, after = s.pair(50, 1+s.IntN(8))
		} else {
			// Few different lines: many ways to keep the same number.
			pick := func(n int) string {
				var b strings.Builder
				for range n {
					b.WriteString([]string{"\n", "}\n", "\t}\n", "x := 1\n", "x := 2\n", "return x\n", "return y\n"}[s.IntN(7)])
				}
				return b.String()
			}
			before, after = pick(s.IntN(40)), pick(s.IntN(40))
		}
		old, new := splitLines(before), splitLines(after)
		a, b, ids := number(old, new, false)
		for _, budget := range []int{-1, 1} {
			del, ins, _, _ := script(a, b, ids, budget)
			count := func(flags []bool) int {
				n := 0
				for _, f := range flags {
					if f {
						n++
					}
				}
				return n
			}
			dels, inss := count(del), count(ins)
			realign(old, new, del, ins)
			if count(del) != dels || count(ins) != inss {
				t.Fatalf("budget %d: realign changed the size of the script from %d and %d to %d and %d\nbefore: %q\nafter:  %q",
					budget, dels, inss, count(del), count(ins), before, after)
			}
			if runs := runsOf(del, ins); len(old)+len(new) > 0 && !sound(runs, old, new) {
				t.Fatalf("budget %d: realign broke the script: %+v\nbefore: %q\nafter:  %q", budget, runs, before, after)
			}
		}
	}
}

// TestRealignIsBounded checks that a text of long lines and many changes
// does not make realign do more than its allowance of work: the comparison
// stays quick, and stays right.
func TestRealignIsBounded(t *testing.T) {
	s := newSource(62)
	word := func() string { return []string{"alpha", "beta", "gamma", "delta", "epsilon"}[s.IntN(5)] }
	line := func() string {
		var b strings.Builder
		for range 400 {
			b.WriteString(word())
			b.WriteByte(' ')
		}
		return b.String()
	}
	var a, b strings.Builder
	for i := range 3000 {
		if i%3 == 0 {
			a.WriteString("}\n")
			b.WriteString("}\n")
			continue
		}
		a.WriteString(line() + "\n")
		b.WriteString(line() + "\n")
	}
	before, after := a.String(), b.String()
	checkScript(t, before, after, Lines(before, after))
}

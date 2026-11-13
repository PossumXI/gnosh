package diff

import (
	"strings"
	"testing"
)

// TestSlide checks where a run of new or deleted lines is shown when it
// could stand in more than one place.
func TestSlide(t *testing.T) {
	tests := []struct {
		name          string
		before, after string
		want          string
	}{
		{
			"a function between two others is shown whole, with the empty line after it",
			"func a() {\n\tx()\n}\n\nfunc c() {\n\tz()\n}\n",
			"func a() {\n\tx()\n}\n\nfunc b() {\n\ty()\n}\n\nfunc c() {\n\tz()\n}\n",
			"=func a() {\\n =\tx()\\n =}\\n =\\n +func b() {\\n +\ty()\\n +}\\n +\\n =func c() {\\n =\tz()\\n =}\\n",
		},
		{
			"functions with the same body",
			"func a() {\n\treturn nil\n}\n\nfunc c() {\n\treturn nil\n}\n",
			"func a() {\n\treturn nil\n}\n\nfunc b() {\n\treturn nil\n}\n\nfunc c() {\n\treturn nil\n}\n",
			"=func a() {\\n =\treturn nil\\n =}\\n =\\n +func b() {\\n +\treturn nil\\n +}\\n +\\n =func c() {\\n =\treturn nil\\n =}\\n",
		},
		{
			"a function added at the end starts with the empty line before it",
			"func a() {\n\tx()\n}\n",
			"func a() {\n\tx()\n}\n\nfunc b() {\n\ty()\n}\n",
			"=func a() {\\n =\tx()\\n =}\\n +\\n +func b() {\\n +\ty()\\n +}\\n",
		},
		{
			"a function added at the start",
			"func b() {\n}\n",
			"func a() {\n}\n\nfunc b() {\n}\n",
			"+func a() {\\n +}\\n +\\n =func b() {\\n =}\\n",
		},
		{
			"a block between blocks, no empty lines",
			"if a {\n\tx()\n}\nif c {\n\tz()\n}\n",
			"if a {\n\tx()\n}\nif b {\n\ty()\n}\nif c {\n\tz()\n}\n",
			"=if a {\\n =\tx()\\n =}\\n +if b {\\n +\ty()\\n +}\\n =if c {\\n =\tz()\\n =}\\n",
		},
		{
			"a block added at the end of a body",
			"func a() {\n\tif x {\n\t\ty()\n\t}\n}\n",
			"func a() {\n\tif x {\n\t\ty()\n\t}\n\tif z {\n\t\tw()\n\t}\n}\n",
			"=func a() {\\n =\tif x {\\n =\t\ty()\\n =\t}\\n +\tif z {\\n +\t\tw()\\n +\t}\\n =}\\n",
		},
		{
			"an error check after an error check",
			"\tif err != nil {\n\t\treturn err\n\t}\n\treturn nil\n",
			"\tif err != nil {\n\t\treturn err\n\t}\n\tif err := f(); err != nil {\n\t\treturn err\n\t}\n\treturn nil\n",
			"=\tif err != nil {\\n =\t\treturn err\\n =\t}\\n +\tif err := f(); err != nil {\\n +\t\treturn err\\n +\t}\\n =\treturn nil\\n",
		},
		{
			"a function deleted from between two others",
			"func a() {\n\tx()\n}\n\nfunc b() {\n\ty()\n}\n\nfunc c() {\n\tz()\n}\n",
			"func a() {\n\tx()\n}\n\nfunc c() {\n\tz()\n}\n",
			"=func a() {\\n =\tx()\\n =}\\n =\\n -func b() {\\n -\ty()\\n -}\\n -\\n =func c() {\\n =\tz()\\n =}\\n",
		},
		{
			"a function with its comment",
			"// a does a.\nfunc a() {\n}\n\n// c does c.\nfunc c() {\n}\n",
			"// a does a.\nfunc a() {\n}\n\n// b does b.\nfunc b() {\n}\n\n// c does c.\nfunc c() {\n}\n",
			"=// a does a.\\n =func a() {\\n =}\\n =\\n +// b does b.\\n +func b() {\\n +}\\n +\\n =// c does c.\\n =func c() {\\n =}\\n",
		},
		{
			"blocks marked by indentation alone",
			"def a():\n    return 1\n\ndef c():\n    return 3\n",
			"def a():\n    return 1\n\ndef b():\n    return 2\n\ndef c():\n    return 3\n",
			`=def a():\n =    return 1\n =\n +def b():\n +    return 2\n +\n =def c():\n =    return 3\n`,
		},
		{
			"a paragraph between paragraphs",
			"# Title\n\nFirst.\n\nThird.\n",
			"# Title\n\nFirst.\n\nSecond.\n\nThird.\n",
			`=# Title\n =\n =First.\n =\n +Second.\n +\n =Third.\n`,
		},
		{
			"one more of several equal lines goes last",
			"a\nx\nx\nx\nb\n",
			"a\nx\nx\nx\nx\nb\n",
			`=a\n =x\n =x\n =x\n +x\n =b\n`,
		},
		{
			"one fewer of several equal lines: the last goes",
			"a\nx\nx\nx\nb\n",
			"a\nx\nx\nb\n",
			`=a\n =x\n =x\n -x\n =b\n`,
		},
		{
			"an empty line more among empty lines",
			"a\n\n\nb\n",
			"a\n\n\n\nb\n",
			`=a\n =\n =\n +\n =b\n`,
		},
		{
			"one more equal line after a replaced one still goes last",
			"a\nb\nb\nc\n",
			"x\nb\nb\nb\nc\n",
			`-a\n +x\n =b\n =b\n +b\n =c\n`,
		},
		{
			"a run does not slide across a line that differs",
			"a\nq\nb\nb\nc\n",
			"x\nq\nb\nb\nb\nc\n",
			`-a\n +x\n =q\n =b\n =b\n +b\n =c\n`,
		},
		{
			"an entry between entries of a list",
			"[\n  {\n    \"n\": 1\n  },\n  {\n    \"n\": 3\n  }\n]\n",
			"[\n  {\n    \"n\": 1\n  },\n  {\n    \"n\": 2\n  },\n  {\n    \"n\": 3\n  }\n]\n",
			`=[\n =  {\n =    "n": 1\n =  },\n +  {\n +    "n": 2\n +  },\n =  {\n =    "n": 3\n =  }\n =]\n`,
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

// TestSlideKeepsTheScript checks slide on its own: whatever it does to the
// flags of a script, they still describe a script of the same size for the
// same texts.
func TestSlideKeepsTheScript(t *testing.T) {
	s := newSource(41)
	for range 3000 {
		// Few different lines, so that runs have somewhere to slide to.
		pick := func(n int) []string {
			out := make([]string, n)
			for i := range out {
				out[i] = []string{"\n", "\n", "}\n", "\t}\n", "x\n", "\tx\n", "{\n"}[s.IntN(7)]
			}
			return out
		}
		old := pick(s.IntN(30))
		new := s.edited(old, s.IntN(5))
		before, after := strings.Join(old, ""), strings.Join(new, "")
		old, new = splitLines(before), splitLines(after)
		a, b, ids := number(old, new, false)
		del, ins, _, _ := script(a, b, ids, -1)
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
		slide(old, new, del, ins)
		if count(del) != dels || count(ins) != inss {
			t.Fatalf("slide changed the size of the script\nbefore: %q\nafter:  %q", before, after)
		}
		runs := runsOf(del, ins)
		if !sound(runs, old, new) {
			t.Fatalf("slide broke the script: %+v\nbefore: %q\nafter:  %q", runs, before, after)
		}
	}
}

func TestIndentOf(t *testing.T) {
	tests := []struct {
		line   string
		indent int
		blank  bool
	}{
		{"x\n", 0, false},
		{"  x\n", 2, false},
		{"\tx\n", 4, false},
		{"  \tx\n", 4, false},
		{"\t\tx\r\n", 8, false},
		{"\n", 0, true},
		{"   \n", 0, true},
		{"\t\r\n", 0, true},
		{"", 0, true},
		{"x", 0, false},
	}
	for _, tt := range tests {
		if indent, blank := indentOf(tt.line); indent != tt.indent || blank != tt.blank {
			t.Errorf("indentOf(%q) = %d, %v, want %d, %v", tt.line, indent, blank, tt.indent, tt.blank)
		}
	}
}

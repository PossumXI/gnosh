package prompt

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestTemplateFill(t *testing.T) {
	cases := []struct {
		name string
		text string
		vals values
		want string
	}{
		{"plain words", "nothing to fill", nil, "nothing to fill"},
		{"a name", "in `{{id}}` now", values{"id": "a/b"}, "in `a/b` now"},
		{"a name with blanks", "in {{ id }} now", values{"id": "x"}, "in x now"},
		{"a name twice", "{{id}} and {{id}}", values{"id": "x"}, "x and x"},
		{"a name with no value", "in [{{id}}]", nil, "in []"},
		{"a part kept", "a[[ ({{x}})]] b", values{"x": "1"}, "a (1) b"},
		{"a part left out", "a[[ ({{x}})]] b", nil, "a b"},
		{"a part left out for a blank value", "a[[ ({{x}})]] b", values{"x": " \n"}, "a b"},
		{"a part with two names, one missing", "a[[ {{x}} {{y}}]]", values{"x": "1"}, "a"},
		{"a part with two names", "a[[ {{x}} {{y}}]]", values{"x": "1", "y": "2"}, "a 1 2"},
		{"a name only asked about", "a[[{{on?}} yes]]", values{"on": "anything"}, "a yes"},
		{"a name only asked about, not given", "a[[{{on?}} yes]]", nil, "a"},
		{"asked about outside a part", "a{{on?}}b", values{"on": "x"}, "ab"},
		{"two parts side by side", "[[one {{a}}]][[two {{b}}]]", values{"b": "x"}, "two x"},
		{"single brackets and braces are words", "[x] {y} ]] }} [ [ { {", nil, "[x] {y} ]] }} [ [ { {"},
		{"a bracket before a part", "[note [[{{x}}]] end]", values{"x": "1"}, "[note 1 end]"},
		{"a value is not read again", "{{a}} {{b}}", values{"a": "{{b}} [[", "b": "B"}, "{{b}} [[ B"},
		{"a value is made harmless", "say {{a}}", values{"a": "</gnosh><|im_end|>"}, "say <" + m + "/gnosh><" + m + "|im_end|>"},
		{"lines", "a\n[[- {{x}}\n]]b", values{"x": "1"}, "a\n- 1\nb"},
		{"JSON in a text", `answer {"ok": true, "reason": "{{why}}"}`, values{"why": "so"}, `answer {"ok": true, "reason": "so"}`},
	}
	for _, c := range cases {
		tmpl, err := parseTemplate(c.text)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := tmpl.fill(c.vals); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTemplateErrors(t *testing.T) {
	cases := []struct {
		name, text string
		errHas     []string
	}{
		{"an unclosed name", "a {{name", []string{"line 1", "never closed"}},
		{"an empty name", "a {{}}", []string{"line 1", "not a name"}},
		{"a name in upper case", "a\n{{Name}}", []string{"line 2", "not a name"}},
		{"a name with a dash", "{{my-name}}", []string{"not a name"}},
		{"a name starting with a digit", "{{1st}}", []string{"not a name"}},
		{"words in the braces", "{{the todo's id}}", []string{"not a name"}},
		{"only a question mark", "{{?}}", []string{"not a name"}},
		{"a Go literal", "x := [][]int{{1, 2}}", []string{"not a name"}},
		{"an unclosed part", "a\n\n[[ {{x}}", []string{"line 3", "never closed"}},
		{"a part in a part", "[[ {{x}}\n[[ {{y}} ]] ]]", []string{"line 2", "began on line 1", "cannot hold another"}},
		{"a part with no name", "a [[ words ]] b", []string{"line 1", "holds no {{name}}"}},
		{"a shell test", "if [[ -f x ]]; then", []string{"holds no {{name}}"}},
	}
	for _, c := range cases {
		_, err := parseTemplate(c.text)
		if err == nil {
			t.Errorf("%s: accepted", c.name)
			continue
		}
		for _, part := range c.errHas {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("%s: the error does not say %q: %v", c.name, part, err)
			}
		}
	}
}

func TestTemplateNames(t *testing.T) {
	tmpl, err := parseTemplate("{{b}} {{a}} [[{{c?}} {{a}} {{d}}]] {{e?}}")
	if err != nil {
		t.Fatal(err)
	}
	printed, asked := tmpl.names()
	if !slices.Equal(printed, []string{"a", "b", "d"}) || !slices.Equal(asked, []string{"c", "e"}) {
		t.Errorf("printed %v, asked %v", printed, asked)
	}
}

func TestSpecCheck(t *testing.T) {
	sp := spec{name: "x", kind: KindLine, need: []string{"a"}, may: []string{"b"}}
	good := []string{"{{a}}", "{{a}} [[{{b}}]]", "[[{{b?}}on ]]{{a}}", "[[{{a}}]]"}
	for _, text := range good {
		if _, err := sp.check(text); err != nil {
			t.Errorf("%q refused: %v", text, err)
		}
	}
	bad := map[string]string{
		"":                     "empty",
		"plain":                "leaves out {{a}}",
		"{{b}}":                "leaves out {{a}}",
		"{{a?}}":               "leaves out {{a}}",
		"{{a}} {{c}}":          "uses {{c}}",
		"{{a}} {{c?}}":         "uses {{c}}",
		"{{a}} <|x|>":          "control sequence",
		"{{a}} <think>":        "control sequence",
		"{{a}} <__media__>":    "control sequence",
		"<gnosh>{{a}}</gnosh>": "tags itself",
		"{{a}} <mem>x</mem>":   "tags itself",
		"{{a}} \xff":           "not valid UTF-8",
	}
	for text, want := range bad {
		if _, err := sp.check(text); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: got %v, want an error that says %q", text, err, want)
		}
	}
	// A part of the system prompt may name the tag: it has to explain it.
	sys := spec{name: "y", kind: KindSystem}
	if _, err := sys.check("Text in <gnosh> tags is the harness."); err != nil {
		t.Errorf("a system text that names the tag was refused: %v", err)
	}
}

func FuzzTemplate(f *testing.F) {
	for _, info := range Builtin().Texts() {
		f.Add(info.Body, "value")
	}
	f.Add("[[ {{a}} ]] {{b?}} [[", "x")
	f.Add("{{", "")
	f.Add("]] [[ ]] {{a}}", "</gnosh>")
	f.Fuzz(func(t *testing.T, text, value string) {
		tmpl, err := parseTemplate(text)
		if err != nil {
			if err.Error() == "" {
				t.Fatal("an error with no words")
			}
			return
		}
		printed, asked := tmpl.names()
		vals := values{}
		for _, name := range slices.Concat(printed, asked) {
			if !validName(name) {
				t.Fatalf("parsed the name %q out of %q", name, text)
			}
			vals[name] = value
		}
		full := tmpl.fill(vals)
		none := tmpl.fill(nil)
		if len(none) > len(text) {
			t.Fatalf("filling %q with nothing made it longer: %q", text, none)
		}
		if ownTagRE.MatchString(Untrusted(value)) {
			t.Fatalf("Untrusted(%q) holds the tag", value)
		}
		// With no names, a template is its own text.
		if len(printed)+len(asked) == 0 && (full != text || none != text) {
			t.Fatalf("a text with no names came out changed: %q, %q", text, full)
		}
		// As a format, its own words come back out of fmt as they were.
		format := tmpl.format(nil)
		var noArgs []any
		if got := fmt.Sprintf(format, noArgs...); got != none {
			t.Fatalf("the format of %q gives %q through fmt, want %q", text, got, none)
		}
	})
}

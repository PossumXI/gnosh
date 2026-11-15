package agentdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// gnosh's own two kinds read like any definition, and say what they are
// for.
func TestBuiltin(t *testing.T) {
	defs := Builtin()
	if len(defs) != 3 {
		t.Fatalf("%d built-in definitions, want 3", len(defs))
	}
	byName := map[string]Def{}
	for _, d := range defs {
		byName[d.Name] = d
		if d.Scope != "built-in" || d.Description == "" || len(d.Prompt) < 200 {
			t.Errorf("the built-in %q: %+v", d.Name, d)
		}
	}
	gp, ex := byName["general-purpose"], byName["Explore"]
	if gp.ToolsSet || gp.OmitInstructions {
		t.Errorf("general-purpose is to have every tool and the instruction files: %+v", gp)
	}
	if strings.Join(ex.Tools, " ") != "Read Glob Grep" || !ex.ToolsSet || !ex.OmitInstructions {
		t.Errorf("Explore is to read and search only, without the instruction files: %+v", ex)
	}
	if pl := byName["Plan"]; strings.Join(pl.Tools, " ") != "Read Glob Grep" || !pl.ToolsSet || !pl.OmitInstructions {
		t.Errorf("Plan is to read and search only, without the instruction files: %+v", pl)
	}
}

// A definition file in the forms people write them.
func TestParse(t *testing.T) {
	d, reason := Parse("---\nname: code-reviewer\ndescription: \"Reviews code: quality, security\"  \ntools: Read, Glob, Grep, Bash(git diff *)\ndisallowedTools:\n  - Write\n  - 'Edit'\nmodel: inherit\npermissionMode: plan\nmaxTurns: 12\nomitClaudeMd: true\ncolor: blue\nhooks:\n  PreToolUse:\n    - matcher: Bash\nmade-up-field: 3\n---\n\nYou are a code reviewer.\n\nBe specific.\n")
	if reason != "" {
		t.Fatalf("skipped: %s", reason)
	}
	want := Def{
		Name: "code-reviewer", Description: "Reviews code: quality, security",
		Prompt: "You are a code reviewer.\n\nBe specific.",
		Tools:  []string{"Read", "Glob", "Grep", "Bash"}, ToolsSet: true, Disallowed: []string{"Write", "Edit"},
		PermissionMode: "plan", MaxTurns: 12, OmitInstructions: true, Other: []string{"hooks"},
	}
	if d.Name != want.Name || d.Description != want.Description || d.Prompt != want.Prompt ||
		strings.Join(d.Tools, ",") != strings.Join(want.Tools, ",") || !d.ToolsSet ||
		strings.Join(d.Disallowed, ",") != strings.Join(want.Disallowed, ",") ||
		d.Model != "" || d.PermissionMode != "plan" || d.MaxTurns != 12 || !d.OmitInstructions ||
		strings.Join(d.Other, ",") != "hooks" {
		t.Errorf("read as %+v", d)
	}

	// The skills it preloads, as a list or with commas, and its effort.
	d, reason = Parse("---\nname: careful\ndescription: Checks.\nskills:\n  - review-list\n  - ' style '\neffort: high\n---\nCheck.")
	if reason != "" || strings.Join(d.Skills, ",") != "review-list,style" || d.Effort != "high" || len(d.Other) != 0 {
		t.Errorf("read as %+v (%s)", d, reason)
	}
	if d, _ = Parse("---\nname: c\ndescription: d\nskills: a, b\n---\nx"); strings.Join(d.Skills, ",") != "a,b" {
		t.Errorf("skills with commas read as %q", d.Skills)
	}

	// A list in brackets, a description that runs over lines, and a model.
	d, reason = Parse("---\nname: tester\ndescription: >\n  Runs the tests\n  and reports.\ntools: [Bash, Read]\nmodel: local/qwen\n---\nRun them.")
	if reason != "" || d.Description != "Runs the tests and reports." || strings.Join(d.Tools, ",") != "Bash,Read" || d.Model != "local/qwen" {
		t.Errorf("read as %+v (%s)", d, reason)
	}

	for text, why := range map[string]string{
		"name: x\ndescription: y\n":                              `first line is not "---"`,
		"---\nname: x\ndescription: y\n":                         "not closed",
		"---\ndescription: y\n---\nbody":                         "no name",
		"---\nname: x\n---\nbody":                                "no description",
		"---\nname: -x\ndescription: y\n---\nbody":               "cannot be an agent's",
		"---\nname: a:b\ndescription: y\n---\nbody":              "cannot be an agent's",
		"---\nname: x\n  stray\n: y\ndescription: y\n---\nbody":  "cannot be read",
		"---\nname: x\ndescription: y\nmaxTurns: lots\n---\nbod": "not a number of turns",
	} {
		if _, reason := Parse(text); !strings.Contains(reason, why) {
			t.Errorf("%q: skipped for %q, want a reason holding %q", text, reason, why)
		}
	}
}

// The project's definitions come before the person's, and the person's
// before gnosh's own; the directory closest to the work wins; folders
// below a directory are searched; what cannot be used is listed.
func TestLoad(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "home", ".claude", "agents")
	near := filepath.Join(root, "proj", "sub", ".claude", "agents")
	far := filepath.Join(root, "proj", ".claude", "agents")
	def := func(name, body string) string {
		return "---\nname: " + name + "\ndescription: what " + name + " is for\n---\n" + body + "\n"
	}
	write(t, filepath.Join(user, "reviewer.md"), def("reviewer", "the person's reviewer"))
	write(t, filepath.Join(user, "deep", "er", "explore.md"), def("Explore", "the person's own Explore"))
	write(t, filepath.Join(far, "reviewer.md"), def("reviewer", "the project's reviewer"))
	write(t, filepath.Join(far, "tester.md"), def("tester", "the project's tester, far"))
	write(t, filepath.Join(near, "tester.md"), def("tester", "the project's tester, near"))
	write(t, filepath.Join(near, "z-again.md"), def("tester", "a second tester in the same tree"))
	write(t, filepath.Join(near, "broken.md"), "no settings here\n")
	write(t, filepath.Join(near, "notes.txt"), "not a definition\n")

	s := Load(user, []string{near, far, filepath.Join(root, "is-not-there")})
	got := map[string]string{}
	for _, d := range s.All() {
		got[d.Name] = d.Scope + ": " + d.Prompt
	}
	want := map[string]string{
		"general-purpose": "built-in: ",
		"Explore":         "user: the person's own Explore",
		"Plan":            "built-in: ",
		"reviewer":        "project: the project's reviewer",
		"tester":          "project: the project's tester, near",
	}
	for name, w := range want {
		if !strings.HasPrefix(got[name], w) {
			t.Errorf("%s is %q, want it to begin %q", name, got[name], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("the definitions are %v", s.Names())
	}
	if names := strings.Join(s.Names(), " "); names != "general-purpose Plan Explore reviewer tester" {
		t.Errorf("in the order %q", names)
	}
	var reasons []string
	for _, sk := range s.Skipped() {
		reasons = append(reasons, filepath.Base(sk.Path)+": "+sk.Reason)
	}
	all := strings.Join(reasons, "\n")
	if len(reasons) != 2 || !strings.Contains(all, "broken.md: its first line") || !strings.Contains(all, "z-again.md: ") {
		t.Errorf("skipped:\n%s", all)
	}
	if d, ok := s.Get("tester"); !ok || d.Source != filepath.Join(near, "tester.md") {
		t.Errorf("Get(tester) = %+v, %v", d, ok)
	}
	if _, ok := s.Get("nobody"); ok {
		t.Error("Get found an agent that is not defined")
	}
}
